package main

import (
	"context"
	"fmt"
	"time"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
	"github.com/KevinBonnoron/polaris/internal/providers/messaging"
	"github.com/KevinBonnoron/polaris/internal/providers/resend"
	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
	"github.com/KevinBonnoron/polaris/internal/store/dokploystore"
)

type DokployDashboard struct {
	Services    []dokploy.Service    `json:"services"`
	Deployments []dokploy.Deployment `json:"deployments"`
}

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

func (app *App) SaveSlackConfig(projectID, webhookURL string) error {
	if app.store == nil {
		return fmt.Errorf("store not ready")
	}
	if projectID == "" {
		return fmt.Errorf("projectId is required")
	}

	cfg := messaging.Config{Webhook: webhookURL}
	if err := messaging.ValidateConfig("slack", cfg); err != nil {
		return err
	}

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

	if _, err := app.store.UpsertProject(*proj); err != nil {
		return fmt.Errorf("save project: %w", err)
	}

	return nil
}

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

func (app *App) RunDokployAction(cfg dokploy.Config, svc dokploy.Service, action string) error {
	return dokploy.RunAction(cfg, svc, dokploy.Action(action))
}

func (app *App) GetDokployDeploymentLogs(cfg dokploy.Config, deploymentID string, tail int) (string, error) {
	return dokploy.FetchDeploymentLogs(cfg, deploymentID, tail)
}

func (app *App) GetDokployServiceLogs(cfg dokploy.Config, svc dokploy.Service, tail int) (string, error) {
	return dokploy.FetchServiceLogs(cfg, svc, tail)
}
