package polaris

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type syncWriteCloser struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriteCloser) Close() error { return nil }

func (w *syncWriteCloser) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func newDecisionTestSession(t *testing.T, agentID string) (*acpSession, *syncWriteCloser, *Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc := NewService(store).WithRunner(NewRunner(filepath.Join(dir, "logs"), filepath.Join(dir, "wt")))

	decided, _ := store.ListAgentDecisions(agentID)
	if decided == nil {
		decided = map[string]string{}
	}
	out := &syncWriteCloser{}
	a := &acpSession{
		svc:       svc,
		agentID:   agentID,
		stdin:     out,
		pending:   map[int]chan acpMessage{},
		perms:     map[string]chan string{},
		permReqID: map[string]int{},
		decided:   decided,
	}
	return a, out, store
}

func permParams(toolCallID string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"toolCall": map[string]any{"toolCallId": toolCallID, "title": "bash"},
		"options": []map[string]any{
			{"optionId": "opt_allow", "name": "Approve"},
			{"optionId": "opt_reject", "name": "Reject"},
		},
	})
	return b
}

// A permission/plan already decided in a prior turn must be answered from the
// recorded decision on the next session/request_permission (e.g. after a
// session/load resume) — never surfaced to the user again.
func TestHandleServerRequestReplaysDecision(t *testing.T) {
	a, out, store := newDecisionTestSession(t, "agent-1")
	if err := store.RecordAgentDecision("agent-1", "tc_42", "Approve"); err != nil {
		t.Fatal(err)
	}
	a.decided["tc_42"] = "Approve"

	a.handleServerRequest(7, "session/request_permission", permParams("tc_42"))

	var reply struct {
		ID     int `json:"id"`
		Result struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &reply); err != nil {
		t.Fatalf("no reply written: %v (%q)", err, out.String())
	}
	if reply.ID != 7 || reply.Result.Outcome.Outcome != "selected" || reply.Result.Outcome.OptionID != "opt_allow" {
		t.Fatalf("expected replayed approval, got %+v", reply)
	}
	// The prompt must not have been surfaced: no pending permission registered.
	a.mu.Lock()
	n := len(a.perms)
	a.mu.Unlock()
	if n != 0 {
		t.Fatalf("decided request should not register a pending prompt, got %d", n)
	}
}

// Answering a fresh permission must persist the decision keyed by the stable
// tool-call id so a later session can replay it.
func TestHandleServerRequestRecordsDecision(t *testing.T) {
	a, _, store := newDecisionTestSession(t, "agent-2")
	if _, err := store.UpsertAgent(Agent{ID: "agent-2", Kind: "opencode"}); err != nil {
		t.Fatal(err)
	}

	go a.handleServerRequest(9, "session/request_permission", permParams("tc_99"))

	// Wait for the prompt to be surfaced, then answer it.
	var ch chan string
	for i := 0; i < 200; i++ {
		a.mu.Lock()
		ch = a.perms["perm-9"]
		a.mu.Unlock()
		if ch != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ch == nil {
		t.Fatal("permission was never surfaced")
	}
	ch <- "Reject"

	for i := 0; i < 200; i++ {
		if v, _ := store.ListAgentDecisions("agent-2"); v["tc_99"] == "Reject" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("decision was not persisted")
}
