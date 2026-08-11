package series

import (
	"math"
	"sort"
)

const (
	// OtherLabel names the line that absorbs the tail of a pivot. Dropping the
	// tail silently would understate a total; naming it and counting its
	// members keeps the chart honest about what it left out.
	OtherLabel = "other"

	// AbsentLabel names the group of series that lack the pivot label
	// entirely, which is different from having it set to the empty string.
	AbsentLabel = "∅ unset"
)

// Line is one plotted line: either a single series or several summed together
// by a pivot.
type Line struct {
	Label   string
	Members int
	Last    float64

	views []SeriesView
}

// Window returns the line's values from oldest to newest inclusive.
//
// A bucket is only a gap if every member is missing there. Summing a partial
// set would show a total dropping whenever one contributor missed a scrape,
// which looks like an incident and is not one.
func (l Line) Window(oldest, latest int64) []float64 {
	if latest < oldest {
		return nil
	}

	out := make([]float64, 0, latest-oldest+1)

	for bucket := oldest; bucket <= latest; bucket++ {
		sum, present := 0.0, false

		for _, view := range l.views {
			if value, ok := view.At(bucket); ok {
				sum += value
				present = true
			}
		}

		if !present {
			out = append(out, math.NaN())

			continue
		}

		out = append(out, sum)
	}

	return out
}

// Selection describes which series to plot and how to group them.
type Selection struct {
	// Family restricts the selection to one metric family. Empty means any.
	Family string

	// Match is the compiled filter. Nil means everything matches.
	Match func(family string, labels Labels) bool

	// PivotKey is the label to split on. Empty means no pivot: one line per
	// series.
	PivotKey string

	// Limit caps how many lines are returned, including the "other" line.
	Limit int
}

// matches reports whether a series passes the selection.
func (s Selection) matches(view SeriesView) bool {
	if s.Family != "" && view.Family != s.Family {
		return false
	}

	if s.Match == nil {
		return true
	}

	return s.Match(view.Family, view.Labels)
}

// Matching returns the series a selection admits, in snapshot order.
func (s *Snapshot) Matching(sel Selection) []SeriesView {
	out := make([]SeriesView, 0, len(s.Series))

	for _, view := range s.Series {
		if sel.matches(view) {
			out = append(out, view)
		}
	}

	return out
}

// Lines returns the lines to plot for a selection.
//
// Without a pivot this is one line per series. With one, series are grouped by
// the value of the pivot label and summed - the prism moment, where a single
// metric separates into its components.
func (s *Snapshot) Lines(sel Selection) []Line {
	views := s.Matching(sel)
	if len(views) == 0 {
		return nil
	}

	if sel.PivotKey == "" {
		return s.plain(views, sel.Limit)
	}

	return s.pivot(views, sel, sel.Limit)
}

// plain returns one line per series.
func (s *Snapshot) plain(views []SeriesView, limit int) []Line {
	if limit > 0 && len(views) > limit {
		views = views[:limit]
	}

	lines := make([]Line, 0, len(views))

	for _, view := range views {
		lines = append(lines, Line{
			Label:   view.Labels.String(),
			Members: 1,
			Last:    view.Last,
			views:   []SeriesView{view},
		})
	}

	return lines
}

// pivot groups series by a label value and sums each group.
func (s *Snapshot) pivot(views []SeriesView, sel Selection, limit int) []Line {
	groups := make(map[string][]SeriesView, len(views))

	for _, view := range views {
		key, ok := view.Labels.Lookup(sel.PivotKey)
		if !ok {
			key = AbsentLabel
		}

		groups[key] = append(groups[key], view)
	}

	lines := make([]Line, 0, len(groups))

	for key, members := range groups {
		lasts := make([]float64, 0, len(members))
		for _, member := range members {
			lasts = append(lasts, member.Last)
		}

		lines = append(lines, Line{
			Label:   key,
			Members: len(members),
			Last:    sumPresent(lasts),
			views:   members,
		})
	}

	// Largest first: the line an operator is looking for is almost always the
	// one carrying the most traffic. A line with no current value sorts to the
	// bottom rather than comparing unequal to everything, itself included,
	// which is what a bare `!=` against a not-a-number would have done.
	sort.Slice(lines, func(i, j int) bool {
		left, right := lines[i].Last, lines[j].Last

		switch {
		case math.IsNaN(left) && math.IsNaN(right):
			return lines[i].Label < lines[j].Label
		case math.IsNaN(left):
			return false
		case math.IsNaN(right):
			return true
		case left != right:
			return left > right
		default:
			return lines[i].Label < lines[j].Label
		}
	})

	return collapse(lines, limit)
}

// collapse folds everything past the limit into a single "other" line.
func collapse(lines []Line, limit int) []Line {
	if limit <= 0 || len(lines) <= limit {
		return lines
	}

	kept := lines[:limit-1]
	tail := lines[limit-1:]

	other := Line{Label: OtherLabel}
	lasts := make([]float64, 0, len(tail))

	for _, line := range tail {
		other.Members += line.Members
		other.views = append(other.views, line.views...)
		lasts = append(lasts, line.Last)
	}

	other.Last = sumPresent(lasts)

	return append(kept, other)
}

// sumPresent totals the values that exist, and reports no value at all only
// when none of them do.
//
// This is the rule Window applies bucket by bucket, stated once for the single
// figure the legend shows. Treating one absent contributor as a zero would make
// a total dip whenever a member missed a scrape; treating it as unknown would
// blank a total that the rest of the members still describe perfectly well.
func sumPresent(values []float64) float64 {
	sum, present := 0.0, false

	for _, value := range values {
		if math.IsNaN(value) {
			continue
		}

		sum += value
		present = true
	}

	if !present {
		return math.NaN()
	}

	return sum
}

// LabelKeys returns the label names present on a family's series, sorted, so
// that the pivot control has something to offer.
func (s *Snapshot) LabelKeys(family string) []string {
	seen := make(map[string]struct{})

	for _, view := range s.Series {
		if family != "" && view.Family != family {
			continue
		}

		for _, label := range view.Labels {
			seen[label.Name] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
