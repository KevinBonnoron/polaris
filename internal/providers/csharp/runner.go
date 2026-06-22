package csharp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/KevinBonnoron/polaris/internal/providers/procrunner"
)

const (
	EventLine = "csharp:run:line"
	EventExit = "csharp:run:exit"
)

type Emitter = procrunner.Emitter

type Runner struct {
	pr *procrunner.Runner
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{pr: procrunner.New(emit, EventLine, EventExit)}
}

// Start runs a dotnet target by name (e.g. "run", "watch", "IIS Express
// (profile)"). The name is resolved to its exact argv so launch profile names
// with spaces stay intact. The returned runID is the handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	args := ResolveArgs(manifestPath, script)
	if len(args) == 0 {
		return "", fmt.Errorf("empty script")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, "dotnet", [][]string{args})
}

// RunCommand runs an arbitrary dotnet command (e.g. "add package Foo"). The
// internal "update-packages" verb is expanded into one `dotnet add package`
// invocation per spec, run sequentially, because the dotnet CLI cannot add
// several packages at once.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	commands := [][]string{args}
	if args[0] == "update-packages" {
		commands = nil
		for _, spec := range args[1:] {
			name, version, _ := strings.Cut(spec, "@")
			cmd := []string{"add", "package", name}
			if version != "" {
				cmd = append(cmd, "--version", version)
			}
			commands = append(commands, cmd)
		}
		if len(commands) == 0 {
			return "", fmt.Errorf("no packages to update")
		}
	}
	return runner.pr.Run(workDir, runEnv, "dotnet", commands)
}

func (runner *Runner) Stop(runID string) error { return runner.pr.Stop(runID) }

func workDirOf(manifestPath string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	return filepath.Dir(manifestPath), nil
}
