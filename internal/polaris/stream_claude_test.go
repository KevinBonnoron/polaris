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

func TestStreamClaudeJSON_ReadImagePathSummary(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type": "tool_use",
				"id":   "toolu_img",
				"name": "Read",
				"input": map[string]any{
					"image_path": "/tmp/polaris_paste_1782371913317374478.png",
					"detail":     "original",
				},
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
	if call.Name != "Read" {
		t.Fatalf("call.Name = %q, want Read", call.Name)
	}
	want := "/tmp/polaris_paste_1782371913317374478.png"
	if call.Content != want {
		t.Fatalf("call.Content = %q, want %q", call.Content, want)
	}
}

func TestStreamClaudeJSON_NamedImageReadPathSummary(t *testing.T) {
	line := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":  "tool_use",
				"id":    "toolu_img",
				"name":  "ReadImage",
				"input": map[string]any{"path": "/tmp/polaris_paste.png", "detail": "original"},
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
	if call.Name != "Read" {
		t.Fatalf("call.Name = %q, want Read", call.Name)
	}
	want := "/tmp/polaris_paste.png"
	if call.Content != want {
		t.Fatalf("call.Content = %q, want %q", call.Content, want)
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

func TestStreamClaudeJSON_AutoDismissSuppressed(t *testing.T) {
	// The real --print sequence: claude calls AskUserQuestion, then auto-dismisses
	// it itself (is_error tool_result for the same id) and emits filler text before
	// the result. The tool_call must survive; the auto result and filler must not.
	askCall := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":  "tool_use",
				"id":    "toolu_ask",
				"name":  "AskUserQuestion",
				"input": map[string]any{"questions": []any{map[string]any{"header": "Indentation", "question": "Tabs or spaces?", "options": []any{map[string]any{"label": "Spaces"}, map[string]any{"label": "Tabs"}}}}},
			}},
		},
	})
	autoResult := mustJSON(map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": "toolu_ask",
				"content":     "Answer questions?",
				"is_error":    true,
			}},
		},
	})
	filler := mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "It looks like the question was dismissed. Let me know."}},
		},
	})
	result := mustJSON(map[string]any{"type": "result", "status": "success"})

	events, _ := collectClaudeEvents(askCall, autoResult, filler, result)
	var sawCall, sawAutoResult, sawFiller bool
	for _, e := range events {
		switch {
		case e.Type == "tool_call" && e.Name == "AskUserQuestion":
			sawCall = true
		case e.Type == "tool_result" && e.ID == "toolu_ask":
			sawAutoResult = true
		case e.Type == "text" && strings.Contains(e.Content, "dismissed"):
			sawFiller = true
		}
	}
	if !sawCall {
		t.Error("expected the AskUserQuestion tool_call to survive")
	}
	if sawAutoResult {
		t.Error("expected the auto is_error tool_result to be suppressed")
	}
	if sawFiller {
		t.Error("expected the post-question filler text to be suppressed")
	}
}

func TestStreamClaudeJSON_SuppressionResetsNextTurn(t *testing.T) {
	askCall := mustJSON(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "toolu_ask", "name": "AskUserQuestion", "input": map[string]any{"questions": []any{map[string]any{"question": "Proceed?"}}}}}},
	})
	autoResult := mustJSON(map[string]any{
		"type":    "user",
		"message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_ask", "content": "Answer questions?", "is_error": true}}},
	})
	turn1End := mustJSON(map[string]any{"type": "result", "status": "success"})
	nextTurnText := mustJSON(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "real answer after the question"}}},
	})

	events, _ := collectClaudeEvents(askCall, autoResult, turn1End, nextTurnText)
	found := false
	for _, e := range events {
		if e.Type == "text" && strings.Contains(e.Content, "real answer") {
			found = true
		}
	}
	if !found {
		t.Error("expected text in the turn after the question to NOT be suppressed")
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

// streamEvent wraps an inner Anthropic API event the way --include-partial-messages does.
func streamEvent(inner map[string]any) string {
	return mustJSON(map[string]any{"type": "stream_event", "event": inner})
}

func thinkingStartEvent() string {
	return streamEvent(map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "thinking", "thinking": ""},
	})
}

func thinkingDeltaEvent(text string) string {
	return streamEvent(map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "thinking_delta", "thinking": text},
	})
}

func thinkingStopEvent() string {
	return streamEvent(map[string]any{"type": "content_block_stop", "index": 0})
}

func assistantWithThinkingAndText(thinking, text string) string {
	return mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "thinking", "thinking": thinking},
				map[string]any{"type": "text", "text": text},
			},
		},
	})
}

// TestStreamClaudeJSON_ThinkingStreamedProgressively verifies that thinking lines
// are emitted from stream_event deltas and NOT duplicated by the assistant event.
func TestStreamClaudeJSON_ThinkingStreamedProgressively(t *testing.T) {
	lines := []string{
		thinkingStartEvent(),
		thinkingDeltaEvent("step one\n"),
		thinkingDeltaEvent("step two\n"),
		thinkingStopEvent(),
		assistantWithThinkingAndText("step one\nstep two\n", "done"),
		mustJSON(map[string]any{"type": "result", "status": "success"}),
	}
	events, _ := collectClaudeEvents(lines...)

	var thinkingEvents, textEvents []StreamEvent
	for _, e := range events {
		switch e.Type {
		case "thinking":
			thinkingEvents = append(thinkingEvents, e)
		case "text":
			textEvents = append(textEvents, e)
		}
	}

	if len(thinkingEvents) != 2 {
		t.Fatalf("expected 2 thinking events (one per line), got %d: %v", len(thinkingEvents), thinkingEvents)
	}
	if thinkingEvents[0].Content != "step one" {
		t.Errorf("thinking[0].Content = %q, want %q", thinkingEvents[0].Content, "step one")
	}
	if thinkingEvents[1].Content != "step two" {
		t.Errorf("thinking[1].Content = %q, want %q", thinkingEvents[1].Content, "step two")
	}
	if len(textEvents) == 0 {
		t.Error("expected text event from assistant, got none")
	}
}

// TestStreamClaudeJSON_ThinkingNoNewlines verifies single-line thinking (no \n in deltas)
// is still deduplicated: the assistant event's thinking block must be suppressed even
// though no line was emitted from the deltas before hasStreamingThinking was set.
func TestStreamClaudeJSON_ThinkingNoNewlines(t *testing.T) {
	lines := []string{
		thinkingStartEvent(),
		thinkingDeltaEvent("no newline here"),
		thinkingStopEvent(),
		assistantWithThinkingAndText("no newline here", "answer"),
		mustJSON(map[string]any{"type": "result", "status": "success"}),
	}
	events, _ := collectClaudeEvents(lines...)

	var thinkingContents []string
	for _, e := range events {
		if e.Type == "thinking" {
			thinkingContents = append(thinkingContents, e.Content)
		}
	}

	if len(thinkingContents) != 1 {
		t.Fatalf("expected exactly 1 thinking event (no duplicate from assistant), got %d: %v", len(thinkingContents), thinkingContents)
	}
	if thinkingContents[0] != "no newline here" {
		t.Errorf("thinking content = %q, want %q", thinkingContents[0], "no newline here")
	}
}

// TestStreamClaudeJSON_ThinkingStateResetsOnResult verifies that thinking state
// is cleared at the result boundary so a subsequent turn starts fresh.
func TestStreamClaudeJSON_ThinkingStateResetsOnResult(t *testing.T) {
	turn1 := []string{
		thinkingStartEvent(),
		thinkingDeltaEvent("turn one thinking\n"),
		thinkingStopEvent(),
		assistantWithThinkingAndText("turn one thinking\n", "turn one text"),
		mustJSON(map[string]any{"type": "result", "status": "success"}),
	}
	// Second turn has no stream_events (simulates a turn without --include-partial-messages
	// or where thinking arrives only via the assembled assistant event).
	turn2 := []string{
		mustJSON(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "thinking", "thinking": "turn two thinking"},
					map[string]any{"type": "text", "text": "turn two text"},
				},
			},
		}),
	}
	events, _ := collectClaudeEvents(append(turn1, turn2...)...)

	var thinkingContents []string
	for _, e := range events {
		if e.Type == "thinking" {
			thinkingContents = append(thinkingContents, e.Content)
		}
	}

	// Turn 1: one thinking event from stream_events (assistant suppressed).
	// Turn 2: no stream_events so assistant thinking must pass through.
	if len(thinkingContents) != 2 {
		t.Fatalf("expected 2 thinking events (one per turn), got %d: %v", len(thinkingContents), thinkingContents)
	}
	if thinkingContents[0] != "turn one thinking" {
		t.Errorf("turn1 thinking = %q", thinkingContents[0])
	}
	if thinkingContents[1] != "turn two thinking" {
		t.Errorf("turn2 thinking = %q, want 'turn two thinking'", thinkingContents[1])
	}
}
