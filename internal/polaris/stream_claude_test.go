package polaris

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func collectClaudeEvents(jsonLines ...string) ([]StreamEvent, streamTurnStats) {
	input := strings.Join(jsonLines, "\n") + "\n"
	reader := strings.NewReader(input)
	var sink bytes.Buffer
	var events []StreamEvent
	var asked []string
	var toks int
	stats := streamClaudeJSON(reader, &sink, func(evt StreamEvent) {
		events = append(events, evt)
	}, func(toolUseID string, _ map[string]any) {
		asked = append(asked, toolUseID)
	}, func(t int, _ usageParts, _ float64) {
		toks = t
	}, nil)
	_ = asked
	_ = toks
	return events, stats
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestStreamClaudeJSON_TextBlock(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "hello world"}},
		},
	})
	events, _ := collectClaudeEvents(line)
	found := false
	for _, e := range events {
		if e.Type == "text" && e.Content == "hello world" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected text event with 'hello world', got %+v", events)
	}
}

func TestStreamClaudeJSON_ToolCall(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":  "tool_use",
				"id":    "toolu_01",
				"name":  "Edit",
				"input": map[string]any{"file_path": "foo.go", "old_string": "a", "new_string": "b"},
			}},
		},
	})
	events, _ := collectClaudeEvents(line)
	var call *StreamEvent
	for i, e := range events {
		if e.Type == "tool_call" {
			call = &events[i]
		}
	}
	if call == nil {
		t.Fatalf("expected tool_call event, got %+v", events)
	}
	if call.Name != "Edit" {
		t.Errorf("expected name=Edit, got %q", call.Name)
	}
	if call.ID != "toolu_01" {
		t.Errorf("expected id=toolu_01, got %q", call.ID)
	}
	if call.Content == "" {
		t.Error("expected non-empty detail in Content")
	}
}

func TestStreamClaudeJSON_ToolResult(t *testing.T) {
	editCall := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":  "tool_use",
				"id":    "toolu_02",
				"name":  "Edit",
				"input": map[string]any{"file_path": "bar.go", "old_string": "x", "new_string": "y"},
			}},
		},
	})
	toolResult := mustJSON(map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": "toolu_02",
				"content":     "OK",
			}},
		},
	})
	events, _ := collectClaudeEvents(editCall, toolResult)
	var result *StreamEvent
	for i, e := range events {
		if e.Type == "tool_result" && e.ID == "toolu_02" {
			result = &events[i]
		}
	}
	if result == nil {
		t.Fatalf("expected tool_result event, got %+v", events)
	}
	if result.Error {
		t.Error("expected non-error result")
	}
}

func TestStreamClaudeJSON_AskUserQuestion(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":  "tool_use",
				"id":    "toolu_ask",
				"name":  "AskUserQuestion",
				"input": map[string]any{"question": "Proceed?", "options": []any{"Yes", "No"}},
			}},
		},
	})
	reader := strings.NewReader(line + "\n")
	var sink bytes.Buffer
	var askIDs []string
	streamClaudeJSON(reader, &sink, func(_ StreamEvent) {}, func(id string, _ map[string]any) {
		askIDs = append(askIDs, id)
	}, nil, nil)
	if len(askIDs) != 1 || askIDs[0] != "toolu_ask" {
		t.Errorf("expected ask callback with toolu_ask, got %v", askIDs)
	}
}

func TestStreamClaudeJSON_TokenAccumulation(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "hi"}},
			"usage":   map[string]any{"input_tokens": float64(10), "output_tokens": float64(5)},
		},
	})
	var lastToks int
	reader := strings.NewReader(line + "\n")
	var sink bytes.Buffer
	streamClaudeJSON(reader, &sink, func(_ StreamEvent) {}, nil, func(t int, _ usageParts, _ float64) {
		lastToks = t
	}, nil)
	if lastToks == 0 {
		t.Error("expected non-zero token count")
	}
}

func TestStreamClaudeJSON_ResultEvent(t *testing.T) {
	line := mustJSON(map[string]any{
		"type":   "result",
		"status": "success",
		"usage":  map[string]any{"input_tokens": float64(100), "output_tokens": float64(50)},
	})
	events, stats := collectClaudeEvents(line)
	var end *StreamEvent
	for i, e := range events {
		if e.Type == "turn_end" {
			end = &events[i]
		}
	}
	if end == nil {
		t.Fatalf("expected turn_end event, got %+v", events)
	}
	if end.Status != "success" {
		t.Errorf("expected status=success, got %q", end.Status)
	}
	if !stats.Succeeded {
		t.Error("expected stats.Succeeded=true")
	}
}

func TestStreamClaudeJSON_UnknownEvent(t *testing.T) {
	line := mustJSON(map[string]any{"type": "debug", "payload": "x"})
	events, _ := collectClaudeEvents(line)
	for _, e := range events {
		if e.Type == "tool_call" || e.Type == "text" {
			t.Errorf("unexpected event from unknown type: %+v", e)
		}
	}
}
