# setupmac

A Go CLI tool that configures a Mac for headless rack operation in a lab
environment. Runs as root, applies settings in order, and prints a structured
pass/fail report for each step.

Supports macOS 10.13 (High Sierra) through 15 (Sequoia) on both Intel
(Mac Mini 2011–2018) and Apple Silicon (Mac Mini M1+).

## Quick start

```bash
curl -fsSL https://raw.githubusercontent.com/wernerstrydom/setupmac/main/run.sh \
  | sudo bash -s -- --username <username>
```

For a dry run that shows what would happen without making any changes:

```bash
curl -fsSL https://raw.githubusercontent.com/wernerstrydom/setupmac/main/run.sh \
  | sudo bash -s -- --dry-run
```

> **Note**: Steps that prompt for a password (FileVault, auto-login) read from
> `/dev/tty` so they work through a curl pipe. If you prefer a plain download:
>
> ```bash
> curl -fsSL https://raw.githubusercontent.com/wernerstrydom/setupmac/main/run.sh \
>   -o /tmp/run.sh && sudo bash /tmp/run.sh --username <username>
> ```

## What it configures

| Step | What it does |
|---|---|
| Power management | Disables all sleep modes; enables auto-restart and Wake-on-LAN |
| Bluetooth Setup Assistant | Suppresses the keyboard/mouse pairing prompt at the login window |
| Universal Control | Disabled on macOS 12.3+; skipped on older hardware |
| Screen saver | Sets login window idle time to 0 to prevent lockout |
| Guest account | Disables Guest login at the login window |
| ARD / Remote Management | On macOS < 12.1: activates Remote Desktop via `kickstart` for all local users with full privileges. On macOS 12.1+: Apple restricts `kickstart`; writes VNC preference keys and warns the operator to enable Remote Management via System Settings > General > Sharing or MDM |
| FileVault | Disables encryption (required for auto-login to function) |
| Auto-login | Configures automatic login for a named user |
| Homebrew (multi-user) | Installs Homebrew under a dedicated `homebrew_owner` service account so any local user can run `brew` transparently |
| Verification | Re-reads every setting and reports current state independently |

## Flags

| Flag | Description |
|---|---|
| `--username <user>` | Username to configure for auto-login (omit to skip) |
| `--vnc-password <pass>` | VNC password for ARD (omit to skip) |
| `--skip-filevault` | Skip the FileVault disable step |
| `--dry-run` | Print commands without executing anything |

## Homebrew multi-user setup

Homebrew is installed under a dedicated `homebrew_owner` service account.
A wrapper script at `/opt/macsetup/brew` delegates every `brew` invocation to
that account transparently — any local user can type `brew install <package>`
without any extra configuration.

```
/opt/macsetup/brew  →  sudo -H -u homebrew_owner /real/path/to/brew "$@"
```

`/opt/macsetup` is placed first in `PATH` by appending an `export PATH` line
to `/etc/zprofile` after macOS's `path_helper` call. This ensures the wrapper
takes precedence over the real `brew` binary on both Intel (`/usr/local/bin`)
and Apple Silicon (`/opt/homebrew/bin`). `/etc/paths.d/00-macsetup` is also
written as a belt-and-suspenders measure.

The sudoers drop-in at `/etc/sudoers.d/homebrew-multiuser` grants:

| Rule | Purpose |
|---|---|
| `%staff → homebrew_owner NOPASSWD: /brew` | Any local user can run `brew` without a password |
| `homebrew_owner → ALL NOPASSWD: ALL` | Homebrew installer calls `sudo` internally |
| `root → homebrew_owner NOPASSWD: ALL` | setupmac runs the installer as `homebrew_owner` |

The Guest account is explicitly denied regardless of group membership.

> **After setup**: PATH changes take effect in new shells only. Run
> `source /etc/zprofile` or open a new terminal before using `brew`.

## Building from source

Requires Go 1.25+.

```bash
git clone https://github.com/wernerstrydom/setupmac.git
cd setupmac
make build          # produces bin/setupmac
make install-system # installs to /usr/local/bin (requires sudo)
```

## Development

Install pre-commit hooks (required before contributing):

```bash
pip install pre-commit
brew install golangci-lint vale
go install golang.org/x/vuln/cmd/govulncheck@latest
pre-commit install
vale sync
```

Hooks run on every commit: `golangci-lint`, `govulncheck`, `go mod tidy`,
dependency updates on `go.mod` changes, `yamllint`, and Vale markdown lint.

### Releasing

Push a version tag to trigger the release workflow. Binaries for both
`darwin/amd64` (Intel) and `darwin/arm64` (Apple Silicon) are built and
published automatically with SHA-256 checksums.

```bash
git tag v1.3.0
git push origin v1.3.0
```
