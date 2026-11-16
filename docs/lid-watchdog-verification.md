# Lid watchdog manual verification / 덮개 감시병 수동 검증

Run this on a disposable local build. It changes the Mac-wide `disablesleep` setting, so keep the final cleanup step even if a check fails.

1. Build and launch the app with `make app` and open `dist/Bulchimbeon.app`.
2. To make a two-minute watch available without changing source, quit the app, then set a temporary persisted timed order and launch it again:

   ```sh
   defaults write io.github.s3onghyun.bulchimbeon dutyOrder -string timed
   defaults write io.github.s3onghyun.bulchimbeon dutyOrderUntil -float "$(($(date +%s) + 120))"
   open dist/Bulchimbeon.app
   ```

3. While the app remains open, select **Stay awake with lid closed** and approve the one enable prompt. Confirm `pmset -g` reports `SleepDisabled 1`, the duty log has no "Lid-closed watch denied/failed" line, and the menu shows no "This watch: …" fallback line (0.10.0–0.10.1 reported every approval as denied).
4. Keep the app running. When the timed watch ends, confirm within roughly five seconds that `pmset -g` reports `SleepDisabled 0`, with no additional password prompt.
5. Repeat with a longer temporary `dutyOrderUntil`, select **Off duty** manually, and confirm the same no-prompt restoration.
6. **Termination signal.** Start an indefinite lid-closed watch again. Confirm the watchdog is running with `pgrep -f 'bulchimbeon-watchdo[g]-'` (prints PIDs only), then stop it the way a restart or shutdown does: `sudo pkill -TERM -f 'bulchimbeon-watchdo[g]-'`. Within about one second `pmset -g` must report `SleepDisabled 0`; within the next five-second check the menu shows "This watch: … (lid sleep restored outside the app)".
7. **Restart.** Start an indefinite lid-closed watch and approve it. Without quitting the app, choose Apple menu › Restart. After logging in, run `pmset -g | grep SleepDisabled` before (or right as) the app starts; it must report `0`. The duty log must have no "⚠️ Last watch ended abnormally" line for this restart.
8. **Left-over flag.** Simulate a watchdog that died without restoring: start an indefinite lid-closed watch, run `sudo pkill -KILL -f 'bulchimbeon-watchdo[g]-'` (SIGKILL skips the restore), then `pkill -KILL -x Bulchimbeon`, and open the app again. Expect the "⚠️ Last watch ended abnormally" duty-log line and notification, the watch back on as a system watch ("This watch: … (app restarted — lid needs approval again)"), and **⚠️ Restore lid sleep** in the menu. Choosing it asks for the admin password once; afterwards `pmset -g` reports `SleepDisabled 0`. Recreate the same state once more and start a new watch from the watch menu instead (for example 30 min, lid-closed mode still selected): it asks for approval, and afterwards `pgrep -f 'bulchimbeon-watchdo[g]-'` shows a new watchdog.
9. **Fast relaunch.** During a lid-closed watch, switch the language from **언어 / Language / 语言** (the app relaunches at once). The duty log must show the watch resuming as a system watch and no "⚠️ Last watch ended abnormally" line. A "Lid sleep restored" line just before it means the new launch waited for the old watchdog.
10. Always finish with `pmset -g`. If it still reports `SleepDisabled 1`, use **Restore lid sleep** in the menu; if that cannot run, execute `sudo pmset -a disablesleep 0` yourself. Remove the temporary settings with:

   ```sh
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrder
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrderUntil
   ```

## 한국어

일회성 로컬 빌드에서만 시험하세요. `disablesleep`은 맥 전체 설정을 바꾸므로, 중간에 실패해도 마지막 정리 단계를 반드시 합니다.

1. `make app`으로 빌드하고 `dist/Bulchimbeon.app`을 엽니다.
2. 소스를 바꾸지 않고 2분 근무를 만들려면 앱을 끈 뒤 아래처럼 임시 시간제 근무를 저장하고 다시 엽니다.

   ```sh
   defaults write io.github.s3onghyun.bulchimbeon dutyOrder -string timed
   defaults write io.github.s3onghyun.bulchimbeon dutyOrderUntil -float "$(($(date +%s) + 120))"
   open dist/Bulchimbeon.app
   ```

3. 앱을 켠 채 **덮개 닫아도 깨어 있음**을 고르고, 켜기용 관리자 승인 한 번만 합니다. `pmset -g`에서 `SleepDisabled 1`을, 근무일지에 "덮개 근무 승인 거부/실패" 줄이 없고 메뉴에 "이번 근무: …" 대체 안내가 없는지 확인합니다 (0.10.0~0.10.1 은 승인해도 매번 거부로 기록했다).
4. 앱을 계속 켜 둡니다. 근무 시각이 끝난 뒤 약 5초 안에 추가 암호 창 없이 `pmset -g`가 `SleepDisabled 0`이 되는지 확인합니다.
5. 더 긴 임시 기한으로 다시 시작해 메뉴에서 수동으로 **근무 종료**를 고른 뒤에도 암호 창 없이 복구되는지 확인합니다.
6. **종료 신호.** 무기한 덮개 근무를 다시 켭니다. `pgrep -f 'bulchimbeon-watchdo[g]-'`(PID 만 출력)로 감시병이 떠 있는지 확인한 뒤, 재시동·시스템 종료와 같은 방식으로 끝냅니다: `sudo pkill -TERM -f 'bulchimbeon-watchdo[g]-'`. 1초쯤 안에 `pmset -g`가 `SleepDisabled 0`이어야 하고, 다음 5초 점검에서 메뉴에 "이번 근무: … (덮개 잠자기가 밖에서 복구됨)"이 보입니다.
7. **재시동.** 무기한 덮개 근무를 켜고 승인합니다. 앱을 끄지 말고 Apple 메뉴 › 재시동을 고릅니다. 로그인한 뒤 앱이 뜨기 전(또는 뜨자마자) `pmset -g | grep SleepDisabled`가 `0`이어야 합니다. 이 재시동에 대해 근무일지에 "⚠️ 지난 근무 비정상 종료" 줄이 없어야 합니다.
8. **남은 플래그.** 원복 없이 죽은 감시병을 흉내 냅니다: 무기한 덮개 근무를 켜고 `sudo pkill -KILL -f 'bulchimbeon-watchdo[g]-'`(SIGKILL 은 원복을 건너뜀), 이어서 `pkill -KILL -x Bulchimbeon` 을 실행한 뒤 앱을 다시 엽니다. 근무일지와 알림에 "⚠️ 지난 근무 비정상 종료"가 남고, 근무는 시스템 근무로 이어지며 ("이번 근무: … (앱 재시작 — 덮개 재승인 필요)"), 메뉴에 **⚠️ 덮개 잠자기 복구**가 보여야 합니다. 누르면 관리자 암호를 한 번 묻고, 그 뒤 `pmset -g`가 `SleepDisabled 0`입니다. 한 번 더 같은 상태를 만들고 이번에는 근무 메뉴에서 새 근무를 시작합니다 (예: 30분, 덮개 근무가 선택된 채): 승인을 묻고, 그 뒤 `pgrep -f 'bulchimbeon-watchdo[g]-'`에 새 감시병이 보여야 합니다.
9. **빠른 재실행.** 덮개 근무 중 **언어 / Language / 语言**에서 언어를 바꿉니다 (앱이 바로 다시 실행됨). 근무일지에 시스템 근무로 복귀한 줄이 있고 "⚠️ 지난 근무 비정상 종료" 줄은 없어야 합니다. 바로 앞의 "덮개 잠자기 복구 완료" 줄은 새 실행이 옛 감시병을 기다렸다는 뜻입니다.
10. 언제나 `pmset -g`로 끝을 확인합니다. 아직 `SleepDisabled 1`이면 메뉴의 **덮개 잠자기 복구**를 쓰고, 그것도 안 되면 직접 `sudo pmset -a disablesleep 0`을 실행합니다. 임시 설정은 아래로 지웁니다.

   ```sh
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrder
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrderUntil
   ```
