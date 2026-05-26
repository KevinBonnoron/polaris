package messaging

import (
	"fmt"
)

// Factory creates a Provider based on the provider type and config.
func Factory(providerType string, cfg Config) (Provider, error) {
	switch providerType {
	case "slack":
		return NewSlack(cfg.Webhook)
	case "discord":
		return NewDiscord(cfg.Webhook)
	case "telegram":
		return NewTelegram(cfg.Token, cfg.Channel)
	default:
		return nil, fmt.Errorf("unknown messaging provider: %s", providerType)
	}
}

// ValidateConfig checks that the config has required fields for the provider.
func ValidateConfig(providerType string, cfg Config) error {
	switch providerType {
	case "slack", "discord":
		if cfg.Webhook == "" {
			return fmt.Errorf("%s: webhook URL is required", providerType)
		}
	case "telegram":
		if cfg.Token == "" {
			return fmt.Errorf("telegram: bot token is required")
		}
		if cfg.Channel == "" {
			return fmt.Errorf("telegram: chat ID is required")
		}
	default:
		return fmt.Errorf("unknown messaging provider: %s", providerType)
	}
	return nil
}
