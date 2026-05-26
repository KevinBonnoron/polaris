package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/providers/nodejs"
)

func (app *App) DetectNodeProject(projectPath string) (*nodejs.Project, error) {
	return nodejs.Detect(projectPath)
}

func (app *App) DetectAllNodeProjects(projectPath string) ([]*nodejs.Project, error) {
	return nodejs.DetectAll(projectPath)
}

func (app *App) ListNodeScripts(manifestPath string) ([]nodejs.Script, error) {
	return nodejs.ListScripts(manifestPath)
}

func (app *App) StartNodeScript(manifestPath, packageManager, script, runEnv string) (string, error) {
	if app.nodeRunner == nil {
		return "", fmt.Errorf("node runner not ready")
	}
	return app.nodeRunner.Start(manifestPath, packageManager, script, runEnv)
}

func (app *App) StopNodeScript(runID string) error {
	if app.nodeRunner == nil {
		return fmt.Errorf("node runner not ready")
	}
	return app.nodeRunner.Stop(runID)
}

func (app *App) ListNodePackages(manifestPath string) ([]nodejs.Dependency, error) {
	return nodejs.ListPackages(manifestPath)
}

func (app *App) ListNodeWorkspaces(manifestPath string) ([]nodejs.Workspace, error) {
	return nodejs.ListWorkspaces(manifestPath)
}

func (app *App) SetNodeDependencyVersion(manifests []string, name, version string) error {
	for _, m := range manifests {
		if err := nodejs.SetDependencyVersion(m, name, version); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) RunNodeCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if app.nodeRunner == nil {
		return "", fmt.Errorf("node runner not ready")
	}
	return app.nodeRunner.RunCommand(manifestPath, packageManager, runEnv, args)
}

func (app *App) CheckOutdatedPackages(manifestPath, packageManager, runEnv string) ([]nodejs.OutdatedPackage, error) {
	return nodejs.CheckOutdatedPackages(manifestPath, packageManager, runEnv)
}

func (app *App) CheckUnusedPackages(manifestPath, packageManager, runEnv string) ([]nodejs.UnusedPackage, error) {
	return nodejs.CheckUnusedPackages(manifestPath, packageManager, runEnv)
}

func (app *App) CheckVulnerabilities(manifestPath, packageManager, runEnv string) ([]nodejs.Vulnerability, error) {
	return nodejs.CheckVulnerabilities(manifestPath, packageManager, runEnv)
}

func (app *App) CheckPackagesInstalled(manifestPath string) (bool, error) {
	return nodejs.CheckPackagesInstalled(manifestPath)
}
