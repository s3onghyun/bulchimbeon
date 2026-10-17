import Foundation

/// 이 앱 말고 잠자기·화면 꺼짐을 막고 있는 다른 프로세스.
/// ("에이전트가 다 끝났는데 화면이 안 꺼진다"의 실제 원인이 Vivaldi 영상 재생이었던 경험에서)
public enum PowerHolders {
    public struct Assertion: Equatable, Sendable {
        public let pid: Int32
        public let processName: String
        public let type: String
        public let name: String
        public init(pid: Int32, processName: String, type: String, name: String) {
            self.pid = pid; self.processName = processName; self.type = type; self.name = name
        }
    }

    /// 진단(별도 프로세스)이나 개발 빌드가 설치본을 "다른 앱"으로 보지 않도록 앱 이름으로도 거른다
    static let ownAppNames: Set<String> = ["Bulchimbeon", "불침번"]

    static let displayTypes: Set<String> = ["PreventUserIdleDisplaySleep", "NoDisplaySleepAssertion"]
    static let systemTypes: Set<String> = ["PreventUserIdleSystemSleep", "PreventSystemSleep", "NoIdleSleepAssertion"]

    /// 항상 떠 있거나 아주 짧게만 잡는 시스템 내부 assertion — 사용자에게 보여줘도 도움이 안 된다.
    static func isNoise(_ a: Assertion) -> Bool {
        ["powerd", "bluetoothd", "WindowServer", "kernel_task", "sharingd", "useractivityd", "mds", "mds_stores"]
            .contains(a.processName)
    }

    /// 메뉴 표시용: (화면을 막는 앱, 잠들지 못하게 하는 앱). 이름 중복 제거, 정렬.
    public static func summarize(_ assertions: [Assertion], ownPID: Int32,
                                 _ lang: L10n.Language = L10n.language) -> (display: [String], system: [String]) {
        var display = Set<String>(), system = Set<String>()
        for a in assertions where a.pid != ownPID && !isNoise(a) && !ownAppNames.contains(a.processName) {
            let label = label(for: a, lang)
            if displayTypes.contains(a.type) { display.insert(label) }
            else if systemTypes.contains(a.type) { system.insert(label) }
        }
        system.subtract(display) // 화면까지 막는 앱은 화면 쪽에만 표시
        return (display.sorted(), system.sorted())
    }

    static func label(for a: Assertion, _ lang: L10n.Language) -> String {
        if a.processName == "coreaudiod" { return L10n.t("오디오 재생", "audio playback", "音频播放", lang) }
        let name = String(a.processName.prefix(40))
        if a.name.localizedCaseInsensitiveContains("video") { return name + L10n.t(" (영상)", " (video)", "（视频）", lang) }
        return name
    }
}
