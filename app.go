package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/csharp"
	"github.com/KevinBonnoron/polaris/internal/providers/nodejs"
	"github.com/KevinBonnoron/polaris/internal/providers/python"
	"github.com/KevinBonnoron/polaris/internal/providers/shell"
	"github.com/KevinBonnoron/polaris/internal/store/dokploystore"
	"github.com/KevinBonnoron/polaris/internal/store/repositorystore"
	"github.com/KevinBonnoron/polaris/internal/store/sentrystore"
	"github.com/KevinBonnoron/polaris/internal/store/ticketsstore"
)

type BackendStatus struct {
	Ready     bool   `json:"ready"`
	LastError string `json:"lastError,omitempty"`
}

type AgentCli struct {
	Kind      string `json:"kind"`
	Binary    string `json:"binary"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type Ide struct {
	ID        string `json:"id"`
	Installed bool   `json:"installed"`
	Binary    string `json:"binary,omitempty"`
	Path      string `json:"path,omitempty"`
}

var agentCliCandidates = []struct {
	kind     string
	binaries []string
}{
	{"claude-code", []string{"claude"}},
	{"copilot", []string{"copilot"}},
	{"codex", []string{"codex"}},
	{"gemini", []string{"gemini"}},
	{"mistral", []string{"vibe-acp", "vibe"}},
	{"cursor", []string{"agent", "cursor"}},
	{"opencode", []string{"opencode"}},
}

// extraBinDirs lists install locations a GUI app often misses because it does
// not inherit the user's shell PATH. opencode's curl installer drops a
// self-contained binary in ~/.opencode/bin; npm-global / pipx / brew land in
// the others.
func extraBinDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".opencode", "bin"),
		filepath.Join(home, ".cursor", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
	}
}

// resolveAgentBinary locates an agent CLI by name. It honours PATH first, then
// falls back to extraBinDirs so a binary installed outside the GUI app's PATH
// (e.g. opencode via curl) is still found. The returned path is absolute when
// found, so callers can exec it regardless of PATH.
func resolveAgentBinary(binaries []string) (name, path string, installed bool) {
	for _, b := range binaries {
		if p, err := exec.LookPath(b); err == nil {
			return b, p, true
		}
	}
	for _, b := range binaries {
		for _, dir := range extraBinDirs() {
			cand := filepath.Join(dir, b)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return b, cand, true
			}
		}
	}
	return binaries[0], "", false
}

var ideCandidates = []struct {
	id       string
	binaries []string
	macOnly  bool
}{
	{id: "vscode", binaries: []string{"code", "code-insiders"}},
	{id: "cursor", binaries: []string{"cursor"}},
	{id: "zed", binaries: []string{"zed", "zeditor"}},
	{id: "windsurf", binaries: []string{"windsurf"}},
	{id: "jetbrains", binaries: []string{"idea", "webstorm", "pycharm", "goland", "clion", "rider", "rubymine", "phpstorm", "datagrip", "fleet"}},
	{id: "sublime", binaries: []string{"subl"}},
	{id: "xcode", binaries: []string{"xed"}, macOnly: true},
	{id: "vim", binaries: []string{"nvim", "vim"}},
	{id: "finder", binaries: []string{"open"}, macOnly: true},
}

type App struct {
	ctx             context.Context
	store           *polaris.Store
	svc             *polaris.Service
	automation      *polaris.AutomationManager
	repositoryStore *repositorystore.Store
	ticketsStore    *ticketsstore.Store
	sentryStore     *sentrystore.Store
	dokployStore    *dokploystore.Store
	nodeRunner      *nodejs.Runner
	pythonRunner    *python.Runner
	csharpRunner    *csharp.Runner
	shellRunner     *shell.Runner

	statusMu sync.RWMutex
	ready    bool
	lastErr  string
}

func NewApp() *App {
	return &App{}
}

// wailsEmitter adapts Wails' v3 event API to polaris.Emitter. EventManager.Emit
// already collapses a single payload (len(data)==1) into the event's data, which
// is what the frontend reads as e.data, so we forward the variadic unchanged.
type wailsEmitter struct{}

func (wailsEmitter) Emit(event string, data ...any) {
	if appRef == nil {
		return
	}
	appRef.Event.Emit(event, data...)
}

func (app *App) setReady(ready bool) {
	app.statusMu.Lock()
	defer app.statusMu.Unlock()
	app.ready = ready
}

func (app *App) setError(err error) {
	app.statusMu.Lock()
	defer app.statusMu.Unlock()
	if err == nil {
		app.lastErr = ""
	} else {
		app.lastErr = err.Error()
	}
}

// ServiceStartup implements the v3 service lifecycle. It returns nil even on
// failure so the window still loads and the UI surfaces the error via
// BackendStatus, matching the v2 graceful-degraded behaviour.
func (app *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	app.ctx = ctx

	dataDir, err := resolveDataDir()
	if err != nil {
		log.Printf("polaris: cannot resolve data dir: %v", err)
		app.setError(fmt.Errorf("resolve data dir: %w", err))
		return nil
	}
	log.Printf("polaris: using data dir %s", dataDir)

	store, err := polaris.OpenStore(filepath.Join(dataDir, "polaris.db"))
	if err != nil {
		log.Printf("polaris: open store failed: %v", err)
		app.setError(fmt.Errorf("open store: %w", err))
		return nil
	}
	store.SetEmitter(wailsEmitter{})
	app.store = store

	logsDir := filepath.Join(dataDir, "logs")
	worktreesDir := filepath.Join(dataDir, "worktrees")

	app.repositoryStore = repositorystore.New(&repositorystore.SQLitePersistence{DB: store.DB()})
	app.ticketsStore = ticketsstore.New(&ticketsstore.SQLitePersistence{DB: store.DB()})
	app.sentryStore = sentrystore.New()
	app.dokployStore = dokploystore.New()

	app.svc = polaris.NewService(store).
		WithRunner(polaris.NewRunner(logsDir, worktreesDir)).
		WithRepositoryStore(app.repositoryStore).
		WithTicketsStore(app.ticketsStore).
		WithSentryStore(app.sentryStore).
		WithDokployStore(app.dokployStore).
		WithBinaryResolver(func(kind string) string {
			for _, c := range agentCliCandidates {
				if c.kind != kind {
					continue
				}
				if _, path, ok := resolveAgentBinary(c.binaries); ok {
					return path
				}
				break
			}
			return ""
		})

	if err := app.svc.RecoverInterruptedAgents(); err != nil {
		log.Printf("polaris: recover interrupted agents: %v", err)
	}

	app.automation = polaris.NewAutomationManager(app.svc)
	if err := app.automation.Start(ctx); err != nil {
		log.Printf("polaris: start automation manager: %v", err)
	}

	app.nodeRunner = nodejs.NewRunner(wailsEmitter{})
	app.pythonRunner = python.NewRunner(wailsEmitter{})
	app.csharpRunner = csharp.NewRunner(wailsEmitter{})
	app.shellRunner = shell.NewRunner(wailsEmitter{})

	app.setReady(true)
	app.setError(nil)
	return nil
}

// ServiceShutdown implements the v3 service lifecycle (called on app quit).
func (app *App) ServiceShutdown() error {
	if app.automation != nil {
		app.automation.Stop()
	}
	if app.repositoryStore != nil {
		app.repositoryStore.Stop()
	}
	if app.ticketsStore != nil {
		app.ticketsStore.Stop()
	}
	if app.sentryStore != nil {
		app.sentryStore.Stop()
	}
	if app.dokployStore != nil {
		app.dokployStore.Stop()
	}
	if app.store != nil {
		if err := app.store.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) BackendStatus() BackendStatus {
	app.statusMu.RLock()
	defer app.statusMu.RUnlock()
	return BackendStatus{
		Ready:     app.ready,
		LastError: app.lastErr,
	}
}

func (app *App) Ping() string {
	return fmt.Sprintf("polaris %s", polaris.AppVersion())
}

func (app *App) AppVersion() string {
	return polaris.AppVersion()
}

func (app *App) CheckForUpdate(force bool) (*polaris.UpdateInfo, error) {
	return polaris.CheckForUpdate(force)
}

func (app *App) OpenExternalURL(rawURL string) {
	if appRef == nil {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return
	}
	_ = appRef.Browser.OpenURL(rawURL)
}

func (app *App) FetchClaudeUsage(force bool) (*polaris.ClaudeUsage, error) {
	return polaris.FetchClaudeUsage(force)
}

func (app *App) FetchCursorUsage(force bool) (*polaris.CursorUsage, error) {
	return polaris.FetchCursorUsage(force)
}

func (app *App) FetchClaudeModels(force bool) ([]polaris.ModelInfo, error) {
	return polaris.FetchClaudeModels(force)
}

func (app *App) SetAppFocused(focused bool) {
	if app.svc != nil {
		app.svc.SetAppFocused(focused)
	}
}

func (app *App) ResetAllData() error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	if app.automation != nil {
		app.automation.Stop()
	}
	if err := app.svc.ResetAll(); err != nil {
		return err
	}
	if app.automation != nil {
		if err := app.automation.Start(app.ctx); err != nil {
			log.Printf("polaris: restart automation manager after reset: %v", err)
		}
	}
	return nil
}

func (app *App) CloneRepository(repoURL, parentDir string) (string, error) {
	if repoURL == "" || parentDir == "" {
		return "", fmt.Errorf("repoURL and parentDir are required")
	}

	repoName := repoNameFromURL(repoURL)
	if repoName == "" {
		return "", fmt.Errorf("could not determine repository name from URL")
	}

	destPath := filepath.Join(parentDir, repoName)
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("destination %q already exists", destPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, destPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone: %s", strings.TrimSpace(string(out)))
	}

	return destPath, nil
}

func repoNameFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.Contains(rawURL, "://") && strings.Contains(rawURL, ":") {
		colon := strings.LastIndexByte(rawURL, ':')
		rawURL = rawURL[colon+1:]
	}
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		rawURL = u.Path
	}
	name := filepath.Base(strings.TrimSuffix(rawURL, ".git"))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func resolveDataDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(cfg, "polaris")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return dir, nil
}
