package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// kcpasswordKey is the XOR key macOS uses to obfuscate /etc/kcpassword.
// The boot process XORs the file contents back with this key to recover the
// plaintext password before mounting home directories.
var kcpasswordKey = []byte{0x7D, 0x89, 0x52, 0x23, 0xD2, 0xBC, 0xDD, 0xEA, 0xA3, 0xB9, 0x1F}

// EnableAutoLogin configures automatic login for username.
// Works on all macOS versions when FileVault is disabled; Apple removed the
// UI option in macOS 13 but the underlying mechanism still functions.
//
// If auto-login is already configured for username and /etc/kcpassword exists,
// the operator is asked whether they want to update the password rather than
// being required to re-enter it unconditionally.
func EnableAutoLogin(r *Runner, username string) []Result {
	if username == "" {
		return []Result{SkipResult("autologin", "--username not provided")}
	}

	alreadyConfigured := currentAutoLoginUser(r) == username
	_, kcpErr := os.Stat("/etc/kcpassword")
	kcpExists := kcpErr == nil

	if alreadyConfigured && kcpExists {
		if r.DryRun {
			return []Result{SkipResult("autologin",
				fmt.Sprintf("auto-login already configured for %s (dry-run)", username))}
		}
		if !confirmYN(fmt.Sprintf("Auto-login already configured for %s. Update password?", username), false) {
			return []Result{SkipResult("autologin",
				fmt.Sprintf("auto-login already configured for %s", username))}
		}
		// Operator confirmed — re-write password only; plist is already correct.
		return []Result{writeKCPassword(username)}
	}

	var results []Result

	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.loginwindow",
		"autoLoginUser", "-string", username,
	)
	if err != nil {
		results = append(results, FailResult("autologin-user", out, err))
		return results
	}
	results = append(results, OKResult("autologin-user", fmt.Sprintf("autoLoginUser set to: %s", username)))

	if r.DryRun {
		fmt.Printf("  [dry-run] write /etc/kcpassword (XOR-encoded password for %s)\n", username)
		results = append(results, OKResult("autologin-kcpassword", "/etc/kcpassword would be written (dry-run)"))
		return results
	}

	results = append(results, writeKCPassword(username))
	return results
}

// currentAutoLoginUser reads the autoLoginUser preference from the loginwindow
// plist. Returns an empty string if unset or on any error.
func currentAutoLoginUser(r *Runner) string {
	out, err := r.Read("defaults", "read",
		"/Library/Preferences/com.apple.loginwindow", "autoLoginUser")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// writeKCPassword prompts for the user's login password and writes the
// XOR-encoded /etc/kcpassword that macOS reads at boot for auto-login.
func writeKCPassword(username string) Result {
	fmt.Fprintf(os.Stderr, "Enter login password for %s: ", username)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return FailResult("autologin-kcpassword", "failed to read password", err)
	}

	encoded := encodeKCPassword(string(passwordBytes))
	if err := os.WriteFile("/etc/kcpassword", encoded, 0600); err != nil {
		return FailResult("autologin-kcpassword",
			fmt.Sprintf("write /etc/kcpassword: %v", err), err)
	}
	return OKResult("autologin-kcpassword", "/etc/kcpassword written")
}

// confirmYN prints a yes/no prompt and returns true if the operator answers y.
// defaultYes controls what an empty (Enter) response means.
func confirmYN(question string, defaultYes bool) bool {
	choices := "[y/N]"
	if defaultYes {
		choices = "[Y/n]"
	}
	fmt.Fprintf(os.Stderr, "%s %s: ", question, choices)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

// encodeKCPassword XOR-encodes the password using the kcpassword key.
// The result is padded to a multiple of the key length with null bytes.
func encodeKCPassword(password string) []byte {
	key := kcpasswordKey
	pw := []byte(password)

	// Pad to next multiple of key length so the last XOR block is complete.
	if rem := len(pw) % len(key); rem != 0 {
		pw = append(pw, make([]byte, len(key)-rem)...)
	}
	if len(pw) == 0 {
		pw = make([]byte, len(key))
	}

	out := make([]byte, len(pw))
	for i, b := range pw {
		out[i] = b ^ key[i%len(key)]
	}
	return out
}
