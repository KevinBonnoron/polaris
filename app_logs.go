package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// stderrWriter wraps os.Stderr and silently discards write errors so that a
// missing console (Windows GUI, no attached terminal) does not prevent the
// primary log file from being written to.
type stderrWriter struct{}

func (stderrWriter) Write(p []byte) (int, error) {
	os.Stderr.Write(p) //nolint:errcheck
	return len(p), nil
}

type AppLogTail struct {
	Lines  []string `json:"lines"`
	Offset int64    `json:"offset"`
	Path   string   `json:"path"`
}

func (app *App) ReadAppLog(offset int64) (AppLogTail, error) {
	if app.appLogPath == "" {
		return AppLogTail{Offset: offset, Path: ""}, fmt.Errorf("app log not initialised")
	}
	f, err := os.Open(app.appLogPath)
	if err != nil {
		return AppLogTail{Offset: offset}, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return AppLogTail{Offset: offset}, err
	}

	// Detect truncation: if the current position is past the file end (e.g.
	// after ClearAppLog), reset to the beginning so new entries are not missed.
	if fi, err := f.Stat(); err == nil && offset > fi.Size() {
		offset = 0
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return AppLogTail{Offset: 0}, err
		}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return AppLogTail{Offset: offset}, err
	}

	newOffset := offset + int64(len(data))
	if len(data) == 0 {
		return AppLogTail{Offset: newOffset}, nil
	}

	raw := strings.TrimRight(string(data), "\n")
	lines := strings.Split(raw, "\n")
	return AppLogTail{Lines: lines, Offset: newOffset, Path: app.appLogPath}, nil
}

func logDebug(format string, args ...any) { log.Printf("[DEBUG] "+format, args...) }
func logInfo(format string, args ...any)  { log.Printf("[INFO] "+format, args...) }
func logWarn(format string, args ...any)  { log.Printf("[WARN] "+format, args...) }
func logError(format string, args ...any) { log.Printf("[ERROR] "+format, args...) }

func (app *App) ClearAppLog() error {
	if app.appLogPath == "" {
		return fmt.Errorf("app log not initialised")
	}
	return os.Truncate(app.appLogPath, 0)
}
