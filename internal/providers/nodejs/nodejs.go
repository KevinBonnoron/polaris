// Package nodejs inspects a working tree to detect a Node.js / Bun / Deno
// project (any package.json-based runtime) and figure out which package
// manager the user is on.
package nodejs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/KevinBonnoron/polaris/internal/providers/devenv"
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

// Detect returns nil (no error) when there is no package.json — that's the
// signal "this isn't a node project".
func Detect(projectPath string) (*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	manifest := filepath.Join(projectPath, "package.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed struct {
		Scripts        map[string]string `json:"scripts"`
		PackageManager string            `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	out := &Project{
		ManifestPath:   manifest,
		PackageManager: detectPackageManager(projectPath, parsed.PackageManager),
		RunEnv:         devenv.Detect(projectPath),
		Scripts:        make([]Script, 0, len(parsed.Scripts)),
	}
	for name, command := range parsed.Scripts {
		out.Scripts = append(out.Scripts, Script{Name: name, Command: command})
	}
	return out, nil
}

var skipWalkDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	".direnv":      true,
	"target":       true,
	".next":        true,
	".venv":        true,
	"venv":         true,
}

const maxWalkDepth = 3

func discoverPackageJsons(root string) []string {
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel != "." && (skipWalkDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= maxWalkDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "package.json" {
			found = append(found, path)
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

func workspaceMemberDirs(manifestPath string) map[string]bool {
	patterns := parseWorkspacePatterns(manifestWorkspaces(manifestPath))
	if len(patterns) == 0 {
		return nil
	}
	rootDir := filepath.Dir(manifestPath)
	covered := map[string]bool{}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(rootDir, pattern, "package.json"))
		for _, m := range matches {
			covered[filepath.Dir(m)] = true
		}
	}
	return covered
}

// DetectAll returns one Project per independent package.json found in the tree.
// Workspace members are excluded (they are handled by the workspace tabs of
// their root). Returns nil, nil when no package.json is found.
func DetectAll(projectPath string) ([]*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	if _, err := os.ReadDir(projectPath); err != nil {
		return nil, err
	}
	all := discoverPackageJsons(projectPath)
	if len(all) == 0 {
		return nil, nil
	}
	covered := map[string]bool{}
	for _, m := range all {
		for dir := range workspaceMemberDirs(m) {
			covered[dir] = true
		}
	}
	var out []*Project
	for _, m := range all {
		if covered[filepath.Dir(m)] {
			continue
		}
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var parsed struct {
			Scripts        map[string]string `json:"scripts"`
			PackageManager string            `json:"packageManager"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}
		p := &Project{
			ManifestPath:   m,
			PackageManager: detectPackageManager(filepath.Dir(m), parsed.PackageManager),
			RunEnv:         devenv.Detect(filepath.Dir(m)),
			Scripts:        make([]Script, 0, len(parsed.Scripts)),
		}
		for name, command := range parsed.Scripts {
			p.Scripts = append(p.Scripts, Script{Name: name, Command: command})
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ListScripts re-reads scripts from a known manifest path. Cheap; called by the
// UI each time the user visits the Node.js page so edits to package.json are
// reflected without reopening the project.
func ListScripts(manifestPath string) ([]Script, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	out := make([]Script, 0, len(parsed.Scripts))
	for name, command := range parsed.Scripts {
		out = append(out, Script{Name: name, Command: command})
	}
	return out, nil
}

type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	// Locations lists every place this package is declared. In a monorepo a
	// package may appear in several workspaces; when those disagree on the
	// version spec it's a mismatch the UI can offer to resolve.
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
	"dependency":     3,
	"devDependency":  2,
	"peerDependency": 1,
}

// ListWorkspaces returns the root package plus every child workspace it declares
// (bun/npm/yarn monorepos), each with its own dependencies. The root comes first;
// children follow in the order of the manifest's workspaces field.
func ListWorkspaces(manifestPath string) ([]Workspace, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	manifests, err := workspaceManifests(manifestPath)
	if err != nil {
		return nil, err
	}
	rootDir := filepath.Dir(manifestPath)
	out := make([]Workspace, 0, len(manifests))
	for i, m := range manifests {
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
			name = rel
		}
		out = append(out, Workspace{
			Name:         name,
			Path:         rel,
			Manifest:     m,
			IsRoot:       i == 0,
			Dependencies: deps,
		})
	}
	return out, nil
}

// ListPackages flattens every workspace's dependencies into a single list, kept
// once per package and classified by its strongest role (dependency >
// devDependency > peerDependency). Each entry carries its Locations so the UI can
// target the right workspace for updates/removals and detect version mismatches.
func ListPackages(manifestPath string) ([]Dependency, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	workspaces, err := ListWorkspaces(manifestPath)
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
// package.json, in whichever dependency map(s) declare it, leaving the rest of
// the file's formatting untouched. Used to align a package to one chosen spec
// across a monorepo's workspaces, including peerDependencies (which `add` can't
// target). A no-op when the package isn't present.
func SetDependencyVersion(manifestPath, name, version string) error {
	if manifestPath == "" || name == "" || version == "" {
		return fmt.Errorf("manifest, name and version are required")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	pattern := regexp.MustCompile(`("` + regexp.QuoteMeta(name) + `"\s*:\s*")[^"]*(")`)
	if !pattern.Match(data) {
		return nil
	}
	out := pattern.ReplaceAll(data, []byte("${1}"+version+"${2}"))
	return os.WriteFile(manifestPath, out, 0o644)
}

// readManifestDeps reads the three dependency maps from a single package.json. A
// missing file yields no deps (not an error) so a stale workspace glob doesn't
// break the whole listing.
func readManifestDeps(manifestPath string) ([]Dependency, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed struct {
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	var out []Dependency
	for name, ver := range parsed.Dependencies {
		out = append(out, Dependency{Name: name, Version: ver, Type: "dependency"})
	}
	for name, ver := range parsed.DevDependencies {
		out = append(out, Dependency{Name: name, Version: ver, Type: "devDependency"})
	}
	for name, ver := range parsed.PeerDependencies {
		out = append(out, Dependency{Name: name, Version: ver, Type: "peerDependency"})
	}
	return out, nil
}

// manifestName reads the "name" field from a package.json, returning "" when the
// file is missing or nameless.
func manifestName(manifestPath string) string {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ""
	}
	return parsed.Name
}

// workspaceManifests returns the root manifest followed by every child workspace
// manifest it points to. The workspaces field accepts either an array of globs
// (bun/npm/yarn modern) or a { "packages": [...] } object (yarn classic), each
// globbed relative to the manifest directory.
func workspaceManifests(manifestPath string) ([]string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	manifests := []string{manifestPath}
	rootDir := filepath.Dir(manifestPath)
	seen := map[string]bool{manifestPath: true}
	for _, pattern := range parseWorkspacePatterns(parsed.Workspaces) {
		matches, _ := filepath.Glob(filepath.Join(rootDir, pattern, "package.json"))
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				manifests = append(manifests, m)
			}
		}
	}
	return manifests, nil
}

func parseWorkspacePatterns(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

// BuildCommand creates an exec.Cmd that runs the package manager with the given
// args, optionally wrapped by the project's run environment (nix develop, etc.).
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
		pm = "npm"
	}

	workDir := filepath.Dir(manifestPath)
	hasWorkspaces := len(parseWorkspacePatterns(manifestWorkspaces(manifestPath))) > 0

	if pm == "bun" {
		// --filter '*' makes bun traverse every workspace; it then appends a
		// Workspace column the parser reads. Without workspaces, plain outdated.
		args := []string{"outdated"}
		if hasWorkspaces {
			args = append(args, "--filter", "*")
		}
		cmd, err := devenv.BuildCommand(context.Background(), workDir, runEnv, pm, args)
		if err != nil {
			return nil, nil
		}
		cmd.Env = os.Environ()
		out, _ := cmd.Output()
		return parseBunOutdated(out), nil
	}

	cmd, err := devenv.BuildCommand(context.Background(), workDir, runEnv, pm, []string{"outdated", "--json"})
	if err != nil {
		return nil, nil
	}
	cmd.Env = os.Environ()

	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	return parseNpmOutdated(out)
}

// parseNpmOutdated handles `npm outdated --json`. Each value is normally an
// object, but in a workspace tree npm emits an array (one entry per dependent
// workspace) — both shapes are supported.
func parseNpmOutdated(out []byte) ([]OutdatedPackage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("could not parse outdated output: %w", err)
	}
	type entry struct {
		Current   string `json:"current"`
		Wanted    string `json:"wanted"`
		Latest    string `json:"latest"`
		Dependent string `json:"dependent"`
	}
	var result []OutdatedPackage
	add := func(name string, e entry) {
		if e.Latest != "" && e.Latest != e.Current {
			result = append(result, OutdatedPackage{
				Name:      name,
				Current:   e.Current,
				Wanted:    e.Wanted,
				Latest:    e.Latest,
				Workspace: e.Dependent,
			})
		}
	}
	for name, msg := range raw {
		var one entry
		if err := json.Unmarshal(msg, &one); err == nil {
			add(name, one)
			continue
		}
		var many []entry
		if err := json.Unmarshal(msg, &many); err == nil {
			for _, e := range many {
				add(name, e)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// manifestWorkspaces returns the raw workspaces field of a package.json (nil when
// absent), used to decide whether outdated must traverse workspaces.
func manifestWorkspaces(manifestPath string) json.RawMessage {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	var parsed struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	return parsed.Workspaces
}

// CheckPackagesInstalled returns true when every declared dependency
// (excluding peers, which are typically provided externally) has a folder in
// node_modules. It's a fast filesystem check, not a full integrity check.
func CheckPackagesInstalled(manifestPath string) (bool, error) {
	if manifestPath == "" {
		return false, fmt.Errorf("empty manifest path")
	}
	manifests, err := workspaceManifests(manifestPath)
	if err != nil {
		return false, err
	}
	rootModules := filepath.Join(filepath.Dir(manifestPath), "node_modules")
	for _, m := range manifests {
		deps, err := readManifestDeps(m)
		if err != nil {
			return false, err
		}
		localModules := filepath.Join(filepath.Dir(m), "node_modules")
		for _, p := range deps {
			if p.Type == "peerDependency" {
				continue
			}
			// A workspace dep is satisfied either by hoisting to the root
			// node_modules or by its own local one.
			if dirExists(filepath.Join(rootModules, p.Name)) || dirExists(filepath.Join(localModules, p.Name)) {
				continue
			}
			return false, nil
		}
	}
	return true, nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

type UnusedPackage struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

// depcheckIgnores are tooling dependencies depcheck routinely false-flags
// because they are consumed via the CLI/config rather than imported.
var depcheckIgnores = []string{"typescript", "@types/*", "tslib"}

// CheckUnusedPackages runs depcheck per workspace — against each workspace's own
// source rather than the (sourceless) monorepo root, which made the result
// meaningless — and attributes each unused package to its workspace. depcheck is
// fetched on demand via the package manager's exec command.
func CheckUnusedPackages(manifestPath, packageManager, runEnv string) ([]UnusedPackage, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "npm"
	}
	workspaces, err := ListWorkspaces(manifestPath)
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
			names := depcheckUnused(filepath.Dir(ws.Manifest), pm, runEnv)
			if len(names) == 0 {
				return
			}
			mu.Lock()
			for _, n := range names {
				result = append(result, UnusedPackage{Name: n, Workspace: ws.Name})
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

func depcheckUnused(workDir, pm, runEnv string) []string {
	ignores := "--ignores=" + strings.Join(depcheckIgnores, ",")
	var execCmd string
	var execArgs []string
	switch pm {
	case "bun":
		execCmd, execArgs = "bunx", []string{"depcheck", "--json", ignores}
	case "pnpm":
		execCmd, execArgs = "pnpm", []string{"dlx", "depcheck", "--json", ignores}
	case "yarn":
		execCmd, execArgs = "yarn", []string{"dlx", "depcheck", "--json", ignores}
	default:
		execCmd, execArgs = "npx", []string{"--yes", "depcheck", "--json", ignores}
	}

	cmd, err := devenv.BuildCommand(context.Background(), workDir, runEnv, execCmd, execArgs)
	if err != nil {
		return nil
	}
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil
	}
	var parsed struct {
		Dependencies    []string `json:"dependencies"`
		DevDependencies []string `json:"devDependencies"`
	}
	if err := json.Unmarshal(trimToJSON(out), &parsed); err != nil {
		return nil
	}
	return append(parsed.Dependencies, parsed.DevDependencies...)
}

type Vulnerability struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

var severityRank = map[string]int{"critical": 4, "high": 3, "moderate": 2, "low": 1, "info": 0}

// CheckVulnerabilities audits the installed dependency tree (bun audit / npm
// audit) and returns one entry per advisory, keyed by package name. The audit
// runs once at the root: it covers every workspace's installed packages, which
// are hoisted into the root tree.
func CheckVulnerabilities(manifestPath, packageManager, runEnv string) ([]Vulnerability, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	pm := packageManager
	if pm == "" {
		pm = "npm"
	}
	workDir := filepath.Dir(manifestPath)
	cmd, err := devenv.BuildCommand(context.Background(), workDir, runEnv, pm, []string{"audit", "--json"})
	if err != nil {
		return nil, err
	}
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	if pm == "bun" {
		return parseBunAudit(out), nil
	}
	return parseNpmAudit(out), nil
}

// parseBunAudit reads `bun audit --json`: a map of package name to its list of
// advisories.
func parseBunAudit(out []byte) []Vulnerability {
	data := trimToJSON(out)
	if len(data) == 0 {
		return nil
	}
	var parsed map[string][]struct {
		URL      string `json:"url"`
		Title    string `json:"title"`
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var result []Vulnerability
	for name, advisories := range parsed {
		for _, a := range advisories {
			result = append(result, Vulnerability{Name: name, Severity: a.Severity, Title: a.Title, URL: a.URL})
		}
	}
	sortVulnerabilities(result)
	return result
}

// parseNpmAudit reads `npm audit --json` (npm v7+): a `vulnerabilities` object
// keyed by package, each with a `via` list of advisory objects (direct) or
// package-name strings (transitive).
func parseNpmAudit(out []byte) []Vulnerability {
	data := trimToJSON(out)
	if len(data) == 0 {
		return nil
	}
	var parsed struct {
		Vulnerabilities map[string]struct {
			Severity string            `json:"severity"`
			Via      []json.RawMessage `json:"via"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var result []Vulnerability
	for name, v := range parsed.Vulnerabilities {
		direct := false
		for _, raw := range v.Via {
			var adv struct {
				Title    string `json:"title"`
				URL      string `json:"url"`
				Severity string `json:"severity"`
			}
			if err := json.Unmarshal(raw, &adv); err == nil && adv.Title != "" {
				severity := adv.Severity
				if severity == "" {
					severity = v.Severity
				}
				result = append(result, Vulnerability{Name: name, Severity: severity, Title: adv.Title, URL: adv.URL})
				direct = true
			}
		}
		if !direct {
			result = append(result, Vulnerability{Name: name, Severity: v.Severity, Title: fmt.Sprintf("Vulnerable transitive dependency (%s)", name)})
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

// trimToJSON drops any leading banner a package manager prints before the JSON
// document (bun audit emits a version line on stdout).
func trimToJSON(out []byte) []byte {
	for i, b := range out {
		if b == '{' || b == '[' {
			return out[i:]
		}
	}
	return nil
}

// parseBunOutdated reads bun's table output. Rows have 4 columns normally, or 5
// under `--filter '*'` where bun appends the originating workspace:
//
//	| @tanstack/react-form | 1.32.1 | 1.33.0 | 1.33.0 |
//	| lodash               | 4.17.0 | 4.17.0 | 4.18.1 | web |
//
// Dependencies are suffixed with " (dev)"/" (peer)" in the Package column.
func parseBunOutdated(out []byte) []OutdatedPackage {
	var result []OutdatedPackage
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|--") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "|"), "|")
		if len(cols) != 4 && len(cols) != 5 {
			continue
		}
		name := strings.TrimSpace(cols[0])
		current := strings.TrimSpace(cols[1])
		wanted := strings.TrimSpace(cols[2])
		latest := strings.TrimSpace(cols[3])
		workspace := ""
		if len(cols) == 5 {
			workspace = strings.TrimSpace(cols[4])
		}
		if name == "" || name == "Package" {
			continue
		}
		name = strings.TrimSuffix(name, " (dev)")
		name = strings.TrimSuffix(name, " (peer)")
		name = strings.TrimSuffix(name, " (optional)")
		if latest == "" || latest == current {
			continue
		}
		result = append(result, OutdatedPackage{
			Name:      name,
			Current:   current,
			Wanted:    wanted,
			Latest:    latest,
			Workspace: workspace,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// The `packageManager` field in package.json wins (it's the source of truth
// per corepack); lockfile sniffing is the fallback.
func detectPackageManager(projectPath, packageManagerField string) string {
	if packageManagerField != "" {
		if at := strings.IndexByte(packageManagerField, '@'); at > 0 {
			return packageManagerField[:at]
		}
		return packageManagerField
	}
	for _, c := range []struct {
		lockfile string
		name     string
	}{
		{"bun.lock", "bun"},
		{"bun.lockb", "bun"},
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
		{"deno.lock", "deno"},
	} {
		if _, err := os.Stat(filepath.Join(projectPath, c.lockfile)); err == nil {
			return c.name
		}
	}
	return "npm"
}
