package timeline_test

import (
	"math"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/timeline"
)

// steps mirrors the window widths the TUI offers.
var steps = []time.Duration{
	30 * time.Second,
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
}

const (
	resolution = 15 * time.Second
	capacity   = 60 // 15 minutes of history
	openAt     = 2  // 5m
)

func newTimeline() *timeline.Timeline {
	return timeline.New(steps, openAt, resolution, capacity)
}

// settle runs the springs until they stop, or fails if they never do.
func settle(t *testing.T, tl *timeline.Timeline) {
	t.Helper()

	const limit = 2000

	for range limit {
		if !tl.Update() {
			return
		}
	}

	t.Fatal("springs never settled")
}

func TestOpensLiveAtTheSelectedWindow(t *testing.T) {
	t.Parallel()

	tl := newTimeline()

	if !tl.Live() {
		t.Error("timeline did not open attached to live")
	}

	if got := tl.Window(); got != 5*time.Minute {
		t.Errorf("window = %v, want 5m", got)
	}

	// The opening frame is a fact, not a transition: nothing should be moving.
	if tl.Moving() {
		t.Error("timeline animated on the first frame")
	}
}

func TestZoomAnimatesRatherThanSnapping(t *testing.T) {
	t.Parallel()

	tl := newTimeline()
	tl.ZoomIn()

	if got := tl.Window(); got != time.Minute {
		t.Fatalf("target window = %v, want 1m", got)
	}

	// One frame in, the drawn width must still be between the two steps.
	tl.Update()

	drawn := tl.AnimatedWindow()
	if drawn <= time.Minute || drawn >= 5*time.Minute {
		t.Errorf("drawn window jumped to %v; it should be mid-transition", drawn)
	}

	settle(t, tl)

	if got := tl.AnimatedWindow(); absDuration(got-time.Minute) > time.Second {
		t.Errorf("settled at %v, want 1m", got)
	}
}

// TestZoomIsGeometric pins the log-space interpolation.
//
// Zooming 1h -> 15m, the geometric mean of the endpoints is 30m and the
// arithmetic mean is 37.5m. Eight frames in, integrating the real spring puts
// a log-space zoom at roughly 28m and a linear one at roughly 34m. Asserting
// that the drawn window is already past the geometric mean at that point
// therefore fails if anyone swaps the interpolation back to linear. The frame
// count is measured, not guessed.
func TestZoomIsGeometric(t *testing.T) {
	t.Parallel()

	const (
		frames    = 8
		geometric = 30 * time.Minute
	)

	tl := timeline.New(steps, len(steps)-1, resolution, 240)
	tl.ZoomIn() // 1h -> 15m

	for range frames {
		tl.Update()
	}

	if got := tl.AnimatedWindow(); got >= geometric {
		t.Errorf(
			"after %d frames the window was %v, still wider than the geometric mean %v; "+
				"the zoom is interpolating linearly",
			frames, got, geometric)
	}
}

func TestZoomOutClampsAtTheBufferHorizon(t *testing.T) {
	t.Parallel()

	tl := newTimeline()

	for range 10 {
		tl.ZoomOut()
	}

	// The buffer holds 15 minutes, so the hour step must be unreachable.
	if got := tl.Window(); got != 15*time.Minute {
		t.Errorf("window = %v, want the 15m horizon", got)
	}

	if !tl.AtHorizon() {
		t.Error("AtHorizon was false at the widest usable window")
	}
}

func TestPanningDetachesFromLive(t *testing.T) {
	t.Parallel()

	tl := newTimeline()
	tl.PanLeft()

	if tl.Live() {
		t.Error("panning back left the window attached to live")
	}

	if tl.Behind() <= 0 {
		t.Error("Behind reported no distance after panning back")
	}
}

func TestPanningForwardReattaches(t *testing.T) {
	t.Parallel()

	tl := newTimeline()

	for range 10 {
		tl.PanLeft()
	}

	for range 20 {
		tl.PanRight()
	}

	if !tl.Live() {
		t.Error("panning forward past live did not reattach")
	}

	if got := tl.Offset(); got != 0 {
		t.Errorf("offset = %d, want 0", got)
	}
}

func TestPanClampsAtTheStartOfTheBuffer(t *testing.T) {
	t.Parallel()

	tl := newTimeline()
	tl.ToOldest()
	settle(t, tl)

	oldest, latest := tl.Range(1000)

	if oldest < 1000-capacity {
		t.Errorf("window ran off the start of the buffer: oldest = %d", oldest)
	}

	if latest > 1000 {
		t.Errorf("window ran past the newest bucket: latest = %d", latest)
	}
}

func TestRangeCoversTheWindow(t *testing.T) {
	t.Parallel()

	tl := newTimeline()
	settle(t, tl)

	oldest, latest := tl.Range(1000)

	if got, want := latest-oldest+1, int64(20); got != want {
		t.Errorf("range covered %d buckets, want %d for a 5m window at 15s", got, want)
	}
}

func absDuration(d time.Duration) time.Duration {
	return time.Duration(math.Abs(float64(d)))
}
