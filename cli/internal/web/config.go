package web

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	_ "github.com/techreloaded-ar/ARchetipo/cli/internal/connector/builtin"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type configView struct {
	Config          config.Config `json:"config"`
	Raw             string        `json:"raw"`
	Path            string        `json:"path"`
	Exists          bool          `json:"exists"`
	RestartRequired bool          `json:"restart_required,omitempty"`
	BackupPath      string        `json:"backup_path,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

type saveConfigReq struct {
	Raw    string         `json:"raw,omitempty"`
	Config *config.Config `json:"config,omitempty"`
}

type configTestResult struct {
	OK        bool              `json:"ok"`
	Connector string            `json:"connector,omitempty"`
	Info      *domain.SetupInfo `json:"info,omitempty"`
	Warnings  []string          `json:"warnings,omitempty"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, raw, exists, path, err := readConfigState(s.cfg.ProjectRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configView{
		Config: cfg,
		Raw:    raw,
		Path:   path,
		Exists: exists,
	})
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	current, _, _, path, err := readConfigState(s.cfg.ProjectRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	var req saveConfigReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	raw, next, err := decodeConfigPayload(s.cfg.ProjectRoot, req)
	if err != nil {
		writeError(w, err)
		return
	}
	backupPath, err := config.SaveRaw(s.cfg.ProjectRoot, raw)
	if err != nil {
		writeError(w, iox.NewInvalidInput("invalid config", "fix the config and retry", err))
		return
	}
	writeJSON(w, http.StatusOK, configView{
		Config:          next,
		Raw:             string(raw),
		Path:            path,
		Exists:          true,
		RestartRequired: configChanged(current, next),
		BackupPath:      backupPath,
	})
}

func (s *Server) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var req saveConfigReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	_, next, err := decodeConfigPayload(s.cfg.ProjectRoot, req)
	if err != nil {
		writeError(w, err)
		return
	}
	result := configTestResult{OK: true, Connector: next.Connector}
	if next.Connector != config.ConnectorFile {
		writeJSON(w, http.StatusOK, configTestResult{
			OK:        true,
			Connector: next.Connector,
			Warnings: []string{
				fmt.Sprintf("full connector initialization for %q is skipped here because it may persist auto-detected metadata; save the config and restart `archetipo view` to verify it end-to-end", next.Connector),
			},
		})
		return
	}
	conn, err := connector.New(next)
	if err != nil {
		writeError(w, iox.NewInvalidInput("invalid config", "check `connector:` and connector-specific settings", err))
		return
	}
	info, err := conn.InitializeConnector(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	result.Info = &info
	writeJSON(w, http.StatusOK, result)
}

func readConfigState(root string) (cfg config.Config, raw string, exists bool, path string, err error) {
	raw, exists, path, err = config.ReadRaw(root)
	if err != nil {
		return config.Config{}, "", false, path, err
	}
	if !exists {
		cfg = config.Default()
		cfg.ProjectRoot = root
		rendered, renderErr := config.RenderFull(cfg)
		if renderErr != nil {
			return config.Config{}, "", false, path, iox.NewInternal("rendering default config", renderErr)
		}
		return cfg, string(rendered), false, path, nil
	}
	cfg, err = config.ValidateRaw(root, []byte(raw))
	if err != nil {
		return config.Config{}, "", false, path, iox.NewInvalidInput("current config is invalid", "fix .archetipo/config.yaml and reload the viewer", err)
	}
	return cfg, raw, true, path, nil
}

func decodeConfigPayload(root string, req saveConfigReq) ([]byte, config.Config, error) {
	hasRaw := strings.TrimSpace(req.Raw) != ""
	hasStructured := req.Config != nil
	if hasRaw == hasStructured {
		return nil, config.Config{}, iox.NewInvalidInput("provide exactly one of `raw` or `config`", "save guided form data as `config`, or advanced YAML as `raw`", nil)
	}
	var raw []byte
	if hasRaw {
		raw = []byte(req.Raw)
	} else {
		candidate := *req.Config
		candidate.ProjectRoot = root
		rendered, err := config.RenderFull(candidate)
		if err != nil {
			return nil, config.Config{}, iox.NewInvalidInput("invalid config", "could not serialize the guided form values", err)
		}
		raw = rendered
	}
	cfg, err := config.ValidateRaw(root, raw)
	if err != nil {
		return nil, config.Config{}, iox.NewInvalidInput("invalid config", "fix the highlighted YAML or guided form fields and retry", err)
	}
	if !connector.IsRegistered(cfg.Connector) {
		return nil, config.Config{}, iox.NewInvalidInput(
			fmt.Sprintf("unknown connector %q", cfg.Connector),
			fmt.Sprintf("use one of: %s", strings.Join(connector.RegisteredNames(), ", ")),
			nil,
		)
	}
	return raw, cfg, nil
}

func configChanged(before, after config.Config) bool {
	before.ProjectRoot = ""
	after.ProjectRoot = ""
	return !reflect.DeepEqual(before, after)
}
