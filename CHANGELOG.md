# Changelog

[호환성 약속](docs/compatibility.md)에 걸리는 변경은 **호환성 깨짐**으로 표시한다.
릴리스 파일과 빌드 출처 증명은 [Releases](https://github.com/s3onghyun/bulchimbeon/releases)에 있다.

## 다음 버전

- 리눅스 `.rpm`: 릴리스에 x86_64·aarch64 용 `bulchimbeon-<버전>-1.<아키텍처>.rpm` 을 더한다. `.deb` 와 같은 파일(`/usr/bin`, 켜지 않은 systemd 사용자 유닛, 트레이 자동 시작 항목, 문서·라이선스)이고, 같은 tarball 의 바이너리를 Ubuntu 저장소의 `rpmbuild` 로 다시 포장한다. `SHA256SUMS`·빌드 출처 증명에 들어가고, 패키징을 바꾸는 PR 마다 Fedora 컨테이너에서 설치·제거를 시험한다 (`packaging/rpm`, `scripts/package-rpm.sh`)
- Fedora COPR 준비: `.copr/Makefile` 이 릴리스 tarball 을 `SHA256SUMS` 로 검증해 소스 rpm 을 만든다. 소유자가 COPR 프로젝트를 만들기 전까지는 공개되지 않는다 (`packaging/rpm/README.md`)
- AUR 준비: `packaging/aur/bulchimbeon-bin` 의 `PKGBUILD`·`.SRCINFO` (x86_64·aarch64, 릴리스 tarball 과 sha256), 그리고 체크섬·출처 증명을 검증한 뒤에만 이를 갱신하는 `scripts/bump-aur.sh`. 소유자가 AUR 에 올리기 전까지는 공개되지 않는다 (`packaging/aur/README.md`)

## 0.8.1 — 2026-10-23

- 리눅스 잠금 수정: 데스크톱 세션이 있으면 그 데스크톱의 절전 억제로 자동 절전을 막는다. GNOME·Cinnamon·MATE·Budgie 는 `org.gnome.SessionManager.Inhibit`(절전 플래그 4 만), KDE 는 `org.freedesktop.PowerManagement.Inhibit`. 지금까지 잡던 logind 잠금은 systemd 256 이하(Ubuntu 24.04, Debian 12)에서 잠금을 쥔 사용자 본인의 요청에는 무시되고, gsd-power·xfce4-power-manager·csd-power 는 사용자로 절전을 요청하므로, GNOME·XFCE·Cinnamon 에서는 근무 중에도 자동 절전이 일어났다 (Ubuntu 24.04 GNOME 에서 실측)
- 리눅스 잠금: 데스크톱이 없으면(헤드리스·SSH·sway·Hyprland) logind `sleep` 잠금을 systemd 257 이상에서는 `block-weak` 로 잡아, 내가 하는 절전(메뉴, `systemctl suspend`)은 그대로 된다. 256 이하는 지금처럼 `block`
- 리눅스 잠금: `idle` 잠금을 더는 잡지 않는다. KDE·swayidle·hypridle 에서도 화면이 설정대로 어두워지고 잠긴다 (화면은 꺼져도 깨어 있음). 대신 sway·Hyprland 에서 idle 데몬이 사용자로 실행하는 자동 절전은 막지 못한다
- 리눅스 잠금: XFCE 에서는 화면 잠금을 지키려고 기본적으로 데스크톱 절전 억제를 잡지 않는다 (xfce4-power-manager 는 어떤 억제든 프레젠테이션 모드처럼 다뤄 화면을 켜 두고 잠그지 않는다). 그래서 systemd 버전과 상관없이 자동 절전을 막지 못하고, `status`·`doctor` 가 그렇게 알린다. 새 리눅스 설정 `xfceKeepAwake`(기본 `false`)를 `bulchimbeon config set xfceKeepAwake true` 로 켜면 `org.freedesktop.PowerManagement.Inhibit` 로 자동 절전을 막는 대신, 근무 중에는 화면이 켜진 채 잠기지 않는다
- 리눅스 잠금: 근무 중에 데스크톱에 로그인·로그아웃하면 다음 점검(5초) 안에 잠금이 데스크톱과 logind 사이를 옮겨 가고, 근무지 이탈로 기록하지 않는다. 데몬이 죽으면 세션 버스 연결과 함께 데스크톱 억제도 풀린다. 데몬은 세션 버스에 클라이언트로만 붙는다 (이름·객체 없음)
- `bulchimbeon status`·`doctor`: 어떤 방법(데스크톱 인터페이스, 또는 logind 와 모드)으로 잠금을 쥐었는지, 왜 logind 인지 보여준다. `doctor` 는 세션 버스의 데스크톱 절전 억제도 시험하고, 지금 쓸 잠금을 잡을 수 있으면 성공으로 끝난다
- 리눅스 트레이: 근무 중(전투휴식 포함)이 아니면 StatusNotifierItem 상태를 `Passive` 로 내려, KDE 는 대기·취침 중인 이등병을 트레이 넘침 영역에 둔다. 근무에 들어가면 `Active` 로 패널에 다시 보인다

## 0.8.0 — 2026-10-23

- 리눅스 트레이 아이콘: `bulchimbeon tray` 가 데스크톱 패널(XFCE 상태 트레이, KDE, GNOME AppIndicator 확장)에 이등병을 맥 메뉴바와 같은 자세·속도로 띄운다. 메뉴에는 `bulchimbeon status` 의 모든 줄(같은 순서), **🧥 군복** 선택, 트레이 닫기(데몬은 계속 근무)가 있다. `.deb` 는 `/etc/xdg/autostart`, `install-linux.sh` 는 `~/.config/autostart` 에 자동 시작 항목을 둔다. 세션 버스나 트레이 호스트가 없거나 데몬을 한 번도 켜지 않은 사용자면 한 줄만 남기고 끝나므로 SSH·헤드리스 기기에서도 그대로다
- 리눅스 설정: `uniformTheme` 을 리눅스에서도 쓴다 (`bulchimbeon config set uniformTheme navy`, 트레이의 군복 메뉴). 전에는 macOS 전용이라 `config set` 이 거부했다
- 스프라이트 공유 픽스처 `testdata/engine/sprites.json`: Swift Core 가 모든 군복·포즈의 프레임과 색을 내보내고, 리눅스 트레이가 같은 파일로 그린다

## 0.7.3 — 2026-10-23

- 군복 메뉴: 앞의 🎽(러닝셔츠) 아이콘이 속옷처럼 보여 🧥(야전상의)로 바꿨다 (#40)

## 0.7.2 — 2026-10-23

- 근무 서기: 메뉴 **🪖 근무 서기**에서 지금 고른 시간 항목(무기한 포함)에 체크 표시가 붙어, 무엇을 골랐는지 다시 볼 수 있다. 재시작 후 이어진 근무에는 표시하지 않는다 (#38)
- 아이디어·문의는 [GitHub Discussions](https://github.com/s3onghyun/bulchimbeon/discussions)에서 받는다. 기능 제안 양식은 영·중·한 3개 언어로 짧게 바꾸고, README 에 Ideas 링크를 달았다 (#37)
- 리눅스에서도 BulchimbeonCore 를 빌드하고 테스트할 수 있다 (macOS 빌드는 그대로) (#34)
- 수정: 사용자 정의 에이전트 이름 검사가 끝에 줄바꿈이 붙은 이름을 받아들이던 문제 (#34)
- 기여 절차: CODEOWNERS 가 go.mod·go.sum·packaging/ 을 보안 민감 경로로 표시하고, 번역 이슈 양식에 `translation` 라벨을 단다 (#35, #33)

## 0.7.1 — 2026-10-21

- 간체 중국어 UI: 메뉴·상태·설정·근무 대사까지 한국어·영어와 함께 간체 중국어를 고를 수 있다 (#27)
- 군복 테마: 메뉴 **🎽 군복**에 모래색·황갈색의 가상 **사막** 군복을 더했다. 실제 군대 복장·문양은 쓰지 않는다 (#26, #20 해결)
- 성능: macOS 에이전트 점검에서 argv 가 판별을 바꾸지 않는 프로세스는 PID·시작 시각만 기억해 매 점검의 `KERN_PROCARGS2` 읽기를 줄였다. argv 자체는 캐시하지 않고, 종료된 프로세스는 다음 점검에서 잊는다 (#31)
- 성능: 아이콘 프레임 타이머에 tolerance 를 줘 불필요한 깨어남을 줄였다 (#31)

## 0.7.0 — 2026-10-16

- 군복 테마: 메뉴 **🎽 군복** 에서 기본 전투복·네이비·설원 중 고른다. 옷 색과 무늬만 바뀌고 얼굴은 그대로, 기본은 지금과 똑같다. 새 설정 키 `uniformTheme` (#17)
- 리눅스 데몬 v0.1 (Go): `bulchimbeon run|status|doctor|config`, systemd 사용자 유닛, tar.gz·.deb 릴리스 파일 (amd64·arm64). 잠자기·화면 꺼짐을 logind inhibitor 두 개로 막고, SSH·헤드리스 기기용 polkit 규칙은 안내만 한다 (#16)
- 엔진 골든 픽스처 `testdata/engine`: Swift Core 와 Go 엔진이 같은 파일을 돌려 동작을 OS 간에 고정
- 1.0 호환성 약속 문서화 (`docs/compatibility.md`): 설정 키, 실행 인자, 근무일지 형식. 테스트로 고정 (#7)
- 설정 키·허용 값을 Core `SettingsKey` 로 모음 (동작 변화 없음)
- README: 이등병 어록, 비슷한 앱(Adrafinil)과 비교
- README 영어 우선 (`README.md` 영어, 한국어는 `README.ko.md`), CONTRIBUTING 영·한 전체판, 이슈 양식(기능 제안·번역 수정 추가)·PR 템플릿 영어 우선 (#8)
- 리눅스 Homebrew: `brew install s3onghyun/tap/bulchimbeon-linux` 후 `brew services start bulchimbeon-linux`. 맥 cask 와 이름이 겹치지 않게 `-linux` 를 붙였다
- 안전 장치: 워크플로 최소 권한·Action SHA 고정, 포크 PR 실행 승인, main 브랜치·`v*` 태그 보호, `AGENTS.md`, `CODEOWNERS`, AI 도움 기여 정책 (#15)
- `scripts/bump-cask.sh`: macOS·리눅스 어디서든 돌고, 리눅스 tarball 두 개도 체크섬·출처 증명을 검증한 뒤 tap 의 formula 를 함께 갱신
- 고마워요 @dannyward630: 첫 기여로 폭염 주의보·전투휴식·근무지 이탈·인수인계 대사를 영·한 문구 테스트에 넣어 주셨습니다 (#14, #10 해결)

## 0.6.9 — 2026-10-14

- 수정: 근무 중 근무 방식을 바꿔도 근무일지에 남지 않던 문제

## 0.6.8 — 2026-10-14

- 수정: 알림이 한 번도 전달되지 않던 문제. ad-hoc 서명 앱은 macOS 가 알림 권한을 주지 않아, 권한이 없으면 AppleScript 알림으로 대신 보낸다 (문구는 argv 로 전달). 공증 후에는 원래 경로를 쓴다
- 확인: 당직 근무 인수인계, 언어 메뉴 실측 (#5)

## 0.6.7 — 2026-10-14

- 수정: 덮개 모드의 당직 감시병이 관리자 셸이 끝날 때 함께 사라지던 문제. perl fork + setsid + 파이프 동기화로 분리 (#4)

## 0.6.6 — 2026-10-14

- 근무 상태 머신을 Core `DutyEngine` 으로 분리, 가짜 시계 시나리오 테스트 (#2)
- 수정: 앱 재시작 때 저장된 근무가 지워지던 초기화 순서 문제

## 0.6.5 — 2026-10-14

- 성능: 순찰 애니메이션을 Core Animation 에 맡겨 깨어남 84% 감소 (#1)

## 0.6.4 — 2026-10-14

- 메뉴에서 언어 선택 (시스템 따름 / 한국어 / English)

## 0.6.3 — 2026-10-14

- 잠자기를 막는 다른 앱을 메뉴에 표시, 진단에 작업 폴더

## 0.6.2 — 2026-10-13

- 메뉴에 근무 이유와 교대까지 남은 시간, 교대 유예 선택

## 0.6.1 — 2026-10-13

- 수정: 에이전트 근무 교대 깜빡임·지연 (CPU 60초 평균)

## 0.6.0 — 2026-10-13

- 열린 세션이 아니라 **일할 때만** 근무, 도우미 세션(`--tools=`) 제외

## 0.5.0 — 2026-10-13

- 폭염 주의보 → 전투휴식(식으면 자동 복귀), 근무지 이탈 감지, 덮개 모드 당직 감시병, 당직 근무 인수인계
- 기여 가이드·이슈 폼

## 0.4.0 — 2026-10-13

- 영어 UI, 근무 형태 이름 정리, 전체화면에서 순찰 숨김, 에너지 최적화

## 0.3.0 – 0.3.2 — 2026-10-13

- 에이전트 감지를 libproc 다중 신호 판별로 교체
- 순찰 말풍선, 재시작 후 근무 유지

## 0.1 – 0.2 — 2026-10-13

- 첫 공개: AI 에이전트용 메뉴바 잠자기 방지, 픽셀아트 이등병, 순찰 모드
