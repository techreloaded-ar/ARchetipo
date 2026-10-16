package workflowrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestCheckpointResumePreservesWorkAndRejectsStaleWriters(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := New(root)
	input := Start{Skill: "archetipo-autopilot", Invocation: "/archetipo-autopilot", Scope: "US-001", State: json.RawMessage(`{"specs":{"US-001":{"review_iterations":2}}}`)}
	r, err := store.Start(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(ctx, r.ID, Checkpoint{Revision: r.Revision, Status: Running, Phase: "implement", WorkerID: "child-1", State: input.State})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := New(root).Start(ctx, input)
	if err != nil || resumed.ID != r.ID || resumed.WorkerID != "child-1" || !sameJSON(resumed.State, input.State) {
		t.Fatalf("lost checkpoint: %+v %v", resumed, err)
	}
	if _, err = store.Update(ctx, r.ID, Checkpoint{Revision: r.Revision, Status: Complete, State: input.State}); err == nil {
		t.Fatal("stale writer completed run")
	}
	input.Scope = "US-002"
	if _, err = store.Start(ctx, input); err == nil {
		t.Fatal("unfinished scope was widened")
	}
	finished, err := store.Update(ctx, r.ID, Checkpoint{Revision: updated.Revision, Status: Complete, State: input.State})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Update(ctx, r.ID, Checkpoint{Revision: finished.Revision, Status: Running, State: input.State}); err == nil {
		t.Fatal("completed history was rewritten")
	}
}

func TestRecoveryIsBoundedAndDoesNotChangeControllerState(t *testing.T) {
	ctx := context.Background()
	store := New(t.TempDir())
	r, err := store.Start(ctx, Start{Skill: "archetipo-example", Invocation: "/archetipo-example", Scope: "example", State: json.RawMessage(`{"completed":["TASK-01"],"review_iterations":2}`)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		r, err = store.Recover(ctx, r.ID, fmt.Sprint(i), "transport failed")
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.Recover(ctx, r.ID, fmt.Sprint(i), "same failure")
		if err != nil || again.Revision != r.Revision {
			t.Fatal("same failed turn consumed a second recovery")
		}
		if i <= 3 && r.Status != Recovering {
			t.Fatal(r.Status)
		}
	}
	if r.Status != Blocked || r.RecoveryAttempts != 3 || !sameJSON(r.State, json.RawMessage(`{"completed":["TASK-01"],"review_iterations":2}`)) {
		t.Fatalf("unbounded or destructive recovery: %+v", r)
	}
	if _, err = store.Read("../outside"); err == nil {
		t.Fatal("unsafe id accepted")
	}
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	_ = json.Compact(&x, a)
	_ = json.Compact(&y, b)
	return x.String() == y.String()
}
