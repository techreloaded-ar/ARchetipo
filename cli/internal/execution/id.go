package execution

import (
	"crypto/rand"
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
