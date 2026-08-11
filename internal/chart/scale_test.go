package chart_test

import (
	"math"
	"testing"

	"github.com/pantherhawk/prism/internal/chart"
)

// TestFormatFitsTheGutter is the regression that P7 bought.
//
// The axis gutter is six columns and its labels are right aligned, so a wider
// label does not spill into the plot where somebody would notice - it runs off
// the left edge and silently loses its leading digits. Envoy's counters put
// 903890 on an axis, the old `%.2fk` made that `903.89k`, and seven characters
// in a six-column gutter drew `03.89k`: a plausible number, three orders of
// magnitude wrong, with nothing about it to suggest anything had gone amiss.
func TestFormatFitsTheGutter(t *testing.T) {
	t.Parallel()

	// The awkward cases are the ones that round up into another digit, since a
	// magnitude-derived precision gets those wrong by exactly one character.
	values := []float64{
		0, 1, -1, 9.96, 10, 99.9, 100, 999, -999,
		1e3, 903890, -903890, 999950, -999950, 99960, -99960, 9996, -9996,
		1e6, 4974900, -4974900, 999999999, 1e9, -1e9, 1.23e12, -1.23e12, 1e21, -1e21,
		0.5, -0.5, 0.0001, -0.0001, 0.9996, -0.9996,
		math.MaxFloat64, math.SmallestNonzeroFloat64,
		math.Inf(1), math.Inf(-1), math.NaN(),
	}

	for _, value := range values {
		got := chart.Format(value)

		if width := len([]rune(got)); width > chart.MaxFormatWidth {
			t.Errorf("Format(%v) = %q, %d runes wide, gutter holds %d",
				value, got, width, chart.MaxFormatWidth)
		}
	}
}

// TestFormatKeepsTheLeadingDigits pins the specific values the old formatter
// mangled, so that a future change to precision cannot quietly bring the bug
// back with a different set of magnitudes.
func TestFormatKeepsTheLeadingDigits(t *testing.T) {
	t.Parallel()

	cases := map[float64]string{
		0:       "0",
		5:       "5.0",
		42:      "42",
		903890:  "903.9k",
		-903890: "-904k",
		1284717: "1.28M",
		4974900: "4.97M",
		2.5e9:   "2.50G",
		0.25:    "0.250",
		1e-9:    "~0",
	}

	for value, want := range cases {
		if got := chart.Format(value); got != want {
			t.Errorf("Format(%v) = %q, want %q", value, got, want)
		}
	}
}

// TestFormatIsMonotonic checks that the formatter never renders a larger value
// as a smaller-looking one within a magnitude band, which is the shape the
// truncation bug took.
func TestFormatIsMonotonic(t *testing.T) {
	t.Parallel()

	previous := math.Inf(-1)

	for value := 1.0; value < 1e9; value *= 1.37 {
		if value <= previous {
			t.Fatalf("test is not walking upwards: %v after %v", value, previous)
		}

		previous = value

		if got := chart.Format(value); got == "" {
			t.Errorf("Format(%v) returned nothing", value)
		}
	}
}
