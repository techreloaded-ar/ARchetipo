package providerconfig

import "testing"

func TestSharedParsing(t *testing.T) {
	if got, err := ParseSeconds(map[string]any{"timeout": float64(12)}, "timeout", 1, 1, 20); err != nil || got != 12 {
		t.Fatalf("ParseSeconds() = %d, %v", got, err)
	}
	if _, err := ParseSeconds(map[string]any{"timeout": 1.5}, "timeout", 1, 1, 20); err == nil {
		t.Fatal("ParseSeconds accepted a fraction")
	}
	if got, err := String(" model ", "model", "", true); err != nil || got != "model" {
		t.Fatalf("String() = %q, %v", got, err)
	}
	if err := RejectUnknownKeys(map[string]any{"z": true, "a": true}, map[string]any{}, "test"); err == nil || err.Error() != `provider configuration field "a" is not a recognized test provider configuration key` {
		t.Fatalf("RejectUnknownKeys() = %v", err)
	}
}
