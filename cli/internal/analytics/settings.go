package analytics

import (
	"net/http"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

const (
	// DefaultEnabled is false — analytics is opt-in.
	DefaultEnabled = false
	// DefaultTimeout is the HTTP client timeout used when Settings.Timeout is zero.
	DefaultTimeout = 2 * time.Second
	// DefaultEndpoint is used when Settings.Endpoint is empty; it points to
	// localhost port 0 so that a zero-config client never sends data to a
	// remote host.
	DefaultEndpoint = "http://localhost:0"
)

// DefaultUserAgent returns the standard User-Agent header value for
// archetipo analytics requests.
func DefaultUserAgent() string {
	return "archetipo-analytics/" + version.Version
}

// Settings configures the analytics client. All fields are set by the
// caller — this package never reads environment variables or config files.
type Settings struct {
	Enabled   bool
	Endpoint  string
	Timeout   time.Duration
	UserAgent string
}

// ApplyDefaults fills zero-value fields with their documented defaults.
func (s *Settings) ApplyDefaults() {
	if s.Endpoint == "" {
		s.Endpoint = DefaultEndpoint
	}
	if s.Timeout == 0 {
		s.Timeout = DefaultTimeout
	}
	if s.UserAgent == "" {
		s.UserAgent = DefaultUserAgent()
	}
}

// NewClient creates a *Client from the given settings, applying defaults
// for any zero-value fields. If h is non-nil it is used as the HTTP client;
// otherwise a default client is created per-request.
func NewClient(s Settings, h *http.Client) *Client {
	s.ApplyDefaults()
	return &Client{settings: s, http: h}
}
