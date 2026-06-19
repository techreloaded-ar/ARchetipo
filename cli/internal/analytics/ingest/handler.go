package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// IngestHandler implements http.Handler for POST /v1/events.
// It accepts single events or batches (JSON array), validates each event
// against the strict schema, applies rate limiting, and stores valid events
// (without any origin-identifying data).
type IngestHandler struct {
	limiter RateLimiter
	store   EventStore
}

// NewIngestHandler creates an IngestHandler backed by the given rate limiter
// and event store.
func NewIngestHandler(limiter RateLimiter, store EventStore) *IngestHandler {
	return &IngestHandler{limiter: limiter, store: store}
}

// ServeHTTP implements http.Handler.
func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Derive origin from client address.
	origin := hashOrigin(r.RemoteAddr)

	// Only POST.
	if r.Method != http.MethodPost {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusMethodNotAllowed,
			"method not allowed", "only POST is accepted")
		return
	}

	// Content-Type must be application/json.
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"invalid_content_type", "Content-Type must be application/json")
		return
	}

	// Rate limit check.
	if !h.limiter.Allow(origin) {
		st := h.limiter.Status(origin)
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", st.Limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", st.Remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", st.Reset.Unix()))
		w.Header().Set("Retry-After", "60")
		http.Error(w, "", http.StatusTooManyRequests)
		return
	}

	// Read body.
	if r.Body == nil {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"empty_body", "request body is required")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"read_error", "unable to read request body")
		return
	}
	if len(body) == 0 {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"empty_body", "request body is required")
		return
	}

	trimmed := strings.TrimSpace(string(body))

	// Detect batch (JSON array) vs single event (JSON object).
	if strings.HasPrefix(trimmed, "[") {
		h.handleBatch(w, trimmed, origin)
		return
	}
	h.handleSingle(w, trimmed, origin)
}

// handleSingle processes a single JSON event object.
func (h *IngestHandler) handleSingle(w http.ResponseWriter, raw string, origin string) {
	evt, err := h.decodeAndValidate([]byte(raw))
	if err != nil {
		writeValidationErrorRL(w, h.limiter, origin, err)
		return
	}
	h.store.Store(evt)
	writeIngestOKRL(w, h.limiter, origin)
}

// handleBatch processes a JSON array of events. Fail-atomic: if any event
// fails validation, the entire batch is rejected and no events are stored.
func (h *IngestHandler) handleBatch(w http.ResponseWriter, raw string, origin string) {
	var rawEvents []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawEvents); err != nil {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"invalid_json", "batch must be a valid JSON array")
		return
	}
	if len(rawEvents) == 0 {
		writeIngestErrorRL(w, h.limiter, origin, http.StatusBadRequest,
			"empty_batch", "batch must contain at least one event")
		return
	}

	// Validate every event before storing any (fail-atomic).
	events := make([]AnalyticsEvent, 0, len(rawEvents))
	for i, rawEvt := range rawEvents {
		evt, err := h.decodeAndValidate(rawEvt)
		if err != nil {
			writeBatchValidationErrorRL(w, h.limiter, origin, err, i)
			return
		}
		events = append(events, evt)
	}

	// All valid: store them.
	for _, evt := range events {
		h.store.Store(evt)
	}
	writeIngestOKRL(w, h.limiter, origin)
}

// decodeAndValidate decodes the raw JSON bytes into an AnalyticsEvent,
// runs denylist + allowlist validation, and returns the event or an error.
func (h *IngestHandler) decodeAndValidate(raw []byte) (AnalyticsEvent, error) {
	var evt AnalyticsEvent
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&evt); err != nil {
		return AnalyticsEvent{}, fmt.Errorf("json decode: %w", err)
	}

	if err := ValidateEvent(raw, evt); err != nil {
		return AnalyticsEvent{}, err
	}

	return evt, nil
}

// readBody reads the full request body (up to 1MB).
func readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return buf, nil
}

// hashOrigin derives a stable, non-reversible key from the client address.
func hashOrigin(addr string) string {
	h := sha256.Sum256([]byte(addr))
	return fmt.Sprintf("%x", h[:16])
}

// ─── Response helpers with rate limit headers ────────────────────────────

func writeIngestOKRL(w http.ResponseWriter, limiter RateLimiter, origin string) {
	st := limiter.Status(origin)
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", st.Limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", st.Remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", st.Reset.Unix()))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func writeIngestErrorRL(w http.ResponseWriter, limiter RateLimiter, origin string,
	status int, errCode, detail string) {
	st := limiter.Status(origin)
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", st.Limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", st.Remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", st.Reset.Unix()))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	resp := map[string]string{"error": errCode}
	if detail != "" {
		resp["detail"] = detail
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}

func writeValidationErrorRL(w http.ResponseWriter, limiter RateLimiter, origin string, err error) {
	var ve *ValidationError
	if e, ok := err.(*ValidationError); ok {
		ve = e
		writeIngestErrorRL(w, limiter, origin, http.StatusBadRequest, "validation_error", ve.Detail)
		return
	}
	writeIngestErrorRL(w, limiter, origin, http.StatusBadRequest, "validation_error", err.Error())
}

func writeBatchValidationErrorRL(w http.ResponseWriter, limiter RateLimiter, origin string, err error, index int) {
	detail := err.Error()
	if e, ok := err.(*ValidationError); ok {
		detail = e.Detail
	}
	writeIngestErrorRL(w, limiter, origin, http.StatusBadRequest, "validation_error",
		fmt.Sprintf("event[%d]: %s", index, detail))
}
