#!/bin/sh
# Build the Linux release artifacts for one architecture, with plain `go build` and
# `dpkg-deb` (no third-party packaging tool):
#
#   <out>/bulchimbeon_<version>_linux_<arch>.tar.gz   binary, user unit, tray autostart entry, install script, README, LICENSE
#   <out>/bulchimbeon_<version>_<arch>.deb            /usr/bin/bulchimbeon, /usr/lib/systemd/user unit and
#                                                     /etc/xdg/autostart tray entry
#
#   ./scripts/package-linux.sh 0.7.0 amd64 [dist/linux]     (run from the repository root)
#
# Builds are reproducible for a given commit: file times come from SOURCE_DATE_EPOCH
# (default: the commit time) and archives are written with fixed owners and order.
set -eu

[ $# -ge 2 ] || { echo "usage: $0 <version> <amd64|arm64> [out-dir]" >&2; exit 2; }
version=$1
arch=$2
out=${3:-dist/linux}
case $version in
[0-9]*) ;;
*) echo "version must start with a digit: $version" >&2; exit 2 ;;
esac
case $version in
*[!0-9A-Za-z.+~-]*) echo "version may contain only letters, digits and . + ~ -: $version" >&2; exit 2 ;;
esac
case $arch in
amd64 | arm64) ;;
*) echo "arch must be amd64 or arm64: $arch" >&2; exit 2 ;;
esac
[ -f go.mod ] && [ -f packaging/linux/bulchimbeon.service ] && [ -f packaging/linux/bulchimbeon-tray.desktop ] ||
	{ echo "run from the repository root" >&2; exit 2; }

SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct)}
export SOURCE_DATE_EPOCH
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
name="bulchimbeon_${version}_linux_${arch}"
mkdir -p "$out" "$work/$name"

CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" \
	-o "$work/$name/bulchimbeon" ./cmd/bulchimbeon

# tarball
cp packaging/linux/bulchimbeon.service packaging/linux/bulchimbeon-tray.desktop scripts/install-linux.sh README.md LICENSE "$work/$name/"
chmod 0755 "$work/$name" "$work/$name/bulchimbeon" "$work/$name/install-linux.sh"
chmod 0644 "$work/$name/bulchimbeon.service" "$work/$name/bulchimbeon-tray.desktop" "$work/$name/README.md" "$work/$name/LICENSE"
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$SOURCE_DATE_EPOCH" \
	-C "$work" -cf - "$name" | gzip -n -9 >"$out/$name.tar.gz"

# .deb: the user unit runs the system-wide binary; each user enables it themselves
deb="$work/deb"
install -D -m 0755 "$work/$name/bulchimbeon" "$deb/usr/bin/bulchimbeon"
install -d "$deb/usr/lib/systemd/user" "$deb/DEBIAN"
sed 's|^ExecStart=%h/.local/bin/bulchimbeon |ExecStart=/usr/bin/bulchimbeon |' packaging/linux/bulchimbeon.service \
	>"$deb/usr/lib/systemd/user/bulchimbeon.service"
grep -q '^ExecStart=/usr/bin/bulchimbeon run$' "$deb/usr/lib/systemd/user/bulchimbeon.service"
chmod 0644 "$deb/usr/lib/systemd/user/bulchimbeon.service"
# the tray starts with every desktop session; it exits at once where there is no tray host
install -d "$deb/etc/xdg/autostart"
sed 's|^Exec=bulchimbeon tray$|Exec=/usr/bin/bulchimbeon tray|' packaging/linux/bulchimbeon-tray.desktop \
	>"$deb/etc/xdg/autostart/bulchimbeon-tray.desktop"
grep -q '^Exec=/usr/bin/bulchimbeon tray$' "$deb/etc/xdg/autostart/bulchimbeon-tray.desktop"
chmod 0644 "$deb/etc/xdg/autostart/bulchimbeon-tray.desktop"
echo /etc/xdg/autostart/bulchimbeon-tray.desktop >"$deb/DEBIAN/conffiles"
chmod 0644 "$deb/DEBIAN/conffiles"
install -D -m 0644 LICENSE "$deb/usr/share/doc/bulchimbeon/copyright"
size=$(du -sk "$deb/usr" "$deb/etc" | awk '{n += $1} END {print n}')
cat >"$deb/DEBIAN/control" <<CONTROL
Package: bulchimbeon
Version: $version
Architecture: $arch
Maintainer: s3onghyun <s3onghyun@users.noreply.github.com>
Installed-Size: $size
Depends: systemd
Section: utils
Priority: optional
Homepage: https://github.com/s3onghyun/bulchimbeon
Description: keep the machine awake while AI coding agents work
 Bulchimbeon watches the current user's processes for AI coding agents
 (Claude Code, Codex, Gemini CLI and others) and holds a logind sleep
 inhibitor only while one of them is actually working.
 .
 Each user enables it with: systemctl --user enable --now bulchimbeon
 .
 Desktop sessions also start a tray icon (bulchimbeon tray) that shows the
 private on duty; it needs a StatusNotifierItem host such as the XFCE Status
 Tray, KDE Plasma or the GNOME AppIndicator extension.
CONTROL
find "$deb" -exec touch -h -d "@$SOURCE_DATE_EPOCH" {} +
dpkg-deb --root-owner-group -Zxz --build "$deb" "$out/bulchimbeon_${version}_${arch}.deb" >/dev/null

ls -l "$out/$name.tar.gz" "$out/bulchimbeon_${version}_${arch}.deb"
