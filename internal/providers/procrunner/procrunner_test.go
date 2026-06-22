package procrunner

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func skipOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires Unix commands (echo/false)")
	}
}

type captureEmitter struct {
	mu     sync.Mutex
	lines  []string
	exit   chan map[string]any
	onExit bool
}

func newCapture() *captureEmitter {
	return &captureEmitter{exit: make(chan map[string]any, 1)}
}

func (c *captureEmitter) Emit(event string, data ...any) {
	m, _ := data[0].(map[string]any)
	c.mu.Lock()
	switch event {
	case "test:line":
		if s, ok := m["line"].(string); ok {
			c.lines = append(c.lines, s)
		}
	case "test:exit":
		c.onExit = true
	}
	c.mu.Unlock()
	if event == "test:exit" {
		c.exit <- m
	}
}

func (c *captureEmitter) sawLine(want string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.lines {
		if l == want {
			return true
		}
	}
	return false
}

func waitExit(t *testing.T, c *captureEmitter) map[string]any {
	t.Helper()
	select {
	case m := <-c.exit:
		return m
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for exit event")
		return nil
	}
}

func TestRunSequenceSuccess(t *testing.T) {
	skipOnWindows(t)
	c := newCapture()
	r := New(c, "test:line", "test:exit")
	if _, err := r.Run(t.TempDir(), "", "echo", [][]string{{"first"}, {"second"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	m := waitExit(t, c)
	if code, _ := m["code"].(int); code != 0 {
		t.Fatalf("exit code = %v, want 0", m["code"])
	}
	if !c.sawLine("first") || !c.sawLine("second") {
		t.Fatalf("missing output from one of the sequenced commands: %v", c.lines)
	}
}

func TestRunStopsSequenceOnFailure(t *testing.T) {
	skipOnWindows(t)
	c := newCapture()
	r := New(c, "test:line", "test:exit")
	// First step exits non-zero, so the second step must never run.
	if _, err := r.Run(t.TempDir(), "", "sh", [][]string{{"-c", "false"}, {"-c", "echo ran-second"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	m := waitExit(t, c)
	if code, _ := m["code"].(int); code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if c.sawLine("ran-second") {
		t.Fatal("second command ran after the first failed")
	}
}

func TestRunRejectsEmptyInput(t *testing.T) {
	r := New(newCapture(), "test:line", "test:exit")
	if _, err := r.Run("", "", "echo", [][]string{{"x"}}); err == nil {
		t.Fatal("expected error for empty work dir")
	}
	if _, err := r.Run(t.TempDir(), "", "echo", nil); err == nil {
		t.Fatal("expected error for empty command list")
	}
}
