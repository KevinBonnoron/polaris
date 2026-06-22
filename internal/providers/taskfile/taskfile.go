// Package taskfile detects a go-task Taskfile in a working tree, lists its
// runnable tasks and runs them through the `task` CLI. Task names are resolved
// with `task --list-all --json` (which expands includes and namespaces); a flat
// parse of the top-level `tasks:` keys is used as a fallback when the task CLI
// isn't on PATH.
package taskfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/devenv"
	"gopkg.in/yaml.v3"
)

type Project struct {
	ManifestPath   string   `json:"manifestPath"`
	PackageManager string   `json:"packageManager"`
	RunEnv         string   `json:"runEnv,omitempty"`
	Scripts        []Script `json:"scripts"`
}

type Script struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// manifestNames are the file names go-task recognises, in preference order.
var manifestNames = []string{
	"Taskfile.yml", "Taskfile.yaml",
	"taskfile.yml", "taskfile.yaml",
	"Taskfile.dist.yml", "Taskfile.dist.yaml",
}

// maxWalkDepth bounds the subfolder scan used to discover Taskfiles that don't
// sit at the repo root.
const maxWalkDepth = 4

// skipWalkDirs are directories never worth descending into when hunting for
// Taskfiles.
var skipWalkDirs = map[string]bool{
	".git":         true,
	".idea":        true,
	".vscode":      true,
	".direnv":      true,
	".task":        true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"bin":          true,
}

// Detect returns nil (no error) when no Taskfile exists — that's the signal
// "this isn't a task project". A root manifest is preferred; otherwise the tree
// is scanned a few levels deep and the shallowest discovered manifest is used.
func Detect(projectPath string) (*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	manifest := manifestInDir(projectPath)
	if manifest == "" {
		found, err := discoverManifests(projectPath)
		if err != nil {
			return nil, err
		}
		if len(found) > 0 {
			manifest = found[0]
		}
	}
	if manifest == "" {
		return nil, nil
	}
	scripts, _ := ListScripts(manifest)
	return &Project{
		ManifestPath:   manifest,
		PackageManager: "task",
		RunEnv:         devenv.Detect(filepath.Dir(manifest)),
		Scripts:        scripts,
	}, nil
}

// DetectAll returns one Project per top-level Taskfile in the tree. The scan
// stops descending once it finds a Taskfile in a directory, so the included
// Taskfiles a root one pulls in (e.g. build/Taskfile.yml) aren't surfaced as
// independent projects. Returns nil, nil when none is found.
func DetectAll(projectPath string) ([]*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	all, err := discoverManifests(projectPath)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}
	out := make([]*Project, 0, len(all))
	for _, m := range all {
		scripts, _ := ListScripts(m)
		out = append(out, &Project{
			ManifestPath:   m,
			PackageManager: "task",
			RunEnv:         devenv.Detect(filepath.Dir(m)),
			Scripts:        scripts,
		})
	}
	return out, nil
}

// manifestInDir returns the Taskfile sitting directly in dir, or "" when none.
func manifestInDir(dir string) string {
	for _, name := range manifestNames {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// discoverManifests does a depth-limited walk from root and returns the
// discovered Taskfiles, shallowest first. A missing or unreadable root is
// surfaced as an error (os.ReadDir, unlike os.Stat, also catches a directory
// that exists but can't be listed); per-entry walk errors (e.g. one unreadable
// subdir) are skipped so they don't abort the whole scan.
func discoverManifests(root string) ([]string, error) {
	if _, err := os.ReadDir(root); err != nil {
		return nil, err
	}
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel != "." {
			if skipWalkDirs[d.Name()] {
				return filepath.SkipDir
			}
			if strings.Count(rel, string(filepath.Separator)) >= maxWalkDepth {
				if m := manifestInDir(path); m != "" {
					found = append(found, m)
				}
				return filepath.SkipDir
			}
		}
		if m := manifestInDir(path); m != "" {
			found = append(found, m)
			// Nested Taskfiles are includes of this one, not separate projects.
			return filepath.SkipDir
		}
		return nil
	})
	// Shallowest first (then alphabetical) so Detect's found[0] is the
	// top-most Taskfile, not just the lexicographically first path.
	sort.Slice(found, func(i, j int) bool {
		di := strings.Count(found[i], string(filepath.Separator))
		dj := strings.Count(found[j], string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return found[i] < found[j]
	})
	return found, nil
}

// ListScripts returns the runnable tasks for the UI. It prefers the task CLI
// (which resolves includes and namespaced tasks); when task isn't available it
// falls back to the top-level `tasks:` keys declared in the manifest.
func ListScripts(manifestPath string) ([]Script, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	if scripts := tasksFromCLI(manifestPath); len(scripts) > 0 {
		return scripts, nil
	}
	return tasksFromYAML(manifestPath)
}

// listJSON mirrors the subset of `task --list-all --json` we read.
type listJSON struct {
	Tasks []struct {
		Name    string `json:"name"`
		Desc    string `json:"desc"`
		Summary string `json:"summary"`
	} `json:"tasks"`
}

// tasksFromCLI asks the task binary for the full task list (includes expanded).
// Returns nil on any failure so the caller can fall back to the YAML parse.
func tasksFromCLI(manifestPath string) []Script {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "task", "--list-all", "--json", "--taskfile", manifestPath)
	cmd.Dir = filepath.Dir(manifestPath)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var parsed listJSON
	if json.Unmarshal(out, &parsed) != nil {
		return nil
	}
	scripts := make([]Script, 0, len(parsed.Tasks))
	for _, task := range parsed.Tasks {
		desc := task.Summary
		if desc == "" {
			desc = task.Desc
		}
		scripts = append(scripts, Script{Name: task.Name, Command: desc})
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Name < scripts[j].Name })
	return scripts
}

// tasksFromYAML returns the top-level task names declared in the manifest. It
// cannot see tasks pulled in via `includes` — that's what the CLI path is for.
func tasksFromYAML(manifestPath string) ([]Script, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	scripts := make([]Script, 0, len(doc.Tasks))
	for name := range doc.Tasks {
		scripts = append(scripts, Script{Name: name})
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Name < scripts[j].Name })
	return scripts, nil
}
