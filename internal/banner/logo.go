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
	logoWidth  = 43
	logoHeight = 15
)

// art is the wordmark: PROM stacked over DATE, rendered in figlet's swampland
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
// as there are colours. Fifteen rows over five bands divides exactly, so each
// wavelength covers three rows and the beam reads as split rather than blurred.
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
// The clamp is load-bearing: rows/bands truncates, so the last rows of an art
// whose height is not a multiple of the band count would index past the end.
func band(i, rows, bands int) int {
	step := rows / bands
	if step == 0 {
		step = 1
	}

	return min(bands-1, max(0, i/step))
}
