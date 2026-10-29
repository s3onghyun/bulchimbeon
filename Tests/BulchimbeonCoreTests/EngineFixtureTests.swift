import Foundation
import Testing
@testable import BulchimbeonCore

/// 공유 골든 픽스처 (`testdata/engine/*.json`). Go 포트(`internal/engine`)도 같은 파일을 돌린다.
/// Swift Core 가 기준이다: 여기가 실패하면 픽스처가 틀린 것이고, Go 쪽이 실패하면 포트가 틀린 것이다.
/// 형식은 `testdata/engine/README.md`.
///
/// Shared golden fixtures. The Swift core defines the truth; the Go port runs the same files.
/// Texts without a language parameter in the core (engine logs, reason labels) are checked in the
/// system language (`L10n.language`); everything else in Korean, English and Simplified Chinese.
@Suite struct EngineFixtureTests {
    private let lang = L10n.language
    private var langKey: String { lang.rawValue }

    // MARK: 카탈로그 · 활동 판별

    @Test func catalog() throws {
        let f = try Fixture.load("catalog.json")

        let builtIn = f["builtIn"].array
        #expect(AgentCatalog.builtIn.map(\.id) == builtIn.map { $0["id"].string ?? "" }, "builtIn 순서·목록")
        for (kind, want) in zip(AgentCatalog.builtIn, builtIn) {
            #expect(kind.displayName == want["displayName"].string)
            #expect(kind.binaryNames == Set(want["binaryNames"].strings), "\(kind.id) binaryNames")
            #expect(kind.pathMarkers == want["pathMarkers"].strings, "\(kind.id) pathMarkers")
            #expect(kind.packageMarkers == want["packageMarkers"].strings, "\(kind.id) packageMarkers")
        }

        for c in f["match"].array {
            let name = c["name"].string ?? ""
            let process = ProcessSnapshot(executablePath: c["exe"].string ?? "", arguments: c["args"].strings)
            let match = AgentCatalog.match(process, kinds: kinds(c["custom"]))
            #expect(match?.kind.id == c["kind"].string, "match: \(name)")
            if let reason = c["reason"].string {
                #expect(match.map { Self.reasonName($0.reason) } == reason, "reason: \(name)")
            }
        }
        for c in f["isCandidate"].array {
            #expect(AgentCatalog.isCandidate(executablePath: c["exe"].string ?? "", argv0: c["argv0"].string,
                                             kinds: kinds(c["custom"])) == c["expect"].bool, "isCandidate: \(c)")
        }
        for c in f["needsFullArguments"].array {
            #expect(AgentCatalog.needsFullArguments(executablePath: c["exe"].string ?? "") == c["expect"].bool, "needsFullArguments: \(c)")
        }
        for c in f["customKinds"].array {
            #expect(AgentCatalog.customKinds(c["input"].strings).map(\.displayName) == c["displayNames"].strings,
                    "customKinds: \(c["name"].string ?? "")")
        }
        for c in f["count"].array {
            let processes = c["processes"].array.map {
                ProcessSnapshot(executablePath: $0["exe"].string ?? "", arguments: $0["args"].strings)
            }
            let counts = AgentCatalog.count(processes, kinds: kinds(c["custom"]))
            #expect(Dictionary(uniqueKeysWithValues: counts.map { ($0.key.id, $0.value) }) == c["expect"].object.mapValues { $0.int ?? -1 },
                    "count: \(c["name"].string ?? "")")
        }
        for c in f["summary"].array {
            let all = kinds(c["custom"])
            var counts: [AgentKind: Int] = [:]
            for (id, n) in c["counts"].object {
                let kind = try #require(all.first { $0.id == id }, "unknown kind \(id)")
                counts[kind] = n.int
            }
            #expect(AgentCatalog.summary(counts) == c["expect"].string, "summary: \(c)")
        }
    }

    @Test func activity() throws {
        let f = try Fixture.load("activity.json")
        #expect(AgentActivity.cpuThreshold == f["cpuThreshold"].double)
        #expect(AgentActivity.busyGrace == f["busyGraceSeconds"].double)
        for c in f["evaluate"].array {
            let processes = c["processes"].array.map { p in
                ProcessSnapshot(pid: Int32(p["pid"].int ?? 0), ppid: Int32(p["ppid"].int ?? 0),
                                executablePath: p["exe"].string ?? "", arguments: p["args"].strings)
            }
            var cpu: [Int32: Double] = [:]
            for (pid, value) in c["cpu"].object { cpu[Int32(pid) ?? -1] = value.double }
            let status = AgentActivity.evaluate(processes, cpu: cpu, kinds: kinds(c["custom"]))
            let got = Dictionary(uniqueKeysWithValues: status.map { ($0.key.id, [$0.value.present, $0.value.busy]) })
            let want = c["expect"].object.mapValues { [$0["present"].int ?? -1, $0["busy"].int ?? -1] }
            #expect(got == want, "evaluate: \(c["name"].string ?? "")")
        }
    }

    @Test func cpuHistory() throws {
        let f = try Fixture.load("cpu_history.json")
        let tolerance = f["tolerance"].double ?? 1e-9
        for c in f["cases"].array {
            var history = CPUHistory(window: c["window"].double ?? 60)
            for (i, sample) in c["samples"].array.enumerated() {
                var times: [Int32: UInt64] = [:]
                for (pid, value) in sample["times"].object { times[Int32(pid) ?? -1] = UInt64(value.double ?? 0) }
                let got = history.add(times, at: Date(timeIntervalSince1970: Fixture.cpuEpoch + (sample["at"].double ?? 0)))
                guard case .object(let want) = sample["expect"] else { continue }
                let label = "\(c["name"].string ?? "") sample \(i)"
                #expect(Set(got.keys) == Set(want.keys.compactMap { Int32($0) }), "\(label): \(got)")
                for (pid, value) in want {
                    let actual = got[Int32(pid) ?? -1] ?? .nan
                    #expect(abs(actual - (value.double ?? .nan)) <= tolerance, "\(label) pid \(pid): \(actual)")
                }
            }
        }
    }

    // MARK: 정책

    @Test func policies() throws {
        let f = try Fixture.load("policies.json")
        for c in f["batteryRetreat"].array {
            let battery = c["battery"].isNull ? nil
                : BatteryStatus(percent: c["battery"]["percent"].int ?? 0, onBattery: c["battery"]["onBattery"].bool ?? false)
            #expect(BatteryPolicy.shouldRetreat(battery, floor: c["floor"].int) == c["expect"].bool, "batteryRetreat: \(c)")
        }
        for c in f["agentWatchDecide"].array {
            let action = AgentWatchPolicy.decide(busyNow: c["busyNow"].bool ?? false, quietFor: c["quietFor"].double ?? 0,
                                                 grace: c["grace"].double ?? 0, onDuty: c["onDuty"].bool ?? false,
                                                 lowBattery: c["lowBattery"].bool ?? false)
            #expect(Self.watchActionName(action) == c["expect"].string, "agentWatchDecide: \(c)")
        }
        for c in f["quietFor"].array {
            let quiet = AgentWatchPolicy.quietFor(now: Date(timeIntervalSince1970: c["now"].double ?? 0),
                                                  lastBusyAt: c["lastBusyAt"].double.map { Date(timeIntervalSince1970: $0) },
                                                  watchStartedAt: Date(timeIntervalSince1970: c["watchStartedAt"].double ?? 0))
            #expect(quiet == c["expect"].double, "quietFor: \(c)")
        }
        #expect(HeatPolicy.coolDown == f["heatCoolDownSeconds"].double)
        for c in f["heatDecide"].array {
            let level = try #require(HeatLevel(name: c["level"].string ?? ""))
            let action = HeatPolicy.decide(level: level, onDuty: c["onDuty"].bool ?? false, resting: c["resting"].bool ?? false,
                                           advised: c["advised"].bool ?? false,
                                           coolSince: c["coolSince"].double.map { Date(timeIntervalSince1970: $0) },
                                           now: Date(timeIntervalSince1970: c["now"].double ?? 0))
            #expect(Self.heatActionName(action) == c["expect"].string, "heatDecide: \(c)")
        }
        for c in f["heatLevelName"].array {
            #expect(HeatLevel(name: c["name"].string ?? "").map(Self.heatName) == c["expect"].string, "heatLevelName: \(c)")
        }
        for c in f["dutyResume"].array {
            let until: Double? = c["until"].string == "inf" ? .infinity : c["until"].double
            let action = DutyResume.decide(stored: c["stored"].string, until: until,
                                           now: Date(timeIntervalSince1970: c["now"].double ?? 0))
            let want: DutyResume.Action
            switch c["expect"]["action"].string {
            case "agentWatch": want = .agentWatch
            case "expired": want = .expired
            case "timed": want = .timed(until: c["expect"]["until"].double.map { Date(timeIntervalSince1970: $0) })
            default: want = .none
            }
            #expect(action == want, "dutyResume: \(c)")
        }
        for c in f["dutyClock"].array {
            #expect(DutyClock.format(c["seconds"].double ?? 0) == c["expect"].string, "dutyClock: \(c)")
        }
    }

    @Test func settings() throws {
        let f = try Fixture.load("settings.json")
        #expect(SettingsKey.allCases.filter(\.isPublic).map(\.rawValue) == f["publicKeys"].strings)
        #expect(SettingsKey.allCases.filter { !$0.isPublic }.map(\.rawValue) == f["internalKeys"].strings)
        #expect(DutyMode.allCases.map(\.rawValue) == f["dutyModes"].strings)
        #expect(SettingsValues.batteryFloors == f["batteryFloors"].array.compactMap(\.int))
        #expect(SettingsValues.defaultBatteryFloor == f["defaultBatteryFloor"].int)
        #expect(SettingsValues.graceMinutes == f["graceMinutes"].array.compactMap(\.int))
        #expect(SettingsValues.defaultGraceMinutes == f["defaultGraceMinutes"].int)
        #expect(SettingsValues.patrolScales == f["patrolScales"].array.compactMap(\.int))
        #expect(SettingsValues.defaultPatrolScale == f["defaultPatrolScale"].int)
        #expect(SettingsValues.defaultUniformTheme == f["defaultUniformTheme"].string)
        #expect(AgentCatalog.customKinds((0..<40).map { "a\($0)" }).count == f["maxCustomAgents"].int)
        #expect(LaunchArgument.allCases.map(\.rawValue) == f["launchArguments"].strings)
        let engine = DutyEngine(now: Date(timeIntervalSince1970: 0))
        let defaults = f["engineDefaults"]
        #expect(engine.mode.rawValue == defaults["mode"].string)
        #expect(engine.batteryFloor == defaults["batteryFloor"].int)
        #expect(engine.countIdleSessions == defaults["countIdleSessions"].bool)
        #expect(engine.graceMinutes == defaults["graceMinutes"].int)
    }

    @Test func dutyLogLines() throws {
        let f = try Fixture.load("duty_log.json")
        for c in f["lines"].array {
            let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            defer { try? FileManager.default.removeItem(at: dir) }
            let file = dir.appendingPathComponent("duty-log.txt")
            DutyLog(fileURL: file).write(c["text"].string ?? "", at: Date(timeIntervalSince1970: c["at"].double ?? 0))
            #expect(try String(contentsOf: file, encoding: .utf8) == c["line"].string, "dutyLog: \(c["name"].string ?? "")")
        }
    }

    // MARK: 대사

    @Test func lines() throws {
        let f = try Fixture.load("lines.json")
        for c in f["detect"].array {
            #expect(L10n.detect(c["preferred"].strings).rawValue == c["expect"].string, "detect: \(c)")
        }
        for c in f["lines"].array {
            let fn = c["fn"].string ?? ""
            let langs: [L10n.Language] = c["systemLanguage"].bool == true ? [lang] : L10n.Language.allCases
            for l in langs {
                let got = try Self.line(fn, c["args"], l)
                #expect(got == c[l.rawValue], "lines: \(fn) \(c["args"]) [\(l)]")
            }
        }
    }

    // MARK: 근무 상태 머신

    @Test func engineScenarios() throws {
        let f = try Fixture.load("engine_scenarios.json")
        let t0 = f["t0"].double ?? 0
        for scenario in f["scenarios"].array { try run(scenario, t0: t0) }
    }

    private func run(_ scenario: JSONValue, t0: Double) throws {
        let name = scenario["name"].string ?? ""
        let config = scenario["config"]
        func date(_ v: JSONValue) -> Date { Date(timeIntervalSince1970: t0 + (v.double ?? 0)) }
        func relative(_ d: Date?) -> JSONValue { d.map { .number($0.timeIntervalSince1970 - t0) } ?? .null }

        let mode = try #require(DutyMode(rawValue: config["mode"].string ?? "system"))
        var engine = DutyEngine(mode: mode, batteryFloor: config["batteryFloor"].int ?? 20,
                                countIdleSessions: config["countIdleSessions"].bool ?? false,
                                graceMinutes: config["graceMinutes"].int ?? 5,
                                lidSleepDisabled: config["lidSleepDisabled"].bool ?? false,
                                lidEngagedByUs: config["lidEngagedByUs"].bool ?? false,
                                now: Date(timeIntervalSince1970: t0))
        var expected: [String: JSONValue] = [
            "isOnDuty": .bool(false), "order": .string("off"), "orderUntil": .null, "resting": .bool(false),
            "mode": .string(mode.rawValue), "effectiveMode": .string(mode.rawValue), "heat": .string("normal"), "quote": .string("off"), "dutyStartedAt": .null,
            "watchDetail": .null, "lidEngagedByUs": .bool(config["lidEngagedByUs"].bool ?? false),
            "lidSleepDisabled": .bool(config["lidSleepDisabled"].bool ?? false), "shiftReason": .null,
        ]

        for (i, step) in scenario["steps"].array.enumerated() {
            let op = step["op"].string ?? ""
            let now = date(step["at"])
            let effects: [DutyEffect]
            switch op {
            case "startTimed":
                effects = engine.startTimed(seconds: step["seconds"].double, label: step["label"].string ?? "", now: now)
            case "startAgentWatch": effects = engine.startAgentWatch(now: now)
            case "stop": effects = engine.stop(now: now)
            case "setMode": effects = engine.setMode(try #require(DutyMode(rawValue: step["mode"].string ?? "")), now: now)
            case "resume":
                let r = step["resume"]
                let action: DutyResume.Action
                switch r["action"].string {
                case "agentWatch": action = .agentWatch
                case "expired": action = .expired
                case "timed": action = .timed(until: r["until"].isNull ? nil : date(r["until"]))
                default: action = .none
                }
                effects = engine.resume(action, now: now)
            case "lidEnableResult":
                let outcome: LidApproval.Outcome = step["ok"].bool == true ? .approved
                    : step.has("failure") ? .failed(Self.lidFailure(step["failure"])) : .cancelled
                effects = engine.lidEnableResult(outcome, now: now)
            case "lidRestoreResult": effects = engine.lidRestoreResult(step["ok"].bool ?? false)
            case "lidSleepRestoredExternally": effects = engine.lidSleepRestoredExternally(now: now)
            case "tick":
                let battery = step["battery"].isNull ? nil
                    : BatteryStatus(percent: step["battery"]["percent"].int ?? 0, onBattery: step["battery"]["onBattery"].bool ?? false)
                effects = engine.tick(DutyObservation(
                    now: now, battery: battery, heat: try #require(HeatLevel(name: step["heat"].string ?? "normal")),
                    agentsOpen: try counts(step["open"]), agentsBusy: try counts(step["busy"]),
                    assertionHeld: step["assertionHeld"].bool ?? true))
            default:
                Issue.record("\(name): unknown op \(op)")
                return
            }
            let label = "\(name) — step \(i + 1) (\(op) at \(step["at"]))"

            let want = try step["effects"].array.map(resolve)
            #expect(effects.map(encode) == want, "\(label): effects")

            for (key, value) in step["state"].object { expected[key] = pick(value) }
            var state: [String: JSONValue] = [
                "isOnDuty": .bool(engine.isOnDuty), "resting": .bool(engine.resting), "mode": .string(engine.mode.rawValue),
                "effectiveMode": .string(engine.effectiveMode.rawValue),
                "heat": .string(Self.heatName(engine.heat)), "quote": .string(Self.quoteName(engine.quote)),
                "dutyStartedAt": relative(engine.dutyStartedAt), "watchDetail": engine.watchDetail.map(JSONValue.string) ?? .null,
                "lidEngagedByUs": .bool(engine.lidEngagedByUs), "lidSleepDisabled": .bool(engine.lidSleepDisabled),
                "shiftReason": engine.shiftReason.map { .string($0.rawValue) } ?? .null,
            ]
            switch engine.order {
            case .off: state["order"] = .string("off"); state["orderUntil"] = .null
            case .agentWatch: state["order"] = .string("agentWatch"); state["orderUntil"] = .null
            case .timed(let until): state["order"] = .string("timed"); state["orderUntil"] = relative(until)
            }
            #expect(state == expected, "\(label): state")

            for check in step["checks"].array {
                let at = date(check["at"])
                if let pose = check["pose"].string {
                    #expect(engine.pose(at: at).rawValue == pose, "\(label): pose at \(check["at"])")
                }
                if check.has("speech") {
                    let want = try check["speech"].string.map { try Self.line($0, .null, lang) }
                    #expect(engine.speech(at: at).map(JSONValue.string) == want, "\(label): speech at \(check["at"])")
                }
            }
        }
    }

    // MARK: 도우미

    private func kinds(_ custom: JSONValue) -> [AgentKind] {
        AgentCatalog.builtIn + AgentCatalog.customKinds(custom.strings)
    }

    private func counts(_ v: JSONValue) throws -> [AgentKind: Int] {
        var result: [AgentKind: Int] = [:]
        for (id, n) in v.object {
            let kind = try #require(AgentCatalog.builtIn.first { $0.id == id }, "unknown kind \(id)")
            result[kind] = n.int
        }
        return result
    }

    /// `{"ko": …, "en": …, "zh": …}` → 시스템 언어 값
    private func pick(_ v: JSONValue) -> JSONValue {
        if case .object(let o) = v, o["ko"] != nil, o["en"] != nil, let value = o[langKey], o.count == 3 { return value }
        return v
    }

    private func encode(_ effect: DutyEffect) -> JSONValue {
        switch effect {
        case .acquire(let mode): .object(["type": .string("acquire"), "mode": .string(mode.rawValue)])
        case .release: .object(["type": .string("release")])
        case .reacquire: .object(["type": .string("reacquire")])
        case .enableLid: .object(["type": .string("enableLid")])
        case .restoreLid: .object(["type": .string("restoreLid")])
        case .notify(let title, let body): .object(["type": .string("notify"), "title": .string(title), "body": .string(body)])
        case .log(let text, let kind): .object(["type": .string("log"), "kind": .string(Self.logKindName(kind)), "text": .string(text)])
        }
    }

    /// 픽스처의 효과 → `encode` 와 같은 모양 (대사 참조와 언어별 문구를 풀어서)
    private func resolve(_ effect: JSONValue) throws -> JSONValue {
        switch effect["type"].string {
        case "notify":
            let notice = try Self.line(effect["line"].string ?? "", pick(effect["args"]), lang)
            return .object(["type": .string("notify"), "title": notice["title"], "body": notice["body"]])
        case "log":
            return .object(["type": .string("log"), "kind": effect["kind"], "text": effect[langKey]])
        default:
            return effect
        }
    }

    static func line(_ fn: String, _ args: JSONValue, _ l: L10n.Language) throws -> JSONValue {
        func notice(_ n: Lines.Notice) -> JSONValue { .object(["title": .string(n.title), "body": .string(n.body)]) }
        func strings(_ s: [String]) -> JSONValue { .array(s.map(JSONValue.string)) }
        switch fn {
        case "onDuty": return strings(Lines.onDuty(l))
        case "standby": return strings(Lines.standby(l))
        case "off": return strings(Lines.off(l))
        case "reportForDuty": return notice(Lines.reportForDuty(l))
        case "agentArrived": return notice(Lines.agentArrived(args[0].string ?? "", l))
        case "handover": return notice(Lines.handover(args[0].string ?? "", l))
        case "timeUp": return notice(Lines.timeUp(l))
        case "batteryRetreat": return notice(Lines.batteryRetreat(args[0].int ?? 0, l))
        case "lidFallback": return notice(Lines.lidFallback(l))
        case "lidApprovalFailed": return notice(Lines.lidApprovalFailed(lidFailure(args[0]), l))
        case "lidResumeFallback": return notice(Lines.lidResumeFallback(l))
        case "lidLeftOn": return notice(Lines.lidLeftOn(l))
        case "heatAdvisory": return notice(Lines.heatAdvisory(l))
        case "heatRest": return notice(Lines.heatRest(l))
        case "heatResume": return notice(Lines.heatResume(lidDowngraded: args[0].bool ?? false, l))
        case "postAbandoned": return notice(Lines.postAbandoned(l))
        case "speech.reporting": return .string(Lines.Speech.reporting(l))
        case "speech.arrived": return .string(Lines.Speech.arrived(l))
        case "speech.handover": return .string(Lines.Speech.handover(l))
        case "speech.timeUp": return .string(Lines.Speech.timeUp(l))
        case "speech.resumed": return .string(Lines.Speech.resumed(l))
        case "speech.heat": return .string(Lines.Speech.heat(l))
        case "speech.rest": return .string(Lines.Speech.rest(l))
        case "speech.backFromRest": return .string(Lines.Speech.backFromRest(l))
        case "speech.notAWOL": return .string(Lines.Speech.notAWOL(l))
        case "speech.handoverReport": return .string(Lines.Speech.handoverReport(l))
        case "speech.waiting": return .string(Lines.Speech.waiting(l))
        // 아래는 Core 에 언어 인자가 없어 시스템 언어로만 확인한다 ("systemLanguage": true)
        case "reason.executableName": return .string(AgentCatalog.Reason.executableName.label)
        case "reason.argv0": return .string(AgentCatalog.Reason.argv0.label)
        case "reason.installPath": return .string(AgentCatalog.Reason.installPath.label)
        case "reason.script": return .string(AgentCatalog.Reason.script.label)
        case "mode.display": return .string(DutyMode.display.title)
        case "mode.system": return .string(DutyMode.system.title)
        case "mode.lid": return .string(DutyMode.lid.title)
        case "watchDetail":
            let detail = DutyEngine.watchDetail(busyNow: args["busyNow"].bool ?? false, countIdle: args["countIdle"].bool ?? false,
                                                summary: args["summary"].string ?? "", openCount: args["openCount"].int ?? 0,
                                                onDuty: args["onDuty"].bool ?? false, quiet: args["quiet"].double ?? 0,
                                                grace: args["grace"].double ?? 0)
            return detail.map(JSONValue.string) ?? .null
        default:
            Issue.record("unknown line function \(fn)")
            return .null
        }
    }

    static func reasonName(_ r: AgentCatalog.Reason) -> String {
        switch r {
        case .executableName: "executableName"
        case .argv0: "argv0"
        case .installPath: "installPath"
        case .script: "script"
        }
    }

    /// `{cause: stopSignal | launch | interrupted (signal) | error (code?)}`
    static func lidFailure(_ v: JSONValue) -> LidApproval.Failure {
        switch v["cause"].string {
        case "stopSignal": .stopSignal
        case "launch": .launch
        case "interrupted": .interrupted(signal: Int32(v["signal"].int ?? 0))
        default: .error(code: v["code"].int)
        }
    }

    static func heatName(_ h: HeatLevel) -> String {
        switch h {
        case .normal: "normal"
        case .caution: "caution"
        case .advisory: "advisory"
        case .alert: "alert"
        }
    }

    static func quoteName(_ q: DutyEngine.QuoteKind) -> String {
        switch q {
        case .onDuty: "onDuty"
        case .standby: "standby"
        case .off: "off"
        }
    }

    static func logKindName(_ k: DutyLogEntry.Kind) -> String {
        switch k {
        case .onDuty: "onDuty"
        case .handover: "handover"
        case .standDown: "standDown"
        case .heat: "heat"
        case .awol: "awol"
        case .other: "other"
        }
    }

    static func watchActionName(_ a: AgentWatchPolicy.Action) -> String {
        switch a {
        case .engage: "engage"
        case .handover: "handover"
        case .none: "none"
        }
    }

    static func heatActionName(_ a: HeatPolicy.Action) -> String {
        switch a {
        case .none: "none"
        case .advisory: "advisory"
        case .rest: "rest"
        case .resume: "resume"
        }
    }
}

/// 픽스처 파일 위치와 읽기
enum Fixture {
    /// 저장소 루트의 `testdata/engine` (이 파일: `Tests/BulchimbeonCoreTests/EngineFixtureTests.swift`)
    static let directory = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("testdata/engine", isDirectory: true)

    /// cpu_history.json 의 `at` 은 이 시각부터의 초
    static let cpuEpoch: Double = 1_000_000

    static func load(_ name: String) throws -> JSONValue {
        try JSONDecoder().decode(JSONValue.self, from: Data(contentsOf: directory.appendingPathComponent(name)))
    }
}

/// 테스트용 최소 JSON 값
enum JSONValue: Decodable, Equatable, CustomStringConvertible {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let b = try? c.decode(Bool.self) { self = .bool(b) }
        else if let n = try? c.decode(Double.self) { self = .number(n) }
        else if let s = try? c.decode(String.self) { self = .string(s) }
        else if let a = try? c.decode([JSONValue].self) { self = .array(a) }
        else { self = .object(try c.decode([String: JSONValue].self)) }
    }

    subscript(key: String) -> JSONValue {
        if case .object(let o) = self { return o[key] ?? .null }
        return .null
    }

    subscript(index: Int) -> JSONValue {
        if case .array(let a) = self, a.indices.contains(index) { return a[index] }
        return .null
    }

    func has(_ key: String) -> Bool {
        if case .object(let o) = self { return o[key] != nil }
        return false
    }

    var isNull: Bool { self == .null }
    var string: String? { if case .string(let s) = self { s } else { nil } }
    var double: Double? { if case .number(let n) = self { n } else { nil } }
    var int: Int? { double.flatMap { $0 == $0.rounded() ? Int($0) : nil } }
    var bool: Bool? { if case .bool(let b) = self { b } else { nil } }
    var array: [JSONValue] { if case .array(let a) = self { a } else { [] } }
    var object: [String: JSONValue] { if case .object(let o) = self { o } else { [:] } }
    var strings: [String] { array.compactMap(\.string) }

    var description: String {
        switch self {
        case .null: "null"
        case .bool(let b): "\(b)"
        case .number(let n): n == n.rounded() && abs(n) < 1e15 ? "\(Int(n))" : "\(n)"
        case .string(let s): s.debugDescription
        case .array(let a): "[" + a.map(\.description).joined(separator: ", ") + "]"
        case .object(let o): "{" + o.sorted { $0.key < $1.key }.map { "\($0.key.debugDescription): \($0.value)" }.joined(separator: ", ") + "}"
        }
    }
}
