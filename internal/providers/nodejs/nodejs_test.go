package nodejs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// monorepo writes a root manifest with two workspaces and returns the root
// manifest path. react is declared at conflicting versions across a and b.
func monorepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rootManifest := writeManifest(t, root, `{
		"name": "root",
		"workspaces": ["packages/*"],
		"devDependencies": {"typescript": "^5.0.0"}
	}`)
	writeManifest(t, filepath.Join(root, "packages", "a"), `{
		"name": "a",
		"dependencies": {"react": "^18.2.0", "lodash": "^4.17.21"}
	}`)
	writeManifest(t, filepath.Join(root, "packages", "b"), `{
		"name": "b",
		"dependencies": {"react": "^17.0.2"},
		"peerDependencies": {"react": ">=17"}
	}`)
	return rootManifest
}

func TestListWorkspaces(t *testing.T) {
	root := monorepo(t)
	ws, err := ListWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 3 {
		t.Fatalf("want 3 workspaces, got %d", len(ws))
	}
	if !ws[0].IsRoot || ws[0].Name != "root" {
		t.Errorf("first workspace should be root, got %+v", ws[0])
	}
	byName := map[string]Workspace{}
	for _, w := range ws {
		byName[w.Name] = w
	}
	if got := len(byName["a"].Dependencies); got != 2 {
		t.Errorf("workspace a: want 2 deps, got %d", got)
	}
	if byName["a"].Path != filepath.Join("packages", "a") {
		t.Errorf("workspace a path = %q", byName["a"].Path)
	}
}

func TestListPackagesDedupAndConflict(t *testing.T) {
	root := monorepo(t)
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Dependency{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	react, ok := byName["react"]
	if !ok {
		t.Fatal("react missing")
	}
	if react.Type != "dependency" {
		t.Errorf("react should rank as dependency, got %q", react.Type)
	}
	if len(react.Locations) != 3 {
		t.Errorf("react should have 3 locations, got %d", len(react.Locations))
	}
	versions := map[string]bool{}
	for _, l := range react.Locations {
		versions[l.Version] = true
	}
	if len(versions) < 2 {
		t.Errorf("react should expose a version conflict, got versions %v", versions)
	}
	if len(byName["lodash"].Locations) != 1 {
		t.Errorf("lodash should have 1 location, got %d", len(byName["lodash"].Locations))
	}
}

func TestListPackagesSingleRepo(t *testing.T) {
	dir := t.TempDir()
	root := writeManifest(t, dir, `{
		"name": "solo",
		"dependencies": {"react": "^18.0.0"},
		"devDependencies": {"vitest": "^1.0.0"}
	}`)
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("want 2 packages, got %d", len(pkgs))
	}
	for _, p := range pkgs {
		if len(p.Locations) != 1 {
			t.Errorf("%s: want 1 location, got %d", p.Name, len(p.Locations))
		}
	}
}

func TestParseBunOutdatedFourColumns(t *testing.T) {
	out := `bun outdated v1.3.13
|----------------------------------------------|
| Package          | Current | Update | Latest |
|------------------|---------|--------|--------|
| typescript (dev) | 5.0.2   | 5.0.2  | 6.0.3  |
|----------------------------------------------|`
	got := parseBunOutdated([]byte(out))
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	if got[0].Name != "typescript" || got[0].Latest != "6.0.3" || got[0].Workspace != "" {
		t.Errorf("unexpected row %+v", got[0])
	}
}

func TestParseBunOutdatedFiveColumns(t *testing.T) {
	out := `bun outdated v1.3.13
| Package          | Current | Update | Latest | Workspace |
|------------------|---------|--------|--------|-----------|
| is-odd           | 1.0.0   | 1.0.0  | 3.0.1  | web       |
| lodash           | 4.17.0  | 4.17.0 | 4.18.1 | web       |
| typescript (dev) | 5.0.2   | 5.0.2  | 6.0.3  | root      |`
	got := parseBunOutdated([]byte(out))
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	byName := map[string]OutdatedPackage{}
	for _, o := range got {
		byName[o.Name] = o
	}
	if byName["is-odd"].Workspace != "web" {
		t.Errorf("is-odd workspace = %q", byName["is-odd"].Workspace)
	}
	if byName["typescript"].Workspace != "root" {
		t.Errorf("typescript workspace = %q", byName["typescript"].Workspace)
	}
}

func TestSetDependencyVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := writeManifest(t, dir, `{
  "name": "root",
  "devDependencies": {
    "turbo": "^2.9.16",
    "typescript": "6.0.3"
  },
  "peerDependencies": {
    "typescript": "^6.0.3"
  }
}`)
	if err := SetDependencyVersion(manifest, "typescript", "6.0.3"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(manifest)
	got := string(out)
	// Both occurrences aligned, the unrelated dep and formatting untouched.
	if want := `"typescript": "6.0.3"`; !contains(got, want) {
		t.Errorf("expected aligned typescript, got:\n%s", got)
	}
	if contains(got, `"typescript": "^6.0.3"`) {
		t.Errorf("peer spec not aligned:\n%s", got)
	}
	if !contains(got, `"turbo": "^2.9.16"`) {
		t.Errorf("unrelated dep changed:\n%s", got)
	}
}

func TestSetDependencyVersionNoMatch(t *testing.T) {
	dir := t.TempDir()
	manifest := writeManifest(t, dir, `{"name":"x","dependencies":{"react":"^18.0.0"}}`)
	before, _ := os.ReadFile(manifest)
	if err := SetDependencyVersion(manifest, "vue", "3.0.0"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Errorf("file changed for absent package")
	}
}

func TestParseBunAudit(t *testing.T) {
	out := `bun audit v1.3.13 (bf2e2cec)
{"lodash":[{"id":1,"url":"https://a","title":"Prototype Pollution","severity":"critical"},{"id":2,"url":"https://b","title":"ReDoS","severity":"moderate"}]}`
	got := parseBunAudit([]byte(out))
	if len(got) != 2 {
		t.Fatalf("want 2 advisories, got %d", len(got))
	}
	// Sorted by severity desc within a package: critical first.
	if got[0].Severity != "critical" || got[0].Name != "lodash" {
		t.Errorf("unexpected first advisory %+v", got[0])
	}
}

func TestParseNpmAudit(t *testing.T) {
	out := `{"vulnerabilities":{"lodash":{"name":"lodash","severity":"high","via":[{"title":"Prototype Pollution","url":"https://x","severity":"high"}]},"minimist":{"name":"minimist","severity":"low","via":["lodash"]}}}`
	got := parseNpmAudit([]byte(out))
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	byName := map[string]Vulnerability{}
	for _, v := range got {
		byName[v.Name] = v
	}
	if byName["lodash"].Title != "Prototype Pollution" || byName["lodash"].URL != "https://x" {
		t.Errorf("lodash advisory mis-parsed: %+v", byName["lodash"])
	}
	if byName["minimist"].Severity != "low" {
		t.Errorf("transitive entry mis-parsed: %+v", byName["minimist"])
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestParseNpmOutdatedObjectAndArray(t *testing.T) {
	// object form (single repo) and array form (workspaces) in one payload.
	out := `{
		"lodash": {"current": "4.17.0", "wanted": "4.17.21", "latest": "4.18.1", "dependent": "root"},
		"react": [
			{"current": "17.0.2", "wanted": "17.0.2", "latest": "19.0.0", "dependent": "client"},
			{"current": "18.2.0", "wanted": "18.2.0", "latest": "19.0.0", "dependent": "server"}
		]
	}`
	got, err := parseNpmOutdated([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	workspaces := map[string]bool{}
	for _, o := range got {
		if o.Name == "react" {
			workspaces[o.Workspace] = true
		}
	}
	if !workspaces["client"] || !workspaces["server"] {
		t.Errorf("react workspaces = %v", workspaces)
	}
}
