package engine

import (
	"encoding/json"
	"fmt"
	"time"
)

// lineCase is one entry of testdata/engine/lines.json.
type lineCase struct {
	Fn             string          `json:"fn"`
	Args           json.RawMessage `json:"args"`
	SystemLanguage bool            `json:"systemLanguage"`
	KO             json.RawMessage `json:"ko"`
	EN             json.RawMessage `json:"en"`
	ZH             json.RawMessage `json:"zh"`
}

func (c lineCase) want(lang Lang) json.RawMessage {
	if lang == ZH {
		return c.ZH
	}
	if lang == KO {
		return c.KO
	}
	return c.EN
}

type noticeJSON struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// lineValue evaluates a lines.json function name in one language. The result is a
// string, a []string or a noticeJSON, matching the fixture's JSON shape.
func lineValue(fn string, args json.RawMessage, lang Lang) (any, error) {
	var list []any
	if len(args) > 0 && args[0] == '[' {
		if err := json.Unmarshal(args, &list); err != nil {
			return nil, err
		}
	}
	str := func(i int) string { return list[i].(string) }
	num := func(i int) int { return int(list[i].(float64)) }
	flag := func(i int) bool { return list[i].(bool) }
	n := func(x Notice) any { return noticeJSON{x.Title, x.Body} }
	switch fn {
	case "onDuty":
		return OnDutyLines(lang), nil
	case "standby":
		return StandbyLines(lang), nil
	case "off":
		return OffLines(lang), nil
	case "reportForDuty":
		return n(ReportForDuty(lang)), nil
	case "agentArrived":
		return n(AgentArrived(str(0), lang)), nil
	case "handover":
		return n(Handover(str(0), lang)), nil
	case "timeUp":
		return n(TimeUp(lang)), nil
	case "batteryRetreat":
		return n(BatteryRetreat(num(0), lang)), nil
	case "lidFallback":
		return n(LidFallback(lang)), nil
	case "lidResumeFallback":
		return n(LidResumeFallback(lang)), nil
	case "lidLeftOn":
		return n(LidLeftOn(lang)), nil
	case "heatAdvisory":
		return n(HeatAdvisoryNotice(lang)), nil
	case "heatRest":
		return n(HeatRest(lang)), nil
	case "heatResume":
		return n(HeatResume(flag(0), lang)), nil
	case "postAbandoned":
		return n(PostAbandoned(lang)), nil
	case "speech.reporting":
		return SpeechReporting(lang), nil
	case "speech.arrived":
		return SpeechArrived(lang), nil
	case "speech.handover":
		return SpeechHandover(lang), nil
	case "speech.timeUp":
		return SpeechTimeUp(lang), nil
	case "speech.resumed":
		return SpeechResumed(lang), nil
	case "speech.heat":
		return SpeechHeat(lang), nil
	case "speech.rest":
		return SpeechRest(lang), nil
	case "speech.backFromRest":
		return SpeechBackFromRest(lang), nil
	case "speech.notAWOL":
		return SpeechNotAWOL(lang), nil
	case "speech.handoverReport":
		return SpeechHandoverReport(lang), nil
	case "speech.waiting":
		return SpeechWaiting(lang), nil
	case "reason.executableName":
		return ReasonExecutableName.Label(lang), nil
	case "reason.argv0":
		return ReasonArgv0.Label(lang), nil
	case "reason.installPath":
		return ReasonInstallPath.Label(lang), nil
	case "reason.script":
		return ReasonScript.Label(lang), nil
	case "mode.display":
		return ModeDisplay.Title(lang), nil
	case "mode.system":
		return ModeSystem.Title(lang), nil
	case "mode.lid":
		return ModeLid.Title(lang), nil
	case "watchDetail":
		var a struct {
			BusyNow   bool    `json:"busyNow"`
			CountIdle bool    `json:"countIdle"`
			Summary   string  `json:"summary"`
			OpenCount int     `json:"openCount"`
			OnDuty    bool    `json:"onDuty"`
			Quiet     float64 `json:"quiet"`
			Grace     float64 `json:"grace"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		d := WatchDetail(a.BusyNow, a.CountIdle, a.Summary, a.OpenCount, a.OnDuty, seconds(a.Quiet), seconds(a.Grace), lang)
		if d == nil {
			return nil, nil
		}
		return *d, nil
	}
	return nil, fmt.Errorf("unknown line function %q", fn)
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
