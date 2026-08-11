package series

import (
	"math"
	"sort"
	"time"
)

// Snapshot is an immutable view of the store at one instant. The TUI renders
// only from these, which is why a slow frame can never stall a scrape and why
// a rendered frame is always internally consistent.
type Snapshot struct {
	At         time.Time
	Resolution time.Duration
	Newest     int64
	Capacity   int
	Series     []SeriesView
	Families   []FamilyView
	Stats      Stats
}

// SeriesView is one series' history within a snapshot.
type SeriesView struct {
	ID     ID
	Family string
	Labels Labels
	Kind   Kind
	Last   float64

	values []float64
	stamps []int64
}

// FamilyView is the cardinality history of one metric family.
type FamilyView struct {
	Name        string
	Cardinality int
	History     []float64

	// Stored is how many of the family's series are actually held. It differs
	// from Cardinality once the budget starts thinning.
	Stored int

	// Ratio is the sampling denominator: 1 means every series is stored.
	Ratio int

	// Estimated reports that Cardinality came from a sketch rather than a
	// count, so the UI can say so rather than implying a precision it does not
	// have.
	Estimated bool
}

// Sampled reports whether the family is being thinned.
func (f FamilyView) Sampled() bool {
	return f.Ratio > 1
}

// Empty reports whether the snapshot holds no observations yet.
func (s *Snapshot) Empty() bool {
	return len(s.Series) == 0
}

// Oldest returns the oldest bucket the snapshot can serve.
func (s *Snapshot) Oldest() int64 {
	return s.Newest - int64(s.Capacity) + 1
}

// BucketsIn returns how many buckets a duration spans, at least one.
func (s *Snapshot) BucketsIn(window time.Duration) int {
	if s.Resolution <= 0 {
		return 1
	}

	return max(int(window/s.Resolution), 1)
}

// At returns the value recorded in a bucket. The second result is false when
// that bucket holds nothing, which is not the same as holding zero: a gap in a
// scrape must render as a gap, not as a drop to the floor.
func (v SeriesView) At(bucket int64) (float64, bool) {
	if len(v.stamps) == 0 {
		return 0, false
	}

	slot := index(bucket, len(v.stamps))
	if v.stamps[slot] != bucket {
		return 0, false
	}

	return v.values[slot], true
}

// Window returns the values from oldest to newest inclusive, with NaN in place
// of any bucket that was never filled.
func (v SeriesView) Window(oldest, newest int64) []float64 {
	if newest < oldest {
		return nil
	}

	out := make([]float64, 0, newest-oldest+1)

	for bucket := oldest; bucket <= newest; bucket++ {
		value, ok := v.At(bucket)
		if !ok {
			value = math.NaN()
		}

		out = append(out, value)
	}

	return out
}

// snapshotLocked builds a snapshot. The caller must hold the store's mutex.
func (s *Store) snapshotLocked(at time.Time) *Snapshot {
	snapshot := &Snapshot{
		At:         at,
		Resolution: s.resolution,
		Newest:     s.newest,
		Capacity:   s.capacity,
		Series:     make([]SeriesView, 0, len(s.series)),
		Families:   make([]FamilyView, 0, len(s.families)),
		Stats:      s.stats,
	}

	for id, entry := range s.series {
		view := SeriesView{
			ID:     id,
			Family: entry.family,
			Labels: entry.labels,
			Kind:   entry.kind,
			values: append([]float64(nil), entry.values...),
			stamps: append([]int64(nil), entry.stamps...),
		}

		// Not-a-number is this package's word for "no value here", the same one
		// Window uses, and chart.Format draws it as a dash. A series whose
		// newest bucket is empty has no current rate, and leaving Last at zero
		// put `api 0` in the legend of the recorded P5 frame beside a chart
		// that correctly showed nothing at all - the reading the whole phase is
		// about, stated as a fact in the one place it is given as a number.
		view.Last = math.NaN()
		if last, ok := view.At(s.newest); ok {
			view.Last = last
		}

		snapshot.Series = append(snapshot.Series, view)
	}

	for name, entry := range s.families {
		snapshot.Families = append(snapshot.Families, FamilyView{
			Name:        name,
			Cardinality: entry.distinct.Count(),
			History:     entry.history(s.newest, s.capacity),
			Stored:      len(entry.stored),
			Ratio:       entry.admission.Ratio(),
			Estimated:   entry.distinct.Estimated(),
		})
	}

	// A stable order matters more than a clever one: rows that reshuffle
	// between frames are unreadable, and the selection would jump with them.
	sort.Slice(snapshot.Series, func(i, j int) bool {
		left, right := snapshot.Series[i], snapshot.Series[j]
		if left.Family != right.Family {
			return left.Family < right.Family
		}

		return left.Labels.String() < right.Labels.String()
	})

	sort.Slice(snapshot.Families, func(i, j int) bool {
		left, right := snapshot.Families[i], snapshot.Families[j]
		if left.Cardinality != right.Cardinality {
			return left.Cardinality > right.Cardinality
		}

		return left.Name < right.Name
	})

	return snapshot
}

// SamplingRatio returns the coarsest sampling in force across every family, so
// the status bar can report in one number whether anything on screen is a
// subset.
func (s *Snapshot) SamplingRatio() int {
	worst := 1

	for _, family := range s.Families {
		worst = max(worst, family.Ratio)
	}

	return worst
}

// history returns the cardinality of a family over the whole ring, oldest
// first, with zero for buckets never filled.
func (f *familyState) history(newest int64, capacity int) []float64 {
	out := make([]float64, 0, capacity)

	for bucket := newest - int64(capacity) + 1; bucket <= newest; bucket++ {
		slot := index(bucket, capacity)
		if f.stamps[slot] != bucket {
			out = append(out, 0)

			continue
		}

		out = append(out, float64(f.counts[slot]))
	}

	return out
}
