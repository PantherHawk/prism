package stream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// ErrNoUpstream is returned when a client is built without somewhere to follow.
var ErrNoUpstream = errors.New("no upstream configured")

// ErrUnexpectedStatus is returned when an upstream answers a stream request
// with anything other than 200.
var ErrUnexpectedStatus = errors.New("unexpected status")

// Reconnection schedule and the bound on one event.
const (
	// The wait before the first reconnection attempt.
	baseBackoff = 500 * time.Millisecond

	// The cap on that wait. Past this the delay stops being politeness and
	// starts being an outage of its own: an operator staring at a reconnecting
	// badge wants it to recover within a breath of the leader coming back.
	maxBackoff = 30 * time.Second

	// How much of a delay is randomised. Ten followers of a restarted leader
	// must not reconnect in lockstep and knock it over again.
	jitterFraction = 0.3

	// maxEventBytes bounds one server-sent event. A frame is samples, not a
	// rendered screen, but an upstream is still a remote input and an
	// unbounded line reader is an unbounded allocation.
	maxEventBytes = 16 << 20
)

// Client follows another prism's stream and feeds what it receives into a sink.
//
// It is deliberately the mirror of [Broker]: frames carry samples, so a
// follower rebuilds exactly the store it would have built by scraping the
// target itself, and the two ends share no state beyond the wire format.
type Client struct {
	cfg  Config
	sink series.Sink
	log  *slog.Logger

	client *http.Client
	stats  series.Stats
}

// NewClient returns a client following cfg.Upstream.
func NewClient(cfg Config, sink series.Sink, log *slog.Logger) (*Client, error) {
	if cfg.Upstream == "" {
		return nil, ErrNoUpstream
	}

	return &Client{
		cfg:  cfg,
		sink: sink,
		log:  log,
		// No client timeout: a stream is a long-lived response body, and a
		// timeout here would sever a healthy connection on a quiet target.
		// Liveness is the upstream's problem, and a dead one shows up as a read
		// error on the body.
		client: &http.Client{},
		stats:  series.Stats{Source: cfg.Upstream},
	}, nil
}

// Run follows the upstream until the context is cancelled, reconnecting with
// backoff whenever the link drops.
//
// A disconnect is never fatal. The history already in the ring is left alone:
// what was observed happened, and losing the link does not unhappen it.
func (c *Client) Run(ctx context.Context, errs chan<- error) {
	c.log.InfoContext(ctx, "following upstream", slog.String("upstream", c.cfg.Upstream))

	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			errs <- nil

			return
		}

		if err := c.follow(ctx); err != nil && ctx.Err() == nil {
			c.log.WarnContext(ctx, "upstream dropped", slog.Any("error", err))
		}

		if ctx.Err() != nil {
			errs <- nil

			return
		}

		delay := backoff(attempt)
		c.reportOutage(delay)

		select {
		case <-ctx.Done():
			errs <- nil

			return
		case <-time.After(delay):
		}
	}
}

// reportOutage tells the sink that the link is down and when it will be tried
// again. The retry time is published rather than merely logged because a chart
// that silently stops updating is the worst failure a monitoring tool has.
func (c *Client) reportOutage(retryIn time.Duration) {
	c.stats.Reconnecting = true
	c.stats.RetryIn = retryIn

	c.sink.Append(time.Now(), nil, c.stats)
}

// follow opens one connection and reads it until it ends.
func (c *Client) follow(ctx context.Context) error {
	url := c.cfg.Upstream + StreamPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.cfg.Upstream, err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.log.DebugContext(ctx, "closing upstream body", slog.Any("error", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", ErrUnexpectedStatus, resp.Status)
	}

	// The connection is up, so anything the operator was told about an outage
	// is no longer true. Clearing it here rather than on the first frame means
	// the badge clears when the link recovers, not when the leader next
	// happens to scrape.
	c.stats.Reconnecting = false
	c.stats.RetryIn = 0

	return c.read(ctx, resp.Body)
}

// read consumes the event stream, feeding each frame to the sink.
func (c *Client) read(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventBytes)

	dropped := 0

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "id:"):
			// How many frames this subscriber missed while it was behind. An
			// unparseable count is simply not reported: it says nothing about
			// the frame that follows, which is the part worth having.
			count, convErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "id:")))
			if convErr == nil {
				dropped = count
			}

		case strings.HasPrefix(line, "data:"):
			c.consume(ctx, strings.TrimSpace(strings.TrimPrefix(line, "data:")), dropped)

			dropped = 0

		default:
			// Blank separators and any field prism does not use.
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	return nil
}

// consume decodes one event and appends it.
//
// A frame that will not decode is dropped with a log line rather than tearing
// the connection down: one malformed event says nothing about the next, and a
// reconnect would cost every frame that arrived while it was in progress.
func (c *Client) consume(ctx context.Context, payload string, dropped int) {
	frame, err := decode([]byte(payload))
	if err != nil {
		c.log.WarnContext(ctx, "undecodable frame", slog.Any("error", err))

		return
	}

	if dropped > 0 {
		c.log.DebugContext(ctx, "frames coalesced upstream", slog.Int("dropped", dropped))
	}

	// The leader's counters describe the scrape; the source and link state are
	// this follower's own, and must not be overwritten by them.
	stats := frame.Stats
	stats.Source = c.cfg.Upstream
	stats.Reconnecting = c.stats.Reconnecting
	stats.RetryIn = c.stats.RetryIn

	c.sink.Append(frame.At, frame.Samples, stats)
}

// backoff returns how long to wait before the given reconnection attempt,
// doubling up to the cap and randomised so that followers spread out.
func backoff(attempt int) time.Duration {
	delay := baseBackoff

	for range attempt {
		delay *= 2
		if delay >= maxBackoff {
			delay = maxBackoff

			break
		}
	}

	// Jitter is subtracted rather than added so that the cap stays a cap.
	//nolint:gosec // spreading reconnects is not a cryptographic decision
	jitter := time.Duration(rand.Float64() * jitterFraction * float64(delay))

	return delay - jitter
}
