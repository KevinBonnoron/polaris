package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveInProject(t *testing.T) {
	base := t.TempDir()
	sibling := base + "-secrets"

	cases := []struct {
		name    string
		rel     string
		wantErr bool
	}{
		{"plain file", "src/main.go", false},
		{"root itself", ".", false},
		{"parent escape", "../etc/passwd", true},
		{"deep parent escape", "a/../../etc/passwd", true},
		{"absolute path is confined, not escaped", "/etc/passwd", false},
		{"sibling prefix bypass", filepath.Join("..", filepath.Base(sibling), "x"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, full, err := resolveInProject(base, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected confinement error, got full=%q", full)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSplitCommandTemplate(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`code --goto "$FILE:$LINE:$COL"`, []string{"code", "--goto", "$FILE:$LINE:$COL"}},
		{`a 'b c' d`, []string{"a", "b c", "d"}},
		{`  spaced   out  `, []string{"spaced", "out"}},
		{``, nil},
	}
	for _, tc := range cases {
		if got := splitCommandTemplate(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommandTemplate(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}
