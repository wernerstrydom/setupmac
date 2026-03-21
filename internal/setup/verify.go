package setup

import (
	"fmt"
	"os"
	"strings"

	"github.com/wstrydom/setupmac/internal/macos"
)

// VerifyAll reads back each configured setting and returns a result per check.
// Read() is used throughout — it always executes even in dry-run mode.
func VerifyAll(r *Runner, ver macos.Version, username string) []Result {
	var results []Result

	results = append(results, verifyPower(r))
	results = append(results, verifyBluetooth(r))
	results = append(results, verifyUniversalControl(r, ver))
	results = append(results, verifyScreenSaver(r))
	results = append(results, verifyARD(r))
	results = append(results, verifyFileVault(r))
	results = append(results, verifyAutoLogin(r, username))

	return results
}

func verifyPower(r *Runner) Result {
	out, err := r.Read("pmset", "-g")
	if err != nil {
		return FailResult("verify-power", fmt.Sprintf("pmset -g failed: %s", out), err)
	}

	expected := map[string]string{
		"sleep":        "0",
		"displaysleep": "0",
		"disksleep":    "0",
		"autopoweroff": "0",
		"standby":      "0",
		"autorestart":  "1",
		"womp":         "1",
	}

	var mismatches []string
	for key, want := range expected {
		if got, ok := parsePmsetValue(out, key); !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s=<not found>", key))
		} else if got != want {
			mismatches = append(mismatches, fmt.Sprintf("%s=%s (want %s)", key, got, want))
		}
	}

	if len(mismatches) > 0 {
		return FailResult("verify-power", "unexpected pmset values: "+strings.Join(mismatches, ", "), nil)
	}
	return OKResult("verify-power", "Power management")
}

// parsePmsetValue finds a key in pmset -g output and returns its value.
func parsePmsetValue(output, key string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// pmset output lines look like: " sleep              0 (sleep prevented by..."
		// or just "sleep 0"
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == key && i+1 < len(fields) {
				return fields[i+1], true
			}
		}
	}
	return "", false
}

func verifyBluetooth(r *Runner) Result {
	var failures []string

	for _, key := range []string{"BluetoothAutoSeekKeyboard", "BluetoothAutoSeekPointingDevice"} {
		out, err := r.Read("defaults", "read",
			"/Library/Preferences/com.apple.Bluetooth", key)
		if err != nil || strings.TrimSpace(out) != "0" {
			failures = append(failures, fmt.Sprintf("%s=%q", key, out))
		}
	}

	if len(failures) > 0 {
		return FailResult("verify-bluetooth", "unexpected values: "+strings.Join(failures, ", "), nil)
	}
	return OKResult("verify-bluetooth", "Bluetooth")
}

func verifyUniversalControl(r *Runner, ver macos.Version) Result {
	if !ver.AtLeast(12, 3) {
		return SkipResult("verify-universal-control", "macOS < 12.3, not applicable")
	}

	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.universalcontrol", "Disabled")
	if err != nil || strings.TrimSpace(out) != "1" {
		return FailResult("verify-universal-control",
			fmt.Sprintf("Disabled=%q (want 1)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-universal-control", "Universal Control")
}

func verifyScreenSaver(r *Runner) Result {
	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.screensaver", "loginWindowIdleTime")
	if err != nil || strings.TrimSpace(out) != "0" {
		return FailResult("verify-screensaver",
			fmt.Sprintf("loginWindowIdleTime=%q (want 0)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-screensaver", "Screen saver")
}

func verifyARD(r *Runner) Result {
	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.RemoteManagement", "VNCLegacyConnectionsEnabled")
	if err != nil || strings.TrimSpace(out) != "1" {
		return FailResult("verify-ard",
			fmt.Sprintf("VNCLegacyConnectionsEnabled=%q (want 1)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-ard", "ARD")
}

func verifyFileVault(r *Runner) Result {
	out, err := r.Read("fdesetup", "status")
	if err != nil {
		return FailResult("verify-filevault", fmt.Sprintf("fdesetup status failed: %s", out), err)
	}
	if strings.Contains(out, "FileVault is Off") {
		return OKResult("verify-filevault", "FileVault")
	}
	return FailResult("verify-filevault", fmt.Sprintf("FileVault is still enabled: %s", out), nil)
}

func verifyAutoLogin(r *Runner, username string) Result {
	if username == "" {
		return SkipResult("verify-autologin", "--username not provided")
	}

	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.loginwindow", "autoLoginUser")
	if err != nil || strings.TrimSpace(out) != username {
		return FailResult("verify-autologin",
			fmt.Sprintf("autoLoginUser=%q (want %q)", strings.TrimSpace(out), username), err)
	}

	if _, err := os.Stat("/etc/kcpassword"); err != nil {
		return FailResult("verify-autologin", "/etc/kcpassword does not exist", err)
	}

	return OKResult("verify-autologin", "Auto-login")
}
