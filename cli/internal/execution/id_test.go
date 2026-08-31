package execution

import (
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

func TestDeriveIDIsStableAndComponentSensitive(t *testing.T) {
	const (
		specCode   = "US-001"
		action     = ActionPlan
		providerID = "arcipelago"
		requestID  = "r1"
	)
	base := DeriveID(specCode, action, providerID, requestID)
	if again := DeriveID(specCode, action, providerID, requestID); again != base {
		t.Fatalf("DeriveID is not stable: %q vs %q", base, again)
	}
	if !strings.HasPrefix(base, "req-") || len(base) != 36 {
		t.Fatalf("unexpected derived id shape: %q", base)
	}
	if !recordfile.ValidID(base) {
		t.Fatalf("derived id %q is not accepted by the store", base)
	}
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"spec code", DeriveID("US-002", action, providerID, requestID)},
		{"action", DeriveID(specCode, "other", providerID, requestID)},
		{"provider id", DeriveID(specCode, action, "other", requestID)},
		{"request id", DeriveID(specCode, action, providerID, "r2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got == base {
				t.Fatalf("changing the %s did not change the derived id: %q", tc.name, tc.got)
			}
			if !recordfile.ValidID(tc.got) {
				t.Fatalf("derived id %q is not accepted by the store", tc.got)
			}
		})
	}
}

// A NUL separator is what keeps two different requests from canonicalizing to
// the same input string: without it, ("US-001p", "lan") and ("US-001", "plan")
// would collide.
func TestDeriveIDSeparatesComponents(t *testing.T) {
	if DeriveID("US-001", "plan", "p", "r") == DeriveID("US-001plan", "", "p", "r") {
		t.Fatal("component boundaries are not encoded in the derived id")
	}
}
