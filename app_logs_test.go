package main

import (
	"os"
	"path/filepath"
	"testing"
)

func appWithLog(t *testing.T, content string) (*App, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return &App{appLogPath: path}, path
}

func TestReadAppLogNoPath(t *testing.T) {
	app := &App{}
	_, err := app.ReadAppLog(0)
	if err == nil {
		t.Fatal("expected error when appLogPath is empty")
	}
}

func TestReadAppLogMissingFile(t *testing.T) {
	app := &App{appLogPath: filepath.Join(t.TempDir(), "missing.log")}
	_, err := app.ReadAppLog(0)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAppLogEmpty(t *testing.T) {
	app, _ := appWithLog(t, "")
	tail, err := app.ReadAppLog(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tail.Lines) != 0 {
		t.Fatalf("want 0 lines, got %d", len(tail.Lines))
	}
	if tail.Offset != 0 {
		t.Fatalf("want offset 0, got %d", tail.Offset)
	}
}

func TestReadAppLogAllLines(t *testing.T) {
	app, _ := appWithLog(t, "line one\nline two\nline three\n")
	tail, err := app.ReadAppLog(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tail.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %v", len(tail.Lines), tail.Lines)
	}
	if tail.Lines[0] != "line one" || tail.Lines[2] != "line three" {
		t.Fatalf("unexpected lines: %v", tail.Lines)
	}
}

func TestReadAppLogIncremental(t *testing.T) {
	app, path := appWithLog(t, "first\n")

	tail1, err := app.ReadAppLog(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tail1.Lines) != 1 || tail1.Lines[0] != "first" {
		t.Fatalf("want [first], got %v", tail1.Lines)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("second\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	tail2, err := app.ReadAppLog(tail1.Offset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tail2.Lines) != 1 || tail2.Lines[0] != "second" {
		t.Fatalf("want [second], got %v", tail2.Lines)
	}
	if tail2.Offset <= tail1.Offset {
		t.Fatalf("offset should have advanced: %d <= %d", tail2.Offset, tail1.Offset)
	}
}

func TestReadAppLogPathInResponse(t *testing.T) {
	app, path := appWithLog(t, "hello\n")
	tail, err := app.ReadAppLog(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tail.Path != path {
		t.Fatalf("want path %q, got %q", path, tail.Path)
	}
}
