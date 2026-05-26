package main

import "fmt"

func (app *App) StartShell(workDir string) (string, error) {
	if app.shellRunner == nil {
		return "", fmt.Errorf("shell runner not ready")
	}
	return app.shellRunner.Start(workDir)
}

func (app *App) WriteToShell(sessionID, data string) error {
	if app.shellRunner == nil {
		return fmt.Errorf("shell runner not ready")
	}
	return app.shellRunner.Write(sessionID, data)
}

func (app *App) ResizeShell(sessionID string, cols, rows int) error {
	if app.shellRunner == nil {
		return fmt.Errorf("shell runner not ready")
	}
	return app.shellRunner.Resize(sessionID, uint16(cols), uint16(rows))
}

func (app *App) StopShell(sessionID string) error {
	if app.shellRunner == nil {
		return fmt.Errorf("shell runner not ready")
	}
	return app.shellRunner.Stop(sessionID)
}
