import Foundation

/// 의견 보내기의 메일 대체 경로 주소. 이 한 줄만 바꾸면 메뉴에 "메일로 보내기" 가 생긴다.
/// 비어 있으면 메일 항목을 숨기고 다음 버전에 생긴다는 안내 한 줄만 보인다.
public enum FeedbackContact {
    public static let email = ""
}

/// 의견에 붙는 환경 정보. 앱 버전·OS 버전·맥 기종·카탈로그 에이전트 이름만 담는다
/// (경로·사용자 이름·호스트 이름·작업 내용은 담지 않는다). 값은 만들 때 허용 문자로 거른다.
public struct FeedbackEnvironment: Equatable, Sendable {
    public let appVersion: String
    public let build: String
    public let osVersion: String
    public let macModel: String?
    public let agents: [String]

    /// 에이전트 이름은 최대 이 개수까지 (본문 길이 제한)
    public static let maxAgents = 8

    public init(appVersion: String?, build: String?, os: OperatingSystemVersion, macModel: String?, agents: [AgentKind]) {
        self.appVersion = Self.clean(appVersion, allowed: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-+") ?? "dev"
        self.build = Self.clean(build, allowed: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-+") ?? "?"
        self.osVersion = "\(os.majorVersion).\(os.minorVersion).\(os.patchVersion)"
        self.macModel = Self.clean(macModel, allowed: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ,")
        // 사용자 지정 에이전트 이름은 사용자가 정한 문자열이라 이름 대신 "custom" 으로만 적는다
        let names = agents.map { $0.id.hasPrefix("custom:") ? "custom" : $0.displayName }
        self.agents = Array(Set(names)).sorted().prefix(Self.maxAgents).map { $0 }
    }

    /// 허용 문자만 이루어진 32자 이하 값만 받는다. 아니면 nil (일부만 잘라 쓰지 않는다)
    static func clean(_ value: String?, allowed: String) -> String? {
        guard let value, !value.isEmpty, value.count <= 32, value.allSatisfy({ allowed.contains($0) }) else { return nil }
        return value
    }
}

/// 메뉴의 "의견 보내기" 선택지
public enum FeedbackChoice: String, CaseIterable, Identifiable, Sendable {
    case githubIssue, email

    public var id: String { rawValue }

    public func title(_ lang: L10n.Language = L10n.language) -> String {
        switch self {
        case .githubIssue: L10n.t("GitHub 이슈 열기", "Open a GitHub issue", "提交 GitHub Issue", lang)
        case .email: L10n.t("메일로 보내기", "Send an email instead", "改用邮件发送", lang)
        }
    }
}

/// 의견 보내기: 미리 채운 GitHub 이슈 또는 메일 주소를 만든다. 앱은 아무것도 보내지 않는다 —
/// 브라우저나 메일 앱을 열 뿐이고, 보내는 것은 사용자가 거기서 직접 한다.
public enum Feedback {
    public static let newIssue = ReleaseURLs.repository + "/issues/new"
    /// 본문 상한 (GitHub 주소 길이 제한 안쪽)
    public static let maxBodyBytes = 2000

    /// 메뉴에 보일 선택지. 메일 주소가 없거나 올바르지 않으면 GitHub 만.
    public static func choices(email: String = FeedbackContact.email) -> [FeedbackChoice] {
        isValidEmail(email) ? [.githubIssue, .email] : [.githubIssue]
    }

    /// 메일 항목이 숨겨졌을 때 보일 한 줄. 메일 주소가 있으면 nil.
    public static func emailHint(email: String = FeedbackContact.email, _ lang: L10n.Language = L10n.language) -> String? {
        isValidEmail(email) ? nil
            : L10n.t("메일로 보내기는 다음 버전에 생겨요.", "An email option comes in a later version.", "邮件方式将在后续版本中提供。", lang)
    }

    public static func privacyLine(_ lang: L10n.Language = L10n.language) -> String {
        L10n.t("앱은 아무것도 보내지 않아요. 직접 보내기 전엔 전송되지 않아요.",
               "The app sends nothing; it is only sent when you submit it.",
               "应用不会发送任何内容，只有你亲自提交才会发送。", lang)
    }

    public static func title(_ lang: L10n.Language = L10n.language) -> String {
        L10n.t("[의견] ", "[Feedback] ", "[反馈] ", lang)
    }

    public static func body(_ env: FeedbackEnvironment, _ lang: L10n.Language = L10n.language) -> String {
        var lines = [
            L10n.t("### 무슨 일이 있었나요?", "### What happened?", "### 发生了什么？", lang),
            "",
            "",
            L10n.t("### 기대한 동작", "### What did you expect?", "### 你期望的结果", lang),
            "",
            "",
            "### Environment",
            "- Bulchimbeon \(env.appVersion) (build \(env.build))",
            "- macOS \(env.osVersion)",
        ]
        if let model = env.macModel { lines.append("- Mac \(model)") }
        lines.append("- Agents: " + (env.agents.isEmpty ? "none detected" : env.agents.joined(separator: ", ")))
        return lines.joined(separator: "\n")
    }

    public static func issueURL(_ env: FeedbackEnvironment, _ lang: L10n.Language = L10n.language) -> URL {
        URL(string: newIssue + "?title=" + encode(title(lang)) + "&body=" + encode(body(env, lang)))!
    }

    /// 메일 주소가 없거나 올바르지 않으면 nil
    public static func mailURL(_ env: FeedbackEnvironment, email: String = FeedbackContact.email,
                               _ lang: L10n.Language = L10n.language) -> URL? {
        guard isValidEmail(email) else { return nil }
        return URL(string: "mailto:" + email + "?subject=" + encode(title(lang)) + "&body=" + encode(body(env, lang)))
    }

    /// 쿼리 값 인코딩: 영숫자와 `-._~` 만 그대로 두고 나머지는 모두 %XX (공백은 `+` 가 아니라 `%20`, mailto 와 같은 규칙)
    public static func encode(_ value: String) -> String {
        var allowed = CharacterSet()
        allowed.insert(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~")
        return value.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
    }

    static func isValidEmail(_ email: String) -> Bool {
        email.range(of: #"^[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+\z"#, options: .regularExpression) != nil
    }
}
