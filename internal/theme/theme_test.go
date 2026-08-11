package theme_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pantherhawk/prism/internal/theme"
)

func TestDefaultModeIsAuto(t *testing.T) {
	t.Parallel()

	if got := theme.Default().Mode; got != theme.ModeAuto {
		t.Errorf("default mode = %q, want %q", got, theme.ModeAuto)
	}
}

// Or is how a command-line flag defers to the configuration file when it was
// not passed. The empty mode has to mean "unset" rather than being validated
// as a mode in its own right.
func TestModeOrFallsBackWhenUnset(t *testing.T) {
	t.Parallel()

	if got := theme.Mode("").Or(theme.ModeDark); got != theme.ModeDark {
		t.Errorf("unset mode resolved to %q, want the fallback %q", got, theme.ModeDark)
	}

	if got := theme.ModeLight.Or(theme.ModeDark); got != theme.ModeLight {
		t.Errorf("a set mode resolved to %q, want it to win", got)
	}
}

func TestValidateAcceptsEveryMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []theme.Mode{theme.ModeAuto, theme.ModeDark, theme.ModeLight} {
		err := theme.Config{Mode: mode}.Validate()
		if err != nil {
			t.Errorf("Validate(%q) = %v, want nil", mode, err)
		}
	}
}

func TestValidateRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	err := theme.Config{Mode: "sepia"}.Validate()

	if !errors.Is(err, theme.ErrUnknownMode) {
		t.Fatalf("Validate(sepia) = %v, want ErrUnknownMode", err)
	}

	// The operator has to be told which value was rejected; "invalid mode" on
	// its own sends them back to the config file to guess.
	if !strings.Contains(err.Error(), "sepia") {
		t.Errorf("error %q does not name the rejected mode", err)
	}
}

func TestForcedModeIgnoresTheTerminal(t *testing.T) {
	t.Parallel()

	if !theme.Resolve(theme.Config{Mode: theme.ModeDark}, false).Dark {
		t.Error("--dark did not override a light terminal")
	}

	if theme.Resolve(theme.Config{Mode: theme.ModeLight}, true).Dark {
		t.Error("--light did not override a dark terminal")
	}
}

func TestAutoFollowsTheTerminal(t *testing.T) {
	t.Parallel()

	if !theme.Resolve(theme.Config{Mode: theme.ModeAuto}, true).Dark {
		t.Error("auto did not follow a dark terminal")
	}

	if theme.Resolve(theme.Config{Mode: theme.ModeAuto}, false).Dark {
		t.Error("auto did not follow a light terminal")
	}
}

// Every palette must fill every role. A nil colour renders as the terminal
// default, which silently undoes the light-mode work in exactly the places
// nobody looks at.
func TestBothPalettesFillEveryRole(t *testing.T) {
	t.Parallel()

	for _, mode := range []theme.Mode{theme.ModeDark, theme.ModeLight} {
		palette := theme.Resolve(theme.Config{Mode: mode}, false)

		roles := map[string]bool{
			"Background": palette.Background == nil,
			"Foreground": palette.Foreground == nil,
			"Muted":      palette.Muted == nil,
			"Chrome":     palette.Chrome == nil,
			"Accent":     palette.Accent == nil,
			"Secondary":  palette.Secondary == nil,
			"Warn":       palette.Warn == nil,
			"OK":         palette.OK == nil,
			"Selection":  palette.Selection == nil,
		}

		for role, missing := range roles {
			if missing {
				t.Errorf("%s palette leaves %s unset", mode, role)
			}
		}

		for i, colour := range palette.Spectrum {
			if colour == nil {
				t.Errorf("%s palette leaves spectrum band %d unset", mode, i)
			}
		}
	}
}

// The banner draws one glyph per spectrum band, so the two lengths are coupled.
func TestSpectrumIsTheSameWidthInBothPalettes(t *testing.T) {
	t.Parallel()

	dark := theme.Resolve(theme.Config{Mode: theme.ModeDark}, false)
	light := theme.Resolve(theme.Config{Mode: theme.ModeLight}, false)

	if len(dark.Spectrum) != len(light.Spectrum) {
		t.Errorf("spectrum is %d bands dark and %d light", len(dark.Spectrum), len(light.Spectrum))
	}

	if len(dark.Spectrum) == 0 {
		t.Error("the spectrum is empty")
	}
}
