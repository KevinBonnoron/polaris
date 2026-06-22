package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

// lockedWriter serialises concurrent writes to an underlying writer. The agent
// log is fed by the stdout parser and the stderr drain goroutines at the same
// time; without this their fmt.Fprintln calls can interleave and produce
// corrupt JSONL lines, which then drop silently when the log is re-parsed.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newLockedWriter(w io.Writer) *lockedWriter { return &lockedWriter{w: w} }

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

var interactivePromptRe = regexp.MustCompile(`(?i)\[['"]?[yn]/[yn]['"]?\]\s*$|\((?:[yn]/[yn]|y/n)\)\s*$`)

// StreamEvent is the canonical event emitted to the log file (JSONL) and
// forwarded to the React frontend via Wails events.
type StreamEvent struct {
	Type    string         `json:"type"`
	Ts      string         `json:"ts"`
	Content string         `json:"content,omitempty"`
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
	Error   bool           `json:"error,omitempty"`
	// RenderedContent carries a precomputed diff or todo list for tool results
	// where the raw input is more informative than the CLI's success message.
	RenderedContent string      `json:"rendered_content,omitempty"`
	Status          string      `json:"status,omitempty"`
	Tokens          int         `json:"tokens,omitempty"`
	CostUSD         float64     `json:"cost_usd,omitempty"`
	Parts           *TokenUsage `json:"parts,omitempty"`
}

// streamTurnStats accumulates the per-turn metrics exposed by stream-json
// output. The runner persists the snapshot when the turn ends.
type streamTurnStats struct {
	Tokens        int
	Parts         usageParts
	CostUSD       float64
	FilesModified int
	ToolsUsed     int
	// Succeeded is set when a result event arrives cleanly (no error flag).
	Succeeded bool
}

type usageParts = TokenUsage

type onAskUserQuestion func(toolUseID string, input map[string]any)
type onTokens func(tokens int, parts usageParts, costUSD float64)

// emitEvent serialises evt as a JSONL line to sink and calls onEvent.
// Lines whose Content matches isTechnicalNoise are silently dropped.
func emitEvent(sink io.Writer, onEvent func(StreamEvent), evt StreamEvent) {
	if evt.Content != "" && isTechnicalNoise(evt.Content) {
		return
	}
	if evt.Content != "" {
		if msg := outputTokenLimitError(evt.Content); msg != "" {
			evt = StreamEvent{Type: "text", Error: true, Content: msg}
		}
	}
	if evt.Ts == "" {
		evt.Ts = time.Now().Format("15:04:05")
	}
	data, _ := json.Marshal(evt)
	fmt.Fprintln(sink, string(data))
	if onEvent != nil {
		onEvent(evt)
	}
}

// streamInteractive reads bytes from an interactive subprocess and flushes
// log events on newline OR when the trailing content looks like a prompt that
// expects user input (e.g. `[y/N]`). When a prompt is detected, onWaiting is
// called so the runner can flip the agent's status to "waiting".
func streamInteractive(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onWaiting func()) {
	br := bufio.NewReaderSize(reader, 4096)
	var pending strings.Builder
	emit := func() {
		text := strings.TrimSpace(strings.TrimRight(pending.String(), "\r"))
		pending.Reset()
		if text == "" {
			return
		}
		emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: text})
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
		if (b == ']' || b == ')') && interactivePromptRe.MatchString(pending.String()) {
			emit()
			if onWaiting != nil {
				onWaiting()
			}
		}
	}
}

// --- shared utilities ---

func renderToolSnapshot(name string, input map[string]any) string {
	if diff := renderEditDiff(name, input); diff != "" {
		return diff
	}
	if list := renderTodoList(name, input); list != "" {
		return list
	}
	return ""
}

func renderTodoList(name string, input map[string]any) string {
	if name != "TodoWrite" {
		return ""
	}
	todos, _ := input["todos"].([]any)
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

func renderEditDiff(name string, input map[string]any) string {
	switch name {
	case "Edit", "Update":
		oldStr, _ := input["old_string"].(string)
		newStr, _ := input["new_string"].(string)
		return diffLines(oldStr, newStr)
	case "Write":
		content, _ := input["content"].(string)
		return prefixLines(content, "+ ")
	case "MultiEdit":
		edits, _ := input["edits"].([]any)
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
	case "AskUserQuestion":
		return " · " + summarizeQuestionLine(input)
	case "ExitPlanMode":
		return " · Plan ready for review"
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

// summarizeQuestionLine renders a one-line label for an AskUserQuestion tool call
// (the first question's header + text, "(+N)" when there are more), shown as the
// `· detail` on the transcript's `→ AskUserQuestion` line.
func summarizeQuestionLine(input map[string]any) string {
	questions, _ := input["questions"].([]any)
	if len(questions) == 0 {
		return "question"
	}
	first, _ := questions[0].(map[string]any)
	if first == nil {
		return "question"
	}
	q, _ := first["question"].(string)
	if h, _ := first["header"].(string); h != "" {
		q = h + ": " + q
	}
	label := truncate(q, 120)
	if len(questions) > 1 {
		label += fmt.Sprintf(" (+%d)", len(questions)-1)
	}
	return label
}

// questionAnswerRecap renders a persisted AskUserQuestion / ExitPlanMode prompt
// together with the user's choice as a multi-line block, kept in the transcript
// (as a tool_result preview) so the exchange stays reviewable after the live panel
// is gone. The ExitPlanMode plan is surfaced under a "Plan" header.
func questionAnswerRecap(input json.RawMessage, answer string) string {
	var payload struct {
		Questions []struct {
			Header   string `json:"header"`
			Question string `json:"question"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(input, &payload) != nil || len(payload.Questions) == 0 {
		return summarizeAnswer(answer)
	}
	var answers []struct {
		Answer json.RawMessage `json:"answer"`
	}
	_ = json.Unmarshal([]byte(answer), &answers)
	answerLabel := func(i int) string {
		if i >= len(answers) {
			return summarizeAnswer(answer)
		}
		var s string
		var arr []string
		switch {
		case json.Unmarshal(answers[i].Answer, &s) == nil:
			return s
		case json.Unmarshal(answers[i].Answer, &arr) == nil:
			return strings.Join(arr, ", ")
		}
		return summarizeAnswer(answer)
	}
	var sb strings.Builder
	for i, q := range payload.Questions {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if q.Header == "Plan" {
			sb.WriteString(q.Question)
			sb.WriteString("\n\nDecision: ")
			sb.WriteString(answerLabel(i))
			continue
		}
		if q.Header != "" {
			sb.WriteString(q.Header)
			sb.WriteString(": ")
		}
		sb.WriteString(q.Question)
		for _, opt := range q.Options {
			sb.WriteString("\n· ")
			sb.WriteString(opt.Label)
		}
		sb.WriteString("\nAnswer: ")
		sb.WriteString(answerLabel(i))
	}
	return sb.String()
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
