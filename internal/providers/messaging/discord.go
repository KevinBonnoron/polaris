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

// Discord implements the Provider interface using Discord Webhooks.
type Discord struct {
	webhookURL string
}

// NewDiscord creates a new Discord provider. webhookURL should be a valid
// Discord webhook URL (e.g. https://discord.com/api/webhooks/...).
func NewDiscord(webhookURL string) (*Discord, error) {
	if webhookURL == "" {
		return nil, errors.New("discord: webhook URL is required")
	}
	return &Discord{webhookURL: webhookURL}, nil
}

// Send delivers a notification to Discord via the configured webhook.
func (d *Discord) Send(ctx context.Context, message Message) error {
	if message.Title == "" && message.Body == "" {
		return errors.New("discord: message title or body is required")
	}

	payload := d.buildPayload(message)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: encode payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// buildPayload constructs a Discord webhook message from a Message.
func (d *Discord) buildPayload(m Message) map[string]any {
	payload := map[string]any{
		"username": "Polaris",
		"avatar_url": "https://via.placeholder.com/256?text=P", // TODO: proper logo
	}

	// Build embed (rich message format)
	embed := map[string]any{
		"title": m.Title,
	}

	// Set color based on message color field
	colorMap := map[string]int{
		"success": 0x28a745, // green
		"error":   0xdc3545, // red
		"warning": 0xffc107, // orange
		"info":    0x17a2b8, // blue
	}
	if color, ok := colorMap[m.Color]; ok {
		embed["color"] = color
	} else {
		embed["color"] = 0x6366f1 // indigo (default)
	}

	if m.Body != "" {
		embed["description"] = m.Body
	}

	if m.URL != "" {
		embed["url"] = m.URL
	}

	if len(m.Tags) > 0 {
		fields := []map[string]any{}
		for _, tag := range m.Tags {
			fields = append(fields, map[string]any{
				"name":   "Tag",
				"value":  tag,
				"inline": true,
			})
		}
		embed["fields"] = fields
	}

	payload["embeds"] = []map[string]any{embed}
	return payload
}
