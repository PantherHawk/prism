// Package banner renders prism's splash screen: an embedded wordmark, coloured
// in horizontal spectrum bands, beside a neofetch-style column of host facts.
//
// It is a pure function of its inputs. Given the same Info, palette and
// dimensions it produces the same string, which makes it trivially
// golden-testable and keeps it out of the frame budget.
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

const (
	// fullWidth and fullHeight are the smallest terminal that gets the logo
	// beside the facts. Below either, the logo is dropped rather than wrapped.
	//
	// The width is the art (55) plus the gutter plus a comfortable info
	// column; the height is the art (23) plus the keyhint and its spacing.
	// That puts the full tier above a stock 80x24 terminal, which is a
	// deliberate trade: the isometric2 wordmark is half again as large as the
	// face it replaced, and a stock terminal draws the compact tier instead.
	fullWidth  = 92
	fullHeight = 26

	// gutter is the gap between the logo and the info column.
	gutter = 4
)

// Render returns the splash screen, centred within width.
//
// It is a pure function of its inputs: the same Info, palette, styles and
// dimensions always produce the same string.
func Render(info Info, palette theme.Palette, styles theme.Styles, width, height int) string {
	block := compact(info, palette, styles)

	// The constants gate the tier, but they assume an info column of ordinary
	// width, and the column is as wide as the facts it was handed — an
	// upstream URL is unbounded. Measuring the composed block catches the case
	// the constants cannot, and costs one discarded join on a screen that is
	// drawn once.
	if width >= fullWidth && height >= fullHeight {
		if wide := full(info, palette, styles); lipgloss.Width(wide) <= width {
			block = wide
		}
	}

	block = lipgloss.JoinVertical(lipgloss.Left, block, "", hint(styles))

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// full draws the logo beside the facts.
//
// The two columns are vertically centred against each other rather than
// top-aligned: the logo is taller than the facts, and top-aligning would pin
// the facts to the very top row of the two-word wordmark instead of letting
// them sit against its visual centre.
func full(info Info, palette theme.Palette, styles theme.Styles) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left, logo(palette)...),
		strings.Repeat(" ", gutter),
		lipgloss.JoinVertical(lipgloss.Left, infoBlock(info, palette, styles)...),
	)
}

// compact drops the logo and titles the facts with the wordmark instead.
//
// This is neofetch's own --off mode rather than a degradation: an art block 55
// columns wide does not belong in a split pane, and wrapping it would be worse
// than omitting it.
func compact(info Info, palette theme.Palette, styles theme.Styles) string {
	lines := append(
		[]string{styles.Accent.Render("PROMDATE"), ""},
		infoBlock(info, palette, styles)...,
	)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
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

// heading renders the user@host line: both when both are known, whichever one
// is known when only one is, and the product name when neither is known.
func heading(info Info, styles theme.Styles) string {
	switch {
	case info.User != "" && info.Host != "":
		return styles.Title.Render(info.User) +
			styles.Chrome.Render("@") +
			styles.Title.Render(info.Host)
	case info.Host != "":
		return styles.Title.Render(info.Host)
	case info.User != "":
		return styles.Title.Render(info.User)
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
