package codex

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

func (*Provider) PermissionProfiles() []execution.PermissionProfile {
	return []execution.PermissionProfile{
		{ID: "provider_default", Label: "Permessi configurati", Description: "Usa il sandbox configurato; chiede quando occorre superarne i limiti."},
		{ID: "workspace", Label: "Autonomia nel progetto", Description: "Scrive nel progetto e nelle directory temporanee; esegue comandi locali e chiede per superare i limiti. Può leggere fuori dal progetto. La rete dei comandi è limitata."},
		{ID: "workspace_no_prompt", Label: "Progetto senza conferme", Description: "Mantiene il sandbox del progetto e delle directory temporanee senza conferme: le operazioni oltre i limiti vengono bloccate. Può leggere fuori dal progetto. La rete dei comandi è limitata."},
		{ID: "full_access", Label: "Accesso completo", Description: "Nessun sandbox per file o rete e nessuna conferma dei comandi. Accesso anche fuori dal progetto."},
	}
}

// nativePermissions never changes the batch configuration. An empty profile
// uses the configured default, not the previous turn's elevated policy.
func nativePermissions(cfg settings, selected string) (profile, sandbox, approval string, err error) {
	profile = selected
	if profile == "" {
		profile = cfg.PermissionProfile
	}
	if profile == "" {
		profile = "provider_default"
	}
	if err = execution.ValidatePermissionProfile((&Provider{}).PermissionProfiles(), profile); err != nil {
		return
	}
	sandbox, approval = cfg.Sandbox, "on-request"
	switch profile {
	case "workspace":
		sandbox = "workspace-write"
	case "workspace_no_prompt":
		sandbox, approval = "workspace-write", "never"
	case "full_access":
		sandbox, approval = "danger-full-access", "never"
	}
	return
}

func nativeSandboxPolicy(mode, dir string) map[string]any {
	switch mode {
	case "danger-full-access":
		return map[string]any{"type": "dangerFullAccess"}
	case "read-only":
		return map[string]any{"type": "readOnly"}
	default:
		return map[string]any{"type": "workspaceWrite", "writableRoots": []string{dir}, "networkAccess": false, "excludeTmpdirEnvVar": false, "excludeSlashTmp": false}
	}
}

func defaultNativePermissionProfile(cfg settings) string {
	profile, _, _, _ := nativePermissions(cfg, "")
	return profile
}
