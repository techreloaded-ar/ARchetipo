package github

import (
	"context"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// TestInitializeConnector_HappyPath verifies the gh call sequence used by
// initialize_connector resolves repo + project metadata into a SetupInfo.
func TestInitializeConnector_HappyPath(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{
			"id":"R_abc","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"
		}`).
		on("project list --owner acme", `{
			"projects":[{"number":4,"id":"PVT_kw","title":"web Backlog","url":"https://gh/p/4"}]
		}`).
		on("api graphql", `{
			"data":{"node":{"fields":{"nodes":[
				{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[
					{"id":"OPT_todo","name":"TODO"},{"id":"OPT_planned","name":"PLANNED"}
				]},
				{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[
					{"id":"OPT_high","name":"HIGH"}
				]},
				{"id":"FID_sp","name":"Story Points","dataType":"NUMBER"}
			]}}}
		}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	info, err := c.InitializeConnector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Repo == nil || info.Repo.Slug != "acme/web" {
		t.Errorf("repo not resolved: %+v", info.Repo)
	}
	if info.Project == nil || info.Project.Number != 4 {
		t.Errorf("project not resolved: %+v", info.Project)
	}
	if info.Project.Fields.StatusOptions["TODO"] != "OPT_todo" {
		t.Errorf("status option lost: %+v", info.Project.Fields.StatusOptions)
	}
	if !m.calledWithPrefix("repo view") {
		t.Errorf("expected gh repo view to be called")
	}
}

// TestProjectPreference_NoExactMatchCreatesNew is the regression test for the
// Artly/Tela bug: when the owner already has projects whose titles contain
// "Backlog" (e.g. "Tela Backlog") but none matches the current repo exactly
// ("Artly Backlog"), the resolver must NOT reuse the unrelated board — it
// must create a fresh one for the current repo.
func TestProjectPreference_NoExactMatchCreatesNew(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"sleli"},"name":"Artly","nameWithOwner":"sleli/Artly"}`).
		on("project list --owner sleli", `{"projects":[
			{"number":11,"id":"PVT11","title":"Tela Backlog","url":""},
			{"number":7,"id":"PVT7","title":"FoodCost Backlog","url":""}
		]}`).
		on("project create --owner sleli --title Artly Backlog --format json",
			`{"number":12,"id":"PVT12","url":"https://gh/p/12"}`).
		on("project field-create 12 --owner sleli --name Priority", "ok").
		on("project field-create 12 --owner sleli --name Story Points", "ok").
		on("api graphql", `{"data":{"node":{"fields":{"nodes":[
			{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[]}
		]}}}}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	info, err := c.InitializeConnector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Project.Number != 12 {
		t.Errorf("expected freshly created project 12, got %d (cross-repo reuse regression)", info.Project.Number)
	}
	if m.calledWithPrefix("project field-list 11") {
		t.Errorf("must NOT load fields of unrelated project 11 (Tela Backlog)")
	}
}

// TestResolveBoard_ConfigPinsProjectNumber verifies that when the config
// already carries owner + project_number the resolver picks that exact
// project, regardless of title-based preferences.
func TestResolveBoard_ConfigPinsProjectNumber(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[
			{"number":4,"id":"PVT4","title":"web Backlog","url":""},
			{"number":9,"id":"PVT9","title":"Other","url":""}
		]}`).
		on("api graphql", `{"data":{"node":{"fields":{"nodes":[]}}}}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	cfg.GitHub.Owner = "acme"
	cfg.GitHub.ProjectNumber = 9
	c := NewWithRunner(cfg, m)

	info, err := c.InitializeConnector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Project.Number != 9 {
		t.Errorf("expected pinned project 9, got %d", info.Project.Number)
	}
}

// TestResolveBoard_StaleConfigNumberFails verifies that an outdated
// project_number in the config produces an explicit precondition error and
// does NOT silently fall back to title-based discovery.
func TestResolveBoard_StaleConfigNumberFails(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[
			{"number":4,"id":"PVT4","title":"web Backlog","url":""}
		]}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	cfg.GitHub.Owner = "acme"
	cfg.GitHub.ProjectNumber = 99 // not present
	c := NewWithRunner(cfg, m)

	_, err := c.InitializeConnector(context.Background())
	if err == nil {
		t.Fatal("expected precondition error for stale project_number")
	}
	if !strings.Contains(err.Error(), "project_number 99") {
		t.Errorf("error message should mention stale number, got: %v", err)
	}
	if m.calledWithPrefix("project field-list 4") {
		t.Errorf("must NOT fall back to title pipeline when config is stale")
	}
}

// TestResolveBoard_NoProjectTriggersCreate verifies that when the user has
// no project at all the resolver creates one, configures the canonical
// custom fields, and aligns the Status options to workflow.statuses.
func TestResolveBoard_NoProjectTriggersCreate(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[]}`).
		on("project create --owner acme --title web Backlog --format json",
			`{"number":11,"id":"PVT11","url":"https://gh/p/11"}`).
		on("project field-create 11 --owner acme --name Priority", "ok").
		on("project field-create 11 --owner acme --name Story Points", "ok").
		on("api graphql", `{"data":{"node":{"fields":{"nodes":[
			{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[]}
		]}}}}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	info, err := c.InitializeConnector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Project.Number != 11 {
		t.Errorf("expected created project 11, got %d", info.Project.Number)
	}
	for _, want := range []string{
		"project create --owner acme",
		"project field-create 11 --owner acme --name Priority",
		"project field-create 11 --owner acme --name Story Points",
		"api graphql",
	} {
		if !m.calledWithPrefix(want) {
			t.Errorf("expected gh call with prefix %q", want)
		}
	}
}

func TestFetchBacklogItems_UsesLeanGraphQLAndFiltersTasks(t *testing.T) {
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[
			{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}
		]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[
			{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[{"id":"OPT_todo","name":"TODO"}]},
			{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_high","name":"HIGH"}]},
			{"id":"FID_sp","name":"Story Points","dataType":"NUMBER"}
		]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{
			"pageInfo":{"endCursor":"","hasNextPage":false},
			"nodes":[
				{
					"id":"PVTI_story",
					"content":{"__typename":"Issue","number":10,"title":"US-001: Setup","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"},{"name":"EP-001: [Foundations]"}]}},
					"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"},
					"priority":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"HIGH"},
					"storyPoints":{"__typename":"ProjectV2ItemFieldNumberValue","number":3}
				},
				{
					"id":"PVTI_task",
					"content":{"__typename":"Issue","number":11,"title":"TASK-01: Do work","url":"https://gh/i/11","labels":{"nodes":[{"name":"EP-001: [Foundations]"}]}},
					"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"}
				}
			]
		}}}}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	stories, err := c.FetchBacklogItems(context.Background(), domain.StatusTodo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 1 || stories[0].Code != "US-001" {
		t.Fatalf("expected only backlog story US-001, got %+v", stories)
	}
	if m.calledWithPrefix("project item-list") {
		t.Fatal("must not call high-cost gh project item-list")
	}
	if m.calledWithPrefix("project field-list") {
		t.Fatal("must not call high-cost gh project field-list")
	}
}

func TestUpdateSpec_TitleOnlyPatchesIssue(t *testing.T) {
	issueBody := "## Spec\\n\\nAs a user, I want X."
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[{"id":"OPT_todo","name":"TODO"}]},{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_high","name":"HIGH"}]},{"id":"FID_sp","name":"Story Points","dataType":"NUMBER"}]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[{"id":"PVTI_story","content":{"__typename":"Issue","number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"}]}},"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"},"priority":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"HIGH"},"storyPoints":{"__typename":"ProjectV2ItemFieldNumberValue","number":3}}]}}}}`).
		on("api repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","body":"`+issueBody+`","url":"https://gh/i/10","labels":[{"name":"archetipo-backlog"}]}`).
		on("api -X PATCH repos/acme/web/issues/10 -f title=US-001: Updated Setup", `{"number":10,"title":"US-001: Updated Setup","body":"`+issueBody+`","url":"https://gh/i/10"}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	newTitle := "Updated Setup"
	res, err := c.UpdateSpec(context.Background(), "US-001", domain.SpecUpdate{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("expected ok=true")
	}
	if !m.calledWithPrefix("api -X PATCH repos/acme/web/issues/10 -f title=US-001: Updated Setup") {
		t.Error("expected PATCH with updated title")
	}
}

func TestUpdateSpec_BodyWithSpecMetaRoundTrips(t *testing.T) {
	currentBody := "## Spec\\n\\nSome body."
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[{"id":"OPT_todo","name":"TODO"}]},{"id":"FID_pri","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_high","name":"HIGH"}]}]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[{"id":"PVTI_story","content":{"__typename":"Issue","number":10,"title":"US-001: Setup","body":"`+currentBody+`","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"}]}},"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"}}]}}}}`).
		on("api repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","body":"`+currentBody+`","url":"https://gh/i/10","labels":[{"name":"archetipo-backlog"}]}`).
		on("api -X PATCH repos/acme/web/issues/10", `{"number":10,"title":"US-001: Updated Setup","url":"https://gh/i/10"}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	newScope := domain.Scope("MVP")
	newBlockedBy := []string{"US-003"}
	newRework := true
	res, err := c.UpdateSpec(context.Background(), "US-001", domain.SpecUpdate{
		Scope:     &newScope,
		BlockedBy: &newBlockedBy,
		Rework:    &newRework,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("expected ok=true")
	}
	patchCalls := m.calls
	foundBody := false
	for _, c := range patchCalls {
		args := strings.Join(c.args, " ")
		if strings.HasPrefix(args, "api -X PATCH repos/acme/web/issues/10") {
			foundBody = true
			if !strings.Contains(args, "archetipo:spec-meta") {
				t.Errorf("expected spec-meta marker in body, got: %s", args)
			}
			if !strings.Contains(args, `"scope":"MVP"`) {
				t.Errorf("expected scope:MVP in marker, got: %s", args)
			}
		}
	}
	if !foundBody {
		t.Error("expected PATCH call for body update")
	}
}

func TestUpdateSpec_EpicChangeUpdatesLabels(t *testing.T) {
	currentBody := "## Spec\\n\\nBody."
	m := newMock(t).
		on("repo view --json", `{"id":"R","owner":{"login":"acme"},"name":"web","nameWithOwner":"acme/web"}`).
		on("project list --owner acme", `{"projects":[{"number":4,"id":"PVT4","title":"web Backlog","url":"https://gh/p/4"}]}`).
		on("api graphql -f query=\nquery($projectId: ID!)", `{"data":{"node":{"fields":{"nodes":[{"id":"FID_status","name":"Status","dataType":"SINGLE_SELECT","options":[{"id":"OPT_todo","name":"TODO"}]}]}}}}`).
		on("api graphql -f query=\nquery($projectId: ID!, $after: String)", `{"data":{"node":{"items":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[{"id":"PVTI_story","content":{"__typename":"Issue","number":10,"title":"US-001: Setup","body":"`+currentBody+`","url":"https://gh/i/10","labels":{"nodes":[{"name":"archetipo-backlog"},{"name":"EP-001: [Foundations]"}]}},"status":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"TODO"}}]}}}}`).
		on("api repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","body":"`+currentBody+`","url":"https://gh/i/10","labels":[{"name":"archetipo-backlog"},{"name":"EP-001: [Foundations]"}]}`).
		on("api -X PATCH repos/acme/web/issues/10", `{"number":10,"title":"US-001: Setup","url":"https://gh/i/10"}`)

	cfg := config.Default()
	cfg.Connector = config.ConnectorGitHub
	c := NewWithRunner(cfg, m)

	newEpic := domain.Epic{Code: "EP-002", Title: "Security"}
	_, err := c.UpdateSpec(context.Background(), "US-001", domain.SpecUpdate{Epic: &newEpic})
	if err != nil {
		t.Fatal(err)
	}
	foundLabels := false
	for _, call := range m.calls {
		args := strings.Join(call.args, " ")
		if strings.HasPrefix(args, "api -X PATCH repos/acme/web/issues/10") {
			foundLabels = true
			if strings.Contains(args, "EP-001") {
				t.Errorf("old EP-001 label should be removed: %s", args)
			}
			if !strings.Contains(args, "EP-002: [Security]") {
				t.Errorf("new EP-002 label missing: %s", args)
			}
			if !strings.Contains(args, "archetipo-backlog") {
				t.Errorf("archetipo-backlog label missing: %s", args)
			}
		}
	}
	if !foundLabels {
		t.Error("expected PATCH call with labels")
	}
}
