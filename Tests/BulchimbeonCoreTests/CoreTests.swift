import Foundation
import Testing
@testable import BulchimbeonCore

@Suite struct AgentCatalogTests {
    private func proc(_ path: String, _ args: [String]) -> ProcessSnapshot {
        ProcessSnapshot(executablePath: path, arguments: args)
    }

    // 실측 (2026-10, macOS 26): 네이티브 설치본은 실행 파일 이름이 버전 번호, argv[0] 만 "claude"
    @Test func nativeInstallerViaShellCommand() {
        let p = proc("/Users/me/.local/share/claude/versions/2.1.296", ["claude"])
        #expect(AgentCatalog.match(p)?.kind.id == "claude-code")
    }

    @Test func nativeInstallerLaunchedByFullPath() {
        // IDE 확장·스크립트가 전체 경로로 띄우면 argv[0] 도 버전 경로 → 설치 경로로 판별
        let path = "/Users/me/.local/share/claude/versions/2.1.296"
        let match = AgentCatalog.match(proc(path, [path, "--resume"]))
        #expect(match?.kind.id == "claude-code")
        #expect(match?.reason == .installPath)
    }

    @Test func homebrewAndIdeBinaries() {
        #expect(AgentCatalog.match(proc("/opt/homebrew/Caskroom/claude-code/2.1.0/claude", ["claude"]))?.reason == .executableName)
        #expect(AgentCatalog.match(proc("/Users/me/.vscode/extensions/anthropic.claude-code-2.1.0/resources/native-binary/claude",
                                        ["claude"]))?.kind.id == "claude-code")
        #expect(AgentCatalog.match(proc("/opt/homebrew/bin/codex", ["codex", "exec"]))?.kind.id == "codex")
        #expect(AgentCatalog.match(proc("/Users/me/.local/bin/cursor-agent", ["cursor-agent"]))?.kind.id == "cursor-agent")
    }

    @Test func npmGlobalInstallRunsBinLink() {
        // npm 전역 설치: shebang 으로 `node /opt/homebrew/bin/claude` — 스크립트 basename 으로 판별
        let match = AgentCatalog.match(proc("/opt/homebrew/Cellar/node/24.1.0/bin/node",
                                            ["node", "/opt/homebrew/bin/claude", "-c"]))
        #expect(match?.kind.id == "claude-code")
        #expect(match?.reason == .script)
    }

    @Test func npmPackagePathAndOtherRunners() {
        #expect(AgentCatalog.match(proc("/Users/me/.volta/tools/image/node/22.0.0/bin/node",
            ["node", "--max-old-space-size=4096", "/usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js"]))?.kind.id == "claude-code")
        #expect(AgentCatalog.match(proc("/Users/me/.bun/bin/bun",
            ["bun", "/Users/me/.bun/install/global/node_modules/@google/gemini-cli/dist/index.js"]))?.kind.id == "gemini-cli")
        #expect(AgentCatalog.match(proc("/usr/local/bin/node",
            ["node", "/usr/local/lib/node_modules/@openai/codex/bin/codex.js"]))?.kind.id == "codex")
    }

    @Test func hookScriptsAreNotAgents() {
        // 실측: Claude Code hook 은 node -e 로 .claude 경로를 참조한다
        #expect(AgentCatalog.match(proc("/opt/homebrew/bin/node",
            ["node", "-e", "require(require('path').join(process.env.CLAUDE_PLUGIN_ROOT,'hooks/x.js'))"])) == nil)
        #expect(AgentCatalog.match(proc("/bin/zsh", ["/bin/zsh", "-c", "source /Users/me/.claude/shell-snapshots/x.sh"])) == nil)
        // 실행 파일 경로에 .claude 가 있어도 설치 마커가 아니면 무시
        #expect(AgentCatalog.match(proc("/Users/me/.claude/plugins/foo/bin/helper", ["helper"])) == nil)
    }

    @Test func desktopAppIsNotAnAgent() {
        #expect(AgentCatalog.match(proc("/Applications/Claude.app/Contents/MacOS/Claude", ["/Applications/Claude.app/Contents/MacOS/Claude"])) == nil)
        #expect(AgentCatalog.match(proc("/Applications/Claude.app/Contents/Helpers/chrome-native-host",
                                        ["chrome-native-host", "chrome-extension://x/"])) == nil)
    }

    @Test func onlyRunnersNeedFullArguments() {
        #expect(AgentCatalog.needsFullArguments(executablePath: "/opt/homebrew/bin/node"))
        #expect(AgentCatalog.needsFullArguments(executablePath: "/Users/me/.bun/bin/bun"))
        #expect(AgentCatalog.needsFullArguments(executablePath: "/usr/bin/ssh") == false)
    }

    @Test func customAgentsAreValidated() {
        let kinds = AgentCatalog.customKinds(["my-agent", "my-agent", "../../etc", "a b", "", String(repeating: "x", count: 40), "goose"])
        #expect(kinds.map(\.displayName) == ["my-agent", "goose"])
        #expect(AgentCatalog.customKinds((0..<30).map { "agent\($0)" }).count == 16)
        let match = AgentCatalog.match(proc("/usr/local/bin/goose", ["goose"]), kinds: AgentCatalog.builtIn + kinds)
        #expect(match?.kind.displayName == "goose")
    }

    @Test func countAndSummary() {
        let processes = [
            proc("/Users/me/.local/share/claude/versions/2.1.296", ["claude"]),
            proc("/Users/me/.local/share/claude/versions/2.1.296", ["claude"]),
            proc("/opt/homebrew/bin/node", ["node", "-e", "require('.claude')"]),
            proc("/opt/homebrew/bin/codex", ["codex"]),
            proc("/bin/zsh", ["-zsh"]),
        ]
        let counts = AgentCatalog.count(processes)
        #expect(counts.values.reduce(0, +) == 3)
        #expect(AgentCatalog.summary(counts) == "Claude Code ×2, Codex")
        #expect(AgentCatalog.summary([:]) == "")
    }
}

@Suite struct ProcArgsTests {
    /// KERN_PROCARGS2 형식 버퍼 생성
    private func buffer(execPath: String, argv: [String], env: [String]) -> [UInt8] {
        var bytes = withUnsafeBytes(of: Int32(argv.count)) { Array($0) }
        bytes += Array(execPath.utf8) + [0, 0, 0, 0] // 경로 + 패딩
        for s in argv + env { bytes += Array(s.utf8) + [0] }
        return bytes
    }

    @Test func parsesArgvOnlyNeverEnvironment() {
        let b = buffer(execPath: "/opt/homebrew/bin/node", argv: ["node", "/opt/homebrew/bin/claude"],
                       env: ["ANTHROPIC_API_KEY=sk-secret"])
        let args = ProcArgs.parse(b, limit: 8)
        #expect(args == ["node", "/opt/homebrew/bin/claude"])
        #expect(!args.joined().contains("sk-secret"))
    }

    @Test func respectsLimit() {
        let b = buffer(execPath: "/bin/x", argv: ["x", "--token=secret"], env: [])
        #expect(ProcArgs.parse(b, limit: 1) == ["x"])
    }

    @Test func malformedBuffersAreSafe() {
        #expect(ProcArgs.parse([], limit: 8) == [])
        #expect(ProcArgs.parse([1, 0], limit: 8) == [])
        #expect(ProcArgs.parse([255, 255, 255, 127] + Array("/x".utf8), limit: 8) == []) // argc 거짓, 데이터 없음
        #expect(ProcArgs.parse(withUnsafeBytes(of: Int32(-3)) { Array($0) } + [0], limit: 8) == [])
    }
}

@Suite struct ProcessArgumentCacheTests {
    let identity = ProcessIdentity(pid: 42, startedAtSeconds: 100, startedAtMicroseconds: 1)
    let kinds = AgentCatalog.builtIn

    @Test func skipsRereadingWhenArgumentsDoNotChangeTheMatch() {
        var cache = ProcessArgumentCache()
        var reads = 0
        let first = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: kinds) { reads += 1; return ["vim"] }
        let second = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: kinds) { reads += 1; return ["vim"] }
        #expect(first == ["vim"])
        #expect(second.isEmpty)
        #expect(reads == 1)
        let match = AgentCatalog.match(ProcessSnapshot(executablePath: "/bin/claude", arguments: []), kinds: kinds)
        let cachedAgent = cache.arguments(for: identity, executablePath: "/bin/claude", kinds: kinds) { ["claude"] }
        #expect(AgentCatalog.match(ProcessSnapshot(executablePath: "/bin/claude", arguments: cachedAgent))?.kind == match?.kind)
    }

    @Test func keepsRereadingWhenArgumentsDecideTheMatch() {
        let cases: [(String, [String])] = [
            ("/bin/claude", ["claude", "-p", "--tools="]),
            ("/opt/homebrew/bin/node", ["node", "/opt/homebrew/bin/codex"]),
            ("/Users/u/.local/bin/2.1.296", ["claude"]),
        ]
        for (path, argv) in cases {
            var cache = ProcessArgumentCache()
            var reads = 0
            _ = cache.arguments(for: identity, executablePath: path, kinds: kinds) { reads += 1; return argv }
            let again = cache.arguments(for: identity, executablePath: path, kinds: kinds) { reads += 1; return argv }
            #expect(again == argv)
            #expect(reads == 2)
        }
    }

    @Test func rereadsAfterExecChangesTheExecutable() {
        var cache = ProcessArgumentCache()
        _ = cache.arguments(for: identity, executablePath: "/bin/zsh", kinds: kinds) { ["-zsh"] }
        let afterExec = cache.arguments(for: identity, executablePath: "/opt/homebrew/bin/node", kinds: kinds) {
            ["node", "/opt/homebrew/bin/codex"]
        }
        #expect(AgentCatalog.match(ProcessSnapshot(executablePath: "/opt/homebrew/bin/node", arguments: afterExec))?.kind.id == "codex")
    }

    @Test func forgetsOnExitKindChangeOrUnreadableArguments() {
        var cache = ProcessArgumentCache()
        var reads = 0
        _ = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: kinds) { reads += 1; return [] }
        _ = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: kinds) { reads += 1; return ["vim"] }
        let custom = kinds + AgentCatalog.customKinds(["vim"])
        _ = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: custom) { reads += 1; return ["vim"] }
        _ = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: custom) { reads += 1; return ["vim"] }
        cache.retain(only: [])
        let afterExit = cache.arguments(for: identity, executablePath: "/usr/bin/vim", kinds: custom) { reads += 1; return ["vim"] }
        #expect(afterExit == ["vim"])
        #expect(reads == 4)
    }
}

@Suite struct PolicyTests {
    @Test func batteryRetreat() {
        #expect(BatteryPolicy.shouldRetreat(.init(percent: 20, onBattery: true), floor: 20))
        #expect(BatteryPolicy.shouldRetreat(.init(percent: 21, onBattery: true), floor: 20) == false)
        #expect(BatteryPolicy.shouldRetreat(.init(percent: 5, onBattery: false), floor: 20) == false, "충전 중에는 철수하지 않음")
        #expect(BatteryPolicy.shouldRetreat(.init(percent: 5, onBattery: true), floor: nil) == false, "기준 끔")
        #expect(BatteryPolicy.shouldRetreat(nil, floor: 20) == false, "배터리 없는 데스크톱")
    }

    @Test func agentWatchTransitions() {
        let g: TimeInterval = 300
        #expect(AgentWatchPolicy.decide(busyNow: true, quietFor: 0, grace: g, onDuty: false, lowBattery: false) == .engage)
        #expect(AgentWatchPolicy.decide(busyNow: true, quietFor: 0, grace: g, onDuty: false, lowBattery: true) == .none, "배터리 부족이면 투입 안 함")
        #expect(AgentWatchPolicy.decide(busyNow: true, quietFor: 0, grace: g, onDuty: true, lowBattery: false) == .none)
        #expect(AgentWatchPolicy.decide(busyNow: false, quietFor: 10, grace: g, onDuty: true, lowBattery: false) == .none, "caffeinate 재실행 사이 공백")
        #expect(AgentWatchPolicy.decide(busyNow: false, quietFor: 300, grace: g, onDuty: true, lowBattery: false) == .handover)
        #expect(AgentWatchPolicy.decide(busyNow: false, quietFor: 999, grace: g, onDuty: false, lowBattery: false) == .none, "조용한데 근무 중도 아님")
    }

    @Test func quietTimeStartsAtWatchStart() {
        let start = Date(timeIntervalSince1970: 1000)
        // 시간 근무 → 에이전트 근무 전환 직후: 일하는 걸 본 적 없어도 시작 시각부터 센다 (실측 버그 재현)
        #expect(AgentWatchPolicy.quietFor(now: start.addingTimeInterval(5), lastBusyAt: nil, watchStartedAt: start) == 5)
        // 근무 시작 전에 본 작업 기록은 무시
        #expect(AgentWatchPolicy.quietFor(now: start.addingTimeInterval(5), lastBusyAt: start.addingTimeInterval(-100), watchStartedAt: start) == 5)
        #expect(AgentWatchPolicy.quietFor(now: start.addingTimeInterval(400), lastBusyAt: start.addingTimeInterval(100), watchStartedAt: start) == 300)
    }

    @Test func dutyResume() {
        let now = Date(timeIntervalSince1970: 1_000_000)
        let t = now.timeIntervalSince1970
        #expect(DutyResume.decide(stored: nil, until: nil, now: now) == .none)
        #expect(DutyResume.decide(stored: "agentWatch", until: nil, now: now) == .agentWatch)
        #expect(DutyResume.decide(stored: "timed", until: nil, now: now) == .timed(until: nil), "무기한")
        #expect(DutyResume.decide(stored: "timed", until: t + 600, now: now) == .timed(until: now.addingTimeInterval(600)), "남은 10분")
        #expect(DutyResume.decide(stored: "timed", until: t - 1, now: now) == .expired, "꺼져 있는 동안 만료")
        #expect(DutyResume.decide(stored: "timed", until: t + 7 * 86400, now: now) == .none, "조작된 값")
        #expect(DutyResume.decide(stored: "timed", until: .infinity, now: now) == .none)
        #expect(DutyResume.decide(stored: "rm -rf /", until: nil, now: now) == .none)
    }

    @Test func timedMenuCheck() {
        let until = Date(timeIntervalSince1970: 1_000_000)
        #expect(TimedMenu.isChecked(nil, order: .timed(until: nil), picked: nil), "무기한")
        #expect(!TimedMenu.isChecked(3600, order: .timed(until: nil), picked: 3600), "무기한 중엔 다른 항목 해제")
        #expect(TimedMenu.isChecked(3600, order: .timed(until: until), picked: 3600))
        #expect(!TimedMenu.isChecked(1800, order: .timed(until: until), picked: 3600))
        #expect(!TimedMenu.isChecked(nil, order: .timed(until: until), picked: 3600))
        #expect(!TimedMenu.isChecked(3600, order: .timed(until: until), picked: nil), "재시작으로 이어진 근무")
        #expect(!TimedMenu.isChecked(nil, order: .off, picked: nil), "근무 종료")
        #expect(!TimedMenu.isChecked(nil, order: .agentWatch, picked: nil), "에이전트 근무로 전환")
        #expect(!TimedMenu.isChecked(3600, order: .agentWatch, picked: 3600))
    }

    @Test func pmsetParsing() {
        #expect(!PmsetParser.sleepDisabled(" SleepDisabled\t\t0\n sleep 1"))
        #expect(PmsetParser.sleepDisabled("System-wide power settings:\n SleepDisabled\t\t1\n"))
        #expect(!PmsetParser.sleepDisabled(""))
    }

    @Test func clockFormat() {
        #expect(DutyClock.format(59) == "0:59")
        #expect(DutyClock.format(3903) == "1:05:03")
        #expect(DutyClock.format(-5) == "0:00")
    }
}

@Suite struct DutyLogTests {
    @Test func newestFirstAndCapped() {
        let log = DutyLog(fileURL: nil, capacity: 2)
        log.write("a"); log.write("b"); log.write("c")
        #expect(log.recent.map(\.text) == ["c", "b"])
    }

    @Test func appendsToFile() throws {
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString).appendingPathComponent("log.txt")
        let log = DutyLog(fileURL: url)
        log.write("근무 투입")
        log.write("근무 교대")
        let text = try String(contentsOf: url, encoding: .utf8)
        #expect(text.contains("근무 투입") && text.contains("근무 교대"))
        #expect(text.split(separator: "\n").count == 2)
    }

    private func tempLogURL() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString).appendingPathComponent("log.txt")
    }

    @Test func fileIsOwnerOnly() throws {
        let url = tempLogURL()
        DutyLog(fileURL: url).write("근무 투입")
        let perms = try FileManager.default.attributesOfItem(atPath: url.path)[.posixPermissions] as? Int
        #expect(perms == 0o600)
        let dirPerms = try FileManager.default
            .attributesOfItem(atPath: url.deletingLastPathComponent().path)[.posixPermissions] as? Int
        #expect(dirPerms == 0o700)
    }

    @Test func refusesToFollowSymlink() throws {
        let url = tempLogURL()
        let dir = url.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let victim = dir.appendingPathComponent("victim.txt")
        try "중요한 파일".write(to: victim, atomically: true, encoding: .utf8)
        try FileManager.default.createSymbolicLink(at: url, withDestinationURL: victim)

        DutyLog(fileURL: url).write("덮어쓰기 시도")
        #expect(try String(contentsOf: victim, encoding: .utf8) == "중요한 파일")
    }

    @Test func rotatesWhenTooLarge() throws {
        let url = tempLogURL()
        let log = DutyLog(fileURL: url, maxFileBytes: 64)
        for i in 0..<10 { log.write("근무 기록 \(i)") }
        let size = try FileManager.default.attributesOfItem(atPath: url.path)[.size] as? Int ?? 0
        #expect(size < 64 * 2)
        #expect(FileManager.default.fileExists(atPath: url.appendingPathExtension("1").path))
    }

    @Test func newlinesCannotForgeEntries() throws {
        let url = tempLogURL()
        DutyLog(fileURL: url).write("정상\n2026-01-01T00:00:00Z  위조된 항목")
        let text = try String(contentsOf: url, encoding: .utf8)
        #expect(text.split(separator: "\n").count == 1)
    }
}

@Suite struct SpriteTests {
    @Test(arguments: Sprites.Pose.allCases)
    func framesAre16x16WithKnownColors(pose: Sprites.Pose) {
        for frame in Sprites.frames(pose) {
            #expect(frame.height == 16)
            for row in frame.rows {
                #expect(row.count == 16, "행 길이 불일치: \(row)")
                for ch in row where ch != "." {
                    #if canImport(CoreGraphics)
                    #expect(Palette.cgColor(ch) != nil, "팔레트에 없는 문자 '\(ch)'")
                    #else
                    #expect(Palette.hex[ch] != nil, "팔레트에 없는 문자 '\(ch)'")
                    #endif
                }
            }
        }
    }

    @Test func appIconSceneIsSquare() {
        let scene = Sprites.appIconScene
        #expect(scene.height == 24)
        #expect(scene.rows.allSatisfy { $0.count == 24 })
    }

    @Test func mirroredFlipsRows() {
        let sprite = PixelSprite(["KY.", ".BU"])
        #expect(sprite.mirrored().rows == [".YK", "UB."])
        #expect(sprite.mirrored().mirrored() == sprite)
    }

    @Test func placingOverlaysOnlyOpaquePixels() {
        let base = PixelSprite(["...", "..."])
        let result = base.placing(PixelSprite(["K.", "YK"]), x: 1, y: 0)
        #expect(result.rows == [".K.", ".YK"])
    }
}

@Suite struct L10nTests {
    private func hasHangul(_ s: String) -> Bool {
        s.unicodeScalars.contains { (0xAC00...0xD7A3).contains($0.value) }
    }

    private func allLines(_ lang: L10n.Language) -> [String] {
        var lines = Lines.onDuty(lang) + Lines.standby(lang) + Lines.off(lang)
        let notices = [Lines.reportForDuty(lang), Lines.agentArrived("Claude Code", lang), Lines.handover("Claude Code", lang),
                       Lines.timeUp(lang), Lines.batteryRetreat(15, lang), Lines.lidFallback(lang),
                       Lines.lidResumeFallback(lang), Lines.lidLeftOn(lang), Lines.heatAdvisory(lang),
                       Lines.heatRest(lang), Lines.heatResume(lidDowngraded: false, lang),
                       Lines.heatResume(lidDowngraded: true, lang), Lines.postAbandoned(lang)]
        lines += notices.flatMap { [$0.title, $0.body] }
        lines += [Lines.Speech.reporting(lang), Lines.Speech.arrived(lang), Lines.Speech.handover(lang),
                  Lines.Speech.timeUp(lang), Lines.Speech.resumed(lang), Lines.Speech.heat(lang),
                  Lines.Speech.rest(lang), Lines.Speech.backFromRest(lang), Lines.Speech.notAWOL(lang),
                  Lines.Speech.handoverReport(lang), Lines.Speech.waiting(lang)]
        return lines
    }

    @Test func chineseLanguageAndPreference() {
        for tag in ["zh", "zh-CN", "zh-Hans", "zh-Hans-CN", "zh_TW.UTF-8"] {
            let lang = L10n.detect([tag, "en"])
            #expect(String(describing: lang) == "zh")
            #expect(Lines.reportForDuty(lang).title == "夜间值班报到！🪖")
        }
        #expect(L10n.Preference(appDomainLanguages: ["zh-Hans"]).appleLanguages == ["zh-Hans"])
        #expect(L10n.Preference(appDomainLanguages: ["zh-CN"]).title == "简体中文")
    }

    @Test func chineseLinesAndThemes() {
        let lines = allLines(.zh)
        #expect(lines.allSatisfy { line in
            !hasHangul(line) && line.unicodeScalars.contains { (0x4E00...0x9FFF).contains($0.value) }
        })
        #expect(Lines.onDuty(.zh).count == Lines.onDuty(.en).count)
        #expect(Lines.standby(.zh).count == Lines.standby(.en).count)
        #expect(Lines.off(.zh).count == Lines.off(.en).count)
        #expect(UniformTheme.all.map { $0.name(.zh) } == ["标准", "海蓝", "雪地", "沙漠"])
        #expect(Lines.agentArrived("测试代理 ×2", .zh).body == "测试代理 ×2 已开工，我来守护！")
        #expect(Lines.batteryRetreat(17, .zh).body.contains("17%"))
        #expect(L10n.t("한국어", "English", "中文", .zh) == "中文")
    }

    @Test func detectsLanguage() {
        #expect(L10n.detect(["ko-KR", "en-US"]) == .ko)
        #expect(L10n.detect(["en-US", "ko-KR"]) == .en)
        #expect(L10n.detect(["ja-JP"]) == .en)
        #expect(L10n.detect([]) == .en)
    }

    @Test func englishHasNoKorean() {
        let lines = allLines(.en)
        #expect(lines.allSatisfy { !$0.isEmpty })
        #expect(lines.filter(hasHangul).isEmpty, "영어 문구에 한글: \(lines.filter(hasHangul))")
    }

    @Test func koreanIsKorean() {
        let lines = allLines(.ko)
        #expect(lines.filter { !hasHangul($0) }.isEmpty, "한국어 문구에 한글 없음: \(lines.filter { !hasHangul($0) })")
    }

    @Test func languagePreferenceRoundTrip() {
        #expect(L10n.Preference(appDomainLanguages: nil) == .system)
        #expect(L10n.Preference(appDomainLanguages: ["ko"]) == .ko)
        #expect(L10n.Preference(appDomainLanguages: ["ko-KR", "en"]) == .ko)
        #expect(L10n.Preference(appDomainLanguages: ["en-US"]) == .en)
        #expect(L10n.Preference(appDomainLanguages: ["ja"]) == .system, "지원하지 않는 언어는 시스템 따름")
        for p in L10n.Preference.allCases {
            #expect(L10n.Preference(appDomainLanguages: p.appleLanguages) == p)
        }
    }

    @Test func bothLanguagesHaveSameShape() {
        #expect(Lines.onDuty(.ko).count == Lines.onDuty(.en).count)
        #expect(Lines.standby(.ko).count == Lines.standby(.en).count)
        #expect(Lines.off(.ko).count == Lines.off(.en).count)
    }
}

@Suite struct HeatTests {
    private let now = Date(timeIntervalSince1970: 1_000_000)

    @Test func mapsThermalState() {
        #if canImport(Darwin)
        #expect(HeatLevel(ProcessInfo.ThermalState.nominal) == .normal)
        #expect(HeatLevel(ProcessInfo.ThermalState.fair) == .caution)
        #expect(HeatLevel(ProcessInfo.ThermalState.serious) == .advisory)
        #expect(HeatLevel(ProcessInfo.ThermalState.critical) == .alert)
        #endif
        #expect(HeatLevel(name: "alert") == .alert)
        #expect(HeatLevel(name: "; rm -rf /") == nil)
    }

    @Test func advisoryOnceThenRest() {
        #expect(HeatPolicy.decide(level: .advisory, onDuty: true, resting: false, advised: false, coolSince: nil, now: now) == .advisory)
        #expect(HeatPolicy.decide(level: .advisory, onDuty: true, resting: false, advised: true, coolSince: nil, now: now) == .none, "주의보는 한 번만")
        #expect(HeatPolicy.decide(level: .alert, onDuty: true, resting: false, advised: true, coolSince: nil, now: now) == .rest)
        #expect(HeatPolicy.decide(level: .alert, onDuty: false, resting: false, advised: false, coolSince: nil, now: now) == .none, "근무 중이 아니면 쉴 것도 없음")
        #expect(HeatPolicy.decide(level: .caution, onDuty: true, resting: false, advised: false, coolSince: nil, now: now) == .none)
    }

    @Test func resumeOnlyAfterCoolDown() {
        let justCooled = now.addingTimeInterval(-30)
        let longCooled = now.addingTimeInterval(-HeatPolicy.coolDown)
        #expect(HeatPolicy.decide(level: .normal, onDuty: false, resting: true, advised: true, coolSince: justCooled, now: now) == .none, "식은 지 30초는 이름")
        #expect(HeatPolicy.decide(level: .normal, onDuty: false, resting: true, advised: true, coolSince: longCooled, now: now) == .resume)
        #expect(HeatPolicy.decide(level: .advisory, onDuty: false, resting: true, advised: true, coolSince: longCooled, now: now) == .none, "아직 덥다")
    }
}

@Suite struct LidWatchdogTests {
    @Test func pkillPatternDoesNotMatchItself() throws {
        // disable 명령을 실행하는 셸 자신의 명령줄이 패턴에 걸리면 pkill 이 자기 부모를 죽인다
        let pattern = LidCommands.watchdogPattern(pid: 4321)
        let regex = try Regex(pattern)
        #expect(LidCommands.disable(pid: 4321).contains(regex) == false)
        #expect(LidCommands.enable(pid: 4321, stopSignal: "/tmp/lid-stop-4321").contains(regex), "감시병 명령줄은 매칭돼야 한다")
    }

    #if os(macOS) // 감시병은 macOS 의 pmset 덮개 닫힘용이다
    /// root 없이 실제로 돌려본다: 가짜 pmset + 가짜 앱 프로세스(sleep). 앱이 죽으면 감시병이 원복해야 한다.
    @Test func watchdogRestoresWhenAppDies() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let log = dir.appendingPathComponent("pmset.log")
        let fakePmset = dir.appendingPathComponent("pmset")
        let stopSignal = dir.appendingPathComponent("lid-stop")
        try "#!/bin/sh\necho \"$@\" >> '\(log.path)'\n".write(to: fakePmset, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fakePmset.path)

        let app = Process()
        app.executableURL = URL(fileURLWithPath: "/bin/sleep")
        app.arguments = ["60"]
        try app.run()

        let shell = Process()
        shell.executableURL = URL(fileURLWithPath: "/bin/sh")
        shell.arguments = ["-c", LidCommands.enable(pid: app.processIdentifier, stopSignal: stopSignal.path, pmset: fakePmset.path)]
        let started = Date()
        try shell.run()
        shell.waitUntilExit()
        #expect(shell.terminationStatus == 0)
        // 실측 버그: 명령이 감시병을 기다리느라 끝나지 않아 osascript 가 영원히 대기했다
        #expect(Date().timeIntervalSince(started) < 2, "켜기 명령은 감시병을 띄우고 바로 끝나야 한다")

        try await Task.sleep(for: .seconds(1))
        #expect(try String(contentsOf: log, encoding: .utf8) == "-a disablesleep 1\n", "켜기만 실행됨")
        // 감시병은 자기 프로세스 그룹의 리더여야 한다 (부모 그룹이 정리돼도 살아남음)
        let ps = Process()
        ps.executableURL = URL(fileURLWithPath: "/usr/bin/pgrep")
        ps.arguments = ["-f", LidCommands.watchdogPattern(pid: app.processIdentifier)]
        let out = Pipe(); ps.standardOutput = out
        try ps.run(); ps.waitUntilExit()
        let found = String(decoding: out.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            .split(separator: "\n").compactMap { Int32($0) }
        #expect(!found.isEmpty, "감시병 프로세스가 떠 있어야 한다")
        // 핵심 성질: 띄운 쪽(이 테스트)의 프로세스 그룹에서 빠져나왔는가. 부모 그룹이 정리돼도 살아남는다.
        // (감시병이 kill/sleep 을 실행하려 잠깐 fork 한 자식도 같은 명령줄로 잡히므로 "리더인가"가 아니라 이걸 본다)
        for pid in found {
            #expect(getpgid(pid) != getpgrp(), "감시병이 원래 프로세스 그룹에 남아 있음: pid \(pid)")
        }

        app.terminate() // 앱 강제 종료
        app.waitUntilExit()
        var restored = false
        for _ in 0..<16 { // 감시 주기 5초 + 여유
            try await Task.sleep(for: .milliseconds(500))
            if (try? String(contentsOf: log, encoding: .utf8))?.contains("disablesleep 0") == true { restored = true; break }
        }
        #expect(restored, "앱이 죽은 뒤 감시병이 disablesleep 0 을 실행해야 한다")
    }

    /// 앱은 파일 내용 없이 플래그만 만든다. 감시병은 앱이 살아 있어도 다음 5초 점검에 원복한다.
    @Test func watchdogRestoresWhenStopSignalExists() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let log = dir.appendingPathComponent("pmset.log")
        let fakePmset = dir.appendingPathComponent("pmset")
        let stopSignal = dir.appendingPathComponent("lid-stop")
        try "#!/bin/sh\necho \"$@\" >> '\(log.path)'\n".write(to: fakePmset, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fakePmset.path)

        let app = Process()
        app.executableURL = URL(fileURLWithPath: "/bin/sleep")
        app.arguments = ["60"]
        try app.run()
        defer { if app.isRunning { app.terminate() } }

        let shell = Process()
        shell.executableURL = URL(fileURLWithPath: "/bin/sh")
        shell.arguments = ["-c", LidCommands.enable(pid: app.processIdentifier, stopSignal: stopSignal.path, pmset: fakePmset.path)]
        try shell.run()
        shell.waitUntilExit()
        try await Task.sleep(for: .seconds(1))
        #expect(FileManager.default.createFile(atPath: stopSignal.path, contents: nil))

        var restored = false
        for _ in 0..<16 {
            try await Task.sleep(for: .milliseconds(500))
            if (try? String(contentsOf: log, encoding: .utf8))?.contains("disablesleep 0") == true { restored = true; break }
        }
        #expect(restored, "정지 플래그 뒤 감시병이 관리자 암호 없이 disablesleep 0 을 실행해야 한다")
    }

    /// 경로에 작은따옴표·공백이 있어도 인용이 깨지지 않고, 플래그가 심볼릭 링크여도 대상을 따라가지 않고 원복한다.
    @Test func watchdogQuotesStopSignalPathAndSeesSymlinkFlag() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        let flagDir = dir.appendingPathComponent("it's a dir")
        try FileManager.default.createDirectory(at: flagDir, withIntermediateDirectories: true)
        let log = dir.appendingPathComponent("pmset.log")
        let fakePmset = dir.appendingPathComponent("pmset")
        let stopSignal = flagDir.appendingPathComponent("lid-stop")
        try "#!/bin/sh\necho \"$@\" >> '\(log.path)'\n".write(to: fakePmset, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fakePmset.path)

        let app = Process()
        app.executableURL = URL(fileURLWithPath: "/bin/sleep")
        app.arguments = ["60"]
        try app.run()
        defer { if app.isRunning { app.terminate() } }

        let shell = Process()
        shell.executableURL = URL(fileURLWithPath: "/bin/sh")
        shell.arguments = ["-c", LidCommands.enable(pid: app.processIdentifier, stopSignal: stopSignal.path, pmset: fakePmset.path)]
        try shell.run()
        shell.waitUntilExit()
        #expect(shell.terminationStatus == 0)
        try await Task.sleep(for: .seconds(1))
        let started = (try? String(contentsOf: log, encoding: .utf8)) ?? ""
        #expect(started.contains("disablesleep 1"))
        #expect(!started.contains("disablesleep 0"), "플래그가 없으면 앱이 살아 있는 동안 원복하지 않는다")

        try FileManager.default.createSymbolicLink(atPath: stopSignal.path, withDestinationPath: dir.appendingPathComponent("missing").path)
        var restored = false
        for _ in 0..<16 {
            try await Task.sleep(for: .milliseconds(500))
            if (try? String(contentsOf: log, encoding: .utf8))?.contains("disablesleep 0") == true { restored = true; break }
        }
        #expect(restored, "인용된 경로의 플래그(깨진 심볼릭 링크 포함)를 감시병이 봐야 한다")
    }

    @Test func watchdogReplacementDoesNotStackForSameApp() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let log = dir.appendingPathComponent("pmset.log")
        let fakePmset = dir.appendingPathComponent("pmset")
        let stopSignal = dir.appendingPathComponent("lid-stop")
        try "#!/bin/sh\necho \"$@\" >> '\(log.path)'\n".write(to: fakePmset, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fakePmset.path)

        let app = Process()
        app.executableURL = URL(fileURLWithPath: "/bin/sleep")
        app.arguments = ["60"]
        try app.run()
        defer { if app.isRunning { app.terminate() } }

        let command = LidCommands.enable(pid: app.processIdentifier, stopSignal: stopSignal.path, pmset: fakePmset.path)
        for _ in 0..<2 {
            let shell = Process()
            shell.executableURL = URL(fileURLWithPath: "/bin/sh")
            shell.arguments = ["-c", command]
            try shell.run()
            shell.waitUntilExit()
            #expect(shell.terminationStatus == 0)
        }
        try await Task.sleep(for: .seconds(1))

        let pgrep = Process()
        pgrep.executableURL = URL(fileURLWithPath: "/usr/bin/pgrep")
        pgrep.arguments = ["-f", LidCommands.watchdogPattern(pid: app.processIdentifier)]
        let out = Pipe(); pgrep.standardOutput = out
        try pgrep.run(); pgrep.waitUntilExit()
        let pids = String(decoding: out.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            .split(separator: "\n").compactMap { Int32($0) }
        #expect(pids.count == 1, "같은 앱 PID에는 감시병 하나만 남아야 한다")

        #expect(FileManager.default.createFile(atPath: stopSignal.path, contents: nil))
        var restored = false
        for _ in 0..<16 {
            try await Task.sleep(for: .milliseconds(500))
            if (try? String(contentsOf: log, encoding: .utf8))?.contains("disablesleep 0") == true { restored = true; break }
        }
        #expect(restored, "교체한 감시병도 정지 신호로 끝나야 한다")
    }
    #endif
}

@Suite struct HandoverReportTests {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)

    private func entry(_ minutes: Double, _ kind: DutyLogEntry.Kind, _ text: String = "x") -> DutyLogEntry {
        DutyLogEntry(date: t0.addingTimeInterval(minutes * 60), text: text, kind: kind)
    }

    @Test func summarizesNightWatch() throws {
        let entries = [entry(5, .onDuty), entry(190, .handover, "근무 교대 — Claude Code"), entry(60, .heat), entry(400, .other)]
        let report = try #require(HandoverReport.make(entries: entries, awayFrom: t0, until: t0.addingTimeInterval(8 * 3600), .ko))
        #expect(report.title.contains("인수인계"))
        #expect(report.body.contains("근무 투입 1회") && report.body.contains("근무 교대 1회") && report.body.contains("폭염 1회"))
        #expect(report.body.contains("근무 교대 — Claude Code"), "마지막 의미 있는 사건")
        let en = try #require(HandoverReport.make(entries: entries, awayFrom: t0, until: t0.addingTimeInterval(8 * 3600), .en))
        #expect(en.title == "Night watch handover 📋")
    }

    @Test func noReportForShortBreaksOrQuietNights() {
        #expect(HandoverReport.make(entries: [entry(1, .onDuty)], awayFrom: t0, until: t0.addingTimeInterval(5 * 60), .ko) == nil, "커피 타임")
        #expect(HandoverReport.make(entries: [entry(30, .other)], awayFrom: t0, until: t0.addingTimeInterval(3600), .ko) == nil, "보고할 일 없음")
        #expect(HandoverReport.make(entries: [entry(-10, .onDuty)], awayFrom: t0, until: t0.addingTimeInterval(3600), .ko) == nil, "자리 비우기 전 일")
    }
}

@Suite struct AgentActivityTests {
    private let claudePath = "/Users/me/.local/share/claude/versions/2.1.296"

    private func proc(_ pid: Int32, _ ppid: Int32, _ path: String, _ args: [String]) -> ProcessSnapshot {
        ProcessSnapshot(pid: pid, ppid: ppid, executablePath: path, arguments: args)
    }

    // 실측: claude-mem 플러그인이 bun 워커로 상시 띄워 두는 헤드리스 세션 (도구 없음)
    @Test func pluginHelperSessionIsNotAnAgent() {
        let helper = proc(27088, 22855, claudePath, ["/Users/me/.local/bin/claude", "--output-format", "stream-json",
            "--input-format", "stream-json", "--tools=", "--no-session-persistence", "--permission-mode=dontAsk"])
        #expect(AgentCatalog.match(helper) == nil)
        // 도구를 쓰는 일반 헤드리스 실행은 에이전트
        let headless = proc(30000, 1, claudePath, ["claude", "-p", "fix the tests", "--output-format", "stream-json"])
        #expect(AgentCatalog.match(headless)?.kind.id == "claude-code")
    }

    @Test func candidatesNeedMoreArguments() {
        #expect(AgentCatalog.isCandidate(executablePath: claudePath, argv0: "claude"))
        #expect(AgentCatalog.isCandidate(executablePath: "/opt/homebrew/bin/codex", argv0: "codex"))
        #expect(AgentCatalog.isCandidate(executablePath: "/bin/zsh", argv0: "-zsh") == false)
    }

    // 실측: 작업 중인 세션만 caffeinate -i -t 300 을 자식으로 띄우고, 입력 대기 세션은 CPU ~0
    @Test func workingVsIdleSessions() {
        let processes = [
            proc(11482, 7109, claudePath, ["claude", "--dangerously-skip-permissions"]),  // 작업 중
            proc(19418, 11482, "/usr/bin/caffeinate", ["caffeinate"]),
            proc(6885, 40271, claudePath, ["claude"]),                                     // 입력 대기
            proc(6900, 6885, "/opt/homebrew/bin/node", ["node", "/x/mcp-server.js"]),      // 대기 중인 MCP 서버
            proc(4813, 4518, claudePath, ["claude", "-c"]),                                // 빌드 돌리는 중
            proc(4900, 4813, "/bin/zsh", ["/bin/zsh"]),
            proc(4901, 4900, "/usr/bin/swift-build", ["swift-build"]),
        ]
        let cpu: [Int32: Double] = [11482: 10.6, 6885: 0.0, 6900: 0.3, 4813: 0.1, 4901: 85]
        let status = AgentActivity.evaluate(processes, cpu: cpu)
        let claude = status.first { $0.key.id == "claude-code" }?.value
        #expect(claude == AgentActivity.Status(present: 3, busy: 2), "caffeinate 신호 1 + 자손 빌드 CPU 1, 대기 1")
    }

    @Test func idleWithoutCPUDataIsNotBusy() {
        // 첫 스캔엔 CPU 차분이 없다 → caffeinate 신호만으로 판단
        let processes = [proc(1, 0, claudePath, ["claude"]), proc(2, 0, claudePath, ["claude"]),
                         proc(3, 2, "/usr/bin/caffeinate", ["caffeinate"])]
        #expect(AgentActivity.evaluate(processes, cpu: [:]).values.first == AgentActivity.Status(present: 2, busy: 1))
    }

    @Test func cyclicParentLinksDoNotHang() {
        let processes = [proc(10, 11, claudePath, ["claude"]), proc(11, 10, "/bin/zsh", ["zsh"])]
        #expect(AgentActivity.evaluate(processes, cpu: [11: 5]).values.first?.busy == 1)
    }
}

@Suite struct CPUHistoryTests {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)
    private let second: UInt64 = 1_000_000_000

    @Test func periodicBlipsAverageOut() {
        // 15초마다 0.2초씩 쓰는 감시 스크립트 (실측 상황 재현): 5초 순간값은 4% 지만 60초 평균은 ~1.3%
        var history = CPUHistory(window: 60)
        var used: UInt64 = 0
        var last: [Int32: Double] = [:]
        for tick in 0...24 { // 5초 간격 2분
            if tick % 3 == 0 { used += second / 5 }
            last = history.add([42: used], at: t0.addingTimeInterval(Double(tick) * 5))
        }
        #expect((last[42] ?? 0) < AgentActivity.cpuThreshold)
    }

    @Test func sustainedWorkStaysBusy() {
        var history = CPUHistory(window: 60)
        var last: [Int32: Double] = [:]
        for tick in 0...24 { // 계속 10% 사용
            last = history.add([7: UInt64(tick) * second / 2], at: t0.addingTimeInterval(Double(tick) * 5))
        }
        #expect(abs((last[7] ?? 0) - 10) < 0.5)
    }

    @Test func firstSampleHasNoData() {
        var history = CPUHistory()
        #expect(history.add([1: 100], at: t0).isEmpty)
    }
}

@Suite struct PowerHoldersTests {
    private func a(_ pid: Int32, _ name: String, _ type: String, _ label: String = "") -> PowerHolders.Assertion {
        .init(pid: pid, processName: name, type: type, name: label)
    }

    // 실측: "에이전트가 끝났는데 화면이 안 꺼진다"의 실제 원인 중 하나가 Vivaldi 영상 재생이었다
    @Test func summarizesRealWorldSnapshot() {
        let snapshot = [
            a(351, "powerd", "PreventUserIdleSystemSleep", "Powerd - Prevent sleep while display is on"),
            a(999, "Bulchimbeon", "PreventUserIdleDisplaySleep", "Bulchimbeon on duty"),
            a(46300, "caffeinate", "PreventUserIdleSystemSleep", "caffeinate command-line tool"),
            a(402, "bluetoothd", "PreventUserIdleSystemSleep", "com.apple.BTStack"),
            a(45807, "Vivaldi", "NoDisplaySleepAssertion", "Video Wake Lock"),
            a(45807, "Vivaldi", "PreventUserIdleSystemSleep", "Video Wake Lock"),
            a(423, "coreaudiod", "PreventUserIdleSystemSleep", "com.apple.audio.context"),
        ]
        let s = PowerHolders.summarize(snapshot, ownPID: 999, .ko)
        #expect(s.display == ["Vivaldi (영상)"])
        #expect(s.system == ["caffeinate", "오디오 재생"], "자기 자신·powerd·bluetoothd 제외, 화면 쪽에 나온 앱은 중복 제외")
    }

    @Test func ownAppIsNeverAnotherHolder() {
        // 진단은 별도 프로세스라 PID 가 달라도, 설치본 불침번은 "다른 앱"이 아니다
        let s = PowerHolders.summarize([a(40601, "불침번", "PreventUserIdleDisplaySleep"),
                                        a(40602, "Bulchimbeon", "PreventUserIdleSystemSleep")], ownPID: 1, .ko)
        #expect(s.display.isEmpty && s.system.isEmpty)
    }

    @Test func emptyWhenOnlyNoise() {
        let s = PowerHolders.summarize([a(351, "powerd", "PreventUserIdleSystemSleep")], ownPID: 1, .en)
        #expect(s.display.isEmpty && s.system.isEmpty)
    }
}
