# Lid watchdog manual verification / 덮개 감시병 수동 검증

Run this on a disposable local build. It changes the Mac-wide `disablesleep` setting, so keep the final cleanup step even if a check fails.

1. Build and launch the app with `make app` and open `dist/Bulchimbeon.app`.
2. To make a two-minute watch available without changing source, quit the app, then set a temporary persisted timed order and launch it again:

   ```sh
   defaults write io.github.s3onghyun.bulchimbeon dutyOrder -string timed
   defaults write io.github.s3onghyun.bulchimbeon dutyOrderUntil -float "$(($(date +%s) + 120))"
   open dist/Bulchimbeon.app
   ```

3. While the app remains open, select **Stay awake with lid closed** and approve the one enable prompt. Confirm `pmset -g` reports `SleepDisabled 1`.
4. Keep the app running. When the timed watch ends, confirm within roughly five seconds that `pmset -g` reports `SleepDisabled 0`, with no additional password prompt.
5. Repeat with a longer temporary `dutyOrderUntil`, select **Off duty** manually, and confirm the same no-prompt restoration.
6. Always finish with `pmset -g`. If it still reports `SleepDisabled 1`, use **Restore lid sleep** in the menu; if that cannot run, execute `sudo pmset -a disablesleep 0` yourself. Remove the temporary settings with:

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

3. 앱을 켠 채 **덮개 닫아도 깨어 있음**을 고르고, 켜기용 관리자 승인 한 번만 합니다. `pmset -g`에서 `SleepDisabled 1`을 확인합니다.
4. 앱을 계속 켜 둡니다. 근무 시각이 끝난 뒤 약 5초 안에 추가 암호 창 없이 `pmset -g`가 `SleepDisabled 0`이 되는지 확인합니다.
5. 더 긴 임시 기한으로 다시 시작해 메뉴에서 수동으로 **근무 종료**를 고른 뒤에도 암호 창 없이 복구되는지 확인합니다.
6. 언제나 `pmset -g`로 끝을 확인합니다. 아직 `SleepDisabled 1`이면 메뉴의 **덮개 잠자기 복구**를 쓰고, 그것도 안 되면 직접 `sudo pmset -a disablesleep 0`을 실행합니다. 임시 설정은 아래로 지웁니다.

   ```sh
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrder
   defaults delete io.github.s3onghyun.bulchimbeon dutyOrderUntil
   ```
