package github

import (
	"context"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// TestSavePlan_TaskDescriptionFallback verifies that when a task has
// Description but empty Body, the GitHub connector uses Description as
// the sub-issue body instead of leaving it empty.
func TestSavePlan_TaskDescriptionFallback(t *testing.T) {
	issueBody := "## Spec\\n\\nAs a user, I want X."
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[
			{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[
				{"id":"OPT_todo","name":"TODO"},{"id":"OPT_planned","name":"PLANNED"}
			]},
			{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_high","name":"HIGH"}]}
		]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[
			{"id":"PVTI_story","content":{"__typename":"Issue","number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"}]}},"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"}}
		]}}}}`).
		on("api repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":[{"name":"archetipo-backlog"}]}`).
		on("api -X PATCH repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","url":"https://gh/i/10"}`).
		// Sub-issue create: the body must contain the Description text.
		on("api -X POST repos/acme/web/issues -f title=TASK-01: Schema DB", `{"number":20,"id":20,"node_id":"I_20","title":"TASK-01: Schema DB","body":"Create database schema","url":"https://gh/i/20"}`).
		on("api -X POST repos/acme/web/issues/10/sub_issues", "ok")

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	plan := domain.PlanInput{
		PlanBody: "## Soluzione\\n\\nDetail.",
		Tasks: []domain.Task{
			{ID: "TASK-01", Title: "Schema DB", Description: "Create database schema", Type: domain.TaskImpl, Status: domain.StatusTodo},
		},
	}
	res, err := c.SavePlan(context.Background(), "US-001", plan)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("expected ok=true")
	}

	// Verify the sub-issue creation call included the Description as body.
	foundCreate := false
	for _, call := range m.calls {
		args := strings.Join(call.args, " ")
		if strings.HasPrefix(args, "api -X POST repos/acme/web/issues -f title=") {
			foundCreate = true
			if !strings.Contains(args, "body=Create database schema") {
				t.Errorf("expected sub-issue body to contain Description text, got: %s", args)
			}
		}
	}
	if !foundCreate {
		t.Error("expected sub-issue create POST call")
	}
}

// TestSavePlan_BodyTakesPrecedenceOverDescription verifies that when both
// Body and Description are populated, Body wins.
func TestSavePlan_BodyTakesPrecedenceOverDescription(t *testing.T) {
	issueBody := "## Spec\\n\\nAs a user, I want X."
	richTaskBody := "## Descrizione\n\nBody wins\n\n## File Coinvolti\n- internal/schema.sql — creare lo schema\n\n## Criteri di Completamento\n- [ ] checklist"
	escapedRichTaskBody := strings.ReplaceAll(richTaskBody, "\n", "\\n")
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[
			{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[
				{"id":"OPT_todo","name":"TODO"},{"id":"OPT_planned","name":"PLANNED"}
			]},
			{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_high","name":"HIGH"}]}
		]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[
			{"id":"PVTI_story","content":{"__typename":"Issue","number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"}]}},"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"}}
		]}}}}`).
		on("api repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":[{"name":"archetipo-backlog"}]}`).
		on("api -X PATCH repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","url":"https://gh/i/10"}`).
		// Sub-issue create: Body takes precedence.
		on("api -X POST repos/acme/web/issues -f title=TASK-01: Schema DB", `{"number":20,"id":20,"node_id":"I_20","title":"TASK-01: Schema DB","body":"`+escapedRichTaskBody+`","url":"https://gh/i/20"}`).
		on("api -X POST repos/acme/web/issues/10/sub_issues", "ok")

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	plan := domain.PlanInput{
		PlanBody: "## Soluzione\\n\\nDetail.",
		Tasks: []domain.Task{
			{ID: "TASK-01", Title: "Schema DB", Description: "Description text", Body: richTaskBody, Type: domain.TaskImpl, Status: domain.StatusTodo},
		},
	}
	res, err := c.SavePlan(context.Background(), "US-001", plan)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("expected ok=true")
	}

	foundCreate := false
	for _, call := range m.calls {
		args := strings.Join(call.args, " ")
		if strings.HasPrefix(args, "api -X POST repos/acme/web/issues -f title=") {
			foundCreate = true
			if strings.Contains(args, "body=Description text") {
				t.Errorf("Body should take precedence over Description, but Description text found: %s", args)
			}
			if !strings.Contains(args, "body=## Descrizione") || !strings.Contains(args, "Criteri di Completamento") {
				t.Errorf("expected rich task body in sub-issue, got: %s", args)
			}
		}
	}
	if !foundCreate {
		t.Error("expected sub-issue create POST call")
	}
}

func TestReadSpecTasksReturnsCleanRichBody(t *testing.T) {
	richTaskBody := "## Descrizione\n\nBody wins\n\n## File Coinvolti\n- internal/schema.sql — creare lo schema\n\n## Criteri di Completamento\n- [ ] checklist"
	escapedRichTaskBody := strings.ReplaceAll(richTaskBody, "\n", "\\n")
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api repos/acme/web/issues/10/sub_issues", `[{"number":20,"title":"TASK-01: Schema DB","body":"`+escapedRichTaskBody+`","state":"open"}]`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)
	c.state.repo = &domain.RepoInfo{Slug: "acme/web"}
	c.state.project = &domain.ProjectInfo{}

	tasks, err := c.ReadSpecTasks(context.Background(), "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Body != richTaskBody {
		t.Fatalf("expected rich task body, got %q", tasks[0].Body)
	}
	if tasks[0].Description != "" {
		t.Fatalf("expected description to stay empty on canonical GitHub read, got %q", tasks[0].Description)
	}
	if tasks[0].Status != domain.StatusTodo {
		t.Fatalf("expected open sub-issue to map to TODO, got %s", tasks[0].Status)
	}
}
