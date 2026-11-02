package web

import (
	"context"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"testing"
)

// implementTestProvider is runTestProvider with the declared capabilities made
// explicit. It exists next to the planning double rather than replacing it
// because the two declarations are the whole point of AC-2: a provider that can
// plan is not thereby a provider that can implement, and the tests need both
// shapes over the same Execute machinery.
//
// Everything else stays production: connector, record store, Template and
// effect confirmation.
type implementTestProvider struct {
	*runTestProvider
	capabilities []execution.Capability
}

func (p *implementTestProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return p.capabilities, nil
}

// blockedImplementProvider declares spec.implement and stays inside Execute
// until the test releases it, which is how the IN PROGRESS window becomes
// observable without a single sleep.

// releasedImplementProvider declares spec.implement and is already free to run.

// persistImplementablePlan brings US-901 to the real starting state of an
// implementation: a persisted plan with more than one open task. It writes
// through the production connector, so the plan the route later refuses to run
// without is the very one the connector reads back.
func persistImplementablePlan(t *testing.T, conn connector.Connector, code string) {
	t.Helper()
	plan := domain.PlanInput{
		PlanBody: "# Piano di " + code + "\n\nDue passi.\n",
		Tasks: []domain.Task{
			{ID: "TASK-01", Title: "Scrivere il codice", Type: domain.TaskImpl, Status: domain.StatusTodo, Body: "## Objective\nCodice.\n"},
			{ID: "TASK-02", Title: "Scrivere i test", Type: domain.TaskTest, Status: domain.StatusTodo, Body: "## Objective\nTest.\n"},
		},
	}
	if _, err := conn.SavePlan(context.Background(), code, plan); err != nil {
		t.Fatal(err)
	}
}

// moveSpecTo puts the spec in an arbitrary workflow status, which is what the
// per-status half of AC-1 needs: the Template answer must depend on the status
// alone, not on how the spec reached it.
func moveSpecTo(t *testing.T, conn connector.Connector, code string, status domain.Status) {
	t.Helper()
	if _, err := conn.TransitionStatus(context.Background(), code, status); err != nil {
		t.Fatal(err)
	}
}

// actionChip returns the action the detail exposes under id. A chip the process
// does not offer in the current status is simply absent, which is itself an
// answer the tests assert on.

// implementingExecute is the provider behaviour of an agent that really carried
// the plan out: it completes every task through the connector, moves the spec to
// REVIEW, and only then hands back its receipt with the summary of code and
// tests.

// AC-1: the implement action exists only where the Template admits it. The
// status alone decides, on the start route and on the chip the browser renders,
// and everything else about the workspace is held constant — plan persisted,
// provider capable, nothing running.

// AC-2: a provider that does not declare spec.implement is refused before
// anything exists. No record under .archetipo/executions/, no transition on the
// spec, and the same sentence next to the disabled chip.

// AC-3: an accepted start moves the spec to IN PROGRESS and the record names
// what is running — provider, spec and the status the run started from — while
// the provider is still working.

// AC-3, the other precondition: an implementation carries out a plan, so a
// PLANNED spec with no persisted plan is refused before any effect, and the chip
// says the same thing.

// AC-4: success is proved by the spec reaching REVIEW with its plan carried
// out, and the record keeps a verifiable summary of the code and tests the run
// produced.

// AC-5a: a dispatch that fails keeps a readable reason on the record and leaves
// the spec out of REVIEW.

// AC-5b: a receipt that declares an implementation nobody performed is not
// believed. The very first terminal state a client can read is already the
// demoted one, so there is no SUCCEEDED window a poll could settle on.

// AC-5b, at the only instant where believing the provider would be visible: the
// implementation receipt is answered, the confirmation is held open, and the
// route the browser polls must still report RUNNING rather than a success that
// is about to be withdrawn.

// The plan run of a conversational provider stays RUNNING while the agent
// waits for an answer, and the spec meanwhile becomes PLANNED with a persisted
// plan. That open run must not keep the Implementa chip inert: since a second
// start is no longer refused, a running execution blocks only its own action.
