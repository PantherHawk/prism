package series

import (
	"hash/fnv"
	"sort"
	"strings"
)

// Kind is how a metric behaves over time, which decides how prism displays it.
//
// It is a string rather than an integer because it travels over the wire in a
// stream frame: a follower reading `"kind":"counter"` can be debugged with
// curl, whereas `"kind":1` freezes an enum's ordering into the protocol.
type Kind string

const (
	// KindUntyped is a metric the target declared no type for.
	KindUntyped Kind = "untyped"
	// KindCounter only ever climbs, so prism shows it as a per-second rate.
	KindCounter Kind = "counter"
	// KindGauge is a level, shown as observed.
	KindGauge Kind = "gauge"
	// KindHistogram is charted by its observation count.
	KindHistogram Kind = "histogram"
	// KindSummary is charted by its observation count.
	KindSummary Kind = "summary"
)

// Unit returns the suffix the header puts after a family name, describing what
// the numbers on the axis actually are. A counter is charted as a rate, and an
// axis that does not say so invites reading a rate as a total.
func (k Kind) Unit() string {
	switch k {
	case KindCounter:
		return "/s"
	case KindHistogram, KindSummary:
		return "obs"
	default:
		// A gauge is already in whatever unit the target chose, and an untyped
		// metric is one prism has no claim to make about.
		return ""
	}
}

// Label is one name/value pair on a series.
type Label struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Labels is a series' full label set. It is a slice rather than a map: label
// sets are small, read far more often than they are built, and a sorted slice
// gives a stable identity and a stable render order for free.
type Labels []Label

// Sort orders the set by label name, which is what makes a fingerprint
// independent of the order the target happened to write the labels in.
func (l Labels) Sort() {
	sort.Slice(l, func(i, j int) bool { return l[i].Name < l[j].Name })
}

// Lookup returns the value of a label. The second result distinguishes a label
// that is absent from one set to the empty string - a pivot has to separate
// those two groups rather than merge them.
func (l Labels) Lookup(name string) (string, bool) {
	for _, label := range l {
		if label.Name == name {
			return label.Value, true
		}
	}

	return "", false
}

// Get returns the value of a label, or the empty string when it is not set.
//
// It is the accessor a matcher wants: `status=200` should simply not match a
// series carrying no status label, and making every matcher branch on a missing
// label to reach that conclusion is noise. Use [Labels.Lookup] where absent and
// empty must be told apart.
func (l Labels) Get(name string) string {
	value, _ := l.Lookup(name)

	return value
}

// String renders the set as `name=value` pairs separated by spaces. The
// Prometheus braces are dropped deliberately: the series panel is narrow, and
// two characters per row buy more of the label than they explain.
func (l Labels) String() string {
	if len(l) == 0 {
		return ""
	}

	var out strings.Builder

	for i, label := range l {
		if i > 0 {
			out.WriteByte(' ')
		}

		out.WriteString(label.Name)
		out.WriteByte('=')
		out.WriteString(label.Value)
	}

	return out.String()
}

// Sample is one observation of one series at one instant.
type Sample struct {
	Family string  `json:"family"`
	Labels Labels  `json:"labels,omitempty"`
	Kind   Kind    `json:"kind"`
	Value  float64 `json:"value"`
}

// ID identifies a series. It is a hash rather than the label set itself so that
// the store can key its ring buffers on a single word: at the cardinalities
// prism exists to diagnose, a map keyed by a rendered label string spends more
// memory on its keys than on the samples they point at.
type ID uint64

// separator delimits the parts of a fingerprint. It cannot appear in a metric
// or label name, and Prometheus forbids it in both, so it is the one byte that
// can never be confused for content.
const separator = 0xff

// Fingerprint returns the identity of a series.
//
// The family and every name and value are written with a delimiter between
// them, so that a label set cannot be made to collide with another by moving
// the boundary between a name and its value. Callers pass sorted labels;
// unsorted input yields a different fingerprint for the same series, which is
// why both [Store.record] and the scraper sort before they get here.
func Fingerprint(family string, labels Labels) ID {
	hash := fnv.New64a()

	write := func(part string) {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{separator})
	}

	write(family)

	for _, label := range labels {
		write(label.Name)
		write(label.Value)
	}

	return ID(mix(hash.Sum64()))
}

// mix is the SplitMix64 finaliser, applied so that a fingerprint is spread
// uniformly across the 64-bit range rather than merely being unique.
//
// Uniqueness is all FNV promises, and it is not enough for what reads these
// identities. The cardinality sketch estimates from the k smallest hashes it
// has seen, and the sampler admits on their low bits; both are only correct
// over a uniform input. Series within one family differ by a few characters in
// one label value, which is precisely the shape FNV spreads poorly: over ten
// thousand such series the unmixed hash makes the sketch report 12,400.
//
//nolint:mnd // the shifts and multipliers are SplitMix64's published constants
func mix(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}
