package setup

// DisableGuestAccount prevents Guest login at the login window.
// This is the primary protection for a headless lab Mac — if Guest cannot
// log in, the sudoers deny in the brew setup is only belt-and-suspenders.
func DisableGuestAccount(r *Runner) Result {
	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.loginwindow",
		"GuestEnabled", "-bool", "false",
	)
	if err != nil {
		return FailResult("guest", out, err)
	}
	return OKResult("guest", "Guest account disabled")
}
