package polaris

import (
	"encoding/json"
	"testing"
)

// vibe-acp encodes rawInput/rawOutput as JSON strings while opencode sends them
// as objects; both must decode to the same rendered detail/output.
func TestACPDecodeInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // acpToolDetail of the decoded input
	}{
		{"opencode object", `{"filePath":"/tmp/a.go"}`, "/tmp/a.go"},
		{"vibe string-encoded object", `"{\"path\":\"/tmp/a.go\"}"`, "/tmp/a.go"},
		{"empty", ``, ""},
		{"null", `null`, ""},
		{"garbage string", `"not json"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := acpToolDetail(acpDecodeInput(json.RawMessage(c.raw))); got != c.want {
				t.Fatalf("detail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestACPRawOutputText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"opencode object", `{"output":"done"}`, "done"},
		{"vibe plain string", `"file contents here"`, "file contents here"},
		{"empty", ``, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := acpRawOutputText(json.RawMessage(c.raw)); got != c.want {
				t.Fatalf("output = %q, want %q", got, c.want)
			}
		})
	}
}

func TestACPVibeResultText(t *testing.T) {
	// rawOutput is a JSON string whose value is the result model serialised as JSON.
	jsonStr := func(model string) string {
		b, _ := json.Marshal(model)
		return string(b)
	}
	cases := []struct {
		name  string
		raw   string
		recap string
		want  string
	}{
		{"read_file content", jsonStr(`{"path":"/a","content":"file body","lines_read":2}`), "Read 2 lines from a", "file body"},
		{"grep matches", jsonStr(`{"matches":"a.go:1: hit","match_count":1}`), "1 match", "a.go:1: hit"},
		{"bash stdout", jsonStr(`{"command":"ls","stdout":"out","stderr":"","returncode":0}`), "Ran ls", "out"},
		{"bash stderr only", jsonStr(`{"command":"x","stdout":"","stderr":"boom","returncode":1}`), "Ran x", "boom"},
		{"no text field falls back to recap", jsonStr(`{"count":3}`), "Did 3 things", "Did 3 things"},
		{"plain error string", jsonStr(`Error: file not found`), "", "Error: file not found"},
		{"empty falls back to recap", ``, "recap", "recap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := acpVibeResultText(json.RawMessage(c.raw), c.recap); got != c.want {
				t.Fatalf("acpVibeResultText = %q, want %q", got, c.want)
			}
		})
	}
}

func TestACPDiffLines(t *testing.T) {
	got := acpDiffLines("old1\nold2", "new1")
	want := "- old1\n- old2\n+ new1\n"
	if got != want {
		t.Fatalf("acpDiffLines = %q, want %q", got, want)
	}
	// pure insertion: no removed lines
	if got := acpDiffLines("", "added"); got != "+ added\n" {
		t.Fatalf("insertion = %q, want %q", got, "+ added\n")
	}
	// pure deletion: no added lines
	if got := acpDiffLines("gone", ""); got != "- gone\n" {
		t.Fatalf("deletion = %q, want %q", got, "- gone\n")
	}
}

func TestACPToolNameMapsVibeIDs(t *testing.T) {
	cases := map[string]string{
		"read_file":      "Read",
		"search_replace": "Edit",
		"write_file":     "Write",
		"read":           "Read", // opencode short id still works
		"skill":          "Skill",
	}
	for in, want := range cases {
		if got := acpToolName(in); got != want {
			t.Fatalf("acpToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountChangedLogPaths(t *testing.T) {
	dir := "/repo"
	// git reports repo-relative; the agent touched a real file plus a plan file
	// written outside the repo (absent from the changed set → must not count).
	changed := []string{"src/app.go", "README.md"}
	logPaths := []string{
		"/repo/src/app.go",   // real change, absolute
		"src/app.go",         // same file, relative form (dedup not required; counts once per occurrence)
		"/tmp/plan-xyz.md",   // scratch file outside repo
		"/repo/untouched.go", // in log but not changed by git
	}
	// app.go (abs) + app.go (rel) both match → 2; plan + untouched excluded.
	if got := countChangedLogPaths(dir, logPaths, changed); got != 2 {
		t.Fatalf("countChangedLogPaths = %d, want 2", got)
	}
	if got := countChangedLogPaths(dir, []string{"/tmp/plan.md"}, changed); got != 0 {
		t.Fatalf("plan-only count = %d, want 0", got)
	}
}

func TestUserToolResultIDs(t *testing.T) {
	var evt map[string]any
	_ = json.Unmarshal([]byte(`{
		"type":"user",
		"message":{"content":[
			{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},
			{"type":"text","text":"ignored"},
			{"type":"tool_result","tool_use_id":"toolu_2","is_error":true,"content":"boom"}
		]}
	}`), &evt)
	ids := userToolResultIDs(evt)
	if len(ids) != 2 || ids[0] != "toolu_1" || ids[1] != "toolu_2" {
		t.Fatalf("userToolResultIDs = %v, want [toolu_1 toolu_2]", ids)
	}
	// non-user / empty events yield nothing
	if got := userToolResultIDs(map[string]any{"type": "assistant"}); got != nil {
		t.Fatalf("non-user event = %v, want nil", got)
	}
}
