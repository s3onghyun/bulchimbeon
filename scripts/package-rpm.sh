#!/bin/sh
# Build the release .rpm for one architecture from the tarball scripts/package-linux.sh wrote,
# with plain `rpmbuild` (Ubuntu's `rpm` package on the release runner; no third-party tool):
#
#   <out>/bulchimbeon-<version>-1.<x86_64|aarch64>.rpm   same files as the .deb (packaging/rpm/bulchimbeon.spec)
#
#   ./scripts/package-linux.sh 0.9.0 amd64 && ./scripts/package-rpm.sh 0.9.0 amd64   (from the repository root)
#
# The binary is the one in the tarball, so the .rpm, .deb and tarball ship the same bytes.
# Builds are reproducible for a given commit, like package-linux.sh: file times and the build
# time come from SOURCE_DATE_EPOCH (default: the commit time).
set -eu

[ $# -ge 2 ] || { echo "usage: $0 <version> <amd64|arm64> [out-dir]" >&2; exit 2; }
version=$1
arch=$2
out=${3:-dist/linux}
case $version in
[0-9]*) ;;
*) echo "version must start with a digit: $version" >&2; exit 2 ;;
esac
# an rpm Version may not contain '-'
case $version in
*[!0-9A-Za-z.+~]*) echo "version may contain only letters, digits and . + ~: $version" >&2; exit 2 ;;
esac
case $arch in
amd64) rpmarch=x86_64 ;;
arm64) rpmarch=aarch64 ;;
*) echo "arch must be amd64 or arm64: $arch" >&2; exit 2 ;;
esac
[ -f go.mod ] && [ -f packaging/rpm/bulchimbeon.spec ] || { echo "run from the repository root" >&2; exit 2; }
tarball="$out/bulchimbeon_${version}_linux_${arch}.tar.gz"
[ -f "$tarball" ] || { echo "$tarball not found: run ./scripts/package-linux.sh $version $arch first" >&2; exit 2; }
command -v rpmbuild >/dev/null 2>&1 || { echo "rpmbuild not found (Debian/Ubuntu: apt install rpm)" >&2; exit 2; }

SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct)}
export SOURCE_DATE_EPOCH
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/SOURCES" "$work/RPMS" "$work/BUILD" "$work/tmp"

# The spec lists both tarballs; `rpmbuild -bb` reads only this architecture's.
cp "$tarball" "$work/SOURCES/"
sed "s/^Version:        0\$/Version:        $version/" packaging/rpm/bulchimbeon.spec >"$work/bulchimbeon.spec"
grep -q "^Version:        $version\$" "$work/bulchimbeon.spec"

# --target only sets the package architecture: nothing is compiled.
rpmbuild -bb --quiet --target "$rpmarch" \
	--define "_topdir $work" \
	--define "_tmppath $work/tmp" \
	--define "_buildhost reproducible" \
	--define "use_source_date_epoch_as_buildtime 1" \
	--define "clamp_mtime_to_source_date_epoch 1" \
	--define "_binary_payload w9.xzdio" \
	"$work/bulchimbeon.spec"

rpm="bulchimbeon-${version}-1.${rpmarch}.rpm"
cp "$work/RPMS/$rpmarch/$rpm" "$out/$rpm"
ls -l "$out/$rpm"
