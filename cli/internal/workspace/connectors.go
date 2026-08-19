package workspace

import "strings"

// connectorLabels describes the connectors an initialization accepts, in the
// order every caller shows them. It is the same list `archetipo init`
// validates `--connector` against, so what the viewer offers and what the CLI
// accepts cannot drift apart.
var connectorLabels = []struct {
	ID    string
	Label string
}{
	{"file", "file — backlog and planning as local Markdown files"},
	{"github", "github — GitHub Projects v2 (requires gh CLI)"},
	{"jira", "jira — Jira Cloud (requires JIRA_EMAIL/JIRA_API_TOKEN)"},
}

// Connectors lists the accepted connector ids.
func Connectors() []string {
	out := make([]string, len(connectorLabels))
	for i, c := range connectorLabels {
		out[i] = c.ID
	}
	return out
}

// ConnectorLabel returns the human-readable description of a connector, or the
// id itself when the connector is unknown.
func ConnectorLabel(id string) string {
	for _, c := range connectorLabels {
		if c.ID == id {
			return c.Label
		}
	}
	return id
}

// IsConnector reports whether id is one an initialization accepts.
func IsConnector(id string) bool {
	for _, c := range connectorLabels {
		if c.ID == id {
			return true
		}
	}
	return false
}

// ConnectorsHint renders the accepted ids for an error hint.
func ConnectorsHint() string {
	return strings.Join(Connectors(), ", ")
}
