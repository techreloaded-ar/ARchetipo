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

func TestConversationActivityLabelsDoNotExposeAgentInstructions(t *testing.T) {
	items := resolveConversationActivities(template.Default(), nil, conversationlog.Record{}, execution.SessionDiscovery{
		SkillsKnown: true,
		Skills: []execution.SessionSkill{
			{Name: "archetipo-design"},
			{Name: "plugin:build-report", Description: "Prepara plugin:build-report dopo archetipo-design. " + strings.Repeat("Use when the user asks for a report. ", 30)},
		},
	}, "")
	item, found := activityByID(items, "native:plugin:build-report")
	if !found || item.Label != "plugin · build report" || item.Description != "" || item.Skill != "plugin:build-report" {
		t.Fatalf("agent instructions became activity presentation: %+v", item)
	}
}

func TestConversationAutopilotUsesTemplatePresentationAndNativeInvocation(t *testing.T) {
	provider := &activityCatalogProvider{
		persistentFakeNativeProvider: newPersistentFakeNativeProvider("native-autopilot", nil), known: true,
		skills: []execution.SessionSkill{{Name: "archetipo-autopilot", Description: "Runs the ARchetipo pipeline autonomously end to end. " + strings.Repeat("Requires a fresh isolated worker. ", 30), Invocation: "/archetipo-autopilot"}},
	}
	srv, _, _ := newRunServer(t, provider, true)
	id := openConversationOK(t, srv).Conversation.ID
	view := readActivityCatalog(t, srv, id)
	activity, found := activityByID(view.Items, "autopilot")
	if !found || !activity.Runnable || activity.Label != "Avvia il workflow autonomo" || activity.Description != "Pianifica, implementa e verifica le storie del backlog in autonomia." || activity.Placeholder == "" {
		t.Fatalf("autopilot lacks user-facing presentation: %+v", activity)
	}
	if _, found := activityByID(view.Items, "native:archetipo-autopilot"); found {
		t.Fatal("autopilot is duplicated by the native catalog")
	}
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{
		"message": "Completa US-001", "skill": activity.Skill,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("selected autopilot = %d: %s", w.Code, w.Body.String())
	}
	if provider.lastStart.Display != "Completa US-001" || !strings.Contains(provider.lastStart.Message, "Workflow conversation: "+id) || len(provider.lastStart.Skills) != 1 || provider.lastStart.Skills[0].Name != "archetipo-autopilot" || provider.lastStart.Session.ConversationID != id {
		t.Fatalf("autopilot changed message, invocation or session: %+v", provider.lastStart)
	}
	closeConversation(t, srv, id)
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
	if provider.lastStart.Display != "Rappresenta la schermata clienti" || !strings.Contains(provider.lastStart.Message, "Workflow conversation: "+id) || len(provider.lastStart.Skills) != 1 || provider.lastStart.Skills[0].Name != design.Skill || provider.lastStart.Session.ConversationID != id {
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

func TestConversationActivitiesIdentifySkillsWithoutDescriptionsAndDoNotDuplicateActions(t *testing.T) {
	record := conversationlog.Record{Version: conversationlog.CurrentVersion,
		Session: &execution.SessionMetadata{Native: execution.NativeSessionReference{ID: "native"}},
		Archive: execution.ArchiveOpen, Recovery: execution.RecoveryResumable, Work: execution.SessionIdle}
	items := resolveConversationActivities(template.Default(), nil, record, execution.SessionDiscovery{
		SkillsKnown: true, Capabilities: []execution.SessionCapability{execution.SessionCapabilitySkillInvocation},
		Skills: []execution.SessionSkill{
			{Name: "plugin:build-report"}, {Name: "plugin:check-report"},
			{Name: "archetipo-inception"}, {Name: "archetipo-spec"},
		},
	}, "")
	first, found := activityByID(items, "native:plugin:build-report")
	second, foundSecond := activityByID(items, "native:plugin:check-report")
	if !found || !foundSecond || first.Label == second.Label || first.Label == "Attività dell’agente" || !first.Runnable {
		t.Fatalf("skills without descriptions are indistinguishable: %+v", items)
	}
	for _, id := range []string{"native:archetipo-inception", "native:archetipo-spec"} {
		if _, found := activityByID(items, id); found {
			t.Fatalf("%s bypasses the process action availability", id)
		}
	}
}

func TestConversationActivitiesExplainTheBlockAndItsRemedy(t *testing.T) {
	items := resolveConversationActivities(template.Default(), []workspaceActionView{{
		WorkspaceAction: template.WorkspaceAction{ID: "spec-draft"}, Offered: true,
		UnavailableReason: "L’attività exec-story è in corso", UnlockedBy: "Completala o annullala",
	}}, conversationlog.Record{}, execution.SessionDiscovery{}, "")
	story, found := activityByID(items, "story")
	if !found || !strings.Contains(story.UnavailableReason, "exec-story") || !strings.Contains(story.UnavailableReason, "annullala") {
		t.Fatalf("the activity hides why it is blocked: %+v", story)
	}
}

func TestConversationActivitiesRespectTheCurrentThreadWork(t *testing.T) {
	for _, state := range []execution.SessionWorkState{execution.SessionTurnActive, execution.SessionIdle} {
		t.Run(string(state), func(t *testing.T) {
			record := conversationlog.Record{Version: conversationlog.CurrentVersion,
				Session: &execution.SessionMetadata{Native: execution.NativeSessionReference{ID: "native"}},
				Archive: execution.ArchiveOpen, Recovery: execution.RecoveryResumable, Work: state}
			items := resolveConversationActivities(template.Default(), []workspaceActionView{{
				WorkspaceAction: template.WorkspaceAction{ID: "spec-draft"}, Offered: true, Runnable: true,
			}}, record, execution.SessionDiscovery{}, "")
			story, found := activityByID(items, "story")
			if !found || story.Runnable != (state == execution.SessionIdle) {
				t.Fatalf("action availability ignores the thread work: %+v", story)
			}
		})
	}
}
