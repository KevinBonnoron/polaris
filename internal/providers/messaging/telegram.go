package messaging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Telegram implements the Provider interface using Telegram Bot API.
type Telegram struct {
	botToken string
	chatID   string
}

// NewTelegram creates a new Telegram provider.
// botToken should be the bot token from @BotFather (e.g. "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11").
// chatID should be the chat/channel ID to send messages to (e.g. "-1001234567890" for groups/channels).
func NewTelegram(botToken, chatID string) (*Telegram, error) {
	if botToken == "" {
		return nil, errors.New("telegram: bot token is required")
	}
	if chatID == "" {
		return nil, errors.New("telegram: chat ID is required")
	}
	return &Telegram{botToken: botToken, chatID: chatID}, nil
}

// Send delivers a notification to Telegram via the Bot API.
func (t *Telegram) Send(ctx context.Context, message Message) error {
	if message.Title == "" && message.Body == "" {
		return errors.New("telegram: message title or body is required")
	}

	text := t.formatMessage(message)
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	payload := url.Values{
		"chat_id":    {t.chatID},
		"text":       {text},
		"parse_mode": {"HTML"},
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// formatMessage constructs an HTML-formatted message for Telegram.
// Telegram HTML formatting: <b>bold</b>, <i>italic</i>, <code>code</code>, <a href="url">link</a>, etc.
func (t *Telegram) formatMessage(m Message) string {
	var text string

	// Title with emoji prefix based on color
	emoji := "🔔"
	switch m.Color {
	case "success":
		emoji = "✅"
	case "error":
		emoji = "❌"
	case "warning":
		emoji = "⚠️"
	case "info":
		emoji = "ℹ️"
	}

	text = fmt.Sprintf("%s <b>%s</b>\n", emoji, escapeHTML(m.Title))

	if m.Body != "" {
		text += fmt.Sprintf("%s\n", escapeHTML(m.Body))
	}

	if m.URL != "" {
		text += fmt.Sprintf("\n<a href=\"%s\">View details →</a>\n", m.URL)
	}

	if len(m.Tags) > 0 {
		text += "\n"
		for _, tag := range m.Tags {
			text += fmt.Sprintf("<code>#%s</code> ", escapeHTML(tag))
		}
	}

	return text
}

// escapeHTML escapes special characters for Telegram HTML formatting.
func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(s)
}
