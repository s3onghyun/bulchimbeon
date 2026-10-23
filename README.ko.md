# 🪖 불침번 (Bulchimbeon)

> **Your Mac's night-watch soldier.**
> 내가 자는 동안 AI 에이전트 곁에서 맥을 깨워두고, 일이 끝나면 "근무 교대합니다!" 하고 재워주는 메뉴바 이등병.
>
> **hook도, 데몬도, 설치되는 root 도우미도 없이 앱 하나로. macOS 13부터.**
> 리눅스에서는 명령줄과 트레이 아이콘이 있는 작은 데몬이 같은 일을 합니다: [Linux](#linux) 참고.

> 🌐 [English](README.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md)
> 💡 [Ideas / 문의 / 建议](https://github.com/s3onghyun/bulchimbeon/discussions)

![불침번 데모](docs/media/demo.gif)

| 상태 | 이등병 | 언제 |
|------|--------|------|
| 💤 취침 | 방탄모 쓰고 꾸벅, zzz | 근무 없음 |
| 👀 근무 대기 | 두리번두리번 | 에이전트 출근 감시 중 |
| 🪖 근무 중 | 손전등 깜빡이며 경계 | 맥이 잠들지 않음 |
| 🫡 근무 교대 | `^ ^` 충성! 경례 | 에이전트 작업 끝 / 근무 시간 종료 |
| 🥵 전투휴식 | 땀 뻘뻘, 눈 감고 열 식히기 | 폭염 경보 |

### 🚶 순찰 모드

<img src="docs/sprites-preview.png" width="360" align="right" alt="스프라이트">

메뉴바 바로 아래, 화면 맨 위를 이등병이 손전등을 들고 좌우로 순찰한다.
근무 대기 중엔 오른쪽 구석에서 두리번, 교대 땐 그 자리에서 경례하며 말풍선으로 "근무 교대합니다! 🫡". 취침 중엔 퇴근(숨김).
클릭은 모두 통과하고, 마우스가 다가오면 반투명해져 비켜준다. 메뉴에서 끄기·크기(2배/3배) 조절.
전체화면 앱(발표·화면 공유·영상) 위에는 나타나지 않는다.

### 🗣️ 이등병 어록

| 언제 | 이등병 |
|------|--------|
| 근무 중 | "졸지 않겠습니다! …아마도요." · "편히 주무십시오. 제가 지키겠습니다!" |
| 대기 중 | "총 닦으며 대기 중…" |
| 맥이 뜨거울 때 | "폭염 주의보! 물 마시는 중 🥵" → "전투휴식! 열 식히는 중 🧊" |
| 배터리 부족 | "당직사관님께 보고드립니다 🔋 배터리 15%… 기력이 다해 근무 철수합니다." |
| 잠자기 방지가 풀렸을 때 | "근무지 이탈 아닙니다! 복귀! 🫡" |
| 자리 비웠다 돌아오면 | "인수인계 드리겠습니다! 📋" |

## 왜 만들었나

AI 에이전트를 돌려놓고 자려면 이렇게 했다.

```bash
caffeinate -d                 # 끄는 걸 잊는다
sudo pmset disablesleep 1     # 되돌리는 걸 잊으면 배터리 0%
```

불침번은 **에이전트가 떠 있는 동안만** 깨어 있고, 끝나면 알아서 근무를 교대한다.

## 기능

| 기능 | 설명 |
|------|------|
| 🤖 에이전트 근무 (자동 교대) | Claude Code · Codex · Gemini CLI · Cursor · Copilot CLI · Aider · OpenCode 감지 (+사용자 지정). **열려만 있는 세션은 세지 않고 실제로 일할 때만 근무**, 조용해지고 5분 뒤 교대 → 있으면 근무 투입, 모두 끝나면 교대 알림 |
| 🪖 근무 서기 | 30분 / 1시간 / 2시간 / 4시간 / 무기한 |
| 근무 형태 | 화면 켜둠 (`caffeinate -d`) · 화면은 꺼져도 맥은 깨어 있음 (`caffeinate -i`) · **덮개 닫아도 깨어 있음** (관리자 권한) |
| 🔋 배터리 철수 | 배터리 사용 중 잔량이 기준(기본 20%) 이하면 강제 철수 |
| ⚠️ 덮개 안전장치 | 근무 종료·앱 종료 시 `disablesleep` 원복, 비정상 종료 후 재실행하면 복구 버튼 |
| 🌡️ 폭염 주의보 → 🔥 전투휴식 | 맥이 뜨거워지면 경고, 아주 뜨거우면 근무를 잠시 멈춰 식히고 **열이 내리면 자동 복귀** (덮개 닫고 가방에 넣는 사고 방지) |
| ⚠️ 근무지 이탈 감지 | 잠자기 방지가 어떤 이유로든 풀려 있으면 즉시 재투입 |
| 🕵️ 당직 감시병 | 덮개 모드 중 앱이 강제 종료돼도 5초 안에 원래 잠자기 설정으로 복구 |
| 📋 당직 근무 인수인계 | 10분 이상 자리를 비웠다 돌아오면 그사이 근무 내역 보고 ("근무 교대 1회 · 폭염 1회 … 이상입니다! 🫡") |
| 🖥️ 다른 앱 표시 | 이 앱 말고 잠자기·화면 꺼짐을 막는 앱도 메뉴에 표시 (예: "Vivaldi (영상)") |
| 🔁 근무 유지 | 앱 재시작·업데이트·재부팅 후에도 남은 근무 시간 그대로 복귀 (덮개 근무는 보안상 시스템 근무로 복귀 후 재승인 안내) |
| 🌐 한국어·영어·중국어(간체) | 기본은 시스템 언어. 메뉴 **언어 / Language / 语言** 에서 따로 고를 수 있음 (시스템은 영어인데 앱은 한국어로 등) |
| 📒 근무일지 | `~/Library/Application Support/Bulchimbeon/duty-log.txt` |

## 비슷한 앱과 비교

같은 문제를 푸는 앱으로 [Adrafinil](https://github.com/kageroumado/adrafinil)이 먼저 나와 있다. 접근이 반대라서 맞는 쪽을 고르면 된다. (2026-10 기준)

| | 불침번 | Adrafinil |
|---|---|---|
| 일하는 중인지 아는 법 | **밖에서 관찰** — 에이전트 프로세스의 `caffeinate` 자식·CPU 사용량 | **에이전트가 직접 알림** — hook(프롬프트 제출 시 획득, 응답 끝나면 해제) + 관찰 |
| 설치하면 바뀌는 것 | **없음.** 앱 하나, 에이전트 설정 파일을 건드리지 않음 | 에이전트 설정에 hook 추가(원클릭 설치·제거), 사용자 데몬, root 도우미 |
| macOS | **13 이상** | 26.4 이상 |
| 덮개 닫고 깨어 있기 | 켤 때마다 관리자 암호, 감시병이 원상 복구 보장 | 설치 때 한 번 승인, 이후 암호 없음 |
| 에이전트가 직접 근무 요청 (긴 빌드 등) | — | CLI·MCP 도구 |
| 발열 | 덮개 열려 있어도 경고, 너무 뜨거우면 쉬었다가 **식으면 자동 복귀** | 덮개 닫혔을 때 과열 시 강제 해제 |
| 배포 | ad-hoc 서명 (공증 준비 중), 개인 Homebrew tap | 공증, Homebrew 공식 cask |

**이럴 땐 Adrafinil**: macOS 26.4 이상이고, hook으로 정확하게 맞물리거나 에이전트가 직접 근무를 요청하게 하고 싶을 때.
**이럴 땐 불침번**: macOS 13~15를 쓰거나, 에이전트 설정·시스템에 아무것도 설치하고 싶지 않거나, 이등병이 지켜주는 게 좋을 때.

## 설치

```bash
brew install --cask s3onghyun/tap/bulchimbeon

# 업데이트 — tap 을 먼저 새로 받아야 새 버전이 보인다 (Homebrew 자동 갱신 주기가 길다)
brew update && brew upgrade --cask bulchimbeon
```

또는 [Releases](https://github.com/s3onghyun/bulchimbeon/releases) 에서 zip 을 받아 `/Applications` 로 옮긴다. (macOS 13+, Apple Silicon·Intel universal)

> **첫 실행:** 아직 Apple 공증(notarization)을 받지 않은 ad-hoc 서명 앱이라 macOS 가 실행을 막는다.
> 한 번 열기를 시도한 뒤 **시스템 설정 → 개인정보 보호 및 보안 → "그래도 열기"** 를 누르면 된다.
> (macOS 15 부터는 우클릭 → 열기로 우회되지 않는다.)
> 공증 전에는 macOS 가 알림 권한을 주지 않아서, 알림이 **"스크립트 편집기"** 이름으로 표시된다.

### 받은 파일 검증 (선택)

릴리스 zip 은 GitHub Actions 에서 빌드되고 빌드 출처 증명(attestation)이 붙는다.

```bash
gh attestation verify Bulchimbeon-<버전>.zip --repo s3onghyun/bulchimbeon
shasum -a 256 -c Bulchimbeon-<버전>.zip.sha256
```

## 개발

Xcode 없이 Command Line Tools 만 있으면 된다. 리눅스 데몬은 Go 모듈이다 (Go 1.27 이상): `go test ./...`, `go build ./cmd/bulchimbeon`.

```bash
make app        # dist/Bulchimbeon.app 생성 (ad-hoc 서명)
make run        # 빌드 후 실행
make test       # 단위·시나리오 테스트
make icons      # 스프라이트 수정 후 앱 아이콘·미리보기 재생성

# 실행과 동시에 에이전트 근무 시작
open dist/Bulchimbeon.app --args --agent-watch

# 엔진이 내리는 명령을 stderr 로 출력 (추적용)
BULCHIMBEON_DEBUG=1 dist/Bulchimbeon.app/Contents/MacOS/Bulchimbeon
```

### 릴리스

```bash
git tag vX.Y.Z && git push origin vX.Y.Z   # → Release 워크플로가 빌드·증명·업로드
./scripts/bump-cask.sh X.Y.Z               # → 체크섬·출처 증명 검증 후 tap 의 cask·리눅스 formula 갱신
```

## 동작 원리

| 근무 형태 | 내부 동작 |
|-----------|----------|
| 화면 켜둠 | `IOPMAssertion` `PreventUserIdleDisplaySleep` — `caffeinate -d` 와 같음 |
| 화면은 꺼져도 맥은 깨어 있음 | `IOPMAssertion` `PreventUserIdleSystemSleep` — `caffeinate -i` 와 같음 (기본값, 절전) |
| 덮개 닫아도 깨어 있음 | 위 + `pmset -a disablesleep 1` (덮개 닫힘은 하드웨어 트리거라 assertion 으로 못 막음) |

### 에이전트 감지

현재 사용자의 프로세스를 libproc 으로 훑어 아래 신호 중 하나라도 맞으면 에이전트로 본다.
부분 문자열 매칭은 하지 않는다 (Claude Code hook 의 `node -e "...'.claude'..."` 를 오탐하므로).

| 설치 방식 | 실제 프로세스 모습 | 판별 근거 |
|-----------|-------------------|-----------|
| Claude Code 네이티브 설치 | 실행 파일 `~/.local/share/claude/versions/2.1.296` (이름이 버전 번호!) | `argv[0]`=`claude` 또는 설치 경로 |
| Homebrew·IDE 확장 내장 바이너리 | `.../claude`, `/opt/homebrew/bin/codex` | 실행 파일 이름 |
| npm 전역 설치 | `node /opt/homebrew/bin/claude` | 스크립트 이름 |
| npx·패키지 경로 실행 | `node .../@anthropic-ai/claude-code/cli.js` | 패키지 경로 |
| bun·deno | `bun .../@google/gemini-cli/...` | 패키지 경로 |

지원: Claude Code · Codex · Gemini CLI · Cursor Agent · Copilot CLI · Aider · OpenCode

**일할 때만 근무.** Claude Code 창을 여러 개 열어 두는 게 보통이라, 열려 있기만 한 세션까지 세면 교대가 영원히 일어나지 않는다.
그래서 "작업 중"인 에이전트만 센다 (메뉴에서 예전 방식으로 바꿀 수 있음).

| 신호 | 근거 (실측) |
|------|------------|
| 자식 프로세스로 `caffeinate` 를 띄움 | Claude Code 는 작업하는 동안만 스스로 `caffeinate -i -t 300` 을 띄운다 |
| 자신 + 자손 프로세스 CPU 합 ≥ 2% | 빌드·테스트 같은 도구 실행 포함. 입력 대기 세션은 0~0.1% |
| `--tools=` (도구 없는 헤드리스 세션) 은 제외 | claude-mem 같은 플러그인이 상시 띄워 두는 도우미 세션 |

내 환경에서 제대로 잡히는지 확인하거나, 목록에 없는 에이전트를 추가하려면:

```bash
/Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose
defaults write io.github.s3onghyun.bulchimbeon customAgents -array goose amp
```

Docker 컨테이너나 SSH 원격 서버에서 도는 에이전트는 이 맥의 프로세스가 아니라서 잡히지 않는다.

```bash
pmset -g assertions | grep Bulchimbeon   # 근무 중인지 확인
```

스프라이트는 `Sources/BulchimbeonCore/Sprites.swift` 의 ASCII 그리드다. 문자 하나가 픽셀 하나이고,
메뉴바(1pt=1px)와 1024px 앱 아이콘을 같은 원본에서 벡터 사각형으로 그려 어느 크기에서도 선명하다.

근무 판단은 시스템 호출이 없는 상태 머신(`DutyEngine`)에 있고, 가짜 시계로 시나리오 테스트한다.
설계 상세 (검토했지만 넣지 않은 신호 포함): [docs/design.md](docs/design.md)

## 로드맵

[마일스톤](https://github.com/s3onghyun/bulchimbeon/milestones)으로 관리한다. 1.0 = 설정 키·실행 인자·근무일지 형식이 더 이상 바뀌지 않는다는 약속 ([호환성 약속](docs/compatibility.md), 변경 내역은 [CHANGELOG](CHANGELOG.md)).

- [x] 치비 이등병 픽셀아트, 순찰 모드, 한국어·영어·중국어(간체)
- [x] 에이전트 **작업 중 / 대기 구분** (열려만 있는 세션은 세지 않음)
- [x] Homebrew cask (개인 tap)
- [ ] Apple 공증 + Homebrew 공식 cask (Developer ID 필요)
- [ ] Privileged helper 로 덮개 모드 암호 프롬프트 제거
- [ ] 해금 스킨: 순라꾼(조선시대 불침번), 선비(현량자고)

## Linux

리눅스에서는 명령줄과 [트레이 아이콘](#트레이-아이콘)이 있는 작은 데몬으로 돕니다.
엔진은 맥 앱과 같습니다. Go 로 옮긴 엔진이 Swift Core 와 같은 골든 테스트([`testdata/engine`](testdata/engine))를
PR 마다 CI 에서 통과해야 합니다. Ubuntu 24.04 (systemd 255)에서 확인했고, systemd·logind 가 필요합니다.
근무 중에는 데스크톱 자체의 절전 억제로, 데스크톱이 없으면 logind 로 자동 절전을 막습니다([아래](#데스크톱별-잠자기-방지)).

### 설치

[리눅스용 Homebrew](https://docs.brew.sh/Homebrew-on-Linux)로 (root 불필요, `brew upgrade` 로 새 버전):

```bash
brew install s3onghyun/tap/bulchimbeon-linux
brew services start bulchimbeon-linux    # systemd 사용자 유닛, 로그인하면 시작
```

설치 방법은 하나만 고르세요. Homebrew 로 깔았다면 tarball 을 또 설치하거나 `.deb` 유닛을 켜지
(`systemctl --user enable bulchimbeon`) 마세요. 데몬이 두 개 돕니다.

또는 [Releases](https://github.com/s3onghyun/bulchimbeon/releases)에서 amd64·arm64 용 tarball 이나 `.deb` 를 받습니다.

```bash
# tarball: root 불필요. ~/.local/bin/bulchimbeon 과 systemd 사용자 유닛을 설치하고 시작
tar -xzf bulchimbeon_<버전>_linux_amd64.tar.gz
cd bulchimbeon_<버전>_linux_amd64 && ./install-linux.sh

# 또는 .deb: /usr/bin/bulchimbeon 과 사용자 유닛 설치. 쓰려는 사용자마다 켭니다
sudo apt install ./bulchimbeon_<버전>_amd64.deb
systemctl --user enable --now bulchimbeon
```

검증(선택): `sha256sum -c SHA256SUMS --ignore-missing`, `gh attestation verify <파일> --repo s3onghyun/bulchimbeon`.
제거: `brew services stop bulchimbeon-linux && brew uninstall bulchimbeon-linux`, `./install-linux.sh --uninstall`, 또는 `systemctl --user disable --now bulchimbeon` 후 `sudo apt remove bulchimbeon`.
설정(`~/.config/bulchimbeon`)과 근무일지(`~/.local/state/bulchimbeon`)는 남깁니다.
로그아웃한 동안에도 돌리려면 한 번 `loginctl enable-linger`.

### 사용

```bash
bulchimbeon status                       # 지금 근무 중인지, 왜 그런지
bulchimbeon doctor                       # 데스크톱 세션·systemd·logind·polkit 점검
bulchimbeon config set agentGraceMinutes 10
bulchimbeon config set customAgents goose amp
bulchimbeon config set language ko       # ko | en | zh | system (기본: LANG 따름)
journalctl --user -u bulchimbeon -f      # 데몬 로그
```

`bulchimbeon run` 이 데몬이고 사용자 유닛이 이것을 실행합니다. 항상 에이전트 근무입니다: 같은 감지 규칙,
5초 점검, 에이전트 프로세스 트리가 CPU 2% 이상(60초 평균, 리눅스에는 `caffeinate` 신호가 없음)이면 투입,
유예 시간 뒤 교대.

### 트레이 아이콘

`bulchimbeon tray` 는 데스크톱 패널에 이등병을 세웁니다. 맥 메뉴바와 같은 자세로 같은 속도로 움직입니다: 데몬이 꺼져
있으면 취침, 에이전트를 기다리면 두리번, 근무 중이면 손전등, 전투휴식이면 땀 뻘뻘. 마우스를 올리면 상태 전체가 보입니다.
메뉴에는 `bulchimbeon status` 의 모든 줄(같은 순서), `uniformTheme` 을 바꾸는 **🧥 군복** 선택,
그리고 **트레이 아이콘 닫기**가 있습니다. 닫아도 아이콘만 사라지고 데몬은 계속 근무합니다.

`.deb` 는 XDG 자동 시작 항목 `/etc/xdg/autostart/bulchimbeon-tray.desktop` 으로 데스크톱에 로그인할 때마다 띄웁니다.
`install-linux.sh` 는 같은 항목을 `~/.config/autostart/` 에 둡니다. Homebrew 로 깔았다면 데스크톱의 시작 프로그램에
`bulchimbeon tray` 를 추가하세요. 보여줄 곳이 없거나 (SSH, 헤드리스 기기, 트레이가 없는 데스크톱) 데몬을 한 번도 켜지 않은 사용자라면
한 줄만 남기고 끝납니다.

KDE·XFCE·GNOME AppIndicator 확장이 쓰는 트레이 규격 StatusNotifierItem 입니다:

| 데스크톱 | 필요한 것 |
|---|---|
| XFCE 4.16 이상 | 패널의 **상태 트레이(Status Tray)** 항목 (`systray`, 기본 패널에 있음) |
| XFCE 4.14 이하 | 패널 항목 `xfce4-statusnotifier-plugin` |
| GNOME | AppIndicator 확장 (`gnome-shell-extension-appindicator`, 우분투는 기본으로 켜져 있음) |
| KDE Plasma | 없음. 근무 중이 아니면(대기·취침) 항목이 Passive 라 Plasma 는 숨은 아이콘에 두고, 근무에 들어가면 패널에 보입니다 |

```bash
bulchimbeon tray &                          # 지금 이 데스크톱 세션에 띄우기
bulchimbeon tray --lang ko                  # 언어 설정과 관계없이 한국어 메뉴 (ko | en | zh)
bulchimbeon config set uniformTheme navy    # default | navy | arctic | desert: 군복 메뉴와 같음
```

로그인할 때 띄우지 않으려면 데스크톱의 세션·시작 프로그램 설정에서 끄거나 자동 시작 파일을 지우세요.

### 리눅스에서의 동작

| | 리눅스 |
|---|---|
| 에이전트 | `/proc` 의 내 프로세스만. 판별 규칙·개인정보 제한은 맥과 같음 |
| 잠자기 방지 | 데스크톱 세션의 절전 억제, 데스크톱이 없으면 logind `sleep` inhibitor ([데스크톱별](#데스크톱별-잠자기-방지)). idle 잠금은 잡지 않음. 데몬이 죽으면(`kill -9` 포함) 잠금도 함께 풀림 |
| 배터리 철수 | `/sys/class/power_supply` |
| 폭염 | hwmon(`coretemp`, `k10temp` 등) 또는 thermal zone: 위험 온도 15 °C 아래면 주의보, 5 °C 아래면 전투휴식 |
| 근무일지 | `~/.local/state/bulchimbeon/duty-log.txt` (맥과 같은 형식) |

### 데스크톱별 잠자기 방지

| 세션 | 근무 중 잠금 | 그대로 되는 것 |
|---|---|---|
| GNOME (Ubuntu, Fedora), Cinnamon, MATE, Budgie | `org.gnome.SessionManager.Inhibit`, 절전 플래그만 | 화면이 어두워지고 잠김, 메뉴로 내가 하는 절전. GNOME 46 (Ubuntu 24.04)은 절전 억제가 있으면 자기 logind block 잠금도 잡아서, 터미널의 `systemctl suspend` 는 `-i` 를 요구합니다 |
| KDE Plasma | `org.freedesktop.PowerManagement.Inhibit` (PowerDevil 은 자동 절전에만 적용) | 화면 어두워짐·꺼짐·잠금, 수동 절전 |
| XFCE | 기본은 데스크톱이 없을 때처럼 logind `sleep` inhibitor. systemd 256 이하에서는 xfce4-power-manager 의 자동 절전을 **막지 못합니다**. `bulchimbeon config set xfceKeepAwake true` 로 `org.freedesktop.PowerManagement.Inhibit` 를 씀 | 화면 꺼짐·잠금, 수동 절전. `xfceKeepAwake` 를 켜면 자동 절전도 막지만, XFCE 전원 관리자는 어떤 억제든 프레젠테이션 모드처럼 다뤄서 근무 중에는 화면이 켜진 채 잠기지 않습니다 |
| 데스크톱 없음: 헤드리스, SSH 만, sway, Hyprland 등 컴포지터 | logind `sleep` inhibitor. systemd 257 이상은 `block-weak`, 256 이하는 `block`. 파이프에 묶인 `systemd-inhibit` 자식이 쥠 | 내가 하는 절전. logind 의 `IdleAction` 과 다른 사용자의 절전 요청은 막습니다. systemd 256 이하에서는 터미널의 `systemctl suspend` 가 `-i` 를 요구합니다 |

logind 만 쓰지 않는 이유: systemd 256 이하(Ubuntu 24.04, Debian 12)의 logind 는 잠금을 쥔 사용자 본인의 요청에는
block inhibitor 를 무시하고, GNOME·XFCE·Cinnamon 의 전원 관리자는 사용자로 logind 에 절전을 요청합니다.
Ubuntu 24.04 GNOME 에서 실측: 불침번이 logind 잠금을 쥐고 있어도 gsd-power 의 자동 절전 요청이 logind 를 통과했습니다.
systemd 257 이상에서는 logind `block` 잠금이 이를 막지만, 내가 직접 하는 절전까지 거부합니다.

데몬은 세션 버스, 즉 `systemd --user` 가 서비스에 주는 사용자 버스에서 데스크톱을 찾습니다. 근무 중에 데스크톱에
로그인하면 다음 점검(5초) 안에 잠금이 데스크톱으로, 로그아웃하면 logind 로 옮겨 갑니다. 데몬의 버스 연결이 끊기면
데스크톱이 억제를 풀기 때문에, 데몬이 죽어도 잠금이 남지 않습니다. `bulchimbeon status` 가 어떤 잠금을 왜 쥐었는지
보여주고, `bulchimbeon doctor` 가 하나씩 시험합니다.

### 한계

- **덮개를 닫으면 여전히 잠듭니다.** logind 는 기본값(`LidSwitchIgnoreInhibited=yes`)으로 덮개에는 inhibitor 를 무시하고,
  불침번은 logind 설정을 바꾸지 않습니다.
- **SSH·헤드리스 기기에서 잠자기 차단에는 polkit 규칙이 필요합니다.** polkit 은 기본적으로 로컬 로그인(화면 앞)에서만
  `sleep` 차단을 허용합니다. SSH 세션이나, 아무도 로컬로 로그인하지 않는 기기의 사용자 서비스에서는 거부됩니다.
  그러면 `bulchimbeon status` 가 잠자기는 막지 못한다고 분명히 알려줍니다.
  한 사용자에게만 허용하려면 위 [English](README.md#limits) 섹션의 규칙을 사용자 이름을 넣어
  `/etc/polkit-1/rules.d/50-bulchimbeon.rules` (root 소유, `0644`)로 저장하세요. `bulchimbeon doctor` 가 이름을 채워 출력합니다.
  `org.freedesktop.login1.inhibit-block-sleep` 하나만 허용하며, 파일을 지우면 원래대로 돌아갑니다. 불침번이 대신 실행하지 않습니다.
  데스크톱 세션은 자체 절전 억제를 쓰므로 polkit 권한이 필요 없습니다. 데스크톱이 없을 때 쓰는 logind 잠금에만 필요합니다.
- **sway, Hyprland 처럼 데스크톱 전원 관리자가 없는 컴포지터는 logind 잠금만 받습니다.** 사용자로 절전하는 idle 데몬
  (`swayidle`, `hypridle` 이 실행하는 `systemctl suspend`)은 막지 못합니다. logind 가 본인의 절전은 통과시키기 때문입니다.
  그렇다고 idle 잠금을 잡으면 이 컴포지터들에서는 화면 잠금까지 멈추므로 잡지 않습니다.
- **XFCE 에서는 기본적으로 자동 절전을 막지 않습니다.** xfce4-power-manager 는 어떤 절전 억제든 프레젠테이션 모드처럼
  다뤄 화면을 켜 두고 잠그지 않으므로, 불침번은 화면 잠금을 건드리지 않고 logind 잠금만 쥡니다. systemd 256 이하에서는
  xfce4-power-manager 의 자동 절전이 이 잠금을 통과합니다. `bulchimbeon status` 와 `doctor` 가 그렇게 알립니다.
  근무 중 화면이 켜진 채 잠기지 않는 것을 감수하고 막으려면 `bulchimbeon config set xfceKeepAwake true`.
- **데스크톱 억제는 내 세션만 지킵니다.** 같은 기기의 다른 사용자 세션(빠른 사용자 전환)은 여전히 기기를 재울 수
  있습니다. 세션 버스가 따로인 데스크톱(`dbus-user-session` 없이 `dbus-launch` 로 시작)은 서비스가 볼 수 없어 logind 잠금을 유지합니다.
- 리눅스에는 "화면은 꺼져도 깨어 있음" 근무 형태만 있고(`dutyMode` 무시), 순찰 모드는 없으며, 알림은 저널로 갑니다.
- Windows 는 계획 중입니다: 플랫폼 계층은 [`internal/platform`](internal/platform).

## 보안

관리자 권한 `pmset` 실행과 프로세스 명령줄 읽기를 다루는 앱이라 위협 모델을 따로 정리했다 → [SECURITY.md](SECURITY.md)

- 네트워크 통신 없음, 서드파티 의존성 0개
- 셸 인젝션 불가 구조, 절대 경로 실행, 최소 환경변수
- 프로세스 인자 원문은 저장·기록하지 않음, AI 대화 기록은 읽지 않음
- hardened runtime 서명, 릴리스 SHA-256 체크섬·빌드 출처 증명, SHA 고정 CI

## 기여

버그 신고(🚨 근무 태도 불량 신고), 에이전트 추가 요청, 번역 수정, 대사·스프라이트 아이디어 모두 환영합니다.
처음이라면 [`good first issue`](https://github.com/s3onghyun/bulchimbeon/labels/good%20first%20issue) 부터 → [CONTRIBUTING.md](CONTRIBUTING.md)

## 주의

덮개를 닫은 채 무거운 작업을 오래 돌리면 발열이 생긴다. 가능하면 전원을 연결하고 쓰자.

## License

MIT
