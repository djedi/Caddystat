package sse

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// DefaultBufferSize is the default channel buffer size for SSE clients.
const DefaultBufferSize = 32

// DroppedCounter is an interface for recording dropped SSE messages.
type DroppedCounter interface {
	RecordSSEDropped()
}

// Event represents an SSE event with a type and payload
type Event struct {
	Type    string
	Payload []byte
}

// Hub is a minimal SSE broadcaster.
type Hub struct {
	mu             sync.Mutex
	clients        map[chan Event]struct{}
	closed         bool
	bufferSize     int
	droppedCounter DroppedCounter
	droppedTotal   atomic.Uint64
}

// HubOption configures Hub behavior.
type HubOption func(*Hub)

// WithBufferSize sets the channel buffer size for new subscribers.
func WithBufferSize(size int) HubOption {
	return func(h *Hub) {
		if size > 0 {
			h.bufferSize = size
		}
	}
}

// WithDroppedCounter sets the counter for tracking dropped messages.
func WithDroppedCounter(counter DroppedCounter) HubOption {
	return func(h *Hub) {
		h.droppedCounter = counter
	}
}

// NewHub creates a new SSE hub with the given options.
func NewHub(opts ...HubOption) *Hub {
	h := &Hub{
		clients:    make(map[chan Event]struct{}),
		bufferSize: DefaultBufferSize,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Close closes all client connections and prevents new subscriptions.
// It returns the number of clients that were disconnected.
func (h *Hub) Close() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return 0
	}
	h.closed = true

	count := len(h.clients)
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
	return count
}

// ClientCount returns the current number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Subscribe returns a channel for events and a cleanup function.
// Returns nil, nil if the hub has been closed.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil
	}
	ch := make(chan Event, h.bufferSize)
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
	}
}

// Broadcast sends data to all subscribers (default "message" event type)
func (h *Hub) Broadcast(payload []byte) {
	h.BroadcastEvent("", payload)
}

// BroadcastEvent sends a named event to all subscribers.
// If a client's buffer is full, the message is dropped for that client.
// Dropped messages are logged and counted for monitoring.
func (h *Hub) BroadcastEvent(eventType string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- Event{Type: eventType, Payload: payload}:
		default:
			// Client buffer full - message dropped
			dropped := h.droppedTotal.Add(1)
			if h.droppedCounter != nil {
				h.droppedCounter.RecordSSEDropped()
			}
			// Log at debug level to avoid spam, but include total count
			slog.Debug("SSE message dropped for slow client",
				"event_type", eventType,
				"total_dropped", dropped,
				"clients", len(h.clients))
		}
	}
}

// DroppedTotal returns the total number of messages dropped since startup.
func (h *Hub) DroppedTotal() uint64 {
	return h.droppedTotal.Load()
}

// SetDroppedCounter sets the counter for recording dropped messages.
// This allows setting the counter after hub creation to resolve circular dependencies.
func (h *Hub) SetDroppedCounter(counter DroppedCounter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.droppedCounter = counter
}
