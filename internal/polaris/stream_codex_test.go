package polaris

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func collectCodexEvents(jsonLines ...string) ([]StreamEvent, streamTurnStats, []string) {
	input := strings.Join(jsonLines, "\n") + "\n"
	reader := strings.NewReader(input)
	var sink bytes.Buffer
	var events []StreamEvent
	var threads []string
	stats := streamCodexJSON(reader, &sink, func(evt StreamEvent) {
		events = append(events, evt)
	}, func(threadID string) {
		threads = append(threads, threadID)
	}, func(_ int, _ usageParts, _ float64) {})
	return events, stats, threads
}

func TestStreamCodexJSON_TextAndThread(t *testing.T) {
	events, _, threads := collectCodexEvents(
		mustJSON(map[string]any{"type": "thread.started", "thread_id": "thread-1"}),
		mustJSON(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": "hello"}}),
	)
	if len(threads) != 1 || threads[0] != "thread-1" {
		t.Fatalf("threads = %v, want [thread-1]", threads)
	}
	found := false
	for _, e := range events {
		if e.Type == "text" && e.Content == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected text event, got %+v", events)
	}
}

func TestStreamCodexJSON_CommandLifecycle(t *testing.T) {
	events, _, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "item.started", "item": map[string]any{"id": "item_1", "type": "command_execution", "command": "echo test"}}),
		mustJSON(map[string]any{"type": "item.completed", "item": map[string]any{"id": "item_1", "type": "command_execution", "command": "echo test", "aggregated_output": "test\n", "exit_code": 0, "status": "completed"}}),
	)
	var call, result *StreamEvent
	for i, e := range events {
		switch {
		case e.Type == "tool_call":
			call = &events[i]
		case e.Type == "tool_result":
			result = &events[i]
		}
	}
	if call == nil {
		t.Fatalf("expected tool_call, got %+v", events)
	}
	if call.Name != "Bash" || call.ID != "item_1" {
		t.Fatalf("unexpected tool_call: %+v", *call)
	}
	if call.Content == "" {
		t.Fatal("expected tool_call summary")
	}
	if result == nil {
		t.Fatalf("expected tool_result, got %+v", events)
	}
	if result.Error {
		t.Fatal("expected successful command_execution")
	}
	if result.Content != "test" {
		t.Fatalf("result.Content = %q, want %q", result.Content, "test")
	}
}

func TestStreamCodexJSON_ResponseItemViewImageLifecycle(t *testing.T) {
	events, stats, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "response_item", "payload": map[string]any{
			"type":      "function_call",
			"name":      "view_image",
			"arguments": `{"path":"/tmp/screenshot.png","detail":"high"}`,
			"call_id":   "call_img",
		}}),
		mustJSON(map[string]any{"type": "response_item", "payload": map[string]any{
			"type":    "function_call_output",
			"call_id": "call_img",
			"output": []any{map[string]any{
				"type":      "input_image",
				"image_url": "data:image/png;base64,very-large",
				"detail":    "high",
			}},
		}}),
	)
	if stats.ToolsUsed != 1 {
		t.Fatalf("ToolsUsed = %d, want 1", stats.ToolsUsed)
	}
	var call, result *StreamEvent
	for i, e := range events {
		switch e.Type {
		case "tool_call":
			call = &events[i]
		case "tool_result":
			result = &events[i]
		}
	}
	if call == nil {
		t.Fatalf("expected tool_call, got %+v", events)
	}
	if call.Name != "Read" || call.ID != "call_img" {
		t.Fatalf("unexpected tool_call: %+v", *call)
	}
	if call.Content != "/tmp/screenshot.png" {
		t.Fatalf("call.Content = %q, want image path summary", call.Content)
	}
	if result == nil {
		t.Fatalf("expected tool_result, got %+v", events)
	}
	if result.Content != "image loaded" {
		t.Fatalf("result.Content = %q, want compact image summary", result.Content)
	}
}

func TestStreamCodexJSON_FunctionOutputSingleTextBlock(t *testing.T) {
	events, _, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "response_item", "payload": map[string]any{
			"type":    "function_call_output",
			"call_id": "call_text",
			"output": map[string]any{
				"type": "output_text",
				"text": "tool output\n",
			},
		}}),
	)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1: %+v", len(events), events)
	}
	if events[0].Type != "tool_result" || events[0].ID != "call_text" || events[0].Content != "tool output" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestRecentCodexImageToolEventsFromSessionFile(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("CODEX_HOME")
	t.Setenv("CODEX_HOME", dir)
	t.Cleanup(func() {
		_ = os.Setenv("CODEX_HOME", oldHome)
	})
	sessions := filepath.Join(dir, "sessions", "2026", "06", "25")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Second)
	lineTime := started.Add(500 * time.Millisecond).Format(time.RFC3339Nano)
	path := filepath.Join(sessions, "rollout-test.jsonl")
	content := strings.Join([]string{
		mustJSON(map[string]any{"timestamp": lineTime, "type": "session_meta", "payload": map[string]any{
			"id": "thread-good",
		}}),
		mustJSON(map[string]any{"timestamp": lineTime, "type": "response_item", "payload": map[string]any{
			"type":      "function_call",
			"name":      "view_image",
			"arguments": `{"path":"/tmp/image.png","detail":"high"}`,
			"call_id":   "call_img",
		}}),
		mustJSON(map[string]any{"timestamp": lineTime, "type": "response_item", "payload": map[string]any{
			"type":    "function_call_output",
			"call_id": "call_img",
			"output": []any{map[string]any{
				"type":      "input_image",
				"image_url": "data:image/png;base64,too-large-to-log",
			}},
		}}),
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	events := recentCodexImageToolEvents(started, "thread-good")
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2: %+v", len(events), events)
	}
	if events[0].Type != "tool_call" || events[0].Name != "Read" || events[0].Content != "/tmp/image.png" {
		t.Fatalf("unexpected call event: %+v", events[0])
	}
	if events[1].Type != "tool_result" || events[1].Content != "image loaded" {
		t.Fatalf("unexpected result event: %+v", events[1])
	}
}

func TestRecentCodexImageToolEventsIgnoresOtherSessions(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("CODEX_HOME")
	t.Setenv("CODEX_HOME", dir)
	t.Cleanup(func() {
		_ = os.Setenv("CODEX_HOME", oldHome)
	})
	sessions := filepath.Join(dir, "sessions", "2026", "06", "25")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Second)
	lineTime := started.Add(500 * time.Millisecond).Format(time.RFC3339Nano)
	content := strings.Join([]string{
		mustJSON(map[string]any{"timestamp": lineTime, "type": "session_meta", "payload": map[string]any{
			"id": "thread-other",
		}}),
		mustJSON(map[string]any{"timestamp": lineTime, "type": "response_item", "payload": map[string]any{
			"type":      "function_call",
			"name":      "view_image",
			"arguments": `{"path":"/tmp/other.png"}`,
			"call_id":   "call_other",
		}}),
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sessions, "rollout-other.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	events := recentCodexImageToolEvents(started, "thread-current")
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0: %+v", len(events), events)
	}
}

func TestMergeEventsIntoLogLinesUsesTimestamps(t *testing.T) {
	lines := []string{
		marshalEvent(StreamEvent{Type: "user_message", Ts: "08:00:00", Content: "look"}),
		marshalEvent(StreamEvent{Type: "text", Ts: "08:00:02", Content: "after"}),
	}
	events := []StreamEvent{
		{Type: "tool_call", Ts: "08:00:01", ID: "call_img", Name: "Read", Content: "/tmp/image.png"},
		{Type: "tool_result", Ts: "08:00:01", ID: "call_img", Content: "image loaded"},
	}

	merged := mergeEventsIntoLogLines(lines, events)
	if len(merged) != 4 {
		t.Fatalf("len(merged) = %d, want 4", len(merged))
	}
	var got []StreamEvent
	for _, line := range merged {
		var evt StreamEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatal(err)
		}
		got = append(got, evt)
	}
	if got[1].Type != "tool_call" || got[2].Type != "tool_result" || got[3].Type != "text" {
		t.Fatalf("unexpected order: %+v", got)
	}
}

func TestStreamCodexJSON_TurnUsage(t *testing.T) {
	_, stats, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "turn.completed", "usage": map[string]any{
			"input_tokens":            100,
			"output_tokens":           20,
			"reasoning_output_tokens": 5,
			"cached_input_tokens":     12,
			"other_field_we_ignore":   99,
		}}),
	)
	if stats.Tokens != 137 {
		t.Fatalf("stats.Tokens = %d, want 137", stats.Tokens)
	}
	if stats.Parts.Input != 100 || stats.Parts.Output != 25 || stats.Parts.CacheRead != 12 {
		t.Fatalf("unexpected parts: %+v", stats.Parts)
	}
	if !stats.Succeeded {
		t.Fatal("expected succeeded turn")
	}
}

func TestStreamCodexJSON_FailedCommandDoesNotFailTurn(t *testing.T) {
	events, stats, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "item.completed", "item": map[string]any{"id": "item_1", "type": "command_execution", "aggregated_output": "failed\n", "exit_code": 1}}),
		mustJSON(map[string]any{"type": "turn.completed", "status": "completed"}),
	)
	var result *StreamEvent
	for i, e := range events {
		if e.Type == "tool_result" {
			result = &events[i]
			break
		}
	}
	if result == nil || !result.Error {
		t.Fatalf("expected failed tool_result, got %+v", events)
	}
	if !stats.Succeeded {
		t.Fatal("expected turn success to follow Codex turn status, not command exit code")
	}
}

func TestStreamCodexJSON_TurnStatusFailure(t *testing.T) {
	events, stats, _ := collectCodexEvents(
		mustJSON(map[string]any{"type": "turn.completed", "status": "failed"}),
	)
	if stats.Succeeded {
		t.Fatal("expected failed turn")
	}
	for _, e := range events {
		if e.Type == "turn_end" && e.Status != "error" {
			t.Fatalf("turn_end status = %q, want error", e.Status)
		}
	}
}

func TestStreamCodexStderrParsesTimestampedLine(t *testing.T) {
	line := "2026-06-24T07:30:43.601330Z ERROR codex_models_manager::manager: failed to refresh available models: timeout waiting for child process to exit Salut."
	evt, ok := parseCodexStderrLine(line)
	if !ok {
		t.Fatal("expected timestamped stderr line to parse")
	}
	if evt.Type != "system" {
		t.Fatalf("evt.Type = %q, want system", evt.Type)
	}
	if evt.Ts != "07:30:43" {
		t.Fatalf("evt.Ts = %q, want 07:30:43", evt.Ts)
	}
	if strings.HasPrefix(strings.ToLower(evt.Content), "error ") {
		t.Fatalf("evt.Content still looks like an error: %q", evt.Content)
	}
	if !strings.Contains(evt.Content, "failed to refresh available models") {
		t.Fatalf("evt.Content = %q, want refresh failure text", evt.Content)
	}
}
