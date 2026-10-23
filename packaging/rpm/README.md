# .rpm and Fedora COPR

`bulchimbeon.spec` repackages the static binary from a release tarball (nothing is compiled at
package time). It installs the same files as the `.deb`:

| Path | |
|------|-|
| `/usr/bin/bulchimbeon` | the binary from `bulchimbeon_<version>_linux_<amd64\|arm64>.tar.gz` |
| `/usr/lib/systemd/user/bulchimbeon.service` | user unit with `ExecStart=/usr/bin/bulchimbeon run`, **not enabled**: no preset, no scriptlets. Fedora requires FESCo approval for services enabled by default, so each user runs `systemctl --user enable --now bulchimbeon` |
| `/etc/xdg/autostart/bulchimbeon-tray.desktop` | tray autostart entry, `%config(noreplace)` like the `.deb`'s conffile |
| `/usr/share/doc/bulchimbeon/README.md`, `/usr/share/licenses/bulchimbeon/LICENSE` | |

`Version` in the spec is `0`; the build scripts set it from the release.

## Release .rpm

`.github/workflows/release.yml` runs, for amd64 and arm64:

```sh
./scripts/package-linux.sh X.Y.Z amd64    # tarball and .deb
./scripts/package-rpm.sh   X.Y.Z amd64    # bulchimbeon-X.Y.Z-1.x86_64.rpm from that tarball
```

`rpmbuild` comes from Ubuntu's own `rpm` package on the release runner (installed with `apt-get` from
Ubuntu's signed archive, like `dpkg-deb`), not from a container image or a downloaded tool. The `.rpm`
files go into `SHA256SUMS`, the build-provenance attestation and the release, next to the `.deb`.
`package-rpm.sh` is reproducible from `SOURCE_DATE_EPOCH`, like `package-linux.sh`.

Local install check (needs docker; a manual check, not run in CI). `package-rpm.sh` needs the tarball
that `package-linux.sh` wrote to `dist/linux` and `rpmbuild` (Ubuntu's `rpm` package); setting
`SOURCE_DATE_EPOCH` means it does not need git inside the container:

```sh
./scripts/package-linux.sh 0.0.0~dev amd64
docker run --rm -v "$PWD:/src" -w /src ubuntu:24.04 sh -c \
'apt-get update -q && apt-get install -y -q --no-install-recommends rpm && SOURCE_DATE_EPOCH=0 ./scripts/package-rpm.sh 0.0.0~dev amd64'
docker run --rm -v "$PWD/dist/linux:/rpms:ro" -w /rpms fedora:44 sh -euxc '
dnf -y -q install ./bulchimbeon-0.0.0~dev-1.x86_64.rpm
bulchimbeon version
rpm -V bulchimbeon
test "$(systemctl --global is-enabled bulchimbeon.service || true)" = disabled
rpm -e bulchimbeon
if rpm -q bulchimbeon; then exit 1; fi
test ! -e /usr/bin/bulchimbeon'
```

## Fedora COPR

COPR builds from a source rpm. `.copr/Makefile` (COPR's "make srpm" source method) runs
`scripts/package-srpm.sh`, which downloads the release's two tarballs and `SHA256SUMS` from GitHub,
stops unless the tarballs match, and builds `bulchimbeon-X.Y.Z-1.src.rpm`. COPR then builds the x86_64
and aarch64 rpms from it, each using its own architecture's tarball. The release is the `v*` tag
checked out, or `version=X.Y.Z`:

```sh
make -f .copr/Makefile srpm outdir=dist/rpm version=X.Y.Z   # locally, on Fedora (needs rpm-build and curl)
```

The script checks the tarballs against the release's `SHA256SUMS`. Verify their build provenance too
before each COPR build: `./scripts/bump-aur.sh X.Y.Z` runs `gh attestation verify` on the same tarballs.

### One-time setup (owner)

Nothing is published until the owner does this; the account is the owner's own.

1. Create a Fedora account at <https://accounts.fedoraproject.org> and log in to
   <https://copr.fedorainfracloud.org> once.
2. Install `copr-cli` (`sudo dnf install copr-cli`, or `pipx install copr-cli`) and save the API token
   from <https://copr.fedorainfracloud.org/api/> to `~/.config/copr`.
3. Create the project, with the current Fedora releases:

   ```sh
   copr-cli create bulchimbeon \
     --chroot fedora-43-x86_64 --chroot fedora-43-aarch64 \
     --chroot fedora-44-x86_64 --chroot fedora-44-aarch64 \
     --description "Keep the machine awake while AI coding agents work" \
     --instructions "sudo dnf copr enable s3onghyun/bulchimbeon && sudo dnf install bulchimbeon, then systemctl --user enable --now bulchimbeon"
   ```

### Each release

After the release workflow has published `vX.Y.Z`:

```sh
copr-cli buildscm bulchimbeon --clone-url https://github.com/s3onghyun/bulchimbeon.git \
  --commit vX.Y.Z --method make_srpm
```

or build the source rpm locally and upload it: `copr-cli build bulchimbeon dist/rpm/bulchimbeon-X.Y.Z-1.src.rpm`.

A COPR webhook is not used: COPR's tag webhook takes the package name from tags named
`<package>-<version>`, which the `vX.Y.Z` tags are not, and a push webhook would rebuild on every push
to `main`, where `.copr/Makefile` stops because `HEAD` is not a release tag.

Users then install with:

```sh
sudo dnf copr enable s3onghyun/bulchimbeon
sudo dnf install bulchimbeon
systemctl --user enable --now bulchimbeon
```
