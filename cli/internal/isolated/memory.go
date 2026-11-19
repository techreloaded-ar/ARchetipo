package isolated

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// EngineMemoryReporter is implemented by a provider that can say how much
// memory its engine gives to containers. It is optional: a provider without it
// starts without the check, and a failed reading never blocks a start.
type EngineMemoryReporter interface {
	EngineMemory(ctx context.Context) (int64, error)
}

// LowEngineMemoryBytes is the threshold under which a start warns. The
// environment runs the viewer, the agents, the previewed app and the services
// of a recipe (a database, for instance) in the engine's memory together; with
// 2 GB and no swap a Docker Desktop VM was observed to exhaust it and stop
// answering altogether, taking the viewer with it. The engine reports a little
// less than what is configured (a 4 GB limit reads as about 3.8 GiB), so the
// threshold sits under 4 GiB to warn only when the configuration is below 4 GB.
var LowEngineMemoryBytes int64 = 3584 << 20

// recommendedEngineMemory is what the warning suggests configuring.
const recommendedEngineMemory = "8 GB"

// EngineMemory reads the memory the engine reports for its containers.
func (d *Docker) EngineMemory(ctx context.Context) (int64, error) {
	out, err := d.run(ctx, EngineProbeTimeout, "info", "--format", "{{.MemTotal}}")
	if err != nil {
		return 0, err
	}
	total, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unreadable engine memory %q: %w", out, err)
	}
	return total, nil
}

// lowEngineMemoryWarning returns the sentence to show before a start when the
// engine has too little memory, or "" when it has enough or reported nothing.
// hostOS is the GOOS of the machine running the CLI, because where the limit is
// set differs between Windows, macOS and Linux.
func lowEngineMemoryWarning(total int64, hostOS string) string {
	if total <= 0 || total >= LowEngineMemoryBytes {
		return ""
	}
	available := fmt.Sprintf("%.1f GB", float64(total)/float64(1<<30))
	return "Attenzione: Docker dà ai container solo " + available + " di memoria. " +
		"L'ambiente isolato fa girare insieme viewer, agenti, app in anteprima e servizi della recipe (per esempio un database): " +
		"con meno di 4 GB la macchina virtuale di Docker può esaurire la memoria e smettere di rispondere, viewer compreso. " +
		engineMemoryHint(hostOS)
}

func engineMemoryHint(hostOS string) string {
	switch hostOS {
	case "windows":
		return "Con il backend WSL 2 il limite è `memory=` nella sezione [wsl2] di %USERPROFILE%\\.wslconfig (conviene impostare anche `swap=`): " +
			"portalo ad almeno " + recommendedEngineMemory + ", esegui `wsl --shutdown` e riavvia Docker Desktop. " +
			"Con il backend Hyper-V alzalo in Docker Desktop → Settings → Resources → Memory."
	case "darwin":
		return "Alzalo ad almeno " + recommendedEngineMemory + " in Docker Desktop → Settings → Resources → Memory e riavvia Docker Desktop."
	default:
		return "Con Docker Engine il limite è la memoria della macchina, con Docker Desktop è in Settings → Resources → Memory: " +
			"servono almeno " + recommendedEngineMemory + " disponibili per l'ambiente."
	}
}
