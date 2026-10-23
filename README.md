# 🪖 Bulchimbeon (불침번)

> **Your Mac's night-watch soldier.**
> A menu bar app that keeps your Mac awake **only while your AI coding agents are actually working**,
> then salutes *"Shift over!"* and lets it sleep.
>
> **No hooks, no daemon, no installed root helper. One app. macOS 13 and later.**
> On Linux, a small daemon with a command line and a tray icon does the same job: see [Linux](#linux).

*Bulchimbeon* (불침번) is the night-watch duty in the Korean army: someone stays up all night so everyone else can sleep.
A pixel-art private takes that shift for your Mac.

> 🌐 [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md)
> 💡 [Ideas / 문의 / 建议](https://github.com/s3onghyun/bulchimbeon/discussions)

![Bulchimbeon demo](docs/media/demo.gif)

| State | The private | When |
|-------|-------------|------|
| 💤 Off duty | Dozing in his helmet, zzz | Nothing to guard |
| 👀 Standing by | Looking around | Waiting for an agent to clock in |
| 🪖 On duty | Flashlight on, on patrol | The Mac won't sleep |
| 🫡 Shift over | `^ ^` salute | Agents went quiet / watch time is up |
| 🥵 Combat rest | Sweating, eyes closed | The Mac is overheating |

### 🚶 Patrol mode

<img src="docs/sprites-preview.png" width="360" align="right" alt="Sprites">

The private patrols just under your menu bar with a flashlight. While standing by he looks around in the corner;
at shift change he salutes with a speech bubble. Clicks pass straight through, he fades when your pointer comes close,
and he stays out of full-screen apps (presentations, screen sharing, video). Size 2× / 3×, or turn him off.

### 🗣️ The private's lines

| When | He says |
|------|---------|
| On watch | "Won't doze off! …probably." · "Sleep tight. I've got this!" |
| Standing by | "Polishing my rifle… standing by." |
| Mac heating up | "Heat advisory! Hydrating 🥵" → "Combat rest! Cooling down 🧊" |
| Low battery | "Reporting to the duty officer 🔋 Battery at 15%… out of strength, standing down." |
| Sleep block lost | "Not AWOL! Back on post! 🫡" |
| You come back | "Handover report! 📋" |

## Why

To leave an agent running overnight I used to do this:

```bash
caffeinate -d                 # ...and forget to turn it off
sudo pmset disablesleep 1     # ...and forget to turn it back on → battery at 0%
```

Bulchimbeon stays awake **only while an agent is working** and hands the Mac back to normal sleep when it's done.

## Features

| Feature | What it does |
|---------|--------------|
| 🤖 Agent watch | Detects Claude Code, Codex, Gemini CLI, Cursor Agent, Copilot CLI, Aider, OpenCode (plus your own). **Only counts agents that are actually working**, not sessions that are merely open. Clocks in when one starts working and hands over 5 minutes (adjustable) after everything goes quiet |
| 🪖 Timed watch | 30 min / 1 h / 2 h / 4 h / indefinitely |
| Watch modes | Keep display on (`caffeinate -d`) · Display may sleep, Mac stays awake (`caffeinate -i`) · **Stay awake with the lid closed** (admin) |
| 🔋 Battery stand-down | Stands down when running on battery below your threshold (default 20%) |
| ⚠️ Lid safety net | Restores normal lid sleep (`disablesleep` back to off) when the watch ends or the app quits; after a crash, the next launch offers **Restore lid sleep** in the menu |
| 🌡️ Heat advisory → 🔥 combat rest | Warns when the Mac heats up; when it gets critical, pauses the watch to cool down and **resumes automatically** once it cools (no more cooking a closed Mac in a bag) |
| ⚠️ Abandoned-post check | If the sleep block disappears for any reason, it is re-engaged immediately |
| 🕵️ Watchdog | In lid-closed mode, the original sleep setting is restored within 5 s even if the app is force-quit |
| 📋 Night-watch handover | Come back after 10+ minutes away and get a report of what happened while you were gone |
| 🔁 Survives restarts | Remaining watch time carries over app restarts, updates and reboots (for security, a lid-closed watch comes back as a system watch; turn lid mode back on from the menu to re-approve it) |
| 🖥️ Who else is holding the Mac awake | The menu shows other apps blocking sleep, e.g. "Vivaldi (video)" |
| 🌐 Korean / English / Simplified Chinese | Follows the system language by default; pick one from **언어 / Language / 语言** in the menu |
| 📒 Duty log | `~/Library/Application Support/Bulchimbeon/duty-log.txt` |

## Compared with similar apps

[Adrafinil](https://github.com/kageroumado/adrafinil) solves the same problem and came first. The two take opposite
approaches, so pick the one that fits. (As of October 2026.)

| | Bulchimbeon | Adrafinil |
|---|---|---|
| How it knows an agent is working | **Watches from outside**: the agent's `caffeinate` child and CPU use | **The agent tells it**: hooks (acquire on prompt submit, release when the reply ends) plus observation |
| What installing changes | **Nothing.** One app; agent config files are never touched | Hooks added to agent configs (one-click install/uninstall), a user daemon, a root helper |
| macOS | **13 and later** | 26.4 and later |
| Awake with the lid closed | Admin password each time you turn it on; a watchdog guarantees the setting is restored | Approved once at install, no password afterwards |
| Agent asks to stay awake (long builds) | — | CLI and MCP tool |
| Heat | Warns even with the lid open; when it gets too hot, rests and **resumes automatically once cool** | Force-releases on overheating while the lid is closed |
| Distribution | Ad-hoc signed (notarization planned), personal Homebrew tap | Notarized, official Homebrew cask |

**Choose Adrafinil** if you're on macOS 26.4+ and want hooks to track turns precisely, or want agents to request
wake time themselves.
**Choose Bulchimbeon** if you're on macOS 13–15, don't want anything installed into your agent configs or system,
or would rather have a private on watch.

## Install

```bash
brew install --cask s3onghyun/tap/bulchimbeon

# Update — refresh the tap first, Homebrew's auto-update interval is long
brew update && brew upgrade --cask bulchimbeon
```

Or download the zip from [Releases](https://github.com/s3onghyun/bulchimbeon/releases) and move it to `/Applications`.
macOS 13+, universal (Apple Silicon and Intel).

> **First launch:** the app is ad-hoc signed and not notarized by Apple yet, so macOS blocks it once.
> Try to open it, then go to **System Settings → Privacy & Security → "Open Anyway"**.
> (Since macOS 15, right-click → Open no longer bypasses this.)
> Until it's notarized, macOS won't grant it notification permission, so notifications show up as **"Script Editor"**.

### Verify the download (optional)

Release zips are built only in GitHub Actions and carry a build provenance attestation.

```bash
gh attestation verify Bulchimbeon-<version>.zip --repo s3onghyun/bulchimbeon
shasum -a 256 -c Bulchimbeon-<version>.zip.sha256
```

## How it works

| Watch mode | Under the hood |
|------------|----------------|
| Keep display on | `IOPMAssertion` `PreventUserIdleDisplaySleep`, same as `caffeinate -d` |
| Display may sleep (default) | `IOPMAssertion` `PreventUserIdleSystemSleep`, same as `caffeinate -i` |
| Lid closed | The above plus `pmset -a disablesleep 1`. Closing the lid is a hardware trigger that assertions can't block |

### Agent detection

Bulchimbeon scans **your own user's** processes with libproc and matches agents by executable name, `argv[0]`,
install path or script path. Never by substring (Claude Code hooks run `node -e "...'.claude'..."`, which would match).

| Install method | What the process looks like | Matched by |
|----------------|-----------------------------|------------|
| Claude Code native installer | executable `~/.local/share/claude/versions/2.1.296` (named after the version!) | `argv[0]` = `claude`, or the install path |
| Homebrew / IDE-bundled binary | `.../claude`, `/opt/homebrew/bin/codex` | executable name |
| npm global install | `node /opt/homebrew/bin/claude` | script name |
| npx / package path | `node .../@anthropic-ai/claude-code/cli.js` | package path |
| bun / deno | `bun .../@google/gemini-cli/...` | package path |

Supported: Claude Code · Codex · Gemini CLI · Cursor Agent · Copilot CLI · Aider · OpenCode

**Only while working.** People keep several Claude Code sessions open, so counting open sessions would never let the
Mac sleep. An agent counts as working when (you can switch back to counting open sessions in the menu):

| Signal | Why (measured on a real machine) |
|--------|----------------------------------|
| It has a `caffeinate` child process | Claude Code spawns `caffeinate -i -t 300` itself, only while it's working |
| Its process tree uses ≥ 2% CPU averaged over 60 s | Covers tool runs like builds and tests. Idle sessions sit at 0–0.1%. A 5-second sample made periodic background jobs look busy forever |
| Headless sessions with no tools (`--tools=`) are ignored | Plugins such as claude-mem keep helper sessions running |

Check what gets detected on your machine, or add an agent that isn't on the list:

```bash
/Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose
defaults write io.github.s3onghyun.bulchimbeon customAgents -array goose amp
```

Agents running inside Docker or on a remote host over SSH are not processes on this Mac, so they aren't detected.

```bash
pmset -g assertions | grep Bulchimbeon   # is it on watch right now?
```

The duty logic lives in a pure state machine (`DutyEngine`) with no system calls, tested scenario by scenario with a
fake clock. Design notes, including signals we tried and rejected: [docs/design.md](docs/design.md).

## Linux

Bulchimbeon also runs on Linux as a small daemon with a command line and a [tray icon](#tray-icon).
It uses the same engine as the macOS app: a Go port that must pass the same golden test cases
([`testdata/engine`](testdata/engine)) as the Swift core, in CI on every pull request.
Tested on Ubuntu 24.04 (systemd 255). It needs systemd and logind. While on duty it blocks automatic suspend
with your desktop's own inhibitor, or with logind's when there is no desktop ([below](#the-sleep-lock-on-each-desktop)).

### Install

With [Homebrew on Linux](https://docs.brew.sh/Homebrew-on-Linux) (no root; `brew upgrade` brings new versions):

```bash
brew install s3onghyun/tap/bulchimbeon-linux
brew services start bulchimbeon-linux    # systemd user unit, started at login
```

Pick one install method. If you use Homebrew, do not also install the tarball or enable the `.deb` unit
(`systemctl --user enable bulchimbeon`), or two daemons will run.

Or download from [Releases](https://github.com/s3onghyun/bulchimbeon/releases): a tarball or a `.deb` for amd64 or arm64.

```bash
# Tarball: no root needed. Installs ~/.local/bin/bulchimbeon and a systemd user unit, then starts it
tar -xzf bulchimbeon_<version>_linux_amd64.tar.gz
cd bulchimbeon_<version>_linux_amd64 && ./install-linux.sh

# or the .deb: installs /usr/bin/bulchimbeon and the user unit; each user then enables it
sudo apt install ./bulchimbeon_<version>_amd64.deb
systemctl --user enable --now bulchimbeon
```

Verify the download (optional): `sha256sum -c SHA256SUMS --ignore-missing`, and
`gh attestation verify <file> --repo s3onghyun/bulchimbeon`.

Uninstall with `brew services stop bulchimbeon-linux && brew uninstall bulchimbeon-linux`,
`./install-linux.sh --uninstall`, or `systemctl --user disable --now bulchimbeon` followed by
`sudo apt remove bulchimbeon`. Your settings (`~/.config/bulchimbeon`) and duty log
(`~/.local/state/bulchimbeon`) are kept.

To keep the service running while you are logged out, run `loginctl enable-linger` once.

### Use

```bash
bulchimbeon status                       # on duty or not, and why
bulchimbeon doctor                       # check the desktop session, systemd, logind and polkit
bulchimbeon config set agentGraceMinutes 10
bulchimbeon config set customAgents goose amp
bulchimbeon config set language ko       # ko | en | zh | system (default: follows LANG)
journalctl --user -u bulchimbeon -f      # the daemon's log
```

```text
$ bulchimbeon status
🪖 Bulchimbeon — on duty for 0:35 (agent watch)
Lock:      blocking auto-suspend via org.gnome.SessionManager · confirmed by the session
Why:       Claude Code pid 1174857 11.7% (process-tree CPU ≥ 2.0%, 60 s average)
Agents:    Claude Code ×7 open, 1 working
Handover:  5 min after the last agent goes quiet (agentGraceMinutes)
Power:     battery: none · heat: normal (coretemp 49 °C, critical 105 °C) · lid: not handled (closing it still suspends)
```

`bulchimbeon run` is the daemon itself; the user unit runs it. It always does agent watch: the
same detection rules, a 5-second check, engage when an agent's process tree uses at least 2% CPU
(60-second average; Linux has no `caffeinate` signal), hand over after the grace time.

### Tray icon

`bulchimbeon tray` puts the private in your desktop panel, in the same poses as on the Mac menu bar and animated the
same way: asleep when the daemon is off, looking around while it waits for an agent, flashlight on while on duty,
sweating through a combat rest. Hover for the full status. The menu shows every line of `bulchimbeon status` in the
same order, then a **🧥 Uniform** picker that sets `uniformTheme`, and **Quit tray**, which closes the icon only:
the daemon keeps watch without it.

The `.deb` starts it with every desktop session through an XDG autostart entry,
`/etc/xdg/autostart/bulchimbeon-tray.desktop`; `install-linux.sh` puts the same entry in `~/.config/autostart/`.
With Homebrew, add `bulchimbeon tray` to your desktop's startup applications. Where there is nothing to show it in
(SSH, a headless machine, a desktop without a tray), or for a user who has never started the daemon, it logs one line
and exits.

It is a StatusNotifierItem, the tray protocol KDE, XFCE and the GNOME AppIndicator extension speak:

| Desktop | What it needs |
|---|---|
| XFCE 4.16 or later | the **Status Tray** panel item (`systray`), which the default panel has |
| XFCE 4.14 or older | the `xfce4-statusnotifier-plugin` panel item |
| GNOME | the AppIndicator extension (`gnome-shell-extension-appindicator`, on by default in Ubuntu) |
| KDE Plasma | nothing. Off duty or standing by, the item is Passive, so Plasma keeps it in the tray's hidden icons until the private goes on duty |

```bash
bulchimbeon tray &                          # show it now, in this desktop session
bulchimbeon tray --lang ko                  # menu in Korean (ko | en | zh), whatever the language setting says
bulchimbeon config set uniformTheme navy    # default | navy | arctic | desert: same as the Uniform menu
```

To stop it starting at login, turn it off in your desktop's session and startup settings, or remove the autostart file.

### How it works on Linux

| | Linux |
|---|---|
| Agents | Your own processes from `/proc`, with the same matching rules and the same privacy limits as on macOS |
| Sleep lock | The desktop session's suspend inhibitor, or a logind `sleep` inhibitor without one ([per desktop](#the-sleep-lock-on-each-desktop)). Never an idle lock. If the daemon dies, even with `kill -9`, the lock goes with it |
| Battery stand-down | `/sys/class/power_supply` |
| Heat | hwmon (`coretemp`, `k10temp`, …) or thermal zones: heat advisory 15 °C below the critical temperature, combat rest 5 °C below |
| Duty log | `~/.local/state/bulchimbeon/duty-log.txt`, same format as on macOS |

### The sleep lock on each desktop

| Session | Lock while on duty | Still works |
|---|---|---|
| GNOME (Ubuntu, Fedora), Cinnamon, MATE, Budgie | `org.gnome.SessionManager.Inhibit` with the suspend flag only | the screen dims and locks; your own suspend from the menu. GNOME 46 (Ubuntu 24.04) backs any suspend inhibitor with a logind block lock of its own, so `systemctl suspend` in a terminal asks for `-i` |
| KDE Plasma | `org.freedesktop.PowerManagement.Inhibit`, which PowerDevil applies to automatic suspend only | the screen dims, turns off and locks; manual suspend |
| XFCE | `org.freedesktop.PowerManagement.Inhibit` (xfce4-power-manager) | manual suspend. XFCE's power manager treats any inhibitor like presentation mode, so the screen also stays on and unlocked while on duty |
| No desktop: headless, SSH only, sway, Hyprland, other compositors | a logind `sleep` inhibitor, `block-weak` on systemd 257 and later, `block` on 256 and older, held by a `systemd-inhibit` child tied to a pipe | your own suspend. It stops logind's `IdleAction` and other users' suspend requests. On systemd 256 and older, `systemctl suspend` in a terminal asks for `-i` |

Why not logind everywhere: on systemd 256 and older (Ubuntu 24.04, Debian 12) logind ignores a block inhibitor
for requests from the user who holds it, and GNOME's, XFCE's and Cinnamon's power managers ask logind to suspend as
you. Measured on Ubuntu 24.04 GNOME: gsd-power's automatic suspend request went through logind while Bulchimbeon held
its lock. On systemd
257 and later a logind `block` lock does stop them, but it also refuses your own manual suspend.

The daemon looks for the desktop on your session bus, the user bus that `systemd --user` gives its services. Log in
to a desktop while it is on duty and the lock moves to the desktop within one check (5 seconds); log out and it moves
back to logind. The desktop drops its inhibitor when the daemon's bus connection closes, so a crash releases it.
`bulchimbeon status` shows which lock is held and why; `bulchimbeon doctor` tries each one.

### Limits

- **Closing the lid still suspends.** By default logind ignores inhibitors for the lid switch
  (`LidSwitchIgnoreInhibited=yes`), and Bulchimbeon never changes logind's settings.
- **Over SSH or on a headless machine, the sleep block needs a polkit rule.** By default polkit
  allows a `sleep` block inhibitor only from a local login (someone at the machine's screen). From an
  SSH session, or as a user service on a machine nobody logs into locally, logind refuses it,
  and `bulchimbeon status` says plainly that sleep is not blocked.
  To allow it for one user, save this as `/etc/polkit-1/rules.d/50-bulchimbeon.rules` (owner root, mode `0644`),
  with your user name; `bulchimbeon doctor` prints it with your name filled in:

  ```js
  // Bulchimbeon: let YOUR-USER-NAME hold a logind "sleep" block inhibitor from an SSH or
  // systemd --user session (polkit's default only allows it from a local session).
  // Grants nothing else. Remove this file to undo.
  polkit.addRule(function (action, subject) {
    if (action.id == "org.freedesktop.login1.inhibit-block-sleep" && subject.user == "YOUR-USER-NAME") return polkit.Result.YES;
  });
  ```

  It grants only `org.freedesktop.login1.inhibit-block-sleep` to that user; delete the file to undo.
  Bulchimbeon never runs this for you. A desktop session does not need it: its own inhibitor needs no polkit
  permission. Only the logind lock, used without a desktop, does.
- **sway, Hyprland and other compositors without a desktop power manager get the logind lock only.** It does not stop
  an idle daemon that suspends as you (`swayidle` or `hypridle` running `systemctl suspend`), because logind lets your
  own suspend through. Bulchimbeon holds no idle lock there either: on those compositors one would also stop the
  screen from locking.
- **The desktop inhibitor covers your own session.** Another user's session on the same machine (fast user switching)
  can still suspend it. A desktop whose session bus is private (started with `dbus-launch`, without
  `dbus-user-session`) is invisible to the service, which then keeps the logind lock.
- Only the "Display may sleep" watch mode exists on Linux (`dutyMode` is ignored), there is no patrol mode, and
  notifications go to the journal.
- Windows is planned: the platform layer is in [`internal/platform`](internal/platform).

## Security

The app runs `pmset` with admin rights and reads other processes' metadata, so it has its own threat model:
[SECURITY.md](SECURITY.md).

- No network access, no third-party dependencies
- No shell-injection surface: absolute paths, minimal environment, the only variable value in the admin command is an integer PID
- Never stores or prints other processes' arguments or environment; never reads AI transcripts
- Hardened runtime, SHA-256 checksums and build provenance for releases, CI actions pinned by commit SHA

## Development

Only the Command Line Tools are needed, no Xcode. The Linux daemon is a Go module (Go 1.27 or later):
`go test ./...` and `go build ./cmd/bulchimbeon`.

```bash
make test       # unit + scenario tests
make app        # dist/Bulchimbeon.app (ad-hoc signed)
make run        # build and launch
make icons      # regenerate the app icon and preview after editing sprites

# launch and start agent watch right away
open dist/Bulchimbeon.app --args --agent-watch

# print the engine's effects to stderr (for tracing)
BULCHIMBEON_DEBUG=1 dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon
```

Want to help? Start with [CONTRIBUTING.md](CONTRIBUTING.md).

### Release (maintainer)

```bash
git tag vX.Y.Z && git push origin vX.Y.Z   # → the Release workflow builds, attests and uploads
./scripts/bump-cask.sh X.Y.Z               # → verifies checksums and provenance, then updates the tap's cask and Linux formula
```

Sprites are ASCII grids in `Sources/BulchimbeonCore/Sprites.swift`: one character per pixel, drawn as vector
rectangles so both the 16 pt menu bar icon and the 1024 px app icon stay crisp.

## Roadmap

Tracked as [milestones](https://github.com/s3onghyun/bulchimbeon/milestones). 1.0 means settings keys, command-line
flags and the duty log format stop changing: see the [compatibility promise](docs/compatibility.md) (in Korean; the
tables are self-explanatory) and the [CHANGELOG](CHANGELOG.md).

- [x] Pixel-art private, patrol mode, Korean/English/Simplified Chinese UI
- [x] Detect working vs. idle agents
- [x] Homebrew cask (personal tap)
- [ ] Apple notarization and the official Homebrew cask
- [ ] Privileged helper to remove the admin prompt in lid-closed mode
- [ ] Unlockable skins: Joseon night watchman, scholar

## Contributing

Bug reports ("🚨 soldier misbehaving"), agent requests, translation fixes, and ideas for lines or sprites are
all welcome. Issues and PRs in English, Korean, Japanese or Chinese are fine. Look for
[`good first issue`](https://github.com/s3onghyun/bulchimbeon/labels/good%20first%20issue) to get started, and read
[CONTRIBUTING.md](CONTRIBUTING.md).

## Caution

Heavy, long-running work with the lid closed makes the Mac warm. Plug in power when you can.

## License

MIT
