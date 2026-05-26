package nodejs

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
	EventLine = "nodejs:run:line"
	EventExit = "nodejs:run:exit"
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
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{emit: emit, runs: map[string]*runHandle{}}
}

// Start launches `<packageManager> run <script>` from the directory containing
// the manifest. The returned runID is the handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("empty script name")
	}
	args := []string{"run", script}
	if packageManager == "deno" {
		args = []string{"task", script}
	}
	return runner.launch(manifestPath, packageManager, runEnv, args)
}

// RunCommand runs an arbitrary package-manager command (e.g. "install",
// "add <pkg>") from the directory containing the manifest.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	return runner.launch(manifestPath, packageManager, runEnv, args)
}

func (runner *Runner) launch(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "npm"
	}

	workDir := filepath.Dir(manifestPath)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := BuildCommand(ctx, workDir, pm, runEnv, args)
	cmd.Env = os.Environ()
	sysexec.Hide(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("start: %w", err)
	}

	runID := newRunID()
	runner.mu.Lock()
	runner.runs[runID] = &runHandle{cmd: cmd, cancel: cancel}
	runner.mu.Unlock()

	runner.emitLine(runID, "system", fmt.Sprintf("$ %s %s", pm, strings.Join(args, " ")))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); runner.pump(runID, "stdout", stdout) }()
	go func() { defer wg.Done(); runner.pump(runID, "stderr", stderr) }()

	go func() {
		waitErr := cmd.Wait()
		wg.Wait()

		runner.mu.Lock()
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

	return runID, nil
}

func (runner *Runner) Stop(runID string) error {
	runner.mu.Lock()
	h, ok := runner.runs[runID]
	runner.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}
	if h.cmd.Process != nil {
		_ = h.cmd.Process.Signal(os.Interrupt)
		go func() {
			time.Sleep(3 * time.Second)
			h.cancel()
		}()
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
