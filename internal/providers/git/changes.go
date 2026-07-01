package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

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

// ProjectDiff returns a unified diff of uncommitted changes only (staged,
// unstaged, and untracked). Unlike AgentDiff it does not include commits.
func ProjectDiff(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var out strings.Builder

	for _, args := range [][]string{{"diff", "--staged"}, {"diff"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		if b, err := cmd.Output(); err == nil {
			out.Write(b)
		}
	}

	for _, path := range untrackedPaths(ctx, dir) {
		cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", path)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		b, _ := cmd.Output()
		out.Write(b)
	}

	return out.String(), nil
}

// ProjectFileStatuses lists paths with uncommitted changes only (staged,
// unstaged, untracked). Unlike AgentFileStatuses it does not include commits.
func ProjectFileStatuses(dir string) ([]FileChangeStatus, error) {
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses := make(map[string]*FileChangeStatus)
	order := []string{}
	upsert := func(path string) *FileChangeStatus {
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
		for i := 0; i < len(records); i++ {
			rec := records[i]
			if len(rec) < 3 {
				continue
			}
			xy := rec[:2]
			path := rec[3:]
			x, y := rune(xy[0]), rune(xy[1])
			// Rename/copy records emit two NUL fields: `XY <dest>\0<orig>\0`.
			// Consume the origin field so it isn't parsed as a bogus entry.
			if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
				i++
			}
			entry := upsert(path)
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

	parseNumstat("diff", "--numstat", "--staged")
	parseNumstat("diff", "--numstat")
	parsePorcelain()

	out := make([]FileChangeStatus, 0, len(order))
	for _, p := range order {
		out = append(out, *statuses[p])
	}
	return out, nil
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

// DiscardPaths restores tracked paths to HEAD (git restore) and removes
// untracked paths (git clean -f). untracked is the subset of paths whose
// FileChangeStatus.Status is "?"; the rest are treated as tracked.
func DiscardPaths(dir string, tracked, untracked []string) error {
	if dir == "" {
		return fmt.Errorf("dir is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(tracked) > 0 {
		args := append([]string{"restore", "--"}, tracked...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git restore: %s", strings.TrimSpace(string(out)))
		}
	}

	if len(untracked) > 0 {
		args := append([]string{"clean", "-f", "--"}, untracked...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		sysexec.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clean: %s", strings.TrimSpace(string(out)))
		}
	}

	return nil
}

// AgentState summarises the working-tree and branch state used to drive the
// commit/push UI. All counts are best-effort: any sub-command failure yields
// a zero value rather than an error so the UI can still render.
type AgentState struct {
	Branch      string `json:"branch"`
	StagedCount int    `json:"stagedCount"`
	AheadCount  int    `json:"aheadCount"`
	BehindCount int    `json:"behindCount"`
	HasUpstream bool   `json:"hasUpstream"`
	IsProtected bool   `json:"isProtected"`
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
		entry := statuses[p]
		if entry.Status == "" {
			continue
		}
		out = append(out, *entry)
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
