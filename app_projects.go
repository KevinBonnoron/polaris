package main

import "github.com/KevinBonnoron/polaris/internal/polaris"

func (app *App) ListProjects() ([]polaris.Project, error) {
	return app.svc.ListProjects()
}

func (app *App) UpsertProject(p polaris.Project) (polaris.Project, error) {
	return app.svc.UpsertProject(p)
}

func (app *App) DeleteProject(id string) error {
	return app.svc.DeleteProject(id)
}

func (app *App) ListAgents(projectID string) ([]polaris.Agent, error) {
	return app.svc.ListAgents(projectID)
}

func (app *App) UpsertAgent(a polaris.Agent) (polaris.Agent, error) {
	return app.svc.UpsertAgent(a)
}

func (app *App) DeleteAgent(id string) error {
	return app.svc.DeleteAgent(id)
}

func (app *App) ListNotifications() ([]polaris.Notification, error) {
	return app.svc.ListNotifications()
}

func (app *App) UpsertNotification(n polaris.Notification) (polaris.Notification, error) {
	return app.svc.UpsertNotification(n)
}

func (app *App) DeleteNotification(id string) error {
	return app.svc.DeleteNotification(id)
}

func (app *App) MarkAllNotificationsRead() error {
	return app.svc.MarkAllNotificationsRead()
}

func (app *App) MarkNotificationRead(id string) error {
	return app.svc.MarkNotificationRead(id)
}

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
