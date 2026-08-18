package banner

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pantherhawk/prism/internal/theme"
)

func testPalette() (theme.Palette, theme.Styles) {
	palette := theme.Resolve(theme.Config{Mode: theme.ModeDark}, true)

	return palette, theme.NewStyles(palette)
}

func fullInfo() Info {
	return Info{
		Version:  "v0.4.1",
		Endpoint: "localhost:9090",
		Buffer:   "15m ring · 1s buckets",
		User:     "alex",
		Host:     "studio",
		OS:       "darwin/arm64",
		Kernel:   "Darwin 25.5.0",
		Shell:    "zsh",
		Term:     "ghostty",
		Go:       "go1.25.4",
		Uptime:   "4 days, 2 hours",
	}
}

func TestInfoBlockLeadsWithUserAtHost(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	first := ansi.Strip(infoBlock(fullInfo(), palette, styles)[0])
	if first != "alex@studio" {
		t.Errorf("info block leads with %q, want %q", first, "alex@studio")
	}
}

func TestInfoBlockRendersEveryPopulatedFact(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	body := ansi.Strip(strings.Join(infoBlock(fullInfo(), palette, styles), "\n"))

	for _, want := range []string{
		"v0.4.1", "localhost:9090", "15m ring · 1s buckets", "darwin/arm64",
		"Darwin 25.5.0", "zsh", "ghostty", "go1.25.4", "4 days, 2 hours",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("info block is missing %q\n%s", want, body)
		}
	}
}

// An unreadable fact must cost its line, not leave a labelled blank.
func TestInfoBlockOmitsEmptyFacts(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	info := fullInfo()
	info.Kernel = ""
	info.Uptime = ""

	body := ansi.Strip(strings.Join(infoBlock(info, palette, styles), "\n"))

	for _, absent := range []string{"kernel", "uptime"} {
		if strings.Contains(body, absent) {
			t.Errorf("info block still labels %q with no value\n%s", absent, body)
		}
	}
}

// With no facts at all the block is still the title and the swatch, never a
// bare list of labels.
func TestInfoBlockSurvivesAnEmptyInfo(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	if got := infoBlock(Info{}, palette, styles); len(got) == 0 {
		t.Error("info block is empty for a zero Info")
	}
}

func TestHeadingCoversEveryAvailabilityOfUserAndHost(t *testing.T) {
	t.Parallel()

	_, styles := testPalette()

	tests := []struct {
		name string
		user string
		host string
		want string
	}{
		{"both known", "alex", "studio", "alex@studio"},
		{"host only", "", "vega", "vega"},
		{"user only", "kay", "", "kay"},
		{"neither known", "", "", "promdate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := Info{User: tt.user, Host: tt.host}

			got := ansi.Strip(heading(info, styles))
			if got != tt.want {
				t.Errorf("heading(%+v) = %q, want %q", info, got, tt.want)
			}
		})
	}
}

func TestInfoBlockEndsWithASwatchPerSpectrumBand(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	block := infoBlock(fullInfo(), palette, styles)
	last := ansi.Strip(block[len(block)-1])

	if want := len(palette.Spectrum) * swatchWidth; len([]rune(last)) != want {
		t.Errorf("swatch row is %d cells, want %d", len([]rune(last)), want)
	}
}

// The full tier must fit the smallest terminal it claims — the constants are
// the art plus its furniture, so art that outgrows them has to fail here
// rather than spill over the edge of a terminal that just qualified.
func TestFullTierFitsItsOwnThreshold(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	out := Render(fullInfo(), palette, styles, fullWidth, fullHeight)

	for i, line := range strings.Split(out, "\n") {
		if got := ansi.StringWidth(line); got > fullWidth {
			t.Errorf("row %d is %d columns wide, want at most %d", i, got, fullWidth)
		}
	}

	if got := len(strings.Split(out, "\n")); got > fullHeight {
		t.Errorf("splash is %d rows, want at most %d", got, fullHeight)
	}
}

func TestFullTierDrawsTheLogoBesideTheFacts(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	out := ansi.Strip(Render(fullInfo(), palette, styles, 120, 34))

	if !strings.Contains(out, "ghostty") {
		t.Fatal("full tier dropped the facts")
	}

	// A row carrying both art and a fact is what "side by side" means. Which
	// art row lands beside which fact depends on the vertical centring, so
	// this asks only that art sits to the left of the fact, not which glyph.
	var joined bool

	for line := range strings.SplitSeq(out, "\n") {
		if i := strings.Index(line, "darwin/arm64"); i > 0 && strings.ContainsAny(line[:i], `\:`) {
			joined = true
		}
	}

	if !joined {
		t.Errorf("no row carries both the logo and a fact:\n%s", out)
	}
}

// Below the full tier the logo is dropped rather than wrapped, which is what
// neofetch --off does. A stock 80x24 terminal is one of these: the isometric2
// wordmark does not fit beside the facts there, and the compact tier is the
// intended answer rather than a clipped logo.
func TestCompactTierDropsTheLogoAndKeepsTheFacts(t *testing.T) {
	t.Parallel()

	for _, size := range []struct {
		name string
		w, h int
	}{
		{"split pane", 62, 20},
		{"stock terminal", 80, 24},
		{"wide but short", 120, 24},
		{"tall but narrow", 80, 40},
	} {
		t.Run(size.name, func(t *testing.T) {
			t.Parallel()

			palette, styles := testPalette()

			out := ansi.Strip(Render(fullInfo(), palette, styles, size.w, size.h))

			if strings.Contains(out, `\/__/`) {
				t.Error("compact tier still drew the logo")
			}

			if !strings.Contains(out, "localhost:9090") {
				t.Error("compact tier dropped the facts")
			}

			if !strings.Contains(out, "PROMDATE") {
				t.Error("compact tier dropped the wordmark title")
			}

			for i, line := range strings.Split(out, "\n") {
				if got := ansi.StringWidth(line); got > size.w {
					t.Errorf("row %d is %d columns wide, want at most %d", i, got, size.w)
				}
			}
		})
	}
}

// A qualifying terminal is not proof the composition fits: the info column is
// as wide as the facts, and the scrape endpoint carries whatever upstream URL
// the operator configured. The logo gives way rather than the row spilling.
func TestFullTierYieldsToAnOversizedFact(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	info := fullInfo()
	info.Endpoint = "https://metrics.internal.example:9090 (upstream)"

	out := ansi.Strip(Render(info, palette, styles, fullWidth, fullHeight))

	if strings.Contains(out, `\/__/`) {
		t.Error("kept the logo beside an info column too wide to sit next to it")
	}

	for i, line := range strings.Split(out, "\n") {
		if got := ansi.StringWidth(line); got > fullWidth {
			t.Errorf("row %d is %d columns wide, want at most %d", i, got, fullWidth)
		}
	}
}

// The keyhint is the only line telling the operator how to leave the splash,
// so it survives on both tiers.
func TestBothTiersKeepTheKeyhint(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	for _, c := range []struct{ w, h int }{{120, 34}, {62, 20}} {
		out := ansi.Strip(Render(fullInfo(), palette, styles, c.w, c.h))
		if !strings.Contains(out, "any key") {
			t.Errorf("%dx%d dropped the keyhint:\n%s", c.w, c.h, out)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	first := Render(fullInfo(), palette, styles, 120, 34)
	if second := Render(fullInfo(), palette, styles, 120, 34); first != second {
		t.Error("Render is not a pure function of its inputs")
	}
}
