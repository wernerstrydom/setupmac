package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Status represents the outcome of a setup step.
type Status int

const (
	OK   Status = iota // step succeeded
	Skip               // step intentionally not applied
	Fail               // step failed
	Warn               // step completed with caveats
)

// Result is the outcome of a single setup sub-step.
type Result struct {
	Step    string
	Status  Status
	Message string
	Err     error
}

// Runner executes system commands, gating on DryRun.
type Runner struct {
	DryRun bool
}

// Run executes a command capturing stdout+stderr combined.
// In dry-run mode it prints what would be run and returns nil.
func (r *Runner) Run(name string, args ...string) (string, error) {
	if r.DryRun {
		fmt.Printf("  [dry-run] %s %s\n", name, strings.Join(args, " "))
		return "", nil
	}
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunSilent executes a command, discarding stderr in real mode.
// A non-zero exit is expected for some calls (e.g. killall when process absent).
func (r *Runner) RunSilent(name string, args ...string) (string, error) {
	if r.DryRun {
		fmt.Printf("  [dry-run] %s %s\n", name, strings.Join(args, " "))
		return "", nil
	}
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// RunWithStdin pipes input to the command's stdin.
// Used by filevault.go to supply credentials to fdesetup.
// In dry-run mode the input value is not revealed in output.
func (r *Runner) RunWithStdin(input, name string, args ...string) (string, error) {
	if r.DryRun {
		fmt.Printf("  [dry-run] echo '<stdin>' | %s %s\n", name, strings.Join(args, " "))
		return "", nil
	}
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(input + "\n")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// RunLive executes a long-running command, streaming stdout and stderr directly
// to the terminal instead of buffering. Use this for commands that produce
// large amounts of output (e.g. the Homebrew installer) to avoid the pipe
// buffer deadlock that occurs when CombinedOutput blocks waiting for a process
// that is itself blocked trying to write to a full pipe.
func (r *Runner) RunLive(name string, args ...string) error {
	if r.DryRun {
		fmt.Printf("  [dry-run] %s %s\n", name, strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Read executes a read-only command, always running even in dry-run mode.
// Used by verify.go to check current state.
func (r *Runner) Read(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Result constructors.

func OKResult(step, message string) Result {
	return Result{Step: step, Status: OK, Message: message}
}

func SkipResult(step, message string) Result {
	return Result{Step: step, Status: Skip, Message: message}
}

func FailResult(step, message string, err error) Result {
	return Result{Step: step, Status: Fail, Message: message, Err: err}
}

func WarnResult(step, message string) Result {
	return Result{Step: step, Status: Warn, Message: message}
}
