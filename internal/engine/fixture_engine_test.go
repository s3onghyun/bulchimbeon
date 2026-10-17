package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type scenarioFile struct {
	T0        float64 `json:"t0"`
	Scenarios []struct {
		Name   string `json:"name"`
		Config struct {
			Mode              *string `json:"mode"`
			BatteryFloor      *int    `json:"batteryFloor"`
			CountIdleSessions bool    `json:"countIdleSessions"`
			GraceMinutes      *int    `json:"graceMinutes"`
			LidSleepDisabled  bool    `json:"lidSleepDisabled"`
			LidEngagedByUs    bool    `json:"lidEngagedByUs"`
		} `json:"config"`
		Steps []scenarioStep `json:"steps"`
	} `json:"scenarios"`
}

type scenarioStep struct {
	Op      string   `json:"op"`
	At      float64  `json:"at"`
	Seconds *float64 `json:"seconds"`
	Label   string   `json:"label"`
	Mode    string   `json:"mode"`
	OK      bool     `json:"ok"`
	Resume  struct {
		Action string   `json:"action"`
		Until  *float64 `json:"until"`
	} `json:"resume"`
	Open          map[string]int             `json:"open"`
	Busy          map[string]int             `json:"busy"`
	Battery       *BatteryStatus             `json:"battery"`
	Heat          string                     `json:"heat"`
	AssertionHeld *bool                      `json:"assertionHeld"`
	Effects       []effectJSON               `json:"effects"`
	State         map[string]json.RawMessage `json:"state"`
	Checks        []struct {
		At     float64         `json:"at"`
		Pose   *string         `json:"pose"`
		Speech json.RawMessage `json:"speech"`
	} `json:"checks"`
}

type effectJSON struct {
	Type string          `json:"type"`
	Mode string          `json:"mode"`
	Line string          `json:"line"`
	Args json.RawMessage `json:"args"`
	Kind string          `json:"kind"`
	KO   string          `json:"ko"`
	EN   string          `json:"en"`
	ZH   string          `json:"zh"`
}

// resolved is an effect in a comparable form, with the language already applied.
type resolved struct {
	Type, Mode, Title, Body, Kind, Text string
}

func encodeEffect(e Effect) resolved {
	r := resolved{Type: e.Type.String()}
	switch e.Type {
	case EffectAcquire:
		r.Mode = string(e.Mode)
	case EffectNotify:
		r.Title, r.Body = e.Notice.Title, e.Notice.Body
	case EffectLog:
		r.Kind, r.Text = e.LogKind.String(), e.Text
	}
	return r
}

func resolveEffect(t *testing.T, e effectJSON, lang Lang) resolved {
	t.Helper()
	r := resolved{Type: e.Type}
	switch e.Type {
	case "acquire":
		r.Mode = e.Mode
	case "notify":
		v, err := lineValue(e.Line, pickLang(e.Args, lang), lang)
		if err != nil {
			t.Fatal(err)
		}
		n := v.(noticeJSON)
		r.Title, r.Body = n.Title, n.Body
	case "log":
		r.Kind, r.Text = e.Kind, T(lang, e.KO, e.EN, e.ZH)
	}
	return r
}

// pickLang turns {"ko": x, "en": y, "zh": z} into the value for lang; anything else is returned as is.
func pickLang(raw json.RawMessage, lang Lang) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil && len(m) == 3 && m["ko"] != nil && m["en"] != nil {
		return m[lang.String()]
	}
	return raw
}

func TestFixtureEngineScenarios(t *testing.T) {
	var f scenarioFile
	load(t, "engine_scenarios.json", &f)
	t0 := unix(f.T0)
	at := func(s float64) time.Time { return t0.Add(seconds(s)) }
	rel := func(p *time.Time) any {
		if p == nil {
			return nil
		}
		return p.Sub(t0).Seconds()
	}

	for _, lang := range langs {
		for _, sc := range f.Scenarios {
			cfg := DefaultConfig()
			cfg.Lang = lang
			if sc.Config.Mode != nil {
				cfg.Mode = DutyMode(*sc.Config.Mode)
			}
			if sc.Config.BatteryFloor != nil {
				cfg.BatteryFloor = *sc.Config.BatteryFloor
			}
			if sc.Config.GraceMinutes != nil {
				cfg.GraceMinutes = *sc.Config.GraceMinutes
			}
			cfg.CountIdleSessions = sc.Config.CountIdleSessions
			cfg.LidSleepDisabled, cfg.LidEngagedByUs = sc.Config.LidSleepDisabled, sc.Config.LidEngagedByUs
			e := New(cfg, t0)

			expected := map[string]any{
				"isOnDuty": false, "order": "off", "orderUntil": nil, "resting": false, "mode": string(cfg.Mode),
				"heat": "normal", "quote": "off", "dutyStartedAt": nil, "watchDetail": nil,
				"lidEngagedByUs": cfg.LidEngagedByUs, "lidSleepDisabled": cfg.LidSleepDisabled,
			}

			for i, st := range sc.Steps {
				now := at(st.At)
				label := fmt.Sprintf("[%s] %s — step %d (%s at %v)", lang, sc.Name, i+1, st.Op, st.At)
				var effects []Effect
				switch st.Op {
				case "startTimed":
					var d *time.Duration
					if st.Seconds != nil {
						s := seconds(*st.Seconds)
						d = &s
					}
					effects = e.StartTimed(d, st.Label, now)
				case "startAgentWatch":
					effects = e.StartAgentWatch(now)
				case "stop":
					effects = e.Stop(now)
				case "setMode":
					effects = e.SetMode(DutyMode(st.Mode), now)
				case "resume":
					a := ResumeAction{}
					switch st.Resume.Action {
					case "agentWatch":
						a.Kind = ResumeAgentWatch
					case "expired":
						a.Kind = ResumeExpired
					case "timed":
						a.Kind = ResumeTimed
						if st.Resume.Until != nil {
							u := at(*st.Resume.Until)
							a.Until = &u
						}
					}
					effects = e.Resume(a, now)
				case "lidEnableResult":
					effects = e.LidEnableResult(st.OK, now)
				case "lidRestoreResult":
					effects = e.LidRestoreResult(st.OK)
				case "tick":
					heat := HeatNormal
					if st.Heat != "" {
						var ok bool
						if heat, ok = ParseHeatLevel(st.Heat); !ok {
							t.Fatalf("%s: heat %q", label, st.Heat)
						}
					}
					effects = e.Tick(Observation{
						Now: now, Battery: st.Battery, Heat: heat,
						AgentsOpen: kindCounts(t, st.Open), AgentsBusy: kindCounts(t, st.Busy),
						AssertionLost: st.AssertionHeld != nil && !*st.AssertionHeld,
					})
				default:
					t.Fatalf("%s: unknown op", label)
				}

				got := []resolved{}
				for _, ef := range effects {
					got = append(got, encodeEffect(ef))
				}
				want := []resolved{}
				for _, ef := range st.Effects {
					want = append(want, resolveEffect(t, ef, lang))
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s: effects\n got  %+v\n want %+v", label, got, want)
				}

				for k, raw := range st.State {
					var v any
					json.Unmarshal(pickLang(raw, lang), &v)
					expected[k] = v
				}
				var detail any
				if d := e.WatchDetail(); d != nil {
					detail = *d
				}
				state := map[string]any{
					"isOnDuty": e.OnDuty(), "order": e.Order().Kind.String(), "orderUntil": rel(e.Order().Until),
					"resting": e.Resting(), "mode": string(e.Mode), "heat": e.Heat().String(), "quote": e.Quote().String(),
					"dutyStartedAt": rel(e.DutyStartedAt()), "watchDetail": detail,
					"lidEngagedByUs": e.LidEngagedByUs(), "lidSleepDisabled": e.LidSleepDisabled(),
				}
				if !reflect.DeepEqual(normalize(state), normalize(expected)) {
					t.Errorf("%s: state\n got  %v\n want %v", label, state, expected)
				}

				for _, c := range st.Checks {
					if c.Pose != nil {
						if got := string(e.Pose(at(c.At))); got != *c.Pose {
							t.Errorf("%s: pose at %v = %s, want %s", label, c.At, got, *c.Pose)
						}
					}
					if len(c.Speech) > 0 {
						var line *string
						json.Unmarshal(c.Speech, &line)
						got, ok := e.Speech(at(c.At))
						switch {
						case line == nil && ok:
							t.Errorf("%s: speech at %v = %q, want none", label, c.At, got)
						case line != nil:
							v, err := lineValue(*line, nil, lang)
							if err != nil {
								t.Fatal(err)
							}
							if !ok || got != v.(string) {
								t.Errorf("%s: speech at %v = %q, want %q", label, c.At, got, v)
							}
						}
					}
				}
			}
		}
	}
}

func kindCounts(t *testing.T, m map[string]int) map[Kind]int {
	out := map[Kind]int{}
	for id, n := range m {
		out[kindByID(t, id, BuiltIn)] = n
	}
	return out
}

// normalize makes numbers comparable after a JSON round trip.
func normalize(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}
