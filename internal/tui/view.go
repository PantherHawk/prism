package tui

import (
	"fmt"
	"math"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/banner"
	"github.com/pantherhawk/prism/internal/chart"
	"github.com/pantherhawk/prism/internal/series"
)

const (
	// minWidth and minHeight are the smallest terminal the dashboard will draw
	// in. Below this it says so rather than rendering something misleading.
	minWidth  = 60
	minHeight = 18

	// Column geometry of the chart gutter.
	axisLabelRight = 6
	axisColumn     = 7
	plotColumn     = 9

	// panelRows is how many entries the series and cardinality panels show.
	panelRows = 3

	// chromeRows is every row of the dashboard that is not the plot or the
	// panel body.
	chromeRows = 9

	// maxPlotted caps how many series are drawn at once. Beyond a handful of
	// lines a chart stops being readable, and the answer is to filter, not to
	// draw more.
	maxPlotted = 5

	// sparkWidth is the width of a cardinality sparkline.
	sparkWidth = 8
)

// View renders the current frame.
func (m model) View() tea.View {
	view := tea.NewView(m.content())

	view.AltScreen = true
	view.BackgroundColor = m.palette.Background
	view.WindowTitle = "prism"

	return view
}

// content renders the body of the frame.
func (m model) content() string {
	switch {
	case m.width < minWidth || m.height < minHeight:
		return m.centred(fmt.Sprintf("terminal too small: need %dx%d", minWidth, minHeight))
	case m.splash:
		return m.centred(banner.Render(m.info, m.palette, m.styles, m.width, m.height))
	default:
		return m.dashboard()
	}
}

// centred places a block in the middle of the screen.
func (m model) centred(body string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

// dashboard draws the full interface.
func (m model) dashboard() string {
	f := newFrame(m.width, m.height, m.table)

	var (
		right    = m.width - 1
		bottom   = m.height - 1
		rows     = m.panelHeight()
		chartTop = 2
		chartH   = max(m.height-chromeRows-rows, 3)
		axisRow  = chartTop + chartH
		timesRow = axisRow + 1
		panelRow = timesRow + 2
		footRow  = panelRow + rows + 1
	)

	m.drawChrome(f, right, bottom)
	m.drawHeader(f, right)
	m.drawChart(f, chartTop, chartH, right)
	m.drawAxis(f, axisRow, timesRow, right)
	m.drawPanels(f, panelRow, rows, right)
	m.drawFooter(f, footRow, right)

	// The help panel is drawn last so that it sits over everything else.
	m.drawHelp(f, right, bottom)

	return f.String()
}

// drawChrome draws the outer box and the title bar.
func (m model) drawChrome(f *frame, right, bottom int) {
	f.fill(0, 0, right, '─', styleChrome)
	f.fill(bottom, 0, right, '─', styleChrome)
	f.put(0, 0, "╭", styleChrome)
	f.put(0, right, "╮", styleChrome)
	f.put(bottom, 0, "╰", styleChrome)
	f.put(bottom, right, "╯", styleChrome)
	f.vline(0, 1, bottom-1, '│', styleChrome)
	f.vline(right, 1, bottom-1, '│', styleChrome)

	f.put(0, 2, " ⬢ prism ", styleAccent)

	state, style := m.liveState()
	f.rput(0, right-2, state, style)
	f.rput(0, right-2-len([]rune(state)), " "+m.source()+" · ", styleMuted)
}

// liveState describes where the data is coming from and whether it is current.
//
// Detached is a state the operator chose, so it is reported plainly rather than
// as a warning. Reconnecting and stalled are states the network imposed, so they
// are not.
func (m model) liveState() (string, uint8) {
	stats := series.Stats{}
	if m.snapshot != nil {
		stats = m.snapshot.Stats
	}

	switch {
	case !m.timeline.Live():
		return fmt.Sprintf("❚❚ %s behind ", clock(m.timeline.Behind())), styleAccent
	case stats.Reconnecting:
		return fmt.Sprintf("⟳ reconnecting %s ", clock(stats.RetryIn)), styleKey
	case stats.LastError != "":
		return "● stalled ", styleKey
	default:
		return "● live ", styleOK
	}
}

// source names where samples are coming from, preferring what the feed reports
// over what the configuration said, so that following an upstream is visible.
func (m model) source() string {
	if m.snapshot != nil && m.snapshot.Stats.Source != "" {
		return m.snapshot.Stats.Source
	}

	return m.info.Endpoint
}

// windowMarker labels the window strip.
const windowMarker = "⌁ window"

// drawHeader draws the metric name and the window selector.
//
// The two halves grow towards each other: the name runs left to right and has
// no length a metric is obliged to stay under, while the strip is laid out from
// the right edge back. They used to be drawn independently, which meant that on
// a narrow enough terminal the name simply continued into the strip's cells and
// the later writer won, character by character. The result was not a truncated
// name or a clipped strip but an interleaving of both - `⌁ windowrk30si[ 1m ]`
// in the frame that caught it - and it read as text something had meant to
// write.
//
// So the strip is measured before anything is drawn, and the name is given what
// is left over. Measuring first is what makes the strip's layout independent of
// the name, which is the property TestHeaderNeverOverwritesTheWindowStrip pins.
func (m model) drawHeader(f *frame, right int) {
	steps := m.timeline.Steps()
	labels := make([]string, len(steps))

	// The window strip reads as a row of options with the active one bracketed,
	// so the zoom keys have somewhere obvious to point.
	col := right - 1

	for i, step := range slices.Backward(steps) {
		labels[i] = short(step)
		if i == m.timeline.Index() {
			labels[i] = "[ " + labels[i] + " ]"
		}

		col -= len([]rune(labels[i])) + 1
	}

	// col now sits on the first column of the strip. The marker ends two short
	// of it, and the name has to stop one short of the marker so that the two
	// halves never sit flush against each other.
	start := col
	limit := start - 2 - len([]rune(windowMarker)) - 1

	m.drawHeaderName(f, limit)

	for i, label := range labels {
		style := styleMuted
		if i == m.timeline.Index() {
			style = styleAccent
		}

		col = f.put(1, col, label, style) + 1
	}

	f.rput(1, start-2, windowMarker, styleMuted)
}

// nameFloor is the fewest columns the metric name keeps when the rest of the
// header would otherwise squeeze it out.
const nameFloor = 16

// drawHeaderName draws the left half of the header - what is being charted -
// into the columns before limit, and nothing at or past it.
//
// The parts are not equal claimants on the room. Bounding the half was enough
// to stop it corrupting the window strip, but drawing it first come first
// served then handed everything to the family name, which is the longest part
// by a distance and the least informative per column: forty columns spent on
// envoy_listener_worker_downstream_cx_total, and `b…` where the pivot key
// should be.
//
// So the short parts reserve their room first and the name takes what is left,
// down to a floor. A cut name beside a legible `by envoy_worker_id (6)` says
// more than a whole name beside nothing, and the part that got cut is the part
// whose tail is the most predictable.
func (m model) drawHeaderName(f *frame, limit int) {
	selected := m.selectedSeries()
	lines := m.lines()

	// "waiting for first scrape" is only true before the first scrape. Once one
	// has landed, having nothing to chart means the filter excluded everything
	// or the target exposed nothing, and saying we are still waiting puts the
	// header in contradiction with the body of its own frame - the recorded
	// p3-empty had `waiting for first scra…` over `nothing matches
	// envoy_worker_id=~99`, forty-three scrapes in.
	var name, unit string

	scraped := m.snapshot != nil && m.snapshot.Stats.Scrapes > 0

	switch {
	case selected != nil:
		name, unit = selected.Family, selected.Kind.Unit()
	case scraped && !m.active.IsZero():
		name = "no match"
	case scraped:
		name = "no series"
	default:
		name = "waiting for first scrape"
	}

	count := fmt.Sprintf("%d series", len(lines))
	if m.pivotKey != "" {
		count = fmt.Sprintf("(%d)", len(lines))
	}

	// A gap of two columns before the unit and two more before what follows it,
	// plus "by " and the key and a space when there is a pivot.
	const gaps = 4

	tail := len([]rune(unit)) + gaps + len([]rune(count))
	if m.pivotKey != "" {
		tail += len("by ") + len([]rune(m.pivotKey)) + 1
	}

	col := f.putUpTo(1, 2, max(limit-tail, 2+nameFloor), name, styleAccent)
	col = f.putUpTo(1, col+2, limit, unit, styleMuted)

	if m.pivotKey == "" {
		f.putUpTo(1, col+2, limit, count, styleSecondary)

		return
	}

	col = f.putUpTo(1, col+2, limit, "by ", styleMuted)
	col = f.putUpTo(1, col, limit, m.pivotKey, styleSecondary)
	f.putUpTo(1, col+1, limit, count, styleMuted)
}

// drawChart plots the selected family.
func (m model) drawChart(f *frame, top, height, right int) {
	lines := m.lines()
	if len(lines) == 0 || m.snapshot == nil {
		m.drawEmpty(f, top, height)

		return
	}

	width := right - 1 - plotColumn
	if width < 1 {
		return
	}

	oldest, latest := m.timeline.Range(m.snapshot.Newest)

	plots := make([][]float64, 0, len(lines))
	for _, line := range lines {
		plots = append(plots, line.Window(oldest, latest))
	}

	low, high := m.scale(plots)

	canvas := chart.New(width, height)
	for i, values := range plots {
		canvas.Plot(i, values, low, high)
	}

	for i, row := range canvas.Rows() {
		col := plotColumn

		for _, run := range row {
			style := styleBase
			if run.Series >= 0 {
				style = spectrumStyle(run.Series, len(m.palette.Spectrum))
			}

			col = f.put(top+i, col, run.Text, style)
		}
	}

	m.drawYAxis(f, top, height, low, high)
}

// drawEmpty explains why the chart is blank.
//
// A filter that matches nothing and a target that has not been scraped yet look
// identical on an empty chart, and the fix for each is completely different.
func (m model) drawEmpty(f *frame, top, height int) {
	row := top + height/2

	if m.active.IsZero() {
		f.put(row, plotColumn, "no samples yet", styleMuted)

		return
	}

	col := f.put(row, plotColumn, "nothing matches ", styleMuted)
	f.put(row, col, m.active.String(), styleAccent)
	f.put(row+1, plotColumn, "press / to edit the filter", styleMuted)
}

// scale returns the vertical range to draw with.
//
// It is the animated value, so a spike rescales the axis by gliding rather than
// teleporting and the eye keeps its place. The fallback covers the frames
// before the first snapshot has been measured.
func (m model) scale(plots [][]float64) (low, high float64) {
	low, high = m.low.Value(), m.high.Value()
	if high > low && !math.IsNaN(low) && !math.IsNaN(high) {
		return low, high
	}

	return chart.Bounds(plots...)
}

// drawYAxis labels the vertical scale.
func (m model) drawYAxis(f *frame, top, height int, low, high float64) {
	const labelEvery = 2

	for row := range height {
		if row%labelEvery != 0 {
			continue
		}

		ratio := 1 - float64(row)/float64(max(height-1, 1))
		f.rput(top+row, axisLabelRight, chart.Format(low+(high-low)*ratio), styleMuted)
		f.put(top+row, axisColumn, "┤", styleChrome)
	}
}

// drawAxis draws the time axis and its labels.
func (m model) drawAxis(f *frame, axisRow, timesRow, right int) {
	f.put(axisRow, axisColumn, "└", styleChrome)
	f.fill(axisRow, axisColumn+1, right-1, '─', styleChrome)

	const ticks = 5

	span := m.timeline.AnimatedWindow()
	behind := m.timeline.Behind()
	width := right - 1 - plotColumn

	step := span / (ticks - 1)

	for tick := range ticks {
		// Labels are ages, not clock times: on a live chart "-2m" stays true
		// as the window slides, where "14:03" would need redrawing every tick.
		age := behind + span - time.Duration(tick)*step

		label := tickLabel(age, step)

		if tick == ticks-1 {
			f.rput(timesRow, right-1, label, styleMuted)

			continue
		}

		f.put(timesRow, plotColumn+tick*width/(ticks-1), label, styleMuted)
	}
}

// panelHeight is how many rows the lower panels get.
//
// While pivoting, the panel is the legend, and it has to be tall enough for
// every line on the chart. A legend that lists three of five lines leaves two
// colours on the chart unexplained, which is worse than a shorter plot.
func (m model) panelHeight() int {
	if m.pivotKey != "" {
		return maxPlotted
	}

	return panelRows
}

// drawPanels draws the series list and the cardinality table.
func (m model) drawPanels(f *frame, top, rows, right int) {
	mid := m.width / 2

	title := " series "
	if m.pivotKey != "" {
		title = " by " + m.pivotKey + " "
	}

	f.fill(top-1, 0, right, '─', styleChrome)
	f.put(top-1, 0, "├", styleChrome)
	f.put(top-1, right, "┤", styleChrome)
	f.put(top-1, mid, "┬", styleChrome)
	f.put(top-1, 2, title, styleMuted)
	f.put(top-1, mid+2, " cardinality ", styleMuted)
	f.vline(mid, top, top+rows-1, '│', styleChrome)

	m.drawSeriesPanel(f, top, rows, mid)
	m.drawCardinalityPanel(f, top, rows, mid, right)

	f.fill(top+rows, 0, right, '─', styleChrome)
	f.put(top+rows, 0, "├", styleChrome)
	f.put(top+rows, right, "┤", styleChrome)
	f.put(top+rows, mid, "┴", styleChrome)
}

// drawSeriesPanel lists the series around the selection.
func (m model) drawSeriesPanel(f *frame, top, rows, mid int) {
	if m.pivotKey != "" {
		m.drawLegend(f, top, rows, mid)

		return
	}

	matching := m.matching()
	if len(matching) == 0 {
		f.put(top, 2, "no series", styleMuted)

		return
	}

	// The window scrolls with the selection rather than paging, so the cursor
	// never jumps to the far edge of the list under the operator.
	first := min(max(m.selected-rows/2, 0), max(len(matching)-rows, 0))

	for row := range rows {
		i := first + row
		if i >= len(matching) {
			return
		}

		view := matching[i]
		style, marker := styleBase, " "

		if i == m.selected {
			style, marker = styleSelected, "▸"
		}

		f.put(top+row, 2, marker, styleAccent)
		f.put(top+row, 4, truncate(view.Labels.String(), mid-12), style)
		f.rput(top+row, mid-2, chart.Format(view.Last), styleSecondary)
	}
}

// drawLegend lists the pivot groups, coloured to match their lines.
//
// While pivoting, this panel is the legend: without it the spectrum on the
// chart is decoration rather than information.
func (m model) drawLegend(f *frame, top, rows, mid int) {
	lines := m.lines()

	for row := range rows {
		if row >= len(lines) {
			return
		}

		line := lines[row]
		style := spectrumStyle(row, len(m.palette.Spectrum))

		f.put(top+row, 2, "━", style)

		label := line.Label
		if line.Members > 1 {
			label += fmt.Sprintf(" ×%d", line.Members)
		}

		f.put(top+row, 4, truncate(label, mid-12), styleBase)
		f.rput(top+row, mid-2, chart.Format(line.Last), styleSecondary)
	}
}

// drawCardinalityPanel lists the widest families and how they are growing.
func (m model) drawCardinalityPanel(f *frame, top, rows, mid, right int) {
	if m.snapshot == nil || len(m.snapshot.Families) == 0 {
		f.put(top, mid+2, "no families", styleMuted)

		return
	}

	for row := range rows {
		if row >= len(m.snapshot.Families) {
			return
		}

		family := m.snapshot.Families[row]
		meta, style := m.familyMeta(family)

		width := right - mid - sparkWidth - 6 - len([]rune(meta))

		f.put(top+row, mid+2, truncate(family.Name, width), styleBase)
		f.rput(top+row, right-sparkWidth-3, meta, style)
		f.put(top+row, right-sparkWidth-1, chart.Sparkline(family.History, sparkWidth), style)
	}
}

// familyMeta renders a family's cardinality with everything the number needs to
// be read correctly: a tilde when it is an estimate, the sampling ratio when
// the chart is drawn from a subset, and a badge when it is over the threshold.
//
// A bare number here would be three different claims wearing the same clothes.
func (m model) familyMeta(family series.FamilyView) (string, uint8) {
	meta := chart.Format(float64(family.Cardinality))
	if family.Estimated {
		meta = "~" + meta
	}

	if family.Sampled() {
		meta += fmt.Sprintf(" 1:%d", family.Ratio)
	}

	if m.warnAt > 0 && family.Cardinality > m.warnAt {
		return meta + " !", styleKey
	}

	return meta, styleSecondary
}

// drawFooter draws the status line and the key legend.
//
// The status is right-aligned and the filter bar grows towards it, so the
// status is measured and placed first and the filter bar is told where it ends.
// This row had the same defect the header did: a long filter or pivot name ran
// straight through the status and left `envoy_wor10/553 series` behind.
func (m model) drawFooter(f *frame, top, right int) {
	limit := right - 1

	if m.snapshot != nil && m.mode != modeFilter {
		status := m.statusLine()
		f.rput(top, right-2, status, styleMuted)

		limit = right - 2 - len([]rune(status)) - 1
	}

	m.drawFilterBar(f, top, limit)

	col := 2
	for _, hint := range m.keys.hints() {
		col = f.put(top+1, col, hint[0]+" ", styleKey)
		col = f.put(top+1, col, hint[1]+"   ", styleMuted)
	}
}

// drawFilterBar draws the filter field, editable or not, within the columns
// before limit. The caller owns everything from limit rightwards.
func (m model) drawFilterBar(f *frame, top, limit int) {
	col := f.put(top, 2, filterPrompt, styleKey)

	if m.mode == modeFilter {
		// The reason a filter would not compile is right-aligned and the text
		// being typed grows towards it, so the reason is placed first and the
		// text is bounded by it. A filter long enough to reach the reason is
		// exactly the filter most likely to have something wrong with it.
		if m.filterErr != "" {
			reason := truncate(m.filterErr, limit/2)
			f.rput(top, limit, reason, styleKey)

			limit -= len([]rune(reason)) + 1
		}

		text := m.input.Value()
		if text == "" {
			f.putUpTo(top, col, limit, m.input.Placeholder, styleChrome)
		} else {
			f.putUpTo(top, col, limit, text, styleBase)
		}

		// The cursor is drawn as a styled cell rather than left to the
		// terminal, because the whole frame is composed before it is emitted.
		f.putUpTo(top, col+min(m.input.Position(), max(limit-col-1, 0)), limit,
			cursorCell(text, m.input.Position()), styleSelected)

		return
	}

	if m.active.IsZero() {
		col = f.putUpTo(top, col, limit, "no filter", styleChrome)
	} else {
		col = f.putUpTo(top, col, limit, m.active.String(), styleAccent)
	}

	col = f.putUpTo(top, col+3, limit, "p ", styleKey)
	col = f.putUpTo(top, col, limit, "pivot ", styleMuted)

	if m.pivotKey == "" {
		f.putUpTo(top, col, limit, "none", styleChrome)
	} else {
		f.putUpTo(top, col, limit, m.pivotKey, styleSecondary)
	}
}

// cursorCell returns the character under the cursor, or a space at the end of
// the line so that the block is always visible.
func cursorCell(text string, position int) string {
	runes := []rune(text)
	if position < 0 || position >= len(runes) {
		return " "
	}

	return string(runes[position])
}

// statusLine summarises the collector's health and the state of the window.
func (m model) statusLine() string {
	stats := m.snapshot.Stats

	matching := len(m.matching())

	status := fmt.Sprintf("%d series · %d scrapes · %s",
		matching, stats.Scrapes, stats.LastLatency.Round(time.Millisecond))

	// When a filter is narrowing the set, say what it is narrowing from.
	if !m.active.IsZero() {
		status = fmt.Sprintf("%d/%d series · %d scrapes · %s",
			matching, len(m.snapshot.Series), stats.Scrapes,
			stats.LastLatency.Round(time.Millisecond))
	}

	if stats.Errors > 0 {
		status += fmt.Sprintf(" · %d errors", stats.Errors)
	}

	// If anything on screen is a subset, the status bar says so. A chart drawn
	// from a sample that does not admit to being one is the single most
	// dishonest thing this program could do.
	if ratio := m.snapshot.SamplingRatio(); ratio > 1 {
		status += fmt.Sprintf(" · sampled 1:%d", ratio)
	}

	// Saying why zooming out stopped responding is cheaper than letting the
	// operator press k four more times wondering what is broken.
	if m.timeline.AtHorizon() {
		status += " · buffer horizon"
	}

	return status
}

// drawHelp slides the key reference up from the bottom edge.
func (m model) drawHelp(f *frame, right, bottom int) {
	entries := m.keys.full()
	full := len(entries) + 3

	height := int(math.Round(m.help.Value() * float64(full)))
	if height <= 1 {
		return
	}

	top := bottom - height + 1

	// The panel is clipped by its own top edge as it rises, so the contents
	// appear to slide out from behind the footer rather than fade in.
	f.fill(top, 0, right, '─', styleChrome)
	f.put(top, 0, "├", styleChrome)
	f.put(top, right, "┤", styleChrome)
	f.put(top, 2, " keys ", styleAccent)

	for row := 1; row < height; row++ {
		f.fill(top+row, 1, right-1, ' ', styleBase)
		f.put(top+row, 0, "│", styleChrome)
		f.put(top+row, right, "│", styleChrome)

		i := row - 1
		if i >= len(entries) {
			continue
		}

		f.rput(top+row, 8, entries[i][0], styleKey)
		f.put(top+row, 11, entries[i][1], styleMuted)
	}

	if height > len(entries)+1 {
		f.fill(bottom, 0, right, '─', styleChrome)
		f.put(bottom, 0, "╰", styleChrome)
		f.put(bottom, right, "╯", styleChrome)
	}
}

// selectedSeries returns the series under the cursor.
//
// The cursor moves through the filtered set, not the whole snapshot, so that
// narrowing the filter cannot leave the selection pointing at something the
// operator can no longer see.
func (m model) selectedSeries() *series.SeriesView {
	matching := m.matching()
	if len(matching) == 0 {
		return nil
	}

	view := matching[min(m.selected, len(matching)-1)]

	return &view
}

// tickLabel renders the age of a tick at a precision the spacing can carry.
//
// The window selector's coarse form is wrong here. At a two and a half minute
// window, rounding every tick to whole minutes prints "-1m" twice in a row and
// the axis stops meaning anything.
func tickLabel(age, step time.Duration) string {
	if age <= 0 {
		return "now"
	}

	if step < time.Minute {
		return fmt.Sprintf("-%ds", int(age.Round(time.Second).Seconds()))
	}

	return "-" + clock(age)
}

// clock renders a duration as the smallest unit pair that stays unambiguous.
//
// The window selector's coarse form is fine for a label the operator picked,
// but everywhere prism reports a measured distance it has to agree with the
// axis, or the header and the ticks quietly contradict each other.
func clock(d time.Duration) string {
	d = d.Round(time.Second)

	switch {
	case d >= time.Hour:
		hours := int(d / time.Hour)
		minutes := int((d % time.Hour) / time.Minute)

		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}

		return fmt.Sprintf("%dh%02dm", hours, minutes)

	case d >= time.Minute:
		minutes := int(d / time.Minute)
		seconds := int((d % time.Minute) / time.Second)

		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}

		return fmt.Sprintf("%dm%02ds", minutes, seconds)

	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// short renders a duration the way the window selector shows it.
func short(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// truncate shortens text to fit a column, marking where it was cut.
func truncate(text string, width int) string {
	// No room is not the same as no limit. Returning the text whole here would
	// hand a caller with nothing to spare the longest string it could have
	// been given, which is how a panel comes to overwrite its neighbour.
	if width <= 0 {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= width {
		return text
	}

	return string(runes[:width-1]) + "…"
}
