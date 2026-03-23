package setup

// ConfigureAutoUpdates enables automatic macOS software update checks,
// background downloads, and automatic installation of security patches.
// Allowing the OS to reboot for updates is intentional on a headless lab
// machine where availability of the latest security fixes matters more than
// uptime continuity.
func ConfigureAutoUpdates(r *Runner) []Result {
	var results []Result

	// Enable the softwareupdate schedule daemon so checks run on a timer
	// rather than only on demand.
	out, err := r.Run("softwareupdate", "--schedule", "on")
	if err != nil {
		return append(results, FailResult("updates-schedule", out, err))
	}
	results = append(results, OKResult("updates-schedule", "software update schedule enabled"))

	type pref struct {
		step   string
		domain string
		key    string
		kind   string
		val    string
		msg    string
	}

	prefs := []pref{
		{"updates-check", "com.apple.SoftwareUpdate", "AutomaticCheckEnabled", "-bool", "true", "automatic update check enabled"},
		{"updates-frequency", "com.apple.SoftwareUpdate", "ScheduleFrequency", "-int", "1", "update check frequency set to daily"},
		{"updates-download", "com.apple.SoftwareUpdate", "AutomaticDownload", "-int", "1", "automatic background download enabled"},
		{"updates-critical", "com.apple.SoftwareUpdate", "CriticalUpdateInstall", "-int", "1", "automatic security patch install enabled"},
		{"updates-appstore", "com.apple.commerce", "AutoUpdate", "-bool", "true", "App Store auto-update enabled"},
		{"updates-reboot", "com.apple.commerce", "AutoUpdateRestartRequired", "-bool", "true", "OS update reboot allowed"},
	}

	for _, p := range prefs {
		out, err := r.Run("defaults", "write", p.domain, p.key, p.kind, p.val)
		if err != nil {
			results = append(results, FailResult(p.step, out, err))
			continue
		}
		results = append(results, OKResult(p.step, p.msg))
	}

	return results
}
