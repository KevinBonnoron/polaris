// Package csharp inspects a working tree to detect a .NET / C# project (.csproj),
// surface its runnable targets (dotnet run / build / test, launch profiles) and
// manage its NuGet dependencies through the dotnet CLI.
package csharp

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

// manifestExts are the project-file extensions that mark a C# project.
var manifestExts = []string{".csproj"}

// maxWalkDepth bounds the subfolder scan used to discover projects that don't
// sit at the repo root (e.g. a solution with projects under src/). Matches the
// python provider.
const maxWalkDepth = 4

// skipWalkDirs are build-output/vendored directories never worth descending into
// when hunting for .csproj manifests.
var skipWalkDirs = map[string]bool{
	"bin":          true,
	"obj":          true,
	".git":         true,
	".vs":          true,
	".idea":        true,
	"node_modules": true,
	"packages":     true,
	"TestResults":  true,
	".godot":       true,
}

// Detect returns nil (no error) when no .csproj exists — that's the signal
// "this isn't a C# project". A root manifest is preferred; otherwise the tree is
// scanned a few levels deep and the shallowest discovered manifest is used.
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
	scripts, _ := ListScripts(manifest)
	return &Project{
		ManifestPath:   manifest,
		PackageManager: "dotnet",
		RunEnv:         detectRunEnv(filepath.Dir(manifest)),
		Scripts:        scripts,
	}, nil
}

// DetectAll returns one Project per .csproj found in the tree. Each project is an
// independent runnable instance. Returns nil, nil when no manifest is found.
func DetectAll(projectPath string) ([]*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	all := discoverManifests(projectPath)
	if len(all) == 0 {
		return nil, nil
	}
	out := make([]*Project, 0, len(all))
	for _, m := range all {
		scripts, _ := ListScripts(m)
		out = append(out, &Project{
			ManifestPath:   m,
			PackageManager: "dotnet",
			RunEnv:         detectRunEnv(filepath.Dir(m)),
			Scripts:        scripts,
		})
	}
	return out, nil
}

// manifestInDir returns a .csproj sitting directly in dir, or "" when none.
func manifestInDir(dir string) string {
	for _, ext := range manifestExts {
		matches, _ := filepath.Glob(filepath.Join(dir, "*"+ext))
		if len(matches) > 0 {
			sort.Strings(matches)
			return matches[0]
		}
	}
	return ""
}

// discoverManifests does a depth-limited walk from root and returns every
// .csproj found, ordered shallowest-first then alphabetically.
func discoverManifests(root string) []string {
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
				found = appendManifests(found, path)
				return filepath.SkipDir
			}
		}
		found = appendManifests(found, path)
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

// appendManifests adds every .csproj sitting directly in dir to found.
func appendManifests(found []string, dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.csproj"))
	sort.Strings(matches)
	return append(found, matches...)
}

// runTarget is a runnable dotnet target with its name and the exact argv to pass
// to `dotnet`. Keeping args as a slice (rather than a re-tokenized string) means
// launch profile names containing spaces (e.g. "IIS Express") stay intact.
type runTarget struct {
	name string
	args []string
}

// runTargets returns the runnable dotnet targets for a project: the standard
// verbs plus one entry per launch profile declared in launchSettings.json.
func runTargets(projectDir string) []runTarget {
	targets := []runTarget{
		{"run", []string{"run"}},
		{"build", []string{"build"}},
		{"test", []string{"test"}},
		{"watch", []string{"watch", "run"}},
		{"publish", []string{"publish"}},
	}
	for _, profile := range launchProfiles(projectDir) {
		targets = append(targets, runTarget{profile + " (profile)", []string{"run", "--launch-profile", profile}})
	}
	return targets
}

// ListScripts returns the runnable dotnet targets for the UI. Each script's
// Command is the display form of the `dotnet ...` invocation it runs.
func ListScripts(manifestPath string) ([]Script, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	targets := runTargets(filepath.Dir(manifestPath))
	scripts := make([]Script, 0, len(targets))
	for _, t := range targets {
		scripts = append(scripts, Script{Name: t.name, Command: "dotnet " + strings.Join(t.args, " ")})
	}
	return scripts, nil
}

// ResolveArgs maps a target name to its exact dotnet argv. A name that matches no
// known target is treated as a raw command and split on whitespace, so custom
// commands (e.g. "run --no-build") still work.
func ResolveArgs(manifestPath, script string) []string {
	for _, t := range runTargets(filepath.Dir(manifestPath)) {
		if t.name == script {
			return t.args
		}
	}
	return strings.Fields(script)
}

// launchProfiles reads the profile names from Properties/launchSettings.json,
// returning them sorted. Missing or unparseable files yield no profiles.
func launchProfiles(projectDir string) []string {
	data, err := os.ReadFile(filepath.Join(projectDir, "Properties", "launchSettings.json"))
	if err != nil {
		return nil
	}
	var parsed struct {
		Profiles map[string]struct {
			CommandName string `json:"commandName"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var out []string
	for name := range parsed.Profiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type Dependency struct {
	Name      string        `json:"name"`
	Version   string        `json:"version"`
	Type      string        `json:"type"`
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

// ListWorkspaces returns the single workspace backing a .csproj, with its NuGet
// PackageReferences resolved (including central versions from a
// Directory.Packages.props found up the tree). One csproj is one workspace.
func ListWorkspaces(projectPath, manifestPath string) ([]Workspace, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	deps, err := readManifestDeps(manifestPath)
	if err != nil {
		return nil, err
	}
	if deps == nil {
		deps = []Dependency{}
	}
	sort.Slice(deps, func(a, b int) bool { return deps[a].Name < deps[b].Name })

	rootDir := projectPath
	if rootDir == "" {
		rootDir = filepath.Dir(manifestPath)
	}
	rel, err := filepath.Rel(rootDir, filepath.Dir(manifestPath))
	if err != nil {
		rel = filepath.Dir(manifestPath)
	}
	name := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	return []Workspace{{
		Name:         name,
		Path:         rel,
		Manifest:     manifestPath,
		IsRoot:       filepath.Dir(manifestPath) == rootDir,
		Dependencies: deps,
	}}, nil
}

// ListPackages returns the project's dependencies, each carrying its single
// declaration location so the UI can target updates/removals consistently with
// the node/python providers.
func ListPackages(projectPath, manifestPath string) ([]Dependency, error) {
	workspaces, err := ListWorkspaces(projectPath, manifestPath)
	if err != nil {
		return nil, err
	}
	var out []Dependency
	for _, ws := range workspaces {
		for _, d := range ws.Dependencies {
			d.Locations = []DepLocation{{Workspace: ws.Name, Manifest: ws.Manifest, Version: d.Version, Type: d.Type}}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// csprojXML mirrors the subset of a .csproj we read: PackageReference items,
// whose version may be an attribute or a nested element.
type csprojXML struct {
	ItemGroups []struct {
		PackageReferences []struct {
			Include     string `xml:"Include,attr"`
			VersionAttr string `xml:"Version,attr"`
			VersionElem string `xml:"Version"`
		} `xml:"PackageReference"`
	} `xml:"ItemGroup"`
}

// packagesPropsXML mirrors the subset of a Directory.Packages.props we read for
// Central Package Management (versions live here, not on the PackageReference).
type packagesPropsXML struct {
	ItemGroups []struct {
		PackageVersions []struct {
			Include     string `xml:"Include,attr"`
			VersionAttr string `xml:"Version,attr"`
		} `xml:"PackageVersion"`
	} `xml:"ItemGroup"`
}

// readManifestDeps parses PackageReference entries from a .csproj, filling in
// versions from a Directory.Packages.props when the project uses Central Package
// Management. A missing file yields no deps (not an error).
func readManifestDeps(manifestPath string) ([]Dependency, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var proj csprojXML
	if err := xml.Unmarshal(data, &proj); err != nil {
		return nil, err
	}
	central := centralVersions(manifestPath)
	var out []Dependency
	for _, group := range proj.ItemGroups {
		for _, ref := range group.PackageReferences {
			if ref.Include == "" {
				continue
			}
			version := ref.VersionAttr
			if version == "" {
				version = strings.TrimSpace(ref.VersionElem)
			}
			if version == "" {
				version = central[ref.Include]
			}
			out = append(out, Dependency{Name: ref.Include, Version: version, Type: "dependency"})
		}
	}
	return out, nil
}

// centralVersions walks up from the project directory to find the nearest
// Directory.Packages.props and returns its package -> version map.
func centralVersions(manifestPath string) map[string]string {
	out := map[string]string{}
	dir := filepath.Dir(manifestPath)
	for {
		propsPath := filepath.Join(dir, "Directory.Packages.props")
		if data, err := os.ReadFile(propsPath); err == nil {
			var props packagesPropsXML
			if xml.Unmarshal(data, &props) == nil {
				for _, group := range props.ItemGroups {
					for _, pv := range group.PackageVersions {
						if pv.Include != "" {
							out[pv.Include] = pv.VersionAttr
						}
					}
				}
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out
}

// SetDependencyVersion rewrites the version of a package inside a single project
// file (or its Directory.Packages.props for Central Package Management), leaving
// the rest of the file untouched. A no-op when the package isn't present.
func SetDependencyVersion(manifestPath, name, version string) error {
	if manifestPath == "" || name == "" || version == "" {
		return fmt.Errorf("manifest, name and version are required")
	}
	if changed, err := rewriteVersion(manifestPath, name, version); err != nil || changed {
		return err
	}
	// Central Package Management: the version lives in Directory.Packages.props.
	dir := filepath.Dir(manifestPath)
	for {
		propsPath := filepath.Join(dir, "Directory.Packages.props")
		if _, err := os.Stat(propsPath); err == nil {
			_, err := rewriteVersion(propsPath, name, version)
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// rewriteVersion replaces the version of name inside the file at path, handling
// the attribute form (Version="..."), the nested-element form (<Version>...),
// and the central PackageVersion form. Returns whether anything changed.
func rewriteVersion(path, name, version string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	q := regexp.QuoteMeta(name)
	changed := false

	// Attribute form, either order. Both are applied (not else-if) so a file that
	// mixes Include-first and Version-first declarations of the same package
	// (e.g. across conditional ItemGroups) has every entry rewritten.
	includeFirst := regexp.MustCompile(`((?:PackageReference|PackageVersion)[^>]*Include="` + q + `"[^>]*Version=")[^"]*(")`)
	if includeFirst.Match(data) {
		data = includeFirst.ReplaceAll(data, []byte("${1}"+version+"${2}"))
		changed = true
	}
	versionFirst := regexp.MustCompile(`((?:PackageReference|PackageVersion)[^>]*Version=")[^"]*("[^>]*Include="` + q + `")`)
	if versionFirst.Match(data) {
		data = versionFirst.ReplaceAll(data, []byte("${1}"+version+"${2}"))
		changed = true
	}

	// <PackageReference Include="name">...<Version>...</Version>...</PackageReference>.
	elemForm := regexp.MustCompile(`(?s)(<PackageReference[^>]*Include="` + q + `"[^>]*>.*?<Version>)[^<]*(</Version>)`)
	if elemForm.Match(data) {
		data = elemForm.ReplaceAll(data, []byte("${1}"+version+"${2}"))
		changed = true
	}

	if !changed {
		return false, nil
	}
	return true, os.WriteFile(path, data, info.Mode())
}

type OutdatedPackage struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Wanted    string `json:"wanted"`
	Latest    string `json:"latest"`
	Workspace string `json:"workspace,omitempty"`
}

// CheckOutdatedPackages runs `dotnet list package --outdated` and parses the
// human-readable table. The project must be restored for dotnet to report.
func CheckOutdatedPackages(manifestPath, packageManager, runEnv string) ([]OutdatedPackage, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	workDir := filepath.Dir(manifestPath)
	cmd := BuildCommand(context.Background(), workDir, runEnv, "dotnet", []string{"list", "package", "--outdated"})
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	var result []OutdatedPackage
	for _, fields := range packageTableRows(out) {
		// > Name  Requested  Resolved  Latest
		if len(fields) < 5 {
			continue
		}
		latest := fields[4]
		if !looksLikeVersion(latest) {
			continue
		}
		result = append(result, OutdatedPackage{
			Name:    fields[1],
			Current: fields[3],
			Wanted:  latest,
			Latest:  latest,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

type Vulnerability struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

var severityRank = map[string]int{"critical": 4, "high": 3, "moderate": 2, "low": 1, "info": 0}

// CheckVulnerabilities runs `dotnet list package --vulnerable` and parses the
// advisories reported for top-level packages.
func CheckVulnerabilities(manifestPath, packageManager, runEnv string) ([]Vulnerability, error) {
	if manifestPath == "" {
		return nil, fmt.Errorf("empty manifest path")
	}
	workDir := filepath.Dir(manifestPath)
	cmd := BuildCommand(context.Background(), workDir, runEnv, "dotnet", []string{"list", "package", "--vulnerable"})
	cmd.Env = os.Environ()
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}
	var result []Vulnerability
	for _, fields := range packageTableRows(out) {
		// > Name  Requested  Resolved  Severity  AdvisoryURL
		if len(fields) < 6 {
			continue
		}
		severity := strings.ToLower(fields[4])
		result = append(result, Vulnerability{
			Name:     fields[1],
			Severity: severity,
			Title:    capitalizeFirst(severity) + " severity advisory",
			URL:      fields[5],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return severityRank[result[i].Severity] > severityRank[result[j].Severity]
	})
	return result, nil
}

// packageTableRows returns the whitespace-split fields of every `dotnet list
// package` table row (the lines beginning with the ">" marker).
func packageTableRows(out []byte) [][]string {
	var rows [][]string
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, ">") {
			continue
		}
		rows = append(rows, strings.Fields(trimmed))
	}
	return rows
}

type UnusedPackage struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

// CheckUnusedPackages is a no-op for .NET: the dotnet CLI has no built-in unused
// reference analysis. Kept for parity with the node/python providers.
func CheckUnusedPackages(projectPath, manifestPath, packageManager, runEnv string) ([]UnusedPackage, error) {
	return nil, nil
}

// CheckPackagesInstalled reports whether the project has been restored, i.e. an
// obj/project.assets.json sits next to the manifest.
func CheckPackagesInstalled(projectPath, manifestPath string) (bool, error) {
	if manifestPath == "" {
		return false, fmt.Errorf("empty manifest path")
	}
	assets := filepath.Join(filepath.Dir(manifestPath), "obj", "project.assets.json")
	if _, err := os.Stat(assets); err == nil {
		return true, nil
	}
	return false, nil
}

// BuildCommand creates an exec.Cmd that runs `name args...` from workDir,
// optionally wrapped by the project's run environment (nix develop, devcontainer).
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

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
