// Package timeline owns which slice of history the chart is showing.
//
// It is deliberately separate from the TUI. Zoom, pan, clamping and the
// live/detached distinction are the fiddliest logic in prism and the easiest
// to get subtly wrong at the edges, so they live behind an interface that can
// be driven from a test without a terminal.
package timeline

import (
	"math"
	"time"

	"github.com/pantherhawk/prism/internal/motion"
)

// Spring tuning.
//
// These numbers were chosen by integrating the springs and reading off the
// settling time and the overshoot, not by feel. At 60fps:
//
//	window  14 / 0.80  settles in ~0.7s, undershoots the target width by ~2%
//	pan     18 / 1.00  settles in ~0.7s, critically damped, no overshoot
//
// The window is slightly underdamped because it is a control the operator is
// driving directly, and a small bounce reads as mass. An earlier 6.0 / 0.58
// took 1.9 seconds to settle and undershot by 16%, which showed a visibly
// narrower window than the one that had been asked for.
//
// Panning is critically damped. Overshoot there would scroll past the buckets
// the operator asked for and then walk back, which reads as a glitch.
const (
	windowFrequency = 14.0
	windowDamping   = 0.80

	panFrequency = 18.0
	panDamping   = 1.00
)

// panFraction is how much of the visible window one pan keystroke moves.
const panFraction = 4

// Timeline tracks the width of the visible window and how far it sits behind
// the newest bucket.
type Timeline struct {
	steps      []time.Duration
	index      int
	offset     int
	resolution time.Duration
	capacity   int

	window *motion.Spring
	pan    *motion.Spring
	group  *motion.Group
}

// New returns a timeline over the given window steps, opening at index.
func New(steps []time.Duration, index int, resolution time.Duration, capacity int) *Timeline {
	window := motion.NewSpring(windowFrequency, windowDamping)
	pan := motion.NewSpring(panFrequency, panDamping)

	t := &Timeline{
		steps:      steps,
		index:      min(max(index, 0), len(steps)-1),
		resolution: resolution,
		capacity:   capacity,
		window:     window,
		pan:        pan,
		group:      motion.NewGroup(window, pan),
	}

	// The window is animated in log space. Each step is roughly a constant
	// factor apart, so a linear interpolation from 1h to 30s spends almost all
	// of its time in the first few seconds and then snaps. In log space every
	// step of the zoom feels the same size, which is how zoom should behave.
	window.Snap(math.Log(t.Window().Seconds()))
	pan.Snap(0)

	return t
}

// Window returns the width the operator selected.
func (t *Timeline) Window() time.Duration {
	return t.steps[t.index]
}

// AnimatedWindow returns the width to draw this frame.
func (t *Timeline) AnimatedWindow() time.Duration {
	return time.Duration(math.Exp(t.window.Value()) * float64(time.Second))
}

// Offset returns the settled distance behind live, in buckets.
func (t *Timeline) Offset() int {
	return t.offset
}

// Live reports whether the window is pinned to the newest bucket.
func (t *Timeline) Live() bool {
	return t.offset == 0
}

// Buckets returns how many buckets the animated window spans.
//
// The count is rounded, not truncated. The zoom spring interpolates in log
// space, so a settled 5m window comes back through exp as 299.999999999s, and
// truncating that division draws 19 buckets of a 20 bucket window - one column
// of the chart quietly missing at every rest position.
func (t *Timeline) Buckets() int {
	if t.resolution <= 0 {
		return 1
	}

	spanned := int(math.Round(float64(t.AnimatedWindow()) / float64(t.resolution)))

	return min(max(spanned, 1), t.capacity)
}

// Range returns the inclusive bucket range to draw, given the newest bucket
// the store holds.
func (t *Timeline) Range(newest int64) (oldest, latest int64) {
	latest = newest - int64(math.Round(t.pan.Value()))
	oldest = latest - int64(t.Buckets()) + 1

	return oldest, latest
}

// ZoomIn narrows the window by one step.
func (t *Timeline) ZoomIn() {
	t.setIndex(t.index - 1)
}

// ZoomOut widens the window by one step, clamped to the history the store
// actually holds. Showing an hour of a fifteen minute buffer would be forty
// five minutes of blank chart pretending to be quiet.
func (t *Timeline) ZoomOut() {
	t.setIndex(t.index + 1)
}

// setIndex moves to a window step and re-aims the springs.
func (t *Timeline) setIndex(index int) {
	index = min(max(index, 0), len(t.steps)-1)

	for index > 0 && t.steps[index] > t.horizon() {
		index--
	}

	t.index = index
	t.window.SetTarget(math.Log(t.Window().Seconds()))
	t.clampPan()
}

// horizon is the widest window the buffer can fill.
func (t *Timeline) horizon() time.Duration {
	return t.resolution * time.Duration(t.capacity)
}

// AtHorizon reports whether the window is as wide as the buffer allows, so the
// UI can say why zooming out stopped doing anything.
func (t *Timeline) AtHorizon() bool {
	return t.index == len(t.steps)-1 || t.steps[t.index+1] > t.horizon()
}

// PanLeft moves the window back in time.
func (t *Timeline) PanLeft() {
	t.setOffset(t.offset + max(t.Buckets()/panFraction, 1))
}

// PanRight moves the window forward, reattaching to live when it arrives.
func (t *Timeline) PanRight() {
	t.setOffset(t.offset - max(t.Buckets()/panFraction, 1))
}

// ToLive reattaches the window to the newest bucket.
func (t *Timeline) ToLive() {
	t.setOffset(0)
}

// ToOldest moves the window to the start of the buffer.
func (t *Timeline) ToOldest() {
	t.setOffset(t.capacity)
}

// setOffset aims the pan spring, clamped to the buffer.
func (t *Timeline) setOffset(offset int) {
	t.offset = min(max(offset, 0), t.maxOffset())
	t.pan.SetTarget(float64(t.offset))
}

// clampPan re-applies the clamp after a zoom, because widening the window
// eats into how far back it can sit.
func (t *Timeline) clampPan() {
	t.setOffset(t.offset)
}

// maxOffset is the furthest back the window can go without running off the
// end of the buffer.
//
// It measures against the target width, not the animated one. Clamping to a
// width that is still moving would let the limit drift for the duration of a
// zoom, and a pan issued mid-zoom would land somewhere the operator did not ask
// for.
func (t *Timeline) maxOffset() int {
	return max(t.capacity-t.targetBuckets(), 0)
}

// targetBuckets is how many buckets the selected window spans once settled.
func (t *Timeline) targetBuckets() int {
	if t.resolution <= 0 {
		return 1
	}

	return min(max(int(t.Window()/t.resolution), 1), t.capacity)
}

// Resize updates the buffer geometry when the store's shape is known.
func (t *Timeline) Resize(resolution time.Duration, capacity int) {
	if resolution <= 0 || capacity <= 0 {
		return
	}

	t.resolution, t.capacity = resolution, capacity
	t.setIndex(t.index)
}

// Update advances the springs and reports whether anything is still moving.
func (t *Timeline) Update() bool {
	return t.group.Update()
}

// Moving reports whether the timeline is mid-animation.
func (t *Timeline) Moving() bool {
	return t.group.Moving()
}

// Index returns the position of the selected window in the step list, so the
// UI can show which one is active.
func (t *Timeline) Index() int {
	return t.index
}

// Steps returns the selectable window widths.
func (t *Timeline) Steps() []time.Duration {
	return t.steps
}

// Behind returns how far the right edge of the window sits behind live.
func (t *Timeline) Behind() time.Duration {
	return time.Duration(t.offset) * t.resolution
}
