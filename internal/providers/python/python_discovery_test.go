package python

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[project]\nname = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A polyglot repo: node at root, python service in a subfolder. Detect must find
// the nested manifest even though the root has no python manifest.
func TestDetectNestedManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"))
	manifest := filepath.Join(root, "inference", "pyproject.toml")
	writeFile(t, manifest)

	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ManifestPath != manifest {
		t.Fatalf("Detect() = %+v, want manifest %s", got, manifest)
	}
}

// A root manifest still wins (keeps uv-workspace expansion anchored at root).
func TestDetectPrefersRoot(t *testing.T) {
	root := t.TempDir()
	rootManifest := filepath.Join(root, "pyproject.toml")
	writeFile(t, rootManifest)
	writeFile(t, filepath.Join(root, "svc", "pyproject.toml"))

	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ManifestPath != rootManifest {
		t.Fatalf("Detect() = %+v, want root manifest", got)
	}
}

// Vendored dirs (.venv, node_modules) must never be discovered.
func TestDiscoverSkipsVendored(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".venv", "lib", "pandas", "pyproject.toml"))
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "pyproject.toml"))
	wanted := filepath.Join(root, "api", "pyproject.toml")
	writeFile(t, wanted)

	got, err := discoverManifests(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != wanted {
		t.Fatalf("discoverManifests() = %v, want [%s]", got, wanted)
	}
}

// Several independent python projects become several workspaces (one tab each).
func TestListWorkspacesScattered(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"))
	a := filepath.Join(root, "api", "pyproject.toml")
	b := filepath.Join(root, "inference", "pyproject.toml")
	writeFile(t, a)
	writeFile(t, b)

	ws, err := ListWorkspaces(root, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("ListWorkspaces() returned %d workspaces, want 2: %+v", len(ws), ws)
	}
	if ws[0].Manifest != a {
		t.Fatalf("primary workspace = %s, want %s", ws[0].Manifest, a)
	}
}
