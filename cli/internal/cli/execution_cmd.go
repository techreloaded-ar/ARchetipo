package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"gopkg.in/yaml.v3"
)

type executionProviderSelection struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config"`
}

type executionDependencies struct {
	registry     *execution.Registry
	newID        execution.IDGenerator
	now          execution.Clock
	storeFactory func(string) (execution.Store, error)
}

func newExecutionCmd(s streams, deps executionDependencies) *cobra.Command {
	root := &cobra.Command{Use: "execution", Short: "Execution provider operations"}
	root.AddCommand(newExecutionRunCmd(s, deps), newExecutionShowCmd(s, deps), newExecutionProviderCmd(s, deps))
	return root
}

func newExecutionProviderCmd(s streams, deps executionDependencies) *cobra.Command {
	root := &cobra.Command{Use: "provider", Short: "Configure the workspace execution provider"}
	root.AddCommand(newExecutionProviderSetDefaultCmd(s, deps), newExecutionProviderShowDefaultCmd(s))
	return root
}

func newExecutionProviderSetDefaultCmd(s streams, deps executionDependencies) *cobra.Command {
	var inputPath string
	cmd := &cobra.Command{
		Use: "set-default id", Short: "Validate and save the default workspace provider", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return errInvalidUsage("execution.default_provider.id is required", "pass execution provider set-default <id> --file <path>")
			}
			if strings.TrimSpace(inputPath) == "" {
				return errInvalidUsage("execution.default_provider.config requires --file", "pass a JSON or YAML mapping with --file <path>")
			}
			body, err := os.ReadFile(inputPath)
			if err != nil {
				return iox.NewInvalidInput("reading execution.default_provider.config", "pass a readable JSON or YAML mapping file", err)
			}
			providerConfig, err := decodeProviderConfig(body)
			if err != nil {
				return iox.NewInvalidInput("invalid execution.default_provider.config", "the file root must be a JSON or YAML mapping", err)
			}
			id := strings.TrimSpace(args[0])
			provider, err := deps.registry.Resolve(id)
			if err != nil {
				return iox.NewInvalidInput("invalid execution.default_provider.id", "register the requested provider before selecting it", err)
			}
			if err := provider.ValidateConfig(cmd.Context(), execution.CloneConfig(providerConfig)); err != nil {
				return mapProviderConfigurationError(err, defaultProviderConfigPath)
			}
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			selection := config.DefaultProviderConfig{ID: id, Config: execution.CloneConfig(providerConfig)}
			if _, err := config.UpdateDefaultProvider(cfg.ProjectRoot, selection); err != nil {
				return iox.NewInternal("saving execution.default_provider", err)
			}
			return writeExecutionProvider(s, cfg, selection)
		},
	}
	cmd.Flags().StringVar(&inputPath, "file", "", "JSON or YAML provider configuration mapping (required)")
	return cmd
}

func decodeProviderConfig(body []byte) (map[string]any, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("provider config root is not a mapping")
	}
	config := map[string]any{}
	if err := document.Content[0].Decode(&config); err != nil {
		return nil, err
	}
	return config, nil
}

func newExecutionProviderShowDefaultCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use: "show-default", Short: "Show the default workspace provider", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			if cfg.Execution.DefaultProvider == nil || strings.TrimSpace(cfg.Execution.DefaultProvider.ID) == "" {
				return iox.NewPrecondition("execution.default_provider is not configured", "run execution provider set-default <id> --file <path>", nil)
			}
			return writeExecutionProvider(s, cfg, *cfg.Execution.DefaultProvider)
		},
	}
}

func writeExecutionProvider(s streams, cfg config.Config, selection config.DefaultProviderConfig) error {
	data := executionProviderSelection{ID: selection.ID, Config: execution.CloneConfig(selection.Config)}
	if err := iox.WriteOKWithWarnings(s.out, "execution_provider", data, cfg.ResolutionNotices); err != nil {
		return iox.NewInternal("encoding output", err)
	}
	return nil
}

func newExecutionRunCmd(s streams, deps executionDependencies) *cobra.Command {
	var providerID string
	var requestID string
	cmd := &cobra.Command{
		Use:   "run US-XXX action",
		Short: "Run an action through the workspace default or an explicit execution provider",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errInvalidUsage("execution run requires a spec code and action", "pass execution run US-XXX plan --provider <id>")
			}
			specCode, action := strings.TrimSpace(args[0]), execution.ActionID(strings.TrimSpace(args[1]))
			if specCode == "" || action == "" {
				return errInvalidUsage("spec code and action are required", "pass execution run US-XXX plan [--provider <id>]")
			}
			if _, err := execution.RequiredCapability(action); err != nil {
				return mapExecutionError(err)
			}
			if cmd.Flags().Changed("request-id") && strings.TrimSpace(requestID) == "" {
				return errInvalidUsage("--request-id requires a non-empty value", "pass --request-id <key> or omit it")
			}
			return withConnectorCfg(cmd, s, "execution", func(ctx context.Context, cfg config.Config, conn connector.Connector) (any, error) {
				resolvedProviderID := strings.TrimSpace(providerID)
				providerConfig := map[string]any{}
				fromDefault := !cmd.Flags().Changed("provider")
				if fromDefault {
					if cfg.Execution.DefaultProvider == nil || strings.TrimSpace(cfg.Execution.DefaultProvider.ID) == "" {
						return nil, iox.NewPrecondition("execution.default_provider is not configured", "run execution provider set-default <id> --file <path>", nil)
					}
					resolvedProviderID = strings.TrimSpace(cfg.Execution.DefaultProvider.ID)
					providerConfig = execution.CloneConfig(cfg.Execution.DefaultProvider.Config)
				} else if resolvedProviderID == "" {
					return nil, errInvalidUsage("--provider requires a non-empty id", "pass --provider <id> or omit it to use the workspace default")
				}
				spec, err := conn.ReadSpecDetail(ctx, specCode)
				if err != nil {
					return nil, err
				}
				store, err := deps.storeFactory(cfg.ProjectRoot)
				if err != nil {
					return nil, iox.NewInternal("creating execution store", err)
				}
				service, err := execution.NewService(deps.registry, store, deps.newID, deps.now)
				if err != nil {
					return nil, iox.NewInternal("creating execution service", err)
				}
				var outcome execution.Execution
				reused := false
				if key := strings.TrimSpace(requestID); key != "" {
					outcome, reused, err = service.RunIdempotent(ctx, spec, action, resolvedProviderID, providerConfig, key)
				} else {
					outcome, err = service.Run(ctx, spec, action, resolvedProviderID, providerConfig)
				}
				if err != nil {
					return nil, mapExecutionRunError(err, fromDefault)
				}
				// A reused record is not re-verified: its effect was already
				// confirmed when it was created, and the spec has legitimately
				// moved on since then.
				if !reused {
					if err := confirmActionEffect(ctx, conn, store, action, specCode, &outcome); err != nil {
						return nil, err
					}
				}
				return executionRunResult{Execution: outcome, Reused: reused}, nil
			})
		},
	}
	cmd.Flags().StringVar(&providerID, "provider", "", "execution provider id (overrides the workspace default and runs with an empty configuration, so it only fits providers that need none)")
	cmd.Flags().StringVar(&requestID, "request-id", "", "idempotency key: repeating the same value returns the execution already created, in whatever state it is in")
	return cmd
}

// executionRunResult is the envelope payload of `execution run`. It inlines the
// record and adds whether it was reused: without that flag a repeated
// --request-id is indistinguishable from a fresh dispatch, and a record left
// RUNNING by an interrupted process would come back for ever with no sign that
// nothing new was started.
type executionRunResult struct {
	execution.Execution
	Reused bool `json:"reused"`
}

// confirmActionEffect renders execution.ConfirmActionEffect in the CLI's own
// envelope. The rule itself lives in the execution package, so the viewer
// applies the identical check instead of a second copy of it that would drift;
// only the remedy is local, because a CLI user retries with a new --request-id.
func confirmActionEffect(ctx context.Context, conn connector.Connector, store execution.Store, action execution.ActionID, specCode string, outcome *execution.Execution) error {
	err := execution.ConfirmActionEffect(ctx, conn, store, action, specCode, outcome)
	if err == nil {
		return nil
	}
	var unconfirmed *execution.UnconfirmedEffectError
	if errors.As(err, &unconfirmed) {
		return iox.NewPrecondition(
			unconfirmed.Error(),
			"inspect the remote run, then retry with a new --request-id once the cause is fixed",
			nil,
		)
	}
	return iox.NewInternal("recording the unconfirmed execution "+outcome.ID, err)
}

// defaultProviderConfigPath is the configuration path a provider field belongs
// to when the provider came from the workspace default.
const defaultProviderConfigPath = "execution.default_provider.config"

// explicitProviderConfigPath names the configuration of a provider selected
// with --provider. It is deliberately not execution.default_provider.config:
// that path is not where an explicit provider reads its configuration from.
const explicitProviderConfigPath = "--provider configuration"

// mapProviderConfigurationError renders a provider configuration rejection with
// the exact field that caused it. The path prefix is a parameter because the
// remedy differs: on the workspace-default path the user fixes
// execution.default_provider.config.<field>, while an explicit --provider
// receives no configuration at all, so pointing there would send the user to
// correct a field that is not the cause.
func mapProviderConfigurationError(err error, pathPrefix string) error {
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		field := strings.TrimSpace(configErr.Field)
		path := pathPrefix
		if field != "" {
			path += "." + field
		}
		if pathPrefix != defaultProviderConfigPath {
			return iox.NewInvalidInput("invalid "+path+": "+configErr.Error(), "--provider receives no configuration; configure the workspace default with execution provider set-default <id> --file <path>", err)
		}
		return iox.NewInvalidInput("invalid "+path+": "+configErr.Error(), "fix "+path+" and retry", err)
	}
	return iox.NewInvalidInput("invalid "+pathPrefix, "fix the provider configuration and retry", err)
}

func mapExecutionRunError(err error, fromDefault bool) error {
	if fromDefault {
		var registryErr *execution.RegistryError
		if errors.As(err, &registryErr) {
			return iox.NewInvalidInput("invalid execution.default_provider.id", "select a registered provider with execution provider set-default", err)
		}
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			return mapProviderConfigurationError(err, defaultProviderConfigPath)
		}
	}
	return mapExecutionError(err)
}

func newExecutionShowCmd(s streams, deps executionDependencies) *cobra.Command {
	return &cobra.Command{
		Use: "show execution-id", Short: "Read a persisted execution", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errInvalidUsage("execution show requires one execution id", "pass execution show <execution-id>")
			}
			id := strings.TrimSpace(args[0])
			if id == "" {
				return errInvalidUsage("execution id is required", "pass execution show <execution-id>")
			}
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			store, err := deps.storeFactory(cfg.ProjectRoot)
			if err != nil {
				return iox.NewInternal("creating execution store", err)
			}
			outcome, err := store.Get(cmd.Context(), id)
			if err != nil {
				return mapExecutionError(err)
			}
			if err := iox.WriteOKWithWarnings(s.out, "execution", outcome, cfg.ResolutionNotices); err != nil {
				return iox.NewInternal("encoding output", err)
			}
			return nil
		},
	}
}

func mapExecutionError(err error) error {
	var actionErr *execution.ActionError
	if errors.As(err, &actionErr) {
		return iox.NewInvalidInput(actionErr.Error(), "supported actions: plan", err)
	}
	var registryErr *execution.RegistryError
	if errors.As(err, &registryErr) {
		return iox.NewPrecondition(registryErr.Error(), "register the requested provider before running the action", err)
	}
	var capabilityErr *execution.CapabilityError
	if errors.As(err, &capabilityErr) {
		return iox.NewPrecondition(capabilityErr.Error(), fmt.Sprintf("provider must declare %s", capabilityErr.Capability), err)
	}
	var storeErr *execution.StoreError
	if errors.As(err, &storeErr) {
		switch storeErr.Kind {
		case execution.StoreNotFound:
			return iox.NewNotFound("execution not found: "+storeErr.ID, "pass an existing execution id", err)
		case execution.StoreInvalidID:
			return iox.NewInvalidInput("invalid execution id", "use the id returned by execution run", err)
		}
	}
	// Without this branch `--provider <id>` without configuration would surface
	// as a misleading E_INTERNAL instead of naming the missing field.
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		return mapProviderConfigurationError(err, explicitProviderConfigPath)
	}
	return iox.NewInternal("execution operation failed", err)
}
