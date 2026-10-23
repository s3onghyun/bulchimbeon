# AUR: bulchimbeon-bin

`bulchimbeon-bin/PKGBUILD` repackages the static binary from the GitHub release tarballs for
x86_64 and aarch64 (the AUR requires the `-bin` suffix for prebuilt packages). It installs the same
files as the `.deb` and `.rpm`: `/usr/bin/bulchimbeon`, the systemd user unit in
`/usr/lib/systemd/user` (not enabled; each user runs `systemctl --user enable --now bulchimbeon`),
the tray's autostart entry in `/etc/xdg/autostart` (a `backup` file), README and LICENSE.
It depends on `systemd`; the tray hosts are optional dependencies.

`.SRCINFO` must match `makepkg --printsrcinfo` for the PKGBUILD; the AUR reads it, not the PKGBUILD.

## Each release

```sh
./scripts/bump-aur.sh X.Y.Z
```

It downloads the release's two Linux tarballs and `SHA256SUMS`, checks the checksums and the
build-provenance attestations (`gh attestation verify`, like `bump-cask.sh`), and only then sets
`pkgver`, `pkgrel=1` and both `sha256sums` in `PKGBUILD` and `.SRCINFO`. It does not need `makepkg`
and pushes nothing. Review the diff, commit it here, then copy both files to the AUR repository
(step 4 below). For a packaging-only fix, raise `pkgrel` in both files by hand.

## One-time setup (owner)

Nothing is published until the owner does this; the account is the owner's own.

1. Create an account at <https://aur.archlinux.org/register>.
2. Add an SSH public key to the account (My Account → SSH Public Key), and in `~/.ssh/config`:

   ```
   Host aur.archlinux.org
     IdentityFile ~/.ssh/aur
     User aur
   ```

3. Clone the (new, empty) package repository; the first push creates the package:

   ```sh
   git clone ssh://aur@aur.archlinux.org/bulchimbeon-bin.git
   ```

4. Copy the two files, then commit and push (repeat this step for every release):

   ```sh
   cp packaging/aur/bulchimbeon-bin/PKGBUILD packaging/aur/bulchimbeon-bin/.SRCINFO bulchimbeon-bin/
   cd bulchimbeon-bin
   git add PKGBUILD .SRCINFO && git commit -m "Update to X.Y.Z" && git push origin HEAD:master   # the AUR accepts only master
   ```

Users then install with an AUR helper (`yay -S bulchimbeon-bin`) or with
`git clone https://aur.archlinux.org/bulchimbeon-bin.git && cd bulchimbeon-bin && makepkg -si`, and enable it
with `systemctl --user enable --now bulchimbeon`.
