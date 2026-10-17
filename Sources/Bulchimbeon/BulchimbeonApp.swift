import AppKit
import BulchimbeonCore
import SwiftUI

@main
struct BulchimbeonApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var duty = DutyController.shared
    @StateObject private var animator = SpriteAnimator()
    @StateObject private var patrol = PatrolController.shared
    @StateObject private var uniform = UniformSetting.shared

    init() {
        // Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose → 진단 출력 후 종료 (UI 없음)
        if CommandLine.arguments.contains(LaunchArgument.diagnose.rawValue) {
            let custom = UserDefaults.standard.stringArray(forKey: SettingsKey.customAgents.rawValue) ?? []
            print(ProcessScanner.diagnose(custom: custom))
            exit(0)
        }
    }

    var body: some Scene {
        MenuBarExtra {
            DutyMenu(duty: duty, patrol: patrol, uniform: uniform)
        } label: {
            MenuBarLabel(duty: duty, animator: animator, uniform: uniform)
        }
        .menuBarExtraStyle(.menu)
    }
}

struct MenuBarLabel: View {
    @ObservedObject var duty: DutyController
    @ObservedObject var animator: SpriteAnimator
    @ObservedObject var uniform: UniformSetting

    var body: some View {
        let frames = SpriteImages.frames(duty.pose)
        HStack(spacing: 3) {
            Image(nsImage: frames[animator.tick % frames.count])
            if let remaining = duty.remainingText {
                Text(remaining).monospacedDigit()
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(L10n.t("불침번: ", "Bulchimbeon: ", "Bulchimbeon：") + duty.statusLine)
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // 번들 밖(swift run)에서도 Dock 아이콘 없이 메뉴바 전용으로
        NSApp.setActivationPolicy(.accessory)
        MainActor.assumeIsolated { PatrolController.shared.start() }

        // --demo → 녹화용 연출 후 종료
        if CommandLine.arguments.contains("--demo") {
            MainActor.assumeIsolated { DemoDirector.run() }
            return
        }

        // open -a Bulchimbeon --args --agent-watch  → 실행과 동시에 에이전트 근무
        if CommandLine.arguments.contains(LaunchArgument.agentWatch.rawValue) {
            Task { @MainActor in await DutyController.shared.startAgentWatch() }
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        MainActor.assumeIsolated { DutyController.shared.shutdown() }
    }
}

struct DutyMenu: View {
    @ObservedObject var duty: DutyController
    @ObservedObject var patrol: PatrolController
    @ObservedObject var uniform: UniformSetting

    private static let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev"
    private static let repoURL = URL(string: "https://github.com/s3onghyun/bulchimbeon")!

    var body: some View {
        Text(L10n.t("불침번 — ", "Bulchimbeon — ", "Bulchimbeon — ") + duty.statusLine)
        if duty.order == .agentWatch, let detail = duty.watchDetail {
            Text(detail)
        }
        Text("“\(duty.quote)”")

        Divider()

        Menu(L10n.t("🪖 근무 서기", "🪖 Stand watch", "🪖 开始值班")) {
            ForEach(DutyController.durations, id: \.title) { option in
                Button(option.title) { Task { await duty.startTimed(option.seconds) } }
            }
        }
        Toggle(L10n.t("🤖 에이전트 근무 (자동 교대)", "🤖 Agent watch (auto shift)", "🤖 代理值班（自动交班）"), isOn: Binding(
            get: { duty.order == .agentWatch },
            set: { on in Task { on ? await duty.startAgentWatch() : await duty.stop() } }
        ))
        Menu(L10n.t("에이전트 근무 설정", "Agent watch settings", "代理值班设置")) {
            Picker(L10n.t("조용해진 뒤 교대까지", "Shift over after quiet for", "空闲多久后交班"), selection: $duty.graceMinutes) {
                ForEach(DutyController.graceOptions, id: \.self) { Text(L10n.t("\($0)분", "\($0) min", "\($0) 分钟")).tag($0) }
            }
            Toggle(L10n.t("열려만 있는 세션도 근무로 인정 (교대 안 함)", "Count idle sessions as working (never hands over)", "将空闲会话也视为工作中（不交班）"),
                   isOn: $duty.countIdleSessions)
        }
        if duty.order != .off {
            Button(L10n.t("💤 근무 종료", "💤 Stand down", "💤 结束值班")) { Task { await duty.stop() } }
        }

        Divider()

        Picker(L10n.t("근무 형태", "Watch mode", "值班模式"), selection: Binding(
            get: { duty.mode },
            set: { mode in Task { await duty.setMode(mode) } }
        )) {
            ForEach(DutyMode.allCases) { Text($0.title).tag($0) }
        }
        Toggle(L10n.t("🚶 순찰 모드 (화면 위 이등병)", "🚶 Patrol mode (soldier on screen)", "🚶 巡逻模式（屏幕上的小士兵）"), isOn: $patrol.enabled)
        if patrol.enabled {
            Picker(L10n.t("이등병 크기", "Soldier size", "士兵大小"), selection: $patrol.scale) {
                ForEach(PatrolController.scales, id: \.self) { Text("\($0)×").tag($0) }
            }
        }
        Picker(L10n.t("🎽 군복", "🎽 Uniform", "🎽 制服"), selection: $uniform.themeID) {
            ForEach(UniformTheme.all) { theme in
                Label { Text(theme.name) } icon: { Image(nsImage: SpriteImages.preview(theme)) }
                    .tag(theme.id)
            }
        }
        Picker(L10n.t("배터리 철수 기준", "Stand down on battery", "低电量停止值班"), selection: $duty.batteryFloor) {
            ForEach(DutyController.batteryFloors, id: \.self) { floor in
                Text(floor == 0 ? L10n.t("끄기", "Never", "关闭") : L10n.t("\(floor)% 이하", "At \(floor)% or below", "电量不高于 \(floor)%")).tag(floor)
            }
        }

        Divider()

        Text(duty.agentLine)
        if let batteryLine = duty.batteryLine { Text(batteryLine) }
        if let heatLine = duty.heatLine { Text(heatLine) }
        if !duty.otherHolders.display.isEmpty {
            Text(L10n.t("🖥️ 화면을 막는 다른 앱: ", "🖥️ Also keeping the display on: ", "🖥️ 同样保持屏幕常亮的应用：") + duty.otherHolders.display.joined(separator: ", "))
        }
        if !duty.otherHolders.system.isEmpty {
            Text(L10n.t("💤 잠들지 못하게 하는 다른 앱: ", "💤 Also keeping the Mac awake: ", "💤 同样阻止 Mac 休眠的应用：") + duty.otherHolders.system.joined(separator: ", "))
        }
        Menu(L10n.t("📒 근무일지", "📒 Duty log", "📒 值班日志")) {
            if duty.recentLog.isEmpty {
                Text(L10n.t("기록 없음", "No entries", "暂无记录"))
            }
            ForEach(Array(duty.recentLog.enumerated()), id: \.offset) { _, entry in
                Text(entry.line)
            }
            Divider()
            Button(L10n.t("전체 일지 열기…", "Open full log…", "打开完整日志…")) {
                if let url = duty.logFileURL { NSWorkspace.shared.open(url) }
            }
        }
        if duty.showsLidRecovery {
            Button(L10n.t("⚠️ 덮개 잠자기 복구", "⚠️ Restore lid sleep", "⚠️ 恢复合盖休眠")) { Task { await duty.restoreLid() } }
        }

        Divider()

        Picker("언어 / Language / 语言", selection: Binding(
            get: { duty.languagePreference },
            set: { duty.setLanguage($0) }
        )) {
            ForEach(L10n.Preference.allCases, id: \.self) { Text($0.title).tag($0) }
        }
        Toggle(L10n.t("로그인 시 자동 실행", "Launch at login", "登录时自动启动"), isOn: Binding(
            get: { duty.launchAtLogin },
            set: { duty.setLaunchAtLogin($0) }
        ))
        Button(L10n.t("불침번 v\(Self.version) · GitHub", "Bulchimbeon v\(Self.version) · GitHub", "Bulchimbeon v\(Self.version) · GitHub")) {
            NSWorkspace.shared.open(Self.repoURL)
        }
        Button(L10n.t("불침번 퇴근 (종료)", "Quit Bulchimbeon", "退出 Bulchimbeon")) { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }
}
