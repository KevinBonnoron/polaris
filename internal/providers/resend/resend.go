package resend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const apiBase = "https://api.resend.com"

type Config struct {
	APIKey    string `json:"apiKey"`
	FromEmail string `json:"fromEmail"`
}

type Domain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Region    string `json:"region"`
	CreatedAt string `json:"createdAt"`
}

type domainsResponse struct {
	Data []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		Region    string `json:"region"`
		CreatedAt string `json:"created_at"`
	} `json:"data"`
}

func ListDomains(cfg Config) ([]Domain, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/domains", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("resend: %s — %s", resp.Status, string(raw))
	}

	var result domainsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	domains := make([]Domain, len(result.Data))
	for i, d := range result.Data {
		domains[i] = Domain{
			ID:        d.ID,
			Name:      d.Name,
			Status:    d.Status,
			Region:    d.Region,
			CreatedAt: d.CreatedAt,
		}
	}
	return domains, nil
}

type Email struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	Subject   string   `json:"subject"`
	LastEvent string   `json:"lastEvent"`
	CreatedAt string   `json:"createdAt"`
}

type emailsResponse struct {
	Data []struct {
		ID        string   `json:"id"`
		From      string   `json:"from"`
		To        []string `json:"to"`
		Subject   string   `json:"subject"`
		LastEvent string   `json:"last_event"`
		CreatedAt string   `json:"created_at"`
	} `json:"data"`
}

func ListEmails(cfg Config, limit int) ([]Email, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/emails?limit=%d", apiBase, limit), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("resend: %s — %s", resp.Status, string(raw))
	}

	var result emailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	emails := make([]Email, len(result.Data))
	for i, e := range result.Data {
		emails[i] = Email{
			ID:        e.ID,
			From:      e.From,
			To:        e.To,
			Subject:   e.Subject,
			LastEvent: e.LastEvent,
			CreatedAt: e.CreatedAt,
		}
	}
	return emails, nil
}

type SendInput struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text,omitempty"`
	HTML    string `json:"html,omitempty"`
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text,omitempty"`
	HTML    string   `json:"html,omitempty"`
}

type sendResponse struct {
	ID string `json:"id"`
}

func Send(cfg Config, input SendInput) (string, error) {
	body := sendRequest{
		From:    cfg.FromEmail,
		To:      []string{input.To},
		Subject: input.Subject,
		Text:    input.Text,
		HTML:    input.HTML,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, apiBase+"/emails", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("resend: %s — %s", resp.Status, string(raw))
	}

	var result sendResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}
