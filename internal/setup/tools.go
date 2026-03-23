package setup

import "os"

// InstallDevTools installs a curated set of command-line tools for a headless
// development workstation accessed over SSH. Tools are grouped by purpose and
// installed in a single brew install call per group so that already-installed
// packages are skipped automatically (brew install is idempotent).
//
// The hashicorp/tap tap is added first because it provides the current
// open-source builds of terraform and packer; the homebrew-core versions are
// outdated or use the BSL licence.
func InstallDevTools(r *Runner) []Result {
	if _, err := os.Stat(brewWrapperPath); err != nil {
		return []Result{WarnResult("dev-tools",
			"Homebrew not installed — run Homebrew (Multi-User) setup first")}
	}

	var results []Result

	// Add taps before installing any formulae that live in them.
	taps := []string{"hashicorp/tap"}
	for _, tap := range taps {
		out, err := r.Run(brewWrapperPath, "tap", tap)
		if err != nil {
			results = append(results, FailResult("tap-"+tap, out, err))
			return results
		}
		results = append(results, OKResult("tap-"+tap, "tapped "+tap))
	}

	type group struct {
		step     string
		packages []string
	}

	groups := []group{
		{"tools-shell", []string{
			"bash", "zsh", "zsh-autosuggestions", "zsh-syntax-highlighting",
			"screen", "vim", "nano", "tree", "moreutils", "pv", "rename",
		}},
		{"tools-file", []string{
			"rsync", "p7zip", "git-lfs",
		}},
		{"tools-network", []string{
			"openssh", "ssh-copy-id", "nmap", "tcping", "telnet", "whois", "wget", "curl",
		}},
		{"tools-gnu", []string{
			"coreutils", "findutils", "gnu-sed", "gnu-tar", "grep", "gawk",
		}},
		{"tools-monitor", []string{
			"htop", "pstree", "pidof",
		}},
		{"tools-security", []string{
			"gnupg", "pwgen", "git-secrets", "pre-commit", "checkov",
		}},
		{"tools-vcs", []string{
			"git", "gh",
		}},
		{"tools-data", []string{
			"jq", "openapi-generator",
		}},
		{"tools-go", []string{
			"go", "golangci-lint", "goreleaser",
		}},
		{"tools-rust", []string{
			"rust",
		}},
		{"tools-python", []string{
			"python@3.12",
		}},
		{"tools-node", []string{
			"node@22", "nvm",
		}},
		{"tools-java", []string{
			"openjdk@21",
		}},
		{"tools-ruby", []string{
			"rbenv",
		}},
		{"tools-infra", []string{
			"ansible", "hashicorp/tap/terraform", "tflint", "tfsec", "hashicorp/tap/packer",
		}},
		{"tools-k8s", []string{
			"kubernetes-cli", "helm", "kind",
		}},
		{"tools-cloud", []string{
			"awscli", "azure-cli",
		}},
		{"tools-containers", []string{
			"podman", "docker-compose",
		}},
		{"tools-build", []string{
			"cmake", "gcc", "autoconf", "automake",
		}},
		{"tools-hardware", []string{
			"ipmitool",
		}},
	}

	for _, g := range groups {
		args := append([]string{"install"}, g.packages...)
		out, err := r.Run(brewWrapperPath, args...)
		if err != nil {
			results = append(results, FailResult(g.step, out, err))
			continue
		}
		results = append(results, OKResult(g.step, "installed "+g.step[len("tools-"):]))
	}

	return results
}
