package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// emptyModelCatalogReason is what a reader is told when a provider declares a
// catalog and then hands back no entries at all. It lives in one place because
// both the provider list and the model-choice route report the very same fact,
// and a reader comparing the two panels must not be told it in two wordings.
const emptyModelCatalogReason = "the provider declared an empty model catalog"

// executionProviderView describes one registered provider to the viewer.
// Capabilities and ConfigFields are what let the browser render a form for a
// provider it knows nothing about; no field of this view can carry a secret,
// because provider configuration holds non-secret settings only.
type executionProviderView struct {
	ID           string                  `json:"id"`
	Label        string                  `json:"label"`
	Capabilities []execution.Capability  `json:"capabilities"`
	ConfigFields []execution.ConfigField `json:"config_fields"`
	// Available says whether the provider's runtime can be used right now, and
	// UnavailableReason says why not. A provider that does not report
	// availability has nothing that can be missing, so it stays available with
	// no reason. Neither field can carry a secret: the reason is the provider's
	// own diagnostic about a missing or unauthenticated runtime.
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	// ModelField is the name of the configuration field a model catalog fills
	// in. It is empty when the provider declares no catalog, which is what
	// tells the browser to keep rendering that field as free text.
	ModelField string `json:"model_field,omitempty"`
	// Models are the catalog entries, exactly as the provider declared them.
	Models []execution.ModelOption `json:"models,omitempty"`
	// ModelsUnavailableReason says, in the provider's own words, why the
	// catalog could not be obtained. A provider that declares a catalog always
	// carries either Models or this reason, never both and never neither, so
	// an empty list can never reach the reader unexplained. Neither field can
	// carry a secret: they are model identifiers and the provider's own
	// diagnostic.
	ModelsUnavailableReason string `json:"models_unavailable_reason,omitempty"`
}

// executionProviderSelectionView is the persisted workspace default.
type executionProviderSelectionView struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config"`
}

type executionProvidersView struct {
	Providers []executionProviderView         `json:"providers"`
	Default   *executionProviderSelectionView `json:"default"`
}

// handleListExecutionProviders answers "which providers can this workspace
// choose from, and which one is chosen now?".
func (s *Server) handleListExecutionProviders(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	view := executionProvidersView{Providers: []executionProviderView{}}
	// The default is read from disk rather than from the config the server
	// started with: it changes while the viewer runs, and the panel must show
	// what is persisted, not what was loaded at boot. It is read before the
	// provider loop because only the current default has a persisted
	// configuration to probe with.
	current, _, _, _, err := readConfigState(ws.cfg.ProjectRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	selection := current.Execution.DefaultProvider
	defaultID := ""
	if selection != nil {
		defaultID = strings.TrimSpace(selection.ID)
	}
	for _, provider := range s.registry.List() {
		// Derived, not asked for directly: DeclaredCapabilities adds the
		// dialogue capability for a provider that really implements the
		// interactive-run interface, so the panel cannot advertise a
		// conversation the provider cannot hold.
		normalized, err := execution.DeclaredCapabilities(r.Context(), provider)
		if err != nil {
			writeError(w, iox.NewInternal("reading capabilities of provider "+provider.ID(), err))
			return
		}
		// Only the current default is probed with the persisted configuration;
		// every other provider is probed with nil and applies its own defaults,
		// because a configuration saved for one provider means nothing to
		// another. An unusable runtime is a fact to report, never an HTTP
		// error, so the probe result only fills two fields of the view.
		var providerConfig map[string]any
		if defaultID != "" && provider.ID() == defaultID && selection != nil {
			providerConfig = selection.Config
		}
		reason := ""
		if err := execution.CheckAvailability(r.Context(), provider, providerConfig); err != nil {
			reason = err.Error()
		}
		providerView := executionProviderView{
			ID:                provider.ID(),
			Label:             provider.ID(),
			Capabilities:      normalized,
			ConfigFields:      execution.DescribeConfig(provider),
			Available:         reason == "",
			UnavailableReason: reason,
		}
		// Only a provider that declares a catalog gets the three model fields;
		// for every other provider they stay zero and never reach the wire.
		if _, declaresModels := provider.(execution.ModelLister); declaresModels {
			providerView.ModelField = execution.ModelFieldName
			switch {
			case reason != "":
				// A runtime that cannot answer the availability probe is
				// already the answer; asking it for a catalog would only spawn
				// a second process every time the panel is opened.
				providerView.ModelsUnavailableReason = reason
			default:
				// Listed with the very configuration the probe used, so the
				// catalog describes the runtime that was just checked.
				models, _, err := execution.ListModels(r.Context(), provider, providerConfig)
				if err != nil {
					providerView.ModelsUnavailableReason = err.Error()
				} else {
					providerView.Models = models
				}
			}
			if len(providerView.Models) == 0 && providerView.ModelsUnavailableReason == "" {
				providerView.ModelsUnavailableReason = emptyModelCatalogReason
			}
		}
		view.Providers = append(view.Providers, providerView)
	}
	if defaultID != "" {
		view.Default = &executionProviderSelectionView{
			ID:     defaultID,
			Config: execution.CloneConfig(selection.Config),
		}
	}
	writeJSON(w, http.StatusOK, view)
}

// executionModelChoiceView answers, before anything is started, "which model
// will this run use, where does that model come from, and what else could I
// pick for this one run?".
//
// Available says only whether *choosing* is possible now; it never says whether
// a run can start. Model, ModelSource and Options are therefore filled in every
// case, including the three unavailable ones, because a run started without a
// choice still uses the configured model and the reader has to see it.
type executionModelChoiceView struct {
	// ProviderID is the workspace default provider, empty when none is set.
	ProviderID string `json:"provider_id"`
	// ModelField is the configuration field a catalog fills in; empty when the
	// provider declares no catalog.
	ModelField string `json:"model_field,omitempty"`
	// Model is the model the run would use, and ModelSource says where it comes
	// from — a workspace configuration or a per-run choice.
	Model       string            `json:"model"`
	ModelSource string            `json:"model_source"`
	Options     map[string]string `json:"options,omitempty"`
	// Models are the alternatives, exactly as the provider declared them.
	Models []execution.ModelOption `json:"models,omitempty"`
	// Available is true only when the catalog is declared, obtainable and not
	// empty; otherwise UnavailableReason says which of the three it is.
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// handleGetExecutionModelChoice reports the model the workspace default
// provider would use and the catalog a single run may choose from.
//
// It never fails on state: a missing default provider, an unregistered
// provider, an unusable runtime and a missing catalog are all facts to report,
// so each of them answers 200 with available:false and a reason, and none of
// them is an HTTP error.
func (s *Server) handleGetExecutionModelChoice(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	// Read from disk, not from the config the server booted with: the Execution
	// panel can change the default while the viewer runs.
	current, _, _, _, err := readConfigState(ws.cfg.ProjectRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	view := executionModelChoiceView{ModelSource: execution.ModelChoiceSourceWorkspace}
	selection := current.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		view.UnavailableReason = "execution.default_provider is not configured"
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.ProviderID = strings.TrimSpace(selection.ID)
	if s.registry == nil {
		view.UnavailableReason = "no execution provider is registered in this viewer"
		writeJSON(w, http.StatusOK, view)
		return
	}
	provider, err := s.registry.Resolve(view.ProviderID)
	if err != nil {
		view.UnavailableReason = err.Error()
		writeJSON(w, http.StatusOK, view)
		return
	}
	// A runtime that cannot answer the availability probe is already the
	// answer; asking it for a catalog would only spawn a second process, the
	// same economy handleListExecutionProviders already applies.
	if err := execution.CheckAvailability(r.Context(), provider, selection.Config); err != nil {
		view.UnavailableReason = err.Error()
		writeJSON(w, http.StatusOK, view)
		return
	}

	resolution := execution.ResolveModelChoice(r.Context(), provider, selection.Config)
	view.Model = resolution.Choice.Model
	view.ModelSource = resolution.Choice.Source
	view.Options = resolution.Choice.Options
	switch {
	case !resolution.Declared:
		view.UnavailableReason = "provider " + provider.ID() + " declares no model catalog"
	case resolution.Reason != "":
		view.ModelField = execution.ModelFieldName
		view.UnavailableReason = resolution.Reason
	case len(resolution.Models) == 0:
		view.ModelField = execution.ModelFieldName
		view.UnavailableReason = emptyModelCatalogReason
	default:
		view.ModelField = execution.ModelFieldName
		view.Models = resolution.Models
		view.Available = true
	}
	writeJSON(w, http.StatusOK, view)
}

type saveDefaultProviderReq struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config"`
}

// handleSaveDefaultExecutionProvider runs the same sequence as `archetipo
// execution provider set-default`: resolve the provider, let the provider
// validate its own configuration, then update only the execution block of the
// config file. Nothing is written when validation fails, which is what keeps
// the previously valid selection intact after a rejection.
func (s *Server) handleSaveDefaultExecutionProvider(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	var req saveDefaultProviderReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, iox.NewInvalidInput("execution.default_provider.id is required", "pick one of the registered providers", nil))
		return
	}
	provider, err := s.registry.Resolve(id)
	if err != nil {
		writeError(w, iox.NewInvalidInput("invalid execution.default_provider.id: "+id, "pick one of the registered providers", err))
		return
	}
	providerConfig := execution.CloneConfig(req.Config)
	if err := provider.ValidateConfig(r.Context(), execution.CloneConfig(providerConfig)); err != nil {
		writeProviderConfigError(w, err)
		return
	}
	selection := config.DefaultProviderConfig{ID: id, Config: providerConfig}
	if _, err := config.UpdateDefaultProvider(ws.cfg.ProjectRoot, selection); err != nil {
		writeError(w, iox.NewInternal("saving execution.default_provider", err))
		return
	}
	writeJSON(w, http.StatusOK, executionProviderSelectionView{
		ID:     selection.ID,
		Config: execution.CloneConfig(selection.Config),
	})
}

// resolveRunModelChoice merges the per-run model choice carried by a start
// request onto the configuration saved for the workspace default provider, and
// returns the configuration the run must use together with the choice it
// represents.
//
// It is shared by the two start routes on purpose: the spec-scoped and the
// workspace-scoped start must accept the very same two fields, refuse the very
// same mistakes and leave the saved configuration untouched in exactly the same
// way, and a second copy of this logic is the only way that could stop being
// true. Nothing here writes .archetipo/config.yaml: persisted is read, cloned
// and merged, never saved.
//
// The returned choice is non-nil whenever the provider declares a catalog —
// including the no-override case, where it is the workspace choice, so the
// record of a run started without choosing still says which model it used. It
// is nil only when the provider declares no catalog and no override was asked
// for: there is nothing to report and nothing to refuse.
func resolveRunModelChoice(ctx context.Context, provider execution.Provider, persisted map[string]any, model string, options map[string]string) (map[string]any, *execution.ModelChoice, error) {
	if strings.TrimSpace(model) == "" && len(options) == 0 {
		// No override: the configuration travels verbatim, so a start without
		// the two fields is byte-for-byte the start of before this spec.
		if provider == nil {
			return execution.CloneConfig(persisted), nil, nil
		}
		resolution := execution.ResolveModelChoice(ctx, provider, persisted)
		if !resolution.Declared {
			return execution.CloneConfig(persisted), nil, nil
		}
		choice := resolution.Choice
		return execution.CloneConfig(persisted), &choice, nil
	}
	if provider == nil {
		return nil, nil, &execution.ModelChoiceUnavailableError{
			Reason: "no execution provider is registered in this viewer",
		}
	}
	effective, choice, err := execution.ApplyModelChoice(ctx, provider, persisted, model, options)
	if err != nil {
		return nil, nil, err
	}
	return effective, &choice, nil
}

// writeRunModelChoiceError keeps apart the two ways a per-run choice can fail,
// because they are two different mistakes: a model or an option value the
// catalog does not admit is invalid input (400, naming the offending field),
// while a catalog that cannot be consulted at all is a state conflict (409) the
// user gets past by starting without a choice.
func writeRunModelChoiceError(w http.ResponseWriter, err error) {
	var unavailable *execution.ModelChoiceUnavailableError
	if errors.As(err, &unavailable) {
		writeError(w, iox.NewConflict(
			unavailable.Reason,
			"start without a model choice to use the configured model",
			err,
		))
		return
	}
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		writeProviderConfigError(w, err)
		return
	}
	writeError(w, iox.NewInternal("resolving the model choice of this run", err))
}

// wrapRunModelChoiceError is the error-returning half of
// writeRunModelChoiceError: it keeps the two typed failures a per-run choice can
// produce exactly as they are — so the single response renderer can still tell
// them apart — and gives the same generic wrapping writeRunModelChoiceError
// applies to everything else. It exists because the start sequence resolves the
// choice far from the ResponseWriter, and the diagnostic must not degrade on the
// way back.
func wrapRunModelChoiceError(err error) error {
	var unavailable *execution.ModelChoiceUnavailableError
	if errors.As(err, &unavailable) {
		return err
	}
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		return err
	}
	return iox.NewInternal("resolving the model choice of this run", err)
}

// writeStartError is the only place an error raised while starting an execution
// becomes an HTTP response. Both start doors — the board and, once it exists,
// the confirmation of an action proposed in a conversation — go through it, so
// the same refusal can never come back worded or coded differently depending on
// which door was pressed.
func writeStartError(w http.ResponseWriter, err error) {
	var unavailable *execution.ModelChoiceUnavailableError
	if errors.As(err, &unavailable) {
		writeRunModelChoiceError(w, err)
		return
	}
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		writeProviderConfigError(w, err)
		return
	}
	writeError(w, err)
}

// writeProviderConfigError renders a rejected provider configuration with the
// exact offending key in `field`, so the form can point at the input the user
// has to fix instead of showing a message about a form it cannot locate.
func writeProviderConfigError(w http.ResponseWriter, err error) {
	message := "invalid execution.default_provider.config"
	field := ""
	var configErr *execution.ConfigurationError
	if errors.As(err, &configErr) {
		field = strings.TrimSpace(configErr.Field)
		message = configErr.Error()
	}
	payload := map[string]any{
		"error": message,
		"code":  iox.CodeInvalidInput,
		"hint":  "fix the highlighted field and save again",
	}
	if field != "" {
		payload["field"] = field
	}
	writeJSON(w, http.StatusBadRequest, payload)
}
