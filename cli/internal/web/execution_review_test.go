package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// These are the acceptance tests of US-035, and they are written against the
// real server, the real connector, the real Template and the real effect
// confirmation. Only the provider is a double, because the provider is the one
// component that in production is an external process.
//
// The oracle of the whole story is a pair of facts that must hold together: the
// review artifact of the spec, and the status of the spec itself. Neither is
// ever mocked, which is what makes "the provider prepared the evidence and did
// not decide" an observation rather than a claim.

// reviewTestProvider is runTestProvider with the declared capabilities made
// explicit, the same shape the implementation tests use: a provider that can
// implement is not thereby a provider that can prepare a review.
type reviewTestProvider struct {
	*runTestProvider
	capabilities []execution.Capability
}

func (p *reviewTestProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return p.capabilities, nil
}

func releasedReviewProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *reviewTestProvider {
	return &reviewTestProvider{
		runTestProvider: releasedProvider(id, execute),
		capabilities:    []execution.Capability{execution.CapabilitySpecReview},
	}
}

func blockedReviewProvider(id string) *reviewTestProvider {
	return &reviewTestProvider{
		runTestProvider: blockedProvider(id),
		capabilities:    []execution.Capability{execution.CapabilitySpecReview},
	}
}

const reviewSummary = "l'incremento aggiunge la rotta di saluto e i suoi test"

// preparingExecute is the behaviour of an agent that did its job: it writes the
// dossier through the connector, naming the execution it is running as, and
// leaves the spec exactly where it found it.
func preparingExecute(conn connector.Connector) func(context.Context, execution.Request) (execution.Result, error) {
	return func(ctx context.Context, request execution.Request) (execution.Result, error) {
		store, ok := conn.(connector.ReviewStore)
		if !ok {
			return execution.Result{}, errors.New("the test connector cannot store reviews")
		}
		artifact, err := store.ReadReview(ctx, request.SpecCode)
		if err != nil {
			return execution.Result{}, err
		}
		artifact.Dossier = &domain.ReviewDossier{
			ExecutionID: request.ExecutionID,
			PreparedAt:  "2026-08-20T10:00:00Z",
			Summary:     reviewSummary,
			Criteria: []domain.ReviewCriterion{
				{ID: "AC-1", Verdict: domain.ReviewCriterionMet, Note: "coperto da handler_test.go"},
				{ID: "AC-2", Verdict: domain.ReviewCriterionUnclear, Note: "nessuna evidenza sul caso vuoto"},
			},
			Blockers: []string{"il README documenta ancora la vecchia rotta"},
		}
		if err := store.SaveReview(ctx, request.SpecCode, artifact); err != nil {
			return execution.Result{}, err
		}
		payload := fmt.Sprintf(
			`{"result_summary":{"spec_code":%q,"status":"REVIEW","criteria":2,"blockers":1},"criteria":2,"blockers":1}`,
			request.SpecCode,
		)
		return execution.Result{Payload: json.RawMessage(payload), ExternalID: "task-review-1"}, nil
	}
}

// selfApprovingExecute is the failure the whole story exists to prevent: the
// agent prepares a perfectly good dossier and then closes the spec itself.
func selfApprovingExecute(conn connector.Connector) func(context.Context, execution.Request) (execution.Result, error) {
	prepare := preparingExecute(conn)
	return func(ctx context.Context, request execution.Request) (execution.Result, error) {
		result, err := prepare(ctx, request)
		if err != nil {
			return execution.Result{}, err
		}
		if _, err := conn.TransitionStatus(ctx, request.SpecCode, domain.StatusDone); err != nil {
			return execution.Result{}, err
		}
		return result, nil
	}
}

// seedSpecUnderReview brings US-901 to the real starting state of a review: a
// plan carried out and the spec waiting for a decision.
func seedSpecUnderReview(t *testing.T, conn connector.Connector) {
	t.Helper()
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusReview)
}

func readReviewArtifact(t *testing.T, srv *Server, code string) domain.Review {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/spec/"+code+"/review", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/spec/%s/review: %d %s", code, w.Code, w.Body.String())
	}
	var review domain.Review
	if err := json.Unmarshal(w.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	return review
}

func postJSON(t *testing.T, srv *Server, path string, payload any) (int, map[string]any) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, path, payload)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, body
}

// AC-1 — the provider prepares a dossier attached to the spec, and applies no
// verdict. The two halves are asserted together on purpose: a dossier without
// an unchanged status would not be a preparation, and an unchanged status
// without a dossier would not be a preparation either.
func TestRunSpecActionReviewPreparesADossierAndDecidesNothing(t *testing.T) {
	var conn connector.Connector
	provider := releasedReviewProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return preparingExecute(conn)(ctx, request)
	})
	srv, _, real := newRunServer(t, provider, true)
	conn = real
	seedSpecUnderReview(t, real)

	status, started := startAction(t, srv, "US-901", "review")
	if status != http.StatusCreated {
		t.Fatalf("POST review: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusSucceeded {
		t.Fatalf("a prepared review ended %s: %#v", record.Status, record.Error)
	}

	review := readReviewArtifact(t, srv, "US-901")
	if review.Dossier == nil {
		t.Fatal("no dossier was attached to the spec")
	}
	if review.Dossier.ExecutionID != id {
		t.Fatalf("dossier execution_id = %q, want the record %q", review.Dossier.ExecutionID, id)
	}
	if review.Dossier.Summary != reviewSummary || len(review.Dossier.Criteria) != 2 {
		t.Fatalf("the dossier does not carry the prepared evidence: %#v", review.Dossier)
	}
	if review.Verdict != nil {
		t.Fatalf("preparing the evidence produced a verdict: %#v", review.Verdict)
	}

	// AC-2 — and the spec has not moved.
	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusReview {
		t.Fatalf("the review moved the spec to %s", detail.Spec.Status)
	}
}

// AC-1 — a provider that does not declare spec.review is refused before
// anything exists: no record on disk and no dossier on the spec.
func TestRunSpecActionReviewRefusesAProviderWithoutTheCapability(t *testing.T) {
	// The default double declares spec.plan only.
	srv, cfg, conn := newRunServer(t, releasedProvider("fake", nil), true)
	seedSpecUnderReview(t, conn)

	status, body := startAction(t, srv, "US-901", "review")
	if status != http.StatusConflict {
		t.Fatalf("incompatible provider: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, string(execution.CapabilitySpecReview)) {
		t.Fatalf("the refusal does not name the missing capability: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
		t.Fatalf("a refused review created %d record(s)", got)
	}
	if review := readReviewArtifact(t, srv, "US-901"); review.Dossier != nil {
		t.Fatalf("a refused review left a dossier: %#v", review.Dossier)
	}
	found, runnable, reason := actionChip(runSpecDetail(t, srv, "US-901"), "review")
	if !found || runnable || !strings.Contains(reason, string(execution.CapabilitySpecReview)) {
		t.Fatalf("the chip does not explain the missing capability: found=%v runnable=%v reason=%q", found, runnable, reason)
	}
}

// AC-1 — the action exists only where the Template admits it, and the status
// alone decides, both on the chip and on the start route.
func TestRunSpecActionReviewIsAdmittedOnlyByTheTemplateStatuses(t *testing.T) {
	cases := []struct {
		status   domain.Status
		admitted bool
	}{
		{domain.StatusTodo, false},
		{domain.StatusPlanned, false},
		{domain.StatusInProgress, false},
		{domain.StatusReview, true},
		{domain.StatusDone, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			provider := blockedReviewProvider("fake")
			srv, _, conn := newRunServer(t, provider, true)
			persistImplementablePlan(t, conn, "US-901")
			moveSpecTo(t, conn, "US-901", tc.status)

			found, runnable, reason := actionChip(runSpecDetail(t, srv, "US-901"), "review")
			if tc.admitted && (!found || !runnable || reason != "") {
				t.Fatalf("review is not offered on %s: found=%v runnable=%v reason=%q", tc.status, found, runnable, reason)
			}
			if !tc.admitted && found && runnable {
				t.Fatalf("review is offered on %s: reason=%q", tc.status, reason)
			}

			status, body := startAction(t, srv, "US-901", "review")
			if !tc.admitted {
				if status != http.StatusConflict {
					t.Fatalf("review on %s: %d %v", tc.status, status, body)
				}
				message, _ := body["error"].(string)
				for _, want := range []string{"review", "US-901", string(tc.status)} {
					if !strings.Contains(message, want) {
						t.Fatalf("the refusal does not name %q: %q", want, message)
					}
				}
				return
			}
			if status != http.StatusCreated {
				t.Fatalf("review on %s: %d %v", tc.status, status, body)
			}
			id, _ := body["id"].(string)
			close(provider.release)
			awaitTerminal(t, srv, id)
		})
	}
}

// AC-2 — an agent that decides in the person's place does not get a success.
// The record is rewritten FAILED with the stable code, and the reason names the
// status the connector really found.
func TestRunSpecActionReviewDemotesAnAgentThatClosedTheSpecItself(t *testing.T) {
	var conn connector.Connector
	provider := releasedReviewProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return selfApprovingExecute(conn)(ctx, request)
	})
	srv, _, real := newRunServer(t, provider, true)
	conn = real
	seedSpecUnderReview(t, real)

	status, started := startAction(t, srv, "US-901", "review")
	if status != http.StatusCreated {
		t.Fatalf("POST review: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed {
		t.Fatalf("a self-approving agent ended %s", record.Status)
	}
	if record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("the demotion lost its code: %#v", record.Error)
	}
	if !strings.Contains(record.Error.Message, string(domain.StatusDone)) {
		t.Fatalf("the reason does not name the status found: %q", record.Error.Message)
	}
}

// AC-2, AC-3 — the sequence is the criterion: the run ends, the spec is still
// waiting, and only a human gesture closes it. The verdict is kept together
// with the execution that prepared the evidence it was taken on, and a spec
// already decided cannot be decided again.
func TestApproveIsTheOnlyThingThatClosesASpecUnderReview(t *testing.T) {
	var conn connector.Connector
	provider := releasedReviewProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return preparingExecute(conn)(ctx, request)
	})
	srv, _, real := newRunServer(t, provider, true)
	conn = real
	seedSpecUnderReview(t, real)

	_, started := startAction(t, srv, "US-901", "review")
	id, _ := started["id"].(string)
	if record := awaitTerminal(t, srv, id); record.Status != execution.StatusSucceeded {
		t.Fatalf("a prepared review ended %s", record.Status)
	}
	// The run is over and the spec has not moved: this is the state the gate
	// exists to hold.
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status != domain.StatusReview {
		t.Fatalf("the spec left review without a human decision: %s", detail.Spec.Status)
	}

	status, body := postJSON(t, srv, "/api/spec/US-901/approve", nil)
	if status != http.StatusOK {
		t.Fatalf("POST approve: %d %v", status, body)
	}
	if body["status"] != string(domain.StatusDone) || body["execution_id"] != id {
		t.Fatalf("the approval does not report what it did: %v", body)
	}
	if integrated, _ := body["integrated"].(bool); integrated {
		t.Fatalf("a workspace without worktrees reported an integration: %v", body)
	}
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status != domain.StatusDone {
		t.Fatalf("the approval left the spec %s", detail.Spec.Status)
	}
	review := readReviewArtifact(t, srv, "US-901")
	if review.Verdict == nil || review.Verdict.Decision != domain.ReviewDecisionApproved {
		t.Fatalf("the verdict was not recorded: %#v", review.Verdict)
	}
	if review.Verdict.ExecutionID != id {
		t.Fatalf("verdict execution_id = %q, want the run that prepared the evidence %q", review.Verdict.ExecutionID, id)
	}
	if strings.TrimSpace(review.Verdict.DecidedAt) == "" {
		t.Fatal("the verdict carries no decision time")
	}
	// The dossier survives the approval: it is the evidence the decision was
	// taken on, and losing it would leave the verdict unexplained.
	if review.Dossier == nil || review.Dossier.ExecutionID != id {
		t.Fatalf("the approval lost the dossier: %#v", review.Dossier)
	}

	// Nothing can be approved twice.
	status, body = postJSON(t, srv, "/api/spec/US-901/approve", nil)
	if status != http.StatusConflict {
		t.Fatalf("second approve: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, string(domain.StatusDone)) {
		t.Fatalf("the refusal does not name the status: %q", message)
	}
}

// AC-2 — the gate refuses a spec that never reached review, whatever it holds.
func TestApproveRefusesASpecThatIsNotUnderReview(t *testing.T) {
	srv, _, conn := newRunServer(t, releasedProvider("fake", nil), true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	status, body := postJSON(t, srv, "/api/spec/US-901/approve", nil)
	if status != http.StatusConflict {
		t.Fatalf("approve on PLANNED: %d %v", status, body)
	}
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status != domain.StatusPlanned {
		t.Fatalf("a refused approval moved the spec to %s", detail.Spec.Status)
	}
	if review := readReviewArtifact(t, srv, "US-901"); review.Verdict != nil {
		t.Fatalf("a refused approval recorded a verdict: %#v", review.Verdict)
	}
}

// AC-4 — requesting changes takes the spec to the rework status the Template
// prescribes, carries the feedback into the body, and keeps the same kind of
// trace an approval keeps.
func TestRequestChangesRecordsItsVerdictAndTheStructuredFeedback(t *testing.T) {
	var conn connector.Connector
	provider := releasedReviewProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return preparingExecute(conn)(ctx, request)
	})
	srv, _, real := newRunServer(t, provider, true)
	conn = real
	seedSpecUnderReview(t, real)

	_, started := startAction(t, srv, "US-901", "review")
	id, _ := started["id"].(string)
	awaitTerminal(t, srv, id)

	// A reviewer leaves one anchored comment before sending the work back.
	existing := readReviewArtifact(t, srv, "US-901")
	existing.Comments = []domain.ReviewComment{
		{File: "src/app.js", Side: "new", Line: 12, Body: "gestisci la lista vuota"},
	}
	w := doJSON(t, srv, http.MethodPut, "/api/spec/US-901/review", existing)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT review: %d %s", w.Code, w.Body.String())
	}

	status, body := postJSON(t, srv, "/api/spec/US-901/request-changes", nil)
	if status != http.StatusOK {
		t.Fatalf("POST request-changes: %d %v", status, body)
	}
	if moved, _ := body["comments_moved"].(float64); moved != 1 {
		t.Fatalf("comments_moved = %v, want 1", body["comments_moved"])
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusTodo {
		t.Fatalf("request-changes left the spec %s", detail.Spec.Status)
	}
	if !detail.Spec.Rework {
		t.Fatal("the spec is not flagged as in rework")
	}
	if !strings.Contains(detail.Spec.Body, domain.ReworkFeedbackHeading) {
		t.Fatalf("the body carries no rework feedback:\n%s", detail.Spec.Body)
	}
	if !strings.Contains(detail.Spec.Body, "src/app.js:12") {
		t.Fatalf("the feedback lost its anchor:\n%s", detail.Spec.Body)
	}

	review := readReviewArtifact(t, srv, "US-901")
	if review.Verdict == nil || review.Verdict.Decision != domain.ReviewDecisionChangesRequested {
		t.Fatalf("the verdict was not recorded: %#v", review.Verdict)
	}
	if review.Verdict.ExecutionID != id {
		t.Fatalf("verdict execution_id = %q, want %q", review.Verdict.ExecutionID, id)
	}
	// The rejected evidence no longer describes the increment, so it goes with
	// the comments; the decision is what survives.
	if review.Dossier != nil {
		t.Fatalf("the rejected dossier survived: %#v", review.Dossier)
	}
	if len(review.Comments) != 0 {
		t.Fatalf("the converted comments survived: %#v", review.Comments)
	}
}

// AC-5 — a provider that fails leaves the spec exactly where it was, makes the
// reason readable, and produces no verdict of any kind.
func TestRunSpecActionReviewFailureLeavesTheSpecWaitingWithAReadableReason(t *testing.T) {
	provider := releasedReviewProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{}, errors.New("codex: not logged in")
	})
	srv, _, conn := newRunServer(t, provider, true)
	seedSpecUnderReview(t, conn)

	status, started := startAction(t, srv, "US-901", "review")
	if status != http.StatusCreated {
		t.Fatalf("POST review: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed {
		t.Fatalf("a failed provider ended %s", record.Status)
	}
	if record.Error == nil || !strings.Contains(record.Error.Message, "codex: not logged in") {
		t.Fatalf("the provider reason is not readable on the record: %#v", record.Error)
	}
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status != domain.StatusReview {
		t.Fatalf("a failed review moved the spec to %s", detail.Spec.Status)
	}
	review := readReviewArtifact(t, srv, "US-901")
	if review.Dossier != nil || review.Verdict != nil {
		t.Fatalf("a failed review left evidence or a verdict: %#v %#v", review.Dossier, review.Verdict)
	}
}

// AC-5 — a run that declares success without having persisted anything is not a
// success either: the spec is untouched and no implicit verdict appears.
func TestRunSpecActionReviewDemotesARunThatPreparedNoEvidence(t *testing.T) {
	provider := releasedReviewProvider("fake", func(_ context.Context, request execution.Request) (execution.Result, error) {
		payload := fmt.Sprintf(`{"result_summary":{"spec_code":%q,"status":"REVIEW","criteria":4,"blockers":0}}`, request.SpecCode)
		return execution.Result{Payload: json.RawMessage(payload)}, nil
	})
	srv, _, conn := newRunServer(t, provider, true)
	seedSpecUnderReview(t, conn)

	_, started := startAction(t, srv, "US-901", "review")
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed {
		t.Fatalf("an empty-handed run ended %s", record.Status)
	}
	if record.Error == nil || !strings.Contains(record.Error.Message, "no review dossier") {
		t.Fatalf("the reason does not say what is missing: %#v", record.Error)
	}
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status != domain.StatusReview {
		t.Fatalf("the spec moved to %s", detail.Spec.Status)
	}
}
