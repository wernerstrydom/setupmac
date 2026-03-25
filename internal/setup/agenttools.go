package setup

import (
	"fmt"
	"os"
)

// agentToolSpec describes one AI CLI tool: how to install it and the service
// account its wrapper script delegates to.
type agentToolSpec struct {
	step       string
	brewArgs   []string // args passed to the brew wrapper; nil means npm install
	npmPkg     string   // npm package name; empty when installed via brew
	binaryName string   // binary filename in brew prefix
	wrapUser   string   // service account the wrapper delegates to
}

var agentToolSpecs = []agentToolSpec{
	{
		step:       "claude",
		brewArgs:   []string{"install", "--cask", "claude-code"},
		binaryName: "claude",
		wrapUser:   "_claude",
	},
	{
		step:       "gemini",
		brewArgs:   []string{"install", "gemini-cli"},
		binaryName: "gemini",
		wrapUser:   "_gemini",
	},
	{
		step:       "codex",
		npmPkg:     "@openai/codex",
		binaryName: "codex",
		wrapUser:   "_codex",
	},
	{
		step:       "copilot",
		brewArgs:   []string{"install", "--cask", "copilot-cli"},
		binaryName: "copilot",
		wrapUser:   "_copilot",
	},
}

// InstallAgentTools installs the AI CLI tools system-wide via Homebrew (casks
// and formulae) or npm, then creates wrapper scripts in /opt/macsetup/ that
// transparently delegate each invocation to the tool's dedicated service
// account — mirroring the brew wrapper pattern.
func InstallAgentTools(r *Runner) []Result {
	if _, err := os.Stat(brewWrapperPath); err != nil {
		return []Result{WarnResult("agent-tools",
			"Homebrew not installed — run Homebrew (Multi-User) setup first")}
	}

	var results []Result
	for _, spec := range agentToolSpecs {
		results = append(results, installAgentTool(r, spec))
	}
	results = append(results, createAgentWrappers(r)...)
	return results
}

// installAgentTool installs a single AI CLI tool via Homebrew or npm.
// The install is idempotent: if the binary already exists the step is skipped.
func installAgentTool(r *Runner, spec agentToolSpec) Result {
	step := "agent-tool-" + spec.step
	binaryPath := BrewPrefix() + "/bin/" + spec.binaryName

	if _, err := os.Stat(binaryPath); err == nil {
		return SkipResult(step, spec.binaryName+" already installed at "+binaryPath)
	}

	if spec.npmPkg != "" {
		// npm global install runs as _homebrew so it writes into the
		// Homebrew-owned lib/node_modules directory without privilege issues.
		npmPath := BrewPrefix() + "/bin/npm"
		out, err := r.Run("sudo", "-n", "-H", "-u", brewUserName,
			npmPath, "install", "-g", spec.npmPkg)
		if err != nil {
			return FailResult(step, out, err)
		}
	} else {
		out, err := r.Run(brewWrapperPath, spec.brewArgs...)
		if err != nil {
			return FailResult(step, out, err)
		}
	}

	return OKResult(step, "installed "+spec.binaryName)
}

// createAgentWrappers writes one wrapper script per tool into /opt/macsetup/.
// Each wrapper checks whether it is already running as the target service
// account; if so it execs the binary directly, otherwise it delegates via
// sudo. This mirrors the brew wrapper and avoids a sudo loop when the agent
// account itself invokes the tool.
func createAgentWrappers(r *Runner) []Result {
	var results []Result
	for _, spec := range agentToolSpecs {
		results = append(results, createAgentWrapper(r, spec))
	}
	return results
}

func createAgentWrapper(r *Runner, spec agentToolSpec) Result {
	step := "agent-wrapper-" + spec.step
	wrapperPath := brewWrapperDir + "/" + spec.binaryName
	realBinPath := BrewPrefix() + "/bin/" + spec.binaryName

	content := fmt.Sprintf("#!/bin/bash\n"+
		"# %s — transparent multi-user %s wrapper\n"+
		"# Delegates to %s via sudo (passwordless via %s),\n"+
		"# unless already running as %s to avoid a sudo loop.\n"+
		"if [ \"$(id -un)\" = \"%s\" ]; then\n"+
		"    exec %s \"$@\"\n"+
		"else\n"+
		"    exec sudo -n -H -u %s %s \"$@\"\n"+
		"fi\n",
		wrapperPath, spec.binaryName,
		spec.wrapUser, agentSudoersPath,
		spec.wrapUser,
		spec.wrapUser,
		realBinPath,
		spec.wrapUser, realBinPath,
	)

	if r.DryRun {
		fmt.Printf("  [dry-run] write %s\n", wrapperPath)
		return OKResult(step, fmt.Sprintf("would create %s (dry-run)", wrapperPath))
	}

	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		return FailResult(step,
			fmt.Sprintf("failed to write %s: %v", wrapperPath, err), err)
	}

	return OKResult(step, spec.binaryName+" wrapper created at "+wrapperPath)
}
