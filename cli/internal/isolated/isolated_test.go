package isolated

import (
	"context"
	"errors"
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
func (f *fakeProvider) EnsureImage(_ context.Context, image string) error {
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

func hasPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
