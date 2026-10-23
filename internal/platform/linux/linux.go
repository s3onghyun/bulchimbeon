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
	lock := NewLock(NewSystemdInhibitor(logind))
	return platform.Platform{
		Name:             "linux",
		Processes:        NewProcScanner(),
		Inhibitor:        lock,
		Battery:          NewPowerSupply(),
		Thermal:          NewThermal(),
		Holders:          logind,
		SetXfceKeepAwake: lock.SetXfceKeepAwake,
		Doctor:           Doctor,
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
// inhibitors; it never changes the system. It reports false if the lock it would take now cannot
// work: the desktop's suspend inhibitor with a desktop session, logind's sleep block without one.
func Doctor(w io.Writer, lang engine.Lang, xfceKeepAwake bool) bool {
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

	// With a desktop session, its own suspend inhibitor is the lock; logind's only without one.
	desktop := tryDesktop(DialSessionBus, xfceKeepAwake)
	onDesktop := desktop.OnDesktop()
	switch {
	case onDesktop:
		mark(true, t("데스크톱 절전 억제: 가능 — ", "desktop suspend inhibitor: allowed: ", "桌面休眠抑制：已允许：")+platform.MethodText(desktop)+
			map[string]string{platform.MethodGnome: t(" (GNOME·Cinnamon·MATE·Budgie 의 전원 관리자가 읽음)", " (read by the GNOME, Cinnamon, MATE and Budgie power managers)",
				"（GNOME、Cinnamon、MATE、Budgie 的电源管理器会读取）")}[desktop.Method])
	case desktop.Fallback == platform.FallbackXfce:
		fmt.Fprintln(w, "  - "+t("데스크톱 절전 억제: XFCE — ", "desktop suspend inhibitor: XFCE: ", "桌面休眠抑制：XFCE：")+platform.FallbackText(desktop, lang))
	case desktop.Fallback == platform.FallbackRefused:
		mark(false, t("데스크톱 절전 억제: 거부됨 — ", "desktop suspend inhibitor: refused: ", "桌面休眠抑制：被拒绝：")+desktop.Detail)
	default:
		fmt.Fprintln(w, "  - "+t("데스크톱 절전 억제: 없음 — ", "desktop suspend inhibitor: none: ", "桌面休眠抑制：无：")+platform.FallbackText(desktop, lang))
	}

	mode := LogindMode(inhibit)
	_, sleepErr := run(5*time.Second, inhibit, "--what=sleep", "--mode="+mode, "--who="+InhibitWho,
		"--why=Bulchimbeon doctor check", trueTool)
	sleepLine := t("logind 잠자기 차단 (", "logind sleep block (", "logind 休眠阻止（") + mode + t(")", ")", "）")
	switch {
	case sleepErr == nil:
		mark(true, sleepLine+t(": 가능", ": allowed", "：已允许"))
	case onDesktop:
		fmt.Fprintln(w, "  - "+sleepLine+t(": 거부됨 (데스크톱 세션이 없을 때만 씀) — ", ": refused (used only without a desktop session): ",
			"：被拒绝（仅在没有桌面会话时使用）：")+sleepErr.Error())
	default:
		ok = false
		mark(false, sleepLine+t(": 거부됨 — ", ": refused: ", "：被拒绝：")+sleepErr.Error())
	}
	if sleepErr != nil && strings.Contains(sleepErr.Error(), "Access denied") {
		name := ""
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
		fmt.Fprintln(w, t(
			"    polkit 은 기본적으로 로컬 로그인(화면 앞)에서만 잠자기 차단을 허용합니다. SSH 나 헤드리스 기기에서는\n"+
				"    logind 가 거부하므로, 데스크톱 세션이 없으면 기기가 잠들 수 있습니다.\n"+
				"    이 사용자에게만 허용하려면 아래 내용을 "+PolkitRulePath+" 로 저장하십시오 (root, 0644).\n"+
				"    불침번은 이 명령을 대신 실행하지 않습니다:",
			"    By default polkit allows a sleep block only from a local login (at the machine's screen). Over SSH or\n"+
				"    on a headless machine logind refuses it, and without a desktop session the machine can still suspend.\n"+
				"    To allow it for this user only, save the rule below as "+PolkitRulePath+" (root, 0644).\n"+
				"    Bulchimbeon never runs this for you:", "    默认情况下，polkit 仅允许本地登录（在设备屏幕前）阻止休眠。通过 SSH 或在无显示器环境中，\n    logind 会拒绝，没有桌面会话时设备仍可休眠。\n    如需仅为此用户授权，请将以下规则保存为 "+PolkitRulePath+"（root 所有，权限 0644）。\n    Bulchimbeon 不会代你执行此操作："))
		fmt.Fprintln(w)
		fmt.Fprintf(w, "    sudo tee %s >/dev/null <<'EOF'\n", PolkitRulePath)
		for _, line := range strings.Split(strings.TrimSuffix(PolkitRule(name), "\n"), "\n") {
			fmt.Fprintln(w, "    "+line)
		}
		fmt.Fprintln(w, "    EOF")
		fmt.Fprintln(w)
	}
	if mode == "block-weak" {
		fmt.Fprintln(w, "  - "+t("logind block-weak (systemd 257 이상): 다른 사용자와 logind 의 IdleAction 은 막고, 내 손으로 하는 절전(메뉴, `systemctl suspend`)은 그대로 됩니다",
			"logind block-weak (systemd 257 or later): stops other users and logind's IdleAction; your own suspend (menu, `systemctl suspend`) still works",
			"logind block-weak（systemd 257 及以上）：阻止其他用户和 logind 的 IdleAction；你自己发起的休眠（菜单、`systemctl suspend`）仍然有效"))
	} else {
		fmt.Fprintln(w, "  - "+t("logind block (systemd 256 이하): 다른 사용자와 logind 의 IdleAction 은 막지만, 내 사용자의 절전 요청은 logind 가 통과시킵니다. 그래서 데스크톱 세션에서는 데스크톱의 절전 억제를 씁니다. 터미널의 `systemctl suspend` 는 `-i` 를 붙여야 합니다",
			"logind block (systemd 256 or older): stops other users and logind's IdleAction, but logind lets your own user's suspend requests through, so a desktop session gets the desktop's inhibitor instead. `systemctl suspend` in a terminal needs `-i`",
			"logind block（systemd 256 及以下）：阻止其他用户和 logind 的 IdleAction，但 logind 会放行你自己用户的休眠请求，因此桌面会话改用桌面的休眠抑制。在终端中使用 `systemctl suspend` 需要加 `-i`"))
	}
	onDuty := platform.LockStatus{Method: platform.MethodLogind, Mode: mode}
	if onDesktop {
		onDuty = desktop
	}
	fmt.Fprintln(w, "  → "+t("근무 중 잠금: ", "lock while on duty: ", "值班时的锁：")+platform.MethodText(onDuty))
	screen := t("화면: 막지 않음 — 설정대로 어두워지고 잠깁니다 (화면은 꺼져도 깨어 있음)", "screen: not held, it dims and locks as you set it (Display may sleep)",
		"屏幕：不阻止，按你的设置变暗并锁定（屏幕可熄灭）")
	if onDesktop && desktop.Desktop == "xfce" {
		screen += t(" · 단, XFCE 전원 관리자는 절전 억제가 있는 동안 화면 꺼짐과 잠금도 미룹니다", " · but XFCE's power manager also keeps the screen on and unlocked while any inhibitor is held",
			" · 但 XFCE 电源管理器在存在任何抑制时也会保持屏幕常亮且不锁定")
	}
	fmt.Fprintln(w, "  - "+screen)

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
