package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/wstrydom/setupmac/internal/macos"
	"github.com/wstrydom/setupmac/internal/setup"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Print commands without executing")
	username := flag.String("username", "", "Username to configure for auto-login")
	vncPassword := flag.String("vnc-password", "", "VNC password for ARD (omit to skip)")
	skipFileVault := flag.Bool("skip-filevault", false, "Skip FileVault disable step")
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
		fvResult = setup.DisableFileVault(r)
	}
	printResult(fvResult)
	all = append(all, fvResult)

	printSection("Auto-login")
	autoResults := setup.EnableAutoLogin(r, *username)
	for _, res := range autoResults {
		printResult(res)
		all = append(all, res)
	}
	// Warn when auto-login was configured but FileVault may still be on.
	if *username != "" && fvResult.Status == setup.Fail {
		warn := setup.WarnResult("autologin-fv-warn",
			"FileVault may still be enabled — auto-login requires FileVault off")
		printResult(warn)
		all = append(all, warn)
	}

	printSection("Homebrew (Multi-User)")
	for _, res := range setup.SetupHomebrew(r, ver) {
		printResult(res)
		all = append(all, res)
	}

	printSection("Verification")
	verResults := setup.VerifyAll(r, ver, *username)
	for _, res := range verResults {
		printResult(res)
	}
	all = append(all, verResults...)

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
	fmt.Println("=== headless-mac-setup ===")
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
		return "\u21b7" // ↷
	case setup.Fail:
		return "\u2717" // ✗
	default:
		return "-"
	}
}

func printSummary(results []setup.Result) {
	counts := map[setup.Status]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	fmt.Printf("\nSummary: %d OK, %d SKIP, %d FAIL\n",
		counts[setup.OK], counts[setup.Skip], counts[setup.Fail])
}
