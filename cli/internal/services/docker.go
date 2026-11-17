package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvEngine selects the container engine binary, as for the isolated launcher.
const EnvEngine = "ARCHETIPO_CONTAINER_ENGINE"

const labelEnvironment = "archetipo.service.environment"

// Docker runs each declared service as a container of the person's engine,
// published only on 127.0.0.1.
type Docker struct {
	Binary string
	// Run executes one engine command; output goes to log.
	Run func(ctx context.Context, log io.Writer, args ...string) (string, error)
	// Probe reports whether the service answers on 127.0.0.1:port.
	Probe func(ctx context.Context, port string) error
}

// NewDocker returns the backend of the configured engine.
func NewDocker() *Docker {
	binary := strings.TrimSpace(os.Getenv(EnvEngine))
	if binary == "" {
		binary = "docker"
	}
	d := &Docker{Binary: binary, Probe: probeTCP}
	d.Run = func(ctx context.Context, log io.Writer, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, d.Binary, args...)
		var out bytes.Buffer
		cmd.Stdout = &out
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if log != nil {
			cmd.Stderr = io.MultiWriter(&stderr, log)
		}
		err := cmd.Run()
		if err != nil {
			detail := strings.TrimSpace(stderr.String())
			if errors.Is(err, exec.ErrNotFound) {
				detail = "the container engine is not installed"
			}
			return "", fmt.Errorf("%s %s: %s", d.Binary, args[0], firstNonEmpty(detail, err.Error()))
		}
		return strings.TrimSpace(out.String()), nil
	}
	return d
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// owned reports whether a container exists and whether it belongs to unit.
func (d *Docker) owned(ctx context.Context, unit Unit) (exists bool, running bool, err error) {
	out, err := d.Run(ctx, nil, "container", "inspect", "--format", "{{.State.Running}} {{index .Config.Labels \""+labelEnvironment+"\"}}", resourceName(unit))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") {
			return false, false, nil
		}
		return false, false, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 || fields[1] != unit.Environment {
		return true, false, fmt.Errorf("container %s exists but does not belong to this checkout; remove it or rename it", resourceName(unit))
	}
	return true, fields[0] == "true", nil
}

// runArgs is the whole container definition. Nothing in it comes from the
// recipe except the image, the variables and the image's own data directory.
func runArgs(unit Unit) []string {
	name := resourceName(unit)
	args := []string{
		"run", "-d", "--name", name,
		"--label", labelEnvironment + "=" + unit.Environment,
		"--label", "archetipo.resource.owner=" + unit.Owner,
		"--label", "archetipo.resource.name=" + unit.Name,
		"-p", "127.0.0.1:" + unit.Port + ":" + strconv.Itoa(unit.Spec.Port),
	}
	keys := make([]string, 0, len(unit.Spec.Env))
	for key := range unit.Spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "-e", key+"="+unit.Spec.Env[key])
	}
	if unit.Spec.Data != "" {
		args = append(args, "-v", name+":"+unit.Spec.Data)
	}
	return append(args, unit.Spec.Image)
}

func (d *Docker) Start(ctx context.Context, unit Unit) error {
	exists, _, err := d.owned(ctx, unit)
	if err != nil {
		return err
	}
	if exists {
		if _, err = d.Run(ctx, unit.Log, "rm", "-f", resourceName(unit)); err != nil {
			return err
		}
	}
	if unit.Spec.Data != "" {
		if _, err = d.Run(ctx, nil, "volume", "inspect", resourceName(unit)); err != nil {
			if _, err = d.Run(ctx, unit.Log, "volume", "create", "--label", labelEnvironment+"="+unit.Environment, resourceName(unit)); err != nil {
				return err
			}
		}
	}
	logf(unit, "starting %s from %s on 127.0.0.1:%s", unit.Name, unit.Spec.Image, unit.Port)
	_, err = d.Run(ctx, unit.Log, runArgs(unit)...)
	return err
}

func (d *Docker) Ready(ctx context.Context, unit Unit) error {
	return waitReady(ctx, unit, func(ctx context.Context) error {
		_, running, err := d.owned(ctx, unit)
		if err != nil {
			return err
		}
		if !running {
			return errors.New("container is not running")
		}
		return d.Probe(ctx, unit.Port)
	})
}

func (d *Docker) Stop(ctx context.Context, unit Unit) error {
	exists, _, err := d.owned(ctx, unit)
	if err != nil || !exists {
		return err
	}
	_, err = d.Run(ctx, unit.Log, "rm", "-f", resourceName(unit))
	return err
}

func (d *Docker) Stopped(ctx context.Context, unit Unit) error {
	exists, running, err := d.owned(ctx, unit)
	if err != nil {
		return err
	}
	if exists && running {
		return errors.New("service container is still running")
	}
	return nil
}

func (d *Docker) Remove(ctx context.Context, unit Unit) error {
	if err := d.Stop(ctx, unit); err != nil {
		return err
	}
	if unit.Spec.Data == "" {
		return nil
	}
	out, err := d.Run(ctx, nil, "volume", "inspect", "--format", "{{index .Labels \""+labelEnvironment+"\"}}", resourceName(unit))
	if err != nil {
		return nil // already gone
	}
	if strings.TrimSpace(out) != unit.Environment {
		return fmt.Errorf("volume %s does not belong to this checkout; left in place", resourceName(unit))
	}
	_, err = d.Run(ctx, unit.Log, "volume", "rm", resourceName(unit))
	return err
}

// waitReady polls check until it passes or ctx ends, reporting the last reason.
func waitReady(ctx context.Context, unit Unit, check func(context.Context) error) error {
	var last error
	for {
		if last = check(ctx); last == nil {
			logf(unit, "%s is ready on 127.0.0.1:%s", unit.Name, unit.Port)
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s did not become ready: %v", unit.Name, last)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// probeTCP tells a listening service from an engine's port forwarder with
// nothing behind it: the forwarder accepts and then closes, a service either
// greets or waits for the client to speak.
func probeTCP(ctx context.Context, port string) error {
	var dialer net.Dialer
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		return nil
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return nil
	}
	return fmt.Errorf("connection closed by the service: %w", err)
}
