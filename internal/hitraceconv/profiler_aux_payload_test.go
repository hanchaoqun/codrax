package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type profilerAuxTestValue struct {
	wire  int
	uint  uint64
	bytes []byte
}

type profilerAuxTestCase struct {
	field  int
	name   string
	values map[int]profilerAuxTestValue
	want   string
}

func profilerAuxVarint(value uint64) profilerAuxTestValue {
	return profilerAuxTestValue{wire: 0, uint: value}
}

func profilerAuxBytes(value string) profilerAuxTestValue {
	return profilerAuxTestValue{wire: 2, bytes: []byte(value)}
}

func profilerAuxSignedValue(value int32) profilerAuxTestValue {
	return profilerAuxVarint(uint64(int64(value)))
}

func profilerAuxTestCases() []profilerAuxTestCase {
	dev := uint64(12<<20 | 80)
	return []profilerAuxTestCase{
		{
			field: 1109, name: "print",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(0x1234), 2: profilerAuxBytes("B|7|Frame"),
			},
			want: "B|7|Frame",
		},
		{
			field: 4009, name: "f2fs_sync_file_enter",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(dev), 2: profilerAuxVarint(0x1234), 3: profilerAuxVarint(0x55),
				4: profilerAuxVarint(0x81a4), 5: profilerAuxVarint(8192), 6: profilerAuxVarint(2),
				7: profilerAuxVarint(16), 8: profilerAuxVarint(7),
			},
			want: "dev=12:80 ino=0x1234 pino=0x55 i_mode=0x81a4 i_size=8192 i_nlink=2 i_blocks=16 i_advise=0x7",
		},
		{
			field: 4010, name: "f2fs_sync_file_exit",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(dev), 2: profilerAuxVarint(0x1234), 3: profilerAuxSignedValue(-2),
				4: profilerAuxSignedValue(1), 5: profilerAuxSignedValue(-5),
			},
			want: "dev=12:80 ino=0x1234 cp_reason=-2 datasync=1 ret=-5",
		},
		{
			field: 4011, name: "f2fs_write_begin",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(dev), 2: profilerAuxVarint(0x1234), 3: profilerAuxVarint(4096),
				4: profilerAuxVarint(1024), 5: profilerAuxVarint(3),
			},
			want: "dev=12:80 ino=0x1234 pos=4096 len=1024 flags=3",
		},
		{
			field: 4012, name: "f2fs_write_end",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(dev), 2: profilerAuxVarint(0x1234), 3: profilerAuxVarint(4096),
				4: profilerAuxVarint(1024), 5: profilerAuxVarint(512),
			},
			want: "dev=12:80 ino=0x1234 pos=4096 len=1024 copied=512",
		},
		{
			field: 4015, name: "mmc_request_done",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(17), 2: profilerAuxSignedValue(-5), 3: profilerAuxBytes("ABCD"),
				4: profilerAuxVarint(1), 5: profilerAuxVarint(18), 6: profilerAuxSignedValue(-6),
				7: profilerAuxBytes("EFGH"), 8: profilerAuxVarint(2), 9: profilerAuxVarint(19),
				10: profilerAuxSignedValue(-7), 11: profilerAuxBytes("IJKL"), 12: profilerAuxVarint(3),
				13: profilerAuxVarint(4096), 14: profilerAuxSignedValue(-8), 15: profilerAuxSignedValue(-1),
				16: profilerAuxVarint(1), 17: profilerAuxVarint(0), 18: profilerAuxVarint(1),
				19: profilerAuxSignedValue(-9), 20: profilerAuxSignedValue(-10), 21: profilerAuxVarint(11),
				22: profilerAuxVarint(0x1234), 23: profilerAuxBytes("mmc0"),
			},
			want: "mmc0: end struct mmc_request[0x1234]: cmd_opcode=17 cmd_err=-5 cmd_retries=1 stop_opcode=18 stop_err=-6 stop_retries=2 sbc_opcode=19 sbc_err=-7 sbc_retries=3 bytes_xfered=4096 data_err=-8 tag=-1 can_retune=1 doing_retune=0 retune_now=1 need_retune=-9 hold_retune=-10 retune_period=11",
		},
		{
			field: 4016, name: "mmc_request_start",
			values: map[int]profilerAuxTestValue{
				1: profilerAuxVarint(17), 2: profilerAuxVarint(0x123), 3: profilerAuxVarint(0x456),
				4: profilerAuxVarint(1), 5: profilerAuxVarint(18), 6: profilerAuxVarint(0x234),
				7: profilerAuxVarint(0x567), 8: profilerAuxVarint(2), 9: profilerAuxVarint(19),
				10: profilerAuxVarint(0x345), 11: profilerAuxVarint(0x678), 12: profilerAuxVarint(3),
				13: profilerAuxVarint(8), 14: profilerAuxVarint(10), 15: profilerAuxVarint(512),
				16: profilerAuxVarint(9), 17: profilerAuxSignedValue(-1), 18: profilerAuxVarint(1),
				19: profilerAuxVarint(0), 20: profilerAuxVarint(1), 21: profilerAuxSignedValue(-2),
				22: profilerAuxSignedValue(-3), 23: profilerAuxVarint(4), 24: profilerAuxVarint(0x1234),
				25: profilerAuxBytes("mmc0"),
			},
			want: "mmc0: start struct mmc_request[0x1234]: cmd_opcode=17 cmd_arg=0x123 cmd_flags=0x456 cmd_retries=1 stop_opcode=18 stop_arg=0x234 stop_flags=0x567 stop_retries=2 sbc_opcode=19 sbc_arg=0x345 sbc_flags=0x678 sbc_retires=3 blocks=8 block_size=512 blk_addr=10 data_flags=0x9 tag=-1 can_retune=1 doing_retune=0 retune_now=1 need_retune=-2 hold_retune=-3 retune_period=4",
		},
	}
}

func TestProfilerAuxPayloadMatrixUsesTypedCanonicalRenderer(t *testing.T) {
	for _, test := range profilerAuxTestCases() {
		t.Run(test.name, func(t *testing.T) {
			event := profilerFtraceEventRecord{Field: test.field, Payload: profilerAuxEncodeValues(test.values)}
			payload, admission, reason := decodeProfilerAuxPayload(event)
			if admission != bodyAdmitted || reason != "" {
				t.Fatalf("admission=%d reason=%q payload=%+v", admission, reason, payload)
			}
			if payload.Name != test.name {
				t.Fatalf("typed name=%q want=%q payload=%+v", payload.Name, test.name, payload)
			}
			body, ok := renderCanonicalProfilerAuxPayload(payload)
			if !ok || body != test.want {
				t.Fatalf("canonical body: ok=%t got=%q want=%q payload=%+v", ok, body, test.want, payload)
			}
			name, rendered, known, degradations := renderProfilerFtraceEventBodyWithAudit(event)
			if !known || name != test.name || rendered != test.want || len(degradations) != 0 {
				t.Fatalf("production path: known=%t name=%q body=%q degradations=%v", known, name, rendered, degradations)
			}
		})
	}
}

func TestProfilerAuxAll73KnownFieldsRejectWrongWireAndDuplicates(t *testing.T) {
	fieldCount := 0
	for _, test := range profilerAuxTestCases() {
		schema := profilerStructuredAuxSchemas[test.field]
		fields := make([]int, 0, len(schema))
		for field := range schema {
			fields = append(fields, field)
		}
		sort.Ints(fields)
		fieldCount += len(fields)
		for _, field := range fields {
			value := test.values[field]
			for _, mutation := range []struct {
				name   string
				extra  []byte
				reason string
			}{
				{name: "wrong_wire", extra: profilerAuxWrongWire(field, value), reason: profilerAuxFieldReason(field, "wrong_wire")},
				{name: "same_duplicate", extra: profilerAuxEncodeField(field, value), reason: profilerAuxFieldReason(field, "duplicate")},
				{name: "conflicting_duplicate", extra: profilerAuxEncodeField(field, profilerAuxAlternateValue(value)), reason: profilerAuxFieldReason(field, "duplicate")},
			} {
				t.Run(test.name+"/field"+strconv.Itoa(field)+"/"+mutation.name, func(t *testing.T) {
					data := append(profilerAuxEncodeValues(test.values), mutation.extra...)
					payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: data})
					if admission != bodyRejected || reason != mutation.reason || !reflect.DeepEqual(payload, profilerAuxPayload{}) {
						t.Fatalf("admission=%d reason=%q want=%q partial=%+v", admission, reason, mutation.reason, payload)
					}
					name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: test.field, Payload: data})
					if known || name != "" || body != "" || len(degradations) != 1 || degradations[0] != mutation.reason {
						t.Fatalf("rejected field escaped production: known=%t name=%q body=%q degradations=%v", known, name, body, degradations)
					}
				})
			}
		}
	}
	if fieldCount != 73 {
		t.Fatalf("aux schema field census drifted: got=%d want=73", fieldCount)
	}
}

func TestProfilerAuxMalformedAndUnknownExtensionSemantics(t *testing.T) {
	for _, test := range profilerAuxTestCases() {
		t.Run(test.name+"/malformed", func(t *testing.T) {
			data := append(profilerAuxEncodeValues(test.values), 0x80)
			payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: data})
			if admission != bodyRejected || reason != "aux_payload_malformed_wire" || !reflect.DeepEqual(payload, profilerAuxPayload{}) {
				t.Fatalf("malformed payload escaped: admission=%d reason=%q payload=%+v", admission, reason, payload)
			}
		})
		t.Run(test.name+"/unknown_extension", func(t *testing.T) {
			baseData := profilerAuxEncodeValues(test.values)
			base, baseAdmission, baseReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: baseData})
			withUnknown := append(baseData, protoBytes(99, []byte("future\nopaque"))...)
			got, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: withUnknown})
			if baseAdmission != bodyAdmitted || admission != bodyAdmitted || baseReason != "" || reason != "" || !reflect.DeepEqual(got, base) {
				t.Fatalf("unknown extension changed typed fact: base=%+v/%d/%q got=%+v/%d/%q", base, baseAdmission, baseReason, got, admission, reason)
			}
		})
	}
}

func TestProfilerAuxProto3DefaultsAndUnionProfileOptionalPresence(t *testing.T) {
	assertSame := func(t *testing.T, field int, omitted, explicit []byte) {
		t.Helper()
		left, leftAdmission, leftReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: field, Payload: omitted})
		right, rightAdmission, rightReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: field, Payload: explicit})
		if leftAdmission != bodyAdmitted || rightAdmission != bodyAdmitted || leftReason != "" || rightReason != "" || !reflect.DeepEqual(left, right) {
			t.Fatalf("absent/default mismatch: omitted=%+v/%d/%q explicit=%+v/%d/%q", left, leftAdmission, leftReason, right, rightAdmission, rightReason)
		}
		leftBody, leftOK := renderCanonicalProfilerAuxPayload(left)
		rightBody, rightOK := renderCanonicalProfilerAuxPayload(right)
		if !leftOK || !rightOK || leftBody != rightBody {
			t.Fatalf("absent/default body mismatch: omitted=%q/%t explicit=%q/%t", leftBody, leftOK, rightBody, rightOK)
		}
	}

	devAndIno := protoPayload(protoVarint(1, uint64(12<<20|80)), protoVarint(2, 0x1234))
	assertSame(t, 4009, devAndIno, protoPayload(devAndIno, profilerAuxExplicitZeroFields(3, 4, 5, 6, 7, 8)))
	assertSame(t, 4010, devAndIno, protoPayload(devAndIno, profilerAuxExplicitZeroFields(3, 4, 5)))
	assertSame(t, 4011, devAndIno, protoPayload(devAndIno, profilerAuxExplicitZeroFields(3, 4)))
	assertSame(t, 4012, devAndIno, protoPayload(devAndIno, profilerAuxExplicitZeroFields(3, 4, 5)))
	mmcDoneIdentity := protoPayload(protoVarint(22, 0x1234), protoBytes(23, []byte("mmc0")))
	assertSame(t, 4015, mmcDoneIdentity, protoPayload(
		profilerAuxExplicitZeroFields(1, 2), protoBytes(3, nil), profilerAuxExplicitZeroFields(4, 5, 6),
		protoBytes(7, nil), profilerAuxExplicitZeroFields(8, 9, 10), protoBytes(11, nil),
		profilerAuxExplicitZeroFields(12, 13, 14, 15, 16, 17, 18, 19, 20, 21), mmcDoneIdentity,
	))
	mmcStartIdentity := protoPayload(protoVarint(24, 0x1234), protoBytes(25, []byte("mmc0")))
	assertSame(t, 4016, mmcStartIdentity, protoPayload(
		profilerAuxExplicitZeroFields(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23),
		mmcStartIdentity,
	))

	printAbsent, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, []byte("I|7|mark"))})
	printZero, zeroAdmission, zeroReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 1109, Payload: protoPayload(protoVarint(1, 0), protoBytes(2, []byte("I|7|mark")))})
	if admission != bodyAdmitted || zeroAdmission != bodyAdmitted || reason != "" || zeroReason != "" ||
		printAbsent.Print == nil || printZero.Print == nil || printAbsent.Print.IPPresent || !printZero.Print.IPPresent || printAbsent.Print.Buffer != printZero.Print.Buffer {
		t.Fatalf("Print ip optionality collapsed: absent=%+v/%d/%q explicit0=%+v/%d/%q", printAbsent, admission, reason, printZero, zeroAdmission, zeroReason)
	}

	flagsAbsent, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4011, Payload: devAndIno})
	flagsZero, zeroAdmission, zeroReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4011, Payload: protoPayload(devAndIno, protoVarint(5, 0))})
	if admission != bodyAdmitted || zeroAdmission != bodyAdmitted || reason != "" || zeroReason != "" ||
		flagsAbsent.F2FS == nil || flagsZero.F2FS == nil || flagsAbsent.F2FS.FlagsPresent || !flagsZero.F2FS.FlagsPresent {
		t.Fatalf("F2FS flags optionality collapsed: absent=%+v/%d/%q explicit0=%+v/%d/%q", flagsAbsent, admission, reason, flagsZero, zeroAdmission, zeroReason)
	}
	absentBody, _ := renderCanonicalProfilerAuxPayload(flagsAbsent)
	zeroBody, _ := renderCanonicalProfilerAuxPayload(flagsZero)
	if strings.Contains(absentBody, "flags=") || !strings.HasSuffix(zeroBody, " flags=0") {
		t.Fatalf("F2FS flags presence not preserved: absent=%q explicit0=%q", absentBody, zeroBody)
	}

	for _, test := range []struct {
		name   string
		field  int
		input  []byte
		reason string
	}{
		{name: "print missing", field: 1109, reason: "missing_or_invalid_print_buf"},
		{name: "print explicit empty", field: 1109, input: protoBytes(2, nil), reason: "missing_or_invalid_print_buf"},
		{name: "f2fs missing dev", field: 4009, input: protoVarint(2, 1), reason: "missing_or_invalid_f2fs_dev"},
		{name: "f2fs explicit zero dev", field: 4009, input: protoPayload(protoVarint(1, 0), protoVarint(2, 1)), reason: "missing_or_invalid_f2fs_dev"},
		{name: "f2fs missing ino", field: 4009, input: protoVarint(1, 1), reason: "missing_or_invalid_f2fs_ino"},
		{name: "f2fs explicit zero ino", field: 4009, input: protoPayload(protoVarint(1, 1), protoVarint(2, 0)), reason: "missing_or_invalid_f2fs_ino"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, gotAdmission, gotReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: test.input})
			if gotAdmission != bodyRejected || gotReason != test.reason {
				t.Fatalf("semantic zero/missing escaped: admission=%d reason=%q want=%q", gotAdmission, gotReason, test.reason)
			}
		})
	}
}

func TestProfilerAuxIntegerSourceBoundsAndSignedEncodings(t *testing.T) {
	base := profilerAuxCasesByField()
	uint32Fields := map[int][]int{
		4009: {4, 6},
		4011: {4, 5},
		4012: {4, 5},
		4015: {1, 4, 5, 8, 9, 12, 13, 16, 17, 18, 21},
		4016: {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 18, 19, 20, 23},
	}
	for eventField, payloadFields := range uint32Fields {
		for _, payloadField := range payloadFields {
			t.Run("uint32/"+strconv.Itoa(eventField)+"/"+strconv.Itoa(payloadField), func(t *testing.T) {
				values := profilerAuxCloneValues(base[eventField].values)
				values[payloadField] = profilerAuxVarint(math.MaxUint32)
				_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyAdmitted || reason != "" {
					t.Fatalf("uint32 max rejected: admission=%d reason=%q", admission, reason)
				}
				values[payloadField] = profilerAuxVarint(uint64(math.MaxUint32) + 1)
				_, admission, reason = decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyRejected || reason != profilerAuxFieldReason(payloadField, "out_of_range") {
					t.Fatalf("uint32 overflow escaped: admission=%d reason=%q", admission, reason)
				}
			})
		}
	}

	for _, eventField := range []int{4009, 4010, 4011, 4012} {
		t.Run("dev_t/"+strconv.Itoa(eventField), func(t *testing.T) {
			values := profilerAuxCloneValues(base[eventField].values)
			values[1] = profilerAuxVarint(math.MaxUint32)
			payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
			if admission != bodyAdmitted || reason != "" {
				t.Fatalf("dev_t MaxUint32 rejected: admission=%d reason=%q", admission, reason)
			}
			body, ok := renderCanonicalProfilerAuxPayload(payload)
			if !ok || !strings.Contains(body, "dev=4095:1048575 ") {
				t.Fatalf("dev_t MaxUint32 did not retain the canonical 12:20 identity: ok=%t body=%q", ok, body)
			}
			if eventField == 4011 {
				endpoint := tracequery.DecodePairingEndpoint("f2fs_write_begin", body, 40)
				if !endpoint.Recognized || !endpoint.KeyKnown || !endpoint.PayloadAdmitted {
					t.Fatalf("dev_t MaxUint32 did not round-trip into a consumable endpoint: %+v body=%q", endpoint, body)
				}
			}
			for _, overflow := range []uint64{uint64(math.MaxUint32) + 1, math.MaxUint64} {
				values[1] = profilerAuxVarint(overflow)
				_, admission, reason = decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyRejected || reason != profilerAuxFieldReason(1, "out_of_range") {
					t.Fatalf("dev_t overflow 0x%x escaped: admission=%d reason=%q", overflow, admission, reason)
				}
			}
			values[1] = profilerAuxVarint(0)
			_, admission, reason = decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
			if admission != bodyRejected || reason != "missing_or_invalid_f2fs_dev" {
				t.Fatalf("zero dev_t identity escaped: admission=%d reason=%q", admission, reason)
			}
		})
	}

	t.Run("f2fs advise source width", func(t *testing.T) {
		values := profilerAuxCloneValues(base[4009].values)
		values[8] = profilerAuxVarint(math.MaxUint8)
		_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4009, Payload: profilerAuxEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("u8 advise max rejected: admission=%d reason=%q", admission, reason)
		}
		values[8] = profilerAuxVarint(uint64(math.MaxUint8) + 1)
		_, admission, reason = decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4009, Payload: profilerAuxEncodeValues(values)})
		if admission != bodyRejected || reason != profilerAuxFieldReason(8, "out_of_range") {
			t.Fatalf("u8 advise overflow escaped: admission=%d reason=%q", admission, reason)
		}
	})

	int32Fields := map[int][]int{
		4010: {3, 4, 5},
		4015: {2, 6, 10, 14, 15, 19, 20},
		4016: {17, 21, 22},
	}
	for eventField, payloadFields := range int32Fields {
		for _, payloadField := range payloadFields {
			t.Run("int32/"+strconv.Itoa(eventField)+"/"+strconv.Itoa(payloadField), func(t *testing.T) {
				var decoded []profilerAuxPayload
				for _, encoded := range []uint64{math.MaxUint32, math.MaxUint64} {
					values := profilerAuxCloneValues(base[eventField].values)
					values[payloadField] = profilerAuxVarint(encoded)
					payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
					if admission != bodyAdmitted || reason != "" {
						t.Fatalf("valid -1 encoding 0x%x rejected: admission=%d reason=%q", encoded, admission, reason)
					}
					decoded = append(decoded, payload)
				}
				if !reflect.DeepEqual(decoded[0], decoded[1]) {
					t.Fatalf("low32/sign-extended -1 decoded differently: low=%+v sign=%+v", decoded[0], decoded[1])
				}
				values := profilerAuxCloneValues(base[eventField].values)
				values[payloadField] = profilerAuxVarint(uint64(1)<<32 | 1)
				_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyRejected || reason != profilerAuxFieldReason(payloadField, "out_of_range") {
					t.Fatalf("int32 high-bit pollution escaped: admission=%d reason=%q", admission, reason)
				}
			})
		}
	}

	uint64Fields := map[int][]int{
		1109: {1}, 4009: {2, 3, 7}, 4010: {2}, 4011: {2}, 4012: {2}, 4015: {22}, 4016: {24},
	}
	for eventField, payloadFields := range uint64Fields {
		for _, payloadField := range payloadFields {
			t.Run("uint64/"+strconv.Itoa(eventField)+"/"+strconv.Itoa(payloadField), func(t *testing.T) {
				values := profilerAuxCloneValues(base[eventField].values)
				values[payloadField] = profilerAuxVarint(math.MaxUint64)
				_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyAdmitted || reason != "" {
					t.Fatalf("uint64 max rejected: admission=%d reason=%q", admission, reason)
				}
			})
		}
	}

	for eventField, payloadField := range map[int]int{4009: 5, 4011: 3, 4012: 3} {
		t.Run("signed_public_magnitude/"+strconv.Itoa(eventField)+"/"+strconv.Itoa(payloadField), func(t *testing.T) {
			values := profilerAuxCloneValues(base[eventField].values)
			values[payloadField] = profilerAuxVarint(math.MaxInt64)
			_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
			if admission != bodyAdmitted || reason != "" {
				t.Fatalf("MaxInt64 size/pos rejected: admission=%d reason=%q", admission, reason)
			}
			values[payloadField] = profilerAuxVarint(uint64(math.MaxInt64) + 1)
			_, admission, reason = decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
			if admission != bodyRejected || reason != profilerAuxFieldReason(payloadField, "out_of_range") {
				t.Fatalf("size/pos above MaxInt64 escaped: admission=%d reason=%q", admission, reason)
			}
		})
	}
}

func TestProfilerAuxPrintShapesAndStringSafety(t *testing.T) {
	valid := []struct {
		name string
		buf  string
		want string
	}{
		{name: "begin trailing space", buf: "B|193|HWC display setCompositionType: ", want: "B|193|HWC display setCompositionType: "},
		{name: "counter metadata", buf: "C|1864|H:DVSyncRate|1|I38", want: "C|1864|H:DVSyncRate|1|I38"},
		{name: "async begin", buf: "G|227|CriticalWorkload|Commit|183146", want: "G|227|CriticalWorkload|Commit|183146"},
		{name: "async end", buf: "H|227|CriticalWorkload|183146", want: "H|227|CriticalWorkload|183146"},
		{name: "instant", buf: "I|227|vsync in 48.62ms", want: "I|227|vsync in 48.62ms"},
		{name: "track instant", buf: "N|227|CriticalWorkload|Layers: +0 -0", want: "N|227|CriticalWorkload|Layers: +0 -0"},
		{name: "end", buf: "E|227", want: "E|227"},
		{name: "ordinary print prose", buf: "driver diagnostic key=value", want: "driver diagnostic key=value"},
		{name: "utf8", buf: "I|7|滑动应用内操作", want: "I|7|滑动应用内操作"},
		{name: "golden long", buf: "B|7|" + strings.Repeat("x", 380), want: "B|7|" + strings.Repeat("x", 380)},
		{name: "single producer newline", buf: "B|7|Frame\n", want: "B|7|Frame"},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, []byte(test.buf))})
			if admission != bodyAdmitted || reason != "" || payload.Print == nil || payload.Print.Buffer != test.want {
				t.Fatalf("valid print rejected/mutated: admission=%d reason=%q payload=%+v want=%q", admission, reason, payload, test.want)
			}
			body, ok := renderCanonicalProfilerAuxPayload(payload)
			if !ok || body != test.want {
				t.Fatalf("print body: got=%q ok=%t want=%q", body, ok, test.want)
			}
		})
	}

	invalid := []struct {
		name string
		buf  []byte
	}{
		{name: "empty", buf: nil},
		{name: "ascii spaces", buf: []byte("   ")},
		{name: "unicode whitespace", buf: []byte("\u3000")},
		{name: "embedded newline", buf: []byte("a\nb")},
		{name: "crlf", buf: []byte("a\r\n")},
		{name: "nul", buf: []byte{'a', 0, 'b'}},
		{name: "tab", buf: []byte("a\tb")},
		{name: "unicode line separator", buf: []byte("a\u2028b")},
		{name: "unicode paragraph separator", buf: []byte("a\u2029b")},
		{name: "invalid utf8", buf: []byte{0xff, 'x'}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, test.buf)})
			if admission != bodyRejected || reason != "missing_or_invalid_print_buf" {
				t.Fatalf("unsafe print escaped: admission=%d reason=%q", admission, reason)
			}
		})
	}

	oversized := profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes)))}
	name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(oversized)
	if known || name != "" || body != "" || len(degradations) != 1 || degradations[0] != "invalid_canonical_aux_line" {
		t.Fatalf("oversized print must reject locally: known=%t name=%q body_len=%d degradations=%v", known, name, len(body), degradations)
	}
}

func TestProfilerAuxMMCNameGateAndResponseNonAuthority(t *testing.T) {
	base := profilerAuxCasesByField()
	for _, eventField := range []int{4015, 4016} {
		nameField := 23
		if eventField == 4016 {
			nameField = 25
		}
		for _, test := range []struct {
			name  string
			value []byte
		}{
			{name: "empty", value: nil},
			{name: "leading space", value: []byte(" mmc0")},
			{name: "internal space", value: []byte("mmc 0")},
			{name: "colon", value: []byte("mmc0:")},
			{name: "bracket", value: []byte("mmc[0]")},
			{name: "equals", value: []byte("mmc=0")},
			{name: "pipe", value: []byte("mmc|0")},
			{name: "comma", value: []byte("mmc0,")},
			{name: "quote", value: []byte("\"mmc0\"")},
			{name: "newline", value: []byte("mmc0\nnext")},
			{name: "invalid utf8", value: []byte{0xff}},
			{name: "over 256 bytes", value: []byte(strings.Repeat("m", 257))},
		} {
			t.Run(strconv.Itoa(eventField)+"/"+test.name, func(t *testing.T) {
				values := profilerAuxCloneValues(base[eventField].values)
				values[nameField] = profilerAuxTestValue{wire: 2, bytes: test.value}
				_, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: eventField, Payload: profilerAuxEncodeValues(values)})
				if admission != bodyRejected || reason != "missing_or_invalid_mmc_name" {
					t.Fatalf("unsafe MMC name escaped: admission=%d reason=%q", admission, reason)
				}
			})
		}
	}

	done := base[4015]
	leftValues := profilerAuxCloneValues(done.values)
	rightValues := profilerAuxCloneValues(done.values)
	leftValues[3], leftValues[7], leftValues[11] = profilerAuxBytes("ABCD"), profilerAuxBytes("EFGH"), profilerAuxBytes("IJKL")
	rightValues[3], rightValues[7], rightValues[11] = profilerAuxBytes("1234"), profilerAuxBytes("5678"), profilerAuxBytes("90ab")
	left, leftAdmission, leftReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4015, Payload: profilerAuxEncodeValues(leftValues)})
	right, rightAdmission, rightReason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4015, Payload: profilerAuxEncodeValues(rightValues)})
	if leftAdmission != bodyAdmitted || rightAdmission != bodyAdmitted || leftReason != "" || rightReason != "" || !reflect.DeepEqual(left, right) {
		t.Fatalf("lossy response fields must be audited then dropped, never promoted to typed facts: left=%+v right=%+v", left, right)
	}
	leftBody, leftOK := renderCanonicalProfilerAuxPayload(left)
	rightBody, rightOK := renderCanonicalProfilerAuxPayload(right)
	if !leftOK || !rightOK || leftBody != rightBody || strings.Contains(leftBody, "ABCD") || strings.Contains(leftBody, "cmd_resp") || strings.Contains(leftBody, "stop_resp") || strings.Contains(leftBody, "sbc_resp") {
		t.Fatalf("lossy response bytes escaped public MMC body: left=%q right=%q", leftBody, rightBody)
	}
	if !strings.Contains(leftBody, " cmd_err=-5 ") || !strings.Contains(leftBody, " data_err=-8 ") || strings.Contains(leftBody, " ret=") {
		t.Fatalf("MMC error semantics regressed: %q", leftBody)
	}

	for _, field := range []int{3, 7, 11} {
		values := profilerAuxCloneValues(done.values)
		values[field] = profilerAuxTestValue{wire: 2, bytes: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
		withinProfile, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4015, Payload: profilerAuxEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" || len(withinProfile.Degradations) != 0 {
			t.Fatalf("16-byte binary response field%d rejected/degraded: payload=%+v admission=%d reason=%q", field, withinProfile, admission, reason)
		}
		values[field] = profilerAuxTestValue{wire: 2, bytes: make([]byte, maxProfilerMMCResponseBytes+1)}
		outOfProfile, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 4015, Payload: profilerAuxEncodeValues(values)})
		wantDegradation := fmt.Sprintf("drop_response_field%d_out_of_source_profile", field)
		if admission != bodyAdmitted || reason != "" || !reflect.DeepEqual(outOfProfile.Degradations, []string{wantDegradation}) {
			t.Fatalf("17-byte response field%d did not remain admitted with field-scoped degradation: payload=%+v admission=%d reason=%q", field, outOfProfile, admission, reason)
		}
		withinBody, withinOK := renderCanonicalProfilerAuxPayload(withinProfile)
		outsideBody, outsideOK := renderCanonicalProfilerAuxPayload(outOfProfile)
		if !withinOK || !outsideOK || withinBody != outsideBody {
			t.Fatalf("drop-only response field%d changed public facts: within=%q/%t outside=%q/%t", field, withinBody, withinOK, outsideBody, outsideOK)
		}
		name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 4015, Payload: profilerAuxEncodeValues(values)})
		if !known || name != "mmc_request_done" || body != outsideBody || !reflect.DeepEqual(degradations, []string{wantDegradation}) {
			t.Fatalf("response field%d degradation was not published locally: known=%t name=%q body=%q degradations=%v", field, known, name, body, degradations)
		}
	}

	coverageValues := profilerAuxCloneValues(done.values)
	coverageValues[3] = profilerAuxTestValue{wire: 2, bytes: make([]byte, maxProfilerMMCResponseBytes+1)}
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 40, 40, "mmc", 4015, profilerAuxEncodeValues(coverageValues)),
	)
	sink, err := newTraceDBRowSink("", 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(protoMessage(2, detail), &seq, sink)
	if err != nil || rows != 1 || seq != 1 || len(sink.rows) != 1 {
		t.Fatalf("drop-only response anomaly suppressed the MMC row: rows=%d seq=%d sink=%+v err=%v", rows, seq, sink.rows, err)
	}
	mmcCoverage := coverageForTable(coverage, "mmc_request_done")
	wantDegradation := "drop_response_field3_out_of_source_profile"
	if mmcCoverage == nil || mmcCoverage.RowsRead != 1 || mmcCoverage.RowsEmitted != 1 ||
		mmcCoverage.FieldSources["degraded_"+wantDegradation+"_rows"] != "1" ||
		!strings.Contains(mmcCoverage.Skipped, wantDegradation+"=1") {
		t.Fatalf("field-scoped MMC response degradation was not disclosed: %+v", mmcCoverage)
	}
}

func TestProfilerAuxMMCCoverageKeepsResponseProvenanceOnDoneOnly(t *testing.T) {
	coverage := make(map[int]*TraceDBCoverage)
	done := profilerFtraceEventRenderCoverage(coverage, 4015)
	if done.FieldSources["response_provenance"] == "" {
		t.Fatal("MMC done coverage lost its audited response-carrier provenance")
	}
	start := profilerFtraceEventRenderCoverage(coverage, 4016)
	if _, exists := start.FieldSources["response_provenance"]; exists {
		t.Fatalf("MMC start coverage invented response fields: %+v", start.FieldSources)
	}
}

func TestProfilerAuxSemanticLabelsCannotRegress(t *testing.T) {
	for _, test := range profilerAuxTestCases() {
		payload, admission, reason := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: test.field, Payload: profilerAuxEncodeValues(test.values)})
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("fixture %s rejected: admission=%d reason=%q", test.name, admission, reason)
		}
		body, ok := renderCanonicalProfilerAuxPayload(payload)
		if !ok {
			t.Fatalf("fixture %s did not render", test.name)
		}
		switch test.field {
		case 4009:
			if !strings.Contains(body, " pino=0x55 ") || strings.Contains(body, "offset=") || strings.Contains(body, "rw=write") {
				t.Fatalf("pino was relabelled as offset/write semantics: %q", body)
			}
		case 4010:
			if !strings.Contains(body, " cp_reason=-2 ") || strings.Contains(body, "offset=") || strings.Contains(body, "rw=write") {
				t.Fatalf("cp_reason was relabelled as offset/write semantics: %q", body)
			}
		case 4011, 4012:
			if !strings.Contains(body, " pos=4096 ") || strings.Contains(body, "offset=") || strings.Contains(body, "rw=write") {
				t.Fatalf("F2FS position semantics regressed: %q", body)
			}
		case 4015:
			if !strings.Contains(body, "struct mmc_request[0x1234]") || !strings.Contains(body, " data_err=-8 ") || strings.Contains(body, " ret=") {
				t.Fatalf("MMC done provenance/error semantics regressed: %q", body)
			}
		case 4016:
			if !strings.Contains(body, "struct mmc_request[0x1234]") || !strings.Contains(body, " tag=-1 ") {
				t.Fatalf("MMC start provenance/tag semantics regressed: %q", body)
			}
		}
	}
}

func TestProfilerAuxRejectedRowsKeepLegalSiblings(t *testing.T) {
	base := profilerAuxCasesByField()
	badPrint := protoBytes(2, []byte("bad\nrow"))
	goodPrint := profilerAuxEncodeValues(base[1109].values)
	badMMCValues := profilerAuxCloneValues(base[4015].values)
	badMMCValues[23] = profilerAuxBytes("bad mmc")
	goodF2FS := profilerAuxEncodeValues(base[4009].values)
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, "bad-print", 1109, badPrint),
		syntheticTracePluginFtraceEvent(2_000, 100, 100, "good-print", 1109, goodPrint),
		syntheticTracePluginFtraceEvent(3_000, 100, 100, "bad-mmc", 4015, profilerAuxEncodeValues(badMMCValues)),
		syntheticTracePluginFtraceEvent(4_000, 100, 100, "good-f2fs", 4009, goodF2FS),
	)
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(protoMessage(2, detail), &seq, sink)
	if err != nil || rows != 2 || seq != 2 || len(sink.rows) != 2 {
		t.Fatalf("local aux rejection poisoned siblings: rows=%d seq=%d sink=%+v coverage=%+v err=%v", rows, seq, sink.rows, coverage, err)
	}
	joined := sink.rows[0].line + "\n" + sink.rows[1].line
	if !strings.Contains(joined, "print: B|7|Frame") || !strings.Contains(joined, "f2fs_sync_file_enter: dev=12:80") || strings.Contains(joined, "bad mmc") || strings.Contains(joined, "bad\nrow") {
		t.Fatalf("valid sibling missing or rejected payload escaped:\n%s", joined)
	}
	printCoverage := coverageForTable(coverage, "print")
	if printCoverage == nil || printCoverage.RowsRead != 2 || printCoverage.RowsEmitted != 1 || !strings.Contains(printCoverage.Skipped, "missing_or_invalid_print_buf=1") {
		t.Fatalf("print local coverage mismatch: %+v", printCoverage)
	}
	mmcCoverage := coverageForTable(coverage, "mmc_request_done")
	if mmcCoverage == nil || mmcCoverage.RowsRead != 1 || mmcCoverage.RowsEmitted != 0 || !strings.Contains(mmcCoverage.Skipped, "missing_or_invalid_mmc_name=1") {
		t.Fatalf("MMC local coverage mismatch: %+v", mmcCoverage)
	}
}

func TestProfilerAuxTraceQueryRoundTripAndMRQIsDisclosureOnly(t *testing.T) {
	base := profilerAuxCasesByField()
	startValues := profilerAuxCloneValues(base[4016].values)
	doneValues := profilerAuxCloneValues(base[4015].values)
	startValues[24] = profilerAuxVarint(0x1111)
	doneValues[22] = profilerAuxVarint(0x2222)
	rows := []struct {
		ts     uint64
		field  int
		values map[int]profilerAuxTestValue
	}{
		{ts: 1_000_000_000, field: 4009, values: base[4009].values},
		{ts: 1_002_000_000, field: 4010, values: base[4010].values},
		{ts: 1_010_000_000, field: 4011, values: base[4011].values},
		{ts: 1_013_000_000, field: 4012, values: base[4012].values},
		{ts: 1_020_000_000, field: 4016, values: startValues},
		{ts: 1_025_000_000, field: 4015, values: doneValues},
		{ts: 1_030_000_000, field: 1109, values: base[1109].values},
	}
	var lines []string
	var mmcStartBody, mmcDoneBody string
	for _, row := range rows {
		event := profilerFtraceEventRecord{Field: row.field, Payload: profilerAuxEncodeValues(row.values), TSNS: row.ts, CPU: 2, PID: 40, TGID: 40, Comm: "io"}
		name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(event)
		if !known || len(degradations) != 0 {
			t.Fatalf("field%d did not render for round-trip: known=%t degradations=%v", row.field, known, degradations)
		}
		if row.field == 4016 {
			mmcStartBody = body
		}
		if row.field == 4015 {
			mmcDoneBody = body
		}
		lines = append(lines, traceDBFormatLine("io", 40, 40, 2, int64(row.ts), 0, 0, name+": "+body))
	}
	startEndpoint := tracequery.DecodePairingEndpoint("mmc_request_start", mmcStartBody, 40)
	doneEndpoint := tracequery.DecodePairingEndpoint("mmc_request_done", mmcDoneBody, 40)
	if !startEndpoint.Recognized || !startEndpoint.KeyKnown || !startEndpoint.PayloadAdmitted ||
		!doneEndpoint.Recognized || !doneEndpoint.KeyKnown || !doneEndpoint.PayloadAdmitted ||
		startEndpoint.SemanticKey != doneEndpoint.SemanticKey {
		t.Fatalf("different disclosed mrq values changed the established coarse pairing identity: start=%+v done=%+v", startEndpoint, doneEndpoint)
	}

	path := filepath.Join(t.TempDir(), "aux-roundtrip.systrace")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("tracequery round-trip: %v", err)
	}
	if len(idx.Events) != len(rows) {
		t.Fatalf("round-trip event count=%d want=%d parse_panics=%d unparsed=%d", len(idx.Events), len(rows), idx.ParseLinePanics, idx.UnparsedLines)
	}
	wantNames := map[string]bool{
		"f2fs_sync_file_enter": false, "f2fs_sync_file_exit": false,
		"f2fs_write_begin": false, "f2fs_write_end": false,
		"mmc_request_start": false, "mmc_request_done": false, "print": false,
	}
	for _, event := range idx.Events {
		if _, expected := wantNames[event.Name]; expected {
			wantNames[event.Name] = true
		}
		if strings.HasPrefix(event.Name, "f2fs_") && (event.FileFields == nil || event.FileFields.Dev == "" || event.FileFields.Ino == "") {
			t.Fatalf("F2FS typed file tuple was not recovered: %+v", event)
		}
		if (event.Name == "f2fs_write_begin" || event.Name == "f2fs_write_end") && event.FileFields.RW != "write" {
			t.Fatalf("F2FS write begin/end lost deterministic consumer operation: %+v", event)
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("round-trip missing event %s: %+v", name, idx.Events)
		}
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0.9, TimeEnd: 1.1})
	paired := map[string]int{}
	for _, item := range stats.StorageLatencyByLayer {
		paired[item.Layer] += item.PairedCount
	}
	if paired["f2fs"] != 2 || paired["mmc"] != 1 {
		t.Fatalf("aux storage round-trip pairing mismatch: paired=%v rows=%+v caveats=%v", paired, stats.StorageLatencyByLayer, stats.Caveats)
	}
}

func profilerAuxEncodeValues(values map[int]profilerAuxTestValue) []byte {
	fields := make([]int, 0, len(values))
	for field := range values {
		fields = append(fields, field)
	}
	sort.Ints(fields)
	var out []byte
	for _, field := range fields {
		out = append(out, profilerAuxEncodeField(field, values[field])...)
	}
	return out
}

func profilerAuxEncodeField(field int, value profilerAuxTestValue) []byte {
	if value.wire == 2 {
		return protoBytes(field, value.bytes)
	}
	return protoVarint(field, value.uint)
}

func profilerAuxWrongWire(field int, value profilerAuxTestValue) []byte {
	if value.wire == 2 {
		return protoVarint(field, 1)
	}
	return protoBytes(field, []byte{1})
}

func profilerAuxAlternateValue(value profilerAuxTestValue) profilerAuxTestValue {
	if value.wire == 2 {
		return profilerAuxTestValue{wire: 2, bytes: append(append([]byte(nil), value.bytes...), []byte("-other")...)}
	}
	return profilerAuxTestValue{wire: 0, uint: value.uint + 1}
}

func profilerAuxCloneValues(values map[int]profilerAuxTestValue) map[int]profilerAuxTestValue {
	out := make(map[int]profilerAuxTestValue, len(values))
	for field, value := range values {
		value.bytes = append([]byte(nil), value.bytes...)
		out[field] = value
	}
	return out
}

func profilerAuxCasesByField() map[int]profilerAuxTestCase {
	out := map[int]profilerAuxTestCase{}
	for _, test := range profilerAuxTestCases() {
		out[test.field] = test
	}
	return out
}

func profilerAuxFieldReason(field int, suffix string) string {
	return "core_field" + strconv.Itoa(field) + "_" + suffix
}

func profilerAuxExplicitZeroFields(fields ...int) []byte {
	var out []byte
	for _, field := range fields {
		out = append(out, protoVarint(field, 0)...)
	}
	return out
}
