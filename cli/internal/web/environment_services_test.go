package web

import (
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func TestEnvironmentServicesDistinguishOwnershipAndRetainedState(t *testing.T) {
	view := environmentPreviewView{View: preview.View{
		State: "ready", Reused: true, Recipe: &preview.Recipe{URL: "http://127.0.0.1:4567/", Argv: []string{"server"}},
		Resources: &verification.ResourceState{Ports: map[string]string{"DB": "7654"}, Resources: []verification.ResourceRecord{{Resource: verification.Resource{Name: "database"}, State: "ready"}}},
	}}
	services := environmentServices("/checkout", view)
	if len(services) != 3 || !strings.Contains(services[0].Owner, "esterno") || services[0].Endpoints[0] != "127.0.0.1:4567" {
		t.Fatalf("lost external server ownership: %+v", services)
	}
	if !strings.Contains(services[1].State, "ultimo stato registrato") || services[1].Directory != "/checkout" {
		t.Fatalf("retained observation presented as current liveness: %+v", services[1])
	}
	view.Reused = false
	view.CleanupError = "porta ancora aperta"
	if state := environmentServices("/checkout", view)[0].State; !strings.Contains(state, "Arresto non verificato") {
		t.Fatal("cleanup failure hidden", state)
	}
}
