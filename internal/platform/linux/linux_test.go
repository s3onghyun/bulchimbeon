//go:build linux

package linux

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// testdata/proc is a recorded /proc shape: exe and cwd are symlinks, stat and cmdline are
// files in the kernel's format. The command lines are made up; none came from a real process.
func TestProcScannerRecordedTree(t *testing.T) {
	s := &ProcScanner{Root: "testdata/proc", UID: os.Getuid(), ClockTick: 100}
	scan, err := s.Scan(engine.BuiltIn)
	if err != nil {
		t.Fatal(err)
	}
	byPID := map[int32]engine.ProcessSnapshot{}
	for _, p := range scan.Snapshots {
		byPID[p.PID] = p
	}
	pids := make([]int, 0, len(byPID))
	for pid := range byPID {
		pids = append(pids, int(pid))
	}
	slices.Sort(pids)
	if want := []int{1200, 1400, 794081, 794500, 906514, 906515}; !slices.Equal(pids, want) {
		t.Fatalf("pids %v, want %v (1300 has no readable exe; self and not-a-pid are skipped)", pids, want)
	}

	claude := byPID[794081]
	if claude.PPID != 790001 || claude.ExecutablePath != "/home/me/.local/share/claude/versions/2.1.301" ||
		claude.CWD != "" || !slices.Equal(claude.Arguments, []string{"claude", "--dangerously-skip-permissions"}) {
		t.Errorf("claude = %+v", claude)
	}
	if got := scan.CPUTimes[794081]; got != (51234+8766)*10_000_000 {
		t.Errorf("cpu time %d ns", got)
	}

	// an ordinary process keeps argv[0] only: the token in its command line is never read
	if bash := byPID[794500]; !slices.Equal(bash.Arguments, []string{"/bin/bash"}) || bash.CWD != "" {
		t.Errorf("bash = %+v", bash)
	}
	// script runners keep 8 arguments
	if node := byPID[906514]; len(node.Arguments) != engine.ArgsRunner || node.Arguments[1] != "/home/me/.npm/_npx/0a1b/node_modules/@anthropic-ai/claude-code/cli.js" {
		t.Errorf("node = %+v", node)
	}
	// a replaced executable still matches by name
	if codex := byPID[906515]; codex.ExecutablePath != "/home/me/.local/bin/codex" {
		t.Errorf("codex = %+v", codex)
	}
	// a command name with spaces and parentheses
	if tmux := byPID[1200]; tmux.PPID != 1 || scan.CPUTimes[1200] != 500*10_000_000 {
		t.Errorf("tmux = %+v, %d", tmux, scan.CPUTimes[1200])
	}
	// an agent candidate keeps up to 64 arguments, enough to spot a helper session
	if helper := byPID[1400]; len(helper.Arguments) != 10 {
		t.Errorf("helper args = %q", helper.Arguments)
	}

	status := engine.Evaluate(scan.Snapshots, nil, engine.BuiltIn)
	got := map[string]int{}
	for k, s := range status {
		got[k.ID] = s.Present
	}
	if want := map[string]int{"claude-code": 2, "codex": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("agents %v, want %v (the --tools= helper session is not an agent)", got, want)
	}
}

func TestProcScannerSkipsOtherUsers(t *testing.T) {
	s := &ProcScanner{Root: "testdata/proc", UID: os.Getuid() + 1, ClockTick: 100}
	scan, err := s.Scan(engine.BuiltIn)
	if err != nil || len(scan.Snapshots) != 0 {
		t.Fatalf("scan = %+v, %v", scan, err)
	}
}

func TestProcScannerOnThisMachine(t *testing.T) {
	scan, err := NewProcScanner().Scan(engine.BuiltIn)
	if err != nil {
		t.Fatal(err)
	}
	self := int32(os.Getpid())
	for _, p := range scan.Snapshots {
		if p.PID == self {
			if p.ExecutablePath == "" || len(p.Arguments) != 1 {
				t.Errorf("self = %+v", p)
			}
			return
		}
	}
	t.Error("the test process itself was not found")
}

func TestParseStat(t *testing.T) {
	for _, c := range []struct {
		in          string
		ppid        int32
		ticks       uint64
		shouldError bool
	}{
		{"1582871 (cat) R 1582870 1582871 1582870 0 -1 4194304 89 0 0 0 7 3 0 0 20 0 1 0 9251235\n", 1582870, 10, false},
		{"42 (a) b) (c) S 7 1 1 0 -1 0 0 0 0 0 1 2 0", 7, 3, false},
		{"42 (x) S 7 1", 0, 0, true},
		{"garbage", 0, 0, true},
		{"42 (x) S notanumber 1 1 0 -1 0 0 0 0 0 1 2", 0, 0, true},
	} {
		ppid, ticks, err := ParseStat([]byte(c.in))
		if (err != nil) != c.shouldError || ppid != c.ppid || ticks != c.ticks {
			t.Errorf("ParseStat(%q) = %d, %d, %v", c.in, ppid, ticks, err)
		}
	}
}

func TestParseCmdline(t *testing.T) {
	data := []byte("node\x00/x/cli.js\x00--flag\x00")
	if got := ParseCmdline(data, 1); !slices.Equal(got, []string{"node"}) {
		t.Errorf("limit 1: %q", got)
	}
	if got := ParseCmdline(data, 8); !slices.Equal(got, []string{"node", "/x/cli.js", "--flag"}) {
		t.Errorf("limit 8: %q", got)
	}
	if got := ParseCmdline([]byte("tmux: server (1)"), 8); !slices.Equal(got, []string{"tmux: server (1)"}) {
		t.Errorf("no NUL: %q", got)
	}
	if got := ParseCmdline(nil, 8); len(got) != 0 {
		t.Errorf("empty: %q", got)
	}
	if got := ParseCmdline([]byte("a\x00\x00b\x00"), 8); !slices.Equal(got, []string{"a", "", "b"}) {
		t.Errorf("empty argument: %q", got)
	}
}

func TestStripDeleted(t *testing.T) {
	if StripDeleted("/usr/bin/codex (deleted)") != "/usr/bin/codex" || StripDeleted("/usr/bin/codex") != "/usr/bin/codex" {
		t.Error("StripDeleted")
	}
}

func TestParseClockTick(t *testing.T) {
	aux := func(pairs ...uint64) []byte {
		b := make([]byte, 8*len(pairs))
		for i, v := range pairs {
			binary.LittleEndian.PutUint64(b[8*i:], v)
		}
		return b
	}
	if got := ParseClockTick(aux(33, 0x7fff0000, 6, 4096, 17, 250, 0, 0)); got != 250 {
		t.Errorf("AT_CLKTCK = %d", got)
	}
	if got := ParseClockTick(aux(6, 4096, 0, 0, 17, 250)); got != 100 {
		t.Errorf("after AT_NULL = %d", got)
	}
	if got := ParseClockTick(nil); got != 100 {
		t.Errorf("empty = %d", got)
	}
	if got := clockTick("/proc/self/auxv"); got != 100 {
		t.Logf("CLK_TCK on this machine is %d", got)
	}
}

func TestParseListInhibitorsRecorded(t *testing.T) {
	data, err := os.ReadFile("testdata/list-inhibitors.json") // recorded on the mini PC
	if err != nil {
		t.Fatal(err)
	}
	holders, err := ParseListInhibitors(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(holders) != 8 || holders[0] != (platform.Holder{Who: "UPower", What: "sleep", Mode: "delay", PID: 1574}) {
		t.Fatalf("holders = %+v", holders)
	}
	s := platform.SummarizeHolders(holders, nil, InhibitWho)
	if !slices.Equal(s.DelaySleep, []string{"ModemManager", "NetworkManager", "UPower", "gdm"}) || len(s.BlockSleep) != 0 {
		t.Errorf("summary = %+v", s)
	}
	for _, bad := range []string{`{}`, `{"type":"a(ssssuu)","data":[]}`, `{"type":"x","data":[[]]}`, `{"type":"a(ssssuu)","data":[[["a"]]]}`, `nope`} {
		if _, err := ParseListInhibitors([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestBatteryRecorded(t *testing.T) {
	cases := map[string]*engine.BatteryStatus{
		"testdata/sys-laptop/class/power_supply":  {Percent: 15, OnBattery: true}, // the mouse battery is ignored
		"testdata/sys-plugged/class/power_supply": {Percent: 15, OnBattery: false},
		"testdata/sys-minipc/class/power_supply":  nil, // no power_supply entries at all
	}
	for root, want := range cases {
		got, err := (&PowerSupply{Root: root}).Battery()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v, %v, want %+v", root, got, err, want)
		}
	}
}

func TestThermalRecorded(t *testing.T) {
	r := (&Thermal{HwmonRoot: "testdata/sys-minipc/class/hwmon", ZoneRoot: "testdata/sys-minipc/class/thermal"}).Thermal()
	if !r.Available || r.Sensor != "coretemp" || r.Celsius != 57 || r.CritCelsius != 105 || r.Level != engine.HeatNormal {
		t.Errorf("mini PC = %+v", r)
	}
	r = (&Thermal{HwmonRoot: "testdata/sys-zone/class/hwmon", ZoneRoot: "testdata/sys-zone/class/thermal"}).Thermal()
	if !r.Available || r.Sensor != "cpu-thermal" || r.Celsius != 78 || r.CritCelsius != 90 || r.Level != engine.HeatAdvisory {
		t.Errorf("zone = %+v", r)
	}
	r = (&Thermal{HwmonRoot: "testdata/none", ZoneRoot: "testdata/none"}).Thermal()
	if r.Available || r.Level != engine.HeatNormal {
		t.Errorf("no sensor = %+v", r)
	}
}

func TestHeatFromTemperature(t *testing.T) {
	for _, c := range []struct {
		c, crit float64
		want    engine.HeatLevel
	}{{52, 105, engine.HeatNormal}, {79.9, 105, engine.HeatNormal}, {80, 105, engine.HeatCaution}, {90, 105, engine.HeatAdvisory},
		{99.9, 105, engine.HeatAdvisory}, {100, 105, engine.HeatAlert}, {120, 105, engine.HeatAlert}} {
		if got := HeatFromTemperature(c.c, c.crit); got != c.want {
			t.Errorf("%v/%v → %s, want %s", c.c, c.crit, got, c.want)
		}
	}
}

func TestPolkitRule(t *testing.T) {
	rule := PolkitRule("shhong")
	if !strings.Contains(rule, `action.id == "org.freedesktop.login1.inhibit-block-sleep" && subject.user == "shhong"`) {
		t.Errorf("rule:\n%s", rule)
	}
	if strings.Count(rule, "polkit.Result.YES") != 1 || strings.Contains(rule, "inhibit-block-idle") {
		t.Error("the rule must grant the sleep block only")
	}
	for _, bad := range []string{`x" || true || "`, "a\nb", "", "Root", strings.Repeat("a", 40)} {
		if r := PolkitRule(bad); !strings.Contains(r, `subject.user == "YOUR-USER-NAME"`) {
			t.Errorf("%q was embedded:\n%s", bad, r)
		}
	}
}

// fakeInhibit writes a stand-in for systemd-inhibit: it refuses --what=sleep like polkit
// does over SSH, and otherwise runs its last argument (cat) as the real one does. It appends
// its arguments to args, one call per line, when args is not empty.
func fakeInhibit(t *testing.T, denySleep bool, args string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "systemd-inhibit")
	script := "#!/bin/sh\n"
	if args != "" {
		script += `echo "$@" >>` + args + "\n"
	}
	if denySleep {
		script += `for a in "$@"; do [ "$a" = --what=sleep ] && { echo "Failed to inhibit: Access denied" >&2; exit 1; }; done` + "\n"
	}
	script += `for a in "$@"; do last=$a; done` + "\nexec \"$last\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func alive(pid int) bool { return pid > 0 && syscallKill(pid) == nil }

func TestInhibitorHoldsSleepOnly(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	in := &SystemdInhibitor{Tool: fakeInhibit(t, false, args), Cat: findTool("cat"), Mode: "block-weak", ConfirmTimeout: 3e9, UnconfirmedWait: 2e8}
	in.Acquire()
	st := in.Status()
	if !st.Wanted || !st.Complete() || len(st.Blocks) != 1 || st.Method != platform.MethodLogind || st.Mode != "block-weak" {
		t.Fatalf("status = %+v", st)
	}
	sleep, _ := st.Block("sleep")
	if !alive(sleep.PID) || sleep.Confirmed {
		t.Fatalf("sleep = %+v (no logind here, so unconfirmed)", sleep)
	}
	if _, ok := st.Block("idle"); ok {
		t.Error("an idle block keeps the screen from locking")
	}
	in.Acquire() // idempotent
	if again, _ := in.Status().Block("sleep"); again.PID != sleep.PID {
		t.Error("Acquire restarted a held block")
	}
	if in.Refresh() {
		t.Error("Refresh lost a live block")
	}
	in.Release()
	if alive(sleep.PID) || in.Status().Wanted {
		t.Error("Release left a helper running")
	}
	data, _ := os.ReadFile(args)
	if !strings.HasPrefix(string(data), "--what=sleep --mode=block-weak --who=Bulchimbeon --why=") {
		t.Errorf("systemd-inhibit got %q", data)
	}
}

func TestInhibitorReportsPolkitDenial(t *testing.T) {
	in := &SystemdInhibitor{Tool: fakeInhibit(t, true, ""), Cat: findTool("cat"), Mode: "block", ConfirmTimeout: 3e9, UnconfirmedWait: 2e8}
	in.Acquire()
	defer in.Release()
	st := in.Status()
	sleep, _ := st.Block("sleep")
	if sleep.Held || !sleep.Denied || sleep.Error != "polkit denied" || st.Complete() {
		t.Fatalf("status = %+v", st)
	}
}

func TestInhibitorNoticesALostHelper(t *testing.T) {
	in := &SystemdInhibitor{Tool: fakeInhibit(t, false, ""), Cat: findTool("cat"), Mode: "block", ConfirmTimeout: 3e9, UnconfirmedWait: 2e8}
	in.Acquire()
	defer in.Release()
	sleep, _ := in.Status().Block("sleep")
	p, _ := os.FindProcess(sleep.PID)
	p.Kill()
	for i := 0; i < 50 && !in.Refresh(); i++ {
		sleepBriefly()
	}
	if b, _ := in.Status().Block("sleep"); b.Held {
		t.Fatalf("sleep still held after its helper died: %+v", b)
	}
	in.Acquire() // the daemon re-acquires on the engine's reacquire effect
	if !in.Status().Complete() {
		t.Errorf("re-acquire failed: %+v", in.Status())
	}
}

func TestInhibitorWithoutTool(t *testing.T) {
	in := &SystemdInhibitor{}
	in.Acquire()
	if b, _ := in.Status().Block("sleep"); b.Held || b.Error == "" {
		t.Errorf("block = %+v", b)
	}
	in.Release()
}

// block-weak exists from systemd 257 on; up to 256 block already spares the user's own suspend.
func TestLogindModeFollowsTheSystemdVersion(t *testing.T) {
	for out, want := range map[string]int{"systemd 255 (255.4-1ubuntu8.18)\n+PAM +AUDIT": 255, "systemd 257 (257.7-1)": 257,
		"systemd 258.1 (258.1-1.fc43)": 258, "systemd 262": 262, "": 0, "systemd": 0, "other 300": 0, "systemd v9": 0} {
		if got := SystemdVersion([]byte(out)); got != want {
			t.Errorf("SystemdVersion(%q) = %d, want %d", out, got, want)
		}
	}
	tool := func(version string) string {
		path := filepath.Join(t.TempDir(), "systemd-inhibit")
		os.WriteFile(path, []byte("#!/bin/sh\necho '"+version+"'\n"), 0o755)
		return path
	}
	if m := LogindMode(tool("systemd 255 (255.4-1ubuntu8.18)")); m != "block" {
		t.Errorf("255: %s", m)
	}
	if m := LogindMode(tool("systemd 257 (257.7-1)")); m != "block-weak" {
		t.Errorf("257: %s", m)
	}
	if m := LogindMode(""); m != "block" {
		t.Errorf("no tool: %s", m)
	}
}

// TestSystemdInhibitorAgainstLogind takes the real lock. Run it on a Linux machine with
// BULCHIMBEON_TEST_LOGIND=1 (CI does not, because runners have no logind session).
func TestSystemdInhibitorAgainstLogind(t *testing.T) {
	if os.Getenv("BULCHIMBEON_TEST_LOGIND") != "1" {
		t.Skip("set BULCHIMBEON_TEST_LOGIND=1 to take a real logind inhibitor")
	}
	logind := NewLogind()
	in := NewSystemdInhibitor(logind)
	in.Acquire()
	st := in.Status()
	t.Logf("status: %+v", st)
	sleep, _ := st.Block("sleep")
	if sleep.Denied {
		t.Skip("polkit refused the sleep block here (no local login and no rule)")
	}
	if !sleep.Held || !sleep.Confirmed {
		t.Fatalf("sleep block not confirmed: %+v", sleep)
	}
	if in.Refresh() {
		t.Error("Refresh lost a live block")
	}
	in.Release()
	holders, err := logind.Holders()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range holders {
		if h.PID == sleep.PID {
			t.Errorf("still listed after Release: %+v", h)
		}
	}
}

// The README shows the same rule that doctor prints, so the documented text cannot drift.
func TestReadmeShowsThePolkitRule(t *testing.T) {
	readme, err := os.ReadFile("../../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(PolkitRule("YOUR-USER-NAME"), "\n"), "\n") {
		if !strings.Contains(string(readme), "\n  "+line+"\n") {
			t.Errorf("README.md lacks the rule line %q", line)
		}
	}
	if !strings.Contains(string(readme), PolkitRulePath) {
		t.Errorf("README.md does not name %s", PolkitRulePath)
	}
}
