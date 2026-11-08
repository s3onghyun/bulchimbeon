# 불침번 (Bulchimbeon) 설계서

| 항목 | 내용 |
|------|------|
| 제품명 | 불침번 (Bulchimbeon) — "Your Mac's night-watch soldier" |
| 형태 | macOS 메뉴바 앱 (SwiftUI `MenuBarExtra`) |
| 최소 OS | macOS 13 Ventura |
| 빌드 | SwiftPM + `.app` 번들 조립 스크립트 (Xcode 불필요) |
| 라이선스 | MIT |
| 한 줄 요약 | 내가 자는 동안 AI 에이전트 곁에서 맥을 깨워두고, 일이 끝나면 "근무 교대" 하고 재워주는 이등병 |

## 1. 문제 정의

| 현재 방식 | 불편 |
|-----------|------|
| `caffeinate -d` 터미널에 띄워두기 | 잊어버리면 영원히 깨어 있음. 에이전트 끝나도 모름 |
| `sudo pmset disablesleep 1` | 덮개 닫아도 깨어 있지만 **되돌리는 걸 잊으면 배터리 0%** |
| Amphetamine 등 | 기능은 충분하지만 "에이전트가 끝났는지" 모름. 재미 없음 |

## 2. 컨셉 — 불침번

| 앱 상태 | 이등병 상태 | 메뉴바 |
|---------|------------|--------|
| 꺼짐 | 생활관 취침 | 💤 |
| 에이전트 근무 대기 | 근무 대기 (에이전트 출근 감시) | 👀 |
| 근무 중 | 경계 근무, "이상 무!" | 🪖 (+ 남은 시간) |
| 에이전트 작업 끝 | "근무 교대합니다!" 알림 후 취침 | 👀 → 대기 복귀 |
| 배터리 부족 | 당직사관에게 보고 후 철수 | 💤 |

톤 가이드: 2등신 치비, **갈굼·부조리 개그 금지**. "나 대신 밤새주는 고생하는 귀여운 애" 포지션.

## 3. 기능 (MVP v0.1)

| # | 기능 | 구현 |
|---|------|------|
| F1 | 근무 서기 (30분/1/2/4시간/무기한) | `IOPMAssertionCreateWithName` |
| F2 | 근무 형태 선택 | 화면까지 / 시스템만 / 덮개 닫아도 |
| F3 | **에이전트 근무 (자동 교대)** | `ps` 폴링 → 에이전트 있으면 근무 투입, 모두 끝나면 교대 알림 후 해제 |
| F4 | 배터리 철수 | 배터리 사용 중 & 잔량 ≤ 기준(기본 20%) → 강제 해제 |
| F5 | 덮개 모드 안전장치 | 해제 시 `disablesleep 0` 복구, 비정상 종료 후 재실행 시 복구 버튼 노출 |
| F6 | 근무일지 | `~/Library/Application Support/Bulchimbeon/duty-log.txt` + 메뉴에 최근 10건 |
| F7 | 로그인 시 자동 실행 | `SMAppService.mainApp` |

### 근무 형태 ↔ 시스템 동작

| 근무 형태 | Power assertion | 덮개 | 권한 |
|-----------|-----------------|------|------|
| 화면까지 켜두기 | `PreventUserIdleDisplaySleep` | 닫으면 잠 | 없음 |
| 시스템만 깨우기 | `PreventUserIdleSystemSleep` | 닫으면 잠 | 없음 |
| 덮개 닫아도 근무 | `PreventUserIdleSystemSleep` + `pmset -a disablesleep 1` | 깨어 있음 | 관리자 암호 (osascript) |

> power assertion은 idle 타이머만 막는다. 덮개 닫힘은 하드웨어 트리거라 `disablesleep` 전역 플래그가 필요하고, 이 플래그는 프로세스가 죽어도 남는다 → F5 안전장치 필수.

## 4. 에이전트 감지 규칙

`ps` 출력 파싱 대신 libproc(`proc_listpids(PROC_UID_ONLY)`, `proc_pidpath`, `sysctl(KERN_PROCARGS2)`)으로
**현재 사용자 프로세스만** 수집한다. 신호를 여러 개 교차해 설치 방식에 상관없이 잡는다.

| 우선순위 | 신호 | 왜 필요한가 |
|----------|------|-------------|
| 1 | 실행 파일 basename | Homebrew·IDE 내장 바이너리 |
| 2 | `argv[0]` basename | 네이티브 설치본은 실행 파일 이름이 `2.1.296` 같은 버전 번호 (실측) |
| 3 | 실행 파일 경로의 설치 마커 | 전체 경로로 실행돼 `argv[0]` 도 버전 경로인 경우 |
| 4 | node/bun/deno 의 스크립트 basename·패키지 마커 | npm 전역 설치(`node /opt/homebrew/bin/claude`), npx |

- 대소문자 구분 → `Claude.app` 데스크톱 앱 제외
- `node -e` / `--eval` 인라인 스크립트는 검사하지 않음 (hook 오탐 방지)
- 사용자 지정: `customAgents` (이름 규칙 검증, 최대 16개)
- 진단: `Bulchimbeon --diagnose` — 감지 결과와 근거만 출력 (인자 미출력)

### 에이전트 근무 상태 머신

```
        에이전트 감지 ≥1                    에이전트 0
 대기(👀) ───────────────▶ 근무(🪖) ───────────────▶ 교대 알림 → 대기(👀)
    ▲                         │ 배터리 ≤ 기준
    │                         ▼
    └──────── 수동 재개 ── 철수(💤)
```

### 검토했지만 넣지 않은 신호 (2026-10-14)

| 신호 | 결론 | 근거 (실측) |
|------|------|------------|
| 대화 기록(transcript) 파일 수정 시각 | ❌ 미채택 | 쉬는 Claude Code 세션 4개의 기록 파일이 같은 초에 갱신됨 — 내용은 작업이 아니라 관리용 기록(`atis-latch`, `system/away_summary`, `system/turn_duration`). 구분하려면 기록 내용을 읽어야 해서 "대화 내용은 읽지 않는다" 원칙과 충돌. 작업 판단은 `caffeinate` 자식·CPU 신호로 한다 (규칙: [README](../README.ko.md)) |
| Claude Code lifecycle hook | ⏸ 보류 | 사용자 `~/.claude/settings.json` 수정 필요, hook 지연이 에이전트를 방해할 위험. 에디터 내장 에이전트·원격 승인 요청이 생기면 기본 꺼짐 선택 기능으로 재검토 |

## 5. 구조

| 모듈 | 파일 | 책임 |
|------|------|------|
| `BulchimbeonCore` (순수 로직, 테스트 대상) | `AgentCatalog.swift` | 에이전트 매칭·ps 출력 파싱 |
| | `Policies.swift` | 배터리 철수 판단, pmset 파싱, 시간 포맷 |
| | `Lines.swift` | 대사 |
| | `DutyLog.swift` | 근무일지 저장 |
| `Bulchimbeon` (앱) | `PowerAssertion.swift` | IOKit assertion 래퍼 |
| | `SystemProbes.swift` | 배터리·ps·pmset·osascript 호출 |
| | `DutyController.swift` | 상태 머신 (5초 tick) |
| | `BulchimbeonApp.swift` | 메뉴바 UI |

## 6. 로드맵

| 버전 | 내용 |
|------|------|
| v0.1 | MVP (위 F1–F7), 이모지 아이콘 |
| v0.2 ✅ | 치비 이등병 픽셀아트 아이콘·애니메이션 (취침/대기/근무/경례 4포즈), 앱 아이콘 |
| v0.3 ✅ | 순찰 모드 (화면 위 오버레이), 보안 강화 (SECURITY.md) |
| v0.3.1 | 에이전트 **유휴 감지** (프로세스 생존이 아니라 CPU·출력 활동 기준) |
| v0.5 ✅ | 폭염 주의보·전투휴식, 근무지 이탈 감지, 당직 감시병(덮개 모드 원복 watchdog), 당직 근무 인수인계 |
| v0.6 | Privileged helper (`SMAppService.daemon`) 로 덮개 모드 암호 프롬프트 제거 |
| v0.5 | 해금 스킨: 순라꾼(조선시대 불침번), 선비(현량자고) / 당직사관 이스터에그 |
| v1.0 | Homebrew cask 배포, 공증(notarization) |
