package sse

import (
	"sync"
	"testing"
	"time"
)

func TestHub_Subscribe(t *testing.T) {
	hub := NewHub()

	ch, cancel := hub.Subscribe()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if cancel == nil {
		t.Fatal("expected non-nil cancel function")
	}

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	cancel()

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after cancel, got %d", hub.ClientCount())
	}
}

func TestHub_MultipleSubscribers(t *testing.T) {
	hub := NewHub()

	ch1, cancel1 := hub.Subscribe()
	ch2, cancel2 := hub.Subscribe()
	ch3, cancel3 := hub.Subscribe()

	if hub.ClientCount() != 3 {
		t.Errorf("expected 3 clients, got %d", hub.ClientCount())
	}

	// Cancel in non-sequential order
	cancel2()
	if hub.ClientCount() != 2 {
		t.Errorf("expected 2 clients, got %d", hub.ClientCount())
	}

	cancel1()
	cancel3()
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}

	// Silence unused variable warnings
	_ = ch1
	_ = ch2
	_ = ch3
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe()
	defer cancel()

	testPayload := []byte(`{"test": "data"}`)
	hub.Broadcast(testPayload)

	select {
	case evt := <-ch:
		if evt.Type != "" {
			t.Errorf("expected empty event type, got %q", evt.Type)
		}
		if string(evt.Payload) != string(testPayload) {
			t.Errorf("expected payload %q, got %q", testPayload, evt.Payload)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast event")
	}
}

func TestHub_BroadcastEvent(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe()
	defer cancel()

	testPayload := []byte(`{"request": "data"}`)
	hub.BroadcastEvent("request", testPayload)

	select {
	case evt := <-ch:
		if evt.Type != "request" {
			t.Errorf("expected event type 'request', got %q", evt.Type)
		}
		if string(evt.Payload) != string(testPayload) {
			t.Errorf("expected payload %q, got %q", testPayload, evt.Payload)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast event")
	}
}

func TestHub_BroadcastToMultiple(t *testing.T) {
	hub := NewHub()

	ch1, cancel1 := hub.Subscribe()
	defer cancel1()
	ch2, cancel2 := hub.Subscribe()
	defer cancel2()

	testPayload := []byte(`{"multi": true}`)
	hub.Broadcast(testPayload)

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if string(evt.Payload) != string(testPayload) {
				t.Errorf("subscriber %d: expected payload %q, got %q", i, testPayload, evt.Payload)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timeout waiting for broadcast", i)
		}
	}
}

func TestHub_Close(t *testing.T) {
	hub := NewHub()

	ch1, _ := hub.Subscribe()
	ch2, _ := hub.Subscribe()

	count := hub.Close()
	if count != 2 {
		t.Errorf("expected 2 closed clients, got %d", count)
	}

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after close, got %d", hub.ClientCount())
	}

	// Verify channels are closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("expected ch1 to be closed")
		}
	default:
		t.Error("expected ch1 to be readable (closed)")
	}

	select {
	case _, ok := <-ch2:
		if ok {
			t.Error("expected ch2 to be closed")
		}
	default:
		t.Error("expected ch2 to be readable (closed)")
	}
}

func TestHub_CloseIdempotent(t *testing.T) {
	hub := NewHub()
	hub.Subscribe()

	count1 := hub.Close()
	if count1 != 1 {
		t.Errorf("first close: expected 1 closed client, got %d", count1)
	}

	count2 := hub.Close()
	if count2 != 0 {
		t.Errorf("second close: expected 0 closed clients, got %d", count2)
	}
}

func TestHub_SubscribeAfterClose(t *testing.T) {
	hub := NewHub()
	hub.Close()

	ch, cancel := hub.Subscribe()
	if ch != nil {
		t.Error("expected nil channel after close")
	}
	if cancel != nil {
		t.Error("expected nil cancel function after close")
	}
}

func TestHub_DoubleCancel(t *testing.T) {
	hub := NewHub()
	_, cancel := hub.Subscribe()

	// Double cancel should not panic
	cancel()
	cancel()

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHub_ConcurrentOperations(t *testing.T) {
	hub := NewHub()
	var wg sync.WaitGroup

	// Spawn multiple goroutines that subscribe, broadcast, and cancel
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, cancel := hub.Subscribe()
			if ch == nil {
				return // Hub was closed
			}
			hub.Broadcast([]byte("test"))
			cancel()
		}()
	}

	wg.Wait()

	// All clients should be cleaned up
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHub_BroadcastDropsWhenBufferFull(t *testing.T) {
	// Use a small buffer size for testing
	hub := NewHub(WithBufferSize(10))
	ch, cancel := hub.Subscribe()
	defer cancel()

	// Fill the buffer (buffer size is 10)
	for i := 0; i < 15; i++ {
		hub.Broadcast([]byte("message"))
	}

	// Should have exactly 10 messages (buffer size)
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 10 {
		t.Errorf("expected 10 buffered messages, got %d", count)
	}

	// Verify dropped count
	if hub.DroppedTotal() != 5 {
		t.Errorf("expected 5 dropped messages, got %d", hub.DroppedTotal())
	}
}

func TestHub_WithBufferSize(t *testing.T) {
	hub := NewHub(WithBufferSize(5))
	ch, cancel := hub.Subscribe()
	defer cancel()

	// Fill the buffer
	for i := 0; i < 8; i++ {
		hub.Broadcast([]byte("message"))
	}

	// Should have exactly 5 messages (custom buffer size)
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 5 {
		t.Errorf("expected 5 buffered messages, got %d", count)
	}

	if hub.DroppedTotal() != 3 {
		t.Errorf("expected 3 dropped messages, got %d", hub.DroppedTotal())
	}
}

type mockDroppedCounter struct {
	count int
	mu    sync.Mutex
}

func (m *mockDroppedCounter) RecordSSEDropped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
}

func (m *mockDroppedCounter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func TestHub_WithDroppedCounter(t *testing.T) {
	counter := &mockDroppedCounter{}
	hub := NewHub(WithBufferSize(5), WithDroppedCounter(counter))
	ch, cancel := hub.Subscribe()
	defer cancel()

	// Fill buffer and cause drops
	for i := 0; i < 10; i++ {
		hub.Broadcast([]byte("message"))
	}

	// Drain channel
	for {
		select {
		case <-ch:
		default:
			goto done
		}
	}
done:

	// Verify counter was called
	if counter.Count() != 5 {
		t.Errorf("expected counter to record 5 drops, got %d", counter.Count())
	}
}

func TestHub_SetDroppedCounter(t *testing.T) {
	hub := NewHub(WithBufferSize(5))
	ch, cancel := hub.Subscribe()
	defer cancel()

	// Cause some drops without counter
	for i := 0; i < 7; i++ {
		hub.Broadcast([]byte("message"))
	}

	// Drain
	for {
		select {
		case <-ch:
		default:
			goto done
		}
	}
done:

	// Now set counter and cause more drops
	counter := &mockDroppedCounter{}
	hub.SetDroppedCounter(counter)

	for i := 0; i < 7; i++ {
		hub.Broadcast([]byte("message"))
	}

	// Drain again
	for {
		select {
		case <-ch:
		default:
			goto done2
		}
	}
done2:

	// Counter should only have drops after it was set
	if counter.Count() != 2 {
		t.Errorf("expected counter to record 2 drops, got %d", counter.Count())
	}

	// But total should include all drops
	if hub.DroppedTotal() != 4 {
		t.Errorf("expected 4 total dropped, got %d", hub.DroppedTotal())
	}
}
