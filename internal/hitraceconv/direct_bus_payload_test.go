package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type directBusTestFixture struct {
	format  eventFormat
	content []byte
}

func TestDirectBusTracefsFieldDeclarationParsing(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		line      string
		want      eventField
	}{
		{
			name:      "SMBus expression array extent stays in declarator",
			eventName: "smbus_write",
			line:      "field:__u8 buf[32 + 2]; offset:24; size:34; signed:0;",
			want:      eventField{Type: "__u8", Name: "buf[32 + 2]", Offset: 24, Size: 34},
		},
		{
			name:      "ordinary scalar remains exact",
			eventName: "i2c_read",
			line:      "field:__u16 addr; offset:14; size:2; signed:0;",
			want:      eventField{Type: "__u16", Name: "addr", Offset: 14, Size: 2},
		},
		{
			name:      "ordinary signed scalar remains exact",
			eventName: "i2c_read",
			line:      "field:int adapter_nr; offset:8; size:4; signed:1;",
			want:      eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		},
		{
			name:      "data locator keeps element array in type",
			eventName: "i2c_write",
			line:      "field:__data_loc __u8[] buf; offset:20; size:4; signed:0;",
			want:      eventField{Type: "__data_loc __u8[]", Name: "buf", Offset: 20, Size: 4},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseFieldLine(test.eventName, test.line)
			if !ok || got != test.want {
				t.Fatalf("parseFieldLine(%q): ok=%v got=%+v want=%+v", test.line, ok, got, test.want)
			}
		})
	}
}

func TestDirectBusCanonicalProfileMatrix(t *testing.T) {
	tests := []struct {
		name    string
		fixture directBusTestFixture
		want    string
	}{
		{
			name:    "i2c read",
			fixture: directBusI2CMessageFixture("i2c_read", 2, nil),
			want:    "i2c-2 #3 a=05a f=0001 l=2",
		},
		{
			name:    "i2c write binary",
			fixture: directBusI2CMessageFixture("i2c_write", 2, []byte{0xab, 0xcd}),
			want:    "i2c-2 #3 a=05a f=0001 l=2 [ab-cd]",
		},
		{
			name:    "i2c reply binary",
			fixture: directBusI2CMessageFixture("i2c_reply", 2, []byte{0x00, 0xff}),
			want:    "i2c-2 #3 a=05a f=0001 l=2 [00-ff]",
		},
		{
			name:    "i2c result signed return",
			fixture: directBusI2CResultFixture(3, -5),
			want:    "i2c-2 n=3 ret=-5",
		},
		{
			name:    "smbus read",
			fixture: directBusSMBusReadFixture(2),
			want:    "i2c-2 a=05a f=0001 c=7 BYTE_DATA",
		},
		{
			name:    "smbus write",
			fixture: directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd}),
			want:    "i2c-2 a=05a f=0001 c=7 WORD_DATA l=2 [ab-cd]",
		},
		{
			name:    "smbus reply",
			fixture: directBusSMBusTransferFixture("smbus_reply", 1, 1, []byte{0xef}),
			want:    "i2c-2 a=05a f=0001 c=7 BYTE l=1 [ef]",
		},
		{
			name:    "smbus result read",
			fixture: directBusSMBusResultFixture(2, 1, -5),
			want:    "i2c-2 a=05a f=0001 c=7 BYTE_DATA rd res=-5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !directBusNameGoverned(test.fixture.format.Name) {
				t.Fatalf("exact bus event missing from registry: %q", test.fixture.format.Name)
			}
			body := directBusAdmittedBody(t, test.fixture)
			if body != test.want {
				t.Fatalf("canonical bus body=%q want=%q", body, test.want)
			}
			line, admission, reason, envelopeOK := renderEventLineDecision(
				renderContext{cmdlines: map[int]string{100: "i2c-worker"}, tgids: map[int]int{100: 100}},
				1_000_000, 2, test.fixture.format, test.fixture.content,
			)
			if !envelopeOK || admission != bodyAdmitted || reason != "" ||
				!strings.HasSuffix(line, test.fixture.format.Name+": "+test.want) {
				t.Fatalf("wrapped bus drifted: envelope=%v admission=%d reason=%q line=%q", envelopeOK, admission, reason, line)
			}
			if len(line) > maxTraceDBSystraceLineBytes || !traceDBSinglePhysicalLine(line, false) {
				t.Fatalf("admitted bus row escaped publication gate: bytes=%d line=%q", len(line), line)
			}
		})
	}
}

func TestDirectBusExactRegistryRejectsNearNamesAndLegacyPrefixFallback(t *testing.T) {
	exact := []string{
		"i2c_read", "i2c_write", "i2c_reply", "i2c_result",
		"smbus_read", "smbus_write", "smbus_reply", "smbus_result",
	}
	for _, name := range exact {
		if !directBusNameGoverned(name) {
			t.Fatalf("exact bus name is not governed: %q", name)
		}
	}

	tests := []struct {
		name    string
		fixture directBusTestFixture
	}{
		{name: "i2c_read_start", fixture: directBusI2CMessageFixture("i2c_read", 2, nil)},
		{name: "i2c_write_done", fixture: directBusI2CMessageFixture("i2c_write", 2, []byte{1, 2})},
		{name: "i2c_reply_vendor", fixture: directBusI2CMessageFixture("i2c_reply", 2, []byte{1, 2})},
		{name: "i2c_result_extra", fixture: directBusI2CResultFixture(1, 0)},
		{name: "I2C_read", fixture: directBusI2CMessageFixture("i2c_read", 2, nil)},
		{name: "i2c_read ", fixture: directBusI2CMessageFixture("i2c_read", 2, nil)},
		{name: "smbus_read_start", fixture: directBusSMBusReadFixture(2)},
		{name: "smbus_write_done", fixture: directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{1, 2})},
		{name: "smbus_reply_vendor", fixture: directBusSMBusTransferFixture("smbus_reply", 1, 1, []byte{1})},
		{name: "smbus_result_extra", fixture: directBusSMBusResultFixture(2, 1, 0)},
		{name: "SMBUS_read", fixture: directBusSMBusReadFixture(2)},
		{name: "smbus_read ", fixture: directBusSMBusReadFixture(2)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := directBusCloneFixture(test.fixture)
			fixture.format.Name = test.name
			if directBusNameGoverned(test.name) {
				t.Fatalf("near-name entered exact bus registry: %q", test.name)
			}
			payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
			if admission != bodyUnsupported || reason != "" {
				t.Fatalf("near-name decoder verdict: admission=%d reason=%q", admission, reason)
			}
			if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
				t.Fatalf("unsupported near-name produced a canonical body: ok=%v body=%q", ok, body)
			}
			body, wrappedAdmission, wrappedReason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0)
			if wrappedAdmission != bodyUnsupported || wrappedReason != "" {
				t.Fatalf("near-name entered a publishing renderer: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
			}
			if line, known := renderEventLine(
				renderContext{cmdlines: map[int]string{100: "i2c-worker"}, tgids: map[int]int{100: 100}},
				1_000_000, 2, fixture.format, fixture.content,
			); known {
				t.Fatalf("near-name generic inventory body became a published row: known=%v line=%q body=%q", known, line, body)
			}
		})
	}
}

func TestDirectBusOfficialDescriptorMutationMatrix(t *testing.T) {
	fixtures := []directBusTestFixture{
		directBusI2CMessageFixture("i2c_read", 2, nil),
		directBusI2CMessageFixture("i2c_write", 2, []byte{0xab, 0xcd}),
		directBusI2CMessageFixture("i2c_reply", 2, []byte{0xab, 0xcd}),
		directBusI2CResultFixture(3, -5),
		directBusSMBusReadFixture(2),
		directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd}),
		directBusSMBusTransferFixture("smbus_reply", 1, 1, []byte{0xab}),
		directBusSMBusResultFixture(2, 1, -5),
	}

	for _, base := range fixtures {
		base := base
		for fieldIndex := 4; fieldIndex < len(base.format.Fields); fieldIndex++ {
			fieldIndex := fieldIndex
			field := base.format.Fields[fieldIndex]
			fieldLabel := cleanFieldName(field.Name)
			prefix := base.format.Name + "/" + fieldLabel

			t.Run(prefix+"/missing", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				fixture.format.Fields = append(fixture.format.Fields[:fieldIndex], fixture.format.Fields[fieldIndex+1:]...)
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_type", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				fixture.format.Fields[fieldIndex].Type = "char *"
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_sign", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				fixture.format.Fields[fieldIndex].Signed = !fixture.format.Fields[fieldIndex].Signed
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_width", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				if fixture.format.Fields[fieldIndex].Size == 34 {
					fixture.format.Fields[fieldIndex].Size = 33
				} else {
					fixture.format.Fields[fieldIndex].Size++
				}
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_offset", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				fixture.format.Fields[fieldIndex].Offset++
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/truncated", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				end := field.Offset + field.Size
				fixture.content = fixture.content[:end-1]
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/clean_alias", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				switch fixture.format.Fields[fieldIndex].Name {
				case "buf":
					fixture.format.Fields[fieldIndex].Name = "__data_loc_buf"
				case "buf[34]":
					fixture.format.Fields[fieldIndex].Name = "buf"
				default:
					fixture.format.Fields[fieldIndex].Name += "[1]"
				}
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/same_duplicate", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				fixture.format.Fields = append(fixture.format.Fields, fixture.format.Fields[fieldIndex])
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/conflicting_duplicate", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				duplicate := fixture.format.Fields[fieldIndex]
				raw := append([]byte(nil), fixture.content[duplicate.Offset:duplicate.Offset+duplicate.Size]...)
				raw[0]++
				duplicate.Offset = len(fixture.content)
				fixture.format.Fields = append(fixture.format.Fields, duplicate)
				fixture.content = append(fixture.content, raw...)
				directBusAssertRejected(t, fixture)
			})
			t.Run(prefix+"/overlap", func(t *testing.T) {
				fixture := directBusCloneFixture(base)
				other := 4
				if fieldIndex == other {
					other++
				}
				fixture.format.Fields[fieldIndex].Offset = fixture.format.Fields[other].Offset
				directBusAssertRejected(t, fixture)
			})
		}

		t.Run(base.format.Name+"/extra_payload_field", func(t *testing.T) {
			fixture := directBusCloneFixture(base)
			fixture.format.Fields = append(fixture.format.Fields, eventField{
				Type: "unsigned int", Name: "vendor_cookie", Offset: len(fixture.content), Size: 4,
			})
			fixture.content = append(fixture.content, 1, 0, 0, 0)
			directBusAssertRejected(t, fixture)
		})
	}
}

func TestDirectBusI2CDynamicPayloadBoundsAndNoPadding(t *testing.T) {
	t.Run("binary is never a C string", func(t *testing.T) {
		fixture := directBusI2CMessageFixture("i2c_write", 3, []byte{0x00, 0x0a, 0xff})
		if body := directBusAdmittedBody(t, fixture); body != "i2c-2 #3 a=05a f=0001 l=3 [00-0a-ff]" {
			t.Fatalf("binary payload changed: %q", body)
		}
	})

	for _, name := range []string{"i2c_write", "i2c_reply"} {
		t.Run(name+"/zero_length", func(t *testing.T) {
			fixture := directBusI2CMessageFixture(name, 0, nil)
			if body := directBusAdmittedBody(t, fixture); body != "i2c-2 #3 a=05a f=0001 l=0 []" {
				t.Fatalf("zero-length canonical body=%q", body)
			}
		})

		tests := []struct {
			name   string
			mutate func(*directBusTestFixture)
		}{
			{
				name: "locator before fixed tail",
				mutate: func(fixture *directBusTestFixture) {
					directBusSetI2CLocator(fixture, 23, 2)
				},
			},
			{
				name: "locator outside record",
				mutate: func(fixture *directBusTestFixture) {
					directBusSetI2CLocator(fixture, len(fixture.content)+1, 2)
				},
			},
			{
				name: "locator shorter than scalar",
				mutate: func(fixture *directBusTestFixture) {
					directBusSetI2CLocator(fixture, 24, 1)
				},
			},
			{
				name: "locator longer than scalar",
				mutate: func(fixture *directBusTestFixture) {
					fixture.content = append(fixture.content, 0xee)
					directBusSetI2CLocator(fixture, 24, 3)
				},
			},
			{
				name: "locator zero does not inherit scalar",
				mutate: func(fixture *directBusTestFixture) {
					directBusSetI2CLocator(fixture, 24, 0)
				},
			},
			{
				name: "truncated payload is not zero padded",
				mutate: func(fixture *directBusTestFixture) {
					fixture.content = fixture.content[:len(fixture.content)-1]
				},
			},
			{
				name: "fixed bytes cannot replace data locator",
				mutate: func(fixture *directBusTestFixture) {
					field := directBusFixtureField(fixture, "buf")
					field.Type = "__u8"
					field.Name = "buf[4]"
				},
			},
		}
		for _, test := range tests {
			t.Run(name+"/"+test.name, func(t *testing.T) {
				fixture := directBusI2CMessageFixture(name, 2, []byte{0xab, 0xcd})
				test.mutate(&fixture)
				directBusAssertRejected(t, fixture)
			})
		}
	}

	t.Run("source width is checked before payload length", func(t *testing.T) {
		fixture := directBusI2CMessageFixture("i2c_write", 2, []byte{0xab, 0xcd})
		field := directBusFixtureField(&fixture, "len")
		field.Type = "__u32"
		field.Size = 4
		binary.LittleEndian.PutUint32(fixture.content[field.Offset:field.Offset+4], ^uint32(0))
		_, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyRejected || reason == "" {
			t.Fatalf("wrong-width length was not rejected before allocation: admission=%d reason=%q", admission, reason)
		}
	})

	t.Run("maximum u16 cannot exceed physical event payload", func(t *testing.T) {
		fixture := directBusI2CMessageFixture("i2c_write", ^uint16(0), nil)
		directBusAssertRejected(t, fixture)
	})
}

func TestDirectBusSMBusProtocolLengthMatrix(t *testing.T) {
	protocols := []struct {
		value uint32
		name  string
	}{
		{0, "QUICK"},
		{1, "BYTE"},
		{2, "BYTE_DATA"},
		{3, "WORD_DATA"},
		{4, "PROC_CALL"},
		{5, "BLOCK_DATA"},
		{6, "I2C_BLOCK_BROKEN"},
		{7, "BLOCK_PROC_CALL"},
		{8, "I2C_BLOCK_DATA"},
	}

	for _, eventName := range []string{"smbus_write", "smbus_reply"} {
		for _, protocol := range protocols {
			protocol := protocol
			t.Run(fmt.Sprintf("%s/%d_%s", eventName, protocol.value, protocol.name), func(t *testing.T) {
				length := directBusSMBusProtocolLength(eventName, protocol.value)
				buf := []byte{0xab, 0xcd}
				if protocol.value == 5 || protocol.value == 7 || protocol.value == 8 {
					buf = []byte{2, 0xaa, 0xbb}
				}
				fixture := directBusSMBusTransferFixture(eventName, protocol.value, uint8(length), buf)
				want := fmt.Sprintf("i2c-2 a=05a f=0001 c=7 %s l=%d %s", protocol.name, length, directBusHex(buf[:length]))
				if body := directBusAdmittedBody(t, fixture); body != want {
					t.Fatalf("protocol body=%q want=%q", body, want)
				}
			})
		}
	}

	t.Run("word data remains binary", func(t *testing.T) {
		fixture := directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0x00, 0x0a, 0xff})
		if body := directBusAdmittedBody(t, fixture); body != "i2c-2 a=05a f=0001 c=7 WORD_DATA l=2 [00-0a]" {
			t.Fatalf("fixed binary carrier became text: %q", body)
		}
	})

	t.Run("maximum block count", func(t *testing.T) {
		buf := make([]byte, 33)
		buf[0] = 32
		for index := 1; index < len(buf); index++ {
			buf[index] = byte(index)
		}
		fixture := directBusSMBusTransferFixture("smbus_write", 5, 33, buf)
		want := "i2c-2 a=05a f=0001 c=7 BLOCK_DATA l=33 " + directBusHex(buf)
		if body := directBusAdmittedBody(t, fixture); body != want {
			t.Fatalf("maximum block body=%q want=%q", body, want)
		}
	})

	invalid := []struct {
		name    string
		fixture directBusTestFixture
	}{
		{
			name:    "unknown protocol",
			fixture: directBusSMBusTransferFixture("smbus_write", 9, 0, nil),
		},
		{
			name:    "fixed protocol length mismatch",
			fixture: directBusSMBusTransferFixture("smbus_write", 3, 1, []byte{0xab, 0xcd}),
		},
		{
			name:    "block scalar count mismatch",
			fixture: directBusSMBusTransferFixture("smbus_write", 5, 2, []byte{2, 0xaa, 0xbb}),
		},
		{
			name:    "block count exceeds 32",
			fixture: directBusSMBusTransferFixture("smbus_write", 5, 34, append([]byte{33}, make([]byte, 33)...)),
		},
		{
			name:    "u8 scalar length exceeds fixed carrier",
			fixture: directBusSMBusTransferFixture("smbus_write", 5, 255, append([]byte{254}, make([]byte, 33)...)),
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			directBusAssertRejected(t, test.fixture)
		})
	}
}

func TestDirectBusSMBusFixedCarrierAndDirection(t *testing.T) {
	t.Run("fixed carrier must be complete", func(t *testing.T) {
		fixture := directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd})
		fixture.content = fixture.content[:len(fixture.content)-1]
		directBusAssertRejected(t, fixture)
	})

	t.Run("fixed carrier type is not C string", func(t *testing.T) {
		fixture := directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd})
		field := directBusFixtureField(&fixture, "buf")
		field.Type = "char"
		directBusAssertRejected(t, fixture)
	})

	t.Run("fixed carrier declaration keeps array extent", func(t *testing.T) {
		fixture := directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd})
		directBusFixtureField(&fixture, "buf").Name = "buf"
		directBusAssertRejected(t, fixture)
	})

	for _, direction := range []struct {
		value uint8
		want  string
	}{
		{0, "i2c-2 a=05a f=0001 c=7 BYTE_DATA wr res=-5"},
		{1, "i2c-2 a=05a f=0001 c=7 BYTE_DATA rd res=-5"},
	} {
		t.Run(fmt.Sprintf("direction_%d", direction.value), func(t *testing.T) {
			fixture := directBusSMBusResultFixture(2, direction.value, -5)
			if body := directBusAdmittedBody(t, fixture); body != direction.want {
				t.Fatalf("direction body=%q want=%q", body, direction.want)
			}
		})
	}

	t.Run("invalid direction", func(t *testing.T) {
		directBusAssertRejected(t, directBusSMBusResultFixture(2, 2, -5))
	})

	t.Run("unknown protocol does not become blank or QUICK", func(t *testing.T) {
		directBusAssertRejected(t, directBusSMBusResultFixture(9, 1, -5))
	})
}

func TestDirectBusCanonicalRendererDefendsTypedInvariants(t *testing.T) {
	t.Run("kind and name must agree", func(t *testing.T) {
		fixture := directBusI2CMessageFixture("i2c_write", 2, []byte{0xab, 0xcd})
		payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("seed admission=%d reason=%q", admission, reason)
		}
		payload.Name = "i2c_read"
		if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
			t.Fatalf("kind/name mismatch rendered: ok=%v body=%q", ok, body)
		}
	})

	t.Run("i2c data length must equal scalar", func(t *testing.T) {
		fixture := directBusI2CMessageFixture("i2c_write", 2, []byte{0xab, 0xcd})
		payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("seed admission=%d reason=%q", admission, reason)
		}
		payload.Data = payload.Data[:1]
		if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
			t.Fatalf("I2C length mismatch rendered: ok=%v body=%q", ok, body)
		}
	})

	t.Run("smbus protocol length relation is rechecked", func(t *testing.T) {
		fixture := directBusSMBusTransferFixture("smbus_write", 3, 2, []byte{0xab, 0xcd})
		payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("seed admission=%d reason=%q", admission, reason)
		}
		payload.Length = 1
		if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
			t.Fatalf("SMBus length mismatch rendered: ok=%v body=%q", ok, body)
		}
		payload.Length = 2
		payload.Protocol = 9
		if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
			t.Fatalf("unknown SMBus protocol rendered: ok=%v body=%q", ok, body)
		}
	})

	t.Run("smbus result direction remains closed", func(t *testing.T) {
		fixture := directBusSMBusResultFixture(2, 1, -5)
		payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("seed admission=%d reason=%q", admission, reason)
		}
		payload.ReadWrite = 2
		if body, ok := renderCanonicalBusPayload(payload); ok || body != "" {
			t.Fatalf("invalid SMBus direction rendered: ok=%v body=%q", ok, body)
		}
	})
}

func TestDirectBusSingleAuthorityAndPublicationBudgetStructure(t *testing.T) {
	counts := map[string]int{
		"directBusNameGoverned":     0,
		"decodeDirectBusPayload":    0,
		"renderCanonicalBusPayload": 0,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.FuncDecl)
			if ok {
				if _, tracked := counts[decl.Name.Name]; tracked {
					counts[decl.Name.Name]++
				}
			}
			return true
		})
	}
	for name, count := range counts {
		if count != 1 {
			t.Fatalf("bus authority %s declaration count=%d want=1", name, count)
		}
	}

	renderSource, err := os.ReadFile("render.go")
	if err != nil {
		t.Fatal(err)
	}
	render := string(renderSource)
	decoderAt := strings.Index(render, "decodeDirectBusPayload(")
	rendererAt := strings.Index(render, "renderCanonicalBusPayload(")
	legacyAt := strings.Index(render, "renderLegacyEventBody(")
	if decoderAt < 0 || rendererAt < 0 || legacyAt < 0 || decoderAt >= rendererAt || rendererAt >= legacyAt {
		t.Fatalf("bus decode/render must dominate legacy rendering: decoder=%d renderer=%d legacy=%d", decoderAt, rendererAt, legacyAt)
	}
	if maxTraceDBSystraceLineBytes != 1<<20 ||
		!strings.Contains(render, "directBusNameGoverned(format.Name)") ||
		!strings.Contains(render, "traceDBSinglePhysicalLine(line, false)") {
		t.Fatal("bus rows are not pinned into the shared single-physical-line/1MiB publication gate")
	}

	officialSource, err := os.ReadFile("official_render.go")
	if err != nil {
		t.Fatal(err)
	}
	official := string(officialSource)
	for _, legacyPrefix := range []string{
		`strings.HasPrefix(name, "i2c_read")`,
		`strings.HasPrefix(name, "i2c_write")`,
		`strings.HasPrefix(name, "i2c_reply")`,
		`strings.HasPrefix(name, "i2c_result")`,
		`strings.HasPrefix(name, "smbus_read")`,
		`strings.HasPrefix(name, "smbus_write")`,
		`strings.HasPrefix(name, "smbus_reply")`,
		`strings.HasPrefix(name, "smbus_result")`,
	} {
		if strings.Contains(official, legacyPrefix) {
			t.Fatalf("legacy broad bus authority remains reachable: %s", legacyPrefix)
		}
	}
}

func directBusAdmittedBody(t *testing.T, fixture directBusTestFixture) string {
	t.Helper()
	payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("bus admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	body, ok := renderCanonicalBusPayload(payload)
	if !ok || body == "" {
		t.Fatalf("canonical bus renderer rejected admitted payload: ok=%v body=%q payload=%+v", ok, body, payload)
	}
	return body
}

func directBusAssertRejected(t *testing.T, fixture directBusTestFixture) {
	t.Helper()
	payload, admission, reason := decodeDirectBusPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyRejected || reason == "" {
		t.Fatalf("malformed governed bus row was not rejected: name=%q admission=%d reason=%q payload=%+v", fixture.format.Name, admission, reason, payload)
	}
	if line, known := renderEventLine(
		renderContext{cmdlines: map[int]string{100: "i2c-worker"}, tgids: map[int]int{100: 100}},
		1_000_000, 2, fixture.format, fixture.content,
	); known || line != "" {
		t.Fatalf("governed bus reject fell through to legacy/header rendering: known=%v line=%q", known, line)
	}
}

func directBusCommonFields() []eventField {
	return []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
}

func directBusFillEnvelope(content []byte) {
	if len(content) < 8 {
		return
	}
	binary.LittleEndian.PutUint16(content[0:2], 700)
	binary.LittleEndian.PutUint32(content[4:8], 100)
}

func directBusI2CMessageFixture(name string, length uint16, data []byte) directBusTestFixture {
	fields := directBusCommonFields()
	fields = append(fields,
		eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "__u16", Name: "msg_nr", Offset: 12, Size: 2},
		eventField{Type: "__u16", Name: "addr", Offset: 14, Size: 2},
		eventField{Type: "__u16", Name: "flags", Offset: 16, Size: 2},
		eventField{Type: "__u16", Name: "len", Offset: 18, Size: 2},
	)
	fixedTail := 20
	if name == "i2c_write" || name == "i2c_reply" {
		fields = append(fields, eventField{Type: "__data_loc __u8[]", Name: "buf", Offset: 20, Size: 4})
		fixedTail = 24
	}
	content := make([]byte, fixedTail+len(data))
	directBusFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 3)
	binary.LittleEndian.PutUint16(content[14:16], 0x5a)
	binary.LittleEndian.PutUint16(content[16:18], 1)
	binary.LittleEndian.PutUint16(content[18:20], length)
	if fixedTail == 24 {
		binary.LittleEndian.PutUint32(content[20:24], uint32((len(data)<<16)|fixedTail))
		copy(content[fixedTail:], data)
	}
	return directBusTestFixture{format: eventFormat{ID: 700, Name: name, Fields: fields}, content: content}
}

func directBusI2CResultFixture(messages uint16, result int16) directBusTestFixture {
	fields := directBusCommonFields()
	fields = append(fields,
		eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "__u16", Name: "nr_msgs", Offset: 12, Size: 2},
		eventField{Type: "__s16", Name: "ret", Offset: 14, Size: 2, Signed: true},
	)
	content := make([]byte, 16)
	directBusFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], messages)
	binary.LittleEndian.PutUint16(content[14:16], uint16(result))
	return directBusTestFixture{format: eventFormat{ID: 701, Name: "i2c_result", Fields: fields}, content: content}
}

func directBusSMBusReadFixture(protocol uint32) directBusTestFixture {
	fields := directBusCommonFields()
	fields = append(fields,
		eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "__u16", Name: "flags", Offset: 12, Size: 2},
		eventField{Type: "__u16", Name: "addr", Offset: 14, Size: 2},
		eventField{Type: "__u8", Name: "command", Offset: 16, Size: 1},
		eventField{Type: "__u32", Name: "protocol", Offset: 20, Size: 4},
		eventField{Type: "__u8", Name: "buf[34]", Offset: 24, Size: 34},
	)
	content := make([]byte, 58)
	directBusFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 1)
	binary.LittleEndian.PutUint16(content[14:16], 0x5a)
	content[16] = 7
	binary.LittleEndian.PutUint32(content[20:24], protocol)
	return directBusTestFixture{format: eventFormat{ID: 702, Name: "smbus_read", Fields: fields}, content: content}
}

func directBusSMBusTransferFixture(name string, protocol uint32, length uint8, data []byte) directBusTestFixture {
	fields := directBusCommonFields()
	fields = append(fields,
		eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "__u16", Name: "addr", Offset: 12, Size: 2},
		eventField{Type: "__u16", Name: "flags", Offset: 14, Size: 2},
		eventField{Type: "__u8", Name: "command", Offset: 16, Size: 1},
		eventField{Type: "__u8", Name: "len", Offset: 17, Size: 1},
		eventField{Type: "__u32", Name: "protocol", Offset: 20, Size: 4},
		eventField{Type: "__u8", Name: "buf[34]", Offset: 24, Size: 34},
	)
	content := make([]byte, 58)
	directBusFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 0x5a)
	binary.LittleEndian.PutUint16(content[14:16], 1)
	content[16] = 7
	content[17] = length
	binary.LittleEndian.PutUint32(content[20:24], protocol)
	copy(content[24:58], data)
	return directBusTestFixture{format: eventFormat{ID: 703, Name: name, Fields: fields}, content: content}
}

func directBusSMBusResultFixture(protocol uint32, readWrite uint8, result int16) directBusTestFixture {
	fields := directBusCommonFields()
	fields = append(fields,
		eventField{Type: "int", Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "__u16", Name: "addr", Offset: 12, Size: 2},
		eventField{Type: "__u16", Name: "flags", Offset: 14, Size: 2},
		eventField{Type: "__u8", Name: "read_write", Offset: 16, Size: 1},
		eventField{Type: "__u8", Name: "command", Offset: 17, Size: 1},
		eventField{Type: "__s16", Name: "res", Offset: 18, Size: 2, Signed: true},
		eventField{Type: "__u32", Name: "protocol", Offset: 20, Size: 4},
	)
	content := make([]byte, 24)
	directBusFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 0x5a)
	binary.LittleEndian.PutUint16(content[14:16], 1)
	content[16] = readWrite
	content[17] = 7
	binary.LittleEndian.PutUint16(content[18:20], uint16(result))
	binary.LittleEndian.PutUint32(content[20:24], protocol)
	return directBusTestFixture{format: eventFormat{ID: 704, Name: "smbus_result", Fields: fields}, content: content}
}

func directBusCloneFixture(fixture directBusTestFixture) directBusTestFixture {
	clone := fixture
	clone.format.Fields = append([]eventField(nil), fixture.format.Fields...)
	clone.content = append([]byte(nil), fixture.content...)
	return clone
}

func directBusFixtureField(fixture *directBusTestFixture, name string) *eventField {
	for index := range fixture.format.Fields {
		if cleanFieldName(fixture.format.Fields[index].Name) == name {
			return &fixture.format.Fields[index]
		}
	}
	panic("direct bus fixture field not found: " + name)
}

func directBusSetI2CLocator(fixture *directBusTestFixture, offset, length int) {
	field := directBusFixtureField(fixture, "buf")
	binary.LittleEndian.PutUint32(fixture.content[field.Offset:field.Offset+4], uint32((length<<16)|offset))
}

func directBusSMBusProtocolLength(name string, protocol uint32) int {
	if protocol == 5 || protocol == 7 || protocol == 8 {
		return 3
	}
	if name == "smbus_write" {
		switch protocol {
		case 2:
			return 1
		case 3, 4:
			return 2
		default:
			return 0
		}
	}
	switch protocol {
	case 1, 2:
		return 1
	case 3, 4:
		return 2
	default:
		return 0
	}
}

func directBusHex(data []byte) string {
	parts := make([]string, 0, len(data))
	for _, value := range data {
		parts = append(parts, fmt.Sprintf("%02x", value))
	}
	return "[" + strings.Join(parts, "-") + "]"
}
