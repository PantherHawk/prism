package stream

import "sync"

// subscriber holds one connected client's pending frame.
//
// There is no queue, and that is the whole design. A slow consumer on an
// unbounded queue turns into unbounded memory on the producer, and prism is a
// tool for diagnosing exactly that failure - it must not be a way to cause one.
// Instead each subscriber has a single slot holding the newest frame, and a
// frame that arrives before the last one was sent replaces it.
//
// Coalescing is safe here because the store derives rates from the timestamps
// carried in each frame rather than from adjacent samples. Dropping an
// intermediate frame costs resolution, not correctness: the next frame still
// spans the full interval since the last one that landed.
type subscriber struct {
	notify chan struct{}

	mu      sync.Mutex
	latest  []byte
	dropped int
	closed  bool
}

// newSubscriber returns a subscriber with an empty slot.
func newSubscriber() *subscriber {
	return &subscriber{notify: make(chan struct{}, 1)}
}

// offer replaces the pending frame and wakes the writer.
func (s *subscriber) offer(payload []byte) {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()

		return
	}

	if s.latest != nil {
		s.dropped++
	}

	s.latest = payload
	s.mu.Unlock()

	// A full notify channel already means "there is something to send", so a
	// missed send here loses nothing.
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// take removes the pending frame and the count of frames skipped before it.
func (s *subscriber) take() ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, dropped := s.latest, s.dropped
	s.latest, s.dropped = nil, 0

	return payload, dropped
}

// close releases the writer.
func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	close(s.notify)
}
