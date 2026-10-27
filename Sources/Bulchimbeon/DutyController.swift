import AppKit
import BulchimbeonCore
import Foundation
import ServiceManagement
import UserNotifications

/// 불침번 앱 쪽 껍데기. 실제 시스템 값을 모아 DutyEngine(Core)에 넘기고, 엔진이 돌려준 일을 실행한다.
/// 판단 규칙은 전부 엔진에 있고 시나리오 테스트로 검증된다 (이슈 #2).
@MainActor
final class DutyController: ObservableObject {
    static let shared = DutyController()

    static let durations: [(title: String, seconds: TimeInterval?)] = [
        (L10n.t("30분", "30 min", "30 分钟"), 1800), (L10n.t("1시간", "1 hour", "1 小时"), 3600), (L10n.t("2시간", "2 hours", "2 小时"), 7200),
        (L10n.t("4시간", "4 hours", "4 小时"), 14400), (L10n.t("무기한", "Indefinitely", "无限期"), nil),
    ]
    static let batteryFloors = SettingsValues.batteryFloors
    static let graceOptions = SettingsValues.graceMinutes

    @Published private(set) var engine: DutyEngine

    // 설정 (메뉴 바인딩) — 바뀌면 저장하고 엔진에 반영
    @Published var countIdleSessions: Bool {
        didSet { defaults.set(countIdleSessions, forKey: Keys.countIdleSessions); engine.countIdleSessions = countIdleSessions }
    }
    @Published var graceMinutes: Int {
        didSet { defaults.set(graceMinutes, forKey: Keys.graceMinutes); engine.graceMinutes = graceMinutes }
    }
    @Published var batteryFloor: Int {
        didSet { defaults.set(batteryFloor, forKey: Keys.batteryFloor); engine.batteryFloor = batteryFloor }
    }

    // 관찰값 (메뉴 표시용)
    @Published private(set) var agents: [AgentKind: Int] = [:]
    @Published private(set) var agentStatus: [AgentKind: AgentActivity.Status] = [:]
    @Published private(set) var otherHolders: (display: [String], system: [String]) = ([], [])
    @Published private(set) var battery: BatteryStatus?
    @Published private(set) var now = Date()
    @Published private(set) var quote = Lines.pick(Lines.off())

    var order: DutyOrder { engine.order }
    /// 마지막으로 고른 근무 시간 (메뉴 체크 표시용)
    private(set) var pickedDuration: TimeInterval?
    var isOnDuty: Bool { engine.isOnDuty }
    var mode: DutyMode { engine.mode }
    var resting: Bool { engine.resting }
    var heat: HeatLevel { engine.heat }
    var watchDetail: String? { engine.watchDetail }
    var lidSleepDisabled: Bool { engine.lidSleepDisabled }

    private enum Keys {
        static let mode = SettingsKey.dutyMode.rawValue
        static let batteryFloor = SettingsKey.batteryFloor.rawValue
        static let lidEngagedByUs = SettingsKey.lidEngagedByUs.rawValue
        static let customAgents = SettingsKey.customAgents.rawValue
        static let order = SettingsKey.dutyOrder.rawValue
        static let orderUntil = SettingsKey.dutyOrderUntil.rawValue
        static let simulateHeat = SettingsKey.simulateHeat.rawValue
        static let countIdleSessions = SettingsKey.agentWatchCountsIdle.rawValue
        static let graceMinutes = SettingsKey.agentGraceMinutes.rawValue
    }

    private let defaults = UserDefaults.standard
    private let log = DutyLog(fileURL: DutyLog.defaultFileURL)
    private let assertion = PowerAssertion()
    private let notifier = Notifier()
    /// 엔진 밖의 말풍선 (인수인계 보고, 데모)
    private var localSpeech: (text: String, until: Date)?
    /// --demo 연출 중이면 실제 상태 대신 이 포즈
    @Published private var demoPose: Sprites.Pose?
    private var quoteKind: DutyEngine.QuoteKind = .off
    private var ticking = false
    private var lastScan = Date.distantPast
    private var cpuHistory = CPUHistory(window: 60)
    /// 화면이 잠긴 시각 (당직 근무 인수인계용)
    private var lockedAt: Date?
    private var timer: Timer?

    private init() {
        // defaults 는 사용자가 `defaults write` 로 바꿀 수 있으므로 허용된 값만 받는다
        let storedFloor = defaults.object(forKey: Keys.batteryFloor) as? Int ?? SettingsValues.defaultBatteryFloor
        let storedGrace = defaults.object(forKey: Keys.graceMinutes) as? Int ?? SettingsValues.defaultGraceMinutes
        let floor = Self.batteryFloors.contains(storedFloor) ? storedFloor : SettingsValues.defaultBatteryFloor
        let grace = Self.graceOptions.contains(storedGrace) ? storedGrace : SettingsValues.defaultGraceMinutes
        let countIdle = defaults.bool(forKey: Keys.countIdleSessions)
        let lidDisabled = SystemProbes.lidSleepDisabled()
        var lidByUs = defaults.bool(forKey: Keys.lidEngagedByUs)
        if !lidDisabled && lidByUs { lidByUs = false } // 당직 감시병이 이미 원복함

        batteryFloor = floor
        graceMinutes = grace
        countIdleSessions = countIdle
        engine = DutyEngine(mode: DutyMode(rawValue: defaults.string(forKey: Keys.mode) ?? "") ?? .system,
                            batteryFloor: floor, countIdleSessions: countIdle, graceMinutes: grace,
                            lidSleepDisabled: lidDisabled, lidEngagedByUs: lidByUs, now: Date())
        // 주의: 여기서 persist() 를 부르면 아직 "근무 없음"인 엔진 상태로 저장된 근무를 지워버린다.
        // 근무 상태 저장은 resume() 이후 run() 이 맡는다. 덮개 플래그 정리만 바로 저장.
        defaults.set(lidByUs, forKey: Keys.lidEngagedByUs)

        if lidDisabled && lidByUs {
            writeLog(L10n.t("⚠️ 지난 근무 비정상 종료 — 덮개 잠자기가 꺼진 채 남아 있음", "⚠️ Last watch ended abnormally — lid sleep still disabled", "⚠️ 上次值班异常结束 — 合盖休眠仍被禁用"))
            notifier.send(Lines.lidLeftOn())
        }

        let center = DistributedNotificationCenter.default()
        center.addObserver(forName: .init("com.apple.screenIsLocked"), object: nil, queue: .main) { _ in
            MainActor.assumeIsolated { DutyController.shared.screenLocked() }
        }
        center.addObserver(forName: .init("com.apple.screenIsUnlocked"), object: nil, queue: .main) { _ in
            MainActor.assumeIsolated { DutyController.shared.screenUnlocked() }
        }

        timer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.tick() }
        }
        Task {
            await resume()
            await tick()
        }
    }

    // MARK: - 메뉴 표시

    var pose: Sprites.Pose { demoPose ?? engine.pose(at: Date()) }

    var currentSpeech: String? {
        if let localSpeech, Date() < localSpeech.until { return localSpeech.text }
        return engine.speech(at: Date())
    }

    private func say(_ text: String, seconds: TimeInterval = 4) {
        localSpeech = (text, Date().addingTimeInterval(seconds))
    }

    /// 시간제 근무의 남은 시간 (메뉴바 표시용)
    var remainingText: String? {
        guard case .timed(let until?) = order else { return nil }
        return DutyClock.format(until.timeIntervalSinceNow)
    }

    var statusLine: String {
        if resting {
            return L10n.t("전투휴식 중 (폭염 경보) — 식으면 복귀", "Combat rest (heat warning) — resumes when cool", "降温休息中（过热警报）— 降温后恢复")
        }
        if isOnDuty, let remainingText {
            return L10n.t("근무 중 · \(remainingText) 남음", "On duty · \(remainingText) left", "值班中 · 剩余 \(remainingText)")
        }
        if isOnDuty, let started = engine.dutyStartedAt {
            let elapsed = DutyClock.format(now.timeIntervalSince(started))
            return L10n.t("근무 중 · \(elapsed) 경과", "On duty · \(elapsed)", "值班中 · 已过 \(elapsed)")
        }
        return order == .agentWatch ? L10n.t("에이전트 근무 대기 중", "Waiting for agents", "等待代理中") : L10n.t("취침 중", "Off duty", "休息中")
    }

    var agentLine: String {
        let summary = AgentCatalog.summary(agents)
        guard !summary.isEmpty else { return L10n.t("감지된 에이전트 없음", "No agents detected", "未检测到代理") }
        let busy = agentStatus.values.reduce(0) { $0 + $1.busy }
        return L10n.t("감지된 에이전트: \(summary) · 작업 중 \(busy)", "Agents: \(summary) · \(busy) working", "代理：\(summary) · \(busy) 个工作中")
    }

    var batteryLine: String? {
        battery.map { "🔋 \($0.percent)%" + ($0.onBattery ? "" : L10n.t(" (전원 연결)", " (charging)", "（已接通电源）")) }
    }

    var heatLine: String? {
        switch heat {
        case .advisory: L10n.t("🌡️ 폭염 주의보", "🌡️ Heat advisory", "🌡️ 高温提醒")
        case .alert: L10n.t("🔥 폭염 경보", "🔥 Heat warning", "🔥 过热警报")
        case .normal, .caution: nil
        }
    }

    var recentLog: [DutyLogEntry] { Array(log.recent.prefix(10)) }
    var logFileURL: URL? { DutyLog.defaultFileURL }

    /// 근무 중이 아닌데 덮개 플래그가 켜져 있으면 복구 버튼을 보여준다.
    var showsLidRecovery: Bool { lidSleepDisabled && !(isOnDuty && mode == .lid) }

    var launchAtLogin: Bool { SMAppService.mainApp.status == .enabled }

    // MARK: - 명령

    func startTimed(_ duration: TimeInterval?) async {
        pickedDuration = duration
        let label = Self.durations.first { $0.seconds == duration }?.title ?? L10n.t("무기한", "Indefinitely", "无限期")
        await run(engine.startTimed(seconds: duration, label: label, now: Date()))
    }

    func startAgentWatch() async {
        await run(engine.startAgentWatch(now: Date()))
        await tick()
    }

    func stop() async {
        await run(engine.stop(now: Date()))
    }

    func setMode(_ newMode: DutyMode) async {
        await run(engine.setMode(newMode, now: Date()))
    }

    func restoreLid() async {
        let ok = await Task.detached { SystemProbes.restoreLidSleepWithWatchdog() }.value
        await run(engine.lidRestoreResult(ok))
    }

    // MARK: - 엔진이 시킨 일 실행

    private static let debug = ProcessInfo.processInfo.environment["BULCHIMBEON_DEBUG"] != nil

    private func run(_ effects: [DutyEffect]) async {
        if Self.debug, !effects.isEmpty { FileHandle.standardError.write(Data("[engine] \(effects) onDuty=\(engine.isOnDuty) held=\(assertion.isHeld)\n".utf8)) }
        for effect in effects {
            switch effect {
            case .acquire(let mode):
                // pmset 은 비ASCII 를 빈 문자열로 표시하므로 이름은 영어
                assertion.acquire(mode == .display ? .display : .system, reason: "Bulchimbeon on duty: keeping AI agents awake")
            case .release:
                assertion.release()
            case .reacquire:
                assertion.reacquire()
            case .enableLid:
                let deadline: Date?
                if case .timed(let until) = engine.order { deadline = until } else { deadline = nil }
                let ok = await Task.detached { SystemProbes.setLidSleepDisabled(true, deadline: deadline) }.value
                await run(engine.lidEnableResult(ok, now: Date()))
            case .restoreLid:
                let ok = await Task.detached { SystemProbes.restoreLidSleepWithWatchdog() }.value
                await run(engine.lidRestoreResult(ok))
            case .notify(let title, let body):
                notifier.send((title, body))
            case .log(let text, let kind):
                writeLog(text, kind: kind)
            }
        }
        persist()
    }

    /// 엔진 상태 중 재시작 후에도 필요한 것을 저장하고, 대사를 갱신한다.
    private func persist() {
        defaults.set(engine.mode.rawValue, forKey: Keys.mode)
        defaults.set(engine.lidEngagedByUs, forKey: Keys.lidEngagedByUs)
        switch engine.order {
        case .off:
            defaults.removeObject(forKey: Keys.order)
            defaults.removeObject(forKey: Keys.orderUntil)
        case .agentWatch:
            defaults.set("agentWatch", forKey: Keys.order)
            defaults.removeObject(forKey: Keys.orderUntil)
        case .timed(let until):
            defaults.set("timed", forKey: Keys.order)
            if let until { defaults.set(until.timeIntervalSince1970, forKey: Keys.orderUntil) }
            else { defaults.removeObject(forKey: Keys.orderUntil) }
        }
        if engine.quote != quoteKind {
            quoteKind = engine.quote
            switch quoteKind {
            case .onDuty: quote = Lines.pick(Lines.onDuty())
            case .standby: quote = Lines.pick(Lines.standby())
            case .off: quote = Lines.pick(Lines.off())
            }
        }
    }

    // MARK: - 언어

    var languagePreference: L10n.Preference {
        let appDomain = Bundle.main.bundleIdentifier.flatMap { defaults.persistentDomain(forName: $0) }
        return L10n.Preference(appDomainLanguages: appDomain?["AppleLanguages"] as? [String])
    }

    /// 언어를 바꾸고 앱을 다시 띄운다. 근무 상태는 저장돼 있어 재시작 후 이어진다.
    func setLanguage(_ preference: L10n.Preference) {
        guard preference != languagePreference else { return }
        if let languages = preference.appleLanguages {
            defaults.set(languages, forKey: "AppleLanguages")
        } else {
            defaults.removeObject(forKey: "AppleLanguages")
        }
        defaults.synchronize()
        writeLog(L10n.t("언어 변경 → ", "Language → ", "语言 → ") + preference.title)
        relaunch()
    }

    /// 1초 뒤 같은 앱을 다시 연다. 앱 경로는 셸 문자열에 넣지 않고 인자($0)로 넘긴다.
    private func relaunch() {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/sh")
        process.arguments = ["-c", "sleep 1; exec /usr/bin/open \"$0\"", Bundle.main.bundlePath]
        process.environment = ["PATH": "/usr/bin:/bin"]
        try? process.run()
        NSApp.terminate(nil)
    }

    func setLaunchAtLogin(_ enabled: Bool) {
        do {
            if enabled { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
        } catch {
            writeLog(L10n.t("로그인 자동 실행 설정 실패: ", "Launch at login failed: ", "设置登录启动失败：") + error.localizedDescription)
        }
        objectWillChange.send()
    }

    /// 앱 종료 시 동기 정리. 덮개 플래그는 반드시 되돌린다.
    func shutdown() {
        assertion.release()
        guard engine.lidEngagedByUs else { return }
        if SystemProbes.restoreLidSleepWithWatchdog() {
            _ = engine.lidRestoreResult(true)
            persist()
        }
    }

    // MARK: - 당직 근무 인수인계

    func screenLocked() {
        lockedAt = Date()
    }

    func screenUnlocked() {
        defer { lockedAt = nil }
        guard let lockedAt,
              let report = HandoverReport.make(entries: log.recent, awayFrom: lockedAt, until: Date())
        else { return }
        notifier.send(report)
        say(Lines.Speech.handoverReport(), seconds: 6)
    }

    /// 녹화용 연출: 대기 → 출근 → 순찰 → 폭염 주의보 → 전투휴식 → 복귀 → 교대 경례 → 취침
    func playDemo() async {
        let steps: [(pose: Sprites.Pose, line: String?, seconds: Double)] = [
            (.standby, Lines.Speech.waiting(), 3.5),
            (.duty, Lines.Speech.arrived(), 3),
            (.duty, nil, 2),
            (.duty, Lines.Speech.heat(), 3),
            (.heat, Lines.Speech.rest(), 4),
            (.duty, Lines.Speech.backFromRest(), 3),
            (.salute, Lines.Speech.handover(), 4),
            (.sleeping, nil, 1.5),
        ]
        for step in steps {
            demoPose = step.pose
            if let line = step.line { say(line, seconds: step.seconds) }
            try? await Task.sleep(for: .seconds(step.seconds))
        }
    }

    // MARK: - 재시작 복귀

    /// 앱 재시작·업데이트·재부팅 후 저장된 근무를 이어간다.
    private func resume() async {
        guard !CommandLine.arguments.contains("--demo") else { return }
        let action = DutyResume.decide(stored: defaults.string(forKey: Keys.order),
                                       until: defaults.object(forKey: Keys.orderUntil) as? Double,
                                       now: Date())
        await run(engine.resume(action, now: Date()))
    }

    // MARK: - 근무 사이클 (5초마다)

    private func tick() async {
        guard !ticking, demoPose == nil else { return }
        ticking = true
        defer { ticking = false }

        // 근무가 없으면 프로세스 스캔은 30초에 한 번 (메뉴 표시용), 근무 중엔 5초마다
        let scanAgents = order != .off || Date().timeIntervalSince(lastScan) >= 30
        let customAgents = defaults.stringArray(forKey: Keys.customAgents) ?? []
        let kinds = AgentCatalog.builtIn + AgentCatalog.customKinds(customAgents)
        let (scanned, battery, assertions) = await Task.detached {
            (scanAgents ? SystemProbes.processes(kinds: kinds) : nil, SystemProbes.battery(), PowerHolderProbe.assertions())
        }.value
        otherHolders = PowerHolders.summarize(assertions, ownPID: getpid())
        now = Date()
        if let scanned {
            let cpu = cpuHistory.add(scanned.cpuTimes, at: now)
            agentStatus = AgentActivity.evaluate(scanned.snapshots, cpu: cpu,
                                                 kinds: kinds)
            agents = agentStatus.mapValues(\.present).filter { $0.value > 0 }
            lastScan = now
        }
        self.battery = battery

        let observation = DutyObservation(now: now, battery: battery, heat: currentHeat(),
                                          agentsOpen: agents,
                                          agentsBusy: agentStatus.mapValues(\.busy).filter { $0.value > 0 },
                                          assertionHeld: assertion.isActuallyHeld())
        await run(engine.tick(observation))
    }

    /// 실제 발열 단계. `defaults write ... simulateHeat advisory|alert|normal` 로 시험할 수 있다 (지우면 실제 값).
    private func currentHeat() -> HeatLevel {
        if let name = defaults.string(forKey: Keys.simulateHeat), let simulated = HeatLevel(name: name) {
            return simulated
        }
        return HeatLevel(ProcessInfo.processInfo.thermalState)
    }

    private func writeLog(_ text: String, kind: DutyLogEntry.Kind = .other) {
        log.write(text, kind: kind)
        objectWillChange.send()
    }
}

/// 알림 전송.
///
/// 실측(#5): macOS 는 Apple 개발자 서명(Team ID)이 없는 앱에 알림 권한을 주지 않는다
/// (UNErrorDomain Code=1 "Notifications are not allowed for this application"). 공증(#6) 전까지는
/// AppleScript `display notification` 으로 대신 보낸다 — 아이콘은 스크립트 편집기로 보인다.
/// 문구는 AppleScript 문자열에 끼워 넣지 않고 argv 로 넘겨 인젝션 여지가 없다.
final class Notifier: NSObject, UNUserNotificationCenterDelegate {
    private let center: UNUserNotificationCenter? =
        Bundle.main.bundleIdentifier != nil ? .current() : nil
    /// 권한이 확인되기 전까지는 대체 경로를 쓴다 (서명 없는 빌드에서는 끝까지 대체 경로)
    private var authorized = false

    override init() {
        super.init()
        center?.delegate = self
        center?.requestAuthorization(options: [.alert, .sound]) { [weak self] granted, error in
            self?.authorized = granted
            if ProcessInfo.processInfo.environment["BULCHIMBEON_DEBUG"] != nil {
                FileHandle.standardError.write(Data("[notify] authorization granted=\(granted) error=\(String(describing: error))\n".utf8))
            }
        }
    }

    func send(_ notice: (title: String, body: String)) {
        if let center, authorized {
            let content = UNMutableNotificationContent()
            content.title = notice.title
            content.body = notice.body
            content.sound = .default
            center.add(UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
            return
        }
        Self.sendViaAppleScript(notice)
    }

    private static func sendViaAppleScript(_ notice: (title: String, body: String)) {
        Task.detached {
            let process = Process()
            process.executableURL = URL(fileURLWithPath: "/usr/bin/osascript")
            process.arguments = ["-e", "on run argv",
                                 "-e", "display notification (item 2 of argv) with title (item 1 of argv)",
                                 "-e", "end run",
                                 String(notice.title.prefix(200)), String(notice.body.prefix(500))]
            process.environment = ["PATH": "/usr/bin:/bin"]
            process.standardOutput = FileHandle.nullDevice
            process.standardError = FileHandle.nullDevice
            try? process.run()
            process.waitUntilExit()
        }
    }

    // 메뉴가 열려 있어 앱이 활성 상태여도 배너를 보여준다
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }
}
