package taskfile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleTaskfile = `version: '3'
tasks:
  dev:
    cmds:
      - echo dev
  build:
    cmds:
      - echo build
  test:
    cmds:
      - echo test
`

func TestDetectFindsRootTaskfile(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "Taskfile.yml")
	writeFile(t, manifest, sampleTaskfile)

	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a project, got nil")
	}
	if got.ManifestPath != manifest {
		t.Fatalf("manifest = %q, want %q", got.ManifestPath, manifest)
	}
	if got.PackageManager != "task" {
		t.Fatalf("packageManager = %q, want task", got.PackageManager)
	}
}

func TestDetectNoTaskfile(t *testing.T) {
	got, err := Detect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

// A missing/unreadable root must surface an error rather than masquerade as
// "no project" (nil, nil).
func TestDetectInvalidPathReturnsError(t *testing.T) {
	if _, err := Detect(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestDetectAllInvalidPathReturnsError(t *testing.T) {
	if _, err := DetectAll(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

// With no root Taskfile, Detect must pick the shallowest manifest, not the
// lexicographically first path (here "a/b" sorts before "z" but is deeper).
func TestDetectPrefersShallowestWhenRootMissing(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "Taskfile.yml")
	shallow := filepath.Join(root, "z", "Taskfile.yml")
	writeFile(t, deep, sampleTaskfile)
	writeFile(t, shallow, sampleTaskfile)

	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a project, got nil")
	}
	if got.ManifestPath != shallow {
		t.Fatalf("manifest = %q, want %q", got.ManifestPath, shallow)
	}
}

// DetectAll must not surface included Taskfiles (e.g. build/Taskfile.yml) as
// separate projects: the scan stops at the shallowest manifest in each subtree.
func TestDetectAllSkipsIncludedTaskfiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Taskfile.yml"), sampleTaskfile)
	writeFile(t, filepath.Join(root, "build", "Taskfile.yml"), sampleTaskfile)

	got, err := DetectAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d: %+v", len(got), got)
	}
	if got[0].ManifestPath != filepath.Join(root, "Taskfile.yml") {
		t.Fatalf("manifest = %q, want root Taskfile.yml", got[0].ManifestPath)
	}
}

func TestTasksFromYAML(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "Taskfile.yml")
	writeFile(t, manifest, sampleTaskfile)

	scripts, err := tasksFromYAML(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"dev": true, "build": true, "test": true}
	if len(scripts) != len(want) {
		t.Fatalf("got %d tasks, want %d: %+v", len(scripts), len(want), scripts)
	}
	for _, s := range scripts {
		if !want[s.Name] {
			t.Errorf("unexpected task %q", s.Name)
		}
	}
}
