//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/scrape"
	"github.com/pantherhawk/prism/internal/series"
)

// scrapeTolerance is the slack allowed when comparing a computed rate. The
// arithmetic is exact; this only guards against float representation.
const scrapeTolerance = 1e-9

// scrapeWorld is the state of one scrape scenario.
type scrapeWorld struct {
	mu   sync.Mutex
	body string
	code int

	server    *httptest.Server
	store     *series.Store
	collector *scrape.Collector
	at        time.Time
}

type scrapeKey struct{}

func scrapeFrom(ctx context.Context) *scrapeWorld {
	w, ok := ctx.Value(scrapeKey{}).(*scrapeWorld)
	if !ok {
		panic("scrape world missing from context")
	}

	return w
}

// serve returns the currently configured exposition body.
func (w *scrapeWorld) serve(rw http.ResponseWriter, _ *http.Request) {
	w.mu.Lock()
	body, code := w.body, w.code
	w.mu.Unlock()

	rw.Header().Set("Content-Type", "text/plain; version=0.0.4")
	rw.WriteHeader(code)
	_, _ = io.WriteString(rw, body)
}

// setBody replaces what the endpoint will serve next.
func (w *scrapeWorld) setBody(body string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.body = body
}

// counterBody renders a single-series counter exposition.
func counterBody(name string, value float64) string {
	return fmt.Sprintf(
		"# HELP %[1]s total requests\n# TYPE %[1]s counter\n%[1]s{cluster=\"api\"} %[2]g\n",
		name, value,
	)
}

// histogramBody renders one histogram the way the text format carries it:
// spread over a bucket line per boundary, a sum and a count, with the family
// itself only named in the TYPE header.
func histogramBody(name string, observations int) string {
	return fmt.Sprintf(`# HELP %[1]s upstream request time
# TYPE %[1]s histogram
%[1]s_bucket{cluster="api",le="0.5"} 1
%[1]s_bucket{cluster="api",le="1"} 2
%[1]s_bucket{cluster="api",le="+Inf"} %[2]d
%[1]s_sum{cluster="api"} 1.5
%[1]s_count{cluster="api"} %[2]d
`, name, observations)
}

// build wires a store and collector against the test server.
func (w *scrapeWorld) build(endpoint string) error {
	quiet := slog.New(slog.DiscardHandler)

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, quiet)
	if err != nil {
		return fmt.Errorf("new store: %w", err)
	}

	cfg := scrape.Default()
	cfg.Endpoint = endpoint
	cfg.Timeout = 2 * time.Second

	collector, err := scrape.New(cfg, store, quiet)
	if err != nil {
		return fmt.Errorf("new collector: %w", err)
	}

	w.store, w.collector = store, collector

	return nil
}

// ---- Given -----------------------------------------------------------------

func (w *scrapeWorld) endpointExposingCounter(
	ctx context.Context, name string, value float64,
) (context.Context, error) {
	w.setBody(counterBody(name, value))
	w.server = httptest.NewServer(http.HandlerFunc(w.serve))

	return ctx, w.build(w.server.URL)
}

func (w *scrapeWorld) endpointReturningMalformedBody(ctx context.Context) (context.Context, error) {
	w.setBody("# TYPE broken counter\nbroken{cluster=\n")
	w.server = httptest.NewServer(http.HandlerFunc(w.serve))

	return ctx, w.build(w.server.URL)
}

func (w *scrapeWorld) endpointThatRefuses(ctx context.Context) (context.Context, error) {
	// A server that is closed immediately gives a port nothing is listening on,
	// which is what a restarting target looks like.
	server := httptest.NewServer(http.HandlerFunc(w.serve))
	url := server.URL
	server.Close()

	return ctx, w.build(url)
}

func (w *scrapeWorld) endpointExposingHistogram(
	ctx context.Context, name string, observations int,
) (context.Context, error) {
	w.setBody(histogramBody(name, observations))
	w.server = httptest.NewServer(http.HandlerFunc(w.serve))

	return ctx, w.build(w.server.URL)
}

// ---- When ------------------------------------------------------------------

func (w *scrapeWorld) itIsScraped(ctx context.Context) (context.Context, error) {
	w.at = time.Unix(1_700_000_000, 0)
	w.collector.Collect(ctx, w.at)

	return ctx, nil
}

func (w *scrapeWorld) reportsAndIsScrapedLater(
	ctx context.Context, value float64, after string,
) (context.Context, error) {
	elapsed, err := time.ParseDuration(after)
	if err != nil {
		return ctx, fmt.Errorf("parse duration: %w", err)
	}

	w.setBody(counterBody("envoy_cluster_upstream_rq_total", value))
	w.at = w.at.Add(elapsed)
	w.collector.Collect(ctx, w.at)

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *scrapeWorld) theSeriesRecords(ctx context.Context, want float64) (context.Context, error) {
	snapshot := w.store.Snapshot()
	if len(snapshot.Series) != 1 {
		return ctx, fmt.Errorf("got %d series, want 1", len(snapshot.Series))
	}

	if got := snapshot.Series[0].Last; math.Abs(got-want) > scrapeTolerance {
		return ctx, fmt.Errorf("rate = %v, want %v", got, want)
	}

	return ctx, nil
}

func (w *scrapeWorld) theScrapeErrorCountIs(ctx context.Context, want int64) (context.Context, error) {
	if got := w.store.Snapshot().Stats.Errors; got != want {
		return ctx, fmt.Errorf("error count = %d, want %d", got, want)
	}

	return ctx, nil
}

func (w *scrapeWorld) aLaterScrapeStillRecords(ctx context.Context) (context.Context, error) {
	w.setBody(counterBody("envoy_cluster_upstream_rq_total", 10))
	w.collector.Collect(ctx, time.Unix(1_700_000_015, 0))

	if len(w.store.Snapshot().Series) == 0 {
		return ctx, errors.New("collector stopped recording after a bad body") //nolint:err113 // assertion
	}

	return ctx, nil
}

func (w *scrapeWorld) theCollectorIsStillRunning(ctx context.Context) (context.Context, error) {
	if w.store.Snapshot().Stats.Scrapes == 0 {
		return ctx, errors.New("no scrape was recorded") //nolint:err113 // assertion
	}

	return ctx, nil
}

func (w *scrapeWorld) oneFamilyNamedIsStored(
	ctx context.Context, want string,
) (context.Context, error) {
	families := w.store.Snapshot().Families

	if len(families) != 1 {
		names := make([]string, 0, len(families))
		for _, family := range families {
			names = append(names, family.Name)
		}

		return ctx, fmt.Errorf("stored %d families %v, want just %q", len(families), names, want)
	}

	if families[0].Name != want {
		return ctx, fmt.Errorf("family = %q, want %q", families[0].Name, want)
	}

	return ctx, nil
}

// noFamilyForTheParts checks that none of the three suffixes became a family of
// its own. It is a separate assertion from the count above because the two fail
// for different reasons: one says the store held the wrong thing, this one says
// the parser never put the histogram back together.
func (w *scrapeWorld) noFamilyForTheParts(
	ctx context.Context, a, b, c string,
) (context.Context, error) {
	for _, family := range w.store.Snapshot().Families {
		for _, suffix := range []string{a, b, c} {
			if strings.HasSuffix(family.Name, suffix) {
				return ctx, fmt.Errorf("%q was stored as a family of its own", family.Name)
			}
		}
	}

	return ctx, nil
}

func (w *scrapeWorld) theSeriesRecordsObservations(
	ctx context.Context, want float64,
) (context.Context, error) {
	snapshot := w.store.Snapshot()
	if len(snapshot.Series) != 1 {
		return ctx, fmt.Errorf("got %d series, want 1", len(snapshot.Series))
	}

	view := snapshot.Series[0]

	if view.Kind != series.KindHistogram {
		return ctx, fmt.Errorf("kind = %q, want %q", view.Kind, series.KindHistogram)
	}

	if math.Abs(view.Last-want) > scrapeTolerance {
		return ctx, fmt.Errorf("observations = %v, want %v", view.Last, want)
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

// initializeScrape registers the scrape steps.
func initializeScrape(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, scrapeKey{}, &scrapeWorld{code: http.StatusOK}), nil
	})

	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w := scrapeFrom(ctx); w.server != nil {
			w.server.Close()
		}

		return ctx, nil
	})

	sc.Step(`^an endpoint exposing counter "([^"]*)" at ([0-9.]+)$`,
		func(ctx context.Context, name string, value float64) (context.Context, error) {
			return scrapeFrom(ctx).endpointExposingCounter(ctx, name, value)
		})
	sc.Step(`^an endpoint returning a malformed body$`,
		func(ctx context.Context) (context.Context, error) {
			return scrapeFrom(ctx).endpointReturningMalformedBody(ctx)
		})
	sc.Step(`^an endpoint that refuses connections$`,
		func(ctx context.Context) (context.Context, error) {
			return scrapeFrom(ctx).endpointThatRefuses(ctx)
		})
	sc.Step(`^an endpoint exposing histogram "([^"]*)" with (\d+) observations$`,
		func(ctx context.Context, name string, observations int) (context.Context, error) {
			return scrapeFrom(ctx).endpointExposingHistogram(ctx, name, observations)
		})

	sc.Step(`^it is scraped$`, func(ctx context.Context) (context.Context, error) {
		return scrapeFrom(ctx).itIsScraped(ctx)
	})
	sc.Step(`^the endpoint reports ([0-9.]+) and is scraped ([0-9a-z]+) later$`,
		func(ctx context.Context, value float64, after string) (context.Context, error) {
			return scrapeFrom(ctx).reportsAndIsScrapedLater(ctx, value, after)
		})

	initializeScrapeAssertions(sc)
}

// initializeScrapeAssertions registers the steps that read the store back.
func initializeScrapeAssertions(sc *godog.ScenarioContext) {
	sc.Step(`^the series records ([0-9.]+) per second$`,
		func(ctx context.Context, want float64) (context.Context, error) {
			return scrapeFrom(ctx).theSeriesRecords(ctx, want)
		})
	sc.Step(`^the scrape error count is (\d+)$`,
		func(ctx context.Context, want int64) (context.Context, error) {
			return scrapeFrom(ctx).theScrapeErrorCountIs(ctx, want)
		})
	sc.Step(`^a later successful scrape still records the series$`,
		func(ctx context.Context) (context.Context, error) {
			return scrapeFrom(ctx).aLaterScrapeStillRecords(ctx)
		})
	sc.Step(`^the collector is still running$`, func(ctx context.Context) (context.Context, error) {
		return scrapeFrom(ctx).theCollectorIsStillRunning(ctx)
	})
	sc.Step(`^one family named "([^"]*)" is stored$`,
		func(ctx context.Context, want string) (context.Context, error) {
			return scrapeFrom(ctx).oneFamilyNamedIsStored(ctx, want)
		})
	sc.Step(`^no family is stored for its "([^"]*)", "([^"]*)" or "([^"]*)" parts$`,
		func(ctx context.Context, a, b, c string) (context.Context, error) {
			return scrapeFrom(ctx).noFamilyForTheParts(ctx, a, b, c)
		})
	sc.Step(`^the series records (\d+) observations$`,
		func(ctx context.Context, want float64) (context.Context, error) {
			return scrapeFrom(ctx).theSeriesRecordsObservations(ctx, want)
		})
}
