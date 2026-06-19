package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// Log is the package-level logger. It writes to io.Discard by default
// so that analytics failures are silent unless the caller opts in.
var Log = log.New(io.Discard, "[analytics] ", 0)

// Client sends analytics events via HTTP POST.
type Client struct {
	settings Settings
	http     *http.Client
}

// Send serialises e as JSON and POSTs it to Settings.Endpoint.
// It returns immediately with nil when Settings.Enabled is false.
// Every network error, non-2xx response, and timeout is logged
// internally and never propagated to the caller.
func (c *Client) Send(ctx context.Context, e Event) error {
	if !c.settings.Enabled {
		return nil
	}

	body, err := json.Marshal(e)
	if err != nil {
		Log.Printf("marshal event: %v", err)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.settings.Endpoint, bytes.NewReader(body))
	if err != nil {
		Log.Printf("create request: %v", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.settings.UserAgent)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		Log.Printf("http post: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		Log.Printf("unexpected status %d", resp.StatusCode)
	}

	// Drain body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.http != nil {
		return c.http
	}
	return &http.Client{Timeout: c.settings.Timeout}
}
