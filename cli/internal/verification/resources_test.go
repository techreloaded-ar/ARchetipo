package verification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type resourceRunner struct {
	calls []string
	envs  [][]string
	fail  map[string]bool
}

func (r *resourceRunner) Run(_ context.Context, _ string, env []string, c Command, out io.Writer) error {
	r.calls = append(r.calls, c.Name)
	r.envs = append(r.envs, env)
	_, _ = out.Write([]byte(c.Name))
	if r.fail[c.Name] {
		return errors.New("fixture failure")
	}
	return nil
}
func managedFixture(t *testing.T) (*Service, *resourceRunner, Recipe) {
	t.Helper()
	s, _, recipe := fixture(t)
	command := func(name string) Command {
		return Command{Name: name, Argv: []string{"go", "version"}, TimeoutSeconds: 1}
	}
	recipe.Ports = []string{"DB", "API"}
	for _, name := range []string{"db", "api"} {
		recipe.Resources = append(recipe.Resources, Resource{Name: name, Start: command(name + "-start"), Probe: command(name + "-probe"), Stop: command(name + "-stop"), Stopped: command(name + "-stopped")})
	}
	runner := &resourceRunner{fail: map[string]bool{}}
	s.Runner = runner
	writeRecipe(t, s.Root, recipe)
	return s, runner, recipe
}
func TestManagedResourcesReuseAndReverseVerifiedStop(t *testing.T) {
	s, r, _ := managedFixture(t)
	first := execute(t, s, "check")
	second := execute(t, s, "check")
	if !first.Passed || !second.Reused {
		t.Fatalf("%+v %+v", first, second)
	}
	if strings.Count(strings.Join(r.calls, ","), "db-start") != 1 {
		t.Fatal(r.calls)
	}
	state, err := s.Resources()
	if err != nil {
		t.Fatal(err)
	}
	if state.Owner == "" || state.Ports["DB"] == state.Ports["API"] {
		t.Fatal(state)
	}
	if !strings.Contains(strings.Join(r.envs[0], "\n"), "ARCHETIPO_RESOURCE_OWNER="+state.Owner) {
		t.Fatal("owner absent")
	}
	cleanup := execute(t, s, "cleanup")
	if !cleanup.Passed {
		t.Fatal(cleanup)
	}
	if !strings.HasSuffix(strings.Join(r.calls, ","), "api-stop,api-stopped,db-stop,db-stopped") {
		t.Fatal(r.calls)
	}
	state, _ = s.Resources()
	for _, resource := range state.Resources {
		if resource.State != "stopped" {
			t.Fatal(state)
		}
	}
}
func TestFailedPartialStartRollsBackRegisteredResources(t *testing.T) {
	s, r, _ := managedFixture(t)
	r.fail["api-start"] = true
	result := execute(t, s, "prepare")
	if result.Passed {
		t.Fatal("start failure passed")
	}
	if !strings.HasSuffix(strings.Join(r.calls, ","), "api-stop,api-stopped,db-stop,db-stopped") {
		t.Fatal(r.calls)
	}
	r.fail["api-start"] = false
	if !execute(t, s, "prepare").Passed {
		t.Fatal("retry failed")
	}
}
func TestFailedStopRetainsOwnerAndRetriesOnlyPendingResources(t *testing.T) {
	s, r, _ := managedFixture(t)
	execute(t, s, "prepare")
	before, _ := s.Resources()
	r.fail["api-stopped"] = true
	if execute(t, s, "cleanup").Passed {
		t.Fatal("failed stopped probe passed")
	}
	after, _ := s.Resources()
	if after.Owner != before.Owner || after.Resources[1].State != "error" || after.Resources[0].State != "stopped" {
		t.Fatal(after)
	}
	r.calls = nil
	r.fail["api-stopped"] = false
	if !execute(t, s, "cleanup").Passed || strings.Join(r.calls, ",") != "api-stop,api-stopped" {
		t.Fatal(r.calls)
	}
}
func TestCleanupUsesJournalAfterRecipeDisappears(t *testing.T) {
	s, r, _ := managedFixture(t)
	execute(t, s, "prepare")
	if err := os.Remove(filepath.Join(s.Root, RecipePath)); err != nil {
		t.Fatal(err)
	}
	r.calls = nil
	if !execute(t, s, "cleanup").Passed || !strings.Contains(strings.Join(r.calls, ","), "db-stop") {
		t.Fatal(r.calls)
	}
}
func TestChangedRecipeStopsOriginalResourcesAndInvalidatesChecks(t *testing.T) {
	s, r, recipe := managedFixture(t)
	first := execute(t, s, "check")
	recipe.Resources[1].Start.Argv = append(recipe.Resources[1].Start.Argv, "new")
	writeRecipe(t, s.Root, recipe)
	second := execute(t, s, "check")
	if !second.Passed || second.Reused || first.PreparationID == second.PreparationID {
		t.Fatal(second)
	}
	if !strings.Contains(strings.Join(r.calls, ","), "api-stop,api-stopped,db-stop,db-stopped,db-start") {
		t.Fatal(r.calls)
	}
}

func TestManagedPrepareFailureStopsDependencies(t *testing.T) {
	s, r, _ := managedFixture(t)
	r.fail["install"] = true
	result := execute(t, s, "prepare")
	if result.Passed {
		t.Fatal("failed install passed")
	}
	state, _ := s.Resources()
	for _, record := range state.Resources {
		if record.State != "stopped" {
			t.Fatal(state)
		}
	}
}

func TestRuntimeDoesNotInheritAnotherCheckoutsOwnerOrPorts(t *testing.T) {
	t.Setenv("ARCHETIPO_PORT_FOREIGN", "9000")
	t.Setenv("ARCHETIPO_RESOURCE_OWNER", "foreign-owner")
	s, r, _ := managedFixture(t)
	execute(t, s, "prepare")
	for _, env := range r.envs {
		for _, value := range env {
			if strings.Contains(value, "FOREIGN") || value == "ARCHETIPO_RESOURCE_OWNER=foreign-owner" {
				t.Fatal(value)
			}
		}
	}
	env, err := s.RuntimeEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range env {
		if strings.Contains(value, "FOREIGN") || value == "ARCHETIPO_RESOURCE_OWNER=foreign-owner" {
			t.Fatal(value)
		}
	}
}

func TestManagedResourceValidation(t *testing.T) {
	s, _, recipe := managedFixture(t)
	recipe.Resources[0].Stop.TimeoutSeconds = 31
	raw, _ := json.Marshal(recipe)
	if _, err := Configure(s.Root, raw); err == nil {
		t.Fatal("unbounded stop accepted")
	}
	recipe.Resources[0].Stop.TimeoutSeconds = 1
	recipe.Ports = []string{"DATABASE", "DATABASE"}
	raw, _ = json.Marshal(recipe)
	if _, err := Configure(s.Root, raw); err == nil {
		t.Fatal("duplicate ports accepted")
	}
}
