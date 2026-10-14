// Package conformance exercises the same batch workflow contract through each
// real local adapter. Only the process/protocol peer is replaced by a fake.
package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type Harness interface {
	Provider() execution.Provider
	Request() execution.Request
	Messages() []string
	Complete(string, bool)
}

type Factory func(*testing.T) Harness

var actions = []execution.ActionID{execution.ActionPlan, execution.ActionImplement, execution.ActionReview, execution.ActionInception, execution.ActionBacklog, execution.ActionSpecDraft}

func Workspace(t *testing.T, skillsDir string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"plan", "implement", "review", "inception", "spec"} {
		path := filepath.Join(dir, skillsDir, "archetipo-"+name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture skill"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func Wait(t *testing.T, expected string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for " + expected)
}

type outcome struct {
	result execution.Result
	err    error
}

func start(t *testing.T, h Harness, action execution.ActionID) (<-chan outcome, execution.Request, execution.RunCollaborator) {
	t.Helper()
	req := h.Request()
	req.Action = action
	req.Capability, _ = execution.RequiredCapability(action)
	req.ExecutionID = "parity-run"
	req.ProviderConfig["timeout_seconds"] = 5
	if scope, _ := execution.ActionScope(action); scope == execution.ScopeSpec {
		req.SpecCode = "US-001"
	}
	collaborator, ok := execution.RunCollaboratorFor(h.Provider())
	if !ok {
		t.Fatal("provider has no run dialogue")
	}
	done := make(chan outcome, 1)
	go func() { result, err := h.Provider().Execute(context.Background(), req); done <- outcome{result, err} }()
	Wait(t, "opening prompt", func() bool { return len(h.Messages()) == 1 })
	return done, req, collaborator
}

func finish(t *testing.T, done <-chan outcome) outcome {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(4 * time.Second):
		t.Fatal("action did not settle")
		return outcome{}
	}
}

func receipt(action execution.ActionID) string {
	switch action {
	case execution.ActionPlan:
		return `{"spec_code":"US-001","status":"PLANNED","tasks":2}`
	case execution.ActionImplement:
		return `{"spec_code":"US-001","status":"REVIEW","tasks_done":2,"tests":"passed"}`
	case execution.ActionReview:
		return `{"spec_code":"US-001","status":"REVIEW","criteria":2,"blockers":0}`
	case execution.ActionInception:
		return `{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`
	case execution.ActionBacklog:
		return `{"artifact":"backlog","status":"WRITTEN","epics":1,"specs":2}`
	case execution.ActionSpecDraft:
		return `{"artifact":"spec_draft","status":"PROPOSED","title":"First story","epic_code":"","epic_title":"First goal","priority":"MEDIUM","points":3,"body":"Story with acceptance criteria"}`
	}
	panic("unknown test action")
}

func waitTurnEnd(t *testing.T, run execution.RunCollaborator, req execution.Request) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	found := errors.New("turn observed")
	err := run.StreamRunEvents(ctx, execution.RunRequest{RunID: req.ExecutionID}, 0, func(event execution.RunEvent) error {
		if event.Kind == localrun.KindTurnEnd {
			return found
		}
		return nil
	})
	if !errors.Is(err, found) {
		t.Fatalf("turn completion not observed: %v", err)
	}
}

// Run requires the same capabilities and behaviour for all six actions:
// questions keep the action alive, answers reach the same process, only valid
// completed receipts succeed, and cancellation between turns remains possible.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			t.Run("question_answer_receipt", func(t *testing.T) {
				h := factory(t)
				capabilities, err := h.Provider().Capabilities(context.Background())
				capability, _ := execution.RequiredCapability(action)
				if err != nil || !execution.Supports(capabilities, capability) {
					t.Fatalf("capability %s missing: %v", capability, err)
				}
				done, req, run := start(t, h, action)
				h.Complete("Which behaviour do you need?", true)
				waitTurnEnd(t, run, req)
				select {
				case got := <-done:
					t.Fatalf("question closed action: %v", got.err)
				default:
				}
				if err := run.SendRunMessage(context.Background(), execution.RunRequest{RunID: req.ExecutionID}, "The first requested behaviour"); err != nil {
					t.Fatal(err)
				}
				Wait(t, "answer delivered", func() bool { return len(h.Messages()) == 2 })
				if got := h.Messages()[1]; got != "The first requested behaviour" {
					t.Fatalf("answer changed: %q", got)
				}
				const secret = "PROTOCOL-SENTINEL-OUTSIDE-RECEIPT"
				h.Complete(secret+"\n"+receipt(action), true)
				got := finish(t, done)
				if got.err != nil {
					t.Fatal(got.err)
				}
				if strings.Contains(string(got.result.Payload), secret) {
					t.Fatal("result leaked harness output")
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(got.result.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload["result_summary"]) == 0 {
					t.Fatal("result omitted receipt")
				}
				if action == execution.ActionSpecDraft {
					var proposal execution.SpecDraftProposal
					if err := json.Unmarshal(payload["spec_draft"], &proposal); err != nil || proposal.EpicTitle != "First goal" || proposal.EpicCode != "" || proposal.BlockedBy == nil {
						t.Fatalf("first proposal damaged: %+v %v", proposal, err)
					}
				}
				snapshot, err := run.ReadRun(context.Background(), execution.RunRequest{RunID: req.ExecutionID})
				if err != nil || snapshot.State != execution.RunClosed {
					t.Fatalf("batch lifecycle not closed: %+v %v", snapshot, err)
				}
			})
			for _, mode := range []string{"invalid_receipt", "interrupted_receipt", "cancel_between_turns"} {
				t.Run(mode, func(t *testing.T) {
					h := factory(t)
					done, req, run := start(t, h, action)
					message := receipt(action)
					if mode == "invalid_receipt" {
						var fields map[string]any
						_ = json.Unmarshal([]byte(message), &fields)
						fields["status"] = "WRONG"
						encoded, _ := json.Marshal(fields)
						message = string(encoded)
					}
					if mode == "cancel_between_turns" {
						message = "Waiting for your answer"
					}
					h.Complete(message, mode != "interrupted_receipt")
					if mode == "cancel_between_turns" {
						waitTurnEnd(t, run, req)
						if err := run.CancelRun(context.Background(), execution.RunRequest{RunID: req.ExecutionID}); err != nil {
							t.Fatal(err)
						}
					}
					got := finish(t, done)
					if got.err == nil || got.result.Payload != nil {
						t.Fatalf("%s accepted an unsuccessful action: %s %v", mode, got.result.Payload, got.err)
					}
				})
			}
		})
	}
}
