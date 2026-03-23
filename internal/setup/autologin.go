package setup

import (
	"fmt"
	"os"
)

// kcpasswordKey is the XOR key macOS uses to obfuscate /etc/kcpassword.
// The boot process XORs the file contents back with this key to recover the
// plaintext password before mounting home directories.
var kcpasswordKey = []byte{0x7D, 0x89, 0x52, 0x23, 0xD2, 0xBC, 0xDD, 0xEA, 0xA3, 0xB9, 0x1F}

// EnableAutoLogin configures automatic login for username.
// Works on all macOS versions when FileVault is disabled; Apple removed the
// UI option in macOS 13 but the underlying mechanism still functions.
//
// password is the user's login password for writing /etc/kcpassword.
// Pass an empty string to skip the kcpassword write (e.g. when auto-login is
// already configured and the operator chose not to update the password).
func EnableAutoLogin(r *Runner, username, password string) []Result {
	if username == "" {
		return []Result{SkipResult("autologin", "--username not provided")}
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

	if password == "" {
		results = append(results, SkipResult("autologin-kcpassword", "unchanged"))
		return results
	}

	if r.DryRun {
		fmt.Printf("  [dry-run] write /etc/kcpassword (XOR-encoded password for %s)\n", username)
		results = append(results, OKResult("autologin-kcpassword", "/etc/kcpassword would be written (dry-run)"))
		return results
	}

	encoded := encodeKCPassword(password)
	if err := os.WriteFile("/etc/kcpassword", encoded, 0600); err != nil {
		results = append(results, FailResult("autologin-kcpassword",
			fmt.Sprintf("write /etc/kcpassword: %v", err), err))
		return results
	}
	results = append(results, OKResult("autologin-kcpassword", "/etc/kcpassword written"))

	return results
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
