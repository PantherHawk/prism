package app

import (
	"errors"

	"github.com/pantherhawk/prism/internal/theme"
)

// ErrConflictingTheme is returned when both --dark and --light are given.
var ErrConflictingTheme = errors.New("--dark and --light are mutually exclusive")

// Overrides are the settings the command line imposes on top of the
// configuration file. The zero value changes nothing.
//
// Flags override the file rather than the other way round because a flag is
// typed for one run and a file is written once: the more specific instruction,
// and the more recent one, should win.
//
// The fields are the flags as given rather than a resolved appearance, so that
// main only has to parse. Deciding what they mean together belongs with the
// code that assembles the configuration they are overriding.
type Overrides struct {
	Dark  bool
	Light bool
}

// Theme returns the appearance the flags select, or the empty mode when neither
// was given and the configuration file should stand.
func (o Overrides) Theme() (theme.Mode, error) {
	return ThemeMode(o.Dark, o.Light)
}

// ThemeMode returns the appearance --dark and --light select.
//
// The absence of both is reported as the empty mode rather than as auto. They
// are not the same thing: auto is a choice the configuration file can make, and
// a flag that quietly rewrote it to auto would override a file that asked for
// dark by not being passed at all.
func ThemeMode(dark, light bool) (theme.Mode, error) {
	switch {
	case dark && light:
		return "", ErrConflictingTheme
	case dark:
		return theme.ModeDark, nil
	case light:
		return theme.ModeLight, nil
	default:
		return "", nil
	}
}
