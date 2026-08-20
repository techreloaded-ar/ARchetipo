package execution

import "testing"

// Every action is bound to exactly one capability and one scope. The table is
// exhaustive on purpose: a new action that forgets one of the two mappings must
// fail here and not at the boundary where a caller dispatches it.
func TestActionMapsToItsCapabilityAndScope(t *testing.T) {
	cases := []struct {
		action     ActionID
		capability Capability
		scope      Scope
	}{
		{ActionPlan, CapabilitySpecPlan, ScopeSpec},
		{ActionImplement, CapabilitySpecImplement, ScopeSpec},
		{ActionInception, CapabilityWorkspaceInception, ScopeWorkspace},
		{ActionBacklog, CapabilityWorkspaceBacklog, ScopeWorkspace},
	}
	for _, tc := range cases {
		t.Run(string(tc.action), func(t *testing.T) {
			capability, err := RequiredCapability(tc.action)
			if err != nil {
				t.Fatalf("RequiredCapability(%q) returned error %v, want none", tc.action, err)
			}
			if capability != tc.capability {
				t.Fatalf("RequiredCapability(%q) = %q, want %q", tc.action, capability, tc.capability)
			}
			scope, err := ActionScope(tc.action)
			if err != nil {
				t.Fatalf("ActionScope(%q) returned error %v, want none", tc.action, err)
			}
			if scope != tc.scope {
				t.Fatalf("ActionScope(%q) = %q, want %q", tc.action, scope, tc.scope)
			}
		})
	}
}

func TestUnknownActionIsRejectedByBothMappings(t *testing.T) {
	const unknown ActionID = "deploy"

	if _, err := RequiredCapability(unknown); err == nil {
		t.Fatal("RequiredCapability of an unknown action returned no error, want *ActionError")
	} else if actionErr, ok := err.(*ActionError); !ok {
		t.Fatalf("RequiredCapability returned %T, want *ActionError", err)
	} else if actionErr.Action != unknown {
		t.Fatalf("RequiredCapability error names action %q, want %q", actionErr.Action, unknown)
	}

	if _, err := ActionScope(unknown); err == nil {
		t.Fatal("ActionScope of an unknown action returned no error, want *ActionError")
	} else if actionErr, ok := err.(*ActionError); !ok {
		t.Fatalf("ActionScope returned %T, want *ActionError", err)
	} else if actionErr.Action != unknown {
		t.Fatalf("ActionScope error names action %q, want %q", actionErr.Action, unknown)
	}
}
