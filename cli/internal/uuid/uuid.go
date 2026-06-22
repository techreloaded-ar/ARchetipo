// Package uuid provides a minimal UUID v4 generator backed by crypto/rand.
package uuid

import (
	"crypto/rand"
	"fmt"
)

// NewV4 returns a random UUID v4 (RFC 9562) string.
func NewV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("uuid v4: crypto/rand read: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10xx
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
