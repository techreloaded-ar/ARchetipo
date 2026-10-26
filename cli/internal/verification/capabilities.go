package verification

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

type Capability struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}
type Capabilities struct {
	WorkingDirectory   string                `json:"working_directory"`
	WorkspaceDirectory string                `json:"workspace_directory"`
	OS                 string                `json:"os"`
	Architecture       string                `json:"architecture"`
	ObservedAt         time.Time             `json:"observed_at"`
	LocalWrite         Capability            `json:"local_write"`
	CommonWrite        Capability            `json:"common_recipe_write"`
	Loopback           Capability            `json:"loopback_binding"`
	Tools              map[string]Capability `json:"tools"`
	Network            Capability            `json:"external_network"`
	Probes             []Result              `json:"probes"`
	ReportPath         string                `json:"report_path"`
}

func writeCapability(root string) Capability {
	file, err := os.CreateTemp(filepath.Join(root, ".archetipo"), ".capability-")
	if err != nil {
		return Capability{State: "unavailable", Detail: err.Error()}
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return Capability{State: "unavailable", Detail: closeErr.Error()}
	}
	if removeErr != nil {
		return Capability{State: "unavailable", Detail: removeErr.Error()}
	}
	return Capability{State: "available", Detail: "temporary file created and removed; does not certify all workspace operations"}
}

// Inspect runs in the invoking process, never in a more privileged viewer on
// its behalf. It does not infer a framework, discover secrets, contact an
// external endpoint or assume that finding a client means its daemon is usable.
func (s *Service) Inspect(ctx context.Context, tools []string, probes []Command) (Capabilities, error) {
	report := Capabilities{WorkingDirectory: s.Root, WorkspaceDirectory: environmentrecipe.Workspace(s.Root), OS: runtime.GOOS, Architecture: runtime.GOARCH, ObservedAt: time.Now().UTC(), Tools: map[string]Capability{}, Network: Capability{State: "unknown", Detail: "not probed; inspect harness policy and explicitly test only required endpoints"}, Probes: []Result{}}
	if err := validateCommands(probes); err != nil {
		return report, err
	}
	for _, probe := range probes {
		if probe.TimeoutSeconds > 30 {
			return report, invalid("capability probes must finish within 30 seconds")
		}
	}
	report.LocalWrite = writeCapability(s.Root)
	report.CommonWrite = writeCapability(report.WorkspaceDirectory)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		report.Loopback = Capability{State: "unavailable", Detail: err.Error()}
	} else {
		_ = listener.Close()
		report.Loopback = Capability{State: "available", Detail: "bound and released an ephemeral IPv4 loopback port"}
	}
	for _, tool := range tools {
		path, findErr := exec.LookPath(tool)
		if findErr != nil {
			report.Tools[tool] = Capability{State: "unavailable", Detail: findErr.Error()}
		} else {
			report.Tools[tool] = Capability{State: "available", Detail: path}
		}
	}
	container, err := s.newResult("capabilities", "")
	if err != nil {
		return report, err
	}
	report.ReportPath = filepath.Join(filepath.Dir(container.ReportPath), "capabilities.json")
	for _, probe := range probes {
		result, newErr := s.newResult("capability:"+probe.Name, "")
		if newErr != nil {
			return report, newErr
		}
		runErr := s.run(ctx, &result, []Command{probe})
		report.Probes = append(report.Probes, result)
		if runErr != nil {
			return report, runErr
		}
	}
	container.Passed, container.Stable, container.Status = true, true, "complete"
	if err = s.saveResult(&container); err != nil {
		return report, err
	}
	return report, recordfile.WriteAtomic(filepath.Dir(report.ReportPath), report.ReportPath, ".capabilities-", report)
}
