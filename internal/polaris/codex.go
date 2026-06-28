package polaris

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func codexSpawnCommand(binary, model, task string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "codex"
	}
	args := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if task != "" {
		args = append(args, "--")
	}
	args = append(args, task)
	return bin, args, nil
}

func codexResumeCommand(binary, sessionID, model, message string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "codex"
	}
	args := []string{"exec", "resume", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, sessionID)
	if message != "" {
		args = append(args, "--", message)
	}
	return bin, args, nil
}

// --- codex image-tool log merge (moved from runner.go) ---
func (service *Service) mergeMissingCodexImageToolEvents(agentID string, logPath string, since time.Time, codexThreadID string) int {
	lines, existing := readLogLinesAndToolKeys(logPath)
	events := recentCodexImageToolEvents(since, codexThreadID)
	missing := make([]StreamEvent, 0, len(events))
	for _, evt := range events {
		if evt.ID != "" {
			key := evt.Type + ":" + evt.ID
			if _, ok := existing[key]; ok {
				continue
			}
			existing[key] = struct{}{}
		}
		missing = append(missing, evt)
	}
	if len(missing) == 0 {
		return 0
	}

	merged := mergeEventsIntoLogLines(lines, missing)
	if err := os.WriteFile(logPath, []byte(strings.Join(merged, "\n")+"\n"), 0o644); err != nil {
		return 0
	}
	service.emitLogResetEvent(agentID)

	added := 0
	for _, evt := range missing {
		if evt.Type == "tool_call" {
			added++
		}
	}
	return added
}

func readLogLinesAndToolKeys(logPath string) ([]string, map[string]struct{}) {
	ids := make(map[string]struct{})
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, ids
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for _, line := range lines {
		var evt StreamEvent
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &evt) != nil {
			continue
		}
		if evt.ID != "" && (evt.Type == "tool_call" || evt.Type == "tool_result") {
			ids[evt.Type+":"+evt.ID] = struct{}{}
		}
	}
	return lines, ids
}

func mergeEventsIntoLogLines(lines []string, events []StreamEvent) []string {
	if len(lines) == 0 {
		out := make([]string, 0, len(events))
		for _, evt := range events {
			out = append(out, marshalEvent(evt))
		}
		return out
	}
	out := make([]string, 0, len(lines)+len(events))
	idx := 0
	for _, line := range lines {
		lineTs := logLineTimestamp(line)
		for idx < len(events) && shouldInsertBefore(events[idx].Ts, lineTs) {
			out = append(out, marshalEvent(events[idx]))
			idx++
		}
		out = append(out, line)
	}
	for idx < len(events) {
		out = append(out, marshalEvent(events[idx]))
		idx++
	}
	return out
}

func logLineTimestamp(line string) string {
	var evt StreamEvent
	if json.Unmarshal([]byte(strings.TrimSpace(line)), &evt) == nil {
		return evt.Ts
	}
	if m := legacyTimestampRe.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

func shouldInsertBefore(eventTs, lineTs string) bool {
	if eventTs == "" || lineTs == "" {
		return false
	}
	return eventTs <= lineTs
}

type timedStreamEvent struct {
	at    time.Time
	order int
	evt   StreamEvent
}

func recentCodexImageToolEvents(since time.Time, codexThreadID string) []StreamEvent {
	if strings.TrimSpace(codexThreadID) == "" {
		return nil
	}
	root := codexSessionsRoot()
	if root == "" {
		return nil
	}
	cutoff := since.Add(-30 * time.Second)
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		if !codexSessionFileMatches(path, codexThreadID) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if len(files) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var out []timedStreamEvent
	for _, path := range files {
		for _, tev := range codexImageToolEventsFromFile(path, cutoff) {
			key := tev.evt.Type + ":" + tev.evt.ID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			tev.order = len(out)
			out = append(out, tev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].at.Equal(out[j].at) {
			return out[i].order < out[j].order
		}
		return out[i].at.Before(out[j].at)
	})
	events := make([]StreamEvent, 0, len(out))
	for _, tev := range out {
		events = append(events, tev.evt)
	}
	return events
}

func codexSessionsRoot() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

func codexSessionFileMatches(path, codexThreadID string) bool {
	codexThreadID = strings.TrimSpace(codexThreadID)
	if codexThreadID == "" {
		return false
	}
	if strings.Contains(filepath.Base(path), codexThreadID) {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		if entry.Type != "session_meta" {
			continue
		}
		var payload struct {
			ID string `json:"id"`
		}
		return json.Unmarshal(entry.Payload, &payload) == nil && payload.ID == codexThreadID
	}
	return false
}

func codexImageToolEventsFromFile(path string, cutoff time.Time) []timedStreamEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	var out []timedStreamEvent
	imageCalls := make(map[string]time.Time)
	for scanner.Scan() {
		var entry struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "response_item" {
			continue
		}
		at, ok := parseCodexSessionTime(entry.Timestamp)
		if !ok || at.Before(cutoff) {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(entry.Payload, &payload) != nil {
			continue
		}
		switch strAny(payload["type"]) {
		case "function_call", "custom_tool_call":
			if strAny(payload["name"]) != "view_image" {
				continue
			}
			evt, ok := codexFunctionCall(payload)
			if !ok {
				continue
			}
			evt.Ts = at.Local().Format("15:04:05")
			imageCalls[evt.ID] = at
			out = append(out, timedStreamEvent{at: at, evt: evt})
		case "function_call_output", "custom_tool_call_output":
			id := codexItemID(payload)
			callAt, ok := imageCalls[id]
			if !ok {
				continue
			}
			evt := codexFunctionResult(payload)
			evt.Ts = callAt.Local().Format("15:04:05")
			out = append(out, timedStreamEvent{at: callAt.Add(time.Millisecond), evt: evt})
		}
	}
	return out
}

func parseCodexSessionTime(raw string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
