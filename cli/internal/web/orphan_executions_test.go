package web

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The tests in this file are about a record left RUNNING by a process that is
// no longer there. Such a record blocks every action of its scope with "already
// running" and, before these tests existed, nothing could ever close it. What
// they pin down is as much what is closed as what is deliberately left alone:
// a record another live process is carrying, one whose work runs on a remote
// system, and one that belongs to a native session.

// orphanProvider can run an inception and exposes interactive runs, so the
// cancel route resolves it and finds no run behind an orphaned record. detached
// makes it declare its work as living outside the viewer, as a remote hub does.
type orphanProvider struct {
	*inceptionProvider
	*workspaceRunsCollaborator
	detached bool
}

func (p *orphanProvider) WorkOutlivesDispatcher() bool { return p.detached }

func newOrphanProvider(detached bool) *orphanProvider {
	return &orphanProvider{
		inceptionProvider:         releasedInceptionProvider("fake", nil),
		workspaceRunsCollaborator: newWorkspaceRunsCollaborator(),
		detached:                  detached,
	}
}

// deadPID is a pid no process holds: the maximum a pid can be on the platforms
// the viewer runs on is far below it.
const deadPID = 1 << 30

// seedRunningInception writes the RUNNING record of a workspace inception, as
// the process that created it left it.
func seedRunningInception(t *testing.T, root, id string, ownerPID int) {
	t.Helper()
	store, err := execution.NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	record := execution.Execution{
		ID:         id,
		SpecCode:   workspaceExecutionKey,
		Action:     execution.ActionInception,
		Capability: execution.CapabilityWorkspaceInception,
		ProviderID: "fake",
		Status:     execution.StatusRunning,
		CreatedAt:  time.Now().UTC().Add(-24 * time.Hour),
		WorkingDir: root,
		OwnerPID:   ownerPID,
	}
	if err := store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

// restartOver starts a new viewer on a workspace, which is where a restart
// reconciles what the previous process left behind.
func restartOver(t *testing.T, cfg config.Config, conn connector.Connector, provider execution.Provider) *Server {
	t.Helper()
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.session().stop(2 * time.Second) })
	return srv
}

func TestARestartClosesAnOrphanedInceptionAndFreesTheAction(t *testing.T) {
	provider := newOrphanProvider(false)
	_, cfg, conn := newRunServer(t, provider, true)
	// A record written before owners were recorded: the case that was found
	// in a real workspace, blocking its inception a day after the viewer died.
	seedRunningInception(t, cfg.ProjectRoot, "exec-orphan-legacy", 0)

	srv := restartOver(t, cfg, conn, provider)

	_, record := readExecution(t, srv, "exec-orphan-legacy")
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Code != execution.OrphanedCode {
		t.Fatalf("the orphan was not closed: %s %#v", record.Status, record.Error)
	}
	if record.CompletedAt == nil {
		t.Fatal("the orphan was closed without a completion time")
	}
	if !strings.Contains(record.Error.Message, "no PRD") {
		t.Fatalf("the failure does not say that no PRD was produced: %q", record.Error.Message)
	}
	view, raw := readWorkspaceActions(t, srv)
	if action := inceptionActionOf(t, view); !action.Runnable {
		t.Fatalf("the inception is still blocked after the orphan was closed: %s", raw)
	}
}

func TestARestartKeepsThePRDAnOrphanedInceptionLeftBehind(t *testing.T) {
	provider := newOrphanProvider(false)
	_, cfg, conn := newRunServer(t, provider, true)
	seedRunningInception(t, cfg.ProjectRoot, "exec-orphan-prd", deadPID)
	// The run wrote a document and died before its receipt: nothing proves it
	// is complete, and nothing proves the workspace did not have it before.
	path := writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nScritto a metà.\n")
	before := mustReadFile(t, path)

	srv := restartOver(t, cfg, conn, provider)

	_, record := readExecution(t, srv, "exec-orphan-prd")
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Code != execution.OrphanedCode {
		t.Fatalf("the orphan was not closed: %s %#v", record.Status, record.Error)
	}
	if !strings.Contains(record.Error.Message, "PRD found on disk was kept") {
		t.Fatalf("the failure does not say the PRD was kept: %q", record.Error.Message)
	}
	if after := mustReadFile(t, path); !bytes.Equal(before, after) {
		t.Fatalf("closing an orphan touched the PRD:\n%s", after)
	}
}

func TestARestartLeavesARunAnotherLiveProcessIsCarrying(t *testing.T) {
	provider := newOrphanProvider(false)
	_, cfg, conn := newRunServer(t, provider, true)
	// The parent of the test binary is alive for as long as the test runs: it
	// plays the second viewer still working on this record.
	seedRunningInception(t, cfg.ProjectRoot, "exec-carried", os.Getppid())

	srv := restartOver(t, cfg, conn, provider)

	if _, record := readExecution(t, srv, "exec-carried"); record.Status != execution.StatusRunning {
		t.Fatalf("a run carried by a live process was closed: %s %#v", record.Status, record.Error)
	}
	w := doJSON(t, srv, http.MethodPost, "/api/execution/exec-carried/run/cancel", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("cancel closed a run carried by a live process: %d %s", w.Code, w.Body.String())
	}
	if _, record := readExecution(t, srv, "exec-carried"); record.Status != execution.StatusRunning {
		t.Fatalf("a refused cancel changed the record: %s", record.Status)
	}
}

func TestARestartLeavesTheRecordOfDetachedWork(t *testing.T) {
	provider := newOrphanProvider(true)
	_, cfg, conn := newRunServer(t, provider, true)
	seedRunningInception(t, cfg.ProjectRoot, "exec-remote", deadPID)

	srv := restartOver(t, cfg, conn, provider)

	if _, record := readExecution(t, srv, "exec-remote"); record.Status != execution.StatusRunning {
		t.Fatalf("the record of work running on a remote system was closed: %s %#v", record.Status, record.Error)
	}
}

func TestARestartLeavesAnActionOfANativeSessionToTheSession(t *testing.T) {
	provider := newOrphanProvider(false)
	first, cfg, conn := newRunServer(t, provider, true)
	seedRunningInception(t, cfg.ProjectRoot, "exec-in-session", deadPID)
	// A conversation that names the execution as its own: closing it is the
	// session's business, through the turn it was carried in.
	conversation := conversationlog.Record{ID: "conv-orphan-test", ExecutionIDs: []string{"exec-in-session"}}
	if err := first.session().conversationStore().Save(context.Background(), conversation); err != nil {
		t.Fatal(err)
	}

	srv := restartOver(t, cfg, conn, provider)

	if _, record := readExecution(t, srv, "exec-in-session"); record.Status != execution.StatusRunning {
		t.Fatalf("an action of a conversation was closed as an orphan: %s %#v", record.Status, record.Error)
	}
}

func TestCancelClosesAnOrphanThatHasNoRun(t *testing.T) {
	provider := newOrphanProvider(false)
	srv, cfg, _ := newRunServer(t, provider, true)
	// Seeded after the viewer started, so only the cancel can close it.
	seedRunningInception(t, cfg.ProjectRoot, "exec-cancel-orphan", deadPID)

	view, _ := readWorkspaceActions(t, srv)
	if action := inceptionActionOf(t, view); action.Runnable {
		t.Fatalf("the orphan does not block the inception before the cancel: %#v", action)
	}

	w := doJSON(t, srv, http.MethodPost, "/api/execution/exec-cancel-orphan/run/cancel", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel on an orphan = %d: %s", w.Code, w.Body.String())
	}
	_, record := readExecution(t, srv, "exec-cancel-orphan")
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Code != execution.OrphanedCode {
		t.Fatalf("the orphan was not closed by the cancel: %s %#v", record.Status, record.Error)
	}
	view, raw := readWorkspaceActions(t, srv)
	if action := inceptionActionOf(t, view); !action.Runnable {
		t.Fatalf("the inception is still blocked after the cancel: %s", raw)
	}
	// A second cancel finds a terminal record and changes nothing.
	if w := doJSON(t, srv, http.MethodPost, "/api/execution/exec-cancel-orphan/run/cancel", nil); w.Code != http.StatusAccepted {
		t.Fatalf("a second cancel = %d: %s", w.Code, w.Body.String())
	}
}
