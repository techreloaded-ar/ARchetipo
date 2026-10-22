package verification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

// ResourceState is a recovery journal, written BEFORE every external mutation.
// It retains the original adapters even if the project recipe changes or vanishes.
// It deliberately contains no inherited environment or credentials.
type ResourceState struct {
	Version   int               `json:"version"`
	Owner     string            `json:"owner"`
	Ports     map[string]string `json:"ports"`
	Resources []ResourceRecord  `json:"resources"`
}
type ResourceRecord struct {
	Resource Resource `json:"resource"`
	State    string   `json:"state"`
	Error    string   `json:"error,omitempty"`
}

func (s *Service) Resources() (ResourceState, error) {
	var state ResourceState
	raw, err := os.ReadFile(filepath.Join(s.dir(), "resources.json"))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(raw, &state); err != nil || state.Version != 1 || state.Owner == "" {
		return state, iox.NewPrecondition("invalid resource journal", "preserve resources.json and reconcile ownership before starting services", err)
	}
	recipe := Recipe{Resources: []Resource{}}
	for _, r := range state.Resources {
		recipe.Resources = append(recipe.Resources, r.Resource)
	}
	for key := range state.Ports {
		recipe.Ports = append(recipe.Ports, key)
	}
	if err = validateResources(recipe); err != nil {
		return state, err
	}
	return state, nil
}
func (s *Service) saveResources(state ResourceState) error {
	return recordfile.WriteAtomic(s.dir(), filepath.Join(s.dir(), "resources.json"), ".resources-", state)
}

func resourceEnv(state ResourceState, name string) []string {
	env := []string{"ARCHETIPO_RESOURCE_OWNER=" + state.Owner, "ARCHETIPO_RESOURCE_NAME=" + name}
	keys := make([]string, 0, len(state.Ports))
	for key := range state.Ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, "ARCHETIPO_PORT_"+key+"="+state.Ports[key])
	}
	return env
}

// RuntimeEnv lets the app and project checks use the exact dependency endpoints
// chosen for this checkout; values are never copied from another increment.
func (s *Service) RuntimeEnv() ([]string, error) {
	state, err := s.Resources()
	if err != nil {
		return nil, err
	}
	return mergeEnv(runtimeBase(), append([]string{
		"ARCHETIPO_ENVIRONMENT_ID=" + s.environmentID(),
		"ARCHETIPO_ENVIRONMENT_ROOT=" + s.Root,
		"ARCHETIPO_ENVIRONMENT_STATE=" + s.dir(),
	}, resourceEnv(state, "")...)), nil
}

func allocatePorts(names []string) (map[string]string, error) {
	ports := map[string]string{}
	listeners := []net.Listener{}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for _, name := range names {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
		_, port, err := net.SplitHostPort(l.Addr().String())
		if err != nil {
			return nil, err
		}
		ports[name] = port
	}
	return ports, nil
}

func (s *Service) resourceCommand(ctx context.Context, result *Result, state ResourceState, record ResourceRecord, command Command) (bool, error) {
	runner := *s
	runner.env = resourceEnv(state, record.Resource.Name)
	err := runner.run(ctx, result, []Command{command})
	return result.Passed, err
}

// stopResources attempts every stop in reverse dependency order, retaining all
// failures. Stop is idempotent and must work after a partially completed Start.
func (s *Service) stopResources(ctx context.Context, result *Result, state *ResourceState) error {
	passed := true
	for n := len(state.Resources) - 1; n >= 0; n-- {
		r := &state.Resources[n]
		if r.State == "stopped" {
			continue
		}
		r.State, r.Error = "stopping", ""
		if err := s.saveResources(*state); err != nil {
			return err
		}
		ok, err := s.resourceCommand(ctx, result, *state, *r, r.Resource.Stop)
		if err != nil {
			return err
		}
		if ok {
			ok, err = s.resourceCommand(ctx, result, *state, *r, r.Resource.Stopped)
			if err != nil {
				return err
			}
		}
		if ok {
			r.State = "stopped"
		} else {
			r.State = "error"
			r.Error = "stop or stopped verification failed; inspect " + result.ReportPath
			passed = false
		}
		if err = s.saveResources(*state); err != nil {
			return err
		}
	}
	result.Passed = passed
	return s.saveResult(result)
}

func (s *Service) ensureResources(ctx context.Context, result *Result, recipe Recipe) error {
	result.Passed = true
	state, err := s.Resources()
	if err != nil {
		return err
	}
	active := false
	old := []Resource{}
	for _, r := range state.Resources {
		old = append(old, r.Resource)
		active = active || r.State != "stopped"
	}
	same := reflect.DeepEqual(old, recipe.Resources) && len(state.Ports) == len(recipe.Ports)
	for _, key := range recipe.Ports {
		same = same && state.Ports[key] != ""
	}
	if active && same {
		for _, r := range state.Resources {
			if r.State != "ready" {
				same = false
				break
			}
			ok, probeErr := s.resourceCommand(ctx, result, state, r, r.Resource.Probe)
			if probeErr != nil {
				return probeErr
			}
			if !ok {
				same = false
				break
			}
		}
		if same {
			s.env = resourceEnv(state, "")
			return nil
		}
		// Preserve the failed readiness evidence; repair gets its own report.
		repair, repairErr := s.newResult("resource-repair", "")
		if repairErr != nil {
			return repairErr
		}
		*result = repair
	}
	if active {
		if err = s.stopResources(ctx, result, &state); err != nil {
			return err
		}
		if !result.Passed {
			return nil
		}
	}
	if len(recipe.Resources) == 0 {
		s.env = nil
		return nil
	}
	ports, err := allocatePorts(recipe.Ports)
	if err != nil {
		return err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return err
	}
	state = ResourceState{Version: 1, Owner: "resource-" + hex.EncodeToString(token), Ports: ports, Resources: []ResourceRecord{}}
	// Unstarted resources are omitted: only registered attempts can own resources.
	for _, resource := range recipe.Resources {
		state.Resources = append(state.Resources, ResourceRecord{Resource: resource, State: "starting"})
		if err = s.saveResources(state); err != nil {
			return err
		}
		r := &state.Resources[len(state.Resources)-1]
		ok, runErr := s.resourceCommand(ctx, result, state, *r, resource.Start)
		if runErr == nil && ok {
			ok, runErr = s.resourceCommand(ctx, result, state, *r, resource.Probe)
		}
		if runErr != nil || !ok {
			r.State, r.Error = "error", "start or readiness failed"
			if saveErr := s.saveResources(state); saveErr != nil {
				return saveErr
			}
			// Cancellation of the failed start must not cancel the compensating stop.
			cleanCtx, cancel := context.WithTimeout(context.Background(), time.Duration(len(state.Resources)*60+5)*time.Second)
			cleanErr := s.stopResources(cleanCtx, result, &state)
			cancel()
			result.Passed = false
			if saveErr := s.saveResult(result); saveErr != nil {
				return saveErr
			}
			if runErr != nil {
				return runErr
			}
			return cleanErr
		}
		r.State = "ready"
		if err = s.saveResources(state); err != nil {
			return err
		}
	}
	s.env = resourceEnv(state, "")
	// Starting a new resource generation invalidates old preparation/check passes.
	if err = s.saveState(stateEmpty()); err != nil {
		return err
	}
	result.Passed = true
	return nil
}

func stateEmpty() state { return state{Checks: map[string]string{}} }

// mergeEnv replaces inherited values, preventing user-supplied runtime owner or
// port variables from shadowing the values assigned by the deterministic layer.
func mergeEnv(base, extra []string) []string {
	result := append([]string(nil), base...)
	for _, value := range extra {
		key, _, _ := strings.Cut(value, "=")
		filtered := result[:0]
		for _, old := range result {
			if !strings.HasPrefix(old, key+"=") {
				filtered = append(filtered, old)
			}
		}
		result = append(filtered, value)
	}
	return result
}

func resourceFailure(result Result) error {
	return iox.NewPrecondition("environment resources did not stop", fmt.Sprintf("inspect %s; ownership journal retained for retry", result.ReportPath), nil)
}

func runtimeBase() []string {
	env := []string{}
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "ARCHETIPO_ENVIRONMENT_") || strings.HasPrefix(value, "ARCHETIPO_RESOURCE_") || strings.HasPrefix(value, "ARCHETIPO_PORT_") {
			continue
		}
		env = append(env, value)
	}
	return env
}
