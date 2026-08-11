// Package banner renders prism's splash screen: the wordmark with one
// wavelength per letter, which is the whole product metaphor in five glyphs.
//
// It is a pure function of its inputs. Given the same Info, palette and width
// it produces the same string, which makes it trivially golden-testable and
// keeps it out of the frame budget.
package banner

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/theme"
)

const (
	// glyphHeight is the number of rows in every wordmark glyph.
	glyphHeight = 5

	// labelWidth pads the runtime labels so their values form a column.
	labelWidth = 6
)

// wordmark is the text rendered as block glyphs. One letter is drawn per
// entry in [theme.Palette.Spectrum], so the two must stay the same length.
const wordmark = "PRISM"

// glyphs is a five-row block font covering exactly the letters in [wordmark].
// It is deliberately hand-built rather than generated: it is five letters, and
// a dependency on a figlet font would be a larger surface than the art.
var glyphs = map[rune][glyphHeight]string{ //nolint:gochecknoglobals // immutable lookup table
	'P': {"████ ", "█   █", "████ ", "█    ", "█    "},
	'R': {"████ ", "█   █", "████ ", "█  █ ", "█   █"},
	'I': {"█████", "  █  ", "  █  ", "  █  ", "█████"},
	'S': {" ████", "█    ", " ███ ", "    █", "████ "},
	'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █"},
}

// Info is the runtime detail shown beneath the wordmark. Empty fields are
// omitted rather than rendered blank.
type Info struct {
	Version  string
	Endpoint string
	Buffer   string
}

// Render returns the full splash screen, centred within width.
func Render(info Info, palette theme.Palette, styles theme.Styles, width int) string {
	lines := make([]string, 0, glyphHeight+8)
	lines = append(lines, mark(palette)...)
	lines = append(lines, "", styles.Muted.Render("split your metrics into their spectrum"), "")
	lines = append(lines, meta(info, styles)...)
	lines = append(lines, "", hint(styles))

	block := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// mark renders the wordmark, colouring each letter with its own wavelength.
func mark(palette theme.Palette) []string {
	rows := make([]string, glyphHeight)

	for row := range glyphHeight {
		var line strings.Builder

		for i, letter := range wordmark {
			glyph, ok := glyphs[letter]
			if !ok {
				continue
			}

			style := lipgloss.NewStyle().Foreground(palette.Spectrum[i%len(palette.Spectrum)])
			line.WriteString(style.Render(glyph[row]))
			line.WriteString(" ")
		}

		rows[row] = line.String()
	}

	return rows
}

// meta renders the version and runtime lines as an aligned two-column block.
func meta(info Info, styles theme.Styles) []string {
	lines := make([]string, 0, 3)

	if info.Version != "" {
		lines = append(lines, join(
			styles.Title.Render("prism"),
			styles.Muted.Render(info.Version),
			styles.Chrome.Render("·"),
			styles.Muted.Render("bubbletea v2 · ultraviolet"),
		))
	}

	if info.Endpoint != "" {
		lines = append(lines, join(
			styles.Muted.Render(label("scrape")),
			styles.Secondary.Render(info.Endpoint),
		))
	}

	if info.Buffer != "" {
		lines = append(lines, join(
			styles.Muted.Render(label("buffer")),
			styles.Base.Render(info.Buffer),
		))
	}

	return lines
}

// hint renders the single line telling the operator what to press.
func hint(styles theme.Styles) string {
	return join(
		styles.Muted.Render("press"),
		styles.Key.Render("any key"),
		styles.Muted.Render("to begin ·"),
		styles.Key.Render("?"),
		styles.Muted.Render("for help"),
	)
}

// label pads a runtime label so that values line up in a column.
func label(text string) string {
	return fmt.Sprintf("%-*s", labelWidth, text)
}

// join concatenates rendered spans with a single space between them.
func join(spans ...string) string {
	return strings.Join(spans, " ")
}
