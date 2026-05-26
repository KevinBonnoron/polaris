// Package docker inspects a working tree for a Dockerfile and surfaces what can
// be learned about it at three levels, mirroring the gh provider's "use what the
// user already has" philosophy: nothing is bundled.
//
//   - Static: parsing the Dockerfile itself (stages, base images, ports, user,
//     cheap best-practice smells). Offline, no daemon, always available.
//   - Daemon: base image and layer sizes via the local `docker` CLI. Needs the
//     docker daemon running and the image present locally.
//   - Tooling: lint via hadolint, CVEs via trivy or grype. Each is detected on
//     $PATH at runtime and simply skipped when absent.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

var (
	ErrHadolintMissing = errors.New("hadolint not installed or not on PATH")
	ErrScannerMissing  = errors.New("no image scanner (trivy or grype) installed")
)

type Project struct {
	DockerfilePath string `json:"dockerfilePath"`
	ComposePath    string `json:"composePath,omitempty"`
}

// Detect returns nil (no error) when there is no Dockerfile — the signal "this
// isn't a docker project". Dockerfiles are commonly kept out of the repo root
// (docker/, .docker/, build/, deploy/) or suffixed (Dockerfile.prod), so we look
// in the usual places and then fall back to a bounded shallow walk.
func Detect(projectPath string) (*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	dockerfile := findDockerfile(projectPath)
	if dockerfile == "" {
		return nil, nil
	}
	out := &Project{DockerfilePath: dockerfile}
	dir := filepath.Dir(dockerfile)
	for _, base := range []string{projectPath, dir} {
		for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			p := filepath.Join(base, name)
			if _, err := os.Stat(p); err == nil {
				out.ComposePath = p
				return out, nil
			}
		}
	}
	return out, nil
}

// isDockerfile reports whether a filename is a Dockerfile ("Dockerfile" or
// "Dockerfile.<suffix>"), case-insensitively.
func isDockerfile(name string) bool {
	lower := strings.ToLower(name)
	return lower == "dockerfile" || strings.HasPrefix(lower, "dockerfile.")
}

// findDockerfile returns the most likely primary Dockerfile, preferring a plain
// root Dockerfile, then conventional subdirectories, then a shallow walk. An
// exact "Dockerfile" always wins over a suffixed variant at the same location.
func findDockerfile(projectPath string) string {
	if p := filepath.Join(projectPath, "Dockerfile"); fileExists(p) {
		return p
	}
	for _, sub := range []string{".", "docker", ".docker", "build", "deploy", "ci"} {
		if p := pickInDir(filepath.Join(projectPath, sub)); p != "" {
			return p
		}
	}
	return walkForDockerfile(projectPath)
}

// pickInDir returns the best Dockerfile directly inside dir: a plain
// "Dockerfile" if present, otherwise the first suffixed variant alphabetically.
func pickInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	variant := ""
	for _, e := range entries {
		if e.IsDir() || !isDockerfile(e.Name()) {
			continue
		}
		if strings.EqualFold(e.Name(), "Dockerfile") {
			return filepath.Join(dir, e.Name())
		}
		if variant == "" {
			variant = filepath.Join(dir, e.Name())
		}
	}
	return variant
}

var skipWalkDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	".direnv":      true,
	"target":       true,
	".next":        true,
}

// walkForDockerfile does a depth-limited walk as a last resort, skipping
// vendored/output directories.
func walkForDockerfile(projectPath string) string {
	found := ""
	const maxDepth = 3
	_ = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(projectPath, path)
			if rel != "." && (skipWalkDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= maxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if isDockerfile(d.Name()) {
			found = path
		}
		return nil
	})
	return found
}

// findDockerfiles collects ALL Dockerfiles in the project tree (conventional
// dirs + bounded walk), deduplicating by absolute path and returning them
// shallowest-first then alphabetically. Unlike findDockerfile which stops at
// the first match, this powers multi-instance detection for monorepos.
func findDockerfiles(projectPath string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	// Collect from conventional subdirectories first.
	for _, sub := range []string{".", "docker", ".docker", "build", "deploy", "ci"} {
		dir := filepath.Join(projectPath, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && isDockerfile(e.Name()) {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}
	// Bounded walk for anything deeper.
	const maxDepth = 3
	_ = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(projectPath, path)
			if rel != "." && (skipWalkDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= maxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if isDockerfile(d.Name()) {
			add(path)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		di := strings.Count(out[i], string(filepath.Separator))
		dj := strings.Count(out[j], string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return out[i] < out[j]
	})
	return out
}

// findComposePath looks for a compose file near a Dockerfile or at the project
// root, returning "" when none is found.
func findComposePath(projectPath, dockerfilePath string) string {
	dir := filepath.Dir(dockerfilePath)
	for _, base := range []string{projectPath, dir} {
		for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			p := filepath.Join(base, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// DetectAll returns one Project per Dockerfile found in the tree. Unlike Detect
// which returns only the best match, this surfaces every Dockerfile so
// monorepos with multiple images can be handled. Returns nil, nil when no
// Dockerfile is found.
func DetectAll(projectPath string) ([]*Project, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}
	paths := findDockerfiles(projectPath)
	if len(paths) == 0 {
		return nil, nil
	}
	out := make([]*Project, 0, len(paths))
	for _, p := range paths {
		out = append(out, &Project{
			DockerfilePath: p,
			ComposePath:    findComposePath(projectPath, p),
		})
	}
	return out, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

type Stage struct {
	Name      string `json:"name"`
	BaseImage string `json:"baseImage"`
	Tag       string `json:"tag"`
	Digest    string `json:"digest,omitempty"`
	Final     bool   `json:"final"`
}

// Finding is a static or lint observation. Severity is "info" | "warning" |
// "error" to line up with hadolint's levels.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
}

type Dockerfile struct {
	Path           string    `json:"path"`
	Stages         []Stage   `json:"stages"`
	ExposedPorts   []string  `json:"exposedPorts"`
	User           string    `json:"user"`
	HasHealthcheck bool      `json:"hasHealthcheck"`
	Findings       []Finding `json:"findings"`
}

var (
	secretKeyRe = regexp.MustCompile(`(?i)(password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)`)
	wsRe        = regexp.MustCompile(`\s+`)
)

// ParseDockerfile reads a Dockerfile and extracts its structure plus a handful
// of cheap static smells. It never builds or contacts the daemon.
func ParseDockerfile(path string) (*Dockerfile, error) {
	if path == "" {
		return nil, fmt.Errorf("empty dockerfile path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	df := &Dockerfile{Path: path, Stages: []Stage{}, ExposedPorts: []string{}, Findings: []Finding{}}
	userAfterFrom := false
	for _, ln := range logicalLines(string(data)) {
		instr, rest := splitInstruction(ln.text)
		switch strings.ToUpper(instr) {
		case "FROM":
			st := parseFrom(rest)
			df.Stages = append(df.Stages, st)
			userAfterFrom = false
			if st.Digest == "" && (st.Tag == "" || st.Tag == "latest") && st.BaseImage != "" && !strings.Contains(st.BaseImage, "${") {
				df.Findings = append(df.Findings, Finding{
					Rule:     "base-image-tag",
					Severity: "warning",
					Message:  fmt.Sprintf("Base image %q is not pinned to a fixed tag or digest; builds are not reproducible.", st.BaseImage),
					Line:     ln.num,
				})
			}
		case "EXPOSE":
			df.ExposedPorts = append(df.ExposedPorts, strings.Fields(rest)...)
		case "USER":
			df.User = strings.TrimSpace(rest)
			userAfterFrom = true
		case "HEALTHCHECK":
			if !strings.EqualFold(strings.TrimSpace(rest), "NONE") {
				df.HasHealthcheck = true
			}
		case "ENV", "ARG":
			if strings.Contains(rest, "=") && secretKeyRe.MatchString(rest) {
				df.Findings = append(df.Findings, Finding{
					Rule:     "secret-in-build",
					Severity: "warning",
					Message:  "Possible secret baked into the image via ENV/ARG; use build secrets or runtime env instead.",
					Line:     ln.num,
				})
			}
		case "ADD":
			for _, f := range strings.Fields(rest) {
				if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
					df.Findings = append(df.Findings, Finding{
						Rule:     "add-url",
						Severity: "info",
						Message:  "ADD with a remote URL; prefer COPY plus an explicit, checksummed download.",
						Line:     ln.num,
					})
					break
				}
			}
		}
	}
	if len(df.Stages) > 0 {
		df.Stages[len(df.Stages)-1].Final = true
		if !userAfterFrom {
			df.Findings = append(df.Findings, Finding{
				Rule:     "root-user",
				Severity: "warning",
				Message:  "Final image runs as root; declare a non-root USER.",
				Line:     0,
			})
		}
	}
	return df, nil
}

type logicalLine struct {
	text string
	num  int
}

// logicalLines strips comments/blank lines and joins backslash continuations
// into a single logical instruction, remembering where each started.
func logicalLines(content string) []logicalLine {
	var out []logicalLine
	var buf strings.Builder
	start := 0
	cont := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if !cont {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			start = i + 1
		}
		if strings.HasSuffix(trimmed, "\\") {
			buf.WriteString(strings.TrimSuffix(trimmed, "\\"))
			buf.WriteByte(' ')
			cont = true
			continue
		}
		buf.WriteString(trimmed)
		out = append(out, logicalLine{text: wsRe.ReplaceAllString(strings.TrimSpace(buf.String()), " "), num: start})
		buf.Reset()
		cont = false
	}
	if buf.Len() > 0 {
		out = append(out, logicalLine{text: wsRe.ReplaceAllString(strings.TrimSpace(buf.String()), " "), num: start})
	}
	return out
}

func splitInstruction(line string) (string, string) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.TrimSpace(parts[1])
}

func parseFrom(rest string) Stage {
	st := Stage{}
	fields := strings.Fields(rest)
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "--") {
			continue
		}
		if strings.EqualFold(f, "AS") && i+1 < len(fields) {
			st.Name = fields[i+1]
			break
		}
		if st.BaseImage == "" {
			st.BaseImage = f
		}
	}
	ref := st.BaseImage
	if at := strings.Index(ref, "@"); at >= 0 {
		st.Digest = ref[at+1:]
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		st.Tag = ref[colon+1:]
	}
	return st
}

type Capabilities struct {
	Docker       bool `json:"docker"`
	DockerDaemon bool `json:"dockerDaemon"`
	Hadolint     bool `json:"hadolint"`
	Trivy        bool `json:"trivy"`
	Grype        bool `json:"grype"`
}

// DetectCapabilities reports which optional tools are available. Like gh, we
// never install anything; the UI greys out whatever is missing.
func DetectCapabilities() Capabilities {
	caps := Capabilities{}
	if _, err := exec.LookPath("docker"); err == nil {
		caps.Docker = true
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
		sysexec.Hide(cmd)
		if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			caps.DockerDaemon = true
		}
	}
	if _, err := exec.LookPath("hadolint"); err == nil {
		caps.Hadolint = true
	}
	if _, err := exec.LookPath("trivy"); err == nil {
		caps.Trivy = true
	}
	if _, err := exec.LookPath("grype"); err == nil {
		caps.Grype = true
	}
	return caps
}

type Image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	ID         string `json:"id"`
	Size       string `json:"size"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// ListBaseImages returns the FROM base images of the Dockerfile that are present
// in the local image store, with their sizes. Stage cross-references (FROM
// builder) and unresolved ${ARG} refs are skipped.
func ListBaseImages(dockerfilePath string) ([]Image, error) {
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, ref := range baseImageRefs(df) {
		want[normalizeRef(ref)] = true
	}
	if len(want) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "image", "ls", "--no-trunc", "--format", "{{json .}}")
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker image ls: %w", err)
	}
	var images []Image
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e struct {
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			ID         string `json:"ID"`
			Size       string `json:"Size"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if !want[e.Repository+":"+e.Tag] {
			continue
		}
		images = append(images, Image{
			Repository: e.Repository,
			Tag:        e.Tag,
			ID:         shortID(e.ID),
			Size:       e.Size,
			SizeBytes:  parseSize(e.Size),
		})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].SizeBytes > images[j].SizeBytes })
	return images, nil
}

func baseImageRefs(df *Dockerfile) []string {
	names := map[string]bool{}
	for _, s := range df.Stages {
		if s.Name != "" {
			names[s.Name] = true
		}
	}
	seen := map[string]bool{}
	var refs []string
	for _, s := range df.Stages {
		ref := s.BaseImage
		if ref == "" || names[ref] || strings.Contains(ref, "${") {
			continue
		}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}

func normalizeRef(ref string) string {
	if at := strings.Index(ref, "@"); at >= 0 {
		return ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		return ref
	}
	return ref + ":latest"
}

type Layer struct {
	CreatedBy string `json:"createdBy"`
	Size      string `json:"size"`
	SizeBytes int64  `json:"sizeBytes"`
}

// ImageHistory returns the layer breakdown of an image, largest first not
// guaranteed (docker returns newest-first); the UI can sort as it likes.
func ImageHistory(ref string) ([]Layer, error) {
	if ref == "" {
		return nil, fmt.Errorf("empty image ref")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "history", "--no-trunc", "--format", "{{json .}}", ref)
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker history: %w", err)
	}
	var layers []Layer
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e struct {
			CreatedBy string `json:"CreatedBy"`
			Size      string `json:"Size"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		layers = append(layers, Layer{
			CreatedBy: cleanLayerCmd(e.CreatedBy),
			Size:      e.Size,
			SizeBytes: parseSize(e.Size),
		})
	}
	return layers, nil
}

func cleanLayerCmd(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/bin/sh -c #(nop) ")
	s = strings.TrimPrefix(s, "/bin/sh -c ")
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

// Lint runs hadolint and maps its findings. Returns ErrHadolintMissing when the
// binary is absent so the caller can show an install hint.
func Lint(dockerfilePath string) ([]Finding, error) {
	if dockerfilePath == "" {
		return nil, fmt.Errorf("empty dockerfile path")
	}
	if _, err := exec.LookPath("hadolint"); err != nil {
		return nil, ErrHadolintMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hadolint", "--format", "json", dockerfilePath)
	sysexec.Hide(cmd)
	out, _ := cmd.Output() // hadolint exits non-zero when it finds issues
	if len(out) == 0 {
		return nil, nil
	}
	var raw []struct {
		Line    int    `json:"line"`
		Code    string `json:"code"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse hadolint output: %w", err)
	}
	findings := make([]Finding, 0, len(raw))
	for _, r := range raw {
		findings = append(findings, Finding{Rule: r.Code, Severity: r.Level, Message: r.Message, Line: r.Line})
	}
	return findings, nil
}

type Vulnerability struct {
	ID           string `json:"id"`
	Package      string `json:"package"`
	Severity     string `json:"severity"`
	Installed    string `json:"installed"`
	FixedVersion string `json:"fixedVersion"`
	Title        string `json:"title"`
}

// ScanImage scans an image ref for HIGH/CRITICAL, fixable vulnerabilities,
// preferring trivy and falling back to grype. The aggressive filtering is
// deliberate: an unfiltered base-image scan returns hundreds of unactionable
// CVEs and drowns the signal.
func ScanImage(ref string) ([]Vulnerability, error) {
	if ref == "" {
		return nil, fmt.Errorf("empty image ref")
	}
	if _, err := exec.LookPath("trivy"); err == nil {
		return scanTrivy(ref)
	}
	if _, err := exec.LookPath("grype"); err == nil {
		return scanGrype(ref)
	}
	return nil, ErrScannerMissing
}

func scanTrivy(ref string) ([]Vulnerability, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "trivy", "image", "--quiet", "--format", "json", "--severity", "HIGH,CRITICAL", "--ignore-unfixed", ref)
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("trivy image: %w", err)
	}
	var parsed struct {
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID  string `json:"VulnerabilityID"`
				PkgName          string `json:"PkgName"`
				InstalledVersion string `json:"InstalledVersion"`
				FixedVersion     string `json:"FixedVersion"`
				Severity         string `json:"Severity"`
				Title            string `json:"Title"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse trivy output: %w", err)
	}
	var vulns []Vulnerability
	for _, r := range parsed.Results {
		for _, v := range r.Vulnerabilities {
			vulns = append(vulns, Vulnerability{
				ID:           v.VulnerabilityID,
				Package:      v.PkgName,
				Severity:     strings.ToUpper(v.Severity),
				Installed:    v.InstalledVersion,
				FixedVersion: v.FixedVersion,
				Title:        v.Title,
			})
		}
	}
	sortVulns(vulns)
	return vulns, nil
}

func scanGrype(ref string) ([]Vulnerability, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "grype", ref, "-o", "json")
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("grype: %w", err)
	}
	var parsed struct {
		Matches []struct {
			Vulnerability struct {
				ID          string `json:"id"`
				Severity    string `json:"severity"`
				Description string `json:"description"`
				Fix         struct {
					Versions []string `json:"versions"`
				} `json:"fix"`
			} `json:"vulnerability"`
			Artifact struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"artifact"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse grype output: %w", err)
	}
	var vulns []Vulnerability
	for _, m := range parsed.Matches {
		sev := strings.ToUpper(m.Vulnerability.Severity)
		if sev != "HIGH" && sev != "CRITICAL" {
			continue
		}
		fixed := strings.Join(m.Vulnerability.Fix.Versions, ", ")
		if fixed == "" {
			continue
		}
		vulns = append(vulns, Vulnerability{
			ID:           m.Vulnerability.ID,
			Package:      m.Artifact.Name,
			Severity:     sev,
			Installed:    m.Artifact.Version,
			FixedVersion: fixed,
			Title:        m.Vulnerability.Description,
		})
	}
	sortVulns(vulns)
	return vulns, nil
}

func sortVulns(v []Vulnerability) {
	rank := map[string]int{"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3}
	sort.Slice(v, func(i, j int) bool {
		if rank[v[i].Severity] != rank[v[j].Severity] {
			return rank[v[i].Severity] < rank[v[j].Severity]
		}
		return v[i].ID < v[j].ID
	})
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(strings.TrimSpace(s[i:])) {
	case "KB":
		return int64(num * 1e3)
	case "MB":
		return int64(num * 1e6)
	case "GB":
		return int64(num * 1e9)
	case "TB":
		return int64(num * 1e12)
	default:
		return int64(num)
	}
}
