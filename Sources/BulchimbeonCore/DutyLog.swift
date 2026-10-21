#if canImport(Darwin)
import Darwin
#else
import Glibc
#endif
import Foundation

public struct DutyLogEntry: Equatable, Sendable {
    public enum Kind: Sendable { case onDuty, handover, standDown, heat, awol, other }

    public let date: Date
    public let text: String
    public var kind: Kind = .other

    public init(date: Date, text: String, kind: Kind = .other) {
        self.date = date
        self.text = text
        self.kind = kind
    }

    public var line: String {
        "\(DutyLogEntry.formatter.string(from: date))  \(text)"
    }

    static let formatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "MM-dd HH:mm"
        return f
    }()
}

/// 근무일지. 메모리에는 최근 `capacity` 건, 파일에는 append.
///
/// 보안:
/// - `O_NOFOLLOW` 로 열어 심볼릭 링크를 따라가지 않는다 (다른 파일 덮어쓰기 방지, TOCTOU 없음)
/// - 파일 0600 / 디렉토리 0700
/// - `maxFileBytes` 를 넘으면 `.1` 로 회전해 디스크를 무한히 쓰지 않는다
/// - 기록하는 문자열은 앱 내부 상수와 에이전트 카탈로그 이름뿐 (프로세스 인자 원문은 절대 기록하지 않음)
public final class DutyLog {
    public private(set) var recent: [DutyLogEntry] = []
    private let fileURL: URL?
    private let capacity: Int
    private let maxFileBytes: Int

    public init(fileURL: URL?, capacity: Int = 50, maxFileBytes: Int = 1_000_000) {
        self.fileURL = fileURL
        self.capacity = capacity
        self.maxFileBytes = maxFileBytes
    }

    public static var defaultFileURL: URL? {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first?
            .appendingPathComponent("Bulchimbeon", isDirectory: true)
            .appendingPathComponent("duty-log.txt")
    }

    public func write(_ text: String, kind: DutyLogEntry.Kind = .other, at date: Date = Date()) {
        let entry = DutyLogEntry(date: date, text: text, kind: kind)
        recent.insert(entry, at: 0) // 최신순
        if recent.count > capacity { recent.removeLast(recent.count - capacity) }
        append(entry)
    }

    private func append(_ entry: DutyLogEntry) {
        guard let fileURL else { return }
        let fm = FileManager.default
        try? fm.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true,
                                attributes: [.posixPermissions: 0o700])
        rotateIfNeeded(fileURL)

        let fd = open(fileURL.path, O_WRONLY | O_APPEND | O_CREAT | O_NOFOLLOW | O_CLOEXEC, 0o600)
        guard fd >= 0 else { return } // 심볼릭 링크면 ELOOP 로 실패 → 쓰지 않음
        let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
        let line = ISO8601DateFormatter().string(from: entry.date) + "  " + Self.singleLine(entry.text) + "\n"
        try? handle.write(contentsOf: Data(line.utf8))
    }

    private func rotateIfNeeded(_ url: URL) {
        var info = stat()
        guard lstat(url.path, &info) == 0, (info.st_mode & S_IFMT) == S_IFREG,
              Int(info.st_size) >= maxFileBytes else { return }
        let rotated = url.appendingPathExtension("1")
        unlink(rotated.path)
        rename(url.path, rotated.path)
    }

    /// 줄바꿈을 제거해 한 항목이 여러 줄로 위장되지 않게 한다 (로그 인젝션 방지)
    static func singleLine(_ text: String) -> String {
        text.components(separatedBy: .newlines).joined(separator: " ")
    }
}
