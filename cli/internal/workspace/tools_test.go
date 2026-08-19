package workspace

import (
	"errors"
	"reflect"
	"testing"
)

func TestToolsExposesTheRegistryInOrder(t *testing.T) {
	want := []string{"claude", "codex", "cursor", "gemini", "opencode", "copilot", "pi"}
	if got := ToolKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ToolKeys() = %v, want %v", got, want)
	}
	for _, tool := range Tools() {
		if tool.Name == "" || tool.SkillsDir == "" {
			t.Fatalf("tool %q has an empty name or skills dir: %+v", tool.Key, tool)
		}
	}
}

func TestToolsReturnsADetachedCopy(t *testing.T) {
	first := Tools()
	first[0].Key = "mutated"
	if second := Tools(); second[0].Key != "claude" {
		t.Fatalf("mutating the returned slice reached the registry: got %q", second[0].Key)
	}
}

func TestResolveToolsPreservesOrderAndDeduplicates(t *testing.T) {
	got, err := ResolveTools([]string{"pi", " CLAUDE ", "pi"})
	if err != nil {
		t.Fatalf("ResolveTools returned an error: %v", err)
	}
	if len(got) != 2 || got[0].Key != "pi" || got[1].Key != "claude" {
		t.Fatalf("ResolveTools = %+v, want [pi claude]", got)
	}
}

func TestResolveToolsRejectsAnUnknownKey(t *testing.T) {
	_, err := ResolveTools([]string{"pi", "nope"})
	var unknown *UnknownToolError
	if !errors.As(err, &unknown) {
		t.Fatalf("ResolveTools error = %v, want *UnknownToolError", err)
	}
	if unknown.Key != "nope" {
		t.Fatalf("UnknownToolError.Key = %q, want %q", unknown.Key, "nope")
	}
}

func TestConnectorsAreTheAcceptedList(t *testing.T) {
	want := []string{"file", "github", "jira"}
	if got := Connectors(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Connectors() = %v, want %v", got, want)
	}
	for _, id := range want {
		if !IsConnector(id) {
			t.Fatalf("IsConnector(%q) = false, want true", id)
		}
		if ConnectorLabel(id) == id {
			t.Fatalf("ConnectorLabel(%q) has no description", id)
		}
	}
	if IsConnector("nope") {
		t.Fatal("IsConnector(\"nope\") = true, want false")
	}
}
