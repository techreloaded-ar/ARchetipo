package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EnvPostgresRoot overrides where PostgreSQL versions are installed, one
// directory per major version holding bin/.
const EnvPostgresRoot = "ARCHETIPO_POSTGRES_ROOT"

// Process runs declared services as processes of the environment, from the
// runtimes installed in its image. It is the backend of the isolated runtime,
// which has no container engine. Only the images it knows are supported; the
// others are refused with the list of what is available.
type Process struct {
	PostgresRoot string
}

// NewProcess returns the backend of the installed runtimes.
func NewProcess() *Process {
	root := strings.TrimSpace(os.Getenv(EnvPostgresRoot))
	if root == "" {
		root = "/usr/lib/postgresql"
	}
	return &Process{PostgresRoot: root}
}

func (p *Process) runtime(unit Unit) (*postgres, error) {
	repository, tag := imageName(unit.Spec.Image)
	if repository != "postgres" {
		return nil, fmt.Errorf("the service %s (%s) is not available in the isolated environment; available here: %s", unit.Name, unit.Spec.Image, strings.Join(p.available(), ", "))
	}
	major, ok := majorVersion(tag)
	if !ok {
		return nil, fmt.Errorf("the service %s must name a PostgreSQL major version in its tag (for example postgres:17-alpine); available here: %s", unit.Name, strings.Join(p.available(), ", "))
	}
	bin := filepath.Join(p.PostgresRoot, strconv.Itoa(major), "bin")
	if _, err := os.Stat(filepath.Join(bin, "postgres")); err != nil {
		return nil, fmt.Errorf("PostgreSQL %d is not available in the isolated environment; available here: %s", major, strings.Join(p.available(), ", "))
	}
	dir := filepath.Join(unit.State, "services", unit.Name)
	return &postgres{bin: bin, dir: dir, data: filepath.Join(dir, "data"), unit: unit}, nil
}

// available lists the supported images, for the refusal message.
func (p *Process) available() []string {
	entries, _ := os.ReadDir(p.PostgresRoot)
	found := []string{}
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(p.PostgresRoot, entry.Name(), "bin", "postgres")); err == nil {
			found = append(found, "postgres:"+entry.Name())
		}
	}
	sort.Strings(found)
	if len(found) == 0 {
		return []string{"none"}
	}
	return found
}

func (p *Process) Start(ctx context.Context, unit Unit) error {
	pg, err := p.runtime(unit)
	if err != nil {
		return err
	}
	return pg.start(ctx)
}

func (p *Process) Ready(ctx context.Context, unit Unit) error {
	pg, err := p.runtime(unit)
	if err != nil {
		return err
	}
	return waitReady(ctx, unit, pg.ready)
}

func (p *Process) Stop(ctx context.Context, unit Unit) error {
	pg, err := p.runtime(unit)
	if err != nil {
		return err
	}
	return pg.stop(ctx)
}

func (p *Process) Stopped(ctx context.Context, unit Unit) error {
	pg, err := p.runtime(unit)
	if err != nil {
		return err
	}
	if pg.running(ctx) {
		return errors.New("the service is still running")
	}
	return nil
}

func (p *Process) Remove(ctx context.Context, unit Unit) error {
	pg, err := p.runtime(unit)
	if err != nil {
		// Nothing of an unsupported service was ever started here.
		return os.RemoveAll(filepath.Join(unit.State, "services", unit.Name))
	}
	if err = pg.stop(ctx); err != nil {
		return err
	}
	return os.RemoveAll(pg.dir)
}
