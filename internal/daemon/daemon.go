// Package daemon is the agent-watch loop outside macOS: every 5 seconds it gathers the
// platform's observations, lets the shared engine decide, carries out the engine's effects
// and writes state.json for `bulchimbeon status`.
//
// Like the macOS app's DutyController it holds no duty logic of its own.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"sort"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
	"github.com/s3onghyun/bulchimbeon/internal/update"
)

const (
	// Interval is the check period, as on macOS.
	Interval = 5 * time.Second
	// RetryInterval is how often a block that could not be taken (polkit) is tried again.
	RetryInterval = time.Minute
	// StateVersion is the state.json schema version. The file is for `status` only and its
	// format is not part of the compatibility promise.
	StateVersion = 1
)

// Config wires the daemon to a platform and its files.
type Config struct {
	Platform platform.Platform
	Paths    Paths
	// Lang overrides the language (`--lang`); nil uses the settings, then the environment.
	Lang   *engine.Lang
	Getenv func(string) string
	Out    io.Writer // journal output
	Now    func() time.Time
	PID    int
	// Update checks for a newer release once a day (nil: never). It only notifies: the offer
	// goes into state.json, so `status` and the tray show it with the command for Install.
	Update *update.Checker
	// Install is how this binary was installed (update.Install*), Arch its GOARCH.
	Install string
	Arch    string
}

// Daemon is one running agent watch.
type Daemon struct {
	cfg       Config
	engine    *engine.Engine
	cpu       *engine.CPUHistory
	log       *engine.DutyLog
	started   time.Time
	settings  settings.Settings
	problems  []string
	lastRetry time.Time
	agents    []AgentState
	battery   *engine.BatteryStatus
	thermal   platform.ThermalReading
	scanErr   string
	lastLock  string
	offer     *UpdateOffer
}

// ResolveLang: `--lang`, then the `language` setting, then LANGUAGE/LC_*/LANG.
func ResolveLang(override *engine.Lang, s settings.Settings, getenv func(string) string) engine.Lang {
	if override != nil {
		return *override
	}
	if l, ok := engine.ParseLang(s.Language); ok {
		return l
	}
	return engine.DetectLanguage(engine.PreferredLanguages(getenv))
}

// New prepares a daemon; Start begins the watch.
func New(cfg Config) *Daemon {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.Install == "" {
		cfg.Install = update.InstallManual
	}
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	d := &Daemon{cfg: cfg, cpu: engine.NewCPUHistory(), log: engine.NewDutyLog(cfg.Paths.Log)}
	d.reloadSettings()
	now := cfg.Now()
	d.started = now
	ec := engine.DefaultConfig()
	ec.Lang = ResolveLang(cfg.Lang, d.settings, cfg.Getenv)
	d.engine = engine.New(ec, now)
	d.applySettings()
	return d
}

// Engine exposes the engine (tests and status).
func (d *Daemon) Engine() *engine.Engine { return d.engine }

func (d *Daemon) reloadSettings() {
	s, problems, err := settings.Load(d.cfg.Paths.Settings)
	if err != nil {
		problems = []string{err.Error()} // keep the last good settings
		if d.settings.DutyMode == "" {
			s = settings.Defaults()
		} else {
			s = d.settings
		}
	}
	d.settings, d.problems = s, problems
}

func (d *Daemon) applySettings() {
	e := d.engine
	e.Lang = ResolveLang(d.cfg.Lang, d.settings, d.cfg.Getenv)
	e.Mode = engine.ModeSystem // the only mode outside macOS for now
	e.BatteryFloor = d.settings.BatteryFloor
	e.GraceMinutes = d.settings.AgentGraceMinutes
	e.CountIdleSessions = d.settings.AgentWatchCountsIdle
	if set := d.cfg.Platform.SetXfceKeepAwake; set != nil {
		set(d.settings.XfceKeepAwake)
	}
}

// Start begins the agent watch.
func (d *Daemon) Start() {
	now := d.cfg.Now()
	fmt.Fprintf(d.cfg.Out, "bulchimbeon: %s (pid %d, %s)\n", d.engine.Lang.String(), d.cfg.PID, d.cfg.Platform.Name)
	for _, p := range d.problems {
		fmt.Fprintln(d.cfg.Out, "settings: "+p)
	}
	d.run(d.engine.StartAgentWatch(now), now)
	d.writeState(now, false)
}

// Tick runs one check.
func (d *Daemon) Tick() {
	now := d.cfg.Now()
	d.reloadSettings()
	d.applySettings()
	kinds := append(slices.Clone(engine.BuiltIn), engine.CustomKinds(d.settings.CustomAgents)...)

	scan, err := d.cfg.Platform.Processes.Scan(kinds)
	d.scanErr = ""
	if err != nil {
		d.scanErr = err.Error()
	}
	cpu := d.cpu.Add(scan.CPUTimes, now)
	open, busy := d.observeAgents(scan.Snapshots, cpu, kinds)

	d.battery, _ = d.cfg.Platform.Battery.Battery()
	d.thermal = d.cfg.Platform.Thermal.Thermal()

	// The daemon always keeps an agent watch. A battery stand-down ends the order (as on
	// macOS, where the user starts it again from the menu); here it re-arms once the
	// battery allows it again.
	if d.engine.Order().Kind == engine.OrderOff && !engine.ShouldRetreat(d.battery, floor(d.settings)) {
		d.run(d.engine.StartAgentWatch(now), now)
	}

	lost := false
	if d.engine.OnDuty() {
		lost = d.cfg.Platform.Inhibitor.Refresh()
		if !lost {
			d.reportLock() // the lock may have moved: a desktop session started or ended
		}
	}
	d.run(d.engine.Tick(engine.Observation{Now: now, Battery: d.battery, Heat: d.thermal.Level,
		AgentsOpen: open, AgentsBusy: busy, AssertionLost: lost}), now)

	// A block polkit refused may be allowed later (a rule was added): try again now and then.
	if inh := d.cfg.Platform.Inhibitor; d.engine.OnDuty() && !inh.Status().Complete() && now.Sub(d.lastRetry) >= RetryInterval {
		d.lastRetry = now
		inh.Acquire()
		d.reportLock()
	}
	d.pollUpdate(now)
	d.writeState(now, false)
}

// pollUpdate starts the once-a-day check when it is due (in the background) and keeps the
// offer for state.json. A new offer is also written to the journal, for headless machines.
func (d *Daemon) pollUpdate(now time.Time) {
	if d.cfg.Update == nil {
		return
	}
	offer, ok := d.cfg.Update.Poll(d.settings.CheckForUpdates, now)
	if !ok {
		d.offer = nil
		return
	}
	u := &UpdateOffer{Version: offer.Version.String(), Critical: offer.Critical, Install: d.cfg.Install, Arch: d.cfg.Arch}
	if d.offer == nil || *d.offer != *u {
		fmt.Fprintln(d.cfg.Out, "update: "+offer.Summary(engine.EN)+" · "+update.Instructions(offer, u.Install, u.Arch, engine.EN)[0])
	}
	d.offer = u
}

func floor(s settings.Settings) *int {
	if s.BatteryFloor > 0 {
		f := s.BatteryFloor
		return &f
	}
	return nil
}

// observeAgents counts open and working agents, and keeps per-process detail for status.
func (d *Daemon) observeAgents(ps []engine.ProcessSnapshot, cpu map[int32]float64, kinds []engine.AgentKind) (open, busy map[engine.Kind]int) {
	open, busy = map[engine.Kind]int{}, map[engine.Kind]int{}
	children := engine.Children(ps)
	byKind := map[engine.Kind]*AgentState{}
	for _, p := range ps {
		k, reason, ok := engine.Match(p, kinds)
		if !ok {
			continue
		}
		total, caffeinate := engine.TreeCPU(p, children, cpu)
		working := caffeinate || total >= engine.CPUThreshold
		open[k.Kind]++
		if working {
			busy[k.Kind]++
		}
		a := byKind[k.Kind]
		if a == nil {
			a = &AgentState{ID: k.ID, Name: k.DisplayName}
			byKind[k.Kind] = a
		}
		a.Open++
		if working {
			a.Working++
		}
		a.Processes = append(a.Processes, ProcessState{PID: p.PID, Working: working, TreeCPU: total, Reason: reason.String()})
	}
	d.agents = d.agents[:0]
	for _, a := range byKind {
		sort.Slice(a.Processes, func(i, j int) bool { return a.Processes[i].PID < a.Processes[j].PID })
		d.agents = append(d.agents, *a)
	}
	sort.Slice(d.agents, func(i, j int) bool { return d.agents[i].Name < d.agents[j].Name })
	return open, busy
}

// run carries out the engine's effects in order.
func (d *Daemon) run(effects []engine.Effect, now time.Time) {
	inh := d.cfg.Platform.Inhibitor
	for _, e := range effects {
		switch e.Type {
		case engine.EffectAcquire:
			inh.Acquire()
			d.lastRetry = now
			d.reportLock()
		case engine.EffectRelease:
			inh.Release()
			d.lastLock = ""
		case engine.EffectReacquire:
			inh.Release()
			inh.Acquire()
			d.lastRetry = now
			d.reportLock()
		case engine.EffectEnableLid:
			// lid mode exists on macOS only; the engine falls back to a system watch
			d.run(d.engine.LidEnableResult(engine.LidOutcome{Kind: engine.LidCancelled}, now), now)
		case engine.EffectRestoreLid:
			// nothing to restore: lid sleep is never changed outside macOS
		case engine.EffectNotify:
			fmt.Fprintf(d.cfg.Out, "%s %s\n", e.Notice.Title, e.Notice.Body)
		case engine.EffectLog:
			if err := d.log.Write(e.Text, e.LogKind, now); err != nil {
				fmt.Fprintln(d.cfg.Out, "duty log: "+err.Error())
			}
			fmt.Fprintln(d.cfg.Out, e.Text)
		}
	}
}

// reportLock prints the lock state to the journal when it changes.
func (d *Daemon) reportLock() {
	line := LockSummary(d.cfg.Platform.Inhibitor.Status(), engine.EN)
	if line != d.lastLock {
		fmt.Fprintln(d.cfg.Out, "lock: "+line)
		d.lastLock = line
	}
}

// Stop releases the lock and marks the state file stopped. The duty log is left alone,
// as when the macOS app quits.
func (d *Daemon) Stop() {
	d.cfg.Platform.Inhibitor.Release()
	d.writeState(d.cfg.Now(), true)
	fmt.Fprintln(d.cfg.Out, "bulchimbeon: stopped")
}

// Run starts the daemon and ticks until ctx is done.
func Run(ctx context.Context, cfg Config) {
	d := New(cfg)
	d.Start()
	d.Tick()
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.Stop()
			return
		case <-ticker.C:
			d.Tick()
		}
	}
}

// State is state.json: what `status` explains. Not part of the compatibility promise.
type State struct {
	Version       int                     `json:"version"`
	PID           int                     `json:"pid"`
	Platform      string                  `json:"platform"`
	StartedAt     time.Time               `json:"startedAt"`
	UpdatedAt     time.Time               `json:"updatedAt"`
	Stopped       bool                    `json:"stopped"`
	Language      string                  `json:"language"`
	Order         string                  `json:"order"`
	OnDuty        bool                    `json:"onDuty"`
	DutyStartedAt *time.Time              `json:"dutyStartedAt,omitempty"`
	Resting       bool                    `json:"resting"`
	Heat          string                  `json:"heat"`
	WatchDetail   string                  `json:"watchDetail,omitempty"`
	Pose          string                  `json:"pose"`
	Speech        string                  `json:"speech,omitempty"`
	Agents        []AgentState            `json:"agents"`
	Lock          platform.LockStatus     `json:"lock"`
	Battery       *engine.BatteryStatus   `json:"battery"`
	Thermal       platform.ThermalReading `json:"thermal"`
	Settings      settings.Settings       `json:"settings"`
	Problems      []string                `json:"settingsProblems,omitempty"`
	ScanError     string                  `json:"scanError,omitempty"`
	LogPath       string                  `json:"logPath"`
	Update        *UpdateOffer            `json:"update,omitempty"`
}

// UpdateOffer is a newer release the daemon found, and how this machine installs it.
type UpdateOffer struct {
	Version  string `json:"version"`
	Critical bool   `json:"critical"`
	Install  string `json:"install"`
	Arch     string `json:"arch"`
}

// AgentState is one agent's open and working processes. No arguments, no directories.
type AgentState struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Open      int            `json:"open"`
	Working   int            `json:"working"`
	Processes []ProcessState `json:"processes"`
}

// ProcessState is one agent process.
type ProcessState struct {
	PID     int32   `json:"pid"`
	Working bool    `json:"working"`
	TreeCPU float64 `json:"treeCPU"`
	Reason  string  `json:"reason"`
}

func (d *Daemon) state(now time.Time, stopped bool) State {
	e := d.engine
	st := State{
		Version: StateVersion, PID: d.cfg.PID, Platform: d.cfg.Platform.Name, StartedAt: d.started, UpdatedAt: now,
		Stopped: stopped, Language: e.Lang.String(), Order: e.Order().Kind.String(), OnDuty: e.OnDuty(),
		DutyStartedAt: e.DutyStartedAt(), Resting: e.Resting(), Heat: e.Heat().String(), Pose: string(e.Pose(now)),
		Agents: append([]AgentState{}, d.agents...), Lock: d.cfg.Platform.Inhibitor.Status(), Battery: d.battery,
		Thermal: d.thermal, Settings: d.settings, Problems: d.problems, ScanError: d.scanErr, LogPath: d.cfg.Paths.Log,
		Update: d.offer,
	}
	if w := e.WatchDetail(); w != nil {
		st.WatchDetail = *w
	}
	if s, ok := e.Speech(now); ok {
		st.Speech = s
	}
	return st
}

func (d *Daemon) writeState(now time.Time, stopped bool) {
	data, err := json.MarshalIndent(d.state(now, stopped), "", "  ")
	if err == nil {
		err = settings.WriteFileAtomic(d.cfg.Paths.State, append(data, '\n'))
	}
	if err != nil {
		fmt.Fprintln(d.cfg.Out, "state: "+err.Error())
	}
}

// ReadState reads state.json.
func ReadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return &st, nil
}
