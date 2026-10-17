import Foundation

/// 당직 근무 인수인계: 자리를 비운 사이(화면 잠금 ~ 해제) 근무 내역 요약.
public enum HandoverReport {
    /// 이보다 짧게 자리를 비웠으면 보고하지 않는다 (잠깐 커피 마시러 간 경우)
    public static let minimumAway: TimeInterval = 10 * 60

    public static func make(entries: [DutyLogEntry], awayFrom: Date, until: Date,
                            _ lang: L10n.Language = L10n.language) -> Lines.Notice? {
        guard until.timeIntervalSince(awayFrom) >= minimumAway else { return nil }
        let during = entries.filter { $0.date >= awayFrom && $0.date <= until && $0.kind != .other }
        guard !during.isEmpty else { return nil }

        func count(_ kind: DutyLogEntry.Kind) -> Int { during.filter { $0.kind == kind }.count }
        let away = DutyClock.format(until.timeIntervalSince(awayFrom))
        var parts = [L10n.t("자리 비운 시간 \(away)", "Away \(away)", "离开了 \(away)", lang)]
        if count(.onDuty) > 0 { parts.append(L10n.t("근무 투입 \(count(.onDuty))회", "\(count(.onDuty)) watch(es)", "值班 \(count(.onDuty)) 次", lang)) }
        if count(.handover) > 0 { parts.append(L10n.t("근무 교대 \(count(.handover))회", "\(count(.handover)) handover(s)", "交班 \(count(.handover)) 次", lang)) }
        if count(.heat) > 0 { parts.append(L10n.t("폭염 \(count(.heat))회", "\(count(.heat)) heat event(s)", "高温事件 \(count(.heat)) 次", lang)) }
        if count(.awol) > 0 { parts.append(L10n.t("근무지 이탈 복귀 \(count(.awol))회", "\(count(.awol)) re-engage(s)", "恢复防休眠 \(count(.awol)) 次", lang)) }

        // 가장 최근 의미 있는 사건 하나 (최신순 기준 첫 항목)
        let latest = during.max { $0.date < $1.date }!
        let last = L10n.t("마지막: ", "Last: ", "最近记录：", lang) + latest.line
        switch lang {
        case .ko: return ("당직 근무 인수인계 📋", parts.joined(separator: " · ") + "\n" + last + "\n이상입니다! 🫡")
        case .en: return ("Night watch handover 📋", parts.joined(separator: " · ") + "\n" + last + "\nThat is all! 🫡")
        case .zh: return ("夜间值班交接 📋", parts.joined(separator: " · ") + "\n" + last + "\n报告完毕！🫡")
        }
    }
}
