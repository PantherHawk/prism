package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/theme"
)

// Style indices. Cells hold an index into the frame's style table rather than a
// [lipgloss.Style], because a style is a fat value and a dashboard is tens of
// thousands of cells: storing one byte per cell keeps a frame in cache, and it
// means a theme change is a new table rather than a walk over every cell.
const (
	styleBase uint8 = iota
	styleMuted
	styleChrome
	styleAccent
	styleSecondary
	styleKey
	styleOK
	styleSelected

	// styleSpectrum is the first of the wavelength styles. The bands follow it
	// contiguously, so a line's colour is an offset rather than a lookup.
	styleSpectrum
)

// buildStyles flattens a palette into the table a frame indexes into. It is
// rebuilt once per theme change, never per frame.
func buildStyles(styles theme.Styles, palette theme.Palette) []lipgloss.Style {
	table := make([]lipgloss.Style, styleSpectrum, int(styleSpectrum)+len(palette.Spectrum))

	table[styleBase] = styles.Base
	table[styleMuted] = styles.Muted
	table[styleChrome] = styles.Chrome
	table[styleAccent] = styles.Accent
	table[styleSecondary] = styles.Secondary
	table[styleKey] = styles.Key
	table[styleOK] = styles.OK
	table[styleSelected] = styles.Selected

	for _, colour := range palette.Spectrum {
		table = append(table, styles.Base.Foreground(colour))
	}

	return table
}

// spectrumStyle returns the style index for the nth plotted line, wrapping when
// there are more lines than wavelengths.
func spectrumStyle(index, bands int) uint8 {
	if bands <= 0 {
		return styleBase
	}

	band := index % bands
	if band < 0 {
		band += bands
	}

	return styleSpectrum + uint8(band) //nolint:gosec // band is bounded by the palette
}

// cell is one character position on screen.
type cell struct {
	glyph rune
	style uint8
}

// frame is the character grid one render composes into.
//
// The dashboard is drawn by absolute position - a box edge, an axis tick, a
// right-aligned status - and composing that from nested lipgloss blocks means
// every panel has to know the width of its neighbours. A grid lets each drawing
// function name the row and column it means, and it gives the help panel
// somewhere to be drawn over the top of everything else.
type frame struct {
	cells [][]cell
	table []lipgloss.Style
}

// newFrame returns a blank frame of the given size.
func newFrame(width, height int, table []lipgloss.Style) *frame {
	cells := make([][]cell, max(height, 0))

	for row := range cells {
		cells[row] = make([]cell, max(width, 0))
		for col := range cells[row] {
			cells[row][col] = cell{glyph: ' ', style: styleBase}
		}
	}

	return &frame{cells: cells, table: table}
}

// String renders the grid.
//
// Runs of identically styled cells are rendered together rather than one cell
// at a time: a box edge is one escape sequence instead of eighty, which is the
// difference between a frame the diffing renderer can skip and one it cannot.
func (f *frame) String() string {
	var out strings.Builder

	for row := range f.cells {
		if row > 0 {
			out.WriteByte('\n')
		}

		f.writeRow(&out, f.cells[row])
	}

	return out.String()
}

// set writes one cell, ignoring anything outside the frame.
//
// Every coordinate in the dashboard is derived from the terminal size, and a
// terminal can be resized to something no layout anticipated. Clipping here
// once is what keeps a stray column from panicking in the middle of a frame.
func (f *frame) set(row, col int, glyph rune, style uint8) {
	if row < 0 || row >= len(f.cells) {
		return
	}

	if col < 0 || col >= len(f.cells[row]) {
		return
	}

	f.cells[row][col] = cell{glyph: glyph, style: style}
}

// put writes text starting at a column and returns the column after it.
//
// The returned column is advanced by the full length of the text even when part
// of it was clipped, so that a caller chaining puts stays in step with the
// layout it thinks it is drawing.
func (f *frame) put(row, col int, text string, style uint8) int {
	for _, glyph := range text {
		f.set(row, col, glyph, style)
		col++
	}

	return col
}

// putUpTo writes text at a column but never at or past limit, cutting it with
// an ellipsis where it does not fit, and returns the column after what it wrote.
//
// put clips at the edge of the frame, which is the right answer for a stray
// coordinate and the wrong one for two panels sharing a row: clipping at the
// edge means the text sails straight through whatever its neighbour has drawn.
// A caller that has a neighbour needs to say where it is.
func (f *frame) putUpTo(row, col, limit int, text string, style uint8) int {
	if col >= limit {
		return col
	}

	return f.put(row, col, truncate(text, limit-col), style)
}

// rput writes text so that it ends at the given column, exclusive.
func (f *frame) rput(row, end int, text string, style uint8) {
	f.put(row, end-len([]rune(text)), text, style)
}

// fill repeats a glyph across a row, from and to inclusive.
func (f *frame) fill(row, from, to int, glyph rune, style uint8) {
	for col := from; col <= to; col++ {
		f.set(row, col, glyph, style)
	}
}

// vline repeats a glyph down a column, from and to inclusive.
func (f *frame) vline(col, from, to int, glyph rune, style uint8) {
	for row := from; row <= to; row++ {
		f.set(row, col, glyph, style)
	}
}

// styleAt returns a style by index, falling back to the base style. The table
// is sized from the palette and the indices come from the views, so the two can
// disagree; rendering the text unstyled is a better answer than a panic.
func (f *frame) styleAt(index uint8) (lipgloss.Style, bool) {
	if int(index) < len(f.table) {
		return f.table[index], true
	}

	if len(f.table) > int(styleBase) {
		return f.table[styleBase], true
	}

	return lipgloss.Style{}, false
}

// writeRow renders one row as a sequence of styled runs.
func (f *frame) writeRow(out *strings.Builder, cells []cell) {
	var (
		run   strings.Builder
		style uint8
		open  bool
	)

	flush := func() {
		if !open || run.Len() == 0 {
			return
		}

		if resolved, ok := f.styleAt(style); ok {
			out.WriteString(resolved.Render(run.String()))
		} else {
			out.WriteString(run.String())
		}

		run.Reset()
	}

	for _, current := range cells {
		if open && current.style != style {
			flush()
		}

		style, open = current.style, true

		run.WriteRune(current.glyph)
	}

	flush()
}
