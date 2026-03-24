package setup

import "os"

const (
	brewMaintenanceScriptPath = "/opt/macsetup/brew-maintenance"
	brewMaintenancePlistPath  = "/Library/LaunchDaemons/com.macsetup.brew-maintenance.plist"
	brewMaintenanceLabel      = "com.macsetup.brew-maintenance"
)

// brewMaintenanceScript runs weekly to keep all Homebrew packages current.
// It delegates to homebrew_owner via the existing brew wrapper so no special
// credentials are needed beyond what the sudoers drop-in already grants.
const brewMaintenanceScript = `#!/bin/bash
# brew-maintenance — daily update, upgrade, and cleanup.
# Managed by setupmac; runs via launchd every day at 03:00.

set -euo pipefail

BREW=/opt/macsetup/brew
LOG=/var/log/brew-maintenance.log

echo "$(date): starting brew maintenance" >> "$LOG"
"$BREW" update          >> "$LOG" 2>&1
"$BREW" upgrade --greedy >> "$LOG" 2>&1
"$BREW" cleanup         >> "$LOG" 2>&1
echo "$(date): brew maintenance complete" >> "$LOG"
`

// brewMaintenancePlist runs brew-maintenance daily at 03:00.
// StartCalendarInterval fires even if the machine was asleep at the scheduled
// time — launchd will run the job at the next opportunity after wake.
const brewMaintenancePlistContent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.macsetup.brew-maintenance</string>
    <key>ProgramArguments</key>
    <array>
        <string>/opt/macsetup/brew-maintenance</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>3</integer>
        <key>Minute</key>
        <integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>/var/log/brew-maintenance.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/brew-maintenance.log</string>
</dict>
</plist>
`

// SetupBrewMaintenance installs a weekly launchd daemon that runs
// brew update, brew upgrade, and brew cleanup as homebrew_owner via the
// existing brew wrapper. This keeps all installed packages current without
// any manual intervention.
func SetupBrewMaintenance(r *Runner) []Result {
	if _, err := os.Stat(brewWrapperPath); err != nil {
		return []Result{WarnResult("brew-maintenance",
			"Homebrew not installed — run Homebrew (Multi-User) setup first")}
	}

	var results []Result
	results = append(results, writeBrewMaintenanceScript(r))
	results = append(results, writeBrewMaintenancePlist(r))
	results = append(results, loadBrewMaintenanceDaemon(r))
	return results
}

func writeBrewMaintenanceScript(r *Runner) Result {
	if _, err := os.Stat(brewMaintenanceScriptPath); err == nil {
		return SkipResult("brew-maintenance-script", brewMaintenanceScriptPath+" already exists")
	}
	if r.DryRun {
		return OKResult("brew-maintenance-script", "would write "+brewMaintenanceScriptPath+" (dry-run)")
	}
	if err := os.WriteFile(brewMaintenanceScriptPath, []byte(brewMaintenanceScript), 0755); err != nil {
		return FailResult("brew-maintenance-script", "failed to write "+brewMaintenanceScriptPath, err)
	}
	return OKResult("brew-maintenance-script", "wrote "+brewMaintenanceScriptPath)
}

func writeBrewMaintenancePlist(r *Runner) Result {
	if _, err := os.Stat(brewMaintenancePlistPath); err == nil {
		return SkipResult("brew-maintenance-plist", brewMaintenancePlistPath+" already exists")
	}
	if r.DryRun {
		return OKResult("brew-maintenance-plist", "would write "+brewMaintenancePlistPath+" (dry-run)")
	}
	if err := os.WriteFile(brewMaintenancePlistPath, []byte(brewMaintenancePlistContent), 0644); err != nil {
		return FailResult("brew-maintenance-plist", "failed to write "+brewMaintenancePlistPath, err)
	}
	if err := os.Chown(brewMaintenancePlistPath, 0, 0); err != nil {
		return FailResult("brew-maintenance-plist", "failed to set root ownership on "+brewMaintenancePlistPath, err)
	}
	return OKResult("brew-maintenance-plist", "wrote "+brewMaintenancePlistPath)
}

func loadBrewMaintenanceDaemon(r *Runner) Result {
	if _, err := r.Read("launchctl", "print", "system/"+brewMaintenanceLabel); err == nil {
		return SkipResult("brew-maintenance-daemon", brewMaintenanceLabel+" already loaded")
	}
	out, err := r.Run("launchctl", "bootstrap", "system", brewMaintenancePlistPath)
	if err != nil {
		return FailResult("brew-maintenance-daemon", "launchctl bootstrap failed: "+out, err)
	}
	return OKResult("brew-maintenance-daemon", brewMaintenanceLabel+" loaded — runs daily at 03:00")
}
