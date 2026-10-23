package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
)

// fake platform -------------------------------------------------------------

type fakeProcs struct {
	procs []engine.ProcessSnapshot
	cpu   map[int32]uint64 // cumulative ns
	rate  map[int32]uint64 // ns added per scan
}

func (f *fakeProcs) Scan([]engine.AgentKind) (platform.ProcessScan, error) {
	times := map[int32]uint64{}
	for pid, r := range f.rate {
		f.cpu[pid] += r
	}
	for pid, v := range f.cpu {
		times[pid] = v
	}
	return platform.ProcessScan{Snapshots: f.procs, CPUTimes: times}, nil
}

type fakeInhibitor struct {
	wanted, denySleep, lose bool
	acquires, releases      int
	pid                     int
	desktop                 bool // held by the desktop session instead of logind
}

func (f *fakeInhibitor) Acquire() {
	if !f.wanted {
		f.pid += 10
	}
	f.wanted = true
	f.acquires++
}
func (f *fakeInhibitor) Release() { f.wanted = false; f.releases++ }
func (f *fakeInhibitor) Refresh() bool {
	if f.lose {
		f.lose, f.wanted = false, false
		return true
	}
	return false
}
func (f *fakeInhibitor) Status() platform.LockStatus {
	st := platform.LockStatus{Wanted: f.wanted, Method: platform.MethodLogind, Mode: "block"}
	if f.wanted {
		st.Fallback = platform.FallbackNoDesktop
		sleep := platform.Block{What: "sleep", Held: true, Confirmed: true, PID: f.pid}
		if f.denySleep {
			sleep = platform.Block{What: "sleep", Denied: true, Error: "polkit denied"}
		}
		if f.desktop {
			st = platform.LockStatus{Wanted: true, Method: platform.MethodGnome}
			sleep = platform.Block{What: "sleep", Held: true, Confirmed: true}
		}
		st.Blocks = []platform.Block{sleep}
	}
	return st
}

type fakeBattery struct{ b *engine.BatteryStatus }

func (f *fakeBattery) Battery() (*engine.BatteryStatus, error) { return f.b, nil }

type fakeThermal struct{ r platform.ThermalReading }

func (f *fakeThermal) Thermal() platform.ThermalReading { return f.r }

type fakeHolders struct{ h []platform.Holder }

func (f *fakeHolders) Holders() ([]platform.Holder, error) { return f.h, nil }

const claudeExe = "/home/me/.local/share/claude/versions/2.1.301"

type rig struct {
	t        *testing.T
	now      time.Time
	procs    *fakeProcs
	inh      *fakeInhibitor
	battery  *fakeBattery
	thermal  *fakeThermal
	out      *bytes.Buffer
	paths    Paths
	d        *Daemon
	lastTick time.Time
}

func newRig(t *testing.T) *rig {
	dir := t.TempDir()
	r := &rig{t: t, now: time.Unix(1_792_140_000, 0),
		procs: &fakeProcs{cpu: map[int32]uint64{}, rate: map[int32]uint64{}},
		inh:   &fakeInhibitor{}, battery: &fakeBattery{},
		thermal: &fakeThermal{platform.ThermalReading{Level: engine.HeatNormal, LevelName: "normal", Sensor: "coretemp",
			Celsius: 52, CritCelsius: 105, Available: true}},
		out:   &bytes.Buffer{},
		paths: Paths{Settings: filepath.Join(dir, "config", "settings.json"), State: filepath.Join(dir, "state", "state.json"), Log: filepath.Join(dir, "state", "duty-log.txt")},
	}
	en := engine.EN
	r.d = New(Config{
		Platform: platform.Platform{Name: "fake", Processes: r.procs, Inhibitor: r.inh, Battery: r.battery,
			Thermal: r.thermal, Holders: &fakeHolders{}},
		Paths: r.paths, Lang: &en, Out: r.out, Now: func() time.Time { return r.now }, PID: 4242,
	})
	r.d.Start()
	return r
}

// session adds an agent process; busy sessions use 10% CPU, idle ones 0.6%.
func (r *rig) session(pid int32, busy bool) {
	r.procs.procs = append(r.procs.procs, engine.ProcessSnapshot{PID: pid, PPID: 1, ExecutablePath: claudeExe, Arguments: []string{"claude"}})
	r.setBusy(pid, busy)
}

func (r *rig) setBusy(pid int32, busy bool) {
	if busy {
		r.procs.rate[pid] = 500_000_000 // 0.5 s per 5 s tick
	} else {
		r.procs.rate[pid] = 30_000_000 // 0.6%
	}
}

// run ticks every 5 s for d.
func (r *rig) run(d time.Duration) {
	for end := r.now.Add(d); r.now.Before(end); {
		r.now = r.now.Add(Interval)
		r.d.Tick()
	}
}

func (r *rig) log() string {
	data, _ := os.ReadFile(r.paths.Log)
	return string(data)
}

func (r *rig) state() *State {
	st, err := ReadState(r.paths.State)
	if err != nil {
		r.t.Fatal(err)
	}
	return st
}

// tests ---------------------------------------------------------------------

func TestEngagesOnWorkAndHandsOverAfterGrace(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.session(200, false)
	r.run(10 * time.Second)
	if !r.d.Engine().OnDuty() || r.inh.acquires != 1 || !strings.Contains(r.log(), "On duty — Claude Code clocked in") {
		t.Fatalf("not on duty: acquires=%d log=%q", r.inh.acquires, r.log())
	}
	st := r.state()
	if !st.OnDuty || st.PID != 4242 || len(st.Agents) != 1 || st.Agents[0].Open != 2 || st.Agents[0].Working != 1 || !st.Lock.Complete() {
		t.Fatalf("state = %+v", st)
	}

	r.setBusy(100, false)
	r.run(4 * time.Minute)
	if !r.d.Engine().OnDuty() {
		t.Fatal("handed over before the grace time (the 60 s average still counts the last work)")
	}
	r.run(3 * time.Minute)
	if r.d.Engine().OnDuty() || r.inh.wanted || !strings.Contains(r.log(), "Shift over — Claude Code finished, all clear") {
		t.Fatalf("no handover: wanted=%v log=%q", r.inh.wanted, r.log())
	}
	if lines := strings.Count(r.log(), "\n"); lines != 3 {
		t.Errorf("duty log has %d lines:\n%s", lines, r.log())
	}
}

func TestIdleSessionsNeverEngage(t *testing.T) {
	r := newRig(t)
	for pid := int32(11); pid <= 15; pid++ {
		r.session(pid, false)
	}
	r.run(10 * time.Minute)
	if r.d.Engine().OnDuty() || r.inh.acquires != 0 {
		t.Fatal("idle sessions put the watch on duty")
	}
	if st := r.state(); st.Agents[0].Open != 5 || st.Agents[0].Working != 0 || st.Lock.Wanted {
		t.Errorf("state = %+v", st)
	}
}

func TestLostLockIsReacquiredAndLogged(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.run(10 * time.Second)
	r.inh.lose = true
	r.run(5 * time.Second)
	if !r.inh.wanted || r.inh.acquires != 2 || !strings.Contains(r.log(), "Post abandoned — re-engaged") {
		t.Fatalf("acquires=%d log=%q", r.inh.acquires, r.log())
	}
}

func TestSleepDeniedIsRetriedAndExplained(t *testing.T) {
	r := newRig(t)
	r.inh.denySleep = true
	r.session(100, true)
	r.run(10 * time.Second)
	if r.inh.acquires != 1 {
		t.Fatalf("acquires = %d", r.inh.acquires)
	}
	if !strings.Contains(r.out.String(), "lock: ⚠️ NOT blocking sleep: polkit refused it") {
		t.Errorf("journal:\n%s", r.out)
	}
	r.run(30 * time.Second)
	if r.inh.acquires != 1 {
		t.Errorf("retried too soon: %d", r.inh.acquires)
	}
	r.run(40 * time.Second)
	if r.inh.acquires != 2 {
		t.Errorf("not retried after a minute: %d", r.inh.acquires)
	}
	text, running := RenderStatus(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now})
	if !running || !strings.Contains(text, "NOT blocking sleep: polkit refused it") || strings.Contains(text, "confirmed in logind") ||
		!strings.Contains(text, "no desktop on the session bus") {
		t.Errorf("status:\n%s", text)
	}
}

func TestBatteryStandDownReArmsWhenCharging(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.run(10 * time.Second)
	r.battery.b = &engine.BatteryStatus{Percent: 15, OnBattery: true}
	r.run(5 * time.Second)
	if r.d.Engine().OnDuty() || r.d.Engine().Order().Kind != engine.OrderOff || !strings.Contains(r.log(), "Battery 15% — stood down") {
		t.Fatalf("no stand-down: %q", r.log())
	}
	r.run(time.Minute)
	if r.d.Engine().Order().Kind != engine.OrderOff {
		t.Fatal("re-armed while still low on battery")
	}
	r.battery.b.OnBattery = false
	r.run(10 * time.Second)
	if !r.d.Engine().OnDuty() {
		t.Fatal("did not re-arm and engage after plugging in")
	}
}

func TestSettingsApplyLive(t *testing.T) {
	r := newRig(t)
	if err := settings.Set(r.paths.Settings, "agentWatchCountsIdle", []string{"true"}); err != nil {
		t.Fatal(err)
	}
	r.session(100, false)
	r.run(5 * time.Second)
	if !r.d.Engine().OnDuty() {
		t.Fatal("agentWatchCountsIdle=true should count an open session")
	}
}

func TestHeatAlertRestsAndResumes(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.run(10 * time.Second)
	r.thermal.r.Level = engine.HeatAlert
	r.run(5 * time.Second)
	if !r.d.Engine().Resting() || r.inh.wanted {
		t.Fatal("a heat alert should start combat rest")
	}
	r.thermal.r.Level = engine.HeatNormal
	r.run(3 * time.Minute)
	if r.d.Engine().Resting() || !r.d.Engine().OnDuty() {
		t.Fatal("did not resume after cooling down")
	}
}

func TestStopReleasesAndMarksState(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.run(10 * time.Second)
	r.d.Stop()
	if r.inh.wanted || !r.state().Stopped {
		t.Fatal("Stop left the lock or the state")
	}
	text, running := RenderStatus(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now})
	if running || !strings.Contains(text, "not running: stopped at") {
		t.Errorf("status:\n%s", text)
	}
}

func TestStatusWhenNotRunning(t *testing.T) {
	now := time.Unix(1_792_140_000, 0)
	cases := map[string]StatusInput{
		"no state file":    {StatePath: "/home/me/.local/state/bulchimbeon/state.json", Home: "/home/me"},
		"is gone":          {State: &State{PID: 7, UpdatedAt: now}, Alive: func(int) bool { return false }},
		"not responding":   {State: &State{PID: 7, UpdatedAt: now.Add(-time.Minute)}, Alive: func(int) bool { return true }},
		"locks still list": {StatePath: "x", Holders: []platform.Holder{{Who: "Bulchimbeon", What: "sleep", PID: 9}}},
	}
	for want, in := range cases {
		in.Lang, in.Now = engine.EN, now
		text, running := RenderStatus(in)
		if running || !strings.Contains(text, want) {
			t.Errorf("%s:\n%s", want, text)
		}
	}
	text, _ := RenderStatus(StatusInput{StatePath: "/home/me/.local/state/bulchimbeon/state.json", Home: "/home/me", Lang: engine.KO, Now: now})
	if !strings.Contains(text, "불침번이 실행 중이 아닙니다: 상태 파일 없음 (~/.local/state/bulchimbeon/state.json)") {
		t.Errorf("ko:\n%s", text)
	}
}

func TestStatusExplainsStandbyAndDuty(t *testing.T) {
	r := newRig(t)
	r.session(100, false)
	r.run(10 * time.Second)
	text, _ := RenderStatus(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now})
	for _, want := range []string{"standing by", "not held: no agent is working", "Claude Code ×1 open, 0 working",
		"5 min after the last agent goes quiet", "battery: none", "coretemp 52 °C", "lid: not handled"} {
		if !strings.Contains(text, want) {
			t.Errorf("standby status lacks %q:\n%s", want, text)
		}
	}
	r.setBusy(100, true)
	r.run(10 * time.Second)
	text, _ = RenderStatus(StatusInput{State: r.state(), Lang: engine.KO, Now: r.now, LogTail: []string{"2026-10-16T08:40:05Z  근무 투입 — Claude Code 출근"},
		Holders: []platform.Holder{{Who: "Bulchimbeon", What: "sleep", Mode: "block", PID: r.inh.pid},
			{Who: "UPower", What: "sleep", Mode: "delay", PID: 3}}})
	for _, want := range []string{"근무 중", "잠자기 차단 중 — logind block (systemd-inhibit pid 10), 세션 버스에 데스크톱 없음", "logind 에서 확인됨", "Claude Code pid 100",
		"잠자기 지연: UPower", "근무 투입 — Claude Code 출근"} {
		if !strings.Contains(text, want) {
			t.Errorf("duty status lacks %q:\n%s", want, text)
		}
	}
}

// The tray shows the report line by line; RenderStatus prints the same lines.
func TestStatusReport(t *testing.T) {
	r := newRig(t)
	r.session(100, true)
	r.run(10 * time.Second)
	holders := []platform.Holder{{Who: "Bulchimbeon", What: "sleep", Mode: "block", PID: r.inh.pid}}
	rep := Status(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now, Holders: holders})
	if !rep.Running || rep.Headline != "🪖 Bulchimbeon — on duty for 0:00 (agent watch)" {
		t.Fatalf("report = %+v", rep)
	}
	var keys []string
	for _, l := range rep.Lines {
		keys = append(keys, l.Key)
		if l.Key == "lock" && (l.Label != "Lock:" || l.Text != "blocking sleep via logind block (systemd-inhibit pid 10), no desktop on the session bus · confirmed in logind") {
			t.Errorf("lock line = %+v", l)
		}
	}
	if strings.Join(keys, " ") != "lock why agents handover power others private log" {
		t.Errorf("keys = %q", keys)
	}
	text, _ := RenderStatus(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now, Holders: holders})
	if !strings.HasPrefix(text, rep.Headline+"\nLock:      blocking sleep via logind block (systemd-inhibit pid 10), no desktop on the session bus · confirmed in logind\n") {
		t.Errorf("terminal status:\n%s", text)
	}
}

// On a desktop the lock is the session's inhibitor: logind does not list it, the session confirms it.
func TestStatusOnADesktop(t *testing.T) {
	r := newRig(t)
	r.inh.desktop = true
	r.session(100, true)
	r.run(10 * time.Second)
	text, _ := RenderStatus(StatusInput{State: r.state(), Lang: engine.EN, Now: r.now})
	want := "Lock:      blocking auto-suspend via org.gnome.SessionManager · confirmed by the session\n"
	if !strings.Contains(text, want) {
		t.Errorf("status:\n%s", text)
	}
	if !strings.Contains(r.out.String(), "lock: blocking auto-suspend via org.gnome.SessionManager") {
		t.Errorf("journal:\n%s", r.out)
	}
	// logout: the lock moves to logind and the journal says so, without a lost-post entry
	r.inh.desktop = false
	r.run(5 * time.Second)
	if !strings.Contains(r.out.String(), "lock: blocking sleep via logind block (systemd-inhibit pid 10), no desktop on the session bus") ||
		strings.Contains(r.log(), "Post abandoned") {
		t.Errorf("journal:\n%s\nlog:\n%s", r.out, r.log())
	}
}

func TestTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o600)
	if got := Tail(path, 3); strings.Join(got, ",") != "b,c,d" {
		t.Errorf("tail = %q", got)
	}
	os.WriteFile(path, nil, 0o600)
	if got := Tail(path, 3); got != nil {
		t.Errorf("empty = %q", got)
	}
	if got := Tail(filepath.Join(path, "missing"), 3); got != nil {
		t.Errorf("missing = %q", got)
	}
}

func TestDefaultPaths(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	p := DefaultPaths(get, "/home/me")
	if p.Settings != "/home/me/.config/bulchimbeon/settings.json" || p.State != "/home/me/.local/state/bulchimbeon/state.json" ||
		p.Log != "/home/me/.local/state/bulchimbeon/duty-log.txt" {
		t.Errorf("defaults = %+v", p)
	}
	env["XDG_CONFIG_HOME"], env["XDG_STATE_HOME"] = "/cfg", "relative/ignored"
	p = DefaultPaths(get, "/home/me")
	if p.Settings != "/cfg/bulchimbeon/settings.json" || p.Log != "/home/me/.local/state/bulchimbeon/duty-log.txt" {
		t.Errorf("xdg = %+v", p)
	}
}

func TestResolveLang(t *testing.T) {
	ko, s := engine.KO, settings.Defaults()
	env := func(k string) string { return map[string]string{"LANG": "ko_KR.UTF-8"}[k] }
	if ResolveLang(nil, s, env) != engine.KO || ResolveLang(nil, s, func(string) string { return "" }) != engine.EN {
		t.Error("environment")
	}
	s.Language = "en"
	if ResolveLang(nil, s, env) != engine.EN || ResolveLang(&ko, s, env) != engine.KO {
		t.Error("setting and override")
	}
}

func TestStatusLabelsAlignInAllLanguages(t *testing.T) {
	for _, label := range []string{"Lock:", "Handover:", "잠금", "에이전트", "다른 앱", "休眠锁：", "其他应用：", ""} {
		w := 0
		for _, r := range padLabel(label) {
			w += cellWidth(r)
		}
		if w != labelWidth {
			t.Errorf("%q padded to %d cells, want %d", label, w, labelWidth)
		}
	}
}
