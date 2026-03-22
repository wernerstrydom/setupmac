package setup

import (
	"fmt"
	"os"
)

const (
	sshBannerPath   = "/etc/ssh/banner"
	motdPath        = "/etc/motd"
	loginBannerDir  = "/Library/Security"
	loginBannerPath = "/Library/Security/PolicyBanner.txt"
)

// bannerText is the standard authorized-use-only notice written to the SSH
// pre-auth banner, MOTD, and macOS login window. The single %s verb is
// replaced with the organization name supplied via --banner-org.
const bannerText = `
AUTHORIZED ACCESS ONLY — %s

Unauthorized use is prohibited. All activity is monitored and logged.
Disconnect immediately if you are not an authorized user.
`

// SetupBanner writes a standard authorized-use-only notice to:
//   - /etc/ssh/banner        (SSH pre-authentication banner)
//   - /etc/motd              (message of the day, shown after login)
//   - /Library/Security/PolicyBanner.txt  (macOS login window notice)
//
// The SSH banner path is returned so the caller can pass it to HardenSSH,
// which sets the Banner directive in sshd_config. An empty string is returned
// when the step is skipped or fails.
func SetupBanner(r *Runner, orgName string) (string, []Result) {
	text := fmt.Sprintf(bannerText, orgName)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s\n", sshBannerPath)
		fmt.Printf("  [dry-run] write %s\n", motdPath)
		fmt.Printf("  [dry-run] write %s\n", loginBannerPath)
		return sshBannerPath, []Result{OKResult("banner",
			fmt.Sprintf("would write login banner for %q (dry-run)", orgName))}
	}

	if err := os.WriteFile(sshBannerPath, []byte(text), 0644); err != nil {
		return "", []Result{FailResult("banner",
			fmt.Sprintf("write %s: %v", sshBannerPath, err), err)}
	}

	if err := os.WriteFile(motdPath, []byte(text), 0644); err != nil {
		return "", []Result{FailResult("banner",
			fmt.Sprintf("write %s: %v", motdPath, err), err)}
	}

	if err := os.MkdirAll(loginBannerDir, 0755); err != nil {
		return "", []Result{FailResult("banner",
			fmt.Sprintf("create %s: %v", loginBannerDir, err), err)}
	}
	if err := os.WriteFile(loginBannerPath, []byte(text), 0644); err != nil {
		return "", []Result{FailResult("banner",
			fmt.Sprintf("write %s: %v", loginBannerPath, err), err)}
	}

	return sshBannerPath, []Result{OKResult("banner",
		fmt.Sprintf("login banner set for %q", orgName))}
}
