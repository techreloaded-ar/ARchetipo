package verification

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/services"
)

type argvRunner struct {
	resourceRunner
	argvs [][]string
}

func (r *argvRunner) Run(ctx context.Context, root string, env []string, c Command, out io.Writer) error {
	r.argvs = append(r.argvs, c.Argv)
	return r.resourceRunner.Run(ctx, root, env, c, out)
}

func envValue(env []string, key string) string {
	value := ""
	for _, entry := range env {
		if k, v, ok := strings.Cut(entry, "="); ok && k == key {
			value = v
		}
	}
	return value
}

func serviceFixture(t *testing.T) (*Service, *argvRunner) {
	t.Helper()
	s, _, recipe := fixture(t)
	recipe.Services = map[string]services.Spec{"db": {
		Image: "postgres:17-alpine", Port: 5432, Data: "/var/lib/postgresql/data",
		Env:    map[string]string{"POSTGRES_USER": "user", "POSTGRES_PASSWORD": "password", "POSTGRES_DB": "app"},
		Expose: map[string]string{"DATABASE_URL": "postgresql://user:password@{host}:{port}/app"},
	}}
	command := func(name string) Command {
		return Command{Name: name, Argv: []string{"go", "version"}, TimeoutSeconds: 1}
	}
	recipe.Ports = []string{"API"}
	recipe.Resources = []Resource{{Name: "api", Start: command("api-start"), Probe: command("api-probe"), Stop: command("api-stop"), Stopped: command("api-stopped")}}
	writeRecipe(t, s.Root, recipe)
	runner := &argvRunner{resourceRunner: resourceRunner{fail: map[string]bool{}}}
	s.Runner = runner
	previous := serviceExecutable
	serviceExecutable = func() (string, error) { return "/opt/archetipo/bin/archetipo", nil }
	t.Cleanup(func() { serviceExecutable = previous })
	return s, runner
}

func TestDeclaredServiceStartsFirstAndExposesItsAddress(t *testing.T) {
	s, r := serviceFixture(t)
	if result := execute(t, s, "prepare"); !result.Passed {
		t.Fatalf("%+v %v", result, r.calls)
	}
	if r.calls[0] != "service-start" || r.calls[1] != "service-ready" || r.calls[2] != "api-start" {
		t.Fatalf("the service must start and be ready before the project's resources: %v", r.calls)
	}
	if got := strings.Join(r.argvs[0], " "); got != "/opt/archetipo/bin/archetipo execution environment _service start" {
		t.Fatalf("service operations run the running binary: %s", got)
	}
	var spec services.Spec
	if err := json.Unmarshal([]byte(envValue(r.envs[0], services.EnvSpec)), &spec); err != nil || spec.Image != "postgres:17-alpine" {
		t.Fatalf("the operation receives its declaration: %v %+v", err, spec)
	}
	if envValue(r.envs[0], "ARCHETIPO_RESOURCE_NAME") != "db" {
		t.Fatal("the operation names its service")
	}
	state, err := s.Resources()
	if err != nil {
		t.Fatal(err)
	}
	port := state.Ports[services.PortKey("db")]
	if port == "" || port == state.Ports["API"] {
		t.Fatalf("the service has its own port: %+v", state.Ports)
	}
	install := r.envs[len(r.envs)-2]
	if r.calls[len(r.calls)-2] != "install" {
		t.Fatalf("unexpected order %v", r.calls)
	}
	if got := envValue(install, "DATABASE_URL"); got != "postgresql://user:password@127.0.0.1:"+port+"/app" {
		t.Fatalf("preparation receives the exposed address, got %q", got)
	}
	runtime, err := s.RuntimeEnv()
	if err != nil || envValue(runtime, "DATABASE_URL") == "" {
		t.Fatalf("the preview receives the exposed address: %v", err)
	}
}

func TestCleanupStopsServicesAndRemovesTheirData(t *testing.T) {
	s, r := serviceFixture(t)
	execute(t, s, "prepare")
	r.calls = nil
	if result := execute(t, s, "cleanup"); !result.Passed {
		t.Fatalf("%+v", result)
	}
	want := "api-stop,api-stopped,service-stop,service-stopped,service-remove"
	if got := strings.Join(r.calls, ","); got != want {
		t.Fatalf("cleanup order: got %s want %s", got, want)
	}
}

func TestStopKeepsServiceDataForTheNextStart(t *testing.T) {
	s, r := serviceFixture(t)
	r.fail["install"] = true
	execute(t, s, "prepare")
	if strings.Contains(strings.Join(r.calls, ","), "service-remove") {
		t.Fatalf("a rollback stops the service but keeps its data: %v", r.calls)
	}
}

func TestDeclaredServicesAreNotWrittenBackAsResources(t *testing.T) {
	s, _ := serviceFixture(t)
	raw, err := os.ReadFile(filepath.Join(s.Root, RecipePath))
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := ConfigureCandidate(s.Root, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Resources) != 2 || recipe.Resources[0].Service == nil {
		t.Fatalf("the loaded recipe carries the derived resource first: %+v", recipe.Resources)
	}
	written, err := os.ReadFile(filepath.Join(s.Root, RecipePath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), `"service"`) || strings.Contains(string(written), "SERVICE_DB") {
		t.Fatalf("derived resources leaked into the recipe: %s", written)
	}
	again, err := ConfigureCandidate(s.Root, written)
	if err != nil || len(again.Resources) != 2 {
		t.Fatalf("a rewritten recipe must load the same way: %v %+v", err, again.Resources)
	}
}

func TestInvalidServiceDeclarationsAreRefused(t *testing.T) {
	cases := map[string]string{
		"bad image":           `{"db":{"image":"Postgres latest","port":5432,"expose":{"URL":"x"}}}`,
		"no expose":           `{"db":{"image":"postgres:17","port":5432}}`,
		"unknown placeholder": `{"db":{"image":"postgres:17","port":5432,"expose":{"URL":"{password}"}}}`,
		"reserved variable":   `{"db":{"image":"postgres:17","port":5432,"expose":{"ARCHETIPO_X":"{port}"}}}`,
		"relative data":       `{"db":{"image":"postgres:17","port":5432,"data":"data","expose":{"URL":"{port}"}}}`,
		"duplicate exposure":  `{"a":{"image":"redis:7","port":6379,"expose":{"URL":"{port}"}},"b":{"image":"redis:7","port":6379,"expose":{"URL":"{port}"}}}`,
	}
	for name, declared := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			raw := `{"version":1,"inputs":[],"outputs":[],"local_files":null,"prepare":[],"probe":[{"name":"ready","argv":["go","version"],"timeout_seconds":1}],"checks":{},"services":` + declared + `}`
			if _, err := parseRecipe(root, []byte(raw)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	root := t.TempDir()
	raw := `{"version":1,"inputs":[],"outputs":[],"local_files":null,"prepare":[],"probe":[{"name":"ready","argv":["go","version"],"timeout_seconds":1}],"checks":{},"resources":[{"name":"db","start":{"name":"a","argv":["x"],"timeout_seconds":1},"probe":{"name":"b","argv":["x"],"timeout_seconds":1},"stop":{"name":"c","argv":["x"],"timeout_seconds":1},"stopped":{"name":"d","argv":["x"],"timeout_seconds":1},"service":{"image":"postgres:17","port":5432,"expose":{"URL":"{port}"}}}]}`
	if _, err := parseRecipe(root, []byte(raw)); err == nil {
		t.Fatal("a resource must not smuggle a service declaration")
	}
}
