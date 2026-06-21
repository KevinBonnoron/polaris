package polaris

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClaudeSessionTurnAccounting verifies the per-turn stat folding the
// persistent session does on each result event: the stream-json `usage` block is
// per-turn (added directly), while `total_cost_usd` and the tool counter are
// cumulative for the session and must be persisted as deltas, so two turns over
// one long-lived process don't double-count cost or tools.
func TestClaudeSessionTurnAccounting(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store).WithRunner(NewRunner(filepath.Join(dir, "logs"), filepath.Join(dir, "wt")))
	proj, err := store.UpsertProject(Project{Name: "t", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := store.UpsertAgent(Agent{ProjectID: proj.ID, Kind: "claude-code", Status: "working", SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}

	c := &claudeSession{
		svc:       svc,
		runner:    svc.runner,
		agentID:   agent.ID,
		running:   true,
		baseParts: agent.Tokens,
		baseCost:  agent.CostUSD,
	}

	// Turn 1: usage is this turn's (output 5), cumulative cost 0.02, 1 tool so far.
	c.onTurnEnd(streamTurnStats{Parts: usageParts{Output: 5}, CostUSD: 0.02, ToolsUsed: 1, Succeeded: true})
	a1, err := store.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := a1.Tokens.Total(); got != 5 {
		t.Errorf("after turn1 tokens = %d, want 5", got)
	}
	if a1.CostUSD != 0.02 {
		t.Errorf("after turn1 cost = %v, want 0.02", a1.CostUSD)
	}
	if a1.ToolsUsed != 1 {
		t.Errorf("after turn1 tools = %d, want 1", a1.ToolsUsed)
	}

	// Turn 2: usage output 7 (per-turn), cumulative cost 0.05, cumulative 3 tools.
	c.onTurnEnd(streamTurnStats{Parts: usageParts{Output: 7}, CostUSD: 0.05, ToolsUsed: 3, Succeeded: true})
	a2, err := store.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := a2.Tokens.Total(); got != 12 {
		t.Errorf("after turn2 tokens = %d, want 12 (5+7)", got)
	}
	// Cost is cumulative upstream, so the delta (0.05-0.02=0.03) is added: total 0.05.
	if a2.CostUSD < 0.0499 || a2.CostUSD > 0.0501 {
		t.Errorf("after turn2 cost = %v, want ~0.05 (no double-count)", a2.CostUSD)
	}
	// Tools delta 3-1=2 added to the prior 1 → 3 (not 1+3=4).
	if a2.ToolsUsed != 3 {
		t.Errorf("after turn2 tools = %d, want 3 (no double-count)", a2.ToolsUsed)
	}

	// The turn drained no queued message, so the agent lands completed.
	if a2.Status != "completed" {
		t.Errorf("status = %q, want completed", a2.Status)
	}
}

// TestSetPendingReplaces verifies claude-code keeps a single replaceable queued
// message: a second send supersedes the first instead of stacking.
func TestSetPendingReplaces(t *testing.T) {
	r := NewRunner(t.TempDir(), t.TempDir())
	r.setPending("a", "first")
	r.setPending("a", "second")

	got, ok := r.popPending("a")
	if !ok || got != "second" {
		t.Fatalf("popPending = (%q, %v), want (\"second\", true)", got, ok)
	}
	if _, ok := r.popPending("a"); ok {
		t.Error("queue should hold only one message; second pop returned a value")
	}
}

// TestStopAndRetractLast verifies Escape's retract: an unanswered last message is
// removed from the log and returned, while a message the agent already responded
// to is left untouched.
func TestStopAndRetractLast(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewService(store).WithRunner(NewRunner(logs, filepath.Join(dir, "wt")))

	writeLog := func(id string, evs ...StreamEvent) {
		var b strings.Builder
		for _, e := range evs {
			data, _ := json.Marshal(e)
			b.Write(data)
			b.WriteByte('\n')
		}
		if err := os.WriteFile(filepath.Join(logs, id+".log"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	readLog := func(id string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(logs, id+".log"))
		if err != nil {
			t.Fatalf("readLog(%q): %v", id, err)
		}
		return string(data)
	}
	// The retract only applies to claude-code agents; an empty sessionId keeps it
	// from touching any real ~/.claude transcript.
	newAgent := func() string {
		a, err := store.UpsertAgent(Agent{Kind: "claude-code", Status: "working"})
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}

	// Unanswered: the last user_message has no response after it.
	idA := newAgent()
	writeLog(idA,
		StreamEvent{Type: "user_message", Content: "first"},
		StreamEvent{Type: "text", Content: "reply"},
		StreamEvent{Type: "user_message", Content: "oops typo"},
	)
	got, err := svc.StopAndRetractLast(idA)
	if err != nil {
		t.Fatal(err)
	}
	if got != "oops typo" {
		t.Errorf("retracted = %q, want %q", got, "oops typo")
	}
	if log := readLog(idA); strings.Contains(log, "oops typo") {
		t.Error("retracted message is still in the log")
	} else if !strings.Contains(log, "first") || !strings.Contains(log, "reply") {
		t.Error("earlier history was wrongly dropped")
	}

	// Answered: a response follows the last user_message, so nothing is retracted.
	idB := newAgent()
	writeLog(idB,
		StreamEvent{Type: "user_message", Content: "do it"},
		StreamEvent{Type: "tool_call", Name: "Bash"},
	)
	got, err = svc.StopAndRetractLast(idB)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("retracted = %q, want \"\" (already answered)", got)
	}
	if !strings.Contains(readLog(idB), "do it") {
		t.Error("answered message should remain in the log")
	}
}

// TestForgetClaudeMessage verifies the claude transcript rewind removes the
// matched user message and everything after it, leaving earlier turns intact.
func TestForgetClaudeMessage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := "/tmp/some-project"
	dir := claudeProjectDir(workDir)
	if dir == "" {
		t.Fatal("claudeProjectDir returned empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sid.jsonl")
	content := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"keep me"},"uuid":"u1","parentUuid":null}`,
		`{"type":"assistant","message":{"role":"assistant","content":"ok"},"uuid":"a1","parentUuid":"u1"}`,
		`{"type":"user","message":{"role":"user","content":"retract me"},"uuid":"u2","parentUuid":"a1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":"partial"},"uuid":"a2","parentUuid":"u2"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	forgetClaudeMessage(workDir, "sid", "retract me")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "retract me") || strings.Contains(s, "partial") {
		t.Errorf("transcript still has the retracted turn:\n%s", s)
	}
	if !strings.Contains(s, "keep me") || !strings.Contains(s, `"content":"ok"`) {
		t.Errorf("earlier turns were wrongly dropped:\n%s", s)
	}

	// A no-match leaves the file untouched.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	forgetClaudeMessage(workDir, "sid", "nonexistent message")
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("no-match should leave the transcript unchanged")
	}
}
