package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

func activityByID(items []conversationActivityView, id string) (conversationActivityView, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return conversationActivityView{}, false
}

func TestConversationActivitiesDoNotGuessAnUnannouncedCatalog(t *testing.T) {
	tpl := template.Default()
	items := resolveConversationActivities(tpl, nil, conversationlog.Record{}, execution.SessionDiscovery{
		Skills: []execution.SessionSkill{{Name: "archetipo-design"}},
	}, "")
	design, found := activityByID(items, "design")
	if !found || design.Runnable || design.UnavailableReason == "" {
		t.Fatalf("unknown catalog became available: %+v", items)
	}
	if _, found := activityByID(items, "native:archetipo-design"); found {
		t.Fatal("unknown catalog was treated as a discovered skill")
	}
	record := conversationlog.Record{Version: conversationlog.CurrentVersion,
		Session: &execution.SessionMetadata{Native: execution.NativeSessionReference{ID: "native"}},
		Archive: execution.ArchiveOpen, Recovery: execution.RecoveryResumable, Work: execution.SessionIdle}
	items = resolveConversationActivities(tpl, nil, record, execution.SessionDiscovery{SkillsKnown: true}, "")
	if _, found := activityByID(items, "design"); found {
		t.Fatal("a known catalog without design still offers it")
	}
}

func TestConversationActivitiesKeepTheProcessesOwnOffersAndReasons(t *testing.T) {
	items := resolveConversationActivities(template.Default(), []workspaceActionView{
		{WorkspaceAction: template.WorkspaceAction{ID: "inception"}, Offered: false},
		{WorkspaceAction: template.WorkspaceAction{ID: "spec-draft"}, Offered: true, Runnable: false, UnavailableReason: "provider unavailable"},
	}, conversationlog.Record{}, execution.SessionDiscovery{}, "cannot converse")
	if _, found := activityByID(items, "product"); found {
		t.Fatal("an inception over an existing PRD is offered")
	}
	story, found := activityByID(items, "story")
	if !found || story.Runnable || story.UnavailableReason != "provider unavailable" || story.Action != "spec-draft" {
		t.Fatalf("process availability was changed: %+v", story)
	}
	free, found := activityByID(items, "conversation")
	if !found || free.Runnable || free.UnavailableReason != "cannot converse" {
		t.Fatalf("conversation availability was changed: %+v", free)
	}
}

func TestConversationActivityLabelsDoNotExposeOtherCatalogNames(t *testing.T) {
	items := resolveConversationActivities(template.Default(), nil, conversationlog.Record{}, execution.SessionDiscovery{
		SkillsKnown: true,
		Skills: []execution.SessionSkill{
			{Name: "archetipo-design"},
			{Name: "helper", Description: "Prepara helper dopo archetipo-design"},
		},
	}, "")
	item, found := activityByID(items, "native:helper")
	if !found || strings.Contains(item.Label, "helper") || strings.Contains(item.Label, "archetipo-design") {
		t.Fatalf("catalog names exposed in a label: %+v", item)
	}
}

type activityCatalogProvider struct {
	*persistentFakeNativeProvider
	known  bool
	skills []execution.SessionSkill
}

func (p *activityCatalogProvider) DiscoverSession(ctx context.Context, request execution.SessionDiscoveryRequest) (execution.SessionDiscovery, error) {
	discovery, err := p.persistentFakeNativeProvider.DiscoverSession(ctx, request)
	discovery.SkillsKnown = p.known
	discovery.Skills = p.skills
	return discovery, err
}

func readActivityCatalog(t *testing.T, srv *Server, id string) conversationActivitiesView {
	t.Helper()
	route := "/api/workspace/activities"
	if id != "" {
		route += "?conversation_id=" + id
	}
	w := doJSON(t, srv, http.MethodGet, route, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("activities = %d: %s", w.Code, w.Body.String())
	}
	var view conversationActivitiesView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestConversationActivitiesUseTheNativeSessionAndLeaveTheMessageUntouched(t *testing.T) {
	provider := &activityCatalogProvider{
		persistentFakeNativeProvider: newPersistentFakeNativeProvider("native-activities", nil), known: true,
		skills: []execution.SessionSkill{{Name: "archetipo-design", Description: "Native design", Invocation: "/archetipo-design"}},
	}
	srv, _, _ := newRunServer(t, provider, true)
	before := readActivityCatalog(t, srv, "")
	if provider.store.next != 0 || provider.store.starts != 0 || before.SkillsKnown {
		t.Fatal("reading the starter opened a session or invented its catalog")
	}
	starter, found := activityByID(before.Items, "design")
	if !found || starter.Kind != "connect" || !starter.Runnable {
		t.Fatalf("native discovery cannot be started from the activity: %+v", starter)
	}
	id := openConversationOK(t, srv).Conversation.ID
	view := readActivityCatalog(t, srv, id)
	design, found := activityByID(view.Items, "design")
	if !found || !design.Runnable || design.Label != "Nuovo mockup" || !view.SkillsKnown || view.ConversationID != id {
		t.Fatalf("native design not offered: %+v", view)
	}
	if provider.store.starts != 0 || strings.Contains(design.Label, design.Skill) {
		t.Fatal("the read starts work or exposes the invocation as its label")
	}
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{
		"message": "Rappresenta la schermata clienti", "skill": design.Skill,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("selected activity = %d: %s", w.Code, w.Body.String())
	}
	if provider.lastStart.Message != "Rappresenta la schermata clienti" || len(provider.lastStart.Skills) != 1 || provider.lastStart.Skills[0].Name != design.Skill || provider.lastStart.Session.ConversationID != id {
		t.Fatalf("activity changed message or session: %+v", provider.lastStart)
	}
	busy := readActivityCatalog(t, srv, id)
	design, found = activityByID(busy.Items, "design")
	if !found || design.Runnable || design.UnavailableReason == "" {
		t.Fatal("a native activity is offered during an active turn")
	}
	closeConversation(t, srv, id)
}

func TestConversationActivitiesDoNotBorrowAnotherConversationsCatalog(t *testing.T) {
	provider := &activityCatalogProvider{persistentFakeNativeProvider: newPersistentFakeNativeProvider("native-activities", nil), known: true}
	srv, _, _ := newRunServer(t, provider, true)
	id := openConversationOK(t, srv).Conversation.ID
	view := readActivityCatalog(t, srv, id)
	if _, found := activityByID(view.Items, "design"); found {
		t.Fatal("design was inferred from installed files rather than this catalog")
	}
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/activities?conversation_id=unknown", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation = %d, want 404", w.Code)
	}
	closeConversation(t, srv, id)
}
