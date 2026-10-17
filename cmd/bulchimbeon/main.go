// Command bulchimbeon keeps a Linux machine awake while AI coding agents are working, and
// lets it sleep once they go quiet. It is the command-line sibling of the macOS menu-bar app
// and shares its engine through internal/engine.
//
//	bulchimbeon run        agent watch daemon (the systemd user unit runs this)
//	bulchimbeon status     is the machine being kept awake, and why
//	bulchimbeon doctor     check systemd, logind and polkit on this machine
//	bulchimbeon config     show or change settings
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/daemon"
	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
)

// version is set by the release build (-ldflags "-X main.version=…").
var version = "dev"

// Exit codes. status follows the LSB convention: 3 means not running.
const (
	exitOK         = 0
	exitFail       = 1
	exitUsage      = 2
	exitNotRunning = 3
)

func main() {
	os.Exit(cli(os.Args[1:], os.Stdout, os.Stderr))
}

func cli(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr, envLang())
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return runCmd(rest, stdout, stderr)
	case "status":
		return statusCmd(rest, stdout, stderr)
	case "doctor":
		return doctorCmd(rest, stdout, stderr)
	case "config":
		return configCmd(rest, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "bulchimbeon "+version)
		return exitOK
	case "help", "-h", "--help":
		usage(stdout, envLang())
		return exitOK
	}
	fmt.Fprintf(stderr, "bulchimbeon: unknown command %q\n\n", cmd)
	usage(stderr, envLang())
	return exitUsage
}

func usage(w io.Writer, lang engine.Lang) {
	fmt.Fprint(w, engine.T(lang, `불침번: AI 코딩 에이전트가 일하는 동안만 이 기기를 깨워 둡니다.

사용법: bulchimbeon <명령> [--lang ko|en|zh]

  run        에이전트 근무 데몬 (systemd 사용자 서비스가 실행)
  status     지금 깨워 두는 중인지, 왜 그런지
  doctor     systemd·logind·polkit 점검 (SSH·헤드리스용 polkit 규칙 안내)
  config     설정 보기·바꾸기: config [list | get <키> | set <키> <값>... | unset <키>]
  version    버전

설정 파일: ~/.config/bulchimbeon/settings.json · 근무일지: ~/.local/state/bulchimbeon/duty-log.txt
`, `Bulchimbeon keeps this machine awake only while AI coding agents are working.

Usage: bulchimbeon <command> [--lang ko|en|zh]

  run        agent watch daemon (the systemd user service runs this)
  status     whether the machine is being kept awake, and why
  doctor     check systemd, logind and polkit (and the polkit rule for SSH or headless use)
  config     show or change settings: config [list | get <key> | set <key> <value>... | unset <key>]
  version    print the version

Settings: ~/.config/bulchimbeon/settings.json · duty log: ~/.local/state/bulchimbeon/duty-log.txt
`, `Bulchimbeon 仅在 AI 编程代理工作期间保持设备唤醒。

用法：bulchimbeon <命令> [--lang ko|en|zh]

  run        运行代理值班守护进程（由 systemd 用户服务启动）
  status     查看设备是否保持唤醒及原因
  doctor     检查 systemd、logind 和 polkit（含 SSH 或无显示器环境的 polkit 规则说明）
  config     查看或修改设置：config [list | get <键> | set <键> <值>... | unset <键>]
  version    显示版本

设置：~/.config/bulchimbeon/settings.json · 值班日志：~/.local/state/bulchimbeon/duty-log.txt
`))
}

// common flags --------------------------------------------------------------

type common struct {
	fs   *flag.FlagSet
	lang string
}

func newFlags(name string, stderr io.Writer) *common {
	c := &common{fs: flag.NewFlagSet("bulchimbeon "+name, flag.ContinueOnError)}
	c.fs.SetOutput(stderr)
	c.fs.StringVar(&c.lang, "lang", "", "language: ko, en or zh (default: the language setting, then LANG)")
	return c
}

// parse reads flags; interspersed positional arguments are returned.
func (c *common) parse(args []string) ([]string, error) {
	var positional []string
	for {
		if err := c.fs.Parse(args); err != nil {
			return nil, err
		}
		if c.fs.NArg() == 0 {
			break
		}
		positional = append(positional, c.fs.Arg(0))
		args = c.fs.Args()[1:]
	}
	if c.lang != "" {
		if _, ok := engine.ParseLang(c.lang); !ok {
			return nil, fmt.Errorf("--lang must be ko, en or zh")
		}
	}
	return positional, nil
}

func (c *common) override() *engine.Lang {
	if l, ok := engine.ParseLang(c.lang); ok {
		return &l
	}
	return nil
}

func (c *common) langFor(s settings.Settings) engine.Lang {
	return daemon.ResolveLang(c.override(), s, os.Getenv)
}

func envLang() engine.Lang { return engine.DetectLanguage(engine.PreferredLanguages(os.Getenv)) }

func paths() (daemon.Paths, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return daemon.Paths{}, "", err
	}
	return daemon.DefaultPaths(os.Getenv, home), home, nil
}

func loadSettings(p daemon.Paths, stderr io.Writer) settings.Settings {
	s, problems, err := settings.Load(p.Settings)
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
	}
	for _, problem := range problems {
		fmt.Fprintln(stderr, "bulchimbeon: settings: ignored "+problem)
	}
	return s
}

// commands ------------------------------------------------------------------

func runCmd(args []string, stdout, stderr io.Writer) int {
	f := newFlags("run", stderr)
	f.fs.Bool("agent-watch", true, "start the agent watch right away (the only mode; accepted for compatibility)")
	if rest, err := f.parse(args); err != nil || len(rest) > 0 {
		return exitUsage
	}
	p, _, err := paths()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	plat, err := newPlatform()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	daemon.Run(ctx, daemon.Config{Platform: plat, Paths: p, Lang: f.override(), Out: stdout, PID: os.Getpid()})
	return exitOK
}

func statusCmd(args []string, stdout, stderr io.Writer) int {
	f := newFlags("status", stderr)
	if rest, err := f.parse(args); err != nil || len(rest) > 0 {
		return exitUsage
	}
	p, home, err := paths()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	in := daemon.StatusInput{StatePath: p.State, Lang: f.langFor(loadSettings(p, stderr)), Now: time.Now(), Home: home, Alive: alive}
	in.State, in.StateErr = daemon.ReadState(p.State)
	if in.StateErr != nil && !errors.Is(in.StateErr, os.ErrNotExist) {
		fmt.Fprintln(stderr, "bulchimbeon: "+in.StateErr.Error())
	}
	if plat, err := newPlatform(); err == nil {
		in.Holders, in.HoldersErr = plat.Holders.Holders()
	} else {
		in.HoldersErr = err
	}
	if in.State != nil {
		in.LogTail = tail(in.State.LogPath, 3)
	}
	text, running := daemon.RenderStatus(in)
	fmt.Fprint(stdout, text)
	if !running {
		return exitNotRunning
	}
	return exitOK
}

func doctorCmd(args []string, stdout, stderr io.Writer) int {
	f := newFlags("doctor", stderr)
	if rest, err := f.parse(args); err != nil || len(rest) > 0 {
		return exitUsage
	}
	p, home, err := paths()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	plat, err := newPlatform()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	s := loadSettings(p, stderr)
	lang := f.langFor(s)
	ok := plat.Doctor(stdout, lang)
	fmt.Fprintln(stdout, "  - "+engine.T(lang, "설정: ", "settings: ", "设置：")+daemon.Tilde(p.Settings, home))
	fmt.Fprintln(stdout, "  - "+engine.T(lang, "근무일지: ", "duty log: ", "值班日志：")+daemon.Tilde(p.Log, home))
	if !ok {
		return exitFail
	}
	return exitOK
}

func configCmd(args []string, stdout, stderr io.Writer) int {
	f := newFlags("config", stderr)
	rest, err := f.parse(args)
	if err != nil {
		return exitUsage
	}
	p, _, err := paths()
	if err != nil {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	if len(rest) == 0 {
		rest = []string{"list"}
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "bulchimbeon: "+err.Error())
		return exitFail
	}
	switch op := rest[0]; {
	case op == "list" && len(rest) == 1:
		s := loadSettings(p, stderr)
		for _, key := range settings.Keys {
			fmt.Fprintf(stdout, "%-22s %s\n", key, show(s, key))
		}
		fmt.Fprintln(stdout, "\n"+p.Settings)
	case op == "get" && len(rest) == 2:
		if !contains(settings.Keys, rest[1]) {
			return fail(fmt.Errorf("unknown key %q (keys: %s)", rest[1], strings.Join(settings.Keys, ", ")))
		}
		fmt.Fprintln(stdout, show(loadSettings(p, stderr), rest[1]))
	case op == "set" && len(rest) >= 2:
		if err := settings.Set(p.Settings, rest[1], rest[2:]); err != nil {
			return fail(err)
		}
	case op == "unset" && len(rest) == 2:
		if err := settings.Unset(p.Settings, rest[1]); err != nil {
			return fail(err)
		}
	default:
		fmt.Fprintln(stderr, "usage: bulchimbeon config [list | get <key> | set <key> <value>... | unset <key>]")
		return exitUsage
	}
	return exitOK
}

func show(s settings.Settings, key string) string {
	switch key {
	case engine.KeyDutyMode:
		return s.DutyMode
	case engine.KeyBatteryFloor:
		return fmt.Sprint(s.BatteryFloor)
	case engine.KeyAgentGraceMinutes:
		return fmt.Sprint(s.AgentGraceMinutes)
	case engine.KeyAgentWatchCountsIdle:
		return fmt.Sprint(s.AgentWatchCountsIdle)
	case engine.KeyCustomAgents:
		return strings.Join(s.CustomAgents, " ")
	case settings.KeyLanguage:
		return s.Language
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// tail returns the last n lines of a file (the duty log), without reading all of it.
func tail(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const chunk = 8 << 10
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	start := max(info.Size()-chunk, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line may be cut
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}
