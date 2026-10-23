#!/usr/bin/env bash
# 릴리스의 리눅스 tarball 두 개를 체크섬·출처 증명으로 검증한 뒤에만
# packaging/aur/bulchimbeon-bin 의 PKGBUILD 와 .SRCINFO 를 그 버전으로 갱신한다.
# 어디에도 push 하지 않는다: AUR 에 올리는 것은 packaging/aur/README.md 의 절차로 소유자가 한다.
# macOS·Linux 모두에서 돈다 (makepkg 불필요).
#   ./scripts/bump-aur.sh 0.9.0
set -euo pipefail

VERSION="${1:?usage: bump-aur.sh <version>}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "버전 형식 오류: $VERSION" >&2; exit 1; }
REPO=s3onghyun/bulchimbeon
DIR="$(cd "$(dirname "$0")/.." && pwd)/packaging/aur/bulchimbeon-bin"
PKGBUILD="$DIR/PKGBUILD"
SRCINFO="$DIR/.SRCINFO"
[[ -f "$PKGBUILD" && -f "$SRCINFO" ]] || { echo "$DIR 에 PKGBUILD·.SRCINFO 가 없음" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

sha256_check() { # 체크섬 파일을 WORK 에서 검증
  if command -v sha256sum >/dev/null; then (cd "$WORK" && sha256sum -c "$1")
  else (cd "$WORK" && shasum -a 256 -c "$1"); fi
}
sha_of() { # 체크섬 파일에서 해당 파일의 sha256
  awk -v f="$2" '{ g = $2; sub(/^\.\//, "", g) } g == f { print $1 }' "$WORK/$1"
}
edit() { # sed -i 는 BSD 와 GNU 의 문법이 달라 임시 파일로 바꾼다
  local file="$1"; shift
  sed -E "$@" "$file" > "$file.tmp" && mv "$file.tmp" "$file"
}

# 릴리스 파일을 받아 체크섬·출처 증명을 검증한 뒤에만 PKGBUILD 에 반영
AMD64="bulchimbeon_${VERSION}_linux_amd64.tar.gz"
ARM64="bulchimbeon_${VERSION}_linux_arm64.tar.gz"
gh release download "v$VERSION" -R "$REPO" -D "$WORK" -p "$AMD64" -p "$ARM64" -p SHA256SUMS
awk -v a="$AMD64" -v b="$ARM64" '{ f = $2; sub(/^\.\//, "", f) } f == a || f == b' \
  "$WORK/SHA256SUMS" > "$WORK/linux.sha256"
[[ "$(wc -l < "$WORK/linux.sha256")" -eq 2 ]] || { echo "SHA256SUMS 에 리눅스 tarball 이 없음" >&2; exit 1; }
sha256_check linux.sha256
for f in "$AMD64" "$ARM64"; do
  gh attestation verify "$WORK/$f" --repo "$REPO" >/dev/null
done
SHA_AMD64="$(sha_of linux.sha256 "$AMD64")"
SHA_ARM64="$(sha_of linux.sha256 "$ARM64")"
[[ "$SHA_AMD64" =~ ^[0-9a-f]{64}$ && "$SHA_ARM64" =~ ^[0-9a-f]{64}$ ]] || { echo "sha256 를 읽지 못함" >&2; exit 1; }

# PKGBUILD: source 는 $pkgver 를 쓰므로 pkgver·pkgrel·sha256sums 만 바꾼다
edit "$PKGBUILD" \
  -e "s/^pkgver=.*/pkgver=$VERSION/" \
  -e "s/^pkgrel=.*/pkgrel=1/" \
  -e "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('$SHA_AMD64')/" \
  -e "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('$SHA_ARM64')/"
# .SRCINFO: makepkg --printsrcinfo 가 쓰는 것과 같은 줄 (source URL 의 버전도)
edit "$SRCINFO" \
  -e "s/^(	pkgver = ).*/\1$VERSION/" \
  -e "s/^(	pkgrel = ).*/\11/" \
  -e "/^	source_(x86_64|aarch64) = /s/[0-9]+\.[0-9]+\.[0-9]+/$VERSION/g" \
  -e "s/^(	sha256sums_x86_64 = ).*/\1$SHA_AMD64/" \
  -e "s/^(	sha256sums_aarch64 = ).*/\1$SHA_ARM64/"

grep -qx "pkgver=$VERSION" "$PKGBUILD" && grep -qx "pkgrel=1" "$PKGBUILD" \
  && grep -qx "sha256sums_x86_64=('$SHA_AMD64')" "$PKGBUILD" \
  && grep -qx "sha256sums_aarch64=('$SHA_ARM64')" "$PKGBUILD" \
  || { echo "PKGBUILD 갱신 실패" >&2; exit 1; }
grep -qx "	pkgver = $VERSION" "$SRCINFO" \
  && grep -qx "	source_x86_64 = https://github.com/$REPO/releases/download/v$VERSION/$AMD64" "$SRCINFO" \
  && grep -qx "	source_aarch64 = https://github.com/$REPO/releases/download/v$VERSION/$ARM64" "$SRCINFO" \
  && grep -qx "	sha256sums_x86_64 = $SHA_AMD64" "$SRCINFO" \
  && grep -qx "	sha256sums_aarch64 = $SHA_ARM64" "$SRCINFO" \
  || { echo ".SRCINFO 갱신 실패" >&2; exit 1; }

echo "✅ bulchimbeon-bin → $VERSION-1 (x86_64 $SHA_AMD64, aarch64 $SHA_ARM64)"
echo "   검토 후 커밋하고, AUR 저장소에 복사해 push: packaging/aur/README.md"
