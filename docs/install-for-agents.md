# Installing Bulchimbeon: steps for an AI agent

This page is written for an AI coding agent (Claude Code, Codex, Gemini CLI and others) that a person
has asked to install Bulchimbeon. A person can follow it too. Every step gives the commands, the check
that tells you it worked, and what to do if it did not.

Bulchimbeon is a macOS menu-bar app (macOS 13 or later) that keeps the Mac awake only while AI coding
agents are working. On Linux it is a small daemon with a command line. Project:
<https://github.com/s3onghyun/bulchimbeon>.

## Rules (read first)

- **Sources.** Install only from the Homebrew cask `s3onghyun/tap/bulchimbeon` (tap `s3onghyun/homebrew-tap`),
  the Linux formula `s3onghyun/tap/bulchimbeon-linux`, or the release assets on
  `https://github.com/s3onghyun/bulchimbeon/releases`. No mirrors, other taps, forks or "latest" links elsewhere.
- **Checksums.** If you download a file yourself, check its SHA-256 against the checksum file from the
  same release before you use it. If it does not match, stop and tell the person.
- **Never remove the quarantine attribute** (`xattr -d com.apple.quarantine`, `xattr -c`, `xattr -r ...`) and
  **never turn Gatekeeper off** (`spctl --master-disable`, `spctl --global-disable`, `spctl --add`). The first-launch
  approval is the person's decision; hand it to them (step M6).
- **No `sudo` for the macOS app.** Neither Homebrew nor copying into `/Applications` needs it. If a step fails with a
  permission error, stop and tell the person rather than retrying with `sudo`.
- **Show before you run.** Tell the person each command before you run it. On Linux, the `.deb` and `.rpm`
  installs need `sudo`; let the person run or approve that command.
- **One command at a time.** Each code line below is one command, so a person can approve them one by one.
  Do not join them with `&&`, `;` or `$(...)`, and do not rely on shell variables surviving between commands.
  Where a line says `X.Y.Z`, write the version from M3 in its place.
- **Don't guess.** If a check fails and the "on failure" note does not cover it, stop and report the exact output.

Pick the branch for this computer: `uname -s` prints `Darwin` (macOS, go to [macOS](#macos)) or `Linux`
(go to [Linux](#linux)).

## macOS

### M1. Check the system

```sh
sw_vers -productVersion
uname -m
```

- **Expect:** a version of 13 or higher (for example `15.5`), and `arm64` or `x86_64`. The app is universal.
- **On failure:** below 13, stop: Bulchimbeon needs macOS 13 (Ventura) or later.

### M2. Is it already installed?

```sh
ls -d /Applications/Bulchimbeon.app
defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString
command -v brew
brew list --cask --versions bulchimbeon
```

Run the last line only if `command -v brew` printed a path.

- `ls` says `No such file or directory`: not installed. Skip the `defaults` line and go to M3.
- `brew list` prints `bulchimbeon X.Y.Z`: installed with Homebrew. To update, run `brew update`, then
  `brew upgrade --cask bulchimbeon`, then go to M5.
- The app exists but `brew list` prints nothing (or there is no Homebrew): installed from the zip. Ask the person
  whether to replace it with the latest release (M4b), or keep it and go to M5.

### M3. Find the latest release

```sh
curl -fsSLI -o /dev/null -w '%{url_effective}\n' https://github.com/s3onghyun/bulchimbeon/releases/latest
```

- **Expect:** `https://github.com/s3onghyun/bulchimbeon/releases/tag/vX.Y.Z`. Note `X.Y.Z`; the steps below use it.
- **On failure:** no network or GitHub is unreachable. Stop and tell the person.

### M4a. Install with Homebrew (preferred when `command -v brew` printed a path)

```sh
brew update
brew install --cask s3onghyun/tap/bulchimbeon
defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString
```

- **Expect:** `🍺  bulchimbeon was successfully installed!`, the caveat about "Open Anyway", and then `X.Y.Z`.
  Homebrew checks the SHA-256 pinned in the cask itself.
- **On failure:** `It seems there is already an App at '/Applications/Bulchimbeon.app'` means a zip install exists;
  ask the person before replacing it (`brew install --cask --force s3onghyun/tap/bulchimbeon`). Any other error:
  stop and report it. Do not fall back to `sudo`.

Then go to M5.

### M4b. Install from the release zip (no Homebrew)

Download into a folder of its own:

```sh
mkdir -p /tmp/bulchimbeon-install
curl -fsSL -o /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip https://github.com/s3onghyun/bulchimbeon/releases/download/vX.Y.Z/Bulchimbeon-X.Y.Z.zip
curl -fsSL -o /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip.sha256 https://github.com/s3onghyun/bulchimbeon/releases/download/vX.Y.Z/Bulchimbeon-X.Y.Z.zip.sha256
```

Check the SHA-256:

```sh
cat /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip.sha256
shasum -a 256 /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip
```

- **Expect:** both lines start with the same 64-character hash.
- **On failure:** the hashes differ, or a download failed: stop. Do not unzip or install anything; tell the person.
- **Optional, if `gh` is installed and signed in:**
  `gh attestation verify /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip --repo s3onghyun/bulchimbeon`
  should end with `✓ Verification succeeded!`. On failure, stop.

Unpack and check the signature:

```sh
ditto -x -k /tmp/bulchimbeon-install/Bulchimbeon-X.Y.Z.zip /tmp/bulchimbeon-install/out
codesign --verify --deep --strict /tmp/bulchimbeon-install/out/Bulchimbeon.app
codesign -dv /tmp/bulchimbeon-install/out/Bulchimbeon.app
```

- **Expect:** the `--verify` line prints nothing and exits 0; `codesign -dv` (it prints to stderr) shows
  `Identifier=io.github.s3onghyun.bulchimbeon`.
- **On failure:** stop and report the output. Do not install.

If an older copy is installed, quit it first with `osascript -e 'quit app "Bulchimbeon"'`. Then copy it in:

```sh
rm -rf /Applications/Bulchimbeon.app
ditto /tmp/bulchimbeon-install/out/Bulchimbeon.app /Applications/Bulchimbeon.app
defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString
```

- **Expect:** `X.Y.Z`. The `rm -rf` touches only that one app bundle; never widen it.
- **On failure:** `Permission denied` means this account cannot write to `/Applications` (a standard account).
  Stop and tell the person; do not use `sudo`.

### M5. Launch with agent watch on

A fresh install starts **off duty**: it guards nothing until **Agent watch** is turned on. The `--agent-watch`
launch flag turns it on, and the app remembers it across restarts.

```sh
pgrep -x Bulchimbeon
```

- **No process ID:** the app is not running. Launch it with agent watch on, wait a few seconds, and check again:

  ```sh
  open /Applications/Bulchimbeon.app --args --agent-watch
  sleep 5
  pgrep -x Bulchimbeon
  ```

  A process ID now means it is running: skip to M6's last part (ask the person to check the menu).
  Still no process ID: macOS has most likely blocked the first launch, because the app is ad-hoc signed and not yet
  notarized by Apple. Go to M6. (Whether macOS asks depends on how the file arrived and whether this Mac approved the
  build before: Homebrew marks the download, so macOS usually asks; a `curl` download is not marked, so it may open
  straight away. Either way, leave the attribute alone.)
- **A process ID right away:** it was already running, and `--args` does not reach a running app. Go to M6's last part.

### M6. Hand-off to the person

If the launch was blocked, stop and give the person these steps, then wait until they say they are done:

1. In the dialog that says Bulchimbeon can't be opened, click **Done**.
2. Open **System Settings → Privacy & Security** and scroll down to **Security**.
3. Next to *"Bulchimbeon" was blocked*, click **Open Anyway**, then confirm with your password or Touch ID.
   (Since macOS 15, right-click → Open no longer does this.)

Screenshot of that setting:
<https://github.com/s3onghyun/bulchimbeon/blob/main/docs/media/gatekeeper-open-anyway.png>

When they confirm, check:

```sh
pgrep -x Bulchimbeon
```

- **Expect:** a process ID.
- **On failure:** ask the person whether they saw **Open Anyway** (it appears only after a blocked launch, for about
  an hour). Do not try to bypass it yourself.

In every case, finish with the person: ask them to click the soldier in the menu bar and check that
**🤖 Agent watch (auto shift)** is ticked (tick it if not; after Open Anyway the app starts without the flag). The
first line of the menu then reads **Bulchimbeon — Waiting for agents** (or **On duty** if an agent is busy).
That, with a process ID from `pgrep`, means the install worked.

Tell the person: notifications from Bulchimbeon show up as **"Script Editor"**. That is expected until the app is
notarized.

### M7. Show the person how to see it on duty

Do not expect this to pass inside the session that installed the app: whether your own session counts as a working
agent depends on how you were started, and an install session is mostly waiting for approvals. Give the person this
check for later, and run it once yourself if they want:

1. Give any agent a real task in another terminal (or just keep using the agent normally), for example running the
   project's tests.
2. Within a few seconds the soldier turns his flashlight on and the menu's first line reads **Bulchimbeon — On duty**.
3. While it is on duty:

   ```sh
   pmset -g assertions
   ```

   One line under `Listed by owning process:` reads like
   `pid 2102(Bulchimbeon): [0x...] 13:31:53 PreventUserIdleSystemSleep named: "Bulchimbeon on duty: keeping AI agents awake"`
   (pid and time differ; with **Keep display on** the kind is `PreventUserIdleDisplaySleep`). To see just that line,
   use `pmset -g assertions | grep -i bulchimbeon`. While idle there is no such line; that is correct.

If it never goes on duty, check what Bulchimbeon detects:

```sh
/Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose
```

It runs a one-off scan, prints a report to standard output and exits; it does not touch the running app. The report
is always at least a heading, `Bulchimbeon agent detection (checked N processes of UID U)` (in Korean or Chinese
if that is the system language), then one `✔ <agent>  pid <n>  matched by: ...` line per detected agent, with
`· working` added when it counts as working, or `No agents detected`. Then come the other apps keeping the Mac or
display awake, and the custom-agent list. Its exact wording is for people and may change between versions.

- Agent listed, not working: normal between steps; check again during a longer task.
- No agents: the person can add one by its executable name:
  `defaults write io.github.s3onghyun.bulchimbeon customAgents -array <name>`.
- No output at all: record the exit code (`echo $?` right after) and report it with the macOS version.
- Still stuck: <https://github.com/s3onghyun/bulchimbeon/discussions/64>.

### M8. Report

Tell the person: the installed version, how it was installed (Homebrew or zip), whether they had to click
Open Anyway, that the menu shows **Agent watch** ticked, and the check from M7 to run later.

## Linux

Needs systemd and logind (tested on Ubuntu 24.04). Pick **one** method; two install methods means two daemons.
The daemon always does agent watch; there is nothing to turn on.

### L1. Check the system

```sh
uname -m
systemctl --user is-system-running
command -v brew
```

- `uname -m`: `x86_64` → `amd64` (rpm: `x86_64`), `aarch64` → `arm64` (rpm: `aarch64`). Anything else: stop.
  Below, `ARCH` means that name.
- `systemctl --user` must answer (`running` or `degraded`). If it fails, stop: the daemon needs a systemd user session.
- Run M3's command to get `X.Y.Z`.

### L2. Install (one of these)

**Homebrew on Linux** (if `brew` exists; no root):

```sh
brew install s3onghyun/tap/bulchimbeon-linux
brew services start bulchimbeon-linux
```

**Tarball** (no root; any distribution, including Arch). Download and check:

```sh
mkdir -p /tmp/bulchimbeon-install
curl -fsSL -o /tmp/bulchimbeon-install/bulchimbeon_X.Y.Z_linux_ARCH.tar.gz https://github.com/s3onghyun/bulchimbeon/releases/download/vX.Y.Z/bulchimbeon_X.Y.Z_linux_ARCH.tar.gz
curl -fsSL -o /tmp/bulchimbeon-install/SHA256SUMS https://github.com/s3onghyun/bulchimbeon/releases/download/vX.Y.Z/SHA256SUMS
grep bulchimbeon_X.Y.Z_linux_ARCH.tar.gz /tmp/bulchimbeon-install/SHA256SUMS
sha256sum /tmp/bulchimbeon-install/bulchimbeon_X.Y.Z_linux_ARCH.tar.gz
```

- **Expect:** the two hashes match. If not, stop.

Then unpack and run its installer (it puts `bulchimbeon` in `~/.local/bin` and starts a systemd user unit):

```sh
tar -xzf /tmp/bulchimbeon-install/bulchimbeon_X.Y.Z_linux_ARCH.tar.gz -C /tmp/bulchimbeon-install
/tmp/bulchimbeon-install/bulchimbeon_X.Y.Z_linux_ARCH/install-linux.sh
```

**`.deb`** (Debian, Ubuntu) or **`.rpm`** (Fedora and others): download and check the same way, with
`bulchimbeon_X.Y.Z_ARCH.deb` or `bulchimbeon-X.Y.Z-1.<x86_64|aarch64>.rpm`, then have the person run the `sudo` line
and enable the unit:

```sh
sudo apt install /tmp/bulchimbeon-install/bulchimbeon_X.Y.Z_ARCH.deb
systemctl --user enable --now bulchimbeon
```

(Fedora: `sudo dnf install /tmp/bulchimbeon-install/bulchimbeon-X.Y.Z-1.x86_64.rpm` instead of the `apt` line.)

### L3. Verify

```sh
bulchimbeon doctor
bulchimbeon status
```

- **Expect:** `doctor` starts with `Bulchimbeon doctor`, marks `systemd-inhibit` and `logind` with `✔`, names the
  lock in a `→ lock while on duty:` line and shows `✔ service: active, enabled`. It may print a polkit rule for SSH or
  headless use; only show it to the person, never install it on their behalf. `status` starts with
  `🪖 Bulchimbeon —`; during an agent's real task (2% CPU over 60 seconds) it says `on duty` and its `Why:` line names
  the agent. As on macOS, give the person that check for later rather than expecting it during the install.
- **On failure:** `command not found` after the tarball: `~/.local/bin` is not on `PATH`; use
  `~/.local/bin/bulchimbeon`. Report what `doctor` says.

Optional tray icon: `bulchimbeon tray`. Report the version, the method and the `doctor` result to the person.
