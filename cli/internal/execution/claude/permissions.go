package claude

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

func (*Provider) PermissionProfiles() []execution.PermissionProfile {
	return []execution.PermissionProfile{
		{ID: "provider_default", Label: "Impostazioni del workspace", Description: "Usa la modalità di permessi scelta nelle impostazioni del workspace. Non limita l’accesso ai file alla cartella del progetto."},
		{ID: "auto", Label: "Conferme gestite da Claude", Description: "Claude valuta quali operazioni autorizzare e quando chiedere conferma. I file accessibili non sono limitati alla cartella del progetto."},
		{ID: "accept_edits", Label: "Modifiche ai file autorizzate", Description: "Autorizza le modifiche ai file; i comandi possono richiedere conferma. I file accessibili non sono limitati alla cartella del progetto."},
		{ID: "full_access", Label: "Accesso completo", Description: "Esegue le operazioni senza chiedere conferma. L’accesso ai file può estendersi oltre la cartella del progetto."},
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
