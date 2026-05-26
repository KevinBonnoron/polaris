package polaris

import (
	"bytes"
	"strings"
	"testing"
)

func collectGeminiEvents(jsonLines ...string) ([]StreamEvent, streamTurnStats) {
	input := strings.Join(jsonLines, "\n") + "\n"
	reader := strings.NewReader(input)
	var sink bytes.Buffer
	var events []StreamEvent
	stats := streamGeminiJSON(reader, &sink, func(evt StreamEvent) {
		events = append(events, evt)
	}, nil, nil, nil, "")
	return events, stats
}

func TestMapGeminiToolName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"read_file", "Read"},
		{"write_file", "Write"},
		{"replace", "Edit"},
		{"run_shell_command", "Bash"},
		{"custom_tool", "custom_tool"},
	}
	for _, c := range cases {
		got := mapGeminiToolName(c.in)
		if got != c.want {
			t.Errorf("mapGeminiToolName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStreamGeminiJSON_ToolCall(t *testing.T) {
	line := mustJSON(map[string]any{
		"type":       "tool_use",
		"tool_id":    "gemini_01",
		"tool_name":  "replace",
		"parameters": map[string]any{"file_path": "foo.go"},
	})
	events, _ := collectGeminiEvents(line)
	var call *StreamEvent
	for i, e := range events {
		if e.Type == "tool_call" {
			call = &events[i]
		}
	}
	if call == nil {
		t.Fatalf("expected tool_call, got %+v", events)
	}
	if call.Name != "Edit" {
		t.Errorf("expected name=Edit, got %q", call.Name)
	}
	if call.ID != "gemini_01" {
		t.Errorf("expected id=gemini_01, got %q", call.ID)
	}
}

func TestStreamGeminiJSON_ToolResult(t *testing.T) {
	call := mustJSON(map[string]any{
		"type":       "tool_use",
		"tool_id":    "gemini_02",
		"tool_name":  "write_file",
		"parameters": map[string]any{"file_path": "out.txt"},
	})
	result := mustJSON(map[string]any{
		"type":    "tool_result",
		"tool_id": "gemini_02",
		"status":  "success",
		"output":  "written",
	})
	events, _ := collectGeminiEvents(call, result)
	var res *StreamEvent
	for i, e := range events {
		if e.Type == "tool_result" && e.ID == "gemini_02" {
			res = &events[i]
		}
	}
	if res == nil {
		t.Fatalf("expected tool_result, got %+v", events)
	}
	if res.Error {
		t.Error("expected non-error result")
	}
}

func TestStreamGeminiJSON_MessageText(t *testing.T) {
	line := mustJSON(map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": "All done.",
	})
	events, _ := collectGeminiEvents(line)
	found := false
	for _, e := range events {
		if e.Type == "text" && e.Content == "All done." {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected text event 'All done.', got %+v", events)
	}
}
