package sse

import "sync"

// Event represents an SSE event with a type and payload
type Event struct {
	Type    string
	Payload []byte
}

// Hub is a minimal SSE broadcaster.
type Hub struct {
	mu      sync.Mutex
	clients map[chan Event]struct{}
	closed  bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan Event]struct{})}
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
	ch := make(chan Event, 10)
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

// BroadcastEvent sends a named event to all subscribers
func (h *Hub) BroadcastEvent(eventType string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- Event{Type: eventType, Payload: payload}:
		default:
		}
	}
}
