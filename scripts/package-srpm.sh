#!/bin/sh
# Build the source rpm for Fedora COPR from a published release, without compiling anything:
#
#   <out>/bulchimbeon-<version>-1.src.rpm   packaging/rpm/bulchimbeon.spec with the release's amd64 and arm64 tarballs
#
#   ./scripts/package-srpm.sh 0.9.0 [dist/rpm]     (run from the repository root; .copr/Makefile runs it on COPR)
#
# Downloads the two tarballs and SHA256SUMS from the GitHub release over HTTPS and stops unless
# the tarballs match SHA256SUMS. Their build-provenance attestations are verified separately,
# with `gh attestation verify` (scripts/bump-aur.sh does it for the same tarballs).
set -eu

[ $# -ge 1 ] || { echo "usage: $0 <version> [out-dir]" >&2; exit 2; }
version=$1
out=${2:-dist/rpm}
echo "$version" | grep -Eqx '[0-9]+\.[0-9]+\.[0-9]+' || { echo "version must be a release, X.Y.Z: $version" >&2; exit 2; }
[ -f packaging/rpm/bulchimbeon.spec ] || { echo "run from the repository root" >&2; exit 2; }
for tool in curl sha256sum rpmbuild; do
	command -v "$tool" >/dev/null 2>&1 || { echo "$tool not found (Fedora: dnf install curl rpm-build)" >&2; exit 2; }
done

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$out" "$work/SOURCES"
base="https://github.com/s3onghyun/bulchimbeon/releases/download/v$version"
amd64="bulchimbeon_${version}_linux_amd64.tar.gz"
arm64="bulchimbeon_${version}_linux_arm64.tar.gz"
for f in SHA256SUMS "$amd64" "$arm64"; do
	curl -fsSL --proto '=https' --tlsv1.2 -o "$work/SOURCES/$f" "$base/$f"
done
awk -v a="$amd64" -v b="$arm64" '{ f = $2; sub(/^\.\//, "", f) } f == a || f == b' \
	"$work/SOURCES/SHA256SUMS" >"$work/SOURCES/tarballs.sha256"
[ "$(wc -l <"$work/SOURCES/tarballs.sha256")" -eq 2 ] || { echo "SHA256SUMS does not list both tarballs" >&2; exit 1; }
(cd "$work/SOURCES" && sha256sum -c tarballs.sha256)

sed "s/^Version:        0\$/Version:        $version/" packaging/rpm/bulchimbeon.spec >"$work/bulchimbeon.spec"
grep -q "^Version:        $version\$" "$work/bulchimbeon.spec"
rpmbuild -bs --quiet \
	--define "_topdir $work" \
	--define "_sourcedir $work/SOURCES" \
	--define "_srcrpmdir $work" \
	"$work/bulchimbeon.spec" >/dev/null

srpm="bulchimbeon-${version}-1.src.rpm"
cp "$work/$srpm" "$out/$srpm"
ls -l "$out/$srpm"
