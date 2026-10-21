import Foundation

/// PID 재사용을 구별하는 프로세스 식별자.
///
/// `proc_bsdinfo`의 시작 시각까지 함께 써서, 종료된 프로세스의 argv가 같은 PID를
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

/// 살아 있는 같은-UID 프로세스의 argv를 점검 사이에만 보관하는 작은 메모리 캐시.
///
/// 값은 디스크·로그·알림으로 절대 나가지 않으며, 다음 스캔에서 사라진 PID의 값은 즉시 버린다.
public struct ProcessArgumentCache {
    private struct Key: Hashable {
        let identity: ProcessIdentity
        let limit: Int
    }

    private var values: [Key: [String]] = [:]

    public init() {}

    /// `limit`은 수집 최소화 규칙의 일부이므로 캐시 키에도 포함한다.
    /// 읽기가 실패해 빈 배열이면 저장하지 않아 다음 점검에서 재시도한다.
    public mutating func arguments(for identity: ProcessIdentity, limit: Int,
                                   read: () -> [String]) -> [String] {
        let key = Key(identity: identity, limit: limit)
        if let cached = values[key] { return cached }
        let arguments = read()
        if !arguments.isEmpty { values[key] = arguments }
        return arguments
    }

    /// 현재 스캔에 없는 프로세스의 argv를 즉시 잊는다.
    public mutating func retain(only liveProcesses: Set<ProcessIdentity>) {
        values = values.filter { liveProcesses.contains($0.key.identity) }
    }
}
