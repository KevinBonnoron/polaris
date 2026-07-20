package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

// BranchNameForIssue produces a stable branch name from a ticket (or generic)
// ticket. Prefix is derived from issueType so bug/chore/feat are all routed
// to their conventional buckets. Summary is slugified and capped so the
// resulting name stays under git's practical 250-char ref limit.
func BranchNameForIssue(issueType, key, summary string) string {
	prefix := branchPrefixForIssueType(issueType)
	keySlug := slugify(key)
	slug := slugify(summary)
	const maxSlug = 60
	if len(slug) > maxSlug {
		slug = strings.TrimRight(slug[:maxSlug], "-")
	}
	switch {
	case keySlug == "" && slug == "":
		return prefix + "/change"
	case slug == "":
		return prefix + "/" + keySlug
	case keySlug == "":
		return prefix + "/" + slug
	default:
		return prefix + "/" + keySlug + "-" + slug
	}
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
	case host == "gitlab.com" || strings.HasSuffix(host, ".gitlab.com") || strings.HasPrefix(host, "gitlab."):
		return ProviderGitLab
	case host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org") || strings.HasPrefix(host, "bitbucket."):
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
	IsProtected  bool   `json:"isProtected,omitempty"`
	IsRemote     bool   `json:"isRemote,omitempty"`
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
	localNames := map[string]bool{}
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
		protected := name == "main" || name == "master"
		info := BranchInfo{Name: name, IsCurrent: isCurrent, IsProtected: protected, Upstream: upstream}
		if wt, ok := worktreeMap[name]; ok && !isCurrent {
			info.WorktreePath = wt
		}
		out = append(out, info)
		localNames[name] = true
	}

	// Append remote-only branches (not yet checked out locally).
	remCmd := exec.CommandContext(ctx, "git", "for-each-ref", "--format=%(refname:short)", "refs/remotes/")
	remCmd.Dir = dir
	sysexec.Hide(remCmd)
	if remOut, err := remCmd.Output(); err == nil {
		for _, raw := range strings.Split(string(remOut), "\n") {
			ref := strings.TrimSpace(raw)
			if ref == "" {
				continue
			}
			// ref looks like "origin/feature/xyz"; split off the remote name.
			slash := strings.IndexByte(ref, '/')
			if slash < 0 {
				continue
			}
			name := ref[slash+1:]
			if name == "HEAD" || localNames[name] {
				continue
			}
			out = append(out, BranchInfo{
				Name:        name,
				IsRemote:    true,
				IsProtected: name == "main" || name == "master",
				Upstream:    ref,
			})
		}
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
	cmd := exec.CommandContext(ctx, "git", "switch", "--", branch)
	cmd.Dir = dir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git switch: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// CreateBranch creates a new branch at startPoint (commit, tag, or branch
// name). When startPoint is empty, HEAD is used.
func CreateBranch(ctx context.Context, repoPath, branchName, startPoint string) error {
	if repoPath == "" || branchName == "" {
		return fmt.Errorf("repoPath and branchName are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"branch", "--", branchName}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteBranch deletes branchName from repoPath. When force is true,
// -D is passed so the branch is removed even when not fully merged.
func DeleteBranch(ctx context.Context, repoPath, branchName string, force bool) error {
	if repoPath == "" || branchName == "" {
		return fmt.Errorf("repoPath and branchName are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.CommandContext(ctx, "git", "branch", flag, "--", branchName)
	cmd.Dir = repoPath
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch %s: %s", flag, strings.TrimSpace(string(out)))
	}
	return nil
}
