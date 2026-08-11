package app_test

import (
	"errors"
	"testing"

	"github.com/pantherhawk/prism/internal/app"
	"github.com/pantherhawk/prism/internal/theme"
)

func TestNeitherThemeFlagLeavesTheConfigurationAlone(t *testing.T) {
	t.Parallel()

	mode, err := app.ThemeMode(false, false)
	if err != nil {
		t.Fatalf("ThemeMode(false, false) = %v, want no error", err)
	}

	// The empty mode is what tells Build not to override the config file, which
	// is different from asking for auto.
	if mode != "" {
		t.Errorf("ThemeMode(false, false) = %q, want no override", mode)
	}
}

func TestEachThemeFlagSelectsItsMode(t *testing.T) {
	t.Parallel()

	dark, err := app.ThemeMode(true, false)
	if err != nil {
		t.Fatalf("--dark: %v", err)
	}

	if dark != theme.ModeDark {
		t.Errorf("--dark selected %q, want %q", dark, theme.ModeDark)
	}

	light, err := app.ThemeMode(false, true)
	if err != nil {
		t.Fatalf("--light: %v", err)
	}

	if light != theme.ModeLight {
		t.Errorf("--light selected %q, want %q", light, theme.ModeLight)
	}
}

// Asking for both is a contradiction, and silently picking one would leave the
// operator staring at a palette they explicitly did not ask for.
func TestBothThemeFlagsIsAnError(t *testing.T) {
	t.Parallel()

	_, err := app.ThemeMode(true, true)

	if !errors.Is(err, app.ErrConflictingTheme) {
		t.Errorf("ThemeMode(true, true) = %v, want ErrConflictingTheme", err)
	}
}

func TestOverridesReplaceTheConfiguredTheme(t *testing.T) {
	t.Parallel()

	configured := theme.ModeAuto

	mode, err := app.Overrides{Light: true}.Theme()
	if err != nil {
		t.Fatalf("--light: %v", err)
	}

	if got := mode.Or(configured); got != theme.ModeLight {
		t.Errorf("override resolved to %q, want %q", got, theme.ModeLight)
	}

	absent, err := app.Overrides{}.Theme()
	if err != nil {
		t.Fatalf("no flags: %v", err)
	}

	if got := absent.Or(configured); got != configured {
		t.Errorf("an absent override resolved to %q, want the configured %q", got, configured)
	}
}

// A contradiction has to reach Build rather than being resolved silently.
func TestConflictingOverridesReportAnError(t *testing.T) {
	t.Parallel()

	_, err := app.Overrides{Dark: true, Light: true}.Theme()

	if !errors.Is(err, app.ErrConflictingTheme) {
		t.Errorf("both flags gave %v, want ErrConflictingTheme", err)
	}
}
