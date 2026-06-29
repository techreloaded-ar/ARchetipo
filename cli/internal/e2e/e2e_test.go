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
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
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
