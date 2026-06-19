package csharp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

const (
	EventLine = "csharp:run:line"
	EventExit = "csharp:run:exit"
)

type Emitter interface {
	Emit(event string, data ...any)
}

type Runner struct {
	emit Emitter

	mu   sync.Mutex
	runs map[string]*runHandle
}

type runHandle struct {
	cmd                *exec.Cmd
	cancel             context.CancelFunc
	done               chan struct{}
	stopped            bool
	containerID        string
	weStartedContainer bool
	cmdName            string
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{emit: emit, runs: map[string]*runHandle{}}
}

// Start runs a dotnet target by name (e.g. "run", "watch", "IIS Express
// (profile)"). The name is resolved to its exact argv so launch profile names
// with spaces stay intact. The returned runID is the handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	args := ResolveArgs(manifestPath, script)
	if len(args) == 0 {
		return "", fmt.Errorf("empty script")
	}
	return runner.launch(manifestPath, runEnv, [][]string{args})
}

// RunCommand runs an arbitrary dotnet command (e.g. "add package Foo"). The
// internal "update-packages" verb is expanded into one `dotnet add package`
// invocation per spec, run sequentially, because the dotnet CLI cannot add
// several packages at once.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	if args[0] == "update-packages" {
		var commands [][]string
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
		return runner.launch(manifestPath, runEnv, commands)
	}
	return runner.launch(manifestPath, runEnv, [][]string{args})
}

// launch runs the given dotnet command sequences under a single runID, streaming
// their combined output. Each entry in commands is one `dotnet <args>` call; they
// run one after another and stop early if one fails or the run is cancelled.
func (runner *Runner) launch(manifestPath, runEnv string, commands [][]string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	if len(commands) == 0 {
		return "", fmt.Errorf("no command")
	}

	workDir := filepath.Dir(manifestPath)
	runID := newRunID()
	ctx, cancel := context.WithCancel(context.Background())

	runner.mu.Lock()
	runner.runs[runID] = &runHandle{cancel: cancel, done: make(chan struct{})}
	runner.mu.Unlock()

	finish := func(code int, errMsg string) {
		runner.mu.Lock()
		if h := runner.runs[runID]; h != nil {
			close(h.done)
		}
		delete(runner.runs, runID)
		runner.mu.Unlock()
		runner.emit.Emit(EventExit, map[string]any{"runId": runID, "code": code, "error": errMsg})
		cancel()
	}

	go func() {
		if runEnv == "devcontainer" {
			runner.emitLine(runID, "system", "Starting devcontainer...")
			containerID, weStarted, err := ensureDevcontainerUp(workDir)
			if err != nil {
				runner.emitLine(runID, "stderr", err.Error())
				finish(1, err.Error())
				return
			}
			runner.mu.Lock()
			h := runner.runs[runID]
			if h != nil {
				h.containerID = containerID
				h.weStartedContainer = weStarted
				h.cmdName = "dotnet"
			}
			stopped := h == nil || h.stopped
			runner.mu.Unlock()
			// Stop may have arrived while the container was still coming up; tear
			// down a container we started rather than leaking it.
			if stopped {
				if weStarted && containerID != "" {
					go func() { _ = exec.Command("docker", "stop", "--time", "3", containerID).Run() }()
				}
				finish(1, "")
				return
			}
		}

		for _, args := range commands {
			code, errMsg, ok := runner.runOne(ctx, runID, workDir, runEnv, args)
			if !ok {
				finish(code, errMsg)
				return
			}
			runner.mu.Lock()
			stopped := runner.runs[runID] == nil || runner.runs[runID].stopped
			runner.mu.Unlock()
			if stopped {
				finish(code, errMsg)
				return
			}
		}
		finish(0, "")
	}()

	return runID, nil
}

// runOne runs a single `dotnet <args>` command to completion, streaming its
// output under runID. It returns the exit code, an error message, and ok=false
// when the command could not be started or exited non-zero (so the sequence
// stops).
func (runner *Runner) runOne(ctx context.Context, runID, workDir, runEnv string, args []string) (int, string, bool) {
	cmd := BuildCommand(ctx, workDir, runEnv, "dotnet", args)
	cmd.Env = os.Environ()
	sysexec.Hide(cmd)
	sysexec.SetProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err.Error(), false
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err.Error(), false
	}
	if err := cmd.Start(); err != nil {
		return -1, err.Error(), false
	}

	runner.mu.Lock()
	if h := runner.runs[runID]; h != nil {
		h.cmd = cmd
	}
	runner.mu.Unlock()

	runner.emitLine(runID, "system", fmt.Sprintf("$ dotnet %s", strings.Join(args, " ")))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); runner.pump(runID, "stdout", stdout) }()
	go func() { defer wg.Done(); runner.pump(runID, "stderr", stderr) }()

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			return ee.ExitCode(), "", false
		}
		return -1, waitErr.Error(), false
	}
	return 0, "", true
}

func (runner *Runner) Stop(runID string) error {
	// Snapshot every field the launch goroutine writes under the same mutex, so
	// the decisions below don't race with a run that is still starting up.
	runner.mu.Lock()
	h, ok := runner.runs[runID]
	if !ok {
		runner.mu.Unlock()
		return fmt.Errorf("run %q not found", runID)
	}
	h.stopped = true
	containerID, weStarted, cmdName := h.containerID, h.weStartedContainer, h.cmdName
	cmd, done, cancel := h.cmd, h.done, h.cancel
	runner.mu.Unlock()

	if containerID != "" {
		if weStarted {
			go func() { _ = exec.Command("docker", "stop", "--time", "3", containerID).Run() }()
			cancel()
			return nil
		}
		go func() {
			_ = exec.Command("docker", "exec", containerID, "pkill", "-SIGTERM", "-f", cmdName).Run()
		}()
	}
	if cmd != nil && cmd.Process != nil {
		select {
		case <-done:
			cancel()
			return nil
		default:
		}
		_ = sysexec.InterruptGroup(cmd)
		go func() {
			time.Sleep(3 * time.Second)
			select {
			case <-done:
			default:
				_ = sysexec.KillGroup(cmd)
			}
			cancel()
		}()
		return nil
	}
	cancel()
	return nil
}

func (runner *Runner) pump(runID, stream string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		runner.emitLine(runID, stream, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		runner.emitLine(runID, stream, fmt.Sprintf("stream read error: %v", err))
	}
}

func (runner *Runner) emitLine(runID, stream, line string) {
	runner.emit.Emit(EventLine, map[string]any{
		"runId":  runID,
		"stream": stream,
		"line":   line,
		"ts":     time.Now().UnixMilli(),
	})
}

func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
