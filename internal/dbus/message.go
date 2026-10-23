package dbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Message types.
const (
	TypeMethodCall   byte = 1
	TypeMethodReturn byte = 2
	TypeError        byte = 3
	TypeSignal       byte = 4
)

// FlagNoReplyExpected marks a method call that wants no reply.
const FlagNoReplyExpected byte = 0x1

// header field codes
const (
	fieldPath        = 1
	fieldInterface   = 2
	fieldMember      = 3
	fieldErrorName   = 4
	fieldReplySerial = 5
	fieldDestination = 6
	fieldSender      = 7
	fieldSignature   = 8
	fieldUnixFDs     = 9
)

// maxMessage caps what this client reads. The specification allows 128 MiB; the tray's
// messages are a few kilobytes.
const maxMessage = 32 << 20

// Message is one D-Bus message.
type Message struct {
	Type        byte
	Flags       byte
	Serial      uint32
	Path        ObjectPath
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Signature   string
	Body        []any
}

// Marshal encodes the message, little-endian.
func (m *Message) Marshal() ([]byte, error) {
	if m.Serial == 0 {
		return nil, errors.New("dbus: serial 0")
	}
	switch m.Type {
	case TypeMethodCall:
		if m.Path == "" || m.Member == "" {
			return nil, errors.New("dbus: method call without path or member")
		}
	case TypeSignal:
		if m.Path == "" || m.Interface == "" || m.Member == "" {
			return nil, errors.New("dbus: signal without path, interface or member")
		}
	case TypeError:
		if m.ErrorName == "" || m.ReplySerial == 0 {
			return nil, errors.New("dbus: error without name or reply serial")
		}
	case TypeMethodReturn:
		if m.ReplySerial == 0 {
			return nil, errors.New("dbus: reply without reply serial")
		}
	default:
		return nil, fmt.Errorf("dbus: message type %d", m.Type)
	}
	var body encoder
	if err := body.values(m.Signature, m.Body); err != nil {
		return nil, err
	}
	var fields []any
	str := func(code byte, sig, v string) {
		if v != "" {
			fields = append(fields, []any{code, MakeVariant(sig, v)})
		}
	}
	if m.Path != "" {
		fields = append(fields, []any{byte(fieldPath), MakeVariant("o", m.Path)})
	}
	str(fieldInterface, "s", m.Interface)
	str(fieldMember, "s", m.Member)
	str(fieldErrorName, "s", m.ErrorName)
	if m.ReplySerial != 0 {
		fields = append(fields, []any{byte(fieldReplySerial), MakeVariant("u", m.ReplySerial)})
	}
	str(fieldDestination, "s", m.Destination)
	str(fieldSender, "s", m.Sender)
	if m.Signature != "" {
		fields = append(fields, []any{byte(fieldSignature), MakeVariant("g", Signature(m.Signature))})
	}
	h := encoder{buf: []byte{'l', m.Type, m.Flags, 1}}
	h.u32(uint32(len(body.buf)))
	h.u32(m.Serial)
	if err := h.value("a(yv)", fields, 0); err != nil {
		return nil, err
	}
	h.align(8)
	if len(h.buf)+len(body.buf) > maxMessage {
		return nil, errors.New("dbus: message too long")
	}
	return append(h.buf, body.buf...), nil
}

// ReadMessage reads one message.
func ReadMessage(r io.Reader) (*Message, error) {
	fixed := make([]byte, 16)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch fixed[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, errors.New("dbus: bad endianness byte")
	}
	if fixed[3] != 1 {
		return nil, fmt.Errorf("dbus: protocol version %d", fixed[3])
	}
	bodyLen := order.Uint32(fixed[4:])
	fieldsLen := order.Uint32(fixed[12:])
	if bodyLen > maxMessage || fieldsLen > maxMessage {
		return nil, errors.New("dbus: message too long")
	}
	headerLen := 16 + int(fieldsLen)
	padded := (headerLen + 7) &^ 7
	if padded+int(bodyLen) > maxMessage {
		return nil, errors.New("dbus: message too long")
	}
	buf := make([]byte, padded+int(bodyLen))
	copy(buf, fixed)
	if _, err := io.ReadFull(r, buf[16:]); err != nil {
		return nil, err
	}

	m := &Message{Type: fixed[1], Flags: fixed[2], Serial: order.Uint32(fixed[8:])}
	d := decoder{buf: buf[:headerLen], pos: 12, order: order}
	v, err := d.value("a(yv)", 0)
	if err != nil {
		return nil, err
	}
	for _, f := range v.([]any) {
		field := f.([]any)
		code, val := field[0].(byte), field[1].(Variant)
		want := map[byte]Signature{fieldPath: "o", fieldInterface: "s", fieldMember: "s", fieldErrorName: "s",
			fieldReplySerial: "u", fieldDestination: "s", fieldSender: "s", fieldSignature: "g", fieldUnixFDs: "u"}[code]
		if want == "" {
			continue // unknown fields are ignored, as the specification says
		}
		if val.Sig != want {
			return nil, fmt.Errorf("dbus: header field %d has type %q", code, val.Sig)
		}
		switch code {
		case fieldPath:
			m.Path = val.Value.(ObjectPath)
		case fieldInterface:
			m.Interface = val.Value.(string)
		case fieldMember:
			m.Member = val.Value.(string)
		case fieldErrorName:
			m.ErrorName = val.Value.(string)
		case fieldReplySerial:
			m.ReplySerial = val.Value.(uint32)
		case fieldDestination:
			m.Destination = val.Value.(string)
		case fieldSender:
			m.Sender = val.Value.(string)
		case fieldSignature:
			m.Signature = string(val.Value.(Signature))
		case fieldUnixFDs:
			if val.Value.(uint32) != 0 {
				return nil, errors.New("dbus: unix file descriptors are not supported")
			}
		}
	}
	for i := headerLen; i < padded; i++ {
		if buf[i] != 0 {
			return nil, errors.New("dbus: nonzero header padding")
		}
	}
	body := decoder{buf: buf[padded:], order: order}
	if m.Body, err = body.values(m.Signature); err != nil {
		return nil, err
	}
	if body.pos != len(body.buf) {
		return nil, errors.New("dbus: body longer than its signature")
	}
	return m, nil
}
