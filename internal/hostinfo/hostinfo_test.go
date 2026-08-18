package hostinfo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/hostinfo"
)

// Collect must never panic and never fail, whatever the environment looks
// like. A splash screen is not worth failing a startup over.
func TestCollectDegradesRatherThanFailing(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("TERM", "")
	t.Setenv("TERM_PROGRAM", "")

	facts := hostinfo.Collect()

	if facts.Shell != "" {
		t.Errorf("shell is %q with no $SHELL set, want empty", facts.Shell)
	}

	if facts.Term != "" {
		t.Errorf("term is %q with no $TERM set, want empty", facts.Term)
	}

	// Go version is compiled in, so it is the one field that is always known.
	if !strings.HasPrefix(facts.Go, "go") {
		t.Errorf("go version is %q, want a go-prefixed string", facts.Go)
	}
}

func TestShellIsReducedToItsBaseName(t *testing.T) {
	t.Setenv("SHELL", "/opt/homebrew/bin/zsh")

	if got := hostinfo.Collect().Shell; got != "zsh" {
		t.Errorf("shell is %q, want %q", got, "zsh")
	}
}

// TERM_PROGRAM names the emulator, which is what neofetch reports; TERM only
// names the terminfo entry, so it is the fallback rather than the first choice.
func TestTermPrefersTheProgramOverTheTerminfoEntry(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")

	if got := hostinfo.Collect().Term; got != "ghostty" {
		t.Errorf("term is %q, want %q", got, "ghostty")
	}

	t.Setenv("TERM_PROGRAM", "")

	if got := hostinfo.Collect().Term; got != "xterm-256color" {
		t.Errorf("term fell back to %q, want %q", got, "xterm-256color")
	}
}

func TestUptimeIsFormattedInDaysHoursAndMinutes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{45 * time.Second, "less than a minute"},
		{9 * time.Minute, "9 mins"},
		{time.Hour + time.Minute, "1 hour, 1 min"},
		{50 * time.Hour, "2 days, 2 hours"},
	} {
		if got := hostinfo.FormatUptime(c.in); got != c.want {
			t.Errorf("FormatUptime(%s) is %q, want %q", c.in, got, c.want)
		}
	}
}
