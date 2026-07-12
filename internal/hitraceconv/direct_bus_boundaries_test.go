package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestDirectBusUnsignedDisplayBoundaries(t *testing.T) {
	for _, boundary := range []uint16{0, ^uint16(0)} {
		t.Run(fmt.Sprintf("i2c_message_%d", boundary), func(t *testing.T) {
			fixture := directBusI2CMessageFixture("i2c_read", 0, nil)
			binary.LittleEndian.PutUint16(fixture.content[12:14], boundary)
			binary.LittleEndian.PutUint16(fixture.content[14:16], boundary)
			binary.LittleEndian.PutUint16(fixture.content[16:18], boundary)
			want := fmt.Sprintf("i2c-2 #%d a=%03x f=%04x l=0", boundary, boundary, boundary)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("u16 message boundary body=%q want=%q", body, want)
			}
		})

		t.Run(fmt.Sprintf("i2c_result_%d", boundary), func(t *testing.T) {
			fixture := directBusI2CResultFixture(boundary, 0)
			want := fmt.Sprintf("i2c-2 n=%d ret=0", boundary)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("u16 result boundary body=%q want=%q", body, want)
			}
		})
	}
}

func TestDirectBusSignedDisplayBoundaries(t *testing.T) {
	for _, boundary := range []int16{-32768, 32767} {
		t.Run(fmt.Sprintf("i2c_ret_%d", boundary), func(t *testing.T) {
			fixture := directBusI2CResultFixture(1, boundary)
			want := fmt.Sprintf("i2c-2 n=1 ret=%d", boundary)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("i2c s16 boundary body=%q want=%q", body, want)
			}
		})

		t.Run(fmt.Sprintf("smbus_res_%d", boundary), func(t *testing.T) {
			fixture := directBusSMBusResultFixture(0, 1, boundary)
			want := fmt.Sprintf("i2c-2 a=05a f=0001 c=7 QUICK rd res=%d", boundary)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("SMBus s16 boundary body=%q want=%q", body, want)
			}
		})
	}
}

func TestDirectBusCommandMaximumIsUnsigned(t *testing.T) {
	tests := []struct {
		name       string
		fixture    directBusTestFixture
		commandOff int
		want       string
	}{
		{
			name:       "read",
			fixture:    directBusSMBusReadFixture(0),
			commandOff: 16,
			want:       "i2c-2 a=05a f=0001 c=ff QUICK",
		},
		{
			name:       "write",
			fixture:    directBusSMBusTransferFixture("smbus_write", 0, 0, nil),
			commandOff: 16,
			want:       "i2c-2 a=05a f=0001 c=ff QUICK l=0 []",
		},
		{
			name:       "reply",
			fixture:    directBusSMBusTransferFixture("smbus_reply", 0, 0, nil),
			commandOff: 16,
			want:       "i2c-2 a=05a f=0001 c=ff QUICK l=0 []",
		},
		{
			name:       "result",
			fixture:    directBusSMBusResultFixture(0, 1, 0),
			commandOff: 17,
			want:       "i2c-2 a=05a f=0001 c=ff QUICK rd res=0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fixture.content[test.commandOff] = 0xff
			if body := directBusAdmittedBody(t, test.fixture); body != test.want {
				t.Fatalf("u8 command boundary body=%q want=%q", body, test.want)
			}
		})
	}
}

func TestDirectBusSMBusReadAndResultProtocolClosedSet(t *testing.T) {
	protocolNames := [...]string{
		"QUICK", "BYTE", "BYTE_DATA", "WORD_DATA", "PROC_CALL", "BLOCK_DATA",
		"I2C_BLOCK_BROKEN", "BLOCK_PROC_CALL", "I2C_BLOCK_DATA",
	}
	for protocol, protocolName := range protocolNames {
		protocol := uint32(protocol)
		t.Run(fmt.Sprintf("read_%d_%s", protocol, protocolName), func(t *testing.T) {
			fixture := directBusSMBusReadFixture(protocol)
			want := fmt.Sprintf("i2c-2 a=05a f=0001 c=7 %s", protocolName)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("SMBus read protocol body=%q want=%q", body, want)
			}
		})

		t.Run(fmt.Sprintf("result_%d_%s", protocol, protocolName), func(t *testing.T) {
			fixture := directBusSMBusResultFixture(protocol, 1, 0)
			want := fmt.Sprintf("i2c-2 a=05a f=0001 c=7 %s rd res=0", protocolName)
			if body := directBusAdmittedBody(t, fixture); body != want {
				t.Fatalf("SMBus result protocol body=%q want=%q", body, want)
			}
		})
	}

	t.Run("read_9_rejected", func(t *testing.T) {
		directBusAssertRejected(t, directBusSMBusReadFixture(9))
	})
	t.Run("result_9_rejected", func(t *testing.T) {
		directBusAssertRejected(t, directBusSMBusResultFixture(9, 1, 0))
	})
}

func TestDirectBusArrayParserKeepsPointerDeclarationCompatible(t *testing.T) {
	got, ok := parseFieldLine(
		"workqueue_execute_start",
		"field:void * work; offset:8; size:8; signed:0;",
	)
	want := eventField{Type: "void *", Name: "work", Offset: 8, Size: 8}
	if !ok || got != want {
		t.Fatalf("pointer field parse regressed: ok=%v got=%+v want=%+v", ok, got, want)
	}
}

func TestDirectBusModernSMBusArrayDeclaratorParsesAndConverts(t *testing.T) {
	item := conversionBusCases()[5] // smbus_write with non-empty binary data.
	formatText := strings.Join(syntheticFormatBlock(item.name, item.id, item.fields), "\n")
	formatText = strings.Replace(formatText, "buf[32 + 2]", "buf[34]", 1)
	if strings.Contains(formatText, "buf[32 + 2]") {
		t.Fatal("modern SMBus fixture retained the OpenHarmony array declarator")
	}

	catalog, err := parseEventFormats([]byte(formatText))
	if err != nil {
		t.Fatalf("parse modern SMBus descriptor: %v", err)
	}
	format, ok := catalog.Formats[item.id]
	if !ok || catalog.Poisoned[item.id] {
		t.Fatalf("modern SMBus descriptor was not admitted: %+v", catalog)
	}
	foundModernBuffer := false
	for _, field := range format.Fields {
		if cleanFieldName(field.Name) == "buf" {
			foundModernBuffer = field.Type == "__u8" && field.Name == "buf[34]" &&
				field.Offset == 24 && field.Size == 34 && !field.Signed
		}
	}
	if !foundModernBuffer {
		t.Fatalf("modern fixed array declaration drifted: %+v", format.Fields)
	}

	result, text, _ := runConversionBusCapture(t, formatText, []syntheticRawEvent{{
		EventID: uint16(item.id),
		Content: item.content,
	}})
	if result.EventsWritten != 1 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 {
		t.Fatalf("modern SMBus conversion was not admitted: result=%+v\n%s", result, text)
	}
	assertConversionBusLineCount(t, text, item.name+": "+item.want, 1)
}
