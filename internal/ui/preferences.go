package ui

import (
	"fmt"
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/vika2603/herdr-palette/internal/palette"
	"github.com/vika2603/herdr-palette/internal/settings"
)

// entryEditor remembers the list being configured and the filter used while
// editing an alias. The source entries themselves always retain their names.
type entryEditor struct {
	query      string
	selected   string
	filter     string
	aliasID    string
	aliasTitle string
}

// entryControl sits above the query so it never takes room from search text
// or a script's argument fields. Drawing and mouse hit testing share its bounds.
func (m model) entryControl() (string, extent, bool) {
	if m.replying != nil {
		return "", extent{}, false
	}
	label, short := "[ Entry settings ]", "[ Settings ]"
	if m.editor != nil {
		label, short = "[ Back to palette ]", "[ Back ]"
		if m.editor.aliasID != "" {
			label, short = "[ Cancel alias ]", "[ Cancel ]"
		}
	}
	if lipgloss.Width(label) > m.cols()-2 {
		label = short
	}
	if lipgloss.Width(label) > m.cols()-2 {
		label = "[+]"
		if m.editor != nil {
			label = "[<]"
		}
	}
	width := lipgloss.Width(label)
	enabled := m.editor != nil || m.choosing == nil && m.confirming == nil && !m.pending
	return label, extent{start: m.cols() - width - 1, width: width}, enabled
}

func (m model) entryToolbar() string {
	label, area, enabled := m.entryControl()
	if label == "" {
		return strings.Repeat(" ", m.cols())
	}
	style := m.styles.plain.faint
	if enabled {
		style = m.styles.field.text.Foreground(m.colours.Accent).Bold(true)
		if m.pointer.Y == 0 && area.contains(m.pointer.X) {
			style = m.styles.fieldHovered.text.Foreground(m.colours.Accent).Bold(true).Underline(true)
		}
	}
	return strings.Repeat(" ", area.start) + style.Render(label) + " "
}

func (m *model) manageEntries() {
	if m.editor != nil {
		m.query.SetValue(m.editor.query)
		m.query.Placeholder = placeholder(m.scope)
		selected := m.editor.selected
		m.editor = nil
		m.failure, m.notice = "", ""
		m.rankKeeping(selected)
		return
	}
	if m.pending || m.choosing != nil || m.confirming != nil {
		return
	}
	selected := ""
	if m.cursor < len(m.ranked) {
		selected = m.ranked[m.cursor].Entry.ID
	}
	m.editor = &entryEditor{query: m.query.Value(), selected: selected}
	m.args = nil
	m.query.SetValue("")
	m.query.Placeholder = "Search entries, including hidden ones…"
	m.failure, m.notice = "", ""
	m.rankKeeping(selected)
}

func (m model) displayEntries(entries []palette.Entry) []palette.Entry {
	rows := make([]palette.Entry, 0, len(entries))
	for _, entry := range entries {
		preference := m.preferences[entry.ID]
		if preference.Hidden && m.editor == nil {
			continue
		}
		original := entry.Title
		if preference.Alias != "" {
			entry.Title = preference.Alias
			entry.Search = strings.TrimSpace(entry.Search + " " + original)
		}
		if m.editor != nil {
			check := "[x] "
			if preference.Hidden {
				check = "[ ] "
			}
			entry.Title = check + entry.Title
			entry.Detail = original
			entry.Status = ""
		}
		rows = append(rows, entry)
	}
	return rows
}

func (m *model) rankKeeping(id string) {
	m.rank()
	for i, row := range m.ranked {
		if row.Entry.ID == id {
			m.cursor = i
			m.reveal()
			break
		}
	}
}

func (m *model) savePreference(id string, preference settings.EntryPreference) bool {
	if m.preferencesErr != nil {
		m.failure = "cannot save entries: " + m.preferencesErr.Error()
		return false
	}
	next := maps.Clone(m.preferences)
	if preference == (settings.EntryPreference{}) {
		delete(next, id)
	} else {
		next[id] = preference
	}
	if err := settings.SaveEntries(m.env, next); err != nil {
		m.failure = "cannot save entries: " + err.Error()
		return false
	}
	m.preferences = next
	m.failure, m.notice = "", ""
	return true
}

func (m *model) toggleEntry() {
	if m.cursor >= len(m.ranked) {
		return
	}
	id := m.ranked[m.cursor].Entry.ID
	preference := m.preferences[id]
	preference.Hidden = !preference.Hidden
	if m.savePreference(id, preference) {
		m.rankKeeping(id)
	}
}

func (m *model) editAlias() {
	if m.cursor >= len(m.ranked) {
		return
	}
	id := m.ranked[m.cursor].Entry.ID
	for _, entry := range m.source.All() {
		if entry.ID == id {
			editor := *m.editor
			editor.filter, editor.aliasID, editor.aliasTitle = m.query.Value(), id, entry.Title
			m.editor = &editor
			m.query.SetValue(m.preferences[id].Alias)
			m.query.CursorEnd()
			m.query.Placeholder = entry.Title + " (empty restores original)"
			m.failure, m.notice = "", ""
			return
		}
	}
}

func (m *model) finishAlias(save bool) {
	editor := *m.editor
	if save {
		preference := m.preferences[editor.aliasID]
		preference.Alias = strings.TrimSpace(strings.Map(printable, m.query.Value()))
		if !m.savePreference(editor.aliasID, preference) {
			return
		}
	}
	m.query.SetValue(editor.filter)
	m.query.Placeholder = "Search entries, including hidden ones…"
	id := editor.aliasID
	editor.aliasID, editor.aliasTitle = "", ""
	m.editor = &editor
	m.failure, m.notice = "", ""
	m.rankKeeping(id)
}

func (m model) keyEntries(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.editor.aliasID != "" {
		switch msg.String() {
		case "enter":
			m.finishAlias(true)
		case "esc":
			m.finishAlias(false)
		default:
			return m.editAliasInput(msg)
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.manageEntries()
	case "space":
		m.toggleEntry()
	case "enter", "ctrl+r":
		m.editAlias()
	case "up", "ctrl+p":
		m.step(-1)
	case "down", "ctrl+n":
		m.step(1)
	case "pgup":
		m.move(-m.rows())
	case "pgdown":
		m.move(m.rows())
	case "tab", "ctrl+x", "ctrl+o":
		// Editing display preferences never executes or closes an entry.
	default:
		return m.editQuery(msg)
	}
	return m, nil
}

func (m model) editAliasInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	m.failure, m.notice = "", ""
	return m, cmd
}

func (m model) entriesFooter() string {
	hints := []hint{{"space/click", "show/hide"}, {spelling.name("enter"), "alias"}, {spelling.name("esc"), "back"}}
	left := fmt.Sprintf(" %d entries · saved automatically", len(m.ranked))
	if m.editor.aliasID != "" {
		hints = []hint{{spelling.name("enter"), "save"}, {spelling.name("esc"), "cancel"}}
		left = " " + m.editor.aliasTitle + " · empty restores original"
	}
	keys := m.fitHints(hints)
	return m.spread(m.styles.plain.meta.Render(truncate(left, max(m.cols()-lipgloss.Width(keys)-2, 1))), keys)
}
