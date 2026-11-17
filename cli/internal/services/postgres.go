package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// postgres is one checkout's PostgreSQL server, with the semantics of the
// official image's variables: POSTGRES_USER (default postgres),
// POSTGRES_PASSWORD and POSTGRES_DB (default the user).
type postgres struct {
	bin  string
	dir  string
	data string
	unit Unit
}

// initialized marks a data directory whose first start completed, including
// the database the recipe asked for.
const initialized = ".archetipo-initialized"

func (pg *postgres) user() string {
	if u := pg.unit.Spec.Env["POSTGRES_USER"]; u != "" {
		return u
	}
	return "postgres"
}

func (pg *postgres) database() string {
	if db := pg.unit.Spec.Env["POSTGRES_DB"]; db != "" {
		return db
	}
	return pg.user()
}

func (pg *postgres) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(pg.bin, name), args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+pg.unit.Spec.Env["POSTGRES_PASSWORD"], "PGCONNECT_TIMEOUT=3")
	cmd.Stdout, cmd.Stderr = pg.unit.Log, pg.unit.Log
	return cmd
}

func (pg *postgres) start(ctx context.Context) error {
	if pg.running(ctx) {
		if pg.listensOn() == pg.unit.Port {
			logf(pg.unit, "%s is already running on 127.0.0.1:%s", pg.unit.Name, pg.unit.Port)
			return nil
		}
		if err := pg.stop(ctx); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(pg.dir, 0o700); err != nil {
		return err
	}
	fresh := false
	if _, err := os.Stat(filepath.Join(pg.dir, initialized)); err != nil {
		// A first start that did not complete leaves nothing to keep.
		if err = os.RemoveAll(pg.data); err != nil {
			return err
		}
		if err = pg.initdb(ctx); err != nil {
			return err
		}
		fresh = true
	}
	if err := pg.launch(); err != nil {
		return err
	}
	if !fresh {
		return nil
	}
	if err := waitReady(ctx, pg.unit, pg.ready); err != nil {
		return err
	}
	if db := pg.database(); db != "postgres" {
		logf(pg.unit, "creating database %s", db)
		if err := pg.command(ctx, "createdb", "-h", "127.0.0.1", "-p", pg.unit.Port, "-U", pg.user(), db).Run(); err != nil {
			return fmt.Errorf("create database %s: %w", db, err)
		}
	}
	return os.WriteFile(filepath.Join(pg.dir, initialized), nil, 0o600)
}

func (pg *postgres) initdb(ctx context.Context) error {
	logf(pg.unit, "initializing PostgreSQL data for %s", pg.unit.Name)
	args := []string{"-D", pg.data, "-U", pg.user(), "-E", "UTF8", "--auth-local=trust"}
	password := pg.unit.Spec.Env["POSTGRES_PASSWORD"]
	if password == "" {
		args = append(args, "--auth-host=trust")
	} else {
		pwfile := filepath.Join(pg.dir, ".pwfile")
		if err := os.WriteFile(pwfile, []byte(password+"\n"), 0o600); err != nil {
			return err
		}
		defer func() { _ = os.Remove(pwfile) }()
		args = append(args, "--auth-host=scram-sha-256", "--pwfile="+pwfile)
	}
	if err := pg.command(ctx, "initdb", args...).Run(); err != nil {
		return fmt.Errorf("initdb: %w", err)
	}
	return nil
}

// launch starts the server detached, listening only on loopback and without
// a socket file, whose path would be too long inside a checkout.
func (pg *postgres) launch() error {
	logPath := filepath.Join(pg.dir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.Command(filepath.Join(pg.bin, "postgres"), "-D", pg.data, "-p", pg.unit.Port,
		"-c", "listen_addresses=127.0.0.1", "-c", "unix_socket_directories=")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detach(cmd)
	logf(pg.unit, "starting PostgreSQL for %s on 127.0.0.1:%s (log: %s)", pg.unit.Name, pg.unit.Port, logPath)
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (pg *postgres) ready(ctx context.Context) error {
	if !pg.running(ctx) {
		return errors.New("the server is not running; see " + filepath.Join(pg.dir, "server.log"))
	}
	cmd := exec.CommandContext(ctx, filepath.Join(pg.bin, "pg_isready"), "-q", "-h", "127.0.0.1", "-p", pg.unit.Port, "-U", pg.user())
	if err := cmd.Run(); err != nil {
		return errors.New("the server does not accept connections yet")
	}
	return nil
}

// running asks pg_ctl, which reads the data directory's own pid file.
func (pg *postgres) running(ctx context.Context) bool {
	if _, err := os.Stat(filepath.Join(pg.data, "postmaster.pid")); err != nil {
		return false
	}
	return exec.CommandContext(ctx, filepath.Join(pg.bin, "pg_ctl"), "-D", pg.data, "status").Run() == nil
}

// listensOn reads the port line of postmaster.pid.
func (pg *postgres) listensOn() string {
	raw, err := os.ReadFile(filepath.Join(pg.data, "postmaster.pid"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 4 {
		return ""
	}
	return strings.TrimSpace(lines[3])
}

func (pg *postgres) stop(ctx context.Context) error {
	if !pg.running(ctx) {
		return nil
	}
	logf(pg.unit, "stopping PostgreSQL for %s", pg.unit.Name)
	if err := pg.command(ctx, "pg_ctl", "-D", pg.data, "-m", "fast", "-w", "-t", "30", "stop").Run(); err != nil {
		return fmt.Errorf("pg_ctl stop: %w", err)
	}
	return nil
}
