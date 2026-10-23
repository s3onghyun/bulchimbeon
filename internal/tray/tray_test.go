package tray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/daemon"
	"github.com/s3onghyun/bulchimbeon/internal/dbus"
	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
	"github.com/s3onghyun/bulchimbeon/internal/sprites"
)

// fake bus ------------------------------------------------------------------

type fakeBus struct {
	mu        sync.Mutex
	in        chan *dbus.Message
	done      chan struct{}
	watcher   bool
	taken     map[string]bool
	refuse    []error // answers to the registrations, in order; then success
	calls     []string
	emits     []string
	replies   map[uint32]*dbus.Message
	closed    bool
	registers chan struct{}
}

func newFakeBus(watcher bool) *fakeBus {
	return &fakeBus{in: make(chan *dbus.Message, 16), done: make(chan struct{}), watcher: watcher,
		replies: map[uint32]*dbus.Message{}, registers: make(chan struct{}, 16)}
}

func (f *fakeBus) UniqueName() string { return ":1.42" }

func (f *fakeBus) Call(_ time.Duration, dest string, path dbus.ObjectPath, iface, member, sig string, args ...any) ([]any, error) {
	f.mu.Lock()
	f.calls = append(f.calls, member)
	f.mu.Unlock()
	switch member {
	case "RequestName":
		if f.taken[args[0].(string)] {
			return []any{uint32(3)}, nil // EXISTS
		}
		return []any{uint32(1)}, nil
	case "NameHasOwner":
		return []any{f.watcher}, nil
	case "RegisterStatusNotifierItem":
		if dest != watcherName || path != watcherPath || !strings.HasPrefix(args[0].(string), "org.kde.StatusNotifierItem-") {
			return nil, errors.New("bad registration")
		}
		f.registers <- struct{}{}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.refuse) > 0 {
			err := f.refuse[0]
			f.refuse = f.refuse[1:]
			return nil, err
		}
		return nil, nil
	}
	return nil, nil
}

func (f *fakeBus) Emit(path dbus.ObjectPath, iface, member, sig string, args ...any) error {
	// a signal must encode against its signature, as on the wire
	if _, err := (&dbus.Message{Type: dbus.TypeSignal, Serial: 1, Path: path, Interface: iface, Member: member, Signature: sig, Body: args}).Marshal(); err != nil {
		return err
	}
	f.mu.Lock()
	f.emits = append(f.emits, member)
	f.mu.Unlock()
	return nil
}

// Reply round-trips the reply through the wire format, so the tests see what a host decodes.
func (f *fakeBus) Reply(call *dbus.Message, sig string, args ...any) error {
	data, err := (&dbus.Message{Type: dbus.TypeMethodReturn, Serial: 1, ReplySerial: call.Serial, Signature: sig, Body: args}).Marshal()
	if err != nil {
		panic(err)
	}
	m, err := dbus.ReadMessage(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	f.mu.Lock()
	f.replies[call.Serial] = m
	f.mu.Unlock()
	return nil
}

func (f *fakeBus) ReplyError(call *dbus.Message, name, text string) error {
	f.mu.Lock()
	f.replies[call.Serial] = &dbus.Message{Type: dbus.TypeError, ErrorName: name, Body: []any{text}}
	f.mu.Unlock()
	return nil
}

func (f *fakeBus) Incoming() <-chan *dbus.Message { return f.in }
func (f *fakeBus) Done() <-chan struct{}          { return f.done }
func (f *fakeBus) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeBus) count(list *[]string, member string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range *list {
		if m == member {
			n++
		}
	}
	return n
}

func (f *fakeBus) reply(serial uint32) *dbus.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.replies[serial]
}

// a tray wired to a fake bus and temporary files ------------------------------

type rig struct {
	t     *testing.T
	bus   *fakeBus
	tray  *tray
	paths daemon.Paths
	now   time.Time
	log   *bytes.Buffer
	n     uint32
}

func newRig(t *testing.T) *rig {
	dir := t.TempDir()
	r := &rig{t: t, bus: newFakeBus(true), now: time.Unix(1_792_140_000, 0), log: &bytes.Buffer{},
		paths: daemon.Paths{Settings: filepath.Join(dir, "config", "settings.json"), State: filepath.Join(dir, "state", "state.json"),
			Log: filepath.Join(dir, "state", "duty-log.txt")}}
	en := engine.EN
	r.tray = &tray{cfg: Config{Paths: r.paths, Lang: &en, Log: r.log, Now: func() time.Time { return r.now },
		Alive: func(int) bool { return true }, Getenv: func(string) string { return "" }},
		bus: r.bus, settings: settings.Defaults(), cache: map[string][]any{}}
	return r
}

// state writes a running daemon's state.json.
func (r *rig) state(order, pose string, onDuty, resting bool) {
	started := r.now.Add(-90 * time.Second)
	st := daemon.State{Version: 1, PID: 77, Platform: "linux", StartedAt: started, UpdatedAt: r.now, Order: order,
		OnDuty: onDuty, Resting: resting, Pose: pose, Heat: "normal", Settings: settings.Defaults(), LogPath: r.paths.Log,
		Agents: []daemon.AgentState{{ID: "claude", Name: "Claude Code", Open: 1, Working: 1,
			Processes: []daemon.ProcessState{{PID: 100, Working: onDuty, TreeCPU: 9.5, Reason: "executableName"}}}}}
	if onDuty {
		st.DutyStartedAt = &started
		st.Lock = platform.LockStatus{Wanted: true, Method: "systemd-inhibit",
			Blocks: []platform.Block{{What: "sleep", Held: true, PID: 5}, {What: "idle", Held: true, PID: 6}}}
	}
	data, _ := json.Marshal(st)
	os.MkdirAll(filepath.Dir(r.paths.State), 0o700)
	if err := os.WriteFile(r.paths.State, data, 0o600); err != nil {
		r.t.Fatal(err)
	}
}

// call sends a method call to the tray and returns the reply.
func (r *rig) call(path dbus.ObjectPath, iface, member, sig string, args ...any) *dbus.Message {
	r.t.Helper()
	r.n++
	m := &dbus.Message{Type: dbus.TypeMethodCall, Serial: r.n, Sender: ":1.7", Path: path, Interface: iface, Member: member, Signature: sig, Body: args}
	// the call itself must be what a host can send
	data, err := m.Marshal()
	if err != nil {
		r.t.Fatal(err)
	}
	if m, err = dbus.ReadMessage(bytes.NewReader(data)); err != nil {
		r.t.Fatal(err)
	}
	r.tray.handle(m, func() {})
	reply := r.bus.reply(r.n)
	if reply == nil {
		r.t.Fatalf("%s: no reply", member)
	}
	return reply
}

// menu is a decoded GetLayout node.
type menu struct {
	id       int32
	props    map[string]any
	children []menu
}

func decodeLayout(v any) menu {
	node := v.([]any)
	m := menu{id: node[0].(int32), props: map[string]any{}}
	for _, kv := range node[1].([]any) {
		kv := kv.([]any)
		m.props[kv[0].(string)] = kv[1].(dbus.Variant).Value
	}
	for _, c := range node[2].([]any) {
		m.children = append(m.children, decodeLayout(c.(dbus.Variant).Value))
	}
	return m
}

func (r *rig) layout() (uint32, menu) {
	r.t.Helper()
	reply := r.call(menuPath, menuIface, "GetLayout", "iias", int32(0), int32(-1), []string{})
	if reply.Type != dbus.TypeMethodReturn {
		r.t.Fatalf("GetLayout: %v", reply.Body)
	}
	return reply.Body[0].(uint32), decodeLayout(reply.Body[1])
}

func labels(items []menu) []string {
	var out []string
	for _, c := range items {
		if l, ok := c.props["label"].(string); ok {
			out = append(out, l)
		} else {
			out = append(out, "--")
		}
	}
	return out
}

// tests ---------------------------------------------------------------------

func TestPoseAndStatusFollowTheStateFile(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct {
		order, pose     string
		onDuty, resting bool
		want            engine.Pose
		headline        string
	}{
		{"agentWatch", "standby", false, false, engine.PoseStandby, "👀 Bulchimbeon — standing by until an agent starts working"},
		{"agentWatch", "duty", true, false, engine.PoseDuty, "🪖 Bulchimbeon — on duty for 1:30 (agent watch)"},
		{"agentWatch", "salute", false, false, engine.PoseSalute, "👀 Bulchimbeon — standing by until an agent starts working"},
		{"agentWatch", "heat", false, true, engine.PoseHeat, "🥵 Bulchimbeon — combat rest (heat warning), resumes when it cools down"},
		{"off", "sleeping", false, false, engine.PoseSleeping, "💤 Bulchimbeon — off duty"},
	} {
		r.state(c.order, c.pose, c.onDuty, c.resting)
		r.tray.refresh()
		if r.tray.view.pose != c.want || r.tray.view.headline != c.headline {
			t.Errorf("%s: pose %s, headline %q", c.pose, r.tray.view.pose, r.tray.view.headline)
		}
	}

	// the status block is `bulchimbeon status`'s lock, why, agents and handover lines
	r.state("agentWatch", "duty", true, false)
	r.tray.refresh()
	want := []string{"Lock: blocking sleep and idle (systemd-inhibit pid 5, 6)",
		"Why: Claude Code pid 100 9.5% (process-tree CPU ≥ 2.0%, 60 s average)",
		"Agents: Claude Code ×1 open, 1 working", "Handover: 5 min after the last agent goes quiet (agentGraceMinutes)"}
	if !reflect.DeepEqual(r.tray.view.status, want) {
		t.Errorf("status = %q", r.tray.view.status)
	}

	// a daemon that is gone, stuck or stopped sleeps, with the status hint
	r.tray.cfg.Alive = func(int) bool { return false }
	r.tray.refresh()
	if r.tray.view.pose != engine.PoseSleeping || !strings.Contains(r.tray.view.headline, "is not running: process 77 is gone") ||
		!reflect.DeepEqual(r.tray.view.status, []string{"Start it: systemctl --user start bulchimbeon  (or bulchimbeon run)"}) {
		t.Errorf("gone: %s %q %q", r.tray.view.pose, r.tray.view.headline, r.tray.view.status)
	}
	r.tray.cfg.Alive = func(int) bool { return true }
	r.now = r.now.Add(time.Minute)
	r.tray.refresh()
	if r.tray.view.pose != engine.PoseSleeping || !strings.Contains(r.tray.view.headline, "not responding") {
		t.Errorf("stale: %s %q", r.tray.view.pose, r.tray.view.headline)
	}
	os.Remove(r.paths.State)
	r.tray.refresh()
	if r.tray.view.pose != engine.PoseSleeping || !strings.Contains(r.tray.view.headline, "no state file") {
		t.Errorf("no file: %s %q", r.tray.view.pose, r.tray.view.headline)
	}
}

func TestMenuLabelsInEveryLanguage(t *testing.T) {
	for lang, want := range map[engine.Lang][]string{
		engine.KO: {"잠금: 잡지 않음 — 일하는 에이전트가 없습니다", "🧥 군복", "트레이 아이콘 닫기 (근무는 계속)", "기본 전투복"},
		engine.EN: {"Lock: not held: no agent is working", "🧥 Uniform", "Quit tray (the watch keeps running)", "Standard"},
		engine.ZH: {"休眠锁：未持有：没有代理在工作", "🧥 制服", "关闭托盘图标（值班继续）", "标准"},
	} {
		r := newRig(t)
		r.tray.cfg.Lang = &lang
		r.state("agentWatch", "standby", false, false)
		r.tray.refresh()
		_, root := r.layout()
		got := labels(root.children)
		if got[1] != want[0] || got[len(got)-3] != want[1] || got[len(got)-1] != want[2] || labels(root.children[len(got)-3].children)[0] != want[3] {
			t.Errorf("%s: %q", lang, got)
		}
	}
	if statusLabel(daemon.StatusLine{Label: "에이전트", Text: "x"}) != "에이전트: x" ||
		statusLabel(daemon.StatusLine{Label: "代理：", Text: "x"}) != "代理：x" ||
		statusLabel(daemon.StatusLine{Text: "Start it"}) != "Start it" {
		t.Error("statusLabel")
	}
	long := strings.Repeat("ab", 80)
	if got := truncate(long, maxLabel); len([]rune(got)) != maxLabel || !strings.HasSuffix(got, "…") {
		t.Errorf("truncate = %q", got)
	}
	if mnemonic("my_agent") != "my__agent" {
		t.Error("underscores are not escaped")
	}
}

func TestMenuLayoutAndUniformChoice(t *testing.T) {
	r := newRig(t)
	r.state("agentWatch", "duty", true, false)
	r.tray.refresh()
	r.tray.animate()

	rev, root := r.layout()
	if root.id != 0 || root.props["children-display"] != "submenu" {
		t.Fatalf("root = %+v", root)
	}
	got := labels(root.children)
	want := []string{"🪖 Bulchimbeon — on duty for 1:30 (agent watch)", "Lock: blocking sleep and idle (systemd-inhibit pid 5, 6)",
		"Why: Claude Code pid 100 9.5% (process-tree CPU ≥ 2.0%, 60 s average)", "Agents: Claude Code ×1 open, 1 working",
		"Handover: 5 min after the last agent goes quiet (agentGraceMinutes)", "--", "🧥 Uniform", "--", "Quit tray (the watch keeps running)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("menu = %q", got)
	}
	for _, c := range root.children[:5] {
		if c.props["enabled"] != false {
			t.Errorf("status line %q is clickable", c.props["label"])
		}
	}
	uniform := root.children[6]
	if uniform.props["children-display"] != "submenu" || len(uniform.children) != len(sprites.Themes()) {
		t.Fatalf("uniform = %+v", uniform)
	}
	radio := func(u menu) []int32 {
		var states []int32
		for _, c := range u.children {
			if c.props["toggle-type"] != "radio" {
				t.Errorf("%v is not a radio item", c.props["label"])
			}
			states = append(states, c.props["toggle-state"].(int32))
		}
		return states
	}
	if !reflect.DeepEqual(radio(uniform), []int32{1, 0, 0, 0}) || !reflect.DeepEqual(labels(uniform.children), []string{"Standard", "Navy", "Arctic", "Desert"}) {
		t.Fatalf("themes = %q %v", labels(uniform.children), radio(uniform))
	}
	icons := r.bus.count(&r.bus.emits, "NewIcon")

	// picking Navy writes the setting the daemon and tray share, re-dresses the private and
	// redraws the menu
	navy := uniform.children[1].id
	if reply := r.call(menuPath, menuIface, "Event", "isvu", navy, "clicked", dbus.MakeVariant("s", ""), uint32(0)); reply.Type != dbus.TypeMethodReturn {
		t.Fatalf("Event = %+v", reply)
	}
	if v, ok, _ := settings.Stored(r.paths.Settings, engine.KeyUniformTheme); !ok || string(v) != `"navy"` {
		t.Fatalf("stored uniformTheme = %s", v)
	}
	rev2, root := r.layout()
	if rev2 <= rev || !reflect.DeepEqual(radio(root.children[6]), []int32{0, 1, 0, 0}) {
		t.Errorf("after navy: revision %d→%d, %v", rev, rev2, radio(root.children[6]))
	}
	if r.bus.count(&r.bus.emits, "NewIcon") != icons+1 || r.bus.count(&r.bus.emits, "LayoutUpdated") == 0 {
		t.Errorf("signals = %q", r.bus.emits)
	}
	if r.tray.view.theme != "navy" {
		t.Errorf("theme = %s", r.tray.view.theme)
	}

	// hovering and other events change nothing; GetGroupProperties and AboutToShow answer
	r.call(menuPath, menuIface, "Event", "isvu", navy+1, "hovered", dbus.MakeVariant("i", int32(0)), uint32(0))
	if v, _, _ := settings.Stored(r.paths.Settings, engine.KeyUniformTheme); string(v) != `"navy"` {
		t.Error("a hover changed the uniform")
	}
	group := r.call(menuPath, menuIface, "GetGroupProperties", "aias", []int32{navy, 4242}, []string{"label"})
	if items := group.Body[0].([]any); len(items) != 1 || items[0].([]any)[0] != navy {
		t.Errorf("GetGroupProperties = %v", group.Body)
	}
	if reply := r.call(menuPath, menuIface, "AboutToShow", "i", int32(0)); reply.Body[0] != false {
		t.Errorf("AboutToShow with nothing new = %v", reply.Body)
	}
	r.state("agentWatch", "standby", false, false)
	if reply := r.call(menuPath, menuIface, "AboutToShow", "i", int32(0)); reply.Body[0] != true {
		t.Errorf("AboutToShow after a change = %v", reply.Body)
	}
	if reply := r.call(menuPath, menuIface, "GetProperty", "is", navy, "label"); reply.Body[0].(dbus.Variant).Value != "Navy" {
		t.Errorf("GetProperty = %v", reply.Body)
	}
	if reply := r.call(menuPath, menuIface, "EventGroup", "a(isvu)", []any{[]any{int32(4242), "clicked", dbus.MakeVariant("s", ""), uint32(0)}}); !reflect.DeepEqual(reply.Body[0], []any{int32(4242)}) {
		t.Errorf("EventGroup = %v", reply.Body)
	}
	if reply := r.call(menuPath, propsIface, "Get", "ss", menuIface, "Version"); reply.Body[0].(dbus.Variant).Value != uint32(3) {
		t.Errorf("Version = %v", reply.Body)
	}
}

func TestItemPropertiesAndCalls(t *testing.T) {
	r := newRig(t)
	r.tray.iconRoot = "/run/user/1000/bulchimbeon/tray-icons"
	r.state("agentWatch", "standby", false, false)
	r.tray.refresh()
	r.tray.animate()
	reply := r.call(itemPath, propsIface, "GetAll", "s", itemIface)
	props := map[string]any{}
	for _, kv := range reply.Body[0].([]any) {
		kv := kv.([]any)
		props[kv[0].(string)] = kv[1].(dbus.Variant).Value
	}
	if props["Id"] != "bulchimbeon" || props["Status"] != "Active" || props["Menu"] != menuPath || props["ItemIsMenu"] != true ||
		props["IconThemePath"] != r.tray.iconRoot || !strings.HasPrefix(props["IconName"].(string), "bulchimbeon-default-") {
		t.Errorf("properties = %v", props)
	}
	if tip := props["ToolTip"].([]any); tip[2] != "👀 Bulchimbeon — standing by until an agent starts working" ||
		props["IconAccessibleDesc"] != tip[2] {
		t.Errorf("tooltip = %v, accessible %v", tip, props["IconAccessibleDesc"])
	}
	var sizes []int32
	for _, p := range props["IconPixmap"].([]any) {
		p := p.([]any)
		if len(p[2].([]byte)) != int(p[0].(int32)*p[1].(int32)*4) {
			t.Errorf("pixmap %v has %d bytes", p[0], len(p[2].([]byte)))
		}
		sizes = append(sizes, p[0].(int32))
	}
	if !reflect.DeepEqual(sizes, []int32{16, 22, 24, 32, 48}) {
		t.Errorf("pixmap sizes = %v", sizes)
	}

	for _, c := range []struct {
		path               dbus.ObjectPath
		iface, member, sig string
		args               []any
		err                string
	}{
		{itemPath, itemIface, "Activate", "ii", []any{int32(1), int32(2)}, ""},
		{itemPath, itemIface, "Scroll", "is", []any{int32(1), "vertical"}, ""},
		{itemPath, itemIface, "Activate", "s", []any{"x"}, errArgs},
		{itemPath, itemIface, "Explode", "", nil, errUnknown},
		{itemPath, propsIface, "Get", "ss", []any{itemIface, "Nope"}, errProperty},
		{itemPath, propsIface, "Set", "ssv", []any{itemIface, "Title", dbus.MakeVariant("s", "x")}, errReadOnly},
		{itemPath, propsIface, "GetAll", "s", []any{"org.example"}, errInterface},
		{itemPath, propsIface, "GetAll", "s", []any{propsIface}, ""},
		{"/elsewhere", itemIface, "Activate", "ii", []any{int32(1), int32(2)}, errUnknown},
		{"/", introIface, "Introspect", "", nil, ""},
		{"/", "org.freedesktop.DBus.Peer", "Ping", "", nil, ""},
	} {
		reply := r.call(c.path, c.iface, c.member, c.sig, c.args...)
		if c.err == "" && reply.Type != dbus.TypeMethodReturn || c.err != "" && reply.ErrorName != c.err {
			t.Errorf("%s %s.%s = %+v", c.path, c.iface, c.member, reply)
		}
	}
	for path, iface := range map[dbus.ObjectPath]string{itemPath: itemIface, menuPath: menuIface, "/": "StatusNotifierItem"} {
		if xml := r.call(path, introIface, "Introspect", "").Body[0].(string); !strings.Contains(xml, iface) {
			t.Errorf("Introspect %s lacks %s", path, iface)
		}
	}
	// a caller that wants no reply gets none
	m := &dbus.Message{Type: dbus.TypeMethodCall, Serial: 999, Flags: dbus.FlagNoReplyExpected, Path: itemPath, Interface: itemIface, Member: "Activate", Signature: "ii", Body: []any{int32(0), int32(0)}}
	r.tray.handle(m, func() {})
}

// The frame for each tick is the macOS one (tick % count), and the host hears NewIcon only
// when the picture changes.
func TestAnimation(t *testing.T) {
	r := newRig(t)
	r.state("agentWatch", "standby", false, false)
	r.tray.refresh()
	r.tray.animate()
	frames := sprites.Named("default").Frames(engine.PoseStandby)
	first := r.bus.count(&r.bus.emits, "NewIcon")
	changes := 0
	for tick := 1; tick <= 12; tick++ {
		r.tray.tick = tick
		r.tray.animate()
		if !reflect.DeepEqual(frames[tick%len(frames)], frames[(tick-1)%len(frames)]) {
			changes++
		}
	}
	if got := r.bus.count(&r.bus.emits, "NewIcon") - first; got != changes || changes != 4 {
		t.Errorf("NewIcon %d times for %d changes", got, changes)
	}
}

func TestPixmapsAreARGBInNetworkOrder(t *testing.T) {
	theme := sprites.Named("default")
	rows := theme.Frames(engine.PoseSleeping)[0] // row 0: "....KKKKKKK..ZZZ"
	p := pixmaps(theme, rows)[0].([]any)
	data := p[2].([]byte)
	if p[0] != int32(16) || !bytes.Equal(data[0:4], []byte{0, 0, 0, 0}) || !bytes.Equal(data[4*4:4*4+4], []byte{0xFF, 0x2A, 0x23, 0x30}) ||
		!bytes.Equal(data[13*4:13*4+4], []byte{0xFF, 0x9D, 0xB8, 0xFF}) {
		t.Errorf("pixels = % x", data[:16*4])
	}
}

func TestIconFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bulchimbeon", "tray-icons")
	if err := writeIcons(root); err != nil {
		t.Fatal(err)
	}
	theme := sprites.Named("arctic")
	rows := theme.Frames(engine.PoseDuty)[0]
	name := iconName(theme, rows)
	for _, d := range iconDirs {
		dir := strconv.Itoa(d.size) + "x" + strconv.Itoa(d.size)
		if d.scale > 1 {
			dir += "@" + strconv.Itoa(d.scale)
		}
		data, err := os.ReadFile(filepath.Join(root, "hicolor", dir, "apps", name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		want := theme.Render(rows, d.size*d.scale)
		if img.Bounds() != want.Bounds() {
			t.Fatalf("%s: %v", dir, img.Bounds())
		}
		for y := 0; y < want.Bounds().Dy(); y++ {
			for x := 0; x < want.Bounds().Dx(); x++ {
				r1, g1, b1, a1 := img.At(x, y).RGBA()
				r2, g2, b2, a2 := want.At(x, y).RGBA()
				if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
					t.Fatalf("%s (%d,%d) differs", dir, x, y)
				}
			}
		}
	}
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("root mode %v %v", info.Mode(), err)
	}
	if iconName(theme, rows) == iconName(sprites.Named("navy"), sprites.Named("navy").Frames(engine.PoseDuty)[0]) ||
		iconName(theme, rows) == iconName(theme, theme.Frames(engine.PoseDuty)[3]) {
		t.Error("icon names collide")
	}
	if err := writeIcons(root); err != nil {
		t.Fatal(err)
	}
}

// runPaths are a settings file and the state file of a daemon that has run.
func runPaths(t *testing.T) daemon.Paths {
	dir := t.TempDir()
	p := daemon.Paths{Settings: filepath.Join(dir, "settings.json"), State: filepath.Join(dir, "state.json")}
	if err := os.WriteFile(p.State, []byte(`{"version": 1, "stopped": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A user who never ran the daemon (the .deb autostarts the tray for everyone) gets no icon.
func TestNoIconWhenTheDaemonNeverRan(t *testing.T) {
	bus := newFakeBus(true)
	var log bytes.Buffer
	err := Run(context.Background(), Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log,
		Paths: daemon.Paths{Settings: filepath.Join(t.TempDir(), "s.json"), State: filepath.Join(t.TempDir(), "state.json")}})
	if err != nil || strings.Count(log.String(), "\n") != 1 || !strings.Contains(log.String(), "never run") ||
		bus.count(&bus.calls, "RequestName") != 0 {
		t.Errorf("Run = %v, log %q, calls %q", err, log.String(), bus.calls)
	}
}

// Headless: with no session bus the tray says so once and exits 0.
func TestNoSessionBus(t *testing.T) {
	var log bytes.Buffer
	err := Run(context.Background(), Config{Dial: func() (Bus, error) { return nil, errors.New("DBUS_SESSION_BUS_ADDRESS is not set") }, Log: &log})
	if err != nil || strings.Count(log.String(), "\n") != 1 || !strings.Contains(log.String(), "no session bus") {
		t.Errorf("Run = %v, log %q", err, log.String())
	}
}

// A session bus without a tray host (no panel, or XFCE without a status tray): one line, exit 0.
func TestNoTrayHostExitsQuietly(t *testing.T) {
	bus := newFakeBus(false)
	var log bytes.Buffer
	runtime := t.TempDir()
	err := Run(context.Background(), Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log, WatcherWait: 20 * time.Millisecond,
		RuntimeDir: runtime, Paths: runPaths(t)})
	if err != nil || strings.Count(log.String(), "\n") != 1 || !strings.Contains(log.String(), "no tray host") {
		t.Errorf("Run = %v, log %q", err, log.String())
	}
	if entries, _ := os.ReadDir(runtime); len(entries) != 0 {
		t.Errorf("wrote %v without a tray host", entries)
	}
	if bus.count(&bus.calls, "RegisterStatusNotifierItem") != 0 || !bus.closed {
		t.Errorf("calls %q closed %v", bus.calls, bus.closed)
	}
}

// A panel that starts after the tray, and one that restarts: the tray registers each time.
func TestRegistersWhenTheHostAppearsAndAgainAfterARestart(t *testing.T) {
	bus := newFakeBus(false)
	var log syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- Run(ctx, Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log, WatcherWait: time.Minute, PID: 7,
			Frames: make(chan time.Time), Paths: runPaths(t)})
	}()
	owner := func(newOwner string) {
		bus.in <- &dbus.Message{Type: dbus.TypeSignal, Sender: dbus.BusName, Interface: dbus.BusInterface, Member: "NameOwnerChanged",
			Signature: "sss", Body: []any{watcherName, "", newOwner}}
	}
	wait := func() {
		select {
		case <-bus.registers:
		case <-time.After(5 * time.Second):
			t.Fatal("no registration")
		}
	}
	owner(":1.5")
	wait()
	owner("")
	owner(":1.9")
	wait()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := bus.count(&bus.calls, "RegisterStatusNotifierItem"); n != 2 {
		t.Errorf("registered %d times", n)
	}
	if !strings.Contains(log.String(), "showing the tray icon") {
		t.Errorf("log %q", log.String())
	}
}

// A second `bulchimbeon tray` in the same session (autostart plus a manual start) leaves.
func TestOneTrayPerSession(t *testing.T) {
	bus := newFakeBus(true)
	bus.taken = map[string]bool{instanceName: true}
	var log bytes.Buffer
	err := Run(context.Background(), Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log, Paths: runPaths(t)})
	if err != nil || strings.Count(log.String(), "\n") != 1 || !strings.Contains(log.String(), "already running") ||
		bus.count(&bus.calls, "RegisterStatusNotifierItem") != 0 {
		t.Errorf("Run = %v, log %q, calls %q", err, log.String(), bus.calls)
	}
}

// A host that refuses the first registration is asked again.
func TestRetriesARefusedRegistration(t *testing.T) {
	defer func(d time.Duration) { retryAfter = d }(retryAfter)
	retryAfter = 10 * time.Millisecond
	bus := newFakeBus(true)
	bus.refuse = []error{errors.New("org.freedesktop.DBus.Error.NoReply")}
	var log syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- Run(ctx, Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log, Frames: make(chan time.Time),
			Paths: runPaths(t)})
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-bus.registers:
		case <-time.After(5 * time.Second):
			t.Fatalf("registration %d missing", i+1)
		}
	}
	for end := time.Now().Add(5 * time.Second); !strings.Contains(log.String(), "showing the tray icon"); time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("not shown after a retry: %q", log.String())
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Quit ends the tray only: Run returns, the daemon's files are untouched.
func TestQuitFromTheMenu(t *testing.T) {
	bus := newFakeBus(true)
	var log syncBuffer
	paths, runtime := runPaths(t), t.TempDir()
	dir := filepath.Dir(paths.Settings)
	done := make(chan error)
	go func() {
		done <- Run(context.Background(), Config{Dial: func() (Bus, error) { return bus, nil }, Log: &log, Frames: make(chan time.Time),
			RuntimeDir: runtime, Paths: paths})
	}()
	select {
	case <-bus.registers:
	case <-time.After(5 * time.Second):
		t.Fatal("no registration")
	}
	bus.in <- &dbus.Message{Type: dbus.TypeMethodCall, Serial: 5, Path: menuPath, Interface: menuIface, Member: "Event",
		Signature: "isvu", Body: []any{idQuit, "clicked", dbus.MakeVariant("s", ""), uint32(0)}}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quit did not end the tray")
	}
	if bus.reply(5) == nil || !strings.Contains(log.String(), "quit") || !bus.closed {
		t.Errorf("reply %v log %q closed %v", bus.reply(5), log.String(), bus.closed)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Errorf("quit wrote the settings: %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(runtime, "bulchimbeon", "tray-icons", "hicolor", "22x22", "apps", "bulchimbeon-default-*.png")); len(files) == 0 {
		t.Error("no icon files for a tray host")
	}
}

// `bulchimbeon run` never talks to the session bus: the daemon's packages do not import the
// D-Bus client or the tray.
func TestDaemonDoesNotUseTheSessionBus(t *testing.T) {
	const module = "github.com/s3onghyun/bulchimbeon/"
	seen := map[string]bool{}
	var walk func(pkg string)
	walk = func(pkg string) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		dir := filepath.Join("..", "..", strings.TrimPrefix(pkg, module))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				if path == module+"internal/dbus" || path == module+"internal/tray" {
					t.Errorf("%s imports %s", pkg, path)
				}
				if strings.HasPrefix(path, module) {
					walk(path)
				}
			}
		}
	}
	walk(module + "internal/daemon")
	walk(module + "internal/platform/linux")
	if !seen[module+"internal/engine"] || !seen[module+"internal/platform"] {
		t.Errorf("walked %v", seen)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
