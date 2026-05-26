// Package messaging provides integrations with messaging platforms
// like Slack, Discord, Telegram, and Teams. All providers implement
// a common interface for sending notifications about agent events.
package messaging

import "context"

// Provider is the interface all messaging providers must implement.
type Provider interface {
	// Send delivers a notification message. ctx can be used for cancellation/timeout.
	Send(ctx context.Context, message Message) error
}

// Message is the payload sent to a messaging provider.
type Message struct {
	// Title of the notification (e.g. "Claude Code completed: Fix auth bug")
	Title string `json:"title"`
	// Body/description of the notification
	Body string `json:"body"`
	// URL to link to (e.g. GitHub PR URL, agent detail URL)
	URL string `json:"url,omitempty"`
	// Color for formatting: "success" (green), "error" (red), "info" (blue), "warning" (orange)
	Color string `json:"color,omitempty"`
	// Tags/labels (e.g. ["polaris", "claude-code", "project-name"])
	Tags []string `json:"tags,omitempty"`
}

// Config holds authentication credentials and endpoints. Each provider
// implementations decode this into their own specific config struct.
type Config struct {
	Webhook string `json:"webhook"` // Incoming webhook URL (Slack, Discord, Teams)
	Token   string `json:"token"`   // API token (some providers)
	Channel string `json:"channel"` // Channel ID or name (optional, provider-specific)
}
