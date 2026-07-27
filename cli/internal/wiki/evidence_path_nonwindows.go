//go:build !windows

package wiki

func validatePlatformEvidenceComponent(_ string, _ bool) error { return nil }
