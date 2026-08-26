package arcipelago

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// availabilityTimeout bounds the probe, and is deliberately not cfg.Timeout.
// That one bounds a remote run and defaults to an hour; borrowing it here would
// let a single unreachable hub hold the viewer's provider panel open for that
// long.
const availabilityTimeout = 5 * time.Second

const pathExternalMe = "/api/external/me"
const pathExternalWorkspaces = "/api/external/workspaces"

// identityEnvelope is the whoami of the external namespace
// (packages/hub/src/api/app.ts). It is the one call that validates the base URL
// and the credential together.
type identityEnvelope struct {
	Kind     string `json:"kind"`
	Identity struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		WorkspaceIDs []string `json:"workspaceIds"`
	} `json:"identity"`
}

type externalWorkspace struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	CwdHint         string   `json:"cwdHint"`
	Requirements    []string `json:"requirements"`
	Archived        bool     `json:"archived"`
	EligibleRunners struct {
		Known   int      `json:"known"`
		Online  int      `json:"online"`
		Missing []string `json:"missing"`
	} `json:"eligibleRunners"`
}

type workspacesEnvelope struct {
	Workspaces []externalWorkspace `json:"workspaces"`
}

// missingCredential is the one sentence this provider says when the token is
// not exported. It is a function and not two string literals because Execute
// and Available both have to say it: two copies would drift, and the one an
// operator reads would stop matching the one the panel shows.
func missingCredential(cfg settings) error {
	return fmt.Errorf("the ARcipelago application credential is not available: export it in the %s environment variable", cfg.TokenEnv)
}

// Available reports whether this provider could dispatch anything right now.
//
// The probe runs cheapest-first and stops at the first thing that is wrong, so
// the reason an operator reads is the first cause rather than a consequence of
// it: the shape of the configuration, then the credential, then the hub, then
// the grant, then whether any machine could take the work.
//
// It costs nothing for a provider that is not the workspace default. The viewer
// probes every registered provider when its panel opens, but passes a nil
// configuration to all but the current one, and parseConnection rejects that on
// base_url without touching the network.
//
// A fleet that is entirely asleep is deliberately *not* unavailable. A runner
// that is merely offline comes back, and the work would simply wait for it —
// reporting that as unavailable would tell an operator to fix something that
// is not broken.
func (p *Provider) Available(ctx context.Context, raw map[string]any) error {
	cfg, err := parseConnection(raw)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return missingCredential(cfg)
	}

	probeCtx, cancel := context.WithTimeout(ctx, availabilityTimeout)
	defer cancel()

	var identity identityEnvelope
	if _, err := p.do(probeCtx, cfg, token, http.MethodGet, pathExternalMe, nil, &identity); err != nil {
		return err
	}

	// Everything past this point needs a destination. A configuration that has
	// none is still usable — it is what `execution setup` holds while it is
	// working out which one to write — so the probe stops here rather than
	// inventing a failure.
	workspaceID, err := parseWorkspaceID(raw["workspace_id"])
	if err != nil {
		return nil
	}
	granted := false
	for _, id := range identity.Identity.WorkspaceIDs {
		if id == workspaceID {
			granted = true
			break
		}
	}
	if !granted {
		return fmt.Errorf(
			"the ARcipelago credential in %s is not granted workspace %q: grant it with `arcipelago apps grant <app> --workspace <name>`, or configure a workspace_id it holds",
			cfg.TokenEnv, workspaceID,
		)
	}

	workspaces, err := p.discoverWorkspaces(probeCtx, cfg, token)
	if err != nil {
		// An older hub does not serve this route. Everything that could be
		// checked has been, and refusing over a route that never existed would
		// make this provider unusable against a hub it can otherwise drive.
		return nil
	}
	for _, workspace := range workspaces {
		if workspace.ID != workspaceID {
			continue
		}
		if workspace.Archived {
			return fmt.Errorf("the ARcipelago workspace %q is archived and takes no new work", workspace.Name)
		}
		if workspace.EligibleRunners.Known == 0 {
			return fmt.Errorf(
				"no runner known to ARcipelago can host the work of workspace %q%s: provision one, or relax what the workspace requires",
				workspace.Name, missingSuffix(workspace.EligibleRunners.Missing),
			)
		}
	}
	return nil
}

var _ execution.AvailabilityReporter = (*Provider)(nil)

// missingSuffix names the capabilities nothing advertises, when the hub says
// which they are. An older hub answers without them, and a bare "no runner can
// host this" is still true.
func missingSuffix(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return " (nothing advertises " + strings.Join(missing, ", ") + ")"
}

// discoverWorkspaces reads the destinations this credential may use.
func (p *Provider) discoverWorkspaces(ctx context.Context, cfg settings, token string) ([]externalWorkspace, error) {
	var envelope workspacesEnvelope
	if _, err := p.do(ctx, cfg, token, http.MethodGet, pathExternalWorkspaces, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Workspaces, nil
}

// DiscoverWorkspaces lists the remote destinations this configuration could
// dispatch to, so a caller can offer them by name instead of asking somebody to
// paste an identifier.
//
// The configuration it takes needs base_url and token_env and nothing else:
// this is the call that finds the value of workspace_id, so requiring one would
// make the question unaskable until its answer was already known.
func (p *Provider) DiscoverWorkspaces(ctx context.Context, raw map[string]any) ([]execution.WorkspaceRef, error) {
	cfg, err := parseConnection(raw)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return nil, missingCredential(cfg)
	}
	discoverCtx, cancel := context.WithTimeout(ctx, availabilityTimeout)
	defer cancel()

	workspaces, err := p.discoverWorkspaces(discoverCtx, cfg, token)
	if err != nil {
		return nil, err
	}
	refs := make([]execution.WorkspaceRef, 0, len(workspaces))
	for _, workspace := range workspaces {
		refs = append(refs, execution.WorkspaceRef{
			ID:             workspace.ID,
			Name:           workspace.Name,
			Detail:         workspaceDetail(workspace),
			Ready:          !workspace.Archived && workspace.EligibleRunners.Known > 0,
			NotReadyReason: notReadyReason(workspace),
		})
	}
	return refs, nil
}

var _ execution.WorkspaceDiscoverer = (*Provider)(nil)

// workspaceDetail is the line shown beside the name: where the work lands, and
// how much of the fleet is awake to take it.
func workspaceDetail(workspace externalWorkspace) string {
	parts := make([]string, 0, 2)
	if cwd := strings.TrimSpace(workspace.CwdHint); cwd != "" {
		parts = append(parts, "runs in "+cwd)
	}
	switch {
	case workspace.EligibleRunners.Online == 1:
		parts = append(parts, "1 runner online")
	case workspace.EligibleRunners.Online > 1:
		parts = append(parts, fmt.Sprintf("%d runners online", workspace.EligibleRunners.Online))
	}
	return strings.Join(parts, ", ")
}

// notReadyReason says why work sent here would not run today, and stays empty
// when it would. A fleet that is only asleep is not a reason: it wakes up.
func notReadyReason(workspace externalWorkspace) string {
	switch {
	case workspace.Archived:
		return "archived, and takes no new work"
	case workspace.EligibleRunners.Known == 0:
		return "no runner can host its work" + missingSuffix(workspace.EligibleRunners.Missing)
	default:
		return ""
	}
}
