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

const (
	customKeysOnlyID = "settings:custom-keybindings-only"
	selectAllID      = "settings:select-all"
	hideAllID        = "settings:hide-all"
	settingsControls = 3
)

func (m model) customKeysOnlyRow() palette.Ranked {
	check := "[ ] "
	if m.preferences.CustomKeysOnly {
		check = "[x] "
	}
	return palette.Ranked{
		Entry:  palette.Entry{ID: customKeysOnlyID, Title: check + "Only custom keybindings", Type: "Settings"},
		Detail: "Hide built-in commands and entries without custom shortcuts",
	}
}

func (m model) bulkVisibilityRows() []palette.Ranked {
	return []palette.Ranked{
		{Entry: palette.Entry{ID: selectAllID, Title: "[ Select all entries ]", Type: "Settings"},
			Detail: "Show every entry, including outside this search"},
		{Entry: palette.Entry{ID: hideAllID, Title: "[ Hide all entries ]", Type: "Settings"},
			Detail: "Hide every entry, including outside this search"},
	}
}

func (m model) onSettingsControl() bool {
	if m.cursor >= len(m.ranked) {
		return false
	}
	switch m.ranked[m.cursor].Entry.ID {
	case customKeysOnlyID, selectAllID, hideAllID:
		return true
	}
	return false
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
		preference := m.preferences.Entries[entry.ID]
		if preference.Hidden && m.editor == nil {
			continue
		}
		if m.editor == nil && m.preferences.CustomKeysOnly &&
			((entry.Kind != palette.KindCustom && entry.Kind != palette.KindPlugin) || strings.TrimSpace(entry.Key) == "") {
			continue
		}
		original := entry.Title
		if preference.Alias != "" {
			entry.Title = preference.Alias
			entry.Search = strings.TrimSpace(entry.Search + " " + original)
		}
		if m.editor != nil {
			entry.Detail = original
			entry.Status = ""
		}
		rows = append(rows, entry)
	}
	return rows
}

// visibility markers are presentation only: including them in ranking would
// move a row to the top as soon as its checkbox changed from [x] to [ ].
func (m model) markVisibility(row palette.Ranked) palette.Ranked {
	check := "[x] "
	if m.preferences.Entries[row.Entry.ID].Hidden {
		check = "[ ] "
	}
	namespace := len([]rune(row.Entry.Namespace()))
	for i, position := range row.Matched {
		if position >= namespace {
			row.Matched[i] += len([]rune(check))
		}
	}
	row.Entry.Title = check + row.Entry.Title
	return row
}

func (m *model) rankKeeping(id string) {
	offset, cursor := m.offset, m.cursor
	m.rank()
	m.cursor = clamp(cursor, len(m.ranked))
	for i, row := range m.ranked {
		if row.Entry.ID == id {
			m.cursor = i
			break
		}
	}
	m.offset = min(offset, max(len(m.lines)-m.rows(), 0))
	m.reveal()
}

func (m *model) savePreference(id string, preference settings.EntryPreference) bool {
	next := m.preferences
	next.Entries = maps.Clone(m.preferences.Entries)
	if preference == (settings.EntryPreference{}) {
		delete(next.Entries, id)
	} else {
		next.Entries[id] = preference
	}
	return m.savePreferences(next)
}

func (m *model) savePreferences(next settings.EntryPreferences) bool {
	if m.preferencesErr != nil {
		m.failure = "cannot save entries: " + m.preferencesErr.Error()
		return false
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
	if id == selectAllID || id == hideAllID {
		m.setAllHidden(id == hideAllID, id)
		return
	}
	if id == customKeysOnlyID {
		next := m.preferences
		next.CustomKeysOnly = !next.CustomKeysOnly
		if m.savePreferences(next) {
			m.rankKeeping(id)
		}
		return
	}
	preference := m.preferences.Entries[id]
	preference.Hidden = !preference.Hidden
	if m.savePreference(id, preference) {
		m.rankKeeping(id)
	}
}

func (m *model) setAllHidden(hidden bool, selected string) {
	entries := m.source.All()
	if len(entries) == 0 {
		return
	}
	next := m.preferences
	next.Entries = maps.Clone(m.preferences.Entries)
	for _, entry := range entries {
		preference := next.Entries[entry.ID]
		preference.Hidden = hidden
		if preference == (settings.EntryPreference{}) {
			delete(next.Entries, entry.ID)
		} else {
			next.Entries[entry.ID] = preference
		}
	}
	if m.savePreferences(next) {
		m.rankKeeping(selected)
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
			m.query.SetValue(m.preferences.Entries[id].Alias)
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
		preference := m.preferences.Entries[editor.aliasID]
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
	case "enter":
		if m.onSettingsControl() {
			m.toggleEntry()
		} else {
			m.editAlias()
		}
	case "ctrl+r":
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
	left := fmt.Sprintf(" %d entries · saved automatically", max(len(m.ranked)-settingsControls, 0))
	if m.onSettingsControl() {
		act := "apply"
		if m.ranked[m.cursor].Entry.ID == customKeysOnlyID {
			act = "toggle"
		}
		hints = []hint{{"space/click", act}, {spelling.name("enter"), act}, {spelling.name("esc"), "back"}}
	}
	if m.editor.aliasID != "" {
		hints = []hint{{spelling.name("enter"), "save"}, {spelling.name("esc"), "cancel"}}
		left = " " + m.editor.aliasTitle + " · empty restores original"
	}
	keys := m.fitHints(hints)
	return m.spread(m.styles.plain.meta.Render(truncate(left, max(m.cols()-lipgloss.Width(keys)-2, 1))), keys)
}
