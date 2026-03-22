package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/wstrydom/setupmac/internal/macos"
)

const (
	brewUserName    = "homebrew_owner"
	brewUserDesc    = "Homebrew Dedicated Owner"
	sudoersPath     = "/etc/sudoers.d/homebrew-multiuser"
	brewWrapperDir  = "/opt/macsetup"
	brewWrapperPath = "/opt/macsetup/brew"
	// pathsDPath is read by macOS path_helper, adding brewWrapperDir to PATH
	// for all shells that source /etc/zprofile (the default on macOS).
	pathsDPath = "/etc/paths.d/00-macsetup"
	zshenvPath = "/etc/zshenv"
	zshenvMark = "# Added by setupmac — brew wrapper path"
)

// BrewPrefix returns the Homebrew installation prefix for the running architecture.
func BrewPrefix() string {
	if runtime.GOARCH == "arm64" {
		return "/opt/homebrew"
	}
	return "/usr/local"
}

func brewBin() string {
	return BrewPrefix() + "/bin/brew"
}

// SetupHomebrew creates a dedicated homebrew_owner service account, installs
// Homebrew under it, writes a sudoers drop-in so any admin user can run brew
// without a password prompt, and places a wrapper at /opt/macsetup/brew that
// transparently delegates to homebrew_owner on both Intel and Apple Silicon.
//
// The wrapper calls the real brew by its fully qualified path, so it works
// on both architectures without any circular reference.
func SetupHomebrew(r *Runner, ver macos.Version) []Result {
	prefix := BrewPrefix()
	bin := brewBin()

	var results []Result

	res := createBrewUser(r, ver)
	results = append(results, res)
	if res.Status == Fail {
		return results
	}

	// Always run, even if createBrewUser was skipped — the user may have been
	// created in a previous run without the -admin flag.
	res = ensureBrewUserAdmin(r)
	results = append(results, res)
	if res.Status == Fail {
		return results
	}

	res = writeBrewSudoers(r, bin)
	results = append(results, res)
	if res.Status == Fail {
		return results
	}

	results = append(results, installHomebrew(r, prefix, bin)...)
	results = append(results, createBrewWrapper(r, bin))
	results = append(results, injectBrewPath(r))

	return results
}

// createBrewUser creates a hidden service account that owns the Homebrew
// installation. If the account already exists the step is skipped.
func createBrewUser(r *Runner, ver macos.Version) Result {
	if _, err := user.Lookup(brewUserName); err == nil {
		return SkipResult("brew-user", fmt.Sprintf("user %q already exists", brewUserName))
	}

	// Generate a random password using crypto/rand. It is never used
	// interactively — it only satisfies the macOS account creation requirement.
	password, err := randomBase64(16)
	if err != nil {
		return FailResult("brew-user", "failed to generate random password", err)
	}

	homeDir := "/var/" + brewUserName

	// -home sets NFSHomeDirectory directly, avoiding a separate dscl call
	// that would fail with eDSPermissionError on macOS 13+.
	// -admin is required so the Homebrew installer can verify sudo access.
	if out, err := r.Run("sysadminctl",
		"-addUser", brewUserName,
		"-fullName", brewUserDesc,
		"-password", password,
		"-home", homeDir,
		"-admin",
	); err != nil {
		return FailResult("brew-user", out, err)
	}

	// macOS 13+ (Ventura and later) rejects dscl writes to IsHidden with
	// eDSPermissionError. Use the loginwindow plist instead, which works on
	// all supported macOS versions. On older macOS, dscl IsHidden is still
	// the canonical mechanism.
	if ver.AtLeast(13, 0) {
		if out, err := r.Run("defaults", "write",
			"/Library/Preferences/com.apple.loginwindow",
			"HiddenUsersList", "-array-add", brewUserName); err != nil {
			return FailResult("brew-user", out, err)
		}
	} else {
		if out, err := r.Run("dscl", ".", "-create",
			"/Users/"+brewUserName, "IsHidden", "1"); err != nil {
			return FailResult("brew-user", out, err)
		}
	}

	if !r.DryRun {
		if err := os.MkdirAll(homeDir, 0755); err != nil {
			return FailResult("brew-user",
				fmt.Sprintf("failed to create home directory %s: %v", homeDir, err), err)
		}
		if err := chownPath(homeDir, brewUserName, "staff"); err != nil {
			return FailResult("brew-user",
				fmt.Sprintf("failed to set ownership of %s: %v", homeDir, err), err)
		}
	}

	return OKResult("brew-user", fmt.Sprintf("created hidden service user %q", brewUserName))
}

// ensureBrewUserAdmin adds homebrew_owner to the admin group if it is not
// already a member. Admin membership is required by the Homebrew installer,
// which validates sudo access before proceeding.
func ensureBrewUserAdmin(r *Runner) Result {
	// dseditgroup exits 0 when the user is already a member.
	if _, err := r.Read("dseditgroup", "-o", "checkmember", "-m", brewUserName, "admin"); err == nil {
		return SkipResult("brew-user-admin", fmt.Sprintf("user %q is already in the admin group", brewUserName))
	}

	out, err := r.Run("dseditgroup", "-o", "edit", "-a", brewUserName, "-t", "user", "admin")
	if err != nil {
		return FailResult("brew-user-admin", out, err)
	}
	return OKResult("brew-user-admin", fmt.Sprintf("added %q to admin group", brewUserName))
}

// writeBrewSudoers writes /etc/sudoers.d/homebrew-multiuser with three rules:
//
//  1. %staff → homebrew_owner, brew binary only (day-to-day use)
//  2. homebrew_owner → ALL, NOPASSWD: ALL (Homebrew installer calls sudo internally)
//  3. root → homebrew_owner, NOPASSWD: ALL (lets root invoke the installer as homebrew_owner)
//
// Rules 2 and 3 are broader than day-to-day needs but safe on a headless lab
// Mac: homebrew_owner has a random password and is not an interactive account.
// Guest is explicitly denied regardless of group membership (last-match-wins).
// The file is validated by visudo before being moved into place.
func writeBrewSudoers(r *Runner, brewBinPath string) Result {
	// sudo uses last-match-wins, so the Guest deny line must come after the
	// staff allow line to override it regardless of Guest's group membership.
	content := fmt.Sprintf(
		"# Passwordless delegation: all local users → %s, brew binary only\n"+
			"%%staff ALL=(%s) NOPASSWD: %s\n"+
			"# Guest account must never bypass authentication.\n"+
			"Guest  ALL=(%s) !ALL\n"+
			"# Allow %s to call sudo during Homebrew install and updates.\n"+
			"%s ALL=(ALL) NOPASSWD: ALL\n"+
			"# Allow root to invoke the Homebrew installer as %s.\n"+
			"root   ALL=(%s) NOPASSWD: ALL\n",
		brewUserName, brewUserName, brewBinPath, brewUserName,
		brewUserName, brewUserName,
		brewUserName, brewUserName,
	)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s:\n    %s", sudoersPath, content)
		return OKResult("brew-sudoers", fmt.Sprintf("would write %s (dry-run)", sudoersPath))
	}

	// Create the temp file inside /etc so os.Rename stays on the same filesystem.
	tmp, err := os.CreateTemp("/etc", ".homebrew-sudoers-*")
	if err != nil {
		return FailResult("brew-sudoers", "failed to create temp file in /etc", err)
	}
	tmpPath := tmp.Name()

	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return FailResult("brew-sudoers", "failed to write sudoers content", err)
	}
	tmp.Close()

	// Validate syntax before placing the file — a bad sudoers file can lock
	// out all sudo access on the machine.
	if out, err := r.Read("visudo", "-c", "-f", tmpPath); err != nil {
		return FailResult("brew-sudoers",
			fmt.Sprintf("visudo validation failed: %s", out), err)
	}

	// sudo requires 0440 root:wheel; os.Chown/os.Chmod avoid shelling out.
	if err := os.Chown(tmpPath, 0, 0); err != nil { // root:wheel = 0:0 on macOS
		return FailResult("brew-sudoers", "failed to set sudoers ownership", err)
	}
	if err := os.Chmod(tmpPath, 0440); err != nil {
		return FailResult("brew-sudoers", "failed to set sudoers permissions", err)
	}

	if err := os.Rename(tmpPath, sudoersPath); err != nil {
		return FailResult("brew-sudoers",
			fmt.Sprintf("failed to place %s: %v", sudoersPath, err), err)
	}

	committed = true
	return OKResult("brew-sudoers", fmt.Sprintf("wrote %s", sudoersPath))
}

// installHomebrew runs the Homebrew installer as homebrew_owner via su.
// On Apple Silicon it pre-creates /opt/homebrew so the installer can write
// into it without root. On Intel /usr/local already exists; we leave it alone
// to avoid changing ownership of unrelated tools.
func installHomebrew(r *Runner, prefix, bin string) []Result {
	var results []Result

	if _, err := os.Stat(bin); err == nil {
		return []Result{SkipResult("brew-install", "Homebrew already installed at "+bin)}
	}

	if runtime.GOARCH == "arm64" {
		if !r.DryRun {
			if err := os.MkdirAll(prefix, 0755); err != nil {
				return []Result{FailResult("brew-prefix",
					fmt.Sprintf("failed to create %s: %v", prefix, err), err)}
			}
			// Ownership lets homebrew_owner write into the prefix during install.
			if err := chownPath(prefix, brewUserName, "admin"); err != nil {
				return []Result{FailResult("brew-prefix",
					fmt.Sprintf("failed to set ownership of %s: %v", prefix, err), err)}
			}
		} else {
			fmt.Printf("  [dry-run] mkdir -p %s && chown %s:admin %s\n",
				prefix, brewUserName, prefix)
		}
		results = append(results, OKResult("brew-prefix",
			fmt.Sprintf("%s created and owned by %s", prefix, brewUserName)))
	}

	// RunLive streams output directly to the terminal. The Homebrew installer
	// produces megabytes of output; using Run (CombinedOutput) would fill the
	// OS pipe buffer and deadlock. sudo -n fails immediately if NOPASSWD is
	// not in effect, rather than hanging on a password prompt.
	const installURL = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"
	installCmd := fmt.Sprintf(`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL %s)"`, installURL)

	if err := r.RunLive("sudo", "-n", "-H", "-u", brewUserName, "/bin/bash", "-c", installCmd); err != nil {
		return append(results, FailResult("brew-install", "Homebrew installer failed", err))
	}
	return append(results, OKResult("brew-install", "Homebrew installed at "+bin))
}

// createBrewWrapper writes /opt/macsetup/brew, a script that transparently
// delegates every brew invocation to homebrew_owner by calling the real brew
// binary at its fully qualified path. This avoids any circular reference and
// works identically on Intel (/usr/local/bin/brew) and Apple Silicon
// (/opt/homebrew/bin/brew).
func createBrewWrapper(r *Runner, brewBinPath string) Result {
	content := fmt.Sprintf("#!/bin/bash\n"+
		"# %s — transparent multi-user brew wrapper\n"+
		"# Any user running 'brew' is automatically delegated to %s.\n"+
		"# Passwordless via %s.\n"+
		"exec sudo -H -u %s %s \"$@\"\n",
		brewWrapperPath, brewUserName, sudoersPath, brewUserName, brewBinPath,
	)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s\n", brewWrapperPath)
		return OKResult("brew-wrapper",
			fmt.Sprintf("would create %s (dry-run)", brewWrapperPath))
	}

	if err := os.MkdirAll(brewWrapperDir, 0755); err != nil {
		return FailResult("brew-wrapper", "failed to create "+brewWrapperDir, err)
	}

	if err := os.WriteFile(brewWrapperPath, []byte(content), 0755); err != nil {
		return FailResult("brew-wrapper",
			fmt.Sprintf("failed to write %s: %v", brewWrapperPath, err), err)
	}

	return OKResult("brew-wrapper", "brew wrapper created at "+brewWrapperPath)
}

// injectBrewPath ensures /opt/macsetup appears first in PATH for all users:
//   - /etc/paths.d/00-macsetup  — picked up by macOS path_helper (all login shells)
//   - /etc/zshenv               — runs before path_helper, guaranteeing precedence
//
// Using both means the wrapper takes priority over both /usr/local/bin/brew
// (Intel) and /opt/homebrew/bin/brew (Apple Silicon) regardless of shell config.
func injectBrewPath(r *Runner) Result {
	if r.DryRun {
		fmt.Printf("  [dry-run] write %s\n", pathsDPath)
		fmt.Printf("  [dry-run] prepend %s in %s\n", brewWrapperDir, zshenvPath)
		return OKResult("brew-path", "would configure PATH (dry-run)")
	}

	// paths.d entry — covers bash, zsh, and any shell that calls path_helper.
	if err := os.WriteFile(pathsDPath, []byte(brewWrapperDir+"\n"), 0644); err != nil {
		return FailResult("brew-path", "failed to write "+pathsDPath, err)
	}

	// /etc/zshenv runs before /etc/zprofile (where path_helper is called),
	// so this guarantees /opt/macsetup is first even when path_helper reorders.
	existing, _ := os.ReadFile(zshenvPath)
	if strings.Contains(string(existing), zshenvMark) {
		return SkipResult("brew-path", zshenvPath+" already updated")
	}

	f, err := os.OpenFile(zshenvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return FailResult("brew-path", "failed to open "+zshenvPath, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n%s\nexport PATH=\"%s:$PATH\"\n", zshenvMark, brewWrapperDir); err != nil {
		return FailResult("brew-path", "failed to write to "+zshenvPath, err)
	}

	return OKResult("brew-path",
		fmt.Sprintf("%s prepended to PATH in %s and %s", brewWrapperDir, zshenvPath, pathsDPath))
}

// chownPath sets the owner of path to the given username and group name,
// resolving UIDs and GIDs via os/user rather than shelling out to chown.
func chownPath(path, username, groupName string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", username, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("parse uid for %q: %w", username, err)
	}

	g, err := user.LookupGroup(groupName)
	if err != nil {
		return fmt.Errorf("lookup group %q: %w", groupName, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return fmt.Errorf("parse gid for %q: %w", groupName, err)
	}

	return os.Chown(path, uid, gid)
}

// randomBase64 generates n cryptographically random bytes and returns them
// base64-encoded.
func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
