package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

const documentationActionRoute = "/api/documentation/wiki/actions"

func newWikiActionServer(t *testing.T) (*Server, *activityCatalogProvider) {
	t.Helper()
	native := newPersistentFakeNativeProvider("wiki-native", nil)
	native.capabilities = execution.WorkflowCapabilities()
	provider := &activityCatalogProvider{persistentFakeNativeProvider: native, known: true, skills: []execution.SessionSkill{{Name: "archetipo-wiki"}}}
	srv, _, _ := newRunServer(t, provider, true)
	t.Cleanup(func() { srv.session().nativeFollowers.closeAll() })
	return srv, provider
}

func seedWikiActionPage(t *testing.T, srv *Server) string {
	t.Helper()
	root := srv.session().cfg.Paths.Wiki
	if !filepath.IsAbs(root) {
		root = filepath.Join(srv.session().cfg.ProjectRoot, root)
	}
	if _, err := wiki.Init(root); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "overview.md")
	if err := os.WriteFile(file, []byte("---\ntype: overview\ntitle: Wiki di prova\ndescription: Una prova\nstatus: generated\n---\n# Wiki\n\nContenuto iniziale.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func startWikiAction(t *testing.T, srv *Server, action, page string) execution.Execution {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, documentationActionRoute, map[string]string{"action": action, "page_id": page})
	if w.Code != http.StatusCreated {
		t.Fatalf("start = %d: %s", w.Code, w.Body.String())
	}
	var record execution.Execution
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func readWikiActions(t *testing.T, srv *Server) documentationActionsView {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, documentationActionRoute, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("actions = %d: %s", w.Code, w.Body.String())
	}
	var view documentationActionsView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestDocumentationActionsUseTemplateLabelsAndWorkspaceState(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	if srv.session().cfg.Wiki.Enabled {
		t.Fatal("test must exercise explicit Wiki work with automatic maintenance off")
	}
	view := readWikiActions(t, srv)
	if len(view.Actions) != 1 || view.Actions[0].Label != "Crea Wiki" || !view.Actions[0].Runnable || provider.store.next != 0 {
		t.Fatalf("absent Wiki: %+v", view)
	}
	// An initialized but empty Wiki is still offered creation, independent of a search query.
	root := filepath.Join(srv.session().cfg.ProjectRoot, srv.session().cfg.Paths.Wiki)
	if _, err := wiki.Init(root); err != nil {
		t.Fatal(err)
	}
	if len(readWikiActions(t, srv).Actions) != 1 {
		t.Fatal("empty Wiki cannot be created")
	}
	seedWikiActionPage(t, srv)
	view = readWikiActions(t, srv)
	if len(view.Actions) != 3 || view.Actions[0].Label != "Aggiorna Wiki" || provider.store.next != 0 {
		t.Fatalf("existing Wiki: %+v", view)
	}
	custom := template.Template{DocumentationActions: []template.DocumentationAction{{WorkspaceAction: template.WorkspaceAction{ID: "future", Label: "Etichetta del processo"}, Placement: "menu", RequiresWiki: true}}}
	got := documentationActions(custom, true, "Motivo del server")
	if len(got) != 1 || got[0].ID != "future" || got[0].Runnable || got[0].UnavailableReason != "Motivo del server" {
		t.Fatalf("template presentation changed: %+v", got)
	}
}

func TestDocumentationRefreshRunsInDedicatedNativeSessionAndReportsPersistedChanges(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	file := seedWikiActionPage(t, srv)
	existing := openConversationOK(t, srv).Conversation.ID
	started := startWikiAction(t, srv, "wiki-refresh", "")
	thread := conversationOfExecution(t, srv, started.ID)
	if thread == existing || provider.store.next != 2 || provider.store.starts != 1 {
		t.Fatal("Wiki action did not get exactly one dedicated native session")
	}
	if len(provider.lastStart.Skills) != 1 || provider.lastStart.Skills[0].Name != "archetipo-wiki" || !strings.Contains(provider.lastStart.Message, "refresh") || !strings.Contains(provider.lastStart.Message, "Do not approve or reconfirm") {
		t.Fatalf("wrong Wiki invocation: %+v", provider.lastStart)
	}
	duplicate := doJSON(t, srv, http.MethodPost, documentationActionRoute, map[string]string{"action": "wiki-lint"})
	if duplicate.Code != http.StatusConflict || provider.store.starts != 1 {
		t.Fatalf("duplicate = %d %s", duplicate.Code, duplicate.Body.String())
	}
	provider.completeCurrentTurn(t, "Quale cambiamento vuoi approfondire?", execution.TurnCompleted)
	// Ending on a question keeps the execution and native session open.
	doJSON(t, srv, http.MethodGet, conversationsRoute+"/"+thread, nil)
	_, pending := readExecution(t, srv, started.ID)
	if pending.Status != execution.StatusRunning {
		t.Fatal("question closed the Wiki action")
	}
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+thread+"/messages", map[string]string{"message": "Verifica tutto il codice corrente"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("answer: %d %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.ReplaceAll(string(raw), "iniziale", "aggiornato")), 0o600); err != nil {
		t.Fatal(err)
	}
	// The durable baseline, not an in-memory closure, also verifies after a restart.
	srv.session().actionConfirmations.Delete(started.ID)
	provider.completeCurrentTurn(t, `Wiki aggiornata.
{"artifact":"wiki","operation":"refresh","status":"COMPLETED"}`, execution.TurnCompleted)
	completed := awaitExecutionStatus(t, srv, started.ID, execution.StatusSucceeded)
	if !strings.Contains(string(completed.Result.Payload), `"wiki_changed_pages":["overview"]`) {
		t.Fatalf("missing verified changes: %s", completed.Result.Payload)
	}
	view := readWikiActions(t, srv)
	if len(view.Activities) != 1 || view.Activities[0].Running || view.Activities[0].ConversationID != thread || len(view.Activities[0].ChangedPages) != 1 {
		t.Fatalf("activity: %+v", view)
	}
	record, err := srv.session().conversationStore().GetMetadata(context.Background(), thread)
	if err != nil || record.Session == nil || record.Archive == execution.ArchiveArchived {
		t.Fatal("receipt closed or lost the native session")
	}
}

func TestDocumentationRejectsInvalidSelectionsAndUnavailableCatalogWithoutExecution(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	for _, body := range []map[string]string{{"action": "other"}, {"action": "wiki-refresh"}, {"action": "wiki-query", "page_id": "../../private"}} {
		w := doJSON(t, srv, http.MethodPost, documentationActionRoute, body)
		if w.Code < 400 || provider.store.next != 0 {
			t.Fatalf("invalid start produced a session: %d %s", w.Code, w.Body.String())
		}
	}
	seedWikiActionPage(t, srv)
	w := doJSON(t, srv, http.MethodPost, documentationActionRoute, map[string]string{"action": "wiki-query", "page_id": "../../private"})
	if w.Code != http.StatusNotFound || provider.store.next != 0 {
		t.Fatalf("invalid page: %d", w.Code)
	}
	provider.skills = nil
	w = doJSON(t, srv, http.MethodPost, documentationActionRoute, map[string]string{"action": "wiki-refresh"})
	records, err := srv.session().store.List(context.Background())
	if w.Code != http.StatusConflict || err != nil || len(records) != 0 || provider.store.starts != 0 {
		t.Fatalf("missing catalog activity started work: %d %s", w.Code, w.Body.String())
	}
}

func TestDocumentationDoesNotAcceptBootstrapReceiptWithoutBootstrapEffect(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	started := startWikiAction(t, srv, "wiki-create", "")
	seedWikiActionPage(t, srv) // One page is not the complete bootstrap map.
	provider.completeCurrentTurn(t, `{"artifact":"wiki","operation":"bootstrap","status":"COMPLETED"}`, execution.TurnCompleted)
	failed := awaitExecutionStatus(t, srv, started.ID, execution.StatusFailed)
	if failed.Error == nil || failed.Error.Code != "UNCONFIRMED_EFFECT" || failed.Result != nil {
		t.Fatalf("false bootstrap accepted: %+v", failed)
	}
}

func TestDocumentationQueryCarriesExactPageAndRejectsWrites(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	file := seedWikiActionPage(t, srv)
	started := startWikiAction(t, srv, "wiki-query", "overview")
	if !strings.Contains(provider.lastStart.Message, "exact Wiki ID is overview") {
		t.Fatal("page context missing")
	}
	raw, _ := os.ReadFile(file)
	if err := os.WriteFile(file, append(raw, []byte("\nUnauthorized change.\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	provider.completeCurrentTurn(t, `{"artifact":"wiki","operation":"query","status":"COMPLETED"}`, execution.TurnCompleted)
	failed := awaitExecutionStatus(t, srv, started.ID, execution.StatusFailed)
	if failed.Error == nil || !strings.Contains(failed.Error.Message, "modificato") {
		t.Fatalf("query writes accepted: %+v", failed)
	}
}

func TestDocumentationMaintenanceCannotApproveChangedPages(t *testing.T) {
	srv, provider := newWikiActionServer(t)
	file := seedWikiActionPage(t, srv)
	started := startWikiAction(t, srv, "wiki-refresh", "")
	if _, err := wiki.Approve(srv.session().cfg.ProjectRoot, filepath.Dir(file), []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	provider.completeCurrentTurn(t, `{"artifact":"wiki","operation":"refresh","status":"COMPLETED"}`, execution.TurnCompleted)
	failed := awaitExecutionStatus(t, srv, started.ID, execution.StatusFailed)
	if failed.Error == nil || !strings.Contains(failed.Error.Message, "approvata") {
		t.Fatalf("automatic approval accepted: %+v", failed)
	}
}
