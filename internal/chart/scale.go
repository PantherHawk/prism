package chart

import (
	"fmt"
	"math"
	"strings"
)

// headroom is the fraction of empty space left above the tallest point, so
// that a peak never touches the top border and become indistinguishable from a
// clipped one.
const headroom = 1.08

// sparkGlyphs is the eighth-block ramp used for cardinality sparklines.
const sparkGlyphs = "▁▂▃▄▅▆▇█"

// Bounds returns the range to scale a set of series to, ignoring gaps.
//
// The floor is pinned to zero for any non-negative data. Auto-scaling the
// bottom of a rate chart to its minimum is the classic way to make a 2% wobble
// look like a catastrophe.
func Bounds(series ...[]float64) (low, high float64) {
	low, high = math.Inf(1), math.Inf(-1)

	for _, values := range series {
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}

			low = math.Min(low, value)
			high = math.Max(high, value)
		}
	}

	if math.IsInf(low, 1) || math.IsInf(high, -1) {
		return 0, 1
	}

	if low >= 0 {
		low = 0
	}

	high *= headroom

	if high <= low {
		high = low + 1
	}

	return low, high
}

// Sparkline renders values as a single line of eighth-blocks.
func Sparkline(values []float64, width int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}

	glyphs := []rune(sparkGlyphs)
	out := make([]rune, 0, width)

	_, high := Bounds(values)
	if high <= 0 {
		high = 1
	}

	// Take the last `width` values: a sparkline shows the recent shape, and
	// squeezing an hour into eight cells shows nothing at all.
	start := max(len(values)-width, 0)

	for _, value := range values[start:] {
		if math.IsNaN(value) {
			out = append(out, ' ')

			continue
		}

		level := int(value / high * float64(len(glyphs)))
		out = append(out, glyphs[min(max(level, 0), len(glyphs)-1)])
	}

	return string(out)
}

// MaxFormatWidth is the widest string [Format] will return, in characters.
//
// The axis gutter is a fixed number of columns and its labels are right
// aligned, so a label wider than the gutter does not overflow into the plot -
// it runs off the left edge and loses its leading digits. That is the one
// failure mode a chart must not have: 903.89k arriving as 03.89k is not a
// clipped label, it is a different number, and nothing about it looks wrong.
// Format is bounded here so the drawing code never has to check.
const MaxFormatWidth = 6

const (
	// exponentAbove is where the suffix ladder runs out and the label falls
	// back to an exponent.
	exponentAbove = 1e12

	// wholeAbove is where a decimal place stops earning its column: at ten and
	// up there are already two significant figures before the point.
	wholeAbove = 10

	// negligible is the magnitude below which an axis label says "~0" rather
	// than spending its whole width on an exponent nobody can read at a glance.
	negligible = 1e-3
)

// magnitudes is the suffix ladder, largest first. It is a table rather than a
// chain of cases so that adding a rung cannot change how [Format] branches.
//
//nolint:gochecknoglobals // immutable ladder
var magnitudes = []struct {
	threshold float64
	suffix    string
}{
	{1e9, "G"},
	{1e6, "M"},
	{1e3, "k"},
}

// Format renders a value the way an operator reads it: three significant
// figures and a magnitude suffix, because a chart axis labelled 1284717 tells
// you less than one labelled 1.28M.
//
// The result is never wider than [MaxFormatWidth]. Decimal places are given up
// as the mantissa's leading digits need the room, rather than being fixed, so
// 903.89k - seven characters, and the label that started all this - is drawn as
// 903.9k and keeps every digit that fits.
func Format(value float64) string {
	absolute := math.Abs(value)

	switch {
	case math.IsNaN(value):
		return "-"
	case math.IsInf(value, 0):
		return infinity(value)
	case absolute == 0:
		return "0"
	case absolute >= exponentAbove:
		return exponent(value)
	case absolute < 1:
		return small(value)
	}

	for _, magnitude := range magnitudes {
		if absolute >= magnitude.threshold {
			return scaled(value, magnitude.threshold, magnitude.suffix)
		}
	}

	if absolute >= wholeAbove {
		return fmt.Sprintf("%.0f", value)
	}

	return fmt.Sprintf("%.1f", value)
}

// infinity names an unbounded value. It should not reach an axis - [Bounds]
// skips infinities - but a label is drawn from whatever the caller passes, and
// "+Inf" is five characters of Go where "+∞" is two of arithmetic.
func infinity(value float64) string {
	if value > 0 {
		return "+∞"
	}

	return "-∞"
}

// scaled renders a value divided down to a magnitude suffix, keeping the finest
// precision that still fits.
//
// It tries decimal places rather than deriving them from the magnitude because
// the two disagree at the edges: -99.96 wants one decimal by its magnitude,
// rounds to -100.0, and comes out a character over. Asking whether the result
// fits is the only rule that cannot be caught out by rounding, and the coarsest
// option - no decimals on a mantissa below 1000, with a sign and a suffix - is
// six characters, so the loop always terminates on something.
func scaled(value, divisor float64, suffix string) string {
	mantissa := value / divisor

	for _, places := range []int{2, 1, 0} {
		out := fmt.Sprintf("%.*f%s", places, mantissa, suffix)
		if len([]rune(out)) <= MaxFormatWidth {
			return out
		}
	}

	return fmt.Sprintf("%.0f%s", mantissa, suffix)
}

// exponent renders a value past the top of the suffix ladder.
//
// The ladder stops at G because an operator reads "4.97M" and does not read
// "0.00000497T". Above a thousand G there is no suffix left that helps, and
// what matters is only that the label stays inside the gutter: a single
// significant figure and an exponent is five characters, six with a sign, which
// is the widest a float64 can make this.
//
// The exponent's "+" is dropped because it costs a column and says nothing -
// the branch is only reached by values far above one.
func exponent(value float64) string {
	return strings.Replace(fmt.Sprintf("%.0e", value), "e+", "e", 1)
}

// small renders a value below one.
//
// %g would reach for an exponent on a value like 0.0000123 and spend eight
// characters doing it. Anything that small is zero as far as an axis is
// concerned, and saying so is more honest than a label nobody can read.
func small(value float64) string {
	if math.Abs(value) < negligible {
		return "~0"
	}

	return fmt.Sprintf("%.3f", value)
}
