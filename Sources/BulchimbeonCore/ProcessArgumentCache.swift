import Foundation

/// PID 재사용을 구별하는 프로세스 식별자.
///
/// `proc_bsdinfo`의 시작 시각까지 함께 써서, 종료된 프로세스의 판별 결과가 같은 PID를
/// 나중에 받은 다른 프로세스에 쓰이지 않게 한다.
public struct ProcessIdentity: Hashable, Sendable {
    public let pid: Int32
    public let startedAtSeconds: UInt64
    public let startedAtMicroseconds: UInt64

    public init(pid: Int32, startedAtSeconds: UInt64, startedAtMicroseconds: UInt64) {
        self.pid = pid
        self.startedAtSeconds = startedAtSeconds
        self.startedAtMicroseconds = startedAtMicroseconds
    }
}

/// argv가 판별 결과를 바꾸지 않는 프로세스를 기억해, 다음 점검부터 argv를 다시 읽지 않게 한다.
///
/// argv 자체는 저장하지 않는다. 남는 것은 PID·시작 시각과 실행 파일 경로뿐이며 (exec 로 경로가 바뀌면 다시 읽는다), 사라진 프로세스는 다음 스캔에서 지운다.
public struct ProcessArgumentCache {
    private var kinds: [AgentKind] = []
    private var argumentFree: [ProcessIdentity: String] = [:]

    public init() {}

    /// argv가 판별에 영향이 없다고 이미 확인된 프로세스면 `read` 없이 빈 배열을 돌려준다.
    /// 그렇지 않으면 읽고, 빈 argv로 판별해도 결과가 같을 때만 그 사실을 기억한다.
    /// 읽기가 실패해 빈 배열이면 기억하지 않아 다음 점검에서 재시도한다.
    public mutating func arguments(for identity: ProcessIdentity, executablePath: String, kinds: [AgentKind],
                                   read: () -> [String]) -> [String] {
        if kinds != self.kinds {
            self.kinds = kinds
            argumentFree = [:]
        }
        if argumentFree[identity] == executablePath { return [] }
        let arguments = read()
        guard !arguments.isEmpty else { return arguments }
        let withArguments = AgentCatalog.match(ProcessSnapshot(executablePath: executablePath, arguments: arguments), kinds: kinds)
        let withoutArguments = AgentCatalog.match(ProcessSnapshot(executablePath: executablePath, arguments: []), kinds: kinds)
        if withArguments?.kind == withoutArguments?.kind, withArguments?.reason == withoutArguments?.reason {
            argumentFree[identity] = executablePath
        }
        return arguments
    }

    /// 현재 스캔에 없는 프로세스를 즉시 잊는다.
    public mutating func retain(only liveProcesses: Set<ProcessIdentity>) {
        argumentFree = argumentFree.filter { liveProcesses.contains($0.key) }
    }
}
