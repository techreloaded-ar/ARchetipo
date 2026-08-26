package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// defaultSetupProvider is the provider `execution setup` configures when none
// is named. It is the only registered one that dispatches somewhere a person
// has to be told about; the local ones need a command name at most.
const defaultSetupProvider = "arcipelago"

// defaultTokenEnvName mirrors the provider's own default. It is repeated here
// only to name the variable in the instructions printed before any provider
// configuration exists to read it from.
const defaultTokenEnvName = "ARCIPELAGO_TOKEN"

type setupFlags struct {
	providerID     string
	url            string
	workspace      string
	tokenEnv       string
	pollInterval   int
	timeout        int
	nonInteractive bool
	noProbe        bool
}

// newExecutionSetupCmd builds `archetipo execution setup`.
//
// It exists because configuring a remote provider used to mean writing a YAML
// file by hand, from a schema documented nowhere, and passing it to
// `execution provider set-default --file`. Every value in that file except one
// is discoverable from the hub itself, and the one that is not — the
// credential — is the one thing that must never be written down.
//
// So this command asks for the hub, reads the rest, and writes the file. What
// it deliberately does *not* do is touch the secret: the token is read from the
// environment to be verified, and if it is absent the command says which line
// to export and stops. That keeps the rule the whole provider boundary is built
// on — secrets come from the environment, and the configuration names only the
// variable — true of the setup path too, and keeps the token out of this
// process, out of the config file, and out of the shell history.
func newExecutionSetupCmd(s streams, deps executionDependencies) *cobra.Command {
	flags := setupFlags{}
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure the workspace execution provider, verifying it against the hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExecutionSetup(cmd, s, deps, flags)
		},
	}
	cmd.Flags().StringVar(&flags.providerID, "provider", defaultSetupProvider, "provider to configure")
	cmd.Flags().StringVar(&flags.url, "url", "", "base URL of the ARcipelago hub")
	cmd.Flags().StringVar(&flags.workspace, "workspace", "", "workspace name or id to dispatch to")
	cmd.Flags().StringVar(&flags.tokenEnv, "token-env", "", "name of the variable holding the credential (default "+defaultTokenEnvName+")")
	cmd.Flags().IntVar(&flags.pollInterval, "poll-interval", 0, "seconds between polls of a remote run")
	cmd.Flags().IntVar(&flags.timeout, "timeout", 0, "seconds a remote run may take")
	cmd.Flags().BoolVar(&flags.nonInteractive, "non-interactive", false, "never prompt; fail naming the missing flag")
	cmd.Flags().BoolVar(&flags.noProbe, "no-probe", false, "write the configuration without contacting the hub")
	return cmd
}

func runExecutionSetup(cmd *cobra.Command, s streams, deps executionDependencies, flags setupFlags) error {
	ctx := cmd.Context()
	provider, err := deps.registry.Resolve(strings.TrimSpace(flags.providerID))
	if err != nil {
		return iox.NewInvalidInput("invalid --provider", "run `archetipo execution provider list` to see the registered providers", err)
	}
	cfg, err := loadConfigFor(cmd)
	if err != nil {
		return err
	}

	// An existing configuration is the default for every question, so
	// reconfiguring never means retyping what is already right.
	existing := map[string]any{}
	if cfg.Execution.DefaultProvider != nil && cfg.Execution.DefaultProvider.ID == provider.ID() {
		existing = execution.CloneConfig(cfg.Execution.DefaultProvider.Config)
	}

	baseURL, err := setupBaseURL(s, flags, existing)
	if err != nil {
		return err
	}
	tokenEnv := firstNonEmpty(flags.tokenEnv, stringField(existing, "token_env"), defaultTokenEnvName)

	candidate := map[string]any{"base_url": baseURL, "token_env": tokenEnv}
	carryInteger(existing, candidate, "poll_interval_seconds", flags.pollInterval)
	carryInteger(existing, candidate, "timeout_seconds", flags.timeout)

	workspaceID := firstNonEmpty(flags.workspace, stringField(existing, "workspace_id"))
	if !flags.noProbe {
		// The credential is verified, never stored. Absent, the command stops
		// here with the line to run: proceeding would write a configuration
		// nobody has checked, and the first failure would arrive much later.
		if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
			return errMissingSetupCredential(tokenEnv)
		}
		if err := execution.CheckAvailability(ctx, provider, candidate); err != nil {
			return iox.NewPrecondition("the execution provider is not reachable with this configuration", err.Error(), err)
		}
		workspaceID, err = setupWorkspace(ctx, s, provider, candidate, flags, workspaceID)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(workspaceID) == "" {
		return errInvalidUsage(
			"the workspace to dispatch to is not known",
			"pass --workspace <name-or-id>, or drop --no-probe so the hub can be asked",
		)
	}
	candidate["workspace_id"] = workspaceID

	if err := provider.ValidateConfig(ctx, execution.CloneConfig(candidate)); err != nil {
		return mapProviderConfigurationError(err, defaultProviderConfigPath)
	}
	selection := config.DefaultProviderConfig{ID: provider.ID(), Config: execution.CloneConfig(candidate)}
	if _, err := config.UpdateDefaultProvider(cfg.ProjectRoot, selection); err != nil {
		return iox.NewInternal("saving execution.default_provider", err)
	}
	writeSetupSummary(s, provider.ID(), tokenEnv, candidate)
	return writeExecutionProvider(s, cfg, selection)
}

// setupBaseURL answers where the hub is, preferring the flag, then what the
// project already had, then a question.
func setupBaseURL(s streams, flags setupFlags, existing map[string]any) (string, error) {
	if url := strings.TrimSpace(flags.url); url != "" {
		return url, nil
	}
	previous := stringField(existing, "base_url")
	if flags.nonInteractive {
		if previous != "" {
			return previous, nil
		}
		return "", errInvalidUsage("the hub URL is not known", "pass --url https://your-hub")
	}
	fmt.Fprintln(s.out)
	if previous != "" {
		fmt.Fprintf(s.out, "Hub URL [%s]: ", previous)
	} else {
		fmt.Fprint(s.out, "Hub URL (for example https://arcipelago.example): ")
	}
	line, err := readLine(s.in)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" && previous != "" {
		return previous, nil
	}
	if line == "" {
		return "", errInvalidUsage("the hub URL is required", "pass --url https://your-hub")
	}
	return line, nil
}

// setupWorkspace picks the destination: the named one, the only one, or the one
// a person chooses from the list.
func setupWorkspace(
	ctx context.Context,
	s streams,
	provider execution.Provider,
	candidate map[string]any,
	flags setupFlags,
	requested string,
) (string, error) {
	refs, declared, err := execution.DiscoverWorkspaces(ctx, provider, candidate)
	if err != nil {
		return "", iox.NewPrecondition("the workspaces of this hub could not be read", err.Error(), err)
	}
	if !declared {
		// A provider that dispatches nowhere in particular has no destination
		// to choose, and asking for one would be a question about nothing.
		return requested, nil
	}
	if len(refs) == 0 {
		return "", iox.NewPrecondition(
			"this credential is granted no workspace",
			"grant it one with `arcipelago apps grant <app> --workspace <name>`",
			nil,
		)
	}
	if requested != "" {
		return resolveWorkspaceRef(refs, requested)
	}
	if len(refs) == 1 {
		only := refs[0]
		fmt.Fprintf(s.out, "\nWorkspace: %s%s\n", only.Name, workspaceSuffix(only))
		fmt.Fprintln(s.out, "  (the only one this credential is granted, so it was chosen for you)")
		warnIfNotReady(s, only)
		return only.ID, nil
	}
	if flags.nonInteractive {
		return "", errInvalidUsage(
			fmt.Sprintf("this credential is granted %d workspaces", len(refs)),
			"pass --workspace <name-or-id>: "+strings.Join(workspaceNames(refs), ", "),
		)
	}

	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, "Select the workspace to dispatch to:")
	fmt.Fprintln(s.out)
	for i, ref := range refs {
		fmt.Fprintf(s.out, "  %d) %s%s\n", i+1, ref.Name, workspaceSuffix(ref))
	}
	fmt.Fprintln(s.out)
	fmt.Fprint(s.out, "Choice [1]: ")
	line, err := readLine(s.in)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}
	index, err := strconv.Atoi(line)
	if err != nil || index < 1 || index > len(refs) {
		return "", iox.NewInvalidInput("invalid selection: "+line, fmt.Sprintf("enter a number from 1 to %d", len(refs)), nil)
	}
	chosen := refs[index-1]
	warnIfNotReady(s, chosen)
	return chosen.ID, nil
}

// resolveWorkspaceRef matches by id, by exact name, then by id prefix, and
// refuses an ambiguous prefix rather than picking one: the value it returns is
// written to a file and used from then on.
func resolveWorkspaceRef(refs []execution.WorkspaceRef, requested string) (string, error) {
	for _, ref := range refs {
		if ref.ID == requested || ref.Name == requested {
			return ref.ID, nil
		}
	}
	matches := make([]execution.WorkspaceRef, 0, len(refs))
	for _, ref := range refs {
		if strings.HasPrefix(ref.ID, requested) {
			matches = append(matches, ref)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", iox.NewInvalidInput(
			fmt.Sprintf("no granted workspace matches %q", requested),
			"choose one of: "+strings.Join(workspaceNames(refs), ", "),
			nil,
		)
	default:
		return "", iox.NewInvalidInput(
			fmt.Sprintf("%q matches %d workspaces", requested, len(matches)),
			"be more specific: "+strings.Join(workspaceNames(matches), ", "),
			nil,
		)
	}
}

// warnIfNotReady says that work sent here would not run today, and does not
// stop. Blocking would force the hub to be finished before the project can be
// configured, and the two are done by different people at different times.
func warnIfNotReady(s streams, ref execution.WorkspaceRef) {
	if ref.Ready {
		return
	}
	fmt.Fprintf(s.err, "\nWarning: %s is not ready — %s.\n", ref.Name, ref.NotReadyReason)
	fmt.Fprintln(s.err, "The configuration is written anyway; work sent there waits until this is fixed.")
}

func workspaceSuffix(ref execution.WorkspaceRef) string {
	switch {
	case !ref.Ready && ref.NotReadyReason != "":
		return " — " + ref.NotReadyReason
	case ref.Detail != "":
		return " — " + ref.Detail
	default:
		return ""
	}
}

func workspaceNames(refs []execution.WorkspaceRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	return names
}

// errMissingSetupCredential is the one place that explains how to obtain a
// credential, because it is the one moment somebody discovers they need one.
func errMissingSetupCredential(tokenEnv string) error {
	return iox.NewPrecondition(
		"the execution credential is not exported in "+tokenEnv,
		"get one with `arcipelago apps create archetipo --workspace <name>` on the hub, then run "+
			"`export "+tokenEnv+"=<token>` in this shell; it is read from the environment and never written to any file",
		nil,
	)
}

func writeSetupSummary(s streams, providerID, tokenEnv string, candidate map[string]any) {
	fmt.Fprintln(s.err)
	fmt.Fprintf(s.err, "Wrote .archetipo/config.yaml — execution.default_provider: %s\n", providerID)
	fmt.Fprintf(s.err, "  hub        %s\n", stringField(candidate, "base_url"))
	fmt.Fprintf(s.err, "  workspace  %s\n", stringField(candidate, "workspace_id"))
	fmt.Fprintf(s.err, "  credential read from %s, never stored\n", tokenEnv)
	fmt.Fprintln(s.err)
	fmt.Fprintln(s.err, "Next:  archetipo doctor")
	fmt.Fprintln(s.err, "       archetipo execution run <SPEC> plan")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringField(config map[string]any, key string) string {
	if value, ok := config[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// carryInteger keeps a tuning value the project already had unless a flag names
// a new one. Neither present means the key is left out entirely, so the
// provider's own default applies and the file does not restate it.
func carryInteger(existing, candidate map[string]any, key string, flag int) {
	if flag > 0 {
		candidate[key] = flag
		return
	}
	if value, ok := existing[key]; ok {
		candidate[key] = value
	}
}
