# 릴리스 서명·공증 (Release signing and notarization)

맥 앱 릴리스는 `.github/workflows/release.yml` 이 만든다. 서명 비밀값이 있느냐에 따라 두 길로 갈린다.

| 저장소 비밀값 `APPLE_DEVELOPER_ID_P12` | 결과 |
|---|---|
| 없음 (지금) | 지금까지와 같다: `scripts/build-app.sh` 의 ad-hoc 서명(hardened runtime) 앱. 로그에 `Notarization skipped …` 한 줄 |
| 있음 | `scripts/notarize-app.sh` 가 같은 앱을 Developer ID 로 다시 서명 → Apple 공증 → 티켓 staple → 같은 이름의 zip·`.sha256` 을 다시 만든다 |

어느 길이든 릴리스 파일 이름(`Bulchimbeon-X.Y.Z.zip`, `Bulchimbeon-X.Y.Z.zip.sha256`), 빌드 출처 증명, Homebrew tap·AUR 갱신
절차(`scripts/bump-cask.sh`, `scripts/bump-aur.sh`)는 같다. 공증 길에서는 staple 한 앱으로 zip 을 다시 만들므로 체크섬도 그 zip 의
값이 되고, 출처 증명도 그 zip 에 붙는다. `bump-cask.sh` 는 릴리스의 `.sha256` 을 읽으니 따로 바꿀 것이 없다.

`APPLE_DEVELOPER_ID_P12` 가 있는데 다른 값이 모자라면 릴리스는 **실패**한다. 조용히 ad-hoc 으로 나가지 않는다.

## 공증 단계가 하는 일

1. 이 실행에서만 쓰는 임시 키체인을 만들고 `.p12` 를 넣는다 (러너의 로그인 키체인은 쓰지 않음). 끝나면 지운다.
2. 팀 ID 가 `APPLE_TEAM_ID` 인 "Developer ID Application" 인증서로 `codesign --force --options runtime --timestamp` 서명.
   서명 식별자(`io.github.s3onghyun.bulchimbeon`)는 ad-hoc 때와 같다. 앱은 entitlements 가 필요 없다 (관리자 명령·알림은
   별도 프로세스인 `/usr/bin/osascript` 가 보낸다).
3. `xcrun notarytool submit` 로 제출하고 (`--wait` 없이) 받은 제출 id 를 바로 출력한다. 그 뒤 30초마다
   `xcrun notarytool info <id>` 로 상태를 물어 `Accepted` 나 `Invalid` 가 될 때까지 최대 `NOTARY_WAIT_MINUTES` 분(기본 90)
   기다린다. `Accepted` 가 아니면 공증 로그(`notarytool log <id>`)를 출력하고 실패한다. 아래 "공증이 오래 걸릴 때" 참고.
4. `xcrun stapler staple` → `stapler validate` → `codesign --verify --deep --strict` → `spctl --assess` 가
   `source=Notarized Developer ID` 인지 확인.
5. staple 된 앱으로 zip·체크섬을 다시 만든다. 다음 `Verify` 단계가 서명·staple·Gatekeeper·체크섬을 한 번 더 본다.

비밀값은 이 한 단계의 환경 변수로만 들어가고 출력하지 않는다. 릴리스 워크플로는 PR 에서 돌지 않는다.

## 비밀값 (Repository secrets)

Settings → Secrets and variables → Actions → Repository secrets. 이름만 여기 적는다. 값은 어디에도 커밋하지 않는다.

| 이름 | 필수 | 내용 |
|---|---|---|
| `APPLE_DEVELOPER_ID_P12` | 예 (이것이 스위치) | Developer ID Application 인증서 + 개인 키를 내보낸 `.p12` 의 base64 |
| `APPLE_DEVELOPER_ID_P12_PASSWORD` | 예 | 그 `.p12` 를 내보낼 때 정한 암호 |
| `APPLE_TEAM_ID` | 예 | 팀 ID (영대문자·숫자 10자, developer.apple.com → Account → Membership details) |
| `APPLE_API_KEY_ID` | 공증 인증 A (권장) | App Store Connect API 키 ID |
| `APPLE_API_ISSUER_ID` | 공증 인증 A | 그 키의 Issuer ID |
| `APPLE_API_KEY_P8` | 공증 인증 A | 내려받은 `AuthKey_<키ID>.p8` 의 base64 |
| `APPLE_ID` | 공증 인증 B | Apple 계정 이메일 |
| `APPLE_APP_SPECIFIC_PASSWORD` | 공증 인증 B | 그 계정의 앱 암호 (app-specific password) |

공증 인증은 A 나 B 하나만 넣는다. A 의 값이 하나라도 있으면 A 를 쓴다. A(API 키)를 권장한다: 개인 Apple 계정 암호와 따로
있고, 역할을 "Developer" 로 좁힐 수 있고, 계정 설정을 건드리지 않고 App Store Connect 에서 바로 폐기할 수 있다.
B 는 비밀값이 둘이라 조금 더 간단하다.

## 가입한 뒤 할 일 (after enrolling)

Apple Developer Program 가입이 승인된 맥에서 한다. 개인 가입이면 본인이 Account Holder 라 Developer ID 인증서를 만들 수 있다.

1. **Developer ID Application 인증서 만들기** (둘 중 하나)
   - Xcode: Settings → Accounts → Apple 계정 추가 → 팀 선택 → Manage Certificates… → `+` → **Developer ID Application**.
   - 웹: 키체인 접근 → 인증서 지원 → 인증 기관에서 인증서 요청… → "디스크에 저장" 으로 CSR 을 만든 뒤
     <https://developer.apple.com/account/resources/certificates/add> → **Developer ID Application** → G2 Sub-CA →
     CSR 올리기 → `.cer` 를 내려받아 더블클릭 (로그인 키체인에 들어간다).
2. **`.p12` 로 내보내기**: 키체인 접근 → 로그인 → 내 인증서 → "Developer ID Application: 이름 (팀ID)" 를 펼쳐 개인 키가
   함께 있는지 확인 → 인증서를 선택하고 내보내기… → 형식 `.p12`, 강한 암호.
3. **비밀값 넣기** (터미널, [GitHub CLI](https://cli.github.com) 로그인 상태. 값이 화면·셸 기록에 남지 않게 표준 입력으로 넘긴다)

   ```sh
   R=s3onghyun/bulchimbeon
   base64 -i DeveloperID.p12 | gh secret set APPLE_DEVELOPER_ID_P12 -R "$R"
   gh secret set APPLE_DEVELOPER_ID_P12_PASSWORD -R "$R"   # 물으면 암호 입력
   gh secret set APPLE_TEAM_ID -R "$R"                     # 물으면 팀 ID 입력
   ```

4. **공증 인증 넣기**
   - A (권장): <https://appstoreconnect.apple.com/access/integrations/api> → Team Keys → 키 생성, 접근 권한 **Developer** →
     Issuer ID 와 Key ID 를 확인하고 `AuthKey_<키ID>.p8` 을 내려받는다 (한 번만 받을 수 있다).

     ```sh
     base64 -i AuthKey_XXXXXXXXXX.p8 | gh secret set APPLE_API_KEY_P8 -R "$R"
     gh secret set APPLE_API_KEY_ID -R "$R"
     gh secret set APPLE_API_ISSUER_ID -R "$R"
     ```

   - B: <https://account.apple.com> → 로그인 및 보안 → 앱 암호 → 새 암호 생성.

     ```sh
     gh secret set APPLE_ID -R "$R"
     gh secret set APPLE_APP_SPECIFIC_PASSWORD -R "$R"
     ```

5. 내려받은 `.p12`·`.p8` 은 암호 관리자 같은 안전한 곳에 보관하거나 지운다. 저장소 폴더 안에 두지 않는다.
6. **수동 점검 실행 (dry run)**: Actions → Release → Run workflow (브랜치 `main`), 또는

   ```sh
   gh workflow run release.yml -R s3onghyun/bulchimbeon --ref main -f NOTARY_WAIT_MINUTES=90
   ```

   `NOTARY_WAIT_MINUTES`(10~300, 기본 90)는 공증 결과를 기다릴 최대 시간이다. 새 계정의 첫 제출들은 Apple 처리가 오래 걸릴 수
   있으니 첫 실행에서는 늘려도 된다. 태그 푸시 릴리스는 늘 90분이다.
   수동 실행은 빌드·서명·공증·확인까지만 하고 Release·출처 증명·리눅스 패키지는 만들지 않는다. 로그에서
   `공증 인증 방식: api-key`(또는 `apple-id`), `공증 결과: Accepted`, `source=Notarized Developer ID` 를 확인한다. 결과 zip 은
   실행 페이지의 artifact `bulchimbeon-dry-run` 에 7일 남는다 (버전 이름은 브랜치 이름, 예: `Bulchimbeon-main.zip`). 내려받아 풀고
   맥에서 `spctl -a -vv -t exec Bulchimbeon.app` 과 `xcrun stapler validate Bulchimbeon.app` 을 보고, 한 번 열어 "그래도 열기"
   없이 뜨는지 본다. 점검용 앱은 `/Applications` 의 앱 대신 쓰지 말고 확인 뒤 지운다.
7. 그 뒤로는 평소처럼 `git tag vX.Y.Z && git push origin vX.Y.Z`.

비밀값을 지우면 (`APPLE_DEVELOPER_ID_P12` 만 지워도) 다시 ad-hoc 길로 돌아간다.

## 공증이 오래 걸릴 때 (slow notarization)

Apple 공증은 보통 몇 분이면 끝나지만, 새 개발자 계정의 첫 제출들은 몇 시간이 걸리기도 한다. 2026-11-17 첫 CI 점검 실행은
예전 스크립트의 `submit --wait --timeout 30m` 이 30분을 넘겨 제출 id 도 남기지 못하고 실패했다. 지금 로그는 이렇게 보인다.

```text
공증 인증 방식: api-key
공증 제출: 2f1c9d3e-4b5a-4c6d-8e7f-0123456789ab
공증 대기 중 (waiting): 0분 00초 경과, 상태 In Progress - submission 2f1c9d3e-…
공증 대기 중 (waiting): 5분 01초 경과, 상태 In Progress - submission 2f1c9d3e-…     ← 5분마다 한 줄
공증 결과: Accepted (submission 2f1c9d3e-…, 7분 32초)
```

| 마지막 줄 | 뜻 | 할 일 |
|---|---|---|
| `공증 결과: Accepted …` 다음 `✅ 공증·staple 완료` | 성공 | 없음 |
| `공증 결과: Invalid …` | Apple 이 거절. 바로 아래에 공증 로그(JSON)가 있다 | 로그의 `issues` 를 고친 뒤 다시 실행 |
| `공증이 아직 진행 중입니다 (still in progress) - submission <id>; …` | 기다린 시간 안에 Apple 이 끝내지 않았다. 제출은 Apple 쪽에서 계속 처리된다 | 아래처럼 맥에서 그 id 의 상태를 보고, 끝난 뒤 `NOTARY_WAIT_MINUTES` 를 늘려 다시 실행 |
| `공증 제출 실패: 제출 id 를 받지 못했습니다 …` | 업로드·인증 실패. 아래에 notarytool 오류 끝부분 | 비밀값(특히 API 키·Issuer ID)과 Apple 계약 동의 상태 확인 |
| `공증 상태를 5번 연속 조회하지 못했습니다 …` | 상태 조회가 연속 실패 (네트워크·Apple 장애) | 맥에서 그 id 를 조회하고 나중에 다시 실행 |

다시 실행하면 새로 서명하고 새로 제출한다. 앞 제출을 이어 기다리지는 않는다.

**맥에서 제출 id 조회하기.** API 키(A)를 쓰면 `.p8` 을 저장소 밖에 두고:

```sh
xcrun notarytool info <id> --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>
xcrun notarytool log  <id> --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>   # 끝난 뒤 상세
xcrun notarytool history  --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>   # 최근 제출 목록
```

자주 본다면 한 번 `xcrun notarytool store-credentials bulchimbeon --key … --key-id … --issuer …` 로 키체인에 넣고
`xcrun notarytool info <id> --keychain-profile bulchimbeon` 으로 줄인다. Apple ID(B)면 `--apple-id <이메일> --team-id <팀 ID>`
를 쓰고 암호는 물을 때 입력한다 (명령줄에 적지 않는다).

## 첫 공증 릴리스 때 알아둘 것

- **zip 으로 설치한 사람의 앱 안 업데이트는 한 번 막힌다.** 앱은 새 앱의 서명 팀이 지금 앱과 같아야 바꾼다
  (`UpdateInstaller.verify`). ad-hoc 앱은 팀 ID 가 없어 첫 공증 버전을 "서명 팀이 다릅니다" 로 거절하고 릴리스 페이지를
  연다. 그 버전만 직접 내려받아 바꾸면 그다음부터는 같은 팀이라 앱 안 업데이트가 된다. Homebrew 설치는 `brew upgrade` 라
  영향이 없다. 첫 공증 릴리스의 릴리스 노트에 적는다. 반대로 공증 뒤 비밀값을 지워 ad-hoc 으로 돌아가도 같은 이유로 한 번 막힌다.
- 공증된 뒤 고칠 문구: tap 의 `Casks/bulchimbeon.rb` caveats ("not notarized by Apple yet"), README 들의 첫 실행 안내,
  `docs/install-for-agents.md`, `SECURITY.md` 의 "공증되지 않았다" 항목. 첫 공증 릴리스를 확인한 뒤 따로 바꾼다.
- 팀 ID 가 생기면 macOS 가 앱에 알림 권한을 줄 수 있다 (`Sources/Bulchimbeon/DutyController.swift` 의 `Notifier` 주석).
  첫 공증 버전에서 알림이 앱 이름으로 오는지 확인한다.
- Developer ID 인증서는 5년 유효하다. 보안 타임스탬프가 있어 인증서가 만료되거나 멤버십이 끝나도 이미 낸 릴리스는 계속 열린다.
  새 인증서도 같은 팀이면 앱 안 업데이트에 영향이 없다. 갱신하면 `APPLE_DEVELOPER_ID_P12`·암호만 바꾼다.
