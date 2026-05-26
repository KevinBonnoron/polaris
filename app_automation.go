package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/polaris"
)

func (app *App) ListAutomations() ([]polaris.Automation, error) {
	return app.svc.ListAutomations()
}

func (app *App) ListAutomationRuns(automationID string, limit int) ([]polaris.AutomationRun, error) {
	return app.svc.ListAutomationRuns(automationID, limit)
}

func (app *App) UpsertAutomation(a polaris.Automation) (polaris.Automation, error) {
	saved, err := app.svc.UpsertAutomation(a)
	if err == nil && app.automation != nil {
		app.automation.Reschedule(saved.ID)
	}
	return saved, err
}

func (app *App) FireAutomation(id string) error {
	if app.automation == nil {
		return fmt.Errorf("automation manager not ready")
	}
	return app.automation.FireManual(id)
}

func (app *App) DeleteAutomation(id string) error {
	if err := app.svc.DeleteAutomation(id); err != nil {
		return err
	}
	if app.automation != nil {
		app.automation.Reschedule(id)
	}
	return nil
}
