// Package godot detects a Godot project (a project.godot file) in a working
// tree and launches it through a configurable command (default "play"). The
// command is run as-is, optionally wrapped by the project's run environment
// (nix develop, devcontainer), so a flake-provided launcher like `play` works.
package godot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Project struct {
	ManifestPath   string `json:"manifestPath"`
	PackageManager string `json:"packageManager"`
	RunEnv         string `json:"runEnv,omitempty"`
	PlayCommand    string `json:"playCommand"`
}

// manifestName is the file that marks a Godot project root.
const manifestName = "project.godot"

// defaultPlayCommand is the launcher assumed when none is configured.
const defaultPlayCommand = "play"

// maxWalkDepth bounds the subfolder scan used to discover project.godot files
// that don't sit at the repo root (a C#/.NET layout often keeps the Godot game
// in a `godot/` subdirectory).
const maxWalkDepth = 4

// skipWalkDirs are directories never worth descending into when hunting for a
// project.godot.
var skipWalkDirs = map[string]bool{
	".git":         true,
	".idea":        true,
	".vscode":      true,
	".direnv":      true,
	".godot":       true,
	".import":      true,
	"node_modules": true,
	"vendor":       true,
	"bin":          true,
	"obj":          true,
}

// Detect returns nil (no error) when no project.godot exists — that's the
// signal "this isn't a Godot project". A root manifest is preferred; otherwise
// the tree is scanned a few levels deep and the shallowest one is used.
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
	return newProject(manifest), nil
}

// DetectAll returns one Project per project.godot in the tree. Returns nil, nil
// when none is found.
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
		out = append(out, newProject(m))
	}
	return out, nil
}

func newProject(manifest string) *Project {
	return &Project{
		ManifestPath:   manifest,
		PackageManager: defaultPlayCommand,
		RunEnv:         detectRunEnv(filepath.Dir(manifest)),
		PlayCommand:    defaultPlayCommand,
	}
}

// manifestInDir returns the project.godot sitting directly in dir, or "".
func manifestInDir(dir string) string {
	p := filepath.Join(dir, manifestName)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// discoverManifests does a depth-limited walk from root and returns the
// discovered project.godot files, shallowest first. A missing or unreadable
// root is surfaced as an error; per-entry walk errors are skipped so one
// unreadable subdir doesn't abort the whole scan.
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
			// A Godot project never nests another; stop descending.
			return filepath.SkipDir
		}
		return nil
	})
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

// BuildCommand creates an exec.Cmd that runs `name args...` from the project
// directory, optionally wrapped by the project's run environment. For nix, the
// flake lives at the repo root while project.godot may sit in a subdirectory,
// so the flake reference is resolved by walking up from workDir; the command
// itself still runs from workDir (so a launcher's `git rev-parse` and
// `--path` resolve correctly).
func BuildCommand(ctx context.Context, workDir, runEnv, name string, args []string) (*exec.Cmd, error) {
	switch runEnv {
	case "nix":
		flakeDir, _ := findFlakeDir(workDir)
		nixArgs := append([]string{"develop", flakeDir, "--command", name}, args...)
		cmd := exec.CommandContext(ctx, "nix", nixArgs...)
		cmd.Dir = workDir
		return cmd, nil
	case "devcontainer":
		containerID, wsFolder, err := devcontainerInfo(workDir)
		if err != nil {
			return nil, err
		}
		if containerID != "" {
			dockerArgs := append([]string{"exec", "-i", "-w", wsFolder, containerID, name}, args...)
			cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
			cmd.Dir = workDir
			return cmd, nil
		}
		dcArgs := append([]string{"exec", "--workspace-folder", workDir, "--", name}, args...)
		cmd := exec.CommandContext(ctx, "devcontainer", dcArgs...)
		cmd.Dir = workDir
		return cmd, nil
	default:
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = workDir
		return cmd, nil
	}
}

// nixFiles mark a directory as a nix flake/devenv root.
var nixFiles = []string{"flake.nix", "devenv.nix", ".devenv"}

// findFlakeDir walks up from dir looking for a flake/devenv marker. It returns
// the directory holding the marker and true, or (dir, false) when none is found
// up to the filesystem root.
func findFlakeDir(dir string) (string, bool) {
	for d := dir; ; {
		for _, name := range nixFiles {
			if _, err := os.Stat(filepath.Join(d, name)); err == nil {
				return d, true
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir, false
		}
		d = parent
	}
}

// detectRunEnv checks for environment-specific files near the project,
// searching upward for nix markers since the flake usually sits at the repo
// root above a `godot/` subdirectory.
func detectRunEnv(projectPath string) string {
	for _, name := range []string{".devcontainer", "devcontainer.json"} {
		if _, err := os.Stat(filepath.Join(projectPath, name)); err == nil {
			return "devcontainer"
		}
	}
	if _, ok := findFlakeDir(projectPath); ok {
		return "nix"
	}
	return ""
}
