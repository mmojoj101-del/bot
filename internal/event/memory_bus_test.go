package event

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPublishSubscribe(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var received int32
	handler := func(e Event) {
		atomic.AddInt32(&received, 1)
	}

	unsub := bus.Subscribe("test.event", handler)
	defer unsub()

	bus.Publish(Event{
		ID:    uuid.New().String(),
		Type:  "test.event",
		Payload: "data",
		Timestamp: time.Now(),
	})

	if atomic.LoadInt32(&received) != 1 {
		t.Fatalf("expected 1 event, got %d", received)
	}
}

func TestPublishSubscribe_MultipleHandlers(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var received1, received2 int32

	bus.Subscribe("test.event", func(e Event) {
		atomic.AddInt32(&received1, 1)
	})
	bus.Subscribe("test.event", func(e Event) {
		atomic.AddInt32(&received2, 1)
	})

	bus.Publish(Event{
		ID:    uuid.New().String(),
		Type:  "test.event",
		Timestamp: time.Now(),
	})

	if atomic.LoadInt32(&received1) != 1 {
		t.Fatalf("handler1: expected 1, got %d", received1)
	}
	if atomic.LoadInt32(&received2) != 1 {
		t.Fatalf("handler2: expected 1, got %d", received2)
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var count int32
	handler := func(e Event) {
		atomic.AddInt32(&count, 1)
	}

	unsub := bus.Subscribe("test.event", handler)

	bus.Publish(Event{ID: "1", Type: "test.event", Timestamp: time.Now()})
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected 1 after first publish, got %d", count)
	}

	unsub()

	bus.Publish(Event{ID: "2", Type: "test.event", Timestamp: time.Now()})
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected 1 after unsubscribe, got %d", count)
	}
}

func TestUnsubscribe_OnlyRemovesSpecific(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var count1, count2 int32

	h1 := func(e Event) { atomic.AddInt32(&count1, 1) }
	h2 := func(e Event) { atomic.AddInt32(&count2, 1) }

	bus.Subscribe("test.event", h1)
	unsub2 := bus.Subscribe("test.event", h2)

	bus.Publish(Event{ID: "1", Type: "test.event", Timestamp: time.Now()})
	if atomic.LoadInt32(&count1) != 1 {
		t.Fatalf("h1: expected 1, got %d", count1)
	}
	if atomic.LoadInt32(&count2) != 1 {
		t.Fatalf("h2: expected 1, got %d", count2)
	}

	unsub2()

	bus.Publish(Event{ID: "2", Type: "test.event", Timestamp: time.Now()})
	if atomic.LoadInt32(&count1) != 2 {
		t.Fatalf("h1 after unsub: expected 2, got %d", count1)
	}
	if atomic.LoadInt32(&count2) != 1 {
		t.Fatalf("h2 after unsub: expected 1, got %d", count2)
	}
}

func TestDifferentEventTypes(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var typeACount, typeBCount int32

	bus.Subscribe("type.a", func(e Event) { atomic.AddInt32(&typeACount, 1) })
	bus.Subscribe("type.b", func(e Event) { atomic.AddInt32(&typeBCount, 1) })

	bus.Publish(Event{ID: "1", Type: "type.a", Timestamp: time.Now()})
	bus.Publish(Event{ID: "2", Type: "type.a", Timestamp: time.Now()})
	bus.Publish(Event{ID: "3", Type: "type.b", Timestamp: time.Now()})

	if atomic.LoadInt32(&typeACount) != 2 {
		t.Fatalf("type.a: expected 2, got %d", typeACount)
	}
	if atomic.LoadInt32(&typeBCount) != 1 {
		t.Fatalf("type.b: expected 1, got %d", typeBCount)
	}
}

func TestNoHandlerForEventType(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	// Should not panic
	bus.Publish(Event{ID: "1", Type: "unknown", Timestamp: time.Now()})
}

func TestClose(t *testing.T) {
	bus := NewMemoryBus()

	var count int32
	bus.Subscribe("test", func(e Event) { atomic.AddInt32(&count, 1) })

	bus.Close()

	// Should not panic after close
	bus.Publish(Event{ID: "1", Type: "test", Timestamp: time.Now()})
	if atomic.LoadInt32(&count) != 0 {
		t.Fatal("should not receive events after close")
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var total int32
	var wg sync.WaitGroup

	// Subscribe
	bus.Subscribe("concurrent", func(e Event) {
		atomic.AddInt32(&total, 1)
	})

	// Publish concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{
				ID:    uuid.New().String(),
				Type:  "concurrent",
				Timestamp: time.Now(),
			})
		}()
	}

	wg.Wait()

	if atomic.LoadInt32(&total) != 100 {
		t.Fatalf("expected 100 events, got %d", total)
	}
}

func TestConcurrentSubscribeUnsubscribe(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := bus.Subscribe("dynamic", func(e Event) {})
			unsub()
		}()
	}

	wg.Wait()

	// Should not deadlock or panic
	bus.Publish(Event{ID: "1", Type: "dynamic", Timestamp: time.Now()})
}

func TestConcurrentMixed(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	var wg sync.WaitGroup

	// Spam subscribe/unsubscribe/publish concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				unsub := bus.Subscribe("mixed", func(e Event) {})
				bus.Publish(Event{
					ID:    uuid.New().String(),
					Type:  "mixed",
					Timestamp: time.Now(),
				})
				bus.Publish(Event{
					ID:    uuid.New().String(),
					Type:  "other",
					Timestamp: time.Now(),
				})
				unsub()
			}
		}(i)
	}

	wg.Wait()
}

func TestUnsubscribe_NonExistent(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	// Should not panic
	bus.Unsubscribe("nonexistent", func(e Event) {})
}
