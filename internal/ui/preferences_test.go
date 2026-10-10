package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/vika2603/herdr-client/herdr"
	"github.com/vika2603/herdr-client/plugin/plugintest"

	"github.com/vika2603/herdr-palette/internal/palette"
	"github.com/vika2603/herdr-palette/internal/settings"
	"github.com/vika2603/herdr-palette/internal/theme"
)

func preferenceModel(t *testing.T) model {
	t.Helper()
	var ran []string
	m := newModel(context.Background(), plugintest.Env(plugintest.ConfigDir(t.TempDir())),
		&herdr.PluginInvocationContext{}, palette.List{Commands: testEntries(&ran)}, nil, theme.Defaults(), Toggle{})
	m.setSize(100, 14)
	return m
}

func manage(t *testing.T, m model) model {
	t.Helper()
	_, area, enabled := m.entryControl()
	if !enabled {
		t.Fatal("entry settings button is disabled")
	}
	m, _ = send(t, m, tea.MouseClickMsg{X: area.start + 1, Y: 0, Button: tea.MouseLeft})
	if m.editor == nil {
		t.Fatal("click did not open entry settings")
	}
	return m
}

func selectEntry(t *testing.T, m *model, id string) {
	t.Helper()
	for i, row := range m.ranked {
		if row.Entry.ID == id {
			m.cursor = i
			m.reveal()
			return
		}
	}
	t.Fatalf("entry %q is missing", id)
}

func TestHiddenEntriesCanBeRestoredAndPersistAcrossPopups(t *testing.T) {
	m := preferenceModel(t)
	m.enter(palette.ScopeHerdr)
	m.query.SetValue("split")
	m.rank()
	m = manage(t, m)
	selectEntry(t, &m, "a")
	m, _ = send(t, m, tea.KeyPressMsg{Code: ' ', Text: " "})
	if !m.preferences["a"].Hidden || !strings.HasPrefix(m.ranked[m.cursor].Entry.Title, "[ ] ") {
		t.Fatal("hidden row was not marked or disappeared from settings")
	}
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.editor != nil || m.scope != palette.ScopeHerdr || m.query.Value() != "split" || len(m.ranked) != 0 {
		t.Fatalf("return lost the scope/filter or retained the hidden row: %+v", m.ranked)
	}
	reopened := newModel(m.ctx, m.env, m.invocation, m.source, nil, m.colours, Toggle{})
	if len(reopened.ranked) != 2 || !reopened.preferences["a"].Hidden {
		t.Fatal("hidden state was not loaded in the next popup")
	}
	m = manage(t, reopened)
	selectEntry(t, &m, "a")
	m, _ = send(t, m, tea.MouseClickMsg{X: 4, Y: headerRows + m.lineOf(m.cursor) - m.offset, Button: tea.MouseLeft})
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	selectEntry(t, &m, "a")
	if m.preferences["a"].Hidden || m.ranked[m.cursor].Entry.Title != "split pane right" {
		t.Fatal("click did not restore the hidden entry")
	}
}

func TestAliasesSaveCancelResetAndKeepOriginalSearch(t *testing.T) {
	m := manage(t, preferenceModel(t))
	selectEntry(t, &m, "a")
	m, cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.editor.aliasID != "a" {
		t.Fatal("enter ran a command instead of editing its alias")
	}
	m, _ = send(t, m, tea.PasteMsg{Content: "  右侧分屏  "})
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = typeQuery(t, m, "右侧分屏")
	if len(m.ranked) != 1 || m.ranked[0].Entry.Title != "右侧分屏" {
		t.Fatalf("alias not displayed/searchable: %+v", m.ranked)
	}
	m.query.SetValue("split pane right")
	m.rank()
	if len(m.ranked) != 1 || m.ranked[0].Entry.ID != "a" || m.source.Commands[0].Title != "split pane right" {
		t.Fatal("original search/name was lost")
	}
	loaded, err := settings.LoadEntries(m.env)
	if err != nil || loaded["a"].Alias != "右侧分屏" {
		t.Fatalf("alias was not saved: %+v, %v", loaded, err)
	}
	m = manage(t, m)
	selectEntry(t, &m, "a")
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.query.SetValue("cancelled")
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.preferences["a"].Alias != "右侧分屏" || m.editor == nil || m.editor.aliasID != "" {
		t.Fatal("esc saved an alias or closed the settings list")
	}
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.query.SetValue("")
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.ranked[0].Entry.Title != "split pane right" {
		t.Fatal("empty alias did not restore the original title")
	}
}

func TestEntrySettingsIncludeAllKindsAndNeverRunActions(t *testing.T) {
	m := preferenceModel(t)
	count := 0
	m.source.Commands[0].Run = func(context.Context, palette.Exec) error { count++; return nil }
	m.source.Commands[0].Close = m.source.Commands[0].Run
	m.source.Commands = append(m.source.Commands, palette.Entry{ID: "selection", Title: "Selected text", NeedsSelection: true})
	m.source.Session.Entries = []palette.Entry{
		{ID: "tab:1", Title: "Tab one", Kind: palette.KindTab},
		{ID: "workspace:1", Title: "Workspace one", Kind: palette.KindWorkspace},
	}
	m = manage(t, m)
	for _, id := range []string{"selection", "tab:1", "workspace:1", "a"} {
		selectEntry(t, &m, id)
	}
	for _, key := range []tea.KeyPressMsg{{Code: 'x', Mod: tea.ModCtrl}, {Code: tea.KeyTab}, {Code: ' ', Text: " "}, {Code: tea.KeyEnter}} {
		var cmd tea.Cmd
		m, cmd = send(t, m, key)
		if cmd != nil {
			t.Fatalf("settings key %v dispatched a command", key)
		}
	}
	if count != 0 || m.editor.aliasID != "a" {
		t.Fatal("settings ran an action")
	}
}

func TestSessionRefreshKeepsPreferencesAndAliasEditing(t *testing.T) {
	m := preferenceModel(t)
	m.source.Session.Entries = []palette.Entry{{ID: "pane:1", Title: "Old name", Kind: palette.KindPane}}
	if !m.savePreference("pane:1", settings.EntryPreference{Hidden: true, Alias: "My pane"}) {
		t.Fatal(m.failure)
	}
	m.setOpen(palette.Session{Entries: []palette.Entry{{ID: "pane:1", Title: "New name", Kind: palette.KindAgent}}})
	for _, row := range m.ranked {
		if row.Entry.ID == "pane:1" {
			t.Fatal("refreshed hidden pane reappeared")
		}
	}
	m = manage(t, m)
	selectEntry(t, &m, "pane:1")
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.query.SetValue("Editing now")
	before := len(m.ranked)
	m, _ = send(t, m, openMsg{session: palette.Session{Entries: []palette.Entry{{ID: "pane:1", Title: "Latest name", Kind: palette.KindAgent}}}})
	if m.query.Value() != "Editing now" || len(m.ranked) != before || m.editor.aliasID != "pane:1" {
		t.Fatal("session refresh interrupted alias editing")
	}
	m, _ = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.preferences["pane:1"].Alias != "Editing now" || !m.preferences["pane:1"].Hidden {
		t.Fatal("alias save lost the hidden preference")
	}
	// Target rows share a command ID; a command alias must not rename them.
	m.editor = nil
	m.choosing = &chooser{}
	m.targets = []palette.Entry{{ID: "pane:1", Title: "Actual target"}}
	m.query.SetValue("")
	m.rank()
	if len(m.ranked) != 1 || m.ranked[0].Entry.Title != "Actual target" {
		t.Fatal("display preferences changed a command's target list")
	}
}

func TestPreferenceLoadAndSaveFailuresKeepExistingSettings(t *testing.T) {
	m := preferenceModel(t)
	path := m.env.ConfigPath(settings.EntriesFile)
	if err := os.WriteFile(path, []byte("[entries"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = newModel(m.ctx, m.env, m.invocation, m.source, nil, m.colours, Toggle{})
	m = manage(t, m)
	selectEntry(t, &m, "a")
	m.toggleEntry()
	if m.failure == "" || m.preferences["a"].Hidden {
		t.Fatal("malformed preferences were silently replaced")
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "[entries" {
		t.Fatalf("malformed file overwritten: %q, %v", content, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	m.preferencesErr = nil
	m.toggleEntry()
	if m.failure == "" || m.preferences["a"].Hidden {
		t.Fatal("failed write changed the in-memory preference")
	}
	files, err := filepath.Glob(m.env.ConfigPath(".entries-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("failed save left temporary files behind")
	}
}

func TestEntrySettingsFitThePopup(t *testing.T) {
	for _, width := range []int{6, 12, 24, 40, 72, 120} {
		m := manage(t, preferenceModel(t))
		m.setSize(width, 14)
		for _, editing := range []bool{false, true} {
			if editing {
				m.editAlias()
				m.query.SetValue(strings.Repeat("备注名称", 30))
			}
			view := ansi.Strip(m.View().Content)
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("settings at width %d overflowed: %q", width, line)
				}
			}
		}
	}
}

func TestEntrySettingsButtonBoundsHoverAndReturn(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := preferenceModel(t)
	m.query.SetValue("split")
	m.rank()
	label, area, enabled := m.entryControl()
	if !enabled || label != "[ Entry settings ]" {
		t.Fatal("settings button is missing")
	}
	toolbar := ansi.Strip(strings.Split(m.View().Content, "\n")[0])
	if toolbar[area.start:area.start+area.width] != label || area.start+area.width != m.cols()-1 {
		t.Fatalf("button not drawn at its click bounds: %q", toolbar)
	}
	for _, x := range []int{area.start, area.start + area.width - 1} {
		if m.targetAt(m.layout(), x, 0).kind != targetEntrySettings {
			t.Fatalf("visible button cell %d is not clickable", x)
		}
	}
	for _, x := range []int{area.start - 1, area.start + area.width} {
		if m.targetAt(m.layout(), x, 0).kind == targetEntrySettings {
			t.Fatalf("cell outside the button %d is clickable", x)
		}
	}
	plain := m.entryToolbar()
	m, _ = send(t, m, tea.MouseMotionMsg{X: area.start + 1, Y: 0})
	if m.entryToolbar() == plain || m.query.Value() != "split" || m.editor != nil {
		t.Fatal("hover did not highlight the button, or changed the query/list")
	}
	m = manage(t, m)
	selectEntry(t, &m, "a")
	m.editAlias()
	m.query.SetValue("unsaved alias")
	label, area, _ = m.entryControl()
	if label != "[ Cancel alias ]" {
		t.Fatal("alias editor has no cancel button")
	}
	m, cmd := click(t, m, area.start+1, 0)
	if cmd != nil || m.editor == nil || m.editor.aliasID != "" || m.preferences["a"].Alias != "" {
		t.Fatal("cancel button did not discard the alias and return to settings")
	}
	label, area, _ = m.entryControl()
	if label != "[ Back to palette ]" {
		t.Fatal("settings list has no return button")
	}
	m, cmd = click(t, m, area.start+1, 0)
	if cmd != nil || m.editor != nil || m.query.Value() != "split" || len(m.ranked) != 1 {
		t.Fatal("return button did not restore the original search")
	}
}

func TestEntrySettingsButtonIsDisabledDuringCommandsAndChoices(t *testing.T) {
	for _, state := range []string{"pending", "confirmation", "choices", "reply"} {
		t.Run(state, func(t *testing.T) {
			m := preferenceModel(t)
			switch state {
			case "pending":
				m.pending = true
			case "confirmation":
				m.confirming = &m.source.Commands[0]
			case "choices":
				m.choosing = &chooser{}
			case "reply":
				m.replying = &m.ranked[0]
			}
			_, area, enabled := m.entryControl()
			if enabled {
				t.Fatal("button enabled while another interaction is active")
			}
			before := m.confirming
			m, cmd := click(t, m, area.start+1, 0)
			if cmd != nil || m.editor != nil || m.confirming != before {
				t.Fatal("disabled button interrupted an active interaction")
			}
		})
	}
}

func TestCtrlSDoesNotOpenOrCloseEntrySettings(t *testing.T) {
	m := preferenceModel(t)
	m, _ = send(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.editor != nil {
		t.Fatal("ctrl+s still opens settings")
	}
	m = manage(t, m)
	m, _ = send(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.editor == nil {
		t.Fatal("ctrl+s still closes settings")
	}
}
