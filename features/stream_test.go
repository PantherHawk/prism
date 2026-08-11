//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/stream"
)

// streamSettle bounds how long a scenario waits for a frame to cross the wire.
const streamSettle = 5 * time.Second

// streamWorld is the state of one streaming scenario.
type streamWorld struct {
	broker   *stream.Broker
	server   *httptest.Server
	store    *series.Store
	cancel   context.CancelFunc
	recorded int
}

type streamKey struct{}

func streamFrom(ctx context.Context) *streamWorld {
	w, ok := ctx.Value(streamKey{}).(*streamWorld)
	if !ok {
		panic("stream world missing from context")
	}

	return w
}

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// await polls until the condition holds or the deadline passes.
func await(what string, cond func() bool) error {
	deadline := time.Now().Add(streamSettle)

	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}

		time.Sleep(5 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for %s", what)
}

// ---- Given -----------------------------------------------------------------

func (w *streamWorld) aLeaderPublishing(ctx context.Context) (context.Context, error) {
	broker, err := stream.New(stream.Default(), quiet())
	if err != nil {
		return ctx, fmt.Errorf("new broker: %w", err)
	}

	w.broker = broker
	w.server = httptest.NewServer(broker.Handler())

	return ctx, nil
}

func (w *streamWorld) aFollowerAttached(ctx context.Context) (context.Context, error) {
	store, err := series.NewStore(time.Minute, 15*time.Second, 0, quiet())
	if err != nil {
		return ctx, fmt.Errorf("new store: %w", err)
	}

	client, err := stream.NewClient(
		stream.Config{Upstream: w.server.URL}, store, quiet())
	if err != nil {
		return ctx, fmt.Errorf("new client: %w", err)
	}

	w.store = store

	followCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	w.cancel = cancel

	errs := make(chan error, 1)
	go client.Run(followCtx, errs)

	return ctx, await("the follower to attach", func() bool { return w.broker.Subscribers() > 0 })
}

// ---- When ------------------------------------------------------------------

func (w *streamWorld) theLeaderRecords(ctx context.Context, count int) (context.Context, error) {
	samples := make([]series.Sample, 0, count)

	for i := range count {
		samples = append(samples, series.Sample{
			Family: "rq",
			Labels: series.Labels{{Name: "id", Value: fmt.Sprint(i)}},
			Kind:   series.KindGauge,
			Value:  float64(i),
		})
	}

	w.recorded = count
	w.broker.Append(time.Unix(1_700_000_000, 0), samples, series.Stats{Scrapes: 1})

	return ctx, await("the frame to arrive", func() bool {
		return len(w.store.Snapshot().Series) == count
	})
}

// theLeaderGoesAway severs the link the way a leader actually disappears:
// abruptly, mid-stream.
//
// The connections are cut before the server is closed because a subscription is
// never idle - it is a response body held open indefinitely - and Close waits
// for outstanding requests. Closing first would deadlock the scenario against
// the very connection it is trying to break. Broker.Shutdown does not have this
// problem: it closes its subscribers before the server, which is what a real
// prism leaving does.
func (w *streamWorld) theLeaderGoesAway(ctx context.Context) (context.Context, error) {
	w.server.CloseClientConnections()
	w.server.Close()
	w.server = nil

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *streamWorld) theFollowerHolds(ctx context.Context, want int) (context.Context, error) {
	if got := len(w.store.Snapshot().Series); got != want {
		return ctx, fmt.Errorf("follower holds %d series, want %d", got, want)
	}

	return ctx, nil
}

// theFollowerStillHolds is the half of the acceptance criterion that is easy to
// get wrong: an outage must not clear the ring. What was already observed
// happened, and losing the link does not unhappen it.
func (w *streamWorld) theFollowerStillHolds(ctx context.Context, want int) (context.Context, error) {
	if err := await("the disconnect to register", func() bool {
		return w.store.Snapshot().Stats.Reconnecting
	}); err != nil {
		return ctx, err
	}

	return w.theFollowerHolds(ctx, want)
}

func (w *streamWorld) theFollowerReportsReconnecting(ctx context.Context) (context.Context, error) {
	if err := await("the outage to be reported", func() bool {
		return w.store.Snapshot().Stats.Reconnecting
	}); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func (w *streamWorld) theFollowerShowsWhenItWillRetry(ctx context.Context) (context.Context, error) {
	if w.store.Snapshot().Stats.RetryIn <= 0 {
		return ctx, errNoRetryShown
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

var errNoRetryShown = errors.New("reconnecting was reported without a retry delay")

// initializeStream registers the streaming steps.
func initializeStream(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, streamKey{}, &streamWorld{}), nil
	})

	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w := streamFrom(ctx)

		// The follower is stopped before the leader, so that the subscription
		// is already gone when the server waits for its connections.
		if w.cancel != nil {
			w.cancel()
		}

		if w.server != nil {
			w.server.CloseClientConnections()
			w.server.Close()
		}

		return ctx, nil
	})

	sc.Step(`^a leader publishing a stream$`, func(ctx context.Context) (context.Context, error) {
		return streamFrom(ctx).aLeaderPublishing(ctx)
	})
	sc.Step(`^a follower attached to it$`, func(ctx context.Context) (context.Context, error) {
		return streamFrom(ctx).aFollowerAttached(ctx)
	})

	sc.Step(`^the leader records (\d+) series$`, func(ctx context.Context, n int) (context.Context, error) {
		return streamFrom(ctx).theLeaderRecords(ctx, n)
	})
	sc.Step(`^the leader goes away$`, func(ctx context.Context) (context.Context, error) {
		return streamFrom(ctx).theLeaderGoesAway(ctx)
	})

	sc.Step(`^the follower holds (\d+) series$`, func(ctx context.Context, n int) (context.Context, error) {
		return streamFrom(ctx).theFollowerHolds(ctx, n)
	})
	sc.Step(`^the follower still holds (\d+) series$`, func(ctx context.Context, n int) (context.Context, error) {
		return streamFrom(ctx).theFollowerStillHolds(ctx, n)
	})
	sc.Step(`^the follower reports reconnecting$`, func(ctx context.Context) (context.Context, error) {
		return streamFrom(ctx).theFollowerReportsReconnecting(ctx)
	})
	sc.Step(`^the follower shows when it will retry$`, func(ctx context.Context) (context.Context, error) {
		return streamFrom(ctx).theFollowerShowsWhenItWillRetry(ctx)
	})
}
