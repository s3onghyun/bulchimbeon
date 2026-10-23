package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMissingFileMeansDefaults(t *testing.T) {
	s, problems, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil || len(problems) != 0 || !reflect.DeepEqual(s, Defaults()) {
		t.Fatalf("Load = %+v, %v, %v", s, problems, err)
	}
}

func TestSetUnsetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bulchimbeon", "settings.json")
	for _, c := range []struct {
		key    string
		values []string
	}{
		{"agentGraceMinutes", []string{"10"}},
		{"batteryFloor", []string{"0"}},
		{"agentWatchCountsIdle", []string{"true"}},
		{"customAgents", []string{"goose", "amp"}},
		{"uniformTheme", []string{"desert"}},
		{"language", []string{"ko"}},
		{"dutyMode", []string{"system"}},
	} {
		if err := Set(path, c.key, c.values); err != nil {
			t.Fatalf("Set %s: %v", c.key, err)
		}
	}
	s, problems, err := Load(path)
	want := Settings{DutyMode: "system", BatteryFloor: 0, AgentGraceMinutes: 10, AgentWatchCountsIdle: true,
		CustomAgents: []string{"goose", "amp"}, UniformTheme: "desert", Language: "ko"}
	if err != nil || len(problems) != 0 || !reflect.DeepEqual(s, want) {
		t.Fatalf("Load = %+v, %v, %v", s, problems, err)
	}
	if err := Unset(path, "agentGraceMinutes"); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := Load(path); s.AgentGraceMinutes != 5 {
		t.Errorf("after unset: %d", s.AgentGraceMinutes)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, %v", info.Mode(), err)
	}
	if dir, _ := os.Stat(filepath.Dir(path)); dir.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", dir.Mode())
	}
	var compact bytes.Buffer
	if v, ok, _ := Stored(path, "customAgents"); !ok || json.Compact(&compact, v) != nil || compact.String() != `["goose","amp"]` {
		t.Errorf("stored customAgents = %s", v)
	}
}

func TestSetRejectsValuesOutsideThePromise(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		key    string
		values []string
		hint   string
	}{
		{"agentGraceMinutes", []string{"7"}, "1, 3, 5 or 10"},
		{"batteryFloor", []string{"15"}, "0, 10, 20 or 30"},
		{"batteryFloor", []string{"x"}, "number"},
		{"agentWatchCountsIdle", []string{"yes"}, "true or false"},
		{"agentWatchCountsIdle", []string{"1"}, "true or false"},
		{"customAgents", []string{"../../etc"}, "invalid agent name"},
		{"customAgents", []string{"goose\n"}, "invalid agent name"},
		{"language", []string{"ja"}, "system, ko, en or zh"},
		{"dutyMode", []string{"hybrid"}, "display, system or lid"},
		{"patrolScale", []string{"2"}, "macOS app"},
		{"uniformTheme", []string{"camo"}, "default, navy, arctic or desert"},
		{"uniformTheme", []string{"Navy"}, "default, navy, arctic or desert"},
		{"simulateHeat", []string{"alert"}, "unknown key"},
		{"nope", []string{"1"}, "unknown key"},
		{"batteryFloor", []string{"10", "20"}, "exactly one"},
	} {
		err := Set(path, c.key, c.values)
		if err == nil || !strings.Contains(err.Error(), c.hint) {
			t.Errorf("Set %s %q = %v, want %q", c.key, c.values, err, c.hint)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a rejected value created the file")
	}
}

func TestHandEditedFileFallsBackPerKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"agentGraceMinutes": 7, "batteryFloor": 10, "customAgents": ["goose", "../x"],
		"patrolEnabled": false, "uniformTheme": "navy", "simulateHeat": "alert", "somethingElse": 1}`), 0o600)
	s, problems, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.AgentGraceMinutes != 5 || s.BatteryFloor != 10 || !reflect.DeepEqual(s.CustomAgents, []string{"goose", "../x"}) ||
		s.UniformTheme != "navy" {
		t.Errorf("settings = %+v", s)
	}
	if len(problems) != 3 || !strings.HasPrefix(problems[0], "agentGraceMinutes") || !strings.HasPrefix(problems[1], "simulateHeat") ||
		!strings.HasPrefix(problems[2], "somethingElse") {
		t.Errorf("problems = %q", problems)
	}
	// unknown keys survive a `config set`
	if err := Set(path, "language", []string{"en"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Stored(path, "somethingElse"); !ok {
		t.Error("config set dropped an unknown key")
	}
}

func TestBrokenFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"agentGraceMinutes": `), 0o600)
	if _, _, err := Load(path); err == nil {
		t.Error("broken JSON loaded")
	}
	if err := Set(path, "language", []string{"ko"}); err == nil {
		t.Error("config set overwrote a broken file")
	}
}

func TestChineseLanguageRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Set(path, "language", []string{"zh"}); err != nil {
		t.Fatal(err)
	}
	s, problems, err := Load(path)
	if err != nil || len(problems) != 0 || s.Language != "zh" {
		t.Fatalf("Load = %+v, %v, %v", s, problems, err)
	}
	if err := Unset(path, "language"); err != nil {
		t.Fatal(err)
	}
	s, _, err = Load(path)
	if err != nil || s.Language != "system" {
		t.Fatalf("unset language: %+v, %v", s, err)
	}
}
