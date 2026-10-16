package preview

import (
	"net"
	"sync"
)

// http.Server.Close does not close hijacked WebSocket connections. Keep those
// connections under preview ownership too, including with borrowed app servers.
type previewListener struct {
	net.Listener
	mu          sync.Mutex
	connections map[*previewConnection]struct{}
	closed      bool
}
type previewConnection struct {
	net.Conn
	owner *previewListener
}

func (l *previewListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	wrapped := &previewConnection{Conn: conn, owner: l}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	l.connections[wrapped] = struct{}{}
	return wrapped, nil
}
func (c *previewConnection) Close() error {
	c.owner.mu.Lock()
	delete(c.owner.connections, c)
	c.owner.mu.Unlock()
	return c.Conn.Close()
}
func (l *previewListener) Close() error {
	err := l.Listener.Close()
	l.mu.Lock()
	l.closed = true
	active := make([]*previewConnection, 0, len(l.connections))
	for c := range l.connections {
		active = append(active, c)
	}
	l.mu.Unlock()
	for _, c := range active {
		_ = c.Close()
	}
	return err
}
