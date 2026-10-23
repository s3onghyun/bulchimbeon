import Foundation
import Testing
@testable import BulchimbeonCore

/// 스프라이트 공유 픽스처 (`testdata/engine/sprites.json`). 리눅스 트레이(`internal/sprites`)가 같은 파일을 그린다.
/// 모든 군복 테마 × 모든 포즈의 프레임(행)과 테마가 입힌 팔레트 색을 Swift Core 에서 그대로 내보낸다.
///
/// Shared sprite fixture. Every registered `UniformTheme` × every `Sprites.Pose`: the frame rows and the
/// resolved palette, exported from the core. The Go tray renders this file, so the two cannot drift apart.
/// After changing sprites or themes, regenerate it with
/// `BULCHIMBEON_WRITE_SPRITES=1 swift test --filter SpriteFixtureTests` and commit the result
/// (and copy it to `internal/sprites/sprites.json`; `go test ./...` checks the copy).
@Suite struct SpriteFixtureTests {
    static let fileName = "sprites.json"

    /// 메뉴바 애니메이션 간격 (`Sources/Bulchimbeon/SpriteImages.swift` 의 `SpriteAnimator`)
    static let frameSeconds = 0.5

    @Test func fixtureMatchesCore() throws {
        let url = Fixture.directory.appendingPathComponent(Self.fileName)
        let want = Self.render()
        if ProcessInfo.processInfo.environment["BULCHIMBEON_WRITE_SPRITES"] == "1" {
            try want.write(to: url, atomically: true, encoding: .utf8)
        }
        let got = try String(contentsOf: url, encoding: .utf8)
        #expect(got == want, "testdata/engine/sprites.json is out of date: BULCHIMBEON_WRITE_SPRITES=1 swift test --filter SpriteFixtureTests")
    }

    /// 픽스처의 프레임 간격이 macOS 메뉴바 타이머와 같은지 (앱 타깃은 리눅스에서 빌드되지 않아 소스로 확인)
    @Test func frameSecondsMatchesMenuBarTimer() throws {
        let source = Fixture.directory.deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/Bulchimbeon/SpriteImages.swift")
        let text = try String(contentsOf: source, encoding: .utf8)
        #expect(text.contains("withTimeInterval: \(Self.frameSeconds)"), "SpriteAnimator interval changed: update frameSeconds")
    }

    /// 내보낸 본문(= 파일, 위 테스트)을 다시 읽어 Core 와 비교한다
    @Test func fixtureCoversEveryThemeAndPose() throws {
        let f = try JSONDecoder().decode(JSONValue.self, from: Data(Self.render().utf8))
        #expect(f["poses"].strings == Sprites.Pose.allCases.map(\.rawValue))
        #expect(f["themes"].array.map { $0["id"].string ?? "" } == UniformTheme.all.map(\.id))
        for (theme, t) in zip(UniformTheme.all, f["themes"].array) {
            let frames = t["frames"].array
            for pose in Sprites.Pose.allCases {
                let got = t["poseFrames"][pose.rawValue].array.map { frames[$0.int ?? -1].strings }
                #expect(got == Sprites.frames(pose, theme: theme).map(\.rows), "\(theme.id)/\(pose)")
            }
            for (ch, hex) in t["palette"].object {
                #expect(hex.string == Self.hex(theme.hex(Character(ch)) ?? 0), "\(theme.id) '\(ch)'")
            }
        }
    }

    /// 픽스처 본문. 사람이 읽기 쉽게 한 행씩 쓴다. 프레임은 테마마다 처음 나온 순서로 한 번씩만.
    static func render() -> String {
        var out = "{\n"
        out += "  \"size\": \(Sprites.frames(.standby)[0].width),\n"
        out += "  \"frameSeconds\": \(frameSeconds),\n"
        out += "  \"transparent\": \".\",\n"
        out += "  \"poses\": [" + Sprites.Pose.allCases.map { quote($0.rawValue) }.joined(separator: ", ") + "],\n"
        out += "  \"themes\": [\n"
        out += UniformTheme.all.map(theme).joined(separator: ",\n")
        out += "\n  ]\n}\n"
        return out
    }

    private static func theme(_ theme: UniformTheme) -> String {
        var frames: [[String]] = []
        var poseFrames: [String] = []
        for pose in Sprites.Pose.allCases {
            let indices = Sprites.frames(pose, theme: theme).map { sprite in
                if let i = frames.firstIndex(of: sprite.rows) { return i }
                frames.append(sprite.rows)
                return frames.count - 1
            }
            poseFrames.append(quote(pose.rawValue) + ": [" + indices.map(String.init).joined(separator: ", ") + "]")
        }
        let used = Set(frames.joined().joined()).subtracting(["."]).sorted()
        let palette = used.map { quote(String($0)) + ": " + quote(hex(theme.hex($0) ?? 0)) }

        var out = "    {\n"
        out += "      \"id\": \(quote(theme.id)),\n"
        out += "      \"name\": {\"ko\": \(quote(theme.nameKo)), \"en\": \(quote(theme.nameEn)), \"zh\": \(quote(theme.nameZh))},\n"
        out += "      \"palette\": {" + palette.joined(separator: ", ") + "},\n"
        out += "      \"poseFrames\": {" + poseFrames.joined(separator: ", ") + "},\n"
        out += "      \"frames\": [\n"
        out += frames.map { rows in
            "        [\n" + rows.map { "          " + quote($0) }.joined(separator: ",\n") + "\n        ]"
        }.joined(separator: ",\n")
        out += "\n      ]\n    }"
        return out
    }

    static func hex(_ v: UInt32) -> String {
        let digits = String(v, radix: 16, uppercase: true)
        return "#" + String(repeating: "0", count: max(6 - digits.count, 0)) + digits
    }

    private static func quote(_ s: String) -> String {
        var out = "\""
        for scalar in s.unicodeScalars {
            switch scalar {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case _ where scalar.value < 0x20: out += "\\u" + String(repeating: "0", count: 4 - String(scalar.value, radix: 16).count) + String(scalar.value, radix: 16)
            default: out.unicodeScalars.append(scalar)
            }
        }
        return out + "\""
    }
}
