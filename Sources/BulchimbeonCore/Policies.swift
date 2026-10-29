import Foundation

public struct BatteryStatus: Equatable, Sendable {
    public let percent: Int
    public let onBattery: Bool
    public init(percent: Int, onBattery: Bool) {
        self.percent = percent
        self.onBattery = onBattery
    }
}

public enum BatteryPolicy {
    /// 배터리 사용 중이고 잔량이 기준 이하이면 철수. floor 가 nil 이면 철수하지 않는다.
    public static func shouldRetreat(_ status: BatteryStatus?, floor: Int?) -> Bool {
        guard let status, let floor, status.onBattery else { return false }
        return status.percent <= floor
    }
}

/// 에이전트 근무 모드의 교대 판단.
///
/// - 투입: 지금 실제로 일하는 에이전트가 있을 때만
/// - 교대: 근무 중인데 `quietFor` 가 유예 시간 이상일 때만
///   (`quietFor` = 지금 − max(마지막으로 일하는 걸 본 시각, 에이전트 근무 시작 시각))
/// 실측: Claude Code 는 `caffeinate -t 300` 을 주기적으로 다시 띄워서 그 사이 짧은 공백이 있다.
/// 공백 한 번에 교대했다가 바로 다시 투입하는 깜빡임을 막는다.
public enum AgentWatchPolicy {
    public enum Action: Equatable { case engage, handover, none }

    public static func decide(busyNow: Bool, quietFor: TimeInterval, grace: TimeInterval,
                              onDuty: Bool, lowBattery: Bool) -> Action {
        if busyNow { return !onDuty && !lowBattery ? .engage : .none }
        return onDuty && quietFor >= grace ? .handover : .none
    }

    public static func quietFor(now: Date, lastBusyAt: Date?, watchStartedAt: Date) -> TimeInterval {
        now.timeIntervalSince(max(lastBusyAt ?? watchStartedAt, watchStartedAt))
    }
}

public enum PmsetParser {
    /// `pmset -g` 출력의 `SleepDisabled` 값. 항목이 없으면 false.
    public static func sleepDisabled(_ output: String) -> Bool {
        for line in output.split(separator: "\n") {
            let parts = line.split(whereSeparator: \.isWhitespace)
            if parts.count >= 2, parts[0] == "SleepDisabled" {
                return parts[1] == "1"
            }
        }
        return false
    }
}

public enum DutyClock {
    /// 경과/남은 시간 표시: 59초 → "0:59", 1시간 5분 3초 → "1:05:03"
    public static func format(_ interval: TimeInterval) -> String {
        let total = max(0, Int(interval.rounded(.down)))
        let h = total / 3600, m = (total % 3600) / 60, s = total % 60
        return h > 0
            ? String(format: "%d:%02d:%02d", h, m, s)
            : String(format: "%d:%02d", m, s)
    }
}

/// 앱 재시작 시 저장된 근무 명령을 이어갈지 판단한다.
public enum DutyResume {
    public enum Action: Equatable {
        case none
        case agentWatch
        case timed(until: Date?) // nil = 무기한
        case expired             // 앱이 꺼져 있는 동안 근무 시간이 끝남
    }

    /// 근무 명령으로 허용하는 최대 남은 시간. 이보다 길면 저장값 조작으로 보고 무시한다.
    static let maxRemaining: TimeInterval = 24 * 3600

    public static func decide(stored: String?, until: Double?, now: Date) -> Action {
        switch stored {
        case "agentWatch":
            return .agentWatch
        case "timed":
            guard let until else { return .timed(until: nil) }
            guard until.isFinite else { return .none }
            let end = Date(timeIntervalSince1970: until)
            if end <= now { return .expired }
            return end.timeIntervalSince(now) <= maxRemaining ? .timed(until: end) : .none
        default:
            return .none
        }
    }
}

/// "근무 서기" 메뉴에서 어느 시간 항목에 체크할지 정한다.
public enum TimedMenu {
    /// seconds: 항목의 시간(nil = 무기한). picked: 이번 실행에서 마지막으로 고른 시간.
    /// 재시작으로 이어진 시간제 근무는 고른 항목을 알 수 없어 체크하지 않는다.
    public static func isChecked(_ seconds: TimeInterval?, order: DutyOrder, picked: TimeInterval?) -> Bool {
        guard case .timed(let until) = order else { return false }
        return until == nil ? seconds == nil : seconds != nil && seconds == picked
    }
}

/// 맥 발열 단계. 군대 온도지수 단계에 빗댄다.
public enum HeatLevel: Int, Comparable, Sendable {
    case normal = 0  // 정상
    case caution     // 관심 — 조치 없음
    case advisory    // 폭염 주의보 — 경고만
    case alert       // 폭염 경보 — 전투휴식 (근무 중단)

    #if canImport(Darwin)
    public init(_ state: ProcessInfo.ThermalState) {
        switch state {
        case .nominal: self = .normal
        case .fair: self = .caution
        case .serious: self = .advisory
        case .critical: self = .alert
        @unknown default: self = .alert // 모르는 단계는 안전한 쪽으로
        }
    }
    #endif

    /// 디버그용 문자열 (`defaults write ... simulateHeat advisory`)
    public init?(name: String) {
        switch name {
        case "normal": self = .normal
        case "caution": self = .caution
        case "advisory": self = .advisory
        case "alert": self = .alert
        default: return nil
        }
    }

    public static func < (lhs: HeatLevel, rhs: HeatLevel) -> Bool { lhs.rawValue < rhs.rawValue }
}

/// 폭염 대응 판단.
public enum HeatPolicy {
    public enum Action: Equatable {
        case none
        case advisory   // 주의보 발령 (경고만)
        case rest       // 경보 → 전투휴식 시작
        case resume     // 충분히 식음 → 근무 복귀
    }

    /// 열이 내린 뒤 이 시간만큼 유지돼야 복귀 (오락가락 방지)
    public static let coolDown: TimeInterval = 120

    public static func decide(level: HeatLevel, onDuty: Bool, resting: Bool, advised: Bool,
                              coolSince: Date?, now: Date) -> Action {
        if resting {
            guard level <= .caution, let coolSince else { return .none }
            return now.timeIntervalSince(coolSince) >= coolDown ? .resume : .none
        }
        guard onDuty else { return .none }
        if level >= .alert { return .rest }
        if level == .advisory && !advised { return .advisory }
        return .none
    }
}

/// 덮개 모드 관리자 명령. 가변 값은 정수 PID와 셸 인용한 사용자 플래그 경로뿐이다.
///
/// 켤 때 "당직 감시병"을 함께 띄운다: 앱(PID)이 사라지거나 정지 플래그가 생기면 5초 안에 `disablesleep 0` 으로 되돌리는
/// root 백그라운드 셸. 스크립트는 파일이 아닌 인라인 문자열 — 사용자 권한으로 수정 가능한 파일을
/// root 로 실행하면 권한 상승 통로가 되기 때문.
public enum LidCommands {
    /// 감시병 표식 `bulchimbeon-watchdog-<pid>` 를 찾는 패턴. 자기 자신(이 명령을 실행하는 셸)과 매칭되지 않도록 `[g]` 정규식 트릭을 쓴다.
    static func markerPattern(_ pid: Int32) -> String { "bulchimbeon-watchdo[g]-\(pid)" }

    /// 명령 원문에 쓰는 표식 셸 단어. 셸이 이어 붙여 감시병 인자로는 `bulchimbeon-watchdog-<pid>` 가 되지만, 원문에는 그 글자가 없다.
    /// 원문은 osascript 인자에도 들어가는데, macOS 10.15+ 는 관리자 명령을 launchd 의 authtrampoline 아래에서 돌려
    /// osascript 가 pkill 의 조상이 아니다. 원문에 표식이 있으면 켤 때의 pkill 이 승인 중인 osascript 를 죽인다 (0.10.0~0.10.1).
    static func markerWord(_ pid: Int32) -> String { "'bulchimbeon-watchdo''g-\(pid)'" }

    /// 실측(이슈 #4, 실제 관리자 권한):
    /// 1. `do shell script ... with administrator privileges` 는 끝날 때 같은 프로세스 그룹을 정리해서
    ///    nohup 으로 띄운 감시병이 함께 죽었다 (일반 권한에서는 살아남음) → setsid() 로 새 세션 분리.
    /// 2. `A && B &` 는 셸이 묶음 전체를 하위 셸로 띄워, 그 하위 셸이 osascript 출력 통로를 쥔 채
    ///    감시병을 기다렸다 → osascript 가 끝나지 않아 앱이 결과를 못 받음.
    /// 그래서 perl 을 포그라운드로 실행하고, perl 이 자식을 fork → 자식이 setsid() 를 마친 것을 파이프로
    /// 확인한 뒤에만 부모가 종료한다 (분리 전에 정리당하는 경쟁 상태 없음). 표준 입출력은 /dev/null.
    /// 시스템 기본 perl(SIP 보호 /usr/bin)만 쓴다 — 앱 바이너리를 root 로 실행하지 않는다
    /// (사용자 권한으로 수정 가능한 파일이라 권한 상승 통로가 된다).
    /// 정지 플래그는 감시병이 내용물을 읽지 않고 `stat`으로 존재만 확인한다. BSD `stat`은
    /// `-L` 없이 lstat을 사용하므로 심볼릭 링크를 따라 다른 경로를 보지 않는다.
    /// 근무 시간은 앱이 관리하고, 근무가 끝나면 앱이 플래그를 만든다. 같은 앱 PID로 다시 켜면 먼저
    /// 기존 감시병을 종료하고 하나만 새로 띄워 중첩하지 않는다.
    /// 감시병의 표식은 `$0`(실행 중 인자)에만 있고 명령 원문에는 없다 (`markerWord`).
    public static func enable(pid: Int32, stopSignal: String, pmset: String = "/usr/bin/pmset") -> String {
        let watch = "while :; do if /usr/bin/stat -f %z \"$1\" >/dev/null 2>&1 || ! /bin/kill -0 \(pid) 2>/dev/null; then \(pmset) -a disablesleep 0; exit 0; fi; /bin/sleep 5; done"
        let detach = "pipe(my $r, my $w); my $p = fork(); die unless defined $p; "
            + "if ($p) { close $w; <$r>; exit 0 } close $r; POSIX::setsid(); close $w; exec @ARGV"
        return "\(pmset) -a disablesleep 1 && { /usr/bin/pkill -f '\(markerPattern(pid))' || true; /usr/bin/perl -MPOSIX -e '\(detach)' /bin/sh -c '\(watch)' \(markerWord(pid)) \(shellQuote(stopSignal)) </dev/null >/dev/null 2>&1; }"
    }

    public static func disable(pid: Int32, pmset: String = "/usr/bin/pmset") -> String {
        "\(pmset) -a disablesleep 0; /usr/bin/pkill -f '\(markerPattern(pid))'; true"
    }

    /// 감시병이 살아 있는지 확인할 때 쓰는 `pgrep -f` 패턴
    public static func watchdogPattern(pid: Int32) -> String { markerPattern(pid) }

    private static func shellQuote(_ string: String) -> String {
        "'" + string.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}

/// 프로세스별 CPU 사용률을 최근 `window` 초 평균으로 계산한다.
///
/// 실측: 5초 순간값으로 보면 15초마다 잠깐 도는 감시 스크립트·개발 서버 폴링이 2~6% 로 튀어
/// "작업 중"이 계속 갱신되고 교대가 영원히 일어나지 않았다. 60초 평균이면 이런 주기적 잡음은 1% 안팎으로 묻힌다.
public struct CPUHistory: Sendable {
    public typealias Sample = (times: [Int32: UInt64], at: Date)

    public let window: TimeInterval
    private var samples: [Sample] = []

    public init(window: TimeInterval = 60) { self.window = window }

    /// 새 샘플을 넣고, 창 크기에 가장 가까운 과거 샘플과 비교한 평균 CPU(%)를 돌려준다.
    public mutating func add(_ times: [Int32: UInt64], at now: Date) -> [Int32: Double] {
        samples.removeAll { now.timeIntervalSince($0.at) > window * 1.5 }
        let base = samples.last { now.timeIntervalSince($0.at) >= window } ?? samples.first
        samples.append((times, now))
        guard let base else { return [:] }
        let wall = now.timeIntervalSince(base.at) * 1_000_000_000
        guard wall > 0 else { return [:] }
        var result: [Int32: Double] = [:]
        for (pid, time) in times {
            guard let before = base.times[pid], time >= before else { continue }
            result[pid] = Double(time - before) / wall * 100
        }
        return result
    }
}
