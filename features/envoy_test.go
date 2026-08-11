//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/scrape"
	"github.com/pantherhawk/prism/internal/series"
)

const (
	// Variable envoyEndpointEnv points the suite at a live admin port. It is set
	// by "make bdd-envoy" after "make envoy-up"; unset, the scenarios replay the
	// recording instead.
	envoyEndpointEnv = "PRISM_ENVOY_ENDPOINT"

	// File envoyFixture holds that recording, captured by "make envoy-fixture".
	envoyFixture = "testdata/envoy-stats.txt"

	// Prefix envoyRenamed replaces the vendor's in the special-casing scenario.
	// It has to be a name no exporter would emit, because the point of it is to
	// prove that prism has never heard of it.
	envoyRenamed = "wavelength_"

	// Width envoyResolution is the bucket the scenarios store at, matching
	// prism's default, so that two scrapes 15s apart land in adjacent buckets.
	envoyResolution = 15 * time.Second

	// Constant errorListLimit caps how many offenders a failure names before summarising
	// the rest. Envoy has three hundred families; a message listing all of them
	// is a message nobody reads to the end.
	errorListLimit = 5
)

// histogramParts are the suffixes Prometheus splits a histogram across. An
// operator thinks of the three as one metric, and prism has to as well.
var histogramParts = []string{"_bucket", "_sum", "_count"}

var (
	errNoEnvoyBody     = errors.New("no exposition was loaded")
	errNoEnvoySnapshot = errors.New("nothing has been scraped yet")
)

// declaration is what the endpoint said about one family, parsed straight out
// of the exposition text rather than through prism's decoder.
//
// Two implementations that agree are evidence; one implementation checked
// against itself is a tautology. This is the second one, and it is deliberately
// naive: it knows the four lines of the text format and nothing else.
type declaration struct {
	kind    series.Kind
	samples int
}

// envoyWorld is the state of one Envoy scenario.
type envoyWorld struct {
	body     string
	endpoint string
	server   *httptest.Server

	at time.Time

	snapshot *series.Snapshot
	renamed  *series.Snapshot
}

type envoyKey struct{}

func envoyFrom(ctx context.Context) *envoyWorld {
	w, ok := ctx.Value(envoyKey{}).(*envoyWorld)
	if !ok {
		panic("envoy world missing from context")
	}

	return w
}

// declared parses the exposition into the families it announced and how many
// sample lines each one carried.
//
// Families with a TYPE line and no samples are reported with a zero count and
// excluded by the callers: prism stores observations, and there is nothing to
// store for a family the target described but did not measure.
func declared(body string) map[string]*declaration {
	out := make(map[string]*declaration)

	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			if name, kind, ok := parseType(line); ok {
				out[name] = &declaration{kind: kind}
			}

			continue
		}

		if decl, ok := out[familyOf(line, out)]; ok {
			decl.samples++
		}
	}

	return out
}

// parseType reads a `# TYPE <name> <kind>` line.
func parseType(line string) (string, series.Kind, bool) {
	const fields = 4

	parts := strings.Fields(line)
	if len(parts) < fields || parts[1] != "TYPE" {
		return "", "", false
	}

	kind, ok := map[string]series.Kind{
		"counter":   series.KindCounter,
		"gauge":     series.KindGauge,
		"histogram": series.KindHistogram,
		"summary":   series.KindSummary,
		"untyped":   series.KindUntyped,
	}[parts[3]]

	return parts[2], kind, ok
}

// familyOf returns the family a sample line belongs to, folding the three parts
// of a histogram or summary back into the name they were declared under.
//
// A suffix is only stripped when the shorter name was declared as a
// distribution. `envoy_server_total_connections` is a gauge whose name ends in
// a word that is not a suffix, and trimming by spelling alone would invent a
// family called `envoy_server_total_`.
func familyOf(line string, decls map[string]*declaration) string {
	name, _, _ := strings.Cut(line, "{")
	name, _, _ = strings.Cut(strings.TrimSpace(name), " ")

	if _, ok := decls[name]; ok {
		return name
	}

	for _, suffix := range histogramParts {
		base, found := strings.CutSuffix(name, suffix)
		if !found {
			continue
		}

		if decl, ok := decls[base]; ok && distribution(decl.kind) {
			return base
		}
	}

	return name
}

// distribution reports whether a kind is split across several exposition lines.
func distribution(kind series.Kind) bool {
	return kind == series.KindHistogram || kind == series.KindSummary
}

// measured returns the families that carried at least one sample.
func measured(decls map[string]*declaration) map[string]*declaration {
	out := make(map[string]*declaration, len(decls))

	for name, decl := range decls {
		if decl.samples > 0 {
			out[name] = decl
		}
	}

	return out
}

// load fetches the exposition once, from the live endpoint when one is
// configured and from the recording otherwise.
func (w *envoyWorld) load() error {
	if endpoint := os.Getenv(envoyEndpointEnv); endpoint != "" {
		body, err := fetch(endpoint)
		if err != nil {
			return err
		}

		w.body, w.endpoint = body, endpoint

		return nil
	}

	body, err := os.ReadFile(envoyFixture)
	if err != nil {
		return fmt.Errorf("read %s (run `make envoy-up envoy-fixture`): %w", envoyFixture, err)
	}

	w.body = string(body)
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(rw, w.body)
	}))
	w.endpoint = w.server.URL

	return nil
}

// fetch reads an exposition endpoint.
//
// The endpoint is this suite's own httptest server or the admin port an
// operator named in PRISM_ENVOY_ENDPOINT, which is why the SSRF check is
// silenced here: there is no untrusted input for it to be reached by.
//
//nolint:gosec // G704: the endpoint is the suite's own server or the operator's
func fetch(endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", endpoint, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s from %s", scrape.ErrUnexpectedStatus, resp.Status, endpoint)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", endpoint, err)
	}

	return string(body), nil
}

// collect scrapes an endpoint into a fresh store and returns the snapshot.
//
// The budget is unlimited so that the reported cardinalities are counts rather
// than estimates: this suite is grading whether Envoy's families arrive, and a
// sketch's error would blur the answer.
func collect(
	ctx context.Context, endpoint string, at time.Time, scrapes int,
) (*series.Snapshot, error) {
	quiet := slog.New(slog.DiscardHandler)

	store, err := series.NewStore(time.Hour, envoyResolution, 0, quiet)
	if err != nil {
		return nil, fmt.Errorf("new store: %w", err)
	}

	cfg := scrape.Default()
	cfg.Endpoint = endpoint
	cfg.Timeout = 30 * time.Second

	collector, err := scrape.New(cfg, store, quiet)
	if err != nil {
		return nil, fmt.Errorf("new collector: %w", err)
	}

	for i := range scrapes {
		collector.Collect(ctx, at.Add(time.Duration(i)*envoyResolution))
	}

	snapshot := store.Snapshot()
	if snapshot.Stats.Errors > 0 {
		return nil, fmt.Errorf("%w: %s", errScrapeFailed, snapshot.Stats.LastError)
	}

	return snapshot, nil
}

var errScrapeFailed = errors.New("scrape failed")

// familyKinds maps each stored family to the kind its series carry.
func familyKinds(snapshot *series.Snapshot) map[string]series.Kind {
	out := make(map[string]series.Kind, len(snapshot.Families))

	for _, view := range snapshot.Series {
		out[view.Family] = view.Kind
	}

	return out
}

// ---- Given -----------------------------------------------------------------

func (w *envoyWorld) envoysStatsEndpoint(ctx context.Context) (context.Context, error) {
	return ctx, w.load()
}

// ---- When ------------------------------------------------------------------

func (w *envoyWorld) itIsScrapedOnce(ctx context.Context) (context.Context, error) {
	w.at = time.Unix(1_700_000_000, 0)

	snapshot, err := collect(ctx, w.endpoint, w.at, 1)
	if err != nil {
		return ctx, err
	}

	w.snapshot = snapshot

	return ctx, nil
}

// itIsScrapedAgainLater re-scrapes into a fresh store so that the snapshot holds
// two buckets. A rate needs two observations, and this is the cheapest way to
// have them without sleeping through a real interval.
func (w *envoyWorld) itIsScrapedAgainLater(
	ctx context.Context, after string,
) (context.Context, error) {
	elapsed, err := time.ParseDuration(after)
	if err != nil {
		return ctx, fmt.Errorf("parse duration: %w", err)
	}

	if elapsed != envoyResolution {
		return ctx, fmt.Errorf("%w: scenario asked for %s, the store buckets at %s",
			errScrapeFailed, after, envoyResolution)
	}

	snapshot, err := collect(ctx, w.endpoint, w.at, 2)
	if err != nil {
		return ctx, err
	}

	w.snapshot = snapshot

	return ctx, nil
}

// theSameExpositionRenamed scrapes the identical bytes with the vendor's prefix
// rewritten, through the same decoder and store.
func (w *envoyWorld) theSameExpositionRenamed(ctx context.Context) (context.Context, error) {
	if w.body == "" {
		return ctx, errNoEnvoyBody
	}

	renamed := strings.ReplaceAll(w.body, "envoy_", envoyRenamed)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(rw, renamed)
	}))
	defer server.Close()

	snapshot, err := collect(ctx, server.URL, w.at, 1)
	if err != nil {
		return ctx, err
	}

	w.renamed = snapshot

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *envoyWorld) everyDeclaredFamilyIsHeld(ctx context.Context) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	held := make(map[string]struct{}, len(w.snapshot.Families))
	for _, family := range w.snapshot.Families {
		held[family.Name] = struct{}{}
	}

	var missing []string

	for name := range measured(declared(w.body)) {
		if _, ok := held[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)

		return ctx, fmt.Errorf("%w: %d of %d families were dropped, including %s",
			errScrapeFailed, len(missing), len(held)+len(missing),
			strings.Join(head(missing), ", "))
	}

	return ctx, nil
}

func (w *envoyWorld) noFamilyIsNamedAfterAHistogramPart(
	ctx context.Context,
) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	decls := declared(w.body)

	var split []string

	for _, family := range w.snapshot.Families {
		for _, suffix := range histogramParts {
			base, found := strings.CutSuffix(family.Name, suffix)
			if !found {
				continue
			}

			// Only a name whose base was declared a distribution is evidence of
			// a split. A counter genuinely called `..._count` is Envoy's choice
			// of words, not prism failing to group anything.
			if decl, ok := decls[base]; ok && distribution(decl.kind) {
				split = append(split, family.Name)
			}
		}
	}

	if len(split) > 0 {
		slices.Sort(split)

		return ctx, fmt.Errorf("%w: %d histogram parts became families of their own: %s",
			errScrapeFailed, len(split), strings.Join(head(split), ", "))
	}

	return ctx, nil
}

func (w *envoyWorld) everyHistogramIsHeldOnce(ctx context.Context) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	kinds := familyKinds(w.snapshot)

	var (
		wrong []string
		seen  int
	)

	for name, decl := range measured(declared(w.body)) {
		if !distribution(decl.kind) {
			continue
		}

		seen++

		if kinds[name] != decl.kind {
			wrong = append(wrong,
				fmt.Sprintf("%s stored as %q, declared %q", name, kinds[name], decl.kind))
		}
	}

	if len(wrong) > 0 {
		slices.Sort(wrong)

		return ctx, fmt.Errorf("%w: %s", errScrapeFailed, strings.Join(head(wrong), "; "))
	}

	if seen == 0 {
		return ctx, fmt.Errorf(
			"%w: the endpoint declared no populated histograms, so this proves nothing",
			errScrapeFailed)
	}

	return ctx, nil
}

func (w *envoyWorld) familyIsACounter(ctx context.Context, name string) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	if got := familyKinds(w.snapshot)[name]; got != series.KindCounter {
		return ctx, fmt.Errorf("%w: %s is %q, want %q", errScrapeFailed, name, got, series.KindCounter)
	}

	return ctx, nil
}

// itsValueIsARate checks that what is charted is a per-second figure and not the
// number Envoy reported.
//
// The exact arithmetic is graded in features/scrape.feature against a counter
// this suite controls. Here the counter is whatever Envoy happens to have
// climbed to, so the assertion is the one that survives not knowing: a rate over
// one bucket is bounded by the traffic in that bucket, and Envoy's cumulative
// totals are orders of magnitude above it.
func (w *envoyWorld) itsValueIsARate(ctx context.Context, name string) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	var charted, cumulative float64

	for _, view := range w.snapshot.Series {
		if view.Family != name {
			continue
		}

		charted += view.Last
	}

	for line := range strings.SplitSeq(w.body, "\n") {
		if !strings.HasPrefix(line, name+"{") && !strings.HasPrefix(line, name+" ") {
			continue
		}

		fields := strings.Fields(line)

		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}

		cumulative += value
	}

	if cumulative == 0 {
		return ctx, fmt.Errorf("%w: %s carried no observations to compare against",
			errScrapeFailed, name)
	}

	// A rate derived from one bucket of a counter that has been climbing since
	// the process started cannot approach the total. If it does, the store
	// recorded the reading rather than the change in it.
	if charted >= cumulative/2 {
		return ctx, fmt.Errorf(
			"%w: %s charts %g against a cumulative %g, which is a total, not a rate",
			errScrapeFailed, name, charted, cumulative)
	}

	return ctx, nil
}

func (w *envoyWorld) pivotingGivesOneLinePerValue(
	ctx context.Context, family, key string,
) (context.Context, error) {
	if w.snapshot == nil {
		return ctx, errNoEnvoySnapshot
	}

	// The expected set is read off the snapshot rather than written down, so the
	// scenario does not have to know which clusters deploy/ happens to define.
	want := make(map[string]struct{})

	for _, view := range w.snapshot.Series {
		if view.Family != family {
			continue
		}

		value, ok := view.Labels.Lookup(key)
		if !ok {
			value = series.AbsentLabel
		}

		want[value] = struct{}{}
	}

	if len(want) < 2 {
		return ctx, fmt.Errorf("%w: %s carries %d distinct %s values, so a pivot separates nothing",
			errScrapeFailed, family, len(want), key)
	}

	lines := w.snapshot.Lines(series.Selection{Family: family, PivotKey: key})
	if len(lines) != len(want) {
		return ctx, fmt.Errorf("%w: pivot gave %d lines, want %d",
			errScrapeFailed, len(lines), len(want))
	}

	for _, line := range lines {
		if _, ok := want[line.Label]; !ok {
			return ctx, fmt.Errorf("%w: pivot invented the line %q", errScrapeFailed, line.Label)
		}
	}

	return ctx, nil
}

// bothScrapesAgree compares the two runs after undoing the rename.
//
// This is what "renders without special-casing" means as an assertion. Any
// branch on the vendor's name — a suffix table keyed to Envoy's spelling, a
// family list, a prefix strip — would make the renamed run come out different,
// and no amount of reading the code proves its absence the way this does.
func (w *envoyWorld) bothScrapesAgree(ctx context.Context) (context.Context, error) {
	if w.snapshot == nil || w.renamed == nil {
		return ctx, errNoEnvoySnapshot
	}

	original := shapeOf(w.snapshot, "")
	renamed := shapeOf(w.renamed, envoyRenamed)

	if len(original) != len(renamed) {
		return ctx, fmt.Errorf("%w: %d families before the rename, %d after",
			errScrapeFailed, len(original), len(renamed))
	}

	var differ []string

	for name, want := range original {
		got, ok := renamed[name]

		switch {
		case !ok:
			differ = append(differ, name+" vanished")
		case got != want:
			differ = append(differ, fmt.Sprintf("%s was %s, became %s", name, want, got))
		default:
		}
	}

	if len(differ) > 0 {
		slices.Sort(differ)

		return ctx, fmt.Errorf("%w: renaming the prefix changed %d families: %s",
			errScrapeFailed, len(differ), strings.Join(head(differ), "; "))
	}

	return ctx, nil
}

// shapeOf reduces a snapshot to what each family is and how big it is, with a
// prefix stripped so the two runs are comparable.
func shapeOf(snapshot *series.Snapshot, prefix string) map[string]string {
	kinds := familyKinds(snapshot)
	out := make(map[string]string, len(snapshot.Families))

	for _, family := range snapshot.Families {
		name := family.Name
		if prefix != "" {
			name = strings.ReplaceAll(name, prefix, "envoy_")
		}

		out[name] = fmt.Sprintf("%s×%d", kinds[family.Name], family.Cardinality)
	}

	return out
}

// head truncates a list for an error message, saying how much it left out. A
// failure listing nine hundred families is a failure nobody reads.
func head(items []string) []string {
	if len(items) <= errorListLimit {
		return items
	}

	return append(items[:errorListLimit:errorListLimit],
		fmt.Sprintf("and %d more", len(items)-errorListLimit))
}

// ---- wiring ----------------------------------------------------------------

// initializeEnvoy registers the Envoy end-to-end steps.
func initializeEnvoy(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, envoyKey{}, &envoyWorld{}), nil
	})

	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w := envoyFrom(ctx); w.server != nil {
			w.server.Close()
		}

		return ctx, nil
	})

	sc.Step(`^Envoy's stats endpoint$`, func(ctx context.Context) (context.Context, error) {
		return envoyFrom(ctx).envoysStatsEndpoint(ctx)
	})

	sc.Step(`^Envoy is scraped$`, func(ctx context.Context) (context.Context, error) {
		return envoyFrom(ctx).itIsScrapedOnce(ctx)
	})
	sc.Step(`^Envoy is scraped again ([0-9a-z]+) later$`,
		func(ctx context.Context, after string) (context.Context, error) {
			return envoyFrom(ctx).itIsScrapedAgainLater(ctx, after)
		})
	sc.Step(`^the same exposition is scraped with every "envoy_" prefix renamed$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).theSameExpositionRenamed(ctx)
		})

	initializeEnvoyAssertions(sc)
}

// initializeEnvoyAssertions registers the Then steps.
func initializeEnvoyAssertions(sc *godog.ScenarioContext) {
	sc.Step(`^every family the endpoint declared is held$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).everyDeclaredFamilyIsHeld(ctx)
		})
	sc.Step(`^no family is named after a histogram part$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).noFamilyIsNamedAfterAHistogramPart(ctx)
		})
	sc.Step(`^every histogram the endpoint declared is held once$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).everyHistogramIsHeldOnce(ctx)
		})
	sc.Step(`^"([^"]*)" is a counter$`,
		func(ctx context.Context, name string) (context.Context, error) {
			return envoyFrom(ctx).familyIsACounter(ctx, name)
		})
	sc.Step(`^its charted value is a rate rather than the cumulative total$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).itsValueIsARate(ctx, "envoy_cluster_upstream_rq_total")
		})
	sc.Step(`^pivoting "([^"]*)" on "([^"]*)" gives one line per cluster$`,
		func(ctx context.Context, family, key string) (context.Context, error) {
			return envoyFrom(ctx).pivotingGivesOneLinePerValue(ctx, family, key)
		})
	sc.Step(`^both scrapes produce the same families, kinds and cardinalities$`,
		func(ctx context.Context) (context.Context, error) {
			return envoyFrom(ctx).bothScrapesAgree(ctx)
		})
}
