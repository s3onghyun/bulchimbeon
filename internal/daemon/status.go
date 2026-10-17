package daemon

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// staleAfter: a state file older than this means the daemon is stuck or gone.
const staleAfter = 3 * Interval

// StatusInput is everything `bulchimbeon status` shows.
type StatusInput struct {
	State      *State
	StateErr   error
	StatePath  string
	Holders    []platform.Holder // live lock listing
	HoldersErr error
	Alive      func(pid int) bool
	LogTail    []string
	Lang       engine.Lang
	Now        time.Time
	Home       string
}

// RenderStatus explains whether the machine is being kept awake and why. It reports false
// when the daemon is not running. The text is for people; its format is not promised.
func RenderStatus(in StatusInput) (string, bool) {
	t := func(ko, en, zh string) string { return engine.T(in.Lang, ko, en, zh) }
	var b strings.Builder
	line := func(label, text string) { b.WriteString(padLabel(label) + text + "\n") }
	st := in.State

	if reason := notRunning(in); reason != "" {
		b.WriteString(t("불침번이 실행 중이 아닙니다: ", "Bulchimbeon is not running: ", "Bulchimbeon 未运行：") + reason + "\n")
		b.WriteString(t("시작: systemctl --user start bulchimbeon  (또는 bulchimbeon run)",
			"Start it: systemctl --user start bulchimbeon  (or bulchimbeon run)", "启动：systemctl --user start bulchimbeon（或 bulchimbeon run）") + "\n")
		if leftovers := ours(in.Holders, nil); len(leftovers) > 0 {
			b.WriteString(t("⚠️ 남아 있는 불침번 잠금: ", "⚠️ Bulchimbeon locks still listed: ", "⚠️ 仍存在 Bulchimbeon 休眠锁：") + strings.Join(leftovers, ", ") + "\n")
		}
		return b.String(), false
	}

	// headline
	switch {
	case st.Resting:
		b.WriteString(t("🥵 불침번 — 전투휴식 중 (폭염 경보). 식으면 복귀합니다", "🥵 Bulchimbeon — combat rest (heat warning), resumes when it cools down", "🥵 Bulchimbeon — 降温休息中（过热警报），降温后恢复") + "\n")
	case st.OnDuty && st.DutyStartedAt != nil:
		clock := engine.FormatClock(in.Now.Sub(*st.DutyStartedAt))
		b.WriteString(t("🪖 불침번 — 근무 중 · "+clock+" 경과 (에이전트 근무)", "🪖 Bulchimbeon — on duty for "+clock+" (agent watch)", "🪖 Bulchimbeon — 已值班 "+clock+"（代理值班）") + "\n")
	case st.Order == "agentWatch":
		b.WriteString(t("👀 불침번 — 근무 대기 중. 에이전트가 일을 시작하면 투입합니다", "👀 Bulchimbeon — standing by until an agent starts working", "👀 Bulchimbeon — 待命中，等待代理开始工作") + "\n")
	default:
		b.WriteString(t("💤 불침번 — 근무 없음", "💤 Bulchimbeon — off duty", "💤 Bulchimbeon — 休息中") + "\n")
	}

	line(t("잠금", "Lock:", "休眠锁："), lockLine(in, t))
	if why := whyLine(st, t); why != "" {
		line(t("이유", "Why:", "原因："), why)
	}
	line(t("에이전트", "Agents:", "代理："), agentsLine(st, t))
	if st.Settings.AgentWatchCountsIdle {
		line(t("교대", "Handover:", "交班："), t("열린 세션도 근무로 인정 (agentWatchCountsIdle) — 모두 닫히면 교대",
			"open sessions count (agentWatchCountsIdle): hands over once all are closed", "计入已打开的会话 (agentWatchCountsIdle)：全部关闭后交班"))
	} else {
		n := strconv.Itoa(st.Settings.AgentGraceMinutes)
		line(t("교대", "Handover:", "交班："), t("마지막 에이전트가 조용해지고 "+n+"분 뒤 (agentGraceMinutes)",
			n+" min after the last agent goes quiet (agentGraceMinutes)", "最后一个代理空闲 "+n+" 分钟后交班 (agentGraceMinutes)"))
	}
	line(t("전원", "Power:", "电源："), powerLine(st, t))
	if in.HoldersErr == nil {
		line(t("다른 앱", "Others:", "其他应用："), othersLine(in, t))
	}
	if st.Speech != "" {
		line(t("이등병", "Private:", "小士兵："), "“"+st.Speech+"”")
	}
	line(t("근무일지", "Log:", "日志："), Tilde(st.LogPath, in.Home))
	for _, l := range in.LogTail {
		line("", l)
	}
	for _, p := range st.Problems {
		line(t("설정", "Settings:", "设置："), t("무시한 값 — ", "ignored ", "已忽略 ")+p)
	}
	if st.ScanError != "" {
		line(t("오류", "Error:", "错误："), st.ScanError)
	}
	return b.String(), true
}

// labelWidth is the label column in terminal cells. Hangul is two cells wide, so padding by
// character count (fmt's %-10s) would misalign the Korean labels.
const labelWidth = 11

func padLabel(label string) string {
	w := 0
	for _, r := range label {
		w += cellWidth(r)
	}
	return label + strings.Repeat(" ", max(labelWidth-w, 1))
}

// cellWidth is 2 for East Asian wide characters (Hangul, CJK, full-width forms), else 1.
func cellWidth(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF, r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF, r >= 0xFE30 && r <= 0xFE4F, r >= 0xFF00 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6:
		return 2
	}
	return 1
}

func notRunning(in StatusInput) string {
	t := func(ko, en, zh string) string { return engine.T(in.Lang, ko, en, zh) }
	st := in.State
	switch {
	case st == nil:
		return t("상태 파일 없음 (", "no state file (", "无状态文件 (") + Tilde(in.StatePath, in.Home) + ")"
	case st.Stopped:
		return t("멈춤 ", "stopped at ", "停止于 ") + st.UpdatedAt.Local().Format("2006-01-02 15:04:05")
	case in.Alive != nil && !in.Alive(st.PID):
		return t("프로세스 ", "process ", "进程 ") + strconv.Itoa(st.PID) + t(" 가 없습니다", " is gone", " 已不存在")
	case in.Now.Sub(st.UpdatedAt) > staleAfter:
		return t("응답 없음 — 마지막 갱신 ", "not responding: last update ", "无响应：距上次更新 ") + engine.FormatClock(in.Now.Sub(st.UpdatedAt)) + t(" 전", " ago", "")
	}
	return ""
}

// LockSummary describes the lock in one line.
func LockSummary(lock platform.LockStatus, lang engine.Lang) string {
	t := func(ko, en, zh string) string { return engine.T(lang, ko, en, zh) }
	if !lock.Wanted {
		return t("잡지 않음", "not held", "未持有")
	}
	sleep, _ := lock.Block("sleep")
	idle, _ := lock.Block("idle")
	pids := func(bs ...platform.Block) string {
		var p []string
		for _, b := range bs {
			if b.Held && b.PID > 0 {
				p = append(p, strconv.Itoa(b.PID))
			}
		}
		if len(p) == 0 {
			return ""
		}
		return " (" + lock.Method + " pid " + strings.Join(p, ", ") + ")"
	}
	switch {
	case sleep.Held && idle.Held:
		return t("잠자기·화면 꺼짐(sleep, idle) 차단 중", "blocking sleep and idle", "正在阻止休眠和空闲熄屏") + pids(sleep, idle)
	case sleep.Held:
		return t("잠자기(sleep) 차단 중, idle 차단 실패: ", "blocking sleep; idle failed: ", "正在阻止休眠；阻止空闲熄屏失败：") + idle.Error + pids(sleep)
	case idle.Held && sleep.Denied:
		return t("⚠️ 잠자기(sleep)는 막지 못함 — polkit 거부 (로컬 로그인이 아님: SSH·헤드리스). 화면 꺼짐(idle)만 차단 중이라 기기는 잠들 수 있습니다. `bulchimbeon doctor` 참고",
			"⚠️ NOT blocking sleep: polkit refused it (not a local login, e.g. SSH or headless). Blocking idle only, so the machine can still suspend. See `bulchimbeon doctor`", "⚠️ 未能阻止休眠：polkit 拒绝授权（非本地登录，例如 SSH 或无显示器环境）。仅阻止空闲熄屏，设备仍可休眠。请运行 `bulchimbeon doctor`") + pids(idle)
	case idle.Held:
		return t("⚠️ 잠자기(sleep)는 막지 못함 — ", "⚠️ NOT blocking sleep: ", "⚠️ 未能阻止休眠：") + sleep.Error + t(". 화면 꺼짐(idle)만 차단 중", ". Blocking idle only", "。仅阻止空闲熄屏") + pids(idle)
	}
	var errs []string
	for _, b := range lock.Blocks {
		if b.Error != "" {
			errs = append(errs, b.What+": "+b.Error)
		}
	}
	return t("⚠️ 잠금을 잡지 못함 — ", "⚠️ NOT held: ", "⚠️ 未持有休眠锁：") + strings.Join(errs, "; ")
}

func lockLine(in StatusInput, t func(string, string, string) string) string {
	st := in.State
	if !st.Lock.Wanted {
		switch {
		case st.Resting:
			return t("잡지 않음 — 전투휴식 중", "not held: combat rest", "未持有：降温休息中")
		case st.Order == "agentWatch":
			return t("잡지 않음 — 일하는 에이전트가 없습니다", "not held: no agent is working", "未持有：没有代理在工作")
		}
		return t("잡지 않음 — 근무 없음", "not held: off duty", "未持有：休息中")
	}
	text := LockSummary(st.Lock, in.Lang)
	if in.HoldersErr != nil {
		return text + t(" · logind 확인 실패: ", " · could not ask logind: ", " · 无法查询 logind：") + in.HoldersErr.Error()
	}
	listed := map[int]bool{}
	for _, h := range in.Holders {
		listed[h.PID] = true
	}
	var missing []string
	held := 0
	for _, b := range st.Lock.Blocks {
		if b.Held {
			held++
			if !listed[b.PID] {
				missing = append(missing, b.What)
			}
		}
	}
	switch {
	case held == 0:
		return text
	case len(missing) == 0:
		return text + t(" · logind 에서 확인됨", " · confirmed in logind", " · 已在 logind 中确认")
	}
	return text + t(" · ⚠️ 지금 logind 목록에 없음: ", " · ⚠️ not in logind's list right now: ", " · ⚠️ 当前不在 logind 列表中：") + strings.Join(missing, ", ")
}

func whyLine(st *State, t func(string, string, string) string) string {
	if st.OnDuty {
		var working []string
		for _, a := range st.Agents {
			for _, p := range a.Processes {
				if p.Working && len(working) < 3 {
					working = append(working, fmt.Sprintf("%s pid %d %.1f%%", a.Name, p.PID, p.TreeCPU))
				}
			}
		}
		if len(working) > 0 && !st.Settings.AgentWatchCountsIdle {
			return strings.Join(working, ", ") + t(" (프로세스 트리 CPU ≥ ", " (process-tree CPU ≥ ", "（进程树 CPU ≥ ") +
				fmt.Sprintf("%.1f%%", engine.CPUThreshold) + t(", 60초 평균)", ", 60 s average)", "，60 秒平均值）")
		}
		return st.WatchDetail
	}
	if st.Order == "agentWatch" && !st.Resting {
		if st.Settings.AgentWatchCountsIdle {
			return t("에이전트 세션이 열리면 투입합니다", "engages as soon as an agent session is open", "代理会话一打开就开始值班")
		}
		return t("에이전트 프로세스 트리가 CPU ", "engages when an agent's process tree uses ≥ ", "代理进程树 CPU 使用率达到 ") +
			fmt.Sprintf("%.1f%%", engine.CPUThreshold) + t(" 이상 쓰면 투입 (60초 평균)", " CPU (60 s average)", " 时开始值班（60 秒平均值）")
	}
	return ""
}

func agentsLine(st *State, t func(string, string, string) string) string {
	if len(st.Agents) == 0 {
		return t("감지된 에이전트 없음", "none detected", "未检测到")
	}
	var parts []string
	for _, a := range st.Agents {
		parts = append(parts, fmt.Sprintf(t("%s %d개 열림 · %d개 작업 중", "%s ×%d open, %d working", "%s：%d 个会话已打开，%d 个工作中"), a.Name, a.Open, a.Working))
	}
	return strings.Join(parts, "; ")
}

func powerLine(st *State, t func(string, string, string) string) string {
	battery := t("배터리 없음", "battery: none", "电池：无")
	if b := st.Battery; b != nil {
		state := t("전원 연결", "plugged in", "已接通电源")
		if b.OnBattery {
			state = t("배터리 사용 중", "on battery", "使用电池")
		}
		battery = fmt.Sprintf(t("배터리 %d%% (%s)", "battery: %d%% (%s)", "电量：%d%%（%s）"), b.Percent, state)
		if st.Settings.BatteryFloor > 0 {
			battery += fmt.Sprintf(t(" · %d%% 이하면 철수", " · stands down at %d%%", " · 电量不高于 %d%% 时结束值班"), st.Settings.BatteryFloor)
		}
	}
	heat := t("온도 센서 없음", "heat: no sensor", "温度：无传感器")
	if r := st.Thermal; r.Available {
		heat = fmt.Sprintf(t("온도 %s (%s %.0f °C, 위험 %.0f °C)", "heat: %s (%s %.0f °C, critical %.0f °C)", "温度：%s（%s %.0f °C，临界值 %.0f °C）"),
			heatName(st.Heat, t), r.Sensor, r.Celsius, r.CritCelsius)
	}
	return battery + " · " + heat + " · " + t("덮개: 처리 안 함 (닫으면 잠듦)", "lid: not handled (closing it still suspends)", "合盖：不处理（合盖仍会休眠）")
}

// heatName names the heat level, after the army heat-index stages in Korean.
func heatName(level string, t func(string, string, string) string) string {
	switch level {
	case "caution":
		return t("관심", "caution", "注意")
	case "advisory":
		return t("폭염 주의보", "advisory", "高温提醒")
	case "alert":
		return t("폭염 경보", "alert", "过热警报")
	}
	return t("정상", "normal", "正常")
}

func othersLine(in StatusInput, t func(string, string, string) string) string {
	var own []int
	for _, b := range in.State.Lock.Blocks {
		own = append(own, b.PID)
	}
	s := platform.SummarizeHolders(in.Holders, own, platform.Who)
	list := func(names []string) string {
		if len(names) == 0 {
			return t("없음", "none", "无")
		}
		return strings.Join(names, ", ")
	}
	return t("잠자기 차단: ", "blocking sleep: ", "阻止休眠：") + list(s.BlockSleep) +
		t(" · 잠자기 지연: ", " · delaying sleep: ", " · 延迟休眠：") + list(s.DelaySleep) +
		t(" · 화면 꺼짐 차단: ", " · blocking idle: ", " · 阻止空闲熄屏：") + list(s.BlockIdle)
}

// ours lists our locks in a lock listing whose PIDs are not in keep.
func ours(holders []platform.Holder, keep map[int]bool) []string {
	var out []string
	for _, h := range holders {
		if h.Who == platform.Who && !keep[h.PID] {
			out = append(out, fmt.Sprintf("%s (pid %d)", h.What, h.PID))
		}
	}
	return out
}
