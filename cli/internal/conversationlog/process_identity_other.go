//go:build !linux

package conversationlog

// processIdentity is not recorded outside Linux, where locks do not outlive
// a container whose processes restart from the same pids.
func processIdentity(int) string { return "" }
