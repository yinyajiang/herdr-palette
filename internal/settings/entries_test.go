package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vika2603/herdr-client/plugin/plugintest"
)

func TestEntryPreferencesRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	env := plugintest.Env(plugintest.ConfigDir(dir))
	preferences := EntryPreferences{
		"plugin:herdr.tools/open": {Hidden: true, Alias: "我的工具 \"开发\""},
		"script:/some path/run":   {Alias: "Run it"},
	}
	if got, err := LoadEntries(env); err != nil || len(got) != 0 {
		t.Fatalf("missing preferences = %v, %v", got, err)
	}
	if err := SaveEntries(env, preferences); err != nil {
		t.Fatal(err)
	}
	got, err := LoadEntries(env)
	if err != nil || !reflect.DeepEqual(got, preferences) {
		t.Fatalf("reloaded = %v, %v, want %v", got, err, preferences)
	}
	delete(preferences, "script:/some path/run")
	if err := SaveEntries(env, preferences); err != nil {
		t.Fatal(err)
	}
	got, err = LoadEntries(env)
	if err != nil || !reflect.DeepEqual(got, preferences) {
		t.Fatalf("replacement = %v, %v, want %v", got, err, preferences)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != EntriesFile {
		t.Fatalf("temporary files left behind: %v, %v", files, err)
	}
}

func TestEntryPreferencesDoNotChangeConfig(t *testing.T) {
	dir := t.TempDir()
	env := plugintest.Env(plugintest.ConfigDir(dir))
	config := "# my colours\naccent = \"#A48BFF\"\nscript_dirs = []\n"
	if err := os.WriteFile(filepath.Join(dir, File), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveEntries(env, EntryPreferences{"one": {Hidden: true}}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, File)); err != nil || string(got) != config {
		t.Fatalf("config changed: %q, %v", got, err)
	}
}

func TestEntryPreferencesReportInvalidFile(t *testing.T) {
	dir := t.TempDir()
	env := plugintest.Env(plugintest.ConfigDir(dir))
	if err := os.WriteFile(filepath.Join(dir, EntriesFile), []byte("[entries"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEntries(env); err == nil {
		t.Fatal("malformed preferences were accepted")
	}
}

func TestEntryPreferencesReportWriteFailure(t *testing.T) {
	dir := t.TempDir()
	env := plugintest.Env(plugintest.ConfigDir(dir))
	// A directory at the destination makes the final rename fail.
	if err := os.Mkdir(filepath.Join(dir, EntriesFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveEntries(env, EntryPreferences{"one": {Hidden: true}}); err == nil {
		t.Fatal("failed replacement reported success")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", files, err)
	}
}
