package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/KevinBonnoron/polaris/internal/polaris"
)

func TestReadCodexModelsCacheFiltersVisibleModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models_cache.json")
	raw := `{
		"models": [
			{"slug": "gpt-5.5", "display_name": "GPT-5.5", "visibility": "list"},
			{"slug": "codex-auto-review", "display_name": "Codex Auto Review", "visibility": "hide"},
			{"slug": "gpt-5.4-mini", "display_name": "GPT-5.4 Mini", "visibility": "list"},
			{"slug": "gpt-5.5", "display_name": "Duplicate", "visibility": "list"}
		]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	models := polaris.ReadCodexModelsCache(path)
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2: %#v", len(models), models)
	}
	if models[0].Value != "gpt-5.5" || models[0].Name != "GPT-5.5" {
		t.Fatalf("models[0] = %#v, want GPT-5.5", models[0])
	}
	if models[1].Value != "gpt-5.4-mini" || models[1].Name != "GPT-5.4 Mini" {
		t.Fatalf("models[1] = %#v, want GPT-5.4 Mini", models[1])
	}
}

func TestListCodexModelsFallsBackWithoutCache(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())

	models := listCodexModels()
	if len(models) == 0 {
		t.Fatal("listCodexModels() returned no fallback models")
	}
	if models[0].Value == "" || models[0].Name == "" {
		t.Fatalf("first fallback model is incomplete: %#v", models[0])
	}
}
