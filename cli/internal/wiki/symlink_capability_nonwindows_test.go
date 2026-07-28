//go:build !windows

package wiki

func symlinkCapabilityUnavailable(error) bool {
	return false
}
