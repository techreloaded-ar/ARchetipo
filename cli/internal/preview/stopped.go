package preview

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

// Signal delivery alone does not prove that an owned application's port closed.
// This probes only the explicit endpoint; it never discovers or kills listeners.
func waitStopped(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	endpoint := net.JoinHostPort(u.Hostname(), port)
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", endpoint, 100*time.Millisecond)
		if dialErr != nil {
			return nil
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			return fmt.Errorf("il server risponde ancora su %s; arresto non verificato, ambiente conservato", endpoint)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
