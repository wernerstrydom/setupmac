package setup

import (
	"fmt"
	"strings"
)

// DisableFileVault checks FileVault status and disables it if active.
// The administrator password is supplied by the caller; all user input is
// collected upfront before any setup step runs.
func DisableFileVault(r *Runner, password string) Result {
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

	out, err := r.RunWithStdin(password, "fdesetup", "disable")
	if err != nil {
		return FailResult("filevault", fmt.Sprintf("fdesetup disable failed: %s", out), err)
	}
	return OKResult("filevault", "FileVault disabled")
}
