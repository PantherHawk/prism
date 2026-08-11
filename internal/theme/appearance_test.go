package theme_test

import (
	"testing"

	"github.com/pantherhawk/prism/internal/theme"
)

func TestAppearanceStartsFromTheConfiguredMode(t *testing.T) {
	t.Parallel()

	if !theme.NewAppearance(theme.Config{Mode: theme.ModeDark}).Dark() {
		t.Error("a dark configuration did not open dark")
	}

	if theme.NewAppearance(theme.Config{Mode: theme.ModeLight}).Dark() {
		t.Error("a light configuration did not open light")
	}
}

func TestAutoFollowsWhatTheTerminalReports(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeAuto})

	if changed := appearance.TerminalIsDark(false); !changed {
		t.Error("the first terminal report was not treated as a change")
	}

	if appearance.Dark() {
		t.Error("auto did not follow a light terminal")
	}

	appearance.TerminalIsDark(true)

	if !appearance.Dark() {
		t.Error("auto did not follow a dark terminal")
	}
}

// A terminal report arrives asynchronously, and on some terminals more than
// once. Re-resolving the palette when nothing changed would rebuild every style
// in the middle of a frame for no reason.
func TestARepeatedTerminalReportIsNotAChange(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeAuto})
	appearance.TerminalIsDark(true)

	if appearance.TerminalIsDark(true) {
		t.Error("an unchanged terminal report was reported as a change")
	}
}

// --dark and --light are instructions, not preferences. A terminal that answers
// the background query afterwards must not undo what the operator asked for.
func TestAConfiguredModeIgnoresTheTerminal(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeDark})

	if changed := appearance.TerminalIsDark(false); changed {
		t.Error("a terminal report overrode an explicitly configured mode")
	}

	if !appearance.Dark() {
		t.Error("--dark was undone by a light terminal")
	}
}

func TestToggleSwitchesTheAppearance(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeAuto})
	appearance.TerminalIsDark(true)

	appearance.Toggle()

	if appearance.Dark() {
		t.Error("toggling a dark appearance did not make it light")
	}

	appearance.Toggle()

	if !appearance.Dark() {
		t.Error("toggling twice did not return to dark")
	}
}

// Once the operator has pressed d, the terminal has lost the argument. A
// background report arriving later must not snap the palette back.
func TestToggleWinsOverALaterTerminalReport(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeAuto})
	appearance.TerminalIsDark(true)
	appearance.Toggle()

	if changed := appearance.TerminalIsDark(true); changed {
		t.Error("a terminal report overrode a toggle")
	}

	if appearance.Dark() {
		t.Error("the toggle was undone by a terminal report")
	}
}

// The palette and the styles derived from it are resolved on change, not per
// frame, so they have to actually move when the appearance does.
func TestPaletteFollowsTheAppearance(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeAuto})
	appearance.TerminalIsDark(true)

	before := appearance.Palette()
	if !before.Dark {
		t.Fatal("the palette did not follow the terminal")
	}

	appearance.Toggle()

	after := appearance.Palette()
	if after.Dark {
		t.Error("the palette did not follow the toggle")
	}

	if before.Background == after.Background {
		t.Error("the light and dark palettes share a background colour")
	}
}

func TestStylesAreRebuiltWithThePalette(t *testing.T) {
	t.Parallel()

	appearance := theme.NewAppearance(theme.Config{Mode: theme.ModeDark})

	dark := appearance.Styles()
	appearance.Toggle()
	light := appearance.Styles()

	if dark.Accent.GetForeground() == light.Accent.GetForeground() {
		t.Error("the styles were not rebuilt when the palette changed")
	}
}
