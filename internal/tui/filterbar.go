package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/pantherhawk/prism/internal/filter"
)

// filterPrompt is drawn before the editable text.
const filterPrompt = "/ "

// newInput returns the filter editor.
//
// textinput handles the editing - unicode widths, word motions, paste - but its
// View is not used. It renders a styled string, and this frame is a grid of
// cells; splicing escape sequences into it would break every column after the
// field. The value and cursor position are drawn into the grid instead.
func newInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "cluster=~outbound.*  status!=200  upstream_rq"
	input.CharLimit = 200

	return input
}

// onFilterKey routes a keypress while the filter bar has focus.
func (m model) onFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.commitFilter()

	case "esc":
		return m.cancelFilter(), nil
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

// commitFilter compiles what was typed.
//
// A filter that does not compile leaves the operator in the field with the
// reason shown, rather than being thrown back to a dashboard that silently
// ignored them.
func (m model) commitFilter() (tea.Model, tea.Cmd) {
	compiled, err := filter.Parse(m.input.Value())
	if err != nil {
		m.filterErr = err.Error()

		return m, nil
	}

	m.filterErr = ""
	m.active = compiled
	m.mode = modeNormal
	m.input.Blur()
	m.clampSelection()
	m.retarget()

	return m, m.animate()
}

// cancelFilter restores the filter that was in force before editing began.
func (m model) cancelFilter() model {
	m.filterErr = ""
	m.mode = modeNormal
	m.input.SetValue(m.active.String())
	m.input.Blur()

	return m
}

// beginFilter focuses the filter bar.
func (m model) beginFilter() (tea.Model, tea.Cmd) {
	m.mode = modeFilter
	m.filterErr = ""
	m.input.SetValue(m.active.String())
	m.input.SetCursor(len(m.active.String()))

	return m, m.input.Focus()
}

// cyclePivot advances the pivot to the next label key available on the selected
// family, wrapping through "no pivot".
//
// A picker would be better once there are many keys; cycling is discoverable
// because the footer names the current pivot, and it costs one keystroke.
func (m model) cyclePivot() model {
	options := m.pivotOptions()
	if len(options) <= 1 {
		return m
	}

	next := 0

	for i, key := range options {
		if key == m.pivotKey {
			next = (i + 1) % len(options)

			break
		}
	}

	m.pivotKey = options[next]
	m.retarget()

	return m
}

// pivotOptions lists the pivot targets, starting with none.
func (m model) pivotOptions() []string {
	if m.snapshot == nil {
		return []string{""}
	}

	selected := m.selectedSeries()
	if selected == nil {
		return []string{""}
	}

	return append([]string{""}, m.snapshot.LabelKeys(selected.Family)...)
}
