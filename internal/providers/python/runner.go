package python

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/KevinBonnoron/polaris/internal/providers/procrunner"
)

const (
	EventLine = "python:run:line"
	EventExit = "python:run:exit"
)

type Emitter = procrunner.Emitter

type Runner struct {
	pr *procrunner.Runner
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{pr: procrunner.New(emit, EventLine, EventExit)}
}

// Start runs a script (a named task or a free command) inside the project's
// environment. For managers with a runner that means `<pm> run <tokens>`; pip has
// no runner, so the command is executed directly. The returned runID is the
// handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	if strings.TrimSpace(script) == "" {
		return "", fmt.Errorf("empty script name")
	}
	name, args := runCommand(packageManager, strings.Fields(script))
	if name == "" {
		return "", fmt.Errorf("empty command")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, name, [][]string{args})
}

// RunCommand runs an arbitrary package-manager command (e.g. "add requests")
// from the directory containing the manifest.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	pm := packageManager
	if pm == "" {
		pm = "pip"
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, pm, [][]string{args})
}

func (runner *Runner) Stop(runID string) error { return runner.pr.Stop(runID) }

// runCommand maps script tokens to the executable + args that run them inside the
// project environment for the given package manager.
func runCommand(packageManager string, tokens []string) (string, []string) {
	switch packageManager {
	case "uv", "poetry", "pdm", "pipenv":
		return packageManager, append([]string{"run"}, tokens...)
	default:
		if len(tokens) == 0 {
			return "", nil
		}
		return tokens[0], tokens[1:]
	}
}

func workDirOf(manifestPath string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	return filepath.Dir(manifestPath), nil
}
