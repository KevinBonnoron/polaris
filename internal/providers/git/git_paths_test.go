package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRelativePaths(t *testing.T) {
	root := "/home/kevin/Workspaces/polaris"
	got := RepoRelativePaths(root, []string{
		"/home/kevin/Workspaces/polaris/app.go", // in-repo absolute -> relativized
		"internal/git.go",                       // already relative -> kept
		"/home/kevin/.claude/plans/x.md",        // out-of-repo -> dropped
		"/home/kevin/Workspaces/polaris/app.go", // duplicate -> deduped
		"",                                      // empty -> skipped
	})
	want := []string{"app.go", "internal/git.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("RepoRelativePaths = %v, want %v", got, want)
	}
}

// A stray path outside the repo (e.g. a plan written under ~/.claude) used to
// fatal the whole `git diff` command, leaving the files tab blank. The diff
// must survive and still cover the in-repo file.
func TestLogDiffIgnoresOutOfRepoPaths(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	target := filepath.Join(repo, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	if err := os.WriteFile(target, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	log := strings.Join([]string{
		"→ Write · " + target,
		"→ Write · " + filepath.Join(os.TempDir(), "out-of-repo-plan.md"),
	}, "\n")

	diff, err := LogDiff(repo, 0, log)
	if err != nil {
		t.Fatalf("LogDiff: %v", err)
	}
	if !strings.Contains(diff, "func main()") {
		t.Fatalf("expected in-repo change in diff, got:\n%s", diff)
	}
}
