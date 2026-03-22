package setup

const socketfilterfw = "/usr/libexec/ApplicationFirewall/socketfilterfw"

// ConfigureFirewall enables the macOS Application Firewall with stealth mode
// and connection logging via socketfilterfw. Stealth mode prevents the machine
// from responding to port scans and unsolicited ICMP probes.
func ConfigureFirewall(r *Runner) []Result {
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

	out, err = r.Run(socketfilterfw, "--setloggingmode", "on")
	if err != nil {
		results = append(results, FailResult("firewall-logging", out, err))
	} else {
		results = append(results, OKResult("firewall-logging", "Firewall logging enabled"))
	}

	return results
}
