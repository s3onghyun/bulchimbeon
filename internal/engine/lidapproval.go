package engine

import "strconv"

// ShiftReason is why a watch falls back from lid mode to a system watch (Swift `ShiftReason`).
type ShiftReason string

const (
	ShiftRestarted             ShiftReason = "restarted"             // app restarted: lid needs approval again
	ShiftApprovalCancelled     ShiftReason = "approvalCancelled"     // admin prompt cancelled
	ShiftApprovalFailed        ShiftReason = "approvalFailed"        // the approval itself failed
	ShiftHeat                  ShiftReason = "heat"                  // back from combat rest
	ShiftLidRestoredExternally ShiftReason = "lidRestoredExternally" // lid sleep came back on outside the app
)

// LidOutcomeKind is the result of the macOS lid-mode admin approval (Swift `LidApproval.Outcome`).
type LidOutcomeKind int

const (
	LidApproved LidOutcomeKind = iota
	LidCancelled
	LidFailed
)

// LidFailureCause says why an approval failed without the user cancelling it.
type LidFailureCause string

const (
	LidFailureStopSignal  LidFailureCause = "stopSignal"  // the stop-flag folder is unusable; no prompt shown
	LidFailureLaunch      LidFailureCause = "launch"      // osascript could not start
	LidFailureInterrupted LidFailureCause = "interrupted" // osascript ended by a signal
	LidFailureError       LidFailureCause = "error"       // AppleScript or the admin command failed
)

// LidFailure is a failed approval: Signal for interrupted, Code (optional) for error.
type LidFailure struct {
	Cause  LidFailureCause
	Signal int
	Code   *int
}

// LidOutcome answers EffectEnableLid.
type LidOutcome struct {
	Kind    LidOutcomeKind
	Failure LidFailure // LidFailed only
}

// Title names the cause for the duty log and the notice.
func (f LidFailure) Title(lang Lang) string {
	switch f.Cause {
	case LidFailureStopSignal:
		return T(lang, "정지 플래그 폴더를 쓸 수 없음", "stop-flag folder unusable", "无法使用停止标志文件夹")
	case LidFailureLaunch:
		return T(lang, "osascript 실행 실패", "couldn't run osascript", "无法运行 osascript")
	case LidFailureInterrupted:
		s := strconv.Itoa(f.Signal)
		return T(lang, "승인 창이 신호 "+s+"로 끊김", "approval cut off by signal "+s, "授权被信号 "+s+" 中断")
	}
	if f.Code != nil {
		c := strconv.Itoa(*f.Code)
		return T(lang, "관리자 명령 오류 "+c, "admin command error "+c, "管理员命令错误 "+c)
	}
	return T(lang, "관리자 명령 오류", "admin command error", "管理员命令错误")
}

// Hint says how to try again.
func (f LidFailure) Hint(lang Lang) string {
	if f.Cause == LidFailureStopSignal {
		return T(lang, "~/Library/Application Support/Bulchimbeon 폴더가 본인 소유이고 다른 사용자가 쓸 수 없는지 확인한 뒤 덮개 근무를 다시 골라주십시오.",
			"Make sure you own ~/Library/Application Support/Bulchimbeon and others can't write to it, then pick lid-closed watch again.",
			"请确认 ~/Library/Application Support/Bulchimbeon 归你所有且其他用户无法写入，然后重新选择合盖值班。")
	}
	return T(lang, "메뉴에서 덮개 근무를 다시 골라주십시오. 계속 실패하면 근무일지와 함께 알려주십시오.",
		"Pick lid-closed watch again from the menu. If it keeps failing, report it with the duty log.",
		"请在菜单中重新选择合盖值班。如果仍然失败，请附上值班日志反馈。")
}
