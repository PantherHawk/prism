package tui

import (
	"strings"
	"testing"

	"github.com/pantherhawk/prism/internal/theme"
)

// plain renders a frame with colour discarded, so a layout assertion reads as
// the picture it is asserting about.
func plain(f *frame) []string {
	rows := make([]string, 0, len(f.cells))

	for row := range f.cells {
		var line strings.Builder
		for col := range f.cells[row] {
			line.WriteRune(f.cells[row][col].glyph)
		}

		rows = append(rows, line.String())
	}

	return rows
}

func testFrame(width, height int) *frame {
	palette := theme.Resolve(theme.Config{Mode: theme.ModeDark}, true)

	return newFrame(width, height, buildStyles(theme.NewStyles(palette), palette))
}

func TestNewFrameIsBlank(t *testing.T) {
	t.Parallel()

	rows := plain(testFrame(4, 2))

	want := []string{"    ", "    "}
	for i, row := range rows {
		if row != want[i] {
			t.Errorf("row %d = %q, want %q", i, row, want[i])
		}
	}
}

func TestPutWritesAtTheColumnAndReturnsTheNext(t *testing.T) {
	t.Parallel()

	f := testFrame(10, 1)

	next := f.put(0, 2, "abc", styleBase)

	if got := plain(f)[0]; got != "  abc     " {
		t.Errorf("row = %q, want %q", got, "  abc     ")
	}

	// The return value is what lets a caller chain puts without counting runes.
	if next != 5 {
		t.Errorf("put returned %d, want 5", next)
	}
}

func TestRputEndsAtTheGivenColumn(t *testing.T) {
	t.Parallel()

	f := testFrame(10, 1)

	f.rput(0, 8, "abc", styleBase)

	if got := plain(f)[0]; got != "     abc  " {
		t.Errorf("row = %q, want %q", got, "     abc  ")
	}
}

func TestFillCoversBothEnds(t *testing.T) {
	t.Parallel()

	f := testFrame(6, 1)

	f.fill(0, 1, 4, '-', styleBase)

	if got := plain(f)[0]; got != " ---- " {
		t.Errorf("row = %q, want %q", got, " ---- ")
	}
}

func TestVlineCoversBothEnds(t *testing.T) {
	t.Parallel()

	f := testFrame(3, 4)

	f.vline(1, 1, 2, '|', styleBase)

	want := []string{"   ", " | ", " | ", "   "}
	for i, row := range plain(f) {
		if row != want[i] {
			t.Errorf("row %d = %q, want %q", i, row, want[i])
		}
	}
}

// Every draw call takes coordinates computed from the terminal size, and a
// terminal can be resized to something no layout anticipated. Clipping in one
// place is what keeps a stray column from panicking mid-frame.
func TestDrawingOutsideTheFrameIsClipped(t *testing.T) {
	t.Parallel()

	f := testFrame(4, 2)

	f.put(0, 2, "abcdef", styleBase)
	f.put(-1, 0, "x", styleBase)
	f.put(9, 0, "x", styleBase)
	f.put(0, -3, "yz", styleBase)
	f.rput(1, 20, "q", styleBase)
	f.fill(1, -5, 99, '.', styleBase)
	f.vline(99, 0, 1, '|', styleBase)

	rows := plain(f)
	if rows[0] != "  ab" {
		t.Errorf("row 0 = %q, want %q", rows[0], "  ab")
	}

	if rows[1] != "...." {
		t.Errorf("row 1 = %q, want %q", rows[1], "....")
	}
}

// A rune that starts before the left edge must still land on the cells it
// overlaps, or a scrolled label would vanish rather than being cut.
func TestPutClipsTheLeadingRunesRatherThanTheWholeString(t *testing.T) {
	t.Parallel()

	f := testFrame(4, 1)

	f.put(0, -2, "abcd", styleBase)

	if got := plain(f)[0]; got != "cd  " {
		t.Errorf("row = %q, want %q", got, "cd  ")
	}
}

func TestStringRendersEveryRow(t *testing.T) {
	t.Parallel()

	f := testFrame(3, 2)
	f.put(0, 0, "ab", styleBase)
	f.put(1, 1, "c", styleBase)

	lines := strings.Split(f.String(), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	// Colour is applied, so compare on the text the styling wraps.
	for i, want := range []string{"ab", "c"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], want)
		}
	}
}

func TestSpectrumStyleWrapsAroundThePalette(t *testing.T) {
	t.Parallel()

	const bands = 5

	first := spectrumStyle(0, bands)
	if got := spectrumStyle(bands, bands); got != first {
		t.Errorf("band %d = %d, want it to wrap to %d", bands, got, first)
	}

	if spectrumStyle(1, bands) == first {
		t.Error("adjacent bands resolved to the same style")
	}
}

// A style index that outruns the table must fall back rather than panic: the
// table is built from the palette, and the two are sized independently.
func TestUnknownStyleIndexFallsBack(t *testing.T) {
	t.Parallel()

	f := testFrame(4, 1)

	f.put(0, 0, "ab", 250)

	if !strings.Contains(f.String(), "ab") {
		t.Error("an out-of-range style index dropped the text")
	}
}

// Every style index the views name has to exist in the table, or a panel would
// silently render with the wrong colour.
func TestBuildStylesCoversEveryNamedIndex(t *testing.T) {
	t.Parallel()

	palette := theme.Resolve(theme.Config{Mode: theme.ModeDark}, true)
	table := buildStyles(theme.NewStyles(palette), palette)

	named := []uint8{
		styleBase, styleMuted, styleChrome, styleAccent,
		styleSecondary, styleKey, styleOK, styleSelected,
	}

	for _, index := range named {
		if int(index) >= len(table) {
			t.Errorf("style index %d is outside a table of %d", index, len(table))
		}
	}

	// The spectrum bands sit past the named styles.
	bands := len(palette.Spectrum)
	if got := int(spectrumStyle(bands-1, bands)); got >= len(table) {
		t.Errorf("the last spectrum band is index %d, outside a table of %d", got, len(table))
	}
}
