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
	Server       *ServerState                `json:"server,omitempty"`
	Resources    *verification.ResourceState `json:"resources,omitempty"`
	CleanupError string                      `json:"cleanup_error,omitempty"`
	Configured   bool                        `json:"configured"`
	Recipe       *Recipe                     `json:"recipe,omitempty"`
	State        string                      `json:"state"`
	URL          string                      `json:"url,omitempty"`
	Token        string                      `json:"token,omitempty"`
	Error        string                      `json:"error,omitempty"`
	ErrorCode    string                      `json:"error_code,omitempty"`
	Log          string                      `json:"log,omitempty"`
	Reused       bool                        `json:"reused"`
	Preparation  *verification.Result        `json:"preparation,omitempty"`
}

type Instance struct {
	root              string
	mu                sync.Mutex
	view              View
	cancel            context.CancelFunc
	done              chan struct{}
	server            *http.Server
	retainEnvironment bool
	runID             string
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

func (i *Instance) commandEnded(root string, err error) {
	state, readErr := readServerState(root)
	if readErr == nil && state != nil && state.RunID == i.runID && state.State == "stopped" && serverStopRequested(root, state.RunID) {
		i.mu.Lock()
		i.view.State = "stopping"
		i.mu.Unlock()
		return
	}
	i.fail(fmt.Errorf("il server della preview è terminato: %v", err))
}

type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	roots     map[string]string
	closed    bool
}

func NewManager() *Manager {
	return &Manager{instances: map[string]*Instance{}, roots: map[string]string{}}
}

func (m *Manager) Status(root, id string) (View, error) {
	m.mu.Lock()
	if owner := m.roots[root]; owner != "" {
		id = owner
	}
	instance := m.instances[id]
	m.mu.Unlock()
	if instance != nil {
		view := instance.Snapshot()
		return resourceView(root, view)
	}
	r, err := Load(root)
	if err != nil {
		// Missing recipes are an ordinary first-use state, invalid ones are errors.
		if code, ok := err.(*iox.CodedError); ok && code.Code == iox.CodePreconditionMissing {
			return resourceView(root, View{State: "unconfigured"})
		}
		return View{}, err
	}
	return resourceView(root, View{Configured: true, Recipe: &r, State: "stopped"})
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
	if owner := m.roots[root]; owner != "" && owner != id {
		if existing := m.instances[owner]; existing != nil {
			if state := existing.Snapshot(); state.State != "error" && state.State != "stopped" {
				return state, nil
			}
		}
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
	if environment, loadErr := verification.Load(root); loadErr == nil && len(environment.Resources) > 0 {
		if !r.PrepareEnvironment || !strings.Contains(r.URL, "{port}") {
			return View{}, invalid("managed environment previews require prepare_environment and a dynamic {port} launch contract")
		}
	}
	dir, err := r.Validate(root)
	if err != nil {
		return View{}, err
	}
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return View{}, err
	}
	lock, err := store.TryLock(context.Background(), "preview-environment")
	if err != nil {
		return View{}, iox.NewConflict("another viewer owns this preview", "stop the existing preview before starting another", err)
	}
	commandLock, err := store.TryLock(context.Background(), "preview-command")
	if err != nil {
		_ = lock.Unlock()
		return View{}, iox.NewConflict("the previous preview command is still ending", "wait for its server tree to stop before starting another preview", err)
	}
	if err = commandLock.Unlock(); err != nil {
		_ = lock.Unlock()
		return View{}, err
	}
	if err = ReconcileStopped(root); err != nil {
		_ = lock.Unlock()
		return View{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := "starting"
	if r.PrepareEnvironment || prepareOnly {
		state = "preparing"
	}
	i := &Instance{root: root, view: View{Configured: true, Recipe: &r, State: state}, cancel: cancel, done: make(chan struct{})}
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
	m.roots[root] = id
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
		i.mu.Lock()
		if i.view.State == "stopping" {
			i.view.State = "stopped"
		}
		i.mu.Unlock()
	}()
	defer func() {
		if listener != nil {
			_ = listener.Close()
			_ = i.server.Close()
		}
	}()
	defer i.cancel()
	service := verification.New(root, "").ForOwnedPreview()
	if r.PrepareEnvironment && !prepareOnly {
		lifetime, err := verification.OwnLifetime(root, i)
		if err != nil {
			i.fail(err)
			return
		}
		if lifetime != nil {
			service = lifetime.Service("")
			watchCtx, stopWatch := context.WithCancel(ctx)
			defer stopWatch()
			go func() {
				select {
				case <-watchCtx.Done():
				case <-lifetime.Done():
					if watchCtx.Err() == nil {
						i.fail(fmt.Errorf("il supervisore dell’ambiente è terminato; verifica i servizi registrati"))
						i.cancel()
					}
				}
			}()
		}
		defer func() {
			i.cancel()
			i.mu.Lock()
			retain := i.retainEnvironment
			i.mu.Unlock()
			var err error
			if retain {
				err = lifetime.Release()
			} else {
				err = lifetime.Close()
			}
			if err != nil {
				i.mu.Lock()
				i.view.CleanupError = err.Error()
				i.mu.Unlock()
				i.fail(err)
			}
		}()
	}
	if r.PrepareEnvironment || prepareOnly {
		result, err := service.Execute(ctx, "prepare", "", "", false)
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
		env, envErr := verification.New(root, "").RuntimeEnv()
		if envErr != nil {
			i.fail(envErr)
			return
		}
		identity, identityErr := newServerState(r.URL)
		if identityErr != nil {
			i.fail(identityErr)
			return
		}
		i.runID = identity.RunID
		go func() { exited <- runCommand(ctx, root, dir, r.URL, identity.RunID, r.Argv, env, i) }()
	}
	defer func() {
		if !reused {
			i.cancel()
			<-exited //nolint:errcheck // Cleanup reaps the already observed command exit.
			if err := waitStopped(r.URL); err != nil {
				i.mu.Lock()
				i.view.CleanupError = err.Error()
				i.mu.Unlock()
				i.fail(err)
			}
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
			i.commandEnded(root, err)
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
		i.commandEnded(root, err)
	}
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stop(id)
}

// StopOwned releases only this manager's preview for the checkout. Development
// must release its lifetime before using the standalone verification contract;
// a terminal run or another viewer remains protected by its own lifetime lock.
func (m *Manager) StopOwned(root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id := m.roots[root]; id != "" {
		return m.stop(id)
	}
	return nil
}

// SuspendOwned stops this manager's application/proxy and releases its lifetime
// lock while retaining the checkout's prepared services, ports and test data.
func (m *Manager) SuspendOwned(root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.roots[root]
	if i := m.instances[id]; i != nil {
		i.mu.Lock()
		i.retainEnvironment = true
		i.mu.Unlock()
		i.cancel()
		<-i.done
		if message := i.Snapshot().CleanupError; message != "" {
			return fmt.Errorf("rilascio ambiente: %s", message)
		}
		delete(m.instances, id)
		delete(m.roots, root)
	}
	return nil
}

func (m *Manager) stop(id string) error {
	if i := m.instances[id]; i != nil {
		i.cancel()
		<-i.done
		if message := i.Snapshot().CleanupError; message != "" {
			// A retry can confirm that an asynchronously stopping server is gone.
			if recipe := i.Snapshot().Recipe; recipe != nil {
				if err := waitStopped(recipe.URL); err != nil {
					return err
				}
			}
		}
		// Also recover an earlier failed cleanup, or services left by Prepare.
		service := verification.New(i.root, "")
		resources, err := service.Resources()
		if err != nil {
			return err
		}
		active := false
		for _, r := range resources.Resources {
			active = active || r.State != "stopped"
		}
		if active {
			result, cleanupErr := service.Execute(context.Background(), "cleanup", "", "", false)
			if cleanupErr != nil {
				i.fail(cleanupErr)
				return cleanupErr
			}
			if !result.Passed {
				err = fmt.Errorf("arresto ambiente non riuscito; consulta %s", result.ReportPath)
				i.fail(err)
				return err
			}
		}
		delete(m.instances, id)
	}
	return nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	defer m.mu.Unlock()
	for id, i := range m.instances {
		i.cancel()
		<-i.done
		// Prepare is an explicit durable setup, reused by a later run. Running
		// previews have their independent lifetime guard and have already cleaned up.
		delete(m.instances, id)
	}
}

// StopEnvironment also works after a viewer restart, when only the durable
// resource journal remains. A live owner's lifetime lock prevents takeover.
func (m *Manager) StopEnvironment(root, id string) error {
	m.mu.Lock()
	if owner := m.roots[root]; owner != "" {
		id = owner
	}
	m.mu.Unlock()
	if err := m.Stop(id); err != nil {
		return err
	}
	if err := requestServerStop(root); err != nil {
		return err
	}
	if err := ReconcileStopped(root); err != nil {
		return err
	}
	service := verification.New(root, "")
	resources, err := service.Resources()
	if err != nil {
		return err
	}
	active := false
	for _, r := range resources.Resources {
		active = active || r.State != "stopped"
	}
	if !active {
		return nil
	}
	result, err := service.Execute(context.Background(), "cleanup", "", "", false)
	if err != nil {
		return err
	}
	if !result.Passed {
		return fmt.Errorf("arresto ambiente non riuscito; consulta %s", result.ReportPath)
	}
	return nil
}

func resourceView(root string, view View) (View, error) {
	server, err := observedServerState(root)
	if err != nil {
		return view, err
	}
	view.Server = server
	if server != nil && server.State != "stopped" && !server.OwnerActive {
		view.State = "error"
		view.CleanupError = "Il proprietario della preview è terminato senza un arresto verificato. Usa Ferma ambiente per controllare il servizio registrato."
		view.Error = view.CleanupError
	}
	resources, err := verification.New(root, "").Resources()
	if err != nil {
		return view, err
	}
	if resources.Owner != "" {
		view.Resources = &resources
	}
	for _, r := range resources.Resources {
		busy := view.State == "preparing" || view.State == "starting" || view.State == "stopping"
		if r.State != "stopped" && r.State != "ready" && (!busy || r.State == "error") {
			view.State = "error"
			view.Error = "L’ambiente contiene servizi registrati da verificare o fermare. Usa Ferma ambiente per riconciliarli."
			break
		}
	}
	return view, nil
}
