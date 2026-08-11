package stream

import "testing"

// TestSlotHoldsOnlyTheNewest is the backpressure contract. A queue here would
// turn a slow consumer into unbounded memory on the producer, which is the
// exact failure prism exists to help diagnose.
func TestSlotHoldsOnlyTheNewest(t *testing.T) {
	t.Parallel()

	sub := newSubscriber()

	for i := range 100 {
		sub.offer([]byte{byte(i)})
	}

	payload, dropped := sub.take()

	if len(payload) != 1 || payload[0] != 99 {
		t.Errorf("slot held %v, want the newest frame", payload)
	}

	if dropped != 99 {
		t.Errorf("dropped = %d, want 99", dropped)
	}
}

// TestNotifyDoesNotBlock pins that offering never waits on a reader. The
// producer is the collector's goroutine; blocking it would stall the scrape.
func TestNotifyDoesNotBlock(t *testing.T) {
	t.Parallel()

	sub := newSubscriber()

	done := make(chan struct{})

	go func() {
		defer close(done)

		for i := range 10_000 {
			sub.offer([]byte{byte(i)})
		}
	}()

	<-done

	if got := len(sub.notify); got != 1 {
		t.Errorf("notify held %d wakeups, want exactly 1 pending", got)
	}
}

func TestTakeClearsTheSlot(t *testing.T) {
	t.Parallel()

	sub := newSubscriber()
	sub.offer([]byte("first"))

	if _, dropped := sub.take(); dropped != 0 {
		t.Errorf("dropped = %d on the first take, want 0", dropped)
	}

	payload, dropped := sub.take()
	if payload != nil || dropped != 0 {
		t.Errorf("second take returned %v/%d, want nil/0", payload, dropped)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	sub := newSubscriber()
	sub.close()
	sub.close()

	// Offering after close must not panic on a closed channel.
	sub.offer([]byte("late"))
}
