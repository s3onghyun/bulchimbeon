import BulchimbeonCore
import Foundation
import AppKit
import Darwin
import IOKit.ps
import IOKit.pwr_mgt

enum SystemProbes {
    /// 내장 배터리 상태. 데스크톱 맥이면 nil.
    static func battery() -> BatteryStatus? {
        guard let blob = IOPSCopyPowerSourcesInfo()?.takeRetainedValue(),
              let sources = IOPSCopyPowerSourcesList(blob)?.takeRetainedValue() as? [CFTypeRef]
        else { return nil }

        for source in sources {
            guard let info = IOPSGetPowerSourceDescription(blob, source)?.takeUnretainedValue() as? [String: Any],
                  info[kIOPSTypeKey] as? String == kIOPSInternalBatteryType,
                  let current = info[kIOPSCurrentCapacityKey] as? Int
            else { continue }
            let max = (info[kIOPSMaxCapacityKey] as? Int).flatMap { $0 > 0 ? $0 : nil } ?? 100
            let onBattery = info[kIOPSPowerSourceStateKey] as? String == kIOPSBatteryPowerValue
            return BatteryStatus(percent: current * 100 / max, onBattery: onBattery)
        }
        return nil
    }

    /// 에이전트 판별용 프로세스 스냅숏 + 프로세스별 누적 CPU 시간(ns)
    static func processes(kinds: [AgentKind]) -> (snapshots: [ProcessSnapshot], cpuTimes: [Int32: UInt64]) {
        ProcessScanner.scan(kinds: kinds)
    }

    /// `pmset -g` 의 SleepDisabled (덮개 모드 전역 플래그)
    static func lidSleepDisabled() -> Bool {
        guard let output = run("/usr/bin/pmset", ["-g"]) else { return false }
        return PmsetParser.sleepDisabled(output)
    }

    /// `pmset -a disablesleep` 변경. 관리자 암호 프롬프트가 뜬다. 취소하면 false.
    ///
    /// 보안: 명령에 들어가는 가변 값은 이 앱의 PID(정수)와 Bool 뿐이라 셸 인젝션 여지가 없다.
    /// 켤 때는 "당직 감시병"(LidCommands.enable)을 함께 띄워, 앱이 강제 종료돼도 5초 안에 원복된다.
    /// 프롬프트에 실행할 명령을 그대로 보여줘, 사용자가 무엇에 암호를 주는지 알 수 있게 한다.
    static func setLidSleepDisabled(_ disabled: Bool) -> Bool {
        let pid = getpid()
        let command = disabled ? LidCommands.enable(pid: pid) : LidCommands.disable(pid: pid)
        let shown = "/usr/bin/pmset -a disablesleep \(disabled ? 1 : 0)"
            + (disabled ? L10n.t(" (+ 앱 종료 시 자동 원복 감시)", " (+ auto-restore watchdog)", "（含应用退出后自动恢复的监控进程）") : "")
        let prompt = L10n.t("불침번이 덮개 잠자기 설정을 바꾸려 합니다.\n실행 명령: ",
                            "Bulchimbeon wants to change the lid sleep setting.\nCommand: ", "Bulchimbeon 将更改合盖休眠设置。\n执行命令：") + shown
        let script = "do shell script \"\(command)\" with prompt \"\(prompt)\" with administrator privileges"
        return run("/usr/bin/osascript", ["-e", script]) != nil
    }

    /// 이 앱의 당직 감시병(root)이 살아 있는지
    static func lidWatchdogAlive() -> Bool {
        run("/usr/bin/pgrep", ["-f", LidCommands.watchdogPattern(pid: getpid())]) != nil
    }

    /// 실행 후 stdout 반환. 종료 코드가 0이 아니면 nil.
    @discardableResult
    static func run(_ path: String, _ arguments: [String]) -> String? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: path)
        process.arguments = arguments
        // 앱 환경(DYLD_*, 사용자 PATH 등)을 넘기지 않는다. LANG=C 로 출력 형식도 고정.
        process.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "C"]
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = FileHandle.nullDevice
        do { try process.run() } catch { return nil }
        // 파이프 버퍼가 차서 멈추지 않도록 종료 대기 전에 먼저 읽는다
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else { return nil }
        return String(decoding: data, as: UTF8.self)
    }
}

/// libproc 기반 프로세스 수집.
///
/// 보안·이식성:
/// - `ps` 출력 파싱 대신 시스템 API 를 직접 쓴다 (출력 형식·잘림·로케일 영향 없음)
/// - 현재 사용자(UID) 프로세스만 본다 — 다른 사용자·root 프로세스의 명령줄은 읽지 않는다
/// - argv 는 러너(node 등)일 때만 앞 8개, 그 외엔 argv[0] 만. 환경변수 영역은 파싱하지 않고 버퍼는 즉시 0 으로 지운다
/// - argv 가 판별을 바꾸지 않는 프로세스는 PID·시작 시각만 기억해 다시 읽지 않는다 (argv 는 캐시하지 않음)
/// - 결과는 메모리에서 판별에만 쓰고 저장·기록하지 않는다
enum ProcessScanner {
    private static var argumentCache = ProcessArgumentCache()

    /// `Bulchimbeon --diagnose`: 감지 결과와 근거를 출력한다. 명령줄 인자는 출력하지 않는다.
    static func diagnose(custom: [String]) -> String {
        let kinds = AgentCatalog.builtIn + AgentCatalog.customKinds(custom)
        let processes = scan(kinds: kinds).snapshots
        var children: [Int32: [ProcessSnapshot]] = [:]
        for p in processes { children[p.ppid, default: []].append(p) }
        var lines = [L10n.t("불침번 에이전트 감지 진단 (UID \(getuid()) 프로세스 \(processes.count)개 검사)",
                            "Bulchimbeon agent detection (checked \(processes.count) processes of UID \(getuid()))", "Bulchimbeon 代理检测诊断（已扫描 UID \(getuid()) 的 \(processes.count) 个进程）"), ""]
        for process in processes {
            guard let (kind, reason) = AgentCatalog.match(process, kinds: kinds) else { continue }
            let busy = AgentActivity.isBusy(process, children: children, cpu: [:])
            lines.append("  ✔ \(kind.displayName)  pid \(process.pid)  " + L10n.t("근거: ", "matched by: ", "匹配依据：") + reason.label
                         + (busy ? L10n.t("  · 작업 중", "  · working", "  · 工作中") : ""))
            if let cwd = process.cwd { lines.append("      " + L10n.t("작업 폴더: ", "cwd: ", "工作目录：") + cwd) }
            lines.append("      " + L10n.t("실행 파일: ", "executable: ", "可执行文件：") + process.executablePath)
        }
        if lines.count == 2 { lines.append("  " + L10n.t("감지된 에이전트 없음", "No agents detected", "未检测到代理")) }
        let holders = PowerHolders.summarize(PowerHolderProbe.assertions(), ownPID: getpid())
        lines += ["", L10n.t("화면을 막는 다른 앱: ", "Also keeping the display on: ", "同样保持屏幕常亮的应用：") + (holders.display.isEmpty ? "-" : holders.display.joined(separator: ", ")),
                  L10n.t("잠들지 못하게 하는 다른 앱: ", "Also keeping the Mac awake: ", "同样阻止 Mac 休眠的应用：") + (holders.system.isEmpty ? "-" : holders.system.joined(separator: ", "))]
        lines += ["", L10n.t("사용자 지정 에이전트: ", "Custom agents: ", "自定义代理：")
                      + (custom.isEmpty ? L10n.t("없음", "none", "无") : custom.joined(separator: ", ")),
                  L10n.t("추가: ", "Add: ", "添加：") + "defaults write io.github.s3onghyun.bulchimbeon customAgents -array <name>"]
        return lines.joined(separator: "\n")
    }

    static func scan(kinds: [AgentKind]) -> (snapshots: [ProcessSnapshot], cpuTimes: [Int32: UInt64]) {
        let uid = getuid()
        let byteCount = proc_listpids(UInt32(PROC_UID_ONLY), uid, nil, 0)
        guard byteCount > 0 else {
            argumentCache.retain(only: [])
            return ([], [:])
        }

        var pids = [pid_t](repeating: 0, count: Int(byteCount) / MemoryLayout<pid_t>.stride + 64)
        let filled = pids.withUnsafeMutableBytes {
            proc_listpids(UInt32(PROC_UID_ONLY), uid, $0.baseAddress, Int32($0.count))
        }
        var snapshots: [ProcessSnapshot] = [], cpuTimes: [Int32: UInt64] = [:]
        var liveProcesses = Set<ProcessIdentity>()
        for pid in pids.prefix(Int(filled) / MemoryLayout<pid_t>.stride) where pid > 0 {
            guard let snapshot = snapshot(pid, kinds: kinds, liveProcesses: &liveProcesses) else { continue }
            snapshots.append(snapshot)
            if let time = cpuTime(pid) { cpuTimes[pid] = time }
        }
        argumentCache.retain(only: liveProcesses)
        return (snapshots, cpuTimes)
    }

    private static func snapshot(_ pid: pid_t, kinds: [AgentKind], liveProcesses: inout Set<ProcessIdentity>) -> ProcessSnapshot? {
        var buffer = [CChar](repeating: 0, count: Int(MAXPATHLEN) * 4) // PROC_PIDPATHINFO_MAXSIZE
        guard proc_pidpath(pid, &buffer, UInt32(buffer.count)) > 0 else { return nil }
        let path = String(cString: buffer)
        guard let process = processInfo(pid) else { return nil }
        let identity = ProcessIdentity(pid: Int32(pid), startedAtSeconds: process.startedAtSeconds,
                                       startedAtMicroseconds: process.startedAtMicroseconds)
        liveProcesses.insert(identity)

        let args = argumentCache.arguments(for: identity, executablePath: path, kinds: kinds) {
            let args = arguments(pid, limit: AgentCatalog.needsFullArguments(executablePath: path) ? 8 : 1)
            // 에이전트 후보면 도우미 세션(--tools=) 판별을 위해 인자를 더 읽는다. 메모리에서만 쓰고 버린다.
            return AgentCatalog.isCandidate(executablePath: path, argv0: args.first) ? arguments(pid, limit: 64) : args
        }
        let isAgentCandidate = AgentCatalog.isCandidate(executablePath: path, argv0: args.first)
        return ProcessSnapshot(pid: pid, ppid: process.ppid, executablePath: path, arguments: args,
                               cwd: isAgentCandidate ? workingDirectory(pid) : nil)
    }

    private static func workingDirectory(_ pid: pid_t) -> String? {
        var info = proc_vnodepathinfo()
        let size = Int32(MemoryLayout<proc_vnodepathinfo>.size)
        guard proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0, &info, size) == size else { return nil }
        return withUnsafeBytes(of: info.pvi_cdir.vip_path) { raw in
            String(cString: raw.bindMemory(to: CChar.self).baseAddress!)
        }
    }

    private static func processInfo(_ pid: pid_t) -> (ppid: Int32, startedAtSeconds: UInt64, startedAtMicroseconds: UInt64)? {
        var info = proc_bsdinfo()
        let size = Int32(MemoryLayout<proc_bsdinfo>.size)
        guard proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, size) == size else { return nil }
        return (Int32(info.pbi_ppid), info.pbi_start_tvsec, info.pbi_start_tvusec)
    }

    /// 누적 CPU 시간 (user + system), 나노초
    private static func cpuTime(_ pid: pid_t) -> UInt64? {
        var info = proc_taskinfo()
        let size = Int32(MemoryLayout<proc_taskinfo>.size)
        guard proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &info, size) == size else { return nil }
        return (info.pti_total_user + info.pti_total_system) * timebase.numer / timebase.denom
    }

    private static let timebase: (numer: UInt64, denom: UInt64) = {
        var info = mach_timebase_info_data_t()
        mach_timebase_info(&info)
        return (UInt64(info.numer), UInt64(max(info.denom, 1)))
    }()

    private static func arguments(_ pid: pid_t, limit: Int) -> [String] {
        var mib: [Int32] = [CTL_KERN, KERN_PROCARGS2, pid]
        var size = 0
        guard sysctl(&mib, 3, nil, &size, nil, 0) == 0, size > 0 else { return [] }
        var bytes = [UInt8](repeating: 0, count: size)
        defer { bytes.withUnsafeMutableBytes { _ = memset_s($0.baseAddress, $0.count, 0, $0.count) } }
        guard sysctl(&mib, 3, &bytes, &size, nil, 0) == 0 else { return [] }
        return ProcArgs.parse(Array(bytes.prefix(size)), limit: limit)
    }
}

/// 다른 앱의 잠자기 방지 assertion 목록
enum PowerHolderProbe {
    static func assertions() -> [PowerHolders.Assertion] {
        var unmanaged: Unmanaged<CFDictionary>?
        guard IOPMCopyAssertionsByProcess(&unmanaged) == kIOReturnSuccess,
              let byPID = unmanaged?.takeRetainedValue() as? [NSNumber: [[String: Any]]] else { return [] }
        var result: [PowerHolders.Assertion] = []
        for (pidNumber, list) in byPID {
            let pid = pidNumber.int32Value
            for item in list {
                guard let type = item["AssertType"] as? String else { continue }
                // root 프로세스는 이름 조회가 막히므로 assertion 에 담긴 "Process Name" 을 먼저 쓴다
                let name = appName(pid) ?? item["Process Name"] as? String ?? processName(pid)
                result.append(.init(pid: pid, processName: name, type: type, name: item["AssertName"] as? String ?? ""))
            }
        }
        return result
    }

    private static func appName(_ pid: Int32) -> String? {
        NSRunningApplication(processIdentifier: pid)?.localizedName
    }

    private static func processName(_ pid: Int32) -> String {
        var buffer = [CChar](repeating: 0, count: 256)
        return proc_name(pid, &buffer, UInt32(buffer.count)) > 0 ? String(cString: buffer) : "pid \(pid)"
    }
}
