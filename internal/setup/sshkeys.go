package setup

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// InstallGitHubKeys fetches public SSH keys from github.com/<githubUser>.keys
// and appends them to ~/.ssh/authorized_keys for each named local user.
//
// The install is idempotent: keys already present in the file are skipped.
// In dry-run mode the HTTP fetch is skipped and no files are written.
func InstallGitHubKeys(r *Runner, githubUser string, localUsers []string) []Result {
	if r.DryRun {
		for _, u := range localUsers {
			fmt.Printf("  [dry-run] fetch https://github.com/%s.keys → ~/.ssh/authorized_keys for %s\n", githubUser, u)
		}
		return []Result{OKResult("ssh-keys",
			fmt.Sprintf("would install keys from github.com/%s (dry-run)", githubUser))}
	}

	keys, err := fetchGitHubKeys(githubUser)
	if err != nil {
		return []Result{FailResult("ssh-keys",
			fmt.Sprintf("fetch github.com/%s.keys: %v — check network and GitHub username", githubUser, err), err)}
	}

	if len(keys) == 0 {
		return []Result{WarnResult("ssh-keys",
			fmt.Sprintf("github.com/%s has no public SSH keys — password auth will remain enabled", githubUser))}
	}

	var results []Result
	for _, name := range localUsers {
		results = append(results, installKeysForUser(name, keys))
	}
	return results
}

func fetchGitHubKeys(githubUser string) ([]string, error) {
	resp, err := http.Get("https://github.com/" + githubUser + ".keys") //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []string
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			keys = append(keys, line)
		}
	}
	return keys, nil
}

func installKeysForUser(name string, keys []string) Result {
	step := "ssh-keys[" + name + "]"

	u, err := user.Lookup(name)
	if err != nil {
		return FailResult(step, fmt.Sprintf("user %q not found", name), err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return FailResult(step, fmt.Sprintf("invalid uid for %s: %v", name, err), err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return FailResult(step, fmt.Sprintf("invalid gid for %s: %v", name, err), err)
	}

	sshDir := filepath.Join(u.HomeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return FailResult(step, fmt.Sprintf("create %s: %v", sshDir, err), err)
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return FailResult(step, fmt.Sprintf("chown %s: %v", sshDir, err), err)
	}

	authFile := filepath.Join(sshDir, "authorized_keys")

	// Build set of existing keys to avoid duplicates.
	existing := map[string]struct{}{}
	if data, err := os.ReadFile(authFile); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				existing[line] = struct{}{}
			}
		}
	}

	var toAdd []string
	for _, k := range keys {
		if _, found := existing[k]; !found {
			toAdd = append(toAdd, k)
		}
	}

	if len(toAdd) == 0 {
		return OKResult(step, fmt.Sprintf("all %d key(s) already present for %s", len(keys), name))
	}

	f, err := os.OpenFile(authFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return FailResult(step, fmt.Sprintf("open %s: %v", authFile, err), err)
	}
	defer f.Close()

	for _, k := range toAdd {
		if _, err := fmt.Fprintln(f, k); err != nil {
			return FailResult(step, fmt.Sprintf("write %s: %v", authFile, err), err)
		}
	}

	if err := os.Chown(authFile, uid, gid); err != nil {
		return FailResult(step, fmt.Sprintf("chown %s: %v", authFile, err), err)
	}

	return OKResult(step, fmt.Sprintf("added %d key(s) for %s", len(toAdd), name))
}
