package isolated

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

// EnvEngine selects the container engine binary. Only standard features are
// used, so an engine with a Docker-compatible command line can replace it.
const EnvEngine = "ARCHETIPO_CONTAINER_ENGINE"

// Limits of the engine commands. An engine can accept the connection and then
// never answer — Docker Desktop with its backend up and its daemon stuck on
// Windows or macOS, a hung docker service on Linux — and an unbounded command
// would hold the CLI forever. Pulling an image is the only command left without
// a limit, because its duration depends on the network.
var (
	// EngineProbeTimeout bounds `info`, the first command of every operation.
	EngineProbeTimeout = 20 * time.Second
	// EngineCommandTimeout bounds every other short command (inspect, run -d,
	// exec, rm, logs, network/volume create).
	EngineCommandTimeout = 2 * time.Minute
	// engineKillGrace is how long a killed engine command may keep its output
	// pipes open (through a child process) before the CLI stops waiting.
	engineKillGrace = 5 * time.Second
)

// errEngineUnresponsive marks an engine command that ran out of its limit.
var errEngineUnresponsive = errors.New("the container engine did not answer in time")

// Runner executes one engine command and returns its standard output.
type Runner func(ctx context.Context, args ...string) (string, error)

// Docker is the first environment provider.
type Docker struct {
	Binary string
	Run    Runner
	// SourceCheckout and Version describe a development binary installed with
	// `npm run install:dev`: when SourceCheckout is set, the development image
	// is built from that checkout instead of pulled, since no registry holds it.
	SourceCheckout string
	Version        string
}

// NewDocker returns the provider backed by the configured engine binary.
func NewDocker() *Docker {
	binary := strings.TrimSpace(os.Getenv(EnvEngine))
	if binary == "" {
		binary = "docker"
	}
	d := &Docker{Binary: binary, SourceCheckout: version.SourceCheckout, Version: version.Version}
	d.Run = func(ctx context.Context, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, d.Binary, args...)
		// Once the context kills the command, a child it started (a credential
		// helper, a CLI plugin) may still hold stdout/stderr: stop waiting for
		// them after a grace period instead of hanging on the pipes.
		cmd.WaitDelay = engineKillGrace
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

// run executes one short engine command within limit. A command that runs out
// of its limit, or that the person interrupts, comes back as a coded error
// naming the command, never as an endless wait.
func (d *Docker) run(ctx context.Context, limit time.Duration, args ...string) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := d.Run(bounded, args...)
	if err == nil {
		return out, nil
	}
	if ctx.Err() != nil {
		return "", iox.NewConflict("operazione interrotta durante `"+d.describe(args)+"`", "rilancia il comando per riprovare", ctx.Err())
	}
	if errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return "", iox.NewPrecondition(
			fmt.Sprintf("Docker non risponde: `%s` non è terminato entro %s.", d.describe(args), limit),
			"riavvia il motore dei container (Docker Desktop su Windows e macOS, il servizio docker su Linux), verifica che `docker info` risponda e riprova",
			fmt.Errorf("%w: %w", errEngineUnresponsive, err))
	}
	return "", err
}

// describe names an engine command for a person, shortening the long `run`
// invocations to their first words.
func (d *Docker) describe(args []string) string {
	const shown = 3
	words := append([]string{d.Binary}, args...)
	if len(words) > shown+1 {
		words = append(words[:shown+1], "…")
	}
	return strings.Join(words, " ")
}

// engineDidNotAnswer reports whether err means the engine never gave an answer,
// as opposed to answering that something is absent or wrong.
func engineDidNotAnswer(err error) bool {
	return errors.Is(err, errEngineUnresponsive) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// exists reports whether an engine object is present. An engine that does not
// answer is an error, not an absent object: treating it as absent would make
// Down report containers removed while they still run.
func (d *Docker) exists(ctx context.Context, kind, name string) (bool, error) {
	_, err := d.run(ctx, EngineCommandTimeout, kind, "inspect", name)
	if err == nil {
		return true, nil
	}
	if engineDidNotAnswer(err) {
		return false, err
	}
	return false, nil
}

// Available reports, in one plain sentence, why the engine cannot be used.
func (d *Docker) Available(ctx context.Context) error {
	if _, err := exec.LookPath(d.Binary); err != nil {
		return iox.NewPrecondition("Docker non è installato.", "installa Docker (Docker Desktop su Windows e macOS, Docker Engine su Linux) e riprova", err)
	}
	engineOS, err := d.run(ctx, EngineProbeTimeout, "info", "--format", "{{.OSType}}")
	if engineDidNotAnswer(err) {
		return err
	}
	if err != nil {
		return iox.NewPrecondition("Docker non è avviato.", "avvia il motore dei container (Docker Desktop su Windows e macOS, il servizio docker su Linux), attendi che sia pronto e riprova", err)
	}
	if engineOS != "linux" {
		return iox.NewPrecondition("l'ambiente isolato richiede container Linux", "in Docker Desktop seleziona Switch to Linux containers e riprova", nil)
	}
	return nil
}

// EnsureImage uses the local image, or pulls it once. A development binary
// that knows its checkout builds the development image from it instead.
func (d *Docker) EnsureImage(ctx context.Context, image string, progress Progress) error {
	if image == devImage && strings.TrimSpace(d.SourceCheckout) != "" {
		return d.ensureDevelopmentImage(ctx, image, progress)
	}
	present, err := d.exists(ctx, "image", image)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	// Unbounded on purpose: a first pull may take minutes on a slow network.
	// The person can still interrupt it.
	if _, err := d.Run(ctx, "pull", image); err != nil {
		if ctx.Err() != nil {
			return iox.NewConflict("download dell'immagine "+image+" interrotto", "rilancia il comando per riprovare", ctx.Err())
		}
		hint := "verifica la connessione e riprova"
		if image == devImage {
			hint = "costruisci l'immagine dalla radice del repository: docker build -f docker/isolated/Dockerfile -t " + devImage + " ."
		}
		return iox.NewPrecondition("immagine dell'ambiente isolato non disponibile: "+image, hint, err)
	}
	return nil
}

// imageVersionLabel records, on a development image, the CLI version it was
// built for, so that a later `npm run install:dev` is noticed and rebuilt.
const imageVersionLabel = "archetipo.version"

// ensureDevelopmentImage builds the development image from the checkout the
// binary was installed from when it is missing or was built for another
// version. A dirty version names no exact content, so it always asks for a
// build and lets the engine's build cache decide whether anything changed.
func (d *Docker) ensureDevelopmentImage(ctx context.Context, image string, progress Progress) error {
	builtFor, err := d.run(ctx, EngineCommandTimeout, "image", "inspect", "--format", `{{ index .Config.Labels "`+imageVersionLabel+`" }}`, image)
	if err != nil && engineDidNotAnswer(err) {
		return err
	}
	present := err == nil
	if present && builtFor == d.Version && !strings.HasSuffix(d.Version, ".dirty") {
		return nil
	}
	manualBuild := "docker build -f docker/isolated/Dockerfile -t " + devImage + " ."
	dockerfile := filepath.Join(d.SourceCheckout, "docker", "isolated", "Dockerfile")
	if _, statErr := os.Stat(dockerfile); statErr != nil {
		if present {
			progress("Il checkout " + d.SourceCheckout + " non è più disponibile: uso l'immagine " + image + " già presente.")
			return nil
		}
		return iox.NewPrecondition("immagine dell'ambiente isolato non disponibile: "+image,
			"il checkout da cui è stato installato archetipo ("+d.SourceCheckout+") non contiene docker/isolated/Dockerfile: rilancia `npm run install:dev` dal repository oppure costruisci l'immagine dalla sua radice con "+manualBuild, statErr)
	}
	progress("Costruisco l'immagine " + image + " dal checkout " + d.SourceCheckout + "…")
	// Unbounded on purpose, like a pull: a first build downloads the base
	// images and the agents. The person can still interrupt it.
	_, err = d.Run(ctx, "build", "-f", dockerfile, "-t", image,
		"--build-arg", "VERSION="+d.Version,
		"--label", imageVersionLabel+"="+d.Version,
		d.SourceCheckout)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return iox.NewConflict("costruzione dell'immagine "+image+" interrotta", "rilancia il comando per riprovare", ctx.Err())
	}
	if present {
		progress("Costruzione dell'immagine " + image + " non riuscita: uso quella già presente, che potrebbe non contenere le ultime modifiche.")
		return nil
	}
	return iox.NewPrecondition("costruzione dell'immagine "+image+" dal checkout "+d.SourceCheckout+" non riuscita",
		"verifica la connessione e che Docker abbia spazio libero, poi riprova; per vedere l'intero log costruiscila dalla radice del repository con "+manualBuild, err)
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
	for _, object := range []struct{ kind, name string }{
		{kind: "network", name: env.Network()},
		{kind: "volume", name: env.Volume()},
	} {
		present, err := d.exists(ctx, object.kind, object.name)
		if err != nil {
			return err
		}
		if !present {
			if _, err = d.run(ctx, EngineCommandTimeout, object.kind, "create", "--label", "archetipo.environment="+env.ID, object.name); err != nil {
				return err
			}
		}
	}
	if _, err := d.run(ctx, EngineCommandTimeout, gatewayArgs(env)...); err != nil {
		if engineDidNotAnswer(err) {
			return err
		}
		return iox.NewConflict("il gateway dell'ambiente isolato non è partito", "verifica che le porte "+strconv.Itoa(env.ViewPort)+" e "+env.PreviewRange()+" siano libere", err)
	}
	if err := gatewayReady(ctx); err != nil {
		logs, _ := d.run(ctx, EngineCommandTimeout, "logs", "--tail", "20", env.Gateway())
		_ = d.Down(ctx, env)
		return iox.NewConflict("il gateway non ha installato le regole di rete", strings.TrimSpace(logs), err)
	}
	if _, err := d.run(ctx, EngineCommandTimeout, workspaceArgs(env)...); err != nil {
		_ = d.Down(ctx, env)
		return err
	}
	return nil
}

// GatewayReady reports whether the gateway created its ready marker.
func (d *Docker) GatewayReady(ctx context.Context, env Environment) bool {
	_, err := d.run(ctx, EngineCommandTimeout, "exec", env.Gateway(), "test", "-f", GatewayReady)
	return err == nil
}

// Logs returns the last lines of a container, for a failed start.
func (d *Docker) Logs(ctx context.Context, name string) string {
	out, _ := d.run(ctx, EngineCommandTimeout, "logs", "--tail", "30", name)
	return out
}

// Down removes the containers and keeps the volume, so work survives.
func (d *Docker) Down(ctx context.Context, env Environment) error {
	for _, name := range []string{env.Workspace(), env.Gateway()} {
		present, err := d.exists(ctx, "container", name)
		if err != nil {
			return err
		}
		if present {
			if _, err = d.run(ctx, EngineCommandTimeout, "rm", "-f", name); err != nil {
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
		present, err := d.exists(ctx, object.kind, object.name)
		if err != nil {
			return err
		}
		if present {
			if _, err = d.run(ctx, EngineCommandTimeout, object.kind, "rm", object.name); err != nil {
				return err
			}
		}
	}
	return nil
}
