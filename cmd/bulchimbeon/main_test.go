package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")

	if code, out, _ := runCLI(t, "help"); code != exitOK || !strings.Contains(out, "Usage: bulchimbeon <command>") {
		t.Errorf("help = %d %q", code, out)
	}
	if code, _, _ := runCLI(t); code != exitUsage {
		t.Errorf("no args = %d", code)
	}
	if code, _, errOut := runCLI(t, "fly"); code != exitUsage || !strings.Contains(errOut, `unknown command "fly"`) {
		t.Errorf("unknown = %d %q", code, errOut)
	}
	if code, out, _ := runCLI(t, "version"); code != exitOK || out != "bulchimbeon dev\n" {
		t.Errorf("version = %d %q", code, out)
	}
	if code, _, _ := runCLI(t, "status", "--lang", "jp"); code != exitUsage {
		t.Errorf("bad --lang = %d", code)
	}

	if code, _, errOut := runCLI(t, "config", "set", "agentGraceMinutes", "10"); code != exitOK {
		t.Fatalf("config set = %d %q", code, errOut)
	}
	if code, out, _ := runCLI(t, "config", "get", "agentGraceMinutes"); code != exitOK || out != "10\n" {
		t.Errorf("config get = %d %q", code, out)
	}
	if code, _, errOut := runCLI(t, "config", "set", "agentGraceMinutes", "7"); code != exitFail || !strings.Contains(errOut, "1, 3, 5 or 10") {
		t.Errorf("config set 7 = %d %q", code, errOut)
	}
	runCLI(t, "config", "set", "language", "ko")
	if code, out, _ := runCLI(t, "status"); code != exitNotRunning || !strings.Contains(out, "불침번이 실행 중이 아닙니다") {
		t.Errorf("status (ko setting) = %d %q", code, out)
	}
	if code, out, _ := runCLI(t, "status", "--lang", "en"); code != exitNotRunning || !strings.Contains(out, "Bulchimbeon is not running") {
		t.Errorf("status --lang en = %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "bulchimbeon", "settings.json")); err != nil {
		t.Error(err)
	}
	if code, out, _ := runCLI(t, "config"); code != exitOK || !strings.Contains(out, "agentGraceMinutes      10") {
		t.Errorf("config list = %d %q", code, out)
	}
}

func TestChineseCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "LANGUAGE", "LC_ALL", "LC_MESSAGES"} {
		t.Setenv(key, "")
	}
	t.Setenv("LANG", "zh_CN.UTF-8")
	if code, out, errOut := runCLI(t, "help"); code != exitOK || !strings.Contains(out, "用法：bulchimbeon") {
		t.Fatalf("Chinese help = %d %q %q", code, out, errOut)
	}
	if code, out, _ := runCLI(t, "status"); code != exitNotRunning || !strings.Contains(out, "Bulchimbeon 未运行") {
		t.Fatalf("Chinese locale = %d %q", code, out)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if code, _, errOut := runCLI(t, "config", "set", "language", "zh"); code != exitOK {
		t.Fatal(errOut)
	}
	if code, out, _ := runCLI(t, "status"); code != exitNotRunning || !strings.Contains(out, "Bulchimbeon 未运行") {
		t.Fatalf("Chinese setting = %d %q", code, out)
	}
	if code, out, _ := runCLI(t, "status", "--lang", "en"); code != exitNotRunning || !strings.Contains(out, "Bulchimbeon is not running") {
		t.Fatalf("English override = %d %q", code, out)
	}
	if code, _, errOut := runCLI(t, "config", "set", "language", "en"); code != exitOK {
		t.Fatal(errOut)
	}
	if code, out, _ := runCLI(t, "status", "--lang", "zh"); code != exitNotRunning || !strings.Contains(out, "Bulchimbeon 未运行") {
		t.Fatalf("Chinese override = %d %q", code, out)
	}
}

// `tray` on a machine with no session bus (SSH, headless, a systemd service): one line, exit 0.
func TestTrayWithoutASessionBus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	code, out, errOut := runCLI(t, "tray")
	if code != exitOK || out != "" || strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, "no session bus") {
		t.Errorf("tray = %d %q %q", code, out, errOut)
	}
	if code, _, _ := runCLI(t, "tray", "now"); code != exitUsage {
		t.Errorf("tray now = %d", code)
	}
	if code, out, _ := runCLI(t, "help"); code != exitOK || !strings.Contains(out, "tray") {
		t.Errorf("help lacks tray: %q", out)
	}
}

// Every settings key shows its value in `config get`, including the defaults.
func TestConfigGetShowsEveryKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	for _, key := range settings.Keys {
		if key == engine.KeyCustomAgents {
			continue // empty by default
		}
		if code, out, _ := runCLI(t, "config", "get", key); code != exitOK || strings.TrimSpace(out) == "" {
			t.Errorf("config get %s = %d %q", key, code, out)
		}
	}
	if code, out, _ := runCLI(t, "config", "get", "xfceKeepAwake"); code != exitOK || out != "false\n" {
		t.Errorf("config get xfceKeepAwake = %d %q", code, out)
	}
	if code, _, errOut := runCLI(t, "config", "set", "xfceKeepAwake", "true"); code != exitOK {
		t.Fatal(errOut)
	}
	if code, out, _ := runCLI(t, "config", "get", "xfceKeepAwake"); code != exitOK || out != "true\n" {
		t.Errorf("config get xfceKeepAwake = %d %q", code, out)
	}
}

// The tray's uniform menu and `config set` share the uniformTheme key.
func TestUniformThemeSetting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	if code, _, errOut := runCLI(t, "config", "set", "uniformTheme", "desert"); code != exitOK {
		t.Fatal(errOut)
	}
	if code, out, _ := runCLI(t, "config", "get", "uniformTheme"); code != exitOK || out != "desert\n" {
		t.Errorf("config get = %d %q", code, out)
	}
	if code, _, errOut := runCLI(t, "config", "set", "uniformTheme", "camo"); code != exitFail || !strings.Contains(errOut, "default, navy, arctic or desert") {
		t.Errorf("config set camo = %d %q", code, errOut)
	}
}
