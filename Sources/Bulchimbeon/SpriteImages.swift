import AppKit
import BulchimbeonCore

/// 스프라이트 → 메뉴바용 NSImage (1pt = 1픽셀, 컬러 유지)
@MainActor
enum SpriteImages {
    /// 지금 입은 군복 (`uniformTheme`). 바뀌면 캐시를 비운다.
    static var theme = UniformTheme.named(UserDefaults.standard.string(forKey: SettingsKey.uniformTheme.rawValue)) {
        didSet { if theme != oldValue { cache = [:] } }
    }
    private static var cache: [Sprites.Pose: [NSImage]] = [:]

    static func frames(_ pose: Sprites.Pose) -> [NSImage] {
        if let cached = cache[pose] { return cached }
        let images = Sprites.frames(pose, theme: theme).map { image($0, theme: theme) }
        cache[pose] = images
        return images
    }

    /// 군복 메뉴의 미리보기 (대기 자세 첫 프레임, 16pt — 레티나에서는 2x 로 그려진다)
    static func preview(_ theme: UniformTheme) -> NSImage {
        image(Sprites.frames(.standby, theme: theme)[0], theme: theme)
    }

    private static func image(_ sprite: PixelSprite, theme: UniformTheme) -> NSImage {
        let image = NSImage(size: NSSize(width: sprite.width, height: sprite.height), flipped: false) { _ in
            guard let ctx = NSGraphicsContext.current?.cgContext else { return false }
            ctx.setShouldAntialias(false)
            sprite.draw(in: ctx, pixel: 1, theme: theme)
            return true
        }
        image.isTemplate = false // 템플릿이면 단색 실루엣이 된다
        return image
    }
}

/// 군복 테마 선택. 사용자가 메뉴에서 고른 것만 입는다 — 언어·지역 등으로 자동 선택하지 않는다.
@MainActor
final class UniformSetting: ObservableObject {
    static let shared = UniformSetting()

    @Published var themeID: String {
        didSet {
            UserDefaults.standard.set(themeID, forKey: SettingsKey.uniformTheme.rawValue)
            SpriteImages.theme = UniformTheme.named(themeID)
            PatrolController.shared.redress()
        }
    }

    private init() { themeID = SpriteImages.theme.id }
}

/// 0.5초 간격 프레임 카운터. 화면이 꺼졌거나 세션이 잠긴 동안은 멈춘다.
@MainActor
final class SpriteAnimator: ObservableObject {
    @Published private(set) var tick = 0
    private var timer: Timer?
    private var screensAsleep = false, sessionInactive = false
    private var observers: [NSObjectProtocol] = []

    init() {
        let center = NSWorkspace.shared.notificationCenter
        let states: [(Notification.Name, ReferenceWritableKeyPath<SpriteAnimator, Bool>, Bool)] = [
            (NSWorkspace.screensDidSleepNotification, \.screensAsleep, true),
            (NSWorkspace.screensDidWakeNotification, \.screensAsleep, false),
            (NSWorkspace.sessionDidResignActiveNotification, \.sessionInactive, true),
            (NSWorkspace.sessionDidBecomeActiveNotification, \.sessionInactive, false),
        ]
        observers = states.map { name, state, value in
            center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor in
                    self?[keyPath: state] = value
                    self?.updateTimer()
                }
            }
        }
        updateTimer()
    }

    deinit {
        observers.forEach(NSWorkspace.shared.notificationCenter.removeObserver)
    }

    private func updateTimer() {
        guard !screensAsleep, !sessionInactive else {
            timer?.invalidate()
            timer = nil
            return
        }
        guard timer == nil else { return }
        let timer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.tick &+= 1 }
        }
        timer.tolerance = 0.1
        self.timer = timer
    }
}
