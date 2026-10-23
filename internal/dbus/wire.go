// Package dbus is a small D-Bus client for the tray icon: the session bus connection, method
// calls, replies and signals, and the marshalling those need.
//
// It is written against the D-Bus specification with the standard library only, because the
// project takes no third-party dependencies, and it needs no cgo, so the static release build
// keeps working. It does not pass unix file descriptors and only speaks the EXTERNAL
// authentication the session bus uses.
package dbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// ObjectPath is a D-Bus object path ('o').
type ObjectPath string

// Signature is a D-Bus type signature ('g').
type Signature string

// Variant is a value with its own signature ('v').
type Variant struct {
	Sig   Signature
	Value any
}

// MakeVariant wraps a value of the given single complete type.
func MakeVariant(sig string, v any) Variant { return Variant{Sig: Signature(sig), Value: v} }

// Limits from the specification.
const (
	maxSignature = 255
	maxArray     = 64 << 20
	maxDepth     = 64
)

// Value representations, both ways: y byte, b bool, n int16, q uint16, i int32, u uint32,
// x int64, t uint64, d float64, s string, o ObjectPath, g Signature, h uint32, v Variant.
// Arrays decode to []any (ay to []byte), structs and dict entries to []any. Arrays also
// encode from []byte, []string, []int32, []Variant, and a{sv} from map[string]Variant
// (written in key order).

// splitType returns the first single complete type of sig and the rest.
func splitType(sig string) (string, string, error) {
	n, err := typeLen(sig, 0, 0, 0)
	if err != nil {
		return "", "", err
	}
	return sig[:n], sig[n:], nil
}

func typeLen(sig string, i, arrays, structs int) (int, error) {
	if i >= len(sig) {
		return 0, errors.New("dbus: incomplete signature " + sig)
	}
	switch c := sig[i]; c {
	case 'y', 'b', 'n', 'q', 'i', 'u', 'x', 't', 'd', 's', 'o', 'g', 'h', 'v':
		return i + 1, nil
	case 'a':
		if arrays >= 32 {
			return 0, errors.New("dbus: arrays nested too deep")
		}
		if i+1 < len(sig) && sig[i+1] == '{' {
			j, err := typeLen(sig, i+2, arrays+1, structs)
			if err != nil {
				return 0, err
			}
			if !strings.ContainsRune("ybnqiuxtdsogh", rune(sig[i+2])) {
				return 0, errors.New("dbus: dict key must be a basic type")
			}
			j, err = typeLen(sig, j, arrays+1, structs)
			if err != nil {
				return 0, err
			}
			if j >= len(sig) || sig[j] != '}' {
				return 0, errors.New("dbus: bad dict entry in " + sig)
			}
			return j + 1, nil
		}
		return typeLen(sig, i+1, arrays+1, structs)
	case '(':
		if structs >= 32 {
			return 0, errors.New("dbus: structs nested too deep")
		}
		j := i + 1
		if j < len(sig) && sig[j] == ')' {
			return 0, errors.New("dbus: empty struct")
		}
		for j < len(sig) && sig[j] != ')' {
			var err error
			if j, err = typeLen(sig, j, arrays, structs+1); err != nil {
				return 0, err
			}
		}
		if j >= len(sig) {
			return 0, errors.New("dbus: unclosed struct in " + sig)
		}
		return j + 1, nil
	default:
		return 0, fmt.Errorf("dbus: bad type %q in %q", c, sig)
	}
}

// ValidSignature reports whether sig is a sequence of complete types.
func ValidSignature(sig string) bool {
	if len(sig) > maxSignature {
		return false
	}
	for sig != "" {
		var err error
		if _, sig, err = splitType(sig); err != nil {
			return false
		}
	}
	return true
}

func alignOf(c byte) int {
	switch c {
	case 'n', 'q':
		return 2
	case 'b', 'i', 'u', 'h', 's', 'o', 'a':
		return 4
	case 'x', 't', 'd', '(', '{':
		return 8
	}
	return 1 // y, g, v
}

// validPath checks an object path: "/" or "/a/b_c", elements [A-Za-z0-9_].
func validPath(p string) bool {
	if p == "/" {
		return true
	}
	if !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return false
	}
	for _, el := range strings.Split(p[1:], "/") {
		if el == "" {
			return false
		}
		for i := 0; i < len(el); i++ {
			c := el[i]
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return false
			}
		}
	}
	return true
}

// encoder -------------------------------------------------------------------

type encoder struct {
	buf []byte
}

func (e *encoder) align(n int) {
	for len(e.buf)%n != 0 {
		e.buf = append(e.buf, 0)
	}
}

func (e *encoder) u32(v uint32) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *encoder) str(s string) {
	e.u32(uint32(len(s)))
	e.buf = append(append(e.buf, s...), 0)
}

func (e *encoder) sig(s string) {
	e.buf = append(append(append(e.buf, byte(len(s))), s...), 0)
}

// values encodes args against a signature of zero or more complete types.
func (e *encoder) values(sig string, args []any) error {
	return e.sequence(sig, args, 0)
}

// sequence encodes one value per complete type of sig.
func (e *encoder) sequence(sig string, args []any, depth int) error {
	whole := sig
	for _, a := range args {
		t, rest, err := splitType(sig)
		if err != nil {
			return fmt.Errorf("dbus: %d values for signature %q", len(args), whole)
		}
		if err := e.value(t, a, depth); err != nil {
			return err
		}
		sig = rest
	}
	if sig != "" {
		return fmt.Errorf("dbus: %d values for signature %q", len(args), whole)
	}
	return nil
}

func (e *encoder) value(t string, v any, depth int) error {
	if depth > maxDepth {
		return errors.New("dbus: value nested too deep")
	}
	bad := func() error { return fmt.Errorf("dbus: cannot encode %T as %q", v, t) }
	switch t[0] {
	case 'y':
		b, ok := v.(byte)
		if !ok {
			return bad()
		}
		e.buf = append(e.buf, b)
	case 'b':
		b, ok := v.(bool)
		if !ok {
			return bad()
		}
		n := uint32(0)
		if b {
			n = 1
		}
		e.u32(n)
	case 'n', 'q':
		var n uint16
		switch x := v.(type) {
		case int16:
			n = uint16(x)
		case uint16:
			n = x
		default:
			return bad()
		}
		e.align(2)
		e.buf = binary.LittleEndian.AppendUint16(e.buf, n)
	case 'i', 'u', 'h':
		var n uint32
		switch x := v.(type) {
		case int32:
			n = uint32(x)
		case uint32:
			n = x
		default:
			return bad()
		}
		e.u32(n)
	case 'x', 't', 'd':
		var n uint64
		switch x := v.(type) {
		case int64:
			n = uint64(x)
		case uint64:
			n = x
		case float64:
			n = math.Float64bits(x)
		default:
			return bad()
		}
		e.align(8)
		e.buf = binary.LittleEndian.AppendUint64(e.buf, n)
	case 's', 'o':
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case ObjectPath:
			s = string(x)
		default:
			return bad()
		}
		if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 || t[0] == 'o' && !validPath(s) {
			return fmt.Errorf("dbus: invalid %q value %q", t, s)
		}
		e.str(s)
	case 'g':
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case Signature:
			s = string(x)
		default:
			return bad()
		}
		if !ValidSignature(s) {
			return fmt.Errorf("dbus: invalid signature %q", s)
		}
		e.sig(s)
	case 'v':
		x, ok := v.(Variant)
		if !ok {
			return bad()
		}
		inner := string(x.Sig)
		if t, rest, err := splitType(inner); err != nil || rest != "" || t == "" {
			return fmt.Errorf("dbus: variant needs one complete type, got %q", inner)
		}
		e.sig(inner)
		return e.value(inner, x.Value, depth+1)
	case 'a':
		return e.array(t, v, depth)
	case '(':
		fields, ok := v.([]any)
		if !ok {
			return bad()
		}
		e.align(8)
		return e.sequence(t[1:len(t)-1], fields, depth+1)
	default:
		return bad()
	}
	return nil
}

func (e *encoder) array(t string, v any, depth int) error {
	elem := t[1:]
	var items []any
	switch x := v.(type) {
	case []any:
		items = x
	case []byte:
		if elem != "y" {
			return fmt.Errorf("dbus: []byte for %q", t)
		}
		e.u32(uint32(len(x)))
		e.buf = append(e.buf, x...)
		return nil
	case []string:
		for _, s := range x {
			items = append(items, s)
		}
	case []int32:
		for _, n := range x {
			items = append(items, n)
		}
	case []Variant:
		for _, n := range x {
			items = append(items, n)
		}
	case map[string]Variant:
		if elem != "{sv}" {
			return fmt.Errorf("dbus: map[string]Variant for %q", t)
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			items = append(items, []any{k, x[k]})
		}
	default:
		return fmt.Errorf("dbus: cannot encode %T as %q", v, t)
	}
	e.u32(0)
	lenAt := len(e.buf) - 4
	e.align(alignOf(elem[0]))
	start := len(e.buf)
	if elem[0] == '{' {
		elem = "(" + elem[1:len(elem)-1] + ")" // a dict entry is laid out as a struct
	}
	for _, item := range items {
		if err := e.value(elem, item, depth+1); err != nil {
			return err
		}
	}
	n := len(e.buf) - start
	if n > maxArray {
		return errors.New("dbus: array too long")
	}
	binary.LittleEndian.PutUint32(e.buf[lenAt:], uint32(n))
	return nil
}

// decoder -------------------------------------------------------------------

type decoder struct {
	buf   []byte
	pos   int
	order binary.ByteOrder
}

var errShort = errors.New("dbus: message too short")

func (d *decoder) align(n int) error {
	for d.pos%n != 0 {
		if d.pos >= len(d.buf) {
			return errShort
		}
		if d.buf[d.pos] != 0 {
			return errors.New("dbus: nonzero padding")
		}
		d.pos++
	}
	return nil
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || d.pos+n > len(d.buf) {
		return nil, errShort
	}
	b := d.buf[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

func (d *decoder) u32() (uint32, error) {
	if err := d.align(4); err != nil {
		return 0, err
	}
	b, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return d.order.Uint32(b), nil
}

func (d *decoder) str() (string, error) {
	n, err := d.u32()
	if err != nil {
		return "", err
	}
	b, err := d.take(int(n) + 1)
	if err != nil {
		return "", err
	}
	if b[n] != 0 || !utf8.Valid(b[:n]) || strings.IndexByte(string(b[:n]), 0) >= 0 {
		return "", errors.New("dbus: bad string")
	}
	return string(b[:n]), nil
}

func (d *decoder) sig() (string, error) {
	b, err := d.take(1)
	if err != nil {
		return "", err
	}
	s, err := d.take(int(b[0]) + 1)
	if err != nil {
		return "", err
	}
	if s[b[0]] != 0 || !ValidSignature(string(s[:b[0]])) {
		return "", errors.New("dbus: bad signature")
	}
	return string(s[:b[0]]), nil
}

// values decodes a signature of zero or more complete types.
func (d *decoder) values(sig string) ([]any, error) {
	var out []any
	for sig != "" {
		t, rest, err := splitType(sig)
		if err != nil {
			return nil, err
		}
		v, err := d.value(t, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		sig = rest
	}
	return out, nil
}

func (d *decoder) value(t string, depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("dbus: value nested too deep")
	}
	switch t[0] {
	case 'y':
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return b[0], nil
	case 'b':
		n, err := d.u32()
		if err != nil {
			return nil, err
		}
		if n > 1 {
			return nil, errors.New("dbus: bad boolean")
		}
		return n == 1, nil
	case 'n', 'q':
		if err := d.align(2); err != nil {
			return nil, err
		}
		b, err := d.take(2)
		if err != nil {
			return nil, err
		}
		if t[0] == 'n' {
			return int16(d.order.Uint16(b)), nil
		}
		return d.order.Uint16(b), nil
	case 'i':
		n, err := d.u32()
		return int32(n), err
	case 'u', 'h':
		return d.u32()
	case 'x', 't', 'd':
		if err := d.align(8); err != nil {
			return nil, err
		}
		b, err := d.take(8)
		if err != nil {
			return nil, err
		}
		n := d.order.Uint64(b)
		switch t[0] {
		case 'x':
			return int64(n), nil
		case 'd':
			return math.Float64frombits(n), nil
		}
		return n, nil
	case 's':
		return d.str()
	case 'o':
		s, err := d.str()
		if err == nil && !validPath(s) {
			err = errors.New("dbus: bad object path")
		}
		return ObjectPath(s), err
	case 'g':
		s, err := d.sig()
		return Signature(s), err
	case 'v':
		s, err := d.sig()
		if err != nil {
			return nil, err
		}
		if inner, rest, err := splitType(s); err != nil || rest != "" || inner == "" {
			return nil, errors.New("dbus: bad variant signature")
		}
		v, err := d.value(s, depth+1)
		return Variant{Sig: Signature(s), Value: v}, err
	case 'a':
		n, err := d.u32()
		if err != nil {
			return nil, err
		}
		if n > maxArray {
			return nil, errors.New("dbus: array too long")
		}
		elem := t[1:]
		if err := d.align(alignOf(elem[0])); err != nil {
			return nil, err
		}
		end := d.pos + int(n)
		if end > len(d.buf) {
			return nil, errShort
		}
		if elem == "y" {
			b, _ := d.take(int(n))
			return append([]byte(nil), b...), nil
		}
		if elem[0] == '{' {
			elem = "(" + elem[1:len(elem)-1] + ")"
		}
		items := []any{}
		for d.pos < end {
			v, err := d.value(elem, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		if d.pos != end {
			return nil, errors.New("dbus: array length mismatch")
		}
		return items, nil
	case '(':
		if err := d.align(8); err != nil {
			return nil, err
		}
		return d.fields(t[1:len(t)-1], depth)
	}
	return nil, fmt.Errorf("dbus: bad type %q", t)
}

func (d *decoder) fields(sig string, depth int) ([]any, error) {
	var out []any
	for sig != "" {
		t, rest, err := splitType(sig)
		if err != nil {
			return nil, err
		}
		v, err := d.value(t, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		sig = rest
	}
	return out, nil
}
