package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHub is a hub that answers the two routes `execution setup` reads. The
// workspaces it serves are the whole variable of these tests: how many there
// are, and whether anything could run their work.
func setupHub(t *testing.T, token string, workspacesJSON string) string {
	t.Helper()
	ids := []string{}
	var parsed struct {
		Workspaces []struct {
			ID string `json:"id"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(workspacesJSON), &parsed); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	for _, workspace := range parsed.Workspaces {
		ids = append(ids, `"`+workspace.ID+`"`)
	}
	identity := `{"kind":"application","identity":{"id":"app-1","name":"archetipo","workspaceIds":[` +
		strings.Join(ids, ",") + `]}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/external/me":
			_, _ = w.Write([]byte(identity))
		case "/api/external/workspaces":
			_, _ = w.Write([]byte(workspacesJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

const oneReadyWorkspace = `{"workspaces":[{"id":"ws-0001","name":"demo","cwdHint":"/workspace",` +
	`"requirements":["project:demo"],"archived":false,` +
	`"eligibleRunners":{"known":2,"online":1,"missing":[]}}]}`

const twoWorkspaces = `{"workspaces":[{"id":"ws-0001","name":"demo","cwdHint":"/workspace",` +
	`"requirements":[],"archived":false,"eligibleRunners":{"known":2,"online":1,"missing":[]}},` +
	`{"id":"ws-0002","name":"staging","requirements":["project:absent"],"archived":false,` +
	`"eligibleRunners":{"known":0,"online":0,"missing":["project:absent"]}}]}`

// persistedProvider reads back what the command wrote.
func persistedProvider(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("reading the written config: %v", err)
	}
	out := runCLI(t, "", "execution", "provider", "show-default")
	if out.exit != 0 {
		t.Fatalf("show-default exit = %d, stderr = %s\nfile:\n%s", out.exit, out.stderr.String(), raw)
	}
	var envelope struct {
		Data struct {
			ID     string         `json:"id"`
			Config map[string]any `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding show-default: %v", err)
	}
	if envelope.Data.ID != "arcipelago" {
		t.Fatalf("persisted provider = %q, want arcipelago", envelope.Data.ID)
	}
	return envelope.Data.Config
}

func TestExecutionSetupChoosesTheOnlyWorkspaceWithoutAsking(t *testing.T) {
	newProject(t)
	baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
	t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

	// No stdin at all: a question asked here would fail the test by blocking on
	// an empty reader, which is exactly the point — with one destination there
	// is nothing to ask.
	out := runCLI(t, "", "execution", "setup", "--url", baseURL)
	if out.exit != 0 {
		t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
	}
	config := persistedProvider(t)
	if config["workspace_id"] != "ws-0001" {
		t.Fatalf("workspace_id = %v, want ws-0001", config["workspace_id"])
	}
	if config["base_url"] != baseURL {
		t.Fatalf("base_url = %v, want %s", config["base_url"], baseURL)
	}
	if config["token_env"] != "ARCIPELAGO_TOKEN" {
		t.Fatalf("token_env = %v, want the name of the variable", config["token_env"])
	}
}

func TestExecutionSetupNeverWritesTheCredentialAnywhere(t *testing.T) {
	newProject(t)
	const secret = "arc_app_do_not_persist_me"
	baseURL := setupHub(t, secret, oneReadyWorkspace)
	t.Setenv("ARCIPELAGO_TOKEN", secret)

	out := runCLI(t, "", "execution", "setup", "--url", baseURL)
	if out.exit != 0 {
		t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
	}
	written, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// The whole point of naming a variable instead of holding a value: the
	// secret must not be in the file, nor in anything the command printed.
	if strings.Contains(string(written), secret) {
		t.Fatal("the credential was written into .archetipo/config.yaml")
	}
	if strings.Contains(out.stdout.String(), secret) || strings.Contains(out.stderr.String(), secret) {
		t.Fatal("the credential was echoed back to the terminal")
	}
}

func TestExecutionSetupWithoutACredentialSaysWhichLineToRun(t *testing.T) {
	newProject(t)
	baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
	t.Setenv("ARCIPELAGO_TOKEN", "")

	out := runCLI(t, "", "execution", "setup", "--url", baseURL)
	if out.exit == 0 {
		t.Fatal("setup succeeded without a credential")
	}
	message := out.stdout.String() + out.stderr.String()
	if !strings.Contains(message, "ARCIPELAGO_TOKEN") || !strings.Contains(message, "apps create") {
		t.Fatalf("output = %s, want the variable and how to obtain a token", message)
	}
	// Nothing is written when nothing was verified: a configuration saved here
	// would fail much later, at the first dispatch.
	if _, err := os.Stat(filepath.Join(".archetipo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("a configuration was written despite the failure")
	}
}

func TestExecutionSetupWithSeveralWorkspaces(t *testing.T) {
	t.Run("non-interactive names the flag and the choices", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", twoWorkspaces)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

		out := runCLI(t, "", "execution", "setup", "--url", baseURL, "--non-interactive")
		if out.exit == 0 {
			t.Fatal("setup picked a workspace on its own where a person had to")
		}
		message := out.stdout.String() + out.stderr.String()
		if !strings.Contains(message, "--workspace") || !strings.Contains(message, "staging") {
			t.Fatalf("output = %s, want the flag and the names to choose from", message)
		}
	})

	t.Run("--workspace resolves by name", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", twoWorkspaces)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

		out := runCLI(t, "", "execution", "setup", "--url", baseURL,
			"--workspace", "staging", "--non-interactive")
		if out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}
		if config := persistedProvider(t); config["workspace_id"] != "ws-0002" {
			t.Fatalf("workspace_id = %v, want ws-0002", config["workspace_id"])
		}
	})

	t.Run("an ambiguous id prefix is refused rather than guessed", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", twoWorkspaces)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

		// "ws-000" prefixes both. The value would be written to a file and used
		// from then on, so picking one is worse than asking again.
		out := runCLI(t, "", "execution", "setup", "--url", baseURL,
			"--workspace", "ws-000", "--non-interactive")
		if out.exit == 0 {
			t.Fatal("an ambiguous workspace prefix was resolved silently")
		}
		if !strings.Contains(out.stdout.String()+out.stderr.String(), "matches 2 workspaces") {
			t.Fatalf("output = %s, want the ambiguity named", out.stdout.String()+out.stderr.String())
		}
	})

	t.Run("interactive selection warns about a destination that cannot run", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", twoWorkspaces)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

		out := runCLI(t, "2\n", "execution", "setup", "--url", baseURL)
		if out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}
		if !strings.Contains(out.stdout.String(), "project:absent") {
			t.Fatalf("the menu did not say why staging cannot run: %s", out.stdout.String())
		}
		if !strings.Contains(out.stderr.String(), "not ready") {
			t.Fatalf("no warning was issued: %s", out.stderr.String())
		}
		// Warned, not blocked: the hub is finished by somebody else, later.
		if config := persistedProvider(t); config["workspace_id"] != "ws-0002" {
			t.Fatalf("workspace_id = %v, want the chosen one written anyway", config["workspace_id"])
		}
	})
}

func TestExecutionSetupLeavesTheConfigurationAloneWhenTheHubRefuses(t *testing.T) {
	newProject(t)
	baseURL := setupHub(t, "the-real-token", oneReadyWorkspace)
	t.Setenv("ARCIPELAGO_TOKEN", "a-stale-token")

	out := runCLI(t, "", "execution", "setup", "--url", baseURL)
	if out.exit == 0 {
		t.Fatal("setup wrote a configuration the hub rejects")
	}
	if !strings.Contains(out.stdout.String()+out.stderr.String(), "credential") {
		t.Fatalf("output = %s, want the credential named as the cause", out.stdout.String())
	}
	if _, err := os.Stat(filepath.Join(".archetipo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("a rejected configuration was written")
	}
}

func TestExecutionSetupWithoutProbingAsksTheHubNothing(t *testing.T) {
	newProject(t)
	// A URL nothing is listening on: --no-probe must not reach for it. This is
	// the case of a machine that cannot see the hub but still has to be set up.
	out := runCLI(t, "", "execution", "setup",
		"--url", "http://127.0.0.1:1", "--workspace", "ws-0001", "--no-probe")
	if out.exit != 0 {
		t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
	}
	if config := persistedProvider(t); config["workspace_id"] != "ws-0001" {
		t.Fatalf("workspace_id = %v, want the one passed", config["workspace_id"])
	}
}

func TestExecutionSetupReusesWhatTheProjectAlreadyHas(t *testing.T) {
	newProject(t)
	baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
	t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")

	first := runCLI(t, "", "execution", "setup", "--url", baseURL, "--timeout", "120")
	if first.exit != 0 {
		t.Fatalf("first setup exit = %d, stderr = %s", first.exit, first.stderr.String())
	}
	// Reconfiguring names neither the URL nor the timeout, and must keep both:
	// re-running setup to change one thing cannot mean retyping the rest.
	second := runCLI(t, "", "execution", "setup", "--non-interactive")
	if second.exit != 0 {
		t.Fatalf("second setup exit = %d, stderr = %s", second.exit, second.stderr.String())
	}
	config := persistedProvider(t)
	if config["base_url"] != baseURL {
		t.Fatalf("base_url = %v, want the one already configured", config["base_url"])
	}
	if fmtInt(config["timeout_seconds"]) != 120 {
		t.Fatalf("timeout_seconds = %v, want 120 carried over", config["timeout_seconds"])
	}
}

func TestExecutionProviderListNamesEveryProviderAndWhyOneIsUnusable(t *testing.T) {
	newProject(t)
	t.Setenv("ARCIPELAGO_TOKEN", "")

	out := runCLI(t, "", "execution", "provider", "list")
	if out.exit != 0 {
		t.Fatalf("list exit = %d, stderr = %s", out.exit, out.stderr.String())
	}
	var envelope struct {
		Data struct {
			Providers []struct {
				ID           string   `json:"id"`
				Capabilities []string `json:"capabilities"`
				ConfigFields []struct {
					Name string `json:"name"`
				} `json:"config_fields"`
				Available bool `json:"available"`
				IsDefault bool `json:"is_default"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding the listing: %v", err)
	}
	providers := envelope.Data.Providers
	if len(providers) < 3 {
		t.Fatalf("got %d providers, want the three registered ones", len(providers))
	}
	// Registration order is display order, and arcipelago is registered first.
	if providers[0].ID != "arcipelago" {
		t.Fatalf("first provider = %q, want arcipelago", providers[0].ID)
	}
	if len(providers[0].ConfigFields) == 0 {
		t.Fatal("the listing does not say which keys arcipelago accepts")
	}
	if len(providers[0].Capabilities) == 0 {
		t.Fatal("the listing does not say what arcipelago can do")
	}
	for _, provider := range providers {
		if provider.IsDefault {
			t.Fatalf("provider %q is marked default in a project that has none", provider.ID)
		}
	}
	// The listing never prints a secret, whatever the environment holds.
	if strings.Contains(out.stdout.String(), "arc_app_") {
		t.Fatal("the listing leaked something that looks like a credential")
	}
}

func fmtInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return -1
	}
}

func TestDoctorDiagnosesTheExecutionProvider(t *testing.T) {
	t.Run("says nothing when no provider is configured", func(t *testing.T) {
		newProject(t)
		out := runCLI(t, "", "doctor")
		report := out.stdout.String()
		if !strings.Contains(report, "execution credential: skipped (no default execution provider)") {
			t.Fatalf("report = %s, want both execution lines skipped", report)
		}
		if !strings.Contains(report, "execution provider: skipped (no default execution provider)") {
			t.Fatalf("report = %s, want the probe skipped too", report)
		}
	})

	t.Run("names the missing variable without touching the network", func(t *testing.T) {
		newProject(t)
		// A hub that would answer, and a credential that is not exported. The
		// diagnosis must be the local one: it is both the commonest failure and
		// the one that needs no round trip.
		baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")
		if out := runCLI(t, "", "execution", "setup", "--url", baseURL); out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}
		t.Setenv("ARCIPELAGO_TOKEN", "")

		out := runCLI(t, "", "doctor")
		report := out.stdout.String()
		if out.exit == 0 {
			t.Fatal("doctor passed with an unusable execution provider")
		}
		if !strings.Contains(report, "ARCIPELAGO_TOKEN is not set") {
			t.Fatalf("report = %s, want the variable named", report)
		}
		if !strings.Contains(report, "execution provider: skipped (credential missing)") {
			t.Fatalf("report = %s, want the probe skipped rather than repeating the cause", report)
		}
	})

	t.Run("carries the provider's own sentence when the hub refuses", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")
		if out := runCLI(t, "", "execution", "setup", "--url", baseURL); out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}
		t.Setenv("ARCIPELAGO_TOKEN", "a-stale-token")

		out := runCLI(t, "", "doctor")
		report := out.stdout.String()
		if out.exit == 0 {
			t.Fatal("doctor passed against a hub that rejects the credential")
		}
		if !strings.Contains(report, "rejected the application credential") {
			t.Fatalf("report = %s, want the provider's own diagnosis", report)
		}
	})

	t.Run("passes when everything is in place", func(t *testing.T) {
		newProject(t)
		baseURL := setupHub(t, "arc_app_test", oneReadyWorkspace)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")
		if out := runCLI(t, "", "execution", "setup", "--url", baseURL); out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}

		out := runCLI(t, "", "doctor")
		report := out.stdout.String()
		if !strings.Contains(report, "✓ execution provider: arcipelago is ready to dispatch") {
			t.Fatalf("report = %s, want the provider reported ready", report)
		}
		if strings.Contains(report, "arc_app_test") {
			t.Fatal("doctor printed the credential")
		}
	})

	t.Run("--offline skips only the check that needs a network", func(t *testing.T) {
		newProject(t)
		t.Setenv("ARCIPELAGO_TOKEN", "arc_app_test")
		// A URL nothing answers: without --offline this check would fail.
		if out := runCLI(t, "", "execution", "setup",
			"--url", "http://127.0.0.1:1", "--workspace", "ws-0001", "--no-probe"); out.exit != 0 {
			t.Fatalf("setup exit = %d, stderr = %s", out.exit, out.stderr.String())
		}

		offlineRun := runCLI(t, "", "doctor", "--offline")
		report := offlineRun.stdout.String()
		if !strings.Contains(report, "✓ execution credential") {
			t.Fatalf("report = %s, want the local half still checked", report)
		}
		if !strings.Contains(report, "execution provider: skipped (--offline)") {
			t.Fatalf("report = %s, want the remote half skipped", report)
		}
	})
}
