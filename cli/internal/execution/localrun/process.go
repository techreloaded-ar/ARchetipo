package localrun

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

const (
	// MaxCapturedOutput bounds how much of a captured stream may ever be echoed
	// back into a diagnostic. It lives here so a local provider does not have to
	// invent its own limit: the streams of an agent can be arbitrarily large and
	// can quote whatever the agent read, so no message composed from one may
	// carry it whole.
	MaxCapturedOutput = 512

	// maxLineBytes bounds one line of the process's standard output. The stream
	// itself is unbounded by design; a single line is not, and buffering an
	// unbounded one would be the single way this consumer could be made to
	// exhaust memory.
	maxLineBytes = 1 << 20

	// lineBuffer bounds how far the reader may run ahead of the consumer.
	lineBuffer = 256
)

// Process is one live local process, seen as what a session needs from it:
// lines out, lines in, a way to ask it to stop and a way to learn how it ended.
// It knows no protocol — it speaks lines, not messages.
type Process interface {
	// Send writes one line to the process's standard input. It is safe for
	// concurrent use: two interleaved writes would produce a corrupt line.
	Send(line []byte) error
	// Lines yields the process's standard output line by line, and is closed
	// when that stream ends.
	Lines() <-chan []byte
	// Signal asks the process to stop. It does not wait for it to do so.
	Signal() error
	// Wait blocks until the process has exited and reports its exit code and the
	// tail of its standard error. A process that could never be run to
	// completion reports -1 with a non-nil error, which is a different situation
	// from a process that ran and failed.
	Wait() (exitCode int, stderr string, err error)
	// Close releases the process's input and resources.
	Close() error
}

// Starter is the single seam between this package and the operating system.
type Starter interface {
	Start(ctx context.Context, dir, name string, args []string) (Process, error)
}

// ExecStarter is the real Starter, backed by os/exec.
type ExecStarter struct{}

var _ Starter = ExecStarter{}

func (ExecStarter) Start(ctx context.Context, dir, name string, args []string) (Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening the input of %s: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening the output of %s: %w", name, err)
	}
	stderr := &boundedBuffer{limit: MaxCapturedOutput * 4}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("running %s: %w", name, err)
	}

	process := &execProcess{cmd: cmd, stdin: stdin, stderr: stderr, lines: make(chan []byte, lineBuffer)}
	go process.read(stdout)
	return process, nil
}

type execProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *boundedBuffer
	lines  chan []byte

	writeMu sync.Mutex
	once    sync.Once
}

func (p *execProcess) read(stdout io.Reader) {
	defer close(p.lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		p.lines <- line
	}
}

func (p *execProcess) Send(line []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing to the local process: %w", err)
	}
	return nil
}

func (p *execProcess) Lines() <-chan []byte { return p.lines }

func (p *execProcess) Signal() error {
	if p.cmd.Process == nil {
		return fmt.Errorf("the local process is not running")
	}
	return p.cmd.Process.Kill()
}

func (p *execProcess) Wait() (int, string, error) {
	err := p.cmd.Wait()
	stderr := p.stderr.String()
	if err == nil {
		return 0, stderr, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stderr, nil
	}
	return -1, stderr, fmt.Errorf("running %s: %w", p.cmd.Path, err)
}

func (p *execProcess) Close() error {
	var err error
	p.once.Do(func() { err = p.stdin.Close() })
	return err
}

// boundedBuffer keeps only the first limit bytes it is given. The rest is
// discarded on the spot rather than kept and truncated later, so a process that
// floods its standard error cannot grow this process's memory.
type boundedBuffer struct {
	mu    sync.Mutex
	limit int
	body  []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.limit - len(b.body); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.body = append(b.body, p[:room]...)
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.body)
}
