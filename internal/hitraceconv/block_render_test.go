package hitraceconv

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestBlockCanonicalDirectStructuredParity209To212(t *testing.T) {
	dev := uint64(12<<20 | 80)
	oldDev := uint64(8<<20 | 1)
	tests := []struct {
		name       string
		field      int
		values     directBlockFixtureValues
		structured []byte
		want       string
	}{
		{
			name: "block_rq_complete", field: 209,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, rwbs: "RCVHS", cmd: "READ", errorCode: -5},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8),
				protoVarint(4, math.MaxUint64-4), protoBytes(5, []byte("RCVHS")), protoBytes(6, []byte("READ"))),
			want: "12,80 RCVHS (READ) 123 + 8 [-5]",
		},
		{
			name: "block_rq_insert", field: 210,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, bytes: 4096, rwbs: "RCVHS", cmd: "READ", comm: "io-worker"},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8), protoVarint(4, 4096),
				protoBytes(5, []byte("RCVHS")), protoBytes(6, []byte("io-worker")), protoBytes(7, []byte("READ"))),
			want: "12,80 RCVHS 4096 (READ) 123 + 8 [io-worker]",
		},
		{
			name: "block_rq_issue", field: 211,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, bytes: 4096, rwbs: "RCVHS", cmd: "READ", comm: "io-worker"},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8), protoVarint(4, 4096),
				protoBytes(5, []byte("RCVHS")), protoBytes(6, []byte("io-worker")), protoBytes(7, []byte("READ"))),
			want: "12,80 RCVHS 4096 (READ) 123 + 8 [io-worker]",
		},
		{
			name: "block_rq_remap", field: 212,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, oldDev: oldDev, oldSector: 77, nrBios: 2, rwbs: "RCVHS"},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8), protoVarint(4, oldDev),
				protoVarint(5, 77), protoVarint(6, 2), protoBytes(7, []byte("RCVHS"))),
			want: "12,80 RCVHS 123 + 8 <- (8,1) 77 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, content := directBlockFixture(tt.name, tt.values)
			direct, directKnown := renderEventBody(decodeEvent(format, content), content, 0)
			name, structured, structuredKnown, degradations := renderProfilerFtraceEventBodyWithAudit(
				profilerFtraceEventRecord{Field: tt.field, Payload: tt.structured})
			if !directKnown || !structuredKnown || name != tt.name || len(degradations) != 0 || direct != tt.want || structured != tt.want {
				t.Fatalf("block parity mismatch: direct_known=%v structured_known=%v name=%q degradations=%v\n direct=%q\n structured=%q\n want=%q",
					directKnown, structuredKnown, name, degradations, direct, structured, tt.want)
			}
		})
	}

	format, content := directBlockFixture("block_bio_remap", directBlockFixtureValues{
		dev: dev, sector: 123, nrSector: 8, oldDev: oldDev, oldSector: 77, rwbs: "RCVHS",
	})
	if body, known := renderEventBody(decodeEvent(format, content), content, 0); !known || body != "12,80 RCVHS 123 + 8 <- (8,1) 77" {
		t.Fatalf("block_bio_remap profile mismatch: known=%v body=%q", known, body)
	}
}

func TestBlockCanonicalDirectStructuredParityBio202204205(t *testing.T) {
	dev := uint64(12<<20 | 80)
	oldDev := uint64(8<<20 | 1)
	tests := []struct {
		name       string
		field      int
		values     directBlockFixtureValues
		structured []byte
		want       string
	}{
		{
			name: "block_bio_complete", field: 202,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, rwbs: "R", errorCode: -5},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8),
				protoVarint(4, math.MaxUint64-4), protoBytes(5, []byte("R"))),
			want: "12,80 R 123 + 8 [-5]",
		},
		{
			name: "block_bio_queue", field: 204,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, rwbs: "R", comm: "io-worker"},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8),
				protoBytes(4, []byte("R")), protoBytes(5, []byte("io-worker"))),
			want: "12,80 R 123 + 8 [io-worker]",
		},
		{
			name: "block_bio_remap", field: 205,
			values: directBlockFixtureValues{dev: dev, sector: 123, nrSector: 8, oldDev: oldDev, oldSector: 77, rwbs: "R"},
			structured: protoPayload(protoVarint(1, dev), protoVarint(2, 123), protoVarint(3, 8),
				protoVarint(4, oldDev), protoVarint(5, 77), protoBytes(6, []byte("R"))),
			want: "12,80 R 123 + 8 <- (8,1) 77",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, content := directBlockFixture(tt.name, tt.values)
			direct, directKnown := renderEventBody(decodeEvent(format, content), content, 0)
			name, structured, structuredKnown, degradations := renderProfilerFtraceEventBodyWithAudit(
				profilerFtraceEventRecord{Field: tt.field, Payload: tt.structured})
			if !directKnown || !structuredKnown || name != tt.name || len(degradations) != 0 || direct != tt.want || structured != tt.want {
				t.Fatalf("bio parity mismatch: direct_known=%v structured_known=%v name=%q degradations=%v direct=%q structured=%q want=%q",
					directKnown, structuredKnown, name, degradations, direct, structured, tt.want)
			}
		})
	}
}

func TestBlockProfilerExactZeroOmissionParity(t *testing.T) {
	tests := []struct {
		field    int
		omitted  []byte
		explicit []byte
		want     string
	}{
		{
			field:    202,
			omitted:  protoBytes(5, []byte("FS")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoBytes(5, []byte("FS"))),
			want:     "0,0 FS 0 + 0 [0]",
		},
		{
			field:    204,
			omitted:  protoBytes(4, []byte("FS")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoBytes(4, []byte("FS")), protoBytes(5, nil)),
			want:     "0,0 FS 0 + 0 []",
		},
		{
			field:    205,
			omitted:  protoBytes(6, []byte("FS")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoVarint(5, 0), protoBytes(6, []byte("FS"))),
			want:     "0,0 FS 0 + 0 <- (0,0) 0",
		},
		{
			field:    209,
			omitted:  protoBytes(5, []byte("FS")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoBytes(5, []byte("FS")), protoBytes(6, nil)),
			want:     "0,0 FS () 0 + 0 [0]",
		},
		{
			field:    211,
			omitted:  protoBytes(5, []byte("FS")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoBytes(5, []byte("FS")), protoBytes(6, nil), protoBytes(7, nil)),
			want:     "0,0 FS 0 () 0 + 0 []",
		},
		{
			// Rendering is inventory-preserving. The downstream pairing gate,
			// not the converter, decides that an ordinary R 0+0 cannot author
			// elapsed latency.
			field:    211,
			omitted:  protoBytes(5, []byte("R")),
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoBytes(5, []byte("R")), protoBytes(6, nil), protoBytes(7, nil)),
			want:     "0,0 R 0 () 0 + 0 []",
		},
	}
	for _, tt := range tests {
		_, omitted, omittedKnown, omittedDegradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: tt.field, Payload: tt.omitted})
		_, explicit, explicitKnown, explicitDegradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: tt.field, Payload: tt.explicit})
		if !omittedKnown || !explicitKnown || len(omittedDegradations) != 0 || len(explicitDegradations) != 0 || omitted != tt.want || explicit != tt.want {
			t.Fatalf("field %d zero parity mismatch: omitted=(%v,%q,%v) explicit=(%v,%q,%v) want=%q",
				tt.field, omittedKnown, omitted, omittedDegradations, explicitKnown, explicit, explicitDegradations, tt.want)
		}
	}

	directFormat, directContent := directBlockFixture("block_rq_issue", directBlockFixtureValues{rwbs: "FS"})
	if directBody, directKnown := renderEventBody(decodeEvent(directFormat, directContent), directContent, 0); !directKnown || directBody != "0,0 FS 0 () 0 + 0 []" {
		t.Fatalf("direct dev0/FS0 inventory was not preserved: known=%v body=%q", directKnown, directBody)
	}
}

func TestBlockProfilerHardWireAndDomainFailures(t *testing.T) {
	validIssue := func() []byte {
		return protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1536),
			protoBytes(5, []byte("R")), protoBytes(6, []byte("io")), protoBytes(7, []byte("READ")))
	}
	tests := []struct {
		name   string
		field  int
		data   []byte
		reason string
	}{
		{name: "malformed wire", field: 211, data: append(validIssue(), 0x80), reason: "block_payload_malformed_wire"},
		{name: "dev wrong wire", field: 211, data: protoPayload(protoBytes(1, []byte{1}), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1), protoBytes(5, []byte("R"))), reason: "core_field1_wrong_wire"},
		{name: "dev duplicate", field: 211, data: append(validIssue(), protoVarint(1, 1)...), reason: "core_field1_duplicate"},
		{name: "dev range", field: 211, data: protoPayload(protoVarint(1, uint64(math.MaxUint32)+1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1), protoBytes(5, []byte("R"))), reason: "core_field1_out_of_range"},
		{name: "sector range", field: 211, data: protoPayload(protoVarint(1, 1), protoVarint(2, uint64(math.MaxInt64)+1), protoVarint(3, 3), protoVarint(4, 1), protoBytes(5, []byte("R"))), reason: "core_field2_out_of_range"},
		{name: "nr sector range", field: 211, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, uint64(math.MaxUint32)+1), protoVarint(4, 1), protoBytes(5, []byte("R"))), reason: "core_field3_out_of_range"},
		{name: "bytes range", field: 211, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, uint64(math.MaxUint32)+1), protoBytes(5, []byte("R"))), reason: "core_field4_out_of_range"},
		{name: "error range", field: 209, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, uint64(math.MaxInt32)+1), protoBytes(5, []byte("R"))), reason: "core_field4_out_of_range"},
		{name: "bio complete error range", field: 202, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, uint64(math.MaxInt32)+1), protoBytes(5, []byte("R"))), reason: "core_field4_out_of_range"},
		{name: "bio queue rwbs absent", field: 204, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoBytes(5, []byte("io"))), reason: "core_field4_missing_or_invalid"},
		{name: "rwbs absent is not RW", field: 211, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1), protoBytes(7, []byte("RW"))), reason: "core_field5_missing_or_invalid"},
		{name: "rwbs wrong wire", field: 211, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1), protoVarint(5, 1)), reason: "core_field5_wrong_wire"},
		{name: "rwbs duplicate", field: 211, data: append(validIssue(), protoBytes(5, []byte("R"))...), reason: "core_field5_duplicate"},
		{name: "old dev range", field: 212, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, uint64(math.MaxUint32)+1), protoVarint(5, 5), protoVarint(6, 1), protoBytes(7, []byte("R"))), reason: "core_field4_out_of_range"},
		{name: "old sector range", field: 212, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 4), protoVarint(5, uint64(math.MaxInt64)+1), protoVarint(6, 1), protoBytes(7, []byte("R"))), reason: "core_field5_out_of_range"},
		{name: "nr bios range", field: 212, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 4), protoVarint(5, 5), protoVarint(6, uint64(math.MaxUint32)+1), protoBytes(7, []byte("R"))), reason: "core_field6_out_of_range"},
		{name: "bio remap old dev range", field: 205, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, uint64(math.MaxUint32)+1), protoVarint(5, 5), protoBytes(6, []byte("R"))), reason: "core_field4_out_of_range"},
		{name: "bio remap rwbs wrong wire", field: 205, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 4), protoVarint(5, 5), protoVarint(6, 1)), reason: "core_field6_wrong_wire"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: tt.field, Payload: tt.data})
			if known || len(degradations) != 1 || degradations[0] != tt.reason {
				t.Fatalf("hard failure mismatch: known=%v degradations=%v want=%q", known, degradations, tt.reason)
			}
		})
	}

	negativeError := protoPayload(protoVarint(1, math.MaxUint32), protoVarint(2, math.MaxInt64), protoVarint(3, math.MaxUint32),
		protoVarint(4, math.MaxUint64), protoBytes(5, []byte("R")))
	_, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 209, Payload: negativeError})
	if !known || len(degradations) != 0 || !strings.HasSuffix(body, "[-1]") {
		t.Fatalf("canonical negative int32 rejected: known=%v body=%q degradations=%v", known, body, degradations)
	}
}

func TestBlockRWBSGrammarIsIdenticalAcrossSources(t *testing.T) {
	invalid := []string{"R|W", "R,W", "123", strings.Repeat("R", 33)}
	for _, rwbs := range invalid {
		t.Run(rwbs, func(t *testing.T) {
			format, content := directBlockFixture("block_rq_issue", directBlockFixtureValues{
				dev: 1, sector: 2, nrSector: 3, bytes: 1536, rwbs: rwbs,
			})
			if _, _, ok := decodeDirectBlockPayload(decodeEvent(format, content), content); ok {
				t.Fatal("direct decoder accepted invalid rwbs")
			}
			structured := protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 1536), protoBytes(5, []byte(rwbs)))
			if _, _, ok, _ := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 211, Payload: structured}); ok {
				t.Fatal("structured decoder accepted invalid rwbs")
			}
			sqlArgs := map[string]traceDBValue{
				"dev": {Valid: true, Text: "1", Datatype: 0}, "sector": {Valid: true, Text: "2", Datatype: 0},
				"nr_sector": {Valid: true, Text: "3", Datatype: 0}, "bytes": {Valid: true, Text: "1536", Datatype: 0},
				"rwbs": {Valid: true, Text: rwbs, Datatype: 1},
			}
			if _, ok := decodeTraceDBBlockPayload("block_rq_issue", sqlArgs, nil); ok {
				t.Fatal("SQL decoder accepted invalid rwbs")
			}
		})
	}
	for _, rwbs := range []string{"RCVHS", "WFCHS", "FS"} {
		if !validBlockRWBS(rwbs) {
			t.Fatalf("customer operation token %q rejected", rwbs)
		}
	}
}

func TestBlockDirectFixedStringsRequireProducerNULTermination(t *testing.T) {
	format, content := directBlockFixture("block_rq_issue", directBlockFixtureValues{
		dev: 1, sector: 2, nrSector: 3, bytes: 1536, rwbs: "ABCDEFGH",
	})
	if _, degradations, ok := decodeDirectBlockPayload(decodeEvent(format, content), content); ok ||
		!stringSliceContains(degradations, "direct_rwbs_truncated_field") {
		t.Fatalf("unterminated required rwbs escaped direct decoder: ok=%v degradations=%v", ok, degradations)
	}

	format, content = directBlockFixture("block_rq_issue", directBlockFixtureValues{
		dev: 1, sector: 2, nrSector: 3, bytes: 1536, rwbs: "R",
		cmd: "ABCDEFGHIJKLMNOP", comm: "abcdefghijklmnop",
	})
	payload, degradations, ok := decodeDirectBlockPayload(decodeEvent(format, content), content)
	if !ok || payload.cmd != "" || payload.comm != "" ||
		!stringSliceContains(degradations, "direct_cmd_truncated_field") ||
		!stringSliceContains(degradations, "direct_comm_truncated_field") ||
		renderCanonicalBlockPayload(payload) != "0,1 R 1536 () 2 + 3 []" {
		t.Fatalf("unterminated optional display was published: ok=%v payload=%+v degradations=%v", ok, payload, degradations)
	}
}

func TestBlockDirectStringsRequirePreciseDescriptorTypes(t *testing.T) {
	baseFormat, baseContent := directBlockFixture("block_rq_issue", directBlockFixtureValues{
		dev: 1, sector: 2, nrSector: 3, bytes: 1536, rwbs: "R",
	})
	for _, fieldType := range []string{"char *", "struct char_token", "union char_token", "enum char_token", "__rel_loc char[]"} {
		t.Run(fieldType, func(t *testing.T) {
			format := eventFormat{Name: baseFormat.Name, Fields: append([]eventField(nil), baseFormat.Fields...)}
			directSetField(&format, "rwbs", func(field *eventField) { field.Type = fieldType })
			if _, degradations, ok := decodeDirectBlockPayload(decodeEvent(format, baseContent), baseContent); ok ||
				!stringSliceContains(degradations, "direct_rwbs_wrong_type") {
				t.Fatalf("imprecise descriptor type escaped: ok=%v degradations=%v", ok, degradations)
			}
		})
	}
	for _, mutate := range []func(*eventField){
		func(field *eventField) { field.Name = "rwbs" },
		func(field *eventField) { field.Name = "rwbs[7]" },
		func(field *eventField) { field.Name = "rwbs[x]" },
	} {
		format := eventFormat{Name: baseFormat.Name, Fields: append([]eventField(nil), baseFormat.Fields...)}
		directSetField(&format, "rwbs", mutate)
		if _, _, ok := decodeDirectBlockPayload(decodeEvent(format, baseContent), baseContent); ok {
			t.Fatal("malformed fixed-array descriptor escaped")
		}
	}

	dataLocFormat := eventFormat{Name: baseFormat.Name, Fields: append([]eventField(nil), baseFormat.Fields...)}
	dataLocContent := append([]byte(nil), baseContent...)
	rwbsOffset := -1
	directSetField(&dataLocFormat, "rwbs", func(field *eventField) {
		rwbsOffset = field.Offset
		field.Type = "__data_loc char[]"
		field.Name = "rwbs"
		field.Size = 4
	})
	text := []byte("RCVHS\x00")
	locOffset := len(dataLocContent)
	dataLocContent = append(dataLocContent, text...)
	binary.LittleEndian.PutUint32(dataLocContent[rwbsOffset:], uint32(len(text))<<16|uint32(locOffset))
	payload, degradations, ok := decodeDirectBlockPayload(decodeEvent(dataLocFormat, dataLocContent), dataLocContent)
	if !ok || payload.rwbs != "RCVHS" || len(degradations) != 0 {
		t.Fatalf("precise __data_loc char[] descriptor rejected: ok=%v payload=%+v degradations=%v", ok, payload, degradations)
	}
}

func TestBlockOptionalDisplayCannotRewritePositionalGrammar(t *testing.T) {
	structured := protoPayload(protoVarint(1, 1), protoVarint(2, 99), protoVarint(3, 8), protoVarint(4, 4096),
		protoBytes(5, []byte("R")), protoBytes(6, []byte("io] forged")), protoBytes(7, []byte("READ) 1 + 1")))
	_, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 211, Payload: structured})
	if !known || body != "0,1 R 4096 () 99 + 8 []" ||
		!stringSliceContains(degradations, "comm_unsafe_omitted") || !stringSliceContains(degradations, "cmd_unsafe_omitted") {
		t.Fatalf("structured optional display isolation failed: known=%v body=%q degradations=%v", known, body, degradations)
	}

	format, content := directBlockFixture("block_rq_issue", directBlockFixtureValues{
		dev: 1, sector: 99, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io] forged", cmd: "READ) 1 + 1",
	})
	payload, directDegradations, directKnown := decodeDirectBlockPayload(decodeEvent(format, content), content)
	if !directKnown || renderCanonicalBlockPayload(payload) != "0,1 R 4096 () 99 + 8 []" ||
		!stringSliceContains(directDegradations, "direct_comm_unsafe_omitted") || !stringSliceContains(directDegradations, "direct_cmd_unsafe_omitted") {
		t.Fatalf("direct optional display isolation failed: known=%v body=%q degradations=%v", directKnown, renderCanonicalBlockPayload(payload), directDegradations)
	}

	wrongOptionalWire := protoPayload(protoVarint(1, 1), protoVarint(2, 99), protoVarint(3, 8), protoVarint(4, 4096),
		protoBytes(5, []byte("R")), protoVarint(6, 1), protoVarint(7, 1))
	_, body, known, degradations = renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 211, Payload: wrongOptionalWire})
	if !known || body != "0,1 R 4096 () 99 + 8 []" ||
		!stringSliceContains(degradations, "comm_wrong_wire") || !stringSliceContains(degradations, "cmd_wrong_wire") {
		t.Fatalf("optional wrong-wire should degrade only display: known=%v body=%q degradations=%v", known, body, degradations)
	}

	bioQueue := protoPayload(protoVarint(1, 1), protoVarint(2, 99), protoVarint(3, 8),
		protoBytes(4, []byte("R")), protoBytes(5, []byte("io] forged")))
	_, body, known, degradations = renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 204, Payload: bioQueue})
	if !known || body != "0,1 R 99 + 8 []" || !stringSliceContains(degradations, "comm_unsafe_omitted") {
		t.Fatalf("bio queue optional display isolation failed: known=%v body=%q degradations=%v", known, body, degradations)
	}
}

func TestBlockDirectPresenceTypeWidthSignAndUnits(t *testing.T) {
	baseFormat, baseContent := directBlockFixture("block_rq_issue", directBlockFixtureValues{
		dev: 1, sector: 9, nrSector: 0, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	withNrBytes := baseFormat
	withNrBytes.Fields = append(append([]eventField(nil), baseFormat.Fields...), eventField{
		Type: "unsigned long", Name: "nr_bytes", Offset: len(baseContent), Size: 8,
	})
	withNrBytesContent := append(append([]byte(nil), baseContent...), make([]byte, 8)...)
	binary.LittleEndian.PutUint64(withNrBytesContent[len(baseContent):], 8192)
	body, known := renderEventBody(decodeEvent(withNrBytes, withNrBytesContent), withNrBytesContent, 0)
	if !known || body != "0,1 R 4096 (READ) 9 + 0 [io]" {
		t.Fatalf("bytes/nr_bytes substituted for sectors: known=%v body=%q", known, body)
	}
	_, structuredBody, structuredKnown, structuredDegradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{
		Field: 211,
		Payload: protoPayload(protoVarint(1, 1), protoVarint(2, 9), protoVarint(3, 0), protoVarint(4, 4096),
			protoBytes(5, []byte("R")), protoBytes(6, []byte("io")), protoBytes(7, []byte("READ"))),
	})
	if !structuredKnown || len(structuredDegradations) != 0 || structuredBody != "0,1 R 4096 (READ) 9 + 0 [io]" {
		t.Fatalf("structured bytes substituted for sectors: known=%v body=%q degradations=%v", structuredKnown, structuredBody, structuredDegradations)
	}

	tests := []struct {
		name   string
		mutate func(eventFormat, []byte) (eventFormat, []byte)
		reason string
	}{
		{
			name: "missing nr sector cannot use bytes",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				format.Fields = directFieldsWithoutCleanName(format.Fields, "nr_sector")
				return format, content
			},
			reason: "direct_nr_sector_missing_field",
		},
		{
			name: "cmd cannot substitute rwbs",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				format.Fields = directFieldsWithoutCleanName(format.Fields, "rwbs")
				return format, content
			},
			reason: "direct_rwbs_missing_field",
		},
		{
			name: "duplicate sector alias",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				format.Fields = append(format.Fields, eventField{Type: "unsigned int", Name: "sectors", Offset: len(content), Size: 4})
				return format, append(content, make([]byte, 4)...)
			},
			reason: "direct_nr_sector_duplicate_alias",
		},
		{
			name: "numeric wrong type",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				directSetField(&format, "dev", func(field *eventField) { field.Type = "char" })
				return format, content
			},
			reason: "direct_dev_wrong_type",
		},
		{
			name: "numeric wrong sign",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				directSetField(&format, "dev", func(field *eventField) { field.Signed = true })
				return format, content
			},
			reason: "direct_dev_wrong_sign",
		},
		{
			name: "numeric wrong width",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				directSetField(&format, "dev", func(field *eventField) { field.Size = 3 })
				return format, content
			},
			reason: "direct_dev_wrong_width",
		},
		{
			name: "numeric out of range",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				binary.LittleEndian.PutUint64(content[:8], uint64(math.MaxUint32)+1)
				return format, content
			},
			reason: "direct_dev_out_of_range",
		},
		{
			name: "truncated hard field",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				return format, content[:7]
			},
			reason: "direct_dev_truncated_field",
		},
		{
			name: "string wrong type",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				directSetField(&format, "rwbs", func(field *eventField) { field.Type = "unsigned long" })
				return format, content
			},
			reason: "direct_rwbs_wrong_type",
		},
		{
			name: "string duplicate alias",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				format.Fields = append(format.Fields, eventField{Type: "char", Name: "op[8]", Offset: len(content), Size: 8})
				return format, append(content, []byte{'R', 0, 0, 0, 0, 0, 0, 0}...)
			},
			reason: "direct_rwbs_duplicate_alias",
		},
		{
			name: "string truncated",
			mutate: func(format eventFormat, content []byte) (eventFormat, []byte) {
				directSetField(&format, "rwbs", func(field *eventField) { field.Offset = len(content) + 1 })
				return format, content
			},
			reason: "direct_rwbs_truncated_field",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := eventFormat{Name: baseFormat.Name, Fields: append([]eventField(nil), baseFormat.Fields...)}
			content := append([]byte(nil), baseContent...)
			format, content = tt.mutate(format, content)
			_, degradations, ok := decodeDirectBlockPayload(decodeEvent(format, content), content)
			if ok || len(degradations) != 1 || degradations[0] != tt.reason {
				t.Fatalf("direct strict failure mismatch: ok=%v degradations=%v want=%q", ok, degradations, tt.reason)
			}
		})
	}
}

func TestBlockProfilerInvalidRowDoesNotPoisonValidSibling(t *testing.T) {
	invalid := protoPayload(protoVarint(1, 1), protoVarint(2, 10), protoVarint(3, 8), protoVarint(4, 4096))
	valid := protoPayload(protoVarint(1, 1), protoVarint(2, 11), protoVarint(3, 8), protoVarint(4, 4096), protoBytes(5, []byte("R")))
	structured := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(1_000_000_000, 100, 100, "io", 211, invalid),
		syntheticTracePluginFtraceEvent(1_100_000_000, 100, 100, "io", 211, valid),
	)
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	item := coverageForTable(coverage, "block_rq_issue")
	if err != nil || rows != 1 || item == nil || item.RowsRead != 2 || item.RowsEmitted != 1 ||
		item.FieldSources["degraded_core_field5_missing_or_invalid_rows"] != "1" ||
		item.FieldSources["pairing_identity"] == "" {
		t.Fatalf("valid sibling or block coverage regressed: rows=%d err=%v item=%+v coverage=%+v", rows, err, item, coverage)
	}
}

func TestBlockProfilerBioDescriptorsReachStructuredExport(t *testing.T) {
	dev := uint64(12<<20 | 80)
	oldDev := uint64(8<<20 | 1)
	structured := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(1_000_000_000, 100, 100, "io", 202,
			protoPayload(protoVarint(1, dev), protoVarint(2, 10), protoVarint(3, 8), protoVarint(4, 0), protoBytes(5, []byte("R")))),
		syntheticTracePluginFtraceEvent(1_100_000_000, 100, 100, "io", 204,
			protoPayload(protoVarint(1, dev), protoVarint(2, 11), protoVarint(3, 8), protoBytes(4, []byte("R")), protoBytes(5, []byte("io")))),
		syntheticTracePluginFtraceEvent(1_200_000_000, 100, 100, "io", 205,
			protoPayload(protoVarint(1, dev), protoVarint(2, 12), protoVarint(3, 8), protoVarint(4, oldDev), protoVarint(5, 77), protoBytes(6, []byte("R")))),
	)
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 3 {
		t.Fatalf("structured bio export failed: rows=%d err=%v coverage=%+v", rows, err, coverage)
	}
	for _, name := range []string{"block_bio_complete", "block_bio_queue", "block_bio_remap"} {
		item := coverageForTable(coverage, name)
		if item == nil || item.RowsRead != 1 || item.RowsEmitted != 1 || item.FieldSources["schema_profile"] == "" {
			t.Fatalf("structured descriptor coverage missing for %s: %+v", name, item)
		}
	}
}

type directBlockFixtureValues struct {
	dev       uint64
	sector    uint64
	nrSector  uint32
	bytes     uint32
	errorCode int32
	rwbs      string
	cmd       string
	comm      string
	oldDev    uint64
	oldSector uint64
	nrBios    uint32
}

func directBlockFixture(name string, values directBlockFixtureValues) (eventFormat, []byte) {
	format := eventFormat{Name: name}
	var content []byte
	addUint := func(fieldType, fieldName string, width int, value uint64, signed bool) {
		offset := len(content)
		content = append(content, make([]byte, width)...)
		switch width {
		case 4:
			binary.LittleEndian.PutUint32(content[offset:], uint32(value))
		case 8:
			binary.LittleEndian.PutUint64(content[offset:], value)
		}
		format.Fields = append(format.Fields, eventField{Type: fieldType, Name: fieldName, Offset: offset, Size: width, Signed: signed})
	}
	addString := func(fieldName string, size int, value string) {
		offset := len(content)
		content = append(content, make([]byte, size)...)
		copy(content[offset:offset+size], value)
		format.Fields = append(format.Fields, eventField{Type: "char", Name: fieldName + "[" + strconv.Itoa(size) + "]", Offset: offset, Size: size})
	}
	addRWBS := func() {
		size := 8
		if len(values.rwbs) > size {
			size = len(values.rwbs)
		}
		addString("rwbs", size, values.rwbs)
	}
	addUint("unsigned long", "dev", 8, values.dev, false)
	addUint("unsigned long", "sector", 8, values.sector, false)
	addUint("unsigned int", "nr_sector", 4, uint64(values.nrSector), false)
	switch name {
	case "block_bio_complete", "block_rq_complete":
		addUint("int", "error", 4, uint64(int64(values.errorCode)), true)
		addRWBS()
		if name == "block_rq_complete" {
			addString("cmd", 16, values.cmd)
		}
	case "block_bio_queue":
		addRWBS()
		addString("comm", 16, values.comm)
	case "block_rq_insert", "block_rq_issue":
		addUint("unsigned int", "bytes", 4, uint64(values.bytes), false)
		addRWBS()
		addString("comm", 16, values.comm)
		addString("cmd", 16, values.cmd)
	case "block_rq_remap", "block_bio_remap":
		addUint("unsigned long", "old_dev", 8, values.oldDev, false)
		addUint("unsigned long", "old_sector", 8, values.oldSector, false)
		if name == "block_rq_remap" {
			addUint("unsigned int", "nr_bios", 4, uint64(values.nrBios), false)
		}
		addRWBS()
	}
	return format, content
}

func directFieldsWithoutCleanName(fields []eventField, name string) []eventField {
	out := make([]eventField, 0, len(fields))
	for _, field := range fields {
		if cleanFieldName(field.Name) != name {
			out = append(out, field)
		}
	}
	return out
}

func directSetField(format *eventFormat, name string, mutate func(*eventField)) {
	for i := range format.Fields {
		if cleanFieldName(format.Fields[i].Name) == name {
			mutate(&format.Fields[i])
			return
		}
	}
}
