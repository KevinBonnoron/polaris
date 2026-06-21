package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/providers/taskfile"
)

func (app *App) DetectTaskfileProject(projectPath string) (*taskfile.Project, error) {
	return taskfile.Detect(projectPath)
}

func (app *App) DetectAllTaskfileProjects(projectPath string) ([]*taskfile.Project, error) {
	return taskfile.DetectAll(projectPath)
}

func (app *App) ListTaskfileTasks(manifestPath string) ([]taskfile.Script, error) {
	return taskfile.ListScripts(manifestPath)
}

func (app *App) StartTaskfileTask(manifestPath, packageManager, task, runEnv string) (string, error) {
	if app.taskfileRunner == nil {
		return "", fmt.Errorf("taskfile runner not ready")
	}
	return app.taskfileRunner.Start(manifestPath, packageManager, task, runEnv)
}

func (app *App) StopTaskfileTask(runID string) error {
	if app.taskfileRunner == nil {
		return fmt.Errorf("taskfile runner not ready")
	}
	return app.taskfileRunner.Stop(runID)
}

func (app *App) RunTaskfileCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if app.taskfileRunner == nil {
		return "", fmt.Errorf("taskfile runner not ready")
	}
	return app.taskfileRunner.RunCommand(manifestPath, packageManager, runEnv, args)
}
