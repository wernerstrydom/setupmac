package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sshdConfigPath = "/etc/ssh/sshd_config"

// HardenSSH applies CIS-recommended directives to /etc/ssh/sshd_config.
//
// Directives always applied:
//   - PermitRootLogin no        (CIS 5.2.2 — no direct root SSH login)
//   - X11Forwarding no          (CIS 5.2.6 — not needed on headless machines)
//   - LogLevel VERBOSE           (CIS 5.2.5 — logs key fingerprint on auth)
//   - ClientAliveInterval 60    (send keepalive every 60 s to detect dead connections)
//   - ClientAliveCountMax 30    (allow 30 missed keepalives ≈ 30 min before disconnect;
//     active sessions are unaffected as network traffic resets the counter)
//
// When disablePasswordAuth is true:
//   - PasswordAuthentication no (disables brute-force vector; only set after keys are confirmed installed)
//
// The original file is backed up to /etc/ssh/sshd_config.bak.
// The modified config is validated with "sshd -t" before the backup is written.
//
// No daemon reload is needed: macOS launchd spawns a new sshd process per
// connection, so the next login picks up the new config automatically.
func HardenSSH(r *Runner, disablePasswordAuth bool) []Result {
	directives := map[string]string{
		"PermitRootLogin":     "no",
		"X11Forwarding":       "no",
		"LogLevel":            "VERBOSE",
		"ClientAliveInterval": "60",
		"ClientAliveCountMax": "30",
	}
	if disablePasswordAuth {
		directives["PasswordAuthentication"] = "no"
	}

	if r.DryRun {
		for k, v := range directives {
			fmt.Printf("  [dry-run] sshd_config: %s %s\n", k, v)
		}
		return []Result{OKResult("ssh-hardening", "would harden "+sshdConfigPath+" (dry-run)")}
	}

	original, err := os.ReadFile(sshdConfigPath)
	if err != nil {
		return []Result{FailResult("ssh-hardening",
			fmt.Sprintf("read %s: %v", sshdConfigPath, err), err)}
	}

	modified := applySSHDirectives(string(original), directives)

	tmp, err := os.CreateTemp(filepath.Dir(sshdConfigPath), ".sshd_config_*")
	if err != nil {
		return []Result{FailResult("ssh-hardening", "create temp file: "+err.Error(), err)}
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck

	if _, err := tmp.WriteString(modified); err != nil {
		tmp.Close()
		return []Result{FailResult("ssh-hardening", "write temp file: "+err.Error(), err)}
	}
	tmp.Close()

	// Validate syntax before committing. r.Read always executes, even in dry-run.
	if out, err := r.Read("sshd", "-t", "-f", tmpPath); err != nil {
		return []Result{FailResult("ssh-hardening",
			fmt.Sprintf("sshd -t validation failed: %s", out), err)}
	}

	backupPath := sshdConfigPath + ".bak"
	if err := os.WriteFile(backupPath, original, 0644); err != nil {
		return []Result{FailResult("ssh-hardening",
			fmt.Sprintf("backup %s: %v", backupPath, err), err)}
	}

	if err := os.Rename(tmpPath, sshdConfigPath); err != nil {
		return []Result{FailResult("ssh-hardening",
			fmt.Sprintf("install %s: %v", sshdConfigPath, err), err)}
	}

	msg := "hardened " + sshdConfigPath
	if disablePasswordAuth {
		msg += "; PasswordAuthentication disabled"
	}
	return []Result{OKResult("ssh-hardening", msg)}
}

// applySSHDirectives returns a modified copy of an sshd_config with the given
// key-value directives applied. For each directive key, the first matching line
// (active or commented-out) is replaced with "Key Value"; any subsequent
// occurrences are dropped. Directives not found in the file are appended.
func applySSHDirectives(config string, directives map[string]string) string {
	replaced := map[string]bool{}
	var out []string

	for line := range strings.SplitSeq(config, "\n") {
		if key, ok := sshdDirectiveKey(line); ok {
			if val, isTarget := directives[key]; isTarget {
				if !replaced[key] {
					out = append(out, key+" "+val)
					replaced[key] = true
				}
				// Drop commented or duplicate occurrences of this directive.
				continue
			}
		}
		out = append(out, line)
	}

	for key, val := range directives {
		if !replaced[key] {
			out = append(out, key+" "+val)
		}
	}

	return strings.Join(out, "\n")
}

// sshdDirectiveKey extracts the directive name from a sshd_config line.
// It matches active directives ("Key Value") and commented-out directives
// where the # immediately precedes the key ("#Key Value") — the convention
// used in the macOS default sshd_config for disabled options.
// Pure comment lines ("# some text") and blank lines return ("", false).
func sshdDirectiveKey(line string) (string, bool) {
	s := strings.TrimLeft(line, " \t")
	if s == "" {
		return "", false
	}

	// Active directive.
	if s[0] != '#' {
		if fields := strings.Fields(s); len(fields) >= 1 {
			return fields[0], true
		}
		return "", false
	}

	// Commented directive: "#Key" with no space between # and the key.
	if len(s) > 1 && s[1] != ' ' && s[1] != '\t' && s[1] != '#' {
		if fields := strings.Fields(s[1:]); len(fields) >= 1 {
			return fields[0], true
		}
	}
	return "", false
}
