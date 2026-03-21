package setup

// ConfigurePower disables all sleep modes and enables auto-restart and Wake-on-LAN.
// All pmset flags used here are available on macOS 10.7+ (all target versions).
func ConfigurePower(r *Runner) Result {
	out, err := r.Run("pmset", "-a",
		"sleep", "0",
		"displaysleep", "0",
		"disksleep", "0",
		"autopoweroff", "0",
		"standby", "0",
		"autorestart", "1",
		"womp", "1",
	)
	if err != nil {
		return FailResult("power", out, err)
	}
	return OKResult("power", "sleep=0 displaysleep=0 disksleep=0 autopoweroff=0 standby=0 autorestart=1 womp=1")
}
