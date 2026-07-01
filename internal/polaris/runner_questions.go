package polaris

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// agentTokensEvent carries a live, mid-turn token/cost snapshot so cards and
// the detail modal can update before the end-of-turn result is persisted. It is
// intentionally not written to the DB — persistTurnStats records the
// authoritative figure when the turn finishes.
const agentTokensEvent = "agent:tokens:updated"

// AgentTokensEventName returns the event the frontend should subscribe to for
// live token updates.
func AgentTokensEventName() string { return agentTokensEvent }

type tokenSnapshot struct {
	tokens  int
	parts   usageParts
	costUSD float64
}

// tokenEmitDebounce caps how often a live-token snapshot reaches the frontend.
// The stream parser pushes a new total on every assistant message; un-throttled,
// each one woke every agent list item's token handler. Mirrors logEmitDebounce.
const tokenEmitDebounce = 100 * time.Millisecond

// emitTokens pushes the running token/cost total to the frontend, throttled to at
// most one emit per agent per debounce window while always carrying the latest
// snapshot. The authoritative end-of-turn figure is persisted separately, so a
// dropped intermediate snapshot is never the final value.
func (service *Service) emitTokens(agentID string, tokens int, parts usageParts, costUSD float64) {
	if service.store == nil {
		return
	}
	service.tokenEmitMu.Lock()
	defer service.tokenEmitMu.Unlock()
	if service.tokenEmitLatest == nil {
		service.tokenEmitLatest = make(map[string]tokenSnapshot)
	}
	service.tokenEmitLatest[agentID] = tokenSnapshot{tokens: tokens, parts: parts, costUSD: costUSD}
	if service.tokenEmitTimer == nil {
		service.tokenEmitTimer = make(map[string]*time.Timer)
	}
	if _, scheduled := service.tokenEmitTimer[agentID]; scheduled {
		return
	}
	service.tokenEmitTimer[agentID] = time.AfterFunc(tokenEmitDebounce, func() {
		service.tokenEmitMu.Lock()
		snap := service.tokenEmitLatest[agentID]
		delete(service.tokenEmitTimer, agentID)
		delete(service.tokenEmitLatest, agentID)
		service.tokenEmitMu.Unlock()
		service.store.Emit(agentTokensEvent, map[string]any{
			"agentId": agentID,
			"tokens":  snap.tokens,
			"costUsd": snap.costUSD,
			"parts":   snap.parts,
		})
	})
}

// askUserQuestionEvent is fired when the agent calls the AskUserQuestion tool.
// The frontend renders a dialog, then replies via RespondToAgentQuestion to
// deliver the answer as a proper tool_result on the agent's stdin.
const askUserQuestionEvent = "agent:ask-user-question"

// AskUserQuestionEventName returns the event the frontend should subscribe to.
func AskUserQuestionEventName() string { return askUserQuestionEvent }

func (service *Service) emitAskUserQuestion(agentID, toolUseID string, input map[string]any) {
	if service.store == nil {
		return
	}
	// Surface only the first question/plan of a turn, then stop the subprocess.
	// claude-code in --print mode can't get an interactive answer mid-turn, so
	// left running it re-emits the same ExitPlanMode / AskUserQuestion in an
	// infinite loop that burns tokens. beginAwait returns false for those
	// duplicates so we ignore them; the user answers the surfaced one via a
	// resume turn.
	if service.runner != nil && !service.runner.beginAwait(agentID) {
		return
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		inputJSON = []byte("{}")
	}
	// Persist before emitting so a viewer that opens the modal later (or
	// after an app restart) can still see the choices instead of finding
	// only a failed agent with no context. Flip to "waiting" so cards show
	// the amber dot — the subprocess is alive but blocked on user input.
	_ = service.store.PatchAgent(agentID, map[string]any{
		"status":          "waiting",
		"pendingQuestion": &PendingQuestion{ToolUseID: toolUseID, Input: inputJSON},
	})
	service.store.Emit(askUserQuestionEvent, map[string]any{
		"agentId":   agentID,
		"toolUseId": toolUseID,
		"input":     input,
	})
	service.notifyAgentEvent(agentID, "waiting", "")
	// Stop the subprocess now that the question is persisted and surfaced; the
	// answer is delivered later on a resumed session.
	if service.runner != nil {
		service.runner.stopForAwait(agentID)
	}
}

// RespondToAgentQuestion delivers the user's answer to a pending
// AskUserQuestion / ExitPlanMode tool call. The persisted question is dropped
// first and unconditionally so the panel never reappears. The answer is then
// delivered as a tool_result on the live subprocess's stdin when it's still
// alive; otherwise (the common claude-code case: it exits in --print mode after
// emitting the tool_use) it is delivered as the next user message on a resumed
// session — claude-code rejects a tool_result on resume (the API treats it as an
// assistant-message prefill).
func (service *Service) RespondToAgentQuestion(agentID, toolUseID, answer string) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	if service.store == nil {
		return errors.New("store not initialised")
	}
	// Read the agent once: we need PendingQuestion.Input before clearing it
	// (to craft the resume message) and the session/project info for the spawn.
	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}
	// Reject a stale answer aimed at a tool_use the agent is no longer awaiting,
	// so it can't clear an unrelated (newer) pending question.
	if agent.PendingQuestion != nil && toolUseID != "" && agent.PendingQuestion.ToolUseID != toolUseID {
		return fmt.Errorf("agent %q is awaiting tool_use %q, not %q", agentID, agent.PendingQuestion.ToolUseID, toolUseID)
	}
	var pendingInput json.RawMessage
	if agent.PendingQuestion != nil {
		pendingInput = agent.PendingQuestion.Input
	}
	// Drop the persisted question immediately and unconditionally, before
	// attempting delivery. This guarantees the panel never reappears — on this
	// view, after an agent switch, or after a restart — no matter which delivery
	// path runs below or whether it succeeds.
	_ = service.store.PatchAgent(agentID, map[string]any{"pendingQuestion": nil})

	// Resolve the tool call in the log so its dot stops pulsing blue, and preserve
	// the exchange: claude auto-dismisses the question and its real tool_result is
	// suppressed, so this marker is the only record. RenderedContent keeps the full
	// question + chosen answer reviewable as the call's expandable preview.
	_ = service.appendAgentEvent(agentID, StreamEvent{Type: "tool_result", ID: toolUseID, Content: summarizeAnswer(answer), RenderedContent: questionAnswerRecap(pendingInput, answer)})

	working := map[string]any{"status": "working"}

	// Clear the await flag so a later question in the same persistent session is
	// surfaced again instead of being deduped as a re-emission.
	service.runner.consumeAwaiting(agentID)

	// opencode: the answer is the chosen option label of a pending ACP
	// permission request; deliver it to the live session.
	service.runner.mu.Lock()
	acp := service.runner.acp[agentID]
	cs := service.runner.claude[agentID]
	service.runner.mu.Unlock()
	if acp != nil {
		if acp.answerPermission(toolUseID, parseAUQAnswerLabel(answer)) {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
	}

	// claude-code persistent session: claude auto-dismissed the interactive tool
	// and ended the turn, so the process is alive but idle. The answer is delivered
	// as the next user turn (a context-aware message, not a tool_result for the
	// already-closed tool), continuing in place without a resume.
	if cs != nil {
		if cs.answerQuestion(planResumeMessage(pendingInput, answer)) {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
	}

	service.runner.mu.Lock()
	proc, ok := service.runner.procs[agentID]
	service.runner.mu.Unlock()
	if ok && proc.stdin != nil {
		if err := writeClaudeToolResult(proc.stdin, toolUseID, answer, false); err == nil {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
		// Stdin write failed (e.g. broken pipe: process already exited in
		// --print mode). Fall through to the resume path below.
	}

	// Subprocess gone: resume the session with a context-aware user message.
	// We bypass Send() intentionally — the approval/rejection message is an
	// internal implementation detail and must not appear as a chat bubble in
	// the conversation. The tool_result log entry above already records the
	// user's choice visually.
	// (tool_result is rejected on resume — the API treats it as an assistant
	// prefill because the incomplete turn was stripped from the session.)
	if agent.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}
	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	_ = service.store.PatchAgent(agentID, working)
	msg := planResumeMessage(pendingInput, answer)
	workDir := agentRunDir(agent, project)

	// claude-code resumes as a fresh persistent session (the live-stdin path above
	// is the normal case; this only runs if its process had already exited).
	if agent.Kind == "claude-code" {
		return service.runner.startClaudeSession(service, agent, workDir, msg, "", true, true)
	}

	binary, args, env, err := service.buildResume(agent, "")
	if err != nil {
		return fmt.Errorf("build resume: %w", err)
	}
	go service.runner.run(service, agentID, agent.Kind, binary, args, workDir, "", true, env, nil)
	return nil
}

// planResumeMessage formats a user-turn message for the resume path after the
// agent subprocess was killed mid-ExitPlanMode / AskUserQuestion. The raw JSON
// payload must never be sent verbatim: the "question" fields can contain the
// full plan text or question wording, which Claude may mistake for new input
// and re-trigger the same tool call.
//
// For ExitPlanMode (detected by the "Plan" header on the first question) we
// emit a clear approval/rejection sentence with the plan as context.
// For AskUserQuestion we emit Q+A pairs so Claude has full context even if the
// tool_use turn was stripped from the session on resume.
func planResumeMessage(input json.RawMessage, answer string) string {
	label := summarizeAnswer(answer)
	if label == "" {
		return answer
	}
	var payload struct {
		Questions []struct {
			Header   string `json:"header"`
			Question string `json:"question"`
		} `json:"questions"`
	}
	if json.Unmarshal(input, &payload) != nil || len(payload.Questions) == 0 {
		return label
	}
	if payload.Questions[0].Header == "Plan" {
		plan := payload.Questions[0].Question
		switch label {
		case "Approve & proceed":
			return fmt.Sprintf("The user approved the plan. Please implement it now. Do not call ExitPlanMode or re-enter plan mode.\n\nApproved plan:\n%s", plan)
		case "Reject":
			return fmt.Sprintf("The user rejected the plan. Please reconsider your approach.\n\nRejected plan:\n%s", plan)
		default:
			return fmt.Sprintf("The user responded to your plan with: %s\n\nPlan:\n%s", label, plan)
		}
	}
	// AskUserQuestion: pair each question with its answer.
	var answers []struct {
		Answer json.RawMessage `json:"answer"`
	}
	if json.Unmarshal([]byte(answer), &answers) != nil {
		return label
	}
	var sb strings.Builder
	for i, q := range payload.Questions {
		if i >= len(answers) {
			break
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("Q: ")
		sb.WriteString(q.Question)
		sb.WriteString("\nA: ")
		var s string
		var arr []string
		switch {
		case json.Unmarshal(answers[i].Answer, &s) == nil:
			sb.WriteString(s)
		case json.Unmarshal(answers[i].Answer, &arr) == nil:
			sb.WriteString(strings.Join(arr, ", "))
		default:
			sb.Write(answers[i].Answer)
		}
	}
	if sb.Len() == 0 {
		return label
	}
	return sb.String()
}
