package banner

import (
	_ "embed"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/theme"
)

const (
	// The art's dimensions. Render's layout maths depends on them, and
	// logo_internal_test.go asserts the asset still matches.
	logoWidth  = 55
	logoHeight = 23
)

// art is the wordmark: PROM stacked over DATE, rendered in figlet's isometric2
// font. It is a file rather than a string literal so that it stays editable as
// art — every other character in it is a backslash, and escaping the whole
// thing into Go source would make it unreadable and unfixable.
//
// No nolint:gochecknoglobals here: gochecknoglobals has a built-in allowance
// for variables carrying a go:embed directive, so it never fires on this var,
// and nolintlint would flag an unused directive if one were added back.
//
//go:embed promdate.ascii
var art string

// logo returns the art with one spectrum band applied per horizontal stripe.
//
// The banding is borrowed from charmbracelet/vhs examples/neofetch: rather
// than interpolating a gradient, the rows are cut into as many equal stripes
// as there are colours, so the beam reads as split rather than blurred. The
// art is 23 rows over five bands, which is why band spreads the remainder
// rather than leaving it on the last colour.
func logo(palette theme.Palette) []string {
	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
	rows := make([]string, len(lines))

	for i, line := range lines {
		colour := palette.Spectrum[band(i, len(lines), len(palette.Spectrum))]
		rows[i] = lipgloss.NewStyle().Foreground(colour).Render(line)
	}

	return rows
}

// band returns the colour index for row i of rows, given bands colours.
//
// The reference divides the row index by a fixed step of rows/bands, which
// works only when the two divide evenly and otherwise dumps the whole
// remainder on the final colour — 23 rows over five bands would be four rows
// each of violet through green under seven rows of red, and the two stacked
// words would not carry the same stretch of spectrum. Scaling the index
// instead spreads the remainder across the ramp: 5, 5, 4, 5, 4.
//
// The clamp is a bounds guard rather than a correction; the scaled index
// cannot overrun for any i below rows.
func band(i, rows, bands int) int {
	return min(bands-1, max(0, i*bands/rows))
}
