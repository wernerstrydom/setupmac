package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"golang.org/x/term"

	"github.com/wstrydom/setupmac/internal/macos"
	"github.com/wstrydom/setupmac/internal/setup"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z".
// commit, build time, and dirty flag are read from VCS info embedded
// automatically by the Go toolchain (Go 1.18+).
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		printVersion()
		return
	}

	dryRun := flag.Bool("dry-run", false, "Print commands without executing")
	username := flag.String("username", "", "Username to configure for auto-login")
	vncPassword := flag.String("vnc-password", "", "VNC password for ARD (omit to skip)")
	skipFileVault := flag.Bool("skip-filevault", false, "Skip FileVault disable step")
	githubKeysUser := flag.String("github-keys-user", "",
		"GitHub username whose SSH keys are fetched and installed (prompt if omitted)")
	bannerOrg := flag.String("banner-org", "",
		"Organization name for the login/SSH banner (prompt if omitted)")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "error: unexpected arguments — setupmac takes flags only")
		flag.Usage()
		os.Exit(2)
	}

	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "error: setupmac must be run as root (use: sudo setupmac)")
		os.Exit(1)
	}

	ver, err := macos.Detect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not detect macOS version: %v\n", err)
		os.Exit(1)
	}

	printHeader(ver)

	r := &setup.Runner{DryRun: *dryRun}
	var all []setup.Result

	// Phase 1: inspect the host so we only ask questions that are relevant.
	fmt.Fprintln(os.Stderr, "Inspecting host...")
	state := setup.Inspect(r)

	// Phase 2: collect all user input upfront — no prompts during execution.
	cfg := loadConfig()
	plan := gatherInputs(*dryRun, *skipFileVault, *username, *githubKeysUser, *bannerOrg, cfg, state)
	keyUsers := collectKeyUsers(plan.Username)

	fmt.Fprintln(os.Stderr)

	printSection("Power Management")
	res := setup.ConfigurePower(r)
	printResult(res)
	all = append(all, res)

	printSection("Bluetooth Setup Assistant")
	for _, res := range setup.DisableBluetoothAssistant(r) {
		printResult(res)
		all = append(all, res)
	}

	printSection("Universal Control")
	res = setup.DisableUniversalControl(r, ver)
	printResult(res)
	all = append(all, res)

	printSection("Screen Saver")
	res = setup.DisableLoginScreenSaver(r)
	printResult(res)
	all = append(all, res)

	printSection("Guest Account")
	res = setup.DisableGuestAccount(r)
	printResult(res)
	all = append(all, res)

	printSection("ARD / Remote Management")
	for _, res := range setup.ConfigureARD(r, ver, *vncPassword) {
		printResult(res)
		all = append(all, res)
	}

	printSection("FileVault")
	var fvResult setup.Result
	if *skipFileVault {
		fvResult = setup.SkipResult("filevault", "--skip-filevault flag set")
	} else {
		fvResult = setup.DisableFileVault(r, plan.FileVaultPassword)
	}
	printResult(fvResult)
	all = append(all, fvResult)

	printSection("Auto-login")
	autoResults := setup.EnableAutoLogin(r, plan.Username, plan.AutoLoginPassword)
	for _, res := range autoResults {
		printResult(res)
		all = append(all, res)
	}
	// Warn when auto-login was configured but FileVault may still be on.
	if plan.Username != "" && fvResult.Status == setup.Fail {
		warn := setup.WarnResult("autologin-fv-warn",
			"FileVault may still be enabled — auto-login requires FileVault off")
		printResult(warn)
		all = append(all, warn)
	}

	printSection("Login Banner")
	bannerFile := ""
	if plan.BannerOrg != "" {
		var bannerResults []setup.Result
		bannerFile, bannerResults = setup.SetupBanner(r, plan.BannerOrg)
		for _, res := range bannerResults {
			printResult(res)
			all = append(all, res)
		}
	} else {
		res := setup.SkipResult("banner", "no organization name provided")
		printResult(res)
		all = append(all, res)
	}

	printSection("SSH Keys")
	var keysInstalled bool
	if plan.GitHubKeysUser != "" {
		for _, res := range setup.InstallGitHubKeys(r, plan.GitHubKeysUser, keyUsers) {
			printResult(res)
			all = append(all, res)
			if res.Status == setup.OK {
				keysInstalled = true
			}
		}
	} else {
		res := setup.SkipResult("ssh-keys", "no GitHub username provided")
		printResult(res)
		all = append(all, res)
	}

	printSection("SSH Hardening")
	for _, res := range setup.HardenSSH(r, keysInstalled, bannerFile) {
		printResult(res)
		all = append(all, res)
	}

	printSection("Application Firewall")
	for _, res := range setup.ConfigureFirewall(r, ver) {
		printResult(res)
		all = append(all, res)
	}

	printSection("Siri")
	res = setup.DisableSiri(r)
	printResult(res)
	all = append(all, res)

	printSection("Homebrew (Multi-User)")
	for _, res := range setup.SetupHomebrew(r, ver) {
		printResult(res)
		all = append(all, res)
	}
	fmt.Println()
	fmt.Println("  NOTE: open a new terminal (or run: source /etc/zprofile) before")
	fmt.Println("  using brew, so the wrapper at /opt/macsetup/brew is on PATH.")

	printSection("Verification")
	verResults := setup.VerifyAll(r, ver, plan.Username)
	for _, res := range verResults {
		printResult(res)
	}
	all = append(all, verResults...)

	saveConfig(savedConfig{
		Username:       plan.Username,
		GitHubKeysUser: plan.GitHubKeysUser,
		BannerOrg:      plan.BannerOrg,
	})

	printSummary(all)

	for _, r := range all {
		if r.Status == setup.Fail {
			os.Exit(1)
		}
	}
}

func printHeader(ver macos.Version) {
	name := ver.MarketingName()
	label := ver.String()
	if name != "" {
		label = fmt.Sprintf("%s (%s)", ver.String(), name)
	}
	fmt.Printf("=== setupmac %s ===\n", version)
	fmt.Printf("macOS %s on %s\n", label, runtime.GOARCH)
}

func printSection(name string) {
	fmt.Printf("\n[%s]\n", name)
}

func printResult(res setup.Result) {
	sym := symbol(res.Status)
	msg := res.Message
	if res.Err != nil && !strings.Contains(msg, res.Err.Error()) {
		msg = fmt.Sprintf("%s (%v)", msg, res.Err)
	}
	fmt.Printf("  %s %s\n", sym, msg)
}

func symbol(s setup.Status) string {
	switch s {
	case setup.OK:
		return "\u2713" // ✓
	case setup.Skip:
		return "\u2014" // —
	case setup.Fail:
		return "\u2717" // ✗
	default:
		return "-"
	}
}

func printVersion() {
	commit, buildTime, modified := vcsInfo()
	fmt.Printf("setupmac %s\n", version)
	fmt.Printf("  commit:  %s", commit)
	if modified {
		fmt.Print(" (modified)")
	}
	fmt.Println()
	fmt.Printf("  built:   %s\n", buildTime)
	fmt.Printf("  go:      %s\n", runtime.Version())
	fmt.Printf("  arch:    %s\n", runtime.GOARCH)
}

// vcsInfo extracts the commit hash, build time, and dirty flag from the VCS
// metadata that the Go toolchain embeds in every binary since Go 1.18.
func vcsInfo() (commit, buildTime string, modified bool) {
	commit, buildTime = "unknown", "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
			if len(commit) > 7 {
				commit = commit[:7]
			}
		case "vcs.time":
			buildTime = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return
}

func printSummary(results []setup.Result) {
	counts := map[setup.Status]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	fmt.Printf("\nSummary: %d OK, %d SKIP, %d FAIL\n",
		counts[setup.OK], counts[setup.Skip], counts[setup.Fail])
}

// setupPlan holds all user-supplied inputs collected before execution begins.
// Passwords are not saved to the config file.
type setupPlan struct {
	Username          string
	GitHubKeysUser    string
	BannerOrg         string
	FileVaultPassword string // non-empty only when FileVault is currently on
	AutoLoginPassword string // non-empty only when a (new) password is needed
}

// gatherInputs inspects the host state and collects all required inputs
// upfront so that no prompts interrupt execution.
func gatherInputs(dryRun, skipFileVault bool, username, githubKeysUser, bannerOrg string, cfg savedConfig, state setup.HostState) setupPlan {
	var plan setupPlan

	plan.Username = resolveWithSaved(username, cfg.Username, "Username for auto-login")
	plan.GitHubKeysUser = resolveWithSaved(githubKeysUser, cfg.GitHubKeysUser, "GitHub username for SSH keys")
	plan.BannerOrg = resolveWithSaved(bannerOrg, cfg.BannerOrg, "Organization name for login banner")

	if !skipFileVault && state.FileVaultEnabled && !dryRun {
		plan.FileVaultPassword = promptPassword("FileVault is enabled. Enter administrator password to disable")
	}

	if plan.Username != "" && !dryRun {
		alreadyConfigured := state.AutoLoginUser == plan.Username && state.KCPasswordExists
		if alreadyConfigured {
			if confirmYN(fmt.Sprintf("Auto-login already configured for %s. Update password?", plan.Username), false) {
				plan.AutoLoginPassword = promptPassword("Enter new login password for " + plan.Username)
			}
		} else {
			plan.AutoLoginPassword = promptPassword("Enter login password for " + plan.Username)
		}
	}

	return plan
}

// resolveWithSaved returns flagVal when it is non-empty. Otherwise it prompts
// interactively, displaying savedVal in brackets as the default — pressing
// Enter accepts it. Returns the empty string to skip when nothing is entered
// and there is no saved value.
func resolveWithSaved(flagVal, savedVal, prompt string) string {
	if flagVal != "" {
		return flagVal
	}
	if savedVal != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", prompt, savedVal)
	} else {
		fmt.Fprintf(os.Stderr, "%s (Enter to skip): ", prompt)
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return savedVal
	}
	input := strings.TrimSpace(line)
	if input == "" {
		return savedVal
	}
	return input
}

// confirmYN prints a yes/no prompt and returns true if the operator answers y.
// defaultYes controls what an empty (Enter) response means.
func confirmYN(question string, defaultYes bool) bool {
	choices := "[y/N]"
	if defaultYes {
		choices = "[Y/n]"
	}
	fmt.Fprintf(os.Stderr, "%s %s: ", question, choices)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

// promptPassword writes prompt to stderr, reads a password without echo,
// and returns it. Returns empty string on any read error.
func promptPassword(prompt string) string {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)
	pwd, _ := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(pwd)
}

// collectKeyUsers builds the deduplicated list of local users who receive the
// GitHub SSH keys: always root (emergency access), the sudo-invoking operator
// ($SUDO_USER), and the --username auto-login user when provided.
func collectKeyUsers(username string) []string {
	seen := map[string]bool{}
	var users []string
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			users = append(users, u)
		}
	}
	add("root")
	add(os.Getenv("SUDO_USER"))
	add(username)
	return users
}
