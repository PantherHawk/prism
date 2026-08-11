// Package stream carries scrapes between prism instances over HTTP.
//
// The broker publishes what it is given as server-sent events; the client
// receives them and feeds a local store. Between them, one prism can scrape a
// target and any number of others can watch without adding load to it.
package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// StreamPath is the endpoint subscribers connect to.
const StreamPath = "/stream"

// shutdownGrace bounds how long Shutdown waits for subscribers to finish.
const shutdownGrace = 2 * time.Second

// Config describes the streaming server and, separately, the upstream this
// prism should follow instead of scraping.
type Config struct {
	Enabled bool          `yaml:"enabled"`
	Address string        `yaml:"address"`
	Timeout time.Duration `yaml:"timeout"`

	// Upstream is the address of another prism to follow. When set, this
	// instance does not scrape: it receives.
	Upstream string `yaml:"upstream"`
}

// Default returns the configuration used when none is supplied. Streaming is
// off by default: opening a port should always be a deliberate act.
func Default() Config {
	return Config{
		Enabled: false,
		Address: "127.0.0.1:9099",
		Timeout: 5 * time.Second,
	}
}

// Broker fans frames out to connected subscribers.
type Broker struct {
	cfg    Config
	log    *slog.Logger
	server *http.Server

	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
}

// New returns a broker for cfg.
func New(cfg Config, log *slog.Logger) (*Broker, error) {
	b := &Broker{
		cfg:         cfg,
		log:         log,
		subscribers: make(map[*subscriber]struct{}),
	}

	mux := http.NewServeMux()
	mux.Handle(StreamPath, b.Handler())

	b.server = &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: cfg.Timeout,
		// No WriteTimeout: a subscription is a long-lived response, and a
		// write deadline would sever every client on a fixed schedule.
	}

	return b, nil
}

// Append publishes a scrape to every subscriber.
//
// It is on the collector's goroutine, so it does as little as possible: with no
// subscribers it does not even encode, and with subscribers it encodes once and
// shares the bytes.
func (b *Broker) Append(at time.Time, samples []series.Sample, stats series.Stats) {
	b.mu.RLock()
	listening := len(b.subscribers) > 0
	b.mu.RUnlock()

	if !listening {
		return
	}

	payload, err := encode(Frame{At: at, Samples: samples, Stats: stats})
	if err != nil {
		b.log.WarnContext(context.Background(), "encode frame", slog.Any("error", err))

		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		sub.offer(payload)
	}
}

// Run serves until ctx is done.
func (b *Broker) Run(ctx context.Context, errs chan<- error) {
	if !b.cfg.Enabled {
		<-ctx.Done()
		errs <- nil

		return
	}

	listener, err := net.Listen("tcp", b.cfg.Address)
	if err != nil {
		errs <- fmt.Errorf("listen on %s: %w", b.cfg.Address, err)

		return
	}

	b.log.InfoContext(ctx, "stream broker listening",
		slog.String("address", listener.Addr().String()),
		slog.String("path", StreamPath),
	)

	if err := b.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- fmt.Errorf("serve stream: %w", err)

		return
	}

	errs <- nil
}

// Shutdown drains subscribers and closes the listener.
func (b *Broker) Shutdown(ctx context.Context) error {
	if !b.cfg.Enabled {
		return nil
	}

	b.mu.Lock()
	for sub := range b.subscribers {
		sub.close()
	}
	b.mu.Unlock()

	deadline, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()

	if err := b.server.Shutdown(deadline); err != nil {
		return fmt.Errorf("shutdown stream: %w", err)
	}

	return nil
}

// Subscribers reports how many clients are attached.
func (b *Broker) Subscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.subscribers)
}

// Handler returns the stream endpoint on its own, so that it can be mounted in
// an existing server rather than opening a second port.
func (b *Broker) Handler() http.Handler {
	return http.HandlerFunc(b.handle)
}

// handle serves one subscription for as long as the client stays connected.
func (b *Broker) handle(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)

		return
	}

	sub := newSubscriber()

	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return

		case _, open := <-sub.notify:
			if !open {
				return
			}

			payload, dropped := sub.take()
			if payload == nil {
				continue
			}

			if err := write(w, payload, dropped); err != nil {
				b.log.DebugContext(r.Context(), "subscriber write failed",
					slog.Any("error", err))

				return
			}

			flusher.Flush()
		}
	}
}

// write emits one server-sent event.
func write(w http.ResponseWriter, payload []byte, dropped int) error {
	// A subscriber that fell behind is told how many frames it missed, so it
	// can say so rather than quietly showing a coarser chart than it thinks.
	if dropped > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", dropped); err != nil {
			return fmt.Errorf("write id: %w", err)
		}
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}
