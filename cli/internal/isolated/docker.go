package isolated

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// EnvEngine selects the container engine binary. Only standard features are
// used, so an engine with a Docker-compatible command line can replace it.
const EnvEngine = "ARCHETIPO_CONTAINER_ENGINE"

// Runner executes one engine command and returns its standard output.
type Runner func(ctx context.Context, args ...string) (string, error)

// Docker is the first environment provider.
type Docker struct {
	Binary string
	Run    Runner
}

// NewDocker returns the provider backed by the configured engine binary.
func NewDocker() *Docker {
	binary := strings.TrimSpace(os.Getenv(EnvEngine))
	if binary == "" {
		binary = "docker"
	}
	d := &Docker{Binary: binary}
	d.Run = func(ctx context.Context, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, d.Binary, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				detail = err.Error()
			}
			return "", &engineError{args: args, detail: detail, err: err}
		}
		return strings.TrimSpace(string(out)), nil
	}
	return d
}

type engineError struct {
	args   []string
	detail string
	err    error
}

func (e *engineError) Error() string { return strings.Join(e.args, " ") + ": " + e.detail }
func (e *engineError) Unwrap() error { return e.err }

func (d *Docker) exists(ctx context.Context, kind, name string) bool {
	_, err := d.Run(ctx, kind, "inspect", name)
	return err == nil
}

// Available reports, in one plain sentence, why the engine cannot be used.
func (d *Docker) Available(ctx context.Context) error {
	if _, err := exec.LookPath(d.Binary); err != nil {
		return iox.NewPrecondition("Docker non è installato.", "installa Docker Desktop e riprova", err)
	}
	if _, err := d.Run(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return iox.NewPrecondition("Docker non è avviato.", "apri Docker Desktop, attendi che sia pronto e riprova", err)
	}
	return nil
}

// EnsureImage uses the local image, or pulls it once.
func (d *Docker) EnsureImage(ctx context.Context, image string) error {
	if d.exists(ctx, "image", image) {
		return nil
	}
	if _, err := d.Run(ctx, "pull", image); err != nil {
		hint := "verifica la connessione e riprova"
		if image == devImage {
			hint = "costruisci l'immagine dalla radice del repository: docker build -f docker/isolated/Dockerfile -t " + devImage + " ."
		}
		return iox.NewPrecondition("immagine dell'ambiente isolato non disponibile: "+image, hint, err)
	}
	return nil
}

// gatewayArgs starts the container that owns the network namespace. It is the
// only one with a capability, NET_ADMIN, and the only one that publishes
// ports, always on host loopback.
func gatewayArgs(env Environment) []string {
	view := strconv.Itoa(env.ViewPort)
	return []string{
		"run", "-d", "--name", env.Gateway(), "--network", env.Network(),
		"--user", "root", "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
		"--security-opt", "no-new-privileges",
		"--label", "archetipo.environment=" + env.ID, "--label", "archetipo.role=gateway",
		"-p", "127.0.0.1:" + view + ":" + view,
		"-p", "127.0.0.1:" + env.PreviewRange() + ":" + env.PreviewRange(),
		env.Image, "archetipo", "view", "_gateway",
	}
}

// CloneDir is where the viewer clones projects added by repository URL: a
// directory of the environment's volume.
const CloneDir = "/home/archetipo/workspaces"

// workspaceArgs starts the container that runs the viewer. It joins the
// gateway's namespace, has no capability and mounts only its own volume.
func workspaceArgs(env Environment) []string {
	return []string{
		"run", "-d", "--name", env.Workspace(), "--network", "container:" + env.Gateway(),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--label", "archetipo.environment=" + env.ID, "--label", "archetipo.role=workspace",
		"-v", env.Volume() + ":/home/archetipo",
		"-e", "ARCHETIPO_PREVIEW_BIND=0.0.0.0",
		"-e", "ARCHETIPO_PREVIEW_PORTS=" + env.PreviewRange(),
		"-e", "ARCHETIPO_CLONE_DIR=" + CloneDir,
		"-e", "ARCHETIPO_RUNTIME=isolated",
		env.Image, "archetipo", "view", "--host", "0.0.0.0", "--port", strconv.Itoa(env.ViewPort), "--no-open",
	}
}

// Up recreates both containers on the existing network and volume. The
// workspace starts only once the gateway reports its rules installed.
func (d *Docker) Up(ctx context.Context, env Environment, gatewayReady func(context.Context) error) error {
	if err := d.Down(ctx, env); err != nil {
		return err
	}
	if !d.exists(ctx, "network", env.Network()) {
		if _, err := d.Run(ctx, "network", "create", "--label", "archetipo.environment="+env.ID, env.Network()); err != nil {
			return err
		}
	}
	if !d.exists(ctx, "volume", env.Volume()) {
		if _, err := d.Run(ctx, "volume", "create", "--label", "archetipo.environment="+env.ID, env.Volume()); err != nil {
			return err
		}
	}
	if _, err := d.Run(ctx, gatewayArgs(env)...); err != nil {
		return iox.NewConflict("il gateway dell'ambiente isolato non è partito", "verifica che le porte "+strconv.Itoa(env.ViewPort)+" e "+env.PreviewRange()+" siano libere", err)
	}
	if err := gatewayReady(ctx); err != nil {
		logs, _ := d.Run(ctx, "logs", "--tail", "20", env.Gateway())
		_ = d.Down(ctx, env)
		return iox.NewConflict("il gateway non ha installato le regole di rete", strings.TrimSpace(logs), err)
	}
	if _, err := d.Run(ctx, workspaceArgs(env)...); err != nil {
		_ = d.Down(ctx, env)
		return err
	}
	return nil
}

// GatewayReady reports whether the gateway created its ready marker.
func (d *Docker) GatewayReady(ctx context.Context, env Environment) bool {
	_, err := d.Run(ctx, "exec", env.Gateway(), "test", "-f", GatewayReady)
	return err == nil
}

// Logs returns the last lines of a container, for a failed start.
func (d *Docker) Logs(ctx context.Context, name string) string {
	out, _ := d.Run(ctx, "logs", "--tail", "30", name)
	return out
}

// Down removes the containers and keeps the volume, so work survives.
func (d *Docker) Down(ctx context.Context, env Environment) error {
	for _, name := range []string{env.Workspace(), env.Gateway()} {
		if d.exists(ctx, "container", name) {
			if _, err := d.Run(ctx, "rm", "-f", name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Remove destroys every engine object owned by an environment. Unlike Down it
// removes the durable volume, so it is used only after explicit confirmation.
func (d *Docker) Remove(ctx context.Context, env Environment) error {
	if err := d.Down(ctx, env); err != nil {
		return err
	}
	for _, object := range []struct {
		kind string
		name string
	}{
		{kind: "network", name: env.Network()},
		{kind: "volume", name: env.Volume()},
	} {
		if d.exists(ctx, object.kind, object.name) {
			if _, err := d.Run(ctx, object.kind, "rm", object.name); err != nil {
				return err
			}
		}
	}
	return nil
}
