package series_test

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// discard is a logger that writes nowhere, so tests stay quiet.
func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestStore(t *testing.T) *series.Store {
	t.Helper()

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	return store
}

func counter(value float64) []series.Sample {
	return []series.Sample{{
		Family: "envoy_cluster_upstream_rq_total",
		Labels: series.Labels{{Name: "cluster", Value: "api"}},
		Kind:   series.KindCounter,
		Value:  value,
	}}
}

func TestCounterBecomesRate(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	store.Append(start, counter(1000), series.Stats{})
	store.Append(start.Add(15*time.Second), counter(1300), series.Stats{})

	snapshot := store.Snapshot()
	if len(snapshot.Series) != 1 {
		t.Fatalf("got %d series, want 1", len(snapshot.Series))
	}

	const want = 20.0

	if got := snapshot.Series[0].Last; math.Abs(got-want) > 1e-9 {
		t.Errorf("rate = %v, want %v", got, want)
	}
}

func TestCounterResetIsNotNegative(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	store.Append(start, counter(100), series.Stats{})
	store.Append(start.Add(15*time.Second), counter(200), series.Stats{})
	store.Append(start.Add(30*time.Second), counter(5), series.Stats{})

	if got := store.Snapshot().Series[0].Last; got != 0 {
		t.Errorf("rate across a reset = %v, want 0", got)
	}
}

// A counter needs two readings before it has a rate. The first one must leave
// a gap, not a zero.
//
// Zero is a claim: at the rates Envoy reports it says the traffic stopped. The
// first frame of every recorded walkthrough carried that claim - the line rose
// out of the floor at the left edge because the opening bucket had been filled
// with a rate nobody could have computed yet.
func TestFirstCounterSampleIsAGapNotZero(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	store.Append(start, counter(1000), series.Stats{})

	snapshot := store.Snapshot()
	if len(snapshot.Series) != 1 {
		t.Fatalf("got %d series, want 1", len(snapshot.Series))
	}

	values := snapshot.Series[0].Window(snapshot.Oldest(), snapshot.Newest)
	for i, value := range values {
		if !math.IsNaN(value) {
			t.Errorf("bucket %d = %v, want a gap: one reading is not a rate", i, value)
		}
	}
}

// The reading that could not be charted still has to be remembered, or the
// second one would have nothing to subtract from and the gap would spread.
func TestRateResumesAfterTheOpeningGap(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	store.Append(start, counter(1000), series.Stats{})
	store.Append(start.Add(15*time.Second), counter(1300), series.Stats{})

	const want = 20.0

	if got := store.Snapshot().Series[0].Last; math.Abs(got-want) > 1e-9 {
		t.Errorf("rate after the opening gap = %v, want %v", got, want)
	}
}

func TestGapsAreNotZero(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	store.Append(start, counter(100), series.Stats{})
	// Skip two intervals, then resume.
	store.Append(start.Add(45*time.Second), counter(400), series.Stats{})

	snapshot := store.Snapshot()
	values := snapshot.Series[0].Window(snapshot.Oldest(), snapshot.Newest)

	var gaps int

	for _, value := range values {
		if math.IsNaN(value) {
			gaps++
		}
	}

	if gaps == 0 {
		t.Error("missing buckets rendered as values; a gap must stay a gap")
	}
}

func TestCardinalityIsCounted(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	samples := make([]series.Sample, 0, 3)
	for _, cluster := range []string{"api", "auth", "web"} {
		samples = append(samples, series.Sample{
			Family: "envoy_cluster_upstream_rq_total",
			Labels: series.Labels{{Name: "cluster", Value: cluster}},
			Kind:   series.KindCounter,
			Value:  1,
		})
	}

	store.Append(start, samples, series.Stats{})

	families := store.Snapshot().Families
	if len(families) != 1 || families[0].Cardinality != 3 {
		t.Fatalf("cardinality = %+v, want one family of 3", families)
	}
}

// A budget is a cap, not a target.
//
// Halving the sampling ratio can only ever land near a budget, never on it, and
// at the 1:1024 floor it stops halving altogether: 100,000 series thinned to a
// thousandth leaves about 98, give or take a standard deviation of ten. Whether
// that lands under 100 depends on nothing more than how the family happens to be
// named, which is why this checks several rather than trusting one.
func TestBudgetIsNeverExceeded(t *testing.T) {
	t.Parallel()

	const (
		budget = 100
		count  = 100_000
	)

	for _, family := range []string{"rq", "http_requests", "a", "b", "d", "envoy_cluster_upstream_rq_total"} {
		store, err := series.NewStore(time.Minute, 15*time.Second, budget, discard())
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}

		samples := make([]series.Sample, 0, count)
		for i := range count {
			samples = append(samples, series.Sample{
				Family: family,
				Labels: series.Labels{{Name: "id", Value: fmt.Sprintf("series-%d", i)}},
				Kind:   series.KindGauge,
				Value:  float64(i),
			})
		}

		store.Append(time.Unix(1_700_000_000, 0), samples, series.Stats{})

		snapshot := store.Snapshot()
		if got := len(snapshot.Series); got > budget {
			t.Errorf("family %q stored %d series against a budget of %d", family, got, budget)
		}

		if got := snapshot.Families[0].Stored; got > budget {
			t.Errorf("family %q reports %d stored against a budget of %d", family, got, budget)
		}
	}
}

// Which series survive the budget must be a function of identity alone. Keeping
// whichever arrived first is the truncation P4 removed: it drops series based on
// nothing but the order the target happened to write them in.
func TestWhichSeriesSurviveDoesNotDependOnArrivalOrder(t *testing.T) {
	t.Parallel()

	const (
		budget = 100
		count  = 100_000
	)

	forward := storedIDs(t, budget, count, false)
	reversed := storedIDs(t, budget, count, true)

	if len(forward) != len(reversed) {
		t.Fatalf("kept %d series forwards and %d backwards", len(forward), len(reversed))
	}

	for id := range forward {
		if _, ok := reversed[id]; !ok {
			t.Fatalf("series %d survived one arrival order but not the other", id)
		}
	}
}

// storedIDs fills a store with count series and returns which survived.
func storedIDs(t *testing.T, budget, count int, reverse bool) map[series.ID]struct{} {
	t.Helper()

	store, err := series.NewStore(time.Minute, 15*time.Second, budget, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	samples := make([]series.Sample, 0, count)
	for i := range count {
		samples = append(samples, series.Sample{
			Family: "rq",
			Labels: series.Labels{{Name: "id", Value: fmt.Sprintf("series-%d", i)}},
			Kind:   series.KindGauge,
			Value:  float64(i),
		})
	}

	if reverse {
		slices.Reverse(samples)
	}

	store.Append(time.Unix(1_700_000_000, 0), samples, series.Stats{})

	kept := make(map[series.ID]struct{})
	for _, view := range store.Snapshot().Series {
		kept[view.ID] = struct{}{}
	}

	return kept
}

func TestRetentionIsBounded(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Unix(1_700_000_000, 0)

	// Twenty intervals into a store that keeps four.
	for i := range 20 {
		store.Append(start.Add(time.Duration(i)*15*time.Second), counter(float64(100*i)), series.Stats{})
	}

	snapshot := store.Snapshot()

	values := snapshot.Series[0].Window(snapshot.Oldest(), snapshot.Newest)
	if len(values) != snapshot.Capacity {
		t.Errorf("window held %d buckets, want %d", len(values), snapshot.Capacity)
	}
}
