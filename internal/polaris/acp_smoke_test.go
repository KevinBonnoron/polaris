package polaris

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestACPSmoke drives a real `opencode acp` session against a live provider to
// validate the ACP integration end to end. Gated behind POLARIS_ACP_SMOKE
// because it needs the opencode binary + network. Set:
//
//	POLARIS_ACP_SMOKE=1 OPENCODE_BIN=~/.opencode/bin/opencode \
//	  ACP_ENDPOINT=https://host/v1 ACP_MODEL=qwen3:latest \
//	  go test ./internal/polaris/ -run TestACPSmoke -v -timeout 200s
func TestACPSmoke(t *testing.T) {
	if os.Getenv("POLARIS_ACP_SMOKE") == "" {
		t.Skip("set POLARIS_ACP_SMOKE=1 to run the live ACP smoke test")
	}
	bin := os.Getenv("OPENCODE_BIN")
	endpoint := os.Getenv("ACP_ENDPOINT")
	model := os.Getenv("ACP_MODEL")
	if bin == "" || endpoint == "" || model == "" {
		t.Fatal("need OPENCODE_BIN, ACP_ENDPOINT, ACP_MODEL")
	}
	if strings.HasPrefix(bin, "~/") {
		home, _ := os.UserHomeDir()
		bin = filepath.Join(home, bin[2:])
	}

	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store).WithRunner(NewRunner(filepath.Join(dir, "logs"), filepath.Join(dir, "wt")))

	proj, err := store.UpsertProject(Project{Name: "smoke", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	prov, err := store.UpsertCustomProvider(CustomProvider{Name: "Smoke", Endpoint: endpoint, APIType: "OpenAI-compatible", Models: []string{model}})
	if err != nil {
		t.Fatal(err)
	}

	agent, err := svc.Spawn(SpawnAgentInput{ProjectID: proj.ID, Kind: "opencode", ProviderID: prov.ID, Model: model, Binary: bin, Task: "Reply with exactly one word: hello. Do not use any tools."})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := store.GetAgent(agent.ID)
		if a != nil && a.Status != "working" {
			break
		}
		time.Sleep(time.Second)
	}
	log, _ := svc.ReadLog(agent.ID)
	final, _ := store.GetAgent(agent.ID)
	t.Logf("status=%s sessionId=%s\n--- log ---\n%s", final.Status, final.SessionID, log)
	if final.SessionID == "" {
		t.Error("no ACP session id was captured")
	}
	if strings.TrimSpace(log) == "" {
		t.Error("empty log")
	}
}

// TestACPPermission drives a turn that needs a bash permission and answers it,
// validating the request_permission -> AskUserQuestion -> reply round trip.
func TestACPPermission(t *testing.T) {
	if os.Getenv("POLARIS_ACP_SMOKE") == "" {
		t.Skip("set POLARIS_ACP_SMOKE=1 to run the live ACP smoke test")
	}
	bin := os.Getenv("OPENCODE_BIN")
	endpoint := os.Getenv("ACP_ENDPOINT")
	model := os.Getenv("ACP_MODEL")
	if strings.HasPrefix(bin, "~/") {
		home, _ := os.UserHomeDir()
		bin = filepath.Join(home, bin[2:])
	}

	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store).WithRunner(NewRunner(filepath.Join(dir, "logs"), filepath.Join(dir, "wt")))
	proj, _ := store.UpsertProject(Project{Name: "smoke", Path: dir})
	prov, _ := store.UpsertCustomProvider(CustomProvider{Name: "Smoke", Endpoint: endpoint, APIType: "OpenAI-compatible", Models: []string{model}})

	agent, err := svc.Spawn(SpawnAgentInput{ProjectID: proj.ID, Kind: "opencode", ProviderID: prov.ID, Model: model, Binary: bin, Task: "Run the bash command `echo hi`. Do it now using the bash tool."})
	if err != nil {
		t.Fatal(err)
	}

	answered := false
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := store.GetAgent(agent.ID)
		if a == nil {
			break
		}
		if !answered && a.Status == "waiting" && a.PendingQuestion != nil {
			t.Logf("permission asked: %s %s", a.PendingQuestion.ToolUseID, a.PendingQuestion.Input)
			if err := svc.RespondToAgentQuestion(agent.ID, a.PendingQuestion.ToolUseID, `[{"question":"perm","answer":"Allow once"}]`); err != nil {
				t.Fatalf("respond: %v", err)
			}
			answered = true
		}
		if a.Status != "working" && a.Status != "waiting" {
			break
		}
		time.Sleep(time.Second)
	}
	log, _ := svc.ReadLog(agent.ID)
	t.Logf("answered=%v\n--- log ---\n%s", answered, log)
	if !answered {
		t.Error("permission was never requested")
	}
	if !strings.Contains(log, "Bash") {
		t.Error("bash tool line not rendered")
	}
}
