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

// Promotion must move every agent change (including a shell-driven edit the log
// never sees, a rename, and a new file) into the worktree, then rewind the
// project root to the spawn snapshot, while a pre-existing uncommitted change the
// agent never touched survives in the root.
func TestSnapshotPromotionMovesAgentChanges(t *testing.T) {
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
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init")
	write("package.json", "{\"deps\":{}}\n")
	write("lib.go", "package lib\n\nfunc A() {}\n")
	write("keep.go", "package keep\n")
	run("add", ".")
	run("commit", "-m", "init")

	// A pre-existing uncommitted change the agent will not touch, captured into
	// the spawn snapshot.
	write("keep.go", "package keep\n\nfunc Pre() {}\n")

	baseTree, err := SnapshotTree(repo)
	if err != nil {
		t.Fatalf("SnapshotTree: %v", err)
	}

	// Agent's work: a shell-style edit to a tracked file (no log event), a rename
	// with a follow-up edit, and a brand new untracked file.
	write("package.json", "{\"deps\":{\"bun\":\"1\"}}\n")
	if err := os.Rename(filepath.Join(repo, "lib.go"), filepath.Join(repo, "lib2.go")); err != nil {
		t.Fatal(err)
	}
	write("lib2.go", "package lib\n\nfunc A() {}\nfunc B() {}\n")
	write("newfile.go", "package newpkg\n")

	scope, err := SnapshotScopedPaths(repo, baseTree)
	if err != nil {
		t.Fatalf("SnapshotScopedPaths: %v", err)
	}
	inScope := map[string]bool{}
	for _, p := range scope {
		inScope[p] = true
	}
	for _, want := range []string{"package.json", "lib.go", "lib2.go", "newfile.go"} {
		if !inScope[want] {
			t.Fatalf("scope missing %q, got %v", want, scope)
		}
	}
	if inScope["keep.go"] {
		t.Fatalf("pre-existing change keep.go must not be in scope, got %v", scope)
	}

	worktree := t.TempDir()
	run("worktree", "add", "-b", "promoted", worktree, "HEAD")
	if err := ApplyScopedChanges(repo, worktree, "HEAD", scope); err != nil {
		t.Fatalf("ApplyScopedChanges: %v", err)
	}

	// Worktree holds the moved work.
	if b, _ := os.ReadFile(filepath.Join(worktree, "package.json")); string(b) != "{\"deps\":{\"bun\":\"1\"}}\n" {
		t.Fatalf("worktree package.json = %q, want shell edit", b)
	}
	if b, _ := os.ReadFile(filepath.Join(worktree, "lib2.go")); string(b) != "package lib\n\nfunc A() {}\nfunc B() {}\n" {
		t.Fatalf("worktree lib2.go = %q, want renamed+edited content", b)
	}
	if _, err := os.Stat(filepath.Join(worktree, "newfile.go")); err != nil {
		t.Fatalf("worktree missing newfile.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "lib.go")); !os.IsNotExist(err) {
		t.Fatalf("worktree should not keep lib.go, stat err = %v", err)
	}

	// Rewind the root to the spawn snapshot.
	if err := RestorePathsToTree(repo, baseTree, scope); err != nil {
		t.Fatalf("RestorePathsToTree: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "package.json")); string(b) != "{\"deps\":{}}\n" {
		t.Fatalf("root package.json = %q, want reverted", b)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "lib.go")); string(b) != "package lib\n\nfunc A() {}\n" {
		t.Fatalf("root lib.go = %q, want restored", b)
	}
	if _, err := os.Stat(filepath.Join(repo, "lib2.go")); !os.IsNotExist(err) {
		t.Fatalf("root should not keep lib2.go, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "newfile.go")); !os.IsNotExist(err) {
		t.Fatalf("root should not keep newfile.go, stat err = %v", err)
	}
	// The pre-existing change the agent never touched survives.
	if b, _ := os.ReadFile(filepath.Join(repo, "keep.go")); string(b) != "package keep\n\nfunc Pre() {}\n" {
		t.Fatalf("root keep.go = %q, want pre-existing change preserved", b)
	}
}
