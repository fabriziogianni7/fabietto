package sessionqueue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"custom-agent/gateway"
)

// TestProcess_FIFOOrdering verifies that messages for the same session are
// processed in FIFO order: the first message completes before the second starts.
func TestProcess_FIFOOrdering(t *testing.T) {
	var order []string
	var mu sync.Mutex
	release := make(chan struct{}) // handler blocks until released

	handler := func(msg gateway.IncomingMessage) string {
		mu.Lock()
		order = append(order, msg.Text)
		mu.Unlock()
		<-release
		return "ok"
	}

	q := New(handler)
	platform, userID := "test", "fifo123"

	// Start first message (will block in handler)
	done1 := make(chan string, 1)
	go func() {
		done1 <- q.Process(gateway.IncomingMessage{Platform: platform, UserID: userID, Text: "first"})
	}()

	// Give handler time to run and block
	time.Sleep(50 * time.Millisecond)

	// Start second message (will queue behind first)
	done2 := make(chan string, 1)
	go func() {
		done2 <- q.Process(gateway.IncomingMessage{Platform: platform, UserID: userID, Text: "second"})
	}()

	// Second should not have started yet (order should only have "first")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := append([]string{}, order...)
	mu.Unlock()
	if len(got) != 1 || got[0] != "first" {
		t.Errorf("expected [first] before release, got %v", got)
	}

	// Release first
	close(release)

	// Wait for both
	r1 := <-done1
	r2 := <-done2
	if r1 != "ok" || r2 != "ok" {
		t.Errorf("expected ok, ok; got %q, %q", r1, r2)
	}

	mu.Lock()
	got = append([]string{}, order...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("expected [first second], got %v", got)
	}
}

// TestProcess_ConcurrentSessions verifies that different sessions process in parallel.
func TestProcess_ConcurrentSessions(t *testing.T) {
	var started, finished int32
	block := make(chan struct{})

	handler := func(msg gateway.IncomingMessage) string {
		atomic.AddInt32(&started, 1)
		<-block
		atomic.AddInt32(&finished, 1)
		return "ok"
	}

	q := New(handler)

	// Start two messages for different sessions
	done1 := make(chan string, 1)
	done2 := make(chan string, 1)
	go func() { done1 <- q.Process(gateway.IncomingMessage{Platform: "test", UserID: "a", Text: "a"}) }()
	go func() { done2 <- q.Process(gateway.IncomingMessage{Platform: "test", UserID: "b", Text: "b"}) }()

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&started) != 2 {
		t.Errorf("expected both sessions to have started, got started=%d", started)
	}

	close(block)
	<-done1
	<-done2
	if atomic.LoadInt32(&finished) != 2 {
		t.Errorf("expected both finished, got %d", finished)
	}
}
