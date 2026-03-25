package setup

import (
	"fmt"
	"os"
)

const (
	agentWorkspaceDir = "/Users/Shared/Workspace"
	workspaceReposDir = "/Users/Shared/Workspace/repos"
	workspaceCacheDir = "/Users/Shared/Workspace/cache"

	// workspaceACL is the macOS ACL entry applied to each workspace directory.
	// file_inherit and directory_inherit make it propagate automatically to all
	// new content created inside — the macOS equivalent of Linux setgid.
	workspaceACL = "group:ai-agents allow list,add_file,search,add_subdirectory," +
		"delete_child,readattr,writeattr,readextattr,writeextattr," +
		"readsecurity,file_inherit,directory_inherit"
)

// SetupAgentWorkspace creates /Users/Shared/Workspace (and repos/ + cache/
// subdirectories) as a shared area for AI agent code checkouts. Each directory
// is owned by root:ai-agents with mode 0770 and an inheritable macOS ACL so
// all files and subdirectories created inside automatically carry the same
// ai-agents access without further permission management.
func SetupAgentWorkspace(r *Runner) []Result {
	var results []Result
	for _, dir := range []string{agentWorkspaceDir, workspaceReposDir, workspaceCacheDir} {
		results = append(results, setupWorkspaceDir(r, dir))
	}
	return results
}

func setupWorkspaceDir(r *Runner, dir string) Result {
	step := "workspace[" + dir + "]"

	if r.DryRun {
		fmt.Printf("  [dry-run] mkdir -p %s\n", dir)
		fmt.Printf("  [dry-run] chown root:%s %s && chmod 0770 %s\n", agentGroupName, dir, dir)
		fmt.Printf("  [dry-run] chmod +a %q %s\n", workspaceACL, dir)
		return OKResult(step, fmt.Sprintf("would create %s (dry-run)", dir))
	}

	if err := os.MkdirAll(dir, 0770); err != nil {
		return FailResult(step, fmt.Sprintf("failed to create %s: %v", dir, err), err)
	}

	if err := chownPath(dir, "root", agentGroupName); err != nil {
		return FailResult(step, fmt.Sprintf("failed to set ownership of %s: %v", dir, err), err)
	}

	if err := os.Chmod(dir, 0770); err != nil {
		return FailResult(step, fmt.Sprintf("failed to chmod %s: %v", dir, err), err)
	}

	// Apply an inheritable ACL so content created inside carries ai-agents
	// access automatically. Go's os.Chmod only sets POSIX bits, so we shell out.
	if out, err := r.Run("chmod", "+a", workspaceACL, dir); err != nil {
		return FailResult(step, out, err)
	}

	return OKResult(step, fmt.Sprintf("created %s with ai-agents ACL", dir))
}
