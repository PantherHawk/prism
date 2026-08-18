package banner

import (
	"slices"
	"strings"
	"testing"

	"github.com/pantherhawk/prism/internal/theme"
)

// The layout maths in Render depends on these, so a hand-edit to the asset
// that changes its shape must fail here rather than silently misalign the
// splash.
func TestTheArtIsTheDeclaredShape(t *testing.T) {
	t.Parallel()

	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")

	if len(lines) != logoHeight {
		t.Fatalf("art is %d rows, want %d", len(lines), logoHeight)
	}

	for i, line := range lines {
		if len(line) > logoWidth {
			t.Errorf("art row %d is %d columns, want at most %d", i, len(line), logoWidth)
		}
	}
}

// Every band must be reachable, or the spectrum is decorative rather than used.
func TestBandCoversEveryColour(t *testing.T) {
	t.Parallel()

	const bands = 5

	seen := make(map[int]bool, bands)
	for i := range logoHeight {
		seen[band(i, logoHeight, bands)] = true
	}

	for b := range bands {
		if !seen[b] {
			t.Errorf("band %d is never selected across %d rows", b, logoHeight)
		}
	}
}

// Every row must index a real colour, whatever shape the art takes. Row counts
// that do not divide by the band count are the interesting case: the art is 23
// rows over five bands today, and a hand-edit changes that number freely.
func TestBandStaysInsideThePaletteForAnyArtHeight(t *testing.T) {
	t.Parallel()

	for _, rows := range []int{7, 13, 16, 23, 27} {
		const bands = 5

		for i := range rows {
			if got := band(i, rows, bands); got < 0 || got >= bands {
				t.Errorf("band(%d, %d, %d) is %d, out of range", i, rows, bands, got)
			}
		}
	}
}

// The remainder is spread across the ramp rather than left on the last colour,
// so no wavelength is more than a row wider than any other. Dividing by a fixed
// step instead would give the art's 23 rows four rows each of the first four
// bands and seven of the last.
func TestBandSpreadsTheRemainderAcrossTheRamp(t *testing.T) {
	t.Parallel()

	const bands = 5

	stripes := make([]int, bands)
	for i := range logoHeight {
		stripes[band(i, logoHeight, bands)]++
	}

	if slices.Max(stripes)-slices.Min(stripes) > 1 {
		t.Errorf("stripes are %v, want none more than one row wider than another", stripes)
	}
}

func TestBandHandlesFewerRowsThanBands(t *testing.T) {
	t.Parallel()

	if got := band(0, 1, 5); got != 0 {
		t.Errorf("band(0, 1, 5) is %d, want 0", got)
	}
}

func TestLogoReturnsOneStyledRowPerArtRow(t *testing.T) {
	t.Parallel()

	rows := logo(theme.Resolve(theme.Config{Mode: theme.ModeDark}, true))

	if len(rows) != logoHeight {
		t.Errorf("logo returned %d rows, want %d", len(rows), logoHeight)
	}
}
