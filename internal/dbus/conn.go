package dbus

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Bus names and paths of the message bus itself.
const (
	BusName      = "org.freedesktop.DBus"
	BusPath      = ObjectPath("/org/freedesktop/DBus")
	BusInterface = "org.freedesktop.DBus"
)

// Error is an error reply.
type Error struct {
	Name    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// ErrClosed is returned once the connection is gone.
var ErrClosed = errors.New("dbus: connection closed")

// SessionBusAddress is DBUS_SESSION_BUS_ADDRESS, or the systemd user bus socket
// ($XDG_RUNTIME_DIR/bus) when that is unset, as libdbus and GDBus do.
func SessionBusAddress(getenv func(string) string) (string, error) {
	if a := getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a, nil
	}
	if dir := getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		path := filepath.Join(dir, "bus")
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return "unix:path=" + path, nil
		}
	}
	return "", errors.New("no session bus (DBUS_SESSION_BUS_ADDRESS is not set)")
}

// Conn is a connection to a message bus. Its methods may be called from any goroutine.
type Conn struct {
	c    net.Conn
	r    *bufio.Reader
	wmu  sync.Mutex
	name string

	mu      sync.Mutex
	serial  uint32
	pending map[uint32]chan *Message
	err     error

	in   chan *Message
	done chan struct{}
}

// Dial connects to the first address that works, authenticates and says Hello.
func Dial(address string) (*Conn, error) {
	var errs []string
	for _, a := range strings.Split(address, ";") {
		if a == "" {
			continue
		}
		network, target, err := parseAddress(a)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		c, err := net.DialTimeout(network, target, 5*time.Second)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		conn, err := newConn(c)
		if err != nil {
			c.Close()
			errs = append(errs, err.Error())
			continue
		}
		return conn, nil
	}
	if len(errs) == 0 {
		return nil, errors.New("dbus: empty bus address")
	}
	return nil, errors.New(strings.Join(errs, "; "))
}

// parseAddress understands unix:path= and unix:abstract=, the transports of a session bus.
func parseAddress(a string) (network, target string, err error) {
	transport, rest, ok := strings.Cut(a, ":")
	if !ok || transport != "unix" {
		return "", "", fmt.Errorf("dbus: unsupported bus address %q", a)
	}
	for _, kv := range strings.Split(rest, ",") {
		k, v, _ := strings.Cut(kv, "=")
		v, err := unescape(v)
		if err != nil {
			return "", "", err
		}
		switch k {
		case "path":
			return "unix", v, nil
		case "abstract":
			return "unix", "@" + v, nil
		}
	}
	return "", "", fmt.Errorf("dbus: unsupported bus address %q", a)
}

func unescape(v string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '%' {
			b.WriteByte(v[i])
			continue
		}
		if i+2 >= len(v) {
			return "", errors.New("dbus: bad escape in bus address")
		}
		n, err := strconv.ParseUint(v[i+1:i+3], 16, 8)
		if err != nil {
			return "", errors.New("dbus: bad escape in bus address")
		}
		b.WriteByte(byte(n))
		i += 2
	}
	return b.String(), nil
}

// newConn authenticates on c, starts reading and says Hello.
func newConn(c net.Conn) (*Conn, error) {
	conn := &Conn{c: c, r: bufio.NewReader(c), pending: map[uint32]chan *Message{},
		in: make(chan *Message, 256), done: make(chan struct{})}
	c.SetDeadline(time.Now().Add(10 * time.Second))
	if err := conn.auth(); err != nil {
		return nil, err
	}
	c.SetDeadline(time.Time{})
	go conn.readLoop()
	reply, err := conn.Call(10*time.Second, BusName, BusPath, BusInterface, "Hello", "")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("dbus: Hello: %w", err)
	}
	name, ok := reply[0].(string)
	if len(reply) != 1 || !ok {
		conn.Close()
		return nil, errors.New("dbus: bad Hello reply")
	}
	conn.name = name
	return conn, nil
}

// auth runs the EXTERNAL mechanism with our uid, the one a session bus accepts.
func (conn *Conn) auth() error {
	uid := hex.EncodeToString([]byte(strconv.Itoa(os.Getuid())))
	if _, err := io.WriteString(conn.c, "\x00AUTH EXTERNAL "+uid+"\r\n"); err != nil {
		return err
	}
	line, err := conn.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("dbus: auth: %w", err)
	}
	if !strings.HasPrefix(line, "OK ") {
		return fmt.Errorf("dbus: auth refused: %s", strings.TrimSpace(line))
	}
	_, err = io.WriteString(conn.c, "BEGIN\r\n")
	return err
}

// UniqueName is the name the bus gave this connection.
func (conn *Conn) UniqueName() string { return conn.name }

// Incoming delivers method calls and signals addressed to this connection.
func (conn *Conn) Incoming() <-chan *Message { return conn.in }

// Done is closed when the connection is lost or closed.
func (conn *Conn) Done() <-chan struct{} { return conn.done }

// Close closes the connection.
func (conn *Conn) Close() error {
	conn.fail(ErrClosed)
	return nil
}

func (conn *Conn) fail(err error) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.err != nil {
		return
	}
	conn.err = err
	conn.c.Close()
	for serial, ch := range conn.pending {
		close(ch)
		delete(conn.pending, serial)
	}
	close(conn.done)
}

func (conn *Conn) readLoop() {
	for {
		m, err := ReadMessage(conn.r)
		if err != nil {
			conn.fail(err)
			return
		}
		switch m.Type {
		case TypeMethodReturn, TypeError:
			conn.mu.Lock()
			ch := conn.pending[m.ReplySerial]
			delete(conn.pending, m.ReplySerial)
			conn.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		case TypeMethodCall, TypeSignal:
			select {
			case conn.in <- m:
			case <-conn.done:
				return
			}
		}
	}
}

// write assigns the next serial, registers reply (if any) for it and writes m. Holding the
// write lock throughout keeps serials in the order they go out.
func (conn *Conn) write(m *Message, reply chan *Message) error {
	conn.wmu.Lock()
	defer conn.wmu.Unlock()
	conn.mu.Lock()
	if conn.err != nil {
		conn.mu.Unlock()
		return ErrClosed
	}
	conn.serial++
	if conn.serial == 0 {
		conn.serial = 1
	}
	m.Serial = conn.serial
	if reply != nil {
		conn.pending[m.Serial] = reply
	}
	conn.mu.Unlock()
	data, err := m.Marshal()
	if err != nil {
		conn.forget(m.Serial)
		return err
	}
	if _, err := conn.c.Write(data); err != nil {
		conn.fail(err)
		return err
	}
	return nil
}

func (conn *Conn) forget(serial uint32) {
	conn.mu.Lock()
	delete(conn.pending, serial)
	conn.mu.Unlock()
}

// Call calls a method and waits up to timeout for the reply's body.
func (conn *Conn) Call(timeout time.Duration, dest string, path ObjectPath, iface, member, sig string, args ...any) ([]any, error) {
	m := &Message{Type: TypeMethodCall, Destination: dest, Path: path, Interface: iface, Member: member, Signature: sig, Body: args}
	ch := make(chan *Message, 1)
	if err := conn.write(m, ch); err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		if reply.Type == TypeError {
			e := &Error{Name: reply.ErrorName}
			if len(reply.Body) > 0 {
				e.Message, _ = reply.Body[0].(string)
			}
			return nil, e
		}
		return reply.Body, nil
	case <-timer.C:
		conn.forget(m.Serial)
		return nil, fmt.Errorf("dbus: %s.%s: no reply within %v", iface, member, timeout)
	}
}

// Emit sends a signal.
func (conn *Conn) Emit(path ObjectPath, iface, member, sig string, args ...any) error {
	return conn.write(&Message{Type: TypeSignal, Path: path, Interface: iface, Member: member, Signature: sig, Body: args}, nil)
}

// Reply answers a method call, unless the caller asked for no reply.
func (conn *Conn) Reply(call *Message, sig string, args ...any) error {
	if call.Flags&FlagNoReplyExpected != 0 {
		return nil
	}
	return conn.write(&Message{Type: TypeMethodReturn, ReplySerial: call.Serial, Destination: call.Sender, Signature: sig, Body: args}, nil)
}

// ReplyError answers a method call with an error.
func (conn *Conn) ReplyError(call *Message, name, text string) error {
	if call.Flags&FlagNoReplyExpected != 0 {
		return nil
	}
	return conn.write(&Message{Type: TypeError, ReplySerial: call.Serial, Destination: call.Sender, ErrorName: name,
		Signature: "s", Body: []any{text}}, nil)
}
