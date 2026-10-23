package dbus

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A body with every type the tray sends: a dbusmenu layout, an icon pixmap list, a tooltip.
var complexSig = "u(ia{sv}av)a(iiay)(sa(iiay)ss)a{sv}vbnqxtdgoh"

func complexBody() []any {
	child := []any{int32(2), map[string]Variant{"label": MakeVariant("s", "Lock: held"), "enabled": MakeVariant("b", false)}, []Variant{}}
	return []any{
		uint32(7),
		[]any{int32(0), map[string]Variant{"children-display": MakeVariant("s", "submenu")}, []Variant{MakeVariant("(ia{sv}av)", child)}},
		[]any{[]any{int32(2), int32(1), []byte{0xFF, 1, 2, 3, 0, 0, 0, 0}}},
		[]any{"", []any{}, "🪖 Bulchimbeon — on duty", ""},
		map[string]Variant{"Version": MakeVariant("u", uint32(3)), "IconThemePath": MakeVariant("as", []string{})},
		MakeVariant("ai", []int32{1, -2}),
		true, int16(-3), uint16(4), int64(-5), uint64(6), 7.5, Signature("a{sv}"), ObjectPath("/MenuBar"), uint32(0),
	}
}

// What a decoded complexBody looks like: arrays and maps become []any.
func decodedComplexBody() []any {
	child := []any{int32(2), []any{[]any{"enabled", MakeVariant("b", false)}, []any{"label", MakeVariant("s", "Lock: held")}}, []any{}}
	return []any{
		uint32(7),
		[]any{int32(0), []any{[]any{"children-display", MakeVariant("s", "submenu")}}, []any{MakeVariant("(ia{sv}av)", child)}},
		[]any{[]any{int32(2), int32(1), []byte{0xFF, 1, 2, 3, 0, 0, 0, 0}}},
		[]any{"", []any{}, "🪖 Bulchimbeon — on duty", ""},
		[]any{[]any{"IconThemePath", MakeVariant("as", []any{})}, []any{"Version", MakeVariant("u", uint32(3))}},
		MakeVariant("ai", []any{int32(1), int32(-2)}),
		true, int16(-3), uint16(4), int64(-5), uint64(6), 7.5, Signature("a{sv}"), ObjectPath("/MenuBar"), uint32(0),
	}
}

func TestMessageRoundTrip(t *testing.T) {
	m := &Message{Type: TypeMethodReturn, Serial: 9, ReplySerial: 4, Destination: ":1.7", Signature: complexSig, Body: complexBody()}
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeMethodReturn || got.Serial != 9 || got.ReplySerial != 4 || got.Destination != ":1.7" || got.Signature != complexSig {
		t.Fatalf("header = %+v", got)
	}
	if !reflect.DeepEqual(got.Body, decodedComplexBody()) {
		t.Fatalf("body =\n%#v\nwant\n%#v", got.Body, decodedComplexBody())
	}
}

// The header of a Hello call, laid out by hand from the specification.
func TestHelloBytes(t *testing.T) {
	m := &Message{Type: TypeMethodCall, Serial: 1, Destination: BusName, Path: BusPath, Interface: BusInterface, Member: "Hello"}
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var want []byte
	want = append(want, 'l', 1, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0)
	var fields []byte
	field := func(code byte, sig, value string) {
		for len(fields)%8 != 0 {
			fields = append(fields, 0)
		}
		fields = append(fields, code, 1, sig[0], 0)
		fields = binary.LittleEndian.AppendUint32(fields, uint32(len(value)))
		fields = append(append(fields, value...), 0)
	}
	field(1, "o", "/org/freedesktop/DBus")
	field(2, "s", "org.freedesktop.DBus")
	field(3, "s", "Hello")
	field(6, "s", "org.freedesktop.DBus")
	want = binary.LittleEndian.AppendUint32(want, uint32(len(fields)))
	want = append(want, fields...)
	for len(want)%8 != 0 {
		want = append(want, 0)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("Hello =\n% x\nwant\n% x", data, want)
	}
}

func TestBigEndianMessagesAreRead(t *testing.T) {
	// a signal "s" body "hi" on /a, interface x.y, member Z, sent big-endian
	var fields []byte
	field := func(code byte, sig, value string) {
		for len(fields)%8 != 0 {
			fields = append(fields, 0)
		}
		fields = append(fields, code, 1, sig[0], 0)
		if sig == "g" {
			fields = append(append(fields, byte(len(value))), value...)
			fields = append(fields, 0)
			return
		}
		fields = binary.BigEndian.AppendUint32(fields, uint32(len(value)))
		fields = append(append(fields, value...), 0)
	}
	field(1, "o", "/a")
	field(2, "s", "x.y")
	field(3, "s", "Z")
	field(8, "g", "s")
	body := binary.BigEndian.AppendUint32(nil, 2)
	body = append(body, 'h', 'i', 0)
	data := []byte{'B', TypeSignal, 0, 1}
	data = binary.BigEndian.AppendUint32(data, uint32(len(body)))
	data = binary.BigEndian.AppendUint32(data, 5)
	data = binary.BigEndian.AppendUint32(data, uint32(len(fields)))
	data = append(data, fields...)
	for len(data)%8 != 0 {
		data = append(data, 0)
	}
	data = append(data, body...)
	m, err := ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if m.Path != "/a" || m.Interface != "x.y" || m.Member != "Z" || !reflect.DeepEqual(m.Body, []any{"hi"}) {
		t.Fatalf("got %+v", m)
	}
}

func TestEncodeRejectsBadValues(t *testing.T) {
	for name, c := range map[string]struct {
		sig  string
		args []any
	}{
		"too few":        {"ss", []any{"a"}},
		"too many":       {"s", []any{"a", "b"}},
		"none expected":  {"", []any{"a"}},
		"wrong type":     {"i", []any{"1"}},
		"bad path":       {"o", []any{ObjectPath("no/slash")}},
		"trailing slash": {"o", []any{ObjectPath("/a/")}},
		"nul in string":  {"s", []any{"a\x00b"}},
		"bad utf-8":      {"s", []any{"\xff"}},
		"variant sig":    {"v", []any{MakeVariant("ss", "a")}},
		"struct fields":  {"(ii)", []any{[]any{int32(1)}}},
		"bytes for as":   {"as", []any{[]byte{1}}},
	} {
		var e encoder
		if err := e.values(c.sig, c.args); err == nil {
			t.Errorf("%s: encoded", name)
		}
	}
}

func TestReadRejectsBrokenMessages(t *testing.T) {
	good, _ := (&Message{Type: TypeSignal, Serial: 1, Path: "/a", Interface: "x.y", Member: "Z", Signature: "s", Body: []any{"hi"}}).Marshal()
	cases := map[string][]byte{
		"short":       good[:10],
		"endianness":  append([]byte{'x'}, good[1:]...),
		"version":     append(append([]byte{}, good[:3]...), append([]byte{2}, good[4:]...)...),
		"cut body":    good[:len(good)-1],
		"huge length": append(append(append([]byte{}, good[:4]...), 0xFF, 0xFF, 0xFF, 0x7F), good[8:]...),
	}
	for name, data := range cases {
		if _, err := ReadMessage(bytes.NewReader(data)); err == nil {
			t.Errorf("%s: read", name)
		}
	}
}

func TestSignatures(t *testing.T) {
	for _, sig := range []string{"", "s", "a{sv}", "(ia{sv}av)", "a(iiay)", "aai", "a{s(ii)}"} {
		if !ValidSignature(sig) {
			t.Errorf("%q rejected", sig)
		}
	}
	for _, sig := range []string{"a", "(", "()", "a{vs}", "{sv}", "a{s}", "z", "(i", strings.Repeat("a", 33) + "i"} {
		if ValidSignature(sig) {
			t.Errorf("%q accepted", sig)
		}
	}
}

func TestAddresses(t *testing.T) {
	env := map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"}
	if a, err := SessionBusAddress(func(k string) string { return env[k] }); err != nil || a != "unix:path=/run/user/1000/bus" {
		t.Errorf("env address = %q, %v", a, err)
	}
	if _, err := SessionBusAddress(func(string) string { return "" }); err == nil {
		t.Error("no address and no runtime dir found a bus")
	}
	dir := t.TempDir()
	l, err := net.Listen("unix", filepath.Join(dir, "bus"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if a, err := SessionBusAddress(func(k string) string { return map[string]string{"XDG_RUNTIME_DIR": dir}[k] }); err != nil || a != "unix:path="+dir+"/bus" {
		t.Errorf("runtime dir address = %q, %v", a, err)
	}
	for addr, want := range map[string]string{
		"unix:path=/tmp/a%20b,guid=00": "/tmp/a b",
		"unix:abstract=/tmp/dbus-x":    "@/tmp/dbus-x",
	} {
		if _, target, err := parseAddress(addr); err != nil || target != want {
			t.Errorf("%s → %q, %v", addr, target, err)
		}
	}
	for _, addr := range []string{"tcp:host=localhost,port=1", "unix:tmpdir=/tmp", "unix:path=/a%2"} {
		if _, _, err := parseAddress(addr); err == nil {
			t.Errorf("%s accepted", addr)
		}
	}
}

// A server that refuses EXTERNAL fails Dial without hanging.
func TestAuthRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bus")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		r.ReadByte()
		r.ReadString('\n')
		c.Write([]byte("REJECTED DBUS_COOKIE_SHA1\r\n"))
		time.Sleep(100 * time.Millisecond)
	}()
	if _, err := Dial("unix:path=" + path); err == nil || !strings.Contains(err.Error(), "auth refused") {
		t.Fatalf("Dial = %v", err)
	}
}

// Against a real dbus-daemon, which checks every message it routes: two connections call each
// other with the tray's types and see each other's signals.
func TestRealBus(t *testing.T) {
	daemon := "/usr/bin/dbus-daemon"
	if _, err := os.Stat(daemon); err != nil {
		t.Skip("no dbus-daemon")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "session.conf")
	os.WriteFile(config, []byte(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig><type>session</type><listen>unix:path=`+dir+`/bus</listen><auth>EXTERNAL</auth>
<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow eavesdrop="true"/><allow own="*"/></policy></busconfig>`), 0o600)
	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--nopidfile", "--print-address=1")
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr = strings.TrimSpace(addr)

	a, err := Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if !strings.HasPrefix(a.UniqueName(), ":") || a.UniqueName() == b.UniqueName() {
		t.Fatalf("names %q %q", a.UniqueName(), b.UniqueName())
	}
	if r, err := a.Call(5*time.Second, BusName, BusPath, BusInterface, "RequestName", "su", "io.github.s3onghyun.Test", uint32(4)); err != nil || r[0] != uint32(1) {
		t.Fatalf("RequestName = %v, %v", r, err)
	}
	if _, err := b.Call(5*time.Second, BusName, BusPath, BusInterface, "AddMatch", "s",
		"type='signal',interface='io.github.s3onghyun.Test',member='Ping'"); err != nil {
		t.Fatal(err)
	}

	// a echoes every call back with the same body
	go func() {
		for m := range a.Incoming() {
			if m.Type == TypeMethodCall && m.Member == "Echo" {
				a.Reply(m, m.Signature, m.Body...)
			} else if m.Type == TypeMethodCall {
				a.ReplyError(m, "org.freedesktop.DBus.Error.UnknownMethod", "no "+m.Member)
			}
		}
	}()
	got, err := b.Call(5*time.Second, "io.github.s3onghyun.Test", "/StatusNotifierItem", "io.github.s3onghyun.Test", "Echo", complexSig, complexBody()...)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, decodedComplexBody()) {
		t.Fatalf("echo =\n%#v", got)
	}
	_, err = b.Call(5*time.Second, "io.github.s3onghyun.Test", "/", "io.github.s3onghyun.Test", "Nope", "")
	if e, ok := err.(*Error); !ok || e.Name != "org.freedesktop.DBus.Error.UnknownMethod" || e.Message != "no Nope" {
		t.Fatalf("error reply = %v", err)
	}

	if err := a.Emit("/StatusNotifierItem", "io.github.s3onghyun.Test", "Ping", "s", "hi"); err != nil {
		t.Fatal(err)
	}
	if m := waitSignal(t, b, "Ping"); m.Sender != a.UniqueName() || !reflect.DeepEqual(m.Body, []any{"hi"}) {
		t.Fatalf("signal = %+v", m)
	}

	// NameOwnerChanged reaches a watcher when a name goes away
	if _, err := b.Call(5*time.Second, BusName, BusPath, BusInterface, "AddMatch", "s",
		"type='signal',sender='org.freedesktop.DBus',member='NameOwnerChanged',arg0='io.github.s3onghyun.Test'"); err != nil {
		t.Fatal(err)
	}
	a.Close()
	if m := waitSignal(t, b, "NameOwnerChanged"); m.Body[0] != "io.github.s3onghyun.Test" || m.Body[2] != "" {
		t.Fatalf("signal = %+v", m)
	}
	if _, err := a.Call(time.Second, BusName, BusPath, BusInterface, "GetId", ""); err != ErrClosed {
		t.Errorf("call on a closed connection = %v", err)
	}
	select {
	case <-a.Done():
	default:
		t.Error("Done not closed")
	}
}

// waitSignal skips other signals (the bus sends NameAcquired first).
func waitSignal(t *testing.T, c *Conn, member string) *Message {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.Incoming():
			if m.Type == TypeSignal && m.Member == member {
				return m
			}
		case <-timeout:
			t.Fatal("no " + member)
		}
	}
}
