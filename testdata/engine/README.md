# Engine fixtures

Golden test cases for the duty engine. The same files are run by two test suites:

| Runner | Engine under test |
|--------|-------------------|
| `Tests/BulchimbeonCoreTests/EngineFixtureTests.swift` (`swift test`, macOS CI job `test`) | the Swift core in `Sources/BulchimbeonCore`, the **behavior reference** |
| `internal/engine/fixture*_test.go` (`go test ./...`, CI job `go (linux)`) | the Go port used by the Linux daemon |

So behavior is locked across operating systems: a change to agent detection, CPU averaging, the duty state
machine, the battery or heat rules, the duty-log line, the private's lines or the sprites must keep both suites green.

## Rules

- **The Swift core defines the truth.** When you change engine behavior, change the Swift core first,
  update the fixture so `swift test` passes, then make the Go port pass the same file. Never edit a
  fixture only to make the Go port pass.
- Edit the JSON by hand. Keep cases small and give each a `name` that says what it pins.
  The one exception is `sprites.json`, which the Swift tests write (below).
- Add a measured process shape (from `ps`, `/proc`, `--diagnose` or `bulchimbeon status`) when you add an agent,
  plus one false-positive case, the same way `CONTRIBUTING.md` asks for the Swift tests.
- Times are seconds. Scenario times are relative to `t0`; CPU-history times are relative to
  1,000,000 s after the Unix epoch.

## Languages

The Go runner checks every text in Korean, English and Simplified Chinese. The Swift runner checks Korean, English and Simplified Chinese
wherever the Swift API takes a language (`Lines`), and the **system language** (`L10n.language`)
for texts the core renders without one: duty-engine log lines, notifications inside scenarios,
`watchDetail`, reason labels and mode titles. Those cases carry `"systemLanguage": true` in
`lines.json`, and scenario texts are given as `{"ko": …, "en": …, "zh": …}`.

## Files

### `catalog.json`: agent detection

| Key | Case shape | Checks |
|-----|-----------|--------|
| `builtIn` | `{id, displayName, binaryNames, pathMarkers, packageMarkers}` | the built-in catalog, in order |
| `match` | `{name, exe, args, custom?, kind, reason?}` | `match` → kind id (`null` = not an agent) and reason (`executableName`, `argv0`, `installPath`, `script`). `custom` lists `customAgents` names; their kind id is `custom:<name>` |
| `isCandidate` | `{exe, argv0, custom?, expect}` | whether the scanner reads more arguments |
| `needsFullArguments` | `{exe, expect}` | script runners (node, bun, deno) |
| `customKinds` | `{name, input, displayNames}` | `customAgents` validation, dedup and the 16-name cap |
| `count` | `{name, processes: [{exe, args}], custom?, expect: {id: n}}` | per-agent process counts |
| `summary` | `{counts: {id: n}, custom?, expect}` | the "Claude Code ×2, Codex" summary |

### `activity.json`: working vs. idle

`cpuThreshold` (percent) and `busyGraceSeconds` pin the constants. Each `evaluate` case is
`{name, processes: [{pid, ppid, exe, args}], cpu: {pid: percent}, custom?, expect: {id: {present, busy}}}`.
A process is busy when it has a `caffeinate` child, or when it and its descendants use at least the threshold.

### `cpu_history.json`: CPU averaging

`{tolerance, cases: [{name, window, samples: [{at, times: {pid: cumulative ns}, expect?}]}]}`.
Samples are added in order to one history; `expect` (`{pid: percent}`, exact key set) is checked when present.

### `policies.json`: pure policies

| Key | Case shape |
|-----|-----------|
| `batteryRetreat` | `{battery: {percent, onBattery} \| null, floor: n \| null, expect}` |
| `agentWatchDecide` | `{busyNow, quietFor, grace, onDuty, lowBattery, expect: engage\|handover\|none}` |
| `quietFor` | `{now, lastBusyAt \| null, watchStartedAt, expect}` |
| `heatCoolDownSeconds` | the cool-down constant |
| `heatDecide` | `{level, onDuty, resting, advised, coolSince \| null, now, expect: none\|advisory\|rest\|resume}` |
| `heatLevelName` | `{name, expect: level \| null}` (the `simulateHeat` names) |
| `dutyResume` | `{stored \| null, until: seconds \| "inf" \| null, now, expect: {action, until?}}` |
| `dutyClock` | `{seconds, expect}` |

### `settings.json`: the compatibility promise

Public and internal settings keys, allowed values and defaults, the custom-agent cap, the promised
launch arguments, and the engine's default settings. See `docs/compatibility.md`.

### `duty_log.json`: the duty-log line

`{lines: [{name, at, text, line}]}`: writing `text` at `at` to an empty duty log must produce exactly `line`.

### `update.json`: the update check

Run by `UpdateFixtureTests` (Swift) and `fixture_update_test.go`. No network: a response is given as `json` (or as raw
`text`), and the fetch itself is faked in `internal/update`.

| Key | Case shape | Checks |
|-----|-----------|--------|
| `checkIntervalSeconds`, `maxResponseBytes`, `latestReleaseAPI` | | at most one request a day, the response size cap, the only URL asked |
| `versions` | `{name, input, expect}` | version parsing (SemVer 2.0, `v` prefix, build metadata dropped, git-describe builds as their tag); `expect` is the normalized text or `null` |
| `compare` | `{a, b, expect: -1 \| 0 \| 1}` | SemVer precedence, checked both ways |
| `releases` | `{name, json \| text, expect: {version, criticalBelow} \| null}` | reading a `releases/latest` response, and the `<!-- bulchimbeon: critical [below X.Y.Z] -->` marker in its body |
| `due` | `{name, enabled, current, now, last \| null, expect}` | whether to ask now (times in seconds) |
| `offers` | `{name, enabled, current, latest \| null, expect: {version, critical} \| null}` | what the menu offers |
| `texts`, `releaseNotesTitle` | `{version, critical, menuTitle, summary}`, each text as `{ko, en, zh}` | the menu item, the `status` summary and the release-notes link title |
| `urls` | `{version, releasePage, macZip, macChecksum}` | addresses built from the version, never taken from the response |
| `requestHeaders` | `{current, expect}` | the only headers sent |

### `lines.json`: the private's lines

`detect` cases check language detection from a preference list. Each `lines` case is
`{fn, args?, systemLanguage?, ko, en, zh}`, where `fn` is a line function (`agentArrived`,
`speech.handover`, `reason.script`, `mode.system`, `watchDetail`, …), `args` its arguments, and
`ko`/`en`/`zh` the expected value: a string, a list of strings, a `{title, body}` notice, or `null`.

### `engine_scenarios.json`: the duty state machine

`{t0, scenarios: [{name, config?, steps}]}`. `config` overrides the engine defaults
(`mode`, `batteryFloor`, `countIdleSessions`, `graceMinutes`, `lidSleepDisabled`, `lidEngagedByUs`).

Each step runs one operation at time `at`, then checks the result:

| Field | Meaning |
|-------|---------|
| `op` | `startTimed` (`seconds` \| null, `label`), `startAgentWatch`, `stop`, `setMode` (`mode`), `resume` (`resume: {action, until?}`), `lidEnableResult` (`ok`; when false, an optional `failure: {cause: stopSignal \| launch \| interrupted \| error, signal?, code?}` makes it a failed approval instead of a cancelled one), `lidRestoreResult` (`ok`), `lidSleepRestoredExternally`, or `tick` |
| tick inputs | `open` and `busy` (`{id: n}`), `battery` (`{percent, onBattery}`), `heat` (default `normal`), `assertionHeld` (default `true`) |
| `effects` | the exact effect list, in order: `{type: acquire, mode}`, `release`, `reacquire`, `enableLid`, `restoreLid`, `{type: notify, line, args?}` (a `lines.json` function; `args` may be `{ko, en}`), `{type: log, kind, ko, en}` |
| `state` | fields that change at this step. Runners keep the full expected state and compare all of it after every step: `isOnDuty`, `order` (`off`, `timed`, `agentWatch`), `orderUntil`, `resting`, `mode` (the user's pick), `effectiveMode` (what this watch keeps after a lid fallback), `shiftReason` (why: `restarted`, `approvalCancelled`, `approvalFailed`, `heat`, `lidRestoredExternally`, or null), `heat`, `quote`, `dutyStartedAt`, `watchDetail` (`{ko, en}` or null), `lidEngagedByUs`, `lidSleepDisabled` |
| `checks` | `{at, pose}` and `{at, speech: lines.json speech function \| null}`, evaluated after the step |

### `sprites.json`: the private's sprites

Generated from the Swift core, not edited by hand: every registered uniform theme (`UniformTheme.all`) and
every pose (`Sprites.Pose`), as `Sprites.frames(pose, theme:)` returns them. The Linux tray icon draws this file.

| Key | Meaning |
|-----|---------|
| `size` | the sprite is `size` × `size` pixels |
| `frameSeconds` | one frame per this many seconds (the macOS menu-bar timer) |
| `transparent` | the row character for a transparent pixel |
| `poses` | the poses, in `Sprites.Pose` order |
| `themes` | `{id, name: {ko, en, zh}, palette: {char: "#RRGGBB"}, poseFrames: {pose: [frame index…]}, frames: [[row…]]}` in menu order. `palette` holds the theme's resolved colors for every character its frames use; `frames` holds each distinct frame once |

After changing `Sprites.swift` or `UniformTheme.swift`, regenerate and copy it, then run both suites:

```sh
BULCHIMBEON_WRITE_SPRITES=1 swift test --filter SpriteFixtureTests
cp testdata/engine/sprites.json internal/sprites/sprites.json
```

`SpriteFixtureTests` fails when the file differs from the core. The Go tests check that
`internal/sprites/sprites.json` (embedded in the binary) is byte-identical to this file, and that the tray
renders every frame pixel for pixel from its rows and palette.
