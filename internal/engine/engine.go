package engine

import "time"

// DutyMode is how the watch keeps the machine awake. The values are the frozen
// `dutyMode` settings values. Linux v0.1 implements "system" only.
type DutyMode string

const (
	ModeDisplay DutyMode = "display"
	ModeSystem  DutyMode = "system"
	ModeLid     DutyMode = "lid"
)

// Title is the menu title of the macOS app (kept identical for parity).
func (m DutyMode) Title(lang Lang) string {
	switch m {
	case ModeDisplay:
		return T(lang, "화면 켜둠 (caffeinate -d)", "Keep display on (caffeinate -d)", "保持屏幕常亮 (caffeinate -d)")
	case ModeLid:
		return T(lang, "덮개 닫아도 깨어 있음 (관리자 권한)", "Stay awake with lid closed (admin)", "合盖后保持唤醒（需要管理员权限）")
	default:
		return T(lang, "화면은 꺼져도 맥은 깨어 있음 (caffeinate -i)", "Display may sleep, Mac stays awake (caffeinate -i)", "允许屏幕休眠，Mac 保持唤醒 (caffeinate -i)")
	}
}

// OrderKind is the standing duty order.
type OrderKind int

const (
	OrderOff OrderKind = iota
	OrderTimed
	OrderAgentWatch
)

func (o OrderKind) String() string {
	return [...]string{"off", "timed", "agentWatch"}[o]
}

// DutyOrder is the current order. Until is set for a timed watch with an end; nil means
// indefinitely.
type DutyOrder struct {
	Kind  OrderKind
	Until *time.Time
}

// LogKind classifies duty-log entries (Swift `DutyLogEntry.Kind`).
type LogKind int

const (
	LogOnDuty LogKind = iota
	LogHandover
	LogStandDown
	LogHeat
	LogAWOL
	LogOther
)

func (k LogKind) String() string {
	return [...]string{"onDuty", "handover", "standDown", "heat", "awol", "other"}[k]
}

// EffectType is what the engine asks the shell to do.
type EffectType int

const (
	EffectAcquire    EffectType = iota // take the sleep block
	EffectRelease                      // drop it
	EffectReacquire                    // the block vanished: take the same kind again
	EffectEnableLid                    // macOS lid mode: ask for admin approval, answer with LidEnableResult
	EffectRestoreLid                   // macOS lid mode: restore lid sleep, answer with LidRestoreResult
	EffectNotify
	EffectLog
)

func (t EffectType) String() string {
	return [...]string{"acquire", "release", "reacquire", "enableLid", "restoreLid", "notify", "log"}[t]
}

// Effect is one instruction. The shell runs a tick's effects in order.
type Effect struct {
	Type    EffectType
	Mode    DutyMode // acquire
	Notice  Notice   // notify
	Text    string   // log
	LogKind LogKind  // log
}

// Observation is what the shell gathers for one 5-second tick.
type Observation struct {
	Now     time.Time
	Battery *BatteryStatus
	Heat    HeatLevel
	// AgentsOpen counts open agent processes per agent, AgentsBusy the working ones.
	AgentsOpen map[Kind]int
	AgentsBusy map[Kind]int
	// AssertionLost is true when the block the engine holds is gone from the system
	// (Swift `assertionHeld == false`). The zero value means "still held".
	AssertionLost bool
}

// QuoteKind picks which set of idle lines to show.
type QuoteKind int

const (
	QuoteOff QuoteKind = iota
	QuoteOnDuty
	QuoteStandby
)

func (q QuoteKind) String() string { return [...]string{"off", "onDuty", "standby"}[q] }

// Pose is the private's sprite pose (Swift `Sprites.Pose`).
type Pose string

const (
	PoseSleeping Pose = "sleeping"
	PoseStandby  Pose = "standby"
	PoseDuty     Pose = "duty"
	PoseSalute   Pose = "salute"
	PoseHeat     Pose = "heat"
)

// Config is the engine's starting settings.
type Config struct {
	Lang              Lang
	Mode              DutyMode
	BatteryFloor      int // 0 = off
	CountIdleSessions bool
	GraceMinutes      int
	LidSleepDisabled  bool
	LidEngagedByUs    bool
}

// DefaultConfig matches the Swift `DutyEngine` defaults.
func DefaultConfig() Config {
	return Config{Mode: ModeSystem, BatteryFloor: DefaultBatteryFloor, GraceMinutes: DefaultGraceMinutes}
}

type speech struct {
	text  string
	until time.Time
}

// Engine is the duty state machine (Swift `DutyEngine`). It makes no system calls, so a
// fake clock can drive it scenario by scenario; every hand-over bug seen in real use was
// a state transition over time (issue #2).
type Engine struct {
	// Settings. The shell may change them between ticks. Mode is the user's pick
	// (stored, checked in the menu); fallbacks never change it.
	Lang              Lang
	Mode              DutyMode
	BatteryFloor      int
	CountIdleSessions bool
	GraceMinutes      int

	// shiftMode replaces lid mode for this watch only (restart, denied prompt, heat); "" when off.
	shiftMode        DutyMode
	order            DutyOrder
	onDuty           bool
	dutyStartedAt    *time.Time
	resting          bool
	heat             HeatLevel
	quote            QuoteKind
	watchDetail      *string
	lastAgentSummary string
	lidSleepDisabled bool
	lidEngagedByUs   bool

	saluteUntil    *time.Time
	speech         *speech
	lastBusyAt     *time.Time
	watchStartedAt time.Time
	heatAdvised    bool
	coolSince      *time.Time
	// awaitingLid: the admin prompt for lid mode is open and nothing is held yet.
	awaitingLid bool
}

// New creates an engine.
func New(cfg Config, now time.Time) *Engine {
	if cfg.Mode == "" {
		cfg.Mode = ModeSystem
	}
	return &Engine{
		Lang: cfg.Lang, Mode: cfg.Mode, BatteryFloor: cfg.BatteryFloor,
		CountIdleSessions: cfg.CountIdleSessions, GraceMinutes: cfg.GraceMinutes,
		lidSleepDisabled: cfg.LidSleepDisabled, lidEngagedByUs: cfg.LidEngagedByUs,
		watchStartedAt: now,
	}
}

func (e *Engine) Order() DutyOrder          { return e.order }
func (e *Engine) OnDuty() bool              { return e.onDuty }
func (e *Engine) DutyStartedAt() *time.Time { return e.dutyStartedAt }
func (e *Engine) Resting() bool             { return e.resting }
func (e *Engine) Heat() HeatLevel           { return e.heat }
func (e *Engine) Quote() QuoteKind          { return e.quote }
func (e *Engine) WatchDetail() *string      { return e.watchDetail }
func (e *Engine) LastAgentSummary() string  { return e.lastAgentSummary }
func (e *Engine) LidSleepDisabled() bool    { return e.lidSleepDisabled }
func (e *Engine) LidEngagedByUs() bool      { return e.lidEngagedByUs }

// EffectiveMode is the mode this watch actually keeps.
func (e *Engine) EffectiveMode() DutyMode {
	if e.shiftMode != "" {
		return e.shiftMode
	}
	return e.Mode
}

// Pose is the sprite pose at a moment.
func (e *Engine) Pose(now time.Time) Pose {
	if e.resting {
		return PoseHeat
	}
	if e.saluteUntil != nil && now.Before(*e.saluteUntil) {
		return PoseSalute
	}
	if e.onDuty {
		return PoseDuty
	}
	if e.order.Kind == OrderAgentWatch {
		return PoseStandby
	}
	return PoseSleeping
}

// Speech is the speech bubble at a moment, if any.
func (e *Engine) Speech(now time.Time) (string, bool) {
	if e.speech == nil || !now.Before(e.speech.until) {
		return "", false
	}
	return e.speech.text, true
}

func (e *Engine) floor() *int {
	if e.BatteryFloor > 0 {
		f := e.BatteryFloor
		return &f
	}
	return nil
}

func (e *Engine) t(ko, en, zh string) string { return T(e.Lang, ko, en, zh) }

func notify(n Notice) Effect { return Effect{Type: EffectNotify, Notice: n} }

func logEffect(text string, kind LogKind) Effect {
	return Effect{Type: EffectLog, Text: text, LogKind: kind}
}

// MARK: commands

// StartTimed starts a timed watch. seconds nil means indefinitely; label is the shown duration.
func (e *Engine) StartTimed(seconds *time.Duration, label string, now time.Time) []Effect {
	order := DutyOrder{Kind: OrderTimed}
	if seconds != nil {
		until := now.Add(*seconds)
		order.Until = &until
	}
	e.order = order
	effects := e.engage(now)
	effects = append(effects, notify(ReportForDuty(e.Lang)))
	e.say(SpeechReporting(e.Lang), now, 4*time.Second)
	title := e.EffectiveMode().Title(e.Lang)
	effects = append(effects, logEffect(e.t("근무 투입 — "+label+", "+title, "On duty — "+label+", "+title, "开始值班 — "+label+"，"+title), LogOnDuty))
	return effects
}

// StartAgentWatch waits for agents and keeps watch while they work.
func (e *Engine) StartAgentWatch(now time.Time) []Effect {
	e.watchStartedAt = now
	e.order = DutyOrder{Kind: OrderAgentWatch}
	e.quote = QuoteStandby
	return []Effect{logEffect(e.t("에이전트 근무 대기 시작", "Started waiting for agents", "开始等待代理"), LogOther)}
}

// Stop ends any watch.
func (e *Engine) Stop(now time.Time) []Effect {
	return e.standDown(nil, e.t("근무 종료 (수동)", "Off duty (manual)", "结束值班（手动）"))
}

// SetMode changes the watch mode the user picked, ending any fallback and re-acquiring if on duty.
func (e *Engine) SetMode(newMode DutyMode, now time.Time) []Effect {
	before := e.EffectiveMode()
	e.Mode = newMode
	e.shiftMode = ""
	if newMode == before || !e.onDuty {
		return nil
	}
	leavingLid := before == ModeLid
	var effects []Effect
	if leavingLid && e.lidEngagedByUs {
		effects = append(effects, Effect{Type: EffectRestoreLid})
	}
	effects = append(effects, e.engage(now)...)
	title := newMode.Title(e.Lang)
	effects = append(effects, logEffect(e.t("근무 방식 변경 — "+title, "Watch mode changed — "+title, "值班模式已更改 — "+title), LogOther))
	return effects
}

// Resume continues a stored order after a restart.
func (e *Engine) Resume(action ResumeAction, now time.Time) []Effect {
	switch action.Kind {
	case ResumeNone:
		return nil
	case ResumeExpired:
		e.order = DutyOrder{}
		return []Effect{logEffect(e.t("앱이 꺼져 있는 동안 근무 시간 종료", "Watch ended while the app was closed", "应用关闭期间，值班时间已到"), LogOther)}
	}
	var effects []Effect
	// An unannounced admin prompt right after login looks like phishing → lid becomes system.
	if e.EffectiveMode() == ModeLid && !e.lidSleepDisabled {
		e.shiftMode = ModeSystem
		effects = append(effects, notify(LidResumeFallback(e.Lang)),
			logEffect(e.t("덮개 근무 → 시스템 근무로 복귀 (관리자 재승인 필요)",
				"Lid-closed watch → resumed as system watch (admin approval needed)", "合盖值班 → 恢复系统值班（需要管理员授权）"), LogOther))
	}
	switch action.Kind {
	case ResumeAgentWatch:
		e.watchStartedAt = now
		e.order = DutyOrder{Kind: OrderAgentWatch}
		e.quote = QuoteStandby
		effects = append(effects, logEffect(e.t("근무 복귀 — 에이전트 근무", "Back on watch — agent watch", "恢复值班 — 代理值班"), LogOther))
	case ResumeTimed:
		e.order = DutyOrder{Kind: OrderTimed, Until: action.Until}
		effects = append(effects, e.engage(now)...)
		e.say(SpeechResumed(e.Lang), now, 4*time.Second)
		left := e.t("무기한", "indefinitely", "无限期")
		if action.Until != nil {
			clock := FormatClock(action.Until.Sub(now))
			left = e.t("남은 시간 "+clock, clock+" left", "剩余 "+clock)
		}
		effects = append(effects, logEffect(e.t("근무 복귀 — ", "Back on watch — ", "恢复值班 — ")+left, LogOther))
	}
	return effects
}

// LidEnableResult answers EffectEnableLid.
func (e *Engine) LidEnableResult(ok bool, now time.Time) []Effect {
	e.awaitingLid = false
	if ok {
		e.lidEngagedByUs = true
		e.lidSleepDisabled = true
		return []Effect{{Type: EffectAcquire, Mode: ModeLid}}
	}
	if e.order.Kind != OrderOff { // a watch ended while the prompt was open: the next one asks again
		e.shiftMode = ModeSystem
	}
	return []Effect{notify(LidFallback(e.Lang)),
		logEffect(e.t("덮개 근무 승인 거부 — 시스템 근무로 대체", "Lid-closed watch denied — switched to system watch", "合盖值班未获授权 — 已切换为系统值班"), LogOther),
		{Type: EffectAcquire, Mode: ModeSystem}}
}

// LidRestoreResult answers EffectRestoreLid.
func (e *Engine) LidRestoreResult(ok bool) []Effect {
	if !ok {
		return nil
	}
	e.lidEngagedByUs = false
	e.lidSleepDisabled = false
	return []Effect{logEffect(e.t("덮개 잠자기 복구 완료", "Lid sleep restored", "已恢复合盖休眠"), LogOther)}
}

// LidSleepRestoredExternally reports that lid sleep came back on during a lid-closed watch
// (the macOS root watchdog, for one): this watch only continues as a system watch.
func (e *Engine) LidSleepRestoredExternally(now time.Time) []Effect {
	if !e.onDuty || e.EffectiveMode() != ModeLid {
		return nil
	}
	e.shiftMode = ModeSystem
	effects := e.engage(now)
	title := ModeSystem.Title(e.Lang)
	return append(effects, logEffect(e.t("근무 방식 변경 — "+title, "Watch mode changed — "+title, "值班模式已更改 — "+title), LogOther))
}

// MARK: tick (every 5 seconds)

// Tick runs one check.
func (e *Engine) Tick(o Observation) []Effect {
	now := o.Now
	var effects []Effect

	if e.onDuty && ShouldRetreat(o.Battery, e.floor()) && o.Battery != nil {
		p := itoa(o.Battery.Percent)
		n := BatteryRetreat(o.Battery.Percent, e.Lang)
		return e.standDown(&n, e.t("배터리 "+p+"% — 근무 철수", "Battery "+p+"% — stood down", "电量 "+p+"% — 结束值班"))
	}

	// heat
	e.heat = o.Heat
	if e.heat <= HeatCaution {
		if e.coolSince == nil {
			t := now
			e.coolSince = &t
		}
		e.heatAdvised = false
	} else {
		e.coolSince = nil
	}
	switch DecideHeat(e.heat, e.onDuty, e.resting, e.heatAdvised, e.coolSince, now) {
	case HeatActionAdvisory:
		e.heatAdvised = true
		effects = append(effects, notify(HeatAdvisoryNotice(e.Lang)), logEffect(e.t("폭염 주의보 발령", "Heat advisory", "高温提醒"), LogHeat))
		e.say(SpeechHeat(e.Lang), now, 6*time.Second)
	case HeatActionRest:
		e.resting = true
		effects = append(effects, e.disengage()...)
		effects = append(effects, notify(HeatRest(e.Lang)),
			logEffect(e.t("폭염 경보 — 전투휴식 실시", "Heat warning — combat rest", "过热警报 — 暂停值班降温"), LogHeat))
		e.say(SpeechRest(e.Lang), now, 8*time.Second)
	case HeatActionResume:
		e.resting = false
		lidDowngraded := e.EffectiveMode() == ModeLid
		if lidDowngraded {
			e.shiftMode = ModeSystem // no lid-closed watch for the rest of this watch after overheating
		}
		effects = append(effects, notify(HeatResume(lidDowngraded, e.Lang)),
			logEffect(e.t("전투휴식 종료 — 근무 복귀", "Combat rest over — back on watch", "降温休息结束 — 恢复值班"), LogOnDuty))
		e.say(SpeechBackFromRest(e.Lang), now, 6*time.Second)
		if e.order.Kind == OrderTimed {
			effects = append(effects, e.engage(now)...) // agent watch decides again below
		}
	}
	if e.resting {
		return effects
	}

	// abandoned post (while the admin prompt is open, nothing is held yet, which is fine)
	if e.onDuty && !e.awaitingLid && o.AssertionLost {
		effects = append(effects, Effect{Type: EffectReacquire}, notify(PostAbandoned(e.Lang)),
			logEffect(e.t("⚠️ 근무지 이탈 감지 — 즉시 재투입", "⚠️ Post abandoned — re-engaged", "⚠️ 防休眠失效 — 已立即恢复"), LogAWOL))
		e.say(SpeechNotAWOL(e.Lang), now, 6*time.Second)
	}

	switch e.order.Kind {
	case OrderTimed:
		if e.order.Until != nil && !now.Before(*e.order.Until) {
			salute := now.Add(10 * time.Second)
			e.saluteUntil = &salute
			e.say(SpeechTimeUp(e.Lang), now, 8*time.Second)
			n := TimeUp(e.Lang)
			effects = append(effects, e.standDown(&n, e.t("근무 시간 종료", "Watch time over", "值班时间已到"))...)
		}
	case OrderAgentWatch:
		effects = append(effects, e.agentWatchTick(o)...)
	}
	return effects
}

func (e *Engine) agentWatchTick(o Observation) []Effect {
	now := o.Now
	// on duty only while working: engage when an agent works now, hand over after the grace time
	busy, open := positive(o.AgentsBusy), positive(o.AgentsOpen)
	var busyNow bool
	if e.CountIdleSessions {
		busyNow = len(open) > 0
		if busyNow {
			e.lastAgentSummary = Summary(open)
		}
	} else {
		busyNow = len(busy) > 0
		if busyNow {
			e.lastAgentSummary = Summary(busy)
		}
	}
	if busyNow {
		t := now
		e.lastBusyAt = &t
	}
	if e.lastAgentSummary == "" {
		e.lastAgentSummary = e.t("에이전트", "Agents", "代理")
	}
	quiet := QuietFor(now, e.lastBusyAt, e.watchStartedAt)
	grace := time.Duration(e.GraceMinutes) * time.Minute
	openCount := 0
	for _, n := range open {
		openCount += n
	}
	e.watchDetail = WatchDetail(busyNow, e.CountIdleSessions, e.lastAgentSummary, openCount, e.onDuty, quiet, grace, e.Lang)

	low := ShouldRetreat(o.Battery, e.floor()) || e.heat >= HeatAlert
	switch DecideWatch(busyNow, quiet, grace, e.onDuty, low) {
	case WatchEngage:
		effects := e.engage(now)
		s := e.lastAgentSummary
		effects = append(effects, notify(AgentArrived(s, e.Lang)),
			logEffect(e.t("근무 투입 — "+s+" 출근", "On duty — "+s+" clocked in", "开始值班 — "+s+" 已开始工作"), LogOnDuty))
		e.say(SpeechArrived(e.Lang), now, 4*time.Second)
		return effects
	case WatchHandover:
		effects := e.disengage()
		e.quote = QuoteStandby
		salute := now.Add(10 * time.Second)
		e.saluteUntil = &salute
		s := e.lastAgentSummary
		effects = append(effects, notify(Handover(s, e.Lang)),
			logEffect(e.t("근무 교대 — "+s+" 작업 종료, 이상 무", "Shift over — "+s+" finished, all clear", "交班 — "+s+" 已完成工作，一切正常"), LogHandover))
		e.say(SpeechHandover(e.Lang), now, 8*time.Second)
		return effects
	}
	return nil
}

// MARK: internals

func (e *Engine) engage(now time.Time) []Effect {
	if !e.onDuty {
		t := now
		e.dutyStartedAt = &t
	}
	e.onDuty = true
	e.quote = QuoteOnDuty
	if e.EffectiveMode() == ModeLid && !e.lidSleepDisabled { // LidEnableResult acquires
		e.awaitingLid = true
		return []Effect{{Type: EffectEnableLid}}
	}
	return []Effect{{Type: EffectAcquire, Mode: e.EffectiveMode()}}
}

func (e *Engine) disengage() []Effect {
	e.onDuty = false
	e.dutyStartedAt = nil
	if e.lidEngagedByUs {
		return []Effect{{Type: EffectRelease}, {Type: EffectRestoreLid}}
	}
	return []Effect{{Type: EffectRelease}}
}

func (e *Engine) standDown(n *Notice, log string) []Effect {
	effects := e.disengage()
	e.resting = false
	e.order = DutyOrder{}
	e.quote = QuoteOff
	e.shiftMode = ""
	if n != nil {
		effects = append(effects, notify(*n))
	}
	return append(effects, logEffect(log, LogStandDown))
}

func (e *Engine) say(text string, now time.Time, d time.Duration) {
	e.speech = &speech{text, now.Add(d)}
}

// WatchDetail is the agent-watch status line (Swift `DutyEngine.watchDetail`).
func WatchDetail(busyNow, countIdle bool, summary string, openCount int, onDuty bool, quiet, grace time.Duration, lang Lang) *string {
	var s string
	switch {
	case countIdle && busyNow:
		n := itoa(openCount)
		s = T(lang, "열린 세션 "+n+"개 (열린 세션 인정 모드)", n+" open session(s) (counting idle sessions)", n+" 个已打开的会话（包括空闲会话）")
	case busyNow:
		s = T(lang, summary+" 작업 중", summary+" working", summary+" 正在工作")
	case !onDuty:
		return nil
	default:
		q, left := FormatClock(quiet), FormatClock(grace-quiet)
		s = T(lang, "조용해진 지 "+q+" · 교대까지 "+left, "Quiet for "+q+" · shift over in "+left, "已空闲 "+q+" · 距交班 "+left)
	}
	return &s
}

func positive(m map[Kind]int) map[Kind]int {
	out := map[Kind]int{}
	for k, n := range m {
		if n > 0 {
			out[k] = n
		}
	}
	return out
}
