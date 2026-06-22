// Package procrunner runs a project command (optionally inside its devcontainer)
// and streams its output as Wails events. The language providers
// (nodejs/python/csharp/taskfile) all share this lifecycle: spawn under a run
// ID, pump stdout/stderr line by line, support graceful then forced stop, and
// tear down a container they started. Each provider supplies only its event
// names and how a script maps to an executable + argv sequence.
package procrunner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/devenv"
	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type Emitter interface {
	Emit(event string, data ...any)
}

type Runner struct {
	emit      Emitter
	eventLine string
	eventExit string

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

func New(emit Emitter, eventLine, eventExit string) *Runner {
	return &Runner{emit: emit, eventLine: eventLine, eventExit: eventExit, runs: map[string]*runHandle{}}
}

// Run executes each argv in commands sequentially under one run ID, stopping
// early if one fails or the run is stopped. The run ID is the streaming/Stop handle.
func (r *Runner) Run(workDir, runEnv, exe string, commands [][]string) (string, error) {
	if workDir == "" {
		return "", fmt.Errorf("empty work dir")
	}
	if len(commands) == 0 {
		return "", fmt.Errorf("no command")
	}

	runID := newRunID()
	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.runs[runID] = &runHandle{cancel: cancel, done: make(chan struct{})}
	r.mu.Unlock()

	finish := func(code int, errMsg string) {
		r.mu.Lock()
		if h := r.runs[runID]; h != nil {
			close(h.done)
		}
		delete(r.runs, runID)
		r.mu.Unlock()
		r.emit.Emit(r.eventExit, map[string]any{"runId": runID, "code": code, "error": errMsg})
		cancel()
	}

	go func() {
		if runEnv == "devcontainer" {
			r.emitLine(runID, "system", "Starting devcontainer...")
			containerID, weStarted, err := devenv.EnsureUp(ctx, workDir)
			if err != nil {
				r.emitLine(runID, "stderr", err.Error())
				finish(1, err.Error())
				return
			}
			r.mu.Lock()
			h := r.runs[runID]
			if h != nil {
				h.containerID = containerID
				h.weStartedContainer = weStarted
				h.cmdName = exe
			}
			stopped := h == nil || h.stopped
			r.mu.Unlock()
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
			code, errMsg, ok := r.runOne(ctx, runID, workDir, runEnv, exe, args)
			if !ok {
				finish(code, errMsg)
				return
			}
			r.mu.Lock()
			stopped := r.runs[runID] == nil || r.runs[runID].stopped
			r.mu.Unlock()
			if stopped {
				finish(code, errMsg)
				return
			}
		}
		finish(0, "")
	}()

	return runID, nil
}

// runOne returns ok=false when the command could not be started or exited
// non-zero, which stops the sequence.
func (r *Runner) runOne(ctx context.Context, runID, workDir, runEnv, exe string, args []string) (int, string, bool) {
	cmd, err := devenv.BuildCommand(ctx, workDir, runEnv, exe, args)
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

	r.mu.Lock()
	if h := r.runs[runID]; h != nil {
		h.cmd = cmd
	}
	r.mu.Unlock()

	r.emitLine(runID, "system", fmt.Sprintf("$ %s %s", exe, strings.Join(args, " ")))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); r.pump(runID, "stdout", stdout) }()
	go func() { defer wg.Done(); r.pump(runID, "stderr", stderr) }()

	// Drain both pipes to EOF before Wait: cmd.Wait closes the StdoutPipe/StderrPipe
	// read ends, so calling it while the pumps are still reading would truncate output.
	wg.Wait()
	waitErr := cmd.Wait()

	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			return ee.ExitCode(), "", false
		}
		return -1, waitErr.Error(), false
	}
	return 0, "", true
}

func (r *Runner) Stop(runID string) error {
	// Snapshot every field the run goroutine writes under the same mutex, so the
	// decisions below don't race with a run that is still starting up.
	r.mu.Lock()
	h, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("run %q not found", runID)
	}
	h.stopped = true
	containerID, weStarted, cmdName := h.containerID, h.weStartedContainer, h.cmdName
	cmd, done, cancel := h.cmd, h.done, h.cancel
	r.mu.Unlock()

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

func (r *Runner) pump(runID, stream string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		r.emitLine(runID, stream, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		r.emitLine(runID, stream, fmt.Sprintf("stream read error: %v", err))
	}
}

func (r *Runner) emitLine(runID, stream, line string) {
	r.emit.Emit(r.eventLine, map[string]any{
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
