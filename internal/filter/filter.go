// Package filter compiles label matcher expressions.
//
// An expression is compiled once, when the operator presses enter, and the
// result is a value that answers a boolean question. Nothing here is parsed
// during a frame: a regex recompiled sixty times a second for two hundred
// series is the difference between a chart that feels instant and one that
// does not.
package filter

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/pantherhawk/prism/internal/series"
)

// FamilyLabel is the pseudo-label naming the metric family, matching the
// convention Prometheus uses so that the syntax is not a surprise.
const FamilyLabel = "__name__"

var (
	// ErrEmptyMatcher is returned when a term has no label name.
	ErrEmptyMatcher = errors.New("matcher has no label name")
	// ErrBadRegex is returned when a regex matcher does not compile.
	ErrBadRegex = errors.New("invalid regular expression")
)

// op is the comparison a matcher performs.
type op uint8

const (
	opEqual op = iota
	opNotEqual
	opMatch
	opNotMatch
)

// operators are scanned longest first so that "!=" is not read as "=".
var operators = []struct { //nolint:gochecknoglobals // immutable lookup table
	token string
	op    op
}{
	{"=~", opMatch},
	{"!~", opNotMatch},
	{"!=", opNotEqual},
	{"=", opEqual},
}

// matcher is one compiled term.
type matcher struct {
	label string
	op    op
	value string
	regex *regexp.Regexp
}

// Filter is a compiled expression. The zero value matches everything.
type Filter struct {
	source   string
	matchers []matcher
	terms    []string
}

// Parse compiles an expression.
//
// Terms are separated by whitespace or commas and are combined with AND, which
// is what an operator narrowing a search expects. A bare word with no operator
// is a substring match on the metric name, because that is what people type
// first and refusing it would be pedantry.
func Parse(expr string) (*Filter, error) {
	f := &Filter{source: strings.TrimSpace(expr)}

	for _, term := range strings.FieldsFunc(expr, isSeparator) {
		parsed, bare, err := parseTerm(term)
		if err != nil {
			return nil, err
		}

		if bare != "" {
			f.terms = append(f.terms, strings.ToLower(bare))

			continue
		}

		f.matchers = append(f.matchers, parsed)
	}

	return f, nil
}

// isSeparator reports whether a rune separates two terms.
func isSeparator(r rune) bool {
	return r == ' ' || r == '\t' || r == ','
}

// parseTerm compiles one term, returning it as a bare substring when it holds
// no operator.
func parseTerm(term string) (matcher, string, error) {
	for _, candidate := range operators {
		label, value, found := strings.Cut(term, candidate.token)
		if !found {
			continue
		}

		label = strings.TrimSpace(label)
		if label == "" {
			return matcher{}, "", fmt.Errorf("%w: %q", ErrEmptyMatcher, term)
		}

		return compile(label, candidate.op, unquote(value))
	}

	return matcher{}, term, nil
}

// compile builds a matcher, compiling the regex if there is one.
func compile(label string, operation op, value string) (matcher, string, error) {
	m := matcher{label: label, op: operation, value: value}

	if operation != opMatch && operation != opNotMatch {
		return m, "", nil
	}

	// Anchored, as Prometheus anchors its own regex matchers: an unanchored
	// "5" would match "500" and "1500" and surprise everyone.
	regex, err := regexp.Compile("^(?:" + value + ")$")
	if err != nil {
		return matcher{}, "", fmt.Errorf("%w: %s: %w", ErrBadRegex, value, err)
	}

	m.regex = regex

	return m, "", nil
}

// unquote strips one layer of matching quotes.
func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}

	return value
}

// IsZero reports whether the filter matches everything.
func (f *Filter) IsZero() bool {
	return f == nil || (len(f.matchers) == 0 && len(f.terms) == 0)
}

// String returns the expression as it was typed.
func (f *Filter) String() string {
	if f == nil {
		return ""
	}

	return f.source
}

// Match reports whether a series satisfies every term.
func (f *Filter) Match(family string, labels series.Labels) bool {
	if f.IsZero() {
		return true
	}

	lower := strings.ToLower(family)
	for _, term := range f.terms {
		if !strings.Contains(lower, term) {
			return false
		}
	}

	for _, m := range f.matchers {
		if !m.match(family, labels) {
			return false
		}
	}

	return true
}

// match applies one matcher.
func (m matcher) match(family string, labels series.Labels) bool {
	value := labels.Get(m.label)
	if m.label == FamilyLabel {
		value = family
	}

	switch m.op {
	case opEqual:
		return value == m.value
	case opNotEqual:
		return value != m.value
	case opMatch:
		return m.regex.MatchString(value)
	case opNotMatch:
		return !m.regex.MatchString(value)
	default:
		return false
	}
}
