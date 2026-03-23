package setup

import "strings"

// ConfigureTimeSync enables NTP, sets Apple's time server, and enables automatic
// timezone detection. These settings are essential for a headless server where
// accurate time affects SSH key validity, log timestamps, and certificate checks.
func ConfigureTimeSync(r *Runner) []Result {
	var results []Result

	// Enable network time synchronisation only when it is currently off.
	// systemsetup -getusingnetworktime prints "Network Time: On" or "Network Time: Off".
	out, _ := r.Read("systemsetup", "-getusingnetworktime")
	if strings.Contains(out, "On") {
		results = append(results, SkipResult("ntp-enable", "network time already enabled"))
	} else {
		out, err := r.Run("systemsetup", "-setusingnetworktime", "on")
		if err != nil {
			return append(results, FailResult("ntp-enable", out, err))
		}
		results = append(results, OKResult("ntp-enable", "network time enabled"))
	}

	out, err := r.Run("systemsetup", "-setnetworktimeserver", "time.apple.com")
	if err != nil {
		return append(results, FailResult("ntp-server", out, err))
	}
	results = append(results, OKResult("ntp-server", "NTP server set to time.apple.com"))

	// Enable automatic timezone so the system clock stays correct when the
	// Mac mini is moved or location data changes.
	out, err = r.Run("defaults", "write",
		"/Library/Preferences/com.apple.timezone.auto", "Active", "-bool", "YES")
	if err != nil {
		return append(results, FailResult("auto-timezone", out, err))
	}

	out, err = r.Run("defaults", "write",
		"/private/var/db/timed/Library/Preferences/com.apple.timed.plist",
		"TMAutomaticTimeOnlyEnabled", "-bool", "YES")
	if err != nil {
		return append(results, FailResult("auto-timezone", out, err))
	}

	out, err = r.Run("defaults", "write",
		"/private/var/db/timed/Library/Preferences/com.apple.timed.plist",
		"TMAutomaticTimeZoneEnabled", "-bool", "YES")
	if err != nil {
		return append(results, FailResult("auto-timezone", out, err))
	}

	results = append(results, OKResult("auto-timezone", "automatic timezone detection enabled"))
	return results
}
