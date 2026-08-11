package sample_test

import (
	"math"
	"testing"

	"github.com/pantherhawk/prism/internal/sample"
)

func hash(i uint64) uint64 {
	value := i + 0x9e3779b97f4a7c15
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func TestAdmitsEverythingUntilTightened(t *testing.T) {
	t.Parallel()

	a := sample.New(100)

	for i := range uint64(1000) {
		if !a.Admits(hash(i)) {
			t.Fatalf("series %d was refused before any tightening", i)
		}
	}

	if a.Sampled() {
		t.Error("Sampled was true before tightening")
	}

	if got := a.Ratio(); got != 1 {
		t.Errorf("Ratio = %d, want 1", got)
	}
}

// TestMembershipIsStable is the reason this is not a reservoir. A series that
// drops out of the chart and comes back between scrapes is worse than one that
// was never drawn, so admission has to be a pure function of identity.
func TestMembershipIsStable(t *testing.T) {
	t.Parallel()

	a := sample.New(100)
	a.Tighten()
	a.Tighten()

	admitted := make([]bool, 500)
	for i := range admitted {
		admitted[i] = a.Admits(hash(uint64(i)))
	}

	for round := range 10 {
		for i := range admitted {
			if got := a.Admits(hash(uint64(i))); got != admitted[i] {
				t.Fatalf("series %d changed admission on round %d", i, round)
			}
		}
	}
}

// TestTighteningOnlyRemoves pins the other half of stability: narrowing the
// sample must never admit something it previously refused.
func TestTighteningOnlyRemoves(t *testing.T) {
	t.Parallel()

	a := sample.New(100)
	a.Tighten()

	before := make([]bool, 2000)
	for i := range before {
		before[i] = a.Admits(hash(uint64(i)))
	}

	a.Tighten()

	for i := range before {
		if a.Admits(hash(uint64(i))) && !before[i] {
			t.Fatalf("series %d was admitted only after the sample narrowed", i)
		}
	}
}

func TestRatioHalvesAndReportsItself(t *testing.T) {
	t.Parallel()

	a := sample.New(100)

	for want := 2; want <= 1024; want *= 2 {
		if !a.Tighten() {
			t.Fatalf("Tighten refused at ratio 1:%d", a.Ratio())
		}

		if got := a.Ratio(); got != want {
			t.Fatalf("Ratio = %d, want %d", got, want)
		}
	}

	if a.Tighten() {
		t.Error("Tighten kept going past the cap; a 1:2048 chart shows nothing")
	}
}

// TestSampleIsRoughlyUniform checks the mixing does its job: the kept fraction
// should track the ratio, not clump.
func TestSampleIsRoughlyUniform(t *testing.T) {
	t.Parallel()

	const (
		population = 20_000
		tolerance  = 0.15
	)

	a := sample.New(100)

	for ratio := 2; ratio <= 16; ratio *= 2 {
		a.Tighten()

		kept := 0

		for i := range uint64(population) {
			if a.Admits(hash(i)) {
				kept++
			}
		}

		expected := float64(population) / float64(ratio)
		if err := math.Abs(float64(kept)-expected) / expected; err > tolerance {
			t.Errorf("at 1:%d kept %d of %d, expected about %.0f (off by %.1f%%)",
				ratio, kept, population, expected, err*100)
		}
	}
}

func TestZeroBudgetAdmitsEverything(t *testing.T) {
	t.Parallel()

	a := sample.New(0)

	if a.Tighten() {
		t.Error("an unlimited family tightened")
	}

	for i := range uint64(1000) {
		if !a.Admits(hash(i)) {
			t.Fatalf("series %d refused by an unlimited family", i)
		}
	}
}
