package isolated

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// EngineCPUReporter is implemented by a provider that can say how many
// processors its engine gives to containers. It is optional, like
// EngineMemoryReporter: a failed reading never blocks a start.
type EngineCPUReporter interface {
	EngineCPUs(ctx context.Context) (int, error)
}

// enoughEngineCPUs is the count from which a start never mentions processors,
// whatever the computer has: the warning is about a limit that visibly slows
// builds and tests, not about using every core.
const enoughEngineCPUs = 8

// hostCPUs counts the processors of the computer running the CLI; tests
// replace it.
var hostCPUs = runtime.NumCPU

// EngineCPUs reads the processors the engine reports for its containers.
func (d *Docker) EngineCPUs(ctx context.Context) (int, error) {
	out, err := d.run(ctx, EngineProbeTimeout, "info", "--format", "{{.NCPU}}")
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("unreadable engine processors %q: %w", out, err)
	}
	return count, nil
}

// fewEngineCPUsWarning returns the sentence to show before a start when the
// engine has fewer than half of the computer's processors and fewer than
// enoughEngineCPUs, or "" otherwise. A project's builds and test runners size
// their parallelism from the processors they see: a suite observed at three
// minutes with two workers had twenty processors idle beside it. hostOS is the
// GOOS of the machine running the CLI, because where the limit is set differs.
func fewEngineCPUsWarning(engine, host int, hostOS string) string {
	if engine <= 0 || host <= 0 || engine >= enoughEngineCPUs || engine*2 >= host {
		return ""
	}
	suggested := (host + 1) / 2
	return fmt.Sprintf("Nota: Docker dà ai container %d processori su %d del computer. ", engine, host) +
		"Build e test dei progetti nell'ambiente isolato scelgono il proprio parallelismo dai processori che vedono: " +
		"con pochi processori le verifiche di ogni modifica durano più del necessario. " +
		engineCPUHint(hostOS, suggested)
}

func engineCPUHint(hostOS string, suggested int) string {
	target := strconv.Itoa(suggested)
	switch hostOS {
	case "windows":
		return "Con il backend WSL 2 il limite è `processors=` nella sezione [wsl2] di %USERPROFILE%\\.wslconfig: " +
			"portalo ad almeno " + target + ", esegui `wsl --shutdown` e riavvia Docker Desktop. " +
			"Con il backend Hyper-V alzalo in Docker Desktop → Settings → Resources → CPU limit."
	case "darwin":
		return "Alzalo ad almeno " + target + " in Docker Desktop → Settings → Resources → CPU limit e riavvia Docker Desktop."
	default:
		return "Con Docker Desktop il limite è in Settings → Resources → CPU limit (almeno " + target + "); " +
			"con Docker Engine i container usano i processori della macchina, salvo un limite imposto al servizio."
	}
}
