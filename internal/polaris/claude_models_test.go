package polaris

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func resetClaudeModelsCacheForTest(t *testing.T) {
	t.Helper()
	modelsCacheMu.Lock()
	old := modelsCache
	oldAt := modelsCacheAt
	modelsCache = nil
	modelsCacheAt = modelsCacheAt.Add(-modelsCacheTTL)
	modelsCacheMu.Unlock()
	t.Cleanup(func() {
		modelsCacheMu.Lock()
		defer modelsCacheMu.Unlock()
		modelsCache = old
		modelsCacheAt = oldAt
	})
}

func TestFetchClaudeModelsLive_ValueIsFullID(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "test-token")
	resetClaudeModelsCacheForTest(t)

	old := claudeModelsHTTP
	defer func() { claudeModelsHTTP = old }()
	claudeModelsHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"data":[
			{"id":"claude-sonnet-5-20260101","display_name":"Claude Sonnet 5"},
			{"id":"claude-sonnet-5-20251101","display_name":"Claude Sonnet 5 (older)"},
			{"id":"claude-fable-5-20260101","display_name":"Claude Fable 5"},
			{"id":"claude-opus-4-8-20260101","display_name":"Claude Opus 4.8"}
		]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	models, err := FetchClaudeModels(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}

	want := []ModelInfo{
		{Value: "claude-sonnet-5-20260101", Name: "Claude Sonnet 5", Family: "sonnet"},
		{Value: "claude-fable-5-20260101", Name: "Claude Fable 5", Family: "fable"},
		{Value: "claude-opus-4-8-20260101", Name: "Claude Opus 4.8", Family: "opus"},
	}
	for i, m := range models {
		if m != want[i] {
			t.Errorf("models[%d] = %+v, want %+v", i, m, want[i])
		}
	}
}

func TestFetchClaudeModelsLive_OmitsBetaHeader(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "test-token")
	resetClaudeModelsCacheForTest(t)

	old := claudeModelsHTTP
	defer func() { claudeModelsHTTP = old }()
	claudeModelsHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("anthropic-beta"); got != "" {
			t.Fatalf("anthropic-beta header = %q, want empty", got)
		}
		body := `{"data":[{"id":"claude-sonnet-5","display_name":"Claude Sonnet 5"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}

	models, err := FetchClaudeModels(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if models[0].Value != "claude-sonnet-5" {
		t.Fatalf("models[0].Value = %q, want claude-sonnet-5", models[0].Value)
	}
}
