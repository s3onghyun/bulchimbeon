import Foundation

/// 감시 대상 AI 에이전트 한 종류.
public struct AgentKind: Hashable, Sendable {
    public let id: String
    public let displayName: String
    /// 실행 파일·argv[0]·스크립트의 basename (대소문자 구분)
    let binaryNames: Set<String>
    /// 실행 파일 경로에 포함되는 설치 위치 마커 (예: 네이티브 설치본은 실행 파일 이름이 버전 번호)
    let pathMarkers: [String]
    /// node/bun/deno 로 실행될 때 스크립트 경로에 포함되는 패키지 마커
    let packageMarkers: [String]
}

/// 판별에 쓰는 프로세스 정보. 같은 사용자(UID)의 프로세스만 수집한다.
public struct ProcessSnapshot: Sendable {
    public let pid: Int32
    public let ppid: Int32
    /// 실제 실행 파일 경로 (proc_pidpath)
    public let executablePath: String
    /// 작업 폴더 (에이전트 후보만 수집 — 진단 출력용)
    public let cwd: String?
    /// argv. 에이전트 후보·스크립트 러너만 앞부분 전체, 그 외엔 argv[0] 만 담긴다.
    /// argv 가 판별을 바꾸지 않는다고 이미 확인된 프로세스는 비어 있다 (`ProcessArgumentCache`).
    public let arguments: [String]
    /// 프로세스 시작 시각. 모르면 nil (그러면 방금 시작한 프로세스처럼 다룬다)
    public let startedAt: Date?

    public init(pid: Int32 = 0, ppid: Int32 = 0, executablePath: String, arguments: [String], cwd: String? = nil,
                startedAt: Date? = nil) {
        self.pid = pid
        self.ppid = ppid
        self.cwd = cwd
        self.executablePath = executablePath
        self.arguments = arguments
        self.startedAt = startedAt
    }
}

public enum AgentCatalog {
    /// 판별 근거 (진단 출력용)
    public enum Reason: Sendable {
        case executableName, argv0, installPath, script

        public var label: String {
            switch self {
            case .executableName: L10n.t("실행 파일 이름", "executable name", "可执行文件名")
            case .argv0: L10n.t("실행 명령(argv[0])", "command (argv[0])", "命令 (argv[0])")
            case .installPath: L10n.t("설치 경로", "install path", "安装路径")
            case .script: L10n.t("스크립트 경로", "script path", "脚本路径")
            }
        }
    }

    public static let builtIn: [AgentKind] = [
        AgentKind(id: "claude-code", displayName: "Claude Code",
                  binaryNames: ["claude"],
                  pathMarkers: ["/.local/share/claude/versions/"], // 네이티브 설치: 실행 파일명이 "2.1.296" 같은 버전
                  packageMarkers: ["@anthropic-ai/claude-code"]),
        AgentKind(id: "codex", displayName: "Codex",
                  binaryNames: ["codex"], pathMarkers: [], packageMarkers: ["@openai/codex"]),
        AgentKind(id: "gemini-cli", displayName: "Gemini CLI",
                  binaryNames: ["gemini"], pathMarkers: [], packageMarkers: ["@google/gemini-cli"]),
        AgentKind(id: "cursor-agent", displayName: "Cursor Agent",
                  binaryNames: ["cursor-agent"], pathMarkers: [], packageMarkers: []),
        AgentKind(id: "copilot-cli", displayName: "Copilot CLI",
                  binaryNames: ["copilot"], pathMarkers: [], packageMarkers: ["@github/copilot"]),
        AgentKind(id: "aider", displayName: "Aider",
                  binaryNames: ["aider"], pathMarkers: [], packageMarkers: []),
        AgentKind(id: "opencode", displayName: "OpenCode",
                  binaryNames: ["opencode"], pathMarkers: [], packageMarkers: ["opencode-ai"]),
    ]

    private static let scriptRunners: Set<String> = ["node", "bun", "deno"]
    private static let inlineEvalFlags: Set<String> = ["-e", "--eval", "-p", "--print"]

    /// 러너일 때만 argv 전체가 필요하다. 그 외 프로세스는 argv[0] 만 읽는다 (최소 수집).
    public static func needsFullArguments(executablePath: String) -> Bool {
        scriptRunners.contains(basename(executablePath))
    }

    /// 실행 파일·argv[0] 로 이미 에이전트 후보이면 도우미 세션 판별을 위해 인자를 더 읽는다.
    public static func isCandidate(executablePath: String, argv0: String?, kinds: [AgentKind] = builtIn) -> Bool {
        let exe = basename(executablePath), a0 = argv0.map(basename)
        return kinds.contains { k in
            k.binaryNames.contains(exe) || (a0.map(k.binaryNames.contains) ?? false)
                || k.pathMarkers.contains(where: executablePath.contains)
        }
    }

    /// 도구가 하나도 없는 헤드리스 세션(`--tools=`)은 에이전트가 아니다.
    /// 실측: claude-mem 플러그인이 메모리 정리용으로 상시 띄워 두는 세션이 이 모양이다.
    static func isHelperSession(_ arguments: [String]) -> Bool {
        arguments.dropFirst().contains("--tools=")
    }

    /// 프로세스 하나를 에이전트로 판별한다.
    /// 부분 문자열 매칭은 하지 않는다 — hook 스크립트(`node -e "...'.claude'..."`)가 오탐되기 때문.
    public static func match(_ process: ProcessSnapshot, kinds: [AgentKind] = builtIn) -> (kind: AgentKind, reason: Reason)? {
        if isHelperSession(process.arguments) { return nil }
        let exeBase = basename(process.executablePath)
        let argv0Base = process.arguments.first.map(basename)

        for kind in kinds {
            if kind.binaryNames.contains(exeBase) { return (kind, .executableName) }
            if let argv0Base, kind.binaryNames.contains(argv0Base) { return (kind, .argv0) }
            if kind.pathMarkers.contains(where: process.executablePath.contains) { return (kind, .installPath) }
        }

        guard scriptRunners.contains(exeBase), process.arguments.count > 1,
              !inlineEvalFlags.contains(process.arguments[1]),
              let script = process.arguments.dropFirst().first(where: { !$0.hasPrefix("-") })
        else { return nil }

        // npm 전역 설치는 `node /opt/homebrew/bin/claude` 처럼 bin 링크 이름으로 실행된다
        let scriptBase = basename(script)
        for kind in kinds where kind.binaryNames.contains(scriptBase)
            || kind.packageMarkers.contains(where: script.contains) {
            return (kind, .script)
        }
        return nil
    }

    /// 에이전트별 프로세스 수
    public static func count(_ processes: [ProcessSnapshot], kinds: [AgentKind] = builtIn) -> [AgentKind: Int] {
        var counts: [AgentKind: Int] = [:]
        for process in processes {
            if let (kind, _) = match(process, kinds: kinds) { counts[kind, default: 0] += 1 }
        }
        return counts
    }

    /// 사용자 지정 에이전트 이름 → AgentKind. 이름 규칙에 맞지 않으면 버린다 (최대 16개).
    /// 예: `defaults write io.github.s3onghyun.bulchimbeon customAgents -array my-agent`
    public static func customKinds(_ names: [String]) -> [AgentKind] {
        var seen = Set<String>()
        var kinds: [AgentKind] = []
        for name in names where kinds.count < 16
            && name.range(of: #"^[A-Za-z0-9][A-Za-z0-9._-]{0,31}\z"#, options: .regularExpression) != nil
            && seen.insert(name).inserted {
            kinds.append(AgentKind(id: "custom:\(name)", displayName: name, binaryNames: [name],
                                   pathMarkers: [], packageMarkers: []))
        }
        return kinds
    }

    /// 메뉴 표시용 요약: "Claude Code ×3, Codex"
    public static func summary(_ counts: [AgentKind: Int]) -> String {
        counts.filter { $0.value > 0 }
            .sorted { ($0.key.displayName, $0.key.id) < ($1.key.displayName, $1.key.id) }
            .map { $0.value > 1 ? "\($0.key.displayName) ×\($0.value)" : $0.key.displayName }
            .joined(separator: ", ")
    }

    static func basename(_ path: String) -> String {
        path.split(separator: "/").last.map(String.init) ?? path
    }
}

/// `sysctl(KERN_PROCARGS2)` 버퍼 파서.
/// 형식: [argc: Int32][exec_path\0][\0 패딩...][argv[0]\0 ... argv[argc-1]\0][환경변수...]
/// 환경변수 영역에는 절대 들어가지 않도록 argc·limit 에서 멈춘다.
public enum ProcArgs {
    public static func parse(_ bytes: [UInt8], limit: Int) -> [String] {
        guard bytes.count > 4, limit > 0 else { return [] }
        let argc = bytes.withUnsafeBytes { Int($0.loadUnaligned(as: Int32.self)) }
        guard argc > 0 else { return [] }

        var i = 4
        while i < bytes.count, bytes[i] != 0 { i += 1 } // exec_path
        while i < bytes.count, bytes[i] == 0 { i += 1 } // 패딩

        var args: [String] = []
        while i < bytes.count, args.count < min(argc, limit) {
            let start = i
            while i < bytes.count, bytes[i] != 0 { i += 1 }
            args.append(String(decoding: bytes[start..<i], as: UTF8.self))
            i += 1
        }
        return args
    }
}

/// 에이전트가 "열려만 있는지" vs "일하는 중인지".
///
/// 실측(2026-10): Claude Code 세션 7개 중 실제 작업 중인 1개만 자식으로 `caffeinate -i -t 300` 을 띄우고 CPU 10%,
/// 나머지(입력 대기)는 caffeinate 없음·CPU 0~0.1%.
///
/// 오탐(2026-11, 0.12.0): 에이전트를 다 닫았는데도 맥이 깨어 있었다. Firstmate 가 입력 대기로 세워 둔 Claude Code 세션
/// 19개는 저마다 오래 사는 자손(몇 초마다 폴링하는 Stop hook 감시 루프, MCP 서버)을 두는데,
/// 1. 자손 전체의 CPU 를 더하니 하나하나는 1% 안팎인 배경 프로세스들이 합쳐서 2% 를 넘었고
/// 2. Stop hook 은 턴 안에서 돈다. Claude Code(2.1.319 실행 파일에서 확인)는 대화형 세션이 "busy"(턴 진행 중)인 동안
///    caffeinate 를 4분마다 새로 띄우므로, 몇 시간째 끝나지 않는 Stop hook 아래의 세션도 caffeinate 를 쥐고 있었다.
/// 3. hook 이 띄운 헤드리스 `claude -p` 도우미는 그 자체로 에이전트로 잡히고 CPU 가 부모 세션에도 더해졌다.
///
/// 그래서 "일하는 중"은 에이전트 자신의 최근 활동으로만 본다.
/// - 자신 + 최근(`backgroundAge` 안에) 시작한 자손의 CPU 합이 `cpuThreshold` 이상 (응답 생성, 빌드·테스트 같은 도구 실행)
/// - 오래된 자손(MCP 서버, hook 감시 루프)은 하나가 혼자 `backgroundCPUThreshold` 이상 쓸 때만 (긴 빌드)
/// - caffeinate 자식은 최근 시작한 다른 자식(도구·hook 실행)이 함께 있을 때만. Claude Code 의 Bash 도구와
///   command hook 은 기본 10분에 끊기므로, 그보다 오래 붙어 있는 자식만 남았다면 턴이 멈춰 서 있는 것이다
/// - 다른 에이전트 아래에서 뜬 에이전트(도우미 세션)는 따로 판단하고, 부모의 자손 합에는 넣지 않는다
public enum AgentActivity {
    public struct Status: Equatable, Sendable {
        public var present = 0
        public var busy = 0
        public init(present: Int = 0, busy: Int = 0) { self.present = present; self.busy = busy }
    }

    public static let cpuThreshold = 2.0 // %
    /// 이보다 오래 산 자손은 배경 프로세스로 본다 (Claude Code Bash 도구·command hook 의 기본 제한 시간 10분)
    public static let backgroundAge: TimeInterval = 10 * 60
    /// 배경 프로세스는 혼자 이만큼 써야 작업으로 본다 (실측: hook 감시 루프 ~1%, MCP 서버 ~0.3%, 빌드 85%)
    public static let backgroundCPUThreshold = 5.0 // %
    /// 모든 에이전트가 조용해진 뒤 이만큼 기다렸다가 교대 (도구 실행 사이의 짧은 공백·응답 대기 흡수)
    public static let busyGrace: TimeInterval = 5 * 60
    /// 자손 탐색 한도 (순환 부모 링크에도 멈춘다)
    static let maxTreeVisit = 512

    /// 왜 일하는 중인지 (첫 번째로 맞은 근거)
    public enum Reason: String, Sendable {
        case caffeinate, recentCPU, background
    }

    /// 자손 하나의 기여 (진단 출력용)
    public struct Contributor: Equatable, Sendable {
        public let pid: Int32
        public let executablePath: String
        public let cpu: Double
        /// 실행 시간(초). 시작 시각을 모르면 nil
        public let age: TimeInterval?
    }

    /// 에이전트 하나의 판단 근거. 명령줄·환경변수는 담지 않는다.
    public struct Explanation: Equatable, Sendable {
        public var busy: Bool { reason != nil }
        public var reason: Reason?
        /// caffeinate 자식의 PID
        public var caffeinate: Int32?
        /// caffeinate 를 뒷받침하는, 최근 시작한 자식(도구·hook 실행)
        public var freshChild: Contributor?
        public var ownCPU: Double = 0
        /// 자신 + 최근 시작한 자손의 CPU 합
        public var recentCPU: Double = 0
        /// 최근 시작한 자손 중 CPU 를 가장 많이 쓴 것
        public var topRecent: Contributor?
        /// 오래된 자손 중 CPU 를 가장 많이 쓴 것
        public var topBackground: Contributor?
        /// 다른 에이전트 아래에서 떴으면 그 에이전트의 PID
        public var parentAgent: Int32?
    }

    public static func evaluate(_ processes: [ProcessSnapshot], cpu: [Int32: Double], now: Date = Date(),
                                kinds: [AgentKind] = AgentCatalog.builtIn) -> [AgentKind: Status] {
        let tree = Tree(processes, kinds: kinds)
        var result: [AgentKind: Status] = [:]
        for p in processes {
            guard let kind = tree.agents[p.pid] else { continue }
            result[kind, default: Status()].present += 1
            if explain(p, tree: tree, cpu: cpu, now: now).busy { result[kind, default: Status()].busy += 1 }
        }
        return result
    }

    /// 프로세스 목록의 부모-자식 관계와 에이전트 판별 결과
    public struct Tree: Sendable {
        public let children: [Int32: [ProcessSnapshot]]
        public let byPID: [Int32: ProcessSnapshot]
        public let agents: [Int32: AgentKind]

        public init(_ processes: [ProcessSnapshot], kinds: [AgentKind] = AgentCatalog.builtIn) {
            var children: [Int32: [ProcessSnapshot]] = [:], byPID: [Int32: ProcessSnapshot] = [:], agents: [Int32: AgentKind] = [:]
            for p in processes {
                children[p.ppid, default: []].append(p)
                byPID[p.pid] = p
                if let (kind, _) = AgentCatalog.match(p, kinds: kinds) { agents[p.pid] = kind }
            }
            self.children = children
            self.byPID = byPID
            self.agents = agents
        }
    }

    public static func explain(_ agent: ProcessSnapshot, tree: Tree, cpu: [Int32: Double], now: Date) -> Explanation {
        var e = Explanation()
        e.parentAgent = parentAgent(of: agent, tree: tree)
        e.ownCPU = cpu[agent.pid] ?? 0
        e.recentCPU = e.ownCPU

        func contributor(_ p: ProcessSnapshot) -> Contributor {
            Contributor(pid: p.pid, executablePath: p.executablePath, cpu: cpu[p.pid] ?? 0,
                        age: p.startedAt.map { max(0, now.timeIntervalSince($0)) })
        }
        func isRecent(_ c: Contributor) -> Bool { (c.age ?? 0) < backgroundAge }
        func more(_ c: Contributor, than old: Contributor?) -> Bool { old.map { c.cpu > $0.cpu } ?? (c.cpu > 0) }

        for child in tree.children[agent.pid, default: []] where child.pid != agent.pid {
            if AgentCatalog.basename(child.executablePath) == "caffeinate" {
                e.caffeinate = e.caffeinate ?? child.pid
            } else if e.freshChild == nil, tree.agents[child.pid] == nil, isRecent(contributor(child)) {
                e.freshChild = contributor(child)
            }
        }

        // 자손: 다른 에이전트를 만나면 그 아래로 내려가지 않는다 (그 에이전트는 따로 판단)
        var queue = tree.children[agent.pid, default: []], visited: Set<Int32> = [agent.pid]
        while let next = queue.popLast(), visited.count < maxTreeVisit {
            guard visited.insert(next.pid).inserted, tree.agents[next.pid] == nil else { continue }
            let c = contributor(next)
            if isRecent(c) {
                e.recentCPU += c.cpu
                if more(c, than: e.topRecent) { e.topRecent = c }
            } else if more(c, than: e.topBackground) {
                e.topBackground = c
            }
            queue += tree.children[next.pid, default: []]
        }

        if e.caffeinate != nil && e.freshChild != nil {
            e.reason = .caffeinate
        } else if e.recentCPU >= cpuThreshold {
            e.reason = .recentCPU
        } else if let b = e.topBackground, b.cpu >= backgroundCPUThreshold {
            e.reason = .background
        }
        return e
    }

    /// 조상 중 가장 가까운 에이전트
    static func parentAgent(of agent: ProcessSnapshot, tree: Tree) -> Int32? {
        var pid = agent.ppid, visited: Set<Int32> = [agent.pid]
        while visited.count < maxTreeVisit, visited.insert(pid).inserted, let p = tree.byPID[pid] {
            if tree.agents[pid] != nil { return pid }
            pid = p.ppid
        }
        return nil
    }
}

extension AgentActivity.Explanation {
    /// `--diagnose` 의 한 줄 요약: 작업 중인지와 그 근거
    public var verdict: String {
        switch reason {
        case .caffeinate: L10n.t("작업 중 (caffeinate + 최근 시작한 자식)", "working (caffeinate + a recently started child)", "工作中（caffeinate + 最近启动的子进程）")
        case .recentCPU: L10n.t("작업 중 (자신과 최근 프로세스의 CPU)", "working (CPU of the agent and its recent processes)", "工作中（代理及其最近进程的 CPU）")
        case .background: L10n.t("작업 중 (오래 도는 프로세스가 바쁨)", "working (a long-running process is busy)", "工作中（长时间运行的进程很忙）")
        case nil: L10n.t("대기", "idle", "空闲")
        }
    }

    /// `--diagnose` 의 근거 줄들. PID·실행 파일 이름·CPU·실행 시간만 쓴다 (명령줄·환경변수 없음).
    public var details: [String] {
        let pct = { (v: Double) in String(format: "%.1f%%", v) }
        func who(_ c: AgentActivity.Contributor) -> String {
            "pid \(c.pid) \(AgentCatalog.basename(c.executablePath)) \(pct(c.cpu)), "
                + L10n.t("실행 ", "running ", "已运行 ") + (c.age.map(DutyClock.format) ?? "?")
        }
        var lines = [L10n.t("CPU: 자신 ", "CPU: own ", "CPU：自身 ") + pct(ownCPU)
                     + L10n.t(" · 최근 프로세스 포함 ", " · with recent processes ", " · 含最近进程 ") + pct(recentCPU)
                     + L10n.t(" (기준 ", " (needs ≥ ", "（阈值 ") + pct(AgentActivity.cpuThreshold) + L10n.t(" 이상)", ")", "）")]
        if let caffeinate {
            if let freshChild {
                lines.append("caffeinate pid \(caffeinate): " + L10n.t("인정 — 최근 시작한 자식 ", "counted, backed by ", "计入 — 最近启动的子进程 ") + who(freshChild))
            } else {
                lines.append("caffeinate pid \(caffeinate): "
                             + L10n.t("인정 안 함 — 최근 \(Int(AgentActivity.backgroundAge / 60))분 안에 시작한 도구·hook 실행이 없음 (긴 Stop hook 등으로 턴이 멈춰 있음)",
                                      "not counted, no tool or hook started in the last \(Int(AgentActivity.backgroundAge / 60)) min (a turn held open, e.g. by a long Stop hook)",
                                      "不计入 — 最近 \(Int(AgentActivity.backgroundAge / 60)) 分钟内没有启动的工具或 hook（回合被长时间的 Stop hook 等挂起）"))
            }
        }
        if let topRecent {
            lines.append(L10n.t("가장 바쁜 최근 프로세스: ", "busiest recent process: ", "最忙的最近进程：") + who(topRecent))
        }
        if let topBackground {
            lines.append(L10n.t("가장 바쁜 오래된 프로세스: ", "busiest long-running process: ", "最忙的长时间运行进程：") + who(topBackground)
                         + L10n.t(" (혼자 ", " (counts alone from ", "（单独达到 ") + pct(AgentActivity.backgroundCPUThreshold)
                         + L10n.t(" 이상일 때만 인정)", ")", " 才计入）"))
        }
        if let parentAgent {
            lines.append(L10n.t("에이전트 pid \(parentAgent) 아래에서 뜬 세션: 따로 판단하고 그 에이전트의 CPU 에는 넣지 않음",
                                "launched under agent pid \(parentAgent): judged on its own, not added to that agent",
                                "在代理 pid \(parentAgent) 下启动：单独判断，不计入该代理"))
        }
        return lines
    }
}
