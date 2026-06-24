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
				id := strAny(item["id"])
				cmd := strAny(item["command"])
				se := StreamEvent{
					Type:  "tool_call",
					ID:    id,
					Name:  "Bash",
					Input: map[string]any{"command": cmd},
				}
				if detail := summarizeToolInput("Bash", se.Input); detail != "" {
					se.Content = strings.TrimPrefix(detail, " · ")
				}
				emitEvent(sink, onEvent, se)
				stats.ToolsUsed++
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
				id := strAny(item["id"])
				output := truncate(strAny(item["aggregated_output"]), 4000)
				exitCode := intAny(item["exit_code"])
				se := StreamEvent{
					Type:   "tool_result",
					ID:     id,
					Name:   "Bash",
					Error:  exitCode != 0,
					Status: strStatus(exitCode == 0),
				}
				se.Content = strings.TrimSpace(output)
				if se.Error && se.Content == "" {
					se.Content = "command failed"
				}
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
