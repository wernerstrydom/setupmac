package setup

import "github.com/wstrydom/setupmac/internal/macos"

// DisableUniversalControl disables cursor/keyboard sharing between Macs.
// Universal Control was introduced in macOS 12.3; the step is skipped on older versions.
func DisableUniversalControl(r *Runner, ver macos.Version) Result {
	if !ver.AtLeast(12, 3) {
		return SkipResult("universal-control", "macOS < 12.3, not applicable")
	}

	// Write to the system-level plist so it applies regardless of which user is logged in.
	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.universalcontrol",
		"Disabled", "-bool", "true",
	)
	if err != nil {
		return FailResult("universal-control", out, err)
	}
	return OKResult("universal-control", "Disabled")
}
