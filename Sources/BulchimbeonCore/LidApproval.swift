import Foundation

/// 덮개 해제 관리자 승인(`osascript -e 'do shell script … with administrator privileges'`)의 결과.
///
/// 실측 (2026-10-28, 0.10.0~0.10.1): osascript 가 0 이 아닌 값으로 끝나면 모두 "승인 거부"로 기록했다. 실제로는 암호를
/// 넣은 뒤 관리자 명령의 pkill 이 osascript 를 죽인 것이었다. 사용자의 취소와 승인 장치의 실패를 가려, 실패는 원인과 함께 알린다.
public enum LidApproval {
    public enum Outcome: Equatable, Sendable {
        case approved
        /// 사용자가 암호 창을 취소함 (AppleScript 오류 -128)
        case cancelled
        /// 승인 장치가 실패함. 사용자의 거부가 아니다
        case failed(Failure)
    }

    public enum Failure: Equatable, Sendable {
        /// 정지 플래그 폴더를 쓸 수 없어 암호 창을 띄우지 않음
        case stopSignal
        /// osascript 를 실행하지 못함
        case launch
        /// osascript 가 신호로 끝남
        case interrupted(signal: Int32)
        /// AppleScript·관리자 명령이 오류로 끝남 (오류 번호를 모르면 nil)
        case error(code: Int?)
    }

    /// AppleScript "User canceled." 오류 번호
    static let userCanceled = -128

    /// osascript 에 넘길 AppleScript. 명령과 안내문은 AppleScript 문자열로 인용한다.
    public static func script(command: String, prompt: String) -> String {
        "do shell script \"\(escaped(command))\" with prompt \"\(escaped(prompt))\" with administrator privileges"
    }

    /// osascript 종료 결과. stderr 는 끝의 오류 번호(`… (-128)`)만 읽고 저장·기록하지 않는다.
    public static func outcome(exitStatus: Int32, signaled: Bool, stderr: String) -> Outcome {
        if signaled { return .failed(.interrupted(signal: exitStatus)) }
        if exitStatus == 0 { return .approved }
        let code = errorNumber(stderr)
        return code == userCanceled ? .cancelled : .failed(.error(code: code))
    }

    /// 실패로 끝났어도 덮개 잠자기가 꺼져 있고 이 앱의 감시병이 살아 있으면 관리자 명령은 끝까지 돌았다 (결과를 전할
    /// osascript 만 끊겼다). 이때는 승인으로 보고 앱이 소유·원복한다. 앱이 모르는 `disablesleep 1` 을 남기지 않는다.
    public static func reconcile(_ outcome: Outcome, sleepDisabled: Bool?, watchdogAlive: Bool) -> Outcome {
        guard case .failed(let failure) = outcome, sleepDisabled == true, watchdogAlive else { return outcome }
        switch failure {
        case .interrupted, .error: return .approved
        case .stopSignal, .launch: return outcome // 관리자 명령을 보내지 않았다
        }
    }

    /// `0:312: execution error: User canceled. (-128)` → -128
    static func errorNumber(_ stderr: String) -> Int? {
        let line = stderr.trimmingCharacters(in: .whitespacesAndNewlines)
        guard line.hasSuffix(")"), let open = line.lastIndex(of: "(") else { return nil }
        return Int(line[line.index(after: open)..<line.index(before: line.endIndex)])
    }

    private static func escaped(_ string: String) -> String {
        string.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
    }
}

extension LidApproval.Failure {
    /// 일지·알림에 쓰는 원인. 들어가는 가변 값은 숫자뿐이다.
    public func title(_ lang: L10n.Language = L10n.language) -> String {
        switch self {
        case .stopSignal: L10n.t("정지 플래그 폴더를 쓸 수 없음", "stop-flag folder unusable", "无法使用停止标志文件夹", lang)
        case .launch: L10n.t("osascript 실행 실패", "couldn't run osascript", "无法运行 osascript", lang)
        case .interrupted(let signal): L10n.t("승인 창이 신호 \(signal)로 끊김", "approval cut off by signal \(signal)", "授权被信号 \(signal) 中断", lang)
        case .error(let code?): L10n.t("관리자 명령 오류 \(code)", "admin command error \(code)", "管理员命令错误 \(code)", lang)
        case .error(nil): L10n.t("관리자 명령 오류", "admin command error", "管理员命令错误", lang)
        }
    }

    /// 다시 시도하는 방법
    public func hint(_ lang: L10n.Language = L10n.language) -> String {
        switch self {
        case .stopSignal:
            L10n.t("~/Library/Application Support/Bulchimbeon 폴더가 본인 소유이고 다른 사용자가 쓸 수 없는지 확인한 뒤 덮개 근무를 다시 골라주십시오.",
                   "Make sure you own ~/Library/Application Support/Bulchimbeon and others can't write to it, then pick lid-closed watch again.",
                   "请确认 ~/Library/Application Support/Bulchimbeon 归你所有且其他用户无法写入，然后重新选择合盖值班。", lang)
        case .launch, .interrupted, .error:
            L10n.t("메뉴에서 덮개 근무를 다시 골라주십시오. 계속 실패하면 근무일지와 함께 알려주십시오.",
                   "Pick lid-closed watch again from the menu. If it keeps failing, report it with the duty log.",
                   "请在菜单中重新选择合盖值班。如果仍然失败，请附上值班日志反馈。", lang)
        }
    }
}
