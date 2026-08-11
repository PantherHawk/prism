package series_test

import (
	"math"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// gauge builds a sample for a family with the given labels.
func gauge(family string, value float64, pairs ...string) series.Sample {
	labels := make(series.Labels, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		labels = append(labels, series.Label{Name: pairs[i], Value: pairs[i+1]})
	}

	labels.Sort()

	return series.Sample{Family: family, Labels: labels, Kind: series.KindGauge, Value: value}
}

// populated returns a snapshot holding one family split across two labels.
func populated(t *testing.T) *series.Snapshot {
	t.Helper()

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	store.Append(time.Unix(1_700_000_000, 0), []series.Sample{
		gauge("rq", 100, "cluster", "api", "status", "200"),
		gauge("rq", 20, "cluster", "api", "status", "500"),
		gauge("rq", 60, "cluster", "auth", "status", "200"),
		gauge("rq", 5, "cluster", "web", "status", "200"),
		gauge("cx", 7, "listener", "http"),
	}, series.Stats{})

	return store.Snapshot()
}

func TestPivotSumsEachGroup(t *testing.T) {
	t.Parallel()

	snapshot := populated(t)

	lines := snapshot.Lines(series.Selection{Family: "rq", PivotKey: "cluster", Limit: 10})
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 clusters", len(lines))
	}

	// Largest first, and api's two status series are summed.
	if lines[0].Label != "api" || lines[0].Last != 120 || lines[0].Members != 2 {
		t.Errorf("first line = %+v, want api/120/2 members", lines[0])
	}

	if lines[1].Label != "auth" || lines[1].Last != 60 {
		t.Errorf("second line = %+v, want auth/60", lines[1])
	}
}

func TestPivotCollapsesTheTail(t *testing.T) {
	t.Parallel()

	snapshot := populated(t)

	lines := snapshot.Lines(series.Selection{Family: "rq", PivotKey: "cluster", Limit: 2})
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	other := lines[1]
	if other.Label != series.OtherLabel {
		t.Fatalf("tail line = %q, want %q", other.Label, series.OtherLabel)
	}

	// auth (60) plus web (5), and the count has to say so.
	if other.Last != 65 || other.Members != 2 {
		t.Errorf("other = %+v, want 65 across 2 members", other)
	}
}

func TestPivotSeparatesAbsentFromEmpty(t *testing.T) {
	t.Parallel()

	snapshot := populated(t)

	lines := snapshot.Lines(series.Selection{Family: "cx", PivotKey: "cluster", Limit: 10})
	if len(lines) != 1 || lines[0].Label != series.AbsentLabel {
		t.Errorf("lines = %+v, want a single %q group", lines, series.AbsentLabel)
	}
}

func TestFilterNarrowsTheSelection(t *testing.T) {
	t.Parallel()

	snapshot := populated(t)

	only200 := func(_ string, labels series.Labels) bool {
		return labels.Get("status") == "200"
	}

	lines := snapshot.Lines(series.Selection{
		Family:   "rq",
		Match:    only200,
		PivotKey: "cluster",
		Limit:    10,
	})

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}

	// api loses its 500s, so it drops from 120 to 100.
	if lines[0].Label != "api" || lines[0].Last != 100 {
		t.Errorf("first line = %+v, want api/100", lines[0])
	}
}

// TestPartialGapIsNotAHole pins the summing rule: a group keeps reporting while
// any member has data. Treating a partial bucket as missing would make a total
// drop out whenever one contributor missed a scrape.
func TestPartialGapIsNotAHole(t *testing.T) {
	t.Parallel()

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	start := time.Unix(1_700_000_000, 0)

	store.Append(start, []series.Sample{
		gauge("rq", 10, "cluster", "api", "status", "200"),
		gauge("rq", 5, "cluster", "api", "status", "500"),
	}, series.Stats{})

	// Second scrape: only one of the two series reports.
	store.Append(start.Add(15*time.Second), []series.Sample{
		gauge("rq", 10, "cluster", "api", "status", "200"),
	}, series.Stats{})

	snapshot := store.Snapshot()

	lines := snapshot.Lines(series.Selection{Family: "rq", PivotKey: "cluster", Limit: 10})
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}

	values := lines[0].Window(snapshot.Newest, snapshot.Newest)
	if len(values) != 1 || math.IsNaN(values[0]) {
		t.Fatalf("values = %v, want a present total", values)
	}

	if values[0] != 10 {
		t.Errorf("total = %v, want 10 from the one reporting member", values[0])
	}
}

func TestLabelKeysAreOffered(t *testing.T) {
	t.Parallel()

	keys := populated(t).LabelKeys("rq")
	if len(keys) != 2 || keys[0] != "cluster" || keys[1] != "status" {
		t.Errorf("keys = %v, want [cluster status]", keys)
	}
}

// The legend shows one number per line, and zero is a claim about traffic. A
// series whose newest bucket is empty has no current value, and the recorded
// P5 frame said `api 0` beside a chart that correctly drew nothing there.
func TestLastIsUnknownWhenTheNewestBucketIsEmpty(t *testing.T) {
	t.Parallel()

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	start := time.Unix(1_700_000_000, 0)
	store.Append(start, []series.Sample{gauge("rq", 100, "cluster", "api")}, series.Stats{})

	// A later scrape that carries nothing: the ring advances past the reading.
	store.Append(start.Add(45*time.Second), nil, series.Stats{})

	snapshot := store.Snapshot()
	if len(snapshot.Series) != 1 {
		t.Fatalf("got %d series, want 1", len(snapshot.Series))
	}

	if got := snapshot.Series[0].Last; !math.IsNaN(got) {
		t.Errorf("Last = %v, want no value: the newest bucket is empty", got)
	}
}

// One member falling silent must not drag a pivoted total to the floor, and a
// group where every member is silent has no total to show. This is the rule
// Window already applies bucket by bucket, applied to the legend's figure.
func TestPivotTotalsOnlyTheMembersThatHaveAValue(t *testing.T) {
	t.Parallel()

	store, err := series.NewStore(time.Minute, 15*time.Second, 0, discard())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	start := time.Unix(1_700_000_000, 0)

	store.Append(start, []series.Sample{
		gauge("rq", 100, "cluster", "api", "status", "200"),
		gauge("rq", 20, "cluster", "api", "status", "500"),
	}, series.Stats{})

	// Only one of api's two series reports in the newest bucket.
	store.Append(start.Add(15*time.Second), []series.Sample{
		gauge("rq", 70, "cluster", "api", "status", "200"),
	}, series.Stats{})

	lines := store.Snapshot().Lines(series.Selection{Family: "rq", PivotKey: "cluster", Limit: 10})
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}

	if lines[0].Last != 70 {
		t.Errorf("total = %v, want 70: the silent member is absent, not zero", lines[0].Last)
	}
}
