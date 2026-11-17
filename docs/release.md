# 릴리스 서명·공증 (Release signing and notarization)

맥 앱 릴리스는 `.github/workflows/release.yml` 이 **두 단계**로 만든다. Apple 공증은 보통 몇 분이면 끝나지만 새 개발자 계정은
몇 시간이 걸리기도 한다 (2026-11-17: 제출 넷이 몇 시간째 `In Progress`, 앞서 `Accepted` 된 둘은 몇 시간·약 9.5시간). 한 작업
안에서 기다리면 다시 돌릴 때마다 새로 빌드·서명·제출해, 늦게 온 `Accepted` 를 붙일 앱이 이미 없다. 그래서 제출한 앱을 그대로
보관하고, Apple 이 끝낸 뒤에 그 앱에 staple 해서 게시한다.

| 단계 | 언제 | 하는 일 | 게시 |
|---|---|---|---|
| 1단계 (제출) | `vX.Y.Z` 태그 푸시 | 시험 → universal 빌드 → Developer ID 서명 → 공증 제출(기다리지 않음) → 서명된 zip·`.sha256`·`submission.json`(제출 id) 을 artifact `bulchimbeon-signed-X.Y.Z` 로, 리눅스 tar.gz·deb·rpm·`SHA256SUMS` 를 `bulchimbeon-linux-X.Y.Z` 로 90일 보관 | 없음 |
| 2단계 (게시) | Apple 이 `Accepted` 한 뒤 수동 실행 `-f run_id=<1단계 run id>` | 1단계 artifact 를 받아 sha256 확인 → `notarytool info` → 같은 앱에 staple → `stapler validate`·`spctl` → `Bulchimbeon-X.Y.Z.zip`·`.sha256` → 빌드 출처 증명 → GitHub Release (맥 zip·체크섬, 리눅스 6개, `SHA256SUMS`) | Release |

2단계에서 Apple 이 아직 `In Progress` 면 아무것도 바꾸지 않고 실패로 끝난다. 나중에 같은 명령으로 다시 돌리면 된다. 1단계
artifact 는 90일 남으니 그 안에만 끝나면 된다. 게시 뒤 Homebrew tap·AUR 은 지금처럼 `scripts/bump-cask.sh`·`scripts/bump-aur.sh`
로 갱신한다 (Release 의 `.sha256`·`SHA256SUMS`·출처 증명을 읽으니 바뀐 것이 없다).
릴리스 제목은 `불침번 vX.Y.Z`, 본문은 기본 브랜치에 `docs/release-notes/vX.Y.Z.md` 가 있으면 그 파일, 없으면 GitHub 자동
생성이다.

서명 비밀값(`APPLE_DEVELOPER_ID_P12`)이 없으면 1단계는 **실패**한다. 공증 없이 ad-hoc 서명 앱을 내지 않는다. 다른 비밀값이
모자라도 실패한다.

## 릴리스 절차 (commands)

```sh
R=s3onghyun/bulchimbeon

# 1단계: 태그를 올리면 시작한다
git tag -a vX.Y.Z -m "불침번 vX.Y.Z" && git push origin vX.Y.Z

# 1단계 run id 찾기. 실행 요약(Summary)에 제출 id 와 2단계 명령이 그대로 적힌다
gh run list -R "$R" -w release.yml -L 5
gh run view <run id> -R "$R"            # 로그의 "공증 제출: <제출 id>"

# Apple 이 끝났는지 맥에서 본다 (아래 "맥에서 제출 id 조회하기"). Accepted 가 되면 2단계
gh workflow run release.yml -R "$R" --ref main -f run_id=<1단계 run id>
gh run list -R "$R" -w release.yml -L 1  # 2단계 실행 확인

# 게시된 뒤
./scripts/bump-cask.sh X.Y.Z
./scripts/bump-aur.sh X.Y.Z
```

2단계는 반드시 `--ref main` 으로 돌린다 (2단계 작업은 기본 브랜치의 워크플로에 있다). 1단계 실행(맥 작업과 리눅스 작업)이 모두
끝난 뒤에 돌린다.

| 2단계 결과 | 뜻 | 할 일 |
|---|---|---|
| 성공, 요약에 `✅ 게시: vX.Y.Z` | Release 가 생겼다 | `bump-cask.sh`·`bump-aur.sh` |
| 실패, `Notarization still in progress` 주석과 `공증이 아직 진행 중입니다 (still in progress)` | Apple 이 아직 처리 중. 아무것도 게시·변경하지 않았다 | 나중에 같은 명령으로 다시 (또는 그 2단계 실행을 Re-run) |
| 실패, `공증이 거절되었습니다 (notarization Invalid)` 와 공증 로그(JSON) | Apple 이 거절 | 로그의 `issues` 를 고치고 새 패치 버전 태그로 1단계부터 |
| 실패, `서명된 zip 의 sha256 이 submission.json 과 다릅니다` | artifact 가 1단계가 남긴 것과 다르다 | 게시하지 말고 원인을 본다. 1단계를 다시 (아래 태그 다시 만들기) |
| 실패, `태그 vX.Y.Z 의 커밋(…)이 1단계가 빌드한 커밋(…)과 다르다` | 1단계 뒤 태그가 옮겨졌다 | 태그를 되돌리거나 그 태그로 1단계를 다시 |
| 실패, `bulchimbeon-signed-* artifact 가 하나 있어야 한다` | run id 가 틀렸거나, 1단계가 실패했거나, 90일이 지났다 | run id 확인. 지났으면 1단계를 다시 |
| 실패, `release … already exists` | 이미 게시했다 | 없음 |

`submission_id` 입력(`-f submission_id=<id>`)은 artifact 에 적힌 제출 id 대신 다른 제출의 상태를 보게 한다. staple 은
제출 id 가 아니라 앱의 코드 해시로 티켓을 찾으므로, 같은 앱이 다른 제출로 `Accepted` 된 경우에만 성공한다. 맞지 않으면
`stapler`·`spctl` 확인에서 실패하고 아무것도 게시하지 않는다.

### 이미 있는 태그를 다시 만들기 (re-run phase 1 for an existing tag)

태그 푸시 실행을 GitHub 에서 Re-run 하면 그 태그 안의 옛 워크플로가 돈다. 이미 있는 태그(예: 이 흐름 전에 만든 `v0.12.6`)로
1단계를 다시 하려면 기본 브랜치의 워크플로를 `tag` 입력으로 돌린다. 소스는 그 태그에서, 공증 스크립트는 이 워크플로의 커밋에서
가져온다.

```sh
gh workflow run release.yml -R "$R" --ref main -f tag=vX.Y.Z
```

결과는 태그 푸시와 같다 (90일 artifact, 리눅스 포함, 게시 없음). 이 실행의 run id 로 2단계를 돌린다. 1단계를 다시 하면 새로
서명·제출하므로 그 run id 의 제출을 기다린다.

## 공증 단계가 하는 일 (`scripts/notarize-app.sh`)

`submit` (1단계, 서명 비밀값 모두 필요):

1. 이 실행에서만 쓰는 임시 키체인을 만들고 `.p12` 를 넣는다 (러너의 로그인 키체인은 쓰지 않음). 끝나면 지운다.
2. 팀 ID 가 `APPLE_TEAM_ID` 인 "Developer ID Application" 인증서로 `codesign --force --options runtime --timestamp` 서명.
   서명 식별자(`io.github.s3onghyun.bulchimbeon`)는 ad-hoc 때와 같다. 앱은 entitlements 가 필요 없다 (관리자 명령·알림은
   별도 프로세스인 `/usr/bin/osascript` 가 보낸다).
3. 서명한 앱을 `dist/notary/Bulchimbeon-X.Y.Z-signed.zip` 으로 묶고 `.sha256` 을 만든 뒤, 그 zip 을 그대로
   `xcrun notarytool submit` 으로 (`--wait` 없이) 제출한다.
4. `dist/notary/submission.json` 에 제출 id, 버전, zip 이름, zip 의 sha256, 빌드한 커밋, 제출 시각을 적고
   `공증 제출: <id>` 를 출력한다.

`finish [<제출 id>]` (2단계, `.p12` 는 받지 않음. 팀 ID 와 공증 인증만):

1. `dist/notary/` 의 zip 이 `submission.json` 의 sha256·`.sha256` 과 맞는지 확인한다.
2. `xcrun notarytool info <id>` (일시적 실패는 세 번까지). `In Progress` 면 종료 코드 75, `Invalid` 면 `notarytool log` 를
   출력하고 1.
3. `Accepted` 면 zip 을 `dist/Bulchimbeon.app` 으로 풀어 서명 팀을 확인하고 `xcrun stapler staple` → `stapler validate` →
   `codesign --verify --deep --strict` → `spctl --assess` 가 `source=Notarized Developer ID` 인지 확인.
4. staple 된 앱으로 `build-app.sh` 와 같은 이름·방식의 `dist/Bulchimbeon-X.Y.Z.zip`·`.sha256` 을 만든다.

비밀값은 각 단계의 환경 변수로만 들어가고 출력하지 않는다. `.p12` 는 1단계 한 단계에만, Release 를 만드는 권한
(`contents: write`)은 Apple 비밀값이 없는 마지막 게시 작업에만 있다. 릴리스 워크플로는 PR 에서 돌지 않는다.

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
6. **수동 점검 실행 (dry run)**: 입력 없이 돌리면 1단계만 한다 (버전 = 브랜치 이름, 리눅스 패키지 없음, artifact 7일).

   ```sh
   gh workflow run release.yml -R s3onghyun/bulchimbeon --ref main
   ```

   로그에서 `공증 인증 방식: api-key`(또는 `apple-id`)와 `공증 제출: <id>` 를 확인한다. Apple 이 `Accepted` 한 뒤 그 run id 로
   2단계를 돌리면 staple·확인까지만 하고 Release·출처 증명은 만들지 않는다. 로그에서 `공증 상태: Accepted`,
   `source=Notarized Developer ID` 를 확인한다. staple 된 zip 은 그 2단계 실행의 artifact `bulchimbeon-dry-run` 에 7일 남는다
   (예: `Bulchimbeon-main.zip`). 내려받아 풀고 맥에서 `spctl -a -vv -t exec Bulchimbeon.app` 과
   `xcrun stapler validate Bulchimbeon.app` 을 보고, 한 번 열어 "그래도 열기" 없이 뜨는지 본다. 점검용 앱은 `/Applications` 의
   앱 대신 쓰지 말고 확인 뒤 지운다.
7. 그 뒤로는 위 "릴리스 절차".

비밀값을 지우면 (`APPLE_DEVELOPER_ID_P12` 만 지워도) 릴리스 1단계가 실패한다. ad-hoc 길로 조용히 돌아가지 않는다.

## 맥에서 제출 id 조회하기 (check a submission from a Mac)

1단계 실행 요약과 로그(`공증 제출: <id>`)에 제출 id 가 있다. API 키(A)를 쓰면 `.p8` 을 저장소 밖에 두고:

```sh
xcrun notarytool info <id> --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>
xcrun notarytool log  <id> --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>   # 끝난 뒤 상세
xcrun notarytool history  --key ~/keys/AuthKey_XXXXXXXXXX.p8 --key-id <키 ID> --issuer <Issuer ID>   # 최근 제출 목록
```

자주 본다면 한 번 `xcrun notarytool store-credentials bulchimbeon --key … --key-id … --issuer …` 로 키체인에 넣고
`xcrun notarytool info <id> --keychain-profile bulchimbeon` 으로 줄인다. 상태가 `Accepted` 가 되면 2단계를 돌린다.
몇 시간마다 한 번이면 충분하다. Apple ID(B)면 `--apple-id <이메일> --team-id <팀 ID>` 를 쓰고 암호는 물을 때 입력한다
(명령줄에 적지 않는다).

`Invalid` 면 `notarytool log` 의 `issues` 가 이유다 (서명·hardened runtime·타임스탬프 문제가 흔하다). 고친 뒤 새 패치 버전으로
태그를 올린다. 이미 올린 태그를 옮기지 않는다.

## 첫 공증 릴리스 때 알아둘 것

- **zip 으로 설치한 사람의 앱 안 업데이트는 한 번 막힌다.** 앱은 새 앱의 서명 팀이 지금 앱과 같아야 바꾼다
  (`UpdateInstaller.verify`). ad-hoc 앱은 팀 ID 가 없어 첫 공증 버전을 "서명 팀이 다릅니다" 로 거절하고 릴리스 페이지를
  연다. 그 버전만 직접 내려받아 바꾸면 그다음부터는 같은 팀이라 앱 안 업데이트가 된다. Homebrew 설치는 `brew upgrade` 라
  영향이 없다. 첫 공증 릴리스의 릴리스 노트에 적는다. 반대로 공증 뒤 ad-hoc 앱을 내면 같은 이유로 한 번 막힌다 (워크플로는 이제 ad-hoc 으로 내지 않는다).
- 공증된 뒤 고칠 문구: tap 의 `Casks/bulchimbeon.rb` caveats ("not notarized by Apple yet"), README 들의 첫 실행 안내,
  `docs/install-for-agents.md`, `SECURITY.md` 의 "공증되지 않았다" 항목. 첫 공증 릴리스를 확인한 뒤 따로 바꾼다.
- 팀 ID 가 생기면 macOS 가 앱에 알림 권한을 줄 수 있다 (`Sources/Bulchimbeon/DutyController.swift` 의 `Notifier` 주석).
  첫 공증 버전에서 알림이 앱 이름으로 오는지 확인한다.
- Developer ID 인증서는 5년 유효하다. 보안 타임스탬프가 있어 인증서가 만료되거나 멤버십이 끝나도 이미 낸 릴리스는 계속 열린다.
  새 인증서도 같은 팀이면 앱 안 업데이트에 영향이 없다. 갱신하면 `APPLE_DEVELOPER_ID_P12`·암호만 바꾼다.
