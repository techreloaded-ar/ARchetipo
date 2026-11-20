package verification

import (
	"context"
	"io"
	"strings"
	"testing"
)

// environmentRecordingRunner keeps the environment each command received.
type environmentRecordingRunner struct {
	environmentByCommand map[string][]string
}

func (r *environmentRecordingRunner) Run(_ context.Context, _ string, env []string, command Command, out io.Writer) error {
	if r.environmentByCommand == nil {
		r.environmentByCommand = map[string][]string{}
	}
	r.environmentByCommand[command.Name] = append([]string(nil), env...)
	_, _ = io.WriteString(out, command.Name+"\n")
	return nil
}

func variableValue(env []string, name string) (string, bool) {
	for _, pair := range env {
		if key, value, _ := strings.Cut(pair, "="); key == name {
			return value, true
		}
	}
	return "", false
}

const incrementBaseSHA = "3bda2da4abc440a4301d160454eeb03b7a6dc5c5"

func TestCheckCommandReceivesIncrementBaseAndSetupDoesNot(t *testing.T) {
	// A value inherited from the caller describes another checkout.
	t.Setenv(EnvBaseSHA, "inherited-from-another-checkout")
	t.Setenv(EnvIncrementID, "inc-inherited")
	s, _, _ := fixture(t)
	runner := &environmentRecordingRunner{}
	s.Runner = runner
	s.ForIncrementBase(IncrementBase{IncrementID: "inc-0123456789abcdef", BaseSHA: incrementBaseSHA, BaseBranch: "main"})
	result := execute(t, s, "check")
	if !result.Passed || !result.Stable || result.BaseSHA != incrementBaseSHA {
		t.Fatalf("check did not record its base: %+v", result)
	}
	for name, want := range map[string]string{EnvBaseSHA: incrementBaseSHA, EnvBaseBranch: "main", EnvIncrementID: "inc-0123456789abcdef"} {
		if got, present := variableValue(runner.environmentByCommand["test"], name); !present || got != want {
			t.Errorf("check command %s = %q (present %t), want %q", name, got, present, want)
		}
	}
	for _, setupCommand := range []string{"install", "ready"} {
		for _, name := range []string{EnvBaseSHA, EnvBaseBranch, EnvIncrementID} {
			if got, present := variableValue(runner.environmentByCommand[setupCommand], name); present {
				t.Errorf("setup command %s received %s=%q; setup does not depend on what an increment changed", setupCommand, name, got)
			}
		}
	}
}

func TestCheckOutsideAnIncrementReceivesNoBase(t *testing.T) {
	t.Setenv(EnvBaseSHA, "inherited-from-another-checkout")
	t.Setenv(EnvBaseBranch, "inherited")
	s, _, _ := fixture(t)
	runner := &environmentRecordingRunner{}
	s.Runner = runner
	result := execute(t, s, "check")
	if !result.Passed || result.BaseSHA != "" {
		t.Fatalf("an ordinary checkout recorded a base: %+v", result)
	}
	for _, name := range []string{EnvBaseSHA, EnvBaseBranch, EnvIncrementID} {
		if got, present := variableValue(runner.environmentByCommand["test"], name); present {
			t.Errorf("check command received %s=%q outside an increment; the CLI must not guess a base", name, got)
		}
	}
}

// Evidence recorded before this CLI passed a base, and the readers that never
// receive one (View, increment ready), must keep agreeing with the check.
func TestIncrementBaseDoesNotChangeTheCheckFingerprint(t *testing.T) {
	s, _, _ := fixture(t)
	s.Runner = &environmentRecordingRunner{}
	withBase := execute(t, s.ForIncrementBase(IncrementBase{IncrementID: "inc-0123456789abcdef", BaseSHA: incrementBaseSHA, BaseBranch: "main"}), "check")
	reader := New(s.Root, s.Results)
	reader.Runner = &environmentRecordingRunner{}
	evidence, err := reader.Evidence(context.Background())
	if err != nil {
		t.Fatalf("a reader without the base rejected the evidence: %v", err)
	}
	if len(evidence) != 1 || evidence[0].ID != withBase.ID || evidence[0].InputKey != withBase.InputKey {
		t.Fatalf("evidence = %+v, want the report %s", evidence, withBase.ID)
	}
	if err = reader.VerifyEvidence(context.Background(), evidence); err != nil {
		t.Fatalf("frozen evidence did not survive a reader without the base: %v", err)
	}
	reused := execute(t, reader, "check")
	if !reused.Reused || reused.InputKey != withBase.InputKey {
		t.Fatalf("the same checkout without the base reran the check: %+v", reused)
	}
}

func TestRecipeCannotDeclareReservedVariablesAsInputs(t *testing.T) {
	for _, reserved := range []string{EnvBaseSHA, EnvBaseBranch, EnvIncrementID, "ARCHETIPO_ENVIRONMENT_ID", "ARCHETIPO_PORT_APP"} {
		s, _, recipe := fixture(t)
		recipe.EnvKeys = []string{reserved}
		writeRecipe(t, s.Root, recipe)
		if _, err := s.Execute(context.Background(), "check", "focused", "", false); err == nil {
			t.Errorf("recipe declaring %s as a project input was accepted", reserved)
		}
	}
}

func TestIncrementBaseNeverInjectsMalformedValues(t *testing.T) {
	env := IncrementBase{IncrementID: "", BaseSHA: incrementBaseSHA, BaseBranch: "main\nEVIL=1"}.env()
	if len(env) != 1 || env[0] != EnvBaseSHA+"="+incrementBaseSHA {
		t.Fatalf("env = %q, want only the base commit", env)
	}
}
