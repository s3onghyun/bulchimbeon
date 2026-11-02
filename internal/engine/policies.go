package engine

import (
	"fmt"
	"math"
	"time"
)

// BatteryStatus is the internal battery. nil means there is none (a desktop).
type BatteryStatus struct {
	Percent   int
	OnBattery bool
}

// ShouldRetreat: on battery and at or below the floor. A nil floor never retreats.
func ShouldRetreat(status *BatteryStatus, floor *int) bool {
	if status == nil || floor == nil || !status.OnBattery {
		return false
	}
	return status.Percent <= *floor
}

// WatchAction is the agent-watch decision (Swift `AgentWatchPolicy.Action`).
type WatchAction int

const (
	WatchNone WatchAction = iota
	WatchEngage
	WatchHandover
)

func (a WatchAction) String() string {
	switch a {
	case WatchEngage:
		return "engage"
	case WatchHandover:
		return "handover"
	default:
		return "none"
	}
}

// DecideWatch: engage only while an agent works right now; hand over only once it has
// been quiet for at least the grace time. Claude Code re-spawns `caffeinate -t 300`, so
// there are short gaps between runs; the grace time stops those from flickering the watch.
func DecideWatch(busyNow bool, quietFor, grace time.Duration, onDuty, lowBattery bool) WatchAction {
	if busyNow {
		if !onDuty && !lowBattery {
			return WatchEngage
		}
		return WatchNone
	}
	if onDuty && quietFor >= grace {
		return WatchHandover
	}
	return WatchNone
}

// QuietFor = now − max(last time an agent was seen working, start of the agent watch).
func QuietFor(now time.Time, lastBusyAt *time.Time, watchStartedAt time.Time) time.Duration {
	from := watchStartedAt
	if lastBusyAt != nil && lastBusyAt.After(watchStartedAt) {
		from = *lastBusyAt
	}
	return now.Sub(from)
}

// FormatClock shows elapsed or remaining time: 59 s → "0:59", 1 h 5 min 3 s → "1:05:03".
func FormatClock(d time.Duration) string {
	total := int64(math.Max(0, math.Floor(d.Seconds())))
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ResumeKind is how to continue a stored duty order after a restart (Swift `DutyResume`).
type ResumeKind int

const (
	ResumeNone ResumeKind = iota
	ResumeAgentWatch
	ResumeTimed
	ResumeExpired     // the watch time ran out while the app was closed
	ResumeFirstLaunch // a fresh install's first launch: agent watch on by default
)

// ResumeAction is a DutyResume decision. Until is set only for a timed watch with an end.
type ResumeAction struct {
	Kind  ResumeKind
	Until *time.Time
}

// maxRemaining: a stored watch with more time left than this is treated as tampering.
const maxRemaining = 24 * time.Hour

// launchedBeforeKeys are left behind by any earlier run (Swift `DutyResume.launchedBeforeKeys`):
// the app saves dutyMode on every run, so a person who turned the watch off still has it.
// Keys the app writes before the first resume (lidEngagedByUs, the update check) do not count.
var launchedBeforeKeys = []string{"dutyOrder", "dutyOrderUntil", "dutyMode", "hasSeenWelcome"}

// IsFirstLaunch reports a fresh install. isStored tells whether a settings key holds a value.
func IsFirstLaunch(isStored func(key string) bool) bool {
	for _, k := range launchedBeforeKeys {
		if isStored(k) {
			return false
		}
	}
	return true
}

// DecideResume decides whether to continue a stored order. until is seconds since 1970.
// firstLaunch only applies when no order is stored; a stored choice always wins.
func DecideResume(stored *string, until *float64, now time.Time, firstLaunch bool) ResumeAction {
	if stored == nil {
		if firstLaunch {
			return ResumeAction{Kind: ResumeFirstLaunch}
		}
		return ResumeAction{}
	}
	switch *stored {
	case "agentWatch":
		return ResumeAction{Kind: ResumeAgentWatch}
	case "timed":
		if until == nil {
			return ResumeAction{Kind: ResumeTimed}
		}
		if math.IsInf(*until, 0) || math.IsNaN(*until) {
			return ResumeAction{}
		}
		end := timeFromUnix(*until)
		if !end.After(now) {
			return ResumeAction{Kind: ResumeExpired}
		}
		if end.Sub(now) <= maxRemaining {
			return ResumeAction{Kind: ResumeTimed, Until: &end}
		}
	}
	return ResumeAction{}
}

func timeFromUnix(seconds float64) time.Time {
	whole := math.Floor(seconds)
	return time.Unix(int64(whole), int64((seconds-whole)*1e9))
}

// HeatLevel follows the army heat-index stages.
type HeatLevel int

const (
	HeatNormal   HeatLevel = iota // 정상
	HeatCaution                   // 관심: no action
	HeatAdvisory                  // 폭염 주의보: warn only
	HeatAlert                     // 폭염 경보: combat rest (pause the watch)
)

var heatNames = []string{"normal", "caution", "advisory", "alert"}

func (h HeatLevel) String() string {
	if h < HeatNormal || h > HeatAlert {
		return "alert"
	}
	return heatNames[h]
}

// ParseHeatLevel reads the debug name (`simulateHeat`).
func ParseHeatLevel(name string) (HeatLevel, bool) {
	for i, n := range heatNames {
		if n == name {
			return HeatLevel(i), true
		}
	}
	return HeatNormal, false
}

// HeatAction is the heat decision (Swift `HeatPolicy.Action`).
type HeatAction int

const (
	HeatActionNone HeatAction = iota
	HeatActionAdvisory
	HeatActionRest
	HeatActionResume
)

func (a HeatAction) String() string {
	return [...]string{"none", "advisory", "rest", "resume"}[a]
}

// CoolDown is how long it must stay cool before the watch resumes (no flapping).
const CoolDown = 120 * time.Second

// DecideHeat is the heat response.
func DecideHeat(level HeatLevel, onDuty, resting, advised bool, coolSince *time.Time, now time.Time) HeatAction {
	if resting {
		if level > HeatCaution || coolSince == nil {
			return HeatActionNone
		}
		if now.Sub(*coolSince) >= CoolDown {
			return HeatActionResume
		}
		return HeatActionNone
	}
	if !onDuty {
		return HeatActionNone
	}
	if level >= HeatAlert {
		return HeatActionRest
	}
	if level == HeatAdvisory && !advised {
		return HeatActionAdvisory
	}
	return HeatActionNone
}

// CPUHistory averages per-process CPU use over the last window.
//
// Measured: with 5-second samples, watch scripts and dev-server polling that run every
// 15 s spiked to 2-6% and kept an agent "working" forever. A 60-second average buries
// that periodic noise at about 1%.
type CPUHistory struct {
	Window  time.Duration
	samples []cpuSample
}

type cpuSample struct {
	times map[int32]uint64
	at    time.Time
}

// NewCPUHistory uses the 60-second window of the macOS app.
func NewCPUHistory() *CPUHistory { return &CPUHistory{Window: 60 * time.Second} }

// Add records cumulative CPU times (nanoseconds) and returns the average CPU percentage
// against the past sample closest to the window.
func (h *CPUHistory) Add(times map[int32]uint64, now time.Time) map[int32]float64 {
	keep := h.samples[:0]
	for _, s := range h.samples {
		if now.Sub(s.at) <= h.Window*3/2 {
			keep = append(keep, s)
		}
	}
	h.samples = keep

	var base *cpuSample
	for i := len(h.samples) - 1; i >= 0; i-- {
		if now.Sub(h.samples[i].at) >= h.Window {
			base = &h.samples[i]
			break
		}
	}
	if base == nil && len(h.samples) > 0 {
		base = &h.samples[0]
	}
	var baseTimes map[int32]uint64
	var baseAt time.Time
	if base != nil {
		baseTimes, baseAt = base.times, base.at
	}
	h.samples = append(h.samples, cpuSample{times, now})

	result := map[int32]float64{}
	if base == nil {
		return result
	}
	wall := now.Sub(baseAt).Seconds() * 1_000_000_000
	if wall <= 0 {
		return result
	}
	for pid, t := range times {
		before, ok := baseTimes[pid]
		if !ok || t < before {
			continue
		}
		result[pid] = float64(t-before) / wall * 100
	}
	return result
}
