#if canImport(CoreGraphics)
import CoreGraphics
#endif

/// 팔레트 문자 → 색. '.' 은 투명.
public enum Palette {
    static let hex: [Character: UInt32] = [
        "K": 0x2A2330, // 외곽선
        "H": 0x5E7140, // 방탄모
        "h": 0x8AA05C, // 방탄모 하이라이트
        "S": 0xFFDDBE, // 피부
        "P": 0xFF9E9E, // 볼터치
        "M": 0xC0504D, // 입
        "U": 0x6E8048, // 전투복
        "u": 0x4F5E33, // 전투복 그림자
        "B": 0x2F2A2A, // 전투화
        "L": 0x9A9A9A, // 손전등
        "Y": 0xFFE66B, // 불빛 / 달
        "y": 0xFFF4B0, // 불빛 (옅음)
        "Z": 0x9DB8FF, // zzz
        "W": 0xFFFFFF, // 별
        "D": 0x2E3B2A, // 땅
        "d": 0x3C4C36, // 땅 하이라이트
    ]

    #if canImport(CoreGraphics)
    public static func cgColor(_ ch: Character) -> CGColor? {
        hex[ch].map(cgColor(hex:))
    }

    public static func cgColor(hex v: UInt32) -> CGColor {
        CGColor(red: CGFloat((v >> 16) & 0xFF) / 255,
                green: CGFloat((v >> 8) & 0xFF) / 255,
                blue: CGFloat(v & 0xFF) / 255,
                alpha: 1)
    }
    #endif
}

public struct PixelSprite: Sendable, Equatable {
    public let rows: [String]
    public var width: Int { rows.first?.count ?? 0 }
    public var height: Int { rows.count }

    public init(_ rows: [String]) { self.rows = rows }

    /// `other` 의 불투명 픽셀을 (x, y) 위치에 덮어쓴 새 스프라이트.
    public func placing(_ other: PixelSprite, x: Int, y: Int) -> PixelSprite {
        var grid = rows.map(Array.init)
        for (dy, row) in other.rows.enumerated() {
            for (dx, ch) in row.enumerated() where ch != "." {
                let gx = x + dx, gy = y + dy
                guard grid.indices.contains(gy), grid[gy].indices.contains(gx) else { continue }
                grid[gy][gx] = ch
            }
        }
        return PixelSprite(grid.map { String($0) })
    }

    /// 좌우 반전 (왼쪽으로 걷기)
    public func mirrored() -> PixelSprite {
        PixelSprite(rows.map { String($0.reversed()) })
    }

    #if canImport(CoreGraphics)
    /// CoreGraphics(y-up) 컨텍스트에 그린다. origin 은 스프라이트의 좌하단. 색은 `theme` 팔레트.
    public func draw(in ctx: CGContext, origin: CGPoint = .zero, pixel: CGFloat, theme: UniformTheme = .default) {
        for (y, row) in rows.enumerated() {
            for (x, ch) in row.enumerated() {
                guard let color = theme.cgColor(ch) else { continue }
                ctx.setFillColor(color)
                ctx.fill(CGRect(x: origin.x + CGFloat(x) * pixel,
                                y: origin.y + CGFloat(height - 1 - y) * pixel,
                                width: pixel, height: pixel))
            }
        }
    }
    #endif
}

/// 2등신 치비 이등병. 16×16, 메뉴바에서 1pt = 1픽셀.
public enum Sprites {
    public enum Pose: String, CaseIterable, Sendable {
        case sleeping // 💤 취침
        case standby  // 👀 근무 대기 — 두리번
        case duty     // 🪖 근무 중 — 손전등 깜빡
        case salute   // 🫡 근무 교대
        case patrol   // 🚶 순찰 — 오른쪽을 보고 걷기 (왼쪽은 mirrored)
        case heat     // 🥵 폭염 — 땀 뻘뻘, 전투휴식
    }

    public static func frames(_ pose: Pose) -> [PixelSprite] {
        switch pose {
        // 0.5초마다 한 프레임
        case .sleeping: [sleepingA, sleepingA, sleepingB, sleepingB]
        case .standby: [standbyLeft, standbyLeft, standbyLeft, standbyRight, standbyRight, standbyRight]
        case .duty: [dutyBeam, dutyBeam, dutyBeam, dutyDark]
        case .salute: [salute, salute, saluteSparkle, saluteSparkle]
        case .patrol: [walkA, walkB]
        case .heat: [heatA, heatB]
        }
    }

    /// `theme` 의 덧그림을 입힌 프레임. 그릴 때도 같은 테마를 `draw(in:theme:)` 에 넘긴다.
    public static func frames(_ pose: Pose, theme: UniformTheme) -> [PixelSprite] {
        frames(pose).map { theme.dress($0) }
    }

    // 머리(방탄모 + 얼굴) 공통부
    private static let helmet = [
        "....KKKKKKK.....",
        "...KhhHHHHHK....",
        "..KhHHHHHHHHK...",
        "..KHHHHHHHHHK...",
        ".KKKKKKKKKKKKK..",
        "..KSSSSSSSSSK...",
    ]
    private static func face(eyes: String, cheeks: String = "..KPSSSMSSSPK...") -> [String] {
        [eyes, eyes, cheeks, "...KSSSSSSSK....", "....KKKKKKK....."]
    }
    private static let legs = [
        "...KUUUuUUUK....",
        "...KUUKKKUUK....",
        "...KBBK.KBBK....",
    ]

    private static let eyesFront = "..KSSKSSSKSSK..."
    private static let eyesLeft  = "..KSKSSSKSSSK..."
    private static let eyesRight = "..KSSSKSSSKSK..."

    static let dutyBeam = PixelSprite(helmet + face(eyes: eyesFront) + [
        "...KUUUuUUUK..yy",
        "..KSUUUuUUUSKLYY",
        "...KUUUuUUUK..yy",
        "...KUUKKKUUK....",
        "...KBBK.KBBK....",
    ])

    // 순찰 걸음: 다리를 벌렸다(A) 모았다(B)
    static let walkA = PixelSprite(Array(dutyBeam.rows.prefix(14)) + [
        "..KUUK...KUUK...",
        ".KBBK.....KBBK..",
    ])

    static let walkB = PixelSprite(Array(dutyBeam.rows.prefix(14)) + [
        "....KUUKKUUK....",
        "....KBBKKBBK....",
    ])

    // 폭염: 감은 눈 + 땀방울이 흘러내림
    static let heatA = sleepingB.placing(PixelSprite(["Z"]), x: 1, y: 6).placing(PixelSprite(["Z"]), x: 13, y: 5)
    static let heatB = sleepingB.placing(PixelSprite(["Z"]), x: 1, y: 8).placing(PixelSprite(["Z"]), x: 13, y: 7)

    static let dutyDark = PixelSprite(helmet + face(eyes: eyesFront) + [
        "...KUUUuUUUK....",
        "..KSUUUuUUUSKL..",
    ] + legs)

    static let standbyLeft = PixelSprite(helmet + face(eyes: eyesLeft) + [
        "...KUUUuUUUK....",
        "..KSUUUuUUUSK...",
    ] + legs)

    static let standbyRight = PixelSprite(helmet + face(eyes: eyesRight) + [
        "...KUUUuUUUK....",
        "..KSUUUuUUUSK...",
    ] + legs)

    private static let eyesClosed = "..KSKKSSSKKSK..."
    private static let sleepingBody = [
        "...KUUUuUUUK....",
        "..KSUUUuUUUSK...",
    ] + legs

    private static let sleepingFace = [
        "..KSSSSSSSSSK...",
        eyesClosed,
        "..KPSSSKSSSPK...",
        "...KSSSSSSSK....",
        "....KKKKKKK.....",
    ]

    static let sleepingA = PixelSprite([
        "....KKKKKKK..ZZZ",
        "...KhhHHHHHK...Z",
        "..KhHHHHHHHHK.Z.",
        "..KHHHHHHHHHKZZZ",
    ] + helmet.dropFirst(4) + sleepingFace + sleepingBody)

    static let sleepingB = PixelSprite([
        "....KKKKKKK.....",
        "...KhhHHHHHK.Z..",
        "..KhHHHHHHHHK...",
    ] + helmet.dropFirst(3) + sleepingFace + sleepingBody)

    // 경례: ^ ^ 웃는 눈, "충성!" 외치는 입, 오른손은 방탄모 챙에
    static let salute = PixelSprite([
        "....KKKKKKK.....",
        "...KhhHHHHHK....",
        "..KhHHHHHHHHK...",
        "..KHHHHHHHHHKK..",
        ".KKKKKKKKKKKSSK.",
        "..KSSSSSSSSSKSK.",
        "..KSSKSSSKSSKUK.",
        "..KSKSKSKSKSKUK.",
        "..KPSSMMMSSPKUK.",
        "...KSSSSSSSKUK..",
        "....KKKKKKKUK...",
        "...KUUUuUUUUK...",
        "..KSUUUuUUUK....",
    ] + legs)

    static let saluteSparkle = salute.placing(PixelSprite([
        "W.",
        ".W",
    ]), x: 0, y: 0).placing(PixelSprite(["Y"]), x: 15, y: 0)

    /// 앱 아이콘용 24×24 장면: 밤하늘 + 달 + 별 + 땅 + 근무 중 이등병
    public static let appIconScene: PixelSprite = PixelSprite([
        "........................",
        "..YYY...........W.......",
        ".YY.............. ......",
        ".YY.......W.............",
        ".YY..................W..",
        "..YYY...................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "........................",
        "dddddddddddddddddddddddd",
        "DDDDDDDDDDDDDDDDDDDDDDDD",
    ].map { $0.replacingOccurrences(of: " ", with: ".") })
    .placing(dutyBeam, x: 3, y: 7)
}
