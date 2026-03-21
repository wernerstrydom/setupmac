package setup

// DisableLoginScreenSaver prevents the screen saver from activating at the
// login window, which would lock out headless VNC/ARD sessions.
func DisableLoginScreenSaver(r *Runner) Result {
	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.screensaver",
		"loginWindowIdleTime", "-int", "0",
	)
	if err != nil {
		return FailResult("screensaver", out, err)
	}
	return OKResult("screensaver", "Login window idle time set to 0")
}
