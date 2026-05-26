package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// streamGeminiJSON parses gemini's `--output-format stream-json` stdout.
// Gemini's read_file tool intentionally leaves output empty (content goes to
// llmContent for the model context only). We recover it by reading the file
// from disk using workDir so the UI can display it in the result accordion.
func streamGeminiJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onAsk onAskUserQuestion, onTok onTokens, onResult func(), workDir string) streamTurnStats {
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

		kind, _ := evt["type"].(string)

		if kind == "user" {
			for _, id := range userToolResultIDs(evt) {
				delete(askPending, id)
			}
		}

		if kind == "tool_result" {
			id, _ := evt["tool_id"].(string)
			status, _ := evt["status"].(string)
			output := evt["output"]
			if id != "" && status != "error" && toolResultText(output) == "" {
				if snap, ok := toolInputs[id]; ok && snap.name == "Read" {
					if fp, _ := snap.input["file_path"].(string); fp != "" {
						if !filepath.IsAbs(fp) {
							fp = filepath.Join(workDir, fp)
						}
						if content, err := os.ReadFile(fp); err == nil {
							evt["output"] = strings.TrimSpace(string(content))
						}
					}
				}
			}
		}

		prevTokens := stats.Tokens
		for _, se := range geminiEventToStreamEvents(evt, filesSet, toolInputs, wrappedAsk, &stats) {
			emitEvent(sink, onEvent, se)
		}
		if onTok != nil && stats.Tokens != prevTokens {
			onTok(stats.Tokens, stats.Parts, stats.CostUSD)
		}
		if kind == "result" && onResult != nil && len(askPending) == 0 {
			onResult()
		}
	}

	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: fmt.Sprintf("stream read error: %v", err)})
	}

	stats.FilesModified = len(filesSet)
	return stats
}

func geminiEventToStreamEvents(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	kind, _ := evt["type"].(string)
	switch kind {
	case "message":
		role, _ := evt["role"].(string)
		if role == "assistant" {
			content, _ := evt["content"].(string)
			if content = strings.TrimSpace(content); content != "" {
				return []StreamEvent{{Type: "text", Content: content}}
			}
		}
		return nil
	case "tool_use":
		return renderGeminiToolUse(evt, files, toolInputs, onAsk, stats)
	case "tool_result":
		return renderGeminiToolResult(evt, toolInputs)
	case "result":
		applyResultUsage(evt, stats)
		isErr, _ := evt["is_error"].(bool)
		status, _ := evt["status"].(string)
		stats.Succeeded = !isErr && status != "error"
		sub := status
		if sub == "" {
			if isErr {
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

func renderGeminiToolUse(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	name, _ := evt["tool_name"].(string)
	name = mapGeminiToolName(name)
	input, _ := evt["parameters"].(map[string]any)
	id, _ := evt["tool_id"].(string)
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
	se := StreamEvent{
		Type:  "tool_call",
		ID:    id,
		Name:  name,
		Input: input,
	}
	if detail := summarizeToolInput(name, input); detail != "" {
		se.Content = strings.TrimPrefix(detail, " · ")
	}
	return []StreamEvent{se}
}

func renderGeminiToolResult(evt map[string]any, toolInputs map[string]toolInputSnapshot) []StreamEvent {
	id, _ := evt["tool_id"].(string)
	status, _ := evt["status"].(string)
	output := evt["output"]

	isErr := status == "error"
	body := toolResultText(output)

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
	return []StreamEvent{se}
}

func mapGeminiToolName(name string) string {
	switch name {
	case "read_file":
		return "Read"
	case "write_file":
		return "Write"
	case "replace":
		return "Edit"
	case "grep_search":
		return "Grep"
	case "run_shell_command":
		return "Bash"
	case "list_directory":
		return "LS"
	case "glob":
		return "Glob"
	default:
		return name
	}
}
