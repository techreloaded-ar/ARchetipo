package codex

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

func (*Provider) PermissionProfiles() []execution.PermissionProfile {
	return []execution.PermissionProfile{
		{ID: "provider_default", Label: "Impostazioni del workspace", Description: "Usa il sandbox scelto nelle impostazioni del workspace. Chiede conferma per le operazioni che richiedono permessi aggiuntivi."},
		{ID: "workspace", Label: "Progetto · con conferme", Description: "Modifica i file nel progetto e nelle cartelle temporanee. Chiede conferma per uscire da questi limiti o usare la rete dai comandi. Può leggere anche fuori dal progetto."},
		{ID: "workspace_no_prompt", Label: "Progetto · senza conferme", Description: "Modifica i file nel progetto e nelle cartelle temporanee senza chiedere conferma. Le operazioni fuori da questi limiti e la rete dei comandi restano bloccate. Può leggere anche fuori dal progetto."},
		{ID: "full_access", Label: "Accesso completo", Description: "Può modificare file anche fuori dal progetto e usare la rete. I comandi vengono eseguiti senza sandbox e senza chiedere conferma."},
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
