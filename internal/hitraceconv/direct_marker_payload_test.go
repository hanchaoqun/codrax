package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDirectMarkerCanonicalProfileMatrix(t *testing.T) {
	longTail := strings.Repeat("界", 126) + " "
	tests := []struct {
		name    string
		fixture directMarkerTestFixture
		want    string
	}{
		{
			name:    "print cstring does not mint address prefix",
			fixture: directMarkerCStringFixture("print", []byte("15|ordinary prose"), true),
			want:    "15|ordinary prose",
		},
		{
			name:    "print accepts exact 32 bit zero IP provenance",
			fixture: directMarkerCStringIP32Fixture([]byte("I|100|ip32"), 0),
			want:    "I|100|ip32",
		},
		{
			name:    "tracing data loc preserves pipes utf8 and trailing space",
			fixture: directMarkerDataLocFixture("tracing_mark_write", []byte("B|100|渲染|阶段 "), false),
			want:    "B|100|渲染|阶段 ",
		},
		{
			name:    "tracing data loc char signedness is nonsemantic",
			fixture: directMarkerSignedDataLocFixture([]byte("I|100|signed-char")),
			want:    "I|100|signed-char",
		},
		{
			name:    "tracing fixed array preserves payload beyond display clamp",
			fixture: directMarkerFixedFixture("tracing_mark_write", []byte(longTail), false, 512),
			want:    longTail,
		},
		{
			name:    "tracing cstring preserves terminal end pipe",
			fixture: directMarkerCStringFixture("tracing_mark_write", []byte("E|100|"), false),
			want:    "E|100|",
		},
		{
			name:    "one terminal lf only is normalized",
			fixture: directMarkerDataLocFixture("tracing_mark_write", []byte("I|100|point \n"), false),
			want:    "I|100|point ",
		},
		{
			name:    "legacy tracing begin",
			fixture: directMarkerLegacyFixture("tracing_mark_write", 1, 100, "legacy begin "),
			want:    "B|100|legacy begin ",
		},
		{
			name:    "legacy tracing end",
			fixture: directMarkerLegacyFixture("tracing_mark_write", 0, 100, ""),
			want:    "E|100|",
		},
		{
			name:    "legacy PID zero remains a source value",
			fixture: directMarkerLegacyFixture("tracing_mark_write", 1, 0, "zero-owner"),
			want:    "B|0|zero-owner",
		},
		{
			name:    "legacy PID signed int32 maximum",
			fixture: directMarkerLegacyFixture("tracing_mark_write", 1, 1<<31-1, "max-owner"),
			want:    "B|2147483647|max-owner",
		},
		{
			name:    "legacy xacct begin",
			fixture: directMarkerLegacyFixture("tracing_mark_write_xacct", 1, 100, "xacct"),
			want:    "B|100|xacct",
		},
		{
			name:    "legacy reversed xacct end",
			fixture: directMarkerLegacyFixture("xacct_tracing_mark_write", 0, 100, ""),
			want:    "E|100|",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ev := decodeEvent(test.fixture.format, test.fixture.content)
			payload, admission, reason := decodeDirectMarkerPayload(ev, test.fixture.content)
			if admission != bodyAdmitted || reason != "" {
				t.Fatalf("admission=%d reason=%q payload=%+v", admission, reason, payload)
			}
			body, ok := renderCanonicalMarkerPayload(payload)
			if !ok || body != test.want {
				t.Fatalf("canonical marker: ok=%v got=%q want=%q", ok, body, test.want)
			}
			line, wrappedAdmission, wrappedReason, envelopeOK := renderEventLineDecision(
				renderContext{cmdlines: map[int]string{100: "marker"}, tgids: map[int]int{100: 100}},
				1_000_000, 0, test.fixture.format, test.fixture.content,
			)
			if !envelopeOK || wrappedAdmission != bodyAdmitted || wrappedReason != "" ||
				!strings.HasSuffix(line, test.fixture.format.Name+": "+test.want) {
				t.Fatalf("wrapped marker drifted: envelope=%v admission=%d reason=%q line=%q", envelopeOK, wrappedAdmission, wrappedReason, line)
			}
			if test.fixture.format.Name == "print" && strings.Contains(line, "0x1234:") {
				t.Fatalf("print IP escaped provenance into the canonical payload: %q", line)
			}
		})
	}
}

func TestDirectMarkerMutationMatrix(t *testing.T) {
	assertRejected := func(t *testing.T, fixture directMarkerTestFixture) string {
		t.Helper()
		_, admission, reason := decodeDirectMarkerPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyRejected || reason == "" {
			t.Fatalf("malformed marker was not rejected: admission=%d reason=%q", admission, reason)
		}
		if line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "marker"}, tgids: map[int]int{100: 100}},
			1_000_000, 0, fixture.format, fixture.content); known || line != "" {
			t.Fatalf("governed reject fell through to a legacy/header body: known=%v line=%q", known, line)
		}
		return reason
	}

	t.Run("print requires exact unsigned IP", func(t *testing.T) {
		missing := directMarkerCStringFixture("print", []byte("I|100|point"), true)
		missing.format.Fields = append([]eventField(nil), missing.format.Fields[:4]...)
		missing.format.Fields = append(missing.format.Fields, eventField{Type: "char", Name: "buf", Offset: 16, Size: 0})
		assertRejected(t, missing)

		signed := directMarkerCStringFixture("print", []byte("I|100|point"), true)
		for index := range signed.format.Fields {
			if signed.format.Fields[index].Name == "ip" {
				signed.format.Fields[index].Signed = true
			}
		}
		assertRejected(t, signed)

		wrongType := directMarkerCStringFixture("print", []byte("I|100|point"), true)
		for index := range wrongType.format.Fields {
			if wrongType.format.Fields[index].Name == "ip" {
				wrongType.format.Fields[index].Type = "unsigned int"
			}
		}
		assertRejected(t, wrongType)

		wrongWidth := directMarkerCStringFixture("print", []byte("I|100|point"), true)
		for index := range wrongWidth.format.Fields {
			if wrongWidth.format.Fields[index].Name == "ip" {
				wrongWidth.format.Fields[index].Size = 2
			}
		}
		assertRejected(t, wrongWidth)

		truncated := directMarkerCStringFixture("print", []byte("I|100|point"), true)
		truncated.content = truncated.content[:15]
		assertRejected(t, truncated)

		duplicate := directMarkerPrintDuplicateIPFixture(false)
		assertRejected(t, duplicate)
		duplicate = directMarkerPrintDuplicateIPFixture(true)
		assertRejected(t, duplicate)
	})

	t.Run("buffer and legacy profiles cannot mix", func(t *testing.T) {
		fixture := directMarkerDataLocFixture("tracing_mark_write", []byte("B|100|point"), false)
		fixture.format.Fields = append(fixture.format.Fields,
			eventField{Type: "unsigned int", Name: "start", Offset: 12, Size: 4},
			eventField{Type: "int", Name: "pid", Offset: 16, Size: 4, Signed: true},
			eventField{Type: "char", Name: "name[16]", Offset: 20, Size: 16},
		)
		payload := []byte("B|100|point\x00")
		fixture.content = make([]byte, 36+len(payload))
		directMarkerFillEnvelope(fixture.content)
		binary.LittleEndian.PutUint32(fixture.content[8:12], uint32((len(payload)<<16)|36))
		binary.LittleEndian.PutUint32(fixture.content[12:16], 1)
		binary.LittleEndian.PutUint32(fixture.content[16:20], 100)
		copy(fixture.content[20:36], []byte("legacy\x00"))
		copy(fixture.content[36:], payload)
		assertRejected(t, fixture)
	})

	t.Run("duplicate physical authorities reject even when values agree", func(t *testing.T) {
		for name, conflict := range map[string]bool{"same": false, "conflicting": true} {
			t.Run(name, func(t *testing.T) {
				fixture := directMarkerFixedFixture("tracing_mark_write", []byte("point"), false, 8)
				fixture.format.Fields = append(fixture.format.Fields,
					eventField{Type: "char", Name: "buf[8]", Offset: 16, Size: 8})
				duplicate := append([]byte(nil), fixture.content[8:16]...)
				if conflict {
					duplicate[0] = 'x'
				}
				fixture.content = append(fixture.content, duplicate...)
				assertRejected(t, fixture)
			})
		}
	})

	t.Run("string carrier type and width are exact", func(t *testing.T) {
		wrongAlias := directMarkerDataLocFixture("tracing_mark_write", []byte("point"), false)
		wrongAlias.format.Fields[len(wrongAlias.format.Fields)-1].Name = "buf"
		assertRejected(t, wrongAlias)

		wrongType := directMarkerDataLocFixture("tracing_mark_write", []byte("point"), false)
		wrongType.format.Fields[len(wrongType.format.Fields)-1].Type = "__data_loc __u8[]"
		assertRejected(t, wrongType)

		wrongWidth := directMarkerDataLocFixture("tracing_mark_write", []byte("point"), false)
		wrongWidth.format.Fields[len(wrongWidth.format.Fields)-1].Size = 8
		assertRejected(t, wrongWidth)

		wrongFixed := directMarkerFixedFixture("tracing_mark_write", []byte("point"), false, 8)
		wrongFixed.format.Fields[len(wrongFixed.format.Fields)-1].Type = "unsigned char"
		assertRejected(t, wrongFixed)
	})

	t.Run("data loc is exact and cannot clamp or hide early NUL", func(t *testing.T) {
		for name, mutate := range map[string]func(*directMarkerTestFixture){
			"offset before fixed tail": func(f *directMarkerTestFixture) {
				binary.LittleEndian.PutUint32(f.content[8:12], uint32((4<<16)|8))
			},
			"length exceeds record": func(f *directMarkerTestFixture) {
				binary.LittleEndian.PutUint32(f.content[8:12], uint32((100<<16)|12))
			},
			"zero length": func(f *directMarkerTestFixture) {
				binary.LittleEndian.PutUint32(f.content[8:12], 12)
			},
			"missing terminal NUL": func(f *directMarkerTestFixture) {
				f.content[len(f.content)-1] = 'x'
			},
			"early NUL": func(f *directMarkerTestFixture) {
				f.content[13] = 0
			},
		} {
			t.Run(name, func(t *testing.T) {
				fixture := directMarkerDataLocFixture("tracing_mark_write", []byte("point"), false)
				mutate(&fixture)
				assertRejected(t, fixture)
			})
		}
	})

	t.Run("fixed and cstring require a terminator", func(t *testing.T) {
		fixed := directMarkerFixedFixture("tracing_mark_write", []byte("point"), false, 8)
		for index := 8; index < len(fixed.content); index++ {
			fixed.content[index] = 'x'
		}
		assertRejected(t, fixed)

		tail := directMarkerCStringFixture("tracing_mark_write", []byte("point"), false)
		tail.content[len(tail.content)-1] = 'x'
		assertRejected(t, tail)
	})

	t.Run("fixed suffix after first NUL is nonsemantic producer padding", func(t *testing.T) {
		fixture := directMarkerFixedFixture("tracing_mark_write", []byte("good"), false, 16)
		copy(fixture.content[13:], []byte("padding"))
		payload, admission, reason := decodeDirectMarkerPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" || payload.Buffer != "good" {
			t.Fatalf("fixed-array terminator semantics drifted: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	t.Run("descriptor overlap and fixed bracket mismatch reject", func(t *testing.T) {
		overlap := directMarkerDataLocFixture("tracing_mark_write", []byte("point"), false)
		overlap.format.Fields = append(overlap.format.Fields, eventField{Type: "unsigned int", Name: "vendor", Offset: 10, Size: 4})
		assertRejected(t, overlap)

		bracket := directMarkerFixedFixture("tracing_mark_write", []byte("point"), false, 16)
		bracket.format.Fields[len(bracket.format.Fields)-1].Name = "buf[15]"
		assertRejected(t, bracket)
	})

	t.Run("legacy scalar boundaries reject", func(t *testing.T) {
		badStart := directMarkerLegacyFixture("tracing_mark_write", 2, 100, "name")
		assertRejected(t, badStart)

		negativePID := directMarkerLegacyFixture("tracing_mark_write", 1, -1, "name")
		assertRejected(t, negativePID)

		tooLargePID := directMarkerLegacyFixture("tracing_mark_write", 1, 1<<31, "name")
		assertRejected(t, tooLargePID)

		for _, missingName := range []string{"start", "pid", "name"} {
			fixture := directMarkerLegacyFixture("tracing_mark_write", 1, 100, "name")
			fields := make([]eventField, 0, len(fixture.format.Fields)-1)
			for _, field := range fixture.format.Fields {
				if cleanFieldName(field.Name) != missingName {
					fields = append(fields, field)
				}
			}
			fixture.format.Fields = fields
			assertRejected(t, fixture)
		}

		wrongStart := directMarkerLegacyFixture("tracing_mark_write", 1, 100, "name")
		wrongStart.format.Fields[4].Type = "unsigned long"
		assertRejected(t, wrongStart)
		wrongPID := directMarkerLegacyFixture("tracing_mark_write", 1, 100, "name")
		wrongPID.format.Fields[5].Type = "unsigned int"
		wrongPID.format.Fields[5].Signed = false
		assertRejected(t, wrongPID)
		wrongName := directMarkerLegacyFixture("tracing_mark_write", 1, 100, "name")
		wrongName.format.Fields[6].Name = "name[63]"
		assertRejected(t, wrongName)
	})

	t.Run("unsafe logical payloads reject without trimming", func(t *testing.T) {
		for name, payload := range map[string][]byte{
			"blank":          []byte("   "),
			"internal lf":    []byte("I|1|bad\nrow"),
			"terminal cr":    []byte("I|1|bad\r"),
			"tab":            []byte("I|1|bad\trow"),
			"invalid utf8":   {0xff, 0xfe},
			"line separator": []byte("I|1|bad\u2028row"),
			"paragraph sep":  []byte("I|1|bad\u2029row"),
			"double lf":      []byte("I|1|bad\n\n"),
		} {
			t.Run(name, func(t *testing.T) {
				assertRejected(t, directMarkerDataLocFixture("tracing_mark_write", payload, false))
			})
		}
	})

	t.Run("full rendered line cap rejects atomically", func(t *testing.T) {
		ctx := renderContext{cmdlines: map[int]string{100: "marker"}, tgids: map[int]int{100: 100}}
		seed := directMarkerCStringFixture("tracing_mark_write", []byte("x"), false)
		seedLine, seedAdmission, seedReason, seedEnvelopeOK := renderEventLineDecision(
			ctx, 1_000_000, 0, seed.format, seed.content)
		if !seedEnvelopeOK || seedAdmission != bodyAdmitted || seedReason != "" {
			t.Fatalf("seed marker did not render: envelope=%v admission=%d reason=%q", seedEnvelopeOK, seedAdmission, seedReason)
		}
		overhead := len(seedLine) - 1
		exact := directMarkerCStringFixture("tracing_mark_write", []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes-overhead)), false)
		exactLine, exactAdmission, exactReason, exactEnvelopeOK := renderEventLineDecision(
			ctx, 1_000_000, 0, exact.format, exact.content)
		if !exactEnvelopeOK || exactAdmission != bodyAdmitted || exactReason != "" || len(exactLine) != maxTraceDBSystraceLineBytes {
			t.Fatalf("exact line cap was not admitted: envelope=%v admission=%d reason=%q len=%d", exactEnvelopeOK, exactAdmission, exactReason, len(exactLine))
		}

		fixture := directMarkerCStringFixture("tracing_mark_write", []byte(strings.Repeat("x", maxTraceDBSystraceLineBytes-overhead+1)), false)
		line, admission, reason, envelopeOK := renderEventLineDecision(
			ctx,
			1_000_000, 0, fixture.format, fixture.content,
		)
		if !envelopeOK || admission != bodyRejected || reason != "invalid_rendered_line" || len(line) <= maxTraceDBSystraceLineBytes {
			t.Fatalf("overlong governed marker escaped: envelope=%v admission=%d reason=%q len=%d", envelopeOK, admission, reason, len(line))
		}
		if public, known := renderEventLine(ctx, 1_000_000, 0, fixture.format, fixture.content); known || public != "" {
			t.Fatalf("overlong governed marker was not rejected atomically: known=%v len=%d", known, len(public))
		}
	})
}

func TestEventFormatZeroSizedTailIsMarkerScoped(t *testing.T) {
	var blocks []string
	blocks = append(blocks, syntheticFormatBlock("print", 200, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char", "buf[]", 8, 0, false),
	})...)
	blocks = append(blocks, syntheticFormatBlock("tracing_mark_write", 201, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char []", "buffer", 8, 0, true),
	})...)
	for index, name := range []string{"tracing_mark_write_xacct", "xacct_tracing_mark_write"} {
		blocks = append(blocks, syntheticFormatBlock(name, 202+index, []string{
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("char", "buf", 8, 0, false),
		})...)
	}
	blocks = append(blocks,
		syntheticFormatBlock("vendor_print", 300, []string{
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("char", "buf", 8, 0, false),
		})...,
	)
	blocks = append(blocks,
		syntheticFormatBlock("print_suffix", 301, []string{
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("char", "buf", 8, 0, false),
		})...,
	)
	blocks = append(blocks,
		syntheticFormatBlock("print", 302, []string{
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("char *", "buf", 8, 0, false),
		})...,
	)
	blocks = append(blocks,
		syntheticFormatBlock("print", 303, []string{
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("char", "str", 8, 0, false),
		})...,
	)
	blocks = append(blocks,
		syntheticFormatBlock("print", 304, []string{
			syntheticField("int", "common_pid", 12, 4, true),
			syntheticField("char", "buf", 8, 0, false),
		})...,
	)
	blocks = append(blocks,
		syntheticFormatBlock("sched_wakeup", 400, matrixWakeupFields())...,
	)

	catalog, err := parseEventFormats([]byte(strings.Join(blocks, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	for id := 200; id <= 201; id++ {
		if catalog.Poisoned[id] || catalog.Formats[id].Name == "" {
			t.Fatalf("governed marker CSTRING descriptor %d was not admitted: %+v", id, catalog)
		}
	}
	for _, id := range []int{202, 203, 300, 301, 302, 303, 304} {
		if !catalog.Poisoned[id] {
			t.Fatalf("out-of-profile zero-sized descriptor %d was admitted: %+v", id, catalog.Formats[id])
		}
	}
	if catalog.Poisoned[400] || catalog.Formats[400].Name != "sched_wakeup" {
		t.Fatalf("malformed marker descriptor poisoned a legal sibling: %+v", catalog)
	}
}

func TestBuiltinDirectMarkerBadRowIsLocal(t *testing.T) {
	format := syntheticFormatBlock("tracing_mark_write", 500, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("__data_loc char[]", "buffer", 8, 4, false),
	})
	goodA := directMarkerDataLocFixture("tracing_mark_write", []byte(strings.Repeat("a", 380)+" "), false).content
	bad := directMarkerDataLocFixture("tracing_mark_write", []byte("I|100|bad\ninjected"), false).content
	goodB := directMarkerDataLocFixture("tracing_mark_write", []byte("I|100|good"), false).content

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(format, "\n")))
	writeSegment(&capture, segmentCmdlines, []byte("100 marker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 500, OffsetNS: 0, Content: goodA},
		{EventID: 500, OffsetNS: 1_000, Content: bad},
		{EventID: 500, OffsetNS: 2_000, Content: goodB},
	}))

	dir := t.TempDir()
	input := filepath.Join(dir, "marker.sys")
	output := filepath.Join(dir, "marker.systrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if result.EventsWritten != 2 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 ||
		strings.Count(text, "tracing_mark_write:") != 2 || strings.Contains(text, "injected") ||
		!strings.Contains(text, strings.Repeat("a", 380)+" ") || !strings.Contains(text, "I|100|good") {
		t.Fatalf("bad marker escaped or legal sibling was lost: result=%+v\n%s", result, text)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 governed direct ftrace event row") ||
		!strings.Contains(caveats, "tracing_mark_write_missing_or_invalid_marker_buf=1") {
		t.Fatalf("marker rejection coverage missing or mislabeled: %s", caveats)
	}
}

func TestBuiltinDirectPrintCStringDescriptorProfiles(t *testing.T) {
	formatA := syntheticFormatBlock("print", 501, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "ip", 8, 8, false),
		syntheticField("char", "buf[]", 16, 0, false),
	})
	formatB := syntheticFormatBlock("print", 502, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "ip", 8, 8, false),
		syntheticField("char []", "buf", 16, 0, true),
	})
	contentA := directMarkerCStringFixture("print", []byte("15|not-a-carved-action "), true).content
	contentB := directMarkerCStringFixture("print", []byte("I|100|typed "), true).content

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(append(formatA, formatB...), "\n")))
	writeSegment(&capture, segmentCmdlines, []byte("100 marker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 501, OffsetNS: 0, Content: contentA},
		{EventID: 502, OffsetNS: 1_000, Content: contentB},
	}))

	dir := t.TempDir()
	input := filepath.Join(dir, "print-cstring.sys")
	output := filepath.Join(dir, "print-cstring.systrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if result.EventsWritten != 2 || strings.Count(text, "print:") != 2 ||
		!strings.Contains(text, "print: 15|not-a-carved-action ") ||
		!strings.Contains(text, "print: I|100|typed ") || strings.Contains(text, "0x1234:") {
		t.Fatalf("size-zero print CSTRING profiles drifted: result=%+v\n%s", result, text)
	}
}

func TestDirectAndStructuredMarkerCanonicalParity(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("B|100|opaque|name "),
		[]byte("E|100|"),
		[]byte(strings.Repeat("长", 126) + " \n"),
	} {
		direct := directMarkerDataLocFixture("tracing_mark_write", raw, false)
		directPayload, directAdmission, directReason := decodeDirectMarkerPayload(
			decodeEvent(direct.format, direct.content), direct.content)
		structuredPayload, structuredAdmission, structuredReason := decodeProfilerAuxPayload(
			profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, raw)})
		if directAdmission != bodyAdmitted || structuredAdmission != bodyAdmitted || directReason != "" || structuredReason != "" ||
			structuredPayload.Print == nil {
			t.Fatalf("shared marker payload admission drifted: direct=%d/%q structured=%d/%q payload=%+v",
				directAdmission, directReason, structuredAdmission, structuredReason, structuredPayload)
		}
		directBody, directOK := renderCanonicalMarkerPayload(directPayload)
		structuredBody, structuredOK := renderCanonicalProfilerAuxPayload(structuredPayload)
		if !directOK || !structuredOK || directBody != structuredBody {
			t.Fatalf("direct/structured canonical body drifted: direct=%q/%t structured=%q/%t", directBody, directOK, structuredBody, structuredOK)
		}
	}

	bad := []byte("I|100|bad\nrow")
	direct := directMarkerDataLocFixture("tracing_mark_write", bad, false)
	_, directAdmission, _ := decodeDirectMarkerPayload(decodeEvent(direct.format, direct.content), direct.content)
	_, structuredAdmission, _ := decodeProfilerAuxPayload(profilerFtraceEventRecord{Field: 1109, Payload: protoBytes(2, bad)})
	if directAdmission != bodyRejected || structuredAdmission != bodyRejected {
		t.Fatalf("shared invalid marker policy drifted: direct=%d structured=%d", directAdmission, structuredAdmission)
	}
}

func TestDirectMarkerSingleAuthorityStructure(t *testing.T) {
	markerBody, err := os.ReadFile("marker_payload.go")
	if err != nil {
		t.Fatal(err)
	}
	marker := string(markerBody)
	for _, name := range []string{"print", "tracing_mark_write", "tracing_mark_write_xacct", "xacct_tracing_mark_write"} {
		if !strings.Contains(marker, `"`+name+`"`) {
			t.Fatalf("governed marker registry lost %q", name)
		}
	}
	if strings.Count(marker, "func decodeDirectMarkerPayload(") != 1 ||
		strings.Count(marker, "func renderCanonicalMarkerPayload(") != 1 {
		t.Fatalf("marker decoder/renderer authority is not unique")
	}

	legacyBody, err := os.ReadFile("official_render.go")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(legacyBody)
	for _, forbidden := range []string{`case name == "print"`, `case name == "tracing_mark_write"`, `tracing_mark_write_xacct`, `xacct_tracing_mark_write`} {
		if strings.Contains(legacy, forbidden) {
			t.Fatalf("legacy marker authority survived: %q", forbidden)
		}
	}
	renderBody, err := os.ReadFile("render.go")
	if err != nil {
		t.Fatal(err)
	}
	render := string(renderBody)
	if strings.Contains(render, "firstTracePayload") || !strings.Contains(render, "decodeDirectMarkerPayload(ev, content)") ||
		!strings.Contains(render, "coreGoverned || directMarkerNameGoverned(format.Name)") {
		t.Fatalf("marker typed-first/final-line authority drifted")
	}
	eventFormatBody, err := os.ReadFile("eventformat.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(eventFormatBody), "parseFieldLine(cur.Name, line)") {
		t.Fatalf("size-zero CSTRING gate is no longer event-aware")
	}
}

func TestDirectMarkerProductionAuthorityGraph(t *testing.T) {
	targets := map[string]bool{
		"decodeDirectMarkerPayload":    true,
		"renderCanonicalMarkerPayload": true,
		"normalizeMarkerBuffer":        true,
		"firstTracePayload":            true,
	}
	definitions := map[string][]string{}
	calls := map[string][]string{}
	registry := []string{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if targets[function.Name.Name] {
				definitions[function.Name.Name] = append(definitions[function.Name.Name], path)
			}
			if function.Name.Name == "directMarkerNameGoverned" {
				ast.Inspect(function.Body, func(node ast.Node) bool {
					literal, ok := node.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						return true
					}
					value, err := strconv.Unquote(literal.Value)
					if err == nil {
						registry = append(registry, value)
					}
					return true
				})
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				identifier, ok := call.Fun.(*ast.Ident)
				if ok && targets[identifier.Name] {
					calls[identifier.Name] = append(calls[identifier.Name], path+"."+function.Name.Name)
				}
				return true
			})
		}
	}
	for _, name := range []string{"decodeDirectMarkerPayload", "renderCanonicalMarkerPayload", "normalizeMarkerBuffer"} {
		if len(definitions[name]) != 1 {
			t.Fatalf("%s definitions=%v", name, definitions[name])
		}
	}
	if len(definitions["firstTracePayload"]) != 0 || len(calls["firstTracePayload"]) != 0 {
		t.Fatalf("legacy marker authority survived: definitions=%v calls=%v", definitions["firstTracePayload"], calls["firstTracePayload"])
	}
	for name, want := range map[string][]string{
		"decodeDirectMarkerPayload": {
			"render.go.renderEventBodyDecisionWithPair",
		},
		"renderCanonicalMarkerPayload": {
			"profiler_aux_payload.go.renderCanonicalProfilerAuxPayload",
			"render.go.renderEventBodyDecisionWithPair",
		},
		"normalizeMarkerBuffer": {
			"marker_payload.go.decodeDirectMarkerPayload",
			"profiler_aux_payload.go.decodeProfilerAuxPayload",
		},
	} {
		sort.Strings(calls[name])
		sort.Strings(want)
		if strings.Join(calls[name], "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s callsites=%v want=%v", name, calls[name], want)
		}
	}
	sort.Strings(registry)
	wantRegistry := []string{"print", "tracing_mark_write", "tracing_mark_write_xacct", "xacct_tracing_mark_write"}
	sort.Strings(wantRegistry)
	if strings.Join(registry, "\n") != strings.Join(wantRegistry, "\n") {
		t.Fatalf("governed marker registry=%v want=%v", registry, wantRegistry)
	}
}

func TestDirectMarkerDescriptorAuditScalesWithoutQuadraticPairScan(t *testing.T) {
	const vendorFields = 8192
	fields := directMarkerCommonFields()
	offset := 8
	for index := 0; index < vendorFields; index++ {
		fields = append(fields, eventField{
			Type: "unsigned char", Name: "vendor_" + strconv.Itoa(index), Offset: offset, Size: 1,
		})
		offset++
	}
	fields = append(fields, eventField{Type: "__data_loc char[]", Name: "buffer", Offset: offset, Size: 4})
	format := eventFormat{Name: "tracing_mark_write", Fields: fields}
	for iteration := 0; iteration < 5; iteration++ {
		if !directMarkerFormatLayoutValid(format) {
			t.Fatal("large non-overlapping marker descriptor was rejected")
		}
	}
	format.Fields[len(format.Fields)-1].Offset = 7
	if directMarkerFormatLayoutValid(format) {
		t.Fatal("large descriptor overlap was not rejected")
	}
}

type directMarkerTestFixture struct {
	format  eventFormat
	content []byte
}

func directMarkerCommonFields() []eventField {
	return []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
}

func directMarkerFillEnvelope(content []byte) {
	if len(content) >= 8 {
		binary.LittleEndian.PutUint32(content[4:8], 100)
	}
}

func directMarkerDataLocFixture(name string, payload []byte, withIP bool) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	offset := 8
	if withIP {
		fields = append(fields, eventField{Type: "unsigned long", Name: "ip", Offset: offset, Size: 8})
		offset += 8
	}
	fields = append(fields, eventField{Type: "__data_loc char[]", Name: directMarkerTestBufferName(name), Offset: offset, Size: 4})
	payloadOffset := offset + 4
	content := make([]byte, payloadOffset+len(payload)+1)
	directMarkerFillEnvelope(content)
	if withIP {
		binary.LittleEndian.PutUint64(content[8:16], 0x1234)
	}
	binary.LittleEndian.PutUint32(content[offset:offset+4], uint32(((len(payload)+1)<<16)|payloadOffset))
	copy(content[payloadOffset:], payload)
	return directMarkerTestFixture{format: eventFormat{Name: name, Fields: fields}, content: content}
}

func directMarkerFixedFixture(name string, payload []byte, withIP bool, size int) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	offset := 8
	if withIP {
		fields = append(fields, eventField{Type: "unsigned long", Name: "ip", Offset: offset, Size: 8})
		offset += 8
	}
	fields = append(fields, eventField{Type: "char", Name: directMarkerTestBufferName(name) + "[" + strconv.Itoa(size) + "]", Offset: offset, Size: size})
	content := make([]byte, offset+size)
	directMarkerFillEnvelope(content)
	if withIP {
		binary.LittleEndian.PutUint64(content[8:16], 0x1234)
	}
	copy(content[offset:], payload)
	return directMarkerTestFixture{format: eventFormat{Name: name, Fields: fields}, content: content}
}

func directMarkerCStringFixture(name string, payload []byte, withIP bool) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	offset := 8
	if withIP {
		fields = append(fields, eventField{Type: "unsigned long", Name: "ip", Offset: offset, Size: 8})
		offset += 8
	}
	fields = append(fields, eventField{Type: "char", Name: directMarkerTestBufferName(name), Offset: offset, Size: 0})
	content := make([]byte, offset+len(payload)+1)
	directMarkerFillEnvelope(content)
	if withIP {
		binary.LittleEndian.PutUint64(content[8:16], 0x1234)
	}
	copy(content[offset:], payload)
	return directMarkerTestFixture{format: eventFormat{Name: name, Fields: fields}, content: content}
}

func directMarkerCStringIP32Fixture(payload []byte, ip uint32) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	fields = append(fields,
		eventField{Type: "unsigned long", Name: "ip", Offset: 8, Size: 4},
		eventField{Type: "char", Name: "buf", Offset: 12, Size: 0},
	)
	content := make([]byte, 12+len(payload)+1)
	directMarkerFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], ip)
	copy(content[12:], payload)
	return directMarkerTestFixture{format: eventFormat{Name: "print", Fields: fields}, content: content}
}

func directMarkerPrintDuplicateIPFixture(conflict bool) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	fields = append(fields,
		eventField{Type: "unsigned long", Name: "ip", Offset: 8, Size: 8},
		eventField{Type: "unsigned long", Name: "ip[1]", Offset: 16, Size: 8},
		eventField{Type: "char", Name: "buf", Offset: 24, Size: 0},
	)
	payload := []byte("I|100|duplicate-ip")
	content := make([]byte, 24+len(payload)+1)
	directMarkerFillEnvelope(content)
	binary.LittleEndian.PutUint64(content[8:16], 0x1234)
	second := uint64(0x1234)
	if conflict {
		second++
	}
	binary.LittleEndian.PutUint64(content[16:24], second)
	copy(content[24:], payload)
	return directMarkerTestFixture{format: eventFormat{Name: "print", Fields: fields}, content: content}
}

func directMarkerSignedDataLocFixture(payload []byte) directMarkerTestFixture {
	fixture := directMarkerDataLocFixture("tracing_mark_write", payload, false)
	fixture.format.Fields[len(fixture.format.Fields)-1].Signed = true
	return fixture
}

func directMarkerTestBufferName(name string) string {
	if name == "tracing_mark_write" {
		return "buffer"
	}
	return "buf"
}

func directMarkerLegacyFixture(name string, start int64, pid int64, markerName string) directMarkerTestFixture {
	fields := directMarkerCommonFields()
	fields = append(fields,
		eventField{Type: "unsigned int", Name: "start", Offset: 8, Size: 4},
		eventField{Type: "int", Name: "pid", Offset: 12, Size: 4, Signed: true},
		eventField{Type: "char", Name: "name[64]", Offset: 16, Size: 64},
	)
	content := make([]byte, 80)
	directMarkerFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], uint32(start))
	binary.LittleEndian.PutUint32(content[12:16], uint32(pid))
	copy(content[16:], []byte(markerName))
	return directMarkerTestFixture{format: eventFormat{Name: name, Fields: fields}, content: content}
}
