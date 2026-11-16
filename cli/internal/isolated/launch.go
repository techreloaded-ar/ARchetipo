package isolated

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Provider creates and runs environments. Docker is the first implementation;
// Podman or a managed virtual machine can follow without touching what runs
// inside.
type Provider interface {
	Available(ctx context.Context) error
	EnsureImage(ctx context.Context, image string) error
	Up(ctx context.Context, env Environment, gatewayReady func(context.Context) error) error
	GatewayReady(ctx context.Context, env Environment) bool
	Logs(ctx context.Context, name string) string
	Down(ctx context.Context, env Environment) error
}

// Timeouts of a start. The first start may also pull the image, which is not
// bounded here.
var (
	GatewayTimeout = 30 * time.Second
	ViewerTimeout  = 60 * time.Second
	pollInterval   = 250 * time.Millisecond
	viewerAnswers  = answers
)

// Progress receives one line per step, for the person waiting.
type Progress func(step string)

// Start brings an environment up and returns once its viewer answers on host
// loopback.
func Start(ctx context.Context, provider Provider, store Store, id, image string, progress Progress) (Environment, error) {
	if err := provider.Available(ctx); err != nil {
		return Environment{}, err
	}
	env, err := store.Ensure(id, image)
	if err != nil {
		return Environment{}, err
	}
	progress("Preparo l'immagine " + env.Image + " (la prima volta può richiedere alcuni minuti)…")
	if err = provider.EnsureImage(ctx, env.Image); err != nil {
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
