package setup

// DisableBluetoothAssistant prevents the Bluetooth Setup Assistant from
// appearing at the login window when no keyboard or mouse is detected.
func DisableBluetoothAssistant(r *Runner) []Result {
	var results []Result

	out, err := r.Run("defaults", "write",
		"/Library/Preferences/com.apple.Bluetooth",
		"BluetoothAutoSeekKeyboard", "-bool", "false",
	)
	if err != nil {
		results = append(results, FailResult("bluetooth-keyboard", out, err))
	} else {
		results = append(results, OKResult("bluetooth-keyboard", "Disabled keyboard auto-seek"))
	}

	out, err = r.Run("defaults", "write",
		"/Library/Preferences/com.apple.Bluetooth",
		"BluetoothAutoSeekPointingDevice", "-bool", "false",
	)
	if err != nil {
		results = append(results, FailResult("bluetooth-mouse", out, err))
	} else {
		results = append(results, OKResult("bluetooth-mouse", "Disabled pointing device auto-seek"))
	}

	// killall exits non-zero if the process isn't running — that's expected and fine.
	_, err = r.RunSilent("killall", "BluetoothSetupAssistant")
	if err != nil {
		results = append(results, WarnResult("bluetooth-kill", "BluetoothSetupAssistant not running"))
	} else {
		results = append(results, OKResult("bluetooth-kill", "Killed BluetoothSetupAssistant"))
	}

	return results
}
