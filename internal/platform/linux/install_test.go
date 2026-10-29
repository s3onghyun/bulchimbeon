//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s3onghyun/bulchimbeon/internal/update"
)

func TestInstallMethod(t *testing.T) {
	root := func(paths ...string) string {
		dir := t.TempDir()
		for _, p := range paths {
			full := filepath.Join(dir, p)
			if filepath.Ext(p) == "" {
				os.MkdirAll(full, 0o755)
			} else {
				os.MkdirAll(filepath.Dir(full), 0o755)
				os.WriteFile(full, nil, 0o644)
			}
		}
		return dir
	}
	for _, c := range []struct {
		name, exe, root, want string
	}{
		{"deb", "/usr/bin/bulchimbeon", root("var/lib/dpkg/info/bulchimbeon.list", "var/lib/rpm"), update.InstallDeb},
		{"AUR", "/usr/bin/bulchimbeon", root("var/lib/pacman/local/bulchimbeon-bin-0.10.2-1"), update.InstallAUR},
		{"rpm (Fedora)", "/usr/bin/bulchimbeon", root("usr/lib/sysimage/rpm"), update.InstallRPM},
		{"rpm (older)", "/usr/bin/bulchimbeon", root("var/lib/rpm"), update.InstallRPM},
		{"another package's dpkg record", "/usr/bin/bulchimbeon", root("var/lib/dpkg/info/other.list"), update.InstallManual},
		{"Homebrew", "/home/linuxbrew/.linuxbrew/Cellar/bulchimbeon-linux/0.10.2/bin/bulchimbeon", root(), update.InstallHomebrew},
		{"install script", "/home/me/.local/bin/bulchimbeon", root("var/lib/dpkg/info/bulchimbeon.list"), update.InstallManual},
		{"built from source", "/home/me/src/bulchimbeon/bulchimbeon", root(), update.InstallManual},
	} {
		if got := installMethod(c.exe, c.root); got != c.want {
			t.Errorf("%s: installMethod(%s) = %s, want %s", c.name, c.exe, got, c.want)
		}
	}
	if got := InstallMethod(); got != update.InstallManual { // the test binary
		t.Errorf("InstallMethod() = %s for the test binary", got)
	}
}
