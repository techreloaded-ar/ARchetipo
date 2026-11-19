package isolated

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStoreGivesEachEnvironmentItsOwnPortBlockAndKeepsIt(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	first, err := store.Ensure(DefaultID, "image:1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Ensure("client-a", "image:1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ViewPort != 47800 || second.ViewPort != 47900 {
		t.Fatalf("view ports %d and %d", first.ViewPort, second.ViewPort)
	}
	if first.PreviewRange() != "47810-47829" || second.PreviewRange() != "47910-47929" {
		t.Fatalf("preview ranges %s and %s", first.PreviewRange(), second.PreviewRange())
	}
	again, err := store.Ensure(DefaultID, "image:2")
	if err != nil {
		t.Fatal(err)
	}
	if again.ViewPort != first.ViewPort || again.Image != "image:2" || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("existing environment changed identity: %+v", again)
	}
	if err = store.Forget(DefaultID); err != nil {
		t.Fatal(err)
	}
	reused, err := store.Ensure("client-b", "image:1")
	if err != nil {
		t.Fatal(err)
	}
	if reused.ViewPort != 47800 {
		t.Fatalf("forgotten block not reused: %d", reused.ViewPort)
	}
	if first.Gateway() == second.Gateway() || first.Volume() == second.Volume() || first.Network() == second.Network() {
		t.Fatal("two environments share an engine object name")
	}
}

func TestStoreCanStartFromASeparatePortBlock(t *testing.T) {
	t.Setenv(EnvFirstPort, "48000")
	store := Store{Dir: t.TempDir()}
	env, err := store.Ensure(DefaultID, "image:1")
	if err != nil {
		t.Fatal(err)
	}
	if env.ViewPort != 48000 || env.PreviewRange() != "48010-48029" {
		t.Fatalf("unexpected block: %+v (%s)", env, env.PreviewRange())
	}
	t.Setenv(EnvFirstPort, "invalid")
	if _, err = (Store{Dir: t.TempDir()}).Ensure(DefaultID, "image:1"); err == nil {
		t.Fatal("invalid first port accepted")
	}
}

func TestEnvironmentIDsMustNameEngineObjectsSafely(t *testing.T) {
	for _, id := range []string{"", "Default", "-x", "a b", "a/b", "../x", strings.Repeat("a", 33)} {
		if ValidateID(id) == nil {
			t.Fatalf("accepted id %q", id)
		}
	}
	for _, id := range []string{"default", "a", "client-1"} {
		if err := ValidateID(id); err != nil {
			t.Fatalf("refused id %q: %v", id, err)
		}
	}
}

func TestContainersKeepTheBoundary(t *testing.T) {
	env := Environment{ID: "default", ViewPort: 47800, Image: "img"}
	gateway := gatewayArgs(env)
	workspace := workspaceArgs(env)
	for _, args := range [][]string{gateway, workspace} {
		joined := strings.Join(args, " ")
		for _, forbidden := range []string{"--privileged", "docker.sock", "/Users", "--pid", "--ipc"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("container args contain %q: %s", forbidden, joined)
			}
		}
		if !slices.Contains(args, "no-new-privileges") || !hasPair(args, "--cap-drop", "ALL") {
			t.Fatalf("container keeps privileges: %s", joined)
		}
	}
	for index, arg := range gateway {
		if arg == "-p" && !strings.HasPrefix(gateway[index+1], "127.0.0.1:") {
			t.Fatalf("port published beyond host loopback: %s", gateway[index+1])
		}
	}
	if !hasPair(gateway, "-p", "127.0.0.1:47800:47800") || !hasPair(gateway, "-p", "127.0.0.1:47810-47829:47810-47829") {
		t.Fatalf("gateway does not publish the viewer and preview range: %v", gateway)
	}
	if hasPair(gateway, "--cap-add", "NET_ADMIN") == false {
		t.Fatal("gateway cannot install its rules")
	}
	if slices.Contains(workspace, "--cap-add") || slices.Contains(workspace, "-p") {
		t.Fatalf("workspace gains a capability or publishes a port: %v", workspace)
	}
	if !hasPair(workspace, "--network", "container:archetipo-default-gateway") {
		t.Fatalf("workspace does not join the gateway namespace: %v", workspace)
	}
	mounts := 0
	for index, arg := range workspace {
		if arg == "-v" {
			mounts++
			if workspace[index+1] != "archetipo-default-home:/home/archetipo" {
				t.Fatalf("unexpected mount %s", workspace[index+1])
			}
		}
	}
	if mounts != 1 {
		t.Fatalf("workspace has %d mounts", mounts)
	}
	if !hasPair(workspace, "-e", "ARCHETIPO_RUNTIME=isolated") {
		t.Fatalf("workspace does not declare the isolated runtime: %v", workspace)
	}
	if !hasPair(workspace, "-e", "ARCHETIPO_CLONE_DIR=/home/archetipo/workspaces") {
		t.Fatalf("workspace does not offer cloning inside its volume: %v", workspace)
	}
	if !hasPair(workspace, "-e", "ARCHETIPO_PREVIEW_PORTS=47810-47829") || !hasPair(workspace, "-e", "ARCHETIPO_PREVIEW_BIND=0.0.0.0") {
		t.Fatalf("workspace preview policy missing: %v", workspace)
	}
}

func TestGatewayRejectsEveryLocalRangeAndNeverAcceptsAll(t *testing.T) {
	rules := GatewayRules()
	for _, network := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16"} {
		if !slices.ContainsFunc(rules, func(rule []string) bool {
			return rule[0] == "iptables" && hasPair(rule, "-d", network) && hasPair(rule, "-j", "REJECT")
		}) {
			t.Fatalf("no rejection for %s", network)
		}
	}
	for _, rule := range rules {
		if hasPair(rule, "-j", "ACCEPT") && !slices.Contains(rule, "lo") && !slices.Contains(rule, "conntrack") {
			t.Fatalf("broad accept rule: %v", rule)
		}
		if slices.Contains(rule, "-P") {
			t.Fatalf("policy change: %v", rule)
		}
	}
	last := rules[len(rules)-1]
	if last[0] != "ip6tables" || !hasPair(last, "-j", "REJECT") {
		t.Fatalf("IPv6 egress not closed: %v", last)
	}
}

func TestUpStartsTheWorkspaceOnlyBehindAReadyGateway(t *testing.T) {
	env := Environment{ID: "default", ViewPort: 47800, Image: "img"}
	var calls []string
	docker := &Docker{Binary: "docker", Run: func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[len(args)-1] == env.Gateway() && args[0] == "container" {
			return "", errors.New("no such container")
		}
		return "", nil
	}}
	err := docker.Up(context.Background(), env, func(context.Context) error { return errors.New("rules failed") })
	if err == nil {
		t.Fatal("Up succeeded with a gateway that never became ready")
	}
	for _, call := range calls {
		if strings.Contains(call, "--name "+env.Workspace()) {
			t.Fatalf("workspace started behind a gateway without rules: %s", call)
		}
	}
	calls = nil
	if err = docker.Up(context.Background(), env, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	gatewayAt, workspaceAt := -1, -1
	for index, call := range calls {
		if strings.Contains(call, "run -d --name "+env.Gateway()) {
			gatewayAt = index
		}
		if strings.Contains(call, "run -d --name "+env.Workspace()) {
			workspaceAt = index
		}
	}
	if gatewayAt < 0 || workspaceAt < gatewayAt {
		t.Fatalf("start order gateway=%d workspace=%d: %v", gatewayAt, workspaceAt, calls)
	}
}

type fakeProvider struct {
	unavailable error
	steps       []string
}

func (f *fakeProvider) Available(context.Context) error {
	f.steps = append(f.steps, "available")
	return f.unavailable
}
func (f *fakeProvider) EnsureImage(_ context.Context, image string, _ Progress) error {
	f.steps = append(f.steps, "image "+image)
	return nil
}
func (f *fakeProvider) Up(ctx context.Context, env Environment, ready func(context.Context) error) error {
	f.steps = append(f.steps, "up "+env.ID)
	return ready(ctx)
}
func (f *fakeProvider) GatewayReady(context.Context, Environment) bool { return true }
func (f *fakeProvider) Logs(context.Context, string) string            { return "" }
func (f *fakeProvider) Down(_ context.Context, env Environment) error {
	f.steps = append(f.steps, "down "+env.ID)
	return nil
}
func (f *fakeProvider) Remove(_ context.Context, env Environment) error {
	f.steps = append(f.steps, "remove "+env.ID)
	return nil
}

func TestStartRecordsTheEnvironmentAndWaitsForTheViewer(t *testing.T) {
	defer func(previous func(context.Context, string) bool, timeout time.Duration) {
		viewerAnswers, ViewerTimeout = previous, timeout
	}(viewerAnswers, ViewerTimeout)
	asked := 0
	viewerAnswers = func(_ context.Context, url string) bool {
		asked++
		if url != "http://127.0.0.1:47800/" {
			t.Fatalf("waited on %s", url)
		}
		return asked > 1
	}
	store := Store{Dir: t.TempDir()}
	provider := &fakeProvider{}
	var progress []string
	env, err := Start(context.Background(), provider, store, DefaultID, "img", func(step string) { progress = append(progress, step) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(provider.steps, ",") != "available,image img,up default" || len(progress) != 2 || asked != 2 {
		t.Fatalf("steps %v progress %v asked %d", provider.steps, progress, asked)
	}
	if recorded, found, _ := store.Get(DefaultID); !found || recorded.ViewPort != env.ViewPort {
		t.Fatalf("environment not recorded: %+v", recorded)
	}
	if _, err = Stop(context.Background(), provider, store, DefaultID); err != nil || provider.steps[len(provider.steps)-1] != "down default" {
		t.Fatalf("stop: %v %v", err, provider.steps)
	}
	if _, err = Remove(context.Background(), provider, store, DefaultID); err != nil || provider.steps[len(provider.steps)-1] != "remove default" {
		t.Fatalf("remove: %v %v", err, provider.steps)
	}
	if _, found, getErr := store.Get(DefaultID); getErr != nil || found {
		t.Fatalf("removed environment still recorded: found=%v err=%v", found, getErr)
	}

	stopped := &fakeProvider{unavailable: errors.New("Docker non è avviato.")}
	if _, err = Start(context.Background(), stopped, Store{Dir: t.TempDir()}, DefaultID, "img", func(string) {}); err == nil || len(stopped.steps) != 1 {
		t.Fatalf("started without an engine: %v %v", err, stopped.steps)
	}
	ViewerTimeout = 10 * time.Millisecond
	viewerAnswers = func(context.Context, string) bool { return false }
	if _, err = Start(context.Background(), &fakeProvider{}, Store{Dir: t.TempDir()}, DefaultID, "img", func(string) {}); err == nil {
		t.Fatal("Start returned before the viewer answered")
	}
}

func TestDockerRemoveDestroysContainersNetworkAndVolume(t *testing.T) {
	env := Environment{ID: "client-a", ViewPort: 47800, Image: "img"}
	exists := map[string]bool{
		"container/" + env.Workspace(): true,
		"container/" + env.Gateway():   true,
		"network/" + env.Network():     true,
		"volume/" + env.Volume():       true,
	}
	var removed []string
	docker := &Docker{Binary: "docker"}
	docker.Run = func(_ context.Context, args ...string) (string, error) {
		if len(args) >= 3 && args[1] == "inspect" {
			if exists[args[0]+"/"+args[2]] {
				return "present", nil
			}
			return "", errors.New("missing")
		}
		if len(args) == 3 && args[0] == "rm" && args[1] == "-f" {
			removed = append(removed, "container/"+args[2])
			return "", nil
		}
		if len(args) == 3 && args[1] == "rm" {
			removed = append(removed, args[0]+"/"+args[2])
			return "", nil
		}
		return "", nil
	}
	if err := docker.Remove(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"container/" + env.Workspace(),
		"container/" + env.Gateway(),
		"network/" + env.Network(),
		"volume/" + env.Volume(),
	}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed %v, want %v", removed, want)
	}
}

func TestDevelopmentBuildsUseTheLocallyBuiltImage(t *testing.T) {
	for version, want := range map[string]string{
		"":                         devImage,
		"dev":                      devImage,
		"0.0.0-dev.g8ca7881":       devImage,
		"0.0.0-dev.g8ca7881.dirty": devImage,
		"v3.1.0":                   releaseImage + ":3.1.0",
		"3.1.0":                    releaseImage + ":3.1.0",
	} {
		if got := imageFor(version); got != want {
			t.Fatalf("imageFor(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestDevelopmentBinaryBuildsItsImageFromTheCheckout(t *testing.T) {
	checkout := t.TempDir()
	dockerfile := filepath.Join(checkout, "docker", "isolated", "Dockerfile")
	if err := os.MkdirAll(filepath.Dir(dockerfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerfile, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const devVersion = "0.0.0-dev.g8ca7881"
	engine := func(builtFor *string, calls *[]string) Runner {
		return func(_ context.Context, args ...string) (string, error) {
			*calls = append(*calls, strings.Join(args, " "))
			switch {
			case args[0] == "image" && args[1] == "inspect":
				if *builtFor == "" {
					return "", errors.New("no such image")
				}
				return *builtFor, nil
			case args[0] == "build":
				*builtFor = devVersion
				return "", nil
			case args[0] == "pull":
				return "", errors.New("pull access denied")
			}
			return "", nil
		}
	}

	builtFor, calls := "", []string{}
	docker := &Docker{Binary: "docker", Run: engine(&builtFor, &calls), SourceCheckout: checkout, Version: devVersion}
	var progress []string
	if err := docker.EnsureImage(context.Background(), devImage, func(step string) { progress = append(progress, step) }); err != nil {
		t.Fatalf("a missing development image was not built: %v", err)
	}
	want := "build -f " + dockerfile + " -t " + devImage + " --build-arg VERSION=" + devVersion + " --label " + imageVersionLabel + "=" + devVersion + " " + checkout
	if len(calls) != 2 || calls[1] != want || len(progress) != 1 {
		t.Fatalf("calls %v progress %v, want build %q", calls, progress, want)
	}

	calls = nil
	if err := docker.EnsureImage(context.Background(), devImage, func(string) {}); err != nil || len(calls) != 1 {
		t.Fatalf("an image built for this version was rebuilt: %v %v", err, calls)
	}

	builtFor, calls = "0.0.0-dev.gaaaaaaa", nil
	if err := docker.EnsureImage(context.Background(), devImage, func(string) {}); err != nil || len(calls) != 2 || !strings.HasPrefix(calls[1], "build ") {
		t.Fatalf("an image of another version was not rebuilt: %v %v", err, calls)
	}

	builtFor, calls = "", nil
	installedWithoutCheckout := &Docker{Binary: "docker", Run: engine(&builtFor, &calls), Version: devVersion}
	if err := installedWithoutCheckout.EnsureImage(context.Background(), devImage, func(string) {}); err == nil || !strings.Contains(calls[len(calls)-1], "pull") {
		t.Fatalf("a binary without a checkout did not fall back to pulling: %v %v", err, calls)
	}
}

func TestDockerAvailableRequiresLinuxContainers(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, engineOS := range []string{"linux", "windows"} {
		t.Run(engineOS, func(t *testing.T) {
			docker := Docker{Binary: binary, Run: func(_ context.Context, args ...string) (string, error) {
				if !slices.Equal(args, []string{"info", "--format", "{{.OSType}}"}) {
					t.Fatalf("unexpected engine command: %v", args)
				}
				return engineOS, nil
			}}
			err := docker.Available(context.Background())
			if (err == nil) != (engineOS == "linux") {
				t.Fatalf("Docker.Available() on %s: %v", engineOS, err)
			}
		})
	}
}

// shortEngineLimits makes the engine limits fit a test and restores them.
func shortEngineLimits(t *testing.T) {
	t.Helper()
	probe, command := EngineProbeTimeout, EngineCommandTimeout
	EngineProbeTimeout, EngineCommandTimeout = 50*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { EngineProbeTimeout, EngineCommandTimeout = probe, command })
}

// returnsWithin fails the test when operation is still running after limit.
func returnsWithin(t *testing.T, limit time.Duration, operation func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- operation() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("still waiting after %s", limit)
		return nil
	}
}

func recordedStore(t *testing.T) Store {
	t.Helper()
	store := Store{Dir: t.TempDir()}
	if _, err := store.Ensure(DefaultID, "img"); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStopReturnsWhenTheEngineNeverAnswers(t *testing.T) {
	shortEngineLimits(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var asked [][]string
	docker := &Docker{Binary: binary, Run: func(ctx context.Context, args ...string) (string, error) {
		asked = append(asked, args)
		<-ctx.Done()
		return "", ctx.Err()
	}}
	err = returnsWithin(t, 5*time.Second, func() error {
		_, stopErr := Stop(context.Background(), docker, recordedStore(t), DefaultID)
		return stopErr
	})
	if !errors.Is(err, errEngineUnresponsive) || !strings.Contains(err.Error(), "Docker non risponde") {
		t.Fatalf("Stop on a silent engine: %v", err)
	}
	if len(asked) != 1 || asked[0][0] != "info" {
		t.Fatalf("went on after the probe timed out: %v", asked)
	}
}

func TestStopNeverReportsContainersRemovedWhenTheEngineStopsAnswering(t *testing.T) {
	shortEngineLimits(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	docker := &Docker{Binary: binary, Run: func(ctx context.Context, args ...string) (string, error) {
		switch args[0] {
		case "info":
			return "linux", nil
		case "rm":
			removed = true
			return "", nil
		}
		<-ctx.Done()
		return "", ctx.Err()
	}}
	err = returnsWithin(t, 5*time.Second, func() error {
		_, stopErr := Stop(context.Background(), docker, recordedStore(t), DefaultID)
		return stopErr
	})
	if !errors.Is(err, errEngineUnresponsive) || removed {
		t.Fatalf("an unanswered inspect was taken for an absent container: err=%v removed=%v", err, removed)
	}
}

func TestStopEndsWhenThePersonInterruptsIt(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	docker := &Docker{Binary: binary, Run: func(ctx context.Context, _ ...string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	ctx, interrupt := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, interrupt)
	err = returnsWithin(t, 5*time.Second, func() error {
		_, stopErr := Stop(ctx, docker, recordedStore(t), DefaultID)
		return stopErr
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "interrotta") {
		t.Fatalf("Stop after an interrupt: %v", err)
	}
}

// TestHelperSilentEngine is not a test: it is the engine process started by
// TestRealRunnerKillsASilentEngine, which never answers.
func TestHelperSilentEngine(t *testing.T) {
	if os.Getenv("ARCHETIPO_TEST_SILENT_ENGINE") != "1" {
		t.Skip("helper process")
	}
	time.Sleep(time.Hour)
}

// The real runner must kill the engine process on every platform: the test
// binary itself plays the silent engine, so no Docker and no shell is needed.
func TestRealRunnerKillsASilentEngine(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARCHETIPO_TEST_SILENT_ENGINE", "1")
	docker := NewDocker()
	docker.Binary = binary
	err = returnsWithin(t, 30*time.Second, func() error {
		_, runErr := docker.run(context.Background(), 500*time.Millisecond, "-test.run=^TestHelperSilentEngine$")
		return runErr
	})
	if !errors.Is(err, errEngineUnresponsive) {
		t.Fatalf("silent engine process: %v", err)
	}
}

func hasPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
