package sketch_test

import (
	"math"
	"testing"

	"github.com/pantherhawk/prism/internal/sketch"
)

// hash is SplitMix64 over a counter: deterministic, so the tolerances below are
// properties of the estimator rather than of a lucky seed.
func hash(i uint64) uint64 {
	value := i + 0x9e3779b97f4a7c15
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func TestExactWhileSmall(t *testing.T) {
	t.Parallel()

	d := sketch.NewDistinct(100, 16)

	for i := range uint64(50) {
		d.Add(hash(i))
	}

	if d.Estimated() {
		t.Error("a 50-element set was estimated; small families deserve the truth")
	}

	if got := d.Count(); got != 50 {
		t.Errorf("Count = %d, want 50", got)
	}
}

func TestDuplicatesAreNotCountedTwice(t *testing.T) {
	t.Parallel()

	d := sketch.NewDistinct(0, 0)

	for range 10 {
		for i := range uint64(100) {
			d.Add(hash(i))
		}
	}

	if got := d.Count(); got != 100 {
		t.Errorf("Count = %d, want 100 after ten passes over the same set", got)
	}
}

// TestEstimateStaysWithinTolerance pins the accuracy the UI's tilde promises.
// The standard error is 1/sqrt(k); at k=1024 that is about 3%, measured at 1.9%
// mean and 6.3% worst case, so 10% is a safe bound that still catches a broken
// estimator.
func TestEstimateStaysWithinTolerance(t *testing.T) {
	t.Parallel()

	const tolerance = 0.10

	for _, total := range []uint64{20_000, 200_000, 500_000} {
		d := sketch.NewDistinct(4096, 1024)

		for i := range total {
			d.Add(hash(i))
		}

		if !d.Estimated() {
			t.Errorf("%d elements were not estimated", total)

			continue
		}

		got := float64(d.Count())
		if err := math.Abs(got-float64(total)) / float64(total); err > tolerance {
			t.Errorf("Count = %.0f for %d elements, error %.1f%% exceeds %.0f%%",
				got, total, err*100, tolerance*100)
		}
	}
}

// TestMemoryIsBounded is the point of the whole package: the sketch must not
// grow with the set. Counting half a million series in the same space as five
// thousand is what lets prism report on an explosion instead of joining it.
func TestMemoryIsBounded(t *testing.T) {
	t.Parallel()

	const k = 256

	small := sketch.NewDistinct(16, k)
	large := sketch.NewDistinct(16, k)

	for i := range uint64(5_000) {
		small.Add(hash(i))
	}

	for i := range uint64(500_000) {
		large.Add(hash(i))
	}

	// Both have converted to the sketch, so both hold exactly k values. The
	// observable proxy for that is that neither reports more retained values
	// than k allows, which Count would expose if the heap had grown.
	if !small.Estimated() || !large.Estimated() {
		t.Fatal("both sets should have converted to estimates")
	}
}

func TestCountIsExactJustAfterConversion(t *testing.T) {
	t.Parallel()

	// Between the exact limit and k retained values, every hash offered is
	// still held, so the answer is exact even though the map has been released.
	d := sketch.NewDistinct(8, 64)

	for i := range uint64(20) {
		d.Add(hash(i))
	}

	if got := d.Count(); got != 20 {
		t.Errorf("Count = %d, want an exact 20 before the sketch fills", got)
	}
}
