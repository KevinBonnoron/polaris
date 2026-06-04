package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// tmpWriteRe matches shell redirections that write to /tmp.
var tmpWriteRe = regexp.MustCompile(`(?:>+|tee)\s+(/tmp/\S+)`)

// streamCursorJSON parses cursor's stream-json stdout. Cursor uses the same
// schema as claude-code but runs as a one-shot process (no interactive stdin),
// so AskUserQuestion and stdin-close callbacks are not needed.
func streamCursorJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onTok onTokens) streamTurnStats {
	return streamClaudeJSON(reader, sink, onEvent, nil, onTok, nil)
}

func streamClaudeJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onAsk onAskUserQuestion, onTok onTokens, onResult func()) streamTurnStats {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	filesSet := make(map[string]struct{})
	toolInputs := make(map[string]toolInputSnapshot)
	var stats streamTurnStats

	askPending := map[string]struct{}{}
	wrappedAsk := onAsk
	if onAsk != nil {
		wrappedAsk = func(toolUseID string, input map[string]any) {
			askPending[toolUseID] = struct{}{}
			onAsk(toolUseID, input)
		}
	}

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
		if kind, _ := evt["type"].(string); kind == "user" {
			for _, id := range userToolResultIDs(evt) {
				delete(askPending, id)
			}
		}
		prevTokens := stats.Tokens
		for _, se := range claudeEventToStreamEvents(evt, filesSet, toolInputs, wrappedAsk, &stats) {
			emitEvent(sink, onEvent, se)
		}
		if onTok != nil && stats.Tokens != prevTokens {
			onTok(stats.Tokens, stats.Parts, stats.CostUSD)
		}
		if kind, _ := evt["type"].(string); kind == "result" && onResult != nil && len(askPending) == 0 {
			onResult()
		}
	}

	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: fmt.Sprintf("stream read error: %v", err)})
	}

	stats.FilesModified = len(filesSet)
	return stats
}

// toolInputSnapshot captures a tool_use's name + input so we can render a
// richer result (e.g. diff for Edit) when the matching tool_result arrives.
type toolInputSnapshot struct {
	name  string
	input map[string]any
}

// claudeEventToStreamEvents converts one stream-json event into zero or more
// StreamEvents and updates the trackers/stats.
func claudeEventToStreamEvents(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	kind, _ := evt["type"].(string)
	switch kind {
	case "system", "init":
		return nil
	case "assistant":
		return renderClaudeAssistant(evt, files, toolInputs, onAsk, stats)
	case "user":
		return renderClaudeUserToolResults(evt, toolInputs)
	case "result":
		applyResultUsage(evt, stats)
		isErr, _ := evt["is_error"].(bool)
		status, _ := evt["status"].(string)
		stats.Succeeded = !isErr && status != "error"
		sub, _ := evt["subtype"].(string)
		if sub == "" {
			if isErr || status == "error" {
				sub = "error"
			} else {
				sub = "success"
			}
		}
		return []StreamEvent{{
			Type:    "turn_end",
			Status:  sub,
			Tokens:  stats.Tokens,
			CostUSD: stats.CostUSD,
			Parts:   &stats.Parts,
		}}
	default:
		return nil
	}
}

func renderClaudeAssistant(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	applyUsage(msg["usage"], stats)
	content, _ := msg["content"].([]any)
	events := make([]StreamEvent, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		btype, _ := block["type"].(string)
		switch btype {
		case "text":
			text, _ := block["text"].(string)
			if t := strings.TrimSpace(text); t != "" {
				events = append(events, StreamEvent{Type: "text", Content: t})
			}
		case "thinking":
			thinking, _ := block["thinking"].(string)
			if t := strings.TrimSpace(thinking); t != "" {
				events = append(events, StreamEvent{Type: "thinking", Content: t})
			}
		case "tool_use":
			name, _ := block["name"].(string)
			input, _ := block["input"].(map[string]any)
			id, _ := block["id"].(string)
			stats.ToolsUsed++
			if fp, ok := extractFilePath(name, input); ok {
				files[fp] = struct{}{}
			}
			if id != "" {
				toolInputs[id] = toolInputSnapshot{name: name, input: input}
			}
			if name == "AskUserQuestion" && id != "" && onAsk != nil {
				onAsk(id, input)
			}
			if name == "ExitPlanMode" && id != "" && onAsk != nil {
				onAsk(id, exitPlanModeQuestion(input, toolInputs))
			}
			se := StreamEvent{
				Type:  "tool_call",
				ID:    id,
				Name:  name,
				Input: input,
			}
			// Embed a summary detail in Content so the frontend doesn't need
			// to re-derive it from Input for the compact display.
			if detail := summarizeToolInput(name, input); detail != "" {
				se.Content = strings.TrimPrefix(detail, " · ")
			}
			events = append(events, se)
		}
	}
	return events
}

func renderClaudeUserToolResults(evt map[string]any, toolInputs map[string]toolInputSnapshot) []StreamEvent {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	events := make([]StreamEvent, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		if t, _ := block["type"].(string); t != "tool_result" {
			continue
		}
		body := toolResultText(block["content"])
		isErr, _ := block["is_error"].(bool)
		id, _ := block["tool_use_id"].(string)
		se := StreamEvent{
			Type:  "tool_result",
			ID:    id,
			Error: isErr,
		}
		if !isErr {
			if snap, ok := toolInputs[id]; ok {
				se.RenderedContent = renderToolSnapshot(snap.name, snap.input)
			}
		}
		se.Content = truncate(body, 4000)
		events = append(events, se)
	}
	return events
}

func userToolResultIDs(evt map[string]any) []string {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	var ids []string
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		if t, _ := block["type"].(string); t != "tool_result" {
			continue
		}
		if id, _ := block["tool_use_id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func applyUsage(raw any, stats *streamTurnStats) {
	if p, ok := usageBreakdown(raw); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

func applyResultUsage(evt map[string]any, stats *streamTurnStats) {
	if cost, ok := numericField(evt, "total_cost_usd"); ok {
		stats.CostUSD = cost
	}
	raw := evt["usage"]
	if raw == nil {
		raw = evt["stats"]
	}
	if p, ok := usageBreakdown(raw); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

func usageBreakdown(raw any) (usageParts, bool) {
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
		Input:         get("input_tokens"),
		Output:        get("output_tokens"),
		CacheCreation: get("cache_creation_input_tokens"),
		CacheRead:     get("cache_read_input_tokens"),
	}
	if res.CacheRead == 0 {
		res.CacheRead = get("cached")
	}
	return res, true
}

// exitPlanModeQuestion turns an ExitPlanMode tool_use into an AskUserQuestion
// payload so the user can approve/reject/revise the plan.
func exitPlanModeQuestion(input map[string]any, toolInputs map[string]toolInputSnapshot) map[string]any {
	plan, _ := input["plan"].(string)
	plan = strings.TrimSpace(plan)
	if strings.HasPrefix(plan, "/") {
		if content, err := os.ReadFile(plan); err == nil {
			plan = strings.TrimSpace(string(content))
		}
	}
	if plan == "" {
		if path := tmpPlanFile(toolInputs); path != "" {
			if content, err := os.ReadFile(path); err == nil {
				plan = strings.TrimSpace(string(content))
			}
		}
	}
	if plan == "" {
		plan = writtenPlanContent(toolInputs)
	}
	if plan == "" {
		plan = "The agent finished planning and wants to proceed."
	}
	return map[string]any{
		"questions": []map[string]any{{
			"header":   "Plan",
			"question": plan,
			"options": []map[string]any{
				{"label": "Approve & proceed"},
				{"label": "Reject"},
			},
		}},
	}
}

func tmpPlanFile(toolInputs map[string]toolInputSnapshot) string {
	for _, snap := range toolInputs {
		if snap.name != "Bash" {
			continue
		}
		cmd, _ := snap.input["command"].(string)
		if m := tmpWriteRe.FindStringSubmatch(cmd); len(m) > 1 {
			return strings.TrimRight(m[1], "\"';")
		}
	}
	return ""
}

func writtenPlanContent(toolInputs map[string]toolInputSnapshot) string {
	var fallback string
	for _, snap := range toolInputs {
		if snap.name != "Write" {
			continue
		}
		fp, _ := snap.input["file_path"].(string)
		content, _ := snap.input["content"].(string)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if strings.Contains(strings.ToLower(fp), "plan") {
			return content
		}
		fallback = content
	}
	return fallback
}
