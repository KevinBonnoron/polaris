package polaris

import "testing"

func TestEscapeLeadingSlash(t *testing.T) {
	zwsp := string(rune(0x200b))
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "hello world", "hello world"},
		{"leading slash escaped", "/foo do this", zwsp + "/foo do this"},
		{"leading whitespace then slash", "  /bar", zwsp + "  /bar"},
		{"slash mid-text untouched", "see src/main.go", "see src/main.go"},
		{"email untouched", "ping me@host.com", "ping me@host.com"},
		{"empty untouched", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := escapeLeadingSlash(c.in); got != c.want {
				t.Fatalf("escapeLeadingSlash(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
