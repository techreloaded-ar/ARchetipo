package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

type View struct {
	Configured  bool                 `json:"configured"`
	Recipe      *Recipe              `json:"recipe,omitempty"`
	State       string               `json:"state"`
	URL         string               `json:"url,omitempty"`
	Token       string               `json:"token,omitempty"`
	Error       string               `json:"error,omitempty"`
	ErrorCode   string               `json:"error_code,omitempty"`
	Log         string               `json:"log,omitempty"`
	Reused      bool                 `json:"reused"`
	Preparation *verification.Result `json:"preparation,omitempty"`
}

type Instance struct {
	mu     sync.Mutex
	view   View
	cancel context.CancelFunc
	done   chan struct{}
	server *http.Server
}

func (i *Instance) Snapshot() View { i.mu.Lock(); defer i.mu.Unlock(); return i.view }
func (i *Instance) Write(p []byte) (int, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.view.Log += string(p)
	if len(i.view.Log) > 16000 {
		i.view.Log = i.view.Log[len(i.view.Log)-16000:]
	}
	return len(p), nil
}
func (i *Instance) fail(err error) {
	i.mu.Lock()
	i.view.State = "error"
	i.view.Error = err.Error()
	var coded *iox.CodedError
	if errors.As(err, &coded) {
		i.view.ErrorCode = coded.Code
	}
	i.mu.Unlock()
}

type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	closed    bool
}

func NewManager() *Manager { return &Manager{instances: map[string]*Instance{}} }

func (m *Manager) Status(root, id string) (View, error) {
	m.mu.Lock()
	instance := m.instances[id]
	m.mu.Unlock()
	if instance != nil {
		return instance.Snapshot(), nil
	}
	r, err := Load(root)
	if err != nil {
		// Missing recipes are an ordinary first-use state, invalid ones are errors.
		if code, ok := err.(*iox.CodedError); ok && code.Code == iox.CodePreconditionMissing {
			return View{State: "unconfigured"}, nil
		}
		return View{}, err
	}
	return View{Configured: true, Recipe: &r, State: "stopped"}, nil
}

func (m *Manager) Start(root, id, parentOrigin string) (View, error) {
	return m.launch(root, id, parentOrigin, false)
}

// Prepare executes the same standalone environment contract as Start, without
// launching an application server. It never invokes an agent or guesses setup.
func (m *Manager) Prepare(root, id string) (View, error) {
	return m.launch(root, id, "", true)
}

func (m *Manager) launch(root, id, parentOrigin string, prepareOnly bool) (View, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return View{}, iox.NewConflict("workspace preview manager is stopped", "open the workspace again", nil)
	}
	if existing := m.instances[id]; existing != nil {
		if state := existing.Snapshot(); state.State != "error" && state.State != "stopped" {
			return state, nil
		}
		existing.cancel()
		<-existing.done
		delete(m.instances, id)
	}
	r, err := Load(root)
	if err != nil {
		return View{}, err
	}
	dir, err := r.Validate(root)
	if err != nil {
		return View{}, err
	}
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return View{}, err
	}
	lock, err := store.TryLock(context.Background(), "preview-"+id)
	if err != nil {
		return View{}, iox.NewConflict("another viewer owns this preview", "stop the existing preview before starting another", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := "starting"
	if r.PrepareEnvironment || prepareOnly {
		state = "preparing"
	}
	i := &Instance{view: View{Configured: true, Recipe: &r, State: state}, cancel: cancel, done: make(chan struct{})}
	var listener net.Listener
	if !prepareOnly {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			cancel()
			_ = lock.Unlock()
			return View{}, err
		}
		bytes := make([]byte, 24)
		if _, err = rand.Read(bytes); err != nil {
			cancel()
			_ = listener.Close()
			_ = lock.Unlock()
			return View{}, err
		}
		target, _ := url.Parse(strings.ReplaceAll(r.URL, "{port}", "1"))
		i.view.URL = "http://" + listener.Addr().String() + target.RequestURI()
		i.view.Token = hex.EncodeToString(bytes)
		i.server = &http.Server{ReadHeaderTimeout: 5 * time.Second}
		listener = &previewListener{Listener: listener, connections: map[*previewConnection]struct{}{}}
	}
	m.instances[id] = i
	go i.run(ctx, root, dir, parentOrigin, r, listener, lock, prepareOnly)
	return i.Snapshot(), nil
}

func ready(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 750 * time.Millisecond, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 400
}

func (i *Instance) run(ctx context.Context, root, dir, parentOrigin string, r Recipe, listener net.Listener, lock *conversationlog.ConversationLock, prepareOnly bool) {
	defer close(i.done)
	defer func() { _ = lock.Unlock() }()
	defer func() {
		if listener != nil {
			_ = listener.Close()
			_ = i.server.Close()
		}
	}()
	defer i.cancel()
	if r.PrepareEnvironment || prepareOnly {
		result, err := verification.New(root, "").Execute(ctx, "prepare", "", "", false)
		i.mu.Lock()
		i.view.Preparation = &result
		i.mu.Unlock()
		if err != nil {
			i.fail(err)
			return
		}
		if !result.Passed || !result.Stable {
			i.fail(fmt.Errorf("preparazione ambiente non riuscita; consulta %s", result.ReportPath))
			return
		}
	}
	if prepareOnly {
		i.mu.Lock()
		i.view.State = "stopped"
		i.mu.Unlock()
		return
	}
	r, owned, err := r.resolvePort()
	if err != nil {
		i.fail(err)
		return
	}
	// Build the proxy only after resolving this instance's application port.
	i.server.Handler = proxy(r, parentOrigin, i.view.Token)
	i.mu.Lock()
	i.view.Recipe = &r
	i.view.State = "starting"
	i.mu.Unlock()
	exited := make(chan error, 1)
	// An existing server is borrowed, never killed by Stop.
	reused := !owned && ready(ctx, r.URL)
	if !reused && len(r.Argv) == 0 {
		i.fail(fmt.Errorf("nessun server risponde a %s e non è configurato un comando di avvio", r.URL))
		return
	}
	if !reused {
		go func() { exited <- runCommand(ctx, dir, r.Argv, i) }()
	}
	defer func() {
		if !reused {
			i.cancel()
			<-exited //nolint:errcheck // Cleanup reaps the already observed command exit.
		}
	}()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for !reused && !ready(ctx, r.URL) {
		select {
		case <-ctx.Done():
			return
		case err := <-exited:
			// Put the result back for the deferred reap.
			exited <- err
			i.fail(fmt.Errorf("il comando della preview è terminato: %v", err))
			return
		case <-deadline.C:
			i.fail(fmt.Errorf("il server non risponde a %s dopo 30 secondi", r.URL))
			return
		case <-tick.C:
		}
	}
	i.mu.Lock()
	i.view.State = "ready"
	i.view.Reused = reused
	i.mu.Unlock()
	served := make(chan error, 1)
	go func() { served <- i.server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err := <-served:
		if err != http.ErrServerClosed {
			i.fail(err)
		}
	case err := <-exited:
		exited <- err
		i.fail(fmt.Errorf("il server della preview è terminato: %v", err))
	}
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := m.instances[id]; i != nil {
		i.cancel()
		<-i.done
		delete(m.instances, id)
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	defer m.mu.Unlock()
	for id, i := range m.instances {
		i.cancel()
		<-i.done
		delete(m.instances, id)
	}
}
