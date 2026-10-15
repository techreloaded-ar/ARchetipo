package claude

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

func (*Provider) PermissionProfiles() []execution.PermissionProfile {
	return []execution.PermissionProfile{
		{ID: "provider_default", Label: "Permessi configurati", Description: "Usa la modalità permission_mode configurata in Claude. Non impone un sandbox nella cartella."},
		{ID: "auto", Label: "Approvazione automatica Claude", Description: "Claude valuta le operazioni e può richiedere conferme. Non impone un sandbox nella cartella."},
		{ID: "accept_edits", Label: "Modifiche autorizzate", Description: "Claude autorizza le modifiche ai file; i comandi possono richiedere conferma. Non impone un sandbox nella cartella."},
		{ID: "full_access", Label: "Accesso completo", Description: "Disattiva i controlli di permesso Claude. Non limita l'accesso alla cartella del progetto."},
	}
}

func nativePermissionMode(cfg settings, selected string) (profile, mode string, err error) {
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
	mode = cfg.PermissionMode
	switch profile {
	case "auto":
		mode = "auto"
	case "accept_edits":
		mode = "acceptEdits"
	case "full_access":
		mode = "bypassPermissions"
	}
	return
}

func defaultNativePermissionProfile(cfg settings) string {
	profile, _, _ := nativePermissionMode(cfg, "")
	return profile
}
