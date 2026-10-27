import Foundation

/// UserDefaults 키 (`io.github.s3onghyun.bulchimbeon` 도메인).
///
/// `isPublic` 인 키는 1.0 호환성 약속 대상이다 — 이름·값 형식을 바꾸지 않는다 (docs/compatibility.md).
/// 나머지는 앱 내부 상태라 언제든 바뀔 수 있다.
public enum SettingsKey: String, CaseIterable, Sendable {
    // 공개 설정 — `defaults write` 로 바꿔도 되는 것
    case dutyMode
    case batteryFloor
    case agentGraceMinutes
    case agentWatchCountsIdle
    case customAgents
    case patrolEnabled
    case patrolScale
    case uniformTheme

    // 내부 상태 — 재시작 복귀용, 손대지 않는다
    case dutyOrder
    case dutyOrderUntil
    case lidEngagedByUs
    /// 첫 실행 안내를 닫았는지
    case hasSeenWelcome
    /// 시험용. `advisory|alert|normal` 로 발열 단계를 흉내 낸다
    case simulateHeat

    public var isPublic: Bool {
        switch self {
        case .dutyMode, .batteryFloor, .agentGraceMinutes, .agentWatchCountsIdle,
             .customAgents, .patrolEnabled, .patrolScale, .uniformTheme:
            true
        case .dutyOrder, .dutyOrderUntil, .lidEngagedByUs, .hasSeenWelcome, .simulateHeat:
            false
        }
    }
}

/// 공개 설정이 받는 값. 범위 밖이면 기본값으로 돌아간다.
public enum SettingsValues {
    public static let batteryFloors = [10, 20, 30, 0] // 0 = 끄기
    public static let defaultBatteryFloor = 20
    public static let graceMinutes = [1, 3, 5, 10]
    public static let defaultGraceMinutes = 5
    public static let patrolScales = [2, 3]
    public static let defaultPatrolScale = 2
    /// `uniformTheme` 값은 `UniformTheme.all` 의 id. 모르는 값이면 기본.
    public static let defaultUniformTheme = UniformTheme.default.id
}

/// 공개 실행 인자. 나머지(`--demo`, `--demo-scale`)는 홍보 녹화용이라 약속하지 않는다.
public enum LaunchArgument: String, CaseIterable, Sendable {
    case agentWatch = "--agent-watch"
    case diagnose = "--diagnose"
}
