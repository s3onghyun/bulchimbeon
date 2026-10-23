# Bulchimbeon (불침번) .rpm, repackaging the static binary from the release tarball for this
# architecture: nothing is compiled and nothing is downloaded while building.
#   scripts/package-rpm.sh   the release .rpm files, from the tarballs scripts/package-linux.sh just built
#   .copr/Makefile           the source rpm for Fedora COPR, from a published release's tarballs
# Both set Version from the release; see packaging/rpm/README.md.
#
# Same files as the .deb. The user unit is installed but not enabled (no preset, no scriptlets):
# each user runs `systemctl --user enable --now bulchimbeon` (Fedora requires FESCo approval
# for services enabled by default).

Name:           bulchimbeon
Version:        0
Release:        1
Summary:        Keep the machine awake while AI coding agents work
License:        MIT
URL:            https://github.com/s3onghyun/bulchimbeon
Source0:        %{url}/releases/download/v%{version}/bulchimbeon_%{version}_linux_amd64.tar.gz
Source1:        %{url}/releases/download/v%{version}/bulchimbeon_%{version}_linux_arm64.tar.gz
ExclusiveArch:  x86_64 aarch64
Requires:       systemd

# A prebuilt static Go binary, already stripped (-s -w): no debug package, no build-id links,
# no brp scripts (they would strip or rewrite it with the build host's tools).
%global debug_package %{nil}
%global _build_id_links none
%global __os_install_post %{nil}
# Fedora's doc and license paths (/usr/share/doc/bulchimbeon) also when built with Ubuntu's rpm
%global _docdir_fmt %{name}

%ifarch x86_64
%global goarch amd64
%endif
%ifarch aarch64
%global goarch arm64
%endif
%global tarname bulchimbeon_%{version}_linux_%{goarch}

%description
Bulchimbeon watches the current user's processes for AI coding agents
(Claude Code, Codex, Gemini CLI and others) and holds the desktop's
suspend inhibitor, or a logind sleep inhibitor without a desktop, only
while one of them is actually working.

Each user enables it with: systemctl --user enable --now bulchimbeon

Desktop sessions also start a tray icon (bulchimbeon tray) that shows the
private on duty; it needs a StatusNotifierItem host such as the XFCE Status
Tray, KDE Plasma or the GNOME AppIndicator extension.

%prep
%setup -q -T -c
%ifarch x86_64
tar -xzf %{SOURCE0}
%endif
%ifarch aarch64
tar -xzf %{SOURCE1}
%endif

%build
# nothing to build: the tarball holds the release binary

%install
cd %{tarname}
install -D -m 0755 bulchimbeon %{buildroot}%{_bindir}/bulchimbeon
# the user unit runs the system-wide binary; each user enables it themselves
install -d %{buildroot}%{_prefix}/lib/systemd/user
sed 's|^ExecStart=%%h/.local/bin/bulchimbeon |ExecStart=%{_bindir}/bulchimbeon |' bulchimbeon.service \
    >%{buildroot}%{_prefix}/lib/systemd/user/bulchimbeon.service
grep -q '^ExecStart=/usr/bin/bulchimbeon run$' %{buildroot}%{_prefix}/lib/systemd/user/bulchimbeon.service
chmod 0644 %{buildroot}%{_prefix}/lib/systemd/user/bulchimbeon.service
# the tray starts with every desktop session; it exits at once where there is no tray host
install -d %{buildroot}%{_sysconfdir}/xdg/autostart
sed -e 's|^TryExec=bulchimbeon$|TryExec=%{_bindir}/bulchimbeon|' -e 's|^Exec=bulchimbeon tray$|Exec=%{_bindir}/bulchimbeon tray|' \
    bulchimbeon-tray.desktop >%{buildroot}%{_sysconfdir}/xdg/autostart/bulchimbeon-tray.desktop
grep -q '^TryExec=/usr/bin/bulchimbeon$' %{buildroot}%{_sysconfdir}/xdg/autostart/bulchimbeon-tray.desktop
grep -q '^Exec=/usr/bin/bulchimbeon tray$' %{buildroot}%{_sysconfdir}/xdg/autostart/bulchimbeon-tray.desktop
chmod 0644 %{buildroot}%{_sysconfdir}/xdg/autostart/bulchimbeon-tray.desktop

%files
%license %{tarname}/LICENSE
%doc %{tarname}/README.md
%{_bindir}/bulchimbeon
%{_prefix}/lib/systemd/user/bulchimbeon.service
%config(noreplace) %{_sysconfdir}/xdg/autostart/bulchimbeon-tray.desktop
