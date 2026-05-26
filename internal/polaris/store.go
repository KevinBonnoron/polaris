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
			source TEXT NOT NULL DEFAULT 'manual',
			summary TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			startedAt INTEGER NOT NULL DEFAULT 0,
			pid INTEGER NOT NULL DEFAULT 0,
			sessionId TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			providerId TEXT NOT NULL DEFAULT '',
			tokensJson TEXT NOT NULL DEFAULT '',
			costUsd REAL NOT NULL DEFAULT 0,
			filesModified INTEGER NOT NULL DEFAULT 0,
			toolsUsed INTEGER NOT NULL DEFAULT 0,
			worktreeJson TEXT NOT NULL DEFAULT '',
			pendingQuestionJson TEXT NOT NULL DEFAULT '',
			allowedToolsJson TEXT NOT NULL DEFAULT ''
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
		// A permission/plan decision the user already made, keyed by the backend's
		// stable tool-call id (which survives a session reload, unlike the in-memory
		// perm-<n> id). On resume an ACP backend may re-request permission for a tool
		// already decided; we replay the recorded answer instead of re-prompting.
		`CREATE TABLE IF NOT EXISTS agent_decisions (
			agentId TEXT NOT NULL,
			toolCallId TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (agentId, toolCallId)
		)`,
	}

	for _, q := range stmts {
		if _, err := store.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	// Older databases carried the worktree and pending-question fields as a set
	// of flat columns (branch, worktreePath, issueKey, prUrl, pendingQuestionId,
	// pendingQuestionInput) appended in a historically-grown order by successive
	// ALTERs. The `branch` column only exists on those legacy tables; its
	// presence is the trigger to rebuild the table into the clean shape above,
	// folding the flat columns into worktreeJson / pendingQuestionJson.
	legacy, err := store.agentsHasColumn(ctx, "branch")
	if err != nil {
		return err
	}
	if legacy {
		if err := store.rebuildLegacyAgents(ctx); err != nil {
			return err
		}
	}

	additiveCurrent := []string{
		`ALTER TABLE agents ADD COLUMN allowedToolsJson TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN updatedAt INTEGER NOT NULL DEFAULT 0`,
		`UPDATE agents SET updatedAt = startedAt WHERE updatedAt = 0`,
		`ALTER TABLE custom_providers ADD COLUMN icon TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range additiveCurrent {
		if _, err := store.db.ExecContext(ctx, q); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	return nil
}

// agentsHasColumn reports whether the agents table has a column named col.
func (store *Store) agentsHasColumn(ctx context.Context, col string) (bool, error) {
	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(agents)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid         int
			name, ctype string
			notnull, pk int
			dflt        sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// rebuildLegacyAgents migrates a legacy agents table (flat worktree /
// pending-question columns) to the clean schema. It first brings any
// intermediate schema up to the full pre-rebuild column set, then rebuilds the
// table, folding the flat columns into the two JSON columns and dropping them.
// Runs at most once: afterwards the `branch` column is gone so the guard fails.
func (store *Store) rebuildLegacyAgents(ctx context.Context) error {
	additive := []string{
		`ALTER TABLE agents ADD COLUMN tokensInput INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensOutput INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensCacheCreate INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN tokensCacheRead INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN providerId TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN pid INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN pendingQuestionId TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN pendingQuestionInput TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range additive {
		if _, err := store.db.ExecContext(ctx, q); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	steps := []string{
		`CREATE TABLE agents_new (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'manual',
			summary TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			startedAt INTEGER NOT NULL DEFAULT 0,
			pid INTEGER NOT NULL DEFAULT 0,
			sessionId TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			providerId TEXT NOT NULL DEFAULT '',
			tokensJson TEXT NOT NULL DEFAULT '',
			costUsd REAL NOT NULL DEFAULT 0,
			filesModified INTEGER NOT NULL DEFAULT 0,
			toolsUsed INTEGER NOT NULL DEFAULT 0,
			worktreeJson TEXT NOT NULL DEFAULT '',
			pendingQuestionJson TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT INTO agents_new (id, projectId, kind, source, summary, status, startedAt, pid, sessionId, model, providerId, tokensJson, costUsd, filesModified, toolsUsed, worktreeJson, pendingQuestionJson)
		 SELECT id, projectId, kind, source, summary, status, startedAt, pid, sessionId, model, providerId,
		   CASE WHEN tokensInput = 0 AND tokensOutput = 0 AND tokensCacheCreate = 0 AND tokensCacheRead = 0 THEN ''
		        ELSE json_object('input', tokensInput, 'output', tokensOutput, 'cacheCreation', tokensCacheCreate, 'cacheRead', tokensCacheRead) END,
		   costUsd, filesModified, toolsUsed,
		   CASE WHEN branch = '' AND worktreePath = '' AND issueKey = '' AND prUrl = '' THEN ''
		        ELSE json_object('branch', branch, 'path', worktreePath, 'issueKey', issueKey, 'prUrl', prUrl) END,
		   CASE WHEN pendingQuestionId = '' OR pendingQuestionInput = '' THEN ''
		        ELSE json_object('toolUseId', pendingQuestionId, 'input', json(pendingQuestionInput)) END
		 FROM agents`,
		`DROP TABLE agents`,
		`ALTER TABLE agents_new RENAME TO agents`,
		`CREATE INDEX IF NOT EXISTS agents_projectId_startedAt ON agents (projectId, startedAt DESC)`,
	}
	for _, q := range steps {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rebuild agents %q: %w", q, err)
		}
	}
	return tx.Commit()
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

// agentColumns is the canonical SELECT column list for the agents table, kept
// in sync with scanAgent's Scan order.
const agentColumns = `id, projectId, kind, source, summary, status, startedAt, updatedAt, pid, sessionId, model, providerId, tokensJson, costUsd, filesModified, toolsUsed, worktreeJson, pendingQuestionJson, allowedToolsJson`

func (store *Store) ListAgents(projectID string) ([]Agent, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if projectID == "" {
		rows, err = store.db.Query(`SELECT ` + agentColumns + ` FROM agents ORDER BY startedAt DESC LIMIT 500`)
	} else {
		rows, err = store.db.Query(`SELECT `+agentColumns+` FROM agents WHERE projectId = ? ORDER BY startedAt DESC LIMIT 500`, projectID)
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
	rows, err := store.db.Query(`SELECT `+agentColumns+` FROM agents WHERE status = ?`, status)
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
	row := store.db.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE id = ?`, id)
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
	a.UpdatedAt = time.Now().Unix()
	if a.Source == "" {
		a.Source = "manual"
	}
	tokensJSON, err := marshalTokenUsage(a.Tokens)
	if err != nil {
		return a, err
	}
	worktreeJSON, err := marshalWorktree(a.Worktree)
	if err != nil {
		return a, err
	}
	pendingQuestionJSON, err := marshalPendingQuestion(a.PendingQuestion)
	if err != nil {
		return a, err
	}
	allowedToolsJSON, err := marshalAllowedTools(a.AllowedTools)
	if err != nil {
		return a, err
	}
	// pid is intentionally left out of the INSERT/UPDATE: it's owned by the
	// runner via PatchAgent, so a plain upsert must not reset a running agent's
	// process id back to 0.
	_, err = store.db.Exec(
		`INSERT INTO agents (id, projectId, kind, source, summary, status, startedAt, updatedAt, sessionId, model, providerId, tokensJson, costUsd, filesModified, toolsUsed, worktreeJson, pendingQuestionJson, allowedToolsJson) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET projectId=excluded.projectId, kind=excluded.kind, source=excluded.source, summary=excluded.summary, status=excluded.status, startedAt=excluded.startedAt, updatedAt=excluded.updatedAt, sessionId=excluded.sessionId, model=excluded.model, providerId=excluded.providerId, tokensJson=excluded.tokensJson, costUsd=excluded.costUsd, filesModified=excluded.filesModified, toolsUsed=excluded.toolsUsed, worktreeJson=excluded.worktreeJson, pendingQuestionJson=excluded.pendingQuestionJson, allowedToolsJson=excluded.allowedToolsJson`,
		a.ID, a.ProjectID, a.Kind, a.Source, a.Summary, a.Status, a.StartedAt, a.UpdatedAt, a.SessionID, a.Model, a.ProviderID, tokensJSON, a.CostUSD, a.FilesModified, a.ToolsUsed, worktreeJSON, pendingQuestionJSON, allowedToolsJSON,
	)
	if err != nil {
		return a, err
	}
	store.emit("agents")
	return a, nil
}

// marshalTokenUsage encodes a TokenUsage to its column form, using "" for the
// zero value so unused agents don't store a noisy all-zero object.
func marshalTokenUsage(t TokenUsage) (string, error) {
	if t == (TokenUsage{}) {
		return "", nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalWorktree encodes a Worktree to its column form, using "" for the zero
// value so empty worktrees don't store a noisy "{}".
func marshalWorktree(w Worktree) (string, error) {
	if w == (Worktree{}) {
		return "", nil
	}
	b, err := json.Marshal(w)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalAllowedTools(tools []string) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalPendingQuestion encodes a *PendingQuestion to its column form. A nil
// pointer (no pending question) maps to "".
func marshalPendingQuestion(q *PendingQuestion) (string, error) {
	if q == nil {
		return "", nil
	}
	b, err := json.Marshal(q)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (store *Store) PatchAgent(id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		switch k {
		case "projectId", "kind", "summary", "status", "startedAt", "sessionId", "source", "costUsd", "filesModified", "toolsUsed", "model", "providerId", "pid":
			cols = append(cols, k+" = ?")
			args = append(args, v)
		case "tokens":
			t, ok := v.(TokenUsage)
			if !ok {
				return fmt.Errorf("agent field %q expects TokenUsage", k)
			}
			s, err := marshalTokenUsage(t)
			if err != nil {
				return err
			}
			cols = append(cols, "tokensJson = ?")
			args = append(args, s)
		case "worktree":
			w, ok := v.(Worktree)
			if !ok {
				return fmt.Errorf("agent field %q expects Worktree", k)
			}
			s, err := marshalWorktree(w)
			if err != nil {
				return err
			}
			cols = append(cols, "worktreeJson = ?")
			args = append(args, s)
		case "pendingQuestion":
			// nil (untyped or a nil *PendingQuestion) clears the column.
			q, _ := v.(*PendingQuestion)
			s, err := marshalPendingQuestion(q)
			if err != nil {
				return err
			}
			cols = append(cols, "pendingQuestionJson = ?")
			args = append(args, s)
		default:
			return fmt.Errorf("unknown agent field %q", k)
		}
	}
	cols = append(cols, "updatedAt = ?")
	args = append(args, time.Now().Unix())
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
	_, _ = store.db.Exec(`DELETE FROM agent_decisions WHERE agentId = ?`, id)
	store.emit("agents")
	return nil
}

// RecordAgentDecision persists the user's answer (an option label, empty for
// cancelled) for a permission/plan request, keyed by the backend's stable
// tool-call id. Idempotent: a re-answer overwrites the prior label.
func (store *Store) RecordAgentDecision(agentID, toolCallID, label string) error {
	if agentID == "" || toolCallID == "" {
		return nil
	}
	_, err := store.db.Exec(
		`INSERT INTO agent_decisions (agentId, toolCallId, label) VALUES (?, ?, ?)
		 ON CONFLICT(agentId, toolCallId) DO UPDATE SET label = excluded.label`,
		agentID, toolCallID, label)
	return err
}

// ListAgentDecisions returns the recorded permission/plan answers for an agent,
// mapping tool-call id to the chosen option label (empty label = cancelled).
func (store *Store) ListAgentDecisions(agentID string) (map[string]string, error) {
	rows, err := store.db.Query(`SELECT toolCallId, label FROM agent_decisions WHERE agentId = ?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		out[id] = label
	}
	return out, rows.Err()
}

// rowScanner abstracts *sql.Row and *sql.Rows so scanAgent works for both.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(r rowScanner) (Agent, error) {
	var a Agent
	var tokensJSON, worktreeJSON, pendingQuestionJSON, allowedToolsJSON string
	err := r.Scan(&a.ID, &a.ProjectID, &a.Kind, &a.Source, &a.Summary, &a.Status, &a.StartedAt, &a.UpdatedAt, &a.PID, &a.SessionID, &a.Model, &a.ProviderID, &tokensJSON, &a.CostUSD, &a.FilesModified, &a.ToolsUsed, &worktreeJSON, &pendingQuestionJSON, &allowedToolsJSON)
	if err != nil {
		return a, err
	}
	if tokensJSON != "" {
		_ = json.Unmarshal([]byte(tokensJSON), &a.Tokens)
	}
	if worktreeJSON != "" {
		_ = json.Unmarshal([]byte(worktreeJSON), &a.Worktree)
	}
	if pendingQuestionJSON != "" {
		a.PendingQuestion = &PendingQuestion{}
		if uerr := json.Unmarshal([]byte(pendingQuestionJSON), a.PendingQuestion); uerr != nil {
			a.PendingQuestion = nil
		}
	}
	if allowedToolsJSON != "" {
		_ = json.Unmarshal([]byte(allowedToolsJSON), &a.AllowedTools)
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	return a, nil
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

const agentDefaultModelsKey = "agentDefaultModels"

func (store *Store) GetAgentDefaultModels() (map[string]string, error) {
	row := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, agentDefaultModelsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out, nil
}

func (store *Store) SetAgentDefaultModel(id, model string) error {
	current, err := store.GetAgentDefaultModels()
	if err != nil {
		return err
	}
	if model == "" {
		delete(current, id)
	} else {
		current[id] = model
	}
	payload, err := json.Marshal(current)
	if err != nil {
		return err
	}
	if _, err := store.db.Exec(
		`INSERT INTO app_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		agentDefaultModelsKey, string(payload),
	); err != nil {
		return err
	}
	store.emit("agentDefaultModels")
	return nil
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
	if out.StatusBarBlocks == nil {
		out.StatusBarBlocks = DefaultAppearanceSettings().StatusBarBlocks
	}
	if out.CardAnimationStyle == "" {
		out.CardAnimationStyle = DefaultAppearanceSettings().CardAnimationStyle
	}
	if out.GitChangesViewMode == "" {
		out.GitChangesViewMode = DefaultAppearanceSettings().GitChangesViewMode
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
	rows, err := store.db.Query(`SELECT id, name, color, icon, endpoint, apiKey, apiType, models FROM custom_providers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomProvider{}
	for rows.Next() {
		var p CustomProvider
		var models string
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.Icon, &p.Endpoint, &p.APIKey, &p.APIType, &models); err != nil {
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

func (store *Store) GetCustomProvider(id string) (*CustomProvider, error) {
	var p CustomProvider
	var models string
	err := store.db.QueryRow(`SELECT id, name, color, icon, endpoint, apiKey, apiType, models FROM custom_providers WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Color, &p.Icon, &p.Endpoint, &p.APIKey, &p.APIType, &models)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if models != "" {
		_ = json.Unmarshal([]byte(models), &p.Models)
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	return &p, nil
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
		`INSERT INTO custom_providers (id, name, color, icon, endpoint, apiKey, apiType, models) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, color=excluded.color, icon=excluded.icon, endpoint=excluded.endpoint, apiKey=excluded.apiKey, apiType=excluded.apiType, models=excluded.models`,
		p.ID, p.Name, p.Color, p.Icon, p.Endpoint, p.APIKey, p.APIType, string(models),
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
