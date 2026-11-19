package isolated

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Provider creates and runs environments. Docker is the first implementation;
// Podman or a managed virtual machine can follow without touching what runs
// inside.
type Provider interface {
	Available(ctx context.Context) error
	EnsureImage(ctx context.Context, image string, progress Progress) error
	Up(ctx context.Context, env Environment, gatewayReady func(context.Context) error) error
	GatewayReady(ctx context.Context, env Environment) bool
	Logs(ctx context.Context, name string) string
	Down(ctx context.Context, env Environment) error
	Remove(ctx context.Context, env Environment) error
}

// Timeouts of a start. The first start may also pull the image, which is not
// bounded here.
var (
	GatewayTimeout = 30 * time.Second
	ViewerTimeout  = 60 * time.Second
	pollInterval   = 250 * time.Millisecond
	viewerAnswers  = answers
	// hostOS names the operating system the launcher runs on; tests replace it.
	hostOS = runtime.GOOS
)

// Progress receives one line per step, for the person waiting.
type Progress func(step string)

// Start brings an environment up and returns once its viewer answers on host
// loopback.
func Start(ctx context.Context, provider Provider, store Store, id, image string, progress Progress) (Environment, error) {
	if err := provider.Available(ctx); err != nil {
		return Environment{}, err
	}
	// Too little engine memory does not stop a start, but it is said before
	// one: otherwise the first sign is a viewer that freezes mid-work.
	if reporter, ok := provider.(EngineMemoryReporter); ok {
		if total, err := reporter.EngineMemory(ctx); err == nil {
			if warning := lowEngineMemoryWarning(total, hostOS); warning != "" {
				progress(warning)
			}
		}
	}
	env, err := store.Ensure(id, image)
	if err != nil {
		return Environment{}, err
	}
	progress("Preparo l'immagine " + env.Image + " (la prima volta può richiedere alcuni minuti)…")
	if err = provider.EnsureImage(ctx, env.Image, progress); err != nil {
		return Environment{}, err
	}
	progress("Avvio l'ambiente isolato " + env.ID + "…")
	ready := func(ctx context.Context) error {
		return poll(ctx, GatewayTimeout, func() bool { return provider.GatewayReady(ctx, env) })
	}
	if err = provider.Up(ctx, env, ready); err != nil {
		return Environment{}, err
	}
	if err = poll(ctx, ViewerTimeout, func() bool { return viewerAnswers(ctx, env.URL()) }); err != nil {
		logs := provider.Logs(ctx, env.Workspace())
		return Environment{}, iox.NewConflict("il viewer dell'ambiente isolato non risponde su "+env.URL(), logs, err)
	}
	return env, nil
}

// Stop removes the containers of an environment and keeps its volume.
func Stop(ctx context.Context, provider Provider, store Store, id string) (Environment, error) {
	env, found, err := store.Get(id)
	if err != nil {
		return Environment{}, err
	}
	if !found {
		return Environment{}, iox.NewNotFound("isolated environment "+id+" is not recorded", "start it with `archetipo view --isolated`", nil)
	}
	if err = provider.Available(ctx); err != nil {
		return Environment{}, err
	}
	return env, provider.Down(ctx, env)
}

// Remove destroys an environment, including its durable volume, and forgets
// its host-side record. Callers must obtain the person's explicit confirmation
// before invoking it because unpushed commits and saved credentials are lost.
func Remove(ctx context.Context, provider Provider, store Store, id string) (Environment, error) {
	env, found, err := store.Get(id)
	if err != nil {
		return Environment{}, err
	}
	if !found {
		return Environment{}, iox.NewNotFound("isolated environment "+id+" is not recorded", "run `archetipo view --isolated` to create it", nil)
	}
	if err = provider.Available(ctx); err != nil {
		return Environment{}, err
	}
	if err = provider.Remove(ctx, env); err != nil {
		return Environment{}, err
	}
	if err = store.Forget(id); err != nil {
		return Environment{}, err
	}
	return env, nil
}

func answers(ctx context.Context, url string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode < 500
}

var errTimeout = errors.New("timed out")

func poll(ctx context.Context, limit time.Duration, done func() bool) error {
	deadline := time.Now().Add(limit)
	for {
		if done() {
			return nil
		}
		if time.Now().After(deadline) {
			return errTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
