package polaris

import (
	"io"
	"testing"
)

func TestBuildSpawnCommandCodexIncludesModel(t *testing.T) {
	bin, args, err := buildSpawnCommand("codex", "", "gpt-5.5", "", "fix the tests", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bin != "codex" {
		t.Fatalf("bin = %q, want codex", bin)
	}
	want := []string{"exec", "--json", "--model", "gpt-5.5", "fix the tests"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestBuildSpawnCommandCodexOmitsEmptyModel(t *testing.T) {
	_, args, err := buildSpawnCommand("codex", "codex-dev", "", "", "fix the tests", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exec", "--json", "fix the tests"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestBuildResumeCommandCodexIncludesModel(t *testing.T) {
	bin, args, err := buildResumeCommand("codex", "", "session-id", "follow up", "manual", "gpt-5.5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bin != "codex" {
		t.Fatalf("bin = %q, want codex", bin)
	}
	want := []string{"exec", "resume", "--json", "--model", "gpt-5.5", "session-id", "follow up"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestCodexDoesNotNeedStdinPipe(t *testing.T) {
	if needsStdinPipe("codex", nil) {
		t.Fatal("codex should not get an open stdin pipe when prompt is passed as an argument")
	}
	if !needsStdinPipe("codex", func(w io.Writer) error { return nil }) {
		t.Fatal("explicit initial stdin writer should force a stdin pipe")
	}
}
