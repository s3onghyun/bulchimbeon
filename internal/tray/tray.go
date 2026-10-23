// Package tray is `bulchimbeon tray`: the private in the desktop panel, as a
// StatusNotifierItem (the KDE/freedesktop tray protocol that XFCE's status tray, KDE and the
// GNOME AppIndicator extension show) with a com.canonical.dbusmenu menu.
//
// It is a separate process from the daemon and only reads what the daemon writes: the pose
// and the `bulchimbeon status` lines come from state.json, the duty log and logind's lock
// listing (asked as `status` asks it), the uniform from the settings file, which its uniform
// menu also writes. Without a session bus or a tray host it exits quietly,
// so the daemon works the same with or without a desktop.
package tray

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/daemon"
	"github.com/s3onghyun/bulchimbeon/internal/dbus"
	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
	"github.com/s3onghyun/bulchimbeon/internal/sprites"
)

// Bus is the part of a D-Bus connection the tray uses (*dbus.Conn; a fake in tests).
type Bus interface {
	UniqueName() string
	Call(timeout time.Duration, dest string, path dbus.ObjectPath, iface, member, sig string, args ...any) ([]any, error)
	Emit(path dbus.ObjectPath, iface, member, sig string, args ...any) error
	Reply(call *dbus.Message, sig string, args ...any) error
	ReplyError(call *dbus.Message, name, text string) error
	Incoming() <-chan *dbus.Message
	Done() <-chan struct{}
	Close() error
}

// Config wires the tray to the session bus and the daemon's files.
type Config struct {
	Dial   func() (Bus, error)
	Paths  daemon.Paths
	Home   string
	Lang   *engine.Lang // `--lang`; nil uses the settings, then the environment
	Getenv func(string) string
	Log    io.Writer
	Now    func() time.Time
	Alive  func(pid int) bool
	// Holders lists the sleep locks in logind, as `bulchimbeon status` asks it.
	Holders func() ([]platform.Holder, error)
	PID     int
	// RuntimeDir is $XDG_RUNTIME_DIR, where the icon files go. Empty: pixmaps only.
	RuntimeDir string
	// WatcherWait is how long to wait at start for a tray host (the panel may start after
	// the tray at login). Default 30 s.
	WatcherWait time.Duration
	// Frames ticks once per animation frame. Default: a sprites.FrameInterval ticker.
	Frames <-chan time.Time
}

const (
	// instanceName keeps one tray per session: autostart and a manual start share a bus.
	instanceName = "io.github.s3onghyun.bulchimbeon.Tray"
	watcherName  = "org.kde.StatusNotifierWatcher"
	watcherPath  = dbus.ObjectPath("/StatusNotifierWatcher")
	itemPath     = dbus.ObjectPath("/StatusNotifierItem")
	menuPath     = dbus.ObjectPath("/MenuBar")
	itemIface    = "org.kde.StatusNotifierItem"
	menuIface    = "com.canonical.dbusmenu"
	propsIface   = "org.freedesktop.DBus.Properties"
	introIface   = "org.freedesktop.DBus.Introspectable"
	peerIface    = "org.freedesktop.DBus.Peer"
	callTimeout  = 5 * time.Second
	errUnknown   = "org.freedesktop.DBus.Error.UnknownMethod"
	errArgs      = "org.freedesktop.DBus.Error.InvalidArgs"
	errProperty  = "org.freedesktop.DBus.Error.UnknownProperty"
	errReadOnly  = "org.freedesktop.DBus.Error.PropertyReadOnly"
	errInterface = "org.freedesktop.DBus.Error.UnknownInterface"
)

// holdersEvery is how often the lock listing is asked again (it runs busctl).
const holdersEvery = 5 * time.Second

// retryAfter is how long to wait before registering again with a host that refused.
var retryAfter = 5 * time.Second

// IconSizes are the pixmap sizes offered to the host: the sprite at 1×, centered in the
// common panel sizes, and at 2× and 3×.
var IconSizes = []int{16, 22, 24, 32, 48}

type tray struct {
	cfg      Config
	bus      Bus
	settings settings.Settings
	view     view
	revision uint32

	holders    []platform.Holder
	holdersErr error
	holdersAt  time.Time
	asking     bool
	holdersCh  chan holdersResult

	tick       int
	frameKey   string
	pixmaps    []any
	cache      map[string][]any
	iconRoot   string // IconThemePath, or empty
	iconName   string
	iconsTried bool

	watcher    bool // a tray host is on the bus
	registered bool
	everShown  bool
	lastErr    error // the last refused registration
	quit       bool
}

type holdersResult struct {
	holders []platform.Holder
	err     error
}

func (t *tray) logf(format string, args ...any) {
	fmt.Fprintf(t.cfg.Log, "bulchimbeon tray: "+format+"\n", args...)
}

// Run shows the tray icon until ctx ends, the user picks quit or the session bus goes away.
// With no session bus, no state file (the daemon never ran for this user), or no tray host
// within WatcherWait, it logs one line and returns nil.
// Once shown, it waits for a host that goes away (a panel restart) and registers again.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.WatcherWait == 0 {
		cfg.WatcherWait = 30 * time.Second
	}
	if cfg.Holders == nil {
		cfg.Holders = func() ([]platform.Holder, error) { return nil, errors.New("no lock listing") }
	}
	if cfg.Frames == nil {
		ticker := time.NewTicker(sprites.FrameInterval())
		defer ticker.Stop()
		cfg.Frames = ticker.C
	}
	t := &tray{cfg: cfg, settings: settings.Defaults(), cache: map[string][]any{}, holdersCh: make(chan holdersResult, 1)}

	bus, err := cfg.Dial()
	if err != nil {
		t.logf("no session bus (%v); no tray icon", err)
		return nil
	}
	defer bus.Close()
	t.bus = bus

	// The .deb starts the tray for every desktop user; one who never ran the daemon gets no
	// icon. A stopped daemon leaves its state file, so it still shows (asleep).
	if _, err := os.Stat(cfg.Paths.State); errors.Is(err, os.ErrNotExist) {
		t.logf("the daemon has never run for this user (no %s); no tray icon. Start it: systemctl --user enable --now bulchimbeon", cfg.Paths.State)
		return nil
	}

	own := func(name string) (bool, error) {
		reply, err := bus.Call(callTimeout, dbus.BusName, dbus.BusPath, dbus.BusInterface, "RequestName", "su", name, uint32(4)) // DO_NOT_QUEUE
		if err != nil {
			return false, fmt.Errorf("own %s: %w", name, err)
		}
		return len(reply) == 1 && reply[0] == uint32(1), nil // 1: primary owner
	}
	if ok, err := own(instanceName); err != nil {
		return err
	} else if !ok {
		t.logf("a tray icon is already running in this session")
		return nil
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", cfg.PID)
	if ok, err := own(name); err != nil || !ok {
		return fmt.Errorf("own %s: %v", name, err)
	}
	if _, err := bus.Call(callTimeout, dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s",
		"type='signal',sender='org.freedesktop.DBus',interface='org.freedesktop.DBus',member='NameOwnerChanged',arg0='"+watcherName+"'"); err != nil {
		return fmt.Errorf("watch for a tray host: %w", err)
	}
	reply, err := bus.Call(callTimeout, dbus.BusName, dbus.BusPath, dbus.BusInterface, "NameHasOwner", "s", watcherName)
	if err != nil {
		return fmt.Errorf("look for a tray host: %w", err)
	}
	t.watcher = len(reply) == 1 && reply[0] == true
	t.holders, t.holdersErr = cfg.Holders()
	t.holdersAt = cfg.Now()
	t.refresh()
	t.animate()

	// Registration runs beside the loop: a host may call back (GetAll) before it answers.
	registered := make(chan error)
	stop := make(chan struct{})
	defer close(stop)
	register := func() {
		t.writeIcons()
		go func() {
			_, err := bus.Call(callTimeout, watcherName, watcherPath, watcherName, "RegisterStatusNotifierItem", "s", name)
			select {
			case registered <- err:
			case <-stop:
			}
		}()
	}
	if t.watcher {
		register()
	}
	deadline := time.NewTimer(cfg.WatcherWait)
	defer deadline.Stop()
	retry := time.NewTimer(time.Hour)
	retry.Stop()
	defer retry.Stop()

	for !t.quit {
		select {
		case <-ctx.Done():
			return nil
		case <-bus.Done():
			t.logf("the session bus closed")
			return nil
		case m := <-bus.Incoming():
			t.handle(m, register)
		case <-cfg.Frames:
			t.tick++
			if t.tick%2 == 0 {
				t.askHolders(false)
				t.refresh()
			}
			t.animate()
		case res := <-t.holdersCh:
			t.asking = false
			t.holders, t.holdersErr = res.holders, res.err
			t.refresh()
			t.animate()
		case err := <-registered:
			switch {
			case err == nil && !t.registered:
				t.registered = true
				if t.everShown {
					t.logf("the tray host is back; showing the tray icon again")
				} else {
					t.logf("showing the tray icon")
				}
				t.everShown = true
			case err != nil:
				if t.everShown && t.lastErr == nil {
					t.logf("could not register with the tray host (%v); trying again", err)
				}
				t.lastErr = err
				if t.watcher {
					retry.Reset(retryAfter)
				}
			}
			if err == nil {
				t.lastErr = nil
			}
		case <-retry.C:
			if t.watcher {
				register()
			}
		case <-deadline.C:
			switch {
			case t.everShown:
			case t.lastErr != nil:
				t.logf("the tray host did not take the icon (%v); no tray icon", t.lastErr)
				return nil
			default:
				t.logf("no tray host (%s) on the session bus; no tray icon. XFCE: add the Status Tray panel item; GNOME: enable the AppIndicator extension", watcherName)
				return nil
			}
		}
	}
	return nil
}

// writeIcons writes the icon files once, before the first registration, so a machine without a
// tray host writes nothing. Only into an existing runtime directory, which the system creates.
func (t *tray) writeIcons() {
	if t.iconsTried {
		return
	}
	t.iconsTried = true
	if info, err := os.Stat(t.cfg.RuntimeDir); err != nil || !info.IsDir() || !filepath.IsAbs(t.cfg.RuntimeDir) {
		return
	}
	root := filepath.Join(t.cfg.RuntimeDir, "bulchimbeon", "tray-icons")
	if err := writeIcons(root); err != nil {
		t.logf("icon files: %v; using pixmaps only", err)
		return
	}
	t.iconRoot = root
	t.frameKey = "" // name the current frame
	t.animate()
}

// askHolders asks logind for its lock listing beside the loop, at most every holdersEvery
// unless the menu is opening; the answer arrives on holdersCh.
func (t *tray) askHolders(menuOpening bool) {
	now := t.cfg.Now()
	if t.asking || (!menuOpening && now.Sub(t.holdersAt) < holdersEvery) {
		return
	}
	t.asking = true
	t.holdersAt = now
	ask, out := t.cfg.Holders, t.holdersCh
	go func() {
		holders, err := ask()
		out <- holdersResult{holders, err}
	}()
}

// refresh re-reads the settings, the state file and the duty log, and tells the host what changed.
func (t *tray) refresh() bool {
	if s, _, err := settings.Load(t.cfg.Paths.Settings); err == nil {
		t.settings = s // a broken file keeps the last good settings, as in the daemon
	}
	st, err := daemon.ReadState(t.cfg.Paths.State)
	var logTail []string
	if err != nil {
		st = nil
	} else {
		logTail = daemon.Tail(st.LogPath, 3)
	}
	v := buildView(snapshot{state: st, statePath: t.cfg.Paths.State, settings: t.settings, holders: t.holders, holdersErr: t.holdersErr,
		logTail: logTail, lang: daemon.ResolveLang(t.cfg.Lang, t.settings, t.cfg.Getenv), now: t.cfg.Now(), alive: t.cfg.Alive, home: t.cfg.Home})
	old := t.view
	t.view = v
	if old.lang != v.lang {
		t.emit(itemPath, itemIface, "NewTitle", "")
	}
	if old.active != v.active {
		t.emit(itemPath, itemIface, "NewStatus", "s", v.itemStatus())
	}
	if sameMenu(old, v) {
		return false
	}
	t.emit(itemPath, itemIface, "NewToolTip", "")
	t.revision++
	t.emit(menuPath, menuIface, "LayoutUpdated", "ui", t.revision, idRoot)
	return true
}

// animate shows the frame for this tick, as the macOS menu bar does: frame tick % count.
func (t *tray) animate() {
	theme := sprites.Named(t.view.theme)
	frames := theme.Frames(t.view.pose)
	rows := frames[t.tick%len(frames)]
	key := theme.ID + "/" + strings.Join(rows, "")
	if key == t.frameKey {
		return
	}
	t.frameKey = key
	if t.iconRoot != "" {
		t.iconName = iconName(theme, rows)
	}
	if cached, ok := t.cache[key]; ok {
		t.pixmaps = cached
	} else {
		t.pixmaps = pixmaps(theme, rows)
		t.cache[key] = t.pixmaps
	}
	t.emit(itemPath, itemIface, "NewIcon", "")
}

func (t *tray) emit(path dbus.ObjectPath, iface, member, sig string, args ...any) {
	if err := t.bus.Emit(path, iface, member, sig, args...); err != nil && !errors.Is(err, dbus.ErrClosed) {
		t.logf("%s: %v", member, err)
	}
}

// pixmaps is the IconPixmap value, a(iiay): ARGB32 in network byte order, one per size.
func pixmaps(theme sprites.Theme, rows []string) []any {
	var out []any
	for _, size := range IconSizes {
		img := theme.Render(rows, size)
		argb := make([]byte, 0, size*size*4)
		for i := 0; i < len(img.Pix); i += 4 {
			argb = append(argb, img.Pix[i+3], img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		}
		out = append(out, []any{int32(size), int32(size), argb})
	}
	return out
}

// itemProperties are the org.kde.StatusNotifierItem properties.
func (t *tray) itemProperties() map[string]dbus.Variant {
	v := t.view
	none := dbus.MakeVariant("a(iiay)", []any{})
	return map[string]dbus.Variant{
		"Category":            str("ApplicationStatus"),
		"Id":                  str("bulchimbeon"),
		"Title":               str(engine.T(v.lang, "불침번", "Bulchimbeon", "Bulchimbeon")),
		"Status":              str(v.itemStatus()),
		"WindowId":            dbus.MakeVariant("i", int32(0)),
		"IconName":            str(t.iconName),
		"IconThemePath":       str(t.iconRoot),
		"IconPixmap":          dbus.MakeVariant("a(iiay)", t.pixmaps),
		"OverlayIconName":     str(""),
		"OverlayIconPixmap":   none,
		"AttentionIconName":   str(""),
		"AttentionIconPixmap": none,
		"AttentionMovieName":  str(""),
		"ToolTip":             dbus.MakeVariant("(sa(iiay)ss)", []any{"", []any{}, v.headline, v.tooltip()}),
		// for screen readers, as the macOS menu bar label
		"IconAccessibleDesc":      str(v.headline),
		"AttentionAccessibleDesc": str(""),
		"ItemIsMenu":              dbus.MakeVariant("b", true),
		"Menu":                    dbus.MakeVariant("o", menuPath),
	}
}

// menuProperties are the com.canonical.dbusmenu properties.
func menuProperties() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"Version":       dbus.MakeVariant("u", uint32(3)),
		"TextDirection": str("ltr"),
		"Status":        str("normal"),
		"IconThemePath": dbus.MakeVariant("as", []string{}),
	}
}

// handle answers one incoming method call or signal.
func (t *tray) handle(m *dbus.Message, register func()) {
	if m.Type == dbus.TypeSignal {
		if m.Sender == dbus.BusName && m.Interface == dbus.BusInterface && m.Member == "NameOwnerChanged" &&
			m.Signature == "sss" && m.Body[0] == watcherName {
			t.watcher = m.Body[2] != ""
			if !t.watcher {
				if t.registered {
					t.logf("the tray host went away; waiting for it to come back")
				}
				t.registered = false
			} else {
				register()
			}
		}
		return
	}
	if m.Type != dbus.TypeMethodCall {
		return
	}
	args := func(sig string) bool {
		if m.Signature != sig {
			t.bus.ReplyError(m, errArgs, "expected signature "+sig)
			return false
		}
		return true
	}
	switch {
	case m.Interface == peerIface && m.Member == "Ping":
		t.bus.Reply(m, "")
	case m.Interface == introIface && m.Member == "Introspect":
		if xml, ok := introspection[m.Path]; ok {
			t.bus.Reply(m, "s", xml)
		} else {
			t.bus.ReplyError(m, "org.freedesktop.DBus.Error.UnknownObject", "no object "+string(m.Path))
		}
	case m.Interface == propsIface && (m.Path == itemPath || m.Path == menuPath):
		t.properties(m, args)
	case m.Path == itemPath && (m.Interface == itemIface || m.Interface == ""):
		switch m.Member {
		case "Activate", "SecondaryActivate", "ContextMenu":
			if args("ii") {
				t.bus.Reply(m, "") // the host shows the menu (ItemIsMenu)
			}
		case "Scroll":
			if args("is") {
				t.bus.Reply(m, "")
			}
		default:
			t.bus.ReplyError(m, errUnknown, "no method "+m.Member)
		}
	case m.Path == menuPath && (m.Interface == menuIface || m.Interface == ""):
		t.menuCall(m, args)
	default:
		t.bus.ReplyError(m, errUnknown, "no method "+m.Interface+"."+m.Member+" on "+string(m.Path))
	}
}

func (t *tray) properties(m *dbus.Message, args func(string) bool) {
	all := map[dbus.ObjectPath]func() (string, map[string]dbus.Variant){
		itemPath: func() (string, map[string]dbus.Variant) { return itemIface, t.itemProperties() },
		menuPath: func() (string, map[string]dbus.Variant) { return menuIface, menuProperties() },
	}
	iface, props := all[m.Path]()
	switch m.Member {
	case "GetAll":
		if args("s") {
			switch m.Body[0] {
			case propsIface, introIface, peerIface:
				// the standard interfaces have no properties (GDBus proxies ask anyway)
				t.bus.Reply(m, "a{sv}", map[string]dbus.Variant{})
				return
			}
			if m.Body[0] != iface {
				t.bus.ReplyError(m, errInterface, "no interface "+m.Body[0].(string))
				return
			}
			t.bus.Reply(m, "a{sv}", props)
		}
	case "Get":
		if args("ss") {
			v, ok := props[m.Body[1].(string)]
			if m.Body[0] != iface || !ok {
				t.bus.ReplyError(m, errProperty, "no property "+m.Body[1].(string))
				return
			}
			t.bus.Reply(m, "v", v)
		}
	case "Set":
		t.bus.ReplyError(m, errReadOnly, "the tray's properties are read-only")
	default:
		t.bus.ReplyError(m, errUnknown, "no method "+m.Member)
	}
}

func (t *tray) menuCall(m *dbus.Message, args func(string) bool) {
	root := t.view.menu()
	switch m.Member {
	case "GetLayout":
		if !args("iias") {
			return
		}
		node, ok := root.find(m.Body[0].(int32))
		if !ok {
			t.bus.ReplyError(m, errArgs, "no menu item")
			return
		}
		t.bus.Reply(m, "u(ia{sv}av)", t.revision, node.layout(m.Body[1].(int32), strings1(m.Body[2])))
	case "GetGroupProperties":
		if !args("aias") {
			return
		}
		names := strings1(m.Body[1])
		out := []any{}
		for _, id := range m.Body[0].([]any) {
			if node, ok := root.find(id.(int32)); ok {
				out = append(out, []any{node.id, node.properties(names)})
			}
		}
		t.bus.Reply(m, "a(ia{sv})", out)
	case "GetProperty":
		if !args("is") {
			return
		}
		node, ok := root.find(m.Body[0].(int32))
		v, has := node.props[m.Body[1].(string)]
		if !ok || !has {
			t.bus.ReplyError(m, errArgs, "no such menu item property")
			return
		}
		t.bus.Reply(m, "v", v)
	case "Event":
		if args("isvu") {
			t.event(m.Body[0].(int32), m.Body[1].(string))
			t.bus.Reply(m, "")
		}
	case "EventGroup":
		if !args("a(isvu)") {
			return
		}
		missing := []int32{}
		for _, e := range m.Body[0].([]any) {
			e := e.([]any)
			if _, ok := root.find(e[0].(int32)); !ok {
				missing = append(missing, e[0].(int32))
				continue
			}
			t.event(e[0].(int32), e[1].(string))
		}
		t.bus.Reply(m, "ai", missing)
	case "AboutToShow":
		if args("i") {
			t.askHolders(true)
			t.bus.Reply(m, "b", t.refresh())
		}
	case "AboutToShowGroup":
		if args("ai") {
			t.askHolders(true)
			changed := t.refresh()
			updates := []int32{}
			if changed {
				updates = append(updates, idRoot)
			}
			t.bus.Reply(m, "aiai", updates, []int32{})
		}
	default:
		t.bus.ReplyError(m, errUnknown, "no method "+m.Member)
	}
}

// event carries out a click: a uniform is written to the settings file (the daemon and the
// tray re-read it), quit ends the tray only.
func (t *tray) event(id int32, kind string) {
	if kind != "clicked" {
		return
	}
	switch themes := sprites.Themes(); {
	case id == idQuit:
		t.logf("quit from the menu; the daemon keeps running")
		t.quit = true
	case id >= idTheme && int(id-idTheme) < len(themes):
		theme := themes[id-idTheme].ID
		if err := settings.Set(t.cfg.Paths.Settings, engine.KeyUniformTheme, []string{theme}); err != nil {
			t.logf("uniform: %v", err)
			return
		}
		t.refresh()
		t.animate()
	}
}

func strings1(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}
