import CoreGraphics

/// 군복 테마 — 같은 이등병, 다른 옷 (#17). 나라가 아니라 테마다.
///
/// 테마는 옷·소품 색(`colors`)과 옷 위의 작은 덧그림(`overlays`: 모자 띠, 무늬)만 바꾼다.
/// 얼굴·피부는 모든 테마가 같다. 국기·실제 계급장·실제 군대의 군복은 넣지 않는다.
/// 언어·지역으로 자동 선택하지 않는다 — 사용자가 메뉴에서 고른 것만 입는다.
///
/// 테마 추가: 아래 `navy`·`arctic` 처럼 하나 만들고 `all` 끝에 넣은 뒤 `make test`.
public struct UniformTheme: Sendable, Identifiable, Equatable {
    /// 테마가 바꿀 수 있는 문자: 방탄모(H h), 전투복(U u), 전투화(B), 손전등(L), 테마 전용 장식(A).
    /// 외곽선·눈(K), 피부(S), 볼(P), 입(M), 불빛(Y y), zzz(Z), 별(W), 땅(D d)은 바꿀 수 없다.
    public static let themeableCharacters: Set<Character> = ["H", "h", "U", "u", "B", "L", "A"]

    /// 옷 위의 덧그림. 기본 스프라이트에서 옷(테마 문자) 픽셀 위에만 찍히고, 얼굴·외곽선·빈칸은 건드리지 않는다.
    public struct Overlay: Sendable, Equatable {
        public let sprite: PixelSprite
        public let x: Int
        public let y: Int

        public init(_ rows: [String], x: Int, y: Int) {
            sprite = PixelSprite(rows)
            self.x = x
            self.y = y
        }
    }

    /// `uniformTheme` 설정에 저장되는 값. 한 번 출시되면 바꾸지 않는다 (docs/compatibility.md).
    public let id: String
    public let nameKo: String
    public let nameEn: String
    /// 팔레트 덮어쓰기. 키는 `themeableCharacters` 만.
    public let colors: [Character: UInt32]
    public let overlays: [Overlay]

    public init(id: String, nameKo: String, nameEn: String, colors: [Character: UInt32] = [:], overlays: [Overlay] = []) {
        let problems = Self.violations(colors: colors, overlays: overlays)
        precondition(problems.isEmpty, "UniformTheme \(id): \(problems.joined(separator: "; "))")
        self.id = id
        self.nameKo = nameKo
        self.nameEn = nameEn
        self.colors = colors
        self.overlays = overlays
    }

    public var name: String { name(L10n.language) }
    public func name(_ lang: L10n.Language) -> String { L10n.t(nameKo, nameEn, lang) }

    /// 테마 규칙 위반 목록 (비어 있으면 통과). `init` 이 이걸로 막고, 테스트도 이걸로 확인한다.
    public static func violations(colors: [Character: UInt32], overlays: [Overlay]) -> [String] {
        var problems: [String] = []
        for ch in colors.keys.sorted() where !themeableCharacters.contains(ch) {
            problems.append("'\(ch)' 는 테마가 바꿀 수 없는 색")
        }
        for overlay in overlays {
            for ch in Set(overlay.sprite.rows.joined()).sorted() where ch != "." {
                if !themeableCharacters.contains(ch) {
                    problems.append("덧그림에 테마 문자가 아닌 '\(ch)'")
                } else if colors[ch] == nil && Palette.hex[ch] == nil {
                    problems.append("덧그림의 '\(ch)' 에 색이 없음")
                }
            }
        }
        return problems
    }

    /// 팔레트 문자 → 색 (테마 덮어쓰기 우선)
    public func hex(_ ch: Character) -> UInt32? { colors[ch] ?? Palette.hex[ch] }

    public func cgColor(_ ch: Character) -> CGColor? { hex(ch).map(Palette.cgColor(hex:)) }

    /// 기본 스프라이트에 이 테마의 덧그림을 입힌다. 옷 픽셀 위에만 찍힌다.
    public func dress(_ sprite: PixelSprite) -> PixelSprite {
        var grid = sprite.rows.map(Array.init)
        for overlay in overlays {
            for (dy, row) in overlay.sprite.rows.enumerated() {
                for (dx, ch) in row.enumerated() where ch != "." {
                    let gx = overlay.x + dx, gy = overlay.y + dy
                    guard grid.indices.contains(gy), grid[gy].indices.contains(gx),
                          Self.themeableCharacters.contains(grid[gy][gx]) else { continue }
                    grid[gy][gx] = ch
                }
            }
        }
        return PixelSprite(grid.map { String($0) })
    }

    // MARK: - 테마 목록

    /// 메뉴에 나오는 순서. 기본이 항상 처음.
    public static let all: [UniformTheme] = [.default, .navy, .arctic, .desert]

    /// 저장된 id → 테마. 모르는 값이면 기본.
    public static func named(_ id: String?) -> UniformTheme {
        all.first { $0.id == id } ?? .default
    }

    /// 지금까지의 전투복. 아무것도 바꾸지 않는다.
    public static let `default` = UniformTheme(id: "default", nameKo: "기본 전투복", nameEn: "Standard")

    /// 남색 모자·근무복, 모자에 흰 띠
    public static let navy = UniformTheme(
        id: "navy", nameKo: "네이비", nameEn: "Navy",
        colors: [
            "H": 0x3A5288, "h": 0x5C76AD,
            "U": 0x3F5C93, "u": 0x2C4470,
            "B": 0x1C1B22,
            "A": 0xE8EDF4,
        ],
        overlays: [
            Overlay(["AAAAAAAAA"], x: 3, y: 3),
        ]
    )

    /// 흰 방탄모·설상복, 옅은 회색 얼룩
    public static let arctic = UniformTheme(
        id: "arctic", nameKo: "설원", nameEn: "Arctic",
        colors: [
            "H": 0xE3E9EE, "h": 0xFFFFFF,
            "U": 0xD5DDE4, "u": 0xA5B2BE,
            "B": 0x56606B,
            "A": 0xB4C2CE,
        ],
        overlays: [
            Overlay(["......A...A.....",
                     "....A...A......."], x: 0, y: 2),
            Overlay([".....A..........",
                     ".........A......",
                     ".....A.........."], x: 0, y: 11),
        ]
    )

    /// 모래색 모자·군복, 진한 모래색 점 몇 개. 일반적인 사막 색일 뿐 실제 군대 무늬가 아니다.
    public static let desert = UniformTheme(
        id: "desert", nameKo: "사막", nameEn: "Desert",
        colors: [
            "H": 0xC9A66B, "h": 0xD9BE8F,
            "U": 0xB8915A, "u": 0x8F6F3F,
            "B": 0x5C4630,
            "A": 0x8A6A3C,
        ],
        overlays: [
            Overlay(["..A.....A.",
                     ".......A.."], x: 1, y: 2),
            Overlay([".....A....",
                     "..A.......",
                     ".......A.."], x: 0, y: 11),
        ]
    )
}
