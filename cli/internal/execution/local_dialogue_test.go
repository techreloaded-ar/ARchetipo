package execution_test

import (
	"context"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/claude"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/codex"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// AC-1, AC-3 — the two local providers are followed and commanded through the
// same rules, so they must also refuse in the same way.
//
// The expected reasons are written once and compared for both. That is the
// whole point of the case: a viewer branches on the reason and not on the
// provider, so the day one of the two starts answering `unsupported` where the
// other answers `not_found`, this test fails instead of the interface quietly
// growing a second dialect.
//
// It lives outside both provider packages on purpose. Asserted from inside one
// of them it would only describe that one; asserted here, on the shared
// interface, it describes the contract the caller actually depends on.
func TestLocalProvidersRefuseTheSameWay(t *testing.T) {
	providers := map[string]execution.Provider{
		codex.ProviderID:  codex.New(codex.Options{}),
		claude.ProviderID: claude.New(claude.Options{}),
	}
	// The reasons are declared once, outside the loop over the providers, so a
	// divergence between the two cannot be described as two different
	// expectations.
	const (
		wantUnknownRun = execution.RunRefusedNotFound
		wantEmpty      = execution.RunRefusedUnsupported
		wantApproval   = execution.RunRefusedUnsupported
	)

	for id, provider := range providers {
		t.Run(id, func(t *testing.T) {
			collaborator, ok := execution.RunCollaboratorFor(provider)
			if !ok {
				t.Fatalf("the local provider %s does not expose an interactive run", id)
			}
			ctx := context.Background()
			unknown := execution.RunRequest{RunID: "a-run-this-process-never-opened"}

			// A run this process never opened: the three commands a viewer can
			// send all report the same reason, and none of them invents a state.
			commands := map[string]error{
				"read":    firstErr(collaborator.ReadRun(ctx, unknown)),
				"message": collaborator.SendRunMessage(ctx, unknown, "ci sei?"),
				"cancel":  collaborator.CancelRun(ctx, unknown),
			}
			for command, err := range commands {
				reason, refused := execution.RefusalOf(err)
				if !refused || reason != wantUnknownRun {
					t.Fatalf("%s on an unknown run got %v; want a %s refusal", command, err, wantUnknownRun)
				}
			}

			// The remaining two need a run that exists, because they are about
			// what is asked of it and not about whether it is there.
			registry := registryOf(t, provider)
			session := localrun.NewSession("run-1", nil)
			session.AttachDialogue(refusingDialogue{})
			registry.Register(session)
			live := execution.RunRequest{RunID: "run-1"}

			for _, message := range []string{"", "   ", "\n\t"} {
				err := collaborator.SendRunMessage(ctx, live, message)
				reason, refused := execution.RefusalOf(err)
				if !refused || reason != wantEmpty {
					t.Fatalf("the empty message %q got %v; want a %s refusal", message, err, wantEmpty)
				}
			}

			err := collaborator.RespondRunApproval(ctx, live, "approval-1", "allow")
			reason, refused := execution.RefusalOf(err)
			if !refused || reason != wantApproval {
				t.Fatalf("answering an approval got %v; want a %s refusal", err, wantApproval)
			}
			// And there is nothing to answer in the first place, which is the
			// reason the refusal is the one above.
			approvals, err := collaborator.ReadRunApprovals(ctx, live)
			if err != nil || len(approvals) != 0 {
				t.Fatalf("a local run reported %d pending approval(s) (err=%v)", len(approvals), err)
			}
		})
	}
}

// registryOf reaches the sessions of a local provider, which is how a run can
// be made to exist without starting a process.
func registryOf(t *testing.T, provider execution.Provider) *localrun.Registry {
	t.Helper()
	holder, ok := provider.(interface{ Registry() *localrun.Registry })
	if !ok {
		t.Fatalf("the local provider %s holds no session registry", provider.ID())
	}
	return holder.Registry()
}

// refusingDialogue stands for a live process that is never actually spoken to:
// every case above is refused before the dialogue is reached, and a dialogue
// that recorded a delivery would prove that one of them was not.
type refusingDialogue struct{}

func (refusingDialogue) Send(context.Context, string) error {
	panic("a refused command must never reach the process")
}

func (refusingDialogue) Interrupt(context.Context) error {
	panic("a refused command must never reach the process")
}

func firstErr(_ execution.RunSnapshot, err error) error { return err }
