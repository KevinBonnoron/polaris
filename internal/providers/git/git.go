// Package git inspects a local working tree to extract metadata the UI can
// use to pre-fill integration configuration (provider, owner, repo, ...).
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

var logFilePathRe = regexp.MustCompile(`→ (?:\[#\S+\]\s+)?(?:Edit|Write|MultiEdit|NotebookEdit|Update) · (.+)$`)

var fileToolNames = map[string]bool{
	"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true, "Update": true,
}

func ExtractLogFilePaths(log string) []string {
	seen := make(map[string]struct{})
	var paths []string
	add := func(fp string) {
		if fp = strings.TrimSpace(fp); fp != "" {
			if _, ok := seen[fp]; !ok {
				seen[fp] = struct{}{}
				paths = append(paths, fp)
			}
		}
	}
	for _, line := range strings.Split(log, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			var evt struct {
				Type    string `json:"type"`
				Name    string `json:"name"`
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(trimmed), &evt) == nil && evt.Type == "tool_call" && fileToolNames[evt.Name] {
				add(evt.Content)
			}
			continue
		}
		if m := logFilePathRe.FindStringSubmatch(line); m != nil {
			add(m[1])
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
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mKHFJGArsuABCDEF]|\x1b\([AB]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

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
		return fmt.Errorf("git push: %s", strings.TrimSpace(stripANSI(string(out))))
	}
	return nil
}
