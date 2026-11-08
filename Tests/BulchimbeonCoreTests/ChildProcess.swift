import Foundation

/// 테스트가 띄우는 자식 프로세스. 끝나기를 스레드를 막지 않고, 기한 안에서만 기다린다.
///
/// 실측 (2026-10-31, PR #63 의 macOS CI): `swift test` 가 아무 자식도 남기지 않은 채 20분 동안 멈췄다.
/// 테스트 안에서 기다릴 수 있는 것은 자식 프로세스뿐이었고, 그 기다림(`waitUntilExit`, 파이프를 끝까지 읽기)에
/// 기한이 없었다. 이제 기한(기본 30초)이나 테스트 취소(`.timeLimit`)에 자식을 SIGKILL 로 끝내고 실패로 보고한다.
/// 종료 알림(`terminationHandler`)이 끝내 오지 않아도 잠깐의 유예 뒤에는 기다림을 스스로 끝낸다.
/// 출력은 파이프 대신 임시 파일로 받아, 읽기가 EOF 를 기다리며 멈출 일이 없다.
final class Child: @unchecked Sendable {
    struct TimedOut: Error, CustomStringConvertible {
        let command: String
        let timeout: Duration
        var description: String { "\(command) did not exit within \(timeout); killed" }
    }

    let process = Process()
    private let lock = NSLock()
    private var exited = false
    private var waiters: [CheckedContinuation<Void, Never>] = []

    /// 바로 실행한다. `output` 을 주면 표준 출력을 그 파일에 쓴다.
    init(_ path: String, _ arguments: [String], output: URL? = nil) throws {
        process.executableURL = URL(fileURLWithPath: path)
        process.arguments = arguments
        if let output {
            guard FileManager.default.createFile(atPath: output.path, contents: nil) else { throw CocoaError(.fileWriteUnknown) }
            process.standardOutput = try FileHandle(forWritingTo: output)
        }
        // 실행 전에 걸어야 아주 빨리 끝나는 자식도 놓치지 않는다
        process.terminationHandler = { [weak self] _ in self?.didExit() }
        try process.run()
    }

    var pid: Int32 { process.processIdentifier }
    var isRunning: Bool { locked { !exited } }
    /// `wait` 이 끝난 뒤에만 읽는다
    var status: Int32 { process.terminationStatus }

    /// 아직 살아 있으면 SIGKILL 을 보낸다 (리눅스 Foundation 은 자식의 SIGTERM 을 막아 둔 채 띄운다).
    /// 끝난 뒤에는 보내지 않아, 재사용된 PID 를 죽이지 않는다. 실제로 보냈으면 true.
    @discardableResult
    func forceKill() -> Bool {
        locked {
            guard !exited else { return false }
            kill(process.processIdentifier, SIGKILL)
            return true
        }
    }

    /// 끝날 때까지 기다린다. 기한을 넘기거나 테스트가 취소되면 SIGKILL 로 끝내고 던진다.
    func wait(timeout: Duration = .seconds(30)) async throws {
        let deadline = Task { [weak self] () -> Bool in
            try await Task.sleep(for: timeout)
            guard let self, self.forceKill() else { return false }
            await self.abandonAfterGrace()
            return true
        }
        await withTaskCancellationHandler {
            await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
                let done = locked {
                    if !exited { waiters.append(c) }
                    return exited
                }
                if done { c.resume() }
            }
        } onCancel: {
            if forceKill() { Task { await self.abandonAfterGrace() } }
        }
        deadline.cancel()
        if (try? await deadline.value) == true {
            throw TimedOut(command: ([process.executableURL?.path ?? "?"] + (process.arguments ?? []).prefix(1)).joined(separator: " "),
                           timeout: timeout)
        }
        try Task.checkCancellation()
    }

    /// 실행해 끝날 때까지 기다리고 종료 코드와 표준 출력을 돌려준다.
    static func run(_ path: String, _ arguments: [String], timeout: Duration = .seconds(30)) async throws -> (status: Int32, output: String) {
        let output = FileManager.default.temporaryDirectory.appendingPathComponent("bulchimbeon-child-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: output) }
        let child = try Child(path, arguments, output: output)
        try await child.wait(timeout: timeout)
        return (child.status, try String(contentsOf: output, encoding: .utf8))
    }

    /// macOS 13 대상에서 `NSLocking.withLock` 의 가용성에 기대지 않도록 직접 잠근다
    private func locked<T>(_ body: () -> T) -> T {
        lock.lock()
        defer { lock.unlock() }
        return body()
    }

    /// SIGKILL 뒤 종료 알림이 사라져도 테스트가 계속 기다리면 안 된다.
    /// 그래서 짧게 알림을 기다려 준 뒤, 오지 않았으면 끝난 것으로 치고 기다리는 쪽을 모두 깨운다.
    /// 늦게 온 알림은 `didExit` 이 이미 비워진 목록을 다시 비울 뿐이라 두 번 깨우지 않는다.
    private func abandonAfterGrace() async {
        try? await Task.sleep(for: .seconds(2))
        didExit()
    }

    private func didExit() {
        let ready = locked {
            exited = true
            defer { waiters = [] }
            return waiters
        }
        for c in ready { c.resume() }
    }
}
