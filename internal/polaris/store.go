package polaris

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/git"
	_ "modernc.org/sqlite"
)

// Emitter receives change notifications and forwards them to the frontend.
// Implementations typically wrap wailsruntime.EventsEmit.
type Emitter interface {
	Emit(event string, data ...any)
}

type Store struct {
	db      *sql.DB
	emitter Emitter
	emitMu  sync.Mutex
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // sqlite writers are serialized anyway; avoids "database is locked"
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (store *Store) Close() error {
	if store.db == nil {
		return nil
	}
	return store.db.Close()
}

// DB exposes the underlying sql.DB so peripheral stores (ghstore, jirastore)
// can share the same connection and migrations stay in this file.
func (store *Store) DB() *sql.DB {
	return store.db
}

func (store *Store) SetEmitter(e Emitter) {
	store.emitMu.Lock()
	defer store.emitMu.Unlock()
	store.emitter = e
}

func (store *Store) emit(collection string) {
	store.emitMu.Lock()
	e := store.emitter
	store.emitMu.Unlock()
	if e != nil {
		e.Emit("collection:" + collection + ":changed")
	}
}

// Emit forwards an arbitrary event to the frontend. Used by the Runner to push
// per-agent log lines directly, without round-tripping through the DB.
func (store *Store) Emit(event string, data ...any) {
	store.emitMu.Lock()
	e := store.emitter
	store.emitMu.Unlock()
	if e != nil {
		e.Emit(event, data...)
	}
}

func (store *Store) migrate() error {
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			color TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			logo TEXT NOT NULL DEFAULT '',
			integrations TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			startedAt INTEGER NOT NULL DEFAULT 0,
			tokens INTEGER NOT NULL DEFAULT 0,
			sessionId TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'manual',
			costUsd REAL NOT NULL DEFAULT 0,
			filesModified INTEGER NOT NULL DEFAULT 0,
			toolsUsed INTEGER NOT NULL DEFAULT 0,
			branch TEXT NOT NULL DEFAULT '',
			worktreePath TEXT NOT NULL DEFAULT '',
			issueKey TEXT NOT NULL DEFAULT '',
			prUrl TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			pendingQuestionId TEXT NOT NULL DEFAULT '',
			pendingQuestionInput TEXT NOT NULL DEFAULT '',
			worktreeJson TEXT NOT NULL DEFAULT '',
			pendingQuestionJson TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS agents_projectId_startedAt ON agents (projectId, startedAt DESC)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL DEFAULT '',
			agentId TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL DEFAULT 'info',
			payloadJson TEXT NOT NULL DEFAULT '{}',
			title TEXT NOT NULL DEFAULT '',
			createdAt INTEGER NOT NULL DEFAULT 0,
			read INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS notifications_createdAt ON notifications (createdAt DESC)`,
		`CREATE TABLE IF NOT EXISTS custom_providers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			color TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '',
			apiKey TEXT NOT NULL DEFAULT '',
			apiType TEXT NOT NULL DEFAULT '',
			models TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS automations (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT '',
			triggerJson TEXT NOT NULL DEFAULT '{}',
			spawnJson TEXT NOT NULL DEFAULT '{}',
			actionsJson TEXT NOT NULL DEFAULT '[]',
			pollIntervalSec INTEGER NOT NULL DEFAULT 60,
			snapshotJson TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS automations_projectId ON automations (projectId)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS provider_cache (
			provider TEXT NOT NULL,
			key TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}',
			fetchedAt INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (provider, key)
		)`,
		`CREATE TABLE IF NOT EXISTS automation_runs (
			id TEXT PRIMARY KEY,
			automationId TEXT NOT NULL,
			projectId TEXT NOT NULL DEFAULT '',
			startedAt INTEGER NOT NULL DEFAULT 0,
			outcome TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			detailsJson TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS automation_runs_automationId_startedAt ON automation_runs (automationId, startedAt DESC)`,
	}

	for _, q := range stmts {
		if _, err := store.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	// Additive column migrations for existing databases. SQLite errors with
	// "duplicate column name" when the column already exists; that's the
	// no-op case we silently ignore.
	additive := []string{
		`ALTER TABLE agents ADD COLUMN pendingQuestionId TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN pendingQuestionInput TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN worktreeJson TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN pendingQuestionJson TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN tokensInput INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensOutput INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensCacheCreate INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensCacheRead INTEGER NOT NULL DEFAULT 0`,
	}
	for _, q := range additive {
		if _, err := store.db.ExecContext(ctx, q); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	// Backfill the new JSON columns from the flat columns they will eventually
	// replace. Only touches rows where the JSON column is still empty so this
	// stays idempotent across restarts.
	backfill := []string{
		`UPDATE agents
		   SET worktreeJson = json_object('branch', branch, 'worktreePath', worktreePath, 'issueKey', issueKey, 'prUrl', prUrl)
		 WHERE worktreeJson = ''
		   AND (branch != '' OR worktreePath != '' OR issueKey != '' OR prUrl != '')`,
		`UPDATE agents
		   SET pendingQuestionJson = json_object('toolUseId', pendingQuestionId, 'input', json(pendingQuestionInput))
		 WHERE pendingQuestionJson = ''
		   AND pendingQuestionId != ''
		   AND pendingQuestionInput != ''`,
	}
	for _, q := range backfill {
		if _, err := store.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("backfill %q: %w", q, err)
		}
	}

	return nil
}

// --- projects ---

func (store *Store) ListProjects() ([]Project, error) {
	rows, err := store.db.Query(`SELECT id, name, color, path, logo, integrations FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		var integrations string
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.Path, &p.Logo, &integrations); err != nil {
			return nil, err
		}
		if integrations != "" {
			_ = json.Unmarshal([]byte(integrations), &p.Integrations)
		}
		if p.Integrations == nil {
			p.Integrations = map[string]IntegrationConfig{}
		}
		p.HasGit = git.IsRepo(p.Path)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (store *Store) GetProject(id string) (*Project, error) {
	row := store.db.QueryRow(`SELECT id, name, color, path, logo, integrations FROM projects WHERE id = ?`, id)
	var p Project
	var integrations string
	if err := row.Scan(&p.ID, &p.Name, &p.Color, &p.Path, &p.Logo, &integrations); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if integrations != "" {
		_ = json.Unmarshal([]byte(integrations), &p.Integrations)
	}
	if p.Integrations == nil {
		p.Integrations = map[string]IntegrationConfig{}
	}
	p.HasGit = git.IsRepo(p.Path)
	return &p, nil
}

func (store *Store) UpsertProject(p Project) (Project, error) {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Integrations == nil {
		p.Integrations = map[string]IntegrationConfig{}
	}
	integrations, err := json.Marshal(p.Integrations)
	if err != nil {
		return p, err
	}
	_, err = store.db.Exec(
		`INSERT INTO projects (id, name, color, path, logo, integrations) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, color=excluded.color, path=excluded.path, logo=excluded.logo, integrations=excluded.integrations`,
		p.ID, p.Name, p.Color, p.Path, p.Logo, string(integrations),
	)
	if err != nil {
		return p, err
	}
	p.HasGit = git.IsRepo(p.Path)
	store.emit("projects")
	return p, nil
}

func (store *Store) DeleteProject(id string) error {
	if _, err := store.db.Exec(`DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}
	store.emit("projects")
	return nil
}

// --- agents ---

func (store *Store) ListAgents(projectID string) ([]Agent, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if projectID == "" {
		rows, err = store.db.Query(`SELECT id, projectId, kind, summary, status, startedAt, tokens, tokensInput, tokensOutput, tokensCacheCreate, tokensCacheRead, sessionId, source, costUsd, filesModified, toolsUsed, branch, worktreePath, issueKey, prUrl, model, pendingQuestionId, pendingQuestionInput FROM agents ORDER BY startedAt DESC LIMIT 500`)
	} else {
		rows, err = store.db.Query(`SELECT id, projectId, kind, summary, status, startedAt, tokens, tokensInput, tokensOutput, tokensCacheCreate, tokensCacheRead, sessionId, source, costUsd, filesModified, toolsUsed, branch, worktreePath, issueKey, prUrl, model, pendingQuestionId, pendingQuestionInput FROM agents WHERE projectId = ? ORDER BY startedAt DESC LIMIT 500`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (store *Store) ListAgentsByStatus(status string) ([]Agent, error) {
	rows, err := store.db.Query(`SELECT id, projectId, kind, summary, status, startedAt, tokens, tokensInput, tokensOutput, tokensCacheCreate, tokensCacheRead, sessionId, source, costUsd, filesModified, toolsUsed, branch, worktreePath, issueKey, prUrl, model, pendingQuestionId, pendingQuestionInput FROM agents WHERE status = ?`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (store *Store) GetAgent(id string) (*Agent, error) {
	row := store.db.QueryRow(`SELECT id, projectId, kind, summary, status, startedAt, tokens, tokensInput, tokensOutput, tokensCacheCreate, tokensCacheRead, sessionId, source, costUsd, filesModified, toolsUsed, branch, worktreePath, issueKey, prUrl, model, pendingQuestionId, pendingQuestionInput FROM agents WHERE id = ?`, id)
	a, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (store *Store) UpsertAgent(a Agent) (Agent, error) {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.StartedAt == 0 {
		a.StartedAt = time.Now().Unix()
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	_, err := store.db.Exec(
		`INSERT INTO agents (id, projectId, kind, summary, status, startedAt, tokens, tokensInput, tokensOutput, tokensCacheCreate, tokensCacheRead, sessionId, source, costUsd, filesModified, toolsUsed, branch, worktreePath, issueKey, prUrl, model, pendingQuestionId, pendingQuestionInput) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET projectId=excluded.projectId, kind=excluded.kind, summary=excluded.summary, status=excluded.status, startedAt=excluded.startedAt, tokens=excluded.tokens, tokensInput=excluded.tokensInput, tokensOutput=excluded.tokensOutput, tokensCacheCreate=excluded.tokensCacheCreate, tokensCacheRead=excluded.tokensCacheRead, sessionId=excluded.sessionId, source=excluded.source, costUsd=excluded.costUsd, filesModified=excluded.filesModified, toolsUsed=excluded.toolsUsed, branch=excluded.branch, worktreePath=excluded.worktreePath, issueKey=excluded.issueKey, prUrl=excluded.prUrl, model=excluded.model, pendingQuestionId=excluded.pendingQuestionId, pendingQuestionInput=excluded.pendingQuestionInput`,
		a.ID, a.ProjectID, a.Kind, a.Summary, a.Status, a.StartedAt, a.Tokens, a.TokensInput, a.TokensOutput, a.TokensCacheCreate, a.TokensCacheRead, a.SessionID, a.Source, a.CostUSD, a.FilesModified, a.ToolsUsed, a.Branch, a.WorktreePath, a.IssueKey, a.PRURL, a.Model, a.PendingQuestionID, a.PendingQuestionInput,
	)
	if err != nil {
		return a, err
	}
	store.emit("agents")
	return a, nil
}

func (store *Store) PatchAgent(id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		switch k {
		case "projectId", "kind", "summary", "status", "startedAt", "tokens", "tokensInput", "tokensOutput", "tokensCacheCreate", "tokensCacheRead", "sessionId", "source", "costUsd", "filesModified", "toolsUsed", "branch", "worktreePath", "issueKey", "prUrl", "model", "pendingQuestionId", "pendingQuestionInput":
			cols = append(cols, k+" = ?")
			args = append(args, v)
		default:
			return fmt.Errorf("unknown agent field %q", k)
		}
	}
	args = append(args, id)
	q := "UPDATE agents SET "
	for i, c := range cols {
		if i > 0 {
			q += ", "
		}
		q += c
	}
	q += " WHERE id = ?"
	if _, err := store.db.Exec(q, args...); err != nil {
		return err
	}
	store.emit("agents")
	return nil
}

func (store *Store) DeleteAgent(id string) error {
	if _, err := store.db.Exec(`DELETE FROM agents WHERE id = ?`, id); err != nil {
		return err
	}
	store.emit("agents")
	return nil
}

// rowScanner abstracts *sql.Row and *sql.Rows so scanAgent works for both.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(r rowScanner) (Agent, error) {
	var a Agent
	err := r.Scan(&a.ID, &a.ProjectID, &a.Kind, &a.Summary, &a.Status, &a.StartedAt, &a.Tokens, &a.TokensInput, &a.TokensOutput, &a.TokensCacheCreate, &a.TokensCacheRead, &a.SessionID, &a.Source, &a.CostUSD, &a.FilesModified, &a.ToolsUsed, &a.Branch, &a.WorktreePath, &a.IssueKey, &a.PRURL, &a.Model, &a.PendingQuestionID, &a.PendingQuestionInput)
	if a.Source == "" {
		a.Source = "manual"
	}
	return a, err
}

// --- notifications ---

func (store *Store) ListNotifications() ([]Notification, error) {
	rows, err := store.db.Query(`SELECT id, projectId, agentId, kind, type, severity, payloadJson, title, createdAt, read FROM notifications ORDER BY createdAt DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func scanNotification(r rowScanner) (Notification, error) {
	var n Notification
	var read int
	var legacyAgentID, legacyKind, payloadJSON string
	if err := r.Scan(&n.ID, &n.ProjectID, &legacyAgentID, &legacyKind, &n.Type, &n.Severity, &payloadJSON, &n.Title, &n.CreatedAt, &read); err != nil {
		return n, err
	}
	n.Read = read != 0
	if payloadJSON != "" && payloadJSON != "{}" {
		_ = json.Unmarshal([]byte(payloadJSON), &n.Payload)
	}
	// Legacy rows persisted before the type/severity refactor only have the
	// flat `kind` column populated. Materialise type+severity+payload from it
	// so the new model sees a uniform shape across upgraded installs.
	if n.Type == "" {
		n.Type, n.Severity, n.Payload = migrateLegacyKind(legacyKind, legacyAgentID)
	}
	if n.Severity == "" {
		n.Severity = SeverityInfo
	}
	return n, nil
}

func migrateLegacyKind(kind, agentID string) (NotificationType, NotificationSeverity, map[string]any) {
	switch kind {
	case "waiting":
		return NotifTypeAgent, SeverityInfo, map[string]any{"agentId": agentID, "event": string(AgentEventWaiting)}
	case "completed":
		return NotifTypeAgent, SeveritySuccess, map[string]any{"agentId": agentID, "event": string(AgentEventCompleted)}
	case "error":
		// Pre-refactor "error" came from both runner agent failures and
		// automation rule failures; agentID is the discriminator.
		if agentID != "" {
			return NotifTypeAgent, SeverityError, map[string]any{"agentId": agentID, "event": string(AgentEventCompleted)}
		}
		return NotifTypeAutomation, SeverityError, map[string]any{}
	case "warning":
		return NotifTypeAutomation, SeverityWarning, map[string]any{}
	case "auto_spawn":
		return NotifTypeAutomation, SeverityInfo, map[string]any{}
	case "auto_action":
		return NotifTypeAutomation, SeveritySuccess, map[string]any{}
	case "info":
		return NotifTypeUser, SeverityInfo, map[string]any{}
	case "success":
		return NotifTypeUser, SeveritySuccess, map[string]any{}
	default:
		return NotifTypeUser, SeverityInfo, map[string]any{}
	}
}

func (store *Store) UpsertNotification(n Notification) (Notification, error) {
	if n.ID == "" {
		n.ID = newID()
	}
	if n.CreatedAt == 0 {
		n.CreatedAt = time.Now().Unix()
	}
	if n.Payload == nil {
		n.Payload = map[string]any{}
	}
	if n.Severity == "" {
		n.Severity = SeverityInfo
	}
	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		return n, err
	}
	read := 0
	if n.Read {
		read = 1
	}
	// Surface the agent ID into the legacy column as well so the existing
	// index/queries keep working until the column is dropped. Other type
	// payloads don't get a dedicated column.
	agentID := n.AgentID()
	_, err = store.db.Exec(
		`INSERT INTO notifications (id, projectId, agentId, kind, type, severity, payloadJson, title, createdAt, read) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET projectId=excluded.projectId, agentId=excluded.agentId, type=excluded.type, severity=excluded.severity, payloadJson=excluded.payloadJson, title=excluded.title, createdAt=excluded.createdAt, read=excluded.read`,
		n.ID, n.ProjectID, agentID, "", n.Type, n.Severity, string(payloadJSON), n.Title, n.CreatedAt, read,
	)
	if err != nil {
		return n, err
	}
	store.emit("notifications")
	return n, nil
}

func (store *Store) DeleteNotification(id string) error {
	if _, err := store.db.Exec(`DELETE FROM notifications WHERE id = ?`, id); err != nil {
		return err
	}
	store.emit("notifications")
	return nil
}

func (store *Store) MarkNotificationRead(id string) error {
	if _, err := store.db.Exec(`UPDATE notifications SET read = 1 WHERE id = ? AND read = 0`, id); err != nil {
		return err
	}
	store.emit("notifications")
	return nil
}

func (store *Store) MarkAllNotificationsRead() error {
	if _, err := store.db.Exec(`UPDATE notifications SET read = 1 WHERE read = 0`); err != nil {
		return err
	}
	store.emit("notifications")
	return nil
}

// --- app settings ---

const notificationSettingsKey = "notifications"

func (store *Store) GetNotificationSettings() (NotificationSettings, error) {
	row := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, notificationSettingsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultNotificationSettings(), nil
		}
		return DefaultNotificationSettings(), err
	}
	out := DefaultNotificationSettings()
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out, nil
}

func (store *Store) UpdateNotificationSettings(in NotificationSettings) (NotificationSettings, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	_, err = store.db.Exec(
		`INSERT INTO app_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		notificationSettingsKey, string(payload),
	)
	if err != nil {
		return in, err
	}
	store.emit("notificationSettings")
	return in, nil
}

const appearanceSettingsKey = "appearance"

func (store *Store) GetAppearanceSettings() (AppearanceSettings, error) {
	row := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, appearanceSettingsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultAppearanceSettings(), nil
		}
		return DefaultAppearanceSettings(), err
	}
	out := DefaultAppearanceSettings()
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	if out.CustomThemes == nil {
		out.CustomThemes = []CustomTheme{}
	}
	return out, nil
}

func (store *Store) UpdateAppearanceSettings(in AppearanceSettings) (AppearanceSettings, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	_, err = store.db.Exec(
		`INSERT INTO app_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		appearanceSettingsKey, string(payload),
	)
	if err != nil {
		return in, err
	}
	return in, nil
}

const shortcutsSettingsKey = "shortcuts"

func (store *Store) GetShortcutsSettings() (ShortcutsSettings, error) {
	row := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, shortcutsSettingsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultShortcutsSettings(), nil
		}
		return DefaultShortcutsSettings(), err
	}
	out := DefaultShortcutsSettings()
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out, nil
}

func (store *Store) UpdateShortcutsSettings(in ShortcutsSettings) (ShortcutsSettings, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	_, err = store.db.Exec(
		`INSERT INTO app_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		shortcutsSettingsKey, string(payload),
	)
	if err != nil {
		return in, err
	}
	return in, nil
}

const generalSettingsKey = "general"

func (store *Store) GetGeneralSettings() (GeneralSettings, error) {
	row := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, generalSettingsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultGeneralSettings(), nil
		}
		return DefaultGeneralSettings(), err
	}
	out := DefaultGeneralSettings()
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out, nil
}

func (store *Store) UpdateGeneralSettings(in GeneralSettings) (GeneralSettings, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	_, err = store.db.Exec(
		`INSERT INTO app_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		generalSettingsKey, string(payload),
	)
	if err != nil {
		return in, err
	}
	return in, nil
}

// --- custom providers ---

func (store *Store) ListCustomProviders() ([]CustomProvider, error) {
	rows, err := store.db.Query(`SELECT id, name, color, endpoint, apiKey, apiType, models FROM custom_providers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomProvider{}
	for rows.Next() {
		var p CustomProvider
		var models string
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.Endpoint, &p.APIKey, &p.APIType, &models); err != nil {
			return nil, err
		}
		if models != "" {
			_ = json.Unmarshal([]byte(models), &p.Models)
		}
		if p.Models == nil {
			p.Models = []string{}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (store *Store) UpsertCustomProvider(p CustomProvider) (CustomProvider, error) {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	models, err := json.Marshal(p.Models)
	if err != nil {
		return p, err
	}
	_, err = store.db.Exec(
		`INSERT INTO custom_providers (id, name, color, endpoint, apiKey, apiType, models) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, color=excluded.color, endpoint=excluded.endpoint, apiKey=excluded.apiKey, apiType=excluded.apiType, models=excluded.models`,
		p.ID, p.Name, p.Color, p.Endpoint, p.APIKey, p.APIType, string(models),
	)
	if err != nil {
		return p, err
	}
	store.emit("customProviders")
	return p, nil
}

func (store *Store) DeleteCustomProvider(id string) error {
	if _, err := store.db.Exec(`DELETE FROM custom_providers WHERE id = ?`, id); err != nil {
		return err
	}
	store.emit("customProviders")
	return nil
}

// --- automations ---

func (store *Store) ListAutomations() ([]Automation, error) {
	rows, err := store.db.Query(`SELECT id, projectId, name, enabled, source, triggerJson, spawnJson, actionsJson, pollIntervalSec, snapshotJson FROM automations ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Automation{}
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (store *Store) GetAutomation(id string) (*Automation, error) {
	row := store.db.QueryRow(`SELECT id, projectId, name, enabled, source, triggerJson, spawnJson, actionsJson, pollIntervalSec, snapshotJson FROM automations WHERE id = ?`, id)
	a, err := scanAutomation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (store *Store) UpsertAutomation(a Automation) (Automation, error) {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.PollIntervalSec <= 0 {
		a.PollIntervalSec = 60
	}
	if a.Actions == nil {
		a.Actions = []AutomationAction{}
	}
	triggerJSON, err := json.Marshal(a.Trigger)
	if err != nil {
		return a, err
	}
	actionsJSON, err := json.Marshal(a.Actions)
	if err != nil {
		return a, err
	}
	enabled := 0
	if a.Enabled {
		enabled = 1
	}
	_, err = store.db.Exec(
		`INSERT INTO automations (id, projectId, name, enabled, source, triggerJson, spawnJson, actionsJson, pollIntervalSec, snapshotJson) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET projectId=excluded.projectId, name=excluded.name, enabled=excluded.enabled, source=excluded.source, triggerJson=excluded.triggerJson, actionsJson=excluded.actionsJson, pollIntervalSec=excluded.pollIntervalSec, snapshotJson=excluded.snapshotJson`,
		a.ID, a.ProjectID, a.Name, enabled, a.Source, string(triggerJSON), "{}", string(actionsJSON), a.PollIntervalSec, a.SnapshotJSON,
	)
	if err != nil {
		return a, err
	}
	store.emit("automations")
	return a, nil
}

// PatchAutomationSnapshot only writes the snapshot column. The manager calls
// this every tick so we avoid round-tripping the rest of the row.
func (store *Store) PatchAutomationSnapshot(id, snapshotJSON string) error {
	if _, err := store.db.Exec(`UPDATE automations SET snapshotJson = ? WHERE id = ?`, snapshotJSON, id); err != nil {
		return err
	}
	// Skip event emission: snapshot churn would flood the frontend without value.
	return nil
}

func (store *Store) DeleteAutomation(id string) error {
	if _, err := store.db.Exec(`DELETE FROM automations WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := store.db.Exec(`DELETE FROM automation_runs WHERE automationId = ?`, id); err != nil {
		return err
	}
	store.emit("automations")
	store.emit("automationRuns")
	return nil
}

// InsertAutomationRun appends one audit-log row. The manager calls this
// after each fire so the UI can surface the per-automation history.
func (store *Store) InsertAutomationRun(run AutomationRun) (AutomationRun, error) {
	if run.ID == "" {
		run.ID = newID()
	}
	if run.StartedAt == 0 {
		run.StartedAt = time.Now().Unix()
	}
	if run.Actions == nil {
		run.Actions = []ActionResult{}
	}
	actionsJSON, err := json.Marshal(run.Actions)
	if err != nil {
		return run, err
	}
	if _, err := store.db.Exec(
		`INSERT INTO automation_runs (id, automationId, projectId, startedAt, outcome, reason, detailsJson) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.AutomationID, run.ProjectID, run.StartedAt, string(run.Outcome), run.Reason, string(actionsJSON),
	); err != nil {
		return run, err
	}
	store.emit("automationRuns")
	return run, nil
}

// ListAutomationRuns returns the most recent runs for an automation, newest
// first. limit defaults to 50 when <=0.
func (store *Store) ListAutomationRuns(automationID string, limit int) ([]AutomationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := store.db.Query(
		`SELECT id, automationId, projectId, startedAt, outcome, reason, detailsJson FROM automation_runs WHERE automationId = ? ORDER BY startedAt DESC LIMIT ?`,
		automationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutomationRun{}
	for rows.Next() {
		var (
			r           AutomationRun
			detailsJSON string
		)
		var outcome string
		if err := rows.Scan(&r.ID, &r.AutomationID, &r.ProjectID, &r.StartedAt, &outcome, &r.Reason, &detailsJSON); err != nil {
			return nil, err
		}
		r.Outcome = AutomationRunOutcome(outcome)
		if detailsJSON != "" {
			_ = json.Unmarshal([]byte(detailsJSON), &r.Actions)
		}
		if r.Actions == nil {
			r.Actions = []ActionResult{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResetAll wipes every user-owned row from the database (projects, agents,
// notifications, custom providers, automations, app settings) and emits change
// events for each collection so the UI refreshes.
func (store *Store) ResetAll() error {
	tables := []string{"projects", "agents", "notifications", "custom_providers", "automations", "automation_runs", "app_settings", "provider_cache"}
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range tables {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return fmt.Errorf("reset %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, name := range []string{"projects", "agents", "notifications", "customProviders", "automations", "notificationSettings"} {
		store.emit(name)
	}
	return nil
}

func scanAutomation(r rowScanner) (Automation, error) {
	var a Automation
	var enabled int
	var triggerJSON, spawnJSON, actionsJSON string
	if err := r.Scan(&a.ID, &a.ProjectID, &a.Name, &enabled, &a.Source, &triggerJSON, &spawnJSON, &actionsJSON, &a.PollIntervalSec, &a.SnapshotJSON); err != nil {
		return a, err
	}
	a.Enabled = enabled != 0
	if triggerJSON != "" {
		_ = json.Unmarshal([]byte(triggerJSON), &a.Trigger)
	}
	if actionsJSON != "" && actionsJSON != "[]" {
		_ = json.Unmarshal([]byte(actionsJSON), &a.Actions)
	}
	// Legacy: rows persisted before the actions-list refactor carry a single
	// spawn object in spawnJson. Materialise it as a one-element action list
	// so the rest of the code path sees a uniform shape.
	if len(a.Actions) == 0 && spawnJSON != "" && spawnJSON != "{}" {
		var legacy struct {
			AgentKind    string `json:"agentKind"`
			Model        string `json:"model"`
			TaskTemplate string `json:"taskTemplate"`
		}
		if err := json.Unmarshal([]byte(spawnJSON), &legacy); err == nil && legacy.AgentKind != "" {
			a.Actions = []AutomationAction{{
				Kind:         "spawn_agent",
				AgentKind:    legacy.AgentKind,
				Model:        legacy.Model,
				TaskTemplate: legacy.TaskTemplate,
			}}
		}
	}
	if a.Actions == nil {
		a.Actions = []AutomationAction{}
	}
	return a, nil
}

func newID() string {
	var b [7]byte
	_, _ = rand.Read(b[:])
	return "r" + hex.EncodeToString(b[:])
}
