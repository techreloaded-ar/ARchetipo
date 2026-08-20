package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

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
				providerView.ModelsUnavailableReason = "the provider declared an empty model catalog"
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
