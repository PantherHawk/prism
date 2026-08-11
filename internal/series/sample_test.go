package series_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/sketch"
)

func TestLabelsSortOrdersByName(t *testing.T) {
	t.Parallel()

	labels := series.Labels{
		{Name: "status", Value: "200"},
		{Name: "cluster", Value: "api"},
	}
	labels.Sort()

	if labels[0].Name != "cluster" || labels[1].Name != "status" {
		t.Errorf("sorted labels = %v, want cluster before status", labels)
	}
}

func TestLabelsStringRendersPairs(t *testing.T) {
	t.Parallel()

	labels := series.Labels{
		{Name: "cluster", Value: "api"},
		{Name: "status", Value: "200"},
	}

	const want = "cluster=api status=200"

	if got := labels.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestLabelsLookup(t *testing.T) {
	t.Parallel()

	labels := series.Labels{{Name: "cluster", Value: "api"}}

	value, ok := labels.Lookup("cluster")
	if !ok || value != "api" {
		t.Errorf("Lookup(cluster) = %q, %v; want api, true", value, ok)
	}

	// An absent label is not the same as one set to the empty string, so the
	// second result has to distinguish them.
	if _, ok := labels.Lookup("status"); ok {
		t.Error("Lookup(status) reported a label that is not set")
	}
}

// Get is the accessor a matcher wants: `status=200` should fail on a series
// with no status label rather than force every caller to handle the miss.
func TestLabelsGetReturnsEmptyWhenAbsent(t *testing.T) {
	t.Parallel()

	labels := series.Labels{{Name: "cluster", Value: "api"}}

	if got := labels.Get("cluster"); got != "api" {
		t.Errorf("Get(cluster) = %q, want api", got)
	}

	if got := labels.Get("status"); got != "" {
		t.Errorf("Get(status) = %q, want empty", got)
	}
}

func TestLabelsLookupDistinguishesEmptyFromAbsent(t *testing.T) {
	t.Parallel()

	labels := series.Labels{{Name: "cluster", Value: ""}}

	value, ok := labels.Lookup("cluster")
	if !ok || value != "" {
		t.Errorf("Lookup of an empty-valued label = %q, %v; want \"\", true", value, ok)
	}
}

func TestFingerprintIsStable(t *testing.T) {
	t.Parallel()

	first := series.Fingerprint("rq", series.Labels{{Name: "cluster", Value: "api"}})
	second := series.Fingerprint("rq", series.Labels{{Name: "cluster", Value: "api"}})

	if first != second {
		t.Errorf("Fingerprint is not stable: %d then %d", first, second)
	}
}

func TestFingerprintSeparatesSeries(t *testing.T) {
	t.Parallel()

	base := series.Fingerprint("rq", series.Labels{{Name: "cluster", Value: "api"}})

	cases := map[string]series.ID{
		"different family": series.Fingerprint("cx",
			series.Labels{{Name: "cluster", Value: "api"}}),
		"different value": series.Fingerprint("rq",
			series.Labels{{Name: "cluster", Value: "web"}}),
		"different name": series.Fingerprint("rq",
			series.Labels{{Name: "listener", Value: "api"}}),
		"no labels": series.Fingerprint("rq", nil),
		"extra label": series.Fingerprint("rq", series.Labels{
			{Name: "cluster", Value: "api"},
			{Name: "status", Value: "200"},
		}),
	}

	for name, other := range cases {
		if other == base {
			t.Errorf("%s collided with the base fingerprint", name)
		}
	}
}

// Fingerprint must not be confusable by shifting the boundary between a name
// and its value: cluster="apistatus" and cluster="api" status="" are different
// series, and a naive concatenation would give them the same identity.
func TestFingerprintIsNotConfusableAcrossBoundaries(t *testing.T) {
	t.Parallel()

	joined := series.Fingerprint("rq", series.Labels{{Name: "cluster", Value: "apistatus"}})
	split := series.Fingerprint("rq", series.Labels{
		{Name: "cluster", Value: "api"},
		{Name: "status", Value: ""},
	})

	if joined == split {
		t.Error("fingerprints collided across a name/value boundary")
	}
}

// Fingerprints feed a K-Minimum-Values sketch, whose estimator is only valid
// over hashes spread uniformly across the 64-bit range. Series in one family
// differ by a few characters in one label value, which is the exact shape that
// finds a weak hash: raw FNV over this sequence reports 12,400 for 10,000.
func TestFingerprintsAreUniformEnoughToCount(t *testing.T) {
	t.Parallel()

	const (
		count     = 10000
		tolerance = 0.05
	)

	distinct := sketch.NewDistinct(0, 0)

	for i := range count {
		labels := series.Labels{{Name: "id", Value: fmt.Sprintf("series-%d", i)}}
		distinct.Add(uint64(series.Fingerprint("rq", labels)))
	}

	got := distinct.Count()

	if drift := math.Abs(float64(got-count)) / count; drift > tolerance {
		t.Errorf("counted %d of %d series (%.1f%% out); fingerprints are not uniform enough",
			got, count, drift*100)
	}
}

func TestCounterUnitIsPerSecond(t *testing.T) {
	t.Parallel()

	// The store converts counters to rates, so the header has to say so.
	if got := series.KindCounter.Unit(); got != "/s" {
		t.Errorf("KindCounter.Unit() = %q, want %q", got, "/s")
	}

	if got := series.KindGauge.Unit(); got != "" {
		t.Errorf("KindGauge.Unit() = %q, want empty", got)
	}
}

// The stream protocol carries samples as JSON, so a follower must rebuild
// exactly what the leader recorded.
func TestSampleSurvivesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := series.Sample{
		Family: "envoy_cluster_upstream_rq_total",
		Labels: series.Labels{{Name: "cluster", Value: "api"}},
		Kind:   series.KindCounter,
		Value:  1300,
	}

	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got series.Sample

	err = json.Unmarshal(payload, &got)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Family != want.Family || got.Kind != want.Kind || got.Value != want.Value {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	if got.Labels.String() != want.Labels.String() {
		t.Errorf("labels round trip = %q, want %q", got.Labels.String(), want.Labels.String())
	}
}
