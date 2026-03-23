package setup

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/wstrydom/setupmac/internal/macos"
)

// VerifyAll reads back each configured setting and returns a result per check.
// Read() is used throughout — it always executes even in dry-run mode.
func VerifyAll(r *Runner, ver macos.Version, username string) []Result {
	var results []Result

	results = append(results, verifyPower(r))
	results = append(results, verifyBluetooth(r))
	results = append(results, verifyAirDrop(r))
	results = append(results, verifyUniversalControl(r, ver))
	results = append(results, verifyScreenSaver(r))
	results = append(results, verifyARD(r))
	results = append(results, verifyFileVault(r))
	results = append(results, verifyGuest(r))
	results = append(results, verifyAutoLogin(r, username))
	results = append(results, verifyBanner())
	results = append(results, verifySSH())
	results = append(results, verifyFirewall(r))
	results = append(results, verifySIP(r))
	results = append(results, verifyHomebrew())
	results = append(results, verifyNTP(r))
	results = append(results, verifyAutoUpdates(r))

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
			// Some hardware (e.g. older Intel Mac Minis) doesn't expose every
			// pmset key. Treat as absent rather than failed.
			continue
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
	for line := range strings.SplitSeq(output, "\n") {
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

func verifyGuest(r *Runner) Result {
	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.loginwindow", "GuestEnabled")
	if err != nil || strings.TrimSpace(out) != "0" {
		return FailResult("verify-guest",
			fmt.Sprintf("GuestEnabled=%q (want 0)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-guest", "Guest account")
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

func verifyHomebrew() Result {
	// All checks use os/user and os.Stat — no shell calls needed.
	if _, err := user.Lookup(brewUserName); err != nil {
		return FailResult("verify-homebrew", "user homebrew_owner does not exist", err)
	}

	if _, err := os.Stat(sudoersPath); err != nil {
		return FailResult("verify-homebrew", sudoersPath+" not found", err)
	}

	bin := brewBin()
	if _, err := os.Stat(bin); err != nil {
		return FailResult("verify-homebrew",
			fmt.Sprintf("brew binary not found at %s", bin), err)
	}

	if _, err := os.Stat(brewWrapperPath); err != nil {
		return FailResult("verify-homebrew",
			"brew wrapper not found at "+brewWrapperPath, err)
	}

	if _, err := os.Stat(pathsDPath); err != nil {
		return FailResult("verify-homebrew",
			"PATH not configured: "+pathsDPath+" not found", err)
	}

	zprofile, _ := os.ReadFile(zprofilePath)
	if !strings.Contains(string(zprofile), pathMark) {
		return FailResult("verify-homebrew",
			brewWrapperDir+" not prepended in "+zprofilePath+" — brew wrapper will not take PATH priority", nil)
	}

	return OKResult("verify-homebrew", "Homebrew")
}

func verifyBanner() Result {
	if _, err := os.Stat(sshBannerPath); err != nil {
		return WarnResult("verify-banner",
			sshBannerPath+" not found — run with --banner-org to configure")
	}
	return OKResult("verify-banner", "Login banner")
}

func verifyAirDrop(r *Runner) Result {
	out, err := r.Read("defaults", "read",
		"com.apple.NetworkBrowser", "DisableAirDrop")
	if err != nil || strings.TrimSpace(out) != "1" {
		return FailResult("verify-airdrop",
			fmt.Sprintf("DisableAirDrop=%q (want 1)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-airdrop", "AirDrop")
}

// verifySSH checks that the CIS-required directives are active in sshd_config.
// It only checks directives that HardenSSH always writes; PasswordAuthentication
// is omitted because it is conditional on GitHub key installation.
func verifySSH() Result {
	data, err := os.ReadFile(sshdConfigPath)
	if err != nil {
		return FailResult("verify-ssh",
			fmt.Sprintf("read %s: %v", sshdConfigPath, err), err)
	}

	expected := map[string]string{
		"PermitRootLogin":     "no",
		"X11Forwarding":       "no",
		"LogLevel":            "VERBOSE",
		"ClientAliveInterval": "60",
		"ClientAliveCountMax": "30",
	}

	config := string(data)
	var mismatches []string
	for key, want := range expected {
		got, found := findSSHDirective(config, key)
		if !found || got != want {
			mismatches = append(mismatches, fmt.Sprintf("%s=%q (want %q)", key, got, want))
		}
	}

	if len(mismatches) > 0 {
		return FailResult("verify-ssh",
			"unexpected sshd_config values: "+strings.Join(mismatches, ", "), nil)
	}
	return OKResult("verify-ssh", "SSH hardening")
}

// findSSHDirective returns the value of an active (non-commented) sshd_config
// directive. The key comparison is case-insensitive to match sshd behaviour.
func findSSHDirective(config, key string) (string, bool) {
	for line := range strings.SplitSeq(config, "\n") {
		s := strings.TrimLeft(line, " \t")
		if s == "" || s[0] == '#' {
			continue
		}
		fields := strings.Fields(s)
		if len(fields) >= 2 && strings.EqualFold(fields[0], key) {
			return fields[1], true
		}
	}
	return "", false
}

func verifyFirewall(r *Runner) Result {
	out, err := r.Read(socketfilterfw, "--getglobalstate")
	if err != nil {
		return FailResult("verify-firewall",
			fmt.Sprintf("socketfilterfw --getglobalstate failed: %s", out), err)
	}
	if strings.Contains(out, "enabled") {
		return OKResult("verify-firewall", "Application Firewall")
	}
	return FailResult("verify-firewall",
		fmt.Sprintf("firewall not enabled: %s", strings.TrimSpace(out)), nil)
}

func verifyNTP(r *Runner) Result {
	out, err := r.Read("systemsetup", "-getusingnetworktime")
	if err != nil {
		return FailResult("verify-ntp", fmt.Sprintf("systemsetup -getusingnetworktime failed: %s", out), err)
	}
	if strings.Contains(out, "On") {
		return OKResult("verify-ntp", "NTP")
	}
	return FailResult("verify-ntp", fmt.Sprintf("network time not enabled: %s", strings.TrimSpace(out)), nil)
}

func verifyAutoUpdates(r *Runner) Result {
	out, err := r.Read("defaults", "read", "com.apple.SoftwareUpdate", "AutomaticCheckEnabled")
	if err != nil || strings.TrimSpace(out) != "1" {
		return FailResult("verify-auto-updates",
			fmt.Sprintf("AutomaticCheckEnabled=%q (want 1)", strings.TrimSpace(out)), err)
	}
	return OKResult("verify-auto-updates", "Auto-updates")
}

// verifySIP reports the System Integrity Protection status. SIP can only be
// changed from Recovery Mode, so a disabled state is reported as a warning
// rather than a hard failure — the operator must fix it outside setupmac.
func verifySIP(r *Runner) Result {
	out, err := r.Read("csrutil", "status")
	if err != nil {
		return FailResult("verify-sip",
			fmt.Sprintf("csrutil status failed: %s", out), err)
	}
	if strings.Contains(out, "enabled") {
		return OKResult("verify-sip", "SIP enabled")
	}
	return WarnResult("verify-sip",
		"SIP is disabled — boot into Recovery Mode and run: csrutil enable")
}
