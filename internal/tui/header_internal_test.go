package tui

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/banner"
	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/theme"
)

// chromeModel builds a model holding one series of the given family, ready to
// draw the header and footer against.
//
// The header and the footer have the same defect and the same invariant, so
// they are exercised from one fixture; what differs is only which row is drawn.
func chromeModel(t *testing.T, family, pivot string) model {
	t.Helper()

	store, err := series.NewStore(time.Minute, 2*time.Second, 0, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	at := time.Unix(1_700_000_000, 0)
	sample := []series.Sample{{
		Family: family,
		Labels: series.Labels{{Name: "envoy_worker_id", Value: "7"}},
		Kind:   series.KindGauge,
		Value:  1,
	}}

	store.Append(at, sample, series.Stats{})
	store.Append(at.Add(2*time.Second), sample, series.Stats{})

	m := newModel(theme.Config{Mode: theme.ModeDark}, banner.Info{}, 0, store)
	m.snapshot = store.Snapshot()
	m.pivotKey = pivot

	return m
}

// headerRow draws just the header at a given width and returns it as text.
func headerRow(t *testing.T, width int, family, pivot string) string {
	t.Helper()

	m := chromeModel(t, family, pivot)
	f := newFrame(width, 2, m.table)
	m.drawHeader(f, width-1)

	return plain(f)[1]
}

// footerRow draws the status row of the footer and returns it as text.
func footerRow(t *testing.T, width int, family, pivot string) string {
	t.Helper()

	m := chromeModel(t, family, pivot)
	f := newFrame(width, 2, m.table)
	m.drawFooter(f, 0, width-1)

	return plain(f)[0]
}

// The same defect as the header, one row from the bottom: the filter and pivot
// names are drawn left to right and the status right to left. The recorded P3
// frame showed `p pivot envoy_wor10/553 series`, the pivot name running into
// the series count with the two sharing cells.
//
// As with the header, the invariant is that the right-hand side does not depend
// on the left, which a comparison states without pinning the status's layout.
func TestFooterNeverOverwritesTheStatusLine(t *testing.T) {
	t.Parallel()

	for _, width := range []int{60, 80, 90, 100, 120, 143} {
		short := []rune(footerRow(t, width, "up", ""))
		long := []rune(footerRow(t, width,
			"envoy_listener_worker_downstream_cx_total", "envoy_worker_id"))

		// The status ends two short of the right edge and is the only thing on
		// the row's right-hand half, so compare from the midpoint rightwards.
		mid := width / 2
		if got, want := string(long[mid:]), string(short[mid:]); got != want {
			t.Errorf("width %d: the status line changed when the pivot grew:\n got %q\nwant %q",
				width, got, want)
		}
	}
}

// The metric name is drawn left to right and the window strip right to left,
// and until this test they were drawn into the same cells without either
// knowing about the other. A long enough family name did not push the strip
// aside or get cut off - it was simply overwritten where the two met, and the
// surviving characters read as a plausible word.
//
// The recorded P3 frame showed `windowrk30si[ 1m ]`: fragments of
// envoy_worker_id with the marker and the 30s step written through them. The
// P7 axis bug was the same shape, a legible string that no code composed.
//
// The invariant is that the right-hand side of the header does not depend on
// the left. Comparing a long name against a short one says that without
// hard-coding the strip's layout, so this keeps holding when a window is added.
// The widths are swept rather than fixed because how much room the name has
// before it reaches the strip is exactly what varies, and the recordings turned
// out to be narrower than a first guess at the tape's pixel width suggested. A
// single width would have passed while the frames on disk showed the bug.
func TestHeaderNeverOverwritesTheWindowStrip(t *testing.T) {
	t.Parallel()

	for _, width := range []int{60, 80, 90, 100, 120, 143} {
		// Compared by column rather than by byte: the ellipsis and the marker
		// are both multi-byte, so a byte offset would slice through a rune and
		// report a difference that is not on screen.
		short := []rune(headerRow(t, width, "up", ""))
		long := []rune(headerRow(t, width,
			"envoy_listener_worker_downstream_cx_total", "envoy_worker_id"))

		marker := slices.Index(short, '⌁')
		if marker < 0 {
			t.Fatalf("width %d: no window marker in the header: %q", width, string(short))
		}

		// From the marker to the end: the strip and everything right of it.
		if got, want := string(long[marker:]), string(short[marker:]); got != want {
			t.Errorf("width %d: the window strip changed when the name grew:\n got %q\nwant %q",
				width, got, want)
		}
	}
}

// Bounding the header half stopped it corrupting the strip but not the second
// question it raised: which part gives way. Drawn in order, the family name
// takes every column it wants and the pivot key - four times shorter and the
// only thing on the row that says how the chart is split - is what disappears.
//
// The recorded frame that prompted this read `..._cx_total  /s  b… ⌁ window`.
func TestHeaderKeepsThePivotKeyWhenTheNameIsLong(t *testing.T) {
	t.Parallel()

	for _, width := range []int{80, 90, 100, 120, 143} {
		row := headerRow(t, width, "envoy_listener_worker_downstream_cx_total", "envoy_worker_id")

		if !strings.Contains(row, "by envoy_worker_id") {
			t.Errorf("width %d: the pivot key was squeezed out by the name: %q", width, row)
		}
	}
}

// Nothing to chart is not the same as nothing scraped. The recorded p3-empty
// frame said `waiting for first scra…` in the header while the body of the same
// frame said `nothing matches envoy_worker_id=~99` and the footer counted
// forty-three scrapes.
func TestHeaderDoesNotClaimToBeWaitingAfterAScrape(t *testing.T) {
	t.Parallel()

	store, err := series.NewStore(time.Minute, 2*time.Second, 0, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// A scrape that landed, carrying no samples.
	store.Append(time.Unix(1_700_000_000, 0), nil, series.Stats{Scrapes: 43})

	m := newModel(theme.Config{Mode: theme.ModeDark}, banner.Info{}, 0, store)
	m.snapshot = store.Snapshot()

	f := newFrame(143, 2, m.table)
	m.drawHeader(f, 142)

	if row := plain(f)[1]; strings.Contains(row, "waiting") {
		t.Errorf("the header still claimed to be waiting after 43 scrapes: %q", row)
	}
}

// Having established the strip survives, the name has to visibly give way
// rather than silently vanishing under it.
func TestHeaderTruncatesTheNameItCannotFit(t *testing.T) {
	t.Parallel()

	row := headerRow(t, 143, strings.Repeat("envoy_very_long_family_name_", 6), "envoy_worker_id")

	if !strings.Contains(row, "…") {
		t.Errorf("an over-long metric name was cut without an ellipsis to show it: %q", row)
	}
}
