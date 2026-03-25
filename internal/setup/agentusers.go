package setup

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/wstrydom/setupmac/internal/macos"
)

const (
	agentGroupName   = "ai-agents"
	agentSudoersPath = "/etc/sudoers.d/ai-agents"
)

// AgentUserNames is the full list of AI agent and runner service accounts.
var AgentUserNames = []string{"_claude", "_gemini", "_codex", "_copilot", "_runner"}

type agentSpec struct {
	name, fullName string
}

var agentSpecs = []agentSpec{
	{"_claude", "Claude AI Agent"},
	{"_gemini", "Gemini AI Agent"},
	{"_codex", "Codex AI Agent"},
	{"_copilot", "GitHub Copilot Agent"},
	{"_runner", "GitHub Actions Runner"},
}

// SetupAgentUsers creates hidden, non-admin service accounts for AI coding
// agents and the GitHub Actions runner. For each account it:
//   - creates the account with /usr/bin/false shell (no interactive login)
//   - adds it to the ai-agents group
//   - installs GitHub public SSH keys if a GitHub username is provided
//
// It also writes /etc/sudoers.d/ai-agents (scoped per-tool delegation) and
// initialises a Podman machine for _runner so it is ready to run containers.
func SetupAgentUsers(r *Runner, ver macos.Version, githubUser string) []Result {
	var results []Result

	res := createAgentGroup(r)
	results = append(results, res)
	if res.Status == Fail {
		return results
	}

	// Pre-fetch GitHub SSH keys once to avoid one HTTP request per user.
	var keys []string
	if githubUser != "" && !r.DryRun {
		var err error
		keys, err = fetchGitHubKeys(githubUser)
		if err != nil {
			results = append(results, WarnResult("agent-ssh-keys",
				fmt.Sprintf("fetch github.com/%s.keys: %v — SSH keys not installed for agent accounts", githubUser, err)))
		}
	}

	for _, spec := range agentSpecs {
		res := createAgentUser(r, ver, spec.name, spec.fullName)
		results = append(results, res)
		if res.Status == Fail {
			continue
		}
		results = append(results, addUserToAgentGroup(r, spec.name))
		if len(keys) > 0 {
			results = append(results, installKeysForUser(spec.name, keys))
		}
	}

	results = append(results, writeAgentSudoers(r))
	results = append(results, initRunnerPodman(r))

	return results
}

// createAgentGroup creates the ai-agents group if it does not already exist.
func createAgentGroup(r *Runner) Result {
	if _, err := r.Read("dseditgroup", "-o", "read", agentGroupName); err == nil {
		return SkipResult("agent-group", "group "+agentGroupName+" already exists")
	}

	out, err := r.Run("dseditgroup", "-o", "create", "-r", "AI Agents", agentGroupName)
	if err != nil {
		return FailResult("agent-group", out, err)
	}
	return OKResult("agent-group", "created group "+agentGroupName)
}

// createAgentUser creates a hidden, non-admin service account. It mirrors the
// createBrewUser pattern but does not grant admin membership and sets the login
// shell to /usr/bin/false so the account cannot be used interactively.
func createAgentUser(r *Runner, ver macos.Version, name, fullName string) Result {
	step := "agent-user[" + name + "]"

	if _, err := user.Lookup(name); err == nil {
		return SkipResult(step, fmt.Sprintf("user %q already exists", name))
	}

	password, err := randomBase64(16)
	if err != nil {
		return FailResult(step, "failed to generate random password", err)
	}

	homeDir := "/var/" + name

	if out, err := r.Run("sysadminctl",
		"-addUser", name,
		"-fullName", fullName,
		"-password", password,
		"-home", homeDir,
	); err != nil {
		return FailResult(step, out, err)
	}

	// Lock the login shell — these accounts are invoked via sudo, never directly.
	if out, err := r.Run("dscl", ".", "-create",
		"/Users/"+name, "UserShell", "/usr/bin/false"); err != nil {
		return FailResult(step, out, err)
	}

	// Hide from the login window using the same version-gated approach as createBrewUser.
	if ver.AtLeast(13, 0) {
		if out, err := r.Run("defaults", "write",
			"/Library/Preferences/com.apple.loginwindow",
			"HiddenUsersList", "-array-add", name); err != nil {
			return FailResult(step, out, err)
		}
	} else {
		if out, err := r.Run("dscl", ".", "-create",
			"/Users/"+name, "IsHidden", "1"); err != nil {
			return FailResult(step, out, err)
		}
	}

	if !r.DryRun {
		// 0700: only the account owner can access its own home directory.
		if err := os.MkdirAll(homeDir, 0700); err != nil {
			return FailResult(step,
				fmt.Sprintf("failed to create home directory %s: %v", homeDir, err), err)
		}
		if err := chownPath(homeDir, name, "staff"); err != nil {
			return FailResult(step,
				fmt.Sprintf("failed to set ownership of %s: %v", homeDir, err), err)
		}
	}

	return OKResult(step, fmt.Sprintf("created hidden service user %q", name))
}

// addUserToAgentGroup adds name to the ai-agents group if not already a member.
func addUserToAgentGroup(r *Runner, name string) Result {
	step := "agent-group[" + name + "]"

	if _, err := r.Read("dseditgroup", "-o", "checkmember", "-m", name, agentGroupName); err == nil {
		return SkipResult(step, fmt.Sprintf("user %q is already in group %s", name, agentGroupName))
	}

	out, err := r.Run("dseditgroup", "-o", "edit", "-a", name, "-t", "user", agentGroupName)
	if err != nil {
		return FailResult(step, out, err)
	}
	return OKResult(step, fmt.Sprintf("added %q to group %s", name, agentGroupName))
}

// writeAgentSudoers writes /etc/sudoers.d/ai-agents, granting staff members
// passwordless delegation to each AI agent account for its specific tool
// binary only. The runner account is limited to podman. Guest is denied.
// Both ARM64 (/opt/homebrew) and Intel (/usr/local) binary paths are listed
// so the rule works on either architecture without modification.
func writeAgentSudoers(r *Runner) Result {
	prefix := BrewPrefix()

	content := fmt.Sprintf(
		"# Passwordless delegation: staff → each AI agent's dedicated tool binary\n"+
			"%%staff ALL=(_claude)  NOPASSWD: %s/bin/claude,   /usr/local/bin/claude\n"+
			"%%staff ALL=(_gemini)  NOPASSWD: %s/bin/gemini,   /usr/local/bin/gemini\n"+
			"%%staff ALL=(_codex)   NOPASSWD: %s/bin/codex,    /usr/local/bin/codex\n"+
			"%%staff ALL=(_copilot) NOPASSWD: %s/bin/copilot,  /usr/local/bin/copilot\n"+
			"# Allow staff to run containers as _runner (Podman only)\n"+
			"%%staff ALL=(_runner)  NOPASSWD: %s/bin/podman,   /usr/local/bin/podman\n"+
			"# Guest account must never bypass authentication\n"+
			"Guest   ALL=(_claude, _gemini, _codex, _copilot, _runner) !ALL\n",
		prefix, prefix, prefix, prefix, prefix,
	)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s:\n    %s", agentSudoersPath, content)
		return OKResult("agent-sudoers", fmt.Sprintf("would write %s (dry-run)", agentSudoersPath))
	}

	tmp, err := os.CreateTemp("/etc", ".ai-agents-sudoers-*")
	if err != nil {
		return FailResult("agent-sudoers", "failed to create temp file in /etc", err)
	}
	tmpPath := tmp.Name()

	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return FailResult("agent-sudoers", "failed to write sudoers content", err)
	}
	tmp.Close()

	if out, err := r.Read("visudo", "-c", "-f", tmpPath); err != nil {
		return FailResult("agent-sudoers",
			fmt.Sprintf("visudo validation failed: %s", out), err)
	}

	if err := os.Chown(tmpPath, 0, 0); err != nil {
		return FailResult("agent-sudoers", "failed to set sudoers ownership", err)
	}
	if err := os.Chmod(tmpPath, 0440); err != nil {
		return FailResult("agent-sudoers", "failed to set sudoers permissions", err)
	}

	if err := os.Rename(tmpPath, agentSudoersPath); err != nil {
		return FailResult("agent-sudoers",
			fmt.Sprintf("failed to place %s: %v", agentSudoersPath, err), err)
	}

	committed = true
	return OKResult("agent-sudoers", fmt.Sprintf("wrote %s", agentSudoersPath))
}

// initRunnerPodman initialises and starts a Podman machine as _runner so the
// account is ready to execute containers. The machine is also configured to
// auto-start after a host reboot via a launchd agent.
func initRunnerPodman(r *Runner) Result {
	// Check whether a machine already exists for _runner.
	out, _ := r.Read("sudo", "-n", "-H", "-u", "_runner", "podman", "machine", "list")
	if strings.Contains(out, "podman-machine-default") {
		return SkipResult("runner-podman", "Podman machine already exists for _runner")
	}

	if r.DryRun {
		fmt.Println("  [dry-run] sudo -H -u _runner podman machine init --now --cpus 2 --memory 4096 --disk-size 50")
		return OKResult("runner-podman", "would initialise Podman machine for _runner (dry-run)")
	}

	if err := r.RunLive("sudo", "-n", "-H", "-u", "_runner",
		"podman", "machine", "init", "--now",
		"--cpus", "2", "--memory", "4096", "--disk-size", "50",
	); err != nil {
		return FailResult("runner-podman", "podman machine init failed", err)
	}

	// Register a launchd agent so the VM restarts automatically after a reboot.
	if out, err := r.Run("sudo", "-n", "-H", "-u", "_runner",
		"podman", "machine", "set", "--autostart=true"); err != nil {
		return FailResult("runner-podman", out, err)
	}

	return OKResult("runner-podman", "Podman machine initialised and set to auto-start for _runner")
}
