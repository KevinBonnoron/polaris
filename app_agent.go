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

func (app *App) ClearAgentLog(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.ClearLog(agentID)
}
