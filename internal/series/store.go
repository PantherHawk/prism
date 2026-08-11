package series

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pantherhawk/prism/internal/sample"
	"github.com/pantherhawk/prism/internal/sketch"
)

var (
	// ErrInvalidRetention is returned when a store is asked to keep no history.
	ErrInvalidRetention = errors.New("retention must be positive")
	// ErrInvalidResolution is returned when the bucket width is not positive
	// or does not divide the retention.
	ErrInvalidResolution = errors.New("resolution must be positive and no larger than retention")
)

// state is the ring for one series.
//
// A slot holds a value only if its stamp matches the bucket index that maps to
// it. That removes the need to clear slots as the window advances: a stale slot
// simply fails the stamp check, which turns bucket expiry from O(series) work
// per tick into no work at all.
type state struct {
	family string
	labels Labels
	kind   Kind

	values []float64
	stamps []int64

	lastRaw    float64
	lastAt     time.Time
	hasLastRaw bool
}

// familyState tracks one metric family: how many distinct series it has, which
// of them are stored, and the history of that count.
//
// Cardinality is counted with a sketch rather than a set. Holding one entry per
// series in order to report how many series there are is the same cost prism is
// warning about, and a tool that dies measuring an explosion is no help during
// one.
type familyState struct {
	distinct  *sketch.Distinct
	admission *sample.Admission
	stored    map[ID]struct{}
	counts    []int
	stamps    []int64
}

// Stats describes the health of whatever is feeding the store.
type Stats struct {
	Scrapes     int64
	Errors      int64
	LastError   string
	LastLatency time.Duration

	// Source names where the samples came from: a scrape endpoint, or the
	// address of an upstream prism.
	Source string

	// Reconnecting reports that the link to an upstream is down and being
	// retried. RetryIn is how long until the next attempt.
	Reconnecting bool
	RetryIn      time.Duration
}

// Store holds the recent history of every observed series. It is safe for
// concurrent use: the collector writes under a mutex, and the TUI reads a
// snapshot published atomically, so a slow render can never block a scrape.
type Store struct {
	mu         sync.Mutex
	resolution time.Duration
	capacity   int
	newest     int64
	series     map[ID]*state
	families   map[string]*familyState
	stats      Stats

	budget int

	published atomic.Pointer[Snapshot]
	log       *slog.Logger
}

// NewStore returns a store retaining at least the given duration of samples at
// the given bucket width, storing at most budget series per family.
func NewStore(
	retention, resolution time.Duration,
	budget int,
	log *slog.Logger,
) (*Store, error) {
	if retention <= 0 {
		return nil, ErrInvalidRetention
	}

	if resolution <= 0 || resolution > retention {
		return nil, ErrInvalidResolution
	}

	store := &Store{
		resolution: resolution,
		capacity:   int(retention / resolution),
		newest:     math.MinInt64,
		series:     make(map[ID]*state),
		families:   make(map[string]*familyState),
		budget:     budget,
		log:        log,
	}

	store.published.Store(&Snapshot{Resolution: resolution, Capacity: store.capacity})

	return store, nil
}

// Resolution reports the width of one bucket.
func (s *Store) Resolution() time.Duration {
	return s.resolution
}

// Retention reports how much history the store keeps.
func (s *Store) Retention() time.Duration {
	return s.resolution * time.Duration(s.capacity)
}

// bucketOf returns the bucket index a timestamp falls in.
func (s *Store) bucketOf(at time.Time) int64 {
	return at.UnixNano() / int64(s.resolution)
}

// Append records one scrape's worth of samples and publishes a new snapshot.
// It takes the scrape timestamp rather than reading the clock so that the
// result is a function of its inputs and the acceptance criteria can pin it.
func (s *Store) Append(at time.Time, samples []Sample, stats Stats) {
	bucket := s.bucketOf(at)

	s.mu.Lock()

	if bucket > s.newest {
		s.newest = bucket
	}

	for _, sample := range samples {
		s.record(bucket, at, sample)
	}

	s.stats = stats
	snapshot := s.snapshotLocked(at)

	s.mu.Unlock()

	s.published.Store(snapshot)
}

// record folds one sample into its series' ring.
//
// Cardinality is counted for every sample, including the ones the family's
// budget refuses to store. Counting and storing are separate concerns: the
// number an operator needs during an explosion is how many series there are,
// which stays exact long after storing them all became unaffordable.
func (s *Store) record(bucket int64, at time.Time, observation Sample) {
	id := Fingerprint(observation.Family, observation.Labels)
	family := s.familyFor(observation.Family)

	family.distinct.Add(uint64(id))
	family.counts[index(bucket, s.capacity)] = family.distinct.Count()
	family.stamps[index(bucket, s.capacity)] = bucket

	if !family.admission.Admits(uint64(id)) {
		// It may have been admitted under a looser ratio. Dropping it here is
		// what keeps memory flat as cardinality climbs.
		s.forget(family, id)

		return
	}

	entry, ok := s.series[id]
	if !ok {
		entry = &state{
			family: observation.Family,
			labels: observation.Labels,
			kind:   observation.Kind,
			values: make([]float64, s.capacity),
			stamps: newStamps(s.capacity),
		}
		s.series[id] = entry
		family.stored[id] = struct{}{}
	}

	// A reading that yielded no rate leaves the bucket untouched. Stamping it
	// is what marks a bucket as filled, so not stamping it is how the gap gets
	// through to the chart - the same path an unscraped bucket takes.
	if value, ok := entry.observe(at, observation.Value); ok {
		entry.values[index(bucket, s.capacity)] = value
		entry.stamps[index(bucket, s.capacity)] = bucket
	}

	s.enforceBudget(family)
}

// familyFor returns the family's state, creating it on first sight.
func (s *Store) familyFor(name string) *familyState {
	entry, ok := s.families[name]
	if ok {
		return entry
	}

	entry = &familyState{
		distinct:  sketch.NewDistinct(0, 0),
		admission: sample.New(s.budget),
		stored:    make(map[ID]struct{}),
		counts:    make([]int, s.capacity),
		stamps:    newStamps(s.capacity),
	}
	s.families[name] = entry

	return entry
}

// forget removes a series from the store entirely.
func (s *Store) forget(family *familyState, id ID) {
	if _, ok := family.stored[id]; !ok {
		return
	}

	delete(family.stored, id)
	delete(s.series, id)
}

// enforceBudget thins a family until it fits, halving the sample each round.
//
// Halving gives ratios of 1:2, 1:4, 1:8 - the numbers reported in the status
// bar. They are coarse on purpose: a continuously adjusted ratio would change
// which series are on screen every scrape.
func (s *Store) enforceBudget(family *familyState) {
	if s.budget <= 0 {
		return
	}

	for len(family.stored) > s.budget {
		if !family.admission.Tighten() {
			s.evictToBudget(family)

			return
		}

		for id := range family.stored {
			if !family.admission.Admits(uint64(id)) {
				delete(family.stored, id)
				delete(s.series, id)
			}
		}
	}
}

// evictToBudget drops the excess once the ratio can tighten no further.
//
// Halving lands near a budget, never on it: a hundred thousand series at the
// 1:1024 floor leave about ninety-eight, but the deviation is around ten either
// way, so roughly half of all families would sit over their budget with nothing
// left to do about it. A budget that the store may exceed by whatever the
// arithmetic leaves over is not protecting the thing it exists to protect.
//
// The series kept are those with the lowest identities. That has to be a
// function of identity and nothing else - dropping whichever arrived last would
// reintroduce the truncation P4 removed, where what you see depends on the order
// the target happened to write its output in. Because identities are fixed, the
// survivors are the same set whatever order the samples arrive in, and a series
// evicted here stays evicted while the family stays this wide.
func (s *Store) evictToBudget(family *familyState) {
	if len(family.stored) <= s.budget {
		return
	}

	ids := make([]ID, 0, len(family.stored))
	for id := range family.stored {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids[s.budget:] {
		delete(family.stored, id)
		delete(s.series, id)
	}
}

// observe converts a raw reading into the value the chart shows. Counters are
// reported as a per-second rate. The second result is false when this reading
// produced no rate at all, which the caller must record as an empty bucket
// rather than as a zero.
//
// That distinction is the whole point of the second result. A rate of zero is
// a statement that nothing happened, and at the rates a busy target reports it
// draws an outage. Two readings are needed before there is a rate to draw, and
// until the second one arrives the honest answer is silence:
//
//   - the first reading of a series has nothing to subtract from;
//   - two readings sharing a timestamp span no time to divide by.
//
// A counter that goes backwards has been reset. That case does report zero,
// and deliberately: unlike the two above it is not an absence of information
// but a known discontinuity, and zero is the convention Prometheus reads a
// reset with. It is pinned by TestCounterResetIsNotNegative.
//
// Every case still updates the remembered reading, so a series recovers on the
// next scrape instead of the gap spreading forward.
func (e *state) observe(at time.Time, raw float64) (float64, bool) {
	if e.kind != KindCounter {
		return raw, true
	}

	defer func() {
		e.lastRaw, e.lastAt, e.hasLastRaw = raw, at, true
	}()

	if !e.hasLastRaw {
		return 0, false
	}

	elapsed := at.Sub(e.lastAt).Seconds()
	if elapsed <= 0 {
		return 0, false
	}

	if raw < e.lastRaw {
		return 0, true
	}

	return (raw - e.lastRaw) / elapsed, true
}

// Snapshot returns the most recently published view. It never blocks and never
// allocates, so the render loop can call it every frame.
func (s *Store) Snapshot() *Snapshot {
	return s.published.Load()
}

// Close releases the store's resources.
func (s *Store) Close(ctx context.Context) error {
	s.log.DebugContext(ctx, "closing series store")

	return nil
}

// index maps a bucket to its ring slot. Go's modulo keeps the sign of the
// dividend, and bucket indices are derived from Unix nanoseconds, so the
// result is masked rather than taken directly.
func index(bucket int64, capacity int) int {
	slot := bucket % int64(capacity)
	if slot < 0 {
		slot += int64(capacity)
	}

	return int(slot)
}

// newStamps returns a stamp ring that matches no bucket.
func newStamps(capacity int) []int64 {
	stamps := make([]int64, capacity)
	for i := range stamps {
		stamps[i] = math.MinInt64
	}

	return stamps
}
