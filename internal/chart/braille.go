// Package chart turns series values into terminal cells.
//
// It is pure: no clock, no I/O, no randomness. Given the same values and the
// same size it produces the same cells, which is what makes the charts golden
// testable and what keeps them inside the frame budget.
package chart

import "math"

const (
	// dotsWide and dotsHigh are the dot resolution of one braille cell, which
	// is what buys a chart four times the vertical detail of block glyphs.
	dotsWide = 2
	dotsHigh = 4

	// brailleBase is the first code point of the Braille Patterns block. The
	// low eight bits of a rune in that block are its dot bitmap.
	brailleBase = 0x2800

	// noSeries marks a cell no line has touched.
	noSeries = -1
)

// dotBits maps a dot position within a cell to its bit in the rune. The order
// is historical rather than logical: dots 1-6 were assigned first and 7-8 were
// added later, which is why the fourth row is not adjacent to the third.
var dotBits = [dotsWide][dotsHigh]byte{ //nolint:gochecknoglobals // immutable lookup table
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// Run is a stretch of cells belonging to one series, so that a whole run can
// be styled with a single escape sequence rather than one per cell.
type Run struct {
	Text   string
	Series int
}

// Row is one terminal line of the plot.
type Row []Run

// Canvas is a dot grid that renders as braille cells.
type Canvas struct {
	width  int
	height int
	bits   []byte
	owner  []int8
}

// New returns a canvas of the given size in cells.
func New(width, height int) *Canvas {
	canvas := &Canvas{
		width:  max(width, 0),
		height: max(height, 0),
		bits:   make([]byte, max(width*height, 0)),
		owner:  make([]int8, max(width*height, 0)),
	}

	canvas.Reset()

	return canvas
}

// Reset clears the canvas, keeping its allocation. Panning redraws the whole
// plot every frame, so this is called far more often than New.
func (c *Canvas) Reset() {
	for i := range c.bits {
		c.bits[i] = 0
		c.owner[i] = noSeries
	}
}

// Size returns the canvas size in cells.
func (c *Canvas) Size() (width, height int) {
	return c.width, c.height
}

// DotSize returns the canvas size in dots.
func (c *Canvas) DotSize() (width, height int) {
	return c.width * dotsWide, c.height * dotsHigh
}

// set lights one dot and attributes its cell to a series. The last line drawn
// over a cell owns its colour, which means later series sit visually on top -
// the same rule as painting them in order.
func (c *Canvas) set(x, y, seriesIndex int) {
	cellX, cellY := x/dotsWide, y/dotsHigh
	if cellX < 0 || cellX >= c.width || cellY < 0 || cellY >= c.height {
		return
	}

	offset := cellY*c.width + cellX
	c.bits[offset] |= dotBits[x%dotsWide][y%dotsHigh]
	c.owner[offset] = int8(seriesIndex)
}

// line draws a straight segment between two dots.
func (c *Canvas) line(x0, y0, x1, y1, seriesIndex int) {
	steps := max(abs(x1-x0), abs(y1-y0), 1)

	for step := 0; step <= steps; step++ {
		ratio := float64(step) / float64(steps)
		x := float64(x0) + float64(x1-x0)*ratio
		y := float64(y0) + float64(y1-y0)*ratio

		c.set(int(math.Round(x)), int(math.Round(y)), seriesIndex)
	}
}

// Plot draws values across the full width of the canvas, scaled so that low
// sits on the bottom row of dots and high on the top.
//
// A NaN is a bucket that was never filled. The line breaks across it rather
// than interpolating: a gap in the data must look like a gap, because a line
// drawn straight through a missing scrape is a claim prism cannot support.
func (c *Canvas) Plot(seriesIndex int, values []float64, low, high float64) {
	dotWidth, dotHeight := c.DotSize()
	if dotWidth == 0 || dotHeight == 0 || len(values) == 0 {
		return
	}

	span := high - low
	if span <= 0 {
		span = 1
	}

	lastX, lastY, haveLast := 0, 0, false

	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			haveLast = false

			continue
		}

		x := 0
		if len(values) > 1 {
			x = int(math.Round(float64(i) * float64(dotWidth-1) / float64(len(values)-1)))
		}

		normalised := (value - low) / span
		y := int(math.Round(float64(dotHeight-1) * (1 - normalised)))
		y = min(max(y, 0), dotHeight-1)

		if haveLast {
			c.line(lastX, lastY, x, y, seriesIndex)
		} else {
			c.set(x, y, seriesIndex)
		}

		lastX, lastY, haveLast = x, y, true
	}
}

// Rows renders the canvas as one Row per terminal line, coalescing adjacent
// cells that belong to the same series.
func (c *Canvas) Rows() []Row {
	rows := make([]Row, 0, c.height)

	for y := range c.height {
		row := make(Row, 0, 8)
		current := Run{Series: noSeries}

		for x := range c.width {
			offset := y*c.width + x
			owner := int(c.owner[offset])
			glyph := ' '

			if c.bits[offset] != 0 {
				glyph = rune(brailleBase + int(c.bits[offset]))
			}

			if owner != current.Series && current.Text != "" {
				row = append(row, current)
				current = Run{Series: owner}
			}

			current.Series = owner
			current.Text += string(glyph)
		}

		if current.Text != "" {
			row = append(row, current)
		}

		rows = append(rows, row)
	}

	return rows
}

// abs returns the absolute value of an int.
func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
