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
	"github.com/KevinBonnoron/polaris/internal/providers/gh"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/providers/jira"
	"github.com/KevinBonnoron/polaris/internal/providers/nodejs"
	"github.com/KevinBonnoron/polaris/internal/store/ghstore"
	"github.com/KevinBonnoron/polaris/internal/store/jirastore"
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
	{"mistral", []string{"mistral"}},
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
	ctx        context.Context
	store      *polaris.Store
	svc        *polaris.Service
	automation *polaris.AutomationManager
	ghStore    *ghstore.Store
	jiraStore  *jirastore.Store
	nodeRunner *nodejs.Runner

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

	app.svc = polaris.NewService(store).
		WithRunner(polaris.NewRunner(logsDir, worktreesDir)).
		WithGhStore(app.ghStore).
		WithJiraStore(app.jiraStore)

	if err := app.svc.RecoverInterruptedAgents(); err != nil {
		log.Printf("polaris: recover interrupted agents: %v", err)
	}

	app.automation = polaris.NewAutomationManager(app.svc)
	if err := app.automation.Start(ctx); err != nil {
		log.Printf("polaris: start automation manager: %v", err)
	}

	app.nodeRunner = nodejs.NewRunner(wailsEmitter{ctx: ctx})

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
			for _, b := range c.binaries {
				if _, err := exec.LookPath(b); err == nil {
					in.Binary = b
					break
				}
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
		if bin, err := exec.LookPath("wl-paste"); err == nil {
			cmd := exec.Command(bin, "--type", "image/png")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
		}
		if bin, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command(bin, "-selection", "clipboard", "-t", "image/png", "-o")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
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

	if agent.WorktreePath != "" {
		if info, statErr := os.Stat(agent.WorktreePath); statErr == nil && info.IsDir() {
			return git.AgentDiff(agent.WorktreePath)
		}
	}

	if agent.Branch != "" {
		proj, projErr := app.store.GetProject(agent.ProjectID)
		if projErr == nil && proj != nil && proj.Path != "" {
			return git.BranchDiff(proj.Path, agent.Branch)
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

	if agent.WorktreePath != "" {
		if info, statErr := os.Stat(agent.WorktreePath); statErr == nil && info.IsDir() {
			// Dedicated worktree — everything in it belongs to this agent.
			return git.AgentFileStatuses(agent.WorktreePath, nil)
		}
	}

	scope := app.agentScopedPaths(agentID)
	if proj, projErr := app.store.GetProject(agent.ProjectID); projErr == nil && proj != nil && proj.Path != "" {
		scope = relativizePaths(proj.Path, scope)
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

// relativizePaths converts absolute paths under root to repo-relative form.
// Paths already relative pass through unchanged. Paths outside root are
// dropped — they can't belong to the current project anyway.
func relativizePaths(root string, paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		rel := p
		if filepath.IsAbs(p) {
			r, err := filepath.Rel(absRoot, p)
			if err != nil || strings.HasPrefix(r, "..") {
				continue
			}
			rel = r
		}
		rel = filepath.ToSlash(rel)
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	return out
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

func (app *App) UnstageAgentFile(agentID, path string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, []string{path})
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
	if agent.WorktreePath != "" {
		if info, statErr := os.Stat(agent.WorktreePath); statErr == nil && info.IsDir() {
			return agent.WorktreePath, nil
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
		scope = relativizePaths(proj.Path, scope)
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

	if agent.WorktreePath != "" {
		if info, statErr := os.Stat(agent.WorktreePath); statErr == nil && info.IsDir() {
			paths, pathErr := git.AgentChangedPaths(agent.WorktreePath)
			if pathErr != nil {
				return pathErr
			}
			if len(paths) == 0 {
				return fmt.Errorf("no changes to stage")
			}
			return git.StagePaths(agent.WorktreePath, paths)
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
			if paths := git.ExtractLogFilePaths(logContent); len(paths) > 0 {
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

func (app *App) DetectAgentClis() []AgentCli {
	out := make([]AgentCli, 0, len(agentCliCandidates))
	for _, c := range agentCliCandidates {
		entry := AgentCli{Kind: c.kind, Binary: c.binaries[0]}
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

func (app *App) DetectGitRemote(projectPath string) (*git.Remote, error) {
	return git.DetectRemote(projectPath)
}

func (app *App) DetectProviderToken(provider string) (*git.ProviderToken, error) {
	return git.DetectProviderToken(provider)
}

func (app *App) DetectNodeProject(projectPath string) (*nodejs.Project, error) {
	return nodejs.Detect(projectPath)
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

func (app *App) DetectDockerProject(projectPath string) (*docker.Project, error) {
	return docker.Detect(projectPath)
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

func (app *App) ListRepoPullRequests(owner, repo string) ([]gh.PullRequest, error) {
	if app.ghStore != nil {
		if err := app.ghStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.ghStore.GetPRs(app.ctx, owner, repo)
	}
	return gh.ListPullRequests(owner, repo)
}

func (app *App) ListRepoIssues(owner, repo string) ([]gh.Issue, error) {
	if app.ghStore != nil {
		if err := app.ghStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.ghStore.GetIssues(app.ctx, owner, repo)
	}
	return gh.ListIssues(owner, repo)
}

// ListRepoWorkflowRuns: page 1 goes through the cache so it can be shared
// with automation polling; deeper pages (loadMore) bypass the cache because
// the store only tracks the first page used for diff detection.
func (app *App) ListRepoWorkflowRuns(owner, repo string, page int) (*gh.WorkflowRunsPage, error) {
	if page > 1 {
		return gh.ListWorkflowRuns(owner, repo, page)
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
		return &gh.WorkflowRunsPage{Runs: runs, HasMore: len(runs) >= 30}, nil
	}
	return gh.ListWorkflowRuns(owner, repo, page)
}

func (app *App) ListRepoBranches(owner, repo string) ([]string, error) {
	return gh.ListBranches(owner, repo)
}

func (app *App) GetGhCurrentUser() (string, error) {
	return gh.GetCurrentUser()
}

func (app *App) GetRepoWorkflowDispatch(owner, repo string, workflowID int64) (*gh.WorkflowDispatchSpec, error) {
	return gh.GetWorkflowDispatch(owner, repo, workflowID)
}

func (app *App) TriggerRepoWorkflow(owner, repo string, workflowID int64, ref string, inputs map[string]string) error {
	return gh.TriggerWorkflowDispatch(owner, repo, workflowID, ref, inputs)
}

func (app *App) CancelRepoWorkflowRun(owner, repo string, runID int64) error {
	return gh.CancelWorkflowRun(owner, repo, runID)
}

func (app *App) RerunRepoWorkflowRun(owner, repo string, runID int64) error {
	return gh.RerunWorkflowRun(owner, repo, runID)
}

func (app *App) FetchJiraSprint(cfg jira.Config) (*jira.Sprint, error) {
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
	return jira.FetchActiveSprint(cfg)
}

func (app *App) ListJiraBoards(cfg jira.Config) ([]jira.BoardInfo, error) {
	return jira.ListBoards(cfg)
}

func (app *App) ListJiraIssueTypes(cfg jira.Config) ([]jira.IssueType, error) {
	return jira.ListProjectIssueTypes(cfg)
}

func (app *App) CreateJiraIssue(cfg jira.Config, in jira.CreateIssueInput) (string, error) {
	return jira.CreateIssue(cfg, in)
}

func (app *App) TransitionJiraIssue(cfg jira.Config, issueKey string, targetStatusIDs []string) error {
	return jira.TransitionIssue(cfg, issueKey, targetStatusIDs)
}

func (app *App) FetchJiraIssueDetail(cfg jira.Config, issueKey string) (*jira.IssueDetail, error) {
	return jira.FetchIssueDetail(cfg, issueKey)
}

func (app *App) ListJiraIssueComments(cfg jira.Config, issueKey string) ([]jira.Comment, error) {
	return jira.ListIssueComments(cfg, issueKey)
}

func (app *App) ListJiraIssueHistory(cfg jira.Config, issueKey string) ([]jira.HistoryEntry, error) {
	return jira.ListIssueHistory(cfg, issueKey)
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
		if strings.HasPrefix(name, ".") && name != ".env.example" {
			continue
		}
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
