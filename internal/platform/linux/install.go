//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/s3onghyun/bulchimbeon/internal/update"
)

// InstallMethod says how the running binary was installed, so `status` and `doctor` can show
// the right update command. It only looks at files; no package manager is run.
func InstallMethod() string {
	exe, err := os.Executable() // /proc/self/exe, symlinks already resolved
	if err != nil {
		return update.InstallManual
	}
	return installMethod(exe, "/")
}

// installMethod: a Homebrew Cellar path, else for /usr/bin/bulchimbeon the package database
// that lists it (dpkg, then pacman for the AUR package, then an rpm database), else the
// tarball's install script.
func installMethod(exe, root string) string {
	if strings.Contains(exe, "/Cellar/bulchimbeon-linux/") {
		return update.InstallHomebrew
	}
	if exe != "/usr/bin/bulchimbeon" {
		return update.InstallManual
	}
	at := func(path string) string { return filepath.Join(root, path) }
	if _, err := os.Stat(at("var/lib/dpkg/info/bulchimbeon.list")); err == nil {
		return update.InstallDeb
	}
	if found, _ := filepath.Glob(at("var/lib/pacman/local/bulchimbeon-bin-*")); len(found) > 0 {
		return update.InstallAUR
	}
	for _, db := range []string{"usr/lib/sysimage/rpm", "var/lib/rpm"} {
		if info, err := os.Stat(at(db)); err == nil && info.IsDir() {
			return update.InstallRPM
		}
	}
	return update.InstallManual
}
