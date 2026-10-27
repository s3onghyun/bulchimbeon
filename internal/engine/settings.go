package engine

import (
	"slices"
	"strconv"
)

// Settings keys and allowed values (Swift `SettingsKey` / `SettingsValues`).
//
// Public keys are covered by the 1.0 compatibility promise in docs/compatibility.md:
// names and value formats do not change. On macOS they live in UserDefaults; on Linux in
// the settings file that `bulchimbeon config` edits.
const (
	KeyDutyMode             = "dutyMode"
	KeyBatteryFloor         = "batteryFloor"
	KeyAgentGraceMinutes    = "agentGraceMinutes"
	KeyAgentWatchCountsIdle = "agentWatchCountsIdle"
	KeyCustomAgents         = "customAgents"
	KeyPatrolEnabled        = "patrolEnabled"
	KeyPatrolScale          = "patrolScale"
	KeyUniformTheme         = "uniformTheme"

	// KeySimulateHeat is internal (not promised): `normal|caution|advisory|alert`, for testing.
	KeySimulateHeat = "simulateHeat"
)

// PublicSettingsKeys lists the promised keys in their frozen order.
var PublicSettingsKeys = []string{KeyDutyMode, KeyBatteryFloor, KeyAgentGraceMinutes, KeyAgentWatchCountsIdle,
	KeyCustomAgents, KeyPatrolEnabled, KeyPatrolScale, KeyUniformTheme}

// InternalSettingsKeys are app state on macOS and may change at any time.
var InternalSettingsKeys = []string{"dutyOrder", "dutyOrderUntil", "lidEngagedByUs", "hasSeenWelcome", KeySimulateHeat}

var (
	DutyModes     = []DutyMode{ModeDisplay, ModeSystem, ModeLid}
	BatteryFloors = []int{10, 20, 30, 0} // 0 = off
	GraceMinutes  = []int{1, 3, 5, 10}
	PatrolScales  = []int{2, 3}
)

const (
	DefaultBatteryFloor = 20
	DefaultGraceMinutes = 5
	DefaultPatrolScale  = 2
	DefaultUniformTheme = "default"
)

// LaunchArguments are the promised macOS launch flags. The Linux CLI accepts
// `--agent-watch` on `run` for the same meaning.
var LaunchArguments = []string{"--agent-watch", "--diagnose"}

// ValidBatteryFloor reports whether v is an allowed `batteryFloor`.
func ValidBatteryFloor(v int) bool { return slices.Contains(BatteryFloors, v) }

// ValidGraceMinutes reports whether v is an allowed `agentGraceMinutes`.
func ValidGraceMinutes(v int) bool { return slices.Contains(GraceMinutes, v) }

func itoa(n int) string { return strconv.Itoa(n) }
