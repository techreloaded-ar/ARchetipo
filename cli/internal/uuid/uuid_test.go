package uuid

import (
	"regexp"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewV4(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		s, err := NewV4()
		if err != nil {
			t.Fatalf("NewV4() returned error: %v", err)
		}
		if s == "" {
			t.Fatal("NewV4() returned empty string")
		}
		if !uuidV4Re.MatchString(s) {
			t.Fatalf("NewV4() returned non-UUIDv4: %q", s)
		}
		if _, dup := seen[s]; dup {
			t.Fatalf("NewV4() returned duplicate UUID: %q", s)
		}
		seen[s] = struct{}{}
	}
}
