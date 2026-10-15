package claude

import (
	"context"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func TestNativePermissionProfileMappingDoesNotChangeBatchPolicy(t *testing.T) {
	for _, tc := range []struct{ profile, mode string }{{"provider_default", "plan"}, {"auto", "auto"}, {"accept_edits", "acceptEdits"}, {"full_access", "bypassPermissions"}} {
		cfg, err := parseConfig(map[string]any{"permission_mode": "plan", "permission_profile": tc.profile})
		if err != nil {
			t.Fatal(err)
		}
		_, mode, err := nativePermissionMode(cfg, "")
		if err != nil || mode != tc.mode || cfg.PermissionMode != "plan" {
			t.Fatalf("%s -> %s, %v", tc.profile, mode, err)
		}
	}
	if _, err := parseConfig(map[string]any{"permission_profile": "workspace"}); err == nil {
		t.Fatal("Claude advertised a workspace sandbox it cannot enforce")
	}
}

func TestClaudePermissionsApplyAtNextTurnWithoutChangingNativeID(t *testing.T) {
	dir := t.TempDir()
	first, elevated, restored := newFakeClaude(), newFakeClaude(), newFakeClaude()
	first.silent, elevated.silent, restored.silent = true, true, true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{first, elevated, restored}}, Now: time.Now})
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "permissions", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t), "permission_mode": "auto"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = provider.ReleaseSession(context.Background(), execution.SessionRequest{Session: created.Session})
	}()
	announceSessionOnFirstMessage(elevated, created.Session.Native.ID, "sonnet")
	started, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "elevated", SubmissionID: "s1", Message: "work", PermissionProfile: "full_access"})
	if err != nil {
		t.Fatal(err)
	}
	assertArgumentsContainInOrder(t, elevated, "--permission-mode", "bypassPermissions", "--session-id", created.Session.Native.ID)
	if started.Turn.Applied.PermissionProfile != "full_access" {
		t.Fatalf("applied = %#v", started.Turn.Applied)
	}
	elevated.emit(resultFrame("done", false))
	waitForNativeIdle(t, provider, created.Session)
	announceSessionOnFirstMessage(restored, created.Session.Native.ID, "sonnet")
	if _, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "restored", SubmissionID: "s2", Message: "again", PermissionProfile: "provider_default"}); err != nil {
		t.Fatal(err)
	}
	assertArgumentsContainInOrder(t, restored, "--permission-mode", "auto", "--resume", created.Session.Native.ID)
	restored.emit(resultFrame("done", false))
	waitForNativeIdle(t, provider, created.Session)
}
