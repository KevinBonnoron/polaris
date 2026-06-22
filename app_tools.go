package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/terminal"
)

// ansiEscape matches ANSI/VT100 escape sequences produced by TUI-based CLIs.
var ansiEscape = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-9;?]*[ -/]*[@-~])`)

func parseModelsOutput(raw string) []string {
	clean := ansiEscape.ReplaceAllString(raw, "")
	var models []string
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Discard lines with spaces or non-printable characters — those are
		// UI chrome, not model IDs.
		valid := true
		for _, r := range line {
			if r == ' ' || !unicode.IsPrint(r) {
				valid = false
				break
			}
		}
		if valid {
			models = append(models, line)
		}
	}
	return models
}

func (app *App) ListOpencodeModels() []string {
	_, path, ok := resolveAgentBinary([]string{"opencode"})
	if !ok {
		return []string{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "models").Output()
	if err != nil {
		return []string{}
	}
	return parseModelsOutput(string(out))
}

// ListCliModels returns the available models for the given agent kind.
// For providers with a known API (gemini, mistral), it reads stored credentials
// and calls the provider API directly. For others, it tries `<binary> models`.
func (app *App) ListCliModels(kind string) []polaris.ModelInfo {
	switch kind {
	case "gemini":
		models, err := polaris.FetchGeminiModels()
		if err != nil {
			return []polaris.ModelInfo{}
		}
		return models
	case "mistral":
		models, err := polaris.FetchMistralModels()
		if err != nil {
			return []polaris.ModelInfo{}
		}
		return models
	}

	var binaries []string
	for _, c := range agentCliCandidates {
		if c.kind == kind {
			binaries = c.binaries
			break
		}
	}
	if len(binaries) == 0 {
		return []polaris.ModelInfo{}
	}
	_, path, ok := resolveAgentBinary(binaries)
	if !ok {
		return []polaris.ModelInfo{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "models").Output()
	if err != nil {
		return []polaris.ModelInfo{}
	}
	ids := parseModelsOutput(string(out))
	models := make([]polaris.ModelInfo, len(ids))
	for i, id := range ids {
		models[i] = polaris.ModelInfo{Value: id, Name: id, Family: ""}
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

	tmpl := settings.IdeCmd
	if tmpl == "" {
		tmpl = `code --goto "$FILE:$LINE:$COL"`
	}
	// Tokenise the template BEFORE substituting, then inject the values into
	// individual argv elements. This keeps a file path containing shell
	// metacharacters from being re-parsed (no "sh -c" / "cmd /C").
	tokens := splitCommandTemplate(tmpl)
	if len(tokens) == 0 {
		return fmt.Errorf("empty IDE command")
	}
	repl := strings.NewReplacer(
		"$FILE", filePath,
		"$LINE", fmt.Sprintf("%d", line),
		"$COL", fmt.Sprintf("%d", col),
	)
	for i, tok := range tokens {
		tokens[i] = repl.Replace(tok)
	}
	cmd := exec.Command(tokens[0], tokens[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the launcher so a short-lived IDE process doesn't linger as a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}

// splitCommandTemplate splits a command template on whitespace, honouring
// single and double quotes for grouping. It performs no variable expansion or
// escape handling — placeholders are substituted into the resulting tokens.
func splitCommandTemplate(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	inToken := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case unicode.IsSpace(r):
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens
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
