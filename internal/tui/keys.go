package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
)

// windows are the selectable widths of the time axis, narrowest first. They
// are the round numbers an operator thinks in, not a continuous zoom: landing
// on "4m37s" helps nobody.
var windows = []time.Duration{ //nolint:gochecknoglobals // immutable lookup table
	30 * time.Second,
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
}

// defaultWindow is the index into windows the dashboard opens on.
const defaultWindow = 2

// keymap holds every binding with its help text attached, so the legend and
// the behaviour cannot drift apart.
type keymap struct {
	Up       key.Binding
	Down     key.Binding
	ZoomIn   key.Binding
	ZoomOut  key.Binding
	PanLeft  key.Binding
	PanRight key.Binding
	Live     key.Binding
	Oldest   key.Binding
	Filter   key.Binding
	Pivot    key.Binding
	Theme    key.Binding
	Help     key.Binding
	Quit     key.Binding
}

// newKeymap returns the default bindings.
//
// j/k zoom and h/l pan, which is the one departure from vi habits here: the
// chart is the primary object on screen, so the home row belongs to it. The
// arrows move the selection.
func newKeymap() keymap {
	return keymap{
		Up:       key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓", "select")),
		Down:     key.NewBinding(key.WithKeys("down"), key.WithHelp("↑/↓", "select")),
		ZoomIn:   key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "zoom")),
		ZoomOut:  key.NewBinding(key.WithKeys("k"), key.WithHelp("j/k", "zoom")),
		PanLeft:  key.NewBinding(key.WithKeys("h"), key.WithHelp("h/l", "pan")),
		PanRight: key.NewBinding(key.WithKeys("l"), key.WithHelp("h/l", "pan")),
		Live:     key.NewBinding(key.WithKeys("L", "G"), key.WithHelp("L", "live")),
		Oldest:   key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "oldest")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Pivot:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pivot")),
		Theme:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "theme")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// hints is the footer legend, short enough to fit on one line.
func (k keymap) hints() [][2]string {
	return [][2]string{
		{"j/k", "zoom"},
		{"h/l", "pan"},
		{"↑/↓", "select"},
		{"/", "filter"},
		{"p", "pivot"},
		{"?", "help"},
		{"q", "quit"},
	}
}

// full is the legend shown in the help panel, including the bindings that do
// not earn a place in the footer.
func (k keymap) full() [][2]string {
	return [][2]string{
		{"j / k", "zoom the time window in and out"},
		{"h / l", "pan back and forward through history"},
		{"L", "reattach to live"},
		{"g", "jump to the oldest bucket held"},
		{"↑ / ↓", "select a series"},
		{"/", "filter by label matchers, then enter"},
		{"p", "pivot: split the family across a label"},
		{"d", "switch between the light and dark palettes"},
		{"?", "close this panel"},
		{"q", "quit"},
	}
}
