package setup

import (
	"fmt"
	"os"
)

const kickstartPath = "/System/Library/CoreServices/RemoteManagement/ARDAgent.app/Contents/Resources/kickstart"

// ConfigureARD enables Apple Remote Desktop for all local users with full
// privileges, enables VNC legacy connections, and optionally sets a VNC password.
func ConfigureARD(r *Runner, vncPassword string) []Result {
	// Pre-flight: confirm the kickstart binary exists before attempting anything.
	if _, err := os.Stat(kickstartPath); err != nil {
		return []Result{FailResult("ard", fmt.Sprintf(
			"ARDAgent kickstart not found at %s — is Remote Management installed?", kickstartPath,
		), err)}
	}

	var results []Result

	// Activate ARD and grant full privileges to all local users.
	out, err := r.Run(kickstartPath,
		"-activate",
		"-configure", "-allowAccessFor", "-allUsers",
		"-configure", "-access", "-on",
		"-configure", "-privs", "-all",
		"-restart", "-agent", "-menu",
	)
	if err != nil {
		// Without activation the subsequent steps are meaningless.
		results = append(results, FailResult("ard-activate", out, err))
		return results
	}
	results = append(results, OKResult("ard-activate", "ARD activated for all users"))

	out, err = r.Run("defaults", "write",
		"/Library/Preferences/com.apple.RemoteManagement",
		"VNCLegacyConnectionsEnabled", "-bool", "true",
	)
	if err != nil {
		results = append(results, FailResult("ard-vnc-legacy", out, err))
	} else {
		results = append(results, OKResult("ard-vnc-legacy", "VNC legacy connections enabled"))
	}

	// -2147483648 (0x80000000) grants all ARD privileges to all local users.
	out, err = r.Run("defaults", "write",
		"/Library/Preferences/com.apple.RemoteManagement",
		"ARD_AllLocalUsersPrivs", "-int", "-2147483648",
	)
	if err != nil {
		results = append(results, FailResult("ard-privs", out, err))
	} else {
		results = append(results, OKResult("ard-privs", "ARD_AllLocalUsersPrivs set"))
	}

	if vncPassword == "" {
		results = append(results, SkipResult("ard-vnc-password", "--vnc-password not provided"))
		return results
	}

	out, err = r.Run(kickstartPath,
		"-configure", "-clientopts",
		"-setvnclegacy", "-vnclegacy", "yes",
		"-setvncpw", "-vncpw", vncPassword,
	)
	if err != nil {
		results = append(results, FailResult("ard-vnc-password", out, err))
	} else {
		results = append(results, OKResult("ard-vnc-password", "VNC password set"))
	}

	return results
}
