# Installing Bulchimbeon: steps for an AI agent

This page is written for an AI coding agent (Claude Code, Codex, Gemini CLI and others) that a person
has asked to install Bulchimbeon. A person can follow it too. Every step gives the command, the check
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
- **Don't guess.** If a check fails and the "on failure" note does not cover it, stop and report the exact output.

Pick the branch for this computer: `uname -s` prints `Darwin` (macOS, go to [macOS](#macos)) or `Linux`
(go to [Linux](#linux)).

## macOS

### M1. Check the system

```sh
sw_vers -productVersion; uname -m
```

- **Expect:** a version of 13 or higher (for example `15.5`), and `arm64` or `x86_64`. The app is universal.
- **On failure:** below 13, stop: Bulchimbeon needs macOS 13 (Ventura) or later.

### M2. Is it already installed?

```sh
ls -d /Applications/Bulchimbeon.app 2>/dev/null && defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString
command -v brew >/dev/null && brew list --cask bulchimbeon 2>/dev/null
```

- Nothing printed: not installed. Continue with M3.
- A version printed and `brew list` printed files: installed with Homebrew. To update, run
  `brew update && brew upgrade --cask bulchimbeon`, then go to M7.
- A version printed but no Homebrew install: installed from the zip. Ask the person whether to replace it with
  the latest release (M4b, which quits the running copy first), or keep it and go to M7.

### M3. Find the latest release

```sh
curl -fsSLI -o /dev/null -w '%{url_effective}\n' https://github.com/s3onghyun/bulchimbeon/releases/latest
```

- **Expect:** `https://github.com/s3onghyun/bulchimbeon/releases/tag/vX.Y.Z`. Write down `X.Y.Z`; the steps below
  call it `VERSION`.
- **On failure:** no network or GitHub is unreachable. Stop and tell the person.

### M4a. Install with Homebrew (preferred when `command -v brew` prints a path)

```sh
brew update && brew install --cask s3onghyun/tap/bulchimbeon
```

- **Expect:** `🍺  bulchimbeon was successfully installed!` and the caveat about "Open Anyway". Homebrew checks the
  SHA-256 pinned in the cask itself.
- **Check:** `defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString` prints `VERSION`.
- **On failure:** `It seems there is already an App at '/Applications/Bulchimbeon.app'` means a zip install exists;
  ask the person before replacing it (`brew install --cask --force s3onghyun/tap/bulchimbeon`). Any other error:
  stop and report it. Do not fall back to `sudo`.

Then go to M5.

### M4b. Install from the release zip (no Homebrew)

Run in one shell, replacing `X.Y.Z` with `VERSION`:

```sh
VERSION=X.Y.Z
WORK="$(mktemp -d)" && cd "$WORK" && echo "$WORK"
BASE="https://github.com/s3onghyun/bulchimbeon/releases/download/v$VERSION"
curl -fsSLO "$BASE/Bulchimbeon-$VERSION.zip" && curl -fsSLO "$BASE/Bulchimbeon-$VERSION.zip.sha256"
shasum -a 256 -c "Bulchimbeon-$VERSION.zip.sha256"
```

- **Expect:** the last line is `Bulchimbeon-X.Y.Z.zip: OK`.
- **On failure:** `FAILED` or `No such file`: stop. Do not unzip or install anything; tell the person.
- **Optional, if `gh` is installed and signed in:** `gh attestation verify "Bulchimbeon-$VERSION.zip" --repo s3onghyun/bulchimbeon`
  should end with `✓ Verification succeeded!`. On failure, stop.

Unpack, check the signature, then copy into `/Applications` (same shell):

```sh
ditto -x -k "Bulchimbeon-$VERSION.zip" "$WORK/out"
codesign --verify --deep --strict "$WORK/out/Bulchimbeon.app" && echo signature-ok
codesign -dv "$WORK/out/Bulchimbeon.app" 2>&1 | grep '^Identifier='
```

- **Expect:** `signature-ok` and `Identifier=io.github.s3onghyun.bulchimbeon`.
- **On failure:** stop and report the output. Do not install.

If an older copy is installed, quit it first: `osascript -e 'quit app "Bulchimbeon"'`. Then:

```sh
rm -rf /Applications/Bulchimbeon.app && ditto "$WORK/out/Bulchimbeon.app" /Applications/Bulchimbeon.app
defaults read /Applications/Bulchimbeon.app/Contents/Info CFBundleShortVersionString
```

- **Expect:** `VERSION`. The `rm -rf` touches only that one app bundle; never widen it.
- **On failure:** `Permission denied` means this account cannot write to `/Applications` (a standard account).
  Stop and tell the person; do not use `sudo`.

### M5. First launch

```sh
open /Applications/Bulchimbeon.app
sleep 5; pgrep -x Bulchimbeon
```

- **Expect:** `pgrep` prints a process ID. The app is running and you can skip M6.
- **No process ID:** macOS has most likely blocked the first launch, because the app is ad-hoc signed and not yet
  notarized by Apple. Go to M6. (Whether macOS asks depends on how the file arrived: Homebrew marks it as
  downloaded, so macOS asks; a `curl` download is not marked, so it may open straight away. Either way, leave
  the attribute alone.)

### M6. Hand-off: the person clicks Open Anyway

Stop and give the person these steps, then wait until they say they are done:

1. In the dialog that says Bulchimbeon can't be opened, click **Done**.
2. Open **System Settings → Privacy & Security** and scroll down to **Security**.
3. Next to *"Bulchimbeon" was blocked*, click **Open Anyway**, then confirm with your password or Touch ID.
   (Since macOS 15, right-click → Open no longer does this.)

Screenshot of that setting:
<https://github.com/s3onghyun/bulchimbeon/blob/main/docs/media/gatekeeper-open-anyway.png>

When they confirm, check:

```sh
pgrep -x Bulchimbeon || { open /Applications/Bulchimbeon.app; sleep 5; pgrep -x Bulchimbeon; }
```

- **Expect:** a process ID, and a pixel-art soldier in the menu bar.
- **On failure:** ask the person whether they saw **Open Anyway** (it appears only after one blocked launch, for
  about an hour). Do not try to bypass it yourself.

Tell the person: notifications from Bulchimbeon show up as **"Script Editor"**. That is expected until the app is
notarized.

### M7. Verify it keeps the Mac awake

The check works while an agent is working. You are one: run this while you are still mid-task, so your own
process counts as working (Claude Code, for example, runs `caffeinate` while it works).

```sh
sleep 15; pmset -g assertions | grep -i bulchimbeon
```

- **Expect:** a line like
  `pid 2102(Bulchimbeon): [0x...] 13:31:53 PreventUserIdleSystemSleep named: "Bulchimbeon on duty: keeping AI agents awake"`
  (the pid and time differ; with **Keep display on** the kind is `PreventUserIdleDisplaySleep`).
- **No line:** run the diagnosis and read it with the person:

  ```sh
  /Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose
  ```

  It lists each detected agent with `✔`, and `· working` when it counts as working. If your agent is listed but
  not working, that is normal for agents that use little CPU between steps: ask the person to give you or another
  agent a longer task (for example running tests) and check again. If no agent is listed, the person can add one by
  name: `defaults write io.github.s3onghyun.bulchimbeon customAgents -array <name>`.
  Still stuck: <https://github.com/s3onghyun/bulchimbeon/discussions/64>.

When idle, Bulchimbeon releases the block and the `grep` prints nothing. That is correct, not a failure.

### M8. Report

Tell the person: the installed version, how it was installed (Homebrew or zip), whether they had to click
Open Anyway, and the `pmset` line from M7 (or the `--diagnose` result).

## Linux

Needs systemd and logind (tested on Ubuntu 24.04). Pick **one** method; two install methods means two daemons.

### L1. Check the system

```sh
uname -m; systemctl --user is-system-running; command -v brew
```

- `uname -m`: `x86_64` → `amd64` (rpm: `x86_64`), `aarch64` → `arm64` (rpm: `aarch64`). Anything else: stop.
- `systemctl --user` must answer (`running` or `degraded`). If it fails, stop: the daemon needs a systemd user session.
- Also run M3's command to get `VERSION`.

### L2. Install (one of these)

**Homebrew on Linux** (if `brew` exists; no root):

```sh
brew install s3onghyun/tap/bulchimbeon-linux && brew services start bulchimbeon-linux
```

**Tarball** (no root; any distribution, including Arch). Replace `X.Y.Z` and `ARCH` (`amd64` or `arm64`):

```sh
VERSION=X.Y.Z; ARCH=amd64
WORK="$(mktemp -d)" && cd "$WORK"
BASE="https://github.com/s3onghyun/bulchimbeon/releases/download/v$VERSION"
curl -fsSLO "$BASE/bulchimbeon_${VERSION}_linux_${ARCH}.tar.gz" && curl -fsSLO "$BASE/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
```

- **Expect:** `./bulchimbeon_X.Y.Z_linux_ARCH.tar.gz: OK`. On `FAILED` or `no file was verified`, stop.

```sh
tar -xzf "bulchimbeon_${VERSION}_linux_${ARCH}.tar.gz"
cd "bulchimbeon_${VERSION}_linux_${ARCH}" && ./install-linux.sh
```

**`.deb`** (Debian, Ubuntu) or **`.rpm`** (Fedora and others): download and check the same way, with
`bulchimbeon_${VERSION}_${ARCH}.deb` or `bulchimbeon-${VERSION}-1.<x86_64|aarch64>.rpm`, then have the person run
the `sudo` line:

```sh
sudo apt install ./bulchimbeon_${VERSION}_${ARCH}.deb          # or: sudo dnf install ./bulchimbeon-${VERSION}-1.x86_64.rpm
systemctl --user enable --now bulchimbeon
```

### L3. Verify

```sh
bulchimbeon doctor
sleep 15; bulchimbeon status
```

- **Expect:** `doctor` starts with `Bulchimbeon doctor`, marks `systemd-inhibit` and `logind` with `✔`, names the
  lock in a `→ lock while on duty:` line and shows `✔ service: active, enabled`. It may print a polkit rule for SSH or
  headless use; only show it to the person, never install it on their behalf. While you are working, `status`
  starts with `🪖 Bulchimbeon — on duty` and its `Why:` line names an agent.
- **On failure:** `command not found` after the tarball: `~/.local/bin` is not on `PATH`; use
  `~/.local/bin/bulchimbeon`. Not on duty: Linux counts an agent as working at 2% CPU over 60 seconds, so run
  `status` again during a longer task. Report what `doctor` says.

Optional tray icon: `bulchimbeon tray`. Report the version, the method and the `status` output to the person.
