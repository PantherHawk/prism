//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/chart"
	"github.com/pantherhawk/prism/internal/timeline"
)

// settleLimit bounds how many frames a spring is given to arrive before the
// scenario gives up on it.
const settleLimit = 2000

// windowSteps mirrors the widths the TUI offers.
var windowSteps = []time.Duration{ //nolint:gochecknoglobals // immutable fixture
	30 * time.Second,
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
}

// windowWorld is the state of one navigation scenario.
type windowWorld struct {
	timeline *timeline.Timeline
	capacity int
	drawn    time.Duration
	gapped   []string
	whole    []string
}

type windowKey struct{}

func windowFrom(ctx context.Context) *windowWorld {
	w, ok := ctx.Value(windowKey{}).(*windowWorld)
	if !ok {
		panic("window world missing from context")
	}

	return w
}

// press applies a key the number of times given.
func (w *windowWorld) press(name string, times int) error {
	for range times {
		switch name {
		case "j":
			w.timeline.ZoomIn()
		case "k":
			w.timeline.ZoomOut()
		case "h":
			w.timeline.PanLeft()
		case "l":
			w.timeline.PanRight()
		case "L":
			w.timeline.ToLive()
		case "g":
			w.timeline.ToOldest()
		default:
			return fmt.Errorf("%w: %q", errUnknownKey, name)
		}
	}

	// One frame of motion, so that a scenario can assert on what the operator
	// sees immediately after the keystroke rather than only on where it lands.
	w.timeline.Update()
	w.drawn = w.timeline.AnimatedWindow()

	return nil
}

// settle advances the springs until they stop.
func (w *windowWorld) settle() error {
	for range settleLimit {
		if !w.timeline.Update() {
			return nil
		}
	}

	return errNeverSettled
}

// ---- Given -----------------------------------------------------------------

func (w *windowWorld) aBuffer(ctx context.Context, retention, resolution string) (context.Context, error) {
	held, err := time.ParseDuration(retention)
	if err != nil {
		return ctx, fmt.Errorf("parse retention: %w", err)
	}

	width, err := time.ParseDuration(resolution)
	if err != nil {
		return ctx, fmt.Errorf("parse resolution: %w", err)
	}

	w.capacity = int(held / width)
	w.timeline = timeline.New(windowSteps, 2, width, w.capacity)

	return ctx, nil
}

func (w *windowWorld) theWindowIsWideAndLive(ctx context.Context, width string) (context.Context, error) {
	want, err := time.ParseDuration(width)
	if err != nil {
		return ctx, fmt.Errorf("parse width: %w", err)
	}

	if w.timeline.Window() != want {
		return ctx, fmt.Errorf("window = %v, want %v", w.timeline.Window(), want)
	}

	if !w.timeline.Live() {
		return ctx, errNotLive
	}

	return ctx, nil
}

func (w *windowWorld) theWindowIsDetached(ctx context.Context) (context.Context, error) {
	if err := w.press("h", 3); err != nil {
		return ctx, err
	}

	if w.timeline.Live() {
		return ctx, errStillLive
	}

	return ctx, nil
}

func (w *windowWorld) aSeriesWithAGap(ctx context.Context) (context.Context, error) {
	gapped := []float64{1, 2, 3, math.NaN(), math.NaN(), 3, 2, 1}
	whole := []float64{1, 2, 3, 3, 3, 3, 2, 1}

	w.gapped = draw(gapped)
	w.whole = draw(whole)

	return ctx, nil
}

// draw renders a series to plain text, discarding colour.
func draw(values []float64) []string {
	canvas := chart.New(16, 4)
	canvas.Plot(0, values, 0, 4)

	rows := canvas.Rows()
	out := make([]string, 0, len(rows))

	for _, row := range rows {
		var line strings.Builder
		for _, run := range row {
			line.WriteString(run.Text)
		}

		out = append(out, line.String())
	}

	return out
}

// ---- When ------------------------------------------------------------------

func (w *windowWorld) iPress(ctx context.Context, name string) (context.Context, error) {
	return ctx, w.press(name, 1)
}

func (w *windowWorld) iPressTimes(ctx context.Context, name string, times int) (context.Context, error) {
	return ctx, w.press(name, times)
}

func (w *windowWorld) theChartIsDrawn(ctx context.Context) (context.Context, error) {
	if len(w.gapped) == 0 {
		return ctx, errNothingDrawn
	}

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *windowWorld) theTargetWindowIs(ctx context.Context, want string) (context.Context, error) {
	target, err := time.ParseDuration(want)
	if err != nil {
		return ctx, fmt.Errorf("parse window: %w", err)
	}

	if got := w.timeline.Window(); got != target {
		return ctx, fmt.Errorf("target window = %v, want %v", got, target)
	}

	return ctx, nil
}

func (w *windowWorld) theDrawnWindowIsBetween(ctx context.Context, lower, upper string) (context.Context, error) {
	low, err := time.ParseDuration(lower)
	if err != nil {
		return ctx, fmt.Errorf("parse lower: %w", err)
	}

	high, err := time.ParseDuration(upper)
	if err != nil {
		return ctx, fmt.Errorf("parse upper: %w", err)
	}

	if w.drawn <= low || w.drawn >= high {
		return ctx, fmt.Errorf(
			"drawn window %v is not between %v and %v; the zoom snapped", w.drawn, low, high)
	}

	return ctx, nil
}

func (w *windowWorld) itSettlesAt(ctx context.Context, want string) (context.Context, error) {
	target, err := time.ParseDuration(want)
	if err != nil {
		return ctx, fmt.Errorf("parse window: %w", err)
	}

	if err := w.settle(); err != nil {
		return ctx, err
	}

	if drift := w.timeline.AnimatedWindow() - target; drift > time.Second || drift < -time.Second {
		return ctx, fmt.Errorf("settled at %v, want %v", w.timeline.AnimatedWindow(), target)
	}

	return ctx, nil
}

func (w *windowWorld) theBufferHorizonIsReported(ctx context.Context) (context.Context, error) {
	if !w.timeline.AtHorizon() {
		return ctx, errNoHorizon
	}

	return ctx, nil
}

func (w *windowWorld) theWindowIsDetachedFromLive(ctx context.Context) (context.Context, error) {
	if w.timeline.Live() {
		return ctx, errStillLive
	}

	return ctx, nil
}

func (w *windowWorld) theWindowSitsBehindLive(ctx context.Context) (context.Context, error) {
	if w.timeline.Behind() <= 0 {
		return ctx, errNotBehind
	}

	return ctx, nil
}

func (w *windowWorld) theWindowIsAttachedToLive(ctx context.Context) (context.Context, error) {
	if !w.timeline.Live() {
		return ctx, errNotLive
	}

	return ctx, nil
}

func (w *windowWorld) theWindowCoversBuckets(ctx context.Context, want int) (context.Context, error) {
	if err := w.settle(); err != nil {
		return ctx, err
	}

	oldest, latest := w.timeline.Range(1000)
	if got := int(latest - oldest + 1); got != want {
		return ctx, fmt.Errorf("window covered %d buckets, want %d", got, want)
	}

	return ctx, nil
}

func (w *windowWorld) theWindowStaysInsideTheBuffer(ctx context.Context) (context.Context, error) {
	const newest = 1000

	oldest, latest := w.timeline.Range(newest)

	if oldest < newest-int64(w.capacity) {
		return ctx, fmt.Errorf("oldest bucket %d is before the buffer start", oldest)
	}

	if latest > newest {
		return ctx, fmt.Errorf("latest bucket %d is after the newest held", latest)
	}

	return ctx, nil
}

func (w *windowWorld) theLineBreaksAcrossTheGap(ctx context.Context) (context.Context, error) {
	if strings.Join(w.gapped, "") == strings.Join(w.whole, "") {
		return ctx, errInterpolated
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

var (
	errUnknownKey   = errors.New("unknown key")
	errNeverSettled = errors.New("springs never settled")
	errNotLive      = errors.New("window is not attached to live")
	errStillLive    = errors.New("window is still attached to live")
	errNotBehind    = errors.New("window reports no distance behind live")
	errNoHorizon    = errors.New("buffer horizon was not reported")
	errNothingDrawn = errors.New("nothing was drawn")
	errInterpolated = errors.New("the gap was interpolated across")
)

// initializeWindow registers the navigation steps.
func initializeWindow(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, windowKey{}, &windowWorld{}), nil
	})

	sc.Step(`^a buffer holding ([0-9a-z ]+?) at ([0-9a-z ]+?) resolution$`,
		func(ctx context.Context, retention, resolution string) (context.Context, error) {
			return windowFrom(ctx).aBuffer(ctx, normalise(retention), normalise(resolution))
		})
	sc.Step(`^the window is ([0-9a-z ]+?) wide and attached to live$`,
		func(ctx context.Context, width string) (context.Context, error) {
			return windowFrom(ctx).theWindowIsWideAndLive(ctx, normalise(width))
		})
	sc.Step(`^the window is detached from live$`, func(ctx context.Context) (context.Context, error) {
		w := windowFrom(ctx)
		if w.timeline.Live() {
			return w.theWindowIsDetached(ctx)
		}

		return w.theWindowIsDetachedFromLive(ctx)
	})
	sc.Step(`^a series with a gap in the middle$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).aSeriesWithAGap(ctx)
	})

	sc.Step(`^I press "([^"]*)"$`, func(ctx context.Context, name string) (context.Context, error) {
		return windowFrom(ctx).iPress(ctx, name)
	})
	sc.Step(`^I press "([^"]*)" (\d+) times$`,
		func(ctx context.Context, name string, times int) (context.Context, error) {
			return windowFrom(ctx).iPressTimes(ctx, name, times)
		})
	sc.Step(`^the chart is drawn$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theChartIsDrawn(ctx)
	})

	sc.Step(`^the target window is ([0-9a-z]+)$`, func(ctx context.Context, w string) (context.Context, error) {
		return windowFrom(ctx).theTargetWindowIs(ctx, w)
	})
	sc.Step(`^the drawn window is between ([0-9a-z]+) and ([0-9a-z]+)$`,
		func(ctx context.Context, lower, upper string) (context.Context, error) {
			return windowFrom(ctx).theDrawnWindowIsBetween(ctx, lower, upper)
		})
	sc.Step(`^it settles at ([0-9a-z]+)$`, func(ctx context.Context, w string) (context.Context, error) {
		return windowFrom(ctx).itSettlesAt(ctx, w)
	})
	sc.Step(`^the buffer horizon is reported$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theBufferHorizonIsReported(ctx)
	})
	sc.Step(`^the window sits behind live$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theWindowSitsBehindLive(ctx)
	})
	sc.Step(`^the window is attached to live$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theWindowIsAttachedToLive(ctx)
	})
	sc.Step(`^the window covers (\d+) buckets$`, func(ctx context.Context, n int) (context.Context, error) {
		return windowFrom(ctx).theWindowCoversBuckets(ctx, n)
	})
	sc.Step(`^the window stays inside the buffer$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theWindowStaysInsideTheBuffer(ctx)
	})
	sc.Step(`^the line breaks across the gap$`, func(ctx context.Context) (context.Context, error) {
		return windowFrom(ctx).theLineBreaksAcrossTheGap(ctx)
	})
}

// normalise turns the prose the feature file uses into a duration string.
func normalise(width string) string {
	replacer := strings.NewReplacer(" minutes", "m", " minute", "m", " seconds", "s", " second", "s")

	return strings.TrimSpace(replacer.Replace(width))
}
