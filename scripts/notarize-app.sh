#!/usr/bin/env bash
# build-app.sh 가 만든 dist/Bulchimbeon.app 을 Developer ID 로 다시 서명 → Apple 공증(notarytool) → staple
# → 같은 이름의 zip·체크섬을 다시 만든다. 릴리스 워크플로가 서명 비밀값이 있을 때만 부른다 (docs/release.md).
# 값은 모두 환경 변수로 받고 출력하지 않는다. set -x 를 켜지 말 것.
#   VERSION                          build-app.sh 와 같은 버전 (zip 이름)
#   APPLE_DEVELOPER_ID_P12           Developer ID Application 인증서 + 개인 키 .p12 의 base64
#   APPLE_DEVELOPER_ID_P12_PASSWORD  그 .p12 를 내보낼 때 정한 암호
#   APPLE_TEAM_ID                    팀 ID (영대문자·숫자 10자)
#   공증 인증은 둘 중 하나 (API 키 쪽이 하나라도 있으면 API 키):
#     APPLE_API_KEY_ID + APPLE_API_ISSUER_ID + APPLE_API_KEY_P8 (.p8 의 base64)
#     APPLE_ID + APPLE_APP_SPECIFIC_PASSWORD
#   NOTARY_WAIT_MINUTES              공증 결과를 기다리는 최대 시간 (분, 기본 90). 넘으면 제출 id 를 남기고 실패
#   NOTARY_POLL_SECONDS              상태를 묻는 간격 (초, 기본 30. 시험용으로만 바꾼다)
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${VERSION:?VERSION 이 필요합니다}"
APP="dist/Bulchimbeon.app"
ZIP="Bulchimbeon-$VERSION.zip"

missing=()
need() { local v; for v in "$@"; do [[ -n "${!v:-}" ]] || missing+=("$v"); done; }
need APPLE_DEVELOPER_ID_P12 APPLE_DEVELOPER_ID_P12_PASSWORD APPLE_TEAM_ID
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
NOTARY_WAIT_MINUTES="${NOTARY_WAIT_MINUTES:-90}"
NOTARY_POLL_SECONDS="${NOTARY_POLL_SECONDS:-30}"
[[ "$NOTARY_WAIT_MINUTES" =~ ^[1-9][0-9]{0,3}$ ]] \
  || { echo "NOTARY_WAIT_MINUTES 는 1 이상의 정수(분)여야 합니다 (must be a positive number of minutes)" >&2; exit 1; }
[[ "$NOTARY_POLL_SECONDS" =~ ^[1-9][0-9]{0,2}$ ]] \
  || { echo "NOTARY_POLL_SECONDS 는 1~999 초여야 합니다" >&2; exit 1; }
[[ "$(uname -s)" == Darwin ]] || { echo "macOS 에서만 돕니다" >&2; exit 1; }
[[ -d "$APP" ]] || { echo "$APP 이 없습니다. 먼저 ./scripts/build-app.sh" >&2; exit 1; }
echo "공증 인증 방식: $AUTH"

WORK="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/bulchimbeon-notarize.XXXXXX")"
KEYCHAIN="$WORK/signing.keychain-db"
OLD_KEYCHAINS=()
while IFS= read -r line; do
  line="${line#"${line%%[![:space:]]*}"}"; line="${line#\"}"; line="${line%\"}"
  [[ -n "$line" ]] && OLD_KEYCHAINS+=("$line")
done < <(security list-keychains -d user)
cleanup() {
  security list-keychains -d user -s ${OLD_KEYCHAINS[@]+"${OLD_KEYCHAINS[@]}"} 2>/dev/null || true
  security delete-keychain "$KEYCHAIN" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# 1. 이 실행에서만 쓰는 임시 키체인에 인증서를 넣는다 (로그인 키체인은 건드리지 않음)
KEYCHAIN_PASSWORD="$(uuidgen)"
security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
security set-keychain-settings -lut 3600 "$KEYCHAIN"
security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
printf '%s' "$APPLE_DEVELOPER_ID_P12" | base64 --decode > "$WORK/developer-id.p12"
security import "$WORK/developer-id.p12" -k "$KEYCHAIN" -f pkcs12 -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -T /usr/bin/codesign >/dev/null
rm -f "$WORK/developer-id.p12"
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN" >/dev/null
security list-keychains -d user -s "$KEYCHAIN" ${OLD_KEYCHAINS[@]+"${OLD_KEYCHAINS[@]}"}

# 팀 ID 가 맞는 Developer ID Application 인증서의 SHA-1 (이름이 겹쳐도 정확히 그 인증서로 서명)
IDENTITY="$(security find-identity -v -p codesigning "$KEYCHAIN" \
  | awk -v team="($APPLE_TEAM_ID)\"" 'index($0, "\"Developer ID Application: ") && index($0, team) { print $2; exit }')"
[[ -n "$IDENTITY" ]] || { echo "이 팀의 Developer ID Application 인증서가 .p12 에 없습니다" >&2; exit 1; }

# 2. Developer ID 서명: hardened runtime + 보안 타임스탬프 (공증 필수 조건). 서명 식별자는 ad-hoc 때와 같다
codesign --force --options runtime --timestamp --keychain "$KEYCHAIN" --sign "$IDENTITY" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
SIGNED_TEAM="$(codesign -dv "$APP" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
[[ "$SIGNED_TEAM" == "$APPLE_TEAM_ID" ]] || { echo "서명된 앱의 TeamIdentifier 가 APPLE_TEAM_ID 와 다릅니다" >&2; exit 1; }

# 3. 공증 제출 → 결과 대기 (최대 NOTARY_WAIT_MINUTES 분)
if [[ "$AUTH" == api-key ]]; then
  printf '%s' "$APPLE_API_KEY_P8" | base64 --decode > "$WORK/AuthKey.p8"
  NOTARY_AUTH=(--key "$WORK/AuthKey.p8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
else
  NOTARY_AUTH=(--apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_SPECIFIC_PASSWORD")
fi
ditto -c -k --keepParent "$APP" "$WORK/submit.zip"

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
is_uuid() { [[ "$1" =~ ^[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}$ ]]; }
elapsed() { printf '%d분 %02d초' $(((SECONDS - START) / 60)) $(((SECONDS - START) % 60)); }

# --wait 없이 제출해 id 부터 받는다. --wait --timeout 은 시간이 넘으면 id 없는 오류 JSON 만 남겨 (2026-11-17 첫 CI 실행),
# 처리가 느린 새 계정의 첫 제출들을 견디지 못했다. id 는 바로 출력해 실패해도 맥에서 조회할 수 있게 한다
xcrun notarytool submit "$WORK/submit.zip" "${NOTARY_AUTH[@]}" --output-format json \
  > "$WORK/submit.json" 2> "$WORK/submit.err" || true
SUBMISSION="$(plutil -extract id raw -o - "$WORK/submit.json" 2>/dev/null || true)"
if ! is_uuid "$SUBMISSION"; then
  echo "공증 제출 실패: 제출 id 를 받지 못했습니다 (notarization submit failed, no submission id). notarytool 출력 끝부분:" >&2
  show_tail "$WORK/submit.json"
  show_tail "$WORK/submit.err"
  exit 1
fi
echo "공증 제출: $SUBMISSION"

# 결과가 나올 때까지 NOTARY_POLL_SECONDS 마다 상태를 묻는다. 상태가 바뀔 때와 5분마다 한 줄씩 남긴다
START=$SECONDS
DEADLINE=$((START + NOTARY_WAIT_MINUTES * 60))
STATUS="" LAST_BEAT=$START FAILURES=0
while :; do
  if xcrun notarytool info "$SUBMISSION" "${NOTARY_AUTH[@]}" --output-format json \
    > "$WORK/info.json" 2> "$WORK/info.err"; then
    FAILURES=0
    NOW="$(plutil -extract status raw -o - "$WORK/info.json" 2>/dev/null || echo unknown)"
  else
    FAILURES=$((FAILURES + 1))
    echo "공증 상태 조회 실패 (notarytool info failed, $FAILURES/5) - submission $SUBMISSION" >&2
    if ((FAILURES >= 5)); then
      show_tail "$WORK/info.err"
      echo "공증 상태를 5번 연속 조회하지 못했습니다 (giving up) - submission $SUBMISSION; check with: xcrun notarytool info $SUBMISSION" >&2
      exit 1
    fi
    NOW="${STATUS:-unknown}"
  fi
  case "$NOW" in Accepted | Invalid | Rejected) STATUS="$NOW"; break ;; esac
  if [[ "$NOW" != "$STATUS" ]] || ((SECONDS - LAST_BEAT >= 300)); then
    echo "공증 대기 중 (waiting): $(elapsed) 경과, 상태 $NOW - submission $SUBMISSION"
    LAST_BEAT=$SECONDS
  fi
  STATUS="$NOW"
  REMAINING=$((DEADLINE - SECONDS))
  if ((REMAINING <= 0)); then
    echo "공증이 아직 진행 중입니다 (still in progress) - submission $SUBMISSION; re-run the job later or check with: xcrun notarytool info $SUBMISSION" >&2
    exit 1
  fi
  sleep $((REMAINING < NOTARY_POLL_SECONDS ? REMAINING : NOTARY_POLL_SECONDS))
done
echo "공증 결과: $STATUS (submission $SUBMISSION, $(elapsed))"
if [[ "$STATUS" != Accepted ]]; then
  # notarytool log 에는 검증한 UUID 만 넘긴다
  xcrun notarytool log "$SUBMISSION" "${NOTARY_AUTH[@]}" || true
  exit 1
fi

# 4. 공증 티켓을 앱에 붙이고 (오프라인에서도 Gatekeeper 통과) 확인
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
spctl --assess --type execute --verbose=2 "$APP" 2>&1 | tee "$WORK/spctl.txt"
grep -q "source=Notarized Developer ID" "$WORK/spctl.txt" \
  || { echo "Gatekeeper 가 공증된 Developer ID 로 보지 않습니다" >&2; exit 1; }

# 5. staple 된 앱으로 build-app.sh 와 같은 이름·방식의 zip·체크섬을 다시 만든다 (cask·업데이트가 보는 파일)
(cd dist && rm -f "$ZIP" "$ZIP.sha256" && ditto -c -k --keepParent Bulchimbeon.app "$ZIP" && shasum -a 256 "$ZIP" > "$ZIP.sha256")
echo "✅ 공증·staple 완료: dist/$ZIP"
