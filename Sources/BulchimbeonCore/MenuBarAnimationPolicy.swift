import Foundation

/// 메뉴 막대 스프라이트의 다음 프레임을 실제로 그릴 조건.
///
/// `NSStatusItem` 하나는 macOS가 보조 화면의 메뉴 막대에도 복제해 그린다. 따라서 화면마다
/// 서로 다른 프레임을 줄 수는 없고, 보이지 않는 동안에는 타이머 자체를 멈추는 편이 낫다.
public struct MenuBarAnimationState: Equatable, Sendable {
    public let frameCount: Int
    public let displaySleeping: Bool
    public let screenLocked: Bool
    public let statusItemVisible: Bool
    public let lowPowerMode: Bool

    public init(frameCount: Int, displaySleeping: Bool, screenLocked: Bool,
                statusItemVisible: Bool, lowPowerMode: Bool) {
        self.frameCount = frameCount
        self.displaySleeping = displaySleeping
        self.screenLocked = screenLocked
        self.statusItemVisible = statusItemVisible
        self.lowPowerMode = lowPowerMode
    }
}

public enum MenuBarAnimationPolicy {
    /// `nil` means there is no visible pixel change to schedule.
    public static func interval(for state: MenuBarAnimationState) -> TimeInterval? {
        guard state.frameCount > 1,
              !state.displaySleeping,
              !state.screenLocked,
              state.statusItemVisible else { return nil }
        // Low Power Mode intentionally favors fewer compositor updates over animation speed.
        return state.lowPowerMode ? 1.5 : 0.5
    }

    /// The status item can be seen while any of its windows is not occluded. With no window
    /// reported yet, assume it is visible so the sprite never freezes on a missing signal.
    public static func statusItemVisible(windowsVisible: [Bool]) -> Bool {
        windowsVisible.isEmpty || windowsVisible.contains(true)
    }
}
