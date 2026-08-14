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

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/theme"
)

// labelWidth pads the runtime labels so their values form a column.
const labelWidth = 8

// swatchWidth is how many cells each spectrum band occupies in the colour row.
const swatchWidth = 3

// Info is the runtime and host detail shown beside the wordmark. Every field
// is a plain string, already formatted: the banner is a pure function of its
// inputs, so nothing here may be derived at render time. Empty fields are
// omitted rather than rendered blank.
type Info struct {
	// prism's own runtime.
	Version  string
	Endpoint string
	Buffer   string

	// The host, from internal/hostinfo.
	User   string
	Host   string
	OS     string
	Kernel string
	Shell  string
	Term   string
	Go     string
	Uptime string
}

// Render returns the splash screen, centred within width.
func Render(info Info, palette theme.Palette, styles theme.Styles, width int) string {
	lines := append(
		[]string{styles.Accent.Render("PROMDATE"), ""},
		infoBlock(info, palette, styles)...,
	)
	lines = append(lines, "", hint(styles))

	block := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// infoBlock renders the neofetch column: a user@host title, a rule, the facts
// as an aligned two-column list, then the spectrum as colour blocks.
//
// The shape follows neofetch's own config, which is a title, a run of
// label/value pairs and a trailing colour row.
func infoBlock(info Info, palette theme.Palette, styles theme.Styles) []string {
	var lines []string

	title := heading(info, styles)
	lines = append(lines, title, styles.Chrome.Render(strings.Repeat("─", ansi.StringWidth(title))))

	for _, fact := range []struct{ label, value string }{
		{"version", info.Version},
		{"scrape", info.Endpoint},
		{"buffer", info.Buffer},
		{"os", info.OS},
		{"kernel", info.Kernel},
		{"shell", info.Shell},
		{"term", info.Term},
		{"go", info.Go},
		{"uptime", info.Uptime},
	} {
		if fact.value == "" {
			continue
		}

		lines = append(lines, join(
			styles.Muted.Render(label(fact.label)),
			styles.Base.Render(fact.value),
		))
	}

	return append(lines, "", swatch(palette))
}

// heading renders the user@host line, falling back to the product name when
// neither is known.
func heading(info Info, styles theme.Styles) string {
	switch {
	case info.User != "" && info.Host != "":
		return styles.Title.Render(info.User) +
			styles.Chrome.Render("@") +
			styles.Title.Render(info.Host)
	case info.Host != "":
		return styles.Title.Render(info.Host)
	default:
		return styles.Title.Render("promdate")
	}
}

// swatch renders the spectrum as solid blocks, the way neofetch closes its
// info column with the terminal's palette.
func swatch(palette theme.Palette) string {
	var row strings.Builder

	for _, colour := range palette.Spectrum {
		row.WriteString(lipgloss.NewStyle().
			Foreground(colour).
			Render(strings.Repeat("█", swatchWidth)))
	}

	return row.String()
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
