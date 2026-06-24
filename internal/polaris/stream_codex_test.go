package polaris

import (
	"bytes"
	"strings"
	"testing"
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
