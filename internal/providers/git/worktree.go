package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

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

// HeadCommit returns the SHA of the current HEAD commit in dir.
func HeadCommit(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateWorktreeAt creates a worktree on a new branch starting from baseRef.
// When baseRef is empty or "HEAD", it behaves like CreateWorktree.
func CreateWorktreeAt(repoPath, worktreePath, branchName, baseRef string) error {
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
	args := []string{"worktree", "add", "-b", branchName, worktreePath}
	if baseRef != "" && baseRef != "HEAD" {
		args = append(args, baseRef)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ApplyScopedChanges transfers working-tree changes from repoPath that touch
// any of the given repo-relative paths into worktreePath. Tracked changes are
// applied via git-apply; untracked files in scope are copied directly.
func ApplyScopedChanges(repoPath, worktreePath, baseRef string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if strings.TrimSpace(baseRef) == "" {
		baseRef = "HEAD"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inScope := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		inScope[p] = struct{}{}
	}

	// Scoped diff of tracked changes (staged + unstaged against baseRef).
	diffArgs := append([]string{"diff", baseRef, "--"}, paths...)
	diffCmd := exec.CommandContext(ctx, "git", diffArgs...)
	diffCmd.Dir = repoPath
	sysexec.Hide(diffCmd)
	var diffStderr bytes.Buffer
	diffCmd.Stderr = &diffStderr
	patch, err := diffCmd.Output()
	if err != nil {
		msg := strings.TrimSpace(diffStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git diff: %s", msg)
	}

	if len(patch) > 0 {
		apply := exec.CommandContext(ctx, "git", "apply", "--")
		apply.Dir = worktreePath
		apply.Stdin = bytes.NewReader(patch)
		sysexec.Hide(apply)
		if out, err := apply.CombinedOutput(); err != nil {
			return fmt.Errorf("git apply: %s", strings.TrimSpace(string(out)))
		}
	}

	// Copy untracked files that are in scope.
	for _, rel := range untrackedPaths(ctx, repoPath) {
		if _, ok := inScope[rel]; !ok {
			continue
		}
		dst := filepath.Join(worktreePath, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", rel, err)
		}
		if err := copyFile(filepath.Join(repoPath, rel), dst); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
	}

	return nil
}

// writeWorkingTree captures the full working tree of repoPath (tracked and
// untracked, honouring .gitignore) as a git tree object and returns its SHA. It
// stages everything into a throwaway index via GIT_INDEX_FILE, so the real index
// is never touched. The resulting tree is a complete snapshot, which lets callers
// diff two snapshots to recover every change regardless of how it was made.
func writeWorkingTree(ctx context.Context, repoPath string) (string, error) {
	tmpIndex, err := os.CreateTemp("", "polaris-snapshot-index-*")
	if err != nil {
		return "", err
	}
	tmpIndexPath := tmpIndex.Name()
	tmpIndex.Close()
	// git refuses to read a zero-byte index file, so start from a clean slate.
	if err := os.Remove(tmpIndexPath); err != nil {
		return "", err
	}
	defer os.Remove(tmpIndexPath)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndexPath)
	gitCmd := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		cmd.Env = env
		sysexec.Hide(cmd)
		return cmd
	}

	// Seed the throwaway index from the repo's real index so `git add -A` reuses
	// git's stat cache and only re-hashes files that actually changed. An empty
	// index forces a cold hash of the entire working tree, which pegs the CPU for
	// seconds on a large repo and stalls agent spawn. Best effort: any failure
	// falls back to the cold path, which still produces the correct tree.
	seeded := seedIndexFromRepo(ctx, repoPath, tmpIndexPath)

	if err := gitCmd("add", "-A").Run(); err != nil {
		if !seeded {
			return "", fmt.Errorf("git add -A: %w", err)
		}
		// The seeded index turned out unusable (e.g. a corrupt real index git could
		// still read); degrade to the cold empty-index path instead of failing the
		// whole snapshot.
		_ = os.Remove(tmpIndexPath)
		if err := gitCmd("add", "-A").Run(); err != nil {
			return "", fmt.Errorf("git add -A: %w", err)
		}
	}
	out, err := gitCmd("write-tree").Output()
	if err != nil {
		return "", fmt.Errorf("git write-tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// envWithout returns env with any assignment of key removed, so a child process
// isn't influenced by an ambient value of it.
func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// seedIndexFromRepo copies the repo's real index to destIndex so a snapshot can
// build on git's stat cache instead of hashing every file from cold. Best
// effort: it returns true only when destIndex holds a usable copy; callers fall
// back to an empty index otherwise.
func seedIndexFromRepo(ctx context.Context, repoPath, destIndex string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-path", "index")
	cmd.Dir = repoPath
	// `git rev-parse --git-path index` honours an ambient GIT_INDEX_FILE, which
	// would point this probe at the wrong staging area; strip it so we always copy
	// the repo's real index.
	cmd.Env = envWithout(os.Environ(), "GIT_INDEX_FILE")
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	realIndex := strings.TrimSpace(string(out))
	if realIndex == "" {
		return false
	}
	if !filepath.IsAbs(realIndex) {
		realIndex = filepath.Join(repoPath, realIndex)
	}
	src, err := os.Open(realIndex)
	if err != nil {
		return false
	}
	defer src.Close()
	dst, err := os.Create(destIndex)
	if err != nil {
		return false
	}
	// A truncated or zero-byte copy leaves an index git can't read, so drop it and
	// report failure so the caller uses the cold empty-index path.
	n, err := io.Copy(dst, src)
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(destIndex)
		return false
	}
	if err := dst.Close(); err != nil || n == 0 {
		_ = os.Remove(destIndex)
		return false
	}
	return true
}
