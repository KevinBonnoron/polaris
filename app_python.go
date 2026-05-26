package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/providers/python"
)

func (app *App) DetectPythonProject(projectPath string) (*python.Project, error) {
	return python.Detect(projectPath)
}

func (app *App) DetectAllPythonProjects(projectPath string) ([]*python.Project, error) {
	return python.DetectAll(projectPath)
}

func (app *App) ListPythonScripts(manifestPath string) ([]python.Script, error) {
	return python.ListScripts(manifestPath)
}

func (app *App) StartPythonScript(manifestPath, packageManager, script, runEnv string) (string, error) {
	if app.pythonRunner == nil {
		return "", fmt.Errorf("python runner not ready")
	}
	return app.pythonRunner.Start(manifestPath, packageManager, script, runEnv)
}

func (app *App) StopPythonScript(runID string) error {
	if app.pythonRunner == nil {
		return fmt.Errorf("python runner not ready")
	}
	return app.pythonRunner.Stop(runID)
}

func (app *App) ListPythonPackages(projectPath, manifestPath string) ([]python.Dependency, error) {
	return python.ListPackages(projectPath, manifestPath)
}

func (app *App) ListPythonWorkspaces(projectPath, manifestPath string) ([]python.Workspace, error) {
	return python.ListWorkspaces(projectPath, manifestPath)
}

func (app *App) SetPythonDependencyVersion(manifests []string, name, version string) error {
	for _, m := range manifests {
		if err := python.SetDependencyVersion(m, name, version); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) RunPythonCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if app.pythonRunner == nil {
		return "", fmt.Errorf("python runner not ready")
	}
	return app.pythonRunner.RunCommand(manifestPath, packageManager, runEnv, args)
}

func (app *App) CheckPythonOutdatedPackages(manifestPath, packageManager, runEnv string) ([]python.OutdatedPackage, error) {
	return python.CheckOutdatedPackages(manifestPath, packageManager, runEnv)
}

func (app *App) CheckPythonUnusedPackages(projectPath, manifestPath, packageManager, runEnv string) ([]python.UnusedPackage, error) {
	return python.CheckUnusedPackages(projectPath, manifestPath, packageManager, runEnv)
}

func (app *App) CheckPythonVulnerabilities(manifestPath, packageManager, runEnv string) ([]python.Vulnerability, error) {
	return python.CheckVulnerabilities(manifestPath, packageManager, runEnv)
}

func (app *App) CheckPythonPackagesInstalled(projectPath, manifestPath string) (bool, error) {
	return python.CheckPackagesInstalled(projectPath, manifestPath)
}
