# AGENTS.md

## Scope

`bulchimbeon` is a macOS menu-bar app (Swift Package Manager, no Xcode project) that keeps
the Mac awake while AI coding agents are running, plus a Linux daemon and CLI written in Go
(a Go module at the repository root; Windows is a stub for later). Read `CONTRIBUTING.md`
before changing anything; it is the source of truth for code layout, ground rules, adding an
agent, sprites and translations. `SECURITY.md` holds the threat model.

## Setup and verification

Requires macOS 13 or later (`Package.swift` targets `.macOS(.v13)`) and the Xcode Command
Line Tools with a Swift 6 or newer toolchain, because the tests use Swift Testing. Full
Xcode is optional. There are no third-party dependencies to install. The app links IOKit
and ServiceManagement, so the Swift package does not build on Linux; the Go module below does.

```sh
make build   # swift build
make test    # swift test, plus the flags the Command Line Tools need
make app     # ./scripts/build-app.sh -> dist/Bulchimbeon.app (ad-hoc signed)
make icons   # after editing sprites: regenerates Resources/AppIcon.icns and docs/sprites-preview.png
```

CI (`.github/workflows/ci.yml`, check name `test`) runs `swift test`, builds the app bundle
and verifies its signature on macOS. Run `make test` and `make app` before opening a pull
request. Do not claim a command passed if it was not run; if you are not on macOS or a
prerequisite is missing, say so and include any failure output.

## Go module (Linux daemon)

Go 1.27 or newer, standard library only (no `require` in `go.mod`).

```sh
go vet ./... && go test ./...       # what the CI job `go (linux)` runs, plus gofmt
go build ./cmd/bulchimbeon          # run | status | doctor | config
./scripts/package-linux.sh <version> <amd64|arm64>   # tar.gz and .deb into dist/linux
```

- Engine truth lives in the Swift core (`Sources/BulchimbeonCore`). `internal/engine` is its Go
  port, and both must pass the shared fixtures in `testdata/engine/` (format in its README).
- To change engine behavior: change the Swift core, update the fixture until `swift test` passes,
  then make `go test ./...` pass. Never edit a fixture just to make the Go port pass.
- `internal/platform/linux` holds every Linux system call (`/proc`, `systemd-inhibit`, `busctl`,
  sysfs); keep `internal/daemon` and `cmd/bulchimbeon` free of duty logic. Run external tools by
  absolute path with a minimal environment, never through a shell, and never print process arguments.
- The polkit rule is only ever printed (`bulchimbeon doctor`, README); never run it.

## Project conventions

- Put logic, especially agent detection and duty decisions, in `Sources/BulchimbeonCore/`
  with tests in `Tests/BulchimbeonCoreTests/`, mirrored in `internal/engine` through the
  fixtures. Keep `Sources/Bulchimbeon/` a thin shell that calls the system.
- No network access apart from the opt-out once-a-day update check (`UpdateCheck` in Core);
  no telemetry. No third-party dependencies.
- Write every user-facing string in both Korean and English: `L10n.t("한국어", "English")`.
- After changing `Sources/BulchimbeonCore/Sprites.swift`, run `make icons` and commit the
  regenerated `Resources/AppIcon.icns` and `docs/sprites-preview.png`.
- Settings keys, launch flags and the duty-log format follow `docs/compatibility.md`.
- Keep a PR small and focused. Do not reformat unrelated files.

## Security-sensitive paths

Treat these as security-sensitive and do not change them incidentally: `.github/workflows/`,
`scripts/` (app build, signing, the Homebrew tap bump, Linux packaging and install),
`Package.swift`, `go.mod`, `Makefile`,
`SECURITY.md`, and anything touching releases or the Homebrew tap. Never add secrets,
tokens, a new third-party Action, or a workflow trigger such as `pull_request_target`.
Never put outside strings into the admin (`pmset`) command, and never store or print other
processes' command lines or environment variables.

## AI-assisted work

AI assistance is welcome for focused, reviewed work. The human submitting the pull request
is responsible for every line, test result, license/copyright implication, and follow-up.
Disclose material AI assistance in the PR template; do not submit unreviewed bulk-generated
changes or claim tests you did not run.
