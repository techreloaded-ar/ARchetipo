package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
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
		{"another action", "deploy", StatusSucceeded},
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

// prdReader is the workspace double for an inception: it decides what a re-read
// of the PRD finds. It also satisfies SpecStateReader because VerifyActionEffect
// takes one reader for every action and narrows it at the only point that knows
// which action is being verified.
type prdReader struct {
	stateReader
	body   string
	prdErr error
	prdRe  int
}

func (r *prdReader) ReadPRD(context.Context) (string, error) {
	r.prdRe++
	if r.prdErr != nil {
		return "", r.prdErr
	}
	return r.body, nil
}

func inceptionRecord() Execution {
	record := succeededRecord()
	record.ID = "exec-inception-1"
	record.SpecCode = ""
	record.Action = ActionInception
	record.Capability = CapabilityWorkspaceInception
	return record
}

// AC-3, negative side: a success is a success only when the PRD can be read
// back from the configured path. The receipt is the agent's word; this is the
// workspace's.
func TestVerifyActionEffectConfirmsAnInceptionOnlyWhenThePRDIsReadable(t *testing.T) {
	confirmed := inceptionRecord()
	reader := &prdReader{body: "# PRD\n\nVisione del prodotto.\n"}
	if err := VerifyActionEffect(context.Background(), reader, ActionInception, "", &confirmed); err != nil {
		t.Fatalf("a confirmed inception was rejected: %v", err)
	}
	if confirmed.Status != StatusSucceeded || confirmed.Result == nil || confirmed.Error != nil {
		t.Fatalf("a confirmed record was rewritten: %#v", confirmed)
	}
	if reader.prdRe != 1 {
		t.Fatalf("the PRD was read %d times", reader.prdRe)
	}
}

func TestVerifyActionEffectDemotesAnInceptionWithoutAPersistedPRD(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reader      SpecStateReader
		wantMessage string
	}{
		{
			name:        "no PRD at the configured path",
			reader:      &prdReader{body: ""},
			wantMessage: "no PRD was persisted at the configured path",
		},
		{
			name:        "a PRD made of whitespace only",
			reader:      &prdReader{body: "   \n\t\n"},
			wantMessage: "no PRD was persisted at the configured path",
		},
		{
			name:        "the PRD cannot be re-read",
			reader:      &prdReader{prdErr: errors.New("workspace unavailable")},
			wantMessage: "workspace unavailable",
		},
		{
			name:        "the connector cannot read a PRD at all",
			reader:      &stateReader{},
			wantMessage: "the connector cannot read the PRD back",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := inceptionRecord()
			err := VerifyActionEffect(context.Background(), tc.reader, ActionInception, "", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("an unconfirmed inception passed: %v", err)
			}
			if unconfirmed.ExecutionID != record.ID {
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
		})
	}
}

// prdDiscarder counts what it was asked to do, so a test can prove the rollback
// did not run at all.
type prdDiscarder struct {
	removed bool
	err     error
	calls   int
}

func (d *prdDiscarder) DiscardPRD(context.Context) (bool, error) {
	d.calls++
	return d.removed, d.err
}

func failedInceptionRecord(message string) Execution {
	record := inceptionRecord()
	record.Status = StatusFailed
	record.Result = nil
	record.Error = &ExecutionError{Code: "PROVIDER_FAILED", Message: message}
	return record
}

// AC-4: a PRD born inside a run that ended badly is taken back, and the note
// about the removal never replaces the reason the run failed.
func TestDiscardPartialPRDRemovesADocumentBornInsideAFailedRun(t *testing.T) {
	discarder := &prdDiscarder{removed: true}
	record := failedInceptionRecord("the agent was interrupted")

	DiscardPartialPRD(context.Background(), discarder, false, &record)

	if discarder.calls != 1 {
		t.Fatalf("the discarder was called %d times", discarder.calls)
	}
	if !strings.Contains(record.Error.Message, "the agent was interrupted") {
		t.Fatalf("the original reason was lost: %q", record.Error.Message)
	}
	if !strings.Contains(record.Error.Message, "removed") {
		t.Fatalf("the removal is not reported: %q", record.Error.Message)
	}
}

// A discarder that finds nothing to remove has nothing to report either: the
// record keeps exactly the reason it failed for.
func TestDiscardPartialPRDStaysSilentWhenThereWasNothingToRemove(t *testing.T) {
	discarder := &prdDiscarder{removed: false}
	record := failedInceptionRecord("the agent was interrupted")

	DiscardPartialPRD(context.Background(), discarder, false, &record)

	if discarder.calls != 1 {
		t.Fatalf("the discarder was called %d times", discarder.calls)
	}
	if record.Error.Message != "the agent was interrupted" {
		t.Fatalf("the message was touched: %q", record.Error.Message)
	}
}

// A rollback that cannot be performed is worth recording, but it never hides
// why the run failed in the first place.
func TestDiscardPartialPRDReportsAFailedRemovalWithoutHidingTheCause(t *testing.T) {
	discarder := &prdDiscarder{err: errors.New("permission denied")}
	record := failedInceptionRecord("the agent was interrupted")

	DiscardPartialPRD(context.Background(), discarder, false, &record)

	if !strings.Contains(record.Error.Message, "the agent was interrupted") ||
		!strings.Contains(record.Error.Message, "permission denied") {
		t.Fatalf("the message lost one of its two halves: %q", record.Error.Message)
	}
}

// AC-5, first half: a PRD the workspace already had belongs to the workspace,
// not to this run, and a succeeded run is precisely the one whose document must
// stay. Neither case may reach the discarder at all.
func TestDiscardPartialPRDNeverRemovesADocumentItDoesNotOwn(t *testing.T) {
	for _, tc := range []struct {
		name         string
		existedBefor bool
		status       ExecutionStatus
	}{
		{"a PRD that predates the run", true, StatusFailed},
		{"a run that succeeded", false, StatusSucceeded},
		{"a run that succeeded over a pre-existing PRD", true, StatusSucceeded},
		{"a run still going", false, StatusRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discarder := &prdDiscarder{removed: true}
			record := failedInceptionRecord("the agent was interrupted")
			record.Status = tc.status

			DiscardPartialPRD(context.Background(), discarder, tc.existedBefor, &record)

			if discarder.calls != 0 {
				t.Fatalf("the discarder was called %d times", discarder.calls)
			}
			if record.Error.Message != "the agent was interrupted" {
				t.Fatalf("the message was touched: %q", record.Error.Message)
			}
		})
	}
}

// A connector without the rollback capability is not itself a failure: skipping
// the rollback must leave the record exactly as it was.
func TestDiscardPartialPRDToleratesAConnectorWithoutADiscarder(t *testing.T) {
	record := failedInceptionRecord("the agent was interrupted")
	DiscardPartialPRD(context.Background(), nil, false, &record)
	if record.Error.Message != "the agent was interrupted" {
		t.Fatalf("the message was touched: %q", record.Error.Message)
	}
	DiscardPartialPRD(context.Background(), &prdDiscarder{removed: true}, false, nil)
}

// backlogReader is the workspace double for a backlog generation: it decides
// what a re-read of the backlog finds. It embeds stateReader for the same
// reason prdReader does — VerifyActionEffect takes one reader for every action
// and narrows it at the only point that knows which action is being verified.
type backlogReader struct {
	stateReader
	summary    domain.BacklogSummary
	backlogErr error
	backlogRe  int
}

func (r *backlogReader) ReadExistingBacklog(context.Context) (domain.BacklogSummary, error) {
	r.backlogRe++
	if r.backlogErr != nil {
		return domain.BacklogSummary{}, r.backlogErr
	}
	return r.summary, nil
}

func backlogRecord() Execution {
	record := succeededRecord()
	record.ID = "exec-backlog-1"
	record.SpecCode = ""
	record.Action = ActionBacklog
	record.Capability = CapabilityWorkspaceBacklog
	return record
}

func generatedBacklog() domain.BacklogSummary {
	return domain.BacklogSummary{
		Codes:    []string{"US-001", "US-002"},
		Titles:   []string{"Prima storia", "Seconda storia"},
		Epics:    []domain.Epic{{Code: "EP-001", Title: "Prima epica"}},
		LastCode: "US-002",
	}
}

// AC-2, positive side: a backlog the connector really holds is confirmed, and
// the confirmation does not touch the record at all.
func TestVerifyActionEffectConfirmsABacklogTheConnectorHolds(t *testing.T) {
	record := backlogRecord()
	reader := &backlogReader{summary: generatedBacklog()}

	if err := VerifyActionEffect(context.Background(), reader, ActionBacklog, "", &record); err != nil {
		t.Fatalf("a confirmed backlog was rejected: %v", err)
	}
	if record.Status != StatusSucceeded || record.Result == nil || record.Error != nil {
		t.Fatalf("a confirmed record was rewritten: %#v", record)
	}
	if record.Result.ExternalID != "task-remote-7" {
		t.Fatalf("the confirmed record lost its remote identifier: %#v", record.Result)
	}
	if reader.backlogRe != 1 {
		t.Fatalf("the backlog was read %d times", reader.backlogRe)
	}
}

// AC-2, negative side: a success the workspace does not back is not a success.
// The record becomes FAILED with UNCONFIRMED_EFFECT, a readable reason, and the
// remote identifier moved into the error rather than lost with the result.
func TestVerifyActionEffectDemotesABacklogTheConnectorDoesNotBack(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reader      SpecStateReader
		wantMessage string
	}{
		{
			name:        "no spec was persisted",
			reader:      &backlogReader{summary: domain.BacklogSummary{}},
			wantMessage: "no backlog was persisted",
		},
		{
			name: "the connector reports there is no backlog at all",
			reader: &backlogReader{backlogErr: iox.NewPrecondition(
				"backlog not found at .archetipo/backlog.yaml",
				"run `archetipo spec add` or `archetipo-spec` first",
				errors.New("backlog missing"),
			)},
			wantMessage: "no backlog was persisted",
		},
		{
			name: "specs without any epic",
			reader: &backlogReader{summary: domain.BacklogSummary{
				Codes:  []string{"US-001", "US-002"},
				Titles: []string{"Prima storia", "Seconda storia"},
			}},
			wantMessage: "the backlog holds 2 spec(s) but no epic",
		},
		{
			name:        "the backlog cannot be re-read",
			reader:      &backlogReader{backlogErr: errors.New("workspace unavailable")},
			wantMessage: "re-reading the backlog failed: workspace unavailable",
		},
		{
			name:        "the connector cannot read a backlog at all",
			reader:      &stateReader{},
			wantMessage: "the connector cannot read the backlog back",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := backlogRecord()
			err := VerifyActionEffect(context.Background(), tc.reader, ActionBacklog, "", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("an unconfirmed backlog passed: %v", err)
			}
			if unconfirmed.ExecutionID != record.ID {
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
			if !strings.Contains(record.Error.Message, tc.wantMessage) {
				t.Fatalf("the record does not carry the reason: %q", record.Error.Message)
			}
		})
	}
}

// backlogDiscarder counts what it was asked to do, so a test can prove the
// rollback did not run at all.
type backlogDiscarder struct {
	removed bool
	err     error
	calls   int
}

func (d *backlogDiscarder) DiscardBacklog(context.Context) (bool, error) {
	d.calls++
	return d.removed, d.err
}

func failedBacklogRecord(message string) Execution {
	record := backlogRecord()
	record.Status = StatusFailed
	record.Result = nil
	record.Error = &ExecutionError{Code: "PROVIDER_FAILED", Message: message}
	return record
}

// AC-4: a backlog born inside a run that ended badly is taken back, and the
// note about the removal is appended after the reason the run failed, never in
// its place.
func TestDiscardPartialBacklogRemovesABacklogBornInsideAFailedRun(t *testing.T) {
	discarder := &backlogDiscarder{removed: true}
	record := failedBacklogRecord("the agent was interrupted")

	DiscardPartialBacklog(context.Background(), discarder, false, &record)

	if discarder.calls != 1 {
		t.Fatalf("the discarder was called %d times", discarder.calls)
	}
	if record.Error.Message != "the agent was interrupted; the partial backlog written by this run has been removed" {
		t.Fatalf("the note did not land after the original reason: %q", record.Error.Message)
	}
}

// A discarder that finds nothing to remove has nothing to report either.
func TestDiscardPartialBacklogStaysSilentWhenThereWasNothingToRemove(t *testing.T) {
	discarder := &backlogDiscarder{removed: false}
	record := failedBacklogRecord("the agent was interrupted")

	DiscardPartialBacklog(context.Background(), discarder, false, &record)

	if discarder.calls != 1 {
		t.Fatalf("the discarder was called %d times", discarder.calls)
	}
	if record.Error.Message != "the agent was interrupted" {
		t.Fatalf("the message was touched: %q", record.Error.Message)
	}
}

// A rollback that cannot be performed is worth recording, but it never hides
// why the run failed in the first place.
func TestDiscardPartialBacklogReportsAFailedRemovalWithoutHidingTheCause(t *testing.T) {
	discarder := &backlogDiscarder{err: errors.New("permission denied")}
	record := failedBacklogRecord("the agent was interrupted")

	DiscardPartialBacklog(context.Background(), discarder, false, &record)

	if !strings.Contains(record.Error.Message, "the agent was interrupted") ||
		!strings.Contains(record.Error.Message, "the partial backlog could not be removed: permission denied") {
		t.Fatalf("the message lost one of its two halves: %q", record.Error.Message)
	}
}

// AC-4, the other three combinations of existedBefore x terminal status: a
// backlog the workspace already had belongs to the workspace, and a run that
// succeeded is precisely the one whose backlog must stay. Neither may reach the
// discarder at all.
func TestDiscardPartialBacklogNeverRemovesABacklogItDoesNotOwn(t *testing.T) {
	for _, tc := range []struct {
		name          string
		existedBefore bool
		status        ExecutionStatus
	}{
		{"a backlog that predates the run", true, StatusFailed},
		{"a run that succeeded", false, StatusSucceeded},
		{"a run that succeeded over a pre-existing backlog", true, StatusSucceeded},
		{"a run still going", false, StatusRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discarder := &backlogDiscarder{removed: true}
			record := failedBacklogRecord("the agent was interrupted")
			record.Status = tc.status

			DiscardPartialBacklog(context.Background(), discarder, tc.existedBefore, &record)

			if discarder.calls != 0 {
				t.Fatalf("the discarder was called %d times", discarder.calls)
			}
			if record.Error.Message != "the agent was interrupted" {
				t.Fatalf("the message was touched: %q", record.Error.Message)
			}
		})
	}
}

// A connector without the rollback capability is not itself a failure: skipping
// the rollback must leave the record exactly as it was, and must not panic.
func TestDiscardPartialBacklogToleratesAConnectorWithoutADiscarder(t *testing.T) {
	record := failedBacklogRecord("the agent was interrupted")
	DiscardPartialBacklog(context.Background(), nil, false, &record)
	if record.Error.Message != "the agent was interrupted" {
		t.Fatalf("the message was touched: %q", record.Error.Message)
	}
	DiscardPartialBacklog(context.Background(), &backlogDiscarder{removed: true}, false, nil)
}

// specMover is the configurable transition double: it records what it was asked
// to write, so a test can prove both that a transition happened and that it did
// not.
type specMover struct {
	calls  int
	code   string
	status domain.Status
	err    error
}

func (m *specMover) TransitionStatus(_ context.Context, specRef string, newStatus domain.Status) (domain.WriteResult, error) {
	m.calls++
	m.code = specRef
	m.status = newStatus
	if m.err != nil {
		return domain.WriteResult{}, m.err
	}
	return domain.WriteResult{OK: true}, nil
}

func TestHasPersistedPlanReportsWhetherThereIsAPlanToCarryOut(t *testing.T) {
	t.Run("a spec with tasks has a plan", func(t *testing.T) {
		reader := &stateReader{tasks: []domain.Task{{ID: "TASK-01", Status: domain.StatusTodo}}}
		got, err := HasPersistedPlan(context.Background(), reader, "US-001")
		if err != nil || !got {
			t.Fatalf("HasPersistedPlan = %v, %v; want true, nil", got, err)
		}
	})
	t.Run("a spec without tasks has none", func(t *testing.T) {
		reader := &stateReader{}
		got, err := HasPersistedPlan(context.Background(), reader, "US-001")
		if err != nil || got {
			t.Fatalf("HasPersistedPlan = %v, %v; want false, nil", got, err)
		}
	})
	// "There is nothing here" is an answer, not an infrastructure failure.
	t.Run("a missing precondition means no plan", func(t *testing.T) {
		reader := &stateReader{tasksErr: iox.NewPrecondition("no plan yet", "", nil)}
		got, err := HasPersistedPlan(context.Background(), reader, "US-001")
		if err != nil || got {
			t.Fatalf("HasPersistedPlan = %v, %v; want false, nil", got, err)
		}
	})
	t.Run("a real read failure is reported", func(t *testing.T) {
		reader := &stateReader{tasksErr: errors.New("disk on fire")}
		if _, err := HasPersistedPlan(context.Background(), reader, "US-001"); err == nil {
			t.Fatal("a failed read was reported as an answer")
		}
	})
}

// AC-3: an accepted start of an implementation moves the spec to IN PROGRESS
// before anything is dispatched, and pressing the action again changes nothing.
func TestBeginActionEffectMovesAnImplementationToInProgress(t *testing.T) {
	t.Run("a planned spec is moved", func(t *testing.T) {
		mover := &specMover{}
		spec := domain.Spec{Code: "US-001", Status: domain.StatusPlanned}
		if err := BeginActionEffect(context.Background(), mover, ActionImplement, spec); err != nil {
			t.Fatal(err)
		}
		if mover.calls != 1 || mover.code != "US-001" || mover.status != domain.StatusInProgress {
			t.Fatalf("transition calls=%d code=%q status=%q", mover.calls, mover.code, mover.status)
		}
	})
	t.Run("a spec already in progress is left alone", func(t *testing.T) {
		mover := &specMover{}
		spec := domain.Spec{Code: "US-001", Status: domain.StatusInProgress}
		if err := BeginActionEffect(context.Background(), mover, ActionImplement, spec); err != nil {
			t.Fatal(err)
		}
		if mover.calls != 0 {
			t.Fatalf("an idempotent start wrote %d time(s)", mover.calls)
		}
	})
	// The start effect only moves a spec forward. A replayed low-level start on
	// a spec that already reached review must never drag it back.
	t.Run("no other status is moved", func(t *testing.T) {
		for _, status := range []domain.Status{domain.StatusTodo, domain.StatusReview, domain.StatusDone} {
			mover := &specMover{}
			spec := domain.Spec{Code: "US-001", Status: status}
			if err := BeginActionEffect(context.Background(), mover, ActionImplement, spec); err != nil {
				t.Fatal(err)
			}
			if mover.calls != 0 {
				t.Fatalf("a spec in %s was moved %d time(s)", status, mover.calls)
			}
		}
	})
	// US-035 AC-2: asking a provider to prepare a review must not move the spec
	// even by a column, or the request itself would already be a step towards
	// closing it.
	t.Run("preparing a review moves nothing", func(t *testing.T) {
		mover := &specMover{}
		for _, status := range []domain.Status{domain.StatusReview, domain.StatusPlanned, domain.StatusInProgress} {
			spec := domain.Spec{Code: "US-001", Status: status}
			if err := BeginActionEffect(context.Background(), mover, ActionReview, spec); err != nil {
				t.Fatal(err)
			}
		}
		if mover.calls != 0 {
			t.Fatalf("starting a review wrote %d time(s)", mover.calls)
		}
	})
	t.Run("another action owes the backlog nothing", func(t *testing.T) {
		mover := &specMover{}
		spec := domain.Spec{Code: "US-001", Status: domain.StatusTodo}
		for _, action := range []ActionID{ActionPlan, ActionInception, ActionBacklog} {
			if err := BeginActionEffect(context.Background(), mover, action, spec); err != nil {
				t.Fatal(err)
			}
		}
		if mover.calls != 0 {
			t.Fatalf("action without a start effect wrote %d time(s)", mover.calls)
		}
	})
	t.Run("a failed transition is reported and names the spec", func(t *testing.T) {
		mover := &specMover{err: errors.New("backlog locked")}
		spec := domain.Spec{Code: "US-001", Status: domain.StatusPlanned}
		err := BeginActionEffect(context.Background(), mover, ActionImplement, spec)
		if err == nil || !strings.Contains(err.Error(), "US-001") || !strings.Contains(err.Error(), "backlog locked") {
			t.Fatalf("error = %v, want one naming the spec and the cause", err)
		}
	})
}

// AC-4: a success is confirmed only by a spec that really reached REVIEW with
// its plan carried out.
func TestVerifyActionEffectConfirmsAnImplementationOnlyWhenThePlanIsCarriedOut(t *testing.T) {
	t.Run("a reviewed spec with a completed plan is a success", func(t *testing.T) {
		reader := &stateReader{
			spec: domain.Spec{Code: "US-001", Status: domain.StatusReview},
			tasks: []domain.Task{
				{ID: "TASK-01", Status: domain.StatusDone},
				{ID: "TASK-02", Status: domain.StatusDone},
			},
		}
		record := succeededRecord()
		record.Action = ActionImplement
		if err := VerifyActionEffect(context.Background(), reader, ActionImplement, "US-001", &record); err != nil {
			t.Fatalf("a carried-out plan was refused: %v", err)
		}
		if record.Status != StatusSucceeded {
			t.Fatalf("the record was demoted: %#v", record)
		}
	})

	// AC-5: every way of claiming a success the connector denies ends as a
	// FAILED record carrying the reason.
	for _, tc := range []struct {
		name   string
		reader *stateReader
		want   string
	}{
		{
			name:   "the spec never reached review",
			reader: &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusInProgress}, tasks: []domain.Task{{ID: "TASK-01", Status: domain.StatusDone}}},
			want:   "US-001 is IN PROGRESS, not REVIEW",
		},
		{
			name:   "there is no plan at all",
			reader: &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
			want:   "holds no plan task",
		},
		{
			name: "a task of the plan is still open",
			reader: &stateReader{
				spec: domain.Spec{Code: "US-001", Status: domain.StatusReview},
				tasks: []domain.Task{
					{ID: "TASK-01", Status: domain.StatusDone},
					{ID: "TASK-07", Status: domain.StatusTodo},
				},
			},
			want: "TASK-07",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := succeededRecord()
			record.Action = ActionImplement
			err := VerifyActionEffect(context.Background(), tc.reader, ActionImplement, "US-001", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("error = %v, want *UnconfirmedEffectError", err)
			}
			if record.Status != StatusFailed || record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
				t.Fatalf("the record was not demoted: %#v", record)
			}
			if !strings.Contains(record.Error.Message, tc.want) {
				t.Fatalf("message = %q, want it to mention %q", record.Error.Message, tc.want)
			}
		})
	}
}

// reviewStateReader is a stateReader that can also answer with a review
// artifact: the shape reviewEffect narrows the reader to at the only point that
// knows the action.
type reviewStateReader struct {
	stateReader
	review    domain.Review
	reviewErr error
}

func (r *reviewStateReader) ReadReview(context.Context, string) (domain.Review, error) {
	if r.reviewErr != nil {
		return domain.Review{}, r.reviewErr
	}
	return r.review, nil
}

func preparedDossier(executionID string) domain.Review {
	return domain.Review{
		Dossier: &domain.ReviewDossier{
			ExecutionID: executionID,
			Summary:     "the increment adds the greeting endpoint",
			Criteria:    []domain.ReviewCriterion{{ID: "AC-1", Verdict: domain.ReviewCriterionMet}},
		},
	}
}

// US-035 AC-1: a review that really left evidence behind, with the spec exactly
// where it was, is the one shape that counts as a success.
func TestVerifyActionEffectConfirmsAReviewThatPreparedEvidenceAndMovedNothing(t *testing.T) {
	reader := &reviewStateReader{
		stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
		review:      preparedDossier("exec-effect-1"),
	}
	record := succeededRecord()
	record.Action = ActionReview
	if err := VerifyActionEffect(context.Background(), reader, ActionReview, "US-001", &record); err != nil {
		t.Fatalf("a prepared review was rejected: %v", err)
	}
	if record.Status != StatusSucceeded {
		t.Fatalf("the record was demoted: %#v", record)
	}
}

// A dossier whose ExecutionID is empty was legitimately prepared by hand, and
// tolerating it is what keeps the CLI command usable outside a run.
func TestVerifyActionEffectAcceptsADossierWithoutAnExecutionID(t *testing.T) {
	reader := &reviewStateReader{
		stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
		review:      preparedDossier(""),
	}
	record := succeededRecord()
	record.Action = ActionReview
	if err := VerifyActionEffect(context.Background(), reader, ActionReview, "US-001", &record); err != nil {
		t.Fatalf("a hand-prepared dossier was rejected: %v", err)
	}
}

// US-035 AC-2 and AC-5: every way a review run can fail to back its own claim
// demotes the record and says why. The first case is the one the story exists
// for — an agent that closed the spec itself.
func TestVerifyActionEffectRefusesAReviewTheWorkspaceDoesNotBack(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader SpecStateReader
		want   string
	}{
		{
			name: "the agent decided in the person's place and closed the spec",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusDone}},
				review:      preparedDossier("exec-effect-1"),
			},
			want: "DONE",
		},
		{
			name: "the agent sent the spec back itself",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusTodo}},
				review:      preparedDossier("exec-effect-1"),
			},
			want: "TODO",
		},
		{
			name: "no evidence was persisted",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
			},
			want: "no review dossier",
		},
		{
			name: "the dossier examined no criterion",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
				review: domain.Review{Dossier: &domain.ReviewDossier{
					Summary: "looks fine",
				}},
			},
			want: "no review dossier",
		},
		{
			name: "the dossier belongs to another execution",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
				review:      preparedDossier("exec-effect-9"),
			},
			want: "exec-effect-9",
		},
		{
			name: "the review artifact cannot be read",
			reader: &reviewStateReader{
				stateReader: stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
				reviewErr:   errors.New("disk on fire"),
			},
			want: "disk on fire",
		},
		{
			name:   "the connector cannot read review artifacts at all",
			reader: &stateReader{spec: domain.Spec{Code: "US-001", Status: domain.StatusReview}},
			want:   "cannot read the review dossier back",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := succeededRecord()
			record.Action = ActionReview
			err := VerifyActionEffect(context.Background(), tc.reader, ActionReview, "US-001", &record)
			var unconfirmed *UnconfirmedEffectError
			if !errors.As(err, &unconfirmed) {
				t.Fatalf("error = %v, want *UnconfirmedEffectError", err)
			}
			if record.Status != StatusFailed || record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
				t.Fatalf("the record was not demoted: %#v", record)
			}
			if !strings.Contains(record.Error.Message, tc.want) {
				t.Fatalf("message = %q, want it to mention %q", record.Error.Message, tc.want)
			}
		})
	}
}
