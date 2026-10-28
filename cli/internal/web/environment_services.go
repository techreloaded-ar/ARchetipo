package web

import (
	"net/url"
	"sort"
	"strings"
)

// Presentation of explicit ownership and retained observations, never a process
// scan or a claim that a persisted resource state proves current liveness.
type environmentService struct {
	Name      string   `json:"name"`
	Directory string   `json:"directory"`
	State     string   `json:"state"`
	Owner     string   `json:"owner"`
	Endpoints []string `json:"endpoints,omitempty"`
}

func environmentServices(root string, view environmentPreviewView) []environmentService {
	services := []environmentService{}
	if view.Recipe != nil && len(view.Recipe.Argv) > 0 || view.Reused || view.Server != nil {
		state := map[string]string{"ready": "In esecuzione", "starting": "In avvio", "preparing": "In preparazione", "stopping": "In arresto", "stopped": "Fermo", "error": "Interrotto"}[view.State]
		if state == "" {
			state = "Stato non verificato"
		}
		owner := "Preview gestita da ARchetipo"
		if view.Reused {
			owner = "Server esterno: Ferma scollega solo la preview"
		}
		if view.Server != nil && !view.Reused && view.Server.State != "stopped" {
			if view.Server.OwnerActive && view.State == "stopped" {
				state = "In esecuzione · altro proprietario ARchetipo"
			} else if !view.Server.OwnerActive {
				owner = "Proprietario terminato · arresto da riconciliare"
			}
		}
		entry := environmentService{Name: "Applicazione", Directory: root, State: state, Owner: owner}
		if view.CleanupError != "" {
			entry.State = "Arresto non verificato: " + view.CleanupError
		}
		address := ""
		if view.Recipe != nil {
			address = view.Recipe.URL
		}
		if view.Server != nil && !view.Reused {
			address = view.Server.URL
		}
		if address != "" && !strings.Contains(address, "{port}") {
			if endpoint, err := url.Parse(address); err == nil {
				entry.Endpoints = []string{endpoint.Host}
			}
		}
		services = append(services, entry)
	}
	if view.Resources == nil {
		return services
	}
	for _, resource := range view.Resources.Resources {
		label := map[string]string{"ready": "Pronto", "starting": "In avvio", "stopped": "Fermo", "error": "Errore"}[resource.State]
		if label == "" {
			label = resource.State
		}
		services = append(services, environmentService{Name: resource.Resource.Name, Directory: root, State: label + " · ultimo stato registrato", Owner: "Risorsa gestita dalla ricetta ambiente"})
	}
	if len(view.Resources.Ports) > 0 {
		ports := []string{}
		for name, port := range view.Resources.Ports {
			ports = append(ports, name+": 127.0.0.1:"+port)
		}
		sort.Strings(ports)
		services = append(services, environmentService{Name: "Porte ambiente", Directory: root, State: "Assegnazioni registrate", Owner: "Ricetta ambiente", Endpoints: ports})
	}
	return services
}
