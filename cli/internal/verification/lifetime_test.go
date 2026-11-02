package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
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

func TestGuardShutdownCancelsCompatibleCheckBeforeCleanup(t *testing.T) {
	s, runner, recipe := fixture(t)
	check := recipe.Checks["focused"]
	check.DuringPreview = true
	recipe.Checks["focused"] = check
	writeRecipe(t, s.Root, recipe)
	execute(t, s, "prepare")
	input, control := io.Pipe()
	status, output := io.Pipe()
	defer func() { _ = input.Close(); _ = control.Close(); _ = status.Close(); _ = output.Close() }()
	guardDone := make(chan error, 1)
	go func() { guardDone <- Guard(context.Background(), input, output) }()
	if err := json.NewEncoder(control).Encode(lifetimeRequest{Root: s.Root}); err != nil {
		t.Fatal(err)
	}
	var ready lifetimeReady
	if err := json.NewDecoder(status).Decode(&ready); err != nil || ready.Error != "" {
		t.Fatal(ready, err)
	}
	started := make(chan struct{})
	checkDone := make(chan struct{})
	runner.run = func(ctx context.Context, _ string, c Command) error {
		if c.Name == "test" {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	go func() { defer close(checkDone); _, _ = s.Execute(context.Background(), "check", "focused", "", true) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("compatible check did not start")
	}
	_ = control.Close() // Parent death/EOF must stop the check, then clean services.
	select {
	case <-checkDone:
	case <-time.After(3 * time.Second):
		t.Fatal("guard did not cancel live check")
	}
	select {
	case err := <-guardDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("guard could not clean after check")
	}
	state, err := s.readState()
	if err != nil || state.PreparationID != "" {
		t.Fatal("cleanup did not revoke setup", err, state)
	}
}
