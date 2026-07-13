package hitraceconv

import (
	"errors"
	"math"
	"testing"
)

func TestConsumeProtoVarintRejectsUint64Overflow(t *testing.T) {
	tests := []struct {
		name  string
		wire  []byte
		want  uint64
		bytes int
		ok    bool
	}{
		{name: "zero", wire: []byte{0}, want: 0, bytes: 1, ok: true},
		{name: "max uint64", wire: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, want: math.MaxUint64, bytes: 10, ok: true},
		{name: "two to sixty four", wire: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02}},
		{name: "tenth byte continuation", wire: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}},
		{name: "truncated continuation", wire: []byte{0x80}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bytes, ok := consumeProtoVarint(tt.wire)
			if got != tt.want || bytes != tt.bytes || ok != tt.ok {
				t.Fatalf("consumeProtoVarint(%x)=(%d,%d,%v), want (%d,%d,%v)", tt.wire, got, bytes, ok, tt.want, tt.bytes, tt.ok)
			}
		})
	}
}

func TestProtoScalarUintRejectsOverflowInsteadOfMintingZero(t *testing.T) {
	// field3 varint key followed by 2^64, which previously wrapped to zero.
	payload := append([]byte{3 << 3}, []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02}...)
	value, state, reason := protoScalarUint(payload, 3)
	if value != 0 || state != protoScalarInvalid || reason != "malformed_wire" {
		t.Fatalf("overflow scalar admitted: value=%d state=%d reason=%q", value, state, reason)
	}
}

func TestWalkProtoFieldsRejectsInvalidFieldNumbers(t *testing.T) {
	const maxField = (1 << 29) - 1
	seen := 0
	if err := walkProtoFields(protoVarint(maxField, 7), func(field int, wire int, raw []byte, value uint64) error {
		seen++
		if field != maxField || wire != 0 || value != 7 {
			t.Fatalf("maximum legal field changed: field=%d wire=%d value=%d", field, wire, value)
		}
		return nil
	}); err != nil || seen != 1 {
		t.Fatalf("maximum legal protobuf field rejected: seen=%d err=%v", seen, err)
	}
	for _, test := range []struct {
		name string
		wire []byte
	}{
		{name: "zero", wire: []byte{0, 0}},
		{name: "above maximum", wire: protoVarint(1<<29, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := walkProtoFields(test.wire, func(int, int, []byte, uint64) error { return nil }); err == nil {
				t.Fatalf("invalid protobuf field admitted: %x", test.wire)
			}
		})
	}
}

func TestWalkProtoFieldsTypedFailuresPreserveCompatibilityAndEndpoint(t *testing.T) {
	tenContinuations := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	tests := []struct {
		name       string
		wire       []byte
		message    string
		failure    protoFieldDecodeFailure
		field      int
		fieldKnown bool
		terminal   bool
	}{
		{name: "malformed key", wire: []byte{0x80}, message: "malformed protobuf field key", failure: protoFieldDecodeMalformedKey},
		{name: "field zero", wire: []byte{0x00, 0x00}, message: "invalid protobuf field number 0", failure: protoFieldDecodeInvalidFieldNumber},
		{name: "varint tail", wire: []byte{0x08, 0x80}, message: "malformed protobuf varint field 1", failure: protoFieldDecodeMalformedValue, field: 1, fieldKnown: true, terminal: true},
		{name: "varint ambiguous tail", wire: append(append([]byte{0x08}, tenContinuations...), 0x01), message: "malformed protobuf varint field 1", failure: protoFieldDecodeMalformedValue, field: 1, fieldKnown: true},
		{name: "fixed64 tail", wire: []byte{0x09, 0x00}, message: "truncated protobuf fixed64 field 1", failure: protoFieldDecodeMalformedValue, field: 1, fieldKnown: true, terminal: true},
		{name: "bytes tail", wire: []byte{0x0a, 0x02, 'x'}, message: "truncated protobuf bytes field 1", failure: protoFieldDecodeMalformedValue, field: 1, fieldKnown: true, terminal: true},
		{name: "fixed32 tail", wire: []byte{0x0d, 0x00}, message: "truncated protobuf fixed32 field 1", failure: protoFieldDecodeMalformedValue, field: 1, fieldKnown: true, terminal: true},
		{name: "unsupported terminal", wire: []byte{0x0b}, message: "unsupported protobuf wire type 3 for field 1", failure: protoFieldDecodeUnsupportedWire, field: 1, fieldKnown: true, terminal: true},
		{name: "unsupported ambiguous tail", wire: []byte{0x0b, 0x08, 0x01}, message: "unsupported protobuf wire type 3 for field 1", failure: protoFieldDecodeUnsupportedWire, field: 1, fieldKnown: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := walkProtoFields(test.wire, func(int, int, []byte, uint64) error { return nil })
			var typed *protoFieldDecodeError
			if !errors.As(err, &typed) || err.Error() != test.message || typed.Failure != test.failure ||
				typed.Field != test.field || typed.FieldKnown != test.fieldKnown || typed.Terminal != test.terminal {
				t.Fatalf("typed failure drifted: err=%v typed=%+v want=(%q,%d,%d,%t,%t)",
					err, typed, test.message, test.failure, test.field, test.fieldKnown, test.terminal)
			}
		})
	}

	sentinel := errors.New("callback sentinel")
	err := walkProtoFields(protoVarint(1, 7), func(int, int, []byte, uint64) error { return sentinel })
	if err != sentinel || !errors.Is(err, sentinel) {
		t.Fatalf("callback error identity changed: got=%v want=%v", err, sentinel)
	}
}
