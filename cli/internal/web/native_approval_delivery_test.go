package web

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

type refusedApprovalNativeProvider struct {
	*persistentFakeNativeProvider
	state execution.DeliveryState
	calls int
}

func (p *refusedApprovalNativeProvider) RespondSessionApproval(_ context.Context, r execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	p.calls++
	return execution.SessionDelivery{SubmissionID: r.SubmissionID, TurnID: r.TurnID, State: p.state}, fmt.Errorf("fixture approval failure")
}
func TestNativeApprovalErrorsDistinguishUnsentFromUncertain(t *testing.T) {
	for _, state := range []execution.DeliveryState{execution.DeliveryUnsent, execution.DeliveryUncertain} {
		t.Run(string(state), func(t *testing.T) {
			p := &refusedApprovalNativeProvider{persistentFakeNativeProvider: newPersistentFakeNativeProvider("native-fake", nil), state: state}
			srv, _, _ := newRunServer(t, p, true)
			id := openConversationOK(t, srv).Conversation.ID
			started := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/messages", map[string]any{"message": "work"})
			if started.Code != 202 {
				t.Fatal(started.Body.String())
			}
			refused := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/approvals/71", map[string]any{"option_id": "allow"})
			if refused.Code != 409 {
				t.Fatalf("approval = %d: %s", refused.Code, refused.Body.String())
			}
			body := refused.Body.String()
			if state == execution.DeliveryUnsent {
				if !strings.Contains(body, "l’autorizzazione non è stata inviata") || strings.Contains(body, "reconcile") {
					t.Fatalf("unsent approval reported uncertain: %s", body)
				}
			} else if !strings.Contains(body, "the approval delivery was not confirmed") || !strings.Contains(body, "reconcile") {
				t.Fatalf("uncertain delivery lost its reconciliation boundary: %s", body)
			}
			record, err := srv.session().conversationStore().GetMetadata(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if last := record.Deliveries[len(record.Deliveries)-1]; last.State != state {
				t.Fatalf("delivery = %#v", last)
			}
			doJSON(t, srv, "GET", conversationsRoute+"/"+id, nil)
			if p.calls != 1 {
				t.Fatalf("approval was automatically retried %d times", p.calls)
			}
		})
	}
}
