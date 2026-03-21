package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
)

const (
	brewUserName    = "homebrew_owner"
	brewUserDesc    = "Homebrew Dedicated Owner"
	sudoersPath     = "/etc/sudoers.d/homebrew-multiuser"
	brewWrapperPath = "/usr/local/bin/brew"
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
// without a password prompt, and (on Apple Silicon) places a transparent
// wrapper at /usr/local/bin/brew.
//
// On Intel, /usr/local/bin/brew IS the real brew binary — placing a wrapper
// there would cause infinite recursion. Intel users call brew via
// 'sudo -H -u homebrew_owner brew', covered by the sudoers drop-in.
func SetupHomebrew(r *Runner) []Result {
	prefix := BrewPrefix()
	bin := brewBin()

	var results []Result

	res := createBrewUser(r)
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

	if runtime.GOARCH == "arm64" {
		results = append(results, createBrewWrapper(r, bin))
	} else {
		// The wrapper would live at the same path as the real brew binary on
		// Intel, which would make it call itself. Skip it and document why.
		results = append(results, SkipResult("brew-wrapper",
			"Intel: /usr/local/bin/brew is the real binary — use 'sudo -H -u homebrew_owner brew'"))
	}

	return results
}

// createBrewUser creates a hidden service account that owns the Homebrew
// installation. If the account already exists the step is skipped.
func createBrewUser(r *Runner) Result {
	if _, err := user.Lookup(brewUserName); err == nil {
		return SkipResult("brew-user", fmt.Sprintf("user %q already exists", brewUserName))
	}

	// Generate a random password using crypto/rand. It is never used
	// interactively — it only satisfies the macOS account creation requirement.
	password, err := randomBase64(16)
	if err != nil {
		return FailResult("brew-user", "failed to generate random password", err)
	}

	// sysadminctl and dscl are macOS-specific tools with no Go stdlib equivalent.
	if out, err := r.Run("sysadminctl",
		"-addUser", brewUserName,
		"-fullName", brewUserDesc,
		"-password", password,
	); err != nil {
		return FailResult("brew-user", out, err)
	}

	// Hide the account from the login screen and System Settings.
	if out, err := r.Run("dscl", ".", "-create",
		"/Users/"+brewUserName, "IsHidden", "1"); err != nil {
		return FailResult("brew-user", out, err)
	}

	homeDir := "/var/" + brewUserName

	if out, err := r.Run("dscl", ".", "-create",
		"/Users/"+brewUserName, "NFSHomeDirectory", homeDir); err != nil {
		return FailResult("brew-user", out, err)
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

// writeBrewSudoers writes /etc/sudoers.d/homebrew-multiuser, granting admin
// group members passwordless sudo to run the brew binary as homebrew_owner.
// The file is validated by visudo before being moved into place.
func writeBrewSudoers(r *Runner, brewBinPath string) Result {
	content := fmt.Sprintf(
		"# Passwordless delegation: admin group → %s, brew binary only\n%%admin ALL=(%s) NOPASSWD: %s\n",
		brewUserName, brewUserName, brewBinPath,
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

	// su is always available to root without a password. We use it (not sudo)
	// because the sudoers rule covers only the brew binary itself, not the
	// installer shell script.
	const installURL = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"
	installCmd := fmt.Sprintf(`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL %s)"`, installURL)

	out, err := r.Run("su", "-", brewUserName, "-c", installCmd)
	if err != nil {
		return append(results, FailResult("brew-install", out, err))
	}
	return append(results, OKResult("brew-install", "Homebrew installed at "+bin))
}

// createBrewWrapper writes a shell script at /usr/local/bin/brew that
// transparently delegates every brew invocation to homebrew_owner via sudo.
// Only called on Apple Silicon where the paths are distinct.
func createBrewWrapper(r *Runner, brewBinPath string) Result {
	content := fmt.Sprintf("#!/bin/bash\n"+
		"# %s — global brew wrapper\n"+
		"# Delegates every brew invocation to the dedicated Homebrew owner account.\n"+
		"# Passwordless via %s.\n"+
		"exec sudo -H -u %s %s \"$@\"\n",
		brewWrapperPath, sudoersPath, brewUserName, brewBinPath,
	)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s\n", brewWrapperPath)
		return OKResult("brew-wrapper",
			fmt.Sprintf("would create wrapper at %s (dry-run)", brewWrapperPath))
	}

	if err := os.MkdirAll("/usr/local/bin", 0755); err != nil {
		return FailResult("brew-wrapper", "failed to create /usr/local/bin", err)
	}

	if err := os.WriteFile(brewWrapperPath, []byte(content), 0755); err != nil {
		return FailResult("brew-wrapper",
			fmt.Sprintf("failed to write %s: %v", brewWrapperPath, err), err)
	}

	return OKResult("brew-wrapper", "global brew wrapper created at "+brewWrapperPath)
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
