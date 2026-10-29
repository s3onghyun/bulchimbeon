import Foundation
import Testing
@testable import BulchimbeonCore

/// 가짜 시계로 돌리는 근무 시나리오. 실사용에서 잡힌 버그는 회귀 테스트로 남긴다.
@Suite struct DutyEngineScenarioTests {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)
    private let claude = AgentCatalog.builtIn.first { $0.id == "claude-code" }!

    private func at(_ seconds: TimeInterval) -> Date { t0.addingTimeInterval(seconds) }
    private func busy(_ s: TimeInterval) -> DutyObservation {
        DutyObservation(now: at(s), agentsOpen: [claude: 1], agentsBusy: [claude: 1])
    }
    private func idle(_ s: TimeInterval, open: Int = 1) -> DutyObservation {
        DutyObservation(now: at(s), agentsOpen: open > 0 ? [claude: open] : [:], agentsBusy: [:])
    }
    private func has(_ effects: [DutyEffect], _ match: (DutyEffect) -> Bool) -> Bool { effects.contains(where: match) }
    private func released(_ e: [DutyEffect]) -> Bool { has(e) { $0 == .release } }
    private func acquired(_ e: [DutyEffect]) -> Bool { has(e) { if case .acquire = $0 { true } else { false } } }
    private func logged(_ e: [DutyEffect], _ kind: DutyLogEntry.Kind) -> Bool {
        has(e) { if case .log(_, let k) = $0 { k == kind } else { false } }
    }
    private func notified(_ e: [DutyEffect]) -> Int { e.filter { if case .notify = $0 { true } else { false } }.count }

    // MARK: 시간 근무

    @Test func timedWatchExpiresWithSalute() {
        var engine = DutyEngine(now: t0)
        let start = engine.startTimed(seconds: 1800, label: "30분", now: t0)
        #expect(start.first == .acquire(.system))
        #expect(engine.isOnDuty && engine.pose(at: t0) == .duty)

        #expect(engine.tick(idle(600)).isEmpty, "10분 뒤 — 아무 일 없음")
        let end = engine.tick(idle(1801))
        #expect(released(end) && logged(end, .standDown))
        #expect(engine.order == .off && !engine.isOnDuty)
        #expect(engine.pose(at: at(1802)) == .salute, "끝나면 10초간 경례")
        #expect(engine.pose(at: at(1815)) == .sleeping)
    }

    /// 실측 (2026-10-14): 근무 중 방식을 바꿨는데 일지엔 이전 방식만 남아 실제 assertion과 어긋났다
    @Test func modeChangeOnDutyIsLogged() {
        var engine = DutyEngine(now: t0)
        _ = engine.startTimed(seconds: 3600, label: "1시간", now: t0)
        let change = engine.setMode(.display, now: at(60))
        #expect(change.first == .acquire(.display))
        #expect(has(change) { if case .log(let text, .other) = $0 { text.contains("caffeinate -d") } else { false } })
        #expect(engine.setMode(.display, now: at(70)).isEmpty, "같은 방식이면 아무 일 없음")

        engine = DutyEngine(now: t0)
        #expect(engine.setMode(.display, now: t0).isEmpty, "근무 중이 아니면 설정만 바뀜")
    }

    // MARK: 에이전트 근무

    @Test func agentClocksInAndHandsOverAfterGrace() {
        var engine = DutyEngine(graceMinutes: 5, now: t0)
        _ = engine.startAgentWatch(now: t0)
        #expect(engine.pose(at: t0) == .standby)

        let arrive = engine.tick(busy(5))
        #expect(acquired(arrive) && logged(arrive, .onDuty))
        #expect(engine.tick(busy(60)).isEmpty, "계속 일하는 중")
        #expect(engine.tick(idle(70)).isEmpty, "caffeinate 재실행 사이 짧은 공백 — 교대하면 안 됨")
        #expect(engine.tick(idle(300)).isEmpty, "조용해진 지 4분")
        let handover = engine.tick(idle(361)) // 마지막 작업(60s) + 5분
        #expect(released(handover) && logged(handover, .handover))
        #expect(engine.order == .agentWatch, "교대 후에도 에이전트 근무 대기는 계속")
        #expect(engine.pose(at: at(362)) == .salute)
        #expect(engine.pose(at: at(380)) == .standby)

        #expect(acquired(engine.tick(busy(400))), "다시 일하면 재투입")
    }

    /// 실측 버그 (2026-10-13 05:42): 시간 근무 → 에이전트 근무 전환 직후 유예 없이 교대했다가 8초 뒤 재투입
    @Test func regressionSwitchingFromTimedDoesNotFlicker() {
        var engine = DutyEngine(graceMinutes: 5, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = engine.startAgentWatch(now: at(100))
        let first = engine.tick(idle(105)) // 마침 caffeinate 공백
        #expect(!released(first), "전환 직후엔 근무 시작 시각부터 유예를 센다")
        #expect(engine.isOnDuty)
        #expect(released(engine.tick(idle(401))), "유예가 지나면 교대")
    }

    /// 실측 버그 (2026-10-13): "열린 세션 인정"이 켜져 있으면 열려 있는 한 교대하지 않는다 (의도된 동작)
    @Test func countingIdleSessionsNeverHandsOverWhileOpen() {
        var engine = DutyEngine(countIdleSessions: true, graceMinutes: 1, now: t0)
        _ = engine.startAgentWatch(now: t0)
        #expect(acquired(engine.tick(idle(5, open: 5))))
        for minute in 1...60 { #expect(!released(engine.tick(idle(Double(minute) * 60, open: 5)))) }
        #expect(engine.watchDetail?.contains("5") == true, "메뉴에 열린 세션 수와 이유 표시")
        #expect(released(engine.tick(idle(3700, open: 0)) + engine.tick(idle(3800, open: 0))), "모두 닫히면 교대")
    }

    @Test func watchDetailCountsDownToHandover() {
        var engine = DutyEngine(graceMinutes: 5, now: t0)
        _ = engine.startAgentWatch(now: t0)
        _ = engine.tick(busy(0))
        _ = engine.tick(idle(120))
        #expect(engine.watchDetail == AgentAwareText.quiet(120, left: 180))
    }

    // MARK: 폭염

    @Test func heatAdvisoryOnceThenCombatRestThenResume() {
        var engine = DutyEngine(now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)

        var hot = idle(10); hot.heat = .advisory
        #expect(notified(engine.tick(hot)) == 1, "주의보 알림")
        hot.now = at(15)
        #expect(notified(engine.tick(hot)) == 0, "주의보는 한 번만")

        var alert = idle(20); alert.heat = .alert
        let rest = engine.tick(alert)
        #expect(released(rest) && logged(rest, .heat))
        #expect(engine.resting && engine.pose(at: at(21)) == .heat)

        #expect(engine.tick(idle(30)).isEmpty, "식기 시작")
        #expect(!acquired(engine.tick(idle(100))), "식은 지 70초 — 아직 쉼")
        let back = engine.tick(idle(151)) // 30초부터 2분
        #expect(acquired(back) && !engine.resting && engine.isOnDuty)
    }

    @Test func heatResumeDowngradesLidMode() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: true, now: t0) // 덮개 이미 해제된 상태
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        var alert = idle(10); alert.heat = .alert
        _ = engine.tick(alert)
        _ = engine.tick(idle(20))
        let back = engine.tick(idle(141))
        #expect(back.contains(.acquire(.system)) && engine.effectiveMode == .system, "과열 직후엔 덮개 근무로 돌아가지 않는다")
    }

    @Test func heatAlertBlocksAgentEngage() {
        var engine = DutyEngine(now: t0)
        _ = engine.startAgentWatch(now: t0)
        var hotBusy = busy(5); hotBusy.heat = .alert
        #expect(!acquired(engine.tick(hotBusy)))
    }

    // MARK: 배터리 · 근무지 이탈

    @Test func lowBatteryStandsDownAndBlocksEngage() {
        var engine = DutyEngine(batteryFloor: 20, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        var low = idle(10); low.battery = BatteryStatus(percent: 15, onBattery: true)
        let retreat = engine.tick(low)
        #expect(released(retreat) && engine.order == .off)

        var watcher = DutyEngine(batteryFloor: 20, now: t0)
        _ = watcher.startAgentWatch(now: t0)
        var lowBusy = busy(5); lowBusy.battery = BatteryStatus(percent: 15, onBattery: true)
        #expect(!acquired(watcher.tick(lowBusy)))
        lowBusy.battery = BatteryStatus(percent: 15, onBattery: false)
        #expect(acquired(watcher.tick(lowBusy)), "충전 중이면 투입")
    }

    @Test func abandonedPostIsReacquired() {
        var engine = DutyEngine(now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        var lost = idle(10); lost.assertionHeld = false
        let effects = engine.tick(lost)
        #expect(effects.first == .reacquire && logged(effects, .awol))
    }

    // MARK: 덮개 모드

    @Test func lidModeRoundTrip() {
        var engine = DutyEngine(mode: .lid, now: t0)
        #expect(engine.startTimed(seconds: nil, label: "무기한", now: t0).first == .enableLid)
        #expect(engine.lidEnableResult(.approved, now: t0) == [.acquire(.lid)])
        #expect(engine.lidEngagedByUs && engine.lidSleepDisabled)
        let stop = engine.stop(now: at(10))
        #expect(Array(stop.prefix(2)) == [.release, .restoreLid])
        _ = engine.lidRestoreResult(true)
        #expect(!engine.lidEngagedByUs && !engine.lidSleepDisabled)
    }

    /// 구조 분리 중 발견: 덮개 암호 창이 떠 있는 동안 점검이 돌면 "근무지 이탈"로 오판했다
    @Test func noFalseAbandonWhileWaitingForAdminPassword() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        var waiting = idle(5); waiting.assertionHeld = false // 아직 안 걸림
        #expect(!engine.tick(waiting).contains(.reacquire))
        _ = engine.lidEnableResult(.approved, now: at(20))
        var lost = idle(25); lost.assertionHeld = false
        #expect(engine.tick(lost).contains(.reacquire), "암호 입력 후엔 다시 감시")
    }

    @Test func lidDeniedFallsBackToSystem() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        let denied = engine.lidEnableResult(.cancelled, now: t0)
        #expect(denied.last == .acquire(.system) && engine.effectiveMode == .system && !engine.lidEngagedByUs)
        #expect(engine.shiftReason == .approvalCancelled)
        #expect(denied.contains(.log(L10n.t("덮개 근무 승인 거부 — 시스템 근무로 대체", "Lid-closed watch denied — switched to system watch",
                                            "合盖值班未获授权 — 已切换为系统值班"), .other)), "취소는 지금처럼 거부로 기록")
    }

    /// 실측 (2026-10-28): 암호를 넣어도 osascript 가 끊겨 매번 "승인 거부"로 기록되고 "관리자 승인이 없어…" 알림이 떴다.
    @Test func lidApprovalFailureNamesTheCauseInsteadOfDenial() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        let failure = LidApproval.Failure.interrupted(signal: 15)
        let effects = engine.lidEnableResult(.failed(failure), now: t0)
        let log = L10n.t("덮개 근무 승인 실패 (\(failure.title(.ko))) — 시스템 근무로 대체",
                         "Lid-closed watch failed (\(failure.title(.en))) — switched to system watch",
                         "合盖值班失败（\(failure.title(.zh))）— 已切换为系统值班")
        let notice = Lines.lidApprovalFailed(failure)
        #expect(effects == [.notify(title: notice.title, body: notice.body), .log(log, .other), .acquire(.system)])
        #expect(engine.effectiveMode == .system && engine.mode == .lid && engine.shiftReason == .approvalFailed)
        #expect(Lines.lidApprovalFailed(failure, .ko) != Lines.lidFallback(.ko), "\"관리자 승인이 없어\" 가 아니라 원인을 알린다")
        #expect(Lines.lidApprovalFailed(failure, .ko).body.contains("신호 15"))
    }

    // MARK: 재시작 복귀

    @Test func resumeTimedContinuesRemainingTime() {
        var engine = DutyEngine(now: t0)
        let effects = engine.resume(.timed(until: at(600)), now: t0)
        #expect(acquired(effects) && engine.isOnDuty)
        #expect(released(engine.tick(idle(601))))
    }

    @Test func resumeLidModeFallsBackWithoutPasswordPrompt() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        let effects = engine.resume(.timed(until: nil), now: t0)
        #expect(!effects.contains(.enableLid), "로그인 직후 예고 없는 관리자 암호 창 금지")
        #expect(effects.contains(.acquire(.system)) && engine.effectiveMode == .system)
    }

    // MARK: 덮개 → 시스템 대체는 이번 근무만

    /// 실측 (2026-10-28): 덮개 근무 중 brew 업그레이드로 앱이 재시작돼 시스템 근무로 대체됐고,
    /// 근무를 마친 뒤에도 메뉴에 caffeinate -i 가 체크돼 있었다
    @Test func resumeLidFallbackKeepsLidPreference() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = engine.resume(.timed(until: nil), now: t0)
        #expect(engine.effectiveMode == .system && engine.mode == .lid, "메뉴 체크는 덮개, 이번 근무는 시스템")
        _ = engine.stop(now: at(60))
        #expect(engine.mode == .lid && engine.effectiveMode == .lid, "근무가 끝나면 고른 덮개 근무로 돌아온다")
        #expect(engine.startTimed(seconds: nil, label: "무기한", now: at(70)).first == .enableLid, "다음 근무는 다시 덮개 근무")
    }

    @Test func lidDeniedKeepsLidPreference() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: 600, label: "10분", now: t0)
        _ = engine.lidEnableResult(.cancelled, now: t0)
        #expect(engine.effectiveMode == .system && engine.mode == .lid)
        _ = engine.tick(idle(601))
        #expect(engine.order == .off)
        #expect(engine.mode == .lid && engine.effectiveMode == .lid, "암호 창을 취소해도 다음 근무는 다시 덮개 근무")
    }

    @Test func lidDeniedAfterStandDownLeavesNextShiftOnLid() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = engine.stop(now: at(5)) // 암호 창이 떠 있는 사이 근무 종료
        _ = engine.lidEnableResult(.cancelled, now: at(10))
        #expect(engine.effectiveMode == .lid)
    }

    @Test func heatResumeKeepsLidPreference() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: true, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        var alert = idle(10); alert.heat = .alert
        _ = engine.tick(alert)
        _ = engine.tick(idle(20))
        _ = engine.tick(idle(141))
        #expect(engine.effectiveMode == .system && engine.mode == .lid)
        _ = engine.stop(now: at(200))
        #expect(engine.mode == .lid && engine.effectiveMode == .lid, "과열 대체도 이번 근무만")
    }

    @Test func lidRestoredExternallyFallsBackForThisShiftOnly() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: true, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = engine.lidRestoreResult(true) // 당직 감시병이 먼저 원복
        let fallback = engine.lidSleepRestoredExternally(now: at(5))
        #expect(fallback.first == .acquire(.system) && engine.effectiveMode == .system && engine.mode == .lid)
        #expect(engine.lidSleepRestoredExternally(now: at(10)).isEmpty, "이미 시스템 근무면 아무 일 없음")
        _ = engine.stop(now: at(20))
        #expect(engine.effectiveMode == .lid)
    }

    @Test func normalLidShiftKeepsPreference() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: 600, label: "10분", now: t0)
        _ = engine.lidEnableResult(.approved, now: t0)
        #expect(engine.effectiveMode == .lid)
        _ = engine.tick(idle(601))
        _ = engine.lidRestoreResult(true)
        #expect(engine.order == .off && engine.mode == .lid && engine.effectiveMode == .lid)
    }

    @Test func userSwitchToSystemChangesPreference() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = engine.resume(.timed(until: nil), now: t0) // 대체 근무 중
        let effects = engine.setMode(.system, now: at(10))
        #expect(!acquired(effects), "이미 시스템 근무로 서 있으니 다시 걸지 않는다")
        // 실측 (2026-10-28): 대체 근무 중 caffeinate -i 를 고른 일이 일지에 없어 dutyMode=system 이 된 까닭을 알 수 없었다
        #expect(effects == [.log(L10n.t("근무 방식 변경 — \(DutyMode.system.title)", "Watch mode changed — \(DutyMode.system.title)",
                                        "值班模式已更改 — \(DutyMode.system.title)"), .other)], "저장되는 방식이 바뀌므로 일지에 남긴다")
        #expect(engine.shiftReason == nil)
        _ = engine.stop(now: at(20))
        #expect(engine.mode == .system && engine.effectiveMode == .system, "직접 고른 방식은 남는다")
        #expect(engine.setMode(.system, now: at(30)).isEmpty, "근무가 없으면 기록하지 않는다 (지금처럼)")
    }

    @Test func shiftReasonFollowsEachFallback() {
        var restarted = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = restarted.resume(.timed(until: nil), now: t0)
        #expect(restarted.shiftReason == .restarted)
        _ = restarted.stop(now: at(5))
        #expect(restarted.shiftReason == nil, "근무가 끝나면 대체도 끝")

        var external = DutyEngine(mode: .lid, lidSleepDisabled: true, now: t0)
        _ = external.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = external.lidRestoreResult(true)
        _ = external.lidSleepRestoredExternally(now: at(5))
        #expect(external.shiftReason == .lidRestoredExternally)
        _ = external.setMode(.lid, now: at(10))
        #expect(external.shiftReason == nil, "메뉴에서 고르면 대체가 끝난다")

        var heat = DutyEngine(mode: .lid, lidSleepDisabled: true, now: t0)
        _ = heat.startTimed(seconds: nil, label: "무기한", now: t0)
        var alert = idle(10); alert.heat = .alert
        _ = heat.tick(alert)
        _ = heat.tick(idle(20))
        _ = heat.tick(idle(141))
        #expect(heat.shiftReason == .heat)
        _ = heat.startTimed(seconds: 600, label: "10분", now: at(200))
        #expect(heat.shiftReason == nil, "새 근무는 새 교대")
    }

    /// 실측 (2026-10-29): 덮개 근무가 시스템 근무로 대체돼 화면이 꺼졌는데 화면 켜둠으로 알고 있었다.
    @Test func shiftLineStatesWhatThisWatchKeepsAwake() {
        var engine = DutyEngine(mode: .lid, now: t0)
        #expect(engine.shiftLine(.ko) == nil)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = engine.lidEnableResult(.failed(.interrupted(signal: 15)), now: t0)
        #expect(engine.shiftLine(.ko) == "이번 근무: 화면은 꺼짐 · 맥만 깨어 있음 (덮개 근무 실패)")
        #expect(engine.shiftLine(.en) == "This watch: display sleeps · only the Mac stays awake (lid-closed watch failed)")
        #expect(engine.shiftLine(.zh) == "本次值班：屏幕会关闭 · 仅 Mac 保持唤醒（合盖值班失败）")
        _ = engine.stop(now: at(10))
        #expect(engine.shiftLine(.ko) == nil)
    }

    @Test func modeEffectSaysWhetherTheDisplaySleeps() {
        #expect(DutyMode.display.effect(.ko) == "화면도 맥도 깨어 있음")
        #expect(DutyMode.system.effect(.ko) == "화면은 꺼짐 · 맥만 깨어 있음")
        #expect(DutyMode.lid.effect(.ko) == "덮개 닫아도 맥 깨어 있음 · 화면은 꺼짐")
        #expect(DutyMode.display.effect(.en) == "display and Mac stay awake")
        #expect(DutyMode.system.effect(.en) == "display sleeps · only the Mac stays awake")
        #expect(DutyMode.lid.effect(.en) == "Mac stays awake with the lid closed · display sleeps")
    }

    /// 실측 근무일지 (2026-10-28, 0.10.1): 덮개 근무를 세 번 시도해 모두 대체됐고, 고른 방식은 끝까지 덮개였다.
    /// 앱은 매 명령 뒤 `engine.mode` 를 `dutyMode` 로 저장하므로, 저장값은 사용자가 고른 것만 따라야 한다.
    @Test func captainsLidAttemptsNeverStoreTheFallback() {
        var engine = DutyEngine(mode: .lid, now: t0) // 05:54:34 에 덮개를 고른 뒤
        _ = engine.startTimed(seconds: 14400, label: "4시간", now: at(0))           // 05:59:57
        _ = engine.lidEnableResult(.failed(.interrupted(signal: 15)), now: at(0))
        #expect(engine.mode == .lid)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: at(56))            // 06:00:53
        _ = engine.lidEnableResult(.failed(.interrupted(signal: 15)), now: at(56))
        #expect(engine.mode == .lid)
        let retry = engine.setMode(.lid, now: at(14_852))                           // 10:07:29
        #expect(retry.first == .enableLid)
        _ = engine.lidEnableResult(.failed(.interrupted(signal: 15)), now: at(14_852))
        #expect(engine.mode == .lid && engine.effectiveMode == .system && engine.shiftReason == .approvalFailed)
        // 그 뒤 메뉴에서 caffeinate -i 를 고르면 저장값이 system 이 되고, 이제 일지에 남는다
        #expect(engine.setMode(.system, now: at(15_000)).count == 1)
        #expect(engine.mode == .system && engine.shiftLine(.ko) == nil)
    }

    @Test func choosingLidAgainEndsFallback() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = engine.resume(.timed(until: nil), now: t0)
        #expect(engine.setMode(.lid, now: at(10)).first == .enableLid, "메뉴에서 덮개를 다시 고르면 이번 근무도 덮개로")
        #expect(engine.effectiveMode == .lid)
    }

    @Test func newTimedWatchAfterAgentWatchFallbackAsksForLid() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = engine.resume(.agentWatch, now: t0)
        #expect(engine.effectiveMode == .system)
        let effects = engine.startTimed(seconds: nil, label: "무기한", now: at(10))
        #expect(effects.first == .enableLid && engine.effectiveMode == .lid, "새 근무는 새 교대: 다시 덮개 승인")
    }

    @Test func newTimedWatchReplacingHeatDowngradeAsksForLid() {
        var engine = DutyEngine(mode: .lid, now: t0)
        _ = engine.startTimed(seconds: nil, label: "무기한", now: t0)
        _ = engine.lidEnableResult(.approved, now: t0)
        var alert = idle(10); alert.heat = .alert
        _ = engine.tick(alert)
        _ = engine.lidRestoreResult(true)
        _ = engine.tick(idle(20))
        _ = engine.tick(idle(141))
        #expect(engine.effectiveMode == .system && engine.isOnDuty)
        let effects = engine.startTimed(seconds: 600, label: "10분", now: at(200))
        #expect(effects.first == .enableLid && engine.effectiveMode == .lid)
    }

    @Test func agentWatchStartedDuringFallbackReengagesAsLid() {
        var engine = DutyEngine(mode: .lid, lidSleepDisabled: false, now: t0)
        _ = engine.resume(.timed(until: nil), now: t0)
        let effects = engine.startAgentWatch(now: at(10))
        #expect(effects.first == .enableLid && engine.effectiveMode == .lid, "시스템 assertion 을 든 채 덮개라고 표시하지 않는다")
    }

    @Test func resumeExpiredStaysOff() {
        var engine = DutyEngine(now: t0)
        _ = engine.resume(.expired, now: t0)
        #expect(engine.order == .off && !engine.isOnDuty)
    }
}

/// 메뉴 문구 기대값 (시스템 언어에 따라 다름)
enum AgentAwareText {
    static func quiet(_ quiet: TimeInterval, left: TimeInterval) -> String {
        L10n.t("조용해진 지 \(DutyClock.format(quiet)) · 교대까지 \(DutyClock.format(left))",
               "Quiet for \(DutyClock.format(quiet)) · shift over in \(DutyClock.format(left))",
               "已空闲 \(DutyClock.format(quiet)) · 距交班 \(DutyClock.format(left))")
    }
}
