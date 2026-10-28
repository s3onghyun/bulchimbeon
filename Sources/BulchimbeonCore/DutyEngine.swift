import Foundation

public enum DutyMode: String, CaseIterable, Identifiable, Sendable {
    case display, system, lid

    public var id: String { rawValue }
    public var title: String {
        switch self {
        case .display: L10n.t("화면 켜둠 (caffeinate -d)", "Keep display on (caffeinate -d)", "保持屏幕常亮 (caffeinate -d)")
        case .system: L10n.t("화면은 꺼져도 맥은 깨어 있음 (caffeinate -i)", "Display may sleep, Mac stays awake (caffeinate -i)", "允许屏幕休眠，Mac 保持唤醒 (caffeinate -i)")
        case .lid: L10n.t("덮개 닫아도 깨어 있음 (관리자 권한)", "Stay awake with lid closed (admin)", "合盖后保持唤醒（需要管理员权限）")
        }
    }
}

public enum DutyOrder: Equatable, Sendable {
    case off
    case timed(until: Date?) // nil = 무기한
    case agentWatch
}

/// 엔진이 앱에 시키는 일. 앱은 순서대로 실행만 한다.
public enum DutyEffect: Equatable, Sendable {
    case acquire(DutyMode)   // 잠자기 방지 걸기 (display 면 화면까지)
    case release
    case reacquire           // 근무지 이탈: 같은 종류로 다시 걸기
    case enableLid           // 관리자 권한으로 덮개 잠자기 해제 → 결과는 lidEnableResult 로
    case restoreLid          // 덮개 잠자기 원복 → 결과는 lidRestoreResult 로
    case notify(title: String, body: String)
    case log(String, DutyLogEntry.Kind)
}

/// 한 번의 점검(5초)에 앱이 모아 오는 관찰값
public struct DutyObservation: Sendable {
    public var now: Date
    public var battery: BatteryStatus?
    public var heat: HeatLevel
    /// 열려 있는 에이전트 (종류별 수)
    public var agentsOpen: [AgentKind: Int]
    /// 작업 중인 에이전트 (종류별 수)
    public var agentsBusy: [AgentKind: Int]
    /// 들고 있어야 할 assertion 이 시스템에 실제로 남아 있는가
    public var assertionHeld: Bool

    public init(now: Date, battery: BatteryStatus? = nil, heat: HeatLevel = .normal,
                agentsOpen: [AgentKind: Int] = [:], agentsBusy: [AgentKind: Int] = [:], assertionHeld: Bool = true) {
        self.now = now; self.battery = battery; self.heat = heat
        self.agentsOpen = agentsOpen; self.agentsBusy = agentsBusy; self.assertionHeld = assertionHeld
    }
}

/// 불침번 근무 상태 머신. 시스템 호출이 없는 순수 로직이라 가짜 시계로 시나리오 테스트를 할 수 있다.
/// (실사용에서 잡힌 교대 버그는 모두 시간 흐름에 따른 상태 전이 문제였다 — 이슈 #2)
public struct DutyEngine: Sendable {
    public enum QuoteKind: Sendable { case onDuty, standby, off }

    // MARK: 설정
    /// 사용자가 메뉴에서 고른 근무 방식 (저장·메뉴 체크 표시). 대체 근무가 이 값을 바꾸지 않는다.
    public var mode: DutyMode
    public var batteryFloor: Int        // 0 = 끄기
    public var countIdleSessions: Bool  // true = 열려만 있어도 근무 (교대 안 함)
    public var graceMinutes: Int

    // MARK: 상태
    public private(set) var order: DutyOrder = .off
    public private(set) var isOnDuty = false
    public private(set) var dutyStartedAt: Date?
    public private(set) var resting = false
    public private(set) var heat: HeatLevel = .normal
    public private(set) var quote: QuoteKind = .off
    public private(set) var watchDetail: String?
    public private(set) var lastAgentSummary = ""
    public private(set) var lidSleepDisabled: Bool
    public private(set) var lidEngagedByUs: Bool
    /// 이번 근무만 덮개 대신 쓰는 방식 (재시작·암호 거부·과열 대체). 근무가 끝나면 nil
    public private(set) var shiftMode: DutyMode?

    /// 지금 실제로 서는 근무 방식
    public var effectiveMode: DutyMode { shiftMode ?? mode }

    private var saluteUntil: Date?
    private var speech: (text: String, until: Date)?
    private var lastBusyAt: Date?
    private var watchStartedAt: Date
    private var heatAdvised = false
    private var coolSince: Date?
    /// 덮개 해제 관리자 암호 창이 떠 있는 중 (아직 assertion 을 걸기 전)
    private var awaitingLid = false

    public init(mode: DutyMode = .system, batteryFloor: Int = 20, countIdleSessions: Bool = false, graceMinutes: Int = 5,
                lidSleepDisabled: Bool = false, lidEngagedByUs: Bool = false, now: Date = Date()) {
        self.mode = mode
        self.batteryFloor = batteryFloor
        self.countIdleSessions = countIdleSessions
        self.graceMinutes = graceMinutes
        self.lidSleepDisabled = lidSleepDisabled
        self.lidEngagedByUs = lidEngagedByUs
        self.watchStartedAt = now
    }

    // MARK: - 표시

    public func pose(at now: Date) -> Sprites.Pose {
        if resting { return .heat }
        if let saluteUntil, now < saluteUntil { return .salute }
        if isOnDuty { return .duty }
        return order == .agentWatch ? .standby : .sleeping
    }

    public func speech(at now: Date) -> String? {
        guard let speech, now < speech.until else { return nil }
        return speech.text
    }

    private var floor: Int? { batteryFloor > 0 ? batteryFloor : nil }

    // MARK: - 명령

    public mutating func startTimed(seconds: TimeInterval?, label: String, now: Date) -> [DutyEffect] {
        order = .timed(until: seconds.map { now.addingTimeInterval($0) })
        var effects = engage(now: now)
        let notice = Lines.reportForDuty()
        effects.append(.notify(title: notice.title, body: notice.body))
        say(Lines.Speech.reporting(), now: now)
        effects.append(.log(L10n.t("근무 투입 — \(label), \(effectiveMode.title)", "On duty — \(label), \(effectiveMode.title)", "开始值班 — \(label)，\(effectiveMode.title)"), .onDuty))
        return effects
    }

    public mutating func startAgentWatch(now: Date) -> [DutyEffect] {
        watchStartedAt = now
        order = .agentWatch
        quote = .standby
        return [.log(L10n.t("에이전트 근무 대기 시작", "Started waiting for agents", "开始等待代理"), .other)]
    }

    public mutating func stop(now: Date) -> [DutyEffect] {
        standDown(notice: nil, log: L10n.t("근무 종료 (수동)", "Off duty (manual)", "结束值班（手动）"))
    }

    /// 사용자가 고른 방식. 대체 근무 중이면 대체를 끝내고 고른 방식으로 선다.
    public mutating func setMode(_ newMode: DutyMode, now: Date) -> [DutyEffect] {
        let before = effectiveMode
        mode = newMode
        shiftMode = nil
        guard newMode != before, isOnDuty else { return [] }
        let leavingLid = before == .lid
        var effects: [DutyEffect] = leavingLid && lidEngagedByUs ? [.restoreLid] : []
        effects += engage(now: now)
        effects.append(.log(L10n.t("근무 방식 변경 — \(newMode.title)", "Watch mode changed — \(newMode.title)", "值班模式已更改 — \(newMode.title)"), .other))
        return effects
    }

    /// 앱 재시작 후 저장된 근무 이어가기
    public mutating func resume(_ action: DutyResume.Action, now: Date) -> [DutyEffect] {
        switch action {
        case .none:
            return []
        case .expired:
            order = .off
            return [.log(L10n.t("앱이 꺼져 있는 동안 근무 시간 종료", "Watch ended while the app was closed", "应用关闭期间，值班时间已到"), .other)]
        case .agentWatch, .timed:
            break
        }
        var effects: [DutyEffect] = []
        // 로그인 직후 예고 없는 관리자 암호 창은 피싱처럼 보인다 → 덮개 근무는 시스템 근무로 복귀
        if effectiveMode == .lid && !lidSleepDisabled {
            shiftMode = .system
            let notice = Lines.lidResumeFallback()
            effects += [.notify(title: notice.title, body: notice.body),
                        .log(L10n.t("덮개 근무 → 시스템 근무로 복귀 (관리자 재승인 필요)",
                                    "Lid-closed watch → resumed as system watch (admin approval needed)", "合盖值班 → 恢复系统值班（需要管理员授权）"), .other)]
        }
        switch action {
        case .agentWatch:
            watchStartedAt = now
            order = .agentWatch
            quote = .standby
            effects.append(.log(L10n.t("근무 복귀 — 에이전트 근무", "Back on watch — agent watch", "恢复值班 — 代理值班"), .other))
        case .timed(let until):
            order = .timed(until: until)
            effects += engage(now: now)
            say(Lines.Speech.resumed(), now: now)
            let left = until.map { DutyClock.format($0.timeIntervalSince(now)) }
            effects.append(.log(L10n.t("근무 복귀 — ", "Back on watch — ", "恢复值班 — ")
                                + (left.map { L10n.t("남은 시간 \($0)", "\($0) left", "剩余 \($0)") } ?? L10n.t("무기한", "indefinitely", "无限期")), .other))
        case .none, .expired:
            break
        }
        return effects
    }

    // MARK: - 비동기 결과

    public mutating func lidEnableResult(_ ok: Bool, now: Date) -> [DutyEffect] {
        awaitingLid = false
        if ok {
            lidEngagedByUs = true
            lidSleepDisabled = true
            return [.acquire(.lid)]
        }
        if order != .off { shiftMode = .system } // 암호 창이 떠 있는 사이 근무를 마쳤으면 다음 근무는 다시 덮개
        let notice = Lines.lidFallback()
        return [.notify(title: notice.title, body: notice.body),
                .log(L10n.t("덮개 근무 승인 거부 — 시스템 근무로 대체", "Lid-closed watch denied — switched to system watch", "合盖值班未获授权 — 已切换为系统值班"), .other),
                .acquire(.system)]
    }

    public mutating func lidRestoreResult(_ ok: Bool) -> [DutyEffect] {
        guard ok else { return [] }
        lidEngagedByUs = false
        lidSleepDisabled = false
        return [.log(L10n.t("덮개 잠자기 복구 완료", "Lid sleep restored", "已恢复合盖休眠"), .other)]
    }

    /// 덮개 근무 중 덮개 잠자기가 밖에서 복구됐다 (당직 감시병 등) → 이번 근무만 시스템 근무로
    public mutating func lidSleepRestoredExternally(now: Date) -> [DutyEffect] {
        guard isOnDuty, effectiveMode == .lid else { return [] }
        shiftMode = .system
        var effects = engage(now: now)
        effects.append(.log(L10n.t("근무 방식 변경 — \(DutyMode.system.title)", "Watch mode changed — \(DutyMode.system.title)",
                                   "值班模式已更改 — \(DutyMode.system.title)"), .other))
        return effects
    }

    // MARK: - 점검 (5초마다)

    public mutating func tick(_ o: DutyObservation) -> [DutyEffect] {
        let now = o.now
        var effects: [DutyEffect] = []

        if isOnDuty, BatteryPolicy.shouldRetreat(o.battery, floor: floor), let battery = o.battery {
            return standDown(notice: Lines.batteryRetreat(battery.percent),
                             log: L10n.t("배터리 \(battery.percent)% — 근무 철수", "Battery \(battery.percent)% — stood down", "电量 \(battery.percent)% — 结束值班"))
        }

        // 폭염 대응
        heat = o.heat
        if heat <= .caution {
            if coolSince == nil { coolSince = now }
            heatAdvised = false
        } else {
            coolSince = nil
        }
        switch HeatPolicy.decide(level: heat, onDuty: isOnDuty, resting: resting, advised: heatAdvised,
                                 coolSince: coolSince, now: now) {
        case .advisory:
            heatAdvised = true
            let notice = Lines.heatAdvisory()
            effects += [.notify(title: notice.title, body: notice.body), .log(L10n.t("폭염 주의보 발령", "Heat advisory", "高温提醒"), .heat)]
            say(Lines.Speech.heat(), now: now, seconds: 6)
        case .rest:
            resting = true
            effects += disengage()
            let notice = Lines.heatRest()
            effects += [.notify(title: notice.title, body: notice.body),
                        .log(L10n.t("폭염 경보 — 전투휴식 실시", "Heat warning — combat rest", "过热警报 — 暂停值班降温"), .heat)]
            say(Lines.Speech.rest(), now: now, seconds: 8)
        case .resume:
            resting = false
            let lidDowngraded = effectiveMode == .lid
            if lidDowngraded { shiftMode = .system } // 과열 직후엔 이번 근무 동안 덮개 근무로 돌아가지 않는다
            let notice = Lines.heatResume(lidDowngraded: lidDowngraded)
            effects += [.notify(title: notice.title, body: notice.body),
                        .log(L10n.t("전투휴식 종료 — 근무 복귀", "Combat rest over — back on watch", "降温休息结束 — 恢复值班"), .onDuty)]
            say(Lines.Speech.backFromRest(), now: now, seconds: 6)
            if case .timed = order { effects += engage(now: now) } // 에이전트 근무는 아래에서 다시 판단
        case .none:
            break
        }
        if resting { return effects }

        // 근무지 이탈 감지 (암호 창이 떠 있는 동안은 아직 안 걸린 게 정상)
        if isOnDuty && !awaitingLid && !o.assertionHeld {
            let notice = Lines.postAbandoned()
            effects += [.reacquire, .notify(title: notice.title, body: notice.body),
                        .log(L10n.t("⚠️ 근무지 이탈 감지 — 즉시 재투입", "⚠️ Post abandoned — re-engaged", "⚠️ 防休眠失效 — 已立即恢复"), .awol)]
            say(Lines.Speech.notAWOL(), now: now, seconds: 6)
        }

        switch order {
        case .off:
            break
        case .timed(let until):
            if let until, now >= until {
                saluteUntil = now.addingTimeInterval(10)
                say(Lines.Speech.timeUp(), now: now, seconds: 8)
                effects += standDown(notice: Lines.timeUp(), log: L10n.t("근무 시간 종료", "Watch time over", "值班时间已到"))
            }
        case .agentWatch:
            effects += agentWatchTick(o)
        }
        return effects
    }

    private mutating func agentWatchTick(_ o: DutyObservation) -> [DutyEffect] {
        let now = o.now
        // 일할 때만 근무: 투입은 지금 일하는 에이전트가 있을 때, 교대는 유예 시간 넘게 조용할 때
        let busy = o.agentsBusy.filter { $0.value > 0 }
        let open = o.agentsOpen.filter { $0.value > 0 }
        let busyNow: Bool
        if countIdleSessions {
            busyNow = !open.isEmpty
            if busyNow { lastAgentSummary = AgentCatalog.summary(open) }
        } else {
            busyNow = !busy.isEmpty
            if busyNow { lastAgentSummary = AgentCatalog.summary(busy) }
        }
        if busyNow { lastBusyAt = now }
        if lastAgentSummary.isEmpty { lastAgentSummary = L10n.t("에이전트", "Agents", "代理") }
        let quiet = AgentWatchPolicy.quietFor(now: now, lastBusyAt: lastBusyAt, watchStartedAt: watchStartedAt)
        let grace = TimeInterval(graceMinutes * 60)
        watchDetail = Self.watchDetail(busyNow: busyNow, countIdle: countIdleSessions, summary: lastAgentSummary,
                                       openCount: open.values.reduce(0, +), onDuty: isOnDuty, quiet: quiet, grace: grace)

        switch AgentWatchPolicy.decide(busyNow: busyNow, quietFor: quiet, grace: grace, onDuty: isOnDuty,
                                       lowBattery: BatteryPolicy.shouldRetreat(o.battery, floor: floor) || heat >= .alert) {
        case .engage:
            var effects = engage(now: now)
            let notice = Lines.agentArrived(lastAgentSummary)
            effects += [.notify(title: notice.title, body: notice.body),
                        .log(L10n.t("근무 투입 — \(lastAgentSummary) 출근", "On duty — \(lastAgentSummary) clocked in", "开始值班 — \(lastAgentSummary) 已开始工作"), .onDuty)]
            say(Lines.Speech.arrived(), now: now)
            return effects
        case .handover:
            var effects = disengage()
            quote = .standby
            saluteUntil = now.addingTimeInterval(10)
            let notice = Lines.handover(lastAgentSummary)
            effects += [.notify(title: notice.title, body: notice.body),
                        .log(L10n.t("근무 교대 — \(lastAgentSummary) 작업 종료, 이상 무",
                                    "Shift over — \(lastAgentSummary) finished, all clear", "交班 — \(lastAgentSummary) 已完成工作，一切正常"), .handover)]
            say(Lines.Speech.handover(), now: now, seconds: 8)
            return effects
        case .none:
            return []
        }
    }

    // MARK: - 내부

    private mutating func engage(now: Date) -> [DutyEffect] {
        if !isOnDuty { dutyStartedAt = now }
        isOnDuty = true
        quote = .onDuty
        if effectiveMode == .lid && !lidSleepDisabled { // 결과에 따라 lidEnableResult 가 acquire
            awaitingLid = true
            return [.enableLid]
        }
        return [.acquire(effectiveMode)]
    }

    private mutating func disengage() -> [DutyEffect] {
        isOnDuty = false
        dutyStartedAt = nil
        return lidEngagedByUs ? [.release, .restoreLid] : [.release]
    }

    private mutating func standDown(notice: Lines.Notice?, log: String) -> [DutyEffect] {
        var effects = disengage()
        resting = false
        order = .off
        quote = .off
        shiftMode = nil
        if let notice { effects.append(.notify(title: notice.title, body: notice.body)) }
        effects.append(.log(log, .standDown))
        return effects
    }

    private mutating func say(_ text: String, now: Date, seconds: TimeInterval = 4) {
        speech = (text, now.addingTimeInterval(seconds))
    }

    public static func watchDetail(busyNow: Bool, countIdle: Bool, summary: String, openCount: Int,
                                   onDuty: Bool, quiet: TimeInterval, grace: TimeInterval) -> String? {
        if countIdle && busyNow {
            return L10n.t("열린 세션 \(openCount)개 (열린 세션 인정 모드)", "\(openCount) open session(s) (counting idle sessions)", "\(openCount) 个已打开的会话（包括空闲会话）")
        }
        if busyNow { return L10n.t("\(summary) 작업 중", "\(summary) working", "\(summary) 正在工作") }
        guard onDuty else { return nil }
        return L10n.t("조용해진 지 \(DutyClock.format(quiet)) · 교대까지 \(DutyClock.format(grace - quiet))",
                      "Quiet for \(DutyClock.format(quiet)) · shift over in \(DutyClock.format(grace - quiet))", "已空闲 \(DutyClock.format(quiet)) · 距交班 \(DutyClock.format(grace - quiet))")
    }
}
