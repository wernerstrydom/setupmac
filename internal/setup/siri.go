package setup

// DisableSiri prevents Siri from running system-wide, reducing unnecessary
// outbound network traffic and attack surface on headless servers.
func DisableSiri(r *Runner) Result {
	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.assistant.support",
		"Assistant Enabled", "-bool", "false",
	)
	if err != nil {
		return FailResult("siri", out, err)
	}
	return OKResult("siri", "Siri disabled")
}
