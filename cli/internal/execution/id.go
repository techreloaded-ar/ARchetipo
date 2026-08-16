package execution

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func RandomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("random execution id: %w", err)
	}
	return "exec-" + hex.EncodeToString(bytes[:]), nil
}

// DeriveID turns an idempotency request key into a deterministic execution id.
// The four components are joined with a NUL byte, which cannot appear in any of
// them, so no two distinct requests can canonicalize to the same input string.
// The result is filename-safe and therefore accepted by validID, and the same
// value is reusable as the external identity carried to a remote system, which
// makes local and remote idempotency coincide.
func DeriveID(specCode string, action ActionID, providerID, requestID string) string {
	sum := sha256.Sum256([]byte(specCode + "\x00" + string(action) + "\x00" + providerID + "\x00" + requestID))
	return "req-" + hex.EncodeToString(sum[:16])
}
