package main

import "github.com/KevinBonnoron/polaris/internal/providers/docker"

func (app *App) DetectDockerProject(projectPath string) (*docker.Project, error) {
	return docker.Detect(projectPath)
}

func (app *App) DetectAllDockerProjects(projectPath string) ([]*docker.Project, error) {
	return docker.DetectAll(projectPath)
}

func (app *App) ParseDockerfile(path string) (*docker.Dockerfile, error) {
	return docker.ParseDockerfile(path)
}

func (app *App) DockerCapabilities() docker.Capabilities {
	return docker.DetectCapabilities()
}

func (app *App) ListDockerBaseImages(dockerfilePath string) ([]docker.Image, error) {
	return docker.ListBaseImages(dockerfilePath)
}

func (app *App) DockerImageHistory(ref string) ([]docker.Layer, error) {
	return docker.ImageHistory(ref)
}

func (app *App) LintDockerfile(path string) ([]docker.Finding, error) {
	return docker.Lint(path)
}

func (app *App) ScanDockerImage(ref string) ([]docker.Vulnerability, error) {
	return docker.ScanImage(ref)
}
