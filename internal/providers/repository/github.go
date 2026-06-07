// Package repository provides integrations with repository hosting platforms
// like GitHub, GitLab, and Bitbucket. We deliberately keep the surface small and
// translate each platform's APIs into our own DTOs so the frontend doesn't
// depend on any single platform's full schema.
package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
	"gopkg.in/yaml.v3"
)

// ErrCLIMissing is returned when the `gh` binary cannot be located on $PATH.
var ErrCLIMissing = errors.New("gh CLI not installed or not on PATH")

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type PullRequest struct {
	Number         int     `json:"number"`
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	URL            string  `json:"url"`
	HeadBranch     string  `json:"headBranch"`
	State          string  `json:"state"`
	Draft          bool    `json:"draft"`
	ReviewDecision string  `json:"reviewDecision"`
	Labels         []Label `json:"labels"`
	CreatedAt      int64   `json:"createdAt"`
	UpdatedAt      int64   `json:"updatedAt"`
}

type Issue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	URL       string   `json:"url"`
	State     string   `json:"state"`
	Labels    []Label  `json:"labels"`
	Assignees []string `json:"assignees"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

type WorkflowRun struct {
	ID           int64  `json:"id"`
	WorkflowID   int64  `json:"workflowId"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	URL          string `json:"url"`
	Branch       string `json:"branch"`
	Event        string `json:"event"`
	CreatedAt    int64  `json:"createdAt"`
	RunStartedAt int64  `json:"runStartedAt,omitempty"`
	UpdatedAt    int64  `json:"updatedAt"`
	PRNumbers    []int  `json:"prNumbers,omitempty"`
}

type WorkflowRunsPage struct {
	Runs    []WorkflowRun `json:"runs"`
	HasMore bool          `json:"hasMore"`
}

const workflowRunsPerPage = 30

const apiTimeout = 15 * time.Second

func ListPullRequests(owner, repo string) ([]PullRequest, error) {
	const query = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pullRequests(states:OPEN,first:30,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number title url headRefName isDraft createdAt updatedAt reviewDecision author{login} labels(first:20){nodes{name color}}}}}}`
	var raw struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						Number         int       `json:"number"`
						Title          string    `json:"title"`
						URL            string    `json:"url"`
						HeadRefName    string    `json:"headRefName"`
						IsDraft        bool      `json:"isDraft"`
						CreatedAt      time.Time `json:"createdAt"`
						UpdatedAt      time.Time `json:"updatedAt"`
						ReviewDecision string    `json:"reviewDecision"`
						Author         *ghUser   `json:"author"`
						Labels         struct {
							Nodes []ghLabel `json:"nodes"`
						} `json:"labels"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := callGraphQL(query, map[string]string{"owner": owner, "name": repo}, &raw); err != nil {
		return nil, err
	}
	nodes := raw.Data.Repository.PullRequests.Nodes
	out := make([]PullRequest, 0, len(nodes))
	for _, p := range nodes {
		author := ""
		if p.Author != nil {
			author = p.Author.Login
		}
		out = append(out, PullRequest{
			Number:         p.Number,
			Title:          p.Title,
			Author:         author,
			URL:            p.URL,
			HeadBranch:     p.HeadRefName,
			State:          "open",
			Draft:          p.IsDraft,
			ReviewDecision: p.ReviewDecision,
			Labels:         convertLabels(p.Labels.Nodes),
			CreatedAt:      p.CreatedAt.Unix(),
			UpdatedAt:      p.UpdatedAt.Unix(),
		})
	}
	return out, nil
}

func ListIssues(owner, repo string) ([]Issue, error) {
	// The /issues endpoint includes PRs; we filter them out via the
	// pull_request field which is only set for PRs.
	var raw []struct {
		Number      int             `json:"number"`
		Title       string          `json:"title"`
		URL         string          `json:"html_url"`
		State       string          `json:"state"`
		User        ghUser          `json:"user"`
		Assignees   []ghUser        `json:"assignees"`
		Labels      []ghLabel       `json:"labels"`
		CreatedAt   time.Time       `json:"created_at"`
		UpdatedAt   time.Time       `json:"updated_at"`
		PullRequest json.RawMessage `json:"pull_request"`
	}
	path := fmt.Sprintf("/repos/%s/%s/issues?state=open&per_page=30", owner, repo)
	if err := callJSON(path, &raw); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(raw))
	for _, i := range raw {
		if len(i.PullRequest) > 0 && string(i.PullRequest) != "null" {
			continue
		}
		assignees := make([]string, 0, len(i.Assignees))
		for _, a := range i.Assignees {
			if a.Login != "" {
				assignees = append(assignees, a.Login)
			}
		}
		out = append(out, Issue{
			Number:    i.Number,
			Title:     i.Title,
			Author:    i.User.Login,
			URL:       i.URL,
			State:     i.State,
			Labels:    convertLabels(i.Labels),
			Assignees: assignees,
			CreatedAt: i.CreatedAt.Unix(),
			UpdatedAt: i.UpdatedAt.Unix(),
		})
	}
	return out, nil
}

// ListBranches returns the branch names of a repository (most recently
// active first, capped at 100 entries).
func ListBranches(owner, repo string) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
	}
	path := fmt.Sprintf("/repos/%s/%s/branches?per_page=100", owner, repo)
	if err := callJSON(path, &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, b := range raw {
		out = append(out, b.Name)
	}
	return out, nil
}

func ListWorkflowRuns(owner, repo string, page int) (*WorkflowRunsPage, error) {
	if page < 1 {
		page = 1
	}
	var raw struct {
		TotalCount   int `json:"total_count"`
		WorkflowRuns []struct {
			ID           int64     `json:"id"`
			WorkflowID   int64     `json:"workflow_id"`
			Name         string    `json:"name"`
			Status       string    `json:"status"`
			Conclusion   string    `json:"conclusion"`
			URL          string    `json:"html_url"`
			Branch       string    `json:"head_branch"`
			Event        string    `json:"event"`
			CreatedAt    time.Time `json:"created_at"`
			RunStartedAt time.Time `json:"run_started_at"`
			UpdatedAt    time.Time `json:"updated_at"`
			PullRequests []struct {
				Number int `json:"number"`
			} `json:"pull_requests"`
		} `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=%d&page=%d", owner, repo, workflowRunsPerPage, page)
	if err := callJSON(path, &raw); err != nil {
		return nil, err
	}
	out := make([]WorkflowRun, 0, len(raw.WorkflowRuns))
	for _, r := range raw.WorkflowRuns {
		prs := make([]int, 0, len(r.PullRequests))
		for _, pr := range r.PullRequests {
			prs = append(prs, pr.Number)
		}
		runStarted := int64(0)
		if !r.RunStartedAt.IsZero() {
			runStarted = r.RunStartedAt.Unix()
		}
		out = append(out, WorkflowRun{
			ID:           r.ID,
			WorkflowID:   r.WorkflowID,
			Name:         r.Name,
			Status:       r.Status,
			Conclusion:   r.Conclusion,
			URL:          r.URL,
			Branch:       r.Branch,
			Event:        r.Event,
			CreatedAt:    r.CreatedAt.Unix(),
			RunStartedAt: runStarted,
			UpdatedAt:    r.UpdatedAt.Unix(),
			PRNumbers:    prs,
		})
	}
	hasMore := page*workflowRunsPerPage < raw.TotalCount
	return &WorkflowRunsPage{Runs: out, HasMore: hasMore}, nil
}

type WorkflowDispatchInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default"`
	Options     []string `json:"options"`
}

type WorkflowDispatchSpec struct {
	Dispatchable bool                    `json:"dispatchable"`
	Inputs       []WorkflowDispatchInput `json:"inputs"`
}

// GetWorkflowDispatch fetches a workflow's YAML and reports whether it is
// manually dispatchable along with its declared inputs.
func GetWorkflowDispatch(owner, repo string, workflowID int64) (*WorkflowDispatchSpec, error) {
	var meta struct {
		Path string `json:"path"`
	}
	if err := callJSON(fmt.Sprintf("/repos/%s/%s/actions/workflows/%d", owner, repo, workflowID), &meta); err != nil {
		return nil, err
	}
	if meta.Path == "" {
		return &WorkflowDispatchSpec{}, nil
	}
	var content struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := callJSON(fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, meta.Path), &content); err != nil {
		return nil, err
	}
	if content.Encoding != "base64" {
		return &WorkflowDispatchSpec{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decode workflow yaml: %w", err)
	}
	return parseDispatchSpec(raw)
}

func parseDispatchSpec(data []byte) (*WorkflowDispatchSpec, error) {
	spec := &WorkflowDispatchSpec{Inputs: []WorkflowDispatchInput{}}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return spec, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return spec, nil
	}
	onNode := yamlMapValue(root, "on")
	if onNode == nil {
		// YAML 1.1 may parse `on` as boolean true; check that as well.
		onNode = yamlMapValue(root, "true")
	}
	if onNode == nil {
		return spec, nil
	}
	switch onNode.Kind {
	case yaml.ScalarNode:
		spec.Dispatchable = onNode.Value == "workflow_dispatch"
	case yaml.SequenceNode:
		for _, item := range onNode.Content {
			if item.Value == "workflow_dispatch" {
				spec.Dispatchable = true
				break
			}
		}
	case yaml.MappingNode:
		wd := yamlMapValue(onNode, "workflow_dispatch")
		if wd == nil {
			return spec, nil
		}
		spec.Dispatchable = true
		if wd.Kind != yaml.MappingNode {
			return spec, nil
		}
		inputs := yamlMapValue(wd, "inputs")
		if inputs == nil || inputs.Kind != yaml.MappingNode {
			return spec, nil
		}
		for i := 0; i+1 < len(inputs.Content); i += 2 {
			key := inputs.Content[i]
			val := inputs.Content[i+1]
			input := WorkflowDispatchInput{Name: key.Value, Type: "string", Options: []string{}}
			if val.Kind == yaml.MappingNode {
				if n := yamlMapValue(val, "description"); n != nil {
					input.Description = n.Value
				}
				if n := yamlMapValue(val, "type"); n != nil {
					input.Type = n.Value
				}
				if n := yamlMapValue(val, "required"); n != nil {
					input.Required = strings.EqualFold(n.Value, "true")
				}
				if n := yamlMapValue(val, "default"); n != nil {
					input.Default = n.Value
				}
				if n := yamlMapValue(val, "options"); n != nil && n.Kind == yaml.SequenceNode {
					for _, opt := range n.Content {
						input.Options = append(input.Options, opt.Value)
					}
				}
			}
			spec.Inputs = append(spec.Inputs, input)
		}
	}
	return spec, nil
}

func yamlMapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// TriggerWorkflowDispatch invokes the `workflow_dispatch` event on a workflow.
// Returns an error when the workflow has no manual trigger configured.
//
// GitHub's dispatches endpoint occasionally returns HTTP 500 even when the run
// was actually queued. To avoid misleading "failed" toasts in that case, on
// error we briefly poll the workflow's runs endpoint and, if a new dispatch
// run appears, treat the call as successful.
// CreatePullRequest opens a PR for the branch currently checked out in
// workdir. Returns the PR URL that gh prints on stdout. The head branch is
// expected to already be pushed to origin — callers should `git push -u` first
// so GitHub knows the ref exists.
func CreatePullRequest(workdir, title, body string) (string, error) {
	if workdir == "" {
		return "", fmt.Errorf("workdir is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", ErrCLIMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// gh infers --head from the current branch checked out in workdir, which
	// is exactly what we want — the agent's worktree.
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	cmd.Dir = workdir
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("gh pr create: %s", output)
	}
	// gh prints the PR URL on its own line; scan for the first https://.../pull/N
	// occurrence to be robust against future banner/warning lines.
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http") && strings.Contains(line, "/pull/") {
			return line, nil
		}
	}
	return output, nil
}

func TriggerWorkflowDispatch(owner, repo string, workflowID int64, ref string, inputs map[string]string) error {
	if ref == "" {
		return fmt.Errorf("ref is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	payload := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		payload["inputs"] = inputs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode dispatch payload: %w", err)
	}
	dispatchedAt := time.Now().Add(-2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/dispatches", owner, repo, workflowID)
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", "-H", "Accept: application/vnd.github+json", "--input", "-", apiPath)
	cmd.Stdin = bytes.NewReader(body)
	sysexec.Hide(cmd)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if isLikelyTransientServerError(msg) && verifyWorkflowDispatched(owner, repo, workflowID, dispatchedAt) {
		return nil
	}
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Errorf("gh api %s: %s", apiPath, msg)
}

type WorkflowSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"` // filename only, e.g. "deploy.yml"
}

func ListWorkflows(owner, repo string) ([]WorkflowSummary, error) {
	var result struct {
		Workflows []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"workflows"`
	}
	if err := callJSON(fmt.Sprintf("/repos/%s/%s/actions/workflows", owner, repo), &result); err != nil {
		return nil, err
	}
	out := make([]WorkflowSummary, 0, len(result.Workflows))
	for _, w := range result.Workflows {
		filename := w.Path
		if idx := strings.LastIndex(w.Path, "/"); idx >= 0 {
			filename = w.Path[idx+1:]
		}
		out = append(out, WorkflowSummary{ID: w.ID, Name: w.Name, Path: filename})
	}
	return out, nil
}

func TriggerWorkflowDispatchByFile(owner, repo, workflowFile, ref string, inputs map[string]string) error {
	if ref == "" {
		return fmt.Errorf("ref is required")
	}
	if workflowFile == "" {
		return fmt.Errorf("workflowFile is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	payload := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		payload["inputs"] = inputs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode dispatch payload: %w", err)
	}
	dispatchedAt := time.Now().Add(-2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflowFile)
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", "-H", "Accept: application/vnd.github+json", "--input", "-", apiPath)
	cmd.Stdin = bytes.NewReader(body)
	sysexec.Hide(cmd)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if isLikelyTransientServerError(msg) && verifyWorkflowDispatchedByFile(owner, repo, workflowFile, dispatchedAt) {
		return nil
	}
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Errorf("gh api %s: %s", apiPath, msg)
}

func verifyWorkflowDispatchedByFile(owner, repo, workflowFile string, since time.Time) bool {
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/runs?event=workflow_dispatch&per_page=5", owner, repo, workflowFile)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var raw struct {
			WorkflowRuns []struct {
				CreatedAt time.Time `json:"created_at"`
			} `json:"workflow_runs"`
		}
		if err := callJSON(apiPath, &raw); err == nil {
			for _, r := range raw.WorkflowRuns {
				if r.CreatedAt.After(since) {
					return true
				}
			}
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(800 * time.Millisecond)
	}
}

func CancelWorkflowRun(owner, repo string, runID int64) error {
	if runID <= 0 {
		return fmt.Errorf("runID is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/cancel", owner, repo, runID)
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", "-H", "Accept: application/vnd.github+json", apiPath)
	sysexec.Hide(cmd)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Errorf("gh api %s: %s", apiPath, msg)
}

func RerunWorkflowRun(owner, repo string, runID int64) error {
	if runID <= 0 {
		return fmt.Errorf("runID is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", owner, repo, runID)
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", "-H", "Accept: application/vnd.github+json", apiPath)
	sysexec.Hide(cmd)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Errorf("gh api %s: %s", apiPath, msg)
}

func isLikelyTransientServerError(msg string) bool {
	// gh CLI surfaces upstream status as "HTTP 5xx" in its error output.
	return strings.Contains(msg, "HTTP 5") || strings.Contains(msg, "\"status\":\"5")
}

func verifyWorkflowDispatched(owner, repo string, workflowID int64, since time.Time) bool {
	apiPath := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs?event=workflow_dispatch&per_page=5", owner, repo, workflowID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var raw struct {
			WorkflowRuns []struct {
				CreatedAt time.Time `json:"created_at"`
			} `json:"workflow_runs"`
		}
		if err := callJSON(apiPath, &raw); err == nil {
			for _, r := range raw.WorkflowRuns {
				if r.CreatedAt.After(since) {
					return true
				}
			}
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(800 * time.Millisecond)
	}
}

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func convertLabels(in []ghLabel) []Label {
	if len(in) == 0 {
		return nil
	}
	out := make([]Label, 0, len(in))
	for _, l := range in {
		out = append(out, Label{Name: l.Name, Color: l.Color})
	}
	return out
}

type IssueComment struct {
	ID         int64  `json:"id"`
	IssueNum   int    `json:"issueNumber"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	URL        string `json:"url"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	IssueTitle string `json:"issueTitle"`
}

// GetCurrentUser returns the login of the `gh`-authenticated user, e.g. "octocat".
func GetCurrentUser() (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := callJSON("/user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// ListIssueComments returns the comments left on a single issue or PR (GitHub
// treats them through the same endpoint).
func ListIssueComments(owner, repo string, number int) ([]IssueComment, error) {
	var raw []struct {
		ID        int64     `json:"id"`
		User      ghUser    `json:"user"`
		Body      string    `json:"body"`
		HTMLURL   string    `json:"html_url"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", owner, repo, number)
	if err := callJSON(path, &raw); err != nil {
		return nil, err
	}
	out := make([]IssueComment, 0, len(raw))
	for _, c := range raw {
		out = append(out, IssueComment{
			ID:        c.ID,
			IssueNum:  number,
			Author:    c.User.Login,
			Body:      c.Body,
			URL:       c.HTMLURL,
			CreatedAt: c.CreatedAt.Unix(),
			UpdatedAt: c.UpdatedAt.Unix(),
		})
	}
	return out, nil
}

func callGraphQL(query string, vars map[string]string, out any) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-f", k+"="+v)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	sysexec.Hide(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("gh api graphql: %s", msg)
		}
		return fmt.Errorf("gh api graphql: %w", err)
	}
	return json.Unmarshal(stdout, out)
}

func callJSON(apiPath string, out any) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrCLIMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "-H", "Accept: application/vnd.github+json", apiPath)
	sysexec.Hide(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("gh api %s: %s", apiPath, msg)
		}
		return fmt.Errorf("gh api %s: %w", apiPath, err)
	}
	return json.Unmarshal(stdout, out)
}
