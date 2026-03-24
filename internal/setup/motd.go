package setup

import (
	"os"
	"strings"
)

const (
	motdScriptPath = "/opt/macsetup/update-motd"
	motdPlistPath  = "/Library/LaunchDaemons/com.macsetup.update-motd.plist"
	motdLabel      = "com.macsetup.update-motd"
)

// motdScript is the shell script written to /opt/macsetup/update-motd.
// It runs every 5 minutes via launchd and writes current system info to
// /etc/motd, which sshd displays after each successful login.
//
// Multiple non-loopback IPv4 addresses are each printed on their own line,
// aligned beneath the "IP:" label so the output stays readable on machines
// with several interfaces or VPN tunnels active.
const motdScript = `#!/bin/bash
# update-motd — writes system info to /etc/motd on each run.
# Managed by setupmac; edit /opt/macsetup/update-motd to customise.

HOSTNAME=$(scutil --get LocalHostName 2>/dev/null || hostname)
OS_NAME=$(sw_vers -productName)
OS_VER=$(sw_vers -productVersion)
ARCH=$(uname -m)

# uptime(1) on macOS: "... up 3 days, 4:12, 2 users, load averages: 0.42 0.38 0.31"
UPTIME=$(uptime | sed -E 's/.*up ([^,]+,[^,]+),.*/\1/' | xargs)
LOAD=$(uptime | awk -F'load averages:' '{print $2}' | xargs)

CPU=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "Unknown")

MEM_TOTAL=$(sysctl -n hw.memsize | awk '{printf "%.0f GB", $1/1073741824}')
PAGE_SIZE=$(vm_stat | awk '/page size/ {print $8}')
USED_PAGES=$(vm_stat | awk '
  /Pages active:/   { gsub(/\./, "", $3); a=$3 }
  /Pages wired/     { gsub(/\./, "", $4); w=$4 }
  END               { print a+w }
')
MEM_USED=$(echo "$PAGE_SIZE $USED_PAGES" | awk '{printf "%.1f GB", $1*$2/1073741824}')

DISK=$(df -h / | awk 'NR==2 {printf "%s used / %s total", $3, $2}')

# All non-loopback, non-link-local IPv4 addresses with their interface name.
# First entry is printed inline with the "IP:" label; subsequent entries are
# indented to align beneath it (11 spaces = length of "  IP:      ").
IPS=$(ifconfig \
  | awk '/^[a-z]/ {iface=$1; gsub(/:$/, "", iface)} \
         /inet / && !/127\.0\.0\.1/ && !/169\.254\./ \
         {print $2, "(" iface ")"}' \
  | awk 'NR==1 {print; next} {printf "           %s\n", $0}')

# Update checks run in the background and cache results for 1 hour so this
# script never blocks on network calls. On the very first run caches won't
# exist yet; the next 5-minute tick will show real counts.
NOW=$(date +%s)
TTL=3600

SW_CACHE=/var/run/update-motd-sw-updates
SW_AGE=$(( NOW - $(stat -f %m "$SW_CACHE" 2>/dev/null || echo 0) ))
if [ ! -f "$SW_CACHE" ] || [ "$SW_AGE" -gt "$TTL" ]; then
    softwareupdate -l > "$SW_CACHE" 2>&1 &
fi

BREW_CACHE=/var/run/update-motd-brew-outdated
BREW_AGE=$(( NOW - $(stat -f %m "$BREW_CACHE" 2>/dev/null || echo 0) ))
if [ ! -f "$BREW_CACHE" ] || [ "$BREW_AGE" -gt "$TTL" ]; then
    /opt/macsetup/brew outdated --greedy > "$BREW_CACHE" 2>&1 &
fi

OS_UPDATES=""
if [ -f "$SW_CACHE" ]; then
    SW_COUNT=$(grep -c '^\*' "$SW_CACHE" 2>/dev/null || true)
    if [ "${SW_COUNT:-0}" -gt 0 ]; then
        OS_UPDATES="macOS has ${SW_COUNT} update(s)  (sudo softwareupdate -ia)"
    fi
fi

BREW_UPDATES=""
if [ -f "$BREW_CACHE" ]; then
    BREW_COUNT=$(grep -c '.' "$BREW_CACHE" 2>/dev/null || true)
    if [ "${BREW_COUNT:-0}" -gt 0 ]; then
        BREW_UPDATES="brew has ${BREW_COUNT} update(s)  (brew upgrade --greedy)"
    fi
fi

SEP=$(printf '=%.0s' {1..51})

cat > /etc/motd << MOTD

${SEP}
${HOSTNAME}
${SEP}
  OS:      ${OS_NAME} ${OS_VER}
  Arch:    ${ARCH}
  Uptime:  ${UPTIME}
  Load:    ${LOAD}
  CPU:     ${CPU}
  Memory:  ${MEM_USED} used / ${MEM_TOTAL} total
  Disk:    ${DISK} (/)
  IP:      ${IPS}
${SEP}
${BREW_UPDATES}
${OS_UPDATES}

MOTD
`

// motdPlist is the launchd daemon plist that runs update-motd at boot and
// every 5 minutes (300 seconds) thereafter.
const motdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.macsetup.update-motd</string>
    <key>ProgramArguments</key>
    <array>
        <string>/opt/macsetup/update-motd</string>
    </array>
    <key>StartInterval</key>
    <integer>300</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/update-motd.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/update-motd.log</string>
</dict>
</plist>
`

// SetupMOTD writes a shell script that generates a system-info summary and
// installs a launchd daemon that runs it every 5 minutes. The result is a
// fresh /etc/motd displayed by sshd after each successful login, showing the
// hostname, OS, uptime, load, CPU, memory, disk, and all active IP addresses.
func SetupMOTD(r *Runner) []Result {
	var results []Result

	results = append(results, writeMotdScript(r))
	results = append(results, writeMotdPlist(r))
	results = append(results, loadMotdDaemon(r))

	return results
}

func writeMotdScript(r *Runner) Result {
	// Skip if the script already exists with identical content — avoids
	// reloading the daemon unnecessarily on repeated runs.
	if existing, err := os.ReadFile(motdScriptPath); err == nil {
		if strings.TrimRight(string(existing), "\n") == strings.TrimRight(motdScript, "\n") {
			return SkipResult("motd-script", motdScriptPath+" already up to date")
		}
	}

	if r.DryRun {
		return OKResult("motd-script", "would write "+motdScriptPath+" (dry-run)")
	}

	if err := os.WriteFile(motdScriptPath, []byte(motdScript), 0755); err != nil {
		return FailResult("motd-script", "failed to write "+motdScriptPath, err)
	}
	return OKResult("motd-script", "wrote "+motdScriptPath)
}

func writeMotdPlist(r *Runner) Result {
	if _, err := os.Stat(motdPlistPath); err == nil {
		return SkipResult("motd-plist", motdPlistPath+" already exists")
	}

	if r.DryRun {
		return OKResult("motd-plist", "would write "+motdPlistPath+" (dry-run)")
	}

	if err := os.WriteFile(motdPlistPath, []byte(motdPlist), 0644); err != nil {
		return FailResult("motd-plist", "failed to write "+motdPlistPath, err)
	}
	// LaunchDaemons plists must be owned by root:wheel.
	if err := os.Chown(motdPlistPath, 0, 0); err != nil {
		return FailResult("motd-plist", "failed to set root ownership on "+motdPlistPath, err)
	}
	return OKResult("motd-plist", "wrote "+motdPlistPath)
}

func loadMotdDaemon(r *Runner) Result {
	// launchctl print exits 0 when the service is already loaded.
	if _, err := r.Read("launchctl", "print", "system/"+motdLabel); err == nil {
		return SkipResult("motd-daemon", motdLabel+" already loaded")
	}

	out, err := r.Run("launchctl", "bootstrap", "system", motdPlistPath)
	if err != nil {
		return FailResult("motd-daemon", "launchctl bootstrap failed: "+out, err)
	}
	return OKResult("motd-daemon", motdLabel+" loaded — /etc/motd will be written momentarily")
}
