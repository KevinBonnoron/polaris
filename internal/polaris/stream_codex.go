package polaris

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"time"
)

var codexStderrRe = regexp.MustCompile(`^(?P<ts>\d{4}-\d{2}-\d{2}T[\d:.]+Z)\s+(?P<level>[A-Z]+)\s+(?P<body>.+)$`)

// streamCodexJSON parses Codex's `--json` newline-delimited events.
// Codex is one-shot and does not use interactive stdin, so we only decode the
// emitted JSONL stream and surface the relevant agent/tool events.
func streamCodexJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onThread func(string), onTok onTokens) streamTurnStats {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var stats streamTurnStats

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}

		var evt map[string]any
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: raw})
			continue
		}

		switch kind := strAny(evt["type"]); kind {
		case "response_item":
			payload, _ := evt["payload"].(map[string]any)
			for _, se := range codexResponseItemToStreamEvents(payload) {
				emitEvent(sink, onEvent, se)
				if se.Type == "tool_call" {
					stats.ToolsUsed++
				}
			}
		case "thread.started":
			if onThread != nil {
				if threadID := strAny(evt["thread_id"]); threadID != "" {
					onThread(threadID)
				}
			}
		case "item.started":
			item, _ := evt["item"].(map[string]any)
			if item == nil {
				continue
			}
			switch strAny(item["type"]) {
			case "command_execution":
				se := codexCommandCall(item)
				emitEvent(sink, onEvent, se)
				stats.ToolsUsed++
			case "function_call", "custom_tool_call":
				if se, ok := codexFunctionCall(item); ok {
					emitEvent(sink, onEvent, se)
					stats.ToolsUsed++
				}
			}
		case "item.completed":
			item, _ := evt["item"].(map[string]any)
			if item == nil {
				continue
			}
			switch strAny(item["type"]) {
			case "agent_message":
				if text := strings.TrimSpace(strAny(item["text"])); text != "" {
					emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: text})
				}
			case "command_execution":
				se := codexCommandResult(item)
				emitEvent(sink, onEvent, se)
			case "function_call_output", "custom_tool_call_output":
				se := codexFunctionResult(item)
				emitEvent(sink, onEvent, se)
			}
		case "turn.completed":
			if p, ok := codexUsageBreakdown(evt["usage"]); ok {
				stats.Parts = p
				stats.Tokens = p.Total()
				if onTok != nil {
					onTok(stats.Tokens, stats.Parts, stats.CostUSD)
				}
			}
			succeeded := true
			switch strings.ToLower(strAny(evt["status"])) {
			case "failed", "error", "cancelled", "canceled":
				succeeded = false
			}
			stats.Succeeded = succeeded
			emitEvent(sink, onEvent, StreamEvent{
				Type:   "turn_end",
				Status: strStatus(succeeded),
				Tokens: stats.Tokens,
				Parts:  &stats.Parts,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: "codex stdout scan error: " + err.Error()})
	}

	stats.FilesModified = 0
	return stats
}

func codexResponseItemToStreamEvents(payload map[string]any) []StreamEvent {
	if payload == nil {
		return nil
	}
	switch strAny(payload["type"]) {
	case "message":
		if strAny(payload["role"]) != "assistant" {
			return nil
		}
		if text := codexMessageText(payload["content"]); text != "" {
			return []StreamEvent{{Type: "text", Content: text}}
		}
	case "function_call", "custom_tool_call":
		if se, ok := codexFunctionCall(payload); ok {
			return []StreamEvent{se}
		}
	case "function_call_output", "custom_tool_call_output":
		return []StreamEvent{codexFunctionResult(payload)}
	}
	return nil
}

func codexCommandCall(item map[string]any) StreamEvent {
	se := StreamEvent{
		Type: "tool_call",
		ID:   codexItemID(item),
		Name: "Bash",
	}
	if detail := summarizeToolInput("Bash", map[string]any{"command": strAny(item["command"])}); detail != "" {
		se.Content = strings.TrimPrefix(detail, " · ")
	}
	return se
}

func codexCommandResult(item map[string]any) StreamEvent {
	exitCode := intAny(item["exit_code"])
	se := StreamEvent{
		Type:   "tool_result",
		ID:     codexItemID(item),
		Name:   "Bash",
		Error:  exitCode != 0,
		Status: strStatus(exitCode == 0),
	}
	se.Content = strings.TrimSpace(truncate(strAny(item["aggregated_output"]), 4000))
	if se.Error && se.Content == "" {
		se.Content = "command failed"
	}
	return se
}

func codexFunctionCall(item map[string]any) (StreamEvent, bool) {
	rawName := strAny(item["name"])
	if rawName == "" {
		return StreamEvent{}, false
	}
	name := codexToolName(rawName)
	input := codexToolInput(item)
	se := StreamEvent{
		Type: "tool_call",
		ID:   codexItemID(item),
		Name: name,
	}
	if name == "Agent" {
		se.Input = input
	} else if detail := summarizeToolInput(name, input); detail != "" {
		se.Content = strings.TrimPrefix(detail, " · ")
	}
	return se, true
}

func codexFunctionResult(item map[string]any) StreamEvent {
	return StreamEvent{
		Type:    "tool_result",
		ID:      codexItemID(item),
		Content: codexOutputText(item["output"]),
		Error:   strings.EqualFold(strAny(item["status"]), "failed"),
		Status:  strAny(item["status"]),
	}
}

func codexItemID(item map[string]any) string {
	for _, key := range []string{"call_id", "id", "tool_call_id"} {
		if id := strAny(item[key]); id != "" {
			return id
		}
	}
	return ""
}

func codexToolInput(item map[string]any) map[string]any {
	rawArgs := item["arguments"]
	if rawArgs == nil {
		rawArgs = item["input"]
	}
	switch v := rawArgs.(type) {
	case map[string]any:
		return v
	case string:
		var input map[string]any
		if json.Unmarshal([]byte(v), &input) == nil && input != nil {
			return input
		}
		if v != "" {
			return map[string]any{"arguments": v}
		}
	}
	return nil
}

func codexToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if name == "view_image" {
		return "Read"
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			b.WriteString(part[1:])
		}
	}
	if b.Len() == 0 {
		return name
	}
	return b.String()
}

func codexOutputText(output any) string {
	switch v := output.(type) {
	case string:
		return strings.TrimSpace(truncate(v, 4000))
	case []any:
		for _, item := range v {
			block, _ := item.(map[string]any)
			if block == nil {
				continue
			}
			switch strAny(block["type"]) {
			case "input_image", "image":
				return "image loaded"
			case "text", "output_text":
				if text := strings.TrimSpace(strAny(block["text"])); text != "" {
					return truncate(text, 4000)
				}
			}
		}
	case map[string]any:
		switch strAny(v["type"]) {
		case "input_image", "image":
			return "image loaded"
		case "text", "output_text":
			if text := strings.TrimSpace(strAny(v["text"])); text != "" {
				return truncate(text, 4000)
			}
		}
	}
	return ""
}

func codexMessageText(content any) string {
	blocks, _ := content.([]any)
	if len(blocks) == 0 {
		return ""
	}
	lines := make([]string, 0, len(blocks))
	for _, item := range blocks {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		switch strAny(block["type"]) {
		case "output_text", "text":
			if text := strings.TrimSpace(strAny(block["text"])); text != "" {
				lines = append(lines, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// streamCodexStderr normalises Codex stderr lines so timestamped logger output
// remains readable in the transcript instead of showing up as raw red text.
func streamCodexStderr(reader io.Reader, sink io.Writer, onEvent func(StreamEvent)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if isTechnicalNoise(text) {
			continue
		}
		if evt, ok := parseCodexStderrLine(text); ok {
			emitEvent(sink, onEvent, evt)
			continue
		}
		emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: text})
	}
	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: "codex stderr scan error: " + err.Error()})
	}
}

func strStatus(ok bool) string {
	if ok {
		return "completed"
	}
	return "error"
}

func codexUsageBreakdown(raw any) (usageParts, bool) {
	usage, _ := raw.(map[string]any)
	if usage == nil {
		return usageParts{}, false
	}
	get := func(k string) int {
		if v, ok := numericField(usage, k); ok {
			return int(v)
		}
		return 0
	}
	res := usageParts{
		Input:     get("input_tokens"),
		Output:    get("output_tokens") + get("reasoning_output_tokens"),
		CacheRead: get("cached_input_tokens"),
	}
	return res, true
}

func strAny(v any) string {
	s, _ := v.(string)
	return s
}

func intAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func parseCodexStderrLine(line string) (StreamEvent, bool) {
	m := codexStderrRe.FindStringSubmatch(line)
	if m == nil {
		return StreamEvent{}, false
	}
	ts := codexTimestamp(m[1])
	body := strings.TrimSpace(m[3])
	if body == "" {
		return StreamEvent{}, false
	}
	return StreamEvent{Type: "system", Ts: ts, Content: body}, true
}

func codexTimestamp(raw string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("15:04:05")
		}
	}
	return ""
}
