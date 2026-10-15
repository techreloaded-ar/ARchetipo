package execution

import (
	"fmt"
	"strings"
	"testing"
)

func TestWikiActionsShareReceiptCapabilityAndWorkspaceScope(t *testing.T) {
	for _, action := range []ActionID{ActionWikiCreate, ActionWikiRefresh, ActionWikiLint, ActionWikiQuery} {
		t.Run(string(action), func(t *testing.T) {
			capability, err := RequiredCapability(action)
			if err != nil || capability != CapabilityWorkspaceWiki {
				t.Fatalf("capability: %s %v", capability, err)
			}
			scope, err := ActionScope(action)
			if err != nil || scope != ScopeWorkspace {
				t.Fatalf("scope: %s %v", scope, err)
			}
			line := fmt.Sprintf(`{"artifact":"wiki","operation":"%s","status":"COMPLETED"}`, WikiOperation(action))
			receipt, found, err := AcceptActionReceipt(action, "", line)
			if err != nil || !found {
				t.Fatalf("receipt: %s %v", receipt, err)
			}
			if _, err := ActionResultFields(action, receipt); err != nil {
				t.Fatal(err)
			}
			if _, found, err := AcceptActionReceipt(action, "", "Una domanda?"); found || err == nil {
				t.Fatal("a question became a receipt")
			}
			if _, found, err := AcceptActionReceipt(action, "", strings.Replace(line, "COMPLETED", "REVIEWED", 1)); !found || err == nil {
				t.Fatal("approval became completion")
			}
			if _, found, err := AcceptActionReceipt(action, "", strings.Replace(line, WikiOperation(action), "other", 1)); !found || err == nil {
				t.Fatal("another operation completed this action")
			}
		})
	}
}
