//go:build linux

package linux

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/dbus"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// The desktop inhibitors are the session's own "do not suspend" request, which its power
// manager reads before it suspends an idle machine: gsd-power (GNOME), csd-power (Cinnamon) and
// mate-power-manager read gnome-session's, and KDE's PowerDevil and xfce4-power-manager serve
// org.freedesktop.PowerManagement. A logind lock does not stop them on systemd 256 and older,
// because they ask logind to suspend as the user and logind skips the caller's own block locks
// (measured on Ubuntu 24.04 GNOME: gsd-power suspended on schedule while the lock was held).
//
// Both are tied to the connection that asked for them: the session drops the inhibitor when that
// connection closes, so a daemon that dies, even by SIGKILL, releases it.

// Bus is the part of a session bus connection the lock uses (*dbus.Conn; a fake in tests).
type Bus interface {
	Call(timeout time.Duration, dest string, path dbus.ObjectPath, iface, member, sig string, args ...any) ([]any, error)
	ReplyError(call *dbus.Message, name, text string) error
	Incoming() <-chan *dbus.Message
	Done() <-chan struct{}
	Close() error
}

// InhibitApp is the application name the desktop inhibitors carry, as GNOME and KDE list them.
const InhibitApp = "bulchimbeon"

// gnomeSuspend is gnome-session's "inhibit suspending" flag. Not 8 (idle), which would keep
// the screen from dimming and locking.
const gnomeSuspend = uint32(4)

const (
	busTimeout = 3 * time.Second
	// redialEvery spaces the attempts to reach a session bus that is not there.
	redialEvery = 30 * time.Second
)

// desktopAPI is one desktop inhibitor interface.
type desktopAPI struct {
	method string // platform.LockStatus.Method
	name   string // the bus name the session serves it under
	path   dbus.ObjectPath
	iface  string
}

// desktopAPIs in the order they are tried. A desktop serves one of them.
var desktopAPIs = []desktopAPI{
	{platform.MethodGnome, "org.gnome.SessionManager", "/org/gnome/SessionManager", "org.gnome.SessionManager"},
	{platform.MethodFreedesktop, "org.freedesktop.PowerManagement", "/org/freedesktop/PowerManagement/Inhibit", "org.freedesktop.PowerManagement.Inhibit"},
}

// The power managers behind org.freedesktop.PowerManagement, by the name each also owns.
var freedesktopOwners = map[string]string{"org.kde.Solid.PowerManagement": "kde", "org.xfce.PowerManager": "xfce"}

func (a desktopAPI) inhibit(bus Bus) (uint32, error) {
	var reply []any
	var err error
	if a.method == platform.MethodGnome {
		reply, err = bus.Call(busTimeout, a.name, a.path, a.iface, "Inhibit", "susu", InhibitApp, uint32(0), InhibitWhy, gnomeSuspend)
	} else {
		reply, err = bus.Call(busTimeout, a.name, a.path, a.iface, "Inhibit", "ss", InhibitApp, InhibitWhy)
	}
	if err != nil {
		return 0, err
	}
	if len(reply) == 1 {
		switch c := reply[0].(type) {
		case uint32:
			return c, nil
		case int32: // KDE's adaptor
			return uint32(c), nil
		}
	}
	return 0, errors.New("unexpected Inhibit reply")
}

func (a desktopAPI) uninhibit(bus Bus, cookie uint32) {
	member := "UnInhibit"
	if a.method == platform.MethodGnome {
		member = "Uninhibit"
	}
	bus.Call(busTimeout, a.name, a.path, a.iface, member, "u", cookie)
}

// blocked asks whether the session blocks suspend now (ours or anyone's inhibitor).
func (a desktopAPI) blocked(bus Bus) (bool, error) {
	var reply []any
	var err error
	if a.method == platform.MethodGnome {
		reply, err = bus.Call(busTimeout, a.name, a.path, a.iface, "IsInhibited", "u", gnomeSuspend)
	} else {
		reply, err = bus.Call(busTimeout, a.name, a.path, a.iface, "HasInhibit", "")
	}
	if err != nil {
		return false, err
	}
	if len(reply) == 1 {
		if b, ok := reply[0].(bool); ok {
			return b, nil
		}
	}
	return false, errors.New("unexpected reply")
}

// nameOwner is the unique name that owns name, "" when nobody does.
func nameOwner(bus Bus, name string) (string, error) {
	reply, err := bus.Call(busTimeout, dbus.BusName, dbus.BusPath, dbus.BusInterface, "GetNameOwner", "s", name)
	var e *dbus.Error
	if errors.As(err, &e) && e.Name == "org.freedesktop.DBus.Error.NameHasNoOwner" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(reply) == 1 {
		if owner, ok := reply[0].(string); ok {
			return owner, nil
		}
	}
	return "", errors.New("unexpected GetNameOwner reply")
}

// desktopHold is a desktop inhibitor this daemon holds.
type desktopHold struct {
	api       desktopAPI
	owner     string // the unique name that granted it; a new owner never saw it
	cookie    uint32
	confirmed bool
	desktop   string
}

// Lock is the Linux sleep lock (platform.Inhibitor). On duty it holds the desktop session's
// suspend inhibitor when the session bus serves one, and a logind "sleep" lock otherwise:
// headless, SSH-only, or a desktop without either interface (sway, Hyprland). It holds no idle
// lock, so the screen dims and locks as the user set it ("Display may sleep").
//
// The daemon is a systemd user service, so the session bus is the user bus that the user's
// desktop sessions share; a desktop that starts or ends while the daemon is on duty is noticed
// on the next Refresh, and the lock moves to it or back to logind.
type Lock struct {
	// Dial connects to the session bus.
	Dial   func() (Bus, error)
	Logind *SystemdInhibitor
	Now    func() time.Time

	mu       sync.Mutex
	wanted   bool
	bus      Bus
	dialedAt time.Time
	held     *desktopHold
	fallback string
	detail   string
}

// NewLock returns the lock for this user's session bus, falling back to l.
func NewLock(l *SystemdInhibitor) *Lock {
	return &Lock{Logind: l, Now: time.Now, Dial: DialSessionBus}
}

// DialSessionBus connects to the session bus: DBUS_SESSION_BUS_ADDRESS, which the systemd user
// manager gives its services, or $XDG_RUNTIME_DIR/bus.
func DialSessionBus() (Bus, error) {
	address, err := dbus.SessionBusAddress(os.Getenv)
	if err != nil {
		return nil, err
	}
	conn, err := dbus.Dial(address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// tryDesktop takes a desktop inhibitor and drops it again, as the daemon would take it now
// (doctor's check). The status names the method, or says why there is none.
func tryDesktop(dial func() (Bus, error)) platform.LockStatus {
	l := &Lock{Dial: dial, Now: time.Now, wanted: true}
	if !l.takeDesktop(true) {
		if l.bus != nil {
			l.bus.Close()
		}
		return platform.LockStatus{Method: platform.MethodLogind, Fallback: l.fallback, Detail: l.detail}
	}
	h := l.held
	h.api.uninhibit(l.bus, h.cookie)
	l.bus.Close()
	return platform.LockStatus{Wanted: true, Method: h.api.method, Desktop: h.desktop,
		Blocks: []platform.Block{{What: "sleep", Held: true, Confirmed: h.confirmed}}}
}

// Acquire implements platform.Inhibitor.
func (l *Lock) Acquire() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.wanted = true
	l.acquire(true)
}

// acquire takes a desktop inhibitor if the session serves one, else logind's. A logind lock
// is released only once a desktop inhibitor is held, so the machine is never left unblocked.
func (l *Lock) acquire(dial bool) {
	if l.held == nil && l.takeDesktop(dial) {
		l.Logind.Release()
	}
	if l.held == nil {
		l.Logind.Acquire()
	}
}

// takeDesktop asks the session for a suspend inhibitor and records why it could not.
// Without dial, a missing bus is tried again only every redialEvery.
func (l *Lock) takeDesktop(dial bool) bool {
	if l.bus != nil && closed(l.bus) {
		l.bus = nil
	}
	if l.bus == nil {
		if !dial && l.Now().Sub(l.dialedAt) < redialEvery {
			return false
		}
		l.dialedAt = l.Now()
		bus, err := l.Dial()
		if err != nil {
			l.fallback, l.detail = platform.FallbackNoBus, err.Error()
			return false
		}
		l.bus = bus
		go drain(bus)
	}
	var refused []string
	for _, api := range desktopAPIs {
		owner, err := nameOwner(l.bus, api.name)
		if err != nil || owner == "" {
			continue
		}
		cookie, err := api.inhibit(l.bus)
		if err != nil {
			refused = append(refused, api.name+": "+err.Error())
			continue
		}
		h := &desktopHold{api: api, owner: owner, cookie: cookie}
		h.confirmed, _ = api.blocked(l.bus)
		if api.method == platform.MethodFreedesktop {
			for name, desktop := range freedesktopOwners {
				if o, err := nameOwner(l.bus, name); err == nil && o == owner {
					h.desktop = desktop
				}
			}
		}
		l.held, l.fallback, l.detail = h, "", ""
		return true
	}
	if len(refused) > 0 {
		l.fallback, l.detail = platform.FallbackRefused, strings.Join(refused, "; ")
	} else {
		l.fallback, l.detail = platform.FallbackNoDesktop, ""
	}
	return false
}

// drain answers what arrives on the connection. The daemon owns no name and exports nothing,
// so every method call gets an error and signals (the bus's NameAcquired) are dropped; without a
// reader the connection would stop delivering replies once its queue filled.
func drain(bus Bus) {
	for {
		select {
		case m := <-bus.Incoming():
			if m.Type == dbus.TypeMethodCall {
				bus.ReplyError(m, "org.freedesktop.DBus.Error.UnknownMethod", "the Bulchimbeon daemon exports nothing")
			}
		case <-bus.Done():
			return
		}
	}
}

func closed(bus Bus) bool {
	select {
	case <-bus.Done():
		return true
	default:
		return false
	}
}

// Release implements platform.Inhibitor. Closing the connection drops a desktop inhibitor
// even if Uninhibit got lost, and leaves nothing on the session bus while off duty.
func (l *Lock) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.wanted = false
	if h := l.held; h != nil && l.bus != nil && !closed(l.bus) {
		h.api.uninhibit(l.bus, h.cookie)
	}
	l.held = nil
	if l.bus != nil {
		l.bus.Close()
		l.bus = nil
	}
	l.Logind.Release()
	l.fallback, l.detail = "", ""
}

// Refresh implements platform.Inhibitor. A desktop inhibitor that went away with its session
// (logout, a restarted session manager, a lost bus) is replaced at once, by a new desktop
// inhibitor or logind's, and is not reported as lost; a desktop that appears while logind holds
// the lock takes it over. Only a logind helper that died is lost, as before.
func (l *Lock) Refresh() (lost bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.wanted {
		return false
	}
	if h := l.held; h != nil {
		if l.stillHeld(h) {
			return false
		}
		l.held = nil
		l.acquire(true)
		return false
	}
	if l.takeDesktop(false) {
		l.Logind.Release()
		return false
	}
	return l.Logind.Refresh()
}

// stillHeld reports whether the session that granted h still has it: our connection is open
// and the same owner serves the interface. Whether the session still blocks suspend only
// updates the confirmation, because a desktop can let the user override an inhibitor (KDE's
// battery applet), and taking it again would undo the user's choice every few seconds.
func (l *Lock) stillHeld(h *desktopHold) bool {
	if l.bus == nil || closed(l.bus) {
		return false
	}
	owner, err := nameOwner(l.bus, h.api.name)
	if err != nil {
		return true // a slow bus is not a lost session; ask again next time
	}
	if owner != h.owner {
		return false
	}
	if blocked, err := h.api.blocked(l.bus); err == nil {
		h.confirmed = blocked
	}
	return true
}

// Status implements platform.Inhibitor.
func (l *Lock) Status() platform.LockStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	if h := l.held; h != nil {
		return platform.LockStatus{Wanted: l.wanted, Method: h.api.method, Desktop: h.desktop,
			Blocks: []platform.Block{{What: "sleep", Held: true, Confirmed: h.confirmed}}}
	}
	st := l.Logind.Status()
	if l.wanted {
		st.Fallback, st.Detail = l.fallback, l.detail
	}
	return st
}
