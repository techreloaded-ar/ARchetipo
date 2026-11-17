package verification

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/services"
)

// serviceExecutable is the binary that performs service operations; tests
// replace it.
var serviceExecutable = os.Executable

// serviceCommand is the derived lifecycle command of a declared service. The
// journal keeps the plain program name: the running binary replaces it.
func serviceCommand(op string, timeout int) Command {
	return Command{Name: "service-" + op, Argv: []string{"archetipo", "execution", "environment", "_service", op}, TimeoutSeconds: timeout}
}

// Service start includes the first pull of the image.
const serviceStartTimeout = 900

// deriveServices turns each declared service into a managed resource with its
// own assigned port, ahead of the project's resources, which may need it.
func (r *Recipe) deriveServices() error {
	if len(r.Services) > 8 {
		return invalid("at most 8 services are supported")
	}
	for _, resource := range r.Resources {
		if resource.Service != nil {
			return invalid("resources cannot declare a service; use services")
		}
	}
	names := make([]string, 0, len(r.Services))
	for name := range r.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	exposed := map[string]string{}
	derived := []Resource{}
	for _, name := range names {
		spec := r.Services[name]
		if err := services.Validate(name, spec); err != nil {
			return invalid(err.Error())
		}
		for key := range spec.Expose {
			if other, taken := exposed[key]; taken {
				return invalid("services " + other + " and " + name + " expose the same variable " + key)
			}
			exposed[key] = name
		}
		copied := spec
		derived = append(derived, Resource{
			Name:    name,
			Start:   serviceCommand(services.OpStart, serviceStartTimeout),
			Probe:   serviceCommand(services.OpReady, 30),
			Stop:    serviceCommand(services.OpStop, 30),
			Stopped: serviceCommand(services.OpStopped, 30),
			Service: &copied,
		})
		r.Ports = append(r.Ports, services.PortKey(name))
	}
	r.Resources = append(derived, r.Resources...)
	return nil
}

// declared is the recipe as written, without what deriveServices added.
func (r Recipe) declared() Recipe {
	out := r
	out.Resources = nil
	for _, resource := range r.Resources {
		if resource.Service == nil {
			out.Resources = append(out.Resources, resource)
		}
	}
	derivedPorts := map[string]bool{}
	for name := range r.Services {
		derivedPorts[services.PortKey(name)] = true
	}
	out.Ports = nil
	for _, port := range r.Ports {
		if !derivedPorts[port] {
			out.Ports = append(out.Ports, port)
		}
	}
	return out
}

// serviceEnv is what a service operation receives besides the resource
// identity: the declaration it acts on.
func serviceEnv(resource Resource) ([]string, error) {
	raw, err := json.Marshal(resource.Service)
	if err != nil {
		return nil, err
	}
	return []string{services.EnvSpec + "=" + string(raw)}, nil
}

// exposedEnv lists the variables the declared services of a journal expose.
func exposedEnv(state ResourceState) []string {
	env := []string{}
	for _, record := range state.Resources {
		if record.Resource.Service == nil {
			continue
		}
		port := state.Ports[services.PortKey(record.Resource.Name)]
		if port == "" {
			continue
		}
		env = append(env, services.Exposed(*record.Resource.Service, "127.0.0.1", port)...)
	}
	return env
}
