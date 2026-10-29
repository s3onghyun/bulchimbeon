// Package settings is the settings front end outside macOS: a small JSON file with the
// same public key names and values as the macOS UserDefaults domain (docs/compatibility.md).
// `bulchimbeon config set` and the tray's uniform menu write it; the daemon and the tray
// re-read it.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/sprites"
)

// KeyLanguage picks the language outside macOS (macOS uses the app's AppleLanguages):
// "system" (or unset) follows LANGUAGE/LC_*/LANG.
const KeyLanguage = "language"

// KeyXfceKeepAwake lets the daemon hold XFCE's suspend inhibitor on duty. Off by default,
// because xfce4-power-manager then also keeps the screen on and unlocked.
const KeyXfceKeepAwake = "xfceKeepAwake"

// Settings are the effective values. Invalid stored values fall back to the defaults,
// as on macOS.
type Settings struct {
	DutyMode             string   `json:"dutyMode"`
	BatteryFloor         int      `json:"batteryFloor"`
	AgentGraceMinutes    int      `json:"agentGraceMinutes"`
	AgentWatchCountsIdle bool     `json:"agentWatchCountsIdle"`
	CustomAgents         []string `json:"customAgents"`
	UniformTheme         string   `json:"uniformTheme"`
	Language             string   `json:"language"`
	XfceKeepAwake        bool     `json:"xfceKeepAwake"`
	CheckForUpdates      bool     `json:"checkForUpdates"`
}

// Defaults are the documented defaults.
func Defaults() Settings {
	return Settings{DutyMode: string(engine.ModeSystem), BatteryFloor: engine.DefaultBatteryFloor,
		AgentGraceMinutes: engine.DefaultGraceMinutes, CustomAgents: []string{}, UniformTheme: engine.DefaultUniformTheme,
		Language: "system", CheckForUpdates: engine.DefaultCheckForUpdates}
}

// Keys that `config set` accepts, in display order.
var Keys = []string{engine.KeyDutyMode, engine.KeyBatteryFloor, engine.KeyAgentGraceMinutes,
	engine.KeyAgentWatchCountsIdle, engine.KeyCustomAgents, engine.KeyUniformTheme, KeyLanguage, KeyXfceKeepAwake,
	engine.KeyCheckForUpdates}

// macOSOnly are public keys with no meaning outside the macOS app.
var macOSOnly = []string{engine.KeyPatrolEnabled, engine.KeyPatrolScale}

// Load reads the settings file. A missing file means defaults. Problems lists stored
// values that were ignored.
func Load(path string) (s Settings, problems []string, err error) {
	s = Defaults()
	raw, err := readRaw(path)
	if err != nil {
		return s, nil, err
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := apply(&s, key, raw[key]); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", key, err))
		}
	}
	return s, problems, nil
}

func apply(s *Settings, key string, value json.RawMessage) error {
	switch key {
	case engine.KeyDutyMode:
		var v string
		if json.Unmarshal(value, &v) != nil || !slices.Contains(engine.DutyModes, engine.DutyMode(v)) {
			return errors.New("expected display, system or lid")
		}
		s.DutyMode = v
	case engine.KeyBatteryFloor:
		var v int
		if json.Unmarshal(value, &v) != nil || !engine.ValidBatteryFloor(v) {
			return errors.New("expected 0, 10, 20 or 30")
		}
		s.BatteryFloor = v
	case engine.KeyAgentGraceMinutes:
		var v int
		if json.Unmarshal(value, &v) != nil || !engine.ValidGraceMinutes(v) {
			return errors.New("expected 1, 3, 5 or 10")
		}
		s.AgentGraceMinutes = v
	case engine.KeyAgentWatchCountsIdle:
		var v bool
		if json.Unmarshal(value, &v) != nil {
			return errors.New("expected true or false")
		}
		s.AgentWatchCountsIdle = v
	case engine.KeyCustomAgents:
		var v []string
		if json.Unmarshal(value, &v) != nil {
			return errors.New("expected a list of names")
		}
		s.CustomAgents = v // engine.CustomKinds drops invalid names, as on macOS
	case engine.KeyUniformTheme:
		var v string
		if json.Unmarshal(value, &v) != nil || !sprites.Valid(v) {
			return errors.New("expected " + themeList())
		}
		s.UniformTheme = v
	case KeyLanguage:
		var v string
		if json.Unmarshal(value, &v) != nil || !slices.Contains([]string{"system", "ko", "en", "zh"}, v) {
			return errors.New("expected system, ko, en or zh")
		}
		s.Language = v
	case KeyXfceKeepAwake:
		var v bool
		if json.Unmarshal(value, &v) != nil {
			return errors.New("expected true or false")
		}
		s.XfceKeepAwake = v
	case engine.KeyCheckForUpdates:
		var v bool
		if json.Unmarshal(value, &v) != nil {
			return errors.New("expected true or false")
		}
		s.CheckForUpdates = v
	case engine.KeyPatrolEnabled, engine.KeyPatrolScale:
		// macOS only; kept in the file, ignored here
	default:
		return errors.New("unknown key")
	}
	return nil
}

// themeList is "default, navy, arctic or desert".
func themeList() string {
	var ids []string
	for _, t := range sprites.Themes() {
		ids = append(ids, t.ID)
	}
	if len(ids) < 2 {
		return strings.Join(ids, "")
	}
	return strings.Join(ids[:len(ids)-1], ", ") + " or " + ids[len(ids)-1]
}

// Parse validates a `config set` value and returns it as stored JSON.
func Parse(key string, values []string) (json.RawMessage, error) {
	if slices.Contains(macOSOnly, key) {
		return nil, fmt.Errorf("%s only applies to the macOS app", key)
	}
	if !slices.Contains(Keys, key) {
		return nil, fmt.Errorf("unknown key %q (keys: %s)", key, strings.Join(Keys, ", "))
	}
	if key == engine.KeyCustomAgents {
		for _, name := range values {
			if !engine.ValidCustomAgentName(name) {
				return nil, fmt.Errorf("invalid agent name %q: letters, digits, '.', '_' or '-', up to 32, starting with a letter or digit", name)
			}
		}
		if len(values) > engine.MaxCustomAgents {
			return nil, fmt.Errorf("at most %d custom agents", engine.MaxCustomAgents)
		}
		return json.Marshal(append([]string{}, values...))
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("%s takes exactly one value", key)
	}
	v := values[0]
	var raw json.RawMessage
	switch key {
	case engine.KeyBatteryFloor, engine.KeyAgentGraceMinutes:
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number", key)
		}
		raw, _ = json.Marshal(n)
	case engine.KeyAgentWatchCountsIdle, KeyXfceKeepAwake, engine.KeyCheckForUpdates:
		b, err := strconv.ParseBool(v)
		if err != nil || (v != "true" && v != "false") {
			return nil, fmt.Errorf("%s must be true or false", key)
		}
		raw, _ = json.Marshal(b)
	default:
		raw, _ = json.Marshal(v)
	}
	var probe Settings
	if err := apply(&probe, key, raw); err != nil {
		return nil, fmt.Errorf("%s: %v", key, err)
	}
	return raw, nil
}

// Set stores one key.
func Set(path, key string, values []string) error {
	value, err := Parse(key, values)
	if err != nil {
		return err
	}
	raw, err := readRaw(path)
	if err != nil {
		return err
	}
	raw[key] = value
	return writeRaw(path, raw)
}

// Unset removes one key, so it falls back to its default.
func Unset(path, key string) error {
	raw, err := readRaw(path)
	if err != nil {
		return err
	}
	delete(raw, key)
	return writeRaw(path, raw)
}

// Stored returns the raw stored value of key, if any.
func Stored(path, key string) (json.RawMessage, bool, error) {
	raw, err := readRaw(path)
	if err != nil {
		return nil, false, err
	}
	v, ok := raw[key]
	return v, ok, nil
}

func readRaw(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	raw := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) == 0 {
		return raw, nil
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return raw, nil
}

func writeRaw(path string, raw map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'))
}

// WriteFileAtomic writes a 0600 file through a temporary file and rename, creating the
// directory as 0700, so readers never see half a file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
