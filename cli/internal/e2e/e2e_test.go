package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records invocations instead of executing them.
type fakeRunner struct {
	calls [][]string
	err   error
	onRun func() // optional side effect, e.g. simulate artifact creation
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.onRun != nil {
		f.onRun()
	}
	return "", f.err
}

func (f *fakeRunner) called(substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), substr) {
			return true
		}
	}
	return false
}

func writePackageJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsure_NoPackageJSON_Precondition(t *testing.T) {
	dir := t.TempDir()
	_, err := Ensure(context.Background(), EnsureOptions{ProjectRoot: dir, Runner: &fakeRunner{}})
	if err == nil {
		t.Fatal("expected precondition error, got nil")
	}
	if !strings.Contains(err.Error(), "E_PRECONDITION") {
		t.Fatalf("expected E_PRECONDITION, got %v", err)
	}
}

func TestEnsure_FreshProject_InstallsConfiguresAndInstallsBrowser(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name":"x","version":"1.0.0"}`)
	fr := &fakeRunner{}

	res, err := Ensure(context.Background(), EnsureOptions{ProjectRoot: dir, Runner: fr})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "installed" {
		t.Fatalf("action = %q, want installed", res.Action)
	}
	if !res.Installed || res.Browser != DefaultBrowser {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !fr.called("npm install --save-dev @playwright/test") {
		t.Fatalf("expected npm install call, calls: %v", fr.calls)
	}
	if !fr.called("npx playwright install " + DefaultBrowser) {
		t.Fatalf("expected single-browser install, calls: %v", fr.calls)
	}
	if fr.called("firefox") || fr.called("webkit") || fr.called("--with-deps") {
		t.Fatalf("must not install extra browsers or deps by default, calls: %v", fr.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "playwright.config.ts")); err != nil {
		t.Fatalf("expected config to be written: %v", err)
	}
}

func TestEnsure_Idempotent_SkipsInstallAndKeepsConfig(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"^1.0.0"}}`)
	existing := "// my config\n"
	if err := os.WriteFile(filepath.Join(dir, "playwright.config.ts"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}

	res, err := Ensure(context.Background(), EnsureOptions{ProjectRoot: dir, Runner: fr})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "already-present" {
		t.Fatalf("action = %q, want already-present", res.Action)
	}
	if fr.called("npm install") {
		t.Fatalf("must not reinstall when already present, calls: %v", fr.calls)
	}
	// Existing config must be preserved untouched.
	got, _ := os.ReadFile(filepath.Join(dir, "playwright.config.ts"))
	if string(got) != existing {
		t.Fatalf("existing config was overwritten: %q", got)
	}
	// Browser install is still ensured (idempotent on Playwright's side).
	if !fr.called("npx playwright install") {
		t.Fatalf("expected browser ensure, calls: %v", fr.calls)
	}
}

func TestEnsure_WithDeps_PassesFlag(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name":"x"}`)
	fr := &fakeRunner{}
	if _, err := Ensure(context.Background(), EnsureOptions{ProjectRoot: dir, WithDeps: true, Runner: fr}); err != nil {
		t.Fatal(err)
	}
	if !fr.called("--with-deps") {
		t.Fatalf("expected --with-deps, calls: %v", fr.calls)
	}
}

func TestRunFunctional_NotInstalled_Precondition(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name":"x"}`)
	_, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: dir, Runner: &fakeRunner{}})
	if err == nil || !strings.Contains(err.Error(), "E_PRECONDITION") {
		t.Fatalf("expected precondition, got %v", err)
	}
}

func TestRunFunctional_PassAndFail(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)

	res, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: dir, Grep: "demo", Runner: &fakeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("expected passed, got %+v", res)
	}

	failing := &fakeRunner{err: context.DeadlineExceeded}
	res, err = RunFunctional(context.Background(), RunOptions{ProjectRoot: dir, Runner: failing})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatalf("expected failed result, got %+v", res)
	}
	if !failing.called("npx playwright test --reporter=list") {
		t.Fatalf("unexpected invocation: %v", failing.calls)
	}
}

func TestRecordDemo_NotInstalled_Precondition(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name":"x"}`)
	_, err := RecordDemo(context.Background(), DemoOptions{ProjectRoot: dir, Spec: "US-1", Runner: &fakeRunner{}})
	if err == nil || !strings.Contains(err.Error(), "E_PRECONDITION") {
		t.Fatalf("expected precondition, got %v", err)
	}
}

func TestRecordDemo_MissingSpec_Invalid(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	_, err := RecordDemo(context.Background(), DemoOptions{ProjectRoot: dir, Runner: &fakeRunner{}})
	if err == nil || !strings.Contains(err.Error(), "E_INVALID_INPUT") {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestRecordDemo_RunsRecordsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	// Existing project config => the ephemeral config must import it.
	if err := os.WriteFile(filepath.Join(dir, "playwright.config.ts"), []byte("// base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}

	res, err := RecordDemo(context.Background(), DemoOptions{
		ProjectRoot:    dir,
		Spec:           "US-001",
		Grep:           "demo",
		TestResultsDir: "out",
		Runner:         fr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || res.OutputDir != filepath.Join("out", "US-001") {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !fr.called("npx playwright test --config "+demoConfigName) || !fr.called("--grep demo") {
		t.Fatalf("unexpected invocation: %v", fr.calls)
	}
	// Output dir created, ephemeral config removed after the run.
	if _, err := os.Stat(filepath.Join(dir, "out", "US-001")); err != nil {
		t.Fatalf("output dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, demoConfigName)); !os.IsNotExist(err) {
		t.Fatalf("ephemeral config should be removed, err=%v", err)
	}
}

func TestRecordDemo_FindsVideo(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	// Simulate Playwright writing a video into the output dir during the run.
	producer := &fakeRunner{}
	produce := func() {
		vidDir := filepath.Join(dir, "docs/test-results", "US-009", "trace")
		_ = os.MkdirAll(vidDir, 0o755)
		_ = os.WriteFile(filepath.Join(vidDir, "video.webm"), []byte("x"), 0o644)
	}
	producer.onRun = produce

	res, err := RecordDemo(context.Background(), DemoOptions{ProjectRoot: dir, Spec: "US-009", Runner: producer})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("docs/test-results", "US-009", "trace", "video.webm")
	if res.VideoPath != want {
		t.Fatalf("video path = %q, want %q", res.VideoPath, want)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	if det, err := Detect(dir); err != nil || det.Framework != "" {
		t.Fatalf("empty dir: det=%+v err=%v", det, err)
	}
	writePackageJSON(t, dir, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	det, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if det.Framework != FrameworkPlaywright || !det.Installed {
		t.Fatalf("expected playwright installed, got %+v", det)
	}
}
