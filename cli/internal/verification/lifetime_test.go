package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGuardExplicitReleaseRetainsResourcesAndUnlocks(t *testing.T) {
	s, _, _ := managedFixture(t)
	execute(t, s, "prepare")
	before, err := s.Resources()
	if err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(lifetimeRequest{Root: s.Root})
	var output bytes.Buffer
	if err = Guard(context.Background(), strings.NewReader(string(request)+"\n{\"operation\":\"release\"}\n"), &output); err != nil {
		t.Fatal(err)
	}
	after, err := s.Resources()
	if err != nil || after.Owner != before.Owner || after.Resources[0].State != "ready" {
		t.Fatalf("release cleaned services: %+v %v", after, err)
	}
	// A standalone check must acquire the lock and retain its original pass.
	if result := execute(t, s, "prepare"); !result.Passed || !result.Reused {
		t.Fatalf("handoff lost preparation or lock: %+v", result)
	}
}

func TestGuardEOFAndInvalidReleaseStillCleanUp(t *testing.T) {
	for _, ending := range []string{"", "{\"operation\":\"unknown\"}\n", "{broken\n"} {
		t.Run(ending, func(t *testing.T) {
			s, _, _ := fixture(t)
			execute(t, s, "prepare")
			request, _ := json.Marshal(lifetimeRequest{Root: s.Root})
			if err := Guard(context.Background(), strings.NewReader(string(request)+"\n"+ending), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			state, err := s.readState()
			if err != nil || state.PreparationID != "" {
				t.Fatalf("lost owner did not invalidate preparation: %+v %v", state, err)
			}
		})
	}
}
