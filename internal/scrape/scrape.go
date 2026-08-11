// Package scrape polls a Prometheus exposition endpoint and feeds the samples
// it parses into a [series.Store].
//
// The response body is decoded as a stream, one metric family at a time, and
// never held in memory whole. Envoy's stats endpoint is routinely megabytes;
// buffering it would put a sawtooth in prism's own heap that is larger than
// everything else the process does.
package scrape

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/pantherhawk/prism/internal/series"
)

// ErrNoEndpoint is returned when no scrape target has been configured.
var ErrNoEndpoint = errors.New("no scrape endpoint configured")

// ErrUnexpectedStatus is returned when the target answers with anything other
// than 200. It is a sentinel so that a scrape loop can tell a target that is
// up but unhappy from one it could not reach at all: the first is worth
// reporting verbatim, the second is worth backing off from.
var ErrUnexpectedStatus = errors.New("unexpected status")

// Config describes the scrape target and how much history to keep for it.
type Config struct {
	Endpoint   string        `yaml:"endpoint"`
	Interval   time.Duration `yaml:"interval"`
	Timeout    time.Duration `yaml:"timeout"`
	Retention  time.Duration `yaml:"retention"`
	UserAgent  string        `yaml:"user_agent"`
	Resolution time.Duration `yaml:"resolution"`

	// FamilyBudget is how many series of one family the store will hold.
	// Beyond it the family is sampled, and the ratio is reported on screen.
	FamilyBudget int `yaml:"family_budget"`

	// CardinalityWarn is the number of distinct series in one family above
	// which it is flagged.
	CardinalityWarn int `yaml:"cardinality_warn"`
}

// Default returns the configuration used when none is supplied. The endpoint
// is Envoy's admin listener, because that is the target prism is built against.
func Default() Config {
	return Config{
		Endpoint:        "http://localhost:9901/stats/prometheus",
		Interval:        15 * time.Second,
		Timeout:         10 * time.Second,
		Retention:       15 * time.Minute,
		UserAgent:       "prism",
		Resolution:      15 * time.Second,
		FamilyBudget:    500,
		CardinalityWarn: 250,
	}
}

// Collector scrapes one endpoint on an interval.
type Collector struct {
	cfg    Config
	client *http.Client
	sink   series.Sink
	log    *slog.Logger
	stats  series.Stats
}

// New returns a collector for cfg. The sink is whatever should receive the
// samples: a store, a network broker, or a fan-out over both.
func New(cfg Config, sink series.Sink, log *slog.Logger) (*Collector, error) {
	if cfg.Endpoint == "" {
		return nil, ErrNoEndpoint
	}

	return &Collector{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
		sink:   sink,
		log:    log,
		stats:  series.Stats{Source: cfg.Endpoint},
	}, nil
}

// Run scrapes until ctx is done.
//
// A failed scrape is never terminal. Targets restart, admin ports close during
// a config reload, and a TUI that exits because one scrape timed out would be
// useless for exactly the incident it was opened to investigate. Failures are
// counted and surfaced in the status bar instead.
func (c *Collector) Run(ctx context.Context, errs chan<- error) {
	c.log.InfoContext(ctx, "collector started",
		slog.String("endpoint", c.cfg.Endpoint),
		slog.Duration("interval", c.cfg.Interval),
	)

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	c.Collect(ctx, time.Now())

	for {
		select {
		case <-ctx.Done():
			errs <- nil

			return
		case at := <-ticker.C:
			c.Collect(ctx, at)
		}
	}
}

// Collect performs one scrape and records the result, successful or not.
//
// The timestamp is a parameter rather than a call to the clock so that a rate
// is a function of its inputs. A test that had to sleep fifteen seconds to
// observe a per-second rate would not get written.
func (c *Collector) Collect(ctx context.Context, at time.Time) {
	started := time.Now()

	samples, err := c.fetch(ctx)

	c.stats.Scrapes++
	c.stats.LastLatency = time.Since(started)

	if err != nil {
		c.stats.Errors++
		c.stats.LastError = err.Error()

		c.log.WarnContext(ctx, "scrape failed", slog.Any("error", err))
		c.sink.Append(at, nil, c.stats)

		return
	}

	c.stats.LastError = ""

	// Every sample is handed over. Truncating the tail of a scrape here would
	// drop whole families arbitrarily, based on nothing but the order the
	// target happened to write them in. Thinning is the store's job, per
	// family, and it reports the ratio it used.
	c.sink.Append(at, samples, c.stats)
}

// fetch streams the endpoint and returns the samples it yielded.
func (c *Collector) fetch(ctx context.Context) ([]series.Sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", string(expfmt.NewFormat(expfmt.TypeTextPlain)))
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", c.cfg.Endpoint, err)
	}

	defer func() {
		// Draining before closing lets the connection return to the pool
		// instead of being torn down and redialled every interval.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrUnexpectedStatus, resp.Status)
	}

	return decode(resp.Body, expfmt.ResponseFormat(resp.Header))
}

// decode reads metric families one at a time and flattens them into samples.
//
// A histogram arrives here already whole. The text format splits one across
// `_bucket`, `_sum` and `_count` lines, but it also declares the family in a
// TYPE header, and the parser uses that header to reassemble the parts before
// yielding anything - so the three suffixes never reach prism as names, and
// there is no table of them anywhere in this package. Folding the suffixes by
// spelling instead would have been guesswork that breaks on the first counter
// an exporter chooses to call `..._count`; Envoy ships two.
//
// features/envoy.feature grades this against Envoy's own exposition.
func decode(body io.Reader, format expfmt.Format) ([]series.Sample, error) {
	decoder := expfmt.NewDecoder(body, format)
	samples := make([]series.Sample, 0, 512)

	for {
		var family dto.MetricFamily

		switch err := decoder.Decode(&family); {
		case errors.Is(err, io.EOF):
			return samples, nil
		case err != nil:
			// Partial results are kept: a truncated body still describes the
			// families that arrived before the break.
			return samples, fmt.Errorf("decode exposition: %w", err)
		}

		samples = appendFamily(samples, &family)
	}
}

// appendFamily flattens one family into one sample per metric.
func appendFamily(samples []series.Sample, family *dto.MetricFamily) []series.Sample {
	name := family.GetName()
	kind := kindOf(family.GetType())

	for _, metric := range family.GetMetric() {
		value, ok := valueOf(metric, family.GetType())
		if !ok {
			continue
		}

		labels := make(series.Labels, 0, len(metric.GetLabel()))
		for _, pair := range metric.GetLabel() {
			labels = append(labels, series.Label{Name: pair.GetName(), Value: pair.GetValue()})
		}

		labels.Sort()

		samples = append(samples, series.Sample{
			Family: name,
			Labels: labels,
			Kind:   kind,
			Value:  value,
		})
	}

	return samples
}

// valueOf extracts the scalar prism charts for a metric. Histograms and
// summaries are reduced to their observation count, which is the one number
// that is meaningful across every bucket and quantile; the distribution itself
// belongs to a later phase.
func valueOf(metric *dto.Metric, kind dto.MetricType) (float64, bool) {
	switch kind {
	case dto.MetricType_COUNTER:
		return metric.GetCounter().GetValue(), metric.GetCounter() != nil
	case dto.MetricType_GAUGE:
		return metric.GetGauge().GetValue(), metric.GetGauge() != nil
	case dto.MetricType_UNTYPED:
		return metric.GetUntyped().GetValue(), metric.GetUntyped() != nil
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		return float64(metric.GetHistogram().GetSampleCount()), metric.GetHistogram() != nil
	case dto.MetricType_SUMMARY:
		return float64(metric.GetSummary().GetSampleCount()), metric.GetSummary() != nil
	default:
		return 0, false
	}
}

// kindOf maps an exposition type onto the way prism displays it.
func kindOf(kind dto.MetricType) series.Kind {
	switch kind {
	case dto.MetricType_COUNTER:
		return series.KindCounter
	case dto.MetricType_GAUGE:
		return series.KindGauge
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		return series.KindHistogram
	case dto.MetricType_SUMMARY:
		return series.KindSummary
	case dto.MetricType_UNTYPED:
		return series.KindUntyped
	default:
		return series.KindUntyped
	}
}
