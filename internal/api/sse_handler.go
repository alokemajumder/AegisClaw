package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

// SSEBroker manages SSE connections and bridges NATS events to browser clients.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string]map[chan SSEEvent]struct{} // subject -> set of client channels
	logger  *slog.Logger
	nc      *nats.Conn
	subs    []*nats.Subscription
}

// SSEEvent is a server-sent event.
type SSEEvent struct {
	Event string `json:"event"`
	Data  string `json:"data"`
	ID    string `json:"id,omitempty"`
}

// NewSSEBroker creates a new SSE broker that bridges NATS subjects to SSE streams.
func NewSSEBroker(nc *nats.Conn, logger *slog.Logger) *SSEBroker {
	return &SSEBroker{
		clients: make(map[string]map[chan SSEEvent]struct{}),
		logger:  logger,
		nc:      nc,
	}
}

// Start subscribes to NATS subjects and broadcasts messages to SSE clients.
func (b *SSEBroker) Start() error {
	subjects := []string{
		natspkg.SubjectRunStatus,
		natspkg.SubjectAgentResult,
		natspkg.SubjectApprovalRequest,
		natspkg.SubjectKillSwitch,
	}

	for _, subject := range subjects {
		sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
			b.broadcast(msg.Subject, msg.Data)
		})
		if err != nil {
			return fmt.Errorf("subscribing to %s for SSE: %w", subject, err)
		}
		b.subs = append(b.subs, sub)
		b.logger.Info("SSE broker subscribed", "subject", subject)
	}

	return nil
}

// Stop unsubscribes from all NATS subjects.
func (b *SSEBroker) Stop() {
	for _, sub := range b.subs {
		sub.Unsubscribe()
	}
}

// broadcast sends a NATS message to all SSE clients subscribed to the given subject.
func (b *SSEBroker) broadcast(subject string, data []byte) {
	b.mu.RLock()
	clients := b.clients[subject]
	b.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	// Map NATS subject to SSE event name
	eventName := subjectToEventName(subject)

	event := SSEEvent{
		Event: eventName,
		Data:  string(data),
		ID:    fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	b.mu.RLock()
	for ch := range clients {
		select {
		case ch <- event:
		default:
			// Client channel full — skip to avoid blocking
			b.logger.Debug("SSE client channel full, dropping event", "subject", subject)
		}
	}
	b.mu.RUnlock()
}

// addClient registers a client channel for the given subjects.
func (b *SSEBroker) addClient(ch chan SSEEvent, subjects []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subject := range subjects {
		if b.clients[subject] == nil {
			b.clients[subject] = make(map[chan SSEEvent]struct{})
		}
		b.clients[subject][ch] = struct{}{}
	}
}

// removeClient unregisters a client channel from all subjects.
func (b *SSEBroker) removeClient(ch chan SSEEvent, subjects []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subject := range subjects {
		if clients, ok := b.clients[subject]; ok {
			delete(clients, ch)
			if len(clients) == 0 {
				delete(b.clients, subject)
			}
		}
	}
}

// subjectToEventName maps NATS subjects to SSE event names.
func subjectToEventName(subject string) string {
	switch subject {
	case natspkg.SubjectRunStatus:
		return "run_status"
	case natspkg.SubjectAgentResult:
		return "agent_result"
	case natspkg.SubjectApprovalRequest:
		return "approval_request"
	case natspkg.SubjectKillSwitch:
		return "kill_switch"
	default:
		return subject
	}
}

// HandleSSE is the HTTP handler for the SSE endpoint.
// Clients connect to /api/v1/events/stream to receive real-time updates.
// The handler authenticates via JWT (same as all other API endpoints) and
// streams NATS events as SSE.
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if h.SSEBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "sse_unavailable", "SSE streaming not available (NATS not connected)")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Determine which subjects to subscribe to
	subjects := []string{
		natspkg.SubjectRunStatus,
		natspkg.SubjectAgentResult,
		natspkg.SubjectApprovalRequest,
		natspkg.SubjectKillSwitch,
	}

	// Create client channel
	ch := make(chan SSEEvent, 64)
	h.SSEBroker.addClient(ch, subjects)
	defer func() {
		h.SSEBroker.removeClient(ch, subjects)
		close(ch)
	}()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Keepalive ticker (every 30s)
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			fmt.Fprintf(w, "event: %s\n", event.Event)
			fmt.Fprintf(w, "data: %s\n", event.Data)
			if event.ID != "" {
				fmt.Fprintf(w, "id: %s\n", event.ID)
			}
			fmt.Fprintf(w, "\n")
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// HandleRunSSE streams events for a specific run.
// Clients connect to /api/v1/runs/{runID}/events to receive run-specific updates.
func (h *Handler) HandleRunSSE(w http.ResponseWriter, r *http.Request) {
	if h.SSEBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "sse_unavailable", "SSE streaming not available (NATS not connected)")
		return
	}

	runID, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe to run status and agent results
	subjects := []string{
		natspkg.SubjectRunStatus,
		natspkg.SubjectAgentResult,
	}

	ch := make(chan SSEEvent, 64)
	h.SSEBroker.addClient(ch, subjects)
	defer func() {
		h.SSEBroker.removeClient(ch, subjects)
		close(ch)
	}()

	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"run_id\":\"%s\"}\n\n", runID)
	flusher.Flush()

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			// Filter: only forward events for this run
			if isRunEvent(event, runID.String()) {
				fmt.Fprintf(w, "event: %s\n", event.Event)
				fmt.Fprintf(w, "data: %s\n", event.Data)
				if event.ID != "" {
					fmt.Fprintf(w, "id: %s\n", event.ID)
				}
				fmt.Fprintf(w, "\n")
				flusher.Flush()
			}
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// isRunEvent checks if an SSE event belongs to a specific run by inspecting the JSON data.
func isRunEvent(event SSEEvent, runID string) bool {
	var payload struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return false
	}
	return payload.Payload.RunID == runID
}

// SetupSSEBroker initializes the SSE broker if NATS is available.
// Call this from the API gateway main after NATS is connected.
func SetupSSEBroker(h *Handler, nc *nats.Conn, logger *slog.Logger) error {
	broker := NewSSEBroker(nc, logger)
	if err := broker.Start(); err != nil {
		return fmt.Errorf("starting SSE broker: %w", err)
	}
	h.SSEBroker = broker
	logger.Info("SSE broker started")
	return nil
}

