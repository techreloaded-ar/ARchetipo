package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// stateReader is the configurable backlog double: it decides what a re-read of
// the spec finds, and counts the reads so a test can prove the function did not
// look at all.
type stateReader struct {
	spec     domain.Spec
	tasks    []domain.Task
	specErr  error
	tasksErr error
	reads    int
}

func (r *stateReader) ReadSpecDetail(context.Context, string) (domain.Spec, error) {
	r.reads++
	if r.specErr != nil {
		return domain.Spec{}, r.specErr
	}
	return r.spec, nil
}

func (r *stateReader) ReadSpecTasks(context.Context, string) ([]domain.Task, error) {
	r.reads++
	if r.tasksErr != nil {
		return nil, r.tasksErr
	}
	return r.tasks, nil
}

func succeededRecord() Execution {
	completed := time.Date(2026, 8, 17, 11, 0, 0, 0, time.UTC)
	return Execution{
		ID:               "exec-effect-1",
		SpecCode:         "US-001",
		Action:           ActionPlan,
		Capability:       CapabilitySpecPlan,
		ProviderID:       "fake",
		SpecStatusBefore: domain.StatusTodo,
		Status:           StatusSucceeded,
		Result:           &Result{Payload: json.RawMessage(`{"ok":true}`), ExternalID: "task-remote-7"},
		CreatedAt:        completed.Add(-time.Minute),
		CompletedAt:      &completed,
	}
}

func seededStore(t *testing.T, record Execution) *spyStore {
	t.Helper()
	return &spyStore{records: map[string]Execution{record.ID: record}}
}

// VerifyActionEffect is the same rule with the write left to the caller, so an
// asynchronous caller can fold the verdict into the single terminal write
// instead of publishing a success and taking it back. The rule must be
// identical and the store must stay untouched.
func TestVerifyActionEffectAppliesTheRuleWithoutPersistingIt(t *testing.T) {
	denying := &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusTodo}}
	record := succeededRecord()
	store := seededStore(t, record)

	err := VerifyActionEffect(context.Background(), denying, ActionPlan, "US-001", &record)
	var unconfirmed *UnconfirmedEffectError
	if !errors.As(err, &unconfirmed) || unconfirmed.ExecutionID != "exec-effect-1" {
		t.Fatalf("an unconfirmed success was accepted: %v", err)
	}
	if record.Status != StatusFailed || record.Result != nil || record.Error == nil ||
		record.Error.Code != "UNCONFIRMED_EFFECT" || record.Error.ExternalID != "task-remote-7" {
		t.Fatalf("the demotion differs from the confirmed one: %#v", record)
	}
	if store.updates != 0 || store.creates != 0 {
		t.Fatalf("the verdict wrote to the store: creates=%d updates=%d", store.creates, store.updates)
	}
	if persisted := store.records["exec-effect-1"]; persisted.Status != StatusSucceeded {
		t.Fatalf("the seeded record was touched: %#v", persisted)
	}

	// And it stays silent on everything it has no business judging.
	backed := &stateReader{
		spec:  domain.Spec{Code: "US-001", Status: domain.StatusPlanned},
		tasks: []domain.Task{{ID: "TASK-01", Title: "Do", Type: domain.TaskImpl, Status: domain.StatusTodo}},
	}
	confirmed := succeededRecord()
	if err := VerifyActionEffect(context.Background(), backed, ActionPlan, "US-001", &confirmed); err != nil {
		t.Fatalf("a confirmed plan was rejected: %v", err)
	}
	failed := succeededRecord()
	failed.Status = StatusFailed
	if err := VerifyActionEffect(context.Background(), denying, ActionPlan, "US-001", &failed); err != nil {
		t.Fatalf("a record that already failed was judged: %v", err)
	}
	if denying.reads != 1 {
		t.Fatalf("the connector was re-read for a record with no claim to check: %d", denying.reads)
	}
}

func TestConfirmActionEffectAcceptsAConfirmedPlan(t *testing.T) {
	record := succeededRecord()
	store := seededStore(t, record)
	reader := &stateReader{
		spec:  domain.Spec{Code: "US-001", Status: domain.StatusPlanned},
		tasks: []domain.Task{{ID: "TASK-01", Title: "Do", Type: domain.TaskImpl, Status: domain.StatusTodo}},
	}
	if err := ConfirmActionEffect(context.Background(), reader, store, ActionPlan, "US-001", &record); err != nil {
		t.Fatalf("a confirmed plan was rejected: %v", err)
	}
	if record.Status != StatusSucceeded || record.Result == nil || record.Error != nil {
		t.Fatalf("a confirmed record was rewritten: %#v", record)
	}
	if store.updates != 0 {
		t.Fatalf("a confirmed record was persisted again: updates=%d", store.updates)
	}
}

// AC-4: a success the connector denies is not a success. The record becomes
// FAILED with a readable reason, and the remote identifier survives the move.
func TestConfirmActionEffectDemotesAnUnconfirmedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reader      *stateReader
		wantMessage string
	}{
		{
			name:        "the spec never left TODO",
			reader:      &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusTodo}},
			wantMessage: "US-001 is TODO, not PLANNED",
		},
		{
			name:        "the spec is PLANNED but holds no task",
			reader:      &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusPlanned}},
			wantMessage: "US-001 is PLANNED but holds no plan task",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := succeededRecord()
			store := seededStore(t, record)
			err := ConfirmActionEffect(context.Background(), tc.reader, store, ActionPlan, "US-001", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("an unconfirmed success passed: %v", err)
			}
			if unconfirmed.ExecutionID != record.ID || !strings.Contains(unconfirmed.Error(), record.ID) {
				t.Fatalf("the error does not name the execution: %v", err)
			}
			if !strings.Contains(unconfirmed.Message, tc.wantMessage) {
				t.Fatalf("the reason is unreadable: %q", unconfirmed.Message)
			}
			if record.Status != StatusFailed || record.Result != nil || record.Error == nil {
				t.Fatalf("the demoted record: %#v", record)
			}
			if record.Error.Code != "UNCONFIRMED_EFFECT" || record.Error.ExternalID != "task-remote-7" {
				t.Fatalf("the demoted record lost code or remote identifier: %#v", record.Error)
			}
			persisted, err := store.Get(context.Background(), record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.Status != StatusFailed || persisted.Result != nil || persisted.Error == nil || persisted.Error.Code != "UNCONFIRMED_EFFECT" {
				t.Fatalf("the persisted record was not demoted: %#v", persisted)
			}
		})
	}
}

// Nothing to confirm means nothing to read: a record that already failed, or an
// action whose effect this function does not know, is left alone.
func TestConfirmActionEffectLeavesUnrelatedRecordsAlone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action ActionID
		status ExecutionStatus
	}{
		{"already failed", ActionPlan, StatusFailed},
		{"still running", ActionPlan, StatusRunning},
		{"another action", "implement", StatusSucceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := succeededRecord()
			record.Status = tc.status
			store := seededStore(t, record)
			reader := &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusTodo}}
			if err := ConfirmActionEffect(context.Background(), reader, store, tc.action, "US-001", &record); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if reader.reads != 0 {
				t.Fatalf("the connector was read for nothing: reads=%d", reader.reads)
			}
			if record.Status != tc.status || store.updates != 0 {
				t.Fatalf("the record was touched: %#v updates=%d", record, store.updates)
			}
		})
	}
}

// A re-read that fails is not a confirmation: swallowing it would let an
// unverifiable claim through as a success.
func TestConfirmActionEffectTreatsAFailedRereadAsUnconfirmed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader *stateReader
	}{
		{"the spec cannot be re-read", &stateReader{specErr: errors.New("backlog unavailable")}},
		{"the plan tasks cannot be read", &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusPlanned}, tasksErr: errors.New("plan unreadable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := succeededRecord()
			store := seededStore(t, record)
			err := ConfirmActionEffect(context.Background(), tc.reader, store, ActionPlan, "US-001", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("a failed re-read was swallowed: %v", err)
			}
			if record.Status != StatusFailed || record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
				t.Fatalf("the record stayed confirmed: %#v", record)
			}
		})
	}
}

// The demotion has to be recorded; a store that cannot write it is an internal
// failure, not an unconfirmed effect the caller should render as a precondition.
func TestConfirmActionEffectReportsAFailedDemotionWrite(t *testing.T) {
	record := succeededRecord()
	store := seededStore(t, record)
	store.updateErr = errors.New("disk full")
	reader := &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusTodo}}
	err := ConfirmActionEffect(context.Background(), reader, store, ActionPlan, "US-001", &record)
	if err == nil {
		t.Fatal("a lost demotion was reported as success")
	}
	var unconfirmed *UnconfirmedEffectError
	if errors.As(err, &unconfirmed) {
		t.Fatalf("a store failure was rendered as an unconfirmed effect: %v", err)
	}
	if !strings.Contains(err.Error(), record.ID) {
		t.Fatalf("the error does not name the execution: %v", err)
	}
}
