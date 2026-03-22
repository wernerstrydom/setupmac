package setup

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// DisableFileVault checks FileVault status and disables it if active.
// Requires an administrator password supplied interactively; fdesetup reads
// credentials from stdin when its stdin is not a TTY.
func DisableFileVault(r *Runner) Result {
	status, err := r.Read("fdesetup", "status")
	if err != nil {
		return FailResult("filevault", fmt.Sprintf("fdesetup status failed: %s", status), err)
	}

	switch {
	case strings.Contains(status, "FileVault is Off"):
		return SkipResult("filevault", "already disabled")
	case strings.Contains(status, "FileVault is On"):
		// proceed below
	default:
		// Covers "Encrypting", "Decrypting", etc.
		return WarnResult("filevault", fmt.Sprintf("unexpected status: %s — wait for completion before disabling", status))
	}

	if r.DryRun {
		_, _ = r.RunWithStdin("", "fdesetup", "disable")
		return OKResult("filevault", "would disable FileVault (dry-run)")
	}

	fmt.Fprint(os.Stderr, "FileVault is enabled. Enter administrator password to disable: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return FailResult("filevault", "failed to read password", err)
	}
	password := string(passwordBytes)

	out, err := r.RunWithStdin(password, "fdesetup", "disable")
	if err != nil {
		return FailResult("filevault", fmt.Sprintf("fdesetup disable failed: %s", out), err)
	}
	return OKResult("filevault", "FileVault disabled")
}
