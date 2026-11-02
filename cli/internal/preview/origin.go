package preview

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

// The browser's local storage belongs to an origin, not to the upstream app.
// Keep the checkout's proxy port across stops, revisions and viewer restarts.
// Never fall back to a new origin if that port is occupied: that would make
// retained browser data appear lost. This is local runtime state, not a recipe.
func listenBrowserOrigin(root string) (net.Listener, error) {
	path := filepath.Join(root, ".archetipo/environment/browser-origin.json")
	var origin struct {
		Version int    `json:"version"`
		Root    string `json:"root"`
		Port    int    `json:"port"`
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	retained := err == nil
	address := "127.0.0.1:0"
	if retained {
		if err = json.Unmarshal(raw, &origin); err != nil || origin.Version != 1 || origin.Root != root || origin.Port < 1 || origin.Port > 65535 {
			return nil, iox.NewPrecondition("indirizzo browser della preview non valido", "conserva i dati locali e verifica i metadati browser-origin.json del worktree", err)
		}
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(origin.Port))
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, iox.NewConflict("indirizzo browser della preview non disponibile", fmt.Sprintf("libera %s e riprova; l’indirizzo viene conservato per mantenere i dati del browser", address), err)
	}
	if !retained {
		origin.Version, origin.Root, origin.Port = 1, root, listener.Addr().(*net.TCPAddr).Port
		if err = recordfile.WriteAtomic(filepath.Dir(path), path, ".browser-origin-", origin); err != nil {
			_ = listener.Close()
			return nil, err
		}
	}
	return listener, nil
}
