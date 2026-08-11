package theme

// Appearance owns which palette is in force and everything allowed to change
// it: the configured mode, what the terminal reports about its background, and
// the operator's `d` key.
//
// It lives here rather than in the Bubble Tea model for the same reason the
// window clamping lives in internal/timeline - the precedence rules between
// three sources of truth are exactly the kind of thing that wants a test, and
// a test that needs a terminal does not get written.
type Appearance struct {
	cfg    Config
	dark   bool
	forced bool

	palette Palette
	styles  Styles
}

// NewAppearance returns the appearance a configuration opens with.
//
// A configured mode of dark or light is treated as already forced: the
// operator said which one they wanted, and a terminal answering the background
// query a moment later does not get to disagree.
func NewAppearance(cfg Config) *Appearance {
	appearance := &Appearance{
		cfg:    cfg,
		dark:   cfg.Mode != ModeLight,
		forced: cfg.Mode != ModeAuto,
	}

	appearance.resolve()

	return appearance
}

// Dark reports which side of the fence the current palette sits on.
func (a *Appearance) Dark() bool {
	return a.dark
}

// Palette returns the colours in force.
func (a *Appearance) Palette() Palette {
	return a.palette
}

// Styles returns the styles derived from the current palette.
func (a *Appearance) Styles() Styles {
	return a.styles
}

// TerminalIsDark records what the terminal reported about its background and
// returns whether that changed anything.
//
// The result is what tells a caller to rebuild everything derived from the
// palette. Terminals answer the background query asynchronously and some answer
// it more than once, so a report that changes nothing has to be cheap.
func (a *Appearance) TerminalIsDark(dark bool) bool {
	if a.forced || a.dark == dark {
		return false
	}

	a.dark = dark
	a.resolve()

	return true
}

// Toggle switches to the other palette and stops following the terminal.
//
// Pressing d is a decision. From here on the terminal's opinion is stale, and
// snapping the palette back the next time it reports would read as the toggle
// having failed.
func (a *Appearance) Toggle() {
	a.dark = !a.dark
	a.forced = true

	a.resolve()
}

// resolve rebuilds the palette and the styles. It runs on a change of
// appearance and never per frame: constructing a style allocates, and a chart
// can hold hundreds of rendered spans.
func (a *Appearance) resolve() {
	// The mode is passed as auto because this type has already applied the
	// configured mode and every override on top of it; asking Resolve to apply
	// the mode again would let a forced config win over a later toggle.
	a.palette = Resolve(Config{Mode: ModeAuto}, a.dark)
	a.styles = NewStyles(a.palette)
}
