package platform

import (
	"slices"
	"testing"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
)

func TestSummarizeHolders(t *testing.T) {
	holders := []Holder{
		{Who: "Bulchimbeon", What: "sleep", Mode: "block", PID: 10},
		{Who: "test", What: "sleep", Mode: "block", PID: 11}, // ours by PID
		{Who: "vlc", What: "sleep:idle", Mode: "block", PID: 20},
		{Who: "UPower", What: "sleep", Mode: "delay", PID: 30},
		{Who: "gdm", What: "handle-power-key:handle-suspend-key", Mode: "block", PID: 40},
		{Who: "firefox", What: "idle", Mode: "block", PID: 50},
		{Who: "vlc", What: "sleep", Mode: "block", PID: 21},
	}
	s := SummarizeHolders(holders, []int{11}, "Bulchimbeon")
	if !slices.Equal(s.BlockSleep, []string{"vlc"}) || !slices.Equal(s.DelaySleep, []string{"UPower"}) ||
		!slices.Equal(s.BlockIdle, []string{"firefox", "vlc"}) {
		t.Errorf("summary = %+v", s)
	}
}

func TestLockStatusComplete(t *testing.T) {
	if !(LockStatus{}).Complete() {
		t.Error("nothing wanted is complete")
	}
	if (LockStatus{Wanted: true}).Complete() {
		t.Error("wanted with no blocks is not complete")
	}
	st := LockStatus{Wanted: true, Blocks: []Block{{What: "sleep", Held: true}, {What: "idle"}}}
	if st.Complete() {
		t.Error("a missing block is not complete")
	}
	if b, ok := st.Block("idle"); !ok || b.Held {
		t.Errorf("Block(idle) = %+v, %v", b, ok)
	}
}

func TestMethodAndFallbackText(t *testing.T) {
	for want, s := range map[string]LockStatus{
		"org.gnome.SessionManager":              {Method: MethodGnome},
		"org.freedesktop.PowerManagement (KDE)": {Method: MethodFreedesktop, Desktop: "kde"},
		"org.freedesktop.PowerManagement":       {Method: MethodFreedesktop},
		"logind block-weak":                     {Method: MethodLogind, Mode: "block-weak"},
		"logind":                                {Method: MethodLogind},
	} {
		if got := MethodText(s); got != want {
			t.Errorf("MethodText(%+v) = %q, want %q", s, got, want)
		}
	}
	if !(LockStatus{Method: MethodGnome}).OnDesktop() || !(LockStatus{Method: MethodFreedesktop}).OnDesktop() || (LockStatus{Method: MethodLogind}).OnDesktop() {
		t.Error("OnDesktop")
	}
	if got := FallbackText(LockStatus{Fallback: FallbackNoBus, Detail: "x"}, engine.EN); got != "no session bus: x" {
		t.Errorf("no bus: %q", got)
	}
	if got := FallbackText(LockStatus{Fallback: FallbackNoDesktop}, engine.KO); got != "세션 버스에 데스크톱 없음" {
		t.Errorf("no desktop: %q", got)
	}
	if FallbackText(LockStatus{}, engine.EN) != "" {
		t.Error("no fallback")
	}
}
