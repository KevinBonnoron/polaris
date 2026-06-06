package polaris

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeProjectDirEncoding(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	base := filepath.Join(home, ".claude", "projects")

	cases := []struct {
		path string
		want string
	}{
		{"/home/user/Workspaces/polaris", "-home-user-Workspaces-polaris"},
		// Dots collapse to '-' too: a worktree under ~/.config must match Claude's
		// own "--config" encoding, not "-.config".
		{"/home/user/.config/polaris/wt", "-home-user--config-polaris-wt"},
		{"/home/user/my_project", "-home-user-my-project"},
		{"/home/user/my project", "-home-user-my-project"},
		{"/home/user/.../test", "-home-user-----test"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := claudeProjectDir(tc.path); got != filepath.Join(base, tc.want) {
			t.Errorf("claudeProjectDir(%q) = %q, want %q", tc.path, got, filepath.Join(base, tc.want))
		}
	}
}
