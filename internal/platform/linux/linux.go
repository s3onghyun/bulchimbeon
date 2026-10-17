//go:build linux

package linux

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// New returns the Linux backend.
func New() platform.Platform {
	logind := NewLogind()
	return platform.Platform{
		Name:      "linux",
		Processes: NewProcScanner(),
		Inhibitor: NewSystemdInhibitor(logind),
		Battery:   NewPowerSupply(),
		Thermal:   NewThermal(),
		Holders:   logind,
		Doctor:    Doctor,
	}
}

// PolkitRulePath is where the optional rule for SSH and headless machines goes.
const PolkitRulePath = "/etc/polkit-1/rules.d/50-bulchimbeon.rules"

var userNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// PolkitRule is the optional rule that lets one user hold a logind "sleep" block inhibitor
// from an SSH session or a user service. polkit allows it by default only from a local
// login. Names that are not plain user names get a placeholder instead.
func PolkitRule(userName string) string {
	if !userNameRE.MatchString(userName) {
		userName = "YOUR-USER-NAME"
	}
	return `// Bulchimbeon: let ` + userName + ` hold a logind "sleep" block inhibitor from an SSH or
// systemd --user session (polkit's default only allows it from a local session).
// Grants nothing else. Remove this file to undo.
polkit.addRule(function (action, subject) {
  if (action.id == "org.freedesktop.login1.inhibit-block-sleep" && subject.user == "` + userName + `") return polkit.Result.YES;
});
`
}

// Doctor checks what Bulchimbeon needs on this machine. It only reads and tries short
// inhibitors; it never changes the system. It reports false if the sleep lock cannot work.
func Doctor(w io.Writer, lang engine.Lang) bool {
	t := func(ko, en, zh string) string { return engine.T(lang, ko, en, zh) }
	ok := true
	mark := func(good bool, text string) {
		if good {
			fmt.Fprintln(w, "  ✔ "+text)
		} else {
			fmt.Fprintln(w, "  ✘ "+text)
		}
	}
	fmt.Fprintln(w, t("불침번 점검 (doctor)", "Bulchimbeon doctor", "Bulchimbeon 诊断"))

	inhibit, trueTool := findTool("systemd-inhibit"), findTool("true")
	mark(inhibit != "", "systemd-inhibit: "+orNone(inhibit, t("없음 — systemd 가 필요합니다", "missing (systemd is required)", "未找到（需要 systemd）")))
	if inhibit == "" {
		return false
	}

	holders, err := NewLogind().Holders()
	if err != nil {
		mark(false, t("logind 조회 실패: ", "cannot list logind inhibitors: ", "无法列出 logind 休眠锁：")+err.Error())
		ok = false
	} else {
		mark(true, fmt.Sprintf(t("logind: 잠금 %d개 조회됨", "logind: %d inhibitors listed", "logind：已列出 %d 个休眠锁"), len(holders)))
	}

	try := func(what string) (bool, string) {
		_, err := run(5*time.Second, inhibit, "--what="+what, "--mode=block", "--who="+InhibitWho,
			"--why=Bulchimbeon doctor check", trueTool)
		if err == nil {
			return true, ""
		}
		return false, err.Error()
	}
	sleepOK, sleepErr := try("sleep")
	idleOK, idleErr := try("idle")
	if sleepOK {
		mark(true, t("잠자기(sleep) 차단: 가능", "sleep block: allowed", "阻止休眠：已允许"))
	} else {
		ok = false
		mark(false, t("잠자기(sleep) 차단: 거부됨 — ", "sleep block: refused: ", "阻止休眠：被拒绝：")+sleepErr)
		if strings.Contains(sleepErr, "Access denied") {
			name := ""
			if u, err := user.Current(); err == nil {
				name = u.Username
			}
			fmt.Fprintln(w, t(
				"    polkit 은 기본적으로 로컬 로그인(화면 앞)에서만 잠자기 차단을 허용합니다. SSH 나 헤드리스 기기에서는\n"+
					"    불침번이 화면 꺼짐(idle)만 막을 수 있고, 기기는 여전히 잠들 수 있습니다.\n"+
					"    이 사용자에게만 허용하려면 아래 내용을 "+PolkitRulePath+" 로 저장하십시오 (root, 0644).\n"+
					"    불침번은 이 명령을 대신 실행하지 않습니다:",
				"    By default polkit allows a sleep block only from a local login (at the machine's screen). Over SSH or\n"+
					"    on a headless machine Bulchimbeon can block idle only, and the machine can still suspend.\n"+
					"    To allow it for this user only, save the rule below as "+PolkitRulePath+" (root, 0644).\n"+
					"    Bulchimbeon never runs this for you:", "    默认情况下，polkit 仅允许本地登录（在设备屏幕前）阻止休眠。通过 SSH 或在无显示器环境中，\n    Bulchimbeon 只能阻止空闲熄屏，设备仍可休眠。\n    如需仅为此用户授权，请将以下规则保存为 "+PolkitRulePath+"（root 所有，权限 0644）。\n    Bulchimbeon 不会代你执行此操作："))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "    sudo tee %s >/dev/null <<'EOF'\n", PolkitRulePath)
			for _, line := range strings.Split(strings.TrimSuffix(PolkitRule(name), "\n"), "\n") {
				fmt.Fprintln(w, "    "+line)
			}
			fmt.Fprintln(w, "    EOF")
			fmt.Fprintln(w)
		}
	}
	if idleOK {
		mark(true, t("화면 꺼짐(idle) 차단: 가능", "idle block: allowed", "阻止空闲熄屏：已允许"))
	} else {
		mark(false, t("화면 꺼짐(idle) 차단: 거부됨 — ", "idle block: refused: ", "阻止空闲熄屏：被拒绝：")+idleErr)
	}

	if b, err := NewPowerSupply().Battery(); err == nil {
		if b == nil {
			mark(true, t("배터리: 없음 (철수 기준 해당 없음)", "battery: none (the battery floor does not apply)", "电池：无（低电量阈值不适用）"))
		} else {
			mark(true, fmt.Sprintf(t("배터리: %d%%%s", "battery: %d%%%s", "电量：%d%%%s"), b.Percent,
				map[bool]string{true: t(" (배터리 사용 중)", " (on battery)", "（使用电池）"), false: t(" (전원 연결)", " (plugged in)", "（已接通电源）")}[b.OnBattery]))
		}
	}
	if r := NewThermal().Thermal(); r.Available {
		mark(true, fmt.Sprintf(t("온도: %s %.0f °C (위험 %.0f °C) → %s", "temperature: %s %.0f °C (critical %.0f °C) → %s", "温度：%s %.0f °C（临界值 %.0f °C）→ %s"),
			r.Sensor, r.Celsius, r.CritCelsius, r.LevelName))
	} else {
		mark(true, t("온도: 센서 없음 (폭염 대응 꺼짐)", "temperature: no sensor (heat response off)", "温度：无传感器（过热保护未启用）"))
	}
	fmt.Fprintln(w, "  - "+t("덮개: 처리하지 않음 — 덮개를 닫으면 여전히 잠듭니다 (logind HandleLidSwitch)",
		"lid: not handled. Closing the lid still suspends (logind HandleLidSwitch)", "合盖：不处理。合盖仍会休眠 (logind HandleLidSwitch)"))

	if systemctl := findTool("systemctl"); systemctl != "" {
		active := strings.TrimSpace(string(runUser(systemctl, "--user", "is-active", "bulchimbeon.service")))
		enabled := strings.TrimSpace(string(runUser(systemctl, "--user", "is-enabled", "bulchimbeon.service")))
		mark(active == "active", fmt.Sprintf(t("서비스: %s, %s (systemctl --user)", "service: %s, %s (systemctl --user)", "服务：%s，%s (systemctl --user)"),
			orNone(active, "unknown"), orNone(enabled, "unknown")))
	}
	if loginctl := findTool("loginctl"); loginctl != "" {
		out, err := run(3*time.Second, loginctl, "show-user", strconv.Itoa(os.Getuid()), "--property=Linger", "--value")
		if err == nil && strings.TrimSpace(string(out)) == "no" {
			fmt.Fprintln(w, "  - "+t("Linger=no: 로그아웃하면 서비스도 멈춥니다. 계속 돌리려면 `loginctl enable-linger`",
				"Linger=no: the service stops when you log out. Keep it running with `loginctl enable-linger`", "Linger=no：退出登录后服务会停止。可使用 `loginctl enable-linger` 使其持续运行"))
		}
	}
	return ok
}

// runUser runs a `systemctl --user` query; it needs the user's runtime directory.
func runUser(path string, args ...string) []byte {
	cmd := []string{}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		cmd = append(cmd, "XDG_RUNTIME_DIR="+dir)
	}
	out, _ := runEnv(3*time.Second, append(minimalEnv[:len(minimalEnv):len(minimalEnv)], cmd...), path, args...)
	return out
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}
