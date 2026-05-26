package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/terminal"
)

func (app *App) ListOpencodeModels() []string {
	_, path, ok := resolveAgentBinary([]string{"opencode"})
	if !ok {
		return []string{}
	}
	out, err := exec.Command(path, "models").Output()
	if err != nil {
		return []string{}
	}
	models := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			models = append(models, line)
		}
	}
	return models
}

func (app *App) DetectAgentClis() []AgentCli {
	out := make([]AgentCli, 0, len(agentCliCandidates))
	for _, c := range agentCliCandidates {
		name, path, installed := resolveAgentBinary(c.binaries)
		out = append(out, AgentCli{Kind: c.kind, Binary: name, Installed: installed, Path: path})
	}
	return out
}

func (app *App) DetectIdes() []Ide {
	isMac := runtime.GOOS == "darwin"
	out := make([]Ide, 0, len(ideCandidates))
	for _, c := range ideCandidates {
		entry := Ide{ID: c.id}
		if c.macOnly && !isMac {
			out = append(out, entry)
			continue
		}
		for _, bin := range c.binaries {
			if p, err := exec.LookPath(bin); err == nil {
				entry.Installed = true
				entry.Binary = bin
				entry.Path = p
				break
			}
		}
		out = append(out, entry)
	}
	return out
}

func (app *App) DetectTerminals() []terminal.Terminal {
	return terminal.Detect()
}

func (app *App) OpenTerminal(id, dir string) error {
	return terminal.OpenInDir(id, dir)
}

func (app *App) OpenInIde(repoDir, relPath string, line, col int) error {
	settings, err := app.svc.GetGeneralSettings()
	if err != nil {
		return err
	}

	filePath := filepath.Join(repoDir, relPath)
	ideId := settings.IdeId
	if ideId == "" {
		ideId = "vscode"
	}

	if ideId == "vscode" || ideId == "cursor" {
		return app.openIdeDiff(ideId, repoDir, relPath, filePath)
	}

	cmd := settings.IdeCmd
	if cmd == "" {
		cmd = `code --goto "$FILE:$LINE:$COL"`
	}
	cmd = strings.ReplaceAll(cmd, "$FILE", filePath)
	cmd = strings.ReplaceAll(cmd, "$LINE", fmt.Sprintf("%d", line))
	cmd = strings.ReplaceAll(cmd, "$COL", fmt.Sprintf("%d", col))

	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmd).Start()
	}
	return exec.Command("sh", "-c", cmd).Start()
}

func (app *App) openIdeDiff(ideId, repoDir, relPath, filePath string) error {
	bin := "code"
	if ideId == "cursor" {
		bin = "cursor"
	}

	gitShow := exec.Command("git", "show", "HEAD:"+relPath)
	gitShow.Dir = repoDir
	oldContent, err := gitShow.Output()
	if err != nil {
		return exec.Command(bin, "--goto", filePath).Start()
	}

	tmpFile, err := os.CreateTemp("", "polaris-diff-*"+filepath.Ext(relPath))
	if err != nil {
		return exec.Command(bin, "--goto", filePath).Start()
	}
	if _, err := tmpFile.Write(oldContent); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return exec.Command(bin, "--goto", filePath).Start()
	}
	tmpFile.Close()

	cmd := exec.Command(bin, "--diff", tmpFile.Name(), filePath)
	if err := cmd.Start(); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	go func() {
		cmd.Wait()
		os.Remove(tmpFile.Name())
	}()
	return nil
}

func (app *App) DetectGitRemote(projectPath string) (*git.Remote, error) {
	return git.DetectRemote(projectPath)
}

func (app *App) DetectProviderToken(provider string) (*git.ProviderToken, error) {
	return git.DetectProviderToken(provider)
}
