package setup

import (
	"os"
	"strings"
)

// HostState captures the parts of the current host configuration that
// determine which questions need to be asked before running setup steps.
type HostState struct {
	FileVaultEnabled bool   // FileVault is currently on
	AutoLoginUser    string // current autoLoginUser plist value; "" if not set
	KCPasswordExists bool   // /etc/kcpassword already exists
}

// Inspect reads the current host state. It is always called before gathering
// user input so that prompts are only shown when they are actually needed.
func Inspect(r *Runner) HostState {
	var s HostState

	out, _ := r.Read("fdesetup", "status")
	s.FileVaultEnabled = strings.Contains(out, "FileVault is On")

	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.loginwindow", "autoLoginUser")
	if err == nil {
		s.AutoLoginUser = strings.TrimSpace(out)
	}

	_, err = os.Stat("/etc/kcpassword")
	s.KCPasswordExists = err == nil

	return s
}
