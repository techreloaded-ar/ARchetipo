package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Unit is one declared service of one checkout.
type Unit struct {
	Name string
	Spec Spec
	// Port is the port of this checkout's service on 127.0.0.1.
	Port string
	// Owner is the resource owner of the current generation.
	Owner string
	// Environment identifies the checkout; names derive from it, so a
	// service's data survives a new generation of the same checkout.
	Environment string
	// State is the checkout's environment state directory.
	State string
	Log   io.Writer
}

// Backend starts and stops declared services. Every operation is idempotent:
// Stop works after a partial Start, Remove after a Stop.
type Backend interface {
	Start(ctx context.Context, unit Unit) error
	Ready(ctx context.Context, unit Unit) error
	Stop(ctx context.Context, unit Unit) error
	Stopped(ctx context.Context, unit Unit) error
	Remove(ctx context.Context, unit Unit) error
}

// Operations understood by Run.
const (
	OpStart   = "start"
	OpReady   = "ready"
	OpStop    = "stop"
	OpStopped = "stopped"
	OpRemove  = "remove"
)

// Variables the resource lifecycle passes to a service operation.
const (
	EnvSpec        = "ARCHETIPO_SERVICE_SPEC"
	EnvRuntime     = "ARCHETIPO_RUNTIME"
	envOwner       = "ARCHETIPO_RESOURCE_OWNER"
	envName        = "ARCHETIPO_RESOURCE_NAME"
	envEnvironment = "ARCHETIPO_ENVIRONMENT_ID"
	envState       = "ARCHETIPO_ENVIRONMENT_STATE"
)

// Select returns the backend of the runtime this process runs in: the
// container engine on a person's computer, installed runtimes inside the
// isolated environment.
func Select() Backend {
	if os.Getenv(EnvRuntime) == "isolated" {
		return NewProcess()
	}
	return NewDocker()
}

// UnitFromEnv reads the unit the resource lifecycle described in env.
func UnitFromEnv(getenv func(string) string, log io.Writer) (Unit, error) {
	unit := Unit{Name: getenv(envName), Owner: getenv(envOwner), Environment: getenv(envEnvironment), State: getenv(envState), Log: log}
	if err := json.Unmarshal([]byte(getenv(EnvSpec)), &unit.Spec); err != nil {
		return unit, fmt.Errorf("service specification missing or invalid: %w", err)
	}
	if err := Validate(unit.Name, unit.Spec); err != nil {
		return unit, err
	}
	unit.Port = getenv("ARCHETIPO_PORT_" + PortKey(unit.Name))
	if unit.Port == "" || unit.Owner == "" || unit.Environment == "" || unit.State == "" {
		return unit, errors.New("service operation started outside the environment resource lifecycle")
	}
	return unit, nil
}

// Run performs one operation on a unit.
func Run(ctx context.Context, backend Backend, op string, unit Unit) error {
	switch op {
	case OpStart:
		if err := backend.Start(ctx, unit); err != nil {
			return err
		}
		return backend.Ready(ctx, unit)
	case OpReady:
		// A probe answers promptly: a service that is down is restarted by
		// the lifecycle, not waited for.
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return backend.Ready(probeCtx, unit)
	case OpStop:
		return backend.Stop(ctx, unit)
	case OpStopped:
		return backend.Stopped(ctx, unit)
	case OpRemove:
		return backend.Remove(ctx, unit)
	}
	return fmt.Errorf("unknown service operation %q", op)
}

// resourceName is the name of the engine objects owned by a unit.
func resourceName(unit Unit) string {
	return "archetipo-svc-" + strings.TrimPrefix(unit.Environment, "env-") + "-" + unit.Name
}

func logf(unit Unit, format string, args ...any) {
	if unit.Log != nil {
		_, _ = fmt.Fprintf(unit.Log, format+"\n", args...)
	}
}
