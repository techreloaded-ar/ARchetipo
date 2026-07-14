package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/viewreg"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/web"
)

// defaultViewPort is the preferred port for `archetipo view`. When --port is
// not set explicitly, the command scans upward from here for a free port.
const defaultViewPort = 8080

func newViewCmd(s streams) *cobra.Command {
	var port int
	var host string
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Open the local Kanban view for the backlog",
		Long: `Start a local HTTP server that serves a Kanban board for the project
backlog and, by default, opens it in the system browser.

If --port is not given, a free port is selected automatically starting from
8080, so repeated invocations never fail on a busy port. Use "archetipo view
list" to see the running viewers and "archetipo view stop" to close them.

The view reads and writes the same local artifacts used by ARchetipo
(backlog, plans, PRD, mockups and .archetipo/config.yaml), so edits made in
the browser persist immediately. Config changes are saved locally but do not
hot-reload the running viewer: restart "archetipo view" to apply connector or
path changes. The server binds to the loopback interface only; no
authentication is performed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return iox.NewInternal("cwd unavailable", err)
			}
			cfg, err := config.Load(cwd)
			if err != nil {
				return iox.NewInvalidInput(err.Error(), "fix the file or remove it to fall back to defaults", err)
			}
			conn, err := connector.New(cfg)
			if err != nil {
				return iox.NewInvalidInput("connector unavailable", "check `connector:` in .archetipo/config.yaml", err)
			}
			// Auto-select a free port unless the user asked for a specific one.
			// With an explicit --port we stay strict: a conflict surfaces as a
			// listen error from Server.Run.
			if !cmd.Flags().Changed("port") {
				free, ferr := findFreePort(host, port, 64)
				if ferr != nil {
					return iox.NewInternal("no free port available", ferr)
				}
				if free != port {
					fmt.Fprintf(s.err, "port %d busy, using %d instead\n", port, free)
				}
				port = free
			}
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			srv, err := web.NewServer(conn, cfg, addr)
			if err != nil {
				return iox.NewInternal("creating server", err)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			defer func() { _ = viewreg.Remove(port) }()
			onReady := func(url string) {
				fmt.Fprintf(s.err, "ARchetipo view ready at %s\n", url)
				fmt.Fprintln(s.err, "Press Ctrl+C to stop.")
				if _, rerr := viewreg.Register(viewreg.Entry{
					PID:         os.Getpid(),
					Host:        host,
					Port:        port,
					ProjectRoot: cfg.ProjectRoot,
					StartedAt:   time.Now(),
				}); rerr != nil {
					fmt.Fprintf(s.err, "(could not register viewer: %v)\n", rerr)
				}
				if !noOpen {
					if err := web.OpenBrowser(url); err != nil {
						fmt.Fprintf(s.err, "(could not open browser: %v)\n", err)
					}
				}
			}
			if err := srv.Run(ctx, onReady); err != nil {
				return iox.NewInternal("view server stopped with error", err)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", defaultViewPort, "TCP port to listen on")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host address to bind to (loopback only by default)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open the browser automatically")
	cmd.AddCommand(newViewListCmd(s), newViewStopCmd(s))
	return cmd
}

// findFreePort returns the first free port on host starting at start, trying up
// to maxTries consecutive ports. The probe listener is closed immediately; the
// real listen happens later in Server.Run. net.Listen is portable across OSes.
func findFreePort(host string, start, maxTries int) (int, error) {
	for p := start; p < start+maxTries; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port found in range %d-%d", start, start+maxTries-1)
}

// newViewListCmd implements `archetipo view list`: enumerate running viewers,
// pruning any whose server no longer answers.
func newViewListCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List running ARchetipo viewers",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			entries, err := viewreg.List()
			if err != nil {
				return iox.NewInternal("reading viewer registry", err)
			}
			entries = viewreg.Prune(entries)
			if len(entries) == 0 {
				fmt.Fprintln(s.out, "No running viewers.")
				return nil
			}
			now := time.Now()
			tw := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PORT\tPID\tPROJECT\tSTARTED")
			for _, e := range entries {
				fmt.Fprintf(tw, "%d\t%d\t%s\t%s\n", e.Port, e.PID, e.ProjectRoot, viewreg.Since(e.StartedAt, now))
			}
			return tw.Flush()
		},
	}
}

// newViewStopCmd implements `archetipo view stop [port]` and `--all`.
func newViewStopCmd(s streams) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "stop [port]",
		Short: "Stop a running ARchetipo viewer",
		Long:  "Stop the viewer on the given port, or all viewers with --all.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if all {
				if len(args) > 0 {
					return iox.NewInvalidInput("--all takes no port argument", "run `archetipo view stop --all` or `archetipo view stop <port>`", nil)
				}
				return stopAll(s)
			}
			if len(args) == 0 {
				return iox.NewInvalidInput("no viewer specified", "pass a port (`archetipo view stop 8080`) or use --all", nil)
			}
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return iox.NewInvalidInput("invalid port: "+args[0], "pass a numeric port, e.g. `archetipo view stop 8080`", err)
			}
			return stopOne(s, port)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "stop every running viewer")
	return cmd
}

func stopOne(s streams, port int) error {
	e, err := viewreg.Stop(port)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return iox.NewInvalidInput(fmt.Sprintf("no viewer registered on port %d", port), "run `archetipo view list` to see running viewers", err)
		}
		return iox.NewInternal("stopping viewer", err)
	}
	fmt.Fprintf(s.out, "✓ stopped port %d (pid %d)\n", e.Port, e.PID)
	return nil
}

func stopAll(s streams) error {
	entries, err := viewreg.List()
	if err != nil {
		return iox.NewInternal("reading viewer registry", err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(s.out, "No running viewers.")
		return nil
	}
	for _, e := range entries {
		stopped, serr := viewreg.Stop(e.Port)
		if serr != nil {
			fmt.Fprintf(s.out, "  ✗ port %d: %v\n", e.Port, serr)
			continue
		}
		fmt.Fprintf(s.out, "  ✓ stopped port %d (pid %d)\n", stopped.Port, stopped.PID)
	}
	return nil
}
