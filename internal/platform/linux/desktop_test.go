//go:build linux

package linux

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/dbus"
	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// fakeSession is a session bus with desktop services on it. Like gnome-session, PowerDevil and
// xfce4-power-manager it keeps each inhibitor with the connection that asked for it and drops it
// when that connection closes or the service goes away.
type fakeSession struct {
	mu        sync.Mutex
	owners    map[string]string // well-known name → unique name
	inhibits  map[uint32]fakeInhibitor
	cookie    uint32
	refuse    map[string]error // Inhibit fails on this service
	kde       bool             // the freedesktop service answers with an int32 cookie, as KDE's does
	unblocked bool             // the session says suspend is not blocked (a user override)
	dialErr   error
	conns     []*fakeConn
	calls     []string // "member dest"
}

type fakeInhibitor struct {
	conn    int
	service string
	flags   uint32
}

type fakeConn struct {
	s    *fakeSession
	id   int
	in   chan *dbus.Message
	done chan struct{}
	once sync.Once

	mu     sync.Mutex
	errors []string // ReplyError names sent
}

func newFakeSession(owners map[string]string) *fakeSession {
	return &fakeSession{owners: owners, inhibits: map[uint32]fakeInhibitor{}, refuse: map[string]error{}}
}

func (s *fakeSession) dial() (Bus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dialErr != nil {
		return nil, s.dialErr
	}
	c := &fakeConn{s: s, id: len(s.conns) + 1, in: make(chan *dbus.Message, 4), done: make(chan struct{})}
	s.conns = append(s.conns, c)
	return c, nil
}

// setOwner moves a well-known name to another connection ("" when the service exits); the
// old owner's inhibitors go with it.
func (s *fakeSession) setOwner(name, unique string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if unique == "" {
		delete(s.owners, name)
	} else {
		s.owners[name] = unique
	}
	for cookie, in := range s.inhibits {
		if in.service == name {
			delete(s.inhibits, cookie)
		}
	}
}

func (s *fakeSession) held() []fakeInhibitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []fakeInhibitor
	for _, in := range s.inhibits {
		out = append(out, in)
	}
	return out
}

func (s *fakeSession) count(member string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if strings.HasPrefix(c, member+" ") {
			n++
		}
	}
	return n
}

func (c *fakeConn) Call(_ time.Duration, dest string, path dbus.ObjectPath, iface, member, sig string, args ...any) ([]any, error) {
	s := c.s
	select {
	case <-c.done:
		return nil, dbus.ErrClosed
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, member+" "+dest)
	unknown := &dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownMethod", Message: member}
	if dest == dbus.BusName {
		if member != "GetNameOwner" || sig != "s" {
			return nil, unknown
		}
		if owner := s.owners[args[0].(string)]; owner != "" {
			return []any{owner}, nil
		}
		return nil, &dbus.Error{Name: "org.freedesktop.DBus.Error.NameHasNoOwner"}
	}
	if s.owners[dest] == "" {
		return nil, &dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}
	}
	gnome := dest == "org.gnome.SessionManager" && path == "/org/gnome/SessionManager" && iface == dest
	fdo := dest == "org.freedesktop.PowerManagement" && path == "/org/freedesktop/PowerManagement/Inhibit" &&
		iface == "org.freedesktop.PowerManagement.Inhibit"
	blocked := func(flag uint32) bool {
		for _, in := range s.inhibits {
			if in.service == dest && in.flags&flag != 0 {
				return !s.unblocked
			}
		}
		return false
	}
	switch {
	case (gnome && member == "Inhibit" && sig == "susu") || (fdo && member == "Inhibit" && sig == "ss"):
		if err := s.refuse[dest]; err != nil {
			return nil, err
		}
		reason := args[1]
		if gnome {
			reason = args[2]
		}
		if args[0] != InhibitApp || reason != InhibitWhy {
			return nil, errors.New("bad Inhibit arguments")
		}
		flags := uint32(4)
		if gnome {
			flags = args[3].(uint32)
		}
		s.cookie++
		s.inhibits[s.cookie] = fakeInhibitor{conn: c.id, service: dest, flags: flags}
		if fdo && s.kde {
			return []any{int32(s.cookie)}, nil
		}
		return []any{s.cookie}, nil
	case (gnome && member == "Uninhibit" || fdo && member == "UnInhibit") && sig == "u":
		delete(s.inhibits, args[0].(uint32))
		return nil, nil
	case gnome && member == "IsInhibited" && sig == "u":
		return []any{blocked(args[0].(uint32))}, nil
	case fdo && member == "HasInhibit" && sig == "":
		return []any{blocked(^uint32(0))}, nil
	}
	return nil, unknown
}

func (c *fakeConn) ReplyError(call *dbus.Message, name, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, name)
	return nil
}

func (c *fakeConn) Incoming() <-chan *dbus.Message { return c.in }
func (c *fakeConn) Done() <-chan struct{}          { return c.done }

// Close drops the connection's inhibitors, as the desktop does when a client disconnects.
func (c *fakeConn) Close() error {
	c.once.Do(func() {
		close(c.done)
		c.s.mu.Lock()
		defer c.s.mu.Unlock()
		for cookie, in := range c.s.inhibits {
			if in.conn == c.id {
				delete(c.s.inhibits, cookie)
			}
		}
	})
	return nil
}

func (c *fakeConn) isClosed() bool { return closed(c) }

const (
	gnomeName = "org.gnome.SessionManager"
	fdoName   = "org.freedesktop.PowerManagement"
)

// newTestLock returns a lock on session whose logind fallback is a fake systemd-inhibit.
func newTestLock(t *testing.T, session *fakeSession) (*Lock, *time.Time) {
	now := time.Unix(1_792_740_000, 0)
	logind := &SystemdInhibitor{Tool: fakeInhibit(t, false, ""), Cat: findTool("cat"), Mode: "block", ConfirmTimeout: 3e9, UnconfirmedWait: 2e8}
	l := &Lock{Dial: session.dial, Logind: logind, Now: func() time.Time { return now }}
	t.Cleanup(l.Release)
	return l, &now
}

func logindHeld(l *Lock) bool {
	b, _ := l.Logind.Status().Block("sleep")
	return b.Held
}

func TestLockUsesGnomeSessionWhenItIsThere(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10", fdoName: ":1.11"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	st := l.Status()
	sleep, _ := st.Block("sleep")
	if st.Method != platform.MethodGnome || !st.Wanted || !st.Complete() || !sleep.Confirmed || sleep.PID != 0 || st.Fallback != "" {
		t.Fatalf("status = %+v", st)
	}
	held := session.held()
	if len(held) != 1 || held[0].service != gnomeName || held[0].flags != 4 {
		t.Fatalf("inhibitors = %+v (suspend only, never idle)", held)
	}
	if logindHeld(l) || l.Logind.Status().Wanted {
		t.Error("logind's lock was taken beside the desktop's")
	}
}

func TestLockUsesFreedesktopOnKDEAndXFCE(t *testing.T) {
	for desktop, owners := range map[string]map[string]string{
		"kde":  {fdoName: ":1.20", "org.kde.Solid.PowerManagement": ":1.20"},
		"xfce": {fdoName: ":1.30", "org.xfce.PowerManager": ":1.30"},
		"":     {fdoName: ":1.40", "org.xfce.PowerManager": ":1.41"}, // a different owner says nothing
	} {
		session := newFakeSession(owners)
		session.kde = desktop == "kde"
		l, _ := newTestLock(t, session)
		l.Acquire()
		st := l.Status()
		if st.Method != platform.MethodFreedesktop || st.Desktop != desktop || !st.Complete() || logindHeld(l) {
			t.Errorf("%q: status = %+v", desktop, st)
		}
		if held := session.held(); len(held) != 1 || held[0].service != fdoName {
			t.Errorf("%q: inhibitors = %+v", desktop, held)
		}
		l.Release()
		if held := session.held(); len(held) != 0 {
			t.Errorf("%q: Release left %+v (KDE's int32 cookie must round-trip)", desktop, held)
		}
	}
}

func TestLockFallsBackToLogind(t *testing.T) {
	// headless or SSH: a session bus with no desktop on it
	session := newFakeSession(map[string]string{})
	l, _ := newTestLock(t, session)
	l.Acquire()
	st := l.Status()
	if st.Method != platform.MethodLogind || st.Mode != "block" || st.Fallback != platform.FallbackNoDesktop || !st.Complete() || !logindHeld(l) {
		t.Fatalf("no desktop: %+v", st)
	}
	l.Release()

	// no session bus at all
	session.dialErr = errors.New("no session bus (DBUS_SESSION_BUS_ADDRESS is not set)")
	l.Acquire()
	if st := l.Status(); st.Fallback != platform.FallbackNoBus || !strings.Contains(st.Detail, "DBUS_SESSION_BUS_ADDRESS") || !logindHeld(l) {
		t.Fatalf("no bus: %+v", st)
	}
	l.Release()

	// the desktop refuses: the next interface is tried, then logind
	session.dialErr = nil
	session.owners = map[string]string{gnomeName: ":1.10", fdoName: ":1.11"}
	session.refuse[gnomeName] = &dbus.Error{Name: "org.gnome.SessionManager.GeneralError", Message: "not running"}
	l.Acquire()
	if st := l.Status(); st.Method != platform.MethodFreedesktop {
		t.Fatalf("gnome refused, freedesktop there: %+v", st)
	}
	l.Release()
	session.refuse[fdoName] = errors.New("nope")
	l.Acquire()
	if st := l.Status(); st.Fallback != platform.FallbackRefused || !strings.Contains(st.Detail, "not running") || !logindHeld(l) {
		t.Fatalf("both refused: %+v", st)
	}
}

// Stand-down releases the inhibitor and closes the connection, so nothing of ours is left on
// the session bus while off duty.
func TestLockReleaseClosesTheConnection(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	conn := session.conns[0]
	l.Release()
	if len(session.held()) != 0 || session.count("Uninhibit") != 1 || !conn.isClosed() || l.Status().Wanted {
		t.Fatalf("held=%+v uninhibit=%d closed=%v", session.held(), session.count("Uninhibit"), conn.isClosed())
	}
	if l.Refresh() {
		t.Error("Refresh off duty reported a loss")
	}
	// a crashed daemon never calls Uninhibit: the closed connection alone releases it
	l.Acquire()
	session.conns[1].Close()
	if len(session.held()) != 0 {
		t.Fatal("the session kept an inhibitor of a closed connection")
	}
}

// A desktop that starts after the daemon went on duty takes the lock over from logind.
func TestLockMovesToADesktopThatStartsLater(t *testing.T) {
	session := newFakeSession(map[string]string{})
	l, _ := newTestLock(t, session)
	l.Acquire()
	logindPID := func() int { b, _ := l.Logind.Status().Block("sleep"); return b.PID }()
	if !logindHeld(l) || logindPID == 0 {
		t.Fatal("logind not held without a desktop")
	}
	session.setOwner(gnomeName, ":1.50") // GNOME login
	if l.Refresh() {
		t.Error("taking over was reported as a lost lock")
	}
	if st := l.Status(); st.Method != platform.MethodGnome || len(session.held()) != 1 {
		t.Fatalf("status = %+v", st)
	}
	if logindHeld(l) || alive(logindPID) {
		t.Error("logind's helper kept running after the desktop took over")
	}
}

// Logout: the desktop's inhibitor goes with the session, and logind takes the lock back at once.
func TestLockMovesBackToLogindAtLogout(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	session.setOwner(gnomeName, "")
	if l.Refresh() {
		t.Error("logout was reported as a lost lock")
	}
	if st := l.Status(); st.Method != platform.MethodLogind || st.Fallback != platform.FallbackNoDesktop || !logindHeld(l) {
		t.Fatalf("status = %+v", st)
	}
}

// A restarted session manager never saw our inhibitor: take a new one from it.
func TestLockReinhibitsARestartedSessionManager(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	session.setOwner(gnomeName, ":1.77")
	if l.Refresh() {
		t.Error("restart reported as lost")
	}
	held := session.held()
	if len(held) != 1 || l.Status().Method != platform.MethodGnome || session.count("Inhibit") != 2 || logindHeld(l) {
		t.Fatalf("held = %+v, Inhibit calls = %d", held, session.count("Inhibit"))
	}
}

// A lost bus connection drops the inhibitor; the lock dials again and takes a new one.
func TestLockRedialsALostConnection(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	session.conns[0].Close()
	if l.Refresh() {
		t.Error("a lost connection that could be replaced was reported as lost")
	}
	if len(session.conns) != 2 || len(session.held()) != 1 || session.held()[0].conn != 2 {
		t.Fatalf("conns = %d, held = %+v", len(session.conns), session.held())
	}
}

// The user can override an inhibitor (KDE's battery applet). The lock reports it unconfirmed and
// does not take it again, which would undo the user's choice every few seconds.
func TestLockRespectsASessionThatOverridesIt(t *testing.T) {
	session := newFakeSession(map[string]string{fdoName: ":1.20", "org.kde.Solid.PowerManagement": ":1.20"})
	l, _ := newTestLock(t, session)
	l.Acquire()
	session.unblocked = true
	if l.Refresh() {
		t.Error("reported as lost")
	}
	sleep, _ := l.Status().Block("sleep")
	if !sleep.Held || sleep.Confirmed || session.count("Inhibit") != 1 {
		t.Fatalf("sleep = %+v, Inhibit calls = %d", sleep, session.count("Inhibit"))
	}
	session.unblocked = false
	l.Refresh()
	if sleep, _ := l.Status().Block("sleep"); !sleep.Confirmed {
		t.Error("not confirmed again")
	}
}

// Without a session bus the lock asks again only every redialEvery, not on every check.
func TestLockRedialsAMissingBusSlowly(t *testing.T) {
	session := newFakeSession(map[string]string{})
	session.dialErr = errors.New("no session bus")
	l, now := newTestLock(t, session)
	l.Acquire()
	session.dialErr = nil
	session.setOwner(gnomeName, ":1.10")
	*now = now.Add(5 * time.Second)
	l.Refresh()
	if l.Status().Method != platform.MethodLogind || len(session.conns) != 0 {
		t.Fatal("dialed again within redialEvery")
	}
	*now = now.Add(redialEvery)
	l.Refresh()
	if l.Status().Method != platform.MethodGnome {
		t.Fatalf("status = %+v", l.Status())
	}
}

// The daemon is a client only: it owns no name, exports nothing, and calls nothing but the
// inhibitor methods. Calls that arrive get an error.
func TestLockIsAClientOnly(t *testing.T) {
	session := newFakeSession(map[string]string{gnomeName: ":1.10", fdoName: ":1.11"})
	l, now := newTestLock(t, session)
	l.Acquire()
	conn := session.conns[0]
	conn.in <- &dbus.Message{Type: dbus.TypeMethodCall, Serial: 7, Sender: ":1.99", Path: "/", Member: "Introspect"}
	conn.in <- &dbus.Message{Type: dbus.TypeSignal, Sender: dbus.BusName, Path: dbus.BusPath, Member: "NameAcquired"}
	for i := 0; i < 50; i++ {
		conn.mu.Lock()
		n := len(conn.errors)
		conn.mu.Unlock()
		if n > 0 {
			break
		}
		sleepBriefly()
	}
	conn.mu.Lock()
	if !slices.Equal(conn.errors, []string{"org.freedesktop.DBus.Error.UnknownMethod"}) {
		t.Errorf("replies = %q", conn.errors)
	}
	conn.mu.Unlock()
	*now = now.Add(time.Minute)
	l.Refresh()
	session.setOwner(gnomeName, "")
	l.Refresh()
	l.Release()
	allowed := []string{"GetNameOwner", "Inhibit", "Uninhibit", "UnInhibit", "IsInhibited", "HasInhibit"}
	for _, call := range session.calls {
		if !slices.Contains(allowed, strings.Fields(call)[0]) {
			t.Errorf("unexpected call %q", call)
		}
	}
}

// doctor's check takes and drops the inhibitor the daemon would take.
func TestTryDesktop(t *testing.T) {
	session := newFakeSession(map[string]string{fdoName: ":1.30", "org.xfce.PowerManager": ":1.30"})
	st := tryDesktop(session.dial)
	if st.Method != platform.MethodFreedesktop || st.Desktop != "xfce" || len(session.held()) != 0 || !session.conns[0].isClosed() {
		t.Fatalf("status = %+v, held = %+v", st, session.held())
	}
	if st := tryDesktop(newFakeSession(map[string]string{}).dial); st.OnDesktop() || st.Fallback != platform.FallbackNoDesktop {
		t.Errorf("no desktop: %+v", st)
	}
}

// TestLockAgainstThisSession takes and drops the desktop inhibitor of the session this test
// runs in. Run it inside a desktop session with BULCHIMBEON_TEST_DESKTOP=1.
func TestLockAgainstThisSession(t *testing.T) {
	if os.Getenv("BULCHIMBEON_TEST_DESKTOP") != "1" {
		t.Skip("set BULCHIMBEON_TEST_DESKTOP=1 to take this desktop session's inhibitor")
	}
	st := tryDesktop(DialSessionBus)
	t.Logf("status: %+v", st)
	if !st.OnDesktop() {
		t.Fatalf("no desktop inhibitor: %s", platform.FallbackText(st, engine.EN))
	}
}
