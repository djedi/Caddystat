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
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan Event]struct{})}
}

// Subscribe returns a channel for events and a cleanup function.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 10)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		close(ch)
		h.mu.Unlock()
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
