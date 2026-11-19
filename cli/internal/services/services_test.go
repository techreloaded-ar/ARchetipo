package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func pgSpec() Spec {
	return Spec{
		Image: "postgres:17-alpine", Port: 5432, Data: "/var/lib/postgresql/data",
		Env:    map[string]string{"POSTGRES_USER": "user", "POSTGRES_PASSWORD": "password", "POSTGRES_DB": "app"},
		Expose: map[string]string{"DATABASE_URL": "postgresql://user:password@{host}:{port}/app", "DB_PORT": "{port}"},
	}
}

func TestValidateAcceptsTheDocumentedShape(t *testing.T) {
	if err := Validate("db", pgSpec()); err != nil {
		t.Fatal(err)
	}
	pinned := pgSpec()
	pinned.Image = "ghcr.io/org/db:1@sha256:" + strings.Repeat("a", 64)
	if err := Validate("db", pinned); err != nil {
		t.Fatal(err)
	}
}

func TestExposedReplacesHostAndPort(t *testing.T) {
	got := strings.Join(Exposed(pgSpec(), "127.0.0.1", "41000"), " ")
	if got != "DATABASE_URL=postgresql://user:password@127.0.0.1:41000/app DB_PORT=41000" {
		t.Fatal(got)
	}
}

func TestImageNamesAndVersions(t *testing.T) {
	for image, want := range map[string]string{
		"postgres:17-alpine":              "postgres 17",
		"docker.io/library/postgres:16.4": "postgres 16",
		"library/postgres:15":             "postgres 15",
		"ghcr.io/org/postgres:17":         "ghcr.io/org/postgres 17",
		"localhost:5000/postgres:17":      "localhost:5000/postgres 17",
	} {
		repository, tag := imageName(image)
		major, _ := majorVersion(tag)
		if got := repository + " " + strconv.Itoa(major); got != want {
			t.Errorf("%s: got %s want %s", image, got, want)
		}
	}
	if _, ok := majorVersion("latest"); ok {
		t.Fatal("latest names no version")
	}
}

func unit(t *testing.T) Unit {
	return Unit{Name: "db", Spec: pgSpec(), Port: "41000", Owner: "resource-1", Environment: "env-abc", State: t.TempDir(), Log: io.Discard}
}

func TestRunArgsPublishOnlyOnLoopbackAndMountOnlyTheDataVolume(t *testing.T) {
	got := strings.Join(runArgs(unit(t)), " ")
	want := "run -d --name archetipo-svc-abc-db --label archetipo.service.environment=env-abc --label archetipo.resource.owner=resource-1 --label archetipo.resource.name=db -p 127.0.0.1:41000:5432 -e POSTGRES_DB=app -e POSTGRES_PASSWORD=password -e POSTGRES_USER=user -v archetipo-svc-abc-db:/var/lib/postgresql/data postgres:17-alpine"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

type fakeEngine struct {
	calls     []string
	container string // inspect output, empty for none
	volume    string
}

func (f *fakeEngine) run(_ context.Context, _ io.Writer, args ...string) (string, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case strings.HasPrefix(call, "container inspect"):
		if f.container == "" {
			return "", errors.New("Error: No such container")
		}
		return f.container, nil
	case strings.HasPrefix(call, "volume inspect"):
		if f.volume == "" {
			return "", errors.New("Error: No such volume")
		}
		return f.volume, nil
	case strings.HasPrefix(call, "run "):
		f.container = "true env-abc"
	case strings.HasPrefix(call, "rm "):
		f.container = ""
	case strings.HasPrefix(call, "volume create"):
		f.volume = "env-abc"
	case strings.HasPrefix(call, "volume rm"):
		f.volume = ""
	}
	return "", nil
}

func TestDockerLifecycleKeepsDataUntilRemove(t *testing.T) {
	engine := &fakeEngine{}
	d := &Docker{Run: engine.run, Probe: func(context.Context, string) error { return nil }}
	u := unit(t)
	ctx := context.Background()
	if err := Run(ctx, d, OpStart, u); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, d, OpStop, u); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, d, OpStopped, u); err != nil {
		t.Fatal(err)
	}
	if engine.volume == "" {
		t.Fatal("stop must keep the data volume")
	}
	if err := Run(ctx, d, OpRemove, u); err != nil {
		t.Fatal(err)
	}
	if engine.volume != "" {
		t.Fatal("remove deletes the data volume")
	}
}

func TestDockerRefusesAContainerItDoesNotOwn(t *testing.T) {
	engine := &fakeEngine{container: "true "}
	d := &Docker{Run: engine.run, Probe: func(context.Context, string) error { return nil }}
	for _, op := range []string{OpStart, OpStop, OpRemove} {
		if err := Run(context.Background(), d, op, unit(t)); err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("%s: %v", op, err)
		}
	}
	for _, call := range engine.calls {
		if strings.HasPrefix(call, "rm ") {
			t.Fatal("a foreign container must never be removed")
		}
	}
}

func listen(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serve(conn)
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func TestProbeTellsAServiceFromAnEmptyForwarder(t *testing.T) {
	ctx := context.Background()
	waits := listen(t, func(c net.Conn) { time.Sleep(time.Second); _ = c.Close() })
	greets := listen(t, func(c net.Conn) { _, _ = c.Write([]byte("hello")); _ = c.Close() })
	closes := listen(t, func(c net.Conn) { _ = c.Close() })
	if err := probeTCP(ctx, waits); err != nil {
		t.Fatalf("a service waiting for its client is ready: %v", err)
	}
	if err := probeTCP(ctx, greets); err != nil {
		t.Fatalf("a service that greets is ready: %v", err)
	}
	if err := probeTCP(ctx, closes); err == nil {
		t.Fatal("a forwarder that closes at once is not a ready service")
	}
}

func TestIsolatedRuntimeRefusesAnUnavailableServiceClearly(t *testing.T) {
	p := &Process{PostgresRoot: t.TempDir()}
	u := unit(t)
	u.Spec.Image = "mysql:8"
	err := p.Start(context.Background(), u)
	if err == nil || !strings.Contains(err.Error(), "not available in the isolated environment") {
		t.Fatal(err)
	}
	u.Spec.Image = "postgres:17"
	if err = p.Start(context.Background(), u); err == nil || !strings.Contains(err.Error(), "PostgreSQL 17 is not available") {
		t.Fatal(err)
	}
}

func TestUnitFromEnvRequiresTheLifecycle(t *testing.T) {
	env := map[string]string{
		EnvSpec:                       `{"image":"postgres:17","port":5432,"expose":{"URL":"{port}"}}`,
		"ARCHETIPO_RESOURCE_NAME":     "db",
		"ARCHETIPO_RESOURCE_OWNER":    "resource-1",
		"ARCHETIPO_ENVIRONMENT_ID":    "env-abc",
		"ARCHETIPO_ENVIRONMENT_STATE": "/tmp/state",
		"ARCHETIPO_PORT_SERVICE_DB":   "41000",
	}
	u, err := UnitFromEnv(func(k string) string { return env[k] }, nil)
	if err != nil || u.Port != "41000" || u.Spec.Port != 5432 {
		t.Fatalf("%v %+v", err, u)
	}
	delete(env, "ARCHETIPO_PORT_SERVICE_DB")
	if _, err = UnitFromEnv(func(k string) string { return env[k] }, nil); err == nil {
		t.Fatal("an operation outside the lifecycle must be refused")
	}
}

// TestInstalledPostgresLifecycle runs the real server where PostgreSQL is
// installed in the Debian layout, as in the isolated image.
func TestInstalledPostgresLifecycle(t *testing.T) {
	root := os.Getenv(EnvPostgresRoot)
	if root == "" {
		root = "/usr/lib/postgresql"
	}
	if _, err := os.Stat(filepath.Join(root, "17", "bin", "postgres")); err != nil {
		t.Skip("PostgreSQL 17 not installed in " + root)
	}
	if os.Geteuid() == 0 {
		t.Skip("PostgreSQL refuses to run as root")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()
	var log bytes.Buffer
	u := unit(t)
	u.Port, u.Log = port, &log
	p := &Process{PostgresRoot: root}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	defer func() { _ = p.Remove(context.Background(), u) }()
	if err = Run(ctx, p, OpStart, u); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	psql := func() string {
		cmd := exec.Command(filepath.Join(root, "17", "bin", "psql"), "-h", "127.0.0.1", "-p", port, "-U", "user", "-d", "app", "-tAc", "select 1")
		cmd.Env = append(os.Environ(), "PGPASSWORD=password")
		out, _ := cmd.CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	if got := psql(); got != "1" {
		t.Fatalf("the declared database answers with the declared password: %q", got)
	}
	if err = Run(ctx, p, OpStop, u); err != nil {
		t.Fatal(err)
	}
	if err = Run(ctx, p, OpStopped, u); err != nil {
		t.Fatal(err)
	}
	if err = Run(ctx, p, OpStart, u); err != nil {
		t.Fatalf("restart on kept data: %v\n%s", err, log.String())
	}
	if err = p.Remove(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(u.State, "services", "db")); !os.IsNotExist(err) {
		t.Fatal("remove deletes the data")
	}
}
