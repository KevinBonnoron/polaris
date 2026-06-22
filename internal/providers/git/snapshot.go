package git

import (
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

// SnapshotTree records the current working tree of repoPath as a tree object and
// returns its SHA. Stored at spawn time, it is the baseline the agent's changes
// are later measured against.
func SnapshotTree(repoPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return writeWorkingTree(ctx, repoPath)
}

// SnapshotScopedPaths returns every repo-relative path that differs between the
// spawn snapshot baseTree and the current working tree. This is the precise set
// of files the agent changed, including shell-driven edits, additions, deletions
// and renames that the log never records.
func SnapshotScopedPaths(repoPath, baseTree string) ([]string, error) {
	if strings.TrimSpace(baseTree) == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	currentTree, err := writeWorkingTree(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	// --no-renames keeps both sides of a rename in scope (the source as a deletion
	// and the destination as an addition) so the move and the root rewind both act
	// on the source path too.
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--no-renames", "-z", baseTree, currentTree)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s..%s: %w", baseTree, currentTree, err)
	}

	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// SnapshotDiff returns a unified diff of everything the agent changed since the
// spawn snapshot baseTree, with rename detection. Because both sides are full
// working-tree snapshots, the diff covers tracked edits, new untracked files and
// shell-driven changes alike.
func SnapshotDiff(repoPath, baseTree string) (string, error) {
	if strings.TrimSpace(baseTree) == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	currentTree, err := writeWorkingTree(ctx, repoPath)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "-M", baseTree, currentTree)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s..%s: %w", baseTree, currentTree, err)
	}
	return string(out), nil
}

// RestorePathsToTree reverts the given repo-relative paths in repoPath back to
// their content in treeish, removing paths that the tree does not contain. It is
// how promotion moves the agent's work out of the project root: after the changes
// are safely in the worktree, the root is rewound to the spawn snapshot so only
// the agent's edits are taken, leaving any pre-existing changes in place.
func RestorePathsToTree(repoPath, treeish string, paths []string) error {
	if len(paths) == 0 || strings.TrimSpace(treeish) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var restore, remove []string
	for _, p := range paths {
		check := exec.CommandContext(ctx, "git", "cat-file", "-e", treeish+":"+p)
		check.Dir = repoPath
		sysexec.Hide(check)
		if check.Run() == nil {
			restore = append(restore, p)
		} else {
			remove = append(remove, p)
		}
	}

	if len(restore) > 0 {
		args := append([]string{"checkout", treeish, "--"}, restore...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		sysexec.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout: %s", strings.TrimSpace(string(out)))
		}
	}

	for _, p := range remove {
		reset := exec.CommandContext(ctx, "git", "reset", "-q", "--", p)
		reset.Dir = repoPath
		sysexec.Hide(reset)
		_ = reset.Run()
		if err := os.Remove(filepath.Join(repoPath, p)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		return os.Symlink(target, dst)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
