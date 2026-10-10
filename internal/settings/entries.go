package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/vika2603/herdr-client/plugin"
)

// EntriesFile keeps display preferences separate from the hand-edited config.
const EntriesFile = "entries.toml"

// EntryPreference is keyed by an entry's stable ID, never its display name.
type EntryPreference struct {
	Hidden bool   `toml:"hidden,omitempty"`
	Alias  string `toml:"alias,omitempty"`
}

type EntryPreferences struct {
	CustomKeysOnly bool                       `toml:"custom_keys_only,omitempty"`
	Entries        map[string]EntryPreference `toml:"entries"`
}

// LoadEntries treats a missing file as defaults. Other errors are returned so
// the editor cannot accidentally overwrite unreadable preferences.
func LoadEntries(env *plugin.Env) (EntryPreferences, error) {
	if env == nil || env.ConfigPath() == "" {
		return EntryPreferences{Entries: make(map[string]EntryPreference)}, nil
	}
	var file EntryPreferences
	_, err := toml.DecodeFile(env.ConfigPath(EntriesFile), &file)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if file.Entries == nil {
		file.Entries = make(map[string]EntryPreference)
	}
	return file, err
}

// SaveEntries replaces the file only after a complete document is written.
func SaveEntries(env *plugin.Env, preferences EntryPreferences) error {
	if env == nil || env.ConfigPath() == "" {
		return fmt.Errorf("plugin config directory is unavailable")
	}
	dir := env.ConfigPath()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".entries-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	err = toml.NewEncoder(file).Encode(preferences)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, EntriesFile))
}
