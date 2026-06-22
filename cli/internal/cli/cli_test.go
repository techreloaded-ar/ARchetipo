package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/cli"
	cconfig "github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Each test uses t.Chdir(t.TempDir()) so the file connector picks up an empty
// project root with default paths. Tests are sequential because t.Chdir is
// process-wide; parallelism would race on the cwd.

type result struct {
	exit   int
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func runCLI(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	r := result{}
	in := io.Reader(strings.NewReader(stdin))
	r.exit = cli.Execute(args, in, &r.stdout, &r.stderr)
	return r
}

func newProject(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(out))
	}
}

func mustOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeWorktreeConfig(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		t.Fatal(err)
	}
	body := `connector: file
worktree:
  enabled: true
  base: main
  dir: .archetipo/worktrees
  branch_prefix: archetipo/
`
	if err := os.WriteFile(filepath.Join(".archetipo", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initGitMain(t *testing.T) {
	t.Helper()
	mustRun(t, "git", "init", "-b", "main")
	mustRun(t, "git", "config", "user.email", "archetipo-test@example.com")
	mustRun(t, "git", "config", "user.name", "ARchetipo Test")
	mustRun(t, "git", "commit", "--allow-empty", "-m", "base")
}

func seedStartedWorktreeSpec(t *testing.T) {
	t.Helper()
	writeWorktreeConfig(t)
	initGitMain(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
}

func writeInputFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(".", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func expectedPlanPath(code string) string {
	return filepath.Join(".", ".archetipo", "plans", code+"-plan.yaml")
}

func decodeOK(t *testing.T, res result) (string, map[string]any) {
	t.Helper()
	if res.exit != 0 {
		t.Fatalf("expected exit 0, got %d. stderr=%s", res.exit, res.stderr.String())
	}
	var env struct {
		Schema string         `json:"schema"`
		Kind   string         `json:"kind"`
		Data   map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.stdout.Bytes(), &env); err != nil {
		t.Fatalf("decoding stdout: %v\nraw=%s", err, res.stdout.String())
	}
	return env.Kind, env.Data
}

func decodeError(t *testing.T, res result) (int, string) {
	t.Helper()
	if res.exit == 0 {
		t.Fatalf("expected non-zero exit, got 0. stdout=%s", res.stdout.String())
	}
	var env struct {
		Schema string `json:"schema"`
		Kind   string `json:"kind"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.stderr.Bytes(), &env); err != nil {
		t.Fatalf("decoding stderr: %v\nraw=%s", err, res.stderr.String())
	}
	return res.exit, env.Error.Code
}

const specJSON = `{"specs":[
	{"code":"US-001","title":"First","priority":"HIGH","points":3,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}},
	{"code":"US-002","title":"Second","priority":"MEDIUM","points":2,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}}
]}`

const planJSON = `{"plan_body":"## Plan\nDo the work","tasks":[
	{"id":"TASK-01","title":"Implement","type":"Impl","status":"TODO"},
	{"id":"TASK-02","title":"Test","type":"Test","status":"TODO"}
]}`

func TestConfigShow(t *testing.T) {
	newProject(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, "", "config", "show")
	kind, data := decodeOK(t, res)
	if kind != "setup" {
		t.Fatalf("expected kind=setup, got %s", kind)
	}
	if data["project_root"] != root {
		t.Fatalf("expected project_root=%s, got %v", root, data["project_root"])
	}
}

func TestSpecAdd_EmptyBacklog(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	res := runCLI(t, "", "spec", "add", "--file", specsFile)
	kind, data := decodeOK(t, res)
	if kind != "write_result" {
		t.Fatalf("expected kind=write_result, got %s", kind)
	}
	if ok, _ := data["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", data["ok"])
	}
	if _, present := data["skipped"]; present {
		t.Fatalf("expected no skipped on initial save, got %v", data["skipped"])
	}
}

func TestSpecAdd_Idempotent(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	first := runCLI(t, "", "spec", "add", "--file", specsFile)
	if first.exit != 0 {
		t.Fatalf("first add failed: %s", first.stderr.String())
	}
	second := runCLI(t, "", "spec", "add", "--file", specsFile)
	_, data := decodeOK(t, second)
	skipped, _ := data["skipped"].([]any)
	if len(skipped) != 2 {
		t.Fatalf("expected 2 skipped codes, got %v", skipped)
	}
	if refs, ok := data["refs"].([]any); ok && len(refs) != 0 {
		t.Fatalf("expected no refs on full skip, got %v", refs)
	}
}

func TestSpecAdd_MixedSkipAndAppend(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	mixed := `{"specs":[
		{"code":"US-001","title":"dup","priority":"HIGH","points":1,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}},
		{"code":"US-003","title":"new","priority":"LOW","points":1,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}}
	]}`
	mixedFile := writeInputFile(t, "mixed.json", mixed)
	res := runCLI(t, "", "spec", "add", "--file", mixedFile)
	_, data := decodeOK(t, res)
	skipped, _ := data["skipped"].([]any)
	if len(skipped) != 1 || skipped[0] != "US-001" {
		t.Fatalf("expected skipped=[US-001], got %v", skipped)
	}
	refs, _ := data["refs"].([]any)
	if len(refs) == 0 {
		t.Fatalf("expected refs for US-003, got empty")
	}
}

func TestSpecList(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "list")
	kind, data := decodeOK(t, res)
	if kind != "backlog" {
		t.Fatalf("expected kind=backlog, got %s", kind)
	}
	items, _ := data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	summary, _ := data["summary"].(map[string]any)
	codes, _ := summary["codes"].([]any)
	if len(codes) != 2 {
		t.Fatalf("expected 2 codes in summary, got %d", len(codes))
	}
}

func TestSpecShow_ByCode(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "show", "US-001")
	kind, data := decodeOK(t, res)
	if kind != "spec" {
		t.Fatalf("expected kind=spec, got %s", kind)
	}
	spec, _ := data["spec"].(map[string]any)
	if spec["code"] != "US-001" {
		t.Fatalf("expected US-001, got %v", spec["code"])
	}
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks before plan, got %d", len(tasks))
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if data["workdir"] != root {
		t.Fatalf("expected workdir=%s, got %v", root, data["workdir"])
	}
}

func TestSpecShow_MissingCodeRejected(t *testing.T) {
	newProject(t)
	res := runCLI(t, "", "spec", "show")
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT, got %s", code)
	}
}

func TestSpecNext_AutoPickByStatus(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "next", "--status", "TODO")
	kind, data := decodeOK(t, res)
	if kind != "spec" {
		t.Fatalf("expected kind=spec, got %s", kind)
	}
	spec, _ := data["spec"].(map[string]any)
	// Auto-pick: priority HIGH first → US-001 (HIGH) before US-002 (MEDIUM).
	if spec["code"] != "US-001" {
		t.Fatalf("expected auto-pick US-001 (HIGH), got %v", spec["code"])
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if data["workdir"] != root {
		t.Fatalf("expected workdir=%s, got %v", root, data["workdir"])
	}
}

func TestSpecShow_WorkdirBeforeAndAfterWorktreeStart(t *testing.T) {
	newProject(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeWorktreeConfig(t)
	initGitMain(t)

	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}

	before := runCLI(t, "", "spec", "show", "US-001")
	_, beforeData := decodeOK(t, before)
	if beforeData["workdir"] != root {
		t.Fatalf("expected workdir before start=%s, got %v", root, beforeData["workdir"])
	}

	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	after := runCLI(t, "", "spec", "show", "US-001")
	_, afterData := decodeOK(t, after)
	wantWorkdir := filepath.Join(root, ".archetipo", "worktrees", "US-001")
	if afterData["workdir"] != wantWorkdir {
		t.Fatalf("expected workdir after start=%s, got %v", wantWorkdir, afterData["workdir"])
	}
	spec, _ := afterData["spec"].(map[string]any)
	if spec["worktree"] != filepath.Join(".archetipo", "worktrees", "US-001") {
		t.Fatalf("expected persisted spec.worktree, got %v", spec["worktree"])
	}
}

func TestSpecNext_MissingStatusRejected(t *testing.T) {
	newProject(t)
	res := runCLI(t, "", "spec", "next")
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT, got %s", code)
	}
}

func TestSpecPlan_TODOToPlanned(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile)
	if res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	// Verify status moved by reading it back.
	show := runCLI(t, "", "spec", "show", "US-001")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "PLANNED" {
		t.Fatalf("expected status PLANNED, got %v", spec["status"])
	}
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks after plan, got %d", len(tasks))
	}
	if _, err := os.Stat(expectedPlanPath("US-001")); err != nil {
		t.Fatalf("expected plan file at %s: %v", expectedPlanPath("US-001"), err)
	}
}

func TestSpecPlan_IdempotentOnPlanned(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("first plan failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile)
	if res.exit != 0 {
		t.Fatalf("re-plan should be idempotent, got exit %d, stderr=%s", res.exit, res.stderr.String())
	}
}

func TestSpecPlan_FromStdin(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, planJSON, "spec", "plan", "US-001", "--file", "-")
	_, data := decodeOK(t, res)
	refs, _ := data["refs"].([]any)
	if len(refs) == 0 {
		t.Fatalf("expected refs after stdin plan save")
	}
	firstRef, _ := refs[0].(map[string]any)
	gotPath, _ := firstRef["path"].(string)
	wantPath, err := filepath.Abs(expectedPlanPath("US-001"))
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != wantPath {
		t.Fatalf("expected first ref path %s, got %s", wantPath, gotPath)
	}
}

func TestSpecStart_ConflictFromTodo(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "start", "US-001")
	_, code := decodeError(t, res)
	if code != iox.CodeConflict {
		t.Fatalf("expected E_CONFLICT, got %s", code)
	}
}

func TestSpecStart_HappyPath(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "start", "US-001")
	if res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
	// Re-running is idempotent.
	again := runCLI(t, "", "spec", "start", "US-001")
	if again.exit != 0 {
		t.Fatalf("idempotent start failed: %s", again.stderr.String())
	}
}

func TestSpecReview_HappyPathWithComment(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
	res := runCLI(t, "Closing notes for the spec", "spec", "review", "US-001")
	if res.exit != 0 {
		t.Fatalf("review failed: %s", res.stderr.String())
	}
	show := runCLI(t, "", "spec", "show", "US-001")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "REVIEW" {
		t.Fatalf("expected REVIEW, got %v", spec["status"])
	}
}

func TestSpecReview_CommentFromFile(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
	commentFile := writeInputFile(t, "comment.md", "## Done\nShipping it.")
	res := runCLI(t, "", "spec", "review", "US-001", "--file", commentFile)
	if res.exit != 0 {
		t.Fatalf("review failed: %s", res.stderr.String())
	}
}

func TestSpecReview_CommitsDirtyWorktreeBeforeReview(t *testing.T) {
	newProject(t)
	writeWorktreeConfig(t)
	initGitMain(t)
	if err := os.WriteFile("hello.txt", []byte("Hello from ARchetipo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "git", "add", "hello.txt")
	mustRun(t, "git", "commit", "-m", "seed hello")

	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}

	worktree := filepath.Join(".archetipo", "worktrees", "US-001")
	if err := os.WriteFile(filepath.Join(worktree, "hello.txt"), []byte("Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "archetipo.txt"), []byte("from ARchetipo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, "Closing notes", "spec", "review", "US-001")
	if res.exit != 0 {
		t.Fatalf("review failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	show := runCLI(t, "", "spec", "show", "US-001")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "REVIEW" {
		t.Fatalf("expected REVIEW, got %v", spec["status"])
	}
	if status := mustOutput(t, "git", "-C", worktree, "status", "--porcelain"); status != "" {
		t.Fatalf("expected clean worktree after review commit, got %q", status)
	}
	diff := mustOutput(t, "git", "diff", "--name-status", "main...archetipo/US-001")
	if !strings.Contains(diff, "M\thello.txt") {
		t.Fatalf("expected review diff to include modified hello.txt, got:\n%s", diff)
	}
	if !strings.Contains(diff, "A\tarchetipo.txt") {
		t.Fatalf("expected review diff to include new archetipo.txt, got:\n%s", diff)
	}

	// Verify default commit subject (no --commit-type / --commit-summary flags).
	subject := mustOutput(t, "git", "-C", worktree, "log", "-1", "--pretty=%s")
	want := "chore(US-001): First"
	if subject != want {
		t.Fatalf("expected commit subject %q, got %q", want, subject)
	}
}

func TestSpecReview_CommitSubjectFeat(t *testing.T) {
	newProject(t)
	writeWorktreeConfig(t)
	initGitMain(t)

	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}

	worktree := filepath.Join(".archetipo", "worktrees", "US-001")
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, "", "spec", "review", "US-001", "--commit-type", "feat", "--commit-summary", "add invite flow")
	if res.exit != 0 {
		t.Fatalf("review failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	subject := mustOutput(t, "git", "-C", worktree, "log", "-1", "--pretty=%s")
	want := "feat(US-001): add invite flow"
	if subject != want {
		t.Fatalf("expected commit subject %q, got %q", want, subject)
	}
}

func TestSpecReview_CommitSubjectFixWithDifferentCode(t *testing.T) {
	newProject(t)
	writeWorktreeConfig(t)
	initGitMain(t)

	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-002", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-002"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}

	worktree := filepath.Join(".archetipo", "worktrees", "US-002")
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, "", "spec", "review", "US-002", "--commit-type", "fix", "--commit-summary", "handle expired token")
	if res.exit != 0 {
		t.Fatalf("review failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	subject := mustOutput(t, "git", "-C", worktree, "log", "-1", "--pretty=%s")
	want := "fix(US-002): handle expired token"
	if subject != want {
		t.Fatalf("expected commit subject %q, got %q", want, subject)
	}
}

func TestSpecReview_CommitSubjectCiWithDifferentCode(t *testing.T) {
	newProject(t)
	writeWorktreeConfig(t)
	initGitMain(t)

	ciJSON := `{"specs":[
		{"code":"US-125","title":"CI Setup","priority":"MEDIUM","points":2,"status":"TODO"}
	]}`
	specsFile := writeInputFile(t, "specs.json", ciJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-125", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-125"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}

	worktree := filepath.Join(".archetipo", "worktrees", "US-125")
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, "", "spec", "review", "US-125", "--commit-type", "ci", "--commit-summary", "add release workflow")
	if res.exit != 0 {
		t.Fatalf("review failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	subject := mustOutput(t, "git", "-C", worktree, "log", "-1", "--pretty=%s")
	want := "ci(US-125): add release workflow"
	if subject != want {
		t.Fatalf("expected commit subject %q, got %q", want, subject)
	}
}

func TestSpecReview_InvalidCommitType(t *testing.T) {
	newProject(t)
	seedStartedWorktreeSpec(t)

	res := runCLI(t, "", "spec", "review", "US-001", "--commit-type", "bogus")
	if res.exit == 0 {
		t.Fatal("expected non-zero exit for invalid --commit-type")
	}
	stderr := res.stderr.String()
	if !strings.Contains(stderr, "E_INVALID_INPUT") {
		t.Fatalf("expected E_INVALID_INPUT error, got: %s", stderr)
	}
}

func TestSpecReview_CleanWorktreeDoesNotCreateCommit(t *testing.T) {
	newProject(t)
	seedStartedWorktreeSpec(t)
	before := mustOutput(t, "git", "rev-parse", "archetipo/US-001")

	res := runCLI(t, "", "spec", "review", "US-001")
	if res.exit != 0 {
		t.Fatalf("review failed: %s", res.stderr.String())
	}
	after := mustOutput(t, "git", "rev-parse", "archetipo/US-001")
	if after != before {
		t.Fatalf("expected no new commit for clean worktree, before=%s after=%s", before, after)
	}
	show := runCLI(t, "", "spec", "show", "US-001")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "REVIEW" {
		t.Fatalf("expected REVIEW, got %v", spec["status"])
	}
}

func seedReviewedSpec(t *testing.T) {
	t.Helper()
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "review", "US-001"); res.exit != 0 {
		t.Fatalf("review failed: %s", res.stderr.String())
	}
}

func TestSpecRequestChanges_HappyPath(t *testing.T) {
	newProject(t)
	seedReviewedSpec(t)
	feedback := `{"comments":[
		{"file":"src/app.js","line":12,"body":"handle the empty list case"},
		{"body":"general note without anchor"}
	]}`
	feedbackFile := writeInputFile(t, "feedback.json", feedback)
	res := runCLI(t, "", "spec", "request-changes", "US-001", "--file", feedbackFile)
	if res.exit != 0 {
		t.Fatalf("request-changes failed: %s", res.stderr.String())
	}
	show := runCLI(t, "", "spec", "show", "US-001")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "TODO" {
		t.Fatalf("expected TODO after request-changes, got %v", spec["status"])
	}
	if rework, _ := spec["rework"].(bool); !rework {
		t.Fatalf("expected rework=true, got %v", spec["rework"])
	}
	body, _ := spec["body"].(string)
	if !strings.Contains(body, "## Rework Feedback") {
		t.Fatalf("expected Rework Feedback section in body, got:\n%s", body)
	}
	if !strings.Contains(body, "**src/app.js:12** — handle the empty list case") {
		t.Fatalf("expected anchored bullet in body, got:\n%s", body)
	}
	if !strings.Contains(body, "- general note without anchor") {
		t.Fatalf("expected unanchored bullet in body, got:\n%s", body)
	}
}

func TestSpecRequestChanges_ConflictWhenNotInReview(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	feedbackFile := writeInputFile(t, "feedback.json", `{"comments":[{"body":"nope"}]}`)
	res := runCLI(t, "", "spec", "request-changes", "US-001", "--file", feedbackFile)
	_, code := decodeError(t, res)
	if code != iox.CodeConflict {
		t.Fatalf("expected E_CONFLICT, got %s", code)
	}
}

func TestSpecRequestChanges_EmptyPayloadRejected(t *testing.T) {
	newProject(t)
	seedReviewedSpec(t)
	feedbackFile := writeInputFile(t, "feedback.json", `{"comments":[{"body":"   "}]}`)
	res := runCLI(t, "", "spec", "request-changes", "US-001", "--file", feedbackFile)
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT, got %s", code)
	}
}

func TestTaskDone_Positional(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "task", "done", "US-001", "TASK-01")
	if res.exit != 0 {
		t.Fatalf("task done failed: %s", res.stderr.String())
	}
}

func TestSpecMove_ChangesStatusAndOrder(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "move", "US-002", "--to", "review"); res.exit != 0 {
		t.Fatalf("spec move failed: %s", res.stderr.String())
	}
	show := runCLI(t, "", "spec", "show", "US-002")
	_, data := decodeOK(t, show)
	spec, _ := data["spec"].(map[string]any)
	if spec["status"] != "REVIEW" {
		t.Fatalf("expected REVIEW after spec move, got %v", spec["status"])
	}
}

func TestSpecMove_InvalidToReturnsInvalidInput(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "spec", "move", "US-001", "--to", "BOGUS")
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT, got %s", code)
	}
}

func TestPRDWrite_FromStdin(t *testing.T) {
	newProject(t)
	res := runCLI(t, "# Product Vision\n\nMVP for early adopters.", "prd", "write")
	kind, data := decodeOK(t, res)
	if kind != "write_result" {
		t.Fatalf("expected kind=write_result, got %s", kind)
	}
	if ok, _ := data["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", data["ok"])
	}
}

func TestPRDWrite_FromFileFlag(t *testing.T) {
	newProject(t)
	prdFile := writeInputFile(t, "PRD.md", "# Product Vision\n\nFrom file.")
	res := runCLI(t, "", "prd", "write", "--file", prdFile)
	kind, data := decodeOK(t, res)
	if kind != "write_result" {
		t.Fatalf("expected kind=write_result, got %s", kind)
	}
	if ok, _ := data["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", data["ok"])
	}
}

func TestPRDWrite_EmptyStdin(t *testing.T) {
	newProject(t)
	// Seed an existing PRD so we can verify it is not overwritten.
	seed := "# Original PRD\n\nThis content must survive."
	seedRes := runCLI(t, seed, "prd", "write")
	_, _ = decodeOK(t, seedRes)

	// Attempt empty stdin write.
	res := runCLI(t, "", "prd", "write")
	exit, code := decodeError(t, res)
	if exit != iox.ExitInvalidInput {
		t.Fatalf("expected exit %d, got %d", iox.ExitInvalidInput, exit)
	}
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected code %s, got %s", iox.CodeInvalidInput, code)
	}

	// Verify existing PRD is untouched.
	got, err := os.ReadFile("docs/PRD.md")
	if err != nil {
		t.Fatalf("reading PRD after empty write: %v", err)
	}
	if string(got) != seed {
		t.Fatalf("PRD was overwritten!\noriginal: %q\ngot:      %q", seed, string(got))
	}
}

func TestPRDWrite_EmptyFile(t *testing.T) {
	newProject(t)
	// Seed an existing PRD so we can verify it is not overwritten.
	seed := "# Original PRD\n\nThis content must survive."
	seedRes := runCLI(t, seed, "prd", "write")
	_, _ = decodeOK(t, seedRes)

	// Write an empty input file and pass it via --file.
	emptyFile := writeInputFile(t, "empty.md", "")
	res := runCLI(t, "", "prd", "write", "--file", emptyFile)
	exit, code := decodeError(t, res)
	if exit != iox.ExitInvalidInput {
		t.Fatalf("expected exit %d, got %d", iox.ExitInvalidInput, exit)
	}
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected code %s, got %s", iox.CodeInvalidInput, code)
	}

	// Verify existing PRD is untouched.
	got, err := os.ReadFile("docs/PRD.md")
	if err != nil {
		t.Fatalf("reading PRD after empty write: %v", err)
	}
	if string(got) != seed {
		t.Fatalf("PRD was overwritten!\noriginal: %q\ngot:      %q", seed, string(got))
	}
}

func TestPRDWrite_WhitespaceOnlyStdin(t *testing.T) {
	newProject(t)
	// Seed an existing PRD.
	seed := "# Original PRD\n\nThis content must survive."
	seedRes := runCLI(t, seed, "prd", "write")
	_, _ = decodeOK(t, seedRes)

	// Attempt whitespace-only stdin write.
	res := runCLI(t, "   \n\t\n  ", "prd", "write")
	exit, code := decodeError(t, res)
	if exit != iox.ExitInvalidInput {
		t.Fatalf("expected exit %d, got %d", iox.ExitInvalidInput, exit)
	}
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected code %s, got %s", iox.CodeInvalidInput, code)
	}

	// Verify existing PRD is untouched.
	got, err := os.ReadFile("docs/PRD.md")
	if err != nil {
		t.Fatalf("reading PRD after whitespace write: %v", err)
	}
	if string(got) != seed {
		t.Fatalf("PRD was overwritten!\noriginal: %q\ngot:      %q", seed, string(got))
	}
}

func TestMetrics_AggregatesBacklogAndFlow(t *testing.T) {
	newProject(t)
	specs := `{"specs":[
		{"code":"US-001","title":"First","priority":"HIGH","points":3,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}},
		{"code":"US-002","title":"Second","priority":"MEDIUM","points":2,"status":"TODO","epic":{"code":"EP-001","title":"Epic"},"blocked_by":["US-001"]}
	]}`
	specsFile := writeInputFile(t, "specs.json", specs)
	planFile := writeInputFile(t, "plan.json", planJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "review", "US-001"); res.exit != 0 {
		t.Fatalf("review failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "move", "US-001", "--to", "done"); res.exit != 0 {
		t.Fatalf("move to done failed: %s", res.stderr.String())
	}

	res := runCLI(t, "", "metrics")
	kind, data := decodeOK(t, res)
	if kind != "metrics" {
		t.Fatalf("expected kind=metrics, got %s", kind)
	}
	totals, _ := data["totals"].(map[string]any)
	if totals["specs"] != float64(2) || totals["points"] != float64(5) {
		t.Fatalf("unexpected totals: %v", totals)
	}
	if totals["done_specs"] != float64(1) || totals["done_points"] != float64(3) {
		t.Fatalf("unexpected done totals: %v", totals)
	}
	if totals["completion_pct"] != float64(60) {
		t.Fatalf("expected completion_pct=60 (3/5 points), got %v", totals["completion_pct"])
	}
	epics, _ := data["by_epic"].([]any)
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic bucket, got %v", data["by_epic"])
	}
	// US-002 is blocked by US-001, which is now DONE: no blocked specs.
	if blocked, present := data["blocked"]; present {
		t.Fatalf("expected no blocked specs (blocker is done), got %v", blocked)
	}
	flow, _ := data["flow"].(map[string]any)
	if flow == nil || flow["measured_specs"] != float64(1) {
		t.Fatalf("expected flow metrics for 1 done spec, got %v", data["flow"])
	}
}

func TestMetrics_ReportsUnmetBlockers(t *testing.T) {
	newProject(t)
	specs := `{"specs":[
		{"code":"US-001","title":"First","priority":"HIGH","points":3,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}},
		{"code":"US-002","title":"Second","priority":"MEDIUM","points":2,"status":"TODO","epic":{"code":"EP-001","title":"Epic"},"blocked_by":["US-001"]}
	]}`
	specsFile := writeInputFile(t, "specs.json", specs)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	res := runCLI(t, "", "metrics")
	_, data := decodeOK(t, res)
	blocked, _ := data["blocked"].([]any)
	if len(blocked) != 1 {
		t.Fatalf("expected 1 blocked spec, got %v", data["blocked"])
	}
	entry, _ := blocked[0].(map[string]any)
	if entry["code"] != "US-002" {
		t.Fatalf("expected US-002 blocked, got %v", entry)
	}
	if _, present := data["flow"]; present {
		t.Fatalf("expected no flow metrics without done specs, got %v", data["flow"])
	}
}

func TestDoctor_FailsWithoutInstalledSkills(t *testing.T) {
	newProject(t)
	// Empty project: no tool directory has skills, so doctor must fail with
	// E_PRECONDITION and still print the per-check report on stdout.
	res := runCLI(t, "", "doctor")
	_, code := decodeError(t, res)
	if code != iox.CodePreconditionMissing {
		t.Fatalf("expected E_PRECONDITION, got %s", code)
	}
	out := res.stdout.String()
	if !strings.Contains(out, "installed skills") {
		t.Fatalf("expected installed-skills check in report, got:\n%s", out)
	}
	if !strings.Contains(out, "archetipo init") {
		t.Fatalf("expected init hint in report, got:\n%s", out)
	}
}

func TestDoctor_PassesAfterInit(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))
	if res := runCLI(t, "", "init", "--tool", "claude", "--connector", "file", "--yes"); res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	res := runCLI(t, "", "doctor")
	if res.exit != 0 {
		t.Fatalf("doctor failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}
	out := res.stdout.String()
	if !strings.Contains(out, "All checks passed.") {
		t.Fatalf("expected all checks to pass, got:\n%s", out)
	}
	if !strings.Contains(out, "skipped (connector is not github)") {
		t.Fatalf("expected gh check to be skipped for file connector, got:\n%s", out)
	}
}

// repoDataDir resolves the repository root (which holds skills/ + .archetipo/)
// so init/doctor tests can run against the real packaged assets.
func repoDataDir(t *testing.T) string {
	t.Helper()
	// This test file lives at cli/internal/cli/; the repo root is three up.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func TestSpecUpdate_TitleChange(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	patchFile := writeInputFile(t, "patch.yaml", "title: Updated First")
	res := runCLI(t, "", "spec", "update", "US-001", "--file", patchFile)
	kind, data := decodeOK(t, res)
	if kind != "write_result" {
		t.Fatalf("expected kind=write_result, got %s", kind)
	}
	if ok, _ := data["ok"].(bool); !ok {
		t.Fatal("expected ok=true")
	}
	// Verify via spec show.
	show := runCLI(t, "", "spec", "show", "US-001")
	_, showData := decodeOK(t, show)
	spec, _ := showData["spec"].(map[string]any)
	if spec["title"] != "Updated First" {
		t.Fatalf("expected title 'Updated First', got %v", spec["title"])
	}
}

func TestSpecUpdate_MultipleFields(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	patchYAML := `title: Multi-update
priority: LOW
scope: MVP
blocked_by:
  - US-003
rework: true`
	patchFile := writeInputFile(t, "patch.yaml", patchYAML)
	res := runCLI(t, "", "spec", "update", "US-001", "--file", patchFile)
	if res.exit != 0 {
		t.Fatalf("update failed: %s", res.stderr.String())
	}
	show := runCLI(t, "", "spec", "show", "US-001")
	_, showData := decodeOK(t, show)
	spec, _ := showData["spec"].(map[string]any)
	if spec["title"] != "Multi-update" {
		t.Errorf("title: %v", spec["title"])
	}
	if spec["priority"] != "LOW" {
		t.Errorf("priority: %v", spec["priority"])
	}
	if spec["scope"] != "MVP" {
		t.Errorf("scope: %v", spec["scope"])
	}
	blocked, _ := spec["blocked_by"].([]any)
	if len(blocked) != 1 || blocked[0] != "US-003" {
		t.Errorf("blocked_by: %v", blocked)
	}
	if rework, _ := spec["rework"].(bool); !rework {
		t.Error("expected rework=true")
	}
}

func TestSpecUpdate_EmptyPatchRejected(t *testing.T) {
	newProject(t)
	specsFile := writeInputFile(t, "specs.json", specJSON)
	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	// YAML with no fields (empty document).
	patchFile := writeInputFile(t, "empty.yaml", "")
	res := runCLI(t, "", "spec", "update", "US-001", "--file", patchFile)
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT for empty patch, got %s", code)
	}
}

func TestSpecUpdate_MissingFileRejected(t *testing.T) {
	newProject(t)
	res := runCLI(t, "", "spec", "update", "US-001")
	_, code := decodeError(t, res)
	if code != iox.CodeInvalidInput {
		t.Fatalf("expected E_INVALID_INPUT for missing --file, got %s", code)
	}
}

// TestSpecStart_AfterIntegrateDependency_CreatesWorktree reproduces the bug
// where US-002 (blocked by US-001) gets no worktree after US-001 is integrated
// because stale branch metadata on the DONE blocker points to a deleted branch.
func TestSpecStart_AfterIntegrateDependency_CreatesWorktree(t *testing.T) {
	newProject(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeWorktreeConfig(t)
	initGitMain(t)

	// Two specs: US-001 (no blocker) and US-002 (blocked by US-001).
	specs := `{"specs":[
		{"code":"US-001","title":"First","priority":"HIGH","points":3,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}},
		{"code":"US-002","title":"Second","priority":"MEDIUM","points":2,"status":"TODO","epic":{"code":"EP-001","title":"Epic"},"blocked_by":["US-001"]}
	]}`
	specsFile := writeInputFile(t, "specs.json", specs)
	planFile := writeInputFile(t, "plan.json", planJSON)

	if res := runCLI(t, "", "spec", "add", "--file", specsFile); res.exit != 0 {
		t.Fatalf("seed add failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-001", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan US-001 failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "plan", "US-002", "--file", planFile); res.exit != 0 {
		t.Fatalf("plan US-002 failed: %s", res.stderr.String())
	}

	// Start US-001 and commit a change in its worktree.
	if res := runCLI(t, "", "spec", "start", "US-001"); res.exit != 0 {
		t.Fatalf("start US-001 failed: %s", res.stderr.String())
	}
	wt001 := filepath.Join(".archetipo", "worktrees", "US-001")
	if err := os.WriteFile(filepath.Join(wt001, "feature.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "git", "-C", wt001, "add", "feature.txt")
	mustRun(t, "git", "-C", wt001, "commit", "-m", "implement US-001")

	// Review and integrate US-001 (deletes branch and worktree).
	if res := runCLI(t, "", "spec", "review", "US-001"); res.exit != 0 {
		t.Fatalf("review US-001 failed: %s", res.stderr.String())
	}
	if res := runCLI(t, "", "spec", "integrate", "US-001"); res.exit != 0 {
		t.Fatalf("integrate US-001 failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	// Now start US-002. It must get its own worktree, not the project root.
	startRes := runCLI(t, "", "spec", "start", "US-002")
	if startRes.exit != 0 {
		t.Fatalf("start US-002 failed: stdout=%s stderr=%s", startRes.stdout.String(), startRes.stderr.String())
	}
	stderr := startRes.stderr.String()
	if strings.Contains(stderr, "worktree setup skipped") {
		t.Fatalf("unexpected worktree setup skipped warning: %s", stderr)
	}

	// Verify US-002 has its own worktree.
	show := runCLI(t, "", "spec", "show", "US-002")
	_, showData := decodeOK(t, show)
	wantWorkdir := filepath.Join(root, ".archetipo", "worktrees", "US-002")
	if showData["workdir"] != wantWorkdir {
		t.Fatalf("expected workdir=%s, got %v", wantWorkdir, showData["workdir"])
	}

	// Also verify US-001 metadata is clean after integrate.
	show001 := runCLI(t, "", "spec", "show", "US-001")
	_, showData001 := decodeOK(t, show001)
	spec001, _ := showData001["spec"].(map[string]any)
	if spec001["branch"] != nil && spec001["branch"] != "" {
		t.Fatalf("expected empty branch after integrate, got %v", spec001["branch"])
	}
	if spec001["worktree"] != nil && spec001["worktree"] != "" {
		t.Fatalf("expected empty worktree after integrate, got %v", spec001["worktree"])
	}
}

func TestVersionCommand(t *testing.T) {
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("exit=%d stderr=%s", res.exit, res.stderr.String())
	}
	if !strings.HasPrefix(res.stdout.String(), "archetipo ") {
		t.Fatalf("unexpected stdout: %q", res.stdout.String())
	}
}

func TestVersionFlagMatchesCommand(t *testing.T) {
	cmd := runCLI(t, "", "version")
	flag := runCLI(t, "", "--version")
	if cmd.stdout.String() != flag.stdout.String() {
		t.Fatalf("mismatch: cmd=%q flag=%q", cmd.stdout.String(), flag.stdout.String())
	}
}

// ---------------- Analytics instrumentation tests (US-004) ----------------

// mockAnalyticsSender captures the last Event sent.
type mockAnalyticsSender struct {
	event    *analytics.Event
	sendErr  error
	called   bool
	captured []analytics.Event
}

func (m *mockAnalyticsSender) Send(_ context.Context, e analytics.Event) error {
	m.called = true
	m.event = &e
	m.captured = append(m.captured, e)
	return m.sendErr
}

func writeAnalyticsConfig(t *testing.T, consent bool, endpoint string) {
	t.Helper()
	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := fmt.Sprintf("connector: file\nanalytics:\n  consent: %v\n  endpoint: %s\n", consent, endpoint)
	if err := os.WriteFile(filepath.Join(".archetipo", "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyticsCommandCompleted_NormalisedAndCorrect(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")

	// version always succeeds without backlog.
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("version failed: stderr=%s", res.stderr.String())
	}
	if !mock.called {
		t.Fatal("expected analytics sender to be called")
	}
	if mock.event == nil {
		t.Fatal("expected event to be captured")
	}
	e := mock.event
	if e.Command != "version" {
		t.Errorf("expected command=version, got %q", e.Command)
	}
	if e.Schema != analytics.DefaultSchema {
		t.Errorf("expected schema=%s, got %q", analytics.DefaultSchema, e.Schema)
	}
	if e.Event != analytics.EventCommandCompleted {
		t.Errorf("expected event=%s, got %q", analytics.EventCommandCompleted, e.Event)
	}
	if e.Success == nil || !*e.Success {
		t.Error("expected success=true")
	}
	if e.ExitCode != 0 {
		t.Errorf("expected exit_code=0, got %d", e.ExitCode)
	}
	if e.ErrorCode != "" {
		t.Errorf("expected empty error_code, got %q", e.ErrorCode)
	}
	if e.DurationMs < 0 {
		t.Errorf("expected duration_ms >= 0, got %d", e.DurationMs)
	}
	if e.Connector != "file" {
		t.Errorf("expected connector=file, got %q", e.Connector)
	}
}

func TestAnalyticsMultipleCommandsNormalised(t *testing.T) {
	tests := []struct {
		args    []string
		wantCmd string
	}{
		{[]string{"version"}, "version"},
		{[]string{"config", "show"}, "config.show"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			newProject(t)
			mock := &mockAnalyticsSender{}
			orig := cli.AnalyticsClientFactory
			cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
			t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

			writeAnalyticsConfig(t, true, "https://example.com/events")

			res := runCLI(t, "", tt.args...)
			if res.exit != 0 {
				t.Fatalf("command %v failed: stderr=%s", tt.args, res.stderr.String())
			}
			if !mock.called {
				t.Fatal("expected analytics sender to be called")
			}
			if mock.event.Command != tt.wantCmd {
				t.Errorf("expected command=%q, got %q", tt.wantCmd, mock.event.Command)
			}
		})
	}
}

func TestAnalyticsErrorCodeForTypedErrors(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")

	// spec show without a code returns E_INVALID_INPUT.
	res := runCLI(t, "", "spec", "show")
	if res.exit != iox.ExitInvalidInput {
		t.Fatalf("expected exit %d, got %d", iox.ExitInvalidInput, res.exit)
	}
	if !mock.called {
		t.Fatal("expected analytics sender to be called")
	}
	e := mock.event
	if e.Command != "spec.show" {
		t.Errorf("expected command=spec.show, got %q", e.Command)
	}
	if e.Success == nil || *e.Success {
		t.Error("expected success=false")
	}
	if e.ExitCode != iox.ExitInvalidInput {
		t.Errorf("expected exit_code=%d, got %d", iox.ExitInvalidInput, e.ExitCode)
	}
	if e.ErrorCode != iox.CodeInvalidInput {
		t.Errorf("expected error_code=%s, got %q", iox.CodeInvalidInput, e.ErrorCode)
	}
}

func TestAnalyticsConsentDisabledNoCall(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, false, "https://example.com/events")

	// version command always works.
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("version failed: stderr=%s", res.stderr.String())
	}
	if mock.called {
		t.Error("expected analytics sender NOT to be called when consent=false")
	}
}

func TestAnalyticsConfigAbsentNoCall(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	// No config file at all.
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("version failed: stderr=%s", res.stderr.String())
	}
	if mock.called {
		t.Error("expected analytics sender NOT to be called when config absent")
	}
}

func TestAnalyticsExitCodeUnchangedWithMockFailure(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{sendErr: io.ErrUnexpectedEOF}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")

	// version always succeeds.
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("expected exit 0 despite analytics failure, got %d. stderr=%s", res.exit, res.stderr.String())
	}
	// Must still have been called.
	if !mock.called {
		t.Error("analytics sender should have been called")
	}
}

func TestAnalyticsOutputUnchangedWithMockFailure(t *testing.T) {
	newProject(t)
	// Record output without analytics.
	resNoAnalytics := runCLI(t, "", "version")

	// Now with analytics mock that fails.
	mock := &mockAnalyticsSender{sendErr: io.ErrUnexpectedEOF}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")
	resWithAnalytics := runCLI(t, "", "version")

	// Exit codes must match.
	if resNoAnalytics.exit != resWithAnalytics.exit {
		t.Errorf("exit code changed: %d vs %d", resNoAnalytics.exit, resWithAnalytics.exit)
	}
	// Stdout must be identical.
	if resNoAnalytics.stdout.String() != resWithAnalytics.stdout.String() {
		t.Errorf("stdout differs with analytics failure:\nno-analytics: %s\nwith-analytics: %s",
			resNoAnalytics.stdout.String(), resWithAnalytics.stdout.String())
	}
}

func TestAnalyticsNotifierIndependence(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")

	// Run version command — notifier runs but analytics should also fire.
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Fatalf("version failed: stderr=%s", res.stderr.String())
	}
	// Analytics should have been called — notifier doesn't block it.
	if !mock.called {
		t.Error("analytics sender should be called independently of notifier")
	}
	// Verify the event is correct.
	if mock.event == nil {
		t.Fatal("expected captured event")
	}
	if mock.event.Command != "version" {
		t.Errorf("expected command=version, got %q", mock.event.Command)
	}
}

func TestAnalyticsEventHasNoRawArgs(t *testing.T) {
	newProject(t)
	mock := &mockAnalyticsSender{}
	orig := cli.AnalyticsClientFactory
	cli.AnalyticsClientFactory = func(_ cconfig.Config) cli.AnalyticsSender { return mock }
	t.Cleanup(func() { cli.AnalyticsClientFactory = orig })

	writeAnalyticsConfig(t, true, "https://example.com/events")

	// Run with arguments — the event command should be normalised, not raw args.
	res := runCLI(t, "", "spec", "show", "US-001")
	_ = res // may fail (no backlog) but analytics still fires

	if !mock.called {
		t.Fatal("expected analytics sender to be called")
	}
	// Command should be the normalised dotted form, not include the code arg.
	if mock.event.Command != "spec.show" {
		t.Errorf("expected command=spec.show, got %q", mock.event.Command)
	}
}

// --- TASK-05: analytics consent prompt during init ---

func TestInit_AnalyticsConsentYes(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Simulate answering "s" to the analytics consent prompt.
	res := runCLI(t, "s\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	// Verify .archetipo/config.yaml contains consent: true.
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "consent: true") {
		t.Fatalf("expected consent: true, got:\n%s", s)
	}
}

func TestInit_AnalyticsConsentNo(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Simulate answering "n" to the analytics consent prompt.
	res := runCLI(t, "n\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "consent: false") {
		t.Fatalf("expected consent: false, got:\n%s", s)
	}
}

func TestInit_AnalyticsConsentDefaultEnter(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Default (Enter) = no consent → consent: false.
	res := runCLI(t, "\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "consent: false") {
		t.Fatalf("expected consent: false (default), got:\n%s", s)
	}
}

func TestInit_AnalyticsConsentYesFlagSkipsPrompt(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// --yes skips the analytics prompt entirely; consent is NOT written.
	// The config template may include a commented consent line — that's fine.
	res := runCLI(t, "", "init", "--tool", "claude", "--connector", "file", "--yes")
	if res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	// The template includes a commented-out consent; the key must not
	// appear uncommented.
	if strings.Contains(s, "\n  consent:") {
		t.Fatalf("expected NO uncommented consent key with --yes, got:\n%s", s)
	}
}

func TestInit_AnalyticsConsentAlreadySetDoesNotReprompt(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// First init: set consent to true via stdin "s".
	res := runCLI(t, "s\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("first init failed: stderr=%s", res.stderr.String())
	}

	// Second init WITHOUT --yes: analytics prompt is skipped because consent
	// already set; answer "n" to the config-overwrite prompt.
	res2 := runCLI(t, "n\n", "init", "--tool", "claude", "--connector", "file")
	if res2.exit != 0 {
		t.Fatalf("second init failed: stderr=%s", res2.stderr.String())
	}

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "consent: true") {
		t.Fatalf("expected consent: true preserved, got:\n%s", s)
	}
}

// --- TASK-07: analytics in doctor ---

func TestDoctor_AnalyticsDefault(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Init without analytics consent (--yes skips prompt).
	res := runCLI(t, "", "init", "--tool", "claude", "--connector", "file", "--yes")
	if res.exit != 0 {
		t.Fatalf("init failed: stderr=%s", res.stderr.String())
	}

	doctorRes := runCLI(t, "", "doctor")
	out := doctorRes.stdout.String()
	if !strings.Contains(out, "analytics") {
		t.Fatalf("expected analytics line in doctor output, got:\n%s", out)
	}
	if !strings.Contains(out, "disabled (default)") {
		t.Fatalf("expected 'disabled (default)' in doctor output, got:\n%s", out)
	}
}

func TestDoctor_AnalyticsEnabled(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Init with analytics consent=true.
	res := runCLI(t, "s\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("init failed: stderr=%s", res.stderr.String())
	}

	doctorRes := runCLI(t, "", "doctor")
	out := doctorRes.stdout.String()
	if !strings.Contains(out, "analytics") {
		t.Fatalf("expected analytics line in doctor output, got:\n%s", out)
	}
	if !strings.Contains(out, "enabled (project_config)") {
		t.Fatalf("expected 'enabled (project_config)' in doctor output, got:\n%s", out)
	}
}

func TestDoctor_AnalyticsDisabled(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// Init with analytics consent=false.
	res := runCLI(t, "n\n", "init", "--tool", "claude", "--connector", "file")
	if res.exit != 0 {
		t.Fatalf("init failed: stderr=%s", res.stderr.String())
	}

	doctorRes := runCLI(t, "", "doctor")
	out := doctorRes.stdout.String()
	if !strings.Contains(out, "analytics") {
		t.Fatalf("expected analytics line in doctor output, got:\n%s", out)
	}
	if !strings.Contains(out, "disabled (project_config)") {
		t.Fatalf("expected 'disabled (project_config)' in doctor output, got:\n%s", out)
	}
}

func TestDoctor_AnalyticsWithoutConfig(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// No config at all — doctor uses defaults, shows disabled.
	doctorRes := runCLI(t, "", "doctor")
	// May fail because of missing installed skills; that's fine.
	out := doctorRes.stdout.String()
	if !strings.Contains(out, "analytics") {
		t.Fatalf("expected analytics line in doctor output, got:\n%s", out)
	}
	if !strings.Contains(out, "disabled (default)") {
		t.Fatalf("expected 'disabled (default)' in doctor output, got:\n%s", out)
	}
}

// --- TASK-03: HTTP integration tests with httptest ---

func TestAnalyticsHTTPIntegration_EndToEnd(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	var payloads []map[string]any
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requestCount++

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payloads = append(payloads, body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	writeAnalyticsConfig(t, true, srv.URL)

	// Run several CLI commands. spec list may fail without a backlog;
	// analytics fires regardless of success/failure.
	runCLI(t, "", "version")
	runCLI(t, "", "config", "show")
	runCLI(t, "", "spec", "list")

	mu.Lock()
	count := requestCount
	mu.Unlock()
	if count != 3 {
		t.Fatalf("expected 3 requests, got %d", count)
	}

	mu.Lock()
	defer mu.Unlock()
	wantedCmds := map[string]bool{"version": false, "config.show": false, "spec.list": false}
	for i, p := range payloads {
		if p["schema"] != "archetipo.analytics/v1" {
			t.Errorf("payload %d: expected schema='archetipo.analytics/v1', got %v", i, p["schema"])
		}
		if p["event"] != "command_completed" {
			t.Errorf("payload %d: expected event='command_completed', got %v", i, p["event"])
		}
		cmd, _ := p["command"].(string)
		if cmd == "" {
			t.Errorf("payload %d: expected non-empty command", i)
		}
		if _, ok := wantedCmds[cmd]; ok {
			wantedCmds[cmd] = true
		}
		// success must be present.
		if _, ok := p["success"]; !ok {
			t.Errorf("payload %d: expected success field", i)
		}
		// exit_code: may be omitted when 0 (omitempty), so check only when present.
		if ec, ok := p["exit_code"].(float64); ok && ec < 0 {
			t.Errorf("payload %d: expected exit_code >= 0, got %v", i, ec)
		}
		dur, _ := p["duration_ms"].(float64)
		if dur < 0 {
			t.Errorf("payload %d: expected duration_ms >= 0, got %v", i, dur)
		}
		conn, _ := p["connector"].(string)
		if conn == "" {
			t.Errorf("payload %d: expected non-empty connector", i)
		}
	}
	for cmd, found := range wantedCmds {
		if !found {
			t.Errorf("expected command %q but was not received", cmd)
		}
	}
}

func TestAnalyticsHTTPIntegration_CommandNormalized(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	var commands []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		cmd, _ := body["command"].(string)
		commands = append(commands, cmd)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	writeAnalyticsConfig(t, true, srv.URL)

	// config show → command should be "config.show"
	runCLI(t, "", "config", "show")
	// spec show with a code (will fail, analytics still fires)
	// command should be "spec.show", NOT "spec.show.US-001"
	runCLI(t, "", "spec", "show", "US-001")

	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(commands), commands)
	}
	if commands[0] != "config.show" {
		t.Errorf("expected first command='config.show', got %q", commands[0])
	}
	if commands[1] != "spec.show" {
		t.Errorf("expected second command='spec.show' (normalized, not 'spec.show.US-001'), got %q", commands[1])
	}
}

// --- TASK-04: Privacy regression — forbidden fields in payload ---

// analyticsDenylist is the set of fields that must NEVER appear in an
// analytics event payload. Keep in sync with docs/analytics.md §6.
var analyticsDenylist = map[string]string{
	"path":              "filesystem path",
	"cwd":               "current working directory",
	"project_root":      "absolute project root path",
	"repo_name":         "repository name",
	"git_remote":        "git remote URL",
	"hostname":          "machine hostname",
	"username":          "user name (PII)",
	"email":             "email address (PII)",
	"token":             "authentication token",
	"issue_url":         "full issue/repo URL",
	"prd_content":       "PRD body content",
	"spec_content":      "spec body content",
	"plan_content":      "implementation plan body",
	"stdin_payload":     "stdin input payload",
	"stdout_payload":    "stdout output payload",
	"stderr_payload":    "stderr output payload",
	"error_message_raw": "raw error message (may contain paths)",
}

func TestAnalyticsPrivacy_NoForbiddenFields(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	var violations []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for key := range body {
			if reason, forbidden := analyticsDenylist[key]; forbidden {
				violations = append(violations, fmt.Sprintf("%s (%s)", key, reason))
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	writeAnalyticsConfig(t, true, srv.URL)

	// Run a variety of commands that exercise different code paths.
	// Some will fail (no backlog/PRD), but analytics fires regardless.
	commands := [][]string{
		{"version"},
		{"config", "show"},
		{"spec", "list"},
		{"spec", "show", "US-001"},
		{"metrics"},
	}
	for _, args := range commands {
		runCLI(t, "", args...)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(violations) > 0 {
		t.Errorf("forbidden fields found in analytics payload:\n%s",
			strings.Join(violations, "\n"))
	}
}

// TestAnalyticsPrivacy_ForbiddenFieldsAcrossAllCommands runs every known
// CLI subcommand and checks that none of them leak forbidden fields.
func TestAnalyticsPrivacy_ForbiddenFieldsAcrossAllCommands(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	var violations []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for key := range body {
			if reason, forbidden := analyticsDenylist[key]; forbidden {
				violations = append(violations, fmt.Sprintf("%s (%s)", key, reason))
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Seed a minimal project so commands that need state work.
	specsFile := writeInputFile(t, "specs.json", specJSON)
	planFile := writeInputFile(t, "plan.json", planJSON)
	runCLI(t, "", "spec", "add", "--file", specsFile)
	runCLI(t, "", "spec", "plan", "US-001", "--file", planFile)

	writeAnalyticsConfig(t, true, srv.URL)

	allCommands := [][]string{
		{"version"},
		{"config", "show"},
		{"spec", "list"},
		{"spec", "show", "US-001"},
		{"spec", "next", "--status", "TODO"},
		{"metrics"},
		{"analytics", "status"},
		{"analytics", "enable"},
		{"analytics", "disable"},
		{"task", "done", "US-001", "TASK-01"},
	}
	for _, args := range allCommands {
		runCLI(t, "", args...)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(violations) > 0 {
		t.Errorf("forbidden fields found in analytics payload:\n%s",
			strings.Join(violations, "\n"))
	}
}

// --- TASK-05: Resilience — unreachable / timeout endpoint ---

func TestAnalyticsResilience_EndpointUnreachable(t *testing.T) {
	// Record baseline: version output WITHOUT analytics.
	newProject(t)
	baseline := runCLI(t, "", "version")

	// Now with analytics pointing to a closed server.
	newProject(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := srv.URL
	srv.Close() // immediately close → connection refused

	writeAnalyticsConfig(t, true, closedURL)
	res := runCLI(t, "", "version")

	// (a) exit code must match.
	if res.exit != baseline.exit {
		t.Errorf("exit code changed: baseline=%d, with-unreachable=%d", baseline.exit, res.exit)
	}
	// (b) stdout must match.
	if res.stdout.String() != baseline.stdout.String() {
		t.Errorf("stdout differs with unreachable endpoint.\nbaseline: %s\ngot:      %s",
			baseline.stdout.String(), res.stdout.String())
	}
	// (c) stderr must NOT contain analytics error messages.
	stderrStr := res.stderr.String()
	for _, forbidden := range []string{"analytics", "telemetria", "connection refused", "timeout"} {
		if strings.Contains(strings.ToLower(stderrStr), forbidden) {
			t.Errorf("stderr contains analytics-related text: %q", stderrStr)
		}
	}
}

func TestAnalyticsResilience_MultipleCommandsWithUnreachableEndpoint(t *testing.T) {
	// Test that CLI commands work correctly with unreachable analytics endpoint.
	// We verify exit codes are correct and JSON envelopes are valid,
	// regardless of analytics endpoint status.

	newProject(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := srv.URL
	srv.Close()

	writeAnalyticsConfig(t, true, closedURL)

	// Commands that should succeed with exit 0.
	// version outputs plain text (not JSON envelope).
	res := runCLI(t, "", "version")
	if res.exit != 0 {
		t.Errorf("version: expected exit 0, got %d (stderr=%s)",
			res.exit, res.stderr.String())
	}
	// config show should succeed.
	res = runCLI(t, "", "config", "show")
	if res.exit != 0 {
		t.Errorf("config show: expected exit 0, got %d (stderr=%s)",
			res.exit, res.stderr.String())
	}
	kind, _ := decodeOK(t, res)
	if kind == "" {
		t.Error("config show: no kind in JSON envelope")
	}

	// Commands that may fail — verify error is the expected type, not analytics.
	res = runCLI(t, "", "spec", "show", "US-001")
	exit, code := decodeError(t, res)
	if exit != 4 {
		t.Errorf("spec show US-001: expected exit=4 (E_PRECONDITION), got %d", exit)
	}
	if code != iox.CodePreconditionMissing {
		t.Errorf("spec show US-001: expected code=%s, got %s", iox.CodePreconditionMissing, code)
	}
}

// TestAnalyticsResilience_TimeoutDoesNotAlterOutput verifies that a slow
// server (response after >2s client timeout) does not cause panics or output
// changes. Since the analytics client has a 2s timeout, we configure the
// server to sleep longer and verify the CLI still works correctly.
func TestAnalyticsResilience_TimeoutDoesNotAlterOutput(t *testing.T) {
	// Baseline: version output WITHOUT analytics.
	newProject(t)
	baseline := runCLI(t, "", "version")

	// Analytics with a server that responds after a long delay.
	newProject(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the 2s client timeout.
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	writeAnalyticsConfig(t, true, srv.URL)
	res := runCLI(t, "", "version")

	if res.exit != baseline.exit {
		t.Errorf("exit code changed: baseline=%d, with-timeout=%d", baseline.exit, res.exit)
	}
	if res.stdout.String() != baseline.stdout.String() {
		t.Errorf("stdout differs with timeout.\nbaseline: %s\ngot:      %s",
			baseline.stdout.String(), res.stdout.String())
	}
	// No panic, no crash. Got here.
}

// --- TASK-06: Consent — disabled, init non-interactive, analytics disable ---

func TestAnalyticsConsent_DisabledSendsNoEvents(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// consent: false + endpoint set to server URL.
	writeAnalyticsConfig(t, false, srv.URL)

	// Run several commands.
	commands := [][]string{
		{"version"},
		{"config", "show"},
		{"spec", "list"},
		{"analytics", "status"},
	}
	for _, args := range commands {
		runCLI(t, "", args...)
	}

	mu.Lock()
	count := requestCount
	mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 analytics requests when consent=false, got %d", count)
	}
}

func TestAnalyticsConsent_InitNonInteractiveDoesNotEnableConsent(t *testing.T) {
	newProject(t)
	t.Setenv("ARCHETIPO_DATA_DIR", repoDataDir(t))

	// --yes skips the analytics prompt; consent must NOT be written.
	res := runCLI(t, "", "init", "--tool", "claude", "--connector", "file", "--yes")
	if res.exit != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", res.stdout.String(), res.stderr.String())
	}

	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	// The template may include a commented-out consent; the key must not
	// appear uncommented.
	if strings.Contains(s, "\n  consent:") {
		t.Fatalf("expected NO uncommented consent key with --yes, got:\n%s", s)
	}

	// Verify that after init without consent, no analytics events are sent.
	var mu sync.Mutex
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Patch config to add endpoint but keep consent unset.
	// Since consent key is absent, analytics is disabled by default.
	patchConfig := `connector: file
paths:
  prd: docs/PRD.md
analytics:
  endpoint: ` + srv.URL + "\n"
	writeConfig(t, patchConfig)

	runCLI(t, "", "version")
	runCLI(t, "", "config", "show")

	mu.Lock()
	count := requestCount
	mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 analytics requests after non-interactive init, got %d", count)
	}
}

func TestAnalyticsConsent_DisableCommandStopsEvents(t *testing.T) {
	newProject(t)

	var mu sync.Mutex
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// First enable analytics and verify events are sent.
	writeAnalyticsConfig(t, true, srv.URL)
	runCLI(t, "", "version")

	mu.Lock()
	beforeDisable := requestCount
	mu.Unlock()
	if beforeDisable < 1 {
		t.Fatal("expected at least 1 analytics request before disable")
	}

	// Run analytics disable.
	res := runCLI(t, "", "analytics", "disable")
	if res.exit != 0 {
		t.Fatalf("analytics disable failed: %s", res.stderr.String())
	}

	// Reset counter and run commands after disable.
	mu.Lock()
	requestCount = 0
	mu.Unlock()

	runCLI(t, "", "version")
	runCLI(t, "", "config", "show")
	runCLI(t, "", "spec", "list")

	mu.Lock()
	afterDisable := requestCount
	mu.Unlock()
	if afterDisable != 0 {
		t.Errorf("expected 0 analytics requests after disable, got %d", afterDisable)
	}

	// Verify config file has consent: false.
	raw, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "consent: false") {
		t.Errorf("expected consent: false in config after disable, got:\n%s", string(raw))
	}
}

// --- TASK-07: Documentation validation ---

func TestAnalyticsDocumentation_ExistsAndComplete(t *testing.T) {
	// Read docs/analytics.md from the project root.
	// The test runs in a temp dir; we need to read the actual file.
	// Use the repo root resolved from the caller path.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot resolve caller path")
	}
	// cli/internal/cli/cli_test.go → repo root is three levels up.
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	analyticsDoc := filepath.Join(repoRoot, "docs", "analytics.md")

	content, err := os.ReadFile(analyticsDoc)
	if err != nil {
		t.Fatalf("docs/analytics.md not found at %s: %v", analyticsDoc, err)
	}
	s := string(content)

	// Must mention the event type.
	if !strings.Contains(s, "command_completed") {
		t.Error("docs/analytics.md does not mention 'command_completed'")
	}

	// Allowed fields table must contain key fields.
	allowedFields := []string{
		"`schema`", "`event`", "`command`", "`archetipo_version`",
		"`os`", "`arch`", "`connector`", "`session_id`", "`timestamp`",
	}
	for _, f := range allowedFields {
		if !strings.Contains(s, f) {
			t.Errorf("docs/analytics.md missing allowed field %s", f)
		}
	}

	// Forbidden fields table must contain key fields.
	forbiddenFields := []string{
		"`path`", "`hostname`", "`username`", "`token`",
	}
	for _, f := range forbiddenFields {
		if !strings.Contains(s, f) {
			t.Errorf("docs/analytics.md missing forbidden field %s", f)
		}
	}

	// Must contain enable/disable instructions.
	if !strings.Contains(s, "analytics enable") {
		t.Error("docs/analytics.md missing 'analytics enable' instruction")
	}
	if !strings.Contains(s, "analytics disable") {
		t.Error("docs/analytics.md missing 'analytics disable' instruction")
	}

	// Must contain consent mechanism explanation.
	if !strings.Contains(s, "archetipo init") {
		t.Error("docs/analytics.md missing reference to 'archetipo init'")
	}

	// Must mention anonymous_installation_id as UUID random.
	if !strings.Contains(s, "anonymous_installation_id") {
		t.Error("docs/analytics.md missing 'anonymous_installation_id'")
	}
}
