package polaris

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ClaudeSession struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	UpdatedAt    int64  `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	// Imported is true when the session is already tracked as an agent in
	// Polaris. These sessions are still returned so the user can see them
	// and send follow-ups, but the UI dims them and moves them to the end.
	Imported bool `json:"imported"`
}

// claudeProjectDir returns the ~/.claude/projects/<encoded> directory for the
// given project path. Claude Code encodes paths by replacing every '/' with '-'.
func claudeProjectDir(projectPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	encoded := strings.ReplaceAll(projectPath, "/", "-")
	return filepath.Join(home, ".claude", "projects", encoded)
}

// ListClaudeCodeSessions returns Claude Code sessions stored on disk for
// projectID that are not already tracked as agents in Polaris.
func (service *Service) ListClaudeCodeSessions(projectID string) ([]ClaudeSession, error) {
	if service.store == nil {
		return nil, errors.New("store not initialised")
	}
	project, err := service.store.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("find project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project %q not found", projectID)
	}

	dir := claudeProjectDir(project.Path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ClaudeSession{}, nil
		}
		return nil, err
	}

	imported := map[string]bool{}
	if agents, err := service.store.ListAgents(projectID); err == nil {
		for _, a := range agents {
			if a.SessionID != "" {
				imported[a.SessionID] = true
			}
		}
	}

	var active, dimmed []ClaudeSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		summary, _, count := parseClaudeJSONL(path)
		s := ClaudeSession{
			ID:           sessionID,
			Summary:      summary,
			UpdatedAt:    info.ModTime().Unix(),
			MessageCount: count,
			Imported:     imported[sessionID],
		}
		if s.Imported {
			dimmed = append(dimmed, s)
		} else {
			active = append(active, s)
		}
	}

	byRecent := func(a, b ClaudeSession) bool { return a.UpdatedAt > b.UpdatedAt }
	sort.Slice(active, func(i, j int) bool { return byRecent(active[i], active[j]) })
	sort.Slice(dimmed, func(i, j int) bool { return byRecent(dimmed[i], dimmed[j]) })

	const maxActive = 30
	if len(active) > maxActive {
		active = active[:maxActive]
	}

	return append(active, dimmed...), nil
}

// TeleportClaudeSession imports an existing terminal Claude Code session into
// Polaris as an idle claude-code agent, ready for follow-up messages.
func (service *Service) TeleportClaudeSession(projectID, sessionID string) (*Agent, error) {
	if service.store == nil {
		return nil, errors.New("store not initialised")
	}
	project, err := service.store.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("find project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project %q not found", projectID)
	}

	dir := claudeProjectDir(project.Path)
	jsonlPath := filepath.Join(dir, sessionID+".jsonl")
	if _, err := os.Stat(jsonlPath); err != nil {
		return nil, fmt.Errorf("session %q not found: %w", sessionID, err)
	}

	summary, model, _ := parseClaudeJSONL(jsonlPath)
	if summary == "" && len(sessionID) >= 8 {
		summary = sessionID[:8]
	}

	agent, err := service.store.UpsertAgent(Agent{
		ProjectID: projectID,
		Kind:      "claude-code",
		Summary:   summary,
		Status:    "idle",
		StartedAt: time.Now().Unix(),
		SessionID: sessionID,
		Source:    "manual",
		Model:     model,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	_ = service.populateTeleportLog(agent.ID, jsonlPath)
	return &agent, nil
}

// populateTeleportLog reads a Claude Code JSONL session file and writes its
// conversation history into the agent's Polaris log file so the user can see
// the prior exchange immediately after teleporting.
func (service *Service) populateTeleportLog(agentID, jsonlPath string) error {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return err
	}
	defer f.Close()

	toolInputs := make(map[string]toolInputSnapshot)
	var stats streamTurnStats

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		kind, _ := raw["type"].(string)
		ts := historyTimestamp(raw["timestamp"])

		emit := func(evt StreamEvent) {
			evt.Ts = ts
			_ = service.appendAgentEvent(agentID, evt)
		}
		switch kind {
		case "user":
			msg, _ := raw["message"].(map[string]any)
			if msg == nil {
				continue
			}
			switch c := msg["content"].(type) {
			case string:
				if c = strings.TrimSpace(c); c != "" && !strings.HasPrefix(c, "<ide_opened_file>") {
					emit(StreamEvent{Type: "user_message", Content: c})
				}
			case []any:
				isToolResult := false
				for _, item := range c {
					if b, ok := item.(map[string]any); ok {
						if t, _ := b["type"].(string); t == "tool_result" {
							isToolResult = true
							break
						}
					}
				}
				if isToolResult {
					for _, se := range renderClaudeUserToolResults(raw, toolInputs) {
						emit(se)
					}
				} else {
					var parts []string
					for _, item := range c {
						if b, ok := item.(map[string]any); ok {
							if t, _ := b["type"].(string); t == "text" {
								if text, _ := b["text"].(string); text != "" && !strings.HasPrefix(text, "<ide_opened_file>") {
									parts = append(parts, text)
								}
							}
						}
					}
					if text := strings.TrimSpace(strings.Join(parts, "\n")); text != "" {
						emit(StreamEvent{Type: "user_message", Content: text})
					}
				}
			}
		case "assistant":
			files := make(map[string]struct{})
			for _, se := range renderClaudeAssistant(raw, files, toolInputs, nil, &stats) {
				emit(se)
			}
		}
	}
	return scanner.Err()
}

func historyTimestamp(v any) string {
	s, _ := v.(string)
	if s == "" {
		return time.Now().Format("15:04:05")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("15:04:05")
		}
	}
	return time.Now().Format("15:04:05")
}

// parseClaudeJSONL reads a Claude Code JSONL conversation file and returns
// the first human message as a summary, the model used, and the total user
// message count.
func parseClaudeJSONL(path string) (summary, model string, count int) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Model   string          `json:"model"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		if model == "" && entry.Type == "assistant" && entry.Message.Model != "" {
			model = entry.Message.Model
		}
		if entry.Type != "user" && entry.Message.Role != "user" {
			continue
		}
		count++
		if summary != "" || len(entry.Message.Content) == 0 {
			continue
		}
		var text string
		if json.Unmarshal(entry.Message.Content, &text) == nil && text != "" {
			if !strings.HasPrefix(text, "<ide_opened_file>") {
				summary = clipSummary(text)
			}
			continue
		}
		var parts []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			ToolUseID string `json:"tool_use_id"`
		}
		if json.Unmarshal(entry.Message.Content, &parts) == nil {
			for _, p := range parts {
				if p.Type == "tool_result" || p.ToolUseID != "" {
					break
				}
				if p.Type == "text" && p.Text != "" && !strings.HasPrefix(p.Text, "<ide_opened_file>") {
					summary = clipSummary(p.Text)
					break
				}
			}
		}
	}
	return summary, model, count
}

func clipSummary(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = s[:idx]
	}
	const max = 120
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
