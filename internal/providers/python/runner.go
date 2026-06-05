package python

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
	EventLine = "python:run:line"
	EventExit = "python:run:exit"
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
	done               chan struct{} // closed once the process has exited and been reaped
	containerID        string        // non-empty for devcontainer runs
	weStartedContainer bool          // true if we started the container (must stop it on exit)
	cmdName            string        // command name, for pkill when container was pre-existing
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{emit: emit, runs: map[string]*runHandle{}}
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
	return runner.launch(manifestPath, runEnv, name, args)
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
	return runner.launch(manifestPath, runEnv, pm, args)
}

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

func (runner *Runner) launch(manifestPath, runEnv, name string, args []string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	if name == "" {
		return "", fmt.Errorf("empty command")
	}

	workDir := filepath.Dir(manifestPath)
	runID := newRunID()
	ctx, cancel := context.WithCancel(context.Background())

	runner.mu.Lock()
	runner.runs[runID] = &runHandle{cancel: cancel, done: make(chan struct{})}
	runner.mu.Unlock()

	go func() {
		if runEnv == "devcontainer" {
			runner.emitLine(runID, "system", "Starting devcontainer...")
			containerID, weStarted, err := ensureDevcontainerUp(workDir)
			if err != nil {
				runner.emitLine(runID, "stderr", err.Error())
				runner.emit.Emit(EventExit, map[string]any{"runId": runID, "code": 1, "error": err.Error()})
				runner.mu.Lock()
				delete(runner.runs, runID)
				runner.mu.Unlock()
				cancel()
				return
			}
			runner.mu.Lock()
			runner.runs[runID].containerID = containerID
			runner.runs[runID].weStartedContainer = weStarted
			runner.runs[runID].cmdName = name
			runner.mu.Unlock()
		}

		cmd := BuildCommand(ctx, workDir, runEnv, name, args)
		cmd.Env = os.Environ()
		sysexec.Hide(cmd)
		sysexec.SetProcessGroup(cmd)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			runner.emit.Emit(EventExit, map[string]any{"runId": runID, "code": -1, "error": err.Error()})
			runner.mu.Lock()
			delete(runner.runs, runID)
			runner.mu.Unlock()
			cancel()
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			runner.emit.Emit(EventExit, map[string]any{"runId": runID, "code": -1, "error": err.Error()})
			runner.mu.Lock()
			delete(runner.runs, runID)
			runner.mu.Unlock()
			cancel()
			return
		}
		if err := cmd.Start(); err != nil {
			runner.emit.Emit(EventExit, map[string]any{"runId": runID, "code": -1, "error": err.Error()})
			runner.mu.Lock()
			delete(runner.runs, runID)
			runner.mu.Unlock()
			cancel()
			return
		}

		runner.mu.Lock()
		runner.runs[runID].cmd = cmd
		runner.mu.Unlock()

		runner.emitLine(runID, "system", fmt.Sprintf("$ %s %s", name, strings.Join(args, " ")))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); runner.pump(runID, "stdout", stdout) }()
		go func() { defer wg.Done(); runner.pump(runID, "stderr", stderr) }()

		go func() {
			waitErr := cmd.Wait()
			wg.Wait()

			runner.mu.Lock()
			if h := runner.runs[runID]; h != nil {
				close(h.done)
			}
			delete(runner.runs, runID)
			runner.mu.Unlock()

			code := 0
			var errMsg string
			if waitErr != nil {
				if ee, ok := waitErr.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					code = -1
					errMsg = waitErr.Error()
				}
			}
			runner.emit.Emit(EventExit, map[string]any{
				"runId": runID,
				"code":  code,
				"error": errMsg,
			})
			cancel()
		}()
	}()

	return runID, nil
}

func (runner *Runner) Stop(runID string) error {
	runner.mu.Lock()
	h, ok := runner.runs[runID]
	runner.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}
	if h.containerID != "" {
		if h.weStartedContainer {
			// We started the container: stop it entirely (kills the process too).
			go func() { _ = exec.Command("docker", "stop", "--time", "3", h.containerID).Run() }()
			h.cancel()
			return nil
		}
		// Container was pre-existing: kill only our process inside, leave container running.
		go func() {
			_ = exec.Command("docker", "exec", h.containerID, "pkill", "-SIGTERM", "-f", h.cmdName).Run()
		}()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		select {
		case <-h.done:
			h.cancel()
			return nil
		default:
		}
		_ = sysexec.InterruptGroup(h.cmd)
		go func(cmd *exec.Cmd, done <-chan struct{}, cancel context.CancelFunc) {
			time.Sleep(3 * time.Second)
			select {
			case <-done:
			default:
				_ = sysexec.KillGroup(cmd)
			}
			cancel()
		}(h.cmd, h.done, h.cancel)
		return nil
	}
	h.cancel()
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
