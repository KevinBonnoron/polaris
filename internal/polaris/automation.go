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
	"github.com/KevinBonnoron/polaris/internal/providers/messaging"
	"github.com/KevinBonnoron/polaris/internal/providers/repository"
	"github.com/KevinBonnoron/polaris/internal/providers/resend"
	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
	"github.com/KevinBonnoron/polaris/internal/providers/tickets"
	"github.com/KevinBonnoron/polaris/internal/store/dokploystore"
	"github.com/KevinBonnoron/polaris/internal/store/repositorystore"
	"github.com/KevinBonnoron/polaris/internal/store/sentrystore"
	"github.com/KevinBonnoron/polaris/internal/store/ticketsstore"
)

// ticketKeyPattern matches typical Ticket issue keys (e.g. PROJ-123).
var ticketKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]+-\d+`)

func extractTicketKey(candidates ...string) string {
	for _, s := range candidates {
		if s == "" {
			continue
		}
		if m := ticketKeyPattern.FindString(strings.ToUpper(s)); m != "" {
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
	TicketIssueKey string
	TicketIssue    *tickets.Issue
	TicketCfg      *tickets.Config

	PR  *repository.PullRequest
	Run *repository.WorkflowRun

	Deployment *dokploy.Deployment

	// Notification label that frames the chain in user-visible logs.
	NotifLabel string
}

// FireManual fires an automation immediately with an empty context (no trigger
// data). Template placeholders remain unresolved. Useful for testing that
// actions are reachable and credentials are valid.
func (automationManager *AutomationManager) FireManual(id string) error {
	a, err := automationManager.svc.store.GetAutomation(id)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("automation %q not found", id)
	}
	fc := fireContext{
		Automation: *a,
		Vars:       map[string]string{},
		NotifLabel: "manual test",
	}
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
	return nil
}

// AutomationManager subscribes to the shared gh and ticket stores and fires
// automation actions when the diff carries a trigger-matching change. It no
// longer owns its own tickers: cadence is set per-integration on the store,
// and a single poll services every automation and every UI screen watching
// the same (owner, repo) or (ticket board) key.
type AutomationManager struct {
	svc *Service

	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	repositoryUnsub func()
	ticketsUnsub    func()
	sentryUnsub     func()
	dokployUnsub    func()
	ghLogin         string
	ghLooked        bool
}

func NewAutomationManager(svc *Service) *AutomationManager {
	return &AutomationManager{svc: svc}
}

// fetchPRs reads PRs from the shared repository store when available so the
// same poll cycle benefits the UI and other automations on the same repo.
func (automationManager *AutomationManager) fetchPRs(ctx context.Context, owner, repo string) ([]repository.PullRequest, error) {
	if s := automationManager.svc.repositoryStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetPRs(ctx, owner, repo)
	}
	return repository.ListPullRequests(owner, repo)
}

func (automationManager *AutomationManager) fetchIssues(ctx context.Context, owner, repo string) ([]repository.Issue, error) {
	if s := automationManager.svc.repositoryStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetIssues(ctx, owner, repo)
	}
	return repository.ListIssues(owner, repo)
}

func (automationManager *AutomationManager) fetchRuns(ctx context.Context, owner, repo string) ([]repository.WorkflowRun, error) {
	if s := automationManager.svc.repositoryStore; s != nil {
		if err := s.Refresh(ctx, owner, repo); err != nil {
			return nil, err
		}
		return s.GetRuns(ctx, owner, repo)
	}
	page, err := repository.ListWorkflowRuns(owner, repo, 1)
	if err != nil {
		return nil, err
	}
	return page.Runs, nil
}

func (automationManager *AutomationManager) fetchSprint(ctx context.Context, cfg tickets.Config) (*tickets.Sprint, error) {
	if s := automationManager.svc.ticketsStore; s != nil {
		k := ticketsstore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
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
	return tickets.FetchActiveSprint(cfg)
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
	if s := automationManager.svc.repositoryStore; s != nil {
		automationManager.repositoryUnsub = s.Subscribe(automationManager.onRepositoryDiff)
	}
	if s := automationManager.svc.ticketsStore; s != nil {
		automationManager.ticketsUnsub = s.Subscribe(automationManager.onTicketsDiff)
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
	repositoryUnsub := automationManager.repositoryUnsub
	ticketsUnsub := automationManager.ticketsUnsub
	sentryUnsub := automationManager.sentryUnsub
	dokployUnsub := automationManager.dokployUnsub
	automationManager.repositoryUnsub = nil
	automationManager.ticketsUnsub = nil
	automationManager.sentryUnsub = nil
	automationManager.dokployUnsub = nil
	automationManager.mu.Unlock()
	if repositoryUnsub != nil {
		repositoryUnsub()
	}
	if ticketsUnsub != nil {
		ticketsUnsub()
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
	case "tickets":
		cfg, ok := automationManager.ticketsConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.ticketsStore; s != nil {
			k := ticketsstore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
			s.Register(ctx, k, cfg, "automation:"+a.ID, interval)
		}
	case "repository":
		cfg, ok := automationManager.repoConfigFor(a.ProjectID)
		if !ok {
			return
		}
		if s := automationManager.svc.repositoryStore; s != nil {
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
	case "tickets":
		if s := automationManager.svc.ticketsStore; s != nil {
			if cfg, ok := automationManager.ticketsConfigFor(a.ProjectID); ok {
				k := ticketsstore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
				s.Unregister(k, "automation:"+a.ID)
			}
		}
	case "repository":
		if s := automationManager.svc.repositoryStore; s != nil {
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

// onTicketsDiff fires every tickets.transition automation whose project's
// board matches the diff's key. Cold-cache diffs (no previous snapshot) are
// suppressed so we never fire on the initial seed.
func (automationManager *AutomationManager) onTicketsDiff(diff ticketsstore.Diff) {
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
		if !a.Enabled || a.Source != "tickets" || (a.Trigger.Kind != "tickets.transition" && a.Trigger.Kind != "tickets.assigned") {
			continue
		}
		cfg, ok := automationManager.ticketsConfigFor(a.ProjectID)
		if !ok {
			continue
		}
		k := ticketsstore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
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

func (automationManager *AutomationManager) ticketsConfigFor(projectID string) (tickets.Config, bool) {
	project, err := automationManager.svc.store.GetProject(projectID)
	if err != nil || project == nil {
		return tickets.Config{}, false
	}
	raw, ok := project.Integrations["tickets"]
	if !ok {
		return tickets.Config{}, false
	}
	cfg := tickets.Config{
		Provider:   stringField(raw, "provider"),
		BaseURL:    stringField(raw, "baseUrl"),
		Email:      stringField(raw, "email"),
		Token:      stringField(raw, "token"),
		ProjectKey: stringField(raw, "projectKey"),
		BoardID:    int64Field(raw, "boardId"),
	}
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" || cfg.ProjectKey == "" {
		return tickets.Config{}, false
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

// matchesTrigger returns true when the issue matches every constraint of the
// rule. It dispatches on the trigger kind: "tickets.transition" fires on a
// column change, "tickets.assigned" fires when the issue becomes assigned to
// you. Both only ever act on your own tickets.
func matchesTrigger(t AutomationTrigger, myEmail string, issue tickets.Issue, prev issueState, hadPrev bool) bool {
	assigneeKey := issue.AssigneeEmail
	if assigneeKey == "" {
		assigneeKey = issue.Assignee
	}

	switch t.Kind {
	case "tickets.transition":
		return matchesTransitionTrigger(t, myEmail, issue, assigneeKey, prev, hadPrev)
	case "tickets.assigned":
		return matchesAssignedTrigger(myEmail, assigneeKey, prev, hadPrev)
	default:
		return false
	}
}

func matchesTransitionTrigger(t AutomationTrigger, myEmail string, issue tickets.Issue, assigneeKey string, prev issueState, hadPrev bool) bool {
	if t.ToStatusID != "" && issue.StatusID != t.ToStatusID {
		return false
	}
	if !assignedToMe(myEmail, assigneeKey) {
		return false
	}

	// Issue never seen before: treat as "transitioned from nothing" → fire if
	// no from-filter, otherwise skip (we can't know which from to claim).
	if !hadPrev {
		return len(t.FromStatusIDs) == 0
	}

	if prev.StatusID == issue.StatusID {
		return false
	}
	if len(t.FromStatusIDs) > 0 && !contains(t.FromStatusIDs, prev.StatusID) {
		return false
	}
	return true
}

func matchesAssignedTrigger(myEmail, assigneeKey string, prev issueState, hadPrev bool) bool {
	if !assignedToMe(myEmail, assigneeKey) {
		return false
	}
	// Fire only when the assignment is new: either the issue just appeared
	// already assigned to me, or its assignee just changed to me.
	if !hadPrev {
		return true
	}
	return prev.Assignee != assigneeKey
}

// assignedToMe reports whether the issue's assignee resolves to the configured
// user. Tickets automations only ever act on your own tickets.
func assignedToMe(myEmail, assigneeKey string) bool {
	return myEmail != "" && strings.EqualFold(assigneeKey, myEmail)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (automationManager *AutomationManager) fire(a Automation, cfg tickets.Config, issue tickets.Issue, fromStatusName string) {
	// Pull the last comment lazily so the placeholder is only meaningful when
	// the template asks for it.
	lastComment := ""
	if anyActionMentions(a.Actions, "{{lastComment}}") {
		if body, err := tickets.FetchLastComment(cfg, issue.Key); err == nil {
			lastComment = body
		}
	}

	fc := fireContext{
		Automation:     a,
		TicketIssueKey: issue.Key,
		TicketIssue:    &issue,
		TicketCfg:      &cfg,
		NotifLabel:     issue.Key,
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

// onRepositoryDiff fires every repository-source automation whose project
// points at the diff's (owner, repo). Cold-cache diffs (no previous snapshot)
// are suppressed so we never fire on the initial seed.
func (automationManager *AutomationManager) onRepositoryDiff(diff repositorystore.Diff) {
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
		case "repository.pr_approved":
			automationManager.dispatchPRApproved(a, cfg, diff)
		case "repository.issue_assigned":
			automationManager.dispatchIssueAssigned(a, cfg, diff)
		default:
			log.Printf("automation %s: unsupported repository trigger %q", a.ID, a.Trigger.Kind)
		}
	}
}

func (automationManager *AutomationManager) dispatchPROpened(a Automation, cfg repoConfig, diff repositorystore.Diff) {
	for _, pr := range diff.PRsOpened {
		if matchesRepoTrigger(a.Trigger, pr) {
			automationManager.firePR(a, cfg, pr)
		}
	}
}

func (automationManager *AutomationManager) dispatchPRBuildResults(a Automation, cfg repoConfig, diff repositorystore.Diff, conclusion string) {
	me := automationManager.currentGHLogin()
	if me == "" {
		return
	}
	prByNumber := make(map[int]repository.PullRequest, len(diff.After.PRs))
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
			if !strings.EqualFold(pr.Author, me) {
				continue
			}
			automationManager.firePRBuildResult(a, cfg, pr, run)
		}
	}
}

func (automationManager *AutomationManager) dispatchIssueAssigned(a Automation, cfg repoConfig, diff repositorystore.Diff) {
	me := automationManager.currentGHLogin()
	if me == "" {
		return
	}
	for _, ev := range diff.IssuesAssigned {
		if !strings.EqualFold(ev.Assignee, me) {
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

	login, err := repository.GetCurrentUser()
	if err != nil {
		log.Printf("automation: gh current user: %v", err)
	}
	automationManager.mu.Lock()
	automationManager.ghLogin = login
	automationManager.ghLooked = true
	automationManager.mu.Unlock()
	return login
}

func (automationManager *AutomationManager) dispatchPRApproved(a Automation, _ repoConfig, diff repositorystore.Diff) {
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

	var fires []repository.PullRequest

	for _, pr := range diff.After.PRs {
		if !strings.EqualFold(pr.Author, me) {
			continue
		}
		if pr.ReviewDecision != "APPROVED" {
			continue
		}
		key := fmt.Sprintf("%d", pr.Number)
		next[key] = struct{}{}
		if seedOnly {
			continue
		}
		if _, seen := prev[key]; seen {
			continue
		}
		fires = append(fires, pr)
	}

	if err := automationManager.svc.store.PatchAutomationSnapshot(a.ID, next.encode()); err != nil {
		log.Printf("automation %s: persist snapshot: %v", a.ID, err)
	}

	for _, pr := range fires {
		automationManager.firePRApproved(a, pr)
	}
}

func (automationManager *AutomationManager) firePRApproved(a Automation, pr repository.PullRequest) {
	fc := fireContext{
		Automation:     a,
		PR:             &pr,
		TicketIssueKey: extractTicketKey(pr.Title),
		NotifLabel:     fmt.Sprintf("PR #%d approved", pr.Number),
		Vars: map[string]string{
			"number": fmt.Sprintf("%d", pr.Number),
			"title":  pr.Title,
			"author": pr.Author,
			"url":    pr.URL,
		},
	}
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
}

// dispatchPRComments still needs its own per-PR comment fetch — the store
// doesn't cache PR comments — but it piggy-backs on the store's tick
// cadence (this handler is only invoked when the store emits a diff for
// the repo). A per-automation snapshot tracks already-seen comment IDs so
// firings are deduplicated across diffs.
func (automationManager *AutomationManager) dispatchPRComments(a Automation, cfg repoConfig, diff repositorystore.Diff) {
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
		pr      repository.PullRequest
		comment repository.IssueComment
	}
	var fires []pendingComment

	for _, pr := range diff.After.PRs {
		if !strings.EqualFold(pr.Author, me) {
			continue
		}
		comments, err := repository.ListIssueComments(cfg.Owner, cfg.Repo, pr.Number)
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
			if strings.EqualFold(c.Author, me) {
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

func (automationManager *AutomationManager) firePRComment(a Automation, _ repoConfig, pr repository.PullRequest, c repository.IssueComment) {
	fc := fireContext{
		Automation:     a,
		PR:             &pr,
		TicketIssueKey: extractTicketKey(pr.Title),
		NotifLabel:     fmt.Sprintf("PR #%d comment by @%s", pr.Number, c.Author),
		Vars: map[string]string{
			"number":        fmt.Sprintf("%d", pr.Number),
			"title":         pr.Title,
			"prAuthor":      pr.Author,
			"commentAuthor": c.Author,
			"comment":       c.Body,
			"url":           c.URL,
		},
	}
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
}

func matchesRepoTrigger(t AutomationTrigger, pr repository.PullRequest) bool {
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

func (automationManager *AutomationManager) firePR(a Automation, _ repoConfig, pr repository.PullRequest) {
	fc := fireContext{
		Automation:     a,
		PR:             &pr,
		TicketIssueKey: extractTicketKey(pr.Title),
		NotifLabel:     fmt.Sprintf("PR #%d", pr.Number),
		Vars: map[string]string{
			"number": fmt.Sprintf("%d", pr.Number),
			"title":  pr.Title,
			"author": pr.Author,
			"url":    pr.URL,
			"branch": "",
			"base":   "",
		},
	}
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
}

func (automationManager *AutomationManager) fireIssueAssigned(a Automation, _ repoConfig, issue repository.Issue, assignee string) {
	fc := fireContext{
		Automation:     a,
		TicketIssueKey: extractTicketKey(issue.Title),
		NotifLabel:     fmt.Sprintf("Issue #%d assigned to @%s", issue.Number, assignee),
		Vars: map[string]string{
			"number":   fmt.Sprintf("%d", issue.Number),
			"title":    issue.Title,
			"author":   issue.Author,
			"assignee": assignee,
			"url":      issue.URL,
		},
	}
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
}

func (automationManager *AutomationManager) firePRBuildResult(a Automation, _ repoConfig, pr repository.PullRequest, run repository.WorkflowRun) {
	fc := fireContext{
		Automation:     a,
		PR:             &pr,
		Run:            &run,
		TicketIssueKey: extractTicketKey(run.Branch, pr.Title),
		NotifLabel:     fmt.Sprintf("PR #%d build %q %s", pr.Number, run.Name, run.Conclusion),
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
	automationManager.attachTicketCfg(&fc)
	automationManager.runActions(fc)
}

// attachTicketCfg looks up the project's Ticket config so tickets_transition actions
// can run on repository-source triggers. Missing config leaves TicketCfg nil;
// ticket actions will then short-circuit with a clear log line.
func (automationManager *AutomationManager) attachTicketCfg(fc *fireContext) {
	cfg, ok := automationManager.ticketsConfigFor(fc.Automation.ProjectID)
	if ok {
		fc.TicketCfg = &cfg
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
		case "resume_pr_agent":
			results = append(results, automationManager.runResumePRAgent(fc, action, i))
		case "tickets_transition":
			results = append(results, automationManager.runTicketsTransition(fc, action, i))
		case "notification":
			results = append(results, automationManager.runNotification(fc, action, i))
		case "send_email":
			results = append(results, automationManager.runSendEmail(fc, action, i))
		case "send_message":
			results = append(results, automationManager.runSendMessage(fc, action, i))
		case "trigger_workflow":
			results = append(results, automationManager.runTriggerWorkflow(fc, action, i))
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
	if fc.TicketIssue != nil {
		input.IssueKey = fc.TicketIssue.Key
		input.IssueSummary = fc.TicketIssue.Summary
		input.IssueType = fc.TicketIssue.IssueType
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

func (automationManager *AutomationManager) runResumePRAgent(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	if fc.PR == nil {
		log.Printf("automation %s action[%d] resume_pr_agent: no PR context (trigger is not a PR trigger)", a.ID, idx)
		return ActionResult{Kind: "resume_pr_agent", Status: "skipped", Detail: "no PR context"}
	}
	agent, err := automationManager.svc.store.GetAgentByPRURL(a.ProjectID, fc.PR.URL)
	if err != nil {
		log.Printf("automation %s action[%d] resume_pr_agent: lookup: %v", a.ID, idx, err)
		return ActionResult{Kind: "resume_pr_agent", Status: "error", Detail: err.Error()}
	}
	if agent == nil {
		log.Printf("automation %s action[%d] resume_pr_agent: no agent found for PR %s", a.ID, idx, fc.PR.URL)
		return ActionResult{Kind: "resume_pr_agent", Status: "skipped", Detail: "no agent found for this PR"}
	}
	message := renderTemplate(action.TaskTemplate, fc.Vars)
	if message == "" {
		log.Printf("automation %s action[%d] resume_pr_agent: empty message after template render", a.ID, idx)
		return ActionResult{Kind: "resume_pr_agent", Status: "skipped", Detail: "empty message"}
	}
	if err := automationManager.svc.Send(agent.ID, message); err != nil {
		log.Printf("automation %s action[%d] resume_pr_agent: send: %v", a.ID, idx, err)
		return ActionResult{Kind: "resume_pr_agent", Status: "error", Detail: err.Error()}
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeAutomation,
		Severity:      SeverityInfo,
		TitleTemplate: fmt.Sprintf("%s · %s · resumed agent %s", fc.NotifLabel, a.Name, agent.Kind),
		Payload:       map[string]any{"automationId": a.ID, "agentId": agent.ID},
	})
	return ActionResult{Kind: "resume_pr_agent", Status: "ok", Detail: agent.ID}
}

func (automationManager *AutomationManager) runTicketsTransition(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	if action.TicketToStatusID == "" {
		log.Printf("automation %s action[%d] ticket_transition: missing toStatusId", a.ID, idx)
		return ActionResult{Kind: "tickets_transition", Status: "skipped", Detail: "missing toStatusId"}
	}
	issueKey := fc.TicketIssueKey
	if action.TicketIssueKey != "" {
		issueKey = renderTemplate(action.TicketIssueKey, fc.Vars)
	}
	if issueKey == "" {
		log.Printf("automation %s action[%d] ticket_transition: no Ticket key resolvable from context", a.ID, idx)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityWarning,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · ticket transition skipped (no key)", a.Name, fc.NotifLabel),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "tickets_transition", Status: "skipped", Detail: "no Ticket key"}
	}
	if fc.TicketCfg == nil {
		log.Printf("automation %s action[%d] ticket_transition: project has no Ticket integration configured", a.ID, idx)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · ticket transition failed (no Ticket config)", a.Name, fc.NotifLabel),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "tickets_transition", Status: "error", Detail: "no Ticket config"}
	}
	err := tickets.TransitionIssue(*fc.TicketCfg, issueKey, []string{action.TicketToStatusID})
	if err != nil {
		severity := SeverityError
		if errors.Is(err, tickets.ErrNoTransition) {
			severity = SeverityWarning
		}
		log.Printf("automation %s action[%d] ticket_transition %s: %v", a.ID, idx, issueKey, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      severity,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · ticket %s transition failed: %v", a.Name, fc.NotifLabel, issueKey, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "tickets_transition", Status: "error", Detail: err.Error()}
	}
	_, _ = automationManager.svc.Notify(NotifyInput{
		ProjectID:     a.ProjectID,
		Type:          NotifTypeAutomation,
		Severity:      SeveritySuccess,
		TitleTemplate: fmt.Sprintf("%s · %s · transitioned %s", fc.NotifLabel, a.Name, issueKey),
		Payload:       map[string]any{"automationId": a.ID},
	})
	return ActionResult{Kind: "tickets_transition", Status: "ok", Detail: issueKey}
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
		Automation:     a,
		TicketIssueKey: extractTicketKey(issue.Title, issue.Culprit),
		NotifLabel:     fmt.Sprintf("Sentry %s", firstNonEmpty(issue.ShortID, issue.ID)),
		Vars: map[string]string{
			"shortId":   issue.ShortID,
			"title":     issue.Title,
			"level":     issue.Level,
			"culprit":   issue.Culprit,
			"project":   issue.Project,
			"permalink": issue.Permalink,
		},
	}
	automationManager.attachTicketCfg(&fc)
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
		Automation:     a,
		Deployment:     &d,
		TicketIssueKey: extractTicketKey(d.Title, d.Description),
		NotifLabel:     fmt.Sprintf("Dokploy %s/%s %s", d.Project, d.Service, d.Status),
		Vars: map[string]string{
			"project":      d.Project,
			"service":      d.Service,
			"title":        d.Title,
			"status":       d.Status,
			"errorMessage": d.ErrorMessage,
		},
	}
	automationManager.attachTicketCfg(&fc)
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

func (automationManager *AutomationManager) runSendMessage(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	provider := action.MessageProvider
	if provider == "" {
		log.Printf("automation %s action[%d] send_message: no provider specified", a.ID, idx)
		return ActionResult{Kind: "send_message", Status: "skipped", Detail: "no provider specified"}
	}

	project, err := automationManager.svc.store.GetProject(a.ProjectID)
	if err != nil || project == nil {
		return ActionResult{Kind: "send_message", Status: "error", Detail: "project not found"}
	}

	raw, ok := project.Integrations[provider]
	if !ok {
		log.Printf("automation %s action[%d] send_message: project has no %s integration", a.ID, idx, provider)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · send message failed (no %s config)", a.Name, fc.NotifLabel, provider),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "send_message", Status: "error", Detail: "no " + provider + " config"}
	}

	cfg := messaging.Config{
		Webhook: stringField(raw, "webhook"),
		Token:   stringField(raw, "token"),
		Channel: stringField(raw, "channel"),
	}

	p, err := messaging.Factory(provider, cfg)
	if err != nil {
		return ActionResult{Kind: "send_message", Status: "error", Detail: err.Error()}
	}

	title := renderTemplate(action.MessageTitle, fc.Vars)
	body := renderTemplate(action.MessageBody, fc.Vars)
	if title == "" {
		title = fmt.Sprintf("%s · %s", fc.NotifLabel, a.Name)
	}

	if err = p.Send(context.Background(), messaging.Message{Title: title, Body: body}); err != nil {
		log.Printf("automation %s action[%d] send_message: %v", a.ID, idx, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · send message failed: %v", a.Name, fc.NotifLabel, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "send_message", Status: "error", Detail: err.Error()}
	}

	return ActionResult{Kind: "send_message", Status: "ok", Detail: provider}
}

func parseWorkflowInputs(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func (automationManager *AutomationManager) runTriggerWorkflow(fc fireContext, action AutomationAction, idx int) ActionResult {
	a := fc.Automation
	cfg, ok := automationManager.repoConfigFor(a.ProjectID)
	if !ok {
		log.Printf("automation %s action[%d] trigger_workflow: no repository integration", a.ID, idx)
		return ActionResult{Kind: "trigger_workflow", Status: "skipped", Detail: "no repository integration"}
	}
	file := renderTemplate(action.WorkflowFile, fc.Vars)
	ref := renderTemplate(action.WorkflowRef, fc.Vars)
	if file == "" || ref == "" {
		log.Printf("automation %s action[%d] trigger_workflow: missing workflowFile or workflowRef", a.ID, idx)
		return ActionResult{Kind: "trigger_workflow", Status: "skipped", Detail: "missing workflowFile or workflowRef"}
	}
	inputs := parseWorkflowInputs(renderTemplate(action.WorkflowInputs, fc.Vars))
	if err := repository.TriggerWorkflowDispatchByFile(cfg.Owner, cfg.Repo, file, ref, inputs); err != nil {
		log.Printf("automation %s action[%d] trigger_workflow: %v", a.ID, idx, err)
		_, _ = automationManager.svc.Notify(NotifyInput{
			ProjectID:     a.ProjectID,
			Type:          NotifTypeAutomation,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Automation %q · %s · trigger_workflow failed: %v", a.Name, fc.NotifLabel, err),
			Payload:       map[string]any{"automationId": a.ID},
		})
		return ActionResult{Kind: "trigger_workflow", Status: "error", Detail: err.Error()}
	}
	return ActionResult{Kind: "trigger_workflow", Status: "ok", Detail: fmt.Sprintf("%s@%s", file, ref)}
}
