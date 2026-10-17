import Foundation

/// Korean, English and Simplified Chinese UI; unsupported languages fall back to English.
public enum L10n {
    public enum Language: String, CaseIterable, Sendable { case ko, en, zh }

    public static let language: Language = detect()

    public static func detect(_ preferred: [String] = Locale.preferredLanguages) -> Language {
        let first = preferred.first?.lowercased().split(whereSeparator: { "-_.@".contains($0) }).first
        switch first {
        case "ko": return .ko
        case "zh": return .zh
        default: return .en
        }
    }

    /// 메뉴의 언어 선택. 앱 전용 설정(`AppleLanguages`, 앱 도메인)으로 저장한다.
    public enum Preference: String, CaseIterable, Sendable {
        case system, ko, en, zh

        /// 앱 도메인에 저장할 값. nil 이면 설정을 지워 시스템 언어를 따른다.
        public var appleLanguages: [String]? {
            switch self {
            case .system: nil
            case .ko: ["ko"]
            case .en: ["en"]
            case .zh: ["zh-Hans"]
            }
        }

        /// 앱 도메인에 저장된 값 → 선택 (전역 설정이 섞인 값이 아니라 앱 도메인 값만 넣어야 한다)
        public init(appDomainLanguages: [String]?) {
            switch appDomainLanguages?.first {
            case let first? where first.hasPrefix("ko"): self = .ko
            case let first? where first.hasPrefix("en"): self = .en
            case let first? where L10n.detect([first]) == .zh: self = .zh
            default: self = .system
            }
        }

        public var title: String {
            switch self {
            case .system: L10n.t("시스템 설정 따름 (System)", "Follow System", "跟随系统")
            case .ko: "한국어"
            case .en: "English"
            case .zh: "简体中文"
            }
        }
    }

    public static func t(_ ko: String, _ en: String, _ zh: String, _ lang: Language = language) -> String {
        switch lang {
        case .ko: ko
        case .en: en
        case .zh: zh
        }
    }
}
