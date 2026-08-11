// Package sample decides which series a family stores once it outgrows its
// budget.
package sample

// maxShift caps how aggressively a family can be thinned. At 1:1024 a chart is
// drawn from a thousandth of the data and there is nothing useful left to show;
// the cardinality figure is the answer at that point, not the chart.
const maxShift = 10

// Admission admits a stable, uniform subset of a family's series.
//
// This is deliberately not reservoir sampling. A reservoir re-rolls its
// membership as new items arrive, so a series would drop out of the chart and
// reappear between scrapes, and a line that flickers is worse than a line that
// was never drawn. Admission here is a pure function of the series identity: a
// series admitted at a given ratio stays admitted at that ratio forever, and
// tightening the ratio only ever removes series, never shuffles them.
type Admission struct {
	budget int
	shift  uint
}

// New returns an admission policy for a family budget. A budget of zero or less
// admits everything.
func New(budget int) *Admission {
	return &Admission{budget: budget}
}

// Budget returns the number of series the family may store.
func (a *Admission) Budget() int {
	return a.budget
}

// Admits reports whether a series identity is in the sample.
func (a *Admission) Admits(id uint64) bool {
	if a.budget <= 0 || a.shift == 0 {
		return true
	}

	return mix(id)&(1<<a.shift-1) == 0
}

// Ratio returns the denominator of the sampling ratio: 1 means everything.
func (a *Admission) Ratio() int {
	return 1 << a.shift
}

// Sampled reports whether anything is being dropped.
func (a *Admission) Sampled() bool {
	return a.shift > 0
}

// Tighten halves the sample and reports whether it changed.
func (a *Admission) Tighten() bool {
	if a.budget <= 0 || a.shift >= maxShift {
		return false
	}

	a.shift++

	return true
}

// mix is the SplitMix64 finaliser.
//
// Series identities are FNV hashes, and admission reads their low bits. FNV's
// low bits are adequate but not excellent, and a family whose labels differ only
// in a trailing digit is exactly the shape that finds the weakness. One round of
// mixing removes the question.
func mix(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}
