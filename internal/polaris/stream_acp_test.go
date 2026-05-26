package polaris

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestACPSession(t *testing.T) (*acpSession, *Service, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc := NewService(store).WithRunner(NewRunner(filepath.Join(dir, "logs"), filepath.Join(dir, "wt")))

	proj, err := store.UpsertProject(Project{Name: "test", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := store.UpsertAgent(Agent{ProjectID: proj.ID, Kind: "opencode", Summary: "test"})
	if err != nil {
		t.Fatal(err)
	}

	sess := &acpSession{
		svc:     svc,
		agentID: agent.ID,
		label:   "test",
		tools:   make(map[string]*acpToolState),
	}
	return sess, svc, agent.ID
}

func readLogEvents(t *testing.T, svc *Service, agentID string) []StreamEvent {
	t.Helper()
	evts, err := svc.ReadLogEvents(agentID)
	if err != nil {
		t.Fatal(err)
	}
	return evts
}

func TestAcpEmit_ToolCall(t *testing.T) {
	sess, svc, agentID := newTestACPSession(t)

	sess.mu.Lock()
	sess.tools["tool_01"] = &acpToolState{
		name:   "Edit",
		detail: "foo.go",
		input:  map[string]any{"file_path": "foo.go"},
	}
	sess.mu.Unlock()

	sess.emitToolCall("tool_01")

	evts := readLogEvents(t, svc, agentID)
	var call *StreamEvent
	for i, e := range evts {
		if e.Type == "tool_call" {
			call = &evts[i]
		}
	}
	if call == nil {
		t.Fatalf("expected tool_call event, got %+v", evts)
	}
	if call.Name != "Edit" {
		t.Errorf("expected name=Edit, got %q", call.Name)
	}
	if call.ID != "tool_01" {
		t.Errorf("expected id=tool_01, got %q", call.ID)
	}
}

func TestAcpEmit_ToolCall_Deduplicated(t *testing.T) {
	sess, svc, agentID := newTestACPSession(t)

	sess.mu.Lock()
	sess.tools["tool_02"] = &acpToolState{name: "Read", detail: "bar.go"}
	sess.mu.Unlock()

	sess.emitToolCall("tool_02")
	sess.emitToolCall("tool_02")

	evts := readLogEvents(t, svc, agentID)
	count := 0
	for _, e := range evts {
		if e.Type == "tool_call" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 tool_call (deduplicated), got %d", count)
	}
}

func TestAcpEmit_ToolResult_Success(t *testing.T) {
	sess, svc, agentID := newTestACPSession(t)

	sess.emitToolResult("tool_03", "file content", "rendered diff", false)

	evts := readLogEvents(t, svc, agentID)
	var res *StreamEvent
	for i, e := range evts {
		if e.Type == "tool_result" {
			res = &evts[i]
		}
	}
	if res == nil {
		t.Fatalf("expected tool_result, got %+v", evts)
	}
	if res.Error {
		t.Error("expected non-error result")
	}
	if res.RenderedContent != "rendered diff" {
		t.Errorf("expected rendered_content='rendered diff', got %q", res.RenderedContent)
	}
}

func TestAcpEmit_ToolResult_Error(t *testing.T) {
	sess, svc, agentID := newTestACPSession(t)

	sess.emitToolResult("tool_04", "permission denied", "", true)

	evts := readLogEvents(t, svc, agentID)
	var res *StreamEvent
	for i, e := range evts {
		if e.Type == "tool_result" {
			res = &evts[i]
		}
	}
	if res == nil {
		t.Fatalf("expected tool_result, got %+v", evts)
	}
	if !res.Error {
		t.Error("expected error=true")
	}
}

func TestAcpEmit_MsgLines(t *testing.T) {
	sess, svc, agentID := newTestACPSession(t)

	sess.emitMsgLines("line one\nline two\n\nline three")

	evts := readLogEvents(t, svc, agentID)
	var texts []string
	for _, e := range evts {
		if e.Type == "text" {
			texts = append(texts, e.Content)
		}
	}
	if len(texts) != 3 {
		t.Fatalf("expected 3 text events (blank line skipped), got %d: %v", len(texts), texts)
	}
	if !strings.Contains(texts[0], "line one") {
		t.Errorf("unexpected first line: %q", texts[0])
	}
}
