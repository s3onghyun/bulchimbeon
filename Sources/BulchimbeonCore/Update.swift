import Foundation

/// 앱 버전. SemVer 2.0 순서를 따른다 (숫자 세 개, 프리릴리스 식별자, 빌드 메타데이터는 무시).
///
/// 태그 앞의 `v` 는 받는다. `git describe` 로 붙은 개발 빌드 꼬리(`0.10.2-3-gabc1234`)는 떼고 그 태그로 비교한다
/// — 태그 뒤의 개발 빌드에 같은 태그를 "새 버전"으로 권하지 않기 위해서다. 읽을 수 없는 값(`dev`, `0.0.0~ci`)은 nil.
public struct AppVersion: Comparable, Hashable, Sendable, CustomStringConvertible {
    public let major: Int
    public let minor: Int
    public let patch: Int
    public let prerelease: [String]

    public init?(_ text: String) {
        var s = Substring(text.trimmingCharacters(in: .whitespacesAndNewlines))
        if s.first == "v" || s.first == "V" { s = s.dropFirst() }
        if let plus = s.firstIndex(of: "+") {
            let build = s[s.index(after: plus)...]
            guard build.split(separator: ".", omittingEmptySubsequences: false).allSatisfy(Self.isIdentifier) else { return nil }
            s = s[..<plus]
        }
        s = Self.droppingGitDescribe(s)

        var core = s
        var pre: [String] = []
        if let dash = s.firstIndex(of: "-") {
            core = s[..<dash]
            let ids = s[s.index(after: dash)...].split(separator: ".", omittingEmptySubsequences: false)
            guard ids.allSatisfy({ Self.isIdentifier($0) && !Self.hasLeadingZero($0) }) else { return nil }
            pre = ids.map(String.init)
        }
        let numbers = core.split(separator: ".", omittingEmptySubsequences: false)
        guard numbers.count == 3,
              numbers.allSatisfy({ (1...9).contains($0.count) && Self.isNumeric($0) && !Self.hasLeadingZero($0) }),
              let major = Int(numbers[0]), let minor = Int(numbers[1]), let patch = Int(numbers[2])
        else { return nil }
        (self.major, self.minor, self.patch, prerelease) = (major, minor, patch, pre)
    }

    public var description: String {
        "\(major).\(minor).\(patch)" + (prerelease.isEmpty ? "" : "-" + prerelease.joined(separator: "."))
    }

    public static func < (a: AppVersion, b: AppVersion) -> Bool {
        if (a.major, a.minor, a.patch) != (b.major, b.minor, b.patch) {
            return (a.major, a.minor, a.patch) < (b.major, b.minor, b.patch)
        }
        // 프리릴리스가 없는 쪽이 크다
        if a.prerelease.isEmpty || b.prerelease.isEmpty { return !a.prerelease.isEmpty && b.prerelease.isEmpty }
        for (x, y) in zip(a.prerelease, b.prerelease) where x != y {
            switch (isNumeric(Substring(x)), isNumeric(Substring(y))) {
            case (true, true): return x.count != y.count ? x.count < y.count : x < y // 앞자리 0 이 없으니 길이 → 사전순
            case (true, false): return true // 숫자 < 영숫자
            case (false, true): return false
            case (false, false): return x.utf8.lexicographicallyPrecedes(y.utf8)
            }
        }
        return a.prerelease.count < b.prerelease.count
    }

    private static func isIdentifier(_ s: Substring) -> Bool {
        !s.isEmpty && s.utf8.allSatisfy { $0 == UInt8(ascii: "-") || isAlphanumeric($0) }
    }

    private static func isNumeric(_ s: Substring) -> Bool {
        !s.isEmpty && s.utf8.allSatisfy { (UInt8(ascii: "0")...UInt8(ascii: "9")).contains($0) }
    }

    private static func hasLeadingZero(_ s: Substring) -> Bool { s.count > 1 && s.first == "0" && isNumeric(s) }

    private static func isAlphanumeric(_ c: UInt8) -> Bool {
        (UInt8(ascii: "0")...UInt8(ascii: "9")).contains(c) || (UInt8(ascii: "a")...UInt8(ascii: "z")).contains(c)
            || (UInt8(ascii: "A")...UInt8(ascii: "Z")).contains(c)
    }

    /// `<태그>-<커밋 수>-g<해시>[-dirty]` → `<태그>`
    private static func droppingGitDescribe(_ s: Substring) -> Substring {
        var parts = s.split(separator: "-", omittingEmptySubsequences: false)
        let dirty = parts.last == "dirty"
        if dirty { parts.removeLast() }
        guard parts.count >= 3, let hash = parts.last, hash.count >= 5, hash.first == "g",
              hash.dropFirst().utf8.allSatisfy({ (UInt8(ascii: "0")...UInt8(ascii: "9")).contains($0) || (UInt8(ascii: "a")...UInt8(ascii: "f")).contains($0) }),
              isNumeric(parts[parts.count - 2])
        else { return s }
        return Substring(parts.dropLast(2).joined(separator: "-"))
    }
}

/// GitHub Releases 의 최신 릴리스에서 읽는 것: 버전과 "권장: 중요 수정" 표시.
///
/// 중요 수정 표시는 릴리스 본문(`docs/release-notes/vX.Y.Z.md`)의 HTML 주석 한 줄이다. 렌더링하면 보이지 않는다.
/// - `<!-- bulchimbeon: critical -->`: 이 릴리스보다 낮은 모든 버전에 권한다
/// - `<!-- bulchimbeon: critical below 0.10.2 -->`: 0.10.2 보다 낮은 버전에만 권한다 (뒤 릴리스가 이전의 중요 수정을 이어 권할 때)
///
/// 주석 안의 공백·대소문자는 상관없고, 여러 줄이면 가장 넓은 범위를 쓴다. 문장 안에 섞인 주석, 모양이 틀린 주석은 무시한다.
public struct ReleaseInfo: Equatable, Sendable {
    public let version: AppVersion
    /// 지금 버전이 이보다 낮으면 중요 수정으로 권한다. nil 이면 일반 업데이트
    public let criticalBelow: AppVersion?

    public init(version: AppVersion, criticalBelow: AppVersion?) {
        self.version = version
        self.criticalBelow = criticalBelow
    }

    private struct Payload: Decodable {
        let tagName: String?
        let body: String?
        let draft: Bool?
        let prerelease: Bool?

        enum CodingKeys: String, CodingKey {
            case tagName = "tag_name", body, draft, prerelease
        }
    }

    /// `GET /repos/{owner}/{repo}/releases/latest` 응답. 초안·프리릴리스·크기 초과·버전이 아닌 태그는 nil.
    public static func parse(_ data: Data) -> ReleaseInfo? {
        guard data.count <= UpdateCheck.maxResponseBytes,
              let payload = try? JSONDecoder().decode(Payload.self, from: data),
              payload.draft != true, payload.prerelease != true,
              let version = payload.tagName.flatMap(AppVersion.init)
        else { return nil }
        return ReleaseInfo(version: version, criticalBelow: criticalBelow(body: payload.body ?? "", release: version))
    }

    static func criticalBelow(body: String, release: AppVersion) -> AppVersion? {
        var widest: AppVersion?
        for line in body.split(whereSeparator: \.isNewline) {
            let text = line.trimmingCharacters(in: .whitespaces)
            guard text.count >= 7, text.hasPrefix("<!--"), text.hasSuffix("-->") else { continue }
            let tokens = String(text.dropFirst(4).dropLast(3))
                .replacingOccurrences(of: ":", with: " : ")
                .split(whereSeparator: \.isWhitespace)
            guard tokens.count >= 3, tokens[0].lowercased() == "bulchimbeon", tokens[1] == ":",
                  tokens[2].lowercased() == "critical"
            else { continue }
            let bound: AppVersion? = switch tokens.count {
            case 3: release
            case 5 where tokens[3].lowercased() == "below": AppVersion(String(tokens[4]))
            default: nil
            }
            if let bound, widest.map({ $0 < bound }) ?? true { widest = bound }
        }
        return widest
    }
}

/// 업데이트 확인 요청: 무엇을, 언제, 어떤 머리글로.
///
/// 보내는 것은 최신 릴리스 주소 하나에 대한 GET 뿐이다. 설정·근무일지·기기 식별자는 보내지 않고, 머리글은 아래 셋뿐
/// (언어는 고정값 `en` 이라 사용자의 시스템 언어가 드러나지 않는다).
public enum UpdateCheck {
    /// 요청은 하루에 한 번까지 (성공·실패 모두 한 번으로 친다)
    public static let interval: TimeInterval = 86_400
    /// 응답이 이보다 크면 읽지 않는다
    public static let maxResponseBytes = 1 << 20
    public static let latestReleaseAPI = URL(string: "https://api.github.com/repos/s3onghyun/bulchimbeon/releases/latest")!

    /// 지금 물어봐도 되는지. 꺼져 있거나 버전을 모르는 빌드(`dev`)면 묻지 않는다. 시계가 뒤로 갔으면 다시 묻는다.
    public static func isDue(enabled: Bool, current: AppVersion?, now: Date, lastCheck: Date?) -> Bool {
        guard enabled, current != nil else { return false }
        guard let lastCheck else { return true }
        return now < lastCheck || now.timeIntervalSince(lastCheck) >= interval
    }

    public static func requestHeaders(current: AppVersion) -> [String: String] {
        ["User-Agent": "Bulchimbeon/\(current)", "Accept": "application/vnd.github+json", "Accept-Language": "en"]
    }
}

/// 릴리스 주소. API 응답의 URL 을 믿지 않고, 확인된 버전으로 직접 만든다.
public enum ReleaseURLs {
    public static let repository = "https://github.com/s3onghyun/bulchimbeon"
    public static let latestPage = URL(string: repository + "/releases/latest")!

    public static func page(_ version: AppVersion) -> URL {
        URL(string: "\(repository)/releases/tag/v\(version)")!
    }

    public static func download(_ version: AppVersion, file: String) -> URL {
        URL(string: "\(repository)/releases/download/v\(version)/\(file)")!
    }

    public static func macZipName(_ version: AppVersion) -> String { "Bulchimbeon-\(version).zip" }
    public static func macZip(_ version: AppVersion) -> URL { download(version, file: macZipName(version)) }
    public static func macChecksum(_ version: AppVersion) -> URL { download(version, file: macZipName(version) + ".sha256") }
}

/// 메뉴에 보일 업데이트 권유.
public struct UpdateOffer: Equatable, Sendable {
    public let version: AppVersion
    /// 권장: 중요 수정
    public let critical: Bool

    public init(version: AppVersion, critical: Bool) {
        self.version = version
        self.critical = critical
    }

    /// 켜져 있고 더 새 버전이 알려져 있을 때만 권한다. 내려가는 버전은 권하지 않는다.
    public static func decide(current: AppVersion?, latest: ReleaseInfo?, enabled: Bool) -> UpdateOffer? {
        guard enabled, let current, let latest, current < latest.version else { return nil }
        return UpdateOffer(version: latest.version, critical: latest.criticalBelow.map { current < $0 } ?? false)
    }

    /// 메뉴 항목: "새 버전 X.Y.Z — 업데이트"
    public func menuTitle(_ lang: L10n.Language = L10n.language) -> String {
        L10n.t("새 버전 \(version) — 업데이트", "New version \(version) — Update", "新版本 \(version) — 更新", lang) + criticalSuffix(lang)
    }

    /// 문장 속에 들어갈 짧은 형태 (리눅스 `status`): "새 버전 X.Y.Z"
    public func summary(_ lang: L10n.Language = L10n.language) -> String {
        L10n.t("새 버전 \(version)", "new version \(version)", "新版本 \(version)", lang) + criticalSuffix(lang)
    }

    private func criticalSuffix(_ lang: L10n.Language) -> String {
        critical ? L10n.t(" (권장: 중요 수정)", " (recommended: important fix)", "（推荐：重要修复）", lang) : ""
    }

    public static func releaseNotesTitle(_ lang: L10n.Language = L10n.language) -> String {
        L10n.t("릴리스 노트 보기", "Release notes", "查看发行说明", lang)
    }

    public var releaseNotesURL: URL { ReleaseURLs.page(version) }
}

/// macOS 앱의 업데이트 설치에서 시스템을 부르지 않는 판단들. 실제 내려받기·서명 확인·교체는 앱 쪽 껍데기가 한다.
///
/// - Homebrew 로 설치했으면 (`<prefix>/Caskroom/bulchimbeon` 기록) 앱이 스스로 바꾸지 않고, 보이는 터미널에서
///   `brew upgrade --cask bulchimbeon` 을 돌린다. cask 는 `auto_updates` 를 선언하지 않으므로 brew 의 기록과 어긋나지 않게.
/// - 아니면 릴리스 zip 을 받아 `.sha256` 과 맞추고, 새 번들의 서명이 지금 앱과 같은지 확인한 뒤에만 교체한다.
public enum UpdateInstaller {
    public enum Method: Equatable, Sendable {
        case homebrew(brew: String)
        case direct
    }

    public static let homebrewPrefixes = ["/opt/homebrew", "/usr/local"]
    public static let caskName = "bulchimbeon"
    public static let bundleIdentifier = "io.github.s3onghyun.bulchimbeon"

    public static func method(isDirectory: (String) -> Bool) -> Method {
        for prefix in homebrewPrefixes where isDirectory("\(prefix)/Caskroom/\(caskName)") {
            return .homebrew(brew: "\(prefix)/bin/brew")
        }
        return .direct
    }

    /// 터미널에서 열 `.command` 스크립트. 바깥 문자열은 들어가지 않는다: brew 경로는 알려진 두 곳 중 하나만 받고,
    /// 문구는 앱 상수를 작은따옴표로 감싼다. 끝나면 앱을 다시 열고, 스크립트 파일은 스스로 지운다.
    public static func homebrewScript(brew: String, lang: L10n.Language = L10n.language) -> String? {
        guard homebrewPrefixes.contains(where: { brew == "\($0)/bin/brew" }) else { return nil }
        let t = { (ko: String, en: String, zh: String) in "echo " + shellQuote(L10n.t(ko, en, zh, lang)) }
        let b = shellQuote(brew)
        return """
        #!/bin/sh
        # Bulchimbeon update through Homebrew. Written by the app when you chose Update; removes itself.
        rm -f -- "$0"
        rmdir -- "${0%/*}" 2>/dev/null
        \(t("불침번 업데이트: Homebrew 로 새 버전을 설치합니다.", "Bulchimbeon update: installing the new version with Homebrew.", "Bulchimbeon 更新：使用 Homebrew 安装新版本。"))
        echo '$ brew update && brew upgrade --cask \(caskName)'
        echo
        \(b) update && \(b) upgrade --cask \(caskName)
        code=$?
        echo
        if [ "$code" -eq 0 ]; then
        \(t("완료. 불침번을 다시 엽니다. 이 창은 닫아도 됩니다.", "Done. Reopening Bulchimbeon; you can close this window.", "完成。正在重新打开 Bulchimbeon，可以关闭此窗口。"))
        \(t("Homebrew 가 이미 최신이라고 하면 tap 이 아직 갱신 전입니다. 잠시 뒤 다시 해 주세요.", "If Homebrew says it is already up to date, the tap has not caught up yet; try again later.", "如果 Homebrew 提示已是最新，说明 tap 尚未更新，请稍后再试。"))
        \(t("macOS 가 새 버전을 막으면: 시스템 설정 → 개인정보 보호 및 보안 → 그래도 열기", "If macOS blocks the new version: System Settings → Privacy & Security → Open Anyway", "如果 macOS 阻止新版本：系统设置 → 隐私与安全性 → 仍要打开"))
        /usr/bin/open -b \(bundleIdentifier)
        else
        \(t("업데이트 실패. 위 출력을 확인하거나 릴리스 페이지에서 직접 받으세요:", "Update failed. Check the output above, or download it from the release page:", "更新失败。请查看上面的输出，或从发行页面下载："))
        echo \(shellQuote(ReleaseURLs.latestPage.absoluteString))
        fi

        """
    }

    public static func shellQuote(_ s: String) -> String {
        "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }

    /// `shasum -a 256` 형식(`<hex>  <파일>`, `*<파일>`, `./<파일>`)에서 그 파일의 SHA-256 (소문자 hex)
    public static func expectedSHA256(checksumFile: String, fileName: String) -> String? {
        for line in checksumFile.split(whereSeparator: \.isNewline) {
            let fields = line.split(whereSeparator: \.isWhitespace)
            guard fields.count == 2 else { continue }
            var name = fields[1]
            if name.hasPrefix("*") { name = name.dropFirst() }
            if name.hasPrefix("./") { name = name.dropFirst(2) }
            let hex = fields[0].lowercased()
            guard name == fileName, hex.count == 64, hex.allSatisfy({ hexDigits.contains($0) }) else { continue }
            return hex
        }
        return nil
    }

    public static func checksumMatches(expected: String, actual: [UInt8]) -> Bool {
        actual.count == 32 && String(actual.flatMap { [hexDigits[Int($0 >> 4)], hexDigits[Int($0 & 0xf)]] }) == expected.lowercased()
    }

    private static let hexDigits = Array("0123456789abcdef")

    /// 앱이 스스로 바뀔 수 있는 곳에 있는지
    public static func locationProblem(bundlePath: String, writable: Bool) -> Failure? {
        if bundlePath.contains("/AppTranslocation/") { return .translocated }
        if !bundlePath.hasSuffix(".app") { return .notAnApp }
        if !writable { return .notWritable }
        return nil
    }

    /// 코드 서명에서 읽은 것 (Security 프레임워크)
    public struct Signature: Equatable, Sendable {
        public var valid: Bool
        public var identifier: String?
        public var teamIdentifier: String?
        public var hardenedRuntime: Bool

        public init(valid: Bool, identifier: String?, teamIdentifier: String?, hardenedRuntime: Bool) {
            self.valid = valid
            self.identifier = identifier
            self.teamIdentifier = teamIdentifier
            self.hardenedRuntime = hardenedRuntime
        }
    }

    /// 내려받아 푼 새 번들
    public struct Candidate: Equatable, Sendable {
        public var signature: Signature
        public var bundleIdentifier: String?
        public var shortVersion: String?

        public init(signature: Signature, bundleIdentifier: String?, shortVersion: String?) {
            self.signature = signature
            self.bundleIdentifier = bundleIdentifier
            self.shortVersion = shortVersion
        }
    }

    /// 새 번들이 지금 앱과 같은 서명 식별자·팀 ID 의 hardened runtime 불침번이고, 권한 바로 그 버전인지.
    /// 지금은 ad-hoc 서명이라 팀 ID 가 양쪽 다 없다. 공증 후에는 같은 팀이어야 한다 (서명이 빠진 번들로 내려갈 수 없음).
    public static func verify(_ candidate: Candidate, running: Signature, expected: AppVersion) -> Failure? {
        let sig = candidate.signature
        guard sig.valid else { return .signatureInvalid }
        guard let identifier = sig.identifier, identifier == running.identifier,
              candidate.bundleIdentifier == bundleIdentifier
        else { return .identifierMismatch }
        guard sig.teamIdentifier == running.teamIdentifier else { return .teamMismatch }
        guard sig.hardenedRuntime else { return .notHardened }
        guard candidate.shortVersion.flatMap(AppVersion.init) == expected else { return .wrongVersion }
        return nil
    }

    public enum Failure: Equatable, Sendable {
        case translocated, notWritable, notAnApp
        case download, checksumMissing, checksumMismatch, unzip
        case signatureInvalid, identifierMismatch, teamMismatch, notHardened, wrongVersion
        case replace, terminal

        public func message(_ lang: L10n.Language = L10n.language) -> String {
            let t = { (ko: String, en: String, zh: String) in L10n.t(ko, en, zh, lang) }
            return switch self {
            case .translocated: t("앱이 임시 위치(App Translocation)에서 실행 중입니다. 응용 프로그램 폴더로 옮긴 뒤 다시 해 주세요",
                                  "the app is running from a temporary location (App Translocation). Move it to Applications and try again",
                                  "应用正在临时位置运行（App Translocation）。请将其移到“应用程序”文件夹后重试")
            case .notWritable: t("앱이 있는 폴더에 쓸 수 없습니다. 릴리스 페이지에서 받아 직접 바꿔 주세요",
                                 "cannot write to the app's folder. Download it from the release page and replace it yourself",
                                 "无法写入应用所在的文件夹。请从发行页面下载并手动替换")
            case .notAnApp: t("앱 번들로 실행 중이 아니라 스스로 업데이트할 수 없습니다",
                              "not running from an app bundle, so it cannot update itself", "未从应用包运行，无法自行更新")
            case .download: t("내려받지 못했습니다", "the download failed", "下载失败")
            case .checksumMissing: t("체크섬 파일을 읽지 못했습니다", "could not read the checksum file", "无法读取校验和文件")
            case .checksumMismatch: t("체크섬이 맞지 않습니다 (파일이 손상됐거나 바뀜)",
                                      "the checksum does not match (the file is damaged or was changed)", "校验和不匹配（文件已损坏或被更改）")
            case .unzip: t("압축을 풀지 못했습니다", "could not unpack the download", "无法解压下载的文件")
            case .signatureInvalid: t("새 앱의 코드 서명이 올바르지 않습니다", "the new app's code signature is not valid", "新应用的代码签名无效")
            case .identifierMismatch: t("새 앱이 불침번이 아닙니다 (서명 식별자가 다름)",
                                        "the new app is not Bulchimbeon (different signing identifier)", "新应用不是 Bulchimbeon（签名标识符不同）")
            case .teamMismatch: t("새 앱의 서명 팀이 지금 앱과 다릅니다", "the new app is signed by a different team than this one",
                                  "新应用的签名团队与当前应用不同")
            case .notHardened: t("새 앱이 hardened runtime 으로 서명되지 않았습니다", "the new app is not signed with the hardened runtime",
                                 "新应用未使用 hardened runtime 签名")
            case .wrongVersion: t("새 앱의 버전이 권한 버전과 다릅니다", "the new app's version is not the one offered", "新应用的版本与提示的版本不同")
            case .replace: t("앱을 바꾸지 못했습니다", "could not replace the app", "无法替换应用")
            case .terminal: t("터미널을 열지 못했습니다", "could not open Terminal", "无法打开“终端”")
            }
        }
    }
}

/// 업데이트 진행 상황 (메뉴 한 줄)
public enum UpdatePhase: Equatable, Sendable {
    case idle
    /// 받은 비율 (전체 크기를 모르면 nil)
    case downloading(Double?)
    case verifying
    case installing
    case failed(UpdateInstaller.Failure)

    public var isBusy: Bool {
        switch self {
        case .downloading, .verifying, .installing: true
        case .idle, .failed: false
        }
    }

    public func menuLine(_ lang: L10n.Language = L10n.language) -> String? {
        let t = { (ko: String, en: String, zh: String) in L10n.t(ko, en, zh, lang) }
        switch self {
        case .idle:
            return nil
        case .downloading(let fraction):
            let percent = fraction.map { " \(Int((min(max($0, 0), 1) * 100).rounded()))%" } ?? ""
            return t("⬇️ 내려받는 중…", "⬇️ Downloading…", "⬇️ 正在下载…") + percent
        case .verifying:
            return t("🔍 확인 중… (체크섬·서명)", "🔍 Verifying the checksum and signature…", "🔍 正在验证校验和与签名…")
        case .installing:
            return t("📦 설치 중… 곧 다시 시작합니다", "📦 Installing… restarting shortly", "📦 正在安装… 即将重新启动")
        case .failed(let failure):
            return t("⚠️ 업데이트 실패: ", "⚠️ Update failed: ", "⚠️ 更新失败：") + failure.message(lang)
        }
    }
}
