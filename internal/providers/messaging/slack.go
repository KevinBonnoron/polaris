package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Slack implements the Provider interface using Slack Incoming Webhooks.
type Slack struct {
	webhookURL string
}

// NewSlack creates a new Slack provider. webhookURL should be a valid
// Slack Incoming Webhook URL (e.g. https://hooks.slack.com/services/T.../B.../X...).
func NewSlack(webhookURL string) (*Slack, error) {
	if webhookURL == "" {
		return nil, errors.New("slack: webhook URL is required")
	}
	return &Slack{webhookURL: webhookURL}, nil
}

// Send delivers a notification to Slack via the configured webhook.
func (s *Slack) Send(ctx context.Context, message Message) error {
	if message.Title == "" && message.Body == "" {
		return errors.New("slack: message title or body is required")
	}

	// Build the Slack message payload (Block Kit format for better formatting)
	payload := s.buildPayload(message)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: encode payload: %w", err)
	}

	// Send to Slack with context timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// buildPayload constructs a Slack Block Kit message from a Message.
func (s *Slack) buildPayload(m Message) map[string]any {
	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": formatTitle(m.Title, m.Color),
			},
		},
	}

	if m.Body != "" {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": m.Body,
			},
		})
	}

	if m.URL != "" {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("<a href=\"%s\">View →</a>", m.URL),
			},
		})
	}

	if len(m.Tags) > 0 {
		tagText := ""
		for _, tag := range m.Tags {
			tagText += fmt.Sprintf("`%s` ", tag)
		}
		blocks = append(blocks, map[string]any{
			"type": "context",
			"elements": []map[string]string{
				{
					"type": "mrkdwn",
					"text": tagText,
				},
			},
		})
	}

	return map[string]any{
		"blocks": blocks,
	}
}

// formatTitle applies color formatting to the title.
// Slack uses emoji prefixes for visual indication.
func formatTitle(title string, color string) string {
	prefix := ""
	switch color {
	case "success":
		prefix = "✅ "
	case "error":
		prefix = "❌ "
	case "warning":
		prefix = "⚠️ "
	case "info":
		prefix = "ℹ️ "
	default:
		prefix = "🔔 "
	}
	return fmt.Sprintf("*%s%s*", prefix, title)
}
