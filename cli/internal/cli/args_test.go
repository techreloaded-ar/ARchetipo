package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestExtractArgs_NilCommand(t *testing.T) {
	if got := extractArgs(nil); got != nil {
		t.Errorf("extractArgs(nil) = %v, want nil", got)
	}
}

func TestExtractArgs_NoFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := extractArgs(cmd); got != nil {
		t.Errorf("extractArgs(no flags) = %v, want nil", got)
	}
}

func TestExtractArgs_SafeFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Run: func(cmd *cobra.Command, args []string) {}}
	cmd.Flags().String("status", "", "")
	cmd.SetArgs([]string{"--status", "TODO"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["status"]; !ok {
		t.Error("expected 'status' key in args")
	} else if v != "TODO" {
		t.Errorf("args[status] = %v, want TODO", v)
	}
}

func TestExtractArgs_ContentSensitiveFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Run: func(cmd *cobra.Command, args []string) {}}
	cmd.Flags().String("file", "", "")
	cmd.SetArgs([]string{"--file", "/secret/path/specs.yaml"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["file"]; !ok {
		t.Error("expected 'file' key in args")
	} else if v != true {
		t.Errorf("args[file] = %v (%T), want true bool", v, v)
	}
}

func TestExtractArgs_PositionalArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use: "show US-XXX",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.SetArgs([]string{"US-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["_0"]; !ok {
		t.Error("expected '_0' key in args")
	} else if v != "US-001" {
		t.Errorf("args[_0] = %v, want US-001", v)
	}
}

func TestExtractArgs_Mixed(t *testing.T) {
	cmd := &cobra.Command{
		Use: "plan US-XXX",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("status", "", "")
	cmd.SetArgs([]string{"--file", "/tmp/plan.yaml", "--status", "PLANNED", "US-005"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	// Content-sensitive flag records presence only.
	if v, ok := args["file"]; !ok {
		t.Error("expected 'file' key in args")
	} else if v != true {
		t.Errorf("args[file] = %v (%T), want true", v, v)
	}
	// Safe flag records actual value.
	if v, ok := args["status"]; !ok {
		t.Error("expected 'status' key in args")
	} else if v != "PLANNED" {
		t.Errorf("args[status] = %v, want PLANNED", v)
	}
	// Positional arg.
	if v, ok := args["_0"]; !ok {
		t.Error("expected '_0' key in args")
	} else if v != "US-005" {
		t.Errorf("args[_0] = %v, want US-005", v)
	}
}

func TestExtractArgs_StringSlice(t *testing.T) {
	cmd := &cobra.Command{
		Use: "init",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().StringSlice("tool", nil, "")
	cmd.SetArgs([]string{"--tool", "claude,pi"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["tool"]; !ok {
		t.Error("expected 'tool' key in args")
	} else {
		slice, ok := v.([]string)
		if !ok {
			t.Fatalf("args[tool] = %v (%T), want []string", v, v)
		}
		if len(slice) != 2 || slice[0] != "claude" || slice[1] != "pi" {
			t.Errorf("args[tool] = %v, want [claude pi]", slice)
		}
	}
}

func TestExtractArgs_BoolFlag(t *testing.T) {
	cmd := &cobra.Command{
		Use: "update",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().Bool("check", false, "")
	cmd.SetArgs([]string{"--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["check"]; !ok {
		t.Error("expected 'check' key in args")
	} else if v != "true" {
		// pflag bool values serialize as "true"/"false" strings.
		t.Errorf("args[check] = %v, want 'true'", v)
	}
}

func TestExtractArgs_CommitSummaryPresence(t *testing.T) {
	cmd := &cobra.Command{
		Use: "review US-XXX",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().String("commit-summary", "", "")
	cmd.Flags().String("commit-type", "", "")
	cmd.SetArgs([]string{"--commit-summary", "Fix login bug", "--commit-type", "fix", "US-003"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	// commit-summary is content-sensitive → presence only.
	if v, ok := args["commit-summary"]; !ok {
		t.Error("expected 'commit-summary' key in args")
	} else if v != true {
		t.Errorf("args[commit-summary] = %v, want true", v)
	}
	// commit-type is safe → actual value.
	if v, ok := args["commit-type"]; !ok {
		t.Error("expected 'commit-type' key in args")
	} else if v != "fix" {
		t.Errorf("args[commit-type] = %v, want fix", v)
	}
	// Positional spec code.
	if v, ok := args["_0"]; !ok {
		t.Error("expected '_0' key in args")
	} else if v != "US-003" {
		t.Errorf("args[_0] = %v, want US-003", v)
	}
}

func TestExtractArgs_IntFlag(t *testing.T) {
	cmd := &cobra.Command{
		Use: "view",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().Int("port", 8080, "")
	cmd.SetArgs([]string{"--port", "3000"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["port"]; !ok {
		t.Error("expected 'port' key in args")
	} else if v != "3000" {
		t.Errorf("args[port] = %v, want '3000'", v)
	}
}

func TestExtractArgs_MultiplePositionals(t *testing.T) {
	cmd := &cobra.Command{
		Use: "done US-XXX TASK-NN",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.SetArgs([]string{"US-001", "TASK-01"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	args := extractArgs(cmd)
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if v, ok := args["_0"]; !ok || v != "US-001" {
		t.Errorf("args[_0] = %v, want US-001", v)
	}
	if v, ok := args["_1"]; !ok || v != "TASK-01" {
		t.Errorf("args[_1] = %v, want TASK-01", v)
	}
}
