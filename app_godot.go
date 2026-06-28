package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/providers/godot"
)

func (app *App) DetectGodotProject(projectPath string) (*godot.Project, error) {
	return godot.Detect(projectPath)
}

func (app *App) DetectAllGodotProjects(projectPath string) ([]*godot.Project, error) {
	return godot.DetectAll(projectPath)
}

func (app *App) StartGodotPlay(manifestPath, packageManager, command, runEnv string) (string, error) {
	if app.godotRunner == nil {
		return "", fmt.Errorf("godot runner not ready")
	}
	return app.godotRunner.Start(manifestPath, packageManager, command, runEnv)
}

func (app *App) StopGodotPlay(runID string) error {
	if app.godotRunner == nil {
		return fmt.Errorf("godot runner not ready")
	}
	return app.godotRunner.Stop(runID)
}

func (app *App) RunGodotCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if app.godotRunner == nil {
		return "", fmt.Errorf("godot runner not ready")
	}
	return app.godotRunner.RunCommand(manifestPath, packageManager, runEnv, args)
}
