package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/polaris"
)

func (app *App) SpawnAgent(in polaris.SpawnAgentInput) (*polaris.Agent, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	if in.Binary == "" {
		for _, c := range agentCliCandidates {
			if c.kind != in.Kind {
				continue
			}
			if _, path, ok := resolveAgentBinary(c.binaries); ok {
				in.Binary = path
			}
			break
		}
	}
	return app.svc.Spawn(in)
}

func (app *App) CancelAgent(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.Cancel(agentID)
}

func (app *App) SendToAgent(agentID, message string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.Send(agentID, message)
}

// InterruptAndSendToAgent aborts the agent's in-flight turn and applies the
// message immediately, instead of queueing it until the current turn finishes.
func (app *App) InterruptAndSendToAgent(agentID, message string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.InterruptAndSend(agentID, message)
}

// ClearAgentQueuedMessage drops the agent's pending follow-up (the queued chip),
// used when the user pulls it back into the input to edit it.
func (app *App) ClearAgentQueuedMessage(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.ClearQueuedMessage(agentID)
}

// StopAndRetractLastMessage stops the agent and, when its current turn had no
// response yet, removes the user's last message from the log and returns it so
// the UI can drop it back into the input. Returns "" if nothing was retracted.
func (app *App) StopAndRetractLastMessage(agentID string) (string, error) {
	if app.svc == nil {
		return "", fmt.Errorf("service not ready")
	}
	return app.svc.StopAndRetractLast(agentID)
}

func (app *App) RespondToAgentQuestion(agentID, toolUseID, answer string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.RespondToAgentQuestion(agentID, toolUseID, answer)
}

func (app *App) ListClaudeCodeSessions(projectID string) ([]polaris.ClaudeSession, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	return app.svc.ListClaudeCodeSessions(projectID)
}

func (app *App) TeleportClaudeSession(projectID, sessionID string) (*polaris.Agent, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	return app.svc.TeleportClaudeSession(projectID, sessionID)
}

func (app *App) ReadAgentLog(agentID string) ([]polaris.StreamEvent, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	return app.svc.ReadLogEvents(agentID)
}

func (app *App) ReadAgentLogFrom(agentID string, offset int64) (polaris.LogTail, error) {
	if app.svc == nil {
		return polaris.LogTail{Offset: offset}, fmt.Errorf("service not ready")
	}
	return app.svc.ReadLogEventsFrom(agentID, offset)
}

func (app *App) ClearAgentLog(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.ClearLog(agentID)
}

func (app *App) CompactAgent(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	go app.svc.Compact(agentID)
	return nil
}
