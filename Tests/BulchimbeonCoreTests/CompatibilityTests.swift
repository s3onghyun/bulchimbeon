import Foundation
import Testing
@testable import BulchimbeonCore

/// 1.0 호환성 약속 (docs/compatibility.md). 여기가 실패하면 사용자의 `defaults write`·스크립트가 깨진다.
/// 의도한 변경이라면 문서·CHANGELOG 에 "호환성 깨짐"으로 적고 메이저 버전을 올린다.
@Suite struct CompatibilityTests {
    @Test func publicSettingsKeysAreFrozen() {
        let keys = SettingsKey.allCases.filter(\.isPublic).map(\.rawValue)
        #expect(keys == ["dutyMode", "batteryFloor", "agentGraceMinutes", "agentWatchCountsIdle",
                         "customAgents", "patrolEnabled", "patrolScale", "uniformTheme", "checkForUpdates"])
    }

    @Test func publicSettingValuesAreFrozen() {
        #expect(DutyMode.allCases.map(\.rawValue) == ["display", "system", "lid"])
        #expect(Set(SettingsValues.batteryFloors) == [0, 10, 20, 30])
        #expect(SettingsValues.defaultBatteryFloor == 20)
        #expect(Set(SettingsValues.graceMinutes) == [1, 3, 5, 10])
        #expect(SettingsValues.defaultGraceMinutes == 5)
        #expect(Set(SettingsValues.patrolScales) == [2, 3])
        #expect(SettingsValues.defaultPatrolScale == 2)
        // 테마는 늘어날 수 있지만, 한 번 나간 id 는 그대로 남는다
        #expect(Set(["default", "navy", "arctic"]).isSubset(of: UniformTheme.all.map(\.id)))
        #expect(SettingsValues.defaultUniformTheme == "default")
        #expect(SettingsValues.defaultCheckForUpdates == true)
    }

    @Test func launchArgumentsAreFrozen() {
        #expect(LaunchArgument.allCases.map(\.rawValue) == ["--agent-watch", "--diagnose"])
    }

    @Test func dutyLogLocationAndLineFormatAreFrozen() throws {
        #if os(macOS) // 근무일지 위치는 macOS 앱의 약속이다
        let path = try #require(DutyLog.defaultFileURL).path
        #expect(path.hasSuffix("/Library/Application Support/Bulchimbeon/duty-log.txt"))
        #endif

        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let file = dir.appendingPathComponent("duty-log.txt")
        DutyLog(fileURL: file).write("근무 투입 — 테스트\n둘째 줄", at: Date(timeIntervalSince1970: 1_000_000))

        let text = try String(contentsOf: file, encoding: .utf8)
        // 한 항목 = 한 줄: ISO 8601(UTC) + 공백 두 칸 + 내용. 줄바꿈은 공백으로 접힌다.
        #expect(text == "1970-01-12T13:46:40Z  근무 투입 — 테스트 둘째 줄\n")
    }
}
