package codex

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func TestNativePermissionProfilesUseExplicitBoundaries(t *testing.T) {
	for _, tc := range []struct{ profile, sandbox, approval string }{
		{"provider_default", "read-only", "on-request"},
		{"workspace", "workspace-write", "on-request"},
		{"workspace_no_prompt", "workspace-write", "never"},
		{"full_access", "danger-full-access", "never"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			cfg, err := parseConfig(map[string]any{"sandbox": "read-only", "permission_profile": tc.profile})
			if err != nil {
				t.Fatal(err)
			}
			profile, sandbox, approval, err := nativePermissions(cfg, "")
			if err != nil || profile != tc.profile || sandbox != tc.sandbox || approval != tc.approval {
				t.Fatalf("effective = %s %s %s %v", profile, sandbox, approval, err)
			}
			if cfg.Sandbox != "read-only" {
				t.Fatal("native profile changed the batch sandbox")
			}
		})
	}
	if _, err := parseConfig(map[string]any{"permission_profile": "invented"}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func TestNativeCodexTurnPermissionsAndExtendedApprovalWire(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir := t.TempDir()
	fake := newFakeCodex()
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: fake})
	created, err := provider.CreateSession(ctx, execution.CreateSessionRequest{ConversationID: "permissions", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provider.ReleaseSession(ctx, execution.SessionRequest{Session: created.Session}) }()
	for i, profile := range []string{"invented", "full_access", "workspace_no_prompt"} {
		started, err := provider.StartTurn(ctx, execution.StartTurnRequest{Session: created.Session, TurnID: profile, SubmissionID: profile, Message: "work", PermissionProfile: profile})
		if i == 0 {
			if err == nil || started.Delivery.State != execution.DeliveryUnsent || countMethod(fake.methodsCalled(), methodTurnStart) != 0 {
				t.Fatal("unknown profile submitted a turn")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		<-fake.turnStarted
		turnParams := fake.allParamsOf(methodTurnStart)
		params := turnParams[len(turnParams)-1]
		policy := params["sandboxPolicy"].(map[string]any)
		if params["approvalPolicy"] != "never" || started.Turn.Applied.PermissionProfile != profile {
			t.Fatalf("turn policy = %#v, applied = %#v", params, started.Turn.Applied)
		}
		if profile == "full_access" && policy["type"] != "dangerFullAccess" {
			t.Fatalf("full policy = %#v", policy)
		}
		if profile == "workspace_no_prompt" && (policy["type"] != "workspaceWrite" || policy["networkAccess"] != false) {
			t.Fatalf("bounded policy = %#v", policy)
		}
		if profile == "workspace_no_prompt" {
			fake.request(70, "item/commandExecution/requestApproval", `{"availableDecisions":["acceptForSession","decline"]}`)
			waitForCodexSession(t, provider, created.Session, func(s execution.SessionSnapshot) bool { return len(s.PendingApprovals) == 1 })
			request := execution.SessionCommandRequest{Session: created.Session, TurnID: profile, SubmissionID: "answer", InteractionID: "70", OptionID: "allow_rule"}
			if delivery, err := provider.RespondSessionApproval(ctx, request); err == nil || delivery.State != execution.DeliveryUnsent {
				t.Fatal("unoffered rule accepted")
			}
			request.OptionID = "allow_session"
			if _, err := provider.RespondSessionApproval(ctx, request); err != nil {
				t.Fatal(err)
			}
			assertFakeResponse(t, fake, "70", `{"decision":"acceptForSession"}`)
			fake.request(71, "item/commandExecution/requestApproval", `{"proposedExecpolicyAmendment":["go","test"]}`)
			waitForCodexSession(t, provider, created.Session, func(s execution.SessionSnapshot) bool { return len(s.PendingApprovals) == 1 })
			request.InteractionID, request.OptionID = "71", "allow_rule"
			if _, err := provider.RespondSessionApproval(ctx, request); err != nil {
				t.Fatal(err)
			}
			assertFakeResponse(t, fake, "71", `{"decision":{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["go","test"]}}}`)
		}
		fake.completeTurn()
		waitForCodexSession(t, provider, created.Session, func(s execution.SessionSnapshot) bool { return s.Work == execution.SessionIdle })
	}
}

func TestApprovalChoicesRespectNativeRestrictions(t *testing.T) {
	for _, tc := range []struct {
		method, params string
		want           []string
	}{
		{"item/commandExecution/requestApproval", `{"availableDecisions":["decline"],"proposedExecpolicyAmendment":["go","test"]}`, []string{"deny"}},
		{"item/commandExecution/requestApproval", `{"availableDecisions":[]}`, []string{}},
		{"item/fileChange/requestApproval", `{"availableDecisions":["accept","acceptForSession","decline"]}`, []string{"allow", "allow_session", "deny"}},
		{"item/fileChange/requestApproval", `{"availableDecisions":[{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["sh"]}}]}`, []string{}},
	} {
		options := approvalOptions(rpcMessage{Method: tc.method, Params: json.RawMessage(tc.params)})
		ids := []string{}
		for _, option := range options {
			ids = append(ids, option.ID)
		}
		if !reflect.DeepEqual(ids, tc.want) {
			t.Fatalf("%s: choices = %v, want %v", tc.params, ids, tc.want)
		}
	}
}
