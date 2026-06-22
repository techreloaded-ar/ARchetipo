package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type analyticsStatusData struct {
	Enabled                        bool   `json:"enabled"`
	Source                         string `json:"source"`
	EndpointConfigured             bool   `json:"endpoint_configured"`
	EndpointHost                   string `json:"endpoint_host"`
	AnonymousInstallationIDPresent bool   `json:"anonymous_installation_id_present"`
}

func decodeAnalyticsStatus(t *testing.T, res result) analyticsStatusData {
	t.Helper()
	kind, data := decodeOK(t, res)
	if kind != "analytics_status" {
		t.Fatalf("expected kind=analytics_status, got %s", kind)
	}
	// The data map uses float64 for numbers by default, but analytics status
	// only has booleans and strings — decode via JSON round-trip for safety.
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal status data: %v", err)
	}
	var st analyticsStatusData
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("unmarshal status data: %v", err)
	}
	return st
}

func decodeWriteResult(t *testing.T, res result) {
	t.Helper()
	kind, data := decodeOK(t, res)
	if kind != "write_result" {
		t.Fatalf("expected kind=write_result, got %s", kind)
	}
	ok, _ := data["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true, got %v", data["ok"])
	}
}

func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".archetipo", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Status tests ---

func TestAnalyticsStatus_DefaultNoConfig(t *testing.T) {
	newProject(t)
	res := runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if st.Enabled {
		t.Errorf("expected enabled=false without config, got true")
	}
	if st.Source != "default" {
		t.Errorf("expected source=default without config, got %q", st.Source)
	}
	if st.EndpointConfigured {
		t.Errorf("expected endpoint_configured=false without config, got true")
	}
	if st.EndpointHost != "none (local noop)" {
		t.Errorf("expected endpoint_host='none (local noop)', got %q", st.EndpointHost)
	}
	if st.AnonymousInstallationIDPresent {
		t.Errorf("expected anonymous_installation_id_present=false without config, got true")
	}
}

func TestAnalyticsStatus_DefaultWithConfigNoAnalytics(t *testing.T) {
	newProject(t)
	writeConfig(t, `connector: file
paths:
  prd: docs/PRD.md
`)
	res := runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if st.Enabled {
		t.Errorf("expected enabled=false without analytics key, got true")
	}
	if st.Source != "default" {
		t.Errorf("expected source=default without analytics key, got %q", st.Source)
	}
	if st.EndpointConfigured {
		t.Errorf("expected endpoint_configured=false without analytics key, got true")
	}
	if st.AnonymousInstallationIDPresent {
		t.Errorf("expected anonymous_installation_id_present=false, got true")
	}
}

func TestAnalyticsStatus_AfterEnable(t *testing.T) {
	newProject(t)
	// Enable
	res := runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Status
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if !st.Enabled {
		t.Errorf("expected enabled=true after enable, got false")
	}
	if st.Source != "project_config" {
		t.Errorf("expected source=project_config after enable, got %q", st.Source)
	}
}

func TestAnalyticsStatus_AfterDisableFromDefault(t *testing.T) {
	newProject(t)
	// Disable (from default — should still create the key)
	res := runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	// Status
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if st.Enabled {
		t.Errorf("expected enabled=false after disable, got true")
	}
	if st.Source != "project_config" {
		t.Errorf("expected source=project_config after disable, got %q", st.Source)
	}
}

func TestAnalyticsStatus_EnableThenDisable(t *testing.T) {
	newProject(t)
	// Enable
	res := runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Disable
	res = runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	// Status
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if st.Enabled {
		t.Errorf("expected enabled=false after enable+disable, got true")
	}
	if st.Source != "project_config" {
		t.Errorf("expected source=project_config after enable+disable, got %q", st.Source)
	}
}

func TestAnalyticsStatus_DisableThenEnable(t *testing.T) {
	newProject(t)
	// Disable
	res := runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	// Enable
	res = runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Status
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if !st.Enabled {
		t.Errorf("expected enabled=true after disable+enable, got false")
	}
	if st.Source != "project_config" {
		t.Errorf("expected source=project_config after disable+enable, got %q", st.Source)
	}
}

// --- Idempotency tests ---

func TestAnalyticsEnable_Idempotent(t *testing.T) {
	newProject(t)
	// First enable
	res := runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Read back config to verify consent: true is written
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "consent: true") {
		t.Fatalf("expected consent: true in config after first enable, got:\n%s", string(raw))
	}

	// Second enable (idempotent)
	res = runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Status should still be enabled
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)
	if !st.Enabled || st.Source != "project_config" {
		t.Errorf("expected enabled=true, source=project_config after idempotent enable, got enabled=%v source=%q", st.Enabled, st.Source)
	}
}

func TestAnalyticsDisable_Idempotent(t *testing.T) {
	newProject(t)
	// First disable
	res := runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	// Read back config to verify consent: false is written
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "consent: false") {
		t.Fatalf("expected consent: false in config after first disable, got:\n%s", string(raw))
	}

	// Second disable (idempotent)
	res = runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	// Status should still be disabled
	res = runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)
	if st.Enabled || st.Source != "project_config" {
		t.Errorf("expected enabled=false, source=project_config after idempotent disable, got enabled=%v source=%q", st.Enabled, st.Source)
	}
}

// --- Config preservation tests ---

func TestAnalyticsEnable_PreservesOtherSections(t *testing.T) {
	newProject(t)
	original := `connector: file
paths:
  prd: docs/PRD.md
  mockups: docs/mockups/
workflow:
  statuses:
    todo: TODO
    planned: PLANNED
    in_progress: IN PROGRESS
    review: REVIEW
    done: DONE
`
	writeConfig(t, original)

	// Enable analytics
	res := runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Verify all original sections are preserved
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{
		"connector: file",
		"prd: docs/PRD.md",
		"mockups: docs/mockups/",
		"todo: TODO",
		"planned: PLANNED",
		"in_progress: IN PROGRESS",
		"review: REVIEW",
		"done: DONE",
		"consent: true",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in preserved config:\n%s", want, content)
		}
	}
}

func TestAnalyticsDisable_PreservesOtherSections(t *testing.T) {
	newProject(t)
	original := `connector: file
file:
  backlog: custom/BL.yaml
`
	writeConfig(t, original)

	// Disable analytics
	res := runCLI(t, "", "analytics", "disable")
	decodeWriteResult(t, res)

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{
		"connector: file",
		"backlog: custom/BL.yaml",
		"consent: false",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in preserved config:\n%s", want, content)
		}
	}
}

func TestAnalyticsEnable_CreatesConfigWhenMissing(t *testing.T) {
	newProject(t)
	// No config.yaml exists
	res := runCLI(t, "", "analytics", "enable")
	decodeWriteResult(t, res)

	// Verify config file was created with analytics section
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, "consent: true") {
		t.Fatalf("expected consent: true in created config, got:\n%s", content)
	}
	if !strings.Contains(content, "analytics:") {
		t.Fatalf("expected analytics: section in created config, got:\n%s", content)
	}
}

// --- Anonymous installation ID tests ---

func TestAnalyticsStatus_AnonymousInstallationIDPresent(t *testing.T) {
	newProject(t)
	writeConfig(t, `connector: file
analytics:
  consent: true
  anonymous_installation_id: "abc-123-def"
`)
	res := runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if !st.Enabled {
		t.Errorf("expected enabled=true")
	}
	if st.Source != "project_config" {
		t.Errorf("expected source=project_config, got %q", st.Source)
	}
	if !st.AnonymousInstallationIDPresent {
		t.Errorf("expected anonymous_installation_id_present=true when ID is set, got false")
	}
}

func TestAnalyticsStatus_AnonymousInstallationIDAbsent(t *testing.T) {
	newProject(t)
	writeConfig(t, `connector: file
analytics:
  consent: true
`)
	res := runCLI(t, "", "analytics", "status")
	st := decodeAnalyticsStatus(t, res)

	if !st.Enabled {
		t.Errorf("expected enabled=true")
	}
	if st.AnonymousInstallationIDPresent {
		t.Errorf("expected anonymous_installation_id_present=false when ID is absent, got true")
	}
}

// --- Error cases ---

func TestAnalyticsStatus_MalformedConfig(t *testing.T) {
	newProject(t)
	writeConfig(t, `connector: [this is not valid yaml
`)
	res := runCLI(t, "", "analytics", "status")
	if res.exit == 0 {
		t.Fatalf("expected non-zero exit for malformed config, got 0")
	}
	exit, code := decodeError(t, res)
	if exit != 2 || code != iox.CodeInvalidInput {
		t.Errorf("expected exit=2, code=%s for malformed config; got exit=%d code=%s", iox.CodeInvalidInput, exit, code)
	}
}
