// Package python inspects a working tree to detect a Python project (pyproject /
// Pipfile / requirements) and figure out which package manager the user is on
// (uv / poetry / pdm / pipenv / pip).
package python

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
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

// manifestNames are the files that mark a Python project, in priority order.
var manifestNames = []string{"pyproject.toml", "Pipfile", "requirements.txt", "setup.py"}

// maxWalkDepth bounds the subfolder scan used to discover Python projects that
// don't sit at the repo root (e.g. a polyglot repo with a python service under
// inference/). Matches the docker provider.
const maxWalkDepth = 3

// skipWalkDirs are vendored/output directories never worth descending into when
// hunting for Python manifests.
var skipWalkDirs = map[string]bool{
	"node_modules":  true,
	".git":          true,
	".venv":         true,
	"venv":          true,
	"env":           true,
	"dist":          true,
	"build":         true,
	"__pycache__":   true,
	".direnv":       true,
	".tox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	"target":        true,
	".next":         true,
}

// Detect returns nil (no error) when no Python manifest exists — that's the
// signal "this isn't a Python project". A root manifest is preferred (and keeps
// uv-workspace expansion anchored at the root); otherwise the tree is scanned a
// few levels deep and the shallowest discovered manifest is used as the primary
// one, so projects nested in subfolders are still surfaced.
func Detect(projectPath string) (*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	manifest := manifestInDir(projectPath)
	if manifest == "" {
		if found := discoverManifests(projectPath); len(found) > 0 {
			manifest = found[0]
		}
	}
	if manifest == "" {
		return nil, nil
	}
	dir := filepath.Dir(manifest)
	scripts, _ := ListScripts(manifest)
	return &Project{
		ManifestPath:   manifest,
		PackageManager: detectPackageManager(dir, manifest),
		RunEnv:         detectRunEnv(dir),
		Scripts:        scripts,
	}, nil
}

// manifestInDir returns the highest-priority manifest sitting directly in dir,
// or "" when the directory holds no Python manifest.
func manifestInDir(dir string) string {
	for _, name := range manifestNames {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// discoverManifests does a depth-limited walk from root and returns the best
// manifest of every directory that has one, ordered shallowest-first then
// alphabetically. Vendored/output directories are skipped.
func discoverManifests(root string) []string {
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel != "." {
			if skipWalkDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= maxWalkDepth {
				if skipWalkDirs[d.Name()] {
					return filepath.SkipDir
				}
				// At max depth: still inspect this dir, but don't recurse further.
				if m := manifestInDir(path); m != "" {
					found = append(found, m)
				}
				return filepath.SkipDir
			}
		}
		if m := manifestInDir(path); m != "" {
			found = append(found, m)
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
	return found
}

// DetectAll returns one Project per independent Python manifest found in the
// tree. Uv workspace members are excluded (they are handled by the workspace
// tabs of their root). Returns nil, nil when no manifest is found.
func DetectAll(projectPath string) ([]*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	all := discoverManifests(projectPath)
	if len(all) == 0 {
		return nil, nil
	}
	covered := map[string]bool{}
	for _, m := range all {
		for _, member := range uvMembers(m) {
			covered[filepath.Dir(member)] = true
		}
	}
	var out []*Project
	for _, m := range all {
		if covered[filepath.Dir(m)] {
			continue
		}
		dir := filepath.Dir(m)
		scripts, _ := ListScripts(m)
		out = append(out, &Project{
			ManifestPath:   m,
			PackageManager: detectPackageManager(dir, m),
			RunEnv:         detectRunEnv(dir),
			Scripts:        scripts,
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// pyproject mirrors the subset of pyproject.toml we read. Poetry dependency
// values and PDM script values are unions (string or table), so they're decoded
// as `any` and narrowed at use.
type pyproject struct {
	Project struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		Scripts              map[string]string   `toml:"scripts"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Dependencies map[string]any `toml:"dependencies"`
			Group        map[string]struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"group"`
			Scripts map[string]any `toml:"scripts"`
		} `toml:"poetry"`
		Pdm struct {
			DevDependencies map[string][]string `toml:"dev-dependencies"`
			Scripts         map[string]any      `toml:"scripts"`
		} `toml:"pdm"`
		Uv struct {
			Workspace struct {
				Members []string `toml:"members"`
			} `toml:"workspace"`
		} `toml:"uv"`
	} `toml:"tool"`
}

// pipfile mirrors the subset of a Pipfile (TOML) we read.
type pipfile struct {
	Packages    map[string]any    `toml:"packages"`
	DevPackages map[string]any    `toml:"dev-packages"`
	Scripts     map[string]string `toml:"scripts"`
}

// ListScripts re-reads runnable scripts from a known manifest. Cheap; called by
// the UI each time the user visits the Python page so edits are reflected
// without reopening the project. Scripts are gathered from PDM/Pipenv task
// tables, Poetry/PEP-621 entry points — whichever the manifest declares.
func ListScripts(manifestPath string) ([]Script, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	base := filepath.Base(manifestPath)
	scripts := map[string]string{}

	switch base {
	case "pyproject.toml":
		var pp pyproject
		if err := unmarshalTOML(manifestPath, &pp); err != nil {
			return nil, err
		}
		for name, cmd := range pp.Tool.Pdm.Scripts {
			if name == "_" {
				continue
			}
			scripts[name] = pdmScriptCommand(cmd)
		}
		for name, target := range pp.Project.Scripts {
			scripts[name] = target
		}
		for name, target := range pp.Tool.Poetry.Scripts {
			scripts[name] = scriptString(target)
		}
	case "Pipfile":
		var pf pipfile
		if err := unmarshalTOML(manifestPath, &pf); err != nil {
			return nil, err
		}
		for name, cmd := range pf.Scripts {
			scripts[name] = cmd
		}
	}

	out := make([]Script, 0, len(scripts))
	for name, command := range scripts {
		out = append(out, Script{Name: name, Command: command})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	// Locations lists every place this package is declared. In a uv workspace a
	// package may appear in several members; when those disagree on the version
	// spec it's a mismatch the UI can offer to resolve.
	Locations []DepLocation `json:"locations,omitempty"`
}

type DepLocation struct {
	Workspace string `json:"workspace"`
	Manifest  string `json:"manifest"`
	Version   string `json:"version"`
	Type      string `json:"type"`
}

type Workspace struct {
	Name         string       `json:"name"`
	Path         string       `json:"path"`
	Manifest     string       `json:"manifest"`
	IsRoot       bool         `json:"isRoot"`
	Dependencies []Dependency `json:"dependencies"`
}

var depTypeRank = map[string]int{
	"dependency":    3,
	"devDependency": 2,
	"optional":      1,
}

// ListWorkspaces returns every Python project under projectPath, each with its
// own dependencies: the primary (configured) manifest first, then any uv
// workspace members it declares, then independent projects discovered in
// subfolders (polyglot repos). A single-project repo yields one workspace.
func ListWorkspaces(projectPath, manifestPath string) ([]Workspace, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	manifests, err := workspaceManifests(projectPath, manifestPath)
	if err != nil {
		return nil, err
	}
	rootDir := projectPath
	if rootDir == "" {
		rootDir = filepath.Dir(manifestPath)
	}
	out := make([]Workspace, 0, len(manifests))
	for _, m := range manifests {
		deps, err := readManifestDeps(m)
		if err != nil {
			return nil, err
		}
		if deps == nil {
			deps = []Dependency{}
		}
		sort.Slice(deps, func(a, b int) bool { return deps[a].Name < deps[b].Name })
		rel, err := filepath.Rel(rootDir, filepath.Dir(m))
		if err != nil {
			rel = filepath.Dir(m)
		}
		name := manifestName(m)
		if name == "" {
			if rel == "." {
				name = filepath.Base(rootDir)
			} else {
				name = rel
			}
		}
		out = append(out, Workspace{
			Name:         name,
			Path:         rel,
			Manifest:     m,
			IsRoot:       filepath.Dir(m) == rootDir,
			Dependencies: deps,
		})
	}
	return out, nil
}

// ListPackages flattens every workspace's dependencies into a single list, kept
// once per package and classified by its strongest role (dependency >
// devDependency > optional). Each entry carries its Locations so the UI can
// target the right workspace for updates/removals and detect version mismatches.
func ListPackages(projectPath, manifestPath string) ([]Dependency, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	workspaces, err := ListWorkspaces(projectPath, manifestPath)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*Dependency)
	for _, ws := range workspaces {
		for _, d := range ws.Dependencies {
			loc := DepLocation{Workspace: ws.Name, Manifest: ws.Manifest, Version: d.Version, Type: d.Type}
			agg, ok := byName[d.Name]
			if !ok {
				dep := Dependency{Name: d.Name, Version: d.Version, Type: d.Type, Locations: []DepLocation{loc}}
				byName[d.Name] = &dep
				continue
			}
			agg.Locations = append(agg.Locations, loc)
			if depTypeRank[d.Type] > depTypeRank[agg.Type] {
				agg.Type = d.Type
				agg.Version = d.Version
			}
		}
	}
	out := make([]Dependency, 0, len(byName))
	for _, d := range byName {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// SetDependencyVersion rewrites the version spec of a package inside a single
// manifest, leaving the rest of the file's formatting untouched. It handles both
// PEP 621 / requirements list-strings ("pkg>=1.0") and Poetry/PDM table entries
// (pkg = "^1.0"). A no-op when the package isn't present.
func SetDependencyVersion(manifestPath, name, version string) error {
	if manifestPath == "" || name == "" || version == "" {
		return fmt.Errorf("manifest, name and version are required")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	q := regexp.QuoteMeta(name)
	changed := false

	// PEP 508 list-string form: "pkg>=1.0" / "pkg" (in pyproject [project] or
	// requirements.txt). Replace the trailing specifier with ==<version>.
	listForm := regexp.MustCompile(`("` + q + `)(\[[^\]]*\])?([^"]*)(")`)
	if listForm.Match(data) {
		data = listForm.ReplaceAll(data, []byte("${1}${2}=="+version+"${4}"))
		changed = true
	}

	// Poetry/PDM table form: pkg = "^1.0".
	tableForm := regexp.MustCompile(`(?m)^(\s*` + q + `\s*=\s*")[^"]*(")`)
	if tableForm.Match(data) {
		data = tableForm.ReplaceAll(data, []byte("${1}"+version+"${2}"))
		changed = true
	}

	if !changed {
		return nil
	}
	return os.WriteFile(manifestPath, data, 0o644)
}

// readManifestDeps reads dependencies from a single manifest (pyproject / Pipfile
// / requirements.txt). A missing file yields no deps (not an error) so a stale
// workspace glob doesn't break the whole listing.
func readManifestDeps(manifestPath string) ([]Dependency, error) {
	base := filepath.Base(manifestPath)
	switch base {
	case "pyproject.toml":
		return readPyprojectDeps(manifestPath)
	case "Pipfile":
		return readPipfileDeps(manifestPath)
	case "requirements.txt":
		return readRequirementsDeps(manifestPath)
	}
	return nil, nil
}

func readPyprojectDeps(manifestPath string) ([]Dependency, error) {
	var pp pyproject
	if err := unmarshalTOML(manifestPath, &pp); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Dependency
	for _, spec := range pp.Project.Dependencies {
		if name, ver := parsePEP508(spec); name != "" {
			out = append(out, Dependency{Name: name, Version: ver, Type: "dependency"})
		}
	}
	for _, specs := range pp.Project.OptionalDependencies {
		for _, spec := range specs {
			if name, ver := parsePEP508(spec); name != "" {
				out = append(out, Dependency{Name: name, Version: ver, Type: "optional"})
			}
		}
	}
	for name, v := range pp.Tool.Poetry.Dependencies {
		if strings.EqualFold(name, "python") {
			continue
		}
		out = append(out, Dependency{Name: name, Version: poetryDepVersion(v), Type: "dependency"})
	}
	for groupName, group := range pp.Tool.Poetry.Group {
		depType := "devDependency"
		if groupName != "dev" {
			depType = "optional"
		}
		for name, v := range group.Dependencies {
			if strings.EqualFold(name, "python") {
				continue
			}
			out = append(out, Dependency{Name: name, Version: poetryDepVersion(v), Type: depType})
		}
	}
	for _, specs := range pp.Tool.Pdm.DevDependencies {
		for _, spec := range specs {
			if name, ver := parsePEP508(spec); name != "" {
				out = append(out, Dependency{Name: name, Version: ver, Type: "devDependency"})
			}
		}
	}
	return out, nil
}

func readPipfileDeps(manifestPath string) ([]Dependency, error) {
	var pf pipfile
	if err := unmarshalTOML(manifestPath, &pf); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Dependency
	for name, v := range pf.Packages {
		out = append(out, Dependency{Name: name, Version: pipfileVersion(v), Type: "dependency"})
	}
	for name, v := range pf.DevPackages {
		out = append(out, Dependency{Name: name, Version: pipfileVersion(v), Type: "devDependency"})
	}
	return out, nil
}

func readRequirementsDeps(manifestPath string) ([]Dependency, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Dependency
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if name, ver := parsePEP508(line); name != "" {
			out = append(out, Dependency{Name: name, Version: ver, Type: "dependency"})
		}
	}
	return out, nil
}

// manifestName reads the project name from a pyproject ([project].name or
// [tool.poetry].name), returning "" when absent.
func manifestName(manifestPath string) string {
	if filepath.Base(manifestPath) != "pyproject.toml" {
		return ""
	}
	var parsed struct {
		Project struct {
			Name string `toml:"name"`
		} `toml:"project"`
		Tool struct {
			Poetry struct {
				Name string `toml:"name"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if err := unmarshalTOML(manifestPath, &parsed); err != nil {
		return ""
	}
	if parsed.Project.Name != "" {
		return parsed.Project.Name
	}
	return parsed.Tool.Poetry.Name
}

// workspaceManifests returns the primary manifest first, followed by every uv
// workspace member it declares, then any independent Python project discovered
// under projectPath (polyglot repos where the python project lives in a
// subfolder). Members are globs under [tool.uv.workspace], resolved relative to
// the manifest directory.
func workspaceManifests(projectPath, manifestPath string) ([]string, error) {
	seen := map[string]bool{}
	manifests := []string{}
	add := func(m string) {
		if m != "" && !seen[m] {
			seen[m] = true
			manifests = append(manifests, m)
		}
	}
	add(manifestPath)
	for _, m := range uvMembers(manifestPath) {
		add(m)
	}
	if projectPath != "" {
		for _, m := range discoverManifests(projectPath) {
			add(m)
			for _, mm := range uvMembers(m) {
				add(mm)
			}
		}
	}
	return manifests, nil
}

// uvMembers returns the member pyproject manifests declared under
// [tool.uv.workspace] in manifestPath, globs resolved relative to its directory.
func uvMembers(manifestPath string) []string {
	if filepath.Base(manifestPath) != "pyproject.toml" {
		return nil
	}
	var pp pyproject
	if err := unmarshalTOML(manifestPath, &pp); err != nil {
		return nil
	}
	rootDir := filepath.Dir(manifestPath)
	var out []string
	for _, pattern := range pp.Tool.Uv.Workspace.Members {
		matches, _ := filepath.Glob(filepath.Join(rootDir, pattern, "pyproject.toml"))
		out = append(out, matches...)
	}
	return out
}

// BuildCommand creates an exec.Cmd that runs `name args...` from workDir,
// optionally wrapped by the project's run environment (nix develop, etc.).
func BuildCommand(ctx context.Context, workDir, runEnv, name string, args []string) *exec.Cmd {
	switch runEnv {
	case "nix":
		nixArgs := append([]string{"develop", workDir, "--command", name}, args...)
		cmd := exec.CommandContext(ctx, "nix", nixArgs...)
		cmd.Dir = workDir
		return cmd
	case "devcontainer":
		if containerID, wsFolder := devcontainerInfo(workDir); containerID != "" {
			dockerArgs := append([]string{"exec", "-i", "-w", wsFolder, containerID, name}, args...)
			cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
			cmd.Dir = workDir
			return cmd
		}
		dcArgs := append([]string{"exec", "--workspace-folder", workDir, "--", name}, args...)
		cmd := exec.CommandContext(ctx, "devcontainer", dcArgs...)
		cmd.Dir = workDir
		return cmd
	default:
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = workDir
		return cmd
	}
}

// detectRunEnv checks for environment-specific files in the project directory.
// Devcontainer is checked first (most specific), then nix.
func detectRunEnv(projectPath string) string {
	for _, name := range []string{".devcontainer", "devcontainer.json"} {
		if _, err := os.Stat(filepath.Join(projectPath, name)); err == nil {
			return "devcontainer"
		}
	}
	for _, name := range []string{"devenv.nix", "flake.nix", ".devenv"} {
		if _, err := os.Stat(filepath.Join(projectPath, name)); err == nil {
			return "nix"
		}
	}
	return ""
}

// ensureDevcontainerUp checks whether the devcontainer for workDir is running,
// starts a stopped container directly via docker, or runs `devcontainer up`
// when the devcontainer CLI is available. Returns the container ID, whether
// this call started it (so the caller can stop it when done), and any error.
func ensureDevcontainerUp(workDir string) (containerID string, weStarted bool, err error) {
	absDir, absErr := filepath.Abs(workDir)
	if absErr != nil {
		absDir = workDir
	}
	filter := "label=devcontainer.local_folder=" + absDir
	// Already running — leave it alone when we're done.
	out, _ := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
	if id := strings.TrimSpace(string(out)); id != "" {
		return id, false, nil
	}
	// Stopped but exists — start it; we own the lifecycle.
	out, _ = exec.Command("docker", "ps", "-a", "--filter", filter, "-q").Output()
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		if err := exec.Command("docker", "start", ids[0]).Run(); err != nil {
			return "", false, err
		}
		return ids[0], true, nil
	}
	// No container — use devcontainer CLI when available.
	if _, err := exec.LookPath("devcontainer"); err == nil {
		cmd := exec.Command("devcontainer", "up", "--workspace-folder", workDir)
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", false, err
		}
		out2, _ := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
		return strings.TrimSpace(string(out2)), true, nil
	}
	return "", false, fmt.Errorf("no devcontainer found for this project; open it in VS Code first or install the devcontainer CLI")
}

// devcontainerInfo returns the container ID and workspace folder path inside
// the container for the devcontainer associated with workDir. Returns ("", "")
// when no running container is found.
func devcontainerInfo(workDir string) (containerID, wsFolder string) {
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		absDir = workDir
	}
	idOut, _ := exec.Command("docker", "ps",
		"--filter", "label=devcontainer.local_folder="+absDir,
		"-q",
	).Output()
	ids := strings.Fields(string(idOut))
	if len(ids) == 0 {
		return "", ""
	}
	containerID = ids[0]
	wsfOut, _ := exec.Command("docker", "inspect", containerID,
		"--format", `{{index .Config.Labels "devcontainer.workspace_folder"}}`).Output()
	wsFolder = strings.TrimSpace(string(wsfOut))
	if wsFolder == "" {
		wsFolder = "/workspaces/" + filepath.Base(workDir)
	}
	return containerID, wsFolder
}

// pipListCommand returns the executable + args that run `pip list ...` inside the
// project's environment for the given package manager, so outdated checks share
// one JSON parser across every manager.
func pipListCommand(pm string, args []string) (string, []string) {
	switch pm {
	case "uv":
		return "uv", append([]string{"pip", "list"}, args...)
	case "poetry", "pdm", "pipenv":
		return pm, append([]string{"run", "pip", "list"}, args...)
	default:
		return "pip", append([]string{"list"}, args...)
	}
}

type OutdatedPackage struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Wanted    string `json:"wanted"`
	Latest    string `json:"latest"`
	Workspace string `json:"workspace,omitempty"`
}

func CheckOutdatedPackages(manifestPath, packageManager, runEnv string) ([]OutdatedPackage, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "pip"
	}
	workDir := filepath.Dir(manifestPath)
	name, args := pipListCommand(pm, []string{"--outdated", "--format", "json"})
	cmd := BuildCommand(context.Background(), workDir, runEnv, name, args)
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	return parsePipOutdated(out)
}

// parsePipOutdated reads `pip list --outdated --format json`: an array of
// {name, version, latest_version} objects.
func parsePipOutdated(out []byte) ([]OutdatedPackage, error) {
	data := trimToJSON(out)
	if len(data) == 0 {
		return nil, nil
	}
	var raw []struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		LatestVersion string `json:"latest_version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("could not parse outdated output: %w", err)
	}
	var result []OutdatedPackage
	for _, e := range raw {
		if e.LatestVersion != "" && e.LatestVersion != e.Version {
			result = append(result, OutdatedPackage{
				Name:    normalizeName(e.Name),
				Current: e.Version,
				Wanted:  e.LatestVersion,
				Latest:  e.LatestVersion,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// CheckPackagesInstalled returns true when every declared dependency has a
// matching dist-info in the project's virtualenv site-packages. It's a fast
// filesystem check, not a full integrity check; when no in-tree venv is found it
// returns true (the manager keeps its env elsewhere — don't nag).
func CheckPackagesInstalled(projectPath, manifestPath string) (bool, error) {
	if manifestPath == "" {
		return false, fmt.Errorf("empty manifest path")
	}
	sitePackages := findSitePackages(filepath.Dir(manifestPath))
	if sitePackages == "" {
		return true, nil
	}
	installed, err := installedDistributions(sitePackages)
	if err != nil {
		return false, err
	}
	manifests, err := workspaceManifests(projectPath, manifestPath)
	if err != nil {
		return false, err
	}
	for _, m := range manifests {
		deps, err := readManifestDeps(m)
		if err != nil {
			return false, err
		}
		for _, p := range deps {
			if p.Type == "optional" {
				continue
			}
			if !installed[normalizeName(p.Name)] {
				return false, nil
			}
		}
	}
	return true, nil
}

// findSitePackages locates the site-packages dir of an in-tree virtualenv
// (.venv / venv / env), returning "" when none exists.
func findSitePackages(projectDir string) string {
	for _, venv := range []string{".venv", "venv", "env"} {
		base := filepath.Join(projectDir, venv)
		if _, err := os.Stat(filepath.Join(base, "pyvenv.cfg")); err != nil {
			continue
		}
		// Windows layout.
		if win := filepath.Join(base, "Lib", "site-packages"); dirExists(win) {
			return win
		}
		// POSIX layout: lib/pythonX.Y/site-packages.
		libDir := filepath.Join(base, "lib")
		entries, err := os.ReadDir(libDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "python") {
				if sp := filepath.Join(libDir, e.Name(), "site-packages"); dirExists(sp) {
					return sp
				}
			}
		}
	}
	return ""
}

// installedDistributions returns the set of normalized distribution names that
// have a *.dist-info or *.egg-info directory in site-packages.
func installedDistributions(sitePackages string) (map[string]bool, error) {
	entries, err := os.ReadDir(sitePackages)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		for _, suffix := range []string{".dist-info", ".egg-info"} {
			if strings.HasSuffix(name, suffix) {
				dist := strings.TrimSuffix(name, suffix)
				if dash := strings.IndexByte(dist, '-'); dash > 0 {
					dist = dist[:dash]
				}
				out[normalizeName(dist)] = true
			}
		}
	}
	return out, nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

type UnusedPackage struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

// CheckUnusedPackages runs deptry per workspace — against each workspace's own
// source rather than the (sourceless) monorepo root — and attributes each unused
// package to its workspace. deptry is fetched on demand via the package manager's
// exec command.
func CheckUnusedPackages(projectPath, manifestPath, packageManager, runEnv string) ([]UnusedPackage, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "pip"
	}
	workspaces, err := ListWorkspaces(projectPath, manifestPath)
	if err != nil {
		return nil, err
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result []UnusedPackage
	)
	for _, ws := range workspaces {
		if len(ws.Dependencies) == 0 {
			continue
		}
		wg.Add(1)
		go func(ws Workspace) {
			defer wg.Done()
			names := deptryUnused(filepath.Dir(ws.Manifest), pm, runEnv)
			if len(names) == 0 {
				return
			}
			mu.Lock()
			for _, n := range names {
				result = append(result, UnusedPackage{Name: normalizeName(n), Workspace: ws.Name})
			}
			mu.Unlock()
		}(ws)
	}
	wg.Wait()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Workspace != result[j].Workspace {
			return result[i].Workspace < result[j].Workspace
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func deptryUnused(workDir, pm, runEnv string) []string {
	tmp, err := os.CreateTemp("", "deptry-*.json")
	if err != nil {
		return nil
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	var execCmd string
	var execArgs []string
	deptryArgs := []string{"deptry", ".", "--json-output", tmpPath}
	switch pm {
	case "uv":
		execCmd, execArgs = "uvx", deptryArgs
	case "poetry", "pdm", "pipenv":
		execCmd, execArgs = pm, append([]string{"run"}, deptryArgs...)
	default:
		execCmd, execArgs = "deptry", deptryArgs[1:]
	}

	cmd := BuildCommand(context.Background(), workDir, runEnv, execCmd, execArgs)
	cmd.Env = os.Environ()
	_ = cmd.Run()

	data, err := os.ReadFile(tmpPath)
	if err != nil || len(data) == 0 {
		return nil
	}
	var issues []struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Module string `json:"module"`
	}
	if err := json.Unmarshal(trimToJSON(data), &issues); err != nil {
		return nil
	}
	var unused []string
	for _, issue := range issues {
		// DEP002: dependency declared but not imported.
		if issue.Error.Code == "DEP002" {
			unused = append(unused, issue.Module)
		}
	}
	return unused
}

type Vulnerability struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

var severityRank = map[string]int{"critical": 4, "high": 3, "moderate": 2, "low": 1, "info": 0}

// CheckVulnerabilities audits the installed dependency tree with pip-audit and
// returns one entry per advisory, keyed by package name. pip-audit is fetched on
// demand via the package manager's exec command.
func CheckVulnerabilities(manifestPath, packageManager, runEnv string) ([]Vulnerability, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "pip"
	}
	workDir := filepath.Dir(manifestPath)

	var execCmd string
	var execArgs []string
	auditArgs := []string{"pip-audit", "--format", "json"}
	switch pm {
	case "uv":
		execCmd, execArgs = "uvx", auditArgs
	case "poetry", "pdm", "pipenv":
		execCmd, execArgs = pm, append([]string{"run"}, auditArgs...)
	default:
		execCmd, execArgs = "pip-audit", auditArgs[1:]
	}

	cmd := BuildCommand(context.Background(), workDir, runEnv, execCmd, execArgs)
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	return parsePipAudit(out), nil
}

// parsePipAudit reads `pip-audit --format json`: a `dependencies` array, each
// with a `vulns` list of advisories. pip-audit doesn't report a severity, so it
// is left empty (the UI renders it as info).
func parsePipAudit(out []byte) []Vulnerability {
	data := trimToJSON(out)
	if len(data) == 0 {
		return nil
	}
	var parsed struct {
		Dependencies []struct {
			Name  string `json:"name"`
			Vulns []struct {
				ID          string   `json:"id"`
				Description string   `json:"description"`
				Aliases     []string `json:"aliases"`
			} `json:"vulns"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var result []Vulnerability
	for _, dep := range parsed.Dependencies {
		for _, v := range dep.Vulns {
			title := v.ID
			if v.Description != "" {
				title = fmt.Sprintf("%s: %s", v.ID, firstSentence(v.Description))
			}
			result = append(result, Vulnerability{
				Name:  normalizeName(dep.Name),
				Title: title,
				URL:   "https://osv.dev/vulnerability/" + v.ID,
			})
		}
	}
	sortVulnerabilities(result)
	return result
}

func sortVulnerabilities(v []Vulnerability) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Name != v[j].Name {
			return v[i].Name < v[j].Name
		}
		return severityRank[v[i].Severity] > severityRank[v[j].Severity]
	})
}

// trimToJSON drops any leading banner a tool prints before the JSON document.
func trimToJSON(out []byte) []byte {
	for i, b := range out {
		if b == '{' || b == '[' {
			return out[i:]
		}
	}
	return nil
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// detectPackageManager resolves the manager from lockfiles first (source of
// truth), then from pyproject [tool] tables, falling back to pip.
func detectPackageManager(projectPath, manifestPath string) string {
	for _, c := range []struct {
		lockfile string
		name     string
	}{
		{"uv.lock", "uv"},
		{"poetry.lock", "poetry"},
		{"pdm.lock", "pdm"},
		{"Pipfile.lock", "pipenv"},
		{"Pipfile", "pipenv"},
	} {
		if _, err := os.Stat(filepath.Join(projectPath, c.lockfile)); err == nil {
			return c.name
		}
	}
	if filepath.Base(manifestPath) == "pyproject.toml" {
		var pp pyproject
		if err := unmarshalTOML(manifestPath, &pp); err == nil {
			switch {
			case len(pp.Tool.Poetry.Dependencies) > 0 || len(pp.Tool.Poetry.Scripts) > 0:
				return "poetry"
			case len(pp.Tool.Pdm.Scripts) > 0 || len(pp.Tool.Pdm.DevDependencies) > 0:
				return "pdm"
			case len(pp.Tool.Uv.Workspace.Members) > 0:
				return "uv"
			}
		}
	}
	return "pip"
}

func unmarshalTOML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, v)
}

// parsePEP508 splits a PEP 508 requirement ("requests[security]>=2.0 ; marker")
// into its distribution name and version specifier.
func parsePEP508(spec string) (name, version string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", ""
	}
	// Drop environment markers and extras.
	if i := strings.IndexByte(spec, ';'); i >= 0 {
		spec = strings.TrimSpace(spec[:i])
	}
	if i := strings.IndexByte(spec, '['); i >= 0 {
		if j := strings.IndexByte(spec, ']'); j > i {
			spec = spec[:i] + spec[j+1:]
		}
	}
	m := pep508Name.FindStringIndex(spec)
	if m == nil {
		return "", ""
	}
	name = spec[m[0]:m[1]]
	version = strings.TrimSpace(spec[m[1]:])
	return name, version
}

var pep508Name = regexp.MustCompile(`^[A-Za-z0-9._-]+`)

// normalizeName applies PEP 503 normalization so the same package compares equal
// across manifests, pip output and site-packages.
func normalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return regexp.MustCompile(`[-_.]+`).ReplaceAllString(name, "-")
}

func poetryDepVersion(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if ver, ok := t["version"].(string); ok {
			return ver
		}
		if g, ok := t["git"].(string); ok {
			return g
		}
		if p, ok := t["path"].(string); ok {
			return p
		}
	}
	return ""
}

func pipfileVersion(v any) string {
	switch t := v.(type) {
	case string:
		if t == "*" {
			return ""
		}
		return t
	case map[string]any:
		if ver, ok := t["version"].(string); ok {
			if ver == "*" {
				return ""
			}
			return ver
		}
	}
	return ""
}

func pdmScriptCommand(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		for _, key := range []string{"cmd", "shell", "call"} {
			if s, ok := t[key].(string); ok {
				return s
			}
			if arr, ok := t[key].([]any); ok {
				parts := make([]string, 0, len(arr))
				for _, p := range arr {
					if s, ok := p.(string); ok {
						parts = append(parts, s)
					}
				}
				return strings.Join(parts, " ")
			}
		}
	}
	return ""
}

func scriptString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
