package taskfile

import (
	"fmt"
	"path/filepath"

	"github.com/KevinBonnoron/polaris/internal/providers/procrunner"
)

const (
	EventLine = "taskfile:run:line"
	EventExit = "taskfile:run:exit"
)

type Emitter = procrunner.Emitter

type Runner struct {
	pr *procrunner.Runner
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{pr: procrunner.New(emit, EventLine, EventExit)}
}

// Start runs a task by name (e.g. "dev", "common:dev:frontend"). The returned
// runID is the handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("empty task name")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, "task", [][]string{{script}})
}

// RunCommand runs an arbitrary `task` invocation (e.g. "--list").
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, "task", [][]string{args})
}

func (runner *Runner) Stop(runID string) error { return runner.pr.Stop(runID) }

func workDirOf(manifestPath string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	return filepath.Dir(manifestPath), nil
}
