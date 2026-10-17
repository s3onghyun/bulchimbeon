import Foundation

/// 이등병 대사집. 톤: 성실하고 귀엽게. 갈굼·부조리 개그는 넣지 않는다.
/// 모든 대사는 언어를 인자로 받는다 (기본값: 시스템 언어).
public enum Lines {
    public typealias Notice = (title: String, body: String)
    public typealias Language = L10n.Language

    public static func onDuty(_ lang: Language = L10n.language) -> [String] {
        switch lang {
        case .ko: return ["근무 중 이상 무!", "편히 주무십시오. 제가 지키겠습니다!", "졸지 않겠습니다! …아마도요.",
               "에이전트 곁을 지키는 중입니다!", "별 보며 근무 서는 중 ✨"]
        case .en: return ["On watch. All clear!", "Sleep tight. I've got this!", "Won't doze off! …probably.",
               "Guarding your agents!", "Night watch under the stars ✨"]
        case .zh: return ["正在站岗，一切正常！", "安心睡吧，这里交给我！", "绝不打瞌睡！……大概吧。",
               "正在守护你的代理！", "星空下的夜间值班 ✨"]
        }
    }

    public static func standby(_ lang: Language = L10n.language) -> [String] {
        switch lang {
        case .ko: return ["근무 대기 중. 에이전트 출근하면 바로 투입합니다!", "총 닦으며 대기 중…", "에이전트 기다리는 중 👀"]
        case .en: return ["Standing by. Ready when an agent clocks in!", "Polishing my rifle… standing by.", "Waiting for agents 👀"]
        case .zh: return ["待命中，代理一开工就上岗！", "擦擦装备，继续待命……", "等待代理中 👀"]
        }
    }

    public static func off(_ lang: Language = L10n.language) -> [String] {
        switch lang {
        case .ko: return ["생활관 취침 중… zzz", "근무 없음. 꿀잠 중 💤"]
        case .en: return ["Asleep in the barracks… zzz", "Off duty. Sweet dreams 💤"]
        case .zh: return ["营房里睡觉中……zzz", "休息时间，做个好梦 💤"]
        }
    }

    public static func reportForDuty(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("불침번 근무 투입! 🪖", "근무 서겠습니다! 이상 무!")
        case .en: return ("Reporting for night watch! 🪖", "On duty. All clear!")
        case .zh: return ("夜间值班报到！🪖", "正在值班，一切正常！")
        }
    }

    public static func agentArrived(_ summary: String, _ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("근무 투입! 🪖", "\(summary) 출근 확인. 곁을 지키겠습니다!")
        case .en: return ("On duty! 🪖", "\(summary) clocked in. I'll keep watch!")
        case .zh: return ("开始值班！🪖", "\(summary) 已开工，我来守护！")
        }
    }

    public static func handover(_ summary: String, _ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("근무 교대합니다! 🫡", "\(summary) 조용해졌습니다 (작업 끝 또는 입력 대기). 이제 푹 주무십시오.")
        case .en: return ("Shift over! 🫡", "\(summary) went quiet (done, or waiting for input). Your Mac can sleep now.")
        case .zh: return ("交班啦！🫡", "\(summary) 已空闲（工作完成或等待输入）。Mac 现在可以休眠了。")
        }
    }

    public static func timeUp(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("근무 시간 종료! ⏰", "다음 근무자 깨우겠습니다. 수고하셨습니다!")
        case .en: return ("Watch is over! ⏰", "Waking the next guard. Good work!")
        case .zh: return ("值班时间到！⏰", "去叫下一位哨兵了，辛苦啦！")
        }
    }

    public static func batteryRetreat(_ percent: Int, _ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("당직사관님께 보고드립니다 🔋", "배터리 \(percent)%… 기력이 다해 근무 철수합니다.")
        case .en: return ("Reporting to the duty officer 🔋", "Battery at \(percent)%… out of strength, standing down.")
        case .zh: return ("向值班长报告 🔋", "电量剩余 \(percent)%……体力不支，结束值班。")
        }
    }

    public static func lidFallback(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("덮개 근무 불가 🙅", "관리자 승인이 없어 시스템 근무로 대체합니다. 덮개는 열어두십시오!")
        case .en: return ("Can't guard with the lid closed 🙅", "No admin approval, so I switched to system watch. Keep the lid open!")
        case .zh: return ("无法合盖值班 🙅", "未获管理员授权，已切换为系统值班。请保持屏幕盖打开！")
        }
    }

    public static func lidResumeFallback(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("근무 복귀! 🪖", "덮개 근무는 관리자 승인이 다시 필요해 시스템 근무로 복귀했습니다. 필요하면 메뉴에서 다시 켜주십시오.")
        case .en: return ("Back on watch! 🪖", "Lid-closed watch needs admin approval again, so I resumed as system watch. Turn it back on from the menu if needed.")
        case .zh: return ("恢复值班！🪖", "合盖值班需要重新获得管理员授权，已恢复为系统值班。如有需要，请从菜单重新开启。")
        }
    }

    public static func lidLeftOn(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("덮개 잠자기가 꺼진 채 남아 있습니다 ⚠️", "지난 근무가 비정상 종료됐습니다. 메뉴에서 '덮개 잠자기 복구'를 눌러주십시오.")
        case .en: return ("Lid sleep is still disabled ⚠️", "The last watch ended abnormally. Choose 'Restore lid sleep' from the menu.")
        case .zh: return ("合盖休眠仍被禁用 ⚠️", "上次值班异常结束。请在菜单中选择“恢复合盖休眠”。")
        }
    }

    public static func heatAdvisory(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("폭염 주의보 🌡️", "맥이 뜨거워지고 있습니다. 그늘에서 물 마시며 근무 중입니다!")
        case .en: return ("Heat advisory 🌡️", "Your Mac is heating up. Staying hydrated on watch!")
        case .zh: return ("高温提醒 🌡️", "Mac 正在升温。值班中也要注意补水！")
        }
    }

    public static func heatRest(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("폭염 경보 🔥 전투휴식 실시!", "맥을 식히는 동안 근무를 잠시 중단합니다. 열이 내리면 자동으로 복귀합니다.")
        case .en: return ("Heat warning 🔥 Combat rest!", "Pausing the watch so your Mac can cool down. I'll resume automatically.")
        case .zh: return ("过热警报 🔥 暂停值班降温！", "暂停值班，让 Mac 降温。温度恢复后会自动返回岗位。")
        }
    }

    public static func heatResume(lidDowngraded: Bool, _ lang: Language = L10n.language) -> Notice {
        let extra = lidDowngraded
            ? L10n.t(" 과열 직후라 덮개 근무 대신 시스템 근무로 복귀합니다.", " Resuming as system watch instead of lid-closed, to be safe.", " 为安全起见，将恢复系统值班，不启用合盖值班。", lang)
            : ""
        switch lang {
        case .ko: return ("전투휴식 종료 🫡", "열이 내렸습니다. 근무 복귀합니다!" + extra)
        case .en: return ("Combat rest over 🫡", "Cooled down. Back on watch!" + extra)
        case .zh: return ("降温休息结束 🫡", "温度已恢复，继续值班！" + extra)
        }
    }

    public static func postAbandoned(_ lang: Language = L10n.language) -> Notice {
        switch lang {
        case .ko: return ("근무지 이탈 감지 ⚠️", "잠자기 방지가 풀려 있어 즉시 재투입했습니다.")
        case .en: return ("Post abandoned? ⚠️", "The sleep block was released, so I re-engaged immediately.")
        case .zh: return ("防休眠失效？⚠️", "检测到防休眠已解除，现已立即恢复。")
        }
    }

    /// 순찰 이등병 말풍선
    public enum Speech {
        public static func reporting(_ lang: Language = L10n.language) -> String { L10n.t("근무 서겠습니다! 🫡", "Reporting for duty! 🫡", "值班报到！🫡", lang) }
        public static func arrived(_ lang: Language = L10n.language) -> String { L10n.t("근무 투입! 이상 무!", "On duty. All clear!", "正在值班，一切正常！", lang) }
        public static func handover(_ lang: Language = L10n.language) -> String { L10n.t("근무 교대합니다! 🫡", "Shift over! 🫡", "交班啦！🫡", lang) }
        public static func timeUp(_ lang: Language = L10n.language) -> String { L10n.t("근무 끝! 충성! 🫡", "Watch over! Salute! 🫡", "值班结束！敬礼！🫡", lang) }
        public static func resumed(_ lang: Language = L10n.language) -> String { L10n.t("근무 복귀했습니다! 🫡", "Back on watch! 🫡", "已返回岗位！🫡", lang) }
        public static func heat(_ lang: Language = L10n.language) -> String { L10n.t("폭염 주의보! 물 마시는 중 🥵", "Heat advisory! Hydrating 🥵", "高温提醒！正在补水 🥵", lang) }
        public static func rest(_ lang: Language = L10n.language) -> String { L10n.t("전투휴식! 열 식히는 중 🧊", "Combat rest! Cooling down 🧊", "暂停值班！正在降温 🧊", lang) }
        public static func backFromRest(_ lang: Language = L10n.language) -> String { L10n.t("전투휴식 끝! 근무 복귀 🫡", "Rest over! Back on post 🫡", "休息结束！返回岗位 🫡", lang) }
        public static func notAWOL(_ lang: Language = L10n.language) -> String { L10n.t("근무지 이탈 아닙니다! 복귀! 🫡", "Not AWOL! Back on post! 🫡", "没有擅离职守！已返回岗位！🫡", lang) }
        public static func handoverReport(_ lang: Language = L10n.language) -> String { L10n.t("인수인계 드리겠습니다! 📋", "Handover report! 📋", "交班报告！📋", lang) }
        public static func waiting(_ lang: Language = L10n.language) -> String { L10n.t("에이전트 출근 기다리는 중 👀", "Waiting for agents 👀", "等待代理中 👀", lang) }
    }

    public static func pick(_ lines: [String]) -> String {
        lines.randomElement() ?? ""
    }
}
