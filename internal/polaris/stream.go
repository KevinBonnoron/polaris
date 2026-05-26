package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// interactivePromptRe matches CLI prompts that wait for keyboard input without
// emitting a trailing newline. The patterns are intentionally tight to avoid
// false positives — extend deliberately, one case at a time.
var interactivePromptRe = regexp.MustCompile(`(?i)\[['"]?[yn]/[yn]['"]?\]\s*$|\((?:[yn]/[yn]|y/n)\)\s*$`)

// streamInteractive reads bytes from an interactive subprocess and flushes
// log lines on newline OR when the trailing content looks like a prompt that
// expects user input (e.g. `[y/N]`). When a prompt is detected, onWaiting is
// called so the runner can flip the agent's status to "waiting".
//
// Compared to streamLines, this function does not rely on bufio.Scanner — a
// prompt without a trailing newline would otherwise sit in the scanner buffer
// until EOF, leaving the UI showing "working" while the subprocess hangs.
func streamInteractive(reader io.Reader, sink io.Writer, onLine func(string), onWaiting func()) {
	br := bufio.NewReaderSize(reader, 4096)
	var pending strings.Builder
	emit := func() {
		text := strings.TrimRight(pending.String(), "\r")
		pending.Reset()
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), trimmed)
		fmt.Fprintln(sink, stamped)
		if onLine != nil {
			onLine(stamped)
		}
	}
	for {
		b, err := br.ReadByte()
		if err != nil {
			emit()
			return
		}
		if b == '\n' {
			emit()
			continue
		}
		pending.WriteByte(b)
		// Only run the (relatively expensive) regex when a likely terminator
		// just landed in the buffer; bracket-closes and question marks are the
		// usual cues.
		if (b == ']' || b == ')') && interactivePromptRe.MatchString(pending.String()) {
			emit()
			if onWaiting != nil {
				onWaiting()
			}
		}
	}
}

// streamTurnStats accumulates the per-turn metrics exposed by claude's
// stream-json output. The runner persists the snapshot to the agent row when
// the `result` event arrives at end-of-turn.
type streamTurnStats struct {
	Tokens        int
	Parts         usageParts
	CostUSD       float64
	FilesModified int
	ToolsUsed     int
}

// usageParts is the streaming-side spelling of TokenUsage: the per-turn usage
// snapshot the parser accumulates before it's folded into the agent row.
type usageParts = TokenUsage

// streamClaudeJSON parses claude's `--output-format stream-json --verbose`
// stdout: one JSON object per line. Each event is rendered to a human-readable
// log line via onLine (and also persisted to logFile), and the final `result`
// event drives onResult with the accumulated usage/cost figures.
//
// Unknown / malformed lines are passed through verbatim so we never silently
// drop output if claude introduces a new event type.
// onAskUserQuestion is invoked when the agent emits a tool_use for
// AskUserQuestion. The handler is expected to surface the prompt to the user
// and eventually call back into the runner with the answer as a tool_result.
type onAskUserQuestion func(toolUseID string, input map[string]any)

// onTokens is invoked whenever a mid-turn usage snapshot is observed so the UI
// can show a live token count instead of waiting for the end-of-turn result.
type onTokens func(tokens int, parts usageParts, costUSD float64)

func streamClaudeJSON(reader io.Reader, logFile io.Writer, onLine func(string), onAsk onAskUserQuestion, onTok onTokens, onResult func()) streamTurnStats {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	filesSet := make(map[string]struct{})
	toolInputs := make(map[string]toolInputSnapshot)
	var stats streamTurnStats

	// askPending tracks AskUserQuestion tool_use ids that have no answer yet. We
	// keep stdin open while any is pending so the process stays alive to receive
	// the user's reply as a real tool_result, instead of exiting and forcing the
	// answer through a fresh --resume turn (which the agent treats as a new user
	// message and re-asks). Cleared when the matching tool_result echoes back.
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
			emitStreamLine(logFile, onLine, raw)
			continue
		}
		// A tool_result echoed in a user event resolves its question.
		if kind, _ := evt["type"].(string); kind == "user" {
			for _, id := range userToolResultIDs(evt) {
				delete(askPending, id)
			}
		}
		prevTokens := stats.Tokens
		for _, line := range renderStreamEvent(evt, filesSet, toolInputs, wrappedAsk, &stats) {
			emitStreamLine(logFile, onLine, line)
		}
		if onTok != nil && stats.Tokens != prevTokens {
			onTok(stats.Tokens, stats.Parts, stats.CostUSD)
		}
		// Close stdin after the result event so Claude exits cleanly — but not
		// while a question is unanswered, so it stays alive to receive the reply.
		if kind, _ := evt["type"].(string); kind == "result" && onResult != nil && len(askPending) == 0 {
			onResult()
		}
	}

	stats.FilesModified = len(filesSet)
	return stats
}

func emitStreamLine(sink io.Writer, onLine func(string), line string) {
	if line == "" {
		return
	}
	stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line)
	fmt.Fprintln(sink, stamped)
	if onLine != nil {
		onLine(stamped)
	}
}

// toolInputSnapshot captures a tool_use's name + input so we can render a
// richer view (e.g. diff for Edit) when the matching tool_result arrives.
type toolInputSnapshot struct {
	name  string
	input map[string]any
}

// renderStreamEvent converts one stream-json event into zero or more
// log-friendly lines and side-effects the trackers/stats.
func renderStreamEvent(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []string {
	kind, _ := evt["type"].(string)
	switch kind {
	case "system":
		sub, _ := evt["subtype"].(string)
		if sub == "init" {
			model, _ := evt["model"].(string)
			toolsArr, _ := evt["tools"].([]any)
			return []string{fmt.Sprintf("[system] session ready (model=%s, tools=%d)", strDefault(model, "?"), len(toolsArr))}
		}
		return nil
	case "assistant":
		return renderAssistant(evt, files, toolInputs, onAsk, stats)
	case "user":
		return renderUserToolResults(evt, toolInputs)
	case "result":
		applyResultUsage(evt, stats)
		sub, _ := evt["subtype"].(string)
		if sub == "" {
			sub = "success"
		}
		return []string{fmt.Sprintf("[result] %s · %d tokens · $%.4f", sub, stats.Tokens, stats.CostUSD)}
	default:
		return nil
	}
}

func renderAssistant(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []string {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	applyUsage(msg["usage"], stats)
	content, _ := msg["content"].([]any)
	lines := make([]string, 0, len(content))
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
				lines = append(lines, t)
			}
		case "thinking":
			thinking, _ := block["thinking"].(string)
			if t := strings.TrimSpace(thinking); t != "" {
				for _, ln := range strings.Split(t, "\n") {
					lines = append(lines, "(thinking) "+ln)
				}
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
			// ExitPlanMode presents a plan for approval. Surface it through the same
			// question panel (plan + approve/reject/other) so the user decides; the
			// answer is delivered back as the tool_result.
			if name == "ExitPlanMode" && id != "" && onAsk != nil {
				onAsk(id, exitPlanModeQuestion(input))
			}
			lines = append(lines, fmt.Sprintf("→ [#%s] %s%s", id, name, summarizeToolInput(name, input)))
		}
	}
	return lines
}

// userToolResultIDs returns the tool_use_ids answered by a user event's
// tool_result blocks. Used to clear pending AskUserQuestion tracking.
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

func renderUserToolResults(evt map[string]any, toolInputs map[string]toolInputSnapshot) []string {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	lines := make([]string, 0, len(content))
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
		prefix := "←"
		if isErr {
			prefix = "✗"
		}
		if !isErr {
			if snap, ok := toolInputs[id]; ok {
				if custom := renderToolSnapshot(snap); custom != "" {
					body = custom
				}
			}
		}
		lines = append(lines, fmt.Sprintf("%s [#%s] %s", prefix, id, truncate(body, 4000)))
	}
	return lines
}

func renderToolSnapshot(snap toolInputSnapshot) string {
	if diff := renderEditDiff(snap); diff != "" {
		return diff
	}
	if list := renderTodoList(snap); list != "" {
		return list
	}
	return ""
}

func renderTodoList(snap toolInputSnapshot) string {
	if snap.name != "TodoWrite" {
		return ""
	}
	todos, _ := snap.input["todos"].([]any)
	if len(todos) == 0 {
		return ""
	}
	lines := make([]string, 0, len(todos))
	for _, t := range todos {
		tm, _ := t.(map[string]any)
		if tm == nil {
			continue
		}
		content, _ := tm["content"].(string)
		status, _ := tm["status"].(string)
		var marker string
		switch status {
		case "completed":
			marker = "[x]"
		case "in_progress":
			marker = "[~]"
		default:
			marker = "[ ]"
		}
		lines = append(lines, marker+" "+content)
	}
	return strings.Join(lines, "\n")
}

// renderEditDiff produces a readable preview for file-modifying tools whose
// success message ("File … has been updated") would otherwise be useless.
func renderEditDiff(snap toolInputSnapshot) string {
	switch snap.name {
	case "Edit", "Update":
		oldStr, _ := snap.input["old_string"].(string)
		newStr, _ := snap.input["new_string"].(string)
		return diffLines(oldStr, newStr)
	case "Write":
		content, _ := snap.input["content"].(string)
		return prefixLines(content, "+ ")
	case "MultiEdit":
		edits, _ := snap.input["edits"].([]any)
		parts := make([]string, 0, len(edits))
		for _, e := range edits {
			em, _ := e.(map[string]any)
			if em == nil {
				continue
			}
			oldStr, _ := em["old_string"].(string)
			newStr, _ := em["new_string"].(string)
			parts = append(parts, diffLines(oldStr, newStr))
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

func diffLines(oldStr, newStr string) string {
	return prefixLines(oldStr, "- ") + "\n" + prefixLines(newStr, "+ ")
}

func prefixLines(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

func applyResultUsage(evt map[string]any, stats *streamTurnStats) {
	if cost, ok := numericField(evt, "total_cost_usd"); ok {
		stats.CostUSD = cost
	}
	if p, ok := usageBreakdown(evt["usage"]); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

// applyUsage records a mid-turn assistant message's usage so the live counter
// tracks the turn as it progresses. Each assistant sub-request re-reports the
// full (growing) context, so the latest snapshot — not the sum — is the right
// estimate; summing would multiply the cached context by the number of rounds.
// The end-of-turn result event later overwrites these with the authoritative
// figure.
func applyUsage(raw any, stats *streamTurnStats) {
	if p, ok := usageBreakdown(raw); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

// usageBreakdown splits a usage map into the four categories the UI surfaces so
// users can see why a turn's total is large (cache re-reads dominate). The bool
// is false when raw is not a usage map.
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
	return usageParts{
		Input:         get("input_tokens"),
		Output:        get("output_tokens"),
		CacheCreation: get("cache_creation_input_tokens"),
		CacheRead:     get("cache_read_input_tokens"),
	}, true
}

func numericField(m map[string]any, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// exitPlanModeQuestion turns an ExitPlanMode tool_use into an AskUserQuestion
// payload: the plan as the prompt plus approve/reject options (the panel also
// offers a free-text "Other" for proposing changes). The chosen answer is sent
// back as the tool_result, so the agent proceeds, revises, or stops accordingly.
func exitPlanModeQuestion(input map[string]any) map[string]any {
	plan, _ := input["plan"].(string)
	if strings.TrimSpace(plan) == "" {
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

// extractFilePath pulls a representative file path out of a tool_use input for
// edit-style tools so we can count distinct files touched in a turn.
func extractFilePath(name string, input map[string]any) (string, bool) {
	switch name {
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "Update":
		if fp, ok := input["file_path"].(string); ok && fp != "" {
			return fp, true
		}
	}
	return "", false
}

func summarizeToolInput(name string, input map[string]any) string {
	if input == nil {
		return ""
	}
	switch name {
	case "Bash":
		if cmd, ok := input["command"].(string); ok && cmd != "" {
			return " · " + truncate(strings.ReplaceAll(cmd, "\n", " "), 160)
		}
	case "Read":
		if fp, ok := input["file_path"].(string); ok && fp != "" {
			detail := fp
			offset, hasOffset := numericField(input, "offset")
			limit, hasLimit := numericField(input, "limit")
			if hasOffset && hasLimit {
				detail += fmt.Sprintf(":%d-%d", int(offset), int(offset)+int(limit)-1)
			} else if hasOffset {
				detail += fmt.Sprintf(":%d+", int(offset))
			} else if hasLimit {
				detail += fmt.Sprintf(":1-%d", int(limit))
			}
			return " · " + detail
		}
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "Update":
		if fp, ok := input["file_path"].(string); ok && fp != "" {
			return " · " + fp
		}
	case "Grep":
		pattern, _ := input["pattern"].(string)
		if pattern == "" {
			return ""
		}
		parts := []string{"/" + truncate(pattern, 80) + "/"}
		if path, ok := input["path"].(string); ok && path != "" {
			parts = append(parts, "in "+path)
		}
		if glob, ok := input["glob"].(string); ok && glob != "" {
			parts = append(parts, glob)
		} else if typ, ok := input["type"].(string); ok && typ != "" {
			parts = append(parts, "*."+typ)
		}
		return " · " + strings.Join(parts, " ")
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return " · " + truncate(p, 120)
		}
	case "WebFetch", "WebSearch":
		if u, ok := input["url"].(string); ok && u != "" {
			return " · " + truncate(u, 120)
		}
		if q, ok := input["query"].(string); ok && q != "" {
			return " · " + truncate(q, 120)
		}
	case "TodoWrite":
		todos, _ := input["todos"].([]any)
		if len(todos) == 0 {
			return ""
		}
		var pending, inProgress, completed int
		for _, t := range todos {
			tm, _ := t.(map[string]any)
			if tm == nil {
				continue
			}
			switch tm["status"] {
			case "completed":
				completed++
			case "in_progress":
				inProgress++
			default:
				pending++
			}
		}
		return fmt.Sprintf(" · %d todos (%d done, %d in progress, %d pending)", len(todos), completed, inProgress, pending)
	}
	return ""
}

func toolResultText(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			block, _ := item.(map[string]any)
			if block == nil {
				continue
			}
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return ""
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func strDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
