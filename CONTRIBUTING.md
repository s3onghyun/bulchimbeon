# Contributing / 기여 가이드

Welcome aboard, recruit 🫡 Bug reports, new agents, translation fixes, and ideas for lines or sprites are all welcome.

**[English](#english)** · **[한국어](#한국어)**

Issues and PRs in English, Korean, Japanese or Chinese are fine. 이슈·PR 은 영어·한국어·일본어·중국어 모두 괜찮습니다.

---

## English

### Where to start

| You want to… | Do this |
|--------------|---------|
| Pick up a small first task | Browse [`good first issue`](https://github.com/s3onghyun/bulchimbeon/labels/good%20first%20issue) and [`help wanted`](https://github.com/s3onghyun/bulchimbeon/labels/help%20wanted). Comment "I'll take this" so nobody duplicates work |
| Report the soldier misbehaving | [🚨 Bug report](https://github.com/s3onghyun/bulchimbeon/issues/new?template=bug-report.yml) |
| Get your agent detected | [🤖 New agent request](https://github.com/s3onghyun/bulchimbeon/issues/new?template=agent-request.yml), or send a PR yourself ([below](#adding-an-agent)) |
| Suggest a feature, line, sprite or skin | [💡 Feature request](https://github.com/s3onghyun/bulchimbeon/issues/new?template=feature-request.yml) |
| Fix a translation | [🌐 Translation fix](https://github.com/s3onghyun/bulchimbeon/issues/new?template=translation.yml), or send a PR directly ([below](#translations)) |
| Report a security problem | **Not** a public issue: [report it privately](https://github.com/s3onghyun/bulchimbeon/security/advisories/new) |

Not sure about something? Open an issue and ask. A question is a perfectly good issue.
For anything bigger than a small fix, please open an issue first so we can agree on the approach before you write code.

### Development setup

You need a Mac with macOS 13 or later and the Xcode Command Line Tools (`xcode-select --install`). Xcode itself is
not required. There are no third-party dependencies, so there is nothing else to install.

```bash
make build      # swift build
make test       # unit + scenario tests (works with Command Line Tools only)
make app        # dist/Bulchimbeon.app (ad-hoc signed)
make run        # build the app and launch it
make icons      # regenerate the app icon and preview after editing sprites

dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose   # show which agents are detected, and why
dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --demo       # scripted demo used for the README recording
BULCHIMBEON_DEBUG=1 dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon   # print engine effects to stderr
```

With full Xcode installed, plain `swift test` also works; `make test` adds flags that the Command Line Tools'
testing framework needs. CI runs `swift test`, builds the app bundle, and verifies its signature on macOS for every PR.

The Linux daemon is a Go module at the repository root (`cmd/bulchimbeon`, `internal/`). It needs Go 1.27 or later
and nothing else; it builds and tests on any OS, and its Linux backend tests run on Linux:

```bash
go test ./...                     # engine, shared fixtures, Linux backend, daemon, CLI
go build ./cmd/bulchimbeon        # then ./bulchimbeon status, doctor
./scripts/install-linux.sh ./bulchimbeon   # install it as your systemd user service (Linux)
BULCHIMBEON_TEST_LOGIND=1 go test ./internal/platform/linux/   # also take a real logind lock
```

CI runs these in the `go (linux)` job.

### Code layout

| Where | What | Tests |
|-------|------|-------|
| `Sources/BulchimbeonCore/` | Pure logic: agent matching, the duty state machine (`DutyEngine`), lines, sprites, duty log, settings keys | **Required** |
| `Sources/Bulchimbeon/` | System calls (IOKit, libproc, `pmset`) and the UI | Checked by running the app |
| `Sources/IconGen/` | Sprites → app icon and preview PNGs | — |
| `Tests/BulchimbeonCoreTests/` | Swift Testing suites for the Core target, including scenario tests with a fake clock | |
| `testdata/engine/` | Golden fixtures that the Swift tests and the Go tests both run ([format](testdata/engine/README.md)) | Required |
| `internal/engine/` | The Go port of the Core engine for the Linux daemon | **Required**: must pass `testdata/engine` |
| `internal/platform/`, `internal/daemon/`, `cmd/bulchimbeon/` | Linux backend (`/proc`, logind, sysfs, the desktop's suspend inhibitor), the daemon loop and the `bulchimbeon` command; a Windows stub | Unit tests with recorded fixtures |
| `internal/sprites/`, `internal/tray/`, `internal/dbus/` | The Linux tray icon: sprites drawn from `testdata/engine/sprites.json`, the StatusNotifierItem and its menu, a small standard-library D-Bus client | Pixel-for-pixel against the fixture; a fake bus; a real `dbus-daemon` when installed |
| `docs/` | Design notes (`design.md`), the [compatibility promise](docs/compatibility.md), media | |

Put decisions in Core with tests, and keep the app target a thin shell that calls the system.
The Swift Core is the behavior reference: change it first, update `testdata/engine` until `swift test` passes, then
make `internal/engine` pass the same fixtures.

### Ground rules

These match the checklist in the PR template.

| Rule | Why |
|------|-----|
| `make test` passes | CI runs the same tests |
| New logic (especially agent detection and duty decisions) lives in `BulchimbeonCore`, with tests | The app target can't be unit-tested |
| **No network access** of any kind, including telemetry and update checks | "No network" is a promise this app makes |
| **No third-party dependencies** | Keeps supply-chain risk at zero |
| Never store or print other processes' command lines or environment variables | They may contain other tools' API keys |
| Never put outside strings into the admin (`pmset`) command | Prevents shell injection |
| If you change security-relevant behavior, update the threat model in [`SECURITY.md`](SECURITY.md) | |
| Every user-facing string is written in Korean, English and Simplified Chinese: `L10n.t("한국어", "English", "中文")` | The app UI supports three languages. If you cannot provide a translation, use English for that argument and flag it in the PR |
| Settings keys, launch flags and the duty-log format follow the [compatibility promise](docs/compatibility.md) | Scripts that use them must not break |
| Tone of the soldier's lines: earnest and cute. **No hazing or bullying jokes** | Everyone should be able to laugh along |

### Adding an agent

1. Run the agent and look at the real process. **Measure, don't guess.**
   ```bash
   ps -Axo comm=,args= | grep -i <name>
   ```
   Native installs sometimes name the executable after the version number (Claude Code does).
2. Add it to `builtIn` in `Sources/BulchimbeonCore/AgentCatalog.swift`.
   - `binaryNames`: basename of the executable or `argv[0]` (case-sensitive)
   - `pathMarkers`: a fragment of the install path, for when the executable name is unusual
   - `packageMarkers`: a fragment of the script path, for when it runs under node, bun or deno
   - Don't match broad substrings. That causes false positives such as hook scripts.
3. Add a test to `Tests/BulchimbeonCoreTests/CoreTests.swift` using **the exact process shape you observed**, plus
   one false-positive case.
4. Add the same kind to `BuiltIn` in `internal/engine/catalog.go` and to `builtIn` and `match` in
   `testdata/engine/catalog.json`, so the Linux daemon detects it too (`go test ./...`).
5. Check with `--diagnose` (macOS) or `bulchimbeon status` (Linux, which lists the detected agents) that it's really
   detected, and paste that output into the PR.

Can't wait for a release? Users can add an agent locally with
`defaults write io.github.s3onghyun.bulchimbeon customAgents -array <name>` (macOS) or
`bulchimbeon config set customAgents <name>` (Linux).

### Sprites

The source of truth is the ASCII grids in `Sources/BulchimbeonCore/Sprites.swift`: one character per pixel, 16×16.
After editing, run `make icons` and commit the regenerated `Resources/AppIcon.icns` and `docs/sprites-preview.png`
with your change. A character that isn't in the palette fails the tests.
The Linux tray draws the same sprites from a fixture the Swift tests write: regenerate it with
`BULCHIMBEON_WRITE_SPRITES=1 swift test --filter SpriteFixtureTests`, copy it with
`cp testdata/engine/sprites.json internal/sprites/`, and commit both ([format](testdata/engine/README.md#spritesjson-the-privates-sprites)).

### Adding a uniform theme

1. In `Sources/BulchimbeonCore/UniformTheme.swift`, add a theme like `navy` or `arctic` and append it to `all`.
   The `id` is lowercase English (for example `desert`); it is stored as a setting value, so it can't change after a
   release. Give the name in Korean, English and Simplified Chinese (`nameKo`, `nameEn`, `nameZh`).
2. Only clothing and props change. `colors` may override only `themeableCharacters` (helmet `H h`, uniform `U u`,
   boots `B`, flashlight `L`, theme accents `A`), and `overlays` land only on clothing pixels. The face, skin and
   outline stay the same for everyone.
3. Themes, not countries: **no national flags, real rank insignia or unit patches, and no real army's actual uniform.**
4. Run `make test`. `UniformThemeTests` checks the theme rules, every pose and frame size, and that the default theme is
   unchanged. On macOS, run `make icons` and commit the new theme row in `docs/sprites-preview.png`.
   Regenerate the sprite fixture for the Linux tray as described under [Sprites](#sprites), then run `go test ./...`.

### Translations

| File | Language | Notes |
|------|----------|-------|
| `README.md` | English | The main README |
| `README.ko.md` | Korean | Maintained alongside the English one |
| `README.ja.md`, `README.zh-CN.md` | Japanese, Simplified Chinese | AI-translated summaries. Native speakers' fixes are very welcome |

- To fix wording, edit the file directly and open a PR. Small PRs are best.
- When a section is missing from the Japanese or Chinese README, translate it from `README.md`.
- Keep the language links at the top of each README and the image paths (`docs/media/demo.gif`, `docs/sprites-preview.png`) unchanged.
- The app UI supports Korean, English and Simplified Chinese. Chinese locales use Simplified Chinese; other unsupported system languages fall back to English. Adding another UI language is
  a bigger change, so please open an issue first.

### AI-assisted contributions

AI tools may be used as an aid. Please keep each pull request focused and review, understand, test, and maintain
every change you submit. If AI materially helped write code, tests, or documentation, disclose that in the
pull-request template.

Do not submit bulk-generated or unreviewed changes, copied material with an unknown license, or a change whose behavior
you cannot explain. By submitting a contribution, you confirm that you have the right to contribute it under this
repository's MIT license. Maintainers may ask for a smaller PR, reproduction, tests, or provenance.

### Releases (maintainer only)

```bash
git tag vX.Y.Z && git push origin vX.Y.Z   # CI builds universal, signs, attests, and publishes the Release
./scripts/bump-cask.sh X.Y.Z               # verifies checksum and provenance, then updates the Homebrew tap
```

### Be kind

Be respectful and patient with everyone, whatever language they write in. Harassment isn't tolerated. The maintainer
may remove comments or block accounts that make the project unwelcoming.

---

## 한국어

### 어디서부터?

| 하고 싶은 것 | 방법 |
|-------------|------|
| 작은 일부터 해보고 싶다 | [`good first issue`](https://github.com/s3onghyun/bulchimbeon/labels/good%20first%20issue)·[`help wanted`](https://github.com/s3onghyun/bulchimbeon/labels/help%20wanted) 라벨을 보세요. 겹치지 않게 "제가 할게요" 댓글을 남겨주세요 |
| 이등병이 이상하게 굴었다 | [🚨 근무 태도 불량 신고](https://github.com/s3onghyun/bulchimbeon/issues/new?template=bug-report.yml) |
| 내 에이전트가 감지 안 된다 | [🤖 새 에이전트 감지 요청](https://github.com/s3onghyun/bulchimbeon/issues/new?template=agent-request.yml) — 또는 직접 PR ([아래](#에이전트-추가하기)) |
| 기능·대사·스프라이트·스킨 아이디어 | [💡 기능 제안](https://github.com/s3onghyun/bulchimbeon/issues/new?template=feature-request.yml) |
| 번역이 어색하다 | [🌐 번역 수정](https://github.com/s3onghyun/bulchimbeon/issues/new?template=translation.yml) — 또는 직접 PR ([아래](#번역)) |
| 보안 문제 | 공개 이슈 말고 [비공개 제보](https://github.com/s3onghyun/bulchimbeon/security/advisories/new) |

모르는 게 있으면 이슈로 물어보세요. 질문도 좋은 이슈입니다.
작은 수정보다 큰 변경은 코드를 쓰기 전에 이슈로 방향을 먼저 맞춰주세요.

### 개발 환경

macOS 13 이상과 Xcode Command Line Tools (`xcode-select --install`) 만 있으면 됩니다. Xcode 는 필요 없습니다.
서드파티 의존성이 없어서 따로 설치할 것도 없습니다.

```bash
make build      # swift build
make test       # 단위·시나리오 테스트 (Command Line Tools 만으로 동작)
make app        # dist/Bulchimbeon.app (ad-hoc 서명)
make run        # 앱 빌드 후 실행
make icons      # 스프라이트 수정 후 아이콘·미리보기 재생성

dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose   # 에이전트 감지 진단 (무엇이 왜 잡히는지)
dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --demo       # README 녹화용 연출
BULCHIMBEON_DEBUG=1 dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon   # 엔진이 내리는 명령을 stderr 로 출력
```

Xcode 가 설치돼 있으면 `swift test` 만으로도 됩니다. `make test` 는 Command Line Tools 의 테스트 프레임워크에 필요한
플래그를 붙여줍니다. CI 는 PR 마다 macOS 에서 `swift test`, 앱 번들 빌드, 서명 검증을 돌립니다.

리눅스 데몬은 저장소 루트의 Go 모듈입니다 (`cmd/bulchimbeon`, `internal/`). Go 1.27 이상만 있으면 되고, 다른 의존성은
없습니다. 어느 OS 에서든 빌드·테스트되며 리눅스 백엔드 테스트는 리눅스에서 돕니다:

```bash
go test ./...                     # 엔진, 공유 픽스처, 리눅스 백엔드, 데몬, CLI
go build ./cmd/bulchimbeon        # 그다음 ./bulchimbeon status, doctor
./scripts/install-linux.sh ./bulchimbeon   # systemd 사용자 서비스로 설치 (리눅스)
BULCHIMBEON_TEST_LOGIND=1 go test ./internal/platform/linux/   # 실제 logind 잠금까지 시험
```

CI 는 이것을 `go (linux)` 잡에서 돌립니다.

### 구조

| 위치 | 내용 | 테스트 |
|------|------|--------|
| `Sources/BulchimbeonCore/` | 순수 로직 — 에이전트 판별, 근무 상태 머신(`DutyEngine`), 대사, 스프라이트, 근무일지, 설정 키 | **필수** |
| `Sources/Bulchimbeon/` | 시스템 호출(IOKit·libproc·`pmset`)과 UI | 실제 실행으로 확인 |
| `Sources/IconGen/` | 스프라이트 → 앱 아이콘·미리보기 PNG | — |
| `Tests/BulchimbeonCoreTests/` | Core 타깃의 Swift Testing 테스트 (가짜 시계로 도는 시나리오 테스트 포함) | |
| `testdata/engine/` | Swift 테스트와 Go 테스트가 함께 돌리는 골든 픽스처 ([형식](testdata/engine/README.md)) | 필수 |
| `internal/engine/` | 리눅스 데몬용 Go 엔진 (Core 엔진을 옮긴 것) | **필수**: `testdata/engine` 통과 |
| `internal/platform/`, `internal/daemon/`, `cmd/bulchimbeon/` | 리눅스 백엔드(`/proc`·logind·sysfs·데스크톱 절전 억제), 데몬 루프, `bulchimbeon` 명령, Windows 자리 | 기록된 픽스처로 단위 테스트 |
| `internal/sprites/`, `internal/tray/`, `internal/dbus/` | 리눅스 트레이 아이콘: `testdata/engine/sprites.json` 으로 그리는 스프라이트, StatusNotifierItem 과 메뉴, 표준 라이브러리로 만든 작은 D-Bus 클라이언트 | 픽스처와 픽셀 단위 비교, 가짜 버스, 설치돼 있으면 실제 `dbus-daemon` |
| `docs/` | 설계서(`design.md`), [호환성 약속](docs/compatibility.md), 미디어 | |

판단 로직은 Core 에 두고 테스트를 붙여주세요. 앱 타깃은 시스템을 호출하는 얇은 껍데기로 유지합니다.
동작의 기준은 Swift Core 입니다: Core 를 먼저 고치고, `swift test` 가 통과하도록 `testdata/engine` 을 갱신한 뒤,
`internal/engine` 이 같은 픽스처를 통과하게 맞춥니다.

### 지켜주세요

PR 템플릿의 체크리스트와 같습니다.

| 원칙 | 이유 |
|------|------|
| `make test` 통과 | CI 가 같은 테스트를 돌립니다 |
| 새 로직(특히 에이전트 감지·근무 판단)은 `BulchimbeonCore` 에 두고 테스트 추가 | 앱 타깃은 단위 테스트를 할 수 없습니다 |
| **네트워크 통신 금지** (텔레메트리·업데이트 확인 포함) | "통신 없음"이 이 앱의 약속입니다 |
| **서드파티 의존성 추가 금지** | 공급망 리스크 0 유지 |
| 프로세스 명령줄·환경변수를 저장·출력하지 않기 | 다른 프로세스의 API 키가 들어 있을 수 있음 |
| 관리자 권한 명령(`pmset`)에 외부 문자열을 넣지 않기 | 셸 인젝션 방지 |
| 보안 동작을 바꾸면 [`SECURITY.md`](SECURITY.md) 위협 모델도 갱신 | |
| 사용자 문구는 한국어·영어·중국어 간체 모두 (`L10n.t("한국어", "English", "中文")`) | 앱 UI 는 3개 언어를 지원합니다. 번역이 어려운 언어는 영어를 넣고 PR 에 적어주세요 |
| 설정 키·실행 인자·근무일지 형식은 [호환성 약속](docs/compatibility.md)을 따르기 | 이것들을 쓰는 스크립트가 깨지면 안 됩니다 |
| 대사 톤: 성실하고 귀엽게. **갈굼·부조리 개그는 넣지 않습니다** | 모두가 웃을 수 있게 |

### 에이전트 추가하기

1. 에이전트를 실행해 두고 실제 프로세스 모습을 확인합니다. **추측 말고 실측으로.**
   ```bash
   ps -Axo comm=,args= | grep -i <name>
   ```
   네이티브 설치본은 실행 파일 이름이 버전 번호인 경우도 있습니다 (Claude Code 가 그렇습니다).
2. `Sources/BulchimbeonCore/AgentCatalog.swift` 의 `builtIn` 에 추가합니다.
   - `binaryNames`: 실행 파일·argv[0] basename (대소문자 구분)
   - `pathMarkers`: 실행 파일 이름이 특이할 때 설치 경로 조각
   - `packageMarkers`: node/bun/deno 로 실행될 때 스크립트 경로 조각
   - 부분 문자열로 넓게 잡지 마세요. hook 스크립트 같은 오탐이 생깁니다.
3. `Tests/BulchimbeonCoreTests/CoreTests.swift` 에 **실측한 프로세스 모습 그대로** 테스트를 추가합니다. 오탐 케이스도 하나 넣어주세요.
4. 같은 종류를 `internal/engine/catalog.go` 의 `BuiltIn` 과 `testdata/engine/catalog.json` 의 `builtIn`·`match` 에도 추가해
   리눅스 데몬도 감지하게 합니다 (`go test ./...`).
5. `--diagnose`(macOS) 또는 `bulchimbeon status`(리눅스, 감지된 에이전트를 보여줌)로 실제로 잡히는지 확인하고, 그 출력을 PR 에 붙여주세요.

릴리스를 기다리기 어렵다면 사용자가 직접 추가할 수도 있습니다:
`defaults write io.github.s3onghyun.bulchimbeon customAgents -array <name>` (macOS),
`bulchimbeon config set customAgents <name>` (리눅스)

### 스프라이트

`Sources/BulchimbeonCore/Sprites.swift` 의 ASCII 그리드가 원본입니다. 문자 하나 = 픽셀 하나, 16×16.
수정 후 `make icons` 를 돌리고 다시 생성된 `Resources/AppIcon.icns`·`docs/sprites-preview.png` 를 함께 커밋해주세요.
팔레트에 없는 문자는 테스트에서 걸립니다.
리눅스 트레이는 Swift 테스트가 써 주는 픽스처로 같은 스프라이트를 그립니다. `BULCHIMBEON_WRITE_SPRITES=1 swift test --filter SpriteFixtureTests`
로 다시 만들고 `cp testdata/engine/sprites.json internal/sprites/` 로 복사한 뒤 둘 다 커밋해주세요 ([형식](testdata/engine/README.md#spritesjson-the-privates-sprites)).

### 군복 테마 추가하기

1. `Sources/BulchimbeonCore/UniformTheme.swift` 에 `navy`·`arctic` 처럼 테마를 하나 만들고 `all` 끝에 넣습니다.
   `id` 는 소문자 영어 (예: `desert`) — 설정 값으로 저장되므로 출시 후에는 바꿀 수 없습니다. 이름은 한국어·영어·중국어 간체 모두 (`nameKo`, `nameEn`, `nameZh`).
2. 바꿀 수 있는 건 옷과 소품뿐입니다. `colors` 는 `themeableCharacters` (방탄모 `H h`, 전투복 `U u`, 전투화 `B`,
   손전등 `L`, 테마 장식 `A`) 만 덮어쓸 수 있고, `overlays` 는 옷 픽셀 위에만 찍힙니다. 얼굴·피부·외곽선은 그대로입니다.
3. 나라가 아니라 테마입니다. **국기, 실제 계급장·부대 마크, 특정 군대의 실제 군복은 넣지 않습니다.**
4. `make test` 를 돌립니다. 테마 규칙, 모든 포즈·프레임의 크기, 기본 테마가 그대로인지를 `UniformThemeTests` 가 확인합니다.
   macOS 에서 `make icons` 를 돌려 `docs/sprites-preview.png` 에 새 테마 줄이 생긴 것을 같이 커밋해주세요.
   [스프라이트](#스프라이트)에 적힌 대로 리눅스 트레이용 스프라이트 픽스처도 다시 만들고 `go test ./...` 를 돌립니다.

### 번역

| 파일 | 언어 | 비고 |
|------|------|------|
| `README.md` | 영어 | 대표 README |
| `README.ko.md` | 한국어 | 영어판과 함께 관리 |
| `README.ja.md`, `README.zh-CN.md` | 일본어, 중국어(간체) | AI 번역 요약본. 원어민의 수정을 환영합니다 |

- 표현을 고치려면 파일을 직접 고쳐 PR 을 보내주세요. 작은 PR 이 좋습니다.
- 일본어·중국어 README 에 빠진 섹션이 있으면 `README.md` 에서 번역해 넣어주세요.
- 각 README 맨 위의 언어 링크와 이미지 경로(`docs/media/demo.gif`, `docs/sprites-preview.png`)는 그대로 둬주세요.
- 앱 UI 는 한국어·영어·중국어 간체를 지원합니다 (중국어 로캘은 간체로, 그 외 미지원 시스템 언어는 영어로 표시). UI 언어 추가는 큰 변경이라 이슈로 먼저 의논해주세요.

### AI 도움을 받은 기여

AI 도구를 보조로 써도 됩니다. PR 하나는 한 가지 문제에 집중하고, 제출하는 모든 변경을 직접 검토·이해·테스트하고
이후에도 책임져 주세요. AI 가 코드·테스트·문서 작성에 실질적으로 도움을 줬다면 PR 템플릿에 밝혀주세요.

대량 생성했거나 검토하지 않은 변경, 라이선스를 알 수 없는 복사물, 동작을 설명할 수 없는 변경은 보내지 마세요.
기여를 제출하면 그 내용을 이 저장소의 MIT 라이선스로 기여할 권리가 있다고 확인하는 것입니다. 메인테이너는 더 작은 PR,
재현 방법, 테스트, 출처를 요청할 수 있습니다.

### 릴리스 (메인테이너 전용)

```bash
git tag vX.Y.Z && git push origin vX.Y.Z   # CI 가 universal 빌드·서명·출처 증명·Release
./scripts/bump-cask.sh X.Y.Z               # 체크섬·출처 증명 검증 후 Homebrew tap 갱신
```

### 서로 존중해주세요

어떤 언어로 쓰든 모두를 존중하고 너그럽게 대해주세요. 괴롭힘은 허용하지 않습니다. 프로젝트 분위기를 해치는 댓글은
메인테이너가 지우거나 계정을 차단할 수 있습니다.
