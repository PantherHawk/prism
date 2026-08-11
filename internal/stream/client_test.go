package stream

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// recorder is a sink that remembers every append.
type recorder struct {
	mu      sync.Mutex
	appends []series.Stats
	samples int
}

func (r *recorder) Append(_ time.Time, samples []series.Sample, stats series.Stats) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.appends = append(r.appends, stats)
	r.samples += len(samples)
}

func (r *recorder) last() (series.Stats, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.appends) == 0 {
		return series.Stats{}, false
	}

	return r.appends[len(r.appends)-1], true
}

func (r *recorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.samples
}

func silent() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestNewClientRejectsAMissingUpstream(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(Config{}, &recorder{}, silent()); err == nil {
		t.Error("NewClient accepted an empty upstream")
	}
}

// Backoff has to grow, or a target that is down gets hammered by every
// follower watching it - which is the load prism exists to avoid adding.
func TestBackoffGrowsAndIsCapped(t *testing.T) {
	t.Parallel()

	previous := time.Duration(0)

	for attempt := range 20 {
		delay := backoff(attempt)

		if delay > maxBackoff {
			t.Fatalf("attempt %d waited %v, past the %v cap", attempt, delay, maxBackoff)
		}

		if delay <= 0 {
			t.Fatalf("attempt %d waited %v, which is not a delay", attempt, delay)
		}

		// Jitter makes this a trend rather than a strict ordering, so compare
		// against the floor of what the previous attempt could have been.
		if attempt > 0 && attempt < 6 && delay < previous/2 {
			t.Errorf("attempt %d waited %v, less than half of the previous %v",
				attempt, delay, previous)
		}

		previous = delay
	}
}

// Jitter is what keeps ten followers of a restarted leader from reconnecting
// in lockstep and knocking it over again.
func TestBackoffIsJittered(t *testing.T) {
	t.Parallel()

	const attempt = 4

	seen := make(map[time.Duration]struct{})
	for range 50 {
		seen[backoff(attempt)] = struct{}{}
	}

	if len(seen) < 2 {
		t.Error("backoff produced the same delay every time; it is not jittered")
	}
}

// sseServer serves one canned event stream and then holds the connection open
// until the client goes away.
func sseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		if _, err := io.WriteString(w, body); err != nil {
			return
		}

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		<-r.Context().Done()
	}))
}

func TestFramesReachTheSink(t *testing.T) {
	t.Parallel()

	payload, err := encode(Frame{
		At: time.Unix(1_700_000_000, 0),
		Samples: []series.Sample{{
			Family: "rq",
			Labels: series.Labels{{Name: "id", Value: "1"}},
			Kind:   series.KindGauge,
			Value:  7,
		}},
		Stats: series.Stats{Scrapes: 3},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	server := sseServer(t, "data: "+string(payload)+"\n\n")
	defer server.Close()

	sink := &recorder{}

	client, err := NewClient(Config{Upstream: server.URL}, sink, silent())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	go client.Run(ctx, make(chan error, 1))

	waitFor(t, "the frame to arrive", func() bool { return sink.total() == 1 })

	stats, _ := sink.last()
	if stats.Scrapes != 3 {
		t.Errorf("stats.Scrapes = %d, want the leader's 3", stats.Scrapes)
	}

	// The follower has to say where it is watching from, or two prisms side by
	// side are indistinguishable.
	if stats.Source == "" {
		t.Error("stats.Source is empty; the follower does not name its upstream")
	}
}

// An outage must be announced with a retry time. A chart that silently stops
// updating is the worst possible failure mode for a monitoring tool.
func TestAnOutageIsReportedWithARetryTime(t *testing.T) {
	t.Parallel()

	sink := &recorder{}

	// Nothing is listening on this port, so every attempt fails immediately.
	client, err := NewClient(Config{Upstream: "http://127.0.0.1:1"}, sink, silent())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	go client.Run(ctx, make(chan error, 1))

	waitFor(t, "the outage to be reported", func() bool {
		stats, ok := sink.last()

		return ok && stats.Reconnecting && stats.RetryIn > 0
	})
}

// The broker prefixes a coalesced frame with an `id:` line carrying how many
// frames the subscriber missed. A client that treated that as part of the
// payload would drop exactly the frames that matter most - the ones sent while
// it was struggling to keep up.
func TestAnIDLineDoesNotSwallowTheFrame(t *testing.T) {
	t.Parallel()

	payload, err := encode(Frame{
		At:      time.Unix(1_700_000_000, 0),
		Samples: []series.Sample{{Family: "rq", Kind: series.KindGauge, Value: 1}},
		Stats:   series.Stats{Scrapes: 1},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	server := sseServer(t, "id: 4\ndata: "+string(payload)+"\n\n")
	defer server.Close()

	sink := &recorder{}

	client, err := NewClient(Config{Upstream: server.URL}, sink, silent())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	go client.Run(ctx, make(chan error, 1))

	waitFor(t, "the frame behind the id line to arrive", func() bool { return sink.total() == 1 })
}

// Run must return when its context is cancelled, and report no error for it:
// an operator pressing ctrl-c is a successful exit.
func TestRunStopsOnCancellation(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{Upstream: "http://127.0.0.1:1"}, &recorder{}, silent())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	errs := make(chan error, 1)

	go client.Run(ctx, errs)

	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Errorf("Run reported %v on cancellation, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}
