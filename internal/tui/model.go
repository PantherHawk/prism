package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/banner"
	"github.com/pantherhawk/prism/internal/chart"
	"github.com/pantherhawk/prism/internal/filter"
	"github.com/pantherhawk/prism/internal/motion"
	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/theme"
	"github.com/pantherhawk/prism/internal/timeline"
)

// mode is what the keyboard is currently driving.
type mode uint8

const (
	// modeNormal drives the chart.
	modeNormal mode = iota
	// modeFilter drives the filter bar.
	modeFilter
)

const (
	// refreshInterval is how often the model picks up a new snapshot. It is
	// faster than the scrape interval so the status clock moves, and far
	// slower than the frame rate because there is nothing new to see between
	// scrapes.
	refreshInterval = 250 * time.Millisecond

	// splashFor is how long the banner stays up before the dashboard replaces
	// it.
	splashFor = 1200 * time.Millisecond

	// The vertical scale is critically damped: overshoot on an axis would make
	// a value appear to cross a threshold it never crossed. It settles in
	// about a second, which is deliberately unhurried - the axis is following
	// the data, not responding to a keystroke.
	scaleFrequency = 12.0
	scaleDamping   = 1.0

	// The help panel is quick, with just enough bounce to read as a panel
	// arriving rather than a repaint.
	helpFrequency = 18.0
	helpDamping   = 0.85
)

type (
	// tickMsg asks the model to pick up the latest snapshot.
	tickMsg time.Time

	// frameMsg advances every animation by one frame.
	frameMsg time.Time

	// dismissSplashMsg retires the banner.
	dismissSplashMsg struct{}
)

// model is the dashboard.
//
// Bubble Tea copies the model on every update, so the springs are held by
// pointer: they are one continuous physical simulation, not a value that gets
// forked and rejoined sixty times a second.
type model struct {
	info   banner.Info
	warnAt int
	store  *series.Store
	keys   keymap

	appearance *theme.Appearance
	palette    theme.Palette
	styles     theme.Styles
	table      []lipgloss.Style

	snapshot *series.Snapshot
	timeline *timeline.Timeline

	mode      mode
	input     textinput.Model
	active    *filter.Filter
	filterErr string
	pivotKey  string

	low     *motion.Spring
	high    *motion.Spring
	help    *motion.Spring
	springs *motion.Group

	width     int
	height    int
	selected  int
	splash    bool
	animating bool
	scaled    bool
	showHelp  bool
}

// newModel returns the initial model.
func newModel(cfg theme.Config, info banner.Info, warnAt int, store *series.Store) model {
	appearance := theme.NewAppearance(cfg)
	palette := appearance.Palette()
	styles := appearance.Styles()

	low := motion.NewSpring(scaleFrequency, scaleDamping)
	high := motion.NewSpring(scaleFrequency, scaleDamping)
	help := motion.NewSpring(helpFrequency, helpDamping)

	snapshot := store.Snapshot()

	return model{
		info:       info,
		warnAt:     warnAt,
		store:      store,
		keys:       newKeymap(),
		appearance: appearance,
		palette:    palette,
		styles:     styles,
		table:      buildStyles(styles, palette),
		snapshot:   snapshot,
		timeline:   timeline.New(windows, defaultWindow, snapshot.Resolution, snapshot.Capacity),
		input:      newInput(),
		active:     &filter.Filter{},
		low:        low,
		high:       high,
		help:       help,
		springs:    motion.NewGroup(low, high, help),
		splash:     true,
	}
}

// Init asks the terminal for its background colour, starts the refresh ticker,
// and schedules the end of the splash screen.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		refresh(),
		tea.Tick(splashFor, func(time.Time) tea.Msg { return dismissSplashMsg{} }),
	)
}

// refresh schedules the next snapshot pickup.
func refresh() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// nextFrame schedules the next animation frame. The name keeps out of the way
// of [frame], the character grid a render composes into.
func nextFrame() tea.Cmd {
	return tea.Tick(time.Second/motion.Rate, func(t time.Time) tea.Msg { return frameMsg(t) })
}

// Update folds a message into the model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		return m, nil

	case tea.BackgroundColorMsg:
		if !m.appearance.TerminalIsDark(msg.IsDark()) {
			return m, nil
		}

		return m.restyled(), nil

	case dismissSplashMsg:
		m.splash = false

		return m, nil

	case tickMsg:
		m.snapshot = m.store.Snapshot()
		m.timeline.Resize(m.snapshot.Resolution, m.snapshot.Capacity)
		m.clampSelection()
		m.retarget()

		return m, tea.Batch(refresh(), m.animate())

	case frameMsg:
		// Both must run: short-circuiting would freeze one group whenever the
		// other settled first.
		moving := m.timeline.Update()
		m.animating = m.springs.Update() || moving

		if m.animating {
			return m, nextFrame()
		}

		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)
	}

	return m, nil
}

// onKey handles a keypress.
func (m model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// ctrl-c is checked before anything else: it has to work from inside a
	// half-typed filter as well as from the dashboard.
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.mode == modeFilter {
		return m.onFilterKey(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case m.splash:
		// Nothing behind the splash can be acted on yet, so swallowing the
		// keystroke is kinder than applying it blind.
		m.splash = false

		return m, nil

	case key.Matches(msg, m.keys.Filter):
		return m.beginFilter()

	case key.Matches(msg, m.keys.Pivot):
		m = m.cyclePivot()

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.SetTarget(boolTo(m.showHelp))

	case key.Matches(msg, m.keys.Down):
		m.selected++
		m.clampSelection()
		m.retarget()

	case key.Matches(msg, m.keys.Up):
		m.selected--
		m.clampSelection()
		m.retarget()

	case key.Matches(msg, m.keys.ZoomIn):
		m.timeline.ZoomIn()
		m.retarget()

	case key.Matches(msg, m.keys.ZoomOut):
		m.timeline.ZoomOut()
		m.retarget()

	case key.Matches(msg, m.keys.PanLeft):
		m.timeline.PanLeft()
		m.retarget()

	case key.Matches(msg, m.keys.PanRight):
		m.timeline.PanRight()
		m.retarget()

	case key.Matches(msg, m.keys.Live):
		m.timeline.ToLive()
		m.retarget()

	case key.Matches(msg, m.keys.Oldest):
		m.timeline.ToOldest()
		m.retarget()

	case key.Matches(msg, m.keys.Theme):
		m.appearance.Toggle()

		return m.restyled(), nil
	}

	return m, m.animate()
}

// animate starts the frame ticker if anything has begun moving and it is not
// already running. Exactly one frame command may be in flight: two would double
// the integration rate and the physics would run at twice the intended speed.
func (m *model) animate() tea.Cmd {
	if m.animating || !(m.timeline.Moving() || m.springs.Moving()) {
		return nil
	}

	m.animating = true

	return nextFrame()
}

// retarget re-aims the vertical scale at the data the target window covers.
//
// It reads the target window rather than the animated one deliberately. If the
// scale chased the animated window, the two springs would pursue each other and
// the axis would keep drifting after the zoom had visibly finished.
func (m *model) retarget() {
	lines := m.lines()
	if m.snapshot == nil || len(lines) == 0 {
		return
	}

	buckets := min(m.snapshot.BucketsIn(m.timeline.Window()), m.snapshot.Capacity)
	latest := m.snapshot.Newest - int64(m.timeline.Offset())
	oldest := latest - int64(buckets) + 1

	plots := make([][]float64, 0, len(lines))
	for _, line := range lines {
		plots = append(plots, line.Window(oldest, latest))
	}

	low, high := chart.Bounds(plots...)

	if !m.scaled {
		// The first scale is a fact, not a change. Animating up from zero
		// would open every session with a swoop nobody asked for.
		m.low.Snap(low)
		m.high.Snap(high)
		m.scaled = true

		return
	}

	m.low.SetTarget(low)
	m.high.SetTarget(high)
}

// restyled picks up the appearance's palette and everything derived from it.
//
// Bubble Tea copies the model on every update, so the resolved palette is
// cached on the copy rather than fetched per draw: a frame asks for a colour
// thousands of times.
func (m model) restyled() model {
	m.palette = m.appearance.Palette()
	m.styles = m.appearance.Styles()
	m.table = buildStyles(m.styles, m.palette)

	return m
}

// clampSelection keeps the cursor inside the series list as it grows and
// shrinks under it.
func (m *model) clampSelection() {
	count := len(m.matching())

	if count == 0 {
		m.selected = 0

		return
	}

	m.selected = min(max(m.selected, 0), count-1)
}

// selection describes what the chart should show.
func (m model) selection() series.Selection {
	sel := series.Selection{
		Match:    m.active.Match,
		PivotKey: m.pivotKey,
		Limit:    maxPlotted,
	}

	// A pivot spans the whole family; without one, the chart shows the family
	// the cursor is in.
	if selected := m.selectedSeries(); selected != nil {
		sel.Family = selected.Family
	}

	return sel
}

// matching returns the series the filter admits, which is what the series
// panel lists and what the cursor moves through.
func (m model) matching() []series.SeriesView {
	if m.snapshot == nil {
		return nil
	}

	return m.snapshot.Matching(series.Selection{Match: m.active.Match})
}

// lines returns the lines to plot.
func (m model) lines() []series.Line {
	if m.snapshot == nil {
		return nil
	}

	return m.snapshot.Lines(m.selection())
}

// boolTo maps a flag onto the spring target that represents it.
func boolTo(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
