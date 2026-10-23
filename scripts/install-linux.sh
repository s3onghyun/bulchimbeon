#!/bin/sh
# Install Bulchimbeon for the current user, without root and without the network:
# the binary goes to ~/.local/bin, the systemd user unit to ~/.config/systemd/user and the
# tray icon's autostart entry to ~/.config/autostart, then the service is enabled and (re)started.
#
#   ./install-linux.sh                     from an extracted release tarball
#   ./scripts/install-linux.sh ./bulchimbeon   from a checkout, after go build ./cmd/bulchimbeon
#   ./install-linux.sh --uninstall         stop, disable and remove (settings and duty log stay)
set -eu

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
bin_dir="$HOME/.local/bin"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
unit="$unit_dir/bulchimbeon.service"
autostart="${XDG_CONFIG_HOME:-$HOME/.config}/autostart/bulchimbeon-tray.desktop"

fail() { echo "install-linux.sh: $*" >&2; exit 1; }

[ "$(uname -s)" = Linux ] || fail "this installs the Linux daemon; on macOS use the menu-bar app"
command -v systemctl >/dev/null 2>&1 || fail "systemctl not found (systemd is required)"

if [ "${1:-}" = --uninstall ]; then
	systemctl --user disable --now bulchimbeon.service 2>/dev/null || true
	rm -f "$unit" "$bin_dir/bulchimbeon" "$autostart"
	systemctl --user daemon-reload
	echo "Removed $bin_dir/bulchimbeon, $unit and $autostart."
	echo "Kept your settings (~/.config/bulchimbeon) and duty log (~/.local/state/bulchimbeon)."
	exit 0
fi

binary=${1:-$here/bulchimbeon}
[ -f "$binary" ] || fail "binary not found: $binary (build it with: go build ./cmd/bulchimbeon)"
if [ -f "$here/bulchimbeon.service" ]; then
	unit_src="$here/bulchimbeon.service" # release tarball
	tray_src="$here/bulchimbeon-tray.desktop"
else
	unit_src="$here/../packaging/linux/bulchimbeon.service" # repository checkout
	tray_src="$here/../packaging/linux/bulchimbeon-tray.desktop"
fi
[ -f "$unit_src" ] || fail "unit file not found next to this script"
[ -f "$tray_src" ] || fail "tray autostart entry not found next to this script"

install -D -m 0755 "$binary" "$bin_dir/bulchimbeon"
install -D -m 0644 "$unit_src" "$unit"
systemctl --user daemon-reload
systemctl --user enable bulchimbeon.service
systemctl --user restart bulchimbeon.service

# The tray icon starts with the desktop session. A desktop entry's Exec has its own quoting
# rules, so only a plain path is written into it.
case $bin_dir in
*[!A-Za-z0-9._/+@-]*)
	tray_note="Note: no tray autostart entry ($bin_dir has characters an autostart entry would need quoted); start \"bulchimbeon tray\" from your desktop's startup settings."
	;;
*)
	mkdir -p "$(dirname -- "$autostart")"
	sed -e "s|^TryExec=bulchimbeon\$|TryExec=$bin_dir/bulchimbeon|" -e "s|^Exec=bulchimbeon tray\$|Exec=$bin_dir/bulchimbeon tray|" "$tray_src" >"$autostart"
	chmod 0644 "$autostart"
	grep -q "^TryExec=$bin_dir/bulchimbeon\$" "$autostart" || fail "could not write $autostart"
	grep -q "^Exec=$bin_dir/bulchimbeon tray\$" "$autostart" || fail "could not write $autostart"
	tray_note="The tray icon starts at your next login ($autostart); to show it now: bulchimbeon tray &"
	;;
esac

echo "Installed $bin_dir/bulchimbeon and $unit; the service is running."
echo "$tray_note"
echo "  bulchimbeon status     is the machine kept awake, and why"
echo "  bulchimbeon doctor     check the desktop session, logind and polkit (SSH or headless machines need a polkit rule)"
echo "  journalctl --user -u bulchimbeon -f"
case ":$PATH:" in *":$bin_dir:"*) ;; *) echo "Note: $bin_dir is not in your PATH." ;; esac
if [ "$(loginctl show-user "$(id -u)" --property=Linger --value 2>/dev/null || true)" = no ]; then
	echo "Note: the service stops when you log out. Keep it running with: loginctl enable-linger"
fi
