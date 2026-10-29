import AppKit
import BulchimbeonCore
import CryptoKit
import Foundation
import Security

/// 새 버전 확인과 업데이트 설치. 언제 묻는지, 무엇을 권하는지, 새 번들을 믿어도 되는지는 Core 의
/// `UpdateCheck`·`UpdateOffer`·`UpdateInstaller` 가 정하고, 여기서는 요청·파일·서명 읽기만 한다.
///
/// 보내는 것은 `UpdateCheck.latestReleaseAPI` 에 대한 GET 하나뿐이다 (하루 한 번까지, `checkForUpdates` 로 끔).
/// 사용자가 메뉴에서 업데이트를 누르고 확인하기 전에는 아무것도 내려받거나 설치하지 않는다.
@MainActor
final class UpdateController: ObservableObject {
    static let shared = UpdateController()

    @Published var enabled: Bool {
        didSet {
            defaults.set(enabled, forKey: Keys.enabled)
            if enabled { Task { await checkIfDue() } }
        }
    }
    @Published private(set) var latest: ReleaseInfo?
    @Published private(set) var phase: UpdatePhase = .idle

    let current = AppVersion(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "")

    var offer: UpdateOffer? { UpdateOffer.decide(current: current, latest: latest, enabled: enabled) }

    private enum Keys {
        static let enabled = SettingsKey.checkForUpdates.rawValue
        static let lastCheck = SettingsKey.updateLastCheck.rawValue
        static let latestVersion = SettingsKey.updateLatestVersion.rawValue
        static let criticalBelow = SettingsKey.updateCriticalBelow.rawValue
    }

    private let defaults = UserDefaults.standard
    private var checking = false
    private var timer: Timer?

    private init() {
        enabled = defaults.object(forKey: Keys.enabled) as? Bool ?? SettingsValues.defaultCheckForUpdates
        // 재시작해도 하루 안에는 다시 묻지 않으므로, 마지막으로 알게 된 것을 보여준다
        if let version = defaults.string(forKey: Keys.latestVersion).flatMap(AppVersion.init) {
            latest = ReleaseInfo(version: version, criticalBelow: defaults.string(forKey: Keys.criticalBelow).flatMap(AppVersion.init))
        }
        guard !CommandLine.arguments.contains("--demo") else { return } // 녹화용 연출 중에는 묻지 않는다
        timer = Timer.scheduledTimer(withTimeInterval: 3600, repeats: true) { _ in
            Task { @MainActor in await UpdateController.shared.checkIfDue() }
        }
        Task { await checkIfDue() }
    }

    // MARK: - 확인 (하루 한 번까지)

    func checkIfDue() async {
        let lastCheck = (defaults.object(forKey: Keys.lastCheck) as? Double).map { Date(timeIntervalSince1970: $0) }
        guard !checking, let current, UpdateCheck.isDue(enabled: enabled, current: current, now: Date(), lastCheck: lastCheck) else { return }
        checking = true
        defer { checking = false }
        // 실패해도 오늘의 한 번으로 친다
        defaults.set(Date().timeIntervalSince1970, forKey: Keys.lastCheck)
        let data = try? await UpdateFiles.download(UpdateCheck.latestReleaseAPI, headers: UpdateCheck.requestHeaders(current: current),
                                                   limit: UpdateCheck.maxResponseBytes)
        guard let data, let release = ReleaseInfo.parse(data) else { return }
        latest = release
        defaults.set(release.version.description, forKey: Keys.latestVersion)
        if let below = release.criticalBelow {
            defaults.set(below.description, forKey: Keys.criticalBelow)
        } else {
            defaults.removeObject(forKey: Keys.criticalBelow)
        }
    }

    func openReleaseNotes() {
        NSWorkspace.shared.open(offer?.releaseNotesURL ?? ReleaseURLs.latestPage)
    }

    // MARK: - 설치 (메뉴에서 누르고 확인했을 때만)

    func install() async {
        guard let offer, !phase.isBusy else { return }
        let method = UpdateInstaller.method { path in
            var isDirectory: ObjCBool = false
            return FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory) && isDirectory.boolValue
        }
        guard confirm(offer, method: method) else { return }
        switch method {
        case .homebrew(let brew): openHomebrewUpgrade(brew: brew)
        case .direct: await installDirectly(offer)
        }
    }

    private func confirm(_ offer: UpdateOffer, method: UpdateInstaller.Method) -> Bool {
        let alert = NSAlert()
        alert.messageText = offer.menuTitle()
        switch method {
        case .homebrew:
            alert.informativeText = L10n.t(
                "Homebrew 로 설치했으므로 터미널에서 brew upgrade --cask bulchimbeon 을 실행합니다. 진행 과정이 터미널에 그대로 보이고, 끝나면 불침번이 다시 열립니다.",
                "Bulchimbeon was installed with Homebrew, so Terminal will run brew upgrade --cask bulchimbeon. Every step shows in Terminal, and Bulchimbeon reopens when it is done.",
                "Bulchimbeon 是通过 Homebrew 安装的，因此将在“终端”中运行 brew upgrade --cask bulchimbeon。每一步都会显示在终端中，完成后 Bulchimbeon 会重新打开。")
        case .direct:
            alert.informativeText = L10n.t(
                "릴리스 zip 을 내려받아 체크섬과 코드 서명을 확인한 뒤 앱을 바꾸고 다시 시작합니다. 하던 근무는 다시 시작한 뒤 이어집니다 (덮개 근무는 시스템 근무로 이어지고 다음 근무에서 다시 승인을 받습니다).",
                "Bulchimbeon will download the release zip, check its checksum and code signature, replace the app and restart. The current watch carries on after the restart (a lid-closed watch comes back as a system watch and asks for approval again next time).",
                "Bulchimbeon 将下载发行版 zip，验证校验和与代码签名，然后替换应用并重新启动。当前值班会在重启后继续（合盖值班会以系统值班继续，下次值班时再次请求授权）。")
        }
        alert.addButton(withTitle: L10n.t("업데이트", "Update", "更新"))
        alert.addButton(withTitle: L10n.t("취소", "Cancel", "取消"))
        NSApp.activate(ignoringOtherApps: true)
        return alert.runModal() == .alertFirstButtonReturn
    }

    /// Homebrew 설치: 앱이 스스로 바꾸지 않고, 상수로 된 `.command` 스크립트를 터미널로 연다 (숨은 셸 없음).
    private func openHomebrewUpgrade(brew: String) {
        guard let current,
              let script = UpdateInstaller.homebrewScript(brew: brew, pid: ProcessInfo.processInfo.processIdentifier, running: current)
        else { return fail(.terminal) }
        let fm = FileManager.default
        let dir = fm.temporaryDirectory.appendingPathComponent("Bulchimbeon-update-\(UUID().uuidString)", isDirectory: true)
        let file = dir.appendingPathComponent("Bulchimbeon-update.command")
        do {
            try fm.createDirectory(at: dir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        } catch {
            return fail(.terminal)
        }
        guard fm.createFile(atPath: file.path, contents: Data(script.utf8), attributes: [.posixPermissions: 0o700]) else {
            return fail(.terminal)
        }
        let terminal = URL(fileURLWithPath: "/System/Applications/Utilities/Terminal.app")
        NSWorkspace.shared.open([file], withApplicationAt: terminal, configuration: NSWorkspace.OpenConfiguration()) { _, error in
            guard error != nil else { return }
            Task { @MainActor in UpdateController.shared.fail(.terminal) }
        }
    }

    /// 직접 설치: zip 과 체크섬을 받아 맞춰 보고, 새 번들의 서명이 지금 앱과 같은지 확인한 뒤에만 바꾸고 다시 시작한다.
    /// 어디서 실패하든 설치된 앱은 그대로다.
    private func installDirectly(_ offer: UpdateOffer) async {
        let fm = FileManager.default
        let appURL = Bundle.main.bundleURL
        let writable = fm.isWritableFile(atPath: appURL.deletingLastPathComponent().path) && fm.isWritableFile(atPath: appURL.path)
        if let problem = UpdateInstaller.locationProblem(bundlePath: appURL.path, writable: writable) { return fail(problem) }
        guard let running = UpdateFiles.signature(of: appURL) else { return fail(.signatureInvalid) }

        // 같은 볼륨의 임시 폴더: 교체가 이름 바꾸기 한 번으로 끝난다
        let work: URL
        do {
            work = try fm.url(for: .itemReplacementDirectory, in: .userDomainMask, appropriateFor: appURL, create: true)
        } catch {
            return fail(.replace)
        }
        defer { try? fm.removeItem(at: work) }

        phase = .downloading(nil)
        let headers = current.map(UpdateCheck.requestHeaders(current:)) ?? [:]
        let zipName = ReleaseURLs.macZipName(offer.version)
        let checksumFile: Data
        let zip: Data
        do {
            checksumFile = try await UpdateFiles.download(ReleaseURLs.macChecksum(offer.version), headers: headers, limit: 4096)
            zip = try await UpdateFiles.download(ReleaseURLs.macZip(offer.version), headers: headers, limit: UpdateFiles.maxZipBytes) { fraction in
                Task { @MainActor in
                    if UpdateController.shared.phase.isBusy { UpdateController.shared.phase = .downloading(fraction) }
                }
            }
        } catch {
            return fail(.download)
        }

        phase = .verifying
        guard let expected = String(data: checksumFile, encoding: .utf8)
            .flatMap({ UpdateInstaller.expectedSHA256(checksumFile: $0, fileName: zipName) })
        else { return fail(.checksumMissing) }
        guard UpdateInstaller.checksumMatches(expected: expected, actual: Array(SHA256.hash(data: zip))) else {
            return fail(.checksumMismatch)
        }
        let zipURL = work.appendingPathComponent(zipName)
        let unpacked = work.appendingPathComponent("unpacked", isDirectory: true)
        do {
            try zip.write(to: zipURL)
            try fm.createDirectory(at: unpacked, withIntermediateDirectories: false)
        } catch {
            return fail(.unzip)
        }
        guard await UpdateFiles.unzip(zipURL, into: unpacked) else { return fail(.unzip) }
        let entries = ((try? fm.contentsOfDirectory(atPath: unpacked.path)) ?? []).filter { !$0.hasPrefix(".") && $0 != "__MACOSX" }
        let newApp = unpacked.appendingPathComponent("Bulchimbeon.app", isDirectory: true)
        guard entries == ["Bulchimbeon.app"] else { return fail(.unzip) }

        let info = NSDictionary(contentsOf: newApp.appendingPathComponent("Contents/Info.plist"))
        let candidate = UpdateInstaller.Candidate(
            signature: UpdateFiles.signature(of: newApp)
                ?? UpdateInstaller.Signature(valid: false, identifier: nil, teamIdentifier: nil, hardenedRuntime: false),
            bundleIdentifier: info?["CFBundleIdentifier"] as? String,
            shortVersion: info?["CFBundleShortVersionString"] as? String)
        if let failure = UpdateInstaller.verify(candidate, running: running, expected: offer.version) { return fail(failure) }

        phase = .installing
        do {
            // 새 번들의 속성만 쓴다 (예전 번들의 확장 속성을 옮겨 붙이지 않음)
            _ = try fm.replaceItemAt(appURL, withItemAt: newApp, backupItemName: nil, options: [.usingNewMetadataOnly])
        } catch {
            return fail(.replace)
        }
        DutyController.shared.relaunch()
    }

    private func fail(_ failure: UpdateInstaller.Failure) {
        phase = .failed(failure)
        let message = failure.message()
        let reason = message.prefix(1).uppercased() + String(message.dropFirst())
        let unchanged = L10n.t("설치된 앱은 그대로입니다. 릴리스 페이지에서 직접 받을 수도 있습니다.",
                               "The installed app is unchanged. You can also download the new version from the release page.",
                               "已安装的应用未更改。你也可以从发行页面下载新版本。")
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = L10n.t("업데이트하지 못했습니다", "Could not update", "无法更新")
        alert.informativeText = reason + "\n\n" + unchanged
        alert.addButton(withTitle: L10n.t("릴리스 페이지 열기", "Open release page", "打开发行页面"))
        alert.addButton(withTitle: L10n.t("닫기", "Close", "关闭"))
        NSApp.activate(ignoringOtherApps: true)
        if alert.runModal() == .alertFirstButtonReturn { openReleaseNotes() }
    }
}

/// 업데이트의 시스템 쪽 일: HTTPS 요청, 압축 풀기, 코드 서명 읽기. 판단은 하지 않는다.
enum UpdateFiles {
    /// 릴리스 zip 크기 상한
    static let maxZipBytes = 64 << 20

    /// 쿠키·캐시·자격 증명을 쓰지 않는 세션
    static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.httpCookieStorage = nil
        config.httpShouldSetCookies = false
        config.urlCredentialStorage = nil
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.timeoutIntervalForRequest = 30
        config.timeoutIntervalForResource = 300
        return URLSession(configuration: config)
    }()

    /// HTTPS GET 하나. 머리글은 Core 가 준 것만 (`UpdateCheck.requestHeaders`). 크기가 limit 를 넘으면 그만둔다.
    static func download(_ url: URL, headers: [String: String], limit: Int,
                         progress: (@Sendable (Double) -> Void)? = nil) async throws -> Data {
        guard url.scheme == "https" else { throw URLError(.unsupportedURL) }
        var request = URLRequest(url: url)
        for (name, value) in headers { request.setValue(value, forHTTPHeaderField: name) }
        let (bytes, response) = try await session.bytes(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200, http.url?.scheme == "https" else {
            throw URLError(.badServerResponse)
        }
        let expected = response.expectedContentLength
        guard expected <= Int64(limit) else { throw URLError(.dataLengthExceedsMaximum) }
        var data = Data()
        if expected > 0 { data.reserveCapacity(Int(expected)) }
        for try await byte in bytes {
            data.append(byte)
            guard data.count <= limit else { throw URLError(.dataLengthExceedsMaximum) }
            if let progress, expected > 0, data.count % (256 << 10) == 0 { progress(Double(data.count) / Double(expected)) }
        }
        return data
    }

    /// `/usr/bin/ditto -x -k` (절대 경로, 최소 환경, 셸 없음)
    static func unzip(_ zip: URL, into dir: URL) async -> Bool {
        await Task.detached { () -> Bool in
            let process = Process()
            process.executableURL = URL(fileURLWithPath: "/usr/bin/ditto")
            process.arguments = ["-x", "-k", zip.path, dir.path]
            process.environment = ["PATH": "/usr/bin:/bin", "LANG": "C"]
            process.standardOutput = FileHandle.nullDevice
            process.standardError = FileHandle.nullDevice
            do { try process.run() } catch { return false }
            process.waitUntilExit()
            return process.terminationStatus == 0
        }.value
    }

    /// 코드 서명 (Security 프레임워크): 엄격 검증 (중첩 코드·모든 아키텍처), 서명 식별자, 팀 ID, hardened runtime.
    static func signature(of url: URL) -> UpdateInstaller.Signature? {
        var code: SecStaticCode?
        guard SecStaticCodeCreateWithPath(url as CFURL, [], &code) == errSecSuccess, let code else { return nil }
        let strict = SecCSFlags(rawValue: UInt32(kSecCSStrictValidate | kSecCSCheckAllArchitectures | kSecCSCheckNestedCode))
        let valid = SecStaticCodeCheckValidity(code, strict, nil) == errSecSuccess
        var info: CFDictionary?
        guard SecCodeCopySigningInformation(code, SecCSFlags(rawValue: UInt32(kSecCSSigningInformation)), &info) == errSecSuccess,
              let dict = info as NSDictionary? as? [String: Any]
        else { return nil }
        let flags = (dict[kSecCodeInfoFlags as String] as? NSNumber)?.uint32Value ?? 0
        return UpdateInstaller.Signature(valid: valid,
                                         identifier: dict[kSecCodeInfoIdentifier as String] as? String,
                                         teamIdentifier: dict[kSecCodeInfoTeamIdentifier as String] as? String,
                                         hardenedRuntime: flags & 0x10000 != 0) // kSecCodeSignatureRuntime
    }
}
