package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"
)

// The shared golden fixtures in testdata/engine. The Swift core defines them
// (Tests/BulchimbeonCoreTests/EngineFixtureTests.swift runs the same files); this port must
// pass them unchanged. Every language-dependent case runs in both Korean and English here.

var langs = []Lang{KO, EN, ZH}

func load(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "engine", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func fixtureKinds(custom []string) []AgentKind {
	return append(slices.Clone(BuiltIn), CustomKinds(custom)...)
}

func kindByID(t *testing.T, id string, kinds []AgentKind) Kind {
	t.Helper()
	for _, k := range kinds {
		if k.ID == id {
			return k.Kind
		}
	}
	t.Fatalf("unknown kind %q", id)
	return Kind{}
}

type procJSON struct {
	PID  int32    `json:"pid"`
	PPID int32    `json:"ppid"`
	Exe  string   `json:"exe"`
	Args []string `json:"args"`
}

func (p procJSON) snapshot() ProcessSnapshot {
	return ProcessSnapshot{PID: p.PID, PPID: p.PPID, ExecutablePath: p.Exe, Arguments: p.Args}
}

func TestFixtureCatalog(t *testing.T) {
	var f struct {
		BuiltIn []struct {
			ID             string   `json:"id"`
			DisplayName    string   `json:"displayName"`
			BinaryNames    []string `json:"binaryNames"`
			PathMarkers    []string `json:"pathMarkers"`
			PackageMarkers []string `json:"packageMarkers"`
		} `json:"builtIn"`
		Match []struct {
			Name   string   `json:"name"`
			Exe    string   `json:"exe"`
			Args   []string `json:"args"`
			Custom []string `json:"custom"`
			Kind   *string  `json:"kind"`
			Reason *string  `json:"reason"`
		} `json:"match"`
		IsCandidate []struct {
			Exe    string   `json:"exe"`
			Argv0  *string  `json:"argv0"`
			Custom []string `json:"custom"`
			Expect bool     `json:"expect"`
		} `json:"isCandidate"`
		NeedsFullArguments []struct {
			Exe    string `json:"exe"`
			Expect bool   `json:"expect"`
		} `json:"needsFullArguments"`
		CustomKinds []struct {
			Name         string   `json:"name"`
			Input        []string `json:"input"`
			DisplayNames []string `json:"displayNames"`
		} `json:"customKinds"`
		Count []struct {
			Name      string         `json:"name"`
			Processes []procJSON     `json:"processes"`
			Custom    []string       `json:"custom"`
			Expect    map[string]int `json:"expect"`
		} `json:"count"`
		Summary []struct {
			Counts map[string]int `json:"counts"`
			Custom []string       `json:"custom"`
			Expect string         `json:"expect"`
		} `json:"summary"`
	}
	load(t, "catalog.json", &f)

	if len(BuiltIn) != len(f.BuiltIn) {
		t.Fatalf("builtIn has %d kinds, fixture %d", len(BuiltIn), len(f.BuiltIn))
	}
	for i, want := range f.BuiltIn {
		k := BuiltIn[i]
		got := []any{k.ID, k.DisplayName, sorted(k.BinaryNames), k.PathMarkers, k.PackageMarkers}
		exp := []any{want.ID, want.DisplayName, sorted(want.BinaryNames), want.PathMarkers, want.PackageMarkers}
		if fmt.Sprint(got) != fmt.Sprint(exp) {
			t.Errorf("builtIn[%d] = %v, want %v", i, got, exp)
		}
	}
	for _, c := range f.Match {
		k, r, ok := Match(ProcessSnapshot{ExecutablePath: c.Exe, Arguments: c.Args}, fixtureKinds(c.Custom))
		switch {
		case c.Kind == nil && ok:
			t.Errorf("match %q: got %s, want none", c.Name, k.ID)
		case c.Kind != nil && (!ok || k.ID != *c.Kind):
			t.Errorf("match %q: got %q (matched=%v), want %s", c.Name, k.ID, ok, *c.Kind)
		case c.Reason != nil && r.String() != *c.Reason:
			t.Errorf("match %q: reason %s, want %s", c.Name, r, *c.Reason)
		}
	}
	for _, c := range f.IsCandidate {
		if got := IsCandidate(c.Exe, c.Argv0, fixtureKinds(c.Custom)); got != c.Expect {
			t.Errorf("isCandidate(%s, %v) = %v", c.Exe, c.Argv0, got)
		}
	}
	for _, c := range f.NeedsFullArguments {
		if got := NeedsFullArguments(c.Exe); got != c.Expect {
			t.Errorf("needsFullArguments(%s) = %v", c.Exe, got)
		}
	}
	for _, c := range f.CustomKinds {
		var names []string
		for _, k := range CustomKinds(c.Input) {
			names = append(names, k.DisplayName)
		}
		if !slices.Equal(names, c.DisplayNames) {
			t.Errorf("customKinds %q = %q, want %q", c.Name, names, c.DisplayNames)
		}
	}
	for _, c := range f.Count {
		var ps []ProcessSnapshot
		for _, p := range c.Processes {
			ps = append(ps, p.snapshot())
		}
		got := map[string]int{}
		for k, n := range Count(ps, fixtureKinds(c.Custom)) {
			got[k.ID] = n
		}
		if !reflect.DeepEqual(got, c.Expect) {
			t.Errorf("count %q = %v, want %v", c.Name, got, c.Expect)
		}
	}
	for _, c := range f.Summary {
		kinds := fixtureKinds(c.Custom)
		counts := map[Kind]int{}
		for id, n := range c.Counts {
			counts[kindByID(t, id, kinds)] = n
		}
		if got := Summary(counts); got != c.Expect {
			t.Errorf("summary(%v) = %q, want %q", c.Counts, got, c.Expect)
		}
	}
}

func TestFixtureActivity(t *testing.T) {
	var f struct {
		CPUThreshold     float64 `json:"cpuThreshold"`
		BusyGraceSeconds float64 `json:"busyGraceSeconds"`
		Evaluate         []struct {
			Name      string             `json:"name"`
			Processes []procJSON         `json:"processes"`
			CPU       map[string]float64 `json:"cpu"`
			Custom    []string           `json:"custom"`
			Expect    map[string]struct {
				Present int `json:"present"`
				Busy    int `json:"busy"`
			} `json:"expect"`
		} `json:"evaluate"`
	}
	load(t, "activity.json", &f)
	if CPUThreshold != f.CPUThreshold || BusyGrace.Seconds() != f.BusyGraceSeconds {
		t.Errorf("constants: threshold %v grace %v", CPUThreshold, BusyGrace)
	}
	for _, c := range f.Evaluate {
		var ps []ProcessSnapshot
		for _, p := range c.Processes {
			ps = append(ps, p.snapshot())
		}
		got := map[string][2]int{}
		for k, s := range Evaluate(ps, pidMap(t, c.CPU), fixtureKinds(c.Custom)) {
			got[k.ID] = [2]int{s.Present, s.Busy}
		}
		want := map[string][2]int{}
		for id, s := range c.Expect {
			want[id] = [2]int{s.Present, s.Busy}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("evaluate %q = %v, want %v", c.Name, got, want)
		}
	}
}

func pidMap[V any](t *testing.T, m map[string]V) map[int32]V {
	t.Helper()
	out := map[int32]V{}
	for k, v := range m {
		pid, err := strconv.ParseInt(k, 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		out[int32(pid)] = v
	}
	return out
}

// cpuEpoch matches Fixture.cpuEpoch in the Swift runner.
var cpuEpoch = time.Unix(1_000_000, 0)

func TestFixtureCPUHistory(t *testing.T) {
	var f struct {
		Tolerance float64 `json:"tolerance"`
		Cases     []struct {
			Name    string  `json:"name"`
			Window  float64 `json:"window"`
			Samples []struct {
				At     float64             `json:"at"`
				Times  map[string]uint64   `json:"times"`
				Expect *map[string]float64 `json:"expect"`
			} `json:"samples"`
		} `json:"cases"`
	}
	load(t, "cpu_history.json", &f)
	for _, c := range f.Cases {
		h := &CPUHistory{Window: seconds(c.Window)}
		for i, s := range c.Samples {
			got := h.Add(pidMap(t, s.Times), cpuEpoch.Add(seconds(s.At)))
			if s.Expect == nil {
				continue
			}
			want := pidMap(t, *s.Expect)
			if len(got) != len(want) {
				t.Errorf("%s sample %d: got %v, want %v", c.Name, i, got, want)
				continue
			}
			for pid, w := range want {
				if g, ok := got[pid]; !ok || math.Abs(g-w) > f.Tolerance {
					t.Errorf("%s sample %d pid %d: got %v, want %v", c.Name, i, pid, g, w)
				}
			}
		}
	}
}

func unix(s float64) time.Time { return timeFromUnix(s) }

func TestFixturePolicies(t *testing.T) {
	var f struct {
		BatteryRetreat []struct {
			Name    string         `json:"name"`
			Battery *BatteryStatus `json:"battery"`
			Floor   *int           `json:"floor"`
			Expect  bool           `json:"expect"`
		} `json:"batteryRetreat"`
		AgentWatchDecide []struct {
			BusyNow    bool    `json:"busyNow"`
			QuietFor   float64 `json:"quietFor"`
			Grace      float64 `json:"grace"`
			OnDuty     bool    `json:"onDuty"`
			LowBattery bool    `json:"lowBattery"`
			Expect     string  `json:"expect"`
		} `json:"agentWatchDecide"`
		QuietFor []struct {
			Now            float64  `json:"now"`
			LastBusyAt     *float64 `json:"lastBusyAt"`
			WatchStartedAt float64  `json:"watchStartedAt"`
			Expect         float64  `json:"expect"`
		} `json:"quietFor"`
		HeatCoolDownSeconds float64 `json:"heatCoolDownSeconds"`
		HeatDecide          []struct {
			Level     string   `json:"level"`
			OnDuty    bool     `json:"onDuty"`
			Resting   bool     `json:"resting"`
			Advised   bool     `json:"advised"`
			CoolSince *float64 `json:"coolSince"`
			Now       float64  `json:"now"`
			Expect    string   `json:"expect"`
		} `json:"heatDecide"`
		HeatLevelName []struct {
			Name   string  `json:"name"`
			Expect *string `json:"expect"`
		} `json:"heatLevelName"`
		DutyResume []struct {
			Stored *string         `json:"stored"`
			Until  json.RawMessage `json:"until"`
			Now    float64         `json:"now"`
			Expect struct {
				Action string   `json:"action"`
				Until  *float64 `json:"until"`
			} `json:"expect"`
		} `json:"dutyResume"`
		DutyClock []struct {
			Seconds float64 `json:"seconds"`
			Expect  string  `json:"expect"`
		} `json:"dutyClock"`
	}
	load(t, "policies.json", &f)

	for _, c := range f.BatteryRetreat {
		if got := ShouldRetreat(c.Battery, c.Floor); got != c.Expect {
			t.Errorf("batteryRetreat %q = %v", c.Name, got)
		}
	}
	for _, c := range f.AgentWatchDecide {
		if got := DecideWatch(c.BusyNow, seconds(c.QuietFor), seconds(c.Grace), c.OnDuty, c.LowBattery).String(); got != c.Expect {
			t.Errorf("agentWatchDecide %+v = %s", c, got)
		}
	}
	for _, c := range f.QuietFor {
		var last *time.Time
		if c.LastBusyAt != nil {
			l := unix(*c.LastBusyAt)
			last = &l
		}
		if got := QuietFor(unix(c.Now), last, unix(c.WatchStartedAt)); got != seconds(c.Expect) {
			t.Errorf("quietFor %+v = %v", c, got)
		}
	}
	if CoolDown != seconds(f.HeatCoolDownSeconds) {
		t.Errorf("coolDown %v", CoolDown)
	}
	for _, c := range f.HeatDecide {
		level, ok := ParseHeatLevel(c.Level)
		if !ok {
			t.Fatalf("bad level %q", c.Level)
		}
		var cool *time.Time
		if c.CoolSince != nil {
			cs := unix(*c.CoolSince)
			cool = &cs
		}
		if got := DecideHeat(level, c.OnDuty, c.Resting, c.Advised, cool, unix(c.Now)).String(); got != c.Expect {
			t.Errorf("heatDecide %+v = %s", c, got)
		}
	}
	for _, c := range f.HeatLevelName {
		level, ok := ParseHeatLevel(c.Name)
		if ok != (c.Expect != nil) || (ok && level.String() != *c.Expect) {
			t.Errorf("heatLevel(%q) = %v, %v", c.Name, level, ok)
		}
	}
	for _, c := range f.DutyResume {
		var until *float64
		switch string(c.Until) {
		case "null", "":
		case `"inf"`:
			inf := math.Inf(1)
			until = &inf
		default:
			var u float64
			if err := json.Unmarshal(c.Until, &u); err != nil {
				t.Fatal(err)
			}
			until = &u
		}
		got := DecideResume(c.Stored, until, unix(c.Now))
		name := map[ResumeKind]string{ResumeNone: "none", ResumeAgentWatch: "agentWatch", ResumeTimed: "timed", ResumeExpired: "expired"}[got.Kind]
		var wantUntil *time.Time
		if c.Expect.Until != nil {
			u := unix(*c.Expect.Until)
			wantUntil = &u
		}
		if name != c.Expect.Action || !sameTime(got.Until, wantUntil) {
			t.Errorf("dutyResume(%v, %s) = %s %v, want %+v", c.Stored, c.Until, name, got.Until, c.Expect)
		}
	}
	for _, c := range f.DutyClock {
		if got := FormatClock(seconds(c.Seconds)); got != c.Expect {
			t.Errorf("dutyClock(%v) = %q, want %q", c.Seconds, got, c.Expect)
		}
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func TestFixtureSettings(t *testing.T) {
	var f struct {
		PublicKeys          []string `json:"publicKeys"`
		InternalKeys        []string `json:"internalKeys"`
		DutyModes           []string `json:"dutyModes"`
		BatteryFloors       []int    `json:"batteryFloors"`
		DefaultBatteryFloor int      `json:"defaultBatteryFloor"`
		GraceMinutes        []int    `json:"graceMinutes"`
		DefaultGraceMinutes int      `json:"defaultGraceMinutes"`
		PatrolScales        []int    `json:"patrolScales"`
		DefaultPatrolScale  int      `json:"defaultPatrolScale"`
		DefaultUniformTheme string   `json:"defaultUniformTheme"`
		MaxCustomAgents     int      `json:"maxCustomAgents"`
		LaunchArguments     []string `json:"launchArguments"`
		EngineDefaults      struct {
			Mode              string `json:"mode"`
			BatteryFloor      int    `json:"batteryFloor"`
			CountIdleSessions bool   `json:"countIdleSessions"`
			GraceMinutes      int    `json:"graceMinutes"`
		} `json:"engineDefaults"`
	}
	load(t, "settings.json", &f)
	var modes []string
	for _, m := range DutyModes {
		modes = append(modes, string(m))
	}
	d := DefaultConfig()
	got := []any{PublicSettingsKeys, InternalSettingsKeys, modes, BatteryFloors, DefaultBatteryFloor, GraceMinutes,
		DefaultGraceMinutes, PatrolScales, DefaultPatrolScale, DefaultUniformTheme, MaxCustomAgents, LaunchArguments,
		string(d.Mode), d.BatteryFloor, d.CountIdleSessions, d.GraceMinutes}
	want := []any{f.PublicKeys, f.InternalKeys, f.DutyModes, f.BatteryFloors, f.DefaultBatteryFloor, f.GraceMinutes,
		f.DefaultGraceMinutes, f.PatrolScales, f.DefaultPatrolScale, f.DefaultUniformTheme, f.MaxCustomAgents, f.LaunchArguments,
		f.EngineDefaults.Mode, f.EngineDefaults.BatteryFloor, f.EngineDefaults.CountIdleSessions, f.EngineDefaults.GraceMinutes}
	for i := range got {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("settings[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestFixtureDutyLog(t *testing.T) {
	var f struct {
		Lines []struct {
			Name string  `json:"name"`
			At   float64 `json:"at"`
			Text string  `json:"text"`
			Line string  `json:"line"`
		} `json:"lines"`
	}
	load(t, "duty_log.json", &f)
	for _, c := range f.Lines {
		path := filepath.Join(t.TempDir(), "duty-log.txt")
		if err := NewDutyLog(path).Write(c.Text, LogOther, unix(c.At)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != c.Line {
			t.Errorf("duty log %q: %q, want %q", c.Name, data, c.Line)
		}
	}
}

func TestFixtureLines(t *testing.T) {
	var f struct {
		Detect []struct {
			Preferred []string `json:"preferred"`
			Expect    string   `json:"expect"`
		} `json:"detect"`
		Lines []lineCase `json:"lines"`
	}
	load(t, "lines.json", &f)
	for _, c := range f.Detect {
		if got := DetectLanguage(c.Preferred).String(); got != c.Expect {
			t.Errorf("detect(%v) = %s", c.Preferred, got)
		}
	}
	for _, c := range f.Lines {
		for _, lang := range langs {
			got, err := lineValue(c.Fn, c.Args, lang)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, got, c.want(lang)) {
				t.Errorf("%s %s [%s] = %v, want %s", c.Fn, c.Args, lang, got, c.want(lang))
			}
		}
	}
}

// sameJSON compares a Go value with a fixture value through JSON.
func sameJSON(t *testing.T, got any, want json.RawMessage) bool {
	t.Helper()
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	json.Unmarshal(b, &g)
	json.Unmarshal(want, &w)
	return reflect.DeepEqual(g, w)
}

func sorted(s []string) []string {
	c := slices.Clone(s)
	sort.Strings(c)
	return c
}
