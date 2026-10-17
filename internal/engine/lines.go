package engine

import (
	"math/rand/v2"
	"strconv"
)

// The private's lines (Swift `Lines`). Tone: earnest and cute, never hazing.
// Every line takes the language explicitly.

// Notice is a notification: title and body.
type Notice struct {
	Title string
	Body  string
}

func OnDutyLines(lang Lang) []string {
	if lang == ZH {
		return []string{"正在站岗，一切正常！", "安心睡吧，这里交给我！", "绝不打瞌睡！……大概吧。",
			"正在守护你的代理！", "星空下的夜间值班 ✨"}
	}
	if lang == KO {
		return []string{"근무 중 이상 무!", "편히 주무십시오. 제가 지키겠습니다!", "졸지 않겠습니다! …아마도요.",
			"에이전트 곁을 지키는 중입니다!", "별 보며 근무 서는 중 ✨"}
	}
	return []string{"On watch. All clear!", "Sleep tight. I've got this!", "Won't doze off! …probably.",
		"Guarding your agents!", "Night watch under the stars ✨"}
}

func StandbyLines(lang Lang) []string {
	if lang == ZH {
		return []string{"待命中，代理一开工就上岗！", "擦擦装备，继续待命……", "等待代理中 👀"}
	}
	if lang == KO {
		return []string{"근무 대기 중. 에이전트 출근하면 바로 투입합니다!", "총 닦으며 대기 중…", "에이전트 기다리는 중 👀"}
	}
	return []string{"Standing by. Ready when an agent clocks in!", "Polishing my rifle… standing by.", "Waiting for agents 👀"}
}

func OffLines(lang Lang) []string {
	if lang == ZH {
		return []string{"营房里睡觉中……zzz", "休息时间，做个好梦 💤"}
	}
	if lang == KO {
		return []string{"생활관 취침 중… zzz", "근무 없음. 꿀잠 중 💤"}
	}
	return []string{"Asleep in the barracks… zzz", "Off duty. Sweet dreams 💤"}
}

func notice(lang Lang, koTitle, koBody, enTitle, enBody, zhTitle, zhBody string) Notice {
	if lang == ZH {
		return Notice{zhTitle, zhBody}
	}
	if lang == KO {
		return Notice{koTitle, koBody}
	}
	return Notice{enTitle, enBody}
}

func ReportForDuty(lang Lang) Notice {
	return notice(lang, "불침번 근무 투입! 🪖", "근무 서겠습니다! 이상 무!",
		"Reporting for night watch! 🪖", "On duty. All clear!", "夜间值班报到！🪖", "正在值班，一切正常！")
}

func AgentArrived(summary string, lang Lang) Notice {
	return notice(lang, "근무 투입! 🪖", summary+" 출근 확인. 곁을 지키겠습니다!",
		"On duty! 🪖", summary+" clocked in. I'll keep watch!", "开始值班！🪖", summary+" 已开工，我来守护！")
}

func Handover(summary string, lang Lang) Notice {
	return notice(lang, "근무 교대합니다! 🫡", summary+" 조용해졌습니다 (작업 끝 또는 입력 대기). 이제 푹 주무십시오.",
		"Shift over! 🫡", summary+" went quiet (done, or waiting for input). Your Mac can sleep now.", "交班啦！🫡", summary+" 已空闲（工作完成或等待输入）。Mac 现在可以休眠了。")
}

func TimeUp(lang Lang) Notice {
	return notice(lang, "근무 시간 종료! ⏰", "다음 근무자 깨우겠습니다. 수고하셨습니다!",
		"Watch is over! ⏰", "Waking the next guard. Good work!", "值班时间到！⏰", "去叫下一位哨兵了，辛苦啦！")
}

func BatteryRetreat(percent int, lang Lang) Notice {
	p := strconv.Itoa(percent)
	return notice(lang, "당직사관님께 보고드립니다 🔋", "배터리 "+p+"%… 기력이 다해 근무 철수합니다.",
		"Reporting to the duty officer 🔋", "Battery at "+p+"%… out of strength, standing down.", "向值班长报告 🔋", "电量剩余 "+p+"%……体力不支，结束值班。")
}

func LidFallback(lang Lang) Notice {
	return notice(lang, "덮개 근무 불가 🙅", "관리자 승인이 없어 시스템 근무로 대체합니다. 덮개는 열어두십시오!",
		"Can't guard with the lid closed 🙅", "No admin approval, so I switched to system watch. Keep the lid open!", "无法合盖值班 🙅", "未获管理员授权，已切换为系统值班。请保持屏幕盖打开！")
}

func LidResumeFallback(lang Lang) Notice {
	return notice(lang, "근무 복귀! 🪖", "덮개 근무는 관리자 승인이 다시 필요해 시스템 근무로 복귀했습니다. 필요하면 메뉴에서 다시 켜주십시오.",
		"Back on watch! 🪖", "Lid-closed watch needs admin approval again, so I resumed as system watch. Turn it back on from the menu if needed.", "恢复值班！🪖", "合盖值班需要重新获得管理员授权，已恢复为系统值班。如有需要，请从菜单重新开启。")
}

func LidLeftOn(lang Lang) Notice {
	return notice(lang, "덮개 잠자기가 꺼진 채 남아 있습니다 ⚠️", "지난 근무가 비정상 종료됐습니다. 메뉴에서 '덮개 잠자기 복구'를 눌러주십시오.",
		"Lid sleep is still disabled ⚠️", "The last watch ended abnormally. Choose 'Restore lid sleep' from the menu.", "合盖休眠仍被禁用 ⚠️", "上次值班异常结束。请在菜单中选择“恢复合盖休眠”。")
}

func HeatAdvisoryNotice(lang Lang) Notice {
	return notice(lang, "폭염 주의보 🌡️", "맥이 뜨거워지고 있습니다. 그늘에서 물 마시며 근무 중입니다!",
		"Heat advisory 🌡️", "Your Mac is heating up. Staying hydrated on watch!", "高温提醒 🌡️", "Mac 正在升温。值班中也要注意补水！")
}

func HeatRest(lang Lang) Notice {
	return notice(lang, "폭염 경보 🔥 전투휴식 실시!", "맥을 식히는 동안 근무를 잠시 중단합니다. 열이 내리면 자동으로 복귀합니다.",
		"Heat warning 🔥 Combat rest!", "Pausing the watch so your Mac can cool down. I'll resume automatically.", "过热警报 🔥 暂停值班降温！", "暂停值班，让 Mac 降温。温度恢复后会自动返回岗位。")
}

func HeatResume(lidDowngraded bool, lang Lang) Notice {
	extra := ""
	if lidDowngraded {
		extra = T(lang, " 과열 직후라 덮개 근무 대신 시스템 근무로 복귀합니다.", " Resuming as system watch instead of lid-closed, to be safe.", " 为安全起见，将恢复系统值班，不启用合盖值班。")
	}
	return notice(lang, "전투휴식 종료 🫡", "열이 내렸습니다. 근무 복귀합니다!"+extra,
		"Combat rest over 🫡", "Cooled down. Back on watch!"+extra, "降温休息结束 🫡", "温度已恢复，继续值班！"+extra)
}

func PostAbandoned(lang Lang) Notice {
	return notice(lang, "근무지 이탈 감지 ⚠️", "잠자기 방지가 풀려 있어 즉시 재투입했습니다.",
		"Post abandoned? ⚠️", "The sleep block was released, so I re-engaged immediately.", "防休眠失效？⚠️", "检测到防休眠已解除，现已立即恢复。")
}

// Speech bubbles (Swift `Lines.Speech`).

func SpeechReporting(lang Lang) string {
	return T(lang, "근무 서겠습니다! 🫡", "Reporting for duty! 🫡", "值班报到！🫡")
}
func SpeechArrived(lang Lang) string {
	return T(lang, "근무 투입! 이상 무!", "On duty. All clear!", "正在值班，一切正常！")
}
func SpeechHandover(lang Lang) string {
	return T(lang, "근무 교대합니다! 🫡", "Shift over! 🫡", "交班啦！🫡")
}
func SpeechTimeUp(lang Lang) string {
	return T(lang, "근무 끝! 충성! 🫡", "Watch over! Salute! 🫡", "值班结束！敬礼！🫡")
}
func SpeechResumed(lang Lang) string {
	return T(lang, "근무 복귀했습니다! 🫡", "Back on watch! 🫡", "已返回岗位！🫡")
}
func SpeechHeat(lang Lang) string {
	return T(lang, "폭염 주의보! 물 마시는 중 🥵", "Heat advisory! Hydrating 🥵", "高温提醒！正在补水 🥵")
}
func SpeechRest(lang Lang) string {
	return T(lang, "전투휴식! 열 식히는 중 🧊", "Combat rest! Cooling down 🧊", "暂停值班！正在降温 🧊")
}
func SpeechBackFromRest(lang Lang) string {
	return T(lang, "전투휴식 끝! 근무 복귀 🫡", "Rest over! Back on post 🫡", "休息结束！返回岗位 🫡")
}
func SpeechNotAWOL(lang Lang) string {
	return T(lang, "근무지 이탈 아닙니다! 복귀! 🫡", "Not AWOL! Back on post! 🫡", "没有擅离职守！已返回岗位！🫡")
}
func SpeechHandoverReport(lang Lang) string {
	return T(lang, "인수인계 드리겠습니다! 📋", "Handover report! 📋", "交班报告！📋")
}
func SpeechWaiting(lang Lang) string {
	return T(lang, "에이전트 출근 기다리는 중 👀", "Waiting for agents 👀", "等待代理中 👀")
}

// Pick returns a random line.
func Pick(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[rand.IntN(len(lines))]
}
