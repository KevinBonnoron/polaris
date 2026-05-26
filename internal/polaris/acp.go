package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

// Several agents are driven over the Agent Client Protocol (ACP): a long-lived
// subprocess speaking newline-delimited JSON-RPC 2.0 on stdin/stdout — no
// network port. opencode runs as `opencode acp`, Mistral as `vibe-acp`. One
// session per agent; follow-up turns are new session/prompt calls on the same
// process. Tool permissions arrive as server->client session/request_permission
// requests and surface through the existing AskUserQuestion panel.
//
// This file holds the transport and rendering that every ACP backend shares.
// Backend-specific wiring (launch command, env/config injection) lives in the
// per-agent files (opencode.go, mistral.go) and is selected by acpRuntime.

// acpRuntime describes how to launch a specific ACP backend and how to label it
// in error messages. opencode needs the `acp` subcommand; vibe-acp is launched
// directly with no args.
type acpRuntime struct {
	binary string
	args   []string
	label  string
	// mode, when set, is an ACP session mode applied after session creation. For
	// mistral this is "auto-approve" — vibe exposes its agent profiles as session
	// modes, and that profile's bypass_tool_permissions runs tools unattended
	// (the equivalent of claude's bypassPermissions and opencode's allow config).
	mode string
}

// acpRuntime resolves the launch command for an ACP-based agent kind. binary,
// when non-empty, is the already-resolved path from detection; otherwise the
// kind's binary is looked up via the service resolver.
func (service *Service) acpRuntime(kind, binary string) (acpRuntime, error) {
	resolve := func(detectKind, fallback string) string {
		if binary != "" {
			return binary
		}
		if service.resolveBinary != nil {
			if r := service.resolveBinary(detectKind); r != "" {
				return r
			}
		}
		return fallback
	}
	switch kind {
	case "opencode":
		return acpRuntime{binary: resolve("opencode", "opencode"), args: []string{"acp"}, label: "opencode"}, nil
	case "mistral":
		return acpRuntime{binary: resolve("mistral", "vibe-acp"), label: "mistral", mode: "auto-approve"}, nil
	default:
		return acpRuntime{}, fmt.Errorf("agent kind %q is not ACP-based", kind)
	}
}

type acpSession struct {
	svc       *Service
	agentID   string
	label     string
	mode      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	sessionID string

	mu      sync.Mutex
	writeMu sync.Mutex
	nextID  int
	pending map[int]chan acpMessage
	closed  bool
	running bool
	// loading is true while a session/load replays the prior transcript; the
	// replayed session/update notifications are skipped so the existing log
	// file isn't duplicated.
	loading bool

	// perms maps a question toolUseID to the channel that delivers the chosen
	// optionId (empty = cancelled) and to the originating JSON-RPC request id.
	perms     map[string]chan string
	permReqID map[string]int

	// decided records permission/plan answers the user already gave, keyed by the
	// backend's stable tool-call id (survives a session reload, unlike the
	// perm-<n> id). On resume an ACP backend re-requests permission for tools it
	// already asked about; we replay the recorded answer instead of re-prompting,
	// so an accepted/refused plan is never proposed twice. Loaded from the store
	// at session start; the value is the chosen option label ("" = cancelled).
	decided map[string]string

	// streaming accumulation (chunks arrive token by token)
	curMsg     strings.Builder
	curThought strings.Builder
	tools      map[string]*acpToolState

	baseTokens int
	baseCost   float64
}

type acpMessage struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// startACPSession spawns the backend described by rt, performs the handshake +
// session/new (or session/load when resumeSessionID is set), then runs the first
// turn. It is called from Spawn for ACP-based agents, and from Send to resume a
// finished session.
func (runner *Runner) startACPSession(svc *Service, agentID string, rt acpRuntime, workDir, task string, env []string, baseTokens int, baseCost float64, resumeSessionID string) error {
	cmd := exec.Command(rt.binary, rt.args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = env
	sysexec.Hide(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	decided, _ := svc.store.ListAgentDecisions(agentID)
	if decided == nil {
		decided = map[string]string{}
	}
	a := &acpSession{
		svc:        svc,
		agentID:    agentID,
		label:      rt.label,
		mode:       rt.mode,
		cmd:        cmd,
		stdin:      stdin,
		pending:    map[int]chan acpMessage{},
		perms:      map[string]chan string{},
		permReqID:  map[string]int{},
		decided:    decided,
		baseTokens: baseTokens,
		baseCost:   baseCost,
	}

	runner.mu.Lock()
	runner.acp[agentID] = a
	runner.mu.Unlock()
	_ = svc.store.PatchAgent(agentID, map[string]any{"pid": cmd.Process.Pid})

	go a.readLoop(stdout)
	go func() {
		if err := a.bootstrap(workDir, resumeSessionID); err != nil {
			a.svc.markAgentError(agentID, fmt.Sprintf("%s acp: %v", a.label, err))
			a.shutdown(runner)
			return
		}
		a.runTurn(runner, task)
	}()
	return nil
}

func (a *acpSession) bootstrap(workDir, resumeSessionID string) error {
	initRes, err := a.call("initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": false, "writeTextFile": false}},
		"clientInfo":         map[string]any{"name": "polaris", "version": "1"},
	})
	if err != nil {
		return err
	}

	if resumeSessionID != "" {
		var caps struct {
			AgentCapabilities struct {
				LoadSession bool `json:"loadSession"`
			} `json:"agentCapabilities"`
		}
		_ = json.Unmarshal(initRes, &caps)
		if !caps.AgentCapabilities.LoadSession {
			return fmt.Errorf("%s build does not support resuming sessions", a.label)
		}
		// session/load replays the prior transcript as session/update events; mute
		// them so we don't re-append the existing log, then resume the live turn.
		a.mu.Lock()
		a.loading = true
		a.mu.Unlock()
		_, err := a.call("session/load", map[string]any{"sessionId": resumeSessionID, "cwd": workDir, "mcpServers": []any{}})
		a.mu.Lock()
		a.loading = false
		a.mu.Unlock()
		if err != nil {
			return err
		}
		a.sessionID = resumeSessionID
		a.applyMode()
		return nil
	}

	res, err := a.call("session/new", map[string]any{"cwd": workDir, "mcpServers": []any{}})
	if err != nil {
		return err
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return err
	}
	if out.SessionID == "" {
		return fmt.Errorf("no session id returned")
	}
	a.sessionID = out.SessionID
	_ = a.svc.store.PatchAgent(a.agentID, map[string]any{"sessionId": out.SessionID})
	a.applyMode()
	return nil
}

// applyMode selects the configured ACP session mode (e.g. mistral's
// "auto-approve") once a session exists. Best-effort: a backend that doesn't
// support the mode just keeps prompting.
func (a *acpSession) applyMode() {
	if a.mode == "" {
		return
	}
	_, _ = a.call("session/set_mode", map[string]any{"sessionId": a.sessionID, "modeId": a.mode})
}

// runTurn sends a prompt and blocks until the turn ends, then drains any queued
// follow-up or marks the agent completed.
func (a *acpSession) runTurn(r *Runner, text string) {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	res, err := a.call("session/prompt", map[string]any{
		"sessionId": a.sessionID,
		"prompt":    []any{map[string]any{"type": "text", "text": escapeLeadingSlash(text)}},
	})
	a.flush()

	a.mu.Lock()
	a.running = false
	a.mu.Unlock()

	if err == nil {
		var pr struct {
			Usage struct {
				TotalTokens  int `json:"totalTokens"`
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(res, &pr) == nil && pr.Usage.TotalTokens > 0 {
			a.baseTokens += pr.Usage.TotalTokens
			parts := usageParts{Input: pr.Usage.InputTokens, Output: pr.Usage.OutputTokens}
			a.svc.persistTurnStats(a.agentID, streamTurnStats{Tokens: pr.Usage.TotalTokens, Parts: parts})
			a.svc.emitTokens(a.agentID, a.baseTokens, parts, a.baseCost)
		}
	}

	if err != nil {
		if !a.isClosed() {
			a.emitEvent(StreamEvent{Type: "system", Content: "✗ " + err.Error()})
			_ = a.svc.store.PatchAgent(a.agentID, map[string]any{"status": "error"})
			a.svc.notifyAgentEvent(a.agentID, "error", err.Error())
		}
		return
	}

	if next, has := r.popPending(a.agentID); has {
		a.runTurn(r, next)
		return
	}
	a.svc.markAgentCompleted(a.agentID)
}

// prompt is the entry for a follow-up message; queued if a turn is in flight.
func (a *acpSession) prompt(r *Runner, text string) {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		r.queueIfRunning(a.agentID, text)
		return
	}
	go a.runTurn(r, text)
}

func (a *acpSession) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m acpMessage
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		switch {
		case m.ID != nil && m.Method == "":
			a.deliverResponse(*m.ID, m)
		case m.Method == "session/update":
			a.handleUpdate(m.Params)
		case m.Method != "" && m.ID != nil:
			a.handleServerRequest(*m.ID, m.Method, m.Params)
		}
	}
	// Scanner stopped: clean EOF or a read error (oversized line, broken pipe).
	// Both mean the connection is gone; surface the reason to in-flight calls.
	reason := a.label + " acp connection closed"
	if err := sc.Err(); err != nil {
		reason = a.label + " acp read error: " + err.Error()
	}

	failure, _ := json.Marshal(reason)
	a.mu.Lock()
	a.closed = true
	for id, ch := range a.pending {
		ch <- acpMessage{Error: failure}
		delete(a.pending, id)
	}

	a.mu.Unlock()
}

func (a *acpSession) deliverResponse(id int, m acpMessage) {
	a.mu.Lock()
	ch := a.pending[id]
	delete(a.pending, id)
	a.mu.Unlock()
	if ch != nil {
		ch <- m
	}
}

func (a *acpSession) call(method string, params any) (json.RawMessage, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, fmt.Errorf("connection closed")
	}
	a.nextID++
	id := a.nextID
	ch := make(chan acpMessage, 1)
	a.pending[id] = ch
	a.mu.Unlock()

	if err := a.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}

	msg := <-ch
	if msg.Error != nil {
		return nil, fmt.Errorf("%s", string(msg.Error))
	}

	return msg.Result, nil
}

func (a *acpSession) reply(id int, result any) {
	_ = a.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (a *acpSession) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_, err = a.stdin.Write(append(b, '\n'))
	return err
}

func (a *acpSession) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// handleUpdate renders session/update notifications into the shared log format.
func (a *acpSession) handleUpdate(params json.RawMessage) {
	a.mu.Lock()
	loading := a.loading
	a.mu.Unlock()
	if loading {
		return
	}
	var p struct {
		Update json.RawMessage `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	var head struct {
		Kind string `json:"sessionUpdate"`
	}
	if json.Unmarshal(p.Update, &head) != nil {
		return
	}

	switch head.Kind {
	case "agent_thought_chunk":
		a.appendThought(acpChunkText(p.Update))
	case "agent_message_chunk":
		a.flushThought()
		a.appendMsg(acpChunkText(p.Update))
	case "tool_call", "tool_call_update":
		a.handleToolUpdate(p.Update)
	}
}

func acpChunkText(update json.RawMessage) string {
	var u struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(update, &u)
	return u.Content.Text
}

func (a *acpSession) handleToolUpdate(update json.RawMessage) {
	// Text precedes the tool in the transcript.
	a.flush()
	var u struct {
		ToolCallID string          `json:"toolCallId"`
		Title      string          `json:"title"`
		Kind       string          `json:"kind"`
		Status     string          `json:"status"`
		RawInput   json.RawMessage `json:"rawInput"`
		Content    []struct {
			Type    string `json:"type"`
			OldText string `json:"oldText"`
			NewText string `json:"newText"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
		RawOutput json.RawMessage `json:"rawOutput"`
		Meta      struct {
			ToolName string `json:"tool_name"`
		} `json:"_meta"`
	}
	if json.Unmarshal(update, &u) != nil {
		return
	}

	// Tool id/name and input arrive on the initial tool_call; the result arrives
	// on a later tool_call_update that may omit them. Merge what each event
	// carries so the call line still renders with its detail at completion.
	// opencode names the tool in `title` (a short id like "read"); vibe-acp puts
	// the canonical id in `_meta.tool_name` and uses `title` for a human summary.
	name := ""
	switch {
	case u.Meta.ToolName != "":
		name = acpToolName(u.Meta.ToolName)
	case u.Title != "":
		name = acpToolName(u.Title)
	case u.Kind != "":
		name = acpToolName(u.Kind)
	}
	decoded := acpDecodeInput(u.RawInput)
	a.mergeTool(u.ToolCallID, name, acpToolDetail(decoded), decoded)

	switch u.Status {
	case "pending", "":
		// initial announcement; wait for the result before emitting.
	case "in_progress":
		a.emitToolCall(u.ToolCallID)
	case "completed", "failed":
		a.emitToolCall(u.ToolCallID)
		var content strings.Builder
		hasDiff := false
		for _, c := range u.Content {
			if c.Type == "diff" {
				hasDiff = true
				content.WriteString(acpDiffLines(c.OldText, c.NewText))
			} else {
				content.WriteString(c.Content.Text)
			}
		}
		var out string
		switch {
		case hasDiff:
			out = content.String()
		case u.Meta.ToolName != "":
			out = acpVibeResultText(u.RawOutput, content.String())
			if diff := acpSearchReplaceToDiff(out); diff != "" {
				out = diff
			}
		default:
			out = acpRawOutputText(u.RawOutput)
			if out == "" {
				out = content.String()
			}
		}
		if out = strings.TrimSpace(out); out != "" {
			a.emitToolResult(u.ToolCallID, capLines(out, 200), "", u.Status == "failed")
		}
	}
}

func (a *acpSession) handleServerRequest(id int, method string, params json.RawMessage) {
	if method != "session/request_permission" {
		a.reply(id, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		return
	}
	a.flush()
	var p struct {
		ToolCall struct {
			ToolCallID string          `json:"toolCallId"`
			Title      string          `json:"title"`
			RawInput   json.RawMessage `json:"rawInput"`
			Meta       struct {
				ToolName string `json:"tool_name"`
			} `json:"_meta"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if json.Unmarshal(params, &p) != nil {
		a.reply(id, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		return
	}

	toolUseID := fmt.Sprintf("perm-%d", id)
	labelToOption := map[string]string{}
	options := make([]map[string]any, 0, len(p.Options))
	for _, o := range p.Options {
		labelToOption[o.Name] = o.OptionID
		options = append(options, map[string]any{"label": o.Name})
	}

	// Already answered (this session or a prior one): replay the recorded
	// decision instead of re-surfacing the prompt. This is what stops an
	// accepted/refused/cancelled plan from being proposed again after a
	// session/load resume re-requests permission for the same tool call.
	toolCallID := p.ToolCall.ToolCallID
	if toolCallID != "" {
		a.mu.Lock()
		prev, ok := a.decided[toolCallID]
		a.mu.Unlock()
		if ok {
			a.replyPermissionOutcome(id, prev, labelToOption)
			return
		}
	}

	name := acpToolName(p.ToolCall.Title)
	if p.ToolCall.Meta.ToolName != "" {
		name = acpToolName(p.ToolCall.Meta.ToolName)
	}
	detail := acpToolDetail(acpDecodeInput(p.ToolCall.RawInput))
	// vibe's request_permission carries only the toolCallId, so the request alone
	// says nothing about the tool. Enrich from the tool_call we already tracked.
	if sName, sDetail := a.toolInfo(p.ToolCall.ToolCallID); sName != "" {
		if name == "" || name == "Tool" {
			name = sName
		}
		if detail == "" {
			detail = sDetail
		}
	}
	if name == "" {
		name = "Tool"
	}
	question := fmt.Sprintf("Allow %s?", name)
	if detail != "" {
		question = fmt.Sprintf("Allow %s · %s?", name, detail)
	}
	input := map[string]any{
		"questions": []map[string]any{{
			"header":   "Permission",
			"question": question,
			"options":  options,
		}},
	}

	ch := make(chan string, 1)
	a.mu.Lock()
	a.perms[toolUseID] = ch
	a.permReqID[toolUseID] = id
	a.mu.Unlock()

	a.svc.emitAskUserQuestion(a.agentID, toolUseID, input)

	go func() {
		label := <-ch
		a.mu.Lock()
		delete(a.perms, toolUseID)
		delete(a.permReqID, toolUseID)
		if toolCallID != "" {
			a.decided[toolCallID] = label
		}
		a.mu.Unlock()
		if toolCallID != "" {
			_ = a.svc.store.RecordAgentDecision(a.agentID, toolCallID, label)
		}
		a.replyPermissionOutcome(id, label, labelToOption)
	}()
}

// replyPermissionOutcome answers a session/request_permission with the chosen
// option label, mapping it to the request's optionId. An empty or unknown label
// is delivered as a cancellation.
func (a *acpSession) replyPermissionOutcome(id int, label string, labelToOption map[string]string) {
	optID := labelToOption[label]
	if optID == "" {
		a.reply(id, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		return
	}
	a.reply(id, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optID}})
}

// summarizeAnswer renders an AskUserQuestion answer payload
// (`[{"question":...,"answer":...}]`) as a short one-line label for the log,
// joining each question's answer. Non-JSON content (e.g. a dismissal message)
// is returned verbatim.
func summarizeAnswer(content string) string {
	var arr []struct {
		Answer json.RawMessage `json:"answer"`
	}
	if json.Unmarshal([]byte(content), &arr) != nil || len(arr) == 0 {
		return content
	}
	labels := make([]string, 0, len(arr))
	for _, a := range arr {
		var s string
		if json.Unmarshal(a.Answer, &s) == nil {
			labels = append(labels, s)
			continue
		}
		var ss []string
		if json.Unmarshal(a.Answer, &ss) == nil && len(ss) > 0 {
			labels = append(labels, strings.Join(ss, ", "))
		}
	}
	if len(labels) == 0 {
		return content
	}
	return strings.Join(labels, " · ")
}

// parseAUQAnswerLabel extracts the chosen option label from an AskUserQuestion
// answer payload (`[{"question":...,"answer":"<label>"}]`).
func parseAUQAnswerLabel(content string) string {
	var arr []struct {
		Answer json.RawMessage `json:"answer"`
	}
	if json.Unmarshal([]byte(content), &arr) != nil || len(arr) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(arr[0].Answer, &s) == nil {
		return s
	}
	var ss []string
	if json.Unmarshal(arr[0].Answer, &ss) == nil && len(ss) > 0 {
		return ss[0]
	}
	return ""
}

// answerPermission delivers the user's choice (an option label) to a pending
// permission request. Returns false when no such request is pending.
func (a *acpSession) answerPermission(toolUseID, label string) bool {
	a.mu.Lock()
	ch := a.perms[toolUseID]
	a.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- label
	return true
}

func (a *acpSession) cancel() {
	if a.sessionID != "" {
		_ = a.write(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": a.sessionID}})
	}
}

func (a *acpSession) shutdown(r *Runner) {
	r.mu.Lock()
	delete(r.acp, a.agentID)
	r.mu.Unlock()
	_ = a.stdin.Close()
	if a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
	if a.svc != nil && a.svc.store != nil {
		_ = a.svc.store.PatchAgent(a.agentID, map[string]any{"pid": 0})
	}
}

// --- tool-name tracking + emit (guarded by mu) ---

type acpToolState struct {
	name    string
	detail  string
	input   map[string]any
	emitted bool
}

func (a *acpSession) toolMap() map[string]*acpToolState {
	if a.tools == nil {
		a.tools = map[string]*acpToolState{}
	}
	return a.tools
}

// mergeTool records the tool's name/detail/input, keeping whatever a prior
// event already supplied (later updates may omit them).
func (a *acpSession) mergeTool(id, name, detail string, input map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := a.toolMap()[id]
	if st == nil {
		st = &acpToolState{}
		a.toolMap()[id] = st
	}
	if name != "" {
		st.name = name
	}
	if detail != "" {
		st.detail = detail
	}
	if input != nil && st.input == nil {
		st.input = input
	}
}

// toolInfo returns the tracked name/detail for a tool call (populated from the
// tool_call event), used to enrich permission prompts that only carry an id.
func (a *acpSession) toolInfo(id string) (name, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st := a.toolMap()[id]; st != nil {
		return st.name, st.detail
	}
	return "", ""
}

// --- shared tool rendering (ACP backends emit the same standard tool ids) ---

// acpToolNames maps the tool ids ACP agents emit to the capitalised names the
// log renderer special-cases (diff view for Edit, todo panel for TodoWrite,
// line-number stripping for Read/Grep). opencode sends short lowercase ids
// (read, edit); vibe-acp sends them in `_meta.tool_name` (read_file, search_replace).
var acpToolNames = map[string]string{
	"read": "Read", "write": "Write", "edit": "Edit", "grep": "Grep",
	"glob": "Glob", "bash": "Bash", "webfetch": "WebFetch",
	"todowrite": "TodoWrite", "task": "Task", "list": "List", "patch": "Patch",
	"read_file": "Read", "write_file": "Write", "search_replace": "Edit",
	"edit_file": "Edit", "list_files": "List", "web_fetch": "WebFetch",
	"web_search": "WebSearch",
}

func acpToolName(tool string) string {
	if n, ok := acpToolNames[tool]; ok {
		return n
	}
	if tool == "" {
		return "Tool"
	}
	return strings.ToUpper(tool[:1]) + tool[1:]
}

// acpDecodeInput normalises a tool's rawInput into a map. opencode sends it as a
// JSON object; vibe-acp sends it as a JSON-encoded string (e.g. "{\"path\":...}").
func acpDecodeInput(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		return m
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

// acpRawOutputText extracts text from a tool's rawOutput. opencode sends an
// object {"output": "..."}; vibe-acp sends a plain (JSON-encoded) string.
func acpRawOutputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	var o struct {
		Output string `json:"output"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Output
	}

	return ""
}

// acpResultPayloadKeys are the result-model fields vibe-acp tools carry the
// meaningful textual output under (read_file/search_replace→content, grep→
// matches, bash→stdout/stderr), checked in order.
var acpResultPayloadKeys = []string{"content", "matches", "stdout", "stderr", "output", "text", "result", "message"}

// searchReplaceBlockRE matches the aider-style SEARCH/REPLACE edit blocks vibe
// emits (5+ markers, like vibe's own SEARCH_REPLACE_BLOCK_RE). Group 1 is the
// original text, group 2 the replacement.
var searchReplaceBlockRE = regexp.MustCompile(`(?s)<{5,} SEARCH\r?\n(.*?)\r?\n?={5,}\r?\n(.*?)\r?\n?>{5,} REPLACE`)

// acpSearchReplaceToDiff converts SEARCH/REPLACE blocks (which vibe-acp puts in
// an edit tool's result when it doesn't send structured diff content) into a
// -/+ diff. Returns "" when the text holds no such block.
func acpSearchReplaceToDiff(text string) string {
	matches := searchReplaceBlockRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder
	for i, m := range matches {
		if i > 0 {
			b.WriteString("\n")
		}

		b.WriteString(acpDiffLines(m[1], m[2]))
	}

	return b.String()
}

// acpDiffLines renders an ACP file-edit diff block (old_text → new_text) as
// removed/added lines prefixed with `- `/`+ `, which the log panel colourises.
func acpDiffLines(oldText, newText string) string {
	var b strings.Builder
	write := func(prefix, text string) {
		for _, ln := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			b.WriteString(prefix)
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}

	if strings.TrimSpace(oldText) != "" {
		write("- ", oldText)
	}

	if strings.TrimSpace(newText) != "" {
		write("+ ", newText)
	}

	return b.String()
}

// acpVibeResultText pulls the human-meaningful output from vibe-acp's rawOutput,
// which is a tool's result model serialised as JSON (e.g. read_file's file text
// lives under `content`). It falls back to the display recap, then the raw JSON.
func acpVibeResultText(raw json.RawMessage, recap string) string {
	s := acpRawOutputText(raw)
	if s == "" {
		return recap
	}
	var obj map[string]any
	if json.Unmarshal([]byte(s), &obj) == nil {
		for _, k := range acpResultPayloadKeys {
			if v, ok := obj[k].(string); ok && strings.TrimSpace(v) != "" {
				return v
			}
		}
		if strings.TrimSpace(recap) != "" {
			return recap
		}
	}
	return s
}

// acpToolDetail builds the descriptor shown next to a tool call. Only one
// line of input is shown (like claude), so it picks the distinguishing param:
// for glob/grep that's `pattern` (with the `path` scope appended), not the
// directory — otherwise two different globs in the same dir look identical.
func acpToolDetail(input map[string]any) string {
	str := func(k string) string {
		s, _ := input[k].(string)
		return strings.TrimSpace(s)
	}
	if p := str("pattern"); p != "" {
		if dir := str("path"); dir != "" {
			return p + " in " + dir
		}

		return p
	}

	for _, k := range []string{"filePath", "file_path", "file", "command", "url", "query", "path", "description"} {
		if v := str(k); v != "" {
			return v
		}
	}

	return ""
}

func capLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}

	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-max)
}
