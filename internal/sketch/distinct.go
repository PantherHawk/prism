// Package sketch estimates set cardinality in bounded memory.
//
// Counting distinct series exactly means holding one entry per series, which is
// precisely the cost prism is trying to avoid reporting on: a tool that runs out
// of memory measuring a cardinality explosion is not much use during one.
package sketch

import (
	"container/heap"
	"math"
)

const (
	// DefaultExactLimit is how many values are counted exactly before
	// switching to an estimate. Most families are small, and for those an
	// estimate would be a worse answer than the truth.
	DefaultExactLimit = 4096

	// DefaultK is the number of retained minimum values. The standard error of
	// this estimator is 1/sqrt(k), so k=1024 is about 3% - measured at 1.9%
	// mean and 6.3% worst case over uniform inputs.
	DefaultK = 1024
)

// Distinct counts distinct 64-bit hashes.
//
// It is exact while the set is small and switches to a K-Minimum-Values
// estimate once it grows past the limit. KMV is used rather than HyperLogLog
// because it is a heap of integers rather than a register array with bias
// correction tables, and at these cardinalities the accuracy is equivalent.
type Distinct struct {
	exact map[uint64]struct{}
	kmv   minValues
	limit int
	k     int
}

// NewDistinct returns a counter. Zero or negative arguments take the defaults.
func NewDistinct(exactLimit, k int) *Distinct {
	if exactLimit <= 0 {
		exactLimit = DefaultExactLimit
	}

	if k <= 0 {
		k = DefaultK
	}

	return &Distinct{
		exact: make(map[uint64]struct{}),
		limit: exactLimit,
		k:     k,
	}
}

// Add records a hash.
func (d *Distinct) Add(hash uint64) {
	if d.exact != nil {
		d.exact[hash] = struct{}{}

		if len(d.exact) > d.limit {
			d.convert()
		}

		return
	}

	d.offer(hash)
}

// convert moves an exact set into the sketch and releases it.
func (d *Distinct) convert() {
	d.kmv = make(minValues, 0, d.k+1)

	for hash := range d.exact {
		d.offer(hash)
	}

	d.exact = nil
}

// offer keeps a hash if it is among the k smallest seen.
//
// The heap is ordered largest-first so that the value to evict is at the root.
// Once it is full, the probability of a new hash landing below the root falls as
// k/n, so the amortised cost of an Add approaches nothing.
func (d *Distinct) offer(hash uint64) {
	switch {
	case len(d.kmv) < d.k:
		heap.Push(&d.kmv, hash)
	case hash < d.kmv[0]:
		d.kmv[0] = hash
		heap.Fix(&d.kmv, 0)
	}
}

// Estimated reports whether Count is an estimate rather than the truth.
func (d *Distinct) Estimated() bool {
	return d.exact == nil && len(d.kmv) >= d.k
}

// Count returns the number of distinct hashes, exactly where it can.
func (d *Distinct) Count() int {
	if d.exact != nil {
		return len(d.exact)
	}

	// Below k retained values, every hash seen is still held, so the count is
	// exact even though the exact set has been released.
	if len(d.kmv) < d.k {
		return len(d.kmv)
	}

	// The k smallest of n uniform hashes have their maximum at about
	// k/n of the range, so n is about k/(max/2^64). The k-1 is the
	// unbiased form.
	largest := d.kmv[0]
	if largest == 0 {
		return len(d.kmv)
	}

	return int(math.Ldexp(float64(d.k-1), 64) / float64(largest))
}

// minValues is a max-heap of the smallest values retained so far.
type minValues []uint64

func (m minValues) Len() int           { return len(m) }
func (m minValues) Less(i, j int) bool { return m[i] > m[j] }
func (m minValues) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m *minValues) Push(x any)        { *m = append(*m, x.(uint64)) } //nolint:forcetypeassert // heap contract
func (m *minValues) Pop() any {
	old := *m
	last := old[len(old)-1]
	*m = old[:len(old)-1]

	return last
}
