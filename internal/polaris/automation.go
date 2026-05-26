package polaris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
	"github.com/KevinBonnoron/polaris/internal/providers/gh"
	"github.com/KevinBonnoron/polaris/internal/providers/jira"
	"github.com/KevinBonnoron/polaris/internal/providers/resend"
	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
	"github.com/KevinBonnoron/polaris/internal/store/dokploystore"
	"github.com/KevinBonnoron/polaris/internal/store/ghstore"
	"github.com/KevinBonnoron/polaris/internal/store/jirastore"
	"github.com/KevinBonnoron/polaris/internal/store/sentrystore"
)

// jiraKeyPattern matches typical Jira issue keys (e.g. PROJ-123).
var jiraKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]+-\d+`)

func extractJiraKey(candidates ...string) string {
	for _, s := range candidates {
		if s == "" {
			continue
		}
		if m := jiraKeyPattern.FindString(strings.ToUpper(s)); m != "" {
			return m
		}
	}
	return ""
}

// fireContext bundles all per-fire data so each action kind can pull what
// it needs from one place. Fields outside the trigger's source remain zero.
type fireContext struct {
	Automation Automation
	Vars       map[string]string

	// Optional context payloads (set depending on trigger source/kind).
	JiraIssueKey string
	JiraIssue    *jira.Issue
	JiraCfg      *jira.Config

	PR  *gh.PullRequest
	Run *gh.WorkflowRun

	Deployment *dokploy.Deployment

	// Notification label that frames the chain in user-visible logs.
	NotifLabel string
}

// AutomationManager subscribes to the shared gh and jira stores and fires
// automation actions when the diff carries a trigger-matching change. It no
// longer owns its own tickers: cadence is set per-integration on the store,
// and a single poll services every automation and every UI screen watching
// the same (owner, repo) or (jira board) key.
type AutomationManager struct {
	svc *Service

	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	ghUnsub      func()
	jiraUnsub    func()
	sentryUnsub  func()
	dokployUnsub func()
	ghLogin      string
	ghLooked     bool
}

func NewAutomationManager(svc *Service) *AutomationManager {
	return &AutomationManager{svc: svc}
}

// fetchPRs reads PRs from the shared gh store when available so the same
// poll cycle benefits the UI and other automations on the same repo.
func (automationManager *AutomationManager) fetchPRs(ctx context.Context, owner, repo string) ([]gh.PullRequest, error) {
	if s := automationManager.svc.ghStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetPRs(ctx, owner, repo)
	}
	return gh.ListPullRequests(owner, repo)
}

func (automationManager *AutomationManager) fetchIssues(ctx context.Context, owner, repo string) ([]gh.Issue, error) {
	if s := automationManager.svc.ghStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetIssues(ctx, owner, repo)
	}
	return gh.ListIssues(owner, repo)
}

func (automationManager *AutomationManager) fetchRuns(ctx context.Context, owner, repo string) ([]gh.WorkflowRun, error) {
	if s := automationManager.svc.ghStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetRuns(ctx, owner, repo)
	}
	page, err := gh.ListWorkflowRuns(owner, repo, 1)
	if err != nil {
		return nil, err
	}
	return page.Runs, nil
}

func (automationManager *AutomationManager) fetchSprint(ctx context.Context, cfg jira.Config) (*jira.Sprint, error) {
	if s := automationManager.svc.jiraStore; s != nil {
		k := jirastore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
		s.SetConfig(k, cfg)
		if err := s.Refresh(ctx, k); err != nil {
			return nil, err
		}
		snap, err := s.GetSnapshot(ctx, k, cfg)
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

// Start subscribes to the shared stores and registers each enabled
// automation so its integration is polled at the configured cadence. Call
// once at app startup, after the stores are wired into the Service.
func (automationManager *AutomationManager) Start(ctx context.Context) error {
	automationManager.mu.Lock()
	if automationManager.cancel != nil {
		automationManager.mu.Unlock()
		return nil
	}
	automationManager.ctx, automationManager.cancel = context.WithCancel(ctx)
	if s := automationManager.svc.ghStore; s != nil {
		automationManager.ghUnsub = s.Subscribe(automationManager.onGhDiff)
	}
	if s := automationManager.svc.jiraStore; s != nil {
		automationManager.jiraUnsub = s.Subscribe(automationManager.onJiraDiff)
	}
	if s := automationManager.svc.sentryStore; s != nil {
		automationManager.sentryUnsub = s.Subscribe(automationManager.onSentryDiff)
	}
	if s := automationManager.svc.dokployStore; s != nil {
		automationManager.dokployUnsub = s.Subscribe(automationManager.onDokployDiff)
	}
	parent := automationManager.ctx
	automationManager.mu.Unlock()

	automations, err := automationManager.svc.ListAutomations()
	if err != nil {
		return err
	}
	for _, a := range automations {
		if a.Enabled {
			automationManager.registerWithStores(parent, a, automationManager.intervalFor(a))
		}
	}
	return nil
}

// Stop tears down store subscriptions and per-automation registrations.
// Safe to call multiple times.
func (automationManager *AutomationManager) Stop() {
	automationManager.mu.Lock()
	if automationManager.cancel != nil {
		automationManager.cancel()
		automationManager.cancel = nil
	}
	ghUnsub := automationManager.ghUnsub
	jiraUnsub := automationManager.jiraUnsub
	sentryUnsub := automationManager.sentryUnsub
	dokployUnsub := automationManager.dokployUnsub
	automationManager.ghUnsub = nil
	automationManager.jiraUnsub = nil
	automationManager.sentryUnsub = nil
	automationManager.dokployUnsub = nil
	automationManager.mu.Unlock()
	if ghUnsub != nil {
		ghUnsub()
	}
	if jiraUnsub != nil {
		jiraUnsub()
	}
	if sentryUnsub != nil {
		sentryUnsub()
	}
	if dokployUnsub != nil {
		dokployUnsub()
	}
	automations, _ := automationManager.svc.ListAutomations()
	for _, a := range automations {
		automationManager.unregisterFromStores(a)
	}
}

// Reschedule reflects an automation upsert or delete into the relevant
// store registration so its polling cadence stays current. A disabled or
// deleted automation is unregistered; an enabled one is (re)registered.
func (automationManager *AutomationManager) Reschedule(automationID string) {
	automationManager.mu.Lock()
	parent := automationManager.ctx
	automationManager.mu.Unlock()
	if parent == nil {
		return
	}

	if prev, _ := automationManager.svc.store.GetAutomation(automationID); prev != nil {
		automationManager.unregisterFromStores(*prev)
	}

	a, err := automationManager.svc.store.GetAutomation(automationID)
	if err != nil || a == nil || !a.Enabled {
		return
	}
	automationManager.registerWithStores(parent, *a, automationManager.intervalFor(*a))
}

// intervalFor reads the polling cadence from the relevant integration
// config. The per-automation PollIntervalSec is no longer consulted — it's
// kept on the struct only for the deprecated DB column.
func (automationManager *AutomationManager) intervalFor(a Automation) time.Duration {
	const fallback = 60 * time.Second
	project, err := automationManager.svc.store.GetProject(a.ProjectID)
	if err != nil || project == nil {
		return fallback
	}
	cfg := project.Integrations[a.Source]
	if cfg == nil && a.Source == "repository" {
		cfg = project.Integrations["repository"]
	}
	if cfg == nil {
		return fallback
	}
	secs := int64Field(cfg, "pollIntervalSec")
	if secs <= 0 {
		return fallback
	}
	d := time.Duration(secs) * time.Second
	if d < 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

func (automationManager *AutomationManager) registerWithStores(ctx context.Context, a Automation, interval time.Duration) {
	switch a.Source {
	case "jira":
		cfg, ok := automationManager.jiraConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.jiraStore; s != nil {
			k := jirastore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
			s.Register(ctx, k, cfg, "automation:"+a.ID, interval)
		}
	case "repository":
		cfg, ok := automationManager.repoConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.ghStore; s != nil {
			s.Register(ctx, cfg.Owner, cfg.Repo, "automation:"+a.ID, interval)
		}
	case "sentry":
		cfg, ok := automationManager.sentryConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.sentryStore; s != nil {
			s.Register(ctx, sentrystore.KeyFor(cfg), cfg, "automation:"+a.ID, interval)
		}
	case "dokploy":
		cfg, ok := automationManager.dokployConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.dokployStore; s != nil {
			s.Register(ctx, dokploystore.KeyFor(cfg), cfg, "automation:"+a.ID, interval)
		}
	}
}

func (automationManager *AutomationManager) unregisterFromStores(a Automation) {
	switch a.Source {
	case "jira":
		if s := automationManager.svc.jiraStore; s != nil {
			if cfg, ok := automationManager.jiraConfigFor(a.ProjectID); ok {
				k := jirastore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
				s.Unregister(k, "automation:"+a.ID)
			}
		}
	case "repository":
		if s := automationManager.svc.ghStore; s != nil {
			if cfg, ok := automationManager.repoConfigFor(a.ProjectID); ok {
				s.Unregister(cfg.Owner, cfg.Repo, "automation:"+a.ID)
			}
		}
	case "sentry":
		if s := automationManager.svc.sentryStore; s != nil {
			if cfg, ok := automationManager.sentryConfigFor(a.ProjectID); ok {
				s.Unregister(sentrystore.KeyFor(cfg), "automation:"+a.ID)
			}
		}
	case "dokploy":
		if s := automationManager.svc.dokployStore; s != nil {
			if cfg, ok := automationManager.dokployConfigFor(a.ProjectID); ok {
				s.Unregister(dokploystore.KeyFor(cfg), "automation:"+a.ID)
			}
		}
	}
}

func hasSnapshot(a Automation) bool {
	return strings.TrimSpace(a.SnapshotJSON) != "" && a.SnapshotJSON != "{}"
}

// issueState is what we remember per issue between ticks.
type issueState struct {
	StatusID string `json:"s"`
	Assignee string `json:"a"`
}

type snapshot map[string]issueState

func parseSnapshot(s string) snapshot {
	if s == "" {
		return snapshot{}
	}
	var out snapshot
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return snapshot{}
	}
	return out
}

func (s snapshot) encode() string {
	buf, _ := json.Marshal(s)
	return string(buf)
}

// onJiraDiff fires every jira.transition automation whose project's Jira
// board matches the diff's key. Cold-cache diffs (no previous snapshot) are
// suppressed so we never fire on the initial seed.
func (automationManager *AutomationManager) onJiraDiff(diff jirastore.Diff) {
	if diff.Before.FetchedAt.IsZero() {
		return
	}
	if diff.After.Sprint == nil {
		return
	}
	automations, err := automationManager.svc.ListAutomations()
	if err != nil {
		return
	}

	statusNames := map[string]string{}
	for _, col := range diff.After.Sprint.Columns {
		for _, id := range col.StatusIDs {
			statusNames[id] = col.Name
		}
	}
	for _, issue := range diff.After.Sprint.Issues {
		if _, ok := statusNames[issue.StatusID]; !ok {
			statusNames[issue.StatusID] = issue.Status
		}
	}

	prevState := map[string]issueState{}
	if diff.Before.Sprint != nil {
		for _, i := range diff.Before.Sprint.Issues {
			ak := i.AssigneeEmail
			if ak == "" {
				ak = i.Assignee
			}
			prevState[i.Key] = issueState{StatusID: i.StatusID, Assignee: ak}
		}
	}

	for _, a := range automations {
		if !a.Enabled || a.Source != "jira" || a.Trigger.Kind != "jira.transition" {
			continue
		}
		cfg, ok := automationManager.jiraConfigFor(a.ProjectID)
		if !ok {
			continue
		}
		k := jirastore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
		if k != diff.Key {
			continue
		}
		for _, issue := range diff.After.Sprint.Issues {
			old, hadPrev := prevState[issue.Key]
			if !matchesTrigger(a.Trigger, cfg.Email, issue, old, hadPrev) {
				continue
			}
			fromName := statusNames[old.StatusID]
			automationManager.fire(a, cfg, issue, fromName)
		}
	}
}

func (automationManager *AutomationManager) jiraConfigFor(projectID string) (jira.Config, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return jira.Config{}, false
	}
	raw, ok := project.Integrations["jira"]
	if !ok {
		return jira.Config{}, false
	}
	cfg := jira.Config{
		BaseURL:    stringField(raw, "baseUrl"),
		Email:      stringField(raw, "email"),
		Token:      stringField(raw, "token"),
		ProjectKey: stringField(raw, "projectKey"),
		BoardID:    int64Field(raw, "boardId"),
	}
	if cfg.BaseURL == "" || cfg.Token == "" {
		return jira.Config{}, false
	}
	return cfg, true
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func int64Field(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}

// matchesTrigger returns true when the issue's transition matches every
// constraint of the rule. It assumes only jira.transition triggers for now.
func matchesTrigger(t AutomationTrigger, myEmail string, issue jira.Issue, prev issueState, hadPrev bool) bool {
	if t.Kind != "jira.transition" {
		return false
	}
	if t.ToStatusID != "" && issue.StatusID != t.ToStatusID {
		return false
	}

	assigneeKey := issue.AssigneeEmail
	if assigneeKey == "" {
		assigneeKey = issue.Assignee
	}
	if !matchesAssignee(t.Assignee, myEmail, assigneeKey, issue.Assignee) {
		return false
	}

	// Issue never seen before: treat as "transitioned from nothing" → fire if
	// no from-filter, otherwise skip (we can't know which from to claim).
	if !hadPrev {
		return len(t.FromStatusIDs) == 0
	}

	statusChanged := prev.StatusID != issue.StatusID
	assigneeChanged := prev.Assignee != assigneeKey

	if statusChanged {
		if len(t.FromStatusIDs) > 0 && !contains(t.FromStatusIDs, prev.StatusID) {
			return false
		}
		return true
	}

	// Status unchanged: only fires when the rule explicitly accepts reassignments
	// AND the assignee just became the configured one.
	if assigneeChanged && t.AlsoOnReassignment {
		return true
	}
	return false
}

func matchesAssignee(filter, myEmail, assigneeKey, displayName string) bool {
	switch filter {
	case "", "any":
		return true
	case "me":
		return myEmail != "" && strings.EqualFold(assigneeKey, myEmail)
	default:
		return strings.EqualFold(assigneeKey, filter) || strings.EqualFold(displayName, filter)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (automationManager *AutomationManager) fire(a Automation, cfg jira.Config, issue jira.Issue, fromStatusName string) {
	// Pull the last comment lazily so the placeholder is only meaningful when
	// the template asks for it.
	lastComment := ""
	if anyActionMentions(a.Actions, "{{lastComment}}") {
		if body, err := jira.FetchLastComment(cfg, issue.Key); err == nil {
			lastComment = body
		}
	}

	fc := fireContext{
		Automation:   a,
		JiraIssueKey: issue.Key,
		JiraIssue:    &issue,
		JiraCfg:      &cfg,
		NotifLabel:   issue.Key,
		Vars: map[string]string{
			"key":         issue.Key,
			"summary":     issue.Summary,
			"fromStatus":  fromStatusName,
			"toStatus":    issue.Status,
			"assignee":    issue.Assignee,
			"url":         issue.URL,
			"lastComment": lastComment,
		},
	}
	automationManager.runActions(fc)
}

func renderTemplate(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

type repoConfig struct {
	Owner string
	Repo  string
}

func (automationManager *AutomationManager) repoConfigFor(projectID string) (repoConfig, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return repoConfig{}, false
	}
	raw, ok := project.Integrations["repository"]
	if !ok {
		return repoConfig{}, false
	}
	cfg := repoConfig{
		Owner: stringField(raw, "owner"),
		Repo:  stringField(raw, "repo"),
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return repoConfig{}, false
	}
	return cfg, true
}

type prSnapshot map[string]struct{}

func parsePRSnapshot(s string) prSnapshot {
	if s == "" {
		return prSnapshot{}
	}
	var keys []string
	if err := json.Unmarshal([]byte(s), &keys); err != nil {
		return prSnapshot{}
	}
	out := make(prSnapshot, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out
}

func (s prSnapshot) encode() string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	buf, _ := json.Marshal(keys)
	return string(buf)
}

// onGhDiff fires every repository-source automation whose project points at
// the diff's (owner, repo). Cold-cache diffs (no previous snapshot) are
// suppressed so we never fire on the initial seed.
func (automationManager *AutomationManager) onGhDiff(diff ghstore.Diff) {
	if diff.Before.FetchedAt.IsZero() {
		return
	}
	automations, err := automationManager.svc.ListAutomations()
	if err != nil {
		return
	}
	for _, a := range automations {
		if !a.Enabled || a.Source != "repository" {
			continue
		}
		cfg, ok := automationManager.repoConfigFor(a.ProjectID)
		if !ok || cfg.Owner != diff.Owner || cfg.Repo != diff.Repo {
			continue
		}
		switch a.Trigger.Kind {
		case "repository.pr_opened":
			automationManager.dispatchPROpened(a, cfg, diff)
		case "repository.pr_comment":
			automationManager.dispatchPRComments(a, cfg, diff)
		case "repository.pr_build_failed":
			automationManager.dispatchPRBuildResults(a, cfg, diff, "failure")
		case "repository.pr_build_success":
			automationManager.dispatchPRBuildResults(a, cfg, diff, "success")
		case "repository.issue_assigned":
			automationManager.dispatchIssueAssigned(a, cfg, diff)
		default:
			log.Printf("automation %s: unsupported repository trigger %q", a.ID, a.Trigger.Kind)
		}
	}
}

func (automationManager *AutomationManager) dispatchPROpened(a Automation, cfg repoConfig, diff ghstore.Diff) {
	for _, pr := range diff.PRsOpened {
		if matchesRepoTrigger(a.Trigger, pr) {
			automationManager.firePR(a, cfg, pr)
		}
	}
}

func (automationManager *AutomationManager) dispatchPRBuildResults(a Automation, cfg repoConfig, diff ghstore.Diff, conclusion string) {
	var me string
	if a.Trigger.OnlyMine {
		me = automationManager.currentGHLogin()
		if me == "" {
			return
		}
	}
	prByNumber := make(map[int]gh.PullRequest, len(diff.After.PRs))
	for _, pr := range diff.After.PRs {
		prByNumber[pr.Number] = pr
	}
	for _, run := range diff.RunsCompleted {
		if run.Conclusion != conclusion || len(run.PRNumbers) == 0 {
			continue
		}
		for _, prNum := range run.PRNumbers {
			pr, ok := prByNumber[prNum]
			if !ok {
				continue
			}
			if a.Trigger.OnlyMine && !strings.EqualFold(pr.Author, me) {
				continue
			}
			automationManager.firePRBuildResult(a, cfg, pr, run)
		}
	}
}

func (automationManager *AutomationManager) dispatchIssueAssigned(a Automation, cfg repoConfig, diff ghstore.Diff) {
	var me string
	if a.Trigger.OnlyMine {
		me = automationManager.currentGHLogin()
		if me == "" {
			return
		}
	}
	for _, ev := range diff.IssuesAssigned {
		if a.Trigger.OnlyMine && !strings.EqualFold(ev.Assignee, me) {
			continue
		}
		automationManager.fireIssueAssigned(a, cfg, ev.Issue, ev.Assignee)
	}
}

// currentGHLogin caches the result of `gh api /user` — it only changes when
// the user re-authenticates, so once-per-process is enough.
func (automationManager *AutomationManager) currentGHLogin() string {
	automationManager.mu.Lock()
	if automationManager.ghLooked {
		login := automationManager.ghLogin
		automationManager.mu.Unlock()
		return login
	}
	automationManager.mu.Unlock()

	login, err := gh.GetCurrentUser()
	if err != nil {
		log.Printf("automation: gh current user: %v", err)
	}
	automationManager.mu.Lock()
	automationManager.ghLogin = login
	automationManager.ghLooked = true
	automationManager.mu.Unlock()
	return login
}

// dispatchPRComments still needs its own per-PR comment fetch — the store
// doesn't cache PR comments — but it piggy-backs on the store's tick
// cadence (this handler is only invoked when the store emits a diff for
// the repo). A per-automation snapshot tracks already-seen comment IDs so
// firings are deduplicated across diffs.
func (automationManager *AutomationManager) dispatchPRComments(a Automation, cfg repoConfig, diff ghstore.Diff) {
	me := automationManager.currentGHLogin()
	if me == "" {
		return
	}

	fresh, err := automationManager.svc.store.GetAutomation(a.ID)
	if err != nil || fresh == nil {
		return
	}
	prev := parsePRSnapshot(fresh.SnapshotJSON)
	next := prSnapshot{}
	seedOnly := !hasSnapshot(*fresh)

	type pendingComment struct {
		pr      gh.PullRequest
		comment gh.IssueComment
	}
	var fires []pendingComment

	for _, pr := range diff.After.PRs {
		if !strings.EqualFold(pr.Author, me) {
			continue
		}
		comments, err := gh.ListIssueComments(cfg.Owner, cfg.Repo, pr.Number)
		if err != nil {
			log.Printf("automation %s: list comments for #%d: %v", a.ID, pr.Number, err)
			continue
		}
		for _, c := range comments {
			key := fmt.Sprintf("%d:%d", pr.Number, c.ID)
			next[key] = struct{}{}
			if seedOnly {
				continue
			}
			if _, seen := prev[key]; seen {
				continue
			}
			if a.Trigger.ExcludeOwnComments && strings.EqualFold(c.Author, me) {
				continue
			}
			fires = append(fires, pendingComment{pr: pr, comment: c})
		}
	}

	if err := automationManager.svc.store.PatchAutomationSnapshot(a.ID, next.encode()); err != nil {
		log.Printf("automation %s: persist snapshot: %v", a.ID, err)
	}

	for _, p := range fires {
		automationManager.firePRComment(a, cfg, p.pr, p.comment)
	}
}

func (automationManager *AutomationManager) firePRComment(a Automation, _ repoConfig, pr gh.PullRequest, c gh.IssueComment) {
	fc := fireContext{
		Automation:   a,
		PR:           &pr,
		JiraIssueKey: extractJiraKey(pr.Title),
		NotifLabel:   fmt.Sprintf("PR #%d comment by @%s", pr.Number, c.Author),
		Vars: map[string]string{
			"number":        fmt.Sprintf("%d", pr.Number),
			"title":         pr.Title,
			"prAuthor":      pr.Author,
			"commentAuthor": c.Author,
			"comment":       c.Body,
			"url":           c.URL,
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

func matchesRepoTrigger(t AutomationTrigger, pr gh.PullRequest) bool {
	if t.Kind != "repository.pr_opened" {
		return false
	}
	if pr.State != "open" {
		return false
	}
	if pr.Draft && !t.IncludeDrafts {
		return false
	}
	if t.AuthorFilter != "" && !strings.EqualFold(pr.Author, t.AuthorFilter) {
		return false
	}
	return true
}

func (automationManager *AutomationManager) firePR(a Automation, _ repoConfig, pr gh.PullRequest) {
	fc := fireContext{
		Automation:   a,
		PR:           &pr,
		JiraIssueKey: extractJiraKey(pr.Title),
		NotifLabel:   fmt.Sprintf("PR #%d", pr.Number),
		Vars: map[string]string{
			"number": fmt.Sprintf("%d", pr.Number),
			"title":  pr.Title,
			"author": pr.Author,
			"url":    pr.URL,
			"branch": "",
			"base":   "",
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

func (automationManager *AutomationManager) fireIssueAssigned(a Automation, _ repoConfig, issue gh.Issue, assignee string) {
	fc := fireContext{
		Automation:   a,
		JiraIssueKey: extractJiraKey(issue.Title),
		NotifLabel:   fmt.Sprintf("Issue #%d assigned to @%s", issue.Number, assignee),
		Vars: map[string]string{
			"number":   fmt.Sprintf("%d", issue.Number),
			"title":    issue.Title,
			"author":   issue.Author,
			"assignee": assignee,
			"url":      issue.URL,
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

func (automationManager *AutomationManager) firePRBuildResult(a Automation, _ repoConfig, pr gh.PullRequest, run gh.WorkflowRun) {
	fc := fireContext{
		Automation:   a,
		PR:           &pr,
		Run:          &run,
		JiraIssueKey: extractJiraKey(run.Branch, pr.Title),
		NotifLabel:   fmt.Sprintf("PR #%d build %q %s", pr.Number, run.Name, run.Conclusion),
		Vars: map[string]string{
			"number":     fmt.Sprintf("%d", pr.Number),
			"title":      pr.Title,
			"workflow":   run.Name,
			"conclusion": run.Conclusion,
			"branch":     run.Branch,
			"url":        pr.URL,
			"runUrl":     run.URL,
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

// attachJiraCfg looks up the project's Jira config so jira_transition actions
// can run on repository-source triggers. Missing config leaves JiraCfg nil;
// jira actions will then short-circuit with a clear log line.
func (automationManager *AutomationManager) attachJiraCfg(fc *fireContext) {
	cfg, ok := automationManager.jiraConfigFor(fc.Automation.ProjectID)
	if ok {
		fc.JiraCfg = &cfg
	}
}

func anyActionMentions(actions []AutomationAction, token string) bool {
	for _, act := range actions {
		if strings.Contains(act.TaskTemplate, token) || strings.Contains(act.NotifyTitle, token) ||
			strings.Contains(act.EmailTo, token) || strings.Contains(act.EmailSubject, token) || strings.Contains(act.EmailBody, token) {
			return true
		}
	}
	return false
}

// runActions executes every action of the automation in order. Each action
// failure is logged + surfaced as a notification, but never aborts the chain.
// At the end it records one AutomationRun row per fire so the UI can list
// what the automation actually did.
func (automationManager *AutomationManager) runActions(fc fireContext) {
	a := fc.Automation
	if len(a.Actions) == 0 {
		return
	}
	results := make([]ActionResult, 0, len(a.Actions))
	for i, action := range a.Actions {
		switch action.Kind {
		case "spawn_agent":
			results = append(results, automationManager.runSpawnAgent(fc, action, i))
		case "jira_transition":
			results = append(results, automationManager.runJiraTransition(fc, action, i))
		case "notification":
			results = append(results, automationManager.runNotification(fc, action, i))
		case "send_email":
			results = append(results, automationManager.runSendEmail(fc, action, i))
		default:
			log.Printf("automation %s: unknown action kind %q at index %d", a.ID, action.Kind, i)
			results = append(results, ActionResult{Kind: action.Kind, Status: "skipped", Detail: "unknown action kind"})
		}
	}
	outcome := RunOutcomeFired
	for _, r := range results {
		if r.Status == "error" {
			outcome = RunOutcomeError
			break
		}
	}
	if _, err := automationManager.svc.store.InsertAutomationRun(AutomationRun{
		AutomationID: a.ID,
		ProjectID:    a.ProjectID,
		Outcome:      outcome,
		Reason:       fc.NotifLabel,
		Actions:      results,
	}); err != nil {
		log.Printf("automation %s: insert run: %v", a.ID, err)
	}
}

func (automationManager *AutomationManager) runSpawnAgent(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	task := renderTemplate(action.TaskTemplate, fc.Vars)
	input := SpawnAgentInput{
		ProjectID: a.ProjectID,
		Kind:      action.AgentKind,
		Model:     action.Model,
		Task:      task,
		Source:    "automation",
	}
	if fc.JiraIssue != nil {
		input.IssueKey = fc.JiraIssue.Key
		input.IssueSummary = fc.JiraIssue.Summary
		input.IssueType = fc.JiraIssue.IssueType
	}
	agent, err := automationManager.svc.Spawn(input)
	if err != nil {
		log.Printf("automation %s action[%d] spawn_agent: %v", a.ID, idx, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · spawn failed: %v", a.Name, fc.NotifLabel, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "spawn_agent", Status: "error", Detail: err.Error()}
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeAutomation,
		Severity:      SeverityInfo,
		TitleTemplate: fmt.Sprintf("%s · %s · spawned %s", fc.NotifLabel, a.Name, agent.Kind),
		Payload:       map[string]any{"automationId": a.ID, "agentId": agent.ID},
	})
	return ActionResult{Kind: "spawn_agent", Status: "ok", Detail: agent.ID}
}

func (automationManager *AutomationManager) runJiraTransition(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	if action.JiraToStatusID == "" {
		log.Printf("automation %s action[%d] jira_transition: missing toStatusId", a.ID, idx)
		return ActionResult{Kind: "jira_transition", Status: "skipped", Detail: "missing toStatusId"}
	}
	issueKey := fc.JiraIssueKey
	if action.JiraIssueKey != "" {
		issueKey = renderTemplate(action.JiraIssueKey, fc.Vars)
	}
	if issueKey == "" {
		log.Printf("automation %s action[%d] jira_transition: no Jira key resolvable from context", a.ID, idx)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityWarning,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · jira transition skipped (no key)", a.Name, fc.NotifLabel),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "jira_transition", Status: "skipped", Detail: "no Jira key"}
	}
	if fc.JiraCfg == nil {
		log.Printf("automation %s action[%d] jira_transition: project has no Jira integration configured", a.ID, idx)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · jira transition failed (no Jira config)", a.Name, fc.NotifLabel),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "jira_transition", Status: "error", Detail: "no Jira config"}
	}
	err := jira.TransitionIssue(*fc.JiraCfg, issueKey, []string{action.JiraToStatusID})
	if err != nil {
		severity := SeverityError
		if errors.Is(err, jira.ErrNoTransition) {
			severity = SeverityWarning
		}
		log.Printf("automation %s action[%d] jira_transition %s: %v", a.ID, idx, issueKey, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      severity,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · jira %s transition failed: %v", a.Name, fc.NotifLabel, issueKey, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "jira_transition", Status: "error", Detail: err.Error()}
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeAutomation,
		Severity:      SeveritySuccess,
		TitleTemplate: fmt.Sprintf("%s · %s · transitioned %s", fc.NotifLabel, a.Name, issueKey),
		Payload:       map[string]any{"automationId": a.ID},
	})
	return ActionResult{Kind: "jira_transition", Status: "ok", Detail: issueKey}
}

func (automationManager *AutomationManager) runNotification(fc fireContext, action AutomationAction, _ int) ActionResult {
	a := fc.Automation
	title := renderTemplate(action.NotifyTitle, fc.Vars)
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s · %s", fc.NotifLabel, a.Name)
	}
	severity := NotificationSeverity(action.NotifyKind)
	if severity == "" {
		severity = SeverityInfo
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeUser,
		Severity:      severity,
		TitleTemplate: title,
		Payload:       map[string]any{"automationId": a.ID},
	})
	return ActionResult{Kind: "notification", Status: "ok", Detail: title}
}

func (automationManager *AutomationManager) resendConfigFor(projectID string) (resend.Config, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return resend.Config{}, false
	}
	raw, ok := project.Integrations["resend"]
	if !ok {
		return resend.Config{}, false
	}
	cfg := resend.Config{
		APIKey:    stringField(raw, "apiKey"),
		FromEmail: stringField(raw, "fromEmail"),
	}
	if cfg.APIKey == "" {
		return resend.Config{}, false
	}
	return cfg, true
}

func (automationManager *AutomationManager) sentryConfigFor(projectID string) (sentrystore.Config, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return sentrystore.Config{}, false
	}
	raw, ok := project.Integrations["sentry"]
	if !ok {
		return sentrystore.Config{}, false
	}
	cfg := sentrystore.Config{
		Token:    stringField(raw, "token"),
		Org:      stringField(raw, "org"),
		URL:      stringField(raw, "url"),
		Projects: splitSlugs(stringField(raw, "projects")),
	}
	if cfg.Token == "" || cfg.Org == "" || len(cfg.Projects) == 0 {
		return sentrystore.Config{}, false
	}
	return cfg, true
}

func splitSlugs(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// sentryLevelRank orders Sentry severities so a min-level filter is a simple
// comparison. Unknown levels rank as "error" so they aren't silently dropped.
func sentryLevelRank(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warning":
		return 2
	case "fatal":
		return 4
	default: // "error" and anything unrecognised
		return 3
	}
}

// onSentryDiff fires every sentry.new_issue automation whose project matches
// the diff's key. IssuesAdded covers both brand-new issues and regressions
// (resolved → unresolved re-entries). Cold-cache diffs are suppressed.
func (automationManager *AutomationManager) onSentryDiff(diff sentrystore.Diff) {
	if diff.Before.FetchedAt.IsZero() {
		return
	}
	if len(diff.IssuesAdded) == 0 {
		return
	}
	automations, err := automationManager.svc.ListAutomations()
	if err != nil {
		return
	}
	for _, a := range automations {
		if !a.Enabled || a.Source != "sentry" || a.Trigger.Kind != "sentry.new_issue" {
			continue
		}
		cfg, ok := automationManager.sentryConfigFor(a.ProjectID)
		if !ok || sentrystore.KeyFor(cfg) != diff.Key {
			continue
		}
		minRank := 0
		if a.Trigger.MinLevel != "" {
			minRank = sentryLevelRank(a.Trigger.MinLevel)
		}
		for _, issue := range diff.IssuesAdded {
			if sentryLevelRank(issue.Level) < minRank {
				continue
			}
			automationManager.fireSentryIssue(a, issue)
		}
	}
}

func (automationManager *AutomationManager) fireSentryIssue(a Automation, issue sentry.Issue) {
	fc := fireContext{
		Automation:   a,
		JiraIssueKey: extractJiraKey(issue.Title, issue.Culprit),
		NotifLabel:   fmt.Sprintf("Sentry %s", firstNonEmpty(issue.ShortID, issue.ID)),
		Vars: map[string]string{
			"shortId":   issue.ShortID,
			"title":     issue.Title,
			"level":     issue.Level,
			"culprit":   issue.Culprit,
			"project":   issue.Project,
			"permalink": issue.Permalink,
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

func (automationManager *AutomationManager) dokployConfigFor(projectID string) (dokploystore.Config, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return dokploystore.Config{}, false
	}
	raw, ok := project.Integrations["dokploy"]
	if !ok {
		return dokploystore.Config{}, false
	}
	cfg := dokploystore.Config{
		BaseURL:  stringField(raw, "baseUrl"),
		APIKey:   stringField(raw, "apiKey"),
		Projects: splitSlugs(stringField(raw, "projects")),
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return dokploystore.Config{}, false
	}
	return cfg, true
}

// onDokployDiff fires every dokploy-source automation whose project matches the
// diff's key. DeploymentsFinished carries deployments that just reached a
// terminal state; each trigger kind filters by the status it cares about.
// Cold-cache diffs are suppressed so we never fire on the initial seed.
func (automationManager *AutomationManager) onDokployDiff(diff dokploystore.Diff) {
	if diff.Before.FetchedAt.IsZero() {
		return
	}
	if len(diff.DeploymentsFinished) == 0 {
		return
	}
	automations, err := automationManager.svc.ListAutomations()
	if err != nil {
		return
	}
	for _, a := range automations {
		if !a.Enabled || a.Source != "dokploy" {
			continue
		}
		var wantStatus string
		switch a.Trigger.Kind {
		case "dokploy.deployment_failed":
			wantStatus = "error"
		case "dokploy.deployment_succeeded":
			wantStatus = "done"
		default:
			log.Printf("automation %s: unsupported dokploy trigger %q", a.ID, a.Trigger.Kind)
			continue
		}
		cfg, ok := automationManager.dokployConfigFor(a.ProjectID)
		if !ok || dokploystore.KeyFor(cfg) != diff.Key {
			continue
		}
		for _, d := range diff.DeploymentsFinished {
			if d.Status != wantStatus {
				continue
			}
			automationManager.fireDokployDeployment(a, d)
		}
	}
}

func (automationManager *AutomationManager) fireDokployDeployment(a Automation, d dokploy.Deployment) {
	fc := fireContext{
		Automation:   a,
		Deployment:   &d,
		JiraIssueKey: extractJiraKey(d.Title, d.Description),
		NotifLabel:   fmt.Sprintf("Dokploy %s/%s %s", d.Project, d.Service, d.Status),
		Vars: map[string]string{
			"project":      d.Project,
			"service":      d.Service,
			"title":        d.Title,
			"status":       d.Status,
			"errorMessage": d.ErrorMessage,
		},
	}
	automationManager.attachJiraCfg(&fc)
	automationManager.runActions(fc)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (automationManager *AutomationManager) runSendEmail(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	cfg, ok := automationManager.resendConfigFor(a.ProjectID)
	if !ok {
		log.Printf("automation %s action[%d] send_email: project has no Resend integration configured", a.ID, idx)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · send email failed (no Resend config)", a.Name, fc.NotifLabel),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "send_email", Status: "error", Detail: "no Resend config"}
	}
	to := renderTemplate(action.EmailTo, fc.Vars)
	subject := renderTemplate(action.EmailSubject, fc.Vars)
	body := renderTemplate(action.EmailBody, fc.Vars)
	if to == "" || subject == "" {
		log.Printf("automation %s action[%d] send_email: missing to or subject", a.ID, idx)
		return ActionResult{Kind: "send_email", Status: "skipped", Detail: "missing to or subject"}
	}
	msgID, err := resend.Send(cfg, resend.SendInput{To: to, Subject: subject, Text: body})
	if err != nil {
		log.Printf("automation %s action[%d] send_email: %v", a.ID, idx, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · send email failed: %v", a.Name, fc.NotifLabel, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "send_email", Status: "error", Detail: err.Error()}
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeAutomation,
		Severity:      SeveritySuccess,
		TitleTemplate: fmt.Sprintf("%s · %s · email sent to %s", fc.NotifLabel, a.Name, to),
		Payload:       map[string]any{"automationId": a.ID},
	})
	return ActionResult{Kind: "send_email", Status: "ok", Detail: msgID}
}
