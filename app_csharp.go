package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/providers/csharp"
)

func (app *App) DetectCSharpProject(projectPath string) (*csharp.Project, error) {
	return csharp.Detect(projectPath)
}

func (app *App) DetectAllCSharpProjects(projectPath string) ([]*csharp.Project, error) {
	return csharp.DetectAll(projectPath)
}

func (app *App) ListCSharpScripts(manifestPath string) ([]csharp.Script, error) {
	return csharp.ListScripts(manifestPath)
}

func (app *App) StartCSharpScript(manifestPath, packageManager, script, runEnv string) (string, error) {
	if app.csharpRunner == nil {
		return "", fmt.Errorf("csharp runner not ready")
	}
	return app.csharpRunner.Start(manifestPath, packageManager, script, runEnv)
}

func (app *App) StopCSharpScript(runID string) error {
	if app.csharpRunner == nil {
		return fmt.Errorf("csharp runner not ready")
	}
	return app.csharpRunner.Stop(runID)
}

func (app *App) ListCSharpPackages(projectPath, manifestPath string) ([]csharp.Dependency, error) {
	return csharp.ListPackages(projectPath, manifestPath)
}

func (app *App) ListCSharpWorkspaces(projectPath, manifestPath string) ([]csharp.Workspace, error) {
	return csharp.ListWorkspaces(projectPath, manifestPath)
}

func (app *App) SetCSharpDependencyVersion(manifests []string, name, version string) error {
	for _, m := range manifests {
		if err := csharp.SetDependencyVersion(m, name, version); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) RunCSharpCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if app.csharpRunner == nil {
		return "", fmt.Errorf("csharp runner not ready")
	}
	return app.csharpRunner.RunCommand(manifestPath, packageManager, runEnv, args)
}

func (app *App) CheckCSharpOutdatedPackages(manifestPath, packageManager, runEnv string) ([]csharp.OutdatedPackage, error) {
	return csharp.CheckOutdatedPackages(manifestPath, packageManager, runEnv)
}

func (app *App) CheckCSharpUnusedPackages(projectPath, manifestPath, packageManager, runEnv string) ([]csharp.UnusedPackage, error) {
	return csharp.CheckUnusedPackages(projectPath, manifestPath, packageManager, runEnv)
}

func (app *App) CheckCSharpVulnerabilities(manifestPath, packageManager, runEnv string) ([]csharp.Vulnerability, error) {
	return csharp.CheckVulnerabilities(manifestPath, packageManager, runEnv)
}

func (app *App) CheckCSharpPackagesInstalled(projectPath, manifestPath string) (bool, error) {
	return csharp.CheckPackagesInstalled(projectPath, manifestPath)
}
