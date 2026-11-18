package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/isolated"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/web"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// The launcher only orchestrates on the host. It returns once the viewer
// answers: the environment keeps running in the engine, not in this process.
func runIsolatedView(s streams, id string, noOpen bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := isolatedStore()
	if err != nil {
		return err
	}
	progress := func(step string) { fmt.Fprintln(s.err, step) }
	env, err := isolated.Start(ctx, isolated.NewDocker(), store, id, isolated.DefaultImage(), progress)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "ARchetipo (ambiente isolato %s): %s\n", env.ID, env.URL())
	fmt.Fprintln(s.err, "Il lavoro resta nell'ambiente. Per fermarlo: archetipo view stop --isolated")
	if !noOpen {
		if err := web.OpenBrowser(env.URL()); err != nil {
			fmt.Fprintf(s.err, "(could not open browser: %v)\n", err)
		}
	}
	return nil
}

func stopIsolatedView(s streams, id string) error {
	store, err := isolatedStore()
	if err != nil {
		return err
	}
	env, err := isolated.Stop(context.Background(), isolated.NewDocker(), store, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "Ambiente isolato %s fermato; il lavoro resta nel volume %s.\n", env.ID, env.Volume())
	return nil
}

func removeIsolatedView(s streams, id string) error {
	store, err := isolatedStore()
	if err != nil {
		return err
	}
	env, err := isolated.Remove(context.Background(), isolated.NewDocker(), store, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "Ambiente isolato %s eliminato, compresi il volume %s, i workspace, le credenziali e il lavoro non pubblicato.\n", env.ID, env.Volume())
	return nil
}

func isolatedStore() (isolated.Store, error) {
	dir, err := workspace.StateDir()
	if err != nil {
		return isolated.Store{}, err
	}
	return isolated.Store{Dir: dir}, nil
}

// newViewGatewayCmd is what the gateway container runs: it installs the egress
// rules in the namespace the workspace joins, then holds it.
func newViewGatewayCmd() *cobra.Command {
	return &cobra.Command{Use: "_gateway", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return isolated.RunGateway(ctx)
	}}
}
