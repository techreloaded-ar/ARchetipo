package execution

import "fmt"

// PermissionProfile is a provider-owned policy, not a promise that all
// providers implement the same sandbox or approval mechanism.
type PermissionProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// PermissionDescriber is an optional static catalog for provider configuration.
// SessionDiscovery is the authority on profiles offered by a native session;
// its catalog may be a subset or may be discovered dynamically instead.
type PermissionDescriber interface {
	PermissionProfiles() []PermissionProfile
}

func ValidatePermissionProfile(profiles []PermissionProfile, id string) error {
	if id == "" {
		return nil
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("permission profile %q is not supported by this provider", id)
}

// PermissionConfigField lets the ordinary provider configuration form save a
// default with the same vocabulary the conversation uses.
func PermissionConfigField(profiles []PermissionProfile) ConfigField {
	choices := make([]ConfigChoice, len(profiles))
	for i, profile := range profiles {
		choices[i] = ConfigChoice(profile)
	}
	return ConfigField{Name: "permission_profile", Label: "Permessi delle nuove conversazioni", Type: "select",
		Help: "Default delle nuove conversazioni. Le conversazioni esistenti mantengono la propria scelta.", Choices: choices}
}
