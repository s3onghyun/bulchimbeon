import Foundation
import Testing
@testable import BulchimbeonCore

/// 업데이트 확인: 공유 픽스처 `testdata/engine/update.json` (Go 포트도 같은 파일을 돌린다) 와
/// macOS 설치 쪽 판단. 네트워크는 쓰지 않는다 — 응답은 픽스처의 JSON 이다.
@Suite struct UpdateFixtureTests {
    private static func raw() throws -> [String: Any] {
        let data = try Data(contentsOf: Fixture.directory.appendingPathComponent("update.json"))
        return try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }

    private static func date(_ v: JSONValue) -> Date? { v.double.map { Date(timeIntervalSince1970: $0) } }

    private static func release(_ v: JSONValue) -> ReleaseInfo? {
        guard let version = v["version"].string.flatMap(AppVersion.init) else { return nil }
        return ReleaseInfo(version: version, criticalBelow: v["criticalBelow"].string.flatMap(AppVersion.init))
    }

    @Test func constants() throws {
        let f = try Fixture.load("update.json")
        #expect(UpdateCheck.interval == f["checkIntervalSeconds"].double)
        #expect(UpdateCheck.maxResponseBytes == f["maxResponseBytes"].int)
        #expect(UpdateCheck.latestReleaseAPI.absoluteString == f["latestReleaseAPI"].string)
    }

    @Test func versions() throws {
        let f = try Fixture.load("update.json")
        for c in f["versions"].array {
            #expect(AppVersion(c["input"].string ?? "")?.description == c["expect"].string, "version: \(c["name"])")
        }
        for c in f["compare"].array {
            let a = try #require(AppVersion(c["a"].string ?? ""), "\(c)")
            let b = try #require(AppVersion(c["b"].string ?? ""), "\(c)")
            let want = c["expect"].int
            #expect((a < b ? -1 : a == b ? 0 : 1) == want, "compare: \(c)")
            #expect((b < a ? -1 : a == b ? 0 : 1) == want.map { -$0 }, "compare reversed: \(c)")
        }
    }

    @Test func releases() throws {
        let f = try Fixture.load("update.json")
        let raw = try #require(try Self.raw()["releases"] as? [[String: Any]])
        for (c, r) in zip(f["releases"].array, raw) {
            let data = if let text = r["text"] as? String { Data(text.utf8) }
                else { try JSONSerialization.data(withJSONObject: try #require(r["json"]), options: [.fragmentsAllowed]) }
            #expect(ReleaseInfo.parse(data) == Self.release(c["expect"]), "release: \(c["name"])")
        }
        // 크기 제한: 지나치게 큰 응답은 읽지 않는다
        let padded = "{\"tag_name\": \"v9.0.0\", \"body\": \"" + String(repeating: "x", count: UpdateCheck.maxResponseBytes) + "\"}"
        #expect(ReleaseInfo.parse(Data(padded.utf8)) == nil)
    }

    @Test func due() throws {
        let f = try Fixture.load("update.json")
        for c in f["due"].array {
            let due = UpdateCheck.isDue(enabled: c["enabled"].bool ?? false, current: AppVersion(c["current"].string ?? ""),
                                        now: try #require(Self.date(c["now"])), lastCheck: Self.date(c["last"]))
            #expect(due == c["expect"].bool, "due: \(c["name"])")
        }
    }

    @Test func offers() throws {
        let f = try Fixture.load("update.json")
        for c in f["offers"].array {
            let offer = UpdateOffer.decide(current: AppVersion(c["current"].string ?? ""), latest: Self.release(c["latest"]),
                                           enabled: c["enabled"].bool ?? false)
            let want = c["expect"]
            #expect(offer?.version.description == want["version"].string, "offer: \(c["name"])")
            #expect(offer?.critical == want["critical"].bool, "offer critical: \(c["name"])")
        }
    }

    @Test func texts() throws {
        let f = try Fixture.load("update.json")
        for c in f["texts"].array {
            let offer = UpdateOffer(version: try #require(AppVersion(c["version"].string ?? "")), critical: c["critical"].bool ?? false)
            for lang in L10n.Language.allCases {
                #expect(offer.menuTitle(lang) == c["menuTitle"][lang.rawValue].string, "menuTitle \(lang): \(c)")
                #expect(offer.summary(lang) == c["summary"][lang.rawValue].string, "summary \(lang): \(c)")
            }
        }
        for lang in L10n.Language.allCases {
            #expect(UpdateOffer.releaseNotesTitle(lang) == f["releaseNotesTitle"][lang.rawValue].string)
        }
    }

    @Test func urlsAndHeaders() throws {
        let f = try Fixture.load("update.json")
        for c in f["urls"].array {
            let v = try #require(AppVersion(c["version"].string ?? ""))
            #expect(ReleaseURLs.page(v).absoluteString == c["releasePage"].string)
            #expect(ReleaseURLs.macZip(v).absoluteString == c["macZip"].string)
            #expect(ReleaseURLs.macChecksum(v).absoluteString == c["macChecksum"].string)
            #expect(UpdateOffer(version: v, critical: false).releaseNotesURL == ReleaseURLs.page(v))
        }
        for c in f["requestHeaders"].array {
            let v = try #require(AppVersion(c["current"].string ?? ""))
            #expect(UpdateCheck.requestHeaders(current: v) == c["expect"].object.compactMapValues(\.string), "headers: \(c)")
        }
    }
}

/// macOS 앱이 Core 에 맡기는 설치 판단 (Homebrew 인지, 체크섬, 새 번들 검증). 리눅스에서도 돈다.
@Suite struct UpdateInstallerTests {
    private let v0102 = AppVersion("0.10.2")!

    @Test func homebrewIsDetectedFromTheCaskroomRecord() {
        #expect(UpdateInstaller.method { $0 == "/opt/homebrew/Caskroom/bulchimbeon" } == .homebrew(brew: "/opt/homebrew/bin/brew"))
        #expect(UpdateInstaller.method { $0 == "/usr/local/Caskroom/bulchimbeon" } == .homebrew(brew: "/usr/local/bin/brew"))
        // 다른 cask 만 있으면 직접 설치
        #expect(UpdateInstaller.method { $0 == "/opt/homebrew/Caskroom/other" } == .direct)
        #expect(UpdateInstaller.method { _ in false } == .direct)
    }

    @Test func homebrewScriptIsConstantAndVisible() throws {
        for lang in L10n.Language.allCases {
            let script = try #require(UpdateInstaller.homebrewScript(brew: "/opt/homebrew/bin/brew", lang: lang))
            #expect(script.hasPrefix("#!/bin/sh\n"))
            #expect(script.contains("'/opt/homebrew/bin/brew' update && '/opt/homebrew/bin/brew' upgrade --cask bulchimbeon"))
            #expect(script.contains("/usr/bin/open -b io.github.s3onghyun.bulchimbeon"))
            #expect(script.contains("rm -f -- \"$0\""))
            // 문구는 작은따옴표로 감싸 셸이 해석하지 않는다
            for line in script.split(separator: "\n") where line.hasPrefix("echo ") {
                #expect(line.hasPrefix("echo '") && line.hasSuffix("'"), "\(line)")
            }
        }
        // brew 경로는 알려진 접두어만
        #expect(UpdateInstaller.homebrewScript(brew: "/tmp/brew", lang: .en) == nil)
        #expect(UpdateInstaller.homebrewScript(brew: "/opt/homebrew/bin/brew; rm -rf ~", lang: .en) == nil)
    }

    @Test func homebrewScriptIsValidShell() throws {
        for lang in L10n.Language.allCases {
            let script = try #require(UpdateInstaller.homebrewScript(brew: "/usr/local/bin/brew", lang: lang))
            let process = Process()
            process.executableURL = URL(fileURLWithPath: "/bin/sh")
            process.arguments = ["-n", "-c", script] // 문법만 확인하고 실행하지 않는다
            try process.run()
            process.waitUntilExit()
            #expect(process.terminationStatus == 0, "\(lang)")
        }
    }

    @Test func shellQuoting() {
        #expect(UpdateInstaller.shellQuote("a b") == "'a b'")
        #expect(UpdateInstaller.shellQuote("it's") == "'it'\\''s'")
    }

    @Test func checksumFile() {
        let hex = String(repeating: "ab", count: 32)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "\(hex)  Bulchimbeon-0.10.2.zip\n", fileName: "Bulchimbeon-0.10.2.zip") == hex)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "\(hex.uppercased()) *Bulchimbeon-0.10.2.zip", fileName: "Bulchimbeon-0.10.2.zip") == hex)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "\(hex)  ./Bulchimbeon-0.10.2.zip", fileName: "Bulchimbeon-0.10.2.zip") == hex)
        // 다른 파일의 체크섬, 짧은 값, 빈 파일은 받지 않는다
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "\(hex)  Bulchimbeon-0.10.1.zip", fileName: "Bulchimbeon-0.10.2.zip") == nil)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "abcd  Bulchimbeon-0.10.2.zip", fileName: "Bulchimbeon-0.10.2.zip") == nil)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: "", fileName: "Bulchimbeon-0.10.2.zip") == nil)
        #expect(UpdateInstaller.expectedSHA256(checksumFile: String(repeating: "zz", count: 32) + "  Bulchimbeon-0.10.2.zip",
                                               fileName: "Bulchimbeon-0.10.2.zip") == nil)
    }

    @Test func checksumComparison() {
        let hex = String(repeating: "0f", count: 32)
        #expect(UpdateInstaller.checksumMatches(expected: hex, actual: Array(repeating: 0x0f, count: 32)))
        #expect(!UpdateInstaller.checksumMatches(expected: hex, actual: Array(repeating: 0x0e, count: 32)))
        #expect(!UpdateInstaller.checksumMatches(expected: hex, actual: []))
    }

    @Test func installLocation() {
        #expect(UpdateInstaller.locationProblem(bundlePath: "/Applications/Bulchimbeon.app", writable: true) == nil)
        #expect(UpdateInstaller.locationProblem(bundlePath: "/Applications/Bulchimbeon.app", writable: false) == .notWritable)
        #expect(UpdateInstaller.locationProblem(
            bundlePath: "/private/var/folders/xy/T/AppTranslocation/1234/d/Bulchimbeon.app", writable: true) == .translocated)
        #expect(UpdateInstaller.locationProblem(bundlePath: "/Users/me/src/bulchimbeon/.build/debug", writable: true) == .notAnApp)
    }

    private func signature(id: String? = "io.github.s3onghyun.bulchimbeon", team: String? = nil,
                           hardened: Bool = true, valid: Bool = true) -> UpdateInstaller.Signature {
        UpdateInstaller.Signature(valid: valid, identifier: id, teamIdentifier: team, hardenedRuntime: hardened)
    }

    private func candidate(_ sig: UpdateInstaller.Signature? = nil, bundleID: String? = "io.github.s3onghyun.bulchimbeon",
                           version: String? = "0.10.2") -> UpdateInstaller.Candidate {
        UpdateInstaller.Candidate(signature: sig ?? signature(), bundleIdentifier: bundleID, shortVersion: version)
    }

    @Test func newBundleMustMatchTheRunningApp() {
        let running = signature()
        let ok = candidate()
        #expect(UpdateInstaller.verify(ok, running: running, expected: v0102) == nil)
        #expect(UpdateInstaller.verify(candidate(signature(valid: false)), running: running, expected: v0102) == .signatureInvalid)
        #expect(UpdateInstaller.verify(candidate(signature(id: "com.example.evil")), running: running, expected: v0102) == .identifierMismatch)
        #expect(UpdateInstaller.verify(candidate(signature(id: nil)), running: running, expected: v0102) == .identifierMismatch)
        #expect(UpdateInstaller.verify(candidate(bundleID: "com.example.evil"), running: running, expected: v0102) == .identifierMismatch)
        #expect(UpdateInstaller.verify(candidate(signature(hardened: false)), running: running, expected: v0102) == .notHardened)
        #expect(UpdateInstaller.verify(candidate(version: "0.10.1"), running: running, expected: v0102) == .wrongVersion)
        #expect(UpdateInstaller.verify(candidate(version: nil), running: running, expected: v0102) == .wrongVersion)
        // 팀 ID: 지금은 둘 다 없다 (ad-hoc). 공증 후에는 같은 팀이어야 하고, 서명이 빠진 번들로 내려갈 수 없다
        let notarized = signature(team: "ABCDE12345")
        #expect(UpdateInstaller.verify(candidate(signature(team: "ABCDE12345")), running: notarized, expected: v0102) == nil)
        #expect(UpdateInstaller.verify(candidate(), running: notarized, expected: v0102) == .teamMismatch)
        #expect(UpdateInstaller.verify(candidate(signature(team: "ZZZZZ99999")), running: notarized, expected: v0102) == .teamMismatch)
        #expect(UpdateInstaller.verify(candidate(signature(team: "ABCDE12345")), running: running, expected: v0102) == .teamMismatch)
    }

    @Test func everyFailureAndPhaseHasAMessageInEveryLanguage() {
        let failures: [UpdateInstaller.Failure] = [.translocated, .notWritable, .notAnApp, .download, .checksumMissing,
                                                    .checksumMismatch, .unzip, .signatureInvalid, .identifierMismatch,
                                                    .teamMismatch, .notHardened, .wrongVersion, .replace, .terminal]
        for lang in L10n.Language.allCases {
            for failure in failures {
                #expect(!failure.message(lang).isEmpty, "\(failure) \(lang)")
            }
            #expect(UpdatePhase.idle.menuLine(lang) == nil)
            #expect(UpdatePhase.downloading(0.5).menuLine(lang)?.contains("50%") == true)
            #expect(UpdatePhase.downloading(nil).menuLine(lang) != nil)
            #expect(UpdatePhase.verifying.menuLine(lang) != nil)
            #expect(UpdatePhase.installing.menuLine(lang) != nil)
            #expect(UpdatePhase.failed(.checksumMismatch).menuLine(lang)?.contains(UpdateInstaller.Failure.checksumMismatch.message(lang)) == true)
        }
        #expect(!UpdatePhase.idle.isBusy && UpdatePhase.verifying.isBusy && !UpdatePhase.failed(.download).isBusy)
    }
}
