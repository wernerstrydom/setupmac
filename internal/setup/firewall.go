package setup

import "github.com/wstrydom/setupmac/internal/macos"

const socketfilterfw = "/usr/libexec/ApplicationFirewall/socketfilterfw"

// ConfigureFirewall enables the macOS Application Firewall with stealth mode
// and (on macOS < 15) connection logging via socketfilterfw. Stealth mode
// prevents the machine from responding to port scans and unsolicited ICMP probes.
//
// --setloggingmode was removed from socketfilterfw in macOS 15 (Sequoia); on
// that version firewall logging is always on and cannot be toggled via this tool.
func ConfigureFirewall(r *Runner, ver macos.Version) []Result {
	var results []Result

	out, err := r.Run(socketfilterfw, "--setglobalstate", "on")
	if err != nil {
		results = append(results, FailResult("firewall-enable", out, err))
		// Abort: if the firewall can't be enabled the other flags are meaningless.
		return results
	}
	results = append(results, OKResult("firewall-enable", "Application Firewall enabled"))

	out, err = r.Run(socketfilterfw, "--setstealthmode", "on")
	if err != nil {
		results = append(results, FailResult("firewall-stealth", out, err))
	} else {
		results = append(results, OKResult("firewall-stealth", "Stealth mode enabled"))
	}

	if ver.AtLeast(15, 0) {
		results = append(results, SkipResult("firewall-logging",
			"--setloggingmode removed in macOS 15; logging is always on"))
	} else {
		out, err = r.Run(socketfilterfw, "--setloggingmode", "on")
		if err != nil {
			results = append(results, FailResult("firewall-logging", out, err))
		} else {
			results = append(results, OKResult("firewall-logging", "Firewall logging enabled"))
		}
	}

	return results
}
