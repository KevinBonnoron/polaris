package shell

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

const (
	EventData = "shell:data"
	EventExit = "shell:exit"
)

type Emitter interface {
	Emit(event string, data ...any)
}

type session struct {
	id     string
	ptm    *os.File
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

type Runner struct {
	emit     Emitter
	mu       sync.Mutex
	sessions map[string]*session
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{emit: emit, sessions: map[string]*session{}}
}

func (r *Runner) Start(workDir string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	id := newID()
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, shell)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	ptm, err := pty.Start(cmd)
	if err != nil {
		cancel()
		return "", fmt.Errorf("pty start: %w", err)
	}

	s := &session{id: id, ptm: ptm, cmd: cmd, cancel: cancel}
	r.mu.Lock()
	r.sessions[id] = s
	r.mu.Unlock()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := ptm.Read(buf)
			if n > 0 {
				r.emit.Emit(EventData, map[string]any{
					"sessionId": id,
					"data":      string(buf[:n]),
				})
			}
			if readErr != nil {
				break
			}
		}

		code := 0
		if waitErr := cmd.Wait(); waitErr != nil {
			if ee, ok := waitErr.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}

		r.mu.Lock()
		delete(r.sessions, id)
		r.mu.Unlock()

		r.emit.Emit(EventExit, map[string]any{
			"sessionId": id,
			"code":      code,
		})
		cancel()
		_ = ptm.Close()
	}()

	return id, nil
}

func (r *Runner) Write(sessionID, data string) error {
	r.mu.Lock()
	s, ok := r.sessions[sessionID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	_, err := s.ptm.WriteString(data)
	return err
}

func (r *Runner) Resize(sessionID string, cols, rows uint16) error {
	r.mu.Lock()
	s, ok := r.sessions[sessionID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return pty.Setsize(s.ptm, &pty.Winsize{Cols: cols, Rows: rows})
}

func (r *Runner) Stop(sessionID string) error {
	r.mu.Lock()
	s, ok := r.sessions[sessionID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	_ = s.ptm.Close()
	s.cancel()
	return nil
}

func (r *Runner) StopAll() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		_ = r.Stop(id)
	}
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
