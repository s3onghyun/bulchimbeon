import Testing
@testable import BulchimbeonCore

/// 군복 테마 (#17): 옷만 바뀌고, 기본은 지금과 똑같고, 얼굴·피부는 누구도 못 바꾼다.
@Suite struct UniformThemeTests {
    /// 테마가 절대 바꿀 수 없는 문자: 외곽선·눈, 피부, 볼, 입, 불빛, zzz, 별, 땅
    private static let faceAndScenery: [Character] = ["K", "S", "P", "M", "Y", "y", "Z", "W", "D", "d"]

    @Test func paletteIsUnchanged() {
        #expect(Palette.hex == SpriteGolden.palette)
    }

    @Test(arguments: Sprites.Pose.allCases)
    func defaultThemeIsByteIdenticalToGolden(pose: Sprites.Pose) throws {
        let indices = try #require(SpriteGolden.poseFrames[pose.rawValue], "골든에 없는 포즈 \(pose)")
        let golden = indices.map { SpriteGolden.uniqueFrames[$0] }
        #expect(Sprites.frames(pose).map(\.rows) == golden)
        #expect(Sprites.frames(pose, theme: .default).map(\.rows) == golden)
        for rows in golden {
            for ch in rows.joined() where ch != "." {
                #expect(UniformTheme.default.hex(ch) == SpriteGolden.palette[ch], "'\(ch)' 색이 바뀜")
            }
        }
    }

    @Test func goldenCoversEveryPose() {
        #expect(Set(SpriteGolden.poseFrames.keys) == Set(Sprites.Pose.allCases.map(\.rawValue)))
    }

    @Test(arguments: UniformTheme.all, Sprites.Pose.allCases)
    func everyThemeRendersEveryFrameAtDefaultSize(theme: UniformTheme, pose: Sprites.Pose) {
        let base = Sprites.frames(pose)
        let themed = Sprites.frames(pose, theme: theme)
        #expect(themed.count == base.count)
        for (frame, original) in zip(themed, base) {
            #expect(frame.width == original.width)
            #expect(frame.height == original.height)
            #expect(frame.rows.allSatisfy { $0.count == original.width })
            for ch in frame.rows.joined() where ch != "." {
                #if canImport(CoreGraphics)
                #expect(theme.cgColor(ch) != nil, "\(theme.id): 색이 없는 문자 '\(ch)'")
                #else
                #expect(theme.hex(ch) != nil, "\(theme.id): 색이 없는 문자 '\(ch)'")
                #endif
            }
        }
    }

    /// 덧그림은 옷 위에만: 테마 문자가 아닌 픽셀(얼굴·외곽선·빈칸)은 기본과 똑같다
    @Test(arguments: UniformTheme.all, Sprites.Pose.allCases)
    func themesOnlyTouchClothing(theme: UniformTheme, pose: Sprites.Pose) {
        for (frame, original) in zip(Sprites.frames(pose, theme: theme), Sprites.frames(pose)) {
            for (row, originalRow) in zip(frame.rows, original.rows) {
                for (ch, originalCh) in zip(row, originalRow) where !UniformTheme.themeableCharacters.contains(originalCh) {
                    #expect(ch == originalCh, "\(theme.id)/\(pose): '\(originalCh)' 가 '\(ch)' 로 바뀜")
                }
            }
        }
    }

    @Test(arguments: UniformTheme.all)
    func faceSkinAndSceneryColorsAreNeverThemed(theme: UniformTheme) {
        for ch in Self.faceAndScenery {
            #expect(theme.hex(ch) == Palette.hex[ch], "\(theme.id): '\(ch)' 색을 바꿈")
        }
        #expect(Set(theme.colors.keys).isSubset(of: UniformTheme.themeableCharacters))
        #expect(UniformTheme.violations(colors: theme.colors, overlays: theme.overlays).isEmpty)
    }

    @Test func whitelistRejectsFaceAndSceneryCharacters() {
        #expect(UniformTheme.themeableCharacters.isDisjoint(with: Self.faceAndScenery))
        for ch in Self.faceAndScenery {
            #expect(!UniformTheme.violations(colors: [ch: 0x000000], overlays: []).isEmpty, "'\(ch)' 색 덮어쓰기가 통과함")
            #expect(!UniformTheme.violations(colors: [:], overlays: [.init([String(ch)], x: 0, y: 0)]).isEmpty,
                    "덧그림의 '\(ch)' 가 통과함")
        }
        // 테마 전용 장식 문자는 색을 줘야 쓸 수 있다
        #expect(!UniformTheme.violations(colors: [:], overlays: [.init(["A"], x: 0, y: 0)]).isEmpty)
        #expect(UniformTheme.violations(colors: ["A": 0x123456], overlays: [.init(["A"], x: 0, y: 0)]).isEmpty)
    }

    @Test func overlaysLandOnlyOnClothingPixels() {
        let theme = UniformTheme(id: "test", nameKo: "시험", nameEn: "Test", colors: ["A": 0x123456],
                                 overlays: [.init(["AAAA"], x: 0, y: 0)])
        let dressed = theme.dress(PixelSprite(["KSU.", "...."]))
        #expect(dressed.rows == ["KSA.", "...."])
    }

    @Test func registryStartsWithDefaultAndHasUniqueIDs() {
        #expect(UniformTheme.all.first == .default)
        #expect(UniformTheme.default.colors.isEmpty && UniformTheme.default.overlays.isEmpty)
        let ids = UniformTheme.all.map(\.id)
        #expect(Set(ids).count == ids.count)
        #expect(ids.allSatisfy { !$0.isEmpty && $0.allSatisfy { ("a"..."z").contains($0) || ("0"..."9").contains($0) || $0 == "-" } })
    }

    @Test func unknownOrMissingIDFallsBackToDefault() {
        #expect(UniformTheme.named(nil) == .default)
        #expect(UniformTheme.named("") == .default)
        #expect(UniformTheme.named("ko") == .default)
        #expect(UniformTheme.named("navy") == .navy)
        #expect(UniformTheme.named("arctic") == .arctic)
    }

    @Test(arguments: UniformTheme.all)
    func namesExistInKoreanAndEnglish(theme: UniformTheme) {
        let ko = theme.name(.ko), en = theme.name(.en)
        #expect(ko.unicodeScalars.contains { (0xAC00...0xD7A3).contains($0.value) }, "\(theme.id): 한국어 이름 없음")
        #expect(!en.isEmpty && !en.unicodeScalars.contains { (0xAC00...0xD7A3).contains($0.value) }, "\(theme.id): 영어 이름 없음")
        #expect(ko != en)
    }
}
