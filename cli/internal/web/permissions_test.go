package web

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func TestConversationPermissionsPersistIndependentlyOfWorkspaceAndActiveTurn(t *testing.T) {
	provider := newPersistentFakeNativeProvider("partial-permissions", nil)
	provider.permissionProfiles = []execution.PermissionProfile{
		{ID: "bounded", Label: "Nel progetto", Description: "Con limiti"},
		{ID: "unrestricted", Label: "Tutto", Description: "Accesso ampio"},
	}
	provider.permissionProfile = "bounded"
	provider.providerConfig = map[string]any{"fixture_policy": "unchanged"}
	srv, cfg, conn := newRunServer(t, provider, true)
	t.Cleanup(srv.session().nativeFollowers.closeAll)
	id := openConversationOK(t, srv).Conversation.ID
	record, err := srv.session().conversationStore().GetMetadata(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	nativeID := record.Session.Native.ID
	workspaceDefault := execution.CloneConfig(record.Session.Environment.ProviderConfig)
	if record.NextTurn.PermissionProfile != "bounded" {
		t.Fatalf("inherited = %#v", record.NextTurn)
	}
	response := doJSON(t, srv, http.MethodGet, conversationsRoute+"/"+id+"/model-choice", nil)
	var catalog nativeConversationModelChoiceView
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog.PermissionProfiles) != 2 || catalog.PermissionProfile != "bounded" {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	for _, profile := range []string{"invented", "unrestricted"} {
		response = doJSON(t, srv, http.MethodPut, conversationsRoute+"/"+id+"/next-turn", map[string]any{"model": "fake-large", "permission_profile": profile})
		if profile == "invented" && response.Code != 400 || profile == "unrestricted" && response.Code != 200 {
			t.Fatalf("profile %s = %d %s", profile, response.Code, response.Body.String())
		}
	}
	// Legacy model-only clients must not silently reset the permission choice.
	response = doJSON(t, srv, http.MethodPut, conversationsRoute+"/"+id+"/next-turn", map[string]any{"model": "fake-large", "model_options": map[string]string{"effort": "high"}})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	response = doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "first"})
	if response.Code != http.StatusAccepted || provider.lastStart.PermissionProfile != "unrestricted" {
		t.Fatalf("turn = %d %s request=%#v", response.Code, response.Body.String(), provider.lastStart)
	}
	// Choosing during an active turn changes the next turn only.
	response = doJSON(t, srv, http.MethodPut, conversationsRoute+"/"+id+"/next-turn", map[string]any{"permission_profile": "bounded"})
	if response.Code != 200 || provider.lastStart.PermissionProfile != "unrestricted" {
		t.Fatal("active turn permissions changed")
	}
	record, err = srv.session().conversationStore().GetMetadata(context.Background(), id)
	if record.NextTurn.Model != "fake-large" || record.NextTurn.Options["effort"] != "high" {
		t.Fatalf("a permission-only update changed the model: %#v", record.NextTurn)
	}
	if err != nil || !reflect.DeepEqual(record.Session.Environment.ProviderConfig, workspaceDefault) {
		t.Fatalf("provider config changed: %#v %v", record, err)
	}
	response = doJSON(t, srv, http.MethodDelete, conversationsRoute+"/"+id, nil)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	srv.session().nativeFollowers.closeAll()
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restarted.session().dispatch.wait(5 * time.Second)
		restarted.session().nativeFollowers.closeAll()
	})
	restored, err := restarted.session().conversationStore().GetMetadata(context.Background(), id)
	if err != nil || restored.NextTurn.PermissionProfile != "bounded" || restored.Session.Native.ID != nativeID {
		t.Fatalf("restored = %#v %v", restored, err)
	}
	secondID := openConversationOK(t, restarted).Conversation.ID
	second, err := restarted.session().conversationStore().GetMetadata(context.Background(), secondID)
	if err != nil || second.NextTurn.PermissionProfile != "bounded" {
		t.Fatalf("default was changed by conversation: %#v %v", second, err)
	}
}

func TestPermissionChoiceIsIndependentOfModelCatalogAndAbsentCapability(t *testing.T) {
	for _, supported := range []bool{false, true} {
		for _, noModels := range []bool{false, true} {
			provider := newPersistentFakeNativeProvider("partial", nil)
			if supported {
				provider.permissionProfiles = []execution.PermissionProfile{{ID: "bounded", Label: "Limited"}}
			}
			provider.noModels = noModels
			srv, _, _ := newRunServer(t, provider, true)
			id := openConversationOK(t, srv).Conversation.ID
			response := doJSON(t, srv, http.MethodPut, conversationsRoute+"/"+id+"/next-turn", map[string]any{"permission_profile": "bounded"})
			want := http.StatusBadRequest
			if supported {
				want = http.StatusOK
			}
			if response.Code != want {
				t.Fatalf("supported %v: %d %s", supported, response.Code, response.Body.String())
			}
		}
	}
}
