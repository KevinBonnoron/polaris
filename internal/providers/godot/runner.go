package godot

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
	EventLine = "godot:run:line"
	EventExit = "godot:run:exit"
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

// Start launches the configured command (e.g. "play" or "play -e"). The command
// string is split on whitespace into argv; the returned runID is the handle for
// streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, command, runEnv string) (string, error) {
	argv := strings.Fields(command)
	if len(argv) == 0 {
		return "", fmt.Errorf("empty command")
	}
	return runner.launch(manifestPath, runEnv, argv)
}

// RunCommand runs an arbitrary argv against the project.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	return runner.launch(manifestPath, runEnv, args)
}

// launch runs the given argv under a fresh runID, streaming its output. argv[0]
// is the binary; the rest are its arguments.
func (runner *Runner) launch(manifestPath, runEnv string, argv []string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	if len(argv) == 0 {
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
				h.cmdName = argv[0]
			}
			stopped := h == nil || h.stopped
			runner.mu.Unlock()
			if stopped {
				if weStarted && containerID != "" {
					go func() { _ = exec.Command("docker", "stop", "--time", "3", containerID).Run() }()
				}
				finish(1, "")
				return
			}
		}

		code, errMsg, ok := runner.runOne(ctx, runID, workDir, runEnv, argv)
		if !ok {
			finish(code, errMsg)
			return
		}
		finish(0, "")
	}()

	return runID, nil
}

// runOne runs a single argv to completion, streaming its output under runID.
func (runner *Runner) runOne(ctx context.Context, runID, workDir, runEnv string, argv []string) (int, string, bool) {
	cmd, err := BuildCommand(ctx, workDir, runEnv, argv[0], argv[1:])
	if err != nil {
		return -1, err.Error(), false
	}
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

	runner.emitLine(runID, "system", fmt.Sprintf("$ %s", strings.Join(argv, " ")))

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
