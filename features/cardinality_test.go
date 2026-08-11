//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/bits"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/series"
)

// cardinalityWorld is the state of one cardinality scenario.
type cardinalityWorld struct {
	budget int
	store  *series.Store
	at     time.Time
}

type cardinalityKey struct{}

func cardinalityFrom(ctx context.Context) *cardinalityWorld {
	w, ok := ctx.Value(cardinalityKey{}).(*cardinalityWorld)
	if !ok {
		panic("cardinality world missing from context")
	}

	return w
}

// family returns the view for the family under test.
func (w *cardinalityWorld) family() (series.FamilyView, error) {
	for _, view := range w.store.Snapshot().Families {
		if view.Name == "rq" {
			return view, nil
		}
	}

	return series.FamilyView{}, errNoFamily
}

// ---- Given -----------------------------------------------------------------

func (w *cardinalityWorld) aFamilyBudgetOf(ctx context.Context, budget int) (context.Context, error) {
	store, err := series.NewStore(time.Minute, 15*time.Second, budget,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return ctx, fmt.Errorf("new store: %w", err)
	}

	w.budget, w.store = budget, store
	w.at = time.Unix(1_700_000_000, 0)

	return ctx, nil
}

// ---- When ------------------------------------------------------------------

// seriesAreObserved appends one scrape holding count distinct series. The label
// value carries the index, so every series is a genuinely different label set
// rather than the same one repeated.
func (w *cardinalityWorld) seriesAreObserved(
	ctx context.Context, count int, family string,
) (context.Context, error) {
	samples := make([]series.Sample, 0, count)

	for i := range count {
		samples = append(samples, series.Sample{
			Family: family,
			Labels: series.Labels{{Name: "id", Value: fmt.Sprintf("series-%d", i)}},
			Kind:   series.KindGauge,
			Value:  float64(i),
		})
	}

	w.store.Append(w.at, samples, series.Stats{})
	w.at = w.at.Add(15 * time.Second)

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *cardinalityWorld) cardinalityIsExactly(ctx context.Context, want int) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if view.Cardinality != want {
		return ctx, fmt.Errorf("cardinality = %d, want exactly %d", view.Cardinality, want)
	}

	return ctx, nil
}

func (w *cardinalityWorld) cardinalityIsWithin(
	ctx context.Context, percent, want int,
) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	tolerance := float64(percent) / 100
	drift := math.Abs(float64(view.Cardinality-want)) / float64(want)

	if drift > tolerance {
		return ctx, fmt.Errorf(
			"cardinality = %d, want within %d%% of %d (off by %.1f%%)",
			view.Cardinality, percent, want, drift*100)
	}

	return ctx, nil
}

func (w *cardinalityWorld) atMostSeriesAreStored(ctx context.Context, want int) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if view.Stored > want {
		return ctx, fmt.Errorf("%d series stored, want at most %d", view.Stored, want)
	}

	if view.Stored != len(w.store.Snapshot().Series) {
		return ctx, fmt.Errorf(
			"family reports %d stored but the snapshot holds %d series",
			view.Stored, len(w.store.Snapshot().Series))
	}

	return ctx, nil
}

func (w *cardinalityWorld) theFamilyIsSampled(ctx context.Context) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if !view.Sampled() {
		return ctx, errNotSampled
	}

	return ctx, nil
}

func (w *cardinalityWorld) theFamilyIsNotSampled(ctx context.Context) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if view.Sampled() {
		return ctx, fmt.Errorf("family was sampled at 1:%d when it fits its budget", view.Ratio)
	}

	return ctx, nil
}

func (w *cardinalityWorld) theRatioIsAPowerOfTwo(ctx context.Context) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if bits.OnesCount(uint(view.Ratio)) != 1 {
		return ctx, fmt.Errorf("ratio 1:%d is not a power of two", view.Ratio)
	}

	return ctx, nil
}

func (w *cardinalityWorld) cardinalityIsNotAnEstimate(ctx context.Context) (context.Context, error) {
	view, err := w.family()
	if err != nil {
		return ctx, err
	}

	if view.Estimated {
		return ctx, errEstimated
	}

	return ctx, nil
}

func (w *cardinalityWorld) snapshotRatioAboveOne(ctx context.Context) (context.Context, error) {
	if got := w.store.Snapshot().SamplingRatio(); got <= 1 {
		return ctx, fmt.Errorf("snapshot sampling ratio = %d, want above 1", got)
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

var (
	errNoFamily   = errors.New(`no family named "rq" in the snapshot`)
	errNotSampled = errors.New("family was not sampled despite exceeding its budget")
	errEstimated  = errors.New("a small family reported an estimate")
)

// initializeCardinality registers the cardinality steps.
func initializeCardinality(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, cardinalityKey{}, &cardinalityWorld{}), nil
	})

	sc.Step(`^a family budget of (\d+)$`, func(ctx context.Context, budget int) (context.Context, error) {
		return cardinalityFrom(ctx).aFamilyBudgetOf(ctx, budget)
	})
	sc.Step(`^(\d+) series of "([^"]*)" are observed$`,
		func(ctx context.Context, count int, family string) (context.Context, error) {
			return cardinalityFrom(ctx).seriesAreObserved(ctx, count, family)
		})

	sc.Step(`^the reported cardinality is exactly (\d+)$`,
		func(ctx context.Context, want int) (context.Context, error) {
			return cardinalityFrom(ctx).cardinalityIsExactly(ctx, want)
		})
	sc.Step(`^the reported cardinality is within (\d+)% of (\d+)$`,
		func(ctx context.Context, percent, want int) (context.Context, error) {
			return cardinalityFrom(ctx).cardinalityIsWithin(ctx, percent, want)
		})
	sc.Step(`^at most (\d+) series are stored$`, func(ctx context.Context, want int) (context.Context, error) {
		return cardinalityFrom(ctx).atMostSeriesAreStored(ctx, want)
	})
	sc.Step(`^the family is sampled$`, func(ctx context.Context) (context.Context, error) {
		return cardinalityFrom(ctx).theFamilyIsSampled(ctx)
	})
	sc.Step(`^the family is not sampled$`, func(ctx context.Context) (context.Context, error) {
		return cardinalityFrom(ctx).theFamilyIsNotSampled(ctx)
	})
	sc.Step(`^the sampling ratio is a power of two$`, func(ctx context.Context) (context.Context, error) {
		return cardinalityFrom(ctx).theRatioIsAPowerOfTwo(ctx)
	})
	sc.Step(`^the cardinality is not an estimate$`, func(ctx context.Context) (context.Context, error) {
		return cardinalityFrom(ctx).cardinalityIsNotAnEstimate(ctx)
	})
	sc.Step(`^the snapshot reports a sampling ratio above 1$`,
		func(ctx context.Context) (context.Context, error) {
			return cardinalityFrom(ctx).snapshotRatioAboveOne(ctx)
		})
}
