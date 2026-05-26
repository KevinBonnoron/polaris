package dokploystore

import (
	"testing"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
)

func svc(id, name string) dokploy.Service {
	return dokploy.Service{ID: id, Name: name, Type: dokploy.ServiceApplication}
}

func projects() []dokploy.Project {
	return []dokploy.Project{
		{ID: "p1", Name: "Backend", Services: []dokploy.Service{svc("a1", "api"), svc("a2", "worker")}},
		{ID: "p2", Name: "Frontend", Services: []dokploy.Service{svc("a3", "web")}},
		{ID: "p3", Name: "Infra", Services: []dokploy.Service{svc("a4", "proxy")}},
	}
}

func serviceIDs(services []dokploy.Service) map[string]bool {
	out := map[string]bool{}
	for _, s := range services {
		out[s.ID] = true
	}
	return out
}

func TestSelectServices_EmptyFilterKeepsAll(t *testing.T) {
	got := selectServices(projects(), nil)
	if len(got) != 4 {
		t.Fatalf("empty filter: want 4 services, got %d", len(got))
	}
}

func TestSelectServices_FiltersByName(t *testing.T) {
	got := selectServices(projects(), []string{"backend"})
	ids := serviceIDs(got)
	if len(got) != 2 || !ids["a1"] || !ids["a2"] {
		t.Fatalf("filter 'backend': want services a1,a2, got %v", ids)
	}
	if ids["a3"] || ids["a4"] {
		t.Fatalf("filter 'backend' leaked other projects' services: %v", ids)
	}
}

func TestSelectServices_CaseAndWhitespaceInsensitive(t *testing.T) {
	got := selectServices(projects(), []string{"  FRONTEND  "})
	ids := serviceIDs(got)
	if len(got) != 1 || !ids["a3"] {
		t.Fatalf("filter '  FRONTEND  ': want service a3, got %v", ids)
	}
}

func TestSelectServices_MultipleNames(t *testing.T) {
	got := selectServices(projects(), []string{"backend", "infra"})
	if len(got) != 3 {
		t.Fatalf("filter 'backend,infra': want 3 services, got %d", len(got))
	}
}

func TestSelectServices_UnknownNameYieldsNothing(t *testing.T) {
	got := selectServices(projects(), []string{"does-not-exist"})
	if len(got) != 0 {
		t.Fatalf("unknown filter: want 0 services, got %d", len(got))
	}
}

func snap(deployments ...dokploy.Deployment) Snapshot {
	return Snapshot{Deployments: deployments}
}

func dep(id, status string) dokploy.Deployment {
	return dokploy.Deployment{ID: id, Status: status}
}

func TestFinishedDeployments_FiresOnTransitionToTerminal(t *testing.T) {
	before := snap(dep("d1", "running"))
	after := snap(dep("d1", "error"))
	got := finishedDeployments(before, after)
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("running→error: want [d1], got %v", got)
	}
}

func TestFinishedDeployments_IgnoresAlreadyTerminal(t *testing.T) {
	before := snap(dep("d1", "done"))
	after := snap(dep("d1", "done"))
	if got := finishedDeployments(before, after); len(got) != 0 {
		t.Fatalf("done→done: want nothing, got %v", got)
	}
}

func TestFinishedDeployments_IgnoresStillRunning(t *testing.T) {
	before := snap(dep("d1", "running"))
	after := snap(dep("d1", "running"))
	if got := finishedDeployments(before, after); len(got) != 0 {
		t.Fatalf("running→running: want nothing, got %v", got)
	}
}

func TestFinishedDeployments_NewTerminalDeployment(t *testing.T) {
	before := snap()
	after := snap(dep("d1", "done"))
	got := finishedDeployments(before, after)
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("new terminal deployment: want [d1], got %v", got)
	}
}
