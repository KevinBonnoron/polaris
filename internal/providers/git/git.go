// Package git inspects a local working tree to extract metadata the UI can
// use to pre-fill integration configuration (provider, owner, repo, ...).
package git

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
	ProviderUnknown   Provider = "unknown"
)

type Remote struct {
	Provider Provider `json:"provider"`
	Host     string   `json:"host"`
	BaseURL  string   `json:"baseUrl"`
	Owner    string   `json:"owner"`
	Repo     string   `json:"repo"`
	URL      string   `json:"url"`
}

// DetectRemote walks up from projectPath to find a `.git` directory, reads
// its config, and returns the parsed `origin` remote. The fallback when the
// project doesn't have one yet is a nil Remote with no error.
func DetectRemote(projectPath string) (*Remote, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}

	gitDir, err := findGitDir(projectPath)
	if err != nil {
		return nil, err
	}

	if gitDir == "" {
		return nil, nil
	}

	originURL, err := readOriginURL(filepath.Join(gitDir, "config"))
	if err != nil {
		return nil, err
	}

	if originURL == "" {
		return nil, nil
	}

	return parseRemote(originURL), nil
}

func findGitDir(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		// Worktrees and submodules use a `.git` file pointing at the real dir.
		if err == nil && !info.IsDir() {
			real, rerr := resolveGitFile(candidate)
			if rerr != nil {
				return "", rerr
			}
			if real != "" {
				return real, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func resolveGitFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return target, nil
}

func readOriginURL(configPath string) (string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}

		if !inOrigin {
			continue
		}

		if eq := strings.IndexByte(line, '='); eq > 0 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if strings.EqualFold(key, "url") {
				return val, nil
			}
		}
	}

	return "", scanner.Err()
}

// parseRemote accepts the common forms produced by git:
//
//	https://host/owner/repo[.git]
//	http://host/owner/repo[.git]
//	ssh://git@host[:port]/owner/repo[.git]
//	git@host:owner/repo[.git]
//
// It populates Provider via a simple host suffix match.
func parseRemote(raw string) *Remote {
	r := &Remote{URL: raw, Provider: ProviderUnknown}
	host, path := splitRemote(raw)
	r.Host = host
	owner, repo := splitPath(path)
	r.Owner = owner
	r.Repo = repo
	r.Provider = providerFor(host)
	if host != "" {
		scheme := "https"
		if strings.HasPrefix(strings.ToLower(raw), "http://") {
			scheme = "http"
		}
		r.BaseURL = scheme + "://" + host
	}
	return r
}

func splitRemote(raw string) (host, path string) {
	raw = strings.TrimSpace(raw)
	// scp-like: git@host:owner/repo
	if !strings.Contains(raw, "://") && strings.Contains(raw, ":") && !strings.HasPrefix(raw, "/") {
		at := strings.IndexByte(raw, '@')
		colon := strings.IndexByte(raw, ':')
		if colon > at {
			return raw[at+1 : colon], raw[colon+1:]
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	return u.Hostname(), strings.TrimPrefix(u.Path, "/")
}

func splitPath(path string) (owner, repo string) {
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return "", ""
	}
	idx := strings.LastIndexByte(path, '/')
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

// ProviderToken describes a token discovered from a third-party CLI.
type ProviderToken struct {
	Provider Provider `json:"provider"`
	Token    string   `json:"token"`
	Source   string   `json:"source"`
}

// DetectProviderToken returns a credential the user has already configured for
// a given provider. Today it shells out to `gh` and `glab`; both CLIs print
// the active token on stdout when asked. Returns (nil, nil) if no token is
// found — callers treat that as "user has to type it themselves".
func DetectProviderToken(provider string) (*ProviderToken, error) {
	switch Provider(provider) {
	case ProviderGitHub:
		if tok, err := runForToken("gh", "auth", "token"); err == nil && tok != "" {
			return &ProviderToken{Provider: ProviderGitHub, Token: tok, Source: "gh"}, nil
		}
		if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
			return &ProviderToken{Provider: ProviderGitHub, Token: tok, Source: "env:GITHUB_TOKEN"}, nil
		}
	case ProviderGitLab:
		if tok, err := runForToken("glab", "auth", "status", "--show-token"); err == nil {
			if t := extractGlabToken(tok); t != "" {
				return &ProviderToken{Provider: ProviderGitLab, Token: t, Source: "glab"}, nil
			}
		}
		if tok := strings.TrimSpace(os.Getenv("GITLAB_TOKEN")); tok != "" {
			return &ProviderToken{Provider: ProviderGitLab, Token: tok, Source: "env:GITLAB_TOKEN"}, nil
		}
	}
	return nil, nil
}

func runForToken(name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// glab prints lines like `✓ Token: glpat-xxxx`. Pick the first one we see.
func extractGlabToken(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(strings.ToLower(line), "token:")
		if idx < 0 {
			continue
		}
		tok := strings.TrimSpace(line[idx+len("token:"):])
		tok = strings.Trim(tok, "\"'")
		if tok != "" && tok != "<no token>" {
			return tok
		}
	}
	return ""
}

// IsRepo returns true when projectPath sits inside a git working tree. We
// reuse findGitDir so worktrees and submodules (which store a `.git` file
// instead of a directory) are recognised.
func IsRepo(projectPath string) bool {
	if projectPath == "" {
		return false
	}
	dir, err := findGitDir(projectPath)
	if err != nil {
		return false
	}
	return dir != ""
}

// CreateWorktree adds a fresh worktree at worktreePath for repoPath, checked
// out on a new branch off the current HEAD. Fails if the path already exists
// (the runner is expected to pick a fresh location per agent).
func CreateWorktree(repoPath, worktreePath, branchName string) error {
	if repoPath == "" || worktreePath == "" || branchName == "" {
		return fmt.Errorf("repoPath, worktreePath and branchName are required")
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return fmt.Errorf("worktree path %q already exists", worktreePath)
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return fmt.Errorf("prepare worktree parent: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveWorktree detaches a worktree previously created by CreateWorktree.
// --force handles the common case where the agent left modified files in the
// tree; the branch itself is preserved so the user can still inspect it.
func RemoveWorktree(repoPath, worktreePath string) error {
	if repoPath == "" || worktreePath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If git no longer knows about the path (already pruned, missing dir),
		// fall back to a manual RemoveAll so we don't leak orphaned worktrees.
		_ = os.RemoveAll(worktreePath)
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// AgentDiff returns a unified diff of all changes made in dir (a worktree or
// repo root). It resolves the branch fork-point via merge-base against the
// default branch, then appends any staged/unstaged changes on top.
func AgentDiff(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var out strings.Builder

	if base := resolveBase(ctx, dir); base != "" {
		cmd := exec.CommandContext(ctx, "git", "diff", base+"..HEAD")
		cmd.Dir = dir
		sysexec.Hide(cmd)
		if b, err := cmd.Output(); err == nil {
			out.Write(b)
		}
	}

	for _, args := range [][]string{{"diff", "--staged"}, {"diff"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		if b, err := cmd.Output(); err == nil {
			out.Write(b)
		}
	}

	// Untracked files never appear in `git diff`, but they are listed in the
	// status pane, so without this the diff card stays blank when a change set
	// is entirely new files. `--no-index` renders them as additions; it exits
	// non-zero whenever the files differ, so the output is kept regardless.
	for _, path := range untrackedPaths(ctx, dir) {
		cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", path)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, _ := cmd.Output()
		out.Write(b)
	}

	return out.String(), nil
}

func untrackedPaths(ctx context.Context, dir string) []string {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--others", "--exclude-standard", "-z")
	cmd.Dir = dir
	sysexec.Hide(cmd)
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, p := range strings.Split(string(b), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// AgentChangedPaths returns the set of working-tree paths touched between the
// fork-point and HEAD, plus any staged/unstaged changes. Used to scope `git
// add` so the user keeps control over unrelated edits sitting in the worktree.
func AgentChangedPaths(dir string) ([]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	seen := make(map[string]struct{})
	var paths []string
	collect := func(args ...string) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, err := cmd.Output()
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(b), "\n") {
			p := strings.TrimSpace(line)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}

	if base := resolveBase(ctx, dir); base != "" {
		collect("diff", "--name-only", base+"..HEAD")
	}
	collect("diff", "--name-only", "--staged")
	collect("diff", "--name-only")
	collect("ls-files", "--others", "--exclude-standard")

	return paths, nil
}

// StagePaths runs `git add -- <paths>` in dir. Paths are scoped to the agent's
// changes — unrelated edits sitting in the worktree are left untouched.
func StagePaths(dir string, paths []string) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}
	if len(paths) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := append([]string{"add", "--"}, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// UnstagePaths runs `git restore --staged -- <paths>` in dir.
func UnstagePaths(dir string, paths []string) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}
	if len(paths) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := append([]string{"restore", "--staged", "--"}, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git restore --staged: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// AgentState summarises the working-tree and branch state used to drive the
// commit/push UI. All counts are best-effort: any sub-command failure yields
// a zero value rather than an error so the UI can still render.
type AgentState struct {
	Branch       string `json:"branch"`
	StagedCount  int    `json:"stagedCount"`
	AheadCount   int    `json:"aheadCount"`
	BehindCount  int    `json:"behindCount"`
	HasUpstream  bool   `json:"hasUpstream"`
	IsProtected  bool   `json:"isProtected"`
}

// CollectAgentState reads the branch + upstream divergence state for dir.
func CollectAgentState(dir string) (AgentState, error) {
	if dir == "" {
		return AgentState{}, fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var state AgentState

	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, err := cmd.Output()
		return strings.TrimSpace(string(b)), err
	}

	if branch, err := run("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		state.Branch = branch
		if branch == "main" || branch == "master" {
			state.IsProtected = true
		}
	}

	if staged, err := run("diff", "--name-only", "--cached"); err == nil && staged != "" {
		state.StagedCount = len(strings.Split(staged, "\n"))
	}

	if upstream, err := run("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil && upstream != "" {
		state.HasUpstream = true
		if counts, err := run("rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
			parts := strings.Fields(counts)
			if len(parts) == 2 {
				state.BehindCount, _ = strconv.Atoi(parts[0])
				state.AheadCount, _ = strconv.Atoi(parts[1])
			}
		}
	}

	return state, nil
}

// Commit creates a commit from the staged index. When amend is true the
// previous commit is rewritten; an empty message keeps the existing message
// (`--no-edit`). When amend is false a non-empty message is required.
func Commit(dir, message string, amend bool) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}
	if !amend && strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{"commit"}
	if amend {
		args = append(args, "--amend")
		if strings.TrimSpace(message) == "" {
			args = append(args, "--no-edit")
		} else {
			args = append(args, "-m", message)
		}
	} else {
		args = append(args, "-m", message)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Push pushes the current branch. When the branch has no upstream it sets one
// against `origin`. When force is true `--force-with-lease` is used (never raw
// `--force`).
func Push(dir string, force bool) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = dir
	sysexec.Hide(branchCmd)
	branchBytes, err := branchCmd.Output()
	if err != nil {
		return fmt.Errorf("resolve branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchBytes))
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("not on a named branch")
	}

	upstreamCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	upstreamCmd.Dir = dir
	sysexec.Hide(upstreamCmd)
	hasUpstream := upstreamCmd.Run() == nil

	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	if hasUpstream {
		// fall through, plain push
	} else {
		args = append(args, "-u", "origin", branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Sync runs `git pull --rebase --autostash` then pushes. Used by the "Commit
// & Sync" UX inspired by VS Code.
func Sync(dir string, force bool) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pull := exec.CommandContext(ctx, "git", "pull", "--rebase", "--autostash")
	pull.Dir = dir
	sysexec.Hide(pull)
	if out, err := pull.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull --rebase: %s", strings.TrimSpace(string(out)))
	}

	return Push(dir, force)
}

// FileChangeStatus describes a single path's working-tree state, scoped to the
// agent's set of changes.
type FileChangeStatus struct {
	Path    string `json:"path"`
	Status  string `json:"status"` // "M" (modified), "A" (added), "D" (deleted), "R" (renamed), "?" (untracked)
	Staged  bool   `json:"staged"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// AgentFileStatuses lists every path touched between the fork-point and the
// working tree, with each path's staged/unstaged state plus added/removed
// counts. When `scope` is non-empty, only paths in that set are returned —
// useful when several agents share a working tree and we must keep each
// agent's view limited to the files it actually touched.
func AgentFileStatuses(dir string, scope []string) ([]FileChangeStatus, error) {
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var scopeSet map[string]struct{}
	if len(scope) > 0 {
		scopeSet = make(map[string]struct{}, len(scope))
		for _, p := range scope {
			scopeSet[p] = struct{}{}
		}
	}
	inScope := func(path string) bool {
		if scopeSet == nil {
			return true
		}
		_, ok := scopeSet[path]
		return ok
	}

	statuses := make(map[string]*FileChangeStatus)
	order := []string{}
	upsert := func(path string) *FileChangeStatus {
		if !inScope(path) {
			return nil
		}
		if existing, ok := statuses[path]; ok {
			return existing
		}
		entry := &FileChangeStatus{Path: path}
		statuses[path] = entry
		order = append(order, path)
		return entry
	}

	parseNumstat := func(args ...string) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, err := cmd.Output()
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 3 {
				continue
			}
			added := parseNumstatCount(fields[0])
			removed := parseNumstatCount(fields[1])
			path := fields[len(fields)-1]
			entry := upsert(path)
			if entry == nil {
				continue
			}
			entry.Added += added
			entry.Removed += removed
		}
	}

	parsePorcelain := func() {
		cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z", "--untracked-files=all")
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, err := cmd.Output()
		if err != nil {
			return
		}
		records := strings.Split(string(b), "\x00")
		for _, rec := range records {
			if len(rec) < 3 {
				continue
			}
			xy := rec[:2]
			path := rec[3:]
			entry := upsert(path)
			if entry == nil {
				continue
			}
			x, y := rune(xy[0]), rune(xy[1])
			switch {
			case x == '?' && y == '?':
				entry.Status = "?"
				entry.Staged = false
			case x != ' ' && y == ' ':
				entry.Staged = true
				entry.Status = string(x)
			case x == ' ' && y != ' ':
				entry.Staged = false
				entry.Status = string(y)
			default:
				entry.Staged = false
				if entry.Status == "" {
					entry.Status = string(y)
				}
			}
		}
	}

	if base := resolveBase(ctx, dir); base != "" {
		parseNumstat("diff", "--numstat", base+"..HEAD")
	}
	parseNumstat("diff", "--numstat", "--staged")
	parseNumstat("diff", "--numstat")
	parsePorcelain()

	out := make([]FileChangeStatus, 0, len(order))
	for _, p := range order {
		out = append(out, *statuses[p])
	}
	return out, nil
}

func parseNumstatCount(field string) int {
	if field == "-" {
		return 0
	}
	n := 0
	for _, c := range field {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// BranchDiff returns the unified diff of branch vs the default branch,
// running from repoPath. Useful when the worktree no longer exists.
func BranchDiff(repoPath, branch string) (string, error) {
	if repoPath == "" || branch == "" {
		return "", fmt.Errorf("repoPath and branch are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		mb := exec.CommandContext(ctx, "git", "merge-base", base, branch)
		mb.Dir = repoPath
		sysexec.Hide(mb)
		mbOut, err := mb.Output()
		if err != nil {
			continue
		}
		mergeBase := strings.TrimSpace(string(mbOut))
		if mergeBase == "" {
			continue
		}
		cmd := exec.CommandContext(ctx, "git", "diff", mergeBase+".."+branch)
		cmd.Dir = repoPath
		sysexec.Hide(cmd)
		if b, err := cmd.Output(); err == nil {
			return string(b), nil
		}
	}

	return "", nil
}

// LogDiff extracts file paths from a rendered agent log, then produces a
// unified diff of those files between the commit at startedAtUnix and HEAD.
// This covers the "working directly on main" scenario where no branch or
// worktree is available.
func LogDiff(repoPath string, startedAtUnix int64, logContent string) (string, error) {
	if repoPath == "" || logContent == "" {
		return "", nil
	}

	files := RepoRelativePaths(repoPath, ExtractLogFilePaths(logContent))
	if len(files) == 0 {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var out strings.Builder

	revCmd := exec.CommandContext(ctx, "git", "rev-list", "-1",
		fmt.Sprintf("--before=@%d", startedAtUnix), "HEAD")
	revCmd.Dir = repoPath
	sysexec.Hide(revCmd)
	if baseOut, err := revCmd.Output(); err == nil {
		if base := strings.TrimSpace(string(baseOut)); base != "" {
			args := append([]string{"diff", base + "..HEAD", "--"}, files...)
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = repoPath
			sysexec.Hide(cmd)
			if b, err := cmd.Output(); err == nil {
				out.Write(b)
			}
		}
	}

	for _, prefix := range [][]string{{"diff", "--staged", "--"}, {"diff", "--"}} {
		args := append(prefix, files...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		sysexec.Hide(cmd)
		if b, err := cmd.Output(); err == nil {
			out.Write(b)
		}
	}

	return out.String(), nil
}

// RepoRelativePaths converts absolute paths under repoPath to repo-relative
// form. Paths already relative pass through unchanged. Paths outside repoPath
// are dropped — passing them to git would fatal the whole command ("outside
// repository"), so a single stray path (e.g. a plan file under ~/.claude)
// must not poison a diff or `git add` over the legitimate paths.
func RepoRelativePaths(repoPath string, paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	absRoot, err := filepath.Abs(repoPath)
	if err != nil {
		absRoot = repoPath
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		rel := p
		if filepath.IsAbs(p) {
			r, err := filepath.Rel(absRoot, p)
			if err != nil || strings.HasPrefix(r, "..") {
				continue
			}
			rel = r
		}
		rel = filepath.ToSlash(rel)
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	return out
}

// FilterStageable returns only the paths that can be meaningfully staged:
// files that exist on disk, or files that are already tracked by git (so a
// deletion can be staged). Paths that no longer exist and were never tracked
// are dropped — git add would fatal on them.
func FilterStageable(dir string, paths []string) []string {
	if len(paths) == 0 {
		return paths
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-files")
	cmd.Dir = dir
	sysexec.Hide(cmd)
	tracked := make(map[string]bool)
	if out, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if p := strings.TrimSpace(line); p != "" {
				tracked[p] = true
			}
		}
	}

	var result []string
	for _, p := range paths {
		if _, statErr := os.Stat(filepath.Join(dir, p)); statErr == nil || tracked[p] {
			result = append(result, p)
		}
	}
	return result
}

var logFilePathRe = regexp.MustCompile(`→ (?:\[#\S+\]\s+)?(?:Edit|Write|MultiEdit|NotebookEdit|Update) · (.+)$`)

func ExtractLogFilePaths(log string) []string {
	seen := make(map[string]struct{})
	var paths []string
	for _, line := range strings.Split(log, "\n") {
		m := logFilePathRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		fp := strings.TrimSpace(m[1])
		if _, ok := seen[fp]; !ok {
			seen[fp] = struct{}{}
			paths = append(paths, fp)
		}
	}
	return paths
}

// resolveBase finds the merge-base between HEAD and the most likely default
// branch. Returns an empty string when none can be determined.
func resolveBase(ctx context.Context, dir string) string {
	for _, ref := range []string{"origin/main", "origin/master", "origin/HEAD", "main", "master"} {
		cmd := exec.CommandContext(ctx, "git", "merge-base", ref, "HEAD")
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, err := cmd.Output()
		if err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s
			}
		}
	}
	return ""
}

// HasCommitsAhead reports whether the worktree's HEAD has at least one commit
// not on origin/HEAD. Used to gate PR creation: pushing a branch with no
// commits would create an empty PR.
func HasCommitsAhead(worktreePath string) (bool, error) {
	if worktreePath == "" {
		return false, fmt.Errorf("worktreePath is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "@{upstream}..HEAD")
	cmd.Dir = worktreePath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// No upstream yet (brand new branch) → any local commit counts.
		cmd2 := exec.CommandContext(ctx, "git", "rev-list", "--count", "HEAD")
		cmd2.Dir = worktreePath
		sysexec.Hide(cmd2)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return false, fmt.Errorf("rev-list: %s", strings.TrimSpace(string(out2)))
		}
		return strings.TrimSpace(string(out2)) != "0", nil
	}
	return strings.TrimSpace(string(out)) != "0", nil
}

// PushBranch publishes the current branch of worktreePath to origin, setting
// it as upstream. Required before `gh pr create` so GitHub knows the head ref.
func PushBranch(worktreePath, branchName string) error {
	if worktreePath == "" || branchName == "" {
		return fmt.Errorf("worktreePath and branchName are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName)
	cmd.Dir = worktreePath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// BranchNameForIssue produces a stable branch name from a Jira (or generic)
// ticket. Prefix is derived from issueType so bug/chore/feat are all routed
// to their conventional buckets. Summary is slugified and capped so the
// resulting name stays under git's practical 250-char ref limit.
func BranchNameForIssue(issueType, key, summary string) string {
	prefix := branchPrefixForIssueType(issueType)
	slug := slugify(summary)
	if slug == "" {
		return prefix + "/" + strings.ToLower(key)
	}
	const maxSlug = 60
	if len(slug) > maxSlug {
		slug = strings.TrimRight(slug[:maxSlug], "-")
	}
	return prefix + "/" + strings.ToLower(key) + "-" + slug
}

func branchPrefixForIssueType(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "bug", "défaut", "defect", "incident":
		return "fix"
	case "task", "tâche", "subtask", "sous-tâche", "chore":
		return "chore"
	case "spike", "research", "recherche":
		return "spike"
	case "epic":
		return "epic"
	default:
		return "feat"
	}
}

// Slug exposes the internal slugify helper so callers outside this package
// can reuse the same naming rules when generating branch names.
func Slug(s string) string { return slugify(s) }

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevDash := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == 'à', r == 'â', r == 'ä':
			b.WriteRune('a')
			prevDash = false
		case r == 'é', r == 'è', r == 'ê', r == 'ë':
			b.WriteRune('e')
			prevDash = false
		case r == 'î', r == 'ï':
			b.WriteRune('i')
			prevDash = false
		case r == 'ô', r == 'ö':
			b.WriteRune('o')
			prevDash = false
		case r == 'ù', r == 'û', r == 'ü':
			b.WriteRune('u')
			prevDash = false
		case r == 'ç':
			b.WriteRune('c')
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := b.String()
	return strings.Trim(out, "-")
}

func providerFor(host string) Provider {
	host = strings.ToLower(host)
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return ProviderGitHub
	case host == "gitlab.com" || strings.HasSuffix(host, "gitlab.com") || strings.Contains(host, "gitlab"):
		return ProviderGitLab
	case host == "bitbucket.org" || strings.HasSuffix(host, "bitbucket.org") || strings.Contains(host, "bitbucket"):
		return ProviderBitbucket
	default:
		return ProviderUnknown
	}
}

// BranchInfo describes a local branch and whether it's currently checked out
// in some worktree of the repo. WorktreePath is non-empty when the branch is
// busy elsewhere — callers should refuse to checkout it from the main repo.
type BranchInfo struct {
	Name         string `json:"name"`
	IsCurrent    bool   `json:"isCurrent"`
	WorktreePath string `json:"worktreePath,omitempty"`
	Upstream     string `json:"upstream,omitempty"`
}

// ListBranches returns the local branches of the repo containing dir. It
// merges `git branch` output with `git worktree list --porcelain` so the
// caller knows which branches are checked out in side worktrees.
func ListBranches(dir string) ([]BranchInfo, error) {
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Map branch → worktree path. The "main" worktree is intentionally NOT
	// tagged as a blocker — we want users to be able to checkout *from* the
	// main repo, even though Git technically has "main" checked out there.
	worktreeMap := map[string]string{}
	wtCmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	wtCmd.Dir = dir
	sysexec.Hide(wtCmd)
	if wtOut, err := wtCmd.Output(); err == nil {
		var curPath, curBranch string
		isMainWorktree := true
		flush := func() {
			if curBranch != "" && curPath != "" && !isMainWorktree {
				worktreeMap[curBranch] = curPath
			}
			curPath, curBranch = "", ""
			isMainWorktree = false
		}
		mainSeen := false
		for _, raw := range strings.Split(string(wtOut), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				flush()
				continue
			}
			switch {
			case strings.HasPrefix(line, "worktree "):
				if !mainSeen {
					isMainWorktree = true
					mainSeen = true
				}
				curPath = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch refs/heads/"):
				curBranch = strings.TrimPrefix(line, "branch refs/heads/")
			}
		}
		flush()
	}

	brCmd := exec.CommandContext(ctx, "git", "branch", "--list", "--format=%(refname:short)|%(HEAD)|%(upstream:short)")
	brCmd.Dir = dir
	sysexec.Hide(brCmd)
	brOut, err := brCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch list: %w", err)
	}

	out := []BranchInfo{}
	for _, raw := range strings.Split(string(brOut), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		isCurrent := parts[1] == "*"
		upstream := ""
		if len(parts) >= 3 {
			upstream = parts[2]
		}
		info := BranchInfo{Name: name, IsCurrent: isCurrent, Upstream: upstream}
		if wt, ok := worktreeMap[name]; ok && !isCurrent {
			info.WorktreePath = wt
		}
		out = append(out, info)
	}
	return out, nil
}

// CheckoutBranch switches dir to the named branch. Git itself refuses when
// branch is checked out in another worktree — we surface that as a clean
// error so the UI can render a useful message.
func CheckoutBranch(dir, branch string) error {
	if dir == "" || branch == "" {
		return fmt.Errorf("dir and branch are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "switch", branch)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git switch: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
