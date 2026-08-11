// Package theme owns every colour prism draws. Nothing outside this package
// may name a colour: a palette is data, resolved once when the theme changes,
// so that a frame never pays for colour decisions and a light-mode bug can only
// live in one file.
package theme

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Mode selects how the palette is chosen.
type Mode string

const (
	// ModeAuto asks the terminal for its background colour and follows it.
	ModeAuto Mode = "auto"
	// ModeDark forces the dark palette.
	ModeDark Mode = "dark"
	// ModeLight forces the light palette.
	ModeLight Mode = "light"
)

// Or returns the mode if one is set, and the fallback otherwise.
//
// The empty mode means "not specified", which is what lets a command-line flag
// defer to the configuration file by simply not being passed. It is deliberately
// not a valid mode: [Config.Validate] rejects it, so an unset mode can never
// reach a palette by accident.
func (m Mode) Or(fallback Mode) Mode {
	if m == "" {
		return fallback
	}

	return m
}

// Config selects the palette. It is the whole of prism's appearance surface.
type Config struct {
	Mode Mode `yaml:"mode"`
}

// Default returns the configuration used when none is supplied.
func Default() Config {
	return Config{Mode: ModeAuto}
}

// Validate reports whether the configured mode is one prism understands.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeAuto, ModeDark, ModeLight:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownMode, c.Mode)
	}
}

// Palette is the full set of colours for one appearance. Fields are named for
// their role, never for the colour itself, so that swapping palettes cannot
// produce a nonsense pairing.
type Palette struct {
	Background color.Color
	Foreground color.Color
	Muted      color.Color
	Chrome     color.Color
	Accent     color.Color
	Secondary  color.Color
	Warn       color.Color
	OK         color.Color
	Selection  color.Color

	// Spectrum runs violet to red. It colours the banner and, from phase 3,
	// the lines produced by pivoting a metric across a label.
	Spectrum []color.Color

	// Dark records which side of the fence this palette sits on, so callers
	// can pick a matching terminal theme without inspecting colours.
	Dark bool
}

// Resolve returns the palette for the configured mode. terminalIsDark is the
// terminal's reported background, used only when the mode is [ModeAuto].
func Resolve(cfg Config, terminalIsDark bool) Palette {
	var dark bool

	switch cfg.Mode {
	case ModeDark:
		dark = true
	case ModeLight:
		dark = false
	default:
		// ModeAuto, and anything Validate would already have rejected: follow
		// the terminal, which is the only safe answer without an instruction.
		dark = terminalIsDark
	}

	pick := lipgloss.LightDark(dark)

	return Palette{
		Background: pick(lipgloss.Color("#fbf9ff"), lipgloss.Color("#141220")),
		Foreground: pick(lipgloss.Color("#2a2440"), lipgloss.Color("#dcd8ec")),
		Muted:      pick(lipgloss.Color("#7d7699"), lipgloss.Color("#6e6890")),
		Chrome:     pick(lipgloss.Color("#cdc4e4"), lipgloss.Color("#3b3358")),
		Accent:     pick(lipgloss.Color("#6a35d9"), lipgloss.Color("#b98bff")),
		Secondary:  pick(lipgloss.Color("#0f8f7a"), lipgloss.Color("#4fe3c8")),
		Warn:       pick(lipgloss.Color("#b06a00"), lipgloss.Color("#ffb454")),
		OK:         pick(lipgloss.Color("#1a7f37"), lipgloss.Color("#7ee787")),
		Selection:  pick(lipgloss.Color("#ece5fb"), lipgloss.Color("#2a2340")),
		Spectrum:   spectrum(pick),
		Dark:       dark,
	}
}

// spectrum returns the five wavelengths of the split beam, violet through red.
func spectrum(pick lipgloss.LightDarkFunc) []color.Color {
	return []color.Color{
		pick(lipgloss.Color("#6d3ad6"), lipgloss.Color("#9b6dff")),
		pick(lipgloss.Color("#2f5fd0"), lipgloss.Color("#5b8dff")),
		pick(lipgloss.Color("#0f9c88"), lipgloss.Color("#3fe0d0")),
		pick(lipgloss.Color("#c07d00"), lipgloss.Color("#ffc14d")),
		pick(lipgloss.Color("#d13060"), lipgloss.Color("#ff5f8f")),
	}
}

// Styles holds the lipgloss styles derived from a palette. They are built once
// per theme change rather than per frame, because constructing a style
// allocates and a chart can hold hundreds of rendered spans.
type Styles struct {
	Base      lipgloss.Style
	Title     lipgloss.Style
	Muted     lipgloss.Style
	Chrome    lipgloss.Style
	Accent    lipgloss.Style
	Secondary lipgloss.Style
	Key       lipgloss.Style
	OK        lipgloss.Style
	Selected  lipgloss.Style
}

// NewStyles derives the style set for a palette.
func NewStyles(palette Palette) Styles {
	base := lipgloss.NewStyle().Foreground(palette.Foreground)

	return Styles{
		Base:      base,
		Title:     base.Foreground(palette.Accent).Bold(true),
		Muted:     base.Foreground(palette.Muted),
		Chrome:    base.Foreground(palette.Chrome),
		Accent:    base.Foreground(palette.Accent),
		Secondary: base.Foreground(palette.Secondary),
		Key:       base.Foreground(palette.Warn).Bold(true),
		OK:        base.Foreground(palette.OK),
		Selected:  base.Foreground(palette.Accent).Background(palette.Selection).Bold(true),
	}
}
