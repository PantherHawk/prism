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
