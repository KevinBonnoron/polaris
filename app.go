package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/docker"
	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
	"github.com/KevinBonnoron/polaris/internal/providers/messaging"
	"github.com/KevinBonnoron/polaris/internal/providers/repository"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/providers/tickets"
	"github.com/KevinBonnoron/polaris/internal/providers/nodejs"
	"github.com/KevinBonnoron/polaris/internal/providers/python"
	"github.com/KevinBonnoron/polaris/internal/providers/shell"
	"github.com/KevinBonnoron/polaris/internal/providers/resend"
	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
	"github.com/KevinBonnoron/polaris/internal/store/dokploystore"
	"github.com/KevinBonnoron/polaris/internal/store/ghstore"
	"github.com/KevinBonnoron/polaris/internal/store/jirastore"
	"github.com/KevinBonnoron/polaris/internal/store/sentrystore"
	"github.com/KevinBonnoron/polaris/internal/terminal"
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
	{"cursor", []string{"cursor"}},
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
	ctx          context.Context
	store        *polaris.Store
	svc          *polaris.Service
	automation   *polaris.AutomationManager
	ghStore      *ghstore.Store
	jiraStore    *jirastore.Store
	sentryStore  *sentrystore.Store
	dokployStore *dokploystore.Store
	nodeRunner   *nodejs.Runner
	pythonRunner *python.Runner
	shellRunner  *shell.Runner

	statusMu sync.RWMutex
	ready    bool
	lastErr  string
}

func NewApp() *App {
	return &App{}
}

// wailsEmitter adapts Wails' runtime event API to polaris.Emitter.
type wailsEmitter struct{ ctx context.Context }

func (w wailsEmitter) Emit(event string, data ...any) {
	if w.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(w.ctx, event, data...)
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

func (app *App) startup(ctx context.Context) {
	app.ctx = ctx

	dataDir, err := resolveDataDir()
	if err != nil {
		log.Printf("polaris: cannot resolve data dir: %v", err)
		app.setError(fmt.Errorf("resolve data dir: %w", err))
		return
	}
	log.Printf("polaris: using data dir %s", dataDir)

	store, err := polaris.OpenStore(filepath.Join(dataDir, "polaris.db"))
	if err != nil {
		log.Printf("polaris: open store failed: %v", err)
		app.setError(fmt.Errorf("open store: %w", err))
		return
	}
	store.SetEmitter(wailsEmitter{ctx: ctx})
	app.store = store

	logsDir := filepath.Join(dataDir, "logs")
	worktreesDir := filepath.Join(dataDir, "worktrees")

	app.ghStore = ghstore.New(&ghstore.SQLitePersistence{DB: store.DB()})
	app.jiraStore = jirastore.New(&jirastore.SQLitePersistence{DB: store.DB()})
	app.sentryStore = sentrystore.New()
	app.dokployStore = dokploystore.New()

	app.svc = polaris.NewService(store).
		WithRunner(polaris.NewRunner(logsDir, worktreesDir)).
		WithGhStore(app.ghStore).
		WithJiraStore(app.jiraStore).
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

	app.nodeRunner = nodejs.NewRunner(wailsEmitter{ctx: ctx})
	app.pythonRunner = python.NewRunner(wailsEmitter{ctx: ctx})
	app.shellRunner = shell.NewRunner(wailsEmitter{ctx: ctx})

	app.setReady(true)
	app.setError(nil)
}

func (app *App) shutdown(ctx context.Context) {
	if app.automation != nil {
		app.automation.Stop()
	}
	if app.ghStore != nil {
		app.ghStore.Stop()
	}
	if app.jiraStore != nil {
		app.jiraStore.Stop()
	}
	if app.sentryStore != nil {
		app.sentryStore.Stop()
	}
	if app.dokployStore != nil {
		app.dokployStore.Stop()
	}
	if app.store != nil {
		_ = app.store.Close()
	}
}

func (app *App) BackendStatus() BackendStatus {
	app.statusMu.RLock()
	defer app.statusMu.RUnlock()
	return BackendStatus{
		Ready:     app.ready,
		LastError: app.lastErr,
	}
}

// --- projects ---

func (app *App) ListProjects() ([]polaris.Project, error) {
	return app.svc.ListProjects()
}

func (app *App) UpsertProject(p polaris.Project) (polaris.Project, error) {
	return app.svc.UpsertProject(p)
}

func (app *App) DeleteProject(id string) error {
	return app.svc.DeleteProject(id)
}

// --- agents ---

func (app *App) ListAgents(projectID string) ([]polaris.Agent, error) {
	return app.svc.ListAgents(projectID)
}

func (app *App) UpsertAgent(a polaris.Agent) (polaris.Agent, error) {
	return app.svc.UpsertAgent(a)
}

func (app *App) DeleteAgent(id string) error {
	return app.svc.DeleteAgent(id)
}

// --- notifications ---

func (app *App) ListNotifications() ([]polaris.Notification, error) {
	return app.svc.ListNotifications()
}

func (app *App) UpsertNotification(n polaris.Notification) (polaris.Notification, error) {
	return app.svc.UpsertNotification(n)
}

func (app *App) DeleteNotification(id string) error {
	return app.svc.DeleteNotification(id)
}

// --- custom providers ---

func (app *App) ListCustomProviders() ([]polaris.CustomProvider, error) {
	return app.svc.ListCustomProviders()
}

func (app *App) UpsertCustomProvider(p polaris.CustomProvider) (polaris.CustomProvider, error) {
	return app.svc.UpsertCustomProvider(p)
}

func (app *App) DeleteCustomProvider(id string) error {
	return app.svc.DeleteCustomProvider(id)
}

func (app *App) TestCustomProvider(p polaris.CustomProvider) (polaris.ProviderHealth, error) {
	return app.svc.TestCustomProvider(p)
}

func (app *App) GetAgentDefaultModels() (map[string]string, error) {
	return app.svc.GetAgentDefaultModels()
}

func (app *App) SetAgentDefaultModel(id, model string) error {
	return app.svc.SetAgentDefaultModel(id, model)
}

func (app *App) SetAgentModel(agentID, model string) error {
	return app.svc.SetAgentModel(agentID, model)
}

func (app *App) MarkAllNotificationsRead() error {
	return app.svc.MarkAllNotificationsRead()
}

func (app *App) MarkNotificationRead(id string) error {
	return app.svc.MarkNotificationRead(id)
}

func (app *App) GetAppearanceSettings() (polaris.AppearanceSettings, error) {
	return app.svc.GetAppearanceSettings()
}

func (app *App) UpdateAppearanceSettings(s polaris.AppearanceSettings) (polaris.AppearanceSettings, error) {
	return app.svc.UpdateAppearanceSettings(s)
}

func (app *App) GetNotificationSettings() (polaris.NotificationSettings, error) {
	return app.svc.GetNotificationSettings()
}

func (app *App) UpdateNotificationSettings(s polaris.NotificationSettings) (polaris.NotificationSettings, error) {
	return app.svc.UpdateNotificationSettings(s)
}

func (app *App) GetShortcutsSettings() (polaris.ShortcutsSettings, error) {
	return app.svc.GetShortcutsSettings()
}

func (app *App) UpdateShortcutsSettings(s polaris.ShortcutsSettings) (polaris.ShortcutsSettings, error) {
	return app.svc.UpdateShortcutsSettings(s)
}

func (app *App) GetGeneralSettings() (polaris.GeneralSettings, error) {
	return app.svc.GetGeneralSettings()
}

func (app *App) UpdateGeneralSettings(s polaris.GeneralSettings) (polaris.GeneralSettings, error) {
	return app.svc.UpdateGeneralSettings(s)
}

func (app *App) SetAppFocused(focused bool) {
	if app.svc != nil {
		app.svc.SetAppFocused(focused)
	}
}

// --- agent runner ---

func (app *App) SpawnAgent(in polaris.SpawnAgentInput) (*polaris.Agent, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	if in.Binary == "" {
		for _, c := range agentCliCandidates {
			if c.kind != in.Kind {
				continue
			}
			if _, path, ok := resolveAgentBinary(c.binaries); ok {
				in.Binary = path
			}
			break
		}
	}
	return app.svc.Spawn(in)
}

func (app *App) CancelAgent(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}

	return app.svc.Cancel(agentID)
}

func (app *App) PickFiles(defaultDir string) ([]string, error) {
	return wailsruntime.OpenMultipleFilesDialog(app.ctx, wailsruntime.OpenDialogOptions{
		DefaultDirectory: defaultDir,
	})
}

func (app *App) SaveTempImage(base64Data, ext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("polaris_screenshot_%d%s", time.Now().UnixNano(), ext)
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (app *App) PasteClipboardImage() (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("polaris_paste_%d.png", time.Now().UnixNano()))

	switch runtime.GOOS {
	case "linux":
		toolFound := false
		if bin, err := exec.LookPath("wl-paste"); err == nil {
			toolFound = true
			cmd := exec.Command(bin, "--type", "image/png")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
		}
		if bin, err := exec.LookPath("xclip"); err == nil {
			toolFound = true
			cmd := exec.Command(bin, "-selection", "clipboard", "-t", "image/png", "-o")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
		}
		if !toolFound {
			return "", fmt.Errorf("clipboard image paste requires wl-clipboard (Wayland) or xclip (X11) — install one of them to paste images")
		}
	case "darwin":
		script := fmt.Sprintf(`try
set img to the clipboard as «class PNGf»
set f to open for access POSIX file %q with write permission
write img to f
close access f
end try`, path)
		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Run(); err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no image in clipboard")
}

func (app *App) ReadFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (app *App) SendToAgent(agentID, message string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}

	return app.svc.Send(agentID, message)
}

func (app *App) RespondToAgentQuestion(agentID, toolUseID, answer string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.RespondToAgentQuestion(agentID, toolUseID, answer)
}

func (app *App) ListClaudeCodeSessions(projectID string) ([]polaris.ClaudeSession, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	return app.svc.ListClaudeCodeSessions(projectID)
}

func (app *App) TeleportClaudeSession(projectID, sessionID string) (*polaris.Agent, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}
	return app.svc.TeleportClaudeSession(projectID, sessionID)
}

func (app *App) CreatePRForAgent(agentID string) (string, error) {
	if app.svc == nil {
		return "", fmt.Errorf("service not ready")
	}
	return app.svc.CreatePRForAgent(agentID)
}

func (app *App) GetAgentDiff(agentID string) (string, error) {
	if app.store == nil {
		return "", fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			return git.AgentDiff(agent.Worktree.Path)
		}
	}

	if agent.Worktree.Branch != "" {
		proj, projErr := app.store.GetProject(agent.ProjectID)
		if projErr == nil && proj != nil && proj.Path != "" {
			return git.BranchDiff(proj.Path, agent.Worktree.Branch)
		}
	}

	if app.svc != nil {
		proj, projErr := app.store.GetProject(agent.ProjectID)
		if projErr == nil && proj != nil && proj.Path != "" {
			logContent, logErr := app.svc.ReadLog(agentID)
			if logErr == nil && logContent != "" {
				return git.LogDiff(proj.Path, agent.StartedAt, logContent)
			}
		}
	}

	return "", nil
}

func (app *App) GetAgentFileStatuses(agentID string) ([]git.FileChangeStatus, error) {
	if app.store == nil {
		return nil, fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			// Dedicated worktree — everything in it belongs to this agent.
			return git.AgentFileStatuses(agent.Worktree.Path, nil)
		}
	}

	scope := app.agentScopedPaths(agentID)
	if proj, projErr := app.store.GetProject(agent.ProjectID); projErr == nil && proj != nil && proj.Path != "" {
		scope = git.RepoRelativePaths(proj.Path, scope)
	}

	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return nil, fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return []git.FileChangeStatus{}, nil
	}
	if len(scope) == 0 {
		// Without a worktree and without log evidence, we can't tell which
		// files belong to this agent — return nothing rather than mixing in
		// other agents' work.
		return []git.FileChangeStatus{}, nil
	}
	return git.AgentFileStatuses(proj.Path, scope)
}

// agentScopedPaths returns the set of file paths attributable to the given
// agent based on its log (Edit/Write/MultiEdit/... tool calls). Returns nil
// when the log is unavailable so callers can decide their fallback.
func (app *App) agentScopedPaths(agentID string) []string {
	if app.svc == nil {
		return nil
	}
	logContent, err := app.svc.ReadLog(agentID)
	if err != nil || logContent == "" {
		return nil
	}
	return git.ExtractLogFilePaths(logContent)
}

func (app *App) GetAgentGitState(agentID string) (git.AgentState, error) {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return git.AgentState{}, err
	}
	return git.CollectAgentState(dir)
}

func (app *App) CommitAgent(agentID, message string, amend bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Commit(dir, message, amend)
}

func (app *App) PushAgent(agentID string, force bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Push(dir, force)
}

func (app *App) SyncAgent(agentID string, force bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Sync(dir, force)
}

func (app *App) StageAgentFile(agentID, path string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, []string{path})
}

func (app *App) StageAgentFiles(agentID string, paths []string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, paths)
}

func (app *App) UnstageAgentFile(agentID, path string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, []string{path})
}

func (app *App) UnstageAgentFiles(agentID string, paths []string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, paths)
}

func (app *App) agentGitDir(agentID string) (string, error) {
	if app.store == nil {
		return "", fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found")
	}
	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			return agent.Worktree.Path, nil
		}
	}
	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return "", fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return "", fmt.Errorf("project path unavailable")
	}
	return proj.Path, nil
}

func (app *App) UnstageAgentChanges(agentID string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}

	// Unstage every path currently in the index — scoped via the same logic
	// as staging so we don't touch other agents' work.
	scope := app.agentScopedPaths(agentID)
	if proj, projErr := app.store.GetProject(app.mustAgentProjectID(agentID)); projErr == nil && proj != nil && proj.Path != "" {
		scope = git.RepoRelativePaths(proj.Path, scope)
	}
	if len(scope) == 0 {
		// No log evidence to scope; fall back to unstaging the entire index.
		return git.UnstagePaths(dir, []string{"."})
	}
	return git.UnstagePaths(dir, scope)
}

func (app *App) mustAgentProjectID(agentID string) string {
	if app.store == nil {
		return ""
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return ""
	}
	return agent.ProjectID
}

func (app *App) StageAgentChanges(agentID string) error {
	if app.store == nil {
		return fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			paths, pathErr := git.AgentChangedPaths(agent.Worktree.Path)
			if pathErr != nil {
				return pathErr
			}
			if len(paths) == 0 {
				return fmt.Errorf("no changes to stage")
			}
			return git.StagePaths(agent.Worktree.Path, paths)
		}
	}

	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return fmt.Errorf("project path unavailable")
	}

	if app.svc != nil {
		if logContent, logErr := app.svc.ReadLog(agentID); logErr == nil && logContent != "" {
			paths := git.FilterStageable(proj.Path, git.RepoRelativePaths(proj.Path, git.ExtractLogFilePaths(logContent)))
			if len(paths) > 0 {
				return git.StagePaths(proj.Path, paths)
			}
		}
	}
	return fmt.Errorf("no changes to stage")
}

func (app *App) ReadAgentLog(agentID string) (string, error) {
	if app.svc == nil {
		return "", fmt.Errorf("service not ready")
	}

	return app.svc.ReadLog(agentID)
}

func (app *App) ReadAgentLogTail(agentID string, n int) ([]string, error) {
	if app.svc == nil {
		return nil, fmt.Errorf("service not ready")
	}

	return app.svc.ReadLogTail(agentID, n)
}

func (app *App) ClearAgentLog(agentID string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}

	return app.svc.ClearLog(agentID)
}

// ListOpencodeModels returns the "provider/model" ids opencode knows about
// (from its own auth/config), for the plain opencode agent kind's model picker.
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

func (app *App) Ping() string {
	return fmt.Sprintf("polaris %s", polaris.AppVersion())
}

func (app *App) AppVersion() string {
	return polaris.AppVersion()
}

func (app *App) CheckForUpdate(force bool) (*polaris.UpdateInfo, error) {
	return polaris.CheckForUpdate(force)
}

func (app *App) OpenExternalURL(url string) {
	if app.ctx == nil {
		return
	}
	wailsruntime.BrowserOpenURL(app.ctx, url)
}

func (app *App) FetchClaudeUsage(force bool) (*polaris.ClaudeUsage, error) {
	return polaris.FetchClaudeUsage(force)
}

func (app *App) FetchClaudeModels(force bool) ([]polaris.ClaudeModel, error) {
	return polaris.FetchClaudeModels(force)
}

func (app *App) DetectGitRemote(projectPath string) (*git.Remote, error) {
	return git.DetectRemote(projectPath)
}

func (app *App) DetectProviderToken(provider string) (*git.ProviderToken, error) {
	return git.DetectProviderToken(provider)
}

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

func (app *App) ListRepoPullRequests(owner, repo string) ([]repository.PullRequest, error) {
	if app.ghStore != nil {
		if err := app.ghStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.ghStore.GetPRs(app.ctx, owner, repo)
	}
	return repository.ListPullRequests(owner, repo)
}

func (app *App) ListRepoIssues(owner, repo string) ([]repository.Issue, error) {
	if app.ghStore != nil {
		if err := app.ghStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.ghStore.GetIssues(app.ctx, owner, repo)
	}
	return repository.ListIssues(owner, repo)
}

// ListRepoWorkflowRuns: page 1 goes through the cache so it can be shared
// with automation polling; deeper pages (loadMore) bypass the cache because
// the store only tracks the first page used for diff detection.
func (app *App) ListRepoWorkflowRuns(owner, repo string, page int) (*repository.WorkflowRunsPage, error) {
	if page > 1 {
		return repository.ListWorkflowRuns(owner, repo, page)
	}
	if app.ghStore != nil {
		if err := app.ghStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		runs, err := app.ghStore.GetRuns(app.ctx, owner, repo)
		if err != nil {
			return nil, err
		}
		// hasMore is approximated: if we got a full page worth of runs we
		// assume there's more — same heuristic the provider applies.
		return &repository.WorkflowRunsPage{Runs: runs, HasMore: len(runs) >= 30}, nil
	}
	return repository.ListWorkflowRuns(owner, repo, page)
}

func (app *App) ListRepoBranches(owner, repo string) ([]string, error) {
	return repository.ListBranches(owner, repo)
}

func (app *App) GetGhCurrentUser() (string, error) {
	return repository.GetCurrentUser()
}

func (app *App) GetRepoWorkflowDispatch(owner, repo string, workflowID int64) (*repository.WorkflowDispatchSpec, error) {
	return repository.GetWorkflowDispatch(owner, repo, workflowID)
}

func (app *App) TriggerRepoWorkflow(owner, repo string, workflowID int64, ref string, inputs map[string]string) error {
	return repository.TriggerWorkflowDispatch(owner, repo, workflowID, ref, inputs)
}

func (app *App) CancelRepoWorkflowRun(owner, repo string, runID int64) error {
	return repository.CancelWorkflowRun(owner, repo, runID)
}

func (app *App) RerunRepoWorkflowRun(owner, repo string, runID int64) error {
	return repository.RerunWorkflowRun(owner, repo, runID)
}

func (app *App) FetchJiraSprint(cfg tickets.Config) (*tickets.Sprint, error) {
	if app.jiraStore != nil {
		k := jirastore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
		app.jiraStore.SetConfig(k, cfg)
		if err := app.jiraStore.Refresh(app.ctx, k); err != nil {
			return nil, err
		}
		snap, err := app.jiraStore.GetSnapshot(app.ctx, k, cfg)
		if err != nil {
			return nil, err
		}
		if snap.Err != nil {
			return nil, snap.Err
		}
		return snap.Sprint, nil
	}
	return tickets.FetchActiveSprint(cfg)
}

func (app *App) ListJiraBoards(cfg tickets.Config) ([]tickets.BoardInfo, error) {
	return tickets.ListBoards(cfg)
}

func (app *App) ListJiraIssueTypes(cfg tickets.Config) ([]tickets.IssueType, error) {
	return tickets.ListProjectIssueTypes(cfg)
}

func (app *App) CreateJiraIssue(cfg tickets.Config, in tickets.CreateIssueInput) (string, error) {
	return tickets.CreateIssue(cfg, in)
}

func (app *App) TransitionJiraIssue(cfg tickets.Config, issueKey string, targetStatusIDs []string) error {
	return tickets.TransitionIssue(cfg, issueKey, targetStatusIDs)
}

func (app *App) FetchJiraIssueDetail(cfg tickets.Config, issueKey string) (*tickets.IssueDetail, error) {
	return tickets.FetchIssueDetail(cfg, issueKey)
}

func (app *App) ListJiraIssueComments(cfg tickets.Config, issueKey string) ([]tickets.Comment, error) {
	return tickets.ListIssueComments(cfg, issueKey)
}

func (app *App) ListJiraIssueHistory(cfg tickets.Config, issueKey string) ([]tickets.HistoryEntry, error) {
	return tickets.ListIssueHistory(cfg, issueKey)
}

// TestMessagingProvider sends a test message via the configured messaging integration for a project.
func (app *App) TestMessagingProvider(providerType string, projectID string) error {
	project, err := app.store.GetProject(projectID)
	if err != nil {
		return err
	}
	if project == nil {
		return fmt.Errorf("project not found")
	}
	raw, ok := project.Integrations[providerType]
	if !ok {
		return fmt.Errorf("%s integration not configured", providerType)
	}
	strField := func(key string) string {
		if v, ok2 := raw[key]; ok2 {
			if s, ok3 := v.(string); ok3 {
				return s
			}
		}
		return ""
	}
	cfg := messaging.Config{
		Webhook: strField("webhook"),
		Token:   strField("token"),
		Channel: strField("channel"),
	}
	p, err := messaging.Factory(providerType, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(app.ctx, 5*time.Second)
	defer cancel()
	return p.Send(ctx, messaging.Message{
		Title: "Polaris Test Notification",
		Body:  fmt.Sprintf("Your %s integration is working correctly.", providerType),
		Color: "info",
		Tags:  []string{"test"},
	})
}

// TestSlackWebhook validates and tests a Slack webhook URL by sending a test message.
// Returns nil on success, error otherwise.
func (app *App) TestSlackWebhook(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	slack, err := messaging.NewSlack(webhookURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(app.ctx, 5*time.Second)
	defer cancel()
	return slack.Send(ctx, messaging.Message{
		Title: "Polaris Test Notification",
		Body:  "This is a test message from Polaris. Your Slack integration is working!",
		Color: "info",
		Tags:  []string{"test"},
	})
}

// SaveSlackConfig persists the Slack integration config for a project.
func (app *App) SaveSlackConfig(projectID, webhookURL string) error {
	if app.store == nil {
		return fmt.Errorf("store not ready")
	}
	if projectID == "" {
		return fmt.Errorf("projectId is required")
	}

	// Validate the config
	cfg := messaging.Config{Webhook: webhookURL}
	if err := messaging.ValidateConfig("slack", cfg); err != nil {
		return err
	}

	// Persist it in the project's integrations
	proj, err := app.store.GetProject(projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if proj == nil {
		return fmt.Errorf("project not found")
	}

	if proj.Integrations == nil {
		proj.Integrations = make(map[string]polaris.IntegrationConfig)
	}
	proj.Integrations["slack"] = polaris.IntegrationConfig{"webhook": webhookURL}

	// Save the updated project
	if _, err := app.store.UpsertProject(*proj); err != nil {
		return fmt.Errorf("save project: %w", err)
	}

	return nil
}

// GetSlackConfig retrieves the Slack integration config for a project, if set.
func (app *App) GetSlackConfig(projectID string) (map[string]any, error) {
	if app.store == nil {
		return nil, fmt.Errorf("store not ready")
	}
	proj, err := app.store.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if proj == nil || proj.Integrations == nil {
		return nil, nil
	}
	return proj.Integrations["slack"], nil
}

func (app *App) ListAutomations() ([]polaris.Automation, error) {
	return app.svc.ListAutomations()
}

func (app *App) ListAutomationRuns(automationID string, limit int) ([]polaris.AutomationRun, error) {
	return app.svc.ListAutomationRuns(automationID, limit)
}

func (app *App) UpsertAutomation(a polaris.Automation) (polaris.Automation, error) {
	saved, err := app.svc.UpsertAutomation(a)
	if err == nil && app.automation != nil {
		app.automation.Reschedule(saved.ID)
	}
	return saved, err
}

func (app *App) FireAutomation(id string) error {
	if app.automation == nil {
		return fmt.Errorf("automation manager not ready")
	}
	return app.automation.FireManual(id)
}

func (app *App) DeleteAutomation(id string) error {
	if err := app.svc.DeleteAutomation(id); err != nil {
		return err
	}
	if app.automation != nil {
		// Reschedule reads the store; after delete it cancels the ticker
		// without restarting since GetAutomation returns nil.
		app.automation.Reschedule(id)
	}
	return nil
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
	// scp-like: git@host:owner/repo or git@host:owner/repo.git
	if !strings.Contains(rawURL, "://") && strings.Contains(rawURL, ":") {
		colon := strings.LastIndexByte(rawURL, ':')
		rawURL = rawURL[colon+1:]
	}
	// strip query/fragment then get last path segment
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		rawURL = u.Path
	}
	name := filepath.Base(strings.TrimSuffix(rawURL, ".git"))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// --- code browser ---

type CodeEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "dist": true,
	"build": true, "__pycache__": true, ".cache": true, ".turbo": true,
	"coverage": true, ".nyc_output": true, "vendor": true, "target": true,
	".gradle": true, ".idea": true, ".vscode": true, "out": true,
	".venv": true, ".svn": true, ".hg": true, ".tox": true, ".yarn": true,
	".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true,
	".terraform": true, ".parcel-cache": true, ".gradle-cache": true,
}

func (app *App) ListProjectDir(projectPath, relDir string) ([]CodeEntry, error) {
	base, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, filepath.FromSlash(relDir))
	if !strings.HasPrefix(dir, base) {
		return nil, fmt.Errorf("path outside project")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []CodeEntry
	for _, e := range entries {
		name := e.Name()
		// Hidden files (.gitignore, .env, .dockerignore) are listable/mentionable;
		// hidden directories are traversed too unless they're known noise (.git,
		// .venv, …) — see skipDirs — so a .git tree doesn't flood the results.
		if e.IsDir() && skipDirs[name] {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(relDir, name))
		var size int64
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
		}
		out = append(out, CodeEntry{Name: name, Path: rel, IsDir: e.IsDir(), Size: size})
	}
	return out, nil
}

func (app *App) ReadProjectFile(projectPath, relPath string) (string, error) {
	base, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	full := filepath.Join(base, filepath.FromSlash(relPath))
	if !strings.HasPrefix(full, base) {
		return "", fmt.Errorf("path outside project")
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	const maxSize = 512 * 1024
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (app *App) WriteProjectFile(projectPath, relPath, content string) error {
	base, err := filepath.Abs(projectPath)
	if err != nil {
		return err
	}
	full := filepath.Join(base, filepath.FromSlash(relPath))
	if !strings.HasPrefix(full, base) {
		return fmt.Errorf("path outside project")
	}
	return os.WriteFile(full, []byte(content), 0644)
}

func (app *App) OpenDirectory(title string) (string, error) {
	if title == "" {
		title = "Choose project folder"
	}

	return wailsruntime.OpenDirectoryDialog(app.ctx, wailsruntime.OpenDialogOptions{
		Title: title,
	})
}

func (app *App) SendResendEmail(cfg resend.Config, input resend.SendInput) (string, error) {
	return resend.Send(cfg, input)
}

func (app *App) ListResendDomains(cfg resend.Config) ([]resend.Domain, error) {
	return resend.ListDomains(cfg)
}

func (app *App) ListResendEmails(cfg resend.Config, limit int) ([]resend.Email, error) {
	return resend.ListEmails(cfg, limit)
}

func (app *App) FetchSentryIssues(cfg sentry.Config) ([]sentry.Issue, error) {
	return sentry.FetchIssues(cfg)
}

func (app *App) FetchSentryLatestEvent(cfg sentry.Config, issueID string) (*sentry.EventDetail, error) {
	return sentry.FetchLatestEvent(cfg, issueID)
}

func (app *App) UpdateSentryIssueStatus(cfg sentry.Config, issueID string, status string) error {
	return sentry.UpdateIssueStatus(cfg, issueID, status)
}

// ListDokployProjectNames returns the project names on the instance so the
// config UI can autocomplete the project filter. This is a one-shot config-time
// lookup, so it calls the provider directly rather than the polling store.
func (app *App) ListDokployProjectNames(cfg dokploy.Config) ([]string, error) {
	projects, err := dokploy.FetchProjects(cfg)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return names, nil
}

// DokployDashboard is the read model for the integration page: every watched
// service (applications, compose stacks and databases) plus the recent build
// deployments of the deployable ones.
type DokployDashboard struct {
	Services    []dokploy.Service    `json:"services"`
	Deployments []dokploy.Deployment `json:"deployments"`
}

// GetDokployDashboard reads services and recent deployments through the shared
// dokployStore. The store owns the only poll loop and applies the project-name
// filter, so the dashboard and automations never double-call the Dokploy API.
func (app *App) GetDokployDashboard(cfg dokploy.Config, projects []string) (*DokployDashboard, error) {
	storeCfg := dokploystore.Config{BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Projects: projects}
	if app.dokployStore == nil {
		return nil, fmt.Errorf("dokploy store not ready")
	}
	snap, err := app.dokployStore.Reload(app.ctx, storeCfg)
	if err != nil {
		return nil, err
	}
	dashboard := &DokployDashboard{Services: snap.Services, Deployments: snap.Deployments}
	if snap.Err != nil {
		return dashboard, snap.Err
	}
	return dashboard, nil
}

// RunDokployAction triggers a lifecycle action (redeploy, start, stop) on a
// Dokploy application or compose service.
func (app *App) RunDokployAction(cfg dokploy.Config, svc dokploy.Service, action string) error {
	return dokploy.RunAction(cfg, svc, dokploy.Action(action))
}

// GetDokployDeploymentLogs returns the build log output for a specific deployment.
func (app *App) GetDokployDeploymentLogs(cfg dokploy.Config, deploymentID string, tail int) (string, error) {
	return dokploy.FetchDeploymentLogs(cfg, deploymentID, tail)
}

// GetDokployServiceLogs returns runtime container logs for an application service.
func (app *App) GetDokployServiceLogs(cfg dokploy.Config, svc dokploy.Service, tail int) (string, error) {
	return dokploy.FetchServiceLogs(cfg, svc, tail)
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
