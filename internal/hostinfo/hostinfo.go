// Package hostinfo collects the host facts prism's splash screen reports.
//
// It is the impure half of the splash, quarantined here so that
// [github.com/pantherhawk/prism/internal/banner] can stay a pure function of
// its inputs. Everything is gathered once at startup and handed on as plain
// strings; nothing in this package is called per frame.
//
// No function here returns an error. Every fact degrades to the empty string
// when its source is missing or fails, and the banner omits empty fields, so a
// host prism cannot introspect costs a line rather than a startup.
package hostinfo

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"
)

// Facts is everything the splash knows about the machine it is running on.
type Facts struct {
	User   string
	Host   string
	OS     string
	Kernel string
	Shell  string
	Term   string
	Go     string
	Uptime string
}

// Collect gathers the host facts. It never fails.
func Collect() Facts {
	return Facts{
		User:   username(),
		Host:   hostname(),
		OS:     runtime.GOOS + "/" + runtime.GOARCH,
		Kernel: kernel(),
		Shell:  shell(),
		Term:   terminal(),
		Go:     runtime.Version(),
		Uptime: FormatUptime(uptime()),
	}
}

func username() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}

	return current.Username
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}

	return name
}

// shell reports the login shell by base name: "/opt/homebrew/bin/zsh" is
// reported as "zsh", because the path is noise on a splash screen.
func shell() string {
	path := os.Getenv("SHELL")
	if path == "" {
		return ""
	}

	return filepath.Base(path)
}

// terminal prefers $TERM_PROGRAM, which names the emulator, over $TERM, which
// only names the terminfo entry it claims to implement.
func terminal() string {
	if program := os.Getenv("TERM_PROGRAM"); program != "" {
		return program
	}

	return os.Getenv("TERM")
}

// hoursPerDay converts hours to days when formatting uptime.
const hoursPerDay = 24

// FormatUptime renders a duration the way neofetch does. A zero duration means
// the uptime could not be read, and formats as empty so the line is omitted.
//
// Exported because the formatting is the only part worth testing directly: the
// duration itself comes from a syscall that a test cannot pin.
func FormatUptime(dur time.Duration) string {
	if dur <= 0 {
		return ""
	}

	if dur < time.Minute {
		return "less than a minute"
	}

	days := int(dur.Hours()) / hoursPerDay
	hours := int(dur.Hours()) % hoursPerDay
	mins := int(dur.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%s, %s", plural(days, "day"), plural(hours, "hour"))
	case hours > 0:
		return fmt.Sprintf("%s, %s", plural(hours, "hour"), plural(mins, "min"))
	default:
		return plural(mins, "min")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}

	return fmt.Sprintf("%d %ss", n, unit)
}
