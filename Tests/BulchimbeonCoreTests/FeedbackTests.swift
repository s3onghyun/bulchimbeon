import Foundation
import Testing
@testable import BulchimbeonCore

/// 의견 보내기: 미리 채운 본문에는 버전·OS 사실만 들어가고, 주소는 올바르게 인코딩된다.
@Suite struct FeedbackTests {
    private static let os = OperatingSystemVersion(majorVersion: 26, minorVersion: 1, patchVersion: 0)

    private static func env(model: String? = "Mac15,3", agents: [AgentKind] = AgentCatalog.builtIn.prefix(2).map { $0 }
                            + AgentCatalog.customKinds(["secret-proj"])) -> FeedbackEnvironment {
        FeedbackEnvironment(appVersion: "0.12.7", build: "42", os: os, macModel: model, agents: agents)
    }

    @Test func bodyHasOnlyVersionAndOSFacts() {
        let body = Feedback.body(Self.env(), .en)
        #expect(body.contains("- Bulchimbeon 0.12.7 (build 42)"))
        #expect(body.contains("- macOS 26.1.0"))
        #expect(body.contains("- Mac Mac15,3"))
        #expect(body.contains("- Agents: Claude Code, Codex, custom"))
        #expect(body.contains("### What happened?"))
        // 사용자 지정 에이전트 이름(사용자가 정한 문자열)·경로·사용자·호스트 정보는 없다
        #expect(!body.contains("secret-proj"))
        // 짧은 사용자·호스트 이름은 우연히 겹칠 수 있어 6자 이상만 본다
        let names = [NSUserName(), ProcessInfo.processInfo.hostName].filter { $0.count >= 6 }
        for leak in ["/", "~", NSHomeDirectory()] + names where !leak.isEmpty {
            #expect(!body.contains(leak), "leak: \(leak)")
        }
    }

    @Test func untrustedValuesAreDroppedNotTrimmed() {
        let e = FeedbackEnvironment(appVersion: "1.0 /Users/me", build: "a&b=c", os: Self.os,
                                    macModel: "MacBookPro18,3\n/Users/me", agents: [])
        #expect(e.appVersion == "dev")
        #expect(e.build == "?")
        #expect(e.macModel == nil)
        let body = Feedback.body(e, .en)
        #expect(!body.contains("- Mac "))
        #expect(body.contains("- Agents: none detected"))
        #expect(!body.contains("/Users"))
    }

    @Test func agentListIsCapped() {
        let many = AgentCatalog.builtIn + AgentCatalog.customKinds(["a", "b"])
        #expect(Self.env(agents: many).agents.count <= FeedbackEnvironment.maxAgents)
    }

    @Test func issueURLIsEncodedAndShort() throws {
        for lang in L10n.Language.allCases {
            let url = Feedback.issueURL(Self.env(agents: AgentCatalog.builtIn), lang)
            let s = url.absoluteString
            #expect(s.hasPrefix("https://github.com/s3onghyun/bulchimbeon/issues/new?title="))
            #expect(Feedback.body(Self.env(agents: AgentCatalog.builtIn), lang).utf8.count < Feedback.maxBodyBytes)
            #expect(s.utf8.count < 8000)
            // 쿼리에는 영숫자·`-._~`·`%XX` 와 구분자만
            let query = try #require(s.split(separator: "?", maxSplits: 1).last)
            #expect(!query.contains(" ") && !query.contains("\n") && !query.contains("+") && !query.contains("#"))
            let items = try #require(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
            #expect(items.map(\.name) == ["title", "body"])
            #expect(items[0].value == Feedback.title(lang))
            #expect(items[1].value == Feedback.body(Self.env(agents: AgentCatalog.builtIn), lang))
        }
    }

    @Test func encodeUsesPercent20AndEscapesSeparators() {
        #expect(Feedback.encode("a b&c=d+e#f\n한") == "a%20b%26c%3Dd%2Be%23f%0A%ED%95%9C")
        #expect(Feedback.encode("Mac15,3 (build 1.2-3~x_y)") == "Mac15%2C3%20%28build%201.2-3~x_y%29")
    }

    @Test func emptyEmailHidesMailChoice() {
        #expect(Feedback.choices(email: "") == [.githubIssue])
        #expect(Feedback.mailURL(Self.env(), email: "") == nil)
        #expect(Feedback.emailHint(email: "", .en) == "An email option comes in a later version.")
        // 올바르지 않은 주소도 숨긴다 (주소에 쿼리를 끼워 넣을 수 없다)
        #expect(Feedback.choices(email: "a@b.com?cc=x@y.com") == [.githubIssue])
        #expect(Feedback.choices(email: "not an email") == [.githubIssue])
    }

    @Test func emailShowsMailChoice() throws {
        let email = "feedback@example.com"
        #expect(Feedback.choices(email: email) == [.githubIssue, .email])
        #expect(Feedback.emailHint(email: email, .en) == nil)
        let url = try #require(Feedback.mailURL(Self.env(), email: email, .en))
        #expect(url.absoluteString.hasPrefix("mailto:feedback@example.com?subject=%5BFeedback%5D%20&body=%23%23%23%20What%20happened%3F"))
        let items = try #require(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
        #expect(items.first { $0.name == "body" }?.value == Feedback.body(Self.env(), .en))
    }

    @Test func shippedContactIsEmptyOrValid() {
        #expect(FeedbackContact.email.isEmpty || Feedback.isValidEmail(FeedbackContact.email))
    }
}
