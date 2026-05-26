// Package dokploy fetches projects and deployment data from a self-hosted
// Dokploy instance via its OpenAPI surface using an x-api-key token. Dokploy
// is hierarchical (project → applications/compose), so we expose the project
// tree and a per-service deployment list and let callers diff them.
package dokploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrMissingConfig is returned when required credentials are absent.
var ErrMissingConfig = errors.New("dokploy: missing base URL or API key")

const apiTimeout = 15 * time.Second

type Config struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

// ServiceType discriminates the kinds a project can hold. Applications and
// compose stacks produce build deployments; databases only carry a status.
type ServiceType string

const (
	ServiceApplication ServiceType = "application"
	ServiceCompose     ServiceType = "compose"
	ServicePostgres    ServiceType = "postgres"
	ServiceMySQL       ServiceType = "mysql"
	ServiceMariaDB     ServiceType = "mariadb"
	ServiceMongo       ServiceType = "mongo"
	ServiceRedis       ServiceType = "redis"
)

// Deployable reports whether the service type produces build deployments that
// can be queried via deployment.allByType. Databases only expose a status.
func (t ServiceType) Deployable() bool {
	return t == ServiceApplication || t == ServiceCompose
}

// Service is one unit (application, compose stack or database) inside a
// project. ID is the entity id used to query deployments. Status mirrors the
// upstream applicationStatus / composeStatus (idle, running, done, error).
type Service struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        ServiceType `json:"type"`
	Status      string      `json:"status"`
	Project     string      `json:"project"`
	Environment string      `json:"environment"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Services    []Service `json:"services"`
}

// Deployment mirrors the upstream deployment row. Status is one of
// running, done, error, cancelled.
type Deployment struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	ApplicationID string `json:"applicationId"`
	ComposeID     string `json:"composeId"`
	CreatedAt     string `json:"createdAt"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	ErrorMessage  string `json:"errorMessage"`

	// Project and Service are not part of the upstream deployment row; the
	// store fills them from the project tree so triggers can reference them.
	Project string `json:"project,omitempty"`
	Service string `json:"service,omitempty"`
}

// IsTerminal reports whether the deployment has reached a final state and will
// not transition further.
func (d Deployment) IsTerminal() bool {
	switch d.Status {
	case "done", "error", "cancelled":
		return true
	default:
		return false
	}
}

// rawService captures every id and status variant so one struct can decode
// any service kind. Only the field matching the kind is populated upstream.
type rawService struct {
	ApplicationID string `json:"applicationId"`
	ComposeID     string `json:"composeId"`
	PostgresID    string `json:"postgresId"`
	MysqlID       string `json:"mysqlId"`
	MariadbID     string `json:"mariadbId"`
	MongoID       string `json:"mongoId"`
	RedisID       string `json:"redisId"`
	Name          string `json:"name"`
	AppStatus     string `json:"applicationStatus"`
	ComposeStatus string `json:"composeStatus"`
}

func (rs rawService) id(t ServiceType) string {
	switch t {
	case ServiceApplication:
		return rs.ApplicationID
	case ServiceCompose:
		return rs.ComposeID
	case ServicePostgres:
		return rs.PostgresID
	case ServiceMySQL:
		return rs.MysqlID
	case ServiceMariaDB:
		return rs.MariadbID
	case ServiceMongo:
		return rs.MongoID
	case ServiceRedis:
		return rs.RedisID
	default:
		return ""
	}
}

func (rs rawService) status(t ServiceType) string {
	if t == ServiceCompose {
		return rs.ComposeStatus
	}
	return rs.AppStatus
}

// rawServices is the set of typed service arrays a project (or one of its
// environments) carries.
type rawServices struct {
	Applications []rawService `json:"applications"`
	Compose      []rawService `json:"compose"`
	Postgres     []rawService `json:"postgres"`
	Mysql        []rawService `json:"mysql"`
	Mariadb      []rawService `json:"mariadb"`
	Mongo        []rawService `json:"mongo"`
	Redis        []rawService `json:"redis"`
}

type rawEnvironment struct {
	Name string `json:"name"`
	rawServices
}

type rawProject struct {
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Dokploy ≥ 0.29 nests services under environments. Older versions exposed
	// them directly on the project; the embedded rawServices reads that flat
	// shape so the integration works across versions.
	Environments []rawEnvironment `json:"environments"`
	rawServices
}

func (r rawProject) services() []Service {
	var out []Service
	collect := func(rs rawServices, env string) {
		appendServices(&out, rs.Applications, ServiceApplication, r.Name, env)
		appendServices(&out, rs.Compose, ServiceCompose, r.Name, env)
		appendServices(&out, rs.Postgres, ServicePostgres, r.Name, env)
		appendServices(&out, rs.Mysql, ServiceMySQL, r.Name, env)
		appendServices(&out, rs.Mariadb, ServiceMariaDB, r.Name, env)
		appendServices(&out, rs.Mongo, ServiceMongo, r.Name, env)
		appendServices(&out, rs.Redis, ServiceRedis, r.Name, env)
	}
	collect(r.rawServices, "")
	for _, env := range r.Environments {
		collect(env.rawServices, env.Name)
	}
	return out
}

func appendServices(out *[]Service, items []rawService, t ServiceType, project, environment string) {
	for _, it := range items {
		id := it.id(t)
		if id == "" {
			continue
		}
		*out = append(*out, Service{ID: id, Name: it.Name, Type: t, Status: it.status(t), Project: project, Environment: environment})
	}
}

func (cfg Config) base() string {
	return strings.TrimRight(cfg.BaseURL, "/")
}

func (cfg Config) get(path string, query url.Values) (*http.Response, error) {
	endpoint := cfg.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: apiTimeout}
	return client.Do(req)
}

func (cfg Config) post(path string, body any) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, cfg.base()+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: apiTimeout}
	return client.Do(req)
}

func decodeOrError(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dokploy: %s — %s", resp.Status, string(raw))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// FetchProjects lists every project with its deployable services flattened.
func FetchProjects(cfg Config) ([]Project, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, ErrMissingConfig
	}
	resp, err := cfg.get("/api/project.all", nil)
	if err != nil {
		return nil, err
	}
	var raws []rawProject
	if err := decodeOrError(resp, &raws); err != nil {
		return nil, err
	}

	projects := make([]Project, 0, len(raws))
	for _, r := range raws {
		projects = append(projects, Project{ID: r.ProjectID, Name: r.Name, Description: r.Description, Services: r.services()})
	}
	return projects, nil
}

// FetchDeployments returns the recent deployments for one service (the API
// caps this at the last handful), newest first.
func FetchDeployments(cfg Config, svc Service) ([]Deployment, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, ErrMissingConfig
	}
	q := url.Values{}
	q.Set("id", svc.ID)
	q.Set("type", string(svc.Type))
	resp, err := cfg.get("/api/deployment.allByType", q)
	if err != nil {
		return nil, err
	}
	type rawDeployment struct {
		Deployment
		DeploymentID string `json:"deploymentId"`
	}
	var raws []rawDeployment
	if err := decodeOrError(resp, &raws); err != nil {
		return nil, err
	}
	deployments := make([]Deployment, len(raws))
	for i, r := range raws {
		d := r.Deployment
		if d.ID == "" {
			d.ID = r.DeploymentID
		}
		deployments[i] = d
	}
	return deployments, nil
}

func readBody(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("dokploy: %s — %s", resp.Status, string(raw))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	s := string(raw)
	if len(s) >= 2 && s[0] == '"' {
		var unquoted string
		if json.Unmarshal(raw, &unquoted) == nil {
			s = unquoted
		}
	}
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, "\n"), nil
}

// FetchDeploymentLogs returns the build log output for a specific deployment.
func FetchDeploymentLogs(cfg Config, deploymentID string, tail int) (string, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return "", ErrMissingConfig
	}
	q := url.Values{}
	q.Set("deploymentId", deploymentID)
	if tail > 0 {
		q.Set("tail", fmt.Sprintf("%d", tail))
	}
	resp, err := cfg.get("/api/deployment.readLogs", q)
	if err != nil {
		return "", err
	}
	return readBody(resp)
}

// FetchServiceLogs returns runtime container logs for an application service.
func FetchServiceLogs(cfg Config, svc Service, tail int) (string, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return "", ErrMissingConfig
	}
	if svc.Type != ServiceApplication {
		return "", fmt.Errorf("dokploy: runtime logs only available for application services")
	}
	q := url.Values{}
	q.Set("applicationId", svc.ID)
	if tail > 0 {
		q.Set("tail", fmt.Sprintf("%d", tail))
	}
	resp, err := cfg.get("/api/application.readLogs", q)
	if err != nil {
		return "", err
	}
	return readBody(resp)
}

// Action is a lifecycle operation that can be triggered on a deployable service.
type Action string

const (
	ActionRedeploy Action = "redeploy"
	ActionStart    Action = "start"
	ActionStop     Action = "stop"
)

// RunAction triggers a lifecycle action on an application or compose service.
// Databases don't support these actions. The endpoint and id field depend on
// the service type (application.* with applicationId, compose.* with composeId).
func RunAction(cfg Config, svc Service, action Action) error {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return ErrMissingConfig
	}
	var path string
	var body map[string]string
	switch svc.Type {
	case ServiceApplication:
		path = "/api/application." + string(action)
		body = map[string]string{"applicationId": svc.ID}
	case ServiceCompose:
		path = "/api/compose." + string(action)
		body = map[string]string{"composeId": svc.ID}
	default:
		return fmt.Errorf("dokploy: %s services do not support actions", svc.Type)
	}

	resp, err := cfg.post(path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dokploy: %s — %s", resp.Status, string(raw))
	}
	return nil
}
