package polaris

import (
	"strings"
	"time"
)

// emitEvent writes a StreamEvent to the agent's log file and emits it live
// via the Wails event bus. This is the base emit primitive for ACP sessions.
func (a *acpSession) emitEvent(evt StreamEvent) {
	if evt.Ts == "" {
		evt.Ts = time.Now().Format("15:04:05")
	}
	_ = a.svc.appendAgentEvent(a.agentID, evt)
}

// emitToolCall emits the tool_call event once per tool, guarded by the
// emitted flag so concurrent/duplicate updates don't duplicate log entries.
func (a *acpSession) emitToolCall(id string) {
	a.mu.Lock()
	st := a.toolMap()[id]
	if st == nil {
		st = &acpToolState{}
		a.toolMap()[id] = st
	}
	if st.emitted {
		a.mu.Unlock()
		return
	}
	st.emitted = true
	name := st.name
	if name == "" {
		name = "Tool"
	}
	detail := st.detail
	input := st.input
	a.mu.Unlock()

	se := StreamEvent{
		Type: "tool_call",
		ID:   id,
		Name: name,
	}
	// Only the Agent tool needs its raw input on the wire (for sub-agent grouping);
	// other tools ship just the translated detail so we never double-send.
	if name == "Agent" {
		se.Input = input
	} else if detail != "" {
		se.Content = detail
	}
	a.emitEvent(se)
}

// emitToolResult emits the tool_result event.
func (a *acpSession) emitToolResult(id string, content string, renderedContent string, isErr bool) {
	if content == "" && renderedContent == "" {
		return
	}
	a.emitEvent(StreamEvent{
		Type:            "tool_result",
		ID:              id,
		Content:         content,
		RenderedContent: renderedContent,
		Error:           isErr,
	})
}

// emitMsgLines emits each line of assistant text as a separate text event,
// which the frontend merges into a single TextBlock.
func (a *acpSession) emitMsgLines(s string) {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		a.emitEvent(StreamEvent{Type: "text", Content: ln})
	}
}

// appendThought buffers streamed thinking and emits complete lines progressively.
func (a *acpSession) appendThought(s string) {
	a.mu.Lock()
	a.curThought.WriteString(s)
	buf := a.curThought.String()
	idx := strings.LastIndex(buf, "\n")
	if idx < 0 {
		a.mu.Unlock()
		return
	}
	ready := buf[:idx]
	a.curThought.Reset()
	a.curThought.WriteString(buf[idx+1:])
	a.mu.Unlock()
	for _, ln := range strings.Split(ready, "\n") {
		if strings.TrimSpace(ln) != "" {
			a.emitEvent(StreamEvent{Type: "thinking", Content: ln})
		}
	}
}

// appendMsg buffers streamed text and emits complete lines progressively.
func (a *acpSession) appendMsg(s string) {
	a.mu.Lock()
	a.curMsg.WriteString(s)
	buf := a.curMsg.String()
	idx := strings.LastIndex(buf, "\n")
	if idx < 0 {
		a.mu.Unlock()
		return
	}
	ready := buf[:idx]
	a.curMsg.Reset()
	a.curMsg.WriteString(buf[idx+1:])
	a.mu.Unlock()
	a.emitMsgLines(ready)
}

func (a *acpSession) flushThought() {
	a.mu.Lock()
	t := a.curThought.String()
	a.curThought.Reset()
	a.mu.Unlock()
	if t == "" {
		return
	}
	for _, ln := range strings.Split(strings.TrimRight(t, "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			a.emitEvent(StreamEvent{Type: "thinking", Content: ln})
		}
	}
}

func (a *acpSession) flush() {
	a.flushThought()
	a.mu.Lock()
	m := a.curMsg.String()
	a.curMsg.Reset()
	a.mu.Unlock()
	if strings.TrimSpace(m) != "" {
		a.emitMsgLines(m)
	}
}
