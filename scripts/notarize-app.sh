#!/usr/bin/env bash
# Apple 공증을 두 단계로 한다. Apple 처리가 몇 시간 걸려도 서명한 앱과 제출을 버리지 않기 위해서다 (docs/release.md).
#
#   notarize-app.sh submit
#     build-app.sh 가 만든 dist/Bulchimbeon.app 을 Developer ID 로 다시 서명 → zip → notarytool submit (기다리지 않음)
#     → dist/notary/ 에 Bulchimbeon-$VERSION-signed.zip, 그 .sha256, submission.json(제출 id·버전·sha256·커밋·시각)
#     을 남기고 "공증 제출: <id>" 를 출력한다. 릴리스 워크플로 1단계가 이 폴더를 artifact 로 보관한다.
#   notarize-app.sh finish [<submission-id>]
#     dist/notary/ 의 그 zip 이 submission.json 과 맞는지 확인 → notarytool info 로 상태 확인
#       Accepted    zip 을 dist/Bulchimbeon.app 으로 풀어 staple → stapler validate → spctl 이
#                   "source=Notarized Developer ID" 인지 확인 → build-app.sh 와 같은 이름·방식으로
#                   dist/Bulchimbeon-$VERSION.zip 과 .sha256 을 만든다 (종료 코드 0)
#       In Progress 아무것도 바꾸지 않고 종료 코드 75 (나중에 다시 돌린다)
#       Invalid     notarytool log 를 출력하고 종료 코드 1
#     id 를 주지 않으면 submission.json 의 id 를 쓴다.
#
# 값은 모두 환경 변수로 받고 출력하지 않는다. set -x 를 켜지 말 것.
#   VERSION                          submit: build-app.sh 와 같은 버전 (zip 이름). finish 는 submission.json 에서 읽는다
#   APPLE_DEVELOPER_ID_P12           submit 만: Developer ID Application 인증서 + 개인 키 .p12 의 base64
#   APPLE_DEVELOPER_ID_P12_PASSWORD  submit 만: 그 .p12 를 내보낼 때 정한 암호
#   APPLE_TEAM_ID                    팀 ID (영대문자·숫자 10자). finish 는 앱의 서명 팀과 비교한다
#   공증 인증은 둘 중 하나 (API 키 쪽이 하나라도 있으면 API 키):
#     APPLE_API_KEY_ID + APPLE_API_ISSUER_ID + APPLE_API_KEY_P8 (.p8 의 base64)
#     APPLE_ID + APPLE_APP_SPECIFIC_PASSWORD
#   RELEASE_ROOT                     dist/ 가 있는 폴더 (기본: 이 스크립트의 저장소). 태그를 다시 빌드할 때 워크플로가 쓴다
set -euo pipefail

usage() { echo "usage: notarize-app.sh submit | finish [<submission-id>]" >&2; exit 2; }
MODE="${1:-}"
case "$MODE" in
  submit) (($# == 1)) || usage ;;
  finish) (($# <= 2)) || usage ;;
  *) usage ;;
esac

cd "${RELEASE_ROOT:-$(dirname "$0")/..}"
APP="dist/Bulchimbeon.app"
NOTARY="dist/notary"
EX_IN_PROGRESS=75

missing=()
need() { local v; for v in "$@"; do [[ -n "${!v:-}" ]] || missing+=("$v"); done; }
[[ "$MODE" == submit ]] && need APPLE_DEVELOPER_ID_P12 APPLE_DEVELOPER_ID_P12_PASSWORD
need APPLE_TEAM_ID
if [[ -n "${APPLE_API_KEY_ID:-}${APPLE_API_ISSUER_ID:-}${APPLE_API_KEY_P8:-}" ]]; then
  AUTH=api-key
  need APPLE_API_KEY_ID APPLE_API_ISSUER_ID APPLE_API_KEY_P8
elif [[ -n "${APPLE_ID:-}${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
  AUTH=apple-id
  need APPLE_ID APPLE_APP_SPECIFIC_PASSWORD
else
  AUTH=none
  missing+=("APPLE_API_KEY_ID/APPLE_API_ISSUER_ID/APPLE_API_KEY_P8 or APPLE_ID/APPLE_APP_SPECIFIC_PASSWORD")
fi
if ((${#missing[@]})); then
  echo "공증 비밀값이 모자랍니다 (missing secrets): ${missing[*]}" >&2
  exit 1
fi
[[ "$APPLE_TEAM_ID" =~ ^[A-Z0-9]{10}$ ]] || { echo "APPLE_TEAM_ID 는 영대문자·숫자 10자여야 합니다" >&2; exit 1; }
[[ "$(uname -s)" == Darwin ]] || { echo "macOS 에서만 돕니다" >&2; exit 1; }
echo "공증 인증 방식: $AUTH"

is_uuid() { [[ "$1" =~ ^[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}$ ]]; }
is_version() { [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]]; }
sha256_of() { shasum -a 256 "$1" | cut -d' ' -f1; }
json_field() { plutil -extract "$2" raw -o - "$1" 2>/dev/null || true; }

WORK="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/bulchimbeon-notarize.XXXXXX")"
KEYCHAIN=""
OLD_KEYCHAINS=()
cleanup() {
  if [[ -n "$KEYCHAIN" ]]; then
    security list-keychains -d user -s ${OLD_KEYCHAINS[@]+"${OLD_KEYCHAINS[@]}"} 2>/dev/null || true
    security delete-keychain "$KEYCHAIN" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

if [[ "$AUTH" == api-key ]]; then
  printf '%s' "$APPLE_API_KEY_P8" | base64 --decode > "$WORK/AuthKey.p8"
  NOTARY_AUTH=(--key "$WORK/AuthKey.p8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
else
  NOTARY_AUTH=(--apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_SPECIFIC_PASSWORD")
fi

# notarytool 의 오류 출력에는 비밀값이 없지만, 혹시 섞여도 가린 뒤 끝부분만 보인다
redact() {
  local s="$1" v
  for v in APPLE_DEVELOPER_ID_P12 APPLE_DEVELOPER_ID_P12_PASSWORD APPLE_API_KEY_ID APPLE_API_ISSUER_ID APPLE_API_KEY_P8 \
    APPLE_ID APPLE_APP_SPECIFIC_PASSWORD; do
    [[ -n "${!v:-}" ]] && s="${s//"${!v}"/<redacted>}"
  done
  printf '%s\n' "$s"
}
show_tail() { redact "$(tail -n 20 "$1" 2>/dev/null || true)" >&2; }

submit() {
  VERSION="${VERSION:?VERSION 이 필요합니다}"
  is_version "$VERSION" || { echo "VERSION 형식 오류 (영문·숫자·._- 만): $VERSION" >&2; exit 1; }
  [[ -d "$APP" ]] || { echo "$APP 이 없습니다. 먼저 ./scripts/build-app.sh" >&2; exit 1; }
  local signed="Bulchimbeon-$VERSION-signed.zip"

  KEYCHAIN="$WORK/signing.keychain-db"
  local line
  while IFS= read -r line; do
    line="${line#"${line%%[![:space:]]*}"}"; line="${line#\"}"; line="${line%\"}"
    [[ -n "$line" ]] && OLD_KEYCHAINS+=("$line")
  done < <(security list-keychains -d user)

  # 1. 이 실행에서만 쓰는 임시 키체인에 인증서를 넣는다 (로그인 키체인은 건드리지 않음)
  local keychain_password
  keychain_password="$(uuidgen)"
  security create-keychain -p "$keychain_password" "$KEYCHAIN"
  security set-keychain-settings -lut 3600 "$KEYCHAIN"
  security unlock-keychain -p "$keychain_password" "$KEYCHAIN"
  printf '%s' "$APPLE_DEVELOPER_ID_P12" | base64 --decode > "$WORK/developer-id.p12"
  security import "$WORK/developer-id.p12" -k "$KEYCHAIN" -f pkcs12 -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -T /usr/bin/codesign >/dev/null
  rm -f "$WORK/developer-id.p12"
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$KEYCHAIN" >/dev/null
  security list-keychains -d user -s "$KEYCHAIN" ${OLD_KEYCHAINS[@]+"${OLD_KEYCHAINS[@]}"}

  # 팀 ID 가 맞는 Developer ID Application 인증서의 SHA-1 (이름이 겹쳐도 정확히 그 인증서로 서명)
  local identity
  identity="$(security find-identity -v -p codesigning "$KEYCHAIN" \
    | awk -v team="($APPLE_TEAM_ID)\"" 'index($0, "\"Developer ID Application: ") && index($0, team) { print $2; exit }')"
  [[ -n "$identity" ]] || { echo "이 팀의 Developer ID Application 인증서가 .p12 에 없습니다" >&2; exit 1; }

  # 2. Developer ID 서명: hardened runtime + 보안 타임스탬프 (공증 필수 조건). 서명 식별자는 ad-hoc 때와 같다
  codesign --force --options runtime --timestamp --keychain "$KEYCHAIN" --sign "$identity" "$APP"
  check_signature

  # 3. 제출할 zip 이 곧 보관할 zip 이다. 2단계는 이 파일을 그대로 풀어 staple 한다
  rm -rf "$NOTARY"
  mkdir -p "$NOTARY"
  ditto -c -k --keepParent "$APP" "$NOTARY/$signed"
  local sha
  sha="$(sha256_of "$NOTARY/$signed")"
  (cd "$NOTARY" && shasum -a 256 "$signed" > "$signed.sha256")

  # --wait 없이 제출해 id 만 받는다. 결과는 2단계(finish)가 따로 묻는다
  xcrun notarytool submit "$NOTARY/$signed" "${NOTARY_AUTH[@]}" --output-format json \
    > "$WORK/submit.json" 2> "$WORK/submit.err" || true
  local id
  id="$(json_field "$WORK/submit.json" id)"
  if ! is_uuid "$id"; then
    echo "공증 제출 실패: 제출 id 를 받지 못했습니다 (notarization submit failed, no submission id). notarytool 출력 끝부분:" >&2
    show_tail "$WORK/submit.json"
    show_tail "$WORK/submit.err"
    exit 1
  fi

  local commit
  commit="$(git rev-parse HEAD 2>/dev/null || true)"
  [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || commit=""
  # 모든 값을 위에서 형식 검사했으므로 따옴표·역슬래시가 들어갈 수 없다
  printf '{\n  "id": "%s",\n  "version": "%s",\n  "zip": "%s",\n  "sha256": "%s",\n  "commit": "%s",\n  "submitted_at": "%s"\n}\n' \
    "$id" "$VERSION" "$signed" "$sha" "$commit" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$NOTARY/submission.json"
  echo "공증 제출: $id"
  echo "서명된 zip: $NOTARY/$signed (sha256 $sha)"
}

# Developer ID 서명이 맞고 팀이 APPLE_TEAM_ID 인지
check_signature() {
  codesign --verify --deep --strict --verbose=2 "$APP"
  local team
  team="$(codesign -dv "$APP" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
  [[ "$team" == "$APPLE_TEAM_ID" ]] || { echo "서명된 앱의 TeamIdentifier 가 APPLE_TEAM_ID 와 다릅니다" >&2; exit 1; }
}

finish() {
  local json="$NOTARY/submission.json"
  [[ -f "$json" ]] || { echo "$json 이 없습니다. 1단계(submit) artifact 를 $NOTARY 에 받아 주세요" >&2; exit 1; }
  local recorded version signed sha
  recorded="$(json_field "$json" id)"
  version="$(json_field "$json" version)"
  signed="$(json_field "$json" zip)"
  sha="$(json_field "$json" sha256)"
  is_uuid "$recorded" || { echo "submission.json 의 id 형식 오류" >&2; exit 1; }
  is_version "$version" || { echo "submission.json 의 version 형식 오류" >&2; exit 1; }
  [[ "$signed" == "Bulchimbeon-$version-signed.zip" ]] || { echo "submission.json 의 zip 이름이 버전과 맞지 않습니다" >&2; exit 1; }
  [[ -f "$NOTARY/$signed" ]] || { echo "$NOTARY/$signed 이 없습니다" >&2; exit 1; }
  [[ "$sha" =~ ^[0-9a-f]{64}$ && "$(sha256_of "$NOTARY/$signed")" == "$sha" ]] \
    || { echo "서명된 zip 의 sha256 이 submission.json 과 다릅니다 (signed zip does not match submission.json)" >&2; exit 1; }
  (cd "$NOTARY" && shasum -a 256 -c "$signed.sha256" >/dev/null) \
    || { echo "서명된 zip 의 sha256 이 $signed.sha256 과 다릅니다" >&2; exit 1; }

  local id="${1:-$recorded}"
  is_uuid "$id" || { echo "제출 id 형식 오류 (submission id must be a UUID)" >&2; exit 1; }
  [[ "$id" == "$recorded" ]] || echo "지정한 제출 id 를 씁니다 (override): $id - artifact 에 적힌 id 는 $recorded"
  echo "공증 확인: submission $id, 버전 $version"

  # 상태 조회. 일시적인 실패는 세 번까지 다시 묻는다
  local status="" try
  for try in 1 2 3; do
    if xcrun notarytool info "$id" "${NOTARY_AUTH[@]}" --output-format json > "$WORK/info.json" 2> "$WORK/info.err"; then
      status="$(json_field "$WORK/info.json" status)"
      break
    fi
    echo "공증 상태 조회 실패 (notarytool info failed, $try/3) - submission $id" >&2
    ((try < 3)) && sleep "${NOTARY_RETRY_SECONDS:-20}"
  done
  if [[ -z "$status" ]]; then
    show_tail "$WORK/info.err"
    echo "공증 상태를 조회하지 못했습니다 (could not read the notarization status) - submission $id; 나중에 다시 실행하세요" >&2
    exit 1
  fi
  echo "공증 상태: $status (submission $id)"
  case "$status" in
    Accepted) ;;
    "In Progress")
      echo "공증이 아직 진행 중입니다 (still in progress) - submission $id. 아무것도 바꾸지 않았습니다; Apple 이 끝낸 뒤 2단계를 다시 실행하세요 (re-run phase 2 later)" >&2
      exit "$EX_IN_PROGRESS" ;;
    *)
      echo "공증이 거절되었습니다 (notarization $status) - submission $id. 공증 로그:" >&2
      # notarytool log 에는 검증한 UUID 만 넘긴다
      xcrun notarytool log "$id" "${NOTARY_AUTH[@]}" || true
      exit 1 ;;
  esac

  # 4. 1단계에서 서명·제출한 바로 그 앱을 풀어 공증 티켓을 붙이고 (오프라인에서도 Gatekeeper 통과) 확인
  rm -rf "$APP"
  ditto -x -k "$NOTARY/$signed" dist
  [[ -d "$APP" ]] || { echo "서명된 zip 에 Bulchimbeon.app 이 없습니다" >&2; exit 1; }
  check_signature
  xcrun stapler staple "$APP"
  xcrun stapler validate "$APP"
  codesign --verify --deep --strict --verbose=2 "$APP"
  spctl --assess --type execute --verbose=2 "$APP" 2>&1 | tee "$WORK/spctl.txt"
  grep -q "source=Notarized Developer ID" "$WORK/spctl.txt" \
    || { echo "Gatekeeper 가 공증된 Developer ID 로 보지 않습니다" >&2; exit 1; }

  # 5. staple 된 앱으로 build-app.sh 와 같은 이름·방식의 zip·체크섬을 만든다 (cask·업데이트가 보는 파일)
  local zip="Bulchimbeon-$version.zip"
  (cd dist && rm -f "$zip" "$zip.sha256" && ditto -c -k --keepParent Bulchimbeon.app "$zip" && shasum -a 256 "$zip" > "$zip.sha256")
  echo "✅ 공증·staple 완료: dist/$zip"
}

if [[ "$MODE" == submit ]]; then submit; else finish "${2:-}"; fi
