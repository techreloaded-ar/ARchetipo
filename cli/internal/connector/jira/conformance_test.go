package jira

import (
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/conformance"
)

// TestConformance runs the shared connector behavioural suite against the jira
// connector, driving it through the in-memory fake Jira backend. Passing the
// same suite as filefs/inmemory is what guarantees a skill written against the
// contract behaves identically on Jira.
func TestConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) connector.Connector {
		t.Setenv("JIRA_EMAIL", "bot@acme.com")
		t.Setenv("JIRA_API_TOKEN", "tok")
		return NewWithDoer(testConfig(), newFakeJira(t))
	})
}
