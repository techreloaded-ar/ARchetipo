package cli_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/workflowrun"
)

func TestWorkflowCheckpointCLI(t *testing.T) {
	newProject(t)
	started := runCLI(t, `{"skill":"archetipo-example","invocation":"/archetipo-example US-001","scope":"US-001","state":{"completed":["TASK-01"]}}`, "execution", "workflow", "start")
	if started.exit != 0 {
		t.Fatal(started.stderr.String())
	}
	var envelope struct {
		Kind string             `json:"kind"`
		Data workflowrun.Record `json:"data"`
	}
	if err := json.Unmarshal(started.stdout.Bytes(), &envelope); err != nil || envelope.Kind != "workflow_run" {
		t.Fatalf("bad envelope: %s", started.stdout.String())
	}
	r := envelope.Data
	checkpoint := fmt.Sprintf(`{"revision":%d,"status":"blocked","phase":"implement","last_error":"missing local privilege","state":{"completed":["TASK-01"]}}`, r.Revision)
	saved := runCLI(t, checkpoint, "execution", "workflow", "checkpoint", r.ID)
	if saved.exit != 0 {
		t.Fatal(saved.stderr.String())
	}
	stale := runCLI(t, checkpoint, "execution", "workflow", "checkpoint", r.ID)
	if stale.exit == 0 {
		t.Fatal("stale checkpoint accepted")
	}
	shown := runCLI(t, "", "execution", "workflow", "show", r.ID)
	if shown.exit != 0 {
		t.Fatal(shown.stderr.String())
	}
	if err := json.Unmarshal(shown.stdout.Bytes(), &envelope); err != nil || envelope.Data.Status != workflowrun.Blocked || envelope.Data.Revision != 2 {
		t.Fatal(shown.stdout.String())
	}
}
