package hitraceconv

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const syntheticSchedBody = "prev_comm=prev prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=next next_pid=101 next_prio=118"

func TestDonghuOfficialSchedSwitchOptionalFieldsRespectFormatPresence(t *testing.T) {
	packed := func(content []byte, offset int) {
		remaining := uint64(5) | uint64(2<<10) | uint64(1<<12) | uint64(3<<13) | uint64(17<<16)
		binary.LittleEndian.PutUint64(content[offset:offset+8], uint64(0xf)|(remaining<<32))
	}
	tests := []struct {
		name          string
		extraFields   []eventField
		fill          func([]byte)
		want          string
		contentLength int
	}{
		{name: "extensions absent", contentLength: 60, want: syntheticSchedBody},
		{
			name:          "expeller exact zero present",
			extraFields:   []eventField{{Type: "unsigned int", Name: "expeller_type", Offset: 60, Size: 4}},
			contentLength: 64,
			want:          syntheticSchedBody + " expeller_type=0",
		},
		{
			name:          "malformed expeller width is omitted",
			extraFields:   []eventField{{Type: "unsigned int", Name: "expeller_type", Offset: 60, Size: 3}},
			contentLength: 63,
			want:          syntheticSchedBody,
		},
		{
			name:          "packed next info exact zero present",
			extraFields:   []eventField{{Type: "unsigned long long", Name: "next_info", Offset: 60, Size: 8}},
			contentLength: 68,
			want:          syntheticSchedBody + " next_info=0,0,0,0,0,0",
		},
		{
			name:          "packed next info nonzero",
			extraFields:   []eventField{{Type: "unsigned long long", Name: "next_info", Offset: 60, Size: 8}},
			contentLength: 68,
			fill:          func(content []byte) { packed(content, 60) },
			want:          syntheticSchedBody + " next_info=f,10,2,1,3,17",
		},
		{
			name: "next info and exact-zero expeller coexist",
			extraFields: []eventField{
				{Type: "unsigned int", Name: "expeller_type", Offset: 60, Size: 4},
				{Type: "unsigned long long", Name: "next_info", Offset: 64, Size: 8},
			},
			contentLength: 72,
			fill:          func(content []byte) { packed(content, 64) },
			want:          syntheticSchedBody + " expeller_type=0 next_info=f,10,2,1,3,17",
		},
		{
			name:          "producer missing sentinel is omitted",
			extraFields:   []eventField{{Type: "unsigned long long", Name: "next_info", Offset: 60, Size: 8}},
			contentLength: 68,
			fill:          func(content []byte) { binary.LittleEndian.PutUint64(content[60:68], math.MaxUint64) },
			want:          syntheticSchedBody,
		},
		{
			name:          "malformed packed width is omitted",
			extraFields:   []eventField{{Type: "unsigned long", Name: "next_info", Offset: 60, Size: 3}},
			contentLength: 63,
			want:          syntheticSchedBody,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := donghuStandardSchedSwitchFormat(tt.extraFields...)
			content := donghuStandardSchedSwitchContent(tt.contentLength)
			if tt.fill != nil {
				tt.fill(content)
			}
			body, known := renderEventBody(decodeEvent(format, content), content, 2)
			if !known || body != tt.want {
				t.Fatalf("sched_switch mismatch: known=%v\n got: %q\nwant: %q", known, body, tt.want)
			}
		})
	}
}

func TestDonghuDirectClockSetRateCPURespectsFieldPresence(t *testing.T) {
	tests := []struct {
		name      string
		fieldSize int
		cpu       uint32
		want      string
	}{
		{name: "missing omits cpu", want: "synthetic_clk state=123456"},
		{name: "exact cpu zero", fieldSize: 4, cpu: 0, want: "synthetic_clk state=123456 cpu_id=0"},
		{name: "nonzero cpu", fieldSize: 4, cpu: 7, want: "synthetic_clk state=123456 cpu_id=7"},
		{name: "maximum cpu", fieldSize: 4, cpu: 4095, want: "synthetic_clk state=123456 cpu_id=4095"},
		{name: "overflow cpu omitted", fieldSize: 4, cpu: 4096, want: "synthetic_clk state=123456"},
		{name: "malformed cpu width omitted", fieldSize: 3, cpu: 7, want: "synthetic_clk state=123456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := []eventField{
				{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
				{Type: "char", Name: "name[16]", Offset: 8, Size: 16},
				{Type: "unsigned int", Name: "state", Offset: 24, Size: 4},
			}
			contentLength := 28
			if tt.fieldSize != 0 {
				fields = append(fields, eventField{Type: "unsigned int", Name: "cpu_id", Offset: 28, Size: tt.fieldSize})
				contentLength += tt.fieldSize
			}
			content := make([]byte, contentLength)
			copy(content[8:24], []byte("synthetic_clk"))
			binary.LittleEndian.PutUint32(content[24:28], 123_456)
			if tt.fieldSize == 4 {
				binary.LittleEndian.PutUint32(content[28:32], tt.cpu)
			} else if tt.fieldSize != 0 {
				content[28] = byte(tt.cpu)
			}
			body, known := renderEventBody(decodeEvent(eventFormat{Name: "clock_set_rate", Fields: fields}, content), content, 3)
			if !known || body != tt.want {
				t.Fatalf("clock mismatch: known=%v got=%q want=%q", known, body, tt.want)
			}
		})
	}
}

func TestDonghuDirectPageCacheOmitsUnprovablePagePointer(t *testing.T) {
	fixture := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
	body, admission, reason := renderEventBodyDecision(
		coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 1,
	)
	if admission != bodyAdmitted || reason != "" || body != "dev 12:48 ino 0x1234 pfn=77 ofs=4096" {
		t.Fatalf("page-cache mismatch: admission=%d reason=%q body=%q", admission, reason, body)
	}
	if strings.Contains(body, "page=") || strings.Contains(body, "order=") {
		t.Fatalf("direct page row exposed an unprovable/undisclosed dimension: %q", body)
	}
}

func TestDonghuProfilerSchedSwitchNextInfoProducerProfile(t *testing.T) {
	base := protoPayload(
		protoBytes(1, []byte("prev")),
		protoVarint(2, 100),
		protoVarint(3, 120),
		protoVarint(4, 1),
		protoBytes(5, []byte("next")),
		protoVarint(6, 101),
		protoVarint(7, 118),
	)
	remaining := uint64(5) | uint64(2<<10) | uint64(1<<12) | uint64(3<<13) | uint64(17<<16)
	nonzero := uint64(0xf) | (remaining << 32)
	tests := []struct {
		name        string
		extra       []byte
		want        string
		degradation string
	}{
		{name: "wire absent exact packed zero", want: syntheticSchedBody + " next_info=0,0,0,0,0,0"},
		{name: "explicit packed zero", extra: protoVarint(8, 0), want: syntheticSchedBody + " next_info=0,0,0,0,0,0"},
		{name: "nonzero", extra: protoVarint(8, nonzero), want: syntheticSchedBody + " next_info=f,10,2,1,3,17"},
		{name: "producer missing sentinel", extra: protoVarint(8, math.MaxUint64), want: syntheticSchedBody},
		{name: "wrong wire degrades", extra: protoBytes(8, []byte{1}), want: syntheticSchedBody, degradation: "next_info_wrong_wire"},
		{name: "duplicate degrades", extra: append(protoVarint(8, nonzero), protoVarint(8, nonzero)...), want: syntheticSchedBody, degradation: "next_info_duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := append(append([]byte(nil), base...), tt.extra...)
			name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 2417, Payload: payload})
			if !known || name != "sched_switch" {
				t.Fatalf("profiler sched switch not rendered: name=%q known=%v body=%q", name, known, body)
			}
			if body != tt.want {
				t.Fatalf("profiler sched mismatch: got=%q want=%q", body, tt.want)
			}
			if tt.degradation == "" && len(degradations) != 0 {
				t.Fatalf("unexpected degradation: %v", degradations)
			}
			if tt.degradation != "" && (len(degradations) != 1 || degradations[0] != tt.degradation) {
				t.Fatalf("degradation mismatch: got=%v want=%q", degradations, tt.degradation)
			}
		})
	}
}

func TestDonghuProfilerSchedIdentityAndPriorityDomains(t *testing.T) {
	payload := func(prevPID, prevPrio, nextPID, nextPrio uint64) []byte {
		return protoPayload(
			protoBytes(1, []byte("prev")), protoVarint(2, prevPID), protoVarint(3, prevPrio), protoVarint(4, 1),
			protoBytes(5, []byte("next")), protoVarint(6, nextPID), protoVarint(7, nextPrio),
		)
	}
	valid := payload(math.MaxInt32, math.MaxUint64-1, math.MaxInt32, math.MaxUint64)
	_, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 2417, Payload: valid})
	if !known || len(degradations) != 0 || !strings.Contains(body, "prev_pid=2147483647") ||
		!strings.Contains(body, "prev_prio=-2") || !strings.Contains(body, "next_pid=2147483647") || !strings.Contains(body, "next_prio=-1") {
		t.Fatalf("valid sched identity/priority domains changed: known=%v body=%q degradations=%v", known, body, degradations)
	}
	tests := []struct {
		name   string
		data   []byte
		reason string
	}{
		{name: "prev pid above int32", data: payload(math.MaxInt32+1, 120, 101, 118), reason: "core_field2_out_of_range"},
		{name: "next pid above int32", data: payload(100, 120, math.MaxInt32+1, 118), reason: "core_field6_out_of_range"},
		{name: "negative prev pid", data: payload(math.MaxUint64, 120, 101, 118), reason: "core_field2_out_of_range"},
		{name: "prev prio above signed int32", data: payload(100, math.MaxInt32+1, 101, 118), reason: "core_field3_out_of_range"},
		{name: "next prio noncanonical negative", data: payload(100, 120, 101, math.MaxUint32), reason: "core_field7_out_of_range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 2417, Payload: tt.data})
			if known || len(degradations) != 1 || degradations[0] != tt.reason {
				t.Fatalf("domain failure mismatch: known=%v degradations=%v want=%q", known, degradations, tt.reason)
			}
		})
	}
}

func TestDonghuProfilerClkSetRateHasNoCPUField(t *testing.T) {
	base := protoPayload(protoBytes(1, []byte("synthetic_clk")), protoVarint(2, 123_456))
	for _, extra := range [][]byte{nil, protoVarint(3, 0), protoVarint(3, 7)} {
		payload := append(append([]byte(nil), base...), extra...)
		name, body, known := renderProfilerFtraceEventBody(profilerFtraceEventRecord{Field: 410, Payload: payload})
		if !known || name != "clock_set_rate" || body != "synthetic_clk state=123456" {
			t.Fatalf("profiler clk set-rate not rendered: name=%q known=%v body=%q", name, known, body)
		}
		if strings.Contains(body, "cpu_id=") {
			t.Fatalf("field 410 schema has no CPU dimension: %q", body)
		}
	}
}

func TestDonghuProfilerPowerClockCPUProducerProfile(t *testing.T) {
	base := protoPayload(protoBytes(1, []byte("synthetic_clk")), protoVarint(2, 123_456))
	tests := []struct {
		name        string
		extra       []byte
		want        string
		degradation string
	}{
		{name: "proto3 omitted exact cpu zero", want: "synthetic_clk state=123456 cpu_id=0"},
		{name: "explicit cpu zero", extra: protoVarint(3, 0), want: "synthetic_clk state=123456 cpu_id=0"},
		{name: "nonzero cpu", extra: protoVarint(3, 7), want: "synthetic_clk state=123456 cpu_id=7"},
		{name: "maximum cpu", extra: protoVarint(3, 4095), want: "synthetic_clk state=123456 cpu_id=4095"},
		{name: "overflow cpu degrades", extra: protoVarint(3, 4096), want: "synthetic_clk state=123456", degradation: "cpu_id_out_of_range"},
		{name: "wrong wire degrades", extra: protoBytes(3, []byte{7}), want: "synthetic_clk state=123456", degradation: "cpu_id_wrong_wire"},
		{name: "duplicate degrades", extra: append(protoVarint(3, 7), protoVarint(3, 7)...), want: "synthetic_clk state=123456", degradation: "cpu_id_duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := append(append([]byte(nil), base...), tt.extra...)
			name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: 2002, Payload: payload})
			if !known || name != "clock_set_rate" {
				t.Fatalf("profiler clock not rendered: name=%q known=%v body=%q", name, known, body)
			}
			if body != tt.want {
				t.Fatalf("profiler clock mismatch: got=%q want=%q", body, tt.want)
			}
			if tt.degradation == "" && len(degradations) != 0 {
				t.Fatalf("unexpected degradation: %v", degradations)
			}
			if tt.degradation != "" && (len(degradations) != 1 || degradations[0] != tt.degradation) {
				t.Fatalf("degradation mismatch: got=%v want=%q", degradations, tt.degradation)
			}
		})
	}
}

func TestDonghuProfilerPowerClockDescriptorIsMapped(t *testing.T) {
	desc, ok := profilerFtraceEventDescriptors[2002]
	if !ok || desc.Name != "clock_set_rate" || desc.Family != "clock" {
		t.Fatalf("power clock_set_rate descriptor missing: ok=%v desc=%+v", ok, desc)
	}
}

func TestDonghuProfilerPageCacheOmitsUnavailablePagePointer(t *testing.T) {
	payload := protoPayload(
		protoVarint(1, 77),
		protoVarint(2, 0x1234),
		protoVarint(3, 1),
		protoVarint(4, uint64((12<<20)|48)),
	)
	for _, field := range []int{1000, 1001} {
		name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: field, Payload: payload})
		if !known || !strings.HasPrefix(name, "mm_filemap_") {
			t.Fatalf("profiler page-cache not rendered: field=%d name=%q known=%v degradations=%v", field, name, known, degradations)
		}
		if body != "dev 12:48 ino 0x1234 pfn=77 ofs=4096" || len(degradations) != 0 {
			t.Fatalf("profiler page-cache mismatch: %q", body)
		}
	}
	for _, field := range []int{1000, 1001} {
		maxDevPayload := protoPayload(
			protoVarint(1, 77), protoVarint(2, 0x1234), protoVarint(3, 1), protoVarint(4, math.MaxUint32),
		)
		_, _, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: field, Payload: maxDevPayload})
		if !known || len(degradations) != 0 {
			t.Fatalf("field%d dev_t MaxUint32 boundary changed: known=%v degradations=%v", field, known, degradations)
		}
		overflowDevPayload := protoPayload(
			protoVarint(1, 77), protoVarint(2, 0x1234), protoVarint(3, 1), protoVarint(4, uint64(math.MaxUint32)+1),
		)
		_, _, known, degradations = renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: field, Payload: overflowDevPayload})
		if known || len(degradations) != 1 || degradations[0] != "filemap_device_invalid" {
			t.Fatalf("field%d dev_t overflow mismatch: known=%v degradations=%v", field, known, degradations)
		}
	}
}

func TestDonghuDirectRendererCoreFieldsFailClosed(t *testing.T) {
	t.Run("typed pid tid boundary", func(t *testing.T) {
		for _, name := range []string{"prev_pid", "next_pid", "prev_tid", "next_tid"} {
			format := eventFormat{Fields: []eventField{{Type: "long long", Name: name, Offset: 0, Size: 8, Signed: true}}}
			content := make([]byte, 8)
			binary.LittleEndian.PutUint64(content, math.MaxInt32)
			if !hasCleanPIDTIDField(decodeEvent(format, content), name) {
				t.Fatalf("%s 8-byte MaxInt32 boundary must remain valid", name)
			}
			binary.LittleEndian.PutUint64(content, math.MaxInt32+1)
			if hasCleanPIDTIDField(decodeEvent(format, content), name) {
				t.Fatalf("%s 8-byte MaxInt32+1 must fail closed", name)
			}
		}
	})

	t.Run("8-byte identity render paths", func(t *testing.T) {
		standard := eventFormat{Name: "sched_switch", Fields: []eventField{
			{Type: "char", Name: "prev_comm[16]", Offset: 0, Size: 16},
			{Type: "long long", Name: "prev_pid", Offset: 16, Size: 8, Signed: true},
			{Type: "int", Name: "prev_prio", Offset: 24, Size: 4, Signed: true},
			{Type: "unsigned int", Name: "prev_state", Offset: 28, Size: 4},
			{Type: "char", Name: "next_comm[16]", Offset: 32, Size: 16},
			{Type: "long long", Name: "next_pid", Offset: 48, Size: 8, Signed: true},
			{Type: "int", Name: "next_prio", Offset: 56, Size: 4, Signed: true},
		}}
		standardContent := make([]byte, 60)
		copy(standardContent[0:16], "prev")
		binary.LittleEndian.PutUint64(standardContent[16:24], math.MaxInt32)
		binary.LittleEndian.PutUint32(standardContent[24:28], 120)
		binary.LittleEndian.PutUint32(standardContent[28:32], 1)
		copy(standardContent[32:48], "next")
		binary.LittleEndian.PutUint64(standardContent[48:56], math.MaxInt32)
		binary.LittleEndian.PutUint32(standardContent[56:60], 118)
		if _, known := renderEventBody(decodeEvent(standard, standardContent), standardContent, 0); !known {
			t.Fatal("standard 8-byte MaxInt32 identities must render")
		}
		for _, offset := range []int{16, 48} {
			bad := append([]byte(nil), standardContent...)
			binary.LittleEndian.PutUint64(bad[offset:offset+8], math.MaxInt32+1)
			if _, known := renderEventBody(decodeEvent(standard, bad), bad, 0); known {
				t.Fatalf("standard 8-byte MaxInt32+1 identity at offset %d must fail", offset)
			}
		}

		harmony := eventFormat{Name: "sched_switch", Fields: []eventField{
			{Type: "char", Name: "pname[16]", Offset: 0, Size: 16},
			{Type: "long long", Name: "prev_tid", Offset: 16, Size: 8, Signed: true},
			{Type: "int", Name: "pprio", Offset: 24, Size: 4, Signed: true},
			{Type: "unsigned int", Name: "pstate", Offset: 28, Size: 4},
			{Type: "char", Name: "nname[16]", Offset: 32, Size: 16},
			{Type: "long long", Name: "next_tid", Offset: 48, Size: 8, Signed: true},
			{Type: "int", Name: "nprio", Offset: 56, Size: 4, Signed: true},
		}}
		harmonyContent := append([]byte(nil), standardContent...)
		if _, known := renderEventBody(decodeEvent(harmony, harmonyContent), harmonyContent, 0); !known {
			t.Fatal("Harmony 8-byte MaxInt32 identities must render")
		}
		for _, offset := range []int{16, 48} {
			bad := append([]byte(nil), harmonyContent...)
			binary.LittleEndian.PutUint64(bad[offset:offset+8], math.MaxInt32+1)
			if _, known := renderEventBody(decodeEvent(harmony, bad), bad, 0); known {
				t.Fatalf("Harmony 8-byte MaxInt32+1 identity at offset %d must fail", offset)
			}
		}
	})

	t.Run("8-byte priority render paths", func(t *testing.T) {
		standard := eventFormat{Name: "sched_switch", Fields: []eventField{
			{Type: "char", Name: "prev_comm[16]", Offset: 0, Size: 16},
			{Type: "int", Name: "prev_pid", Offset: 16, Size: 4, Signed: true},
			{Type: "long long", Name: "prev_prio", Offset: 20, Size: 8, Signed: true},
			{Type: "unsigned int", Name: "prev_state", Offset: 28, Size: 4},
			{Type: "char", Name: "next_comm[16]", Offset: 32, Size: 16},
			{Type: "int", Name: "next_pid", Offset: 48, Size: 4, Signed: true},
			{Type: "long long", Name: "next_prio", Offset: 52, Size: 8, Signed: true},
		}}
		harmony := eventFormat{Name: "sched_switch", Fields: []eventField{
			{Type: "char", Name: "pname[16]", Offset: 0, Size: 16},
			{Type: "int", Name: "prev_tid", Offset: 16, Size: 4, Signed: true},
			{Type: "long long", Name: "pprio", Offset: 20, Size: 8, Signed: true},
			{Type: "unsigned int", Name: "pstate", Offset: 28, Size: 4},
			{Type: "char", Name: "nname[16]", Offset: 32, Size: 16},
			{Type: "int", Name: "next_tid", Offset: 48, Size: 4, Signed: true},
			{Type: "long long", Name: "nprio", Offset: 52, Size: 8, Signed: true},
		}}
		for name, format := range map[string]eventFormat{"standard": standard, "harmony": harmony} {
			content := make([]byte, 60)
			copy(content[0:16], "prev")
			binary.LittleEndian.PutUint32(content[16:20], 100)
			binary.LittleEndian.PutUint64(content[20:28], math.MaxUint64-1)
			binary.LittleEndian.PutUint32(content[28:32], 1)
			copy(content[32:48], "next")
			binary.LittleEndian.PutUint32(content[48:52], 101)
			binary.LittleEndian.PutUint64(content[52:60], 301)
			body, known := renderEventBody(decodeEvent(format, content), content, 0)
			if !known || !strings.Contains(body, "prev_prio=-2") || !strings.Contains(body, "next_prio=301") {
				t.Fatalf("%s valid signed-int32 priorities changed: known=%v body=%q", name, known, body)
			}
			for _, offset := range []int{20, 52} {
				bad := append([]byte(nil), content...)
				binary.LittleEndian.PutUint64(bad[offset:offset+8], math.MaxInt32+1)
				if _, known := renderEventBody(decodeEvent(format, bad), bad, 0); known {
					t.Fatalf("%s out-of-range priority at offset %d must fail closed", name, offset)
				}
			}
		}
	})

	t.Run("standard sched core", func(t *testing.T) {
		for _, missing := range []string{"prev_comm", "prev_pid", "prev_prio", "prev_state", "next_comm", "next_pid", "next_prio"} {
			format := donghuStandardSchedSwitchFormat()
			format.Fields = withoutCleanField(format.Fields, missing)
			content := donghuStandardSchedSwitchContent(60)
			_, known := renderEventBody(decodeEvent(format, content), content, 0)
			if known {
				t.Fatalf("standard sched_switch missing %s must be header-only", missing)
			}
		}
		format := donghuStandardSchedSwitchFormat()
		format.Fields[1].Type = "unsigned long"
		content := donghuStandardSchedSwitchContent(60)
		if _, known := renderEventBody(decodeEvent(format, content), content, 0); known {
			t.Fatal("numeric prev_comm type must not mint a sched_switch payload")
		}
		format = donghuStandardSchedSwitchFormat()
		format.Fields[2].Type = "char"
		content = donghuStandardSchedSwitchContent(60)
		if _, known := renderEventBody(decodeEvent(format, content), content, 0); known {
			t.Fatal("char-declared prev_pid must not mint a scheduler identity")
		}
		content = donghuStandardSchedSwitchContent(60)
		content[8] = '\n'
		format = donghuStandardSchedSwitchFormat()
		if _, known := renderEventBody(decodeEvent(format, content), content, 0); known {
			t.Fatal("control-bearing prev_comm must not mint a sched_switch payload")
		}
		for _, badCG := range [][]byte{
			[]byte("a\nb"), []byte("top cpu_id=7"), []byte("top=bad"), []byte("top|bad"), []byte("top\u00a0cpu"), {0xff, 0xfe},
		} {
			format := donghuStandardSchedSwitchFormat(eventField{Type: "char", Name: "cg[16]", Offset: 60, Size: 16})
			content := donghuStandardSchedSwitchContent(76)
			copy(content[60:76], badCG)
			body, known := renderEventBody(decodeEvent(format, content), content, 0)
			if !known || body != syntheticSchedBody {
				t.Fatalf("invalid optional cgroup must be omitted only: known=%v body=%q", known, body)
			}
		}
		for _, offset := range []int{24, 52} {
			content := donghuStandardSchedSwitchContent(60)
			binary.LittleEndian.PutUint32(content[offset:offset+4], math.MaxUint32)
			if _, known := renderEventBody(decodeEvent(donghuStandardSchedSwitchFormat(), content), content, 0); known {
				t.Fatalf("negative standard scheduler identity at offset %d must fail closed", offset)
			}
		}
		negativePrio := donghuStandardSchedSwitchContent(60)
		binary.LittleEndian.PutUint32(negativePrio[28:32], ^uint32(1))
		binary.LittleEndian.PutUint32(negativePrio[56:60], math.MaxUint32)
		body, known := renderEventBody(decodeEvent(donghuStandardSchedSwitchFormat(), negativePrio), negativePrio, 0)
		if !known || !strings.Contains(body, "prev_prio=-2") || !strings.Contains(body, "next_prio=-1") {
			t.Fatalf("negative scheduler priorities are valid values: known=%v body=%q", known, body)
		}
	})

	t.Run("harmony sched core", func(t *testing.T) {
		for _, missing := range []string{"pname", "prev_tid", "pprio", "pstate", "nname", "next_tid", "nprio"} {
			format := donghuHarmonySchedSwitchFormat(eventField{Name: "ninfo[8]", Offset: 60, Size: 8})
			format.Fields = withoutCleanField(format.Fields, missing)
			content := donghuHarmonySchedSwitchContent(68)
			_, known := renderEventBody(decodeEvent(format, content), content, 0)
			if known {
				t.Fatalf("Harmony sched_switch missing %s must be header-only", missing)
			}
		}
		format := donghuHarmonySchedSwitchFormat()
		prevIdle := donghuHarmonySchedSwitchContent(52)
		clear(prevIdle[0:16])
		binary.LittleEndian.PutUint32(prevIdle[16:20], 0)
		body, known := renderEventBody(decodeEvent(format, prevIdle), prevIdle, 2)
		wantPrevIdle := "prev_comm=tppmgr-idle-2 prev_pid=0 prev_prio=120 prev_state=S ==> next_comm=next next_pid=101 next_prio=118"
		if !known || body != wantPrevIdle {
			t.Fatalf("blank exact-tid0 prev comm must use idle fallback: known=%v body=%q", known, body)
		}
		nextIdle := donghuHarmonySchedSwitchContent(52)
		clear(nextIdle[28:44])
		binary.LittleEndian.PutUint32(nextIdle[44:48], 0)
		body, known = renderEventBody(decodeEvent(format, nextIdle), nextIdle, 3)
		wantNextIdle := "prev_comm=prev prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=tppmgr-idle-3 next_pid=0 next_prio=118"
		if !known || body != wantNextIdle {
			t.Fatalf("blank exact-tid0 next comm must use idle fallback: known=%v body=%q", known, body)
		}
		for _, bad := range [][]byte{nil, []byte("a\nb"), {0xff, 0xfe}} {
			content := donghuHarmonySchedSwitchContent(52)
			clear(content[0:16])
			copy(content[0:16], bad)
			if bad == nil {
				binary.LittleEndian.PutUint32(content[16:20], 100)
			} else {
				binary.LittleEndian.PutUint32(content[16:20], 0)
			}
			if _, known := renderEventBody(decodeEvent(format, content), content, 1); known {
				t.Fatalf("invalid Harmony comm bad=%x must fail closed", bad)
			}
		}
		for _, offset := range []int{16, 44} {
			content := donghuHarmonySchedSwitchContent(52)
			binary.LittleEndian.PutUint32(content[offset:offset+4], math.MaxUint32)
			if _, known := renderEventBody(decodeEvent(format, content), content, 1); known {
				t.Fatalf("negative Harmony scheduler identity at offset %d must fail closed", offset)
			}
		}
		harmonyPrio := donghuHarmonySchedSwitchContent(52)
		binary.LittleEndian.PutUint32(harmonyPrio[20:24], ^uint32(1))
		binary.LittleEndian.PutUint32(harmonyPrio[48:52], 301)
		body, known = renderEventBody(decodeEvent(format, harmonyPrio), harmonyPrio, 1)
		if !known || !strings.Contains(body, "prev_prio=-2") || !strings.Contains(body, "next_prio=301") {
			t.Fatalf("Harmony scheduler priority range must remain signed/open: known=%v body=%q", known, body)
		}
	})

	t.Run("clock name and state", func(t *testing.T) {
		format := eventFormat{Name: "clock_set_rate", Fields: []eventField{
			{Type: "char", Name: "name[16]", Offset: 0, Size: 16},
			{Type: "unsigned int", Name: "state", Offset: 16, Size: 4},
		}}
		content := make([]byte, 20)
		copy(content, "synthetic_clk")
		binary.LittleEndian.PutUint32(content[16:20], 123_456)
		for _, missing := range []string{"name", "state"} {
			candidate := format
			candidate.Fields = withoutCleanField(format.Fields, missing)
			_, known := renderEventBody(decodeEvent(candidate, content), content, 0)
			if known {
				t.Fatalf("clock_set_rate missing %s must be header-only", missing)
			}
		}
		_, known := renderEventBody(decodeEvent(format, content[:18]), content[:18], 0)
		if known {
			t.Fatal("truncated clock state must be header-only")
		}
		for _, badName := range [][]byte{
			[]byte("a\nb"), []byte("clk cpu_id=7"), []byte("clk=bad"), []byte("clk|bad"), []byte("clk\u00a0cpu"), {0xff, 0xfe},
		} {
			badContent := make([]byte, 20)
			copy(badContent[0:16], badName)
			binary.LittleEndian.PutUint32(badContent[16:20], 123_456)
			if _, known := renderEventBody(decodeEvent(format, badContent), badContent, 0); known {
				t.Fatalf("invalid clock name %x must be header-only", badName)
			}
		}
		charState := format
		charState.Fields = append([]eventField(nil), format.Fields...)
		charState.Fields[1].Type = "char"
		if _, known := renderEventBody(decodeEvent(charState, content), content, 0); known {
			t.Fatal("char-declared clock state must fail closed")
		}

		dataLocPayload := []byte("synthetic_dataloc\x00")
		dataLocContent := make([]byte, 8+len(dataLocPayload))
		binary.LittleEndian.PutUint32(dataLocContent[0:4], uint32(len(dataLocPayload))<<16|8)
		binary.LittleEndian.PutUint32(dataLocContent[4:8], 321)
		copy(dataLocContent[8:], dataLocPayload)
		dataLocFormat := eventFormat{Name: "clock_set_rate", Fields: []eventField{
			{Type: "__data_loc char[]", Name: "name", Offset: 0, Size: 4},
			{Type: "unsigned int", Name: "state", Offset: 4, Size: 4},
		}}
		body, known := renderEventBody(decodeEvent(dataLocFormat, dataLocContent), dataLocContent, 0)
		if !known || body != "synthetic_dataloc state=321" {
			t.Fatalf("valid exact4 data_loc clock name mismatch: known=%v body=%q", known, body)
		}

		legacyPayload := []byte("synthetic_legacy\x00")
		legacyContent := make([]byte, 16+len(legacyPayload))
		binary.LittleEndian.PutUint32(legacyContent[0:4], 16)
		binary.LittleEndian.PutUint32(legacyContent[4:8], 654)
		copy(legacyContent[16:], legacyPayload)
		legacyFormat := eventFormat{Name: "clock_set_rate", Fields: []eventField{
			{Type: "unsigned int", Name: "name", Offset: 0, Size: 4},
			{Type: "unsigned int", Name: "state", Offset: 4, Size: 4},
		}}
		body, known = renderEventBody(decodeEvent(legacyFormat, legacyContent), legacyContent, 0)
		if !known || body != "synthetic_legacy state=654" {
			t.Fatalf("valid legacy clock-name locator mismatch: known=%v body=%q", known, body)
		}
		for _, badType := range []string{"unsigned long", "float", "bool"} {
			badFormat := legacyFormat
			badFormat.Fields = append([]eventField(nil), legacyFormat.Fields...)
			badFormat.Fields[0].Type = badType
			if _, known := renderEventBody(decodeEvent(badFormat, legacyContent), legacyContent, 0); known {
				t.Fatalf("legacy locator type %q must fail closed", badType)
			}
		}
		noNULPayload := []byte("legacy_without_nul")
		noNULContent := make([]byte, 16+len(noNULPayload))
		binary.LittleEndian.PutUint32(noNULContent[0:4], 16)
		binary.LittleEndian.PutUint32(noNULContent[4:8], 654)
		copy(noNULContent[16:], noNULPayload)
		if _, known := renderEventBody(decodeEvent(legacyFormat, noNULContent), noNULContent, 0); known {
			t.Fatal("legacy locator without a terminating NUL must fail closed")
		}
		relLocContent := make([]byte, 8)
		copy(relLocContent[0:4], "ABCD")
		binary.LittleEndian.PutUint32(relLocContent[4:8], 654)
		relLocFormat := eventFormat{Name: "clock_set_rate", Fields: []eventField{
			{Type: "__rel_loc char[]", Name: "name", Offset: 0, Size: 4},
			{Type: "unsigned int", Name: "state", Offset: 4, Size: 4},
		}}
		if _, known := renderEventBody(decodeEvent(relLocFormat, relLocContent), relLocContent, 0); known {
			t.Fatal("printable __rel_loc bytes must not be mistaken for an inline clock name")
		}
		for _, size := range []int{1, 2, 8} {
			badContent := make([]byte, 40)
			switch size {
			case 1:
				badContent[0] = 16
			case 2:
				binary.LittleEndian.PutUint16(badContent[0:2], 16)
			case 8:
				binary.LittleEndian.PutUint64(badContent[0:8], 16)
			}
			binary.LittleEndian.PutUint32(badContent[8:12], 321)
			copy(badContent[16:], "synthetic_legacy")
			badFormat := eventFormat{Name: "clock_set_rate", Fields: []eventField{
				{Type: "unsigned long", Name: "name", Offset: 0, Size: size},
				{Type: "unsigned int", Name: "state", Offset: 8, Size: 4},
			}}
			if _, known := renderEventBody(decodeEvent(badFormat, badContent), badContent, 0); known {
				t.Fatalf("legacy numeric clock-name locator width %d must fail", size)
			}
		}
		badDataLoc := dataLocFormat
		badDataLoc.Fields = append([]eventField(nil), dataLocFormat.Fields...)
		badDataLoc.Fields[0].Size = 8
		if _, known := renderEventBody(decodeEvent(badDataLoc, dataLocContent), dataLocContent, 0); known {
			t.Fatal("non-exact4 data_loc clock-name locator must fail")
		}
		dualNameContent := make([]byte, 36)
		copy(dualNameContent[0:16], "clock_a")
		copy(dualNameContent[16:32], "clock_b")
		binary.LittleEndian.PutUint32(dualNameContent[32:36], 321)
		dualNameFormat := eventFormat{Name: "clock_set_rate", Fields: []eventField{
			{Type: "char", Name: "name[16]", Offset: 0, Size: 16},
			{Type: "char", Name: "clk_name[16]", Offset: 16, Size: 16},
			{Type: "unsigned int", Name: "state", Offset: 32, Size: 4},
		}}
		if _, known := renderEventBody(decodeEvent(dualNameFormat, dualNameContent), dualNameContent, 0); known {
			t.Fatal("dual physical clock-name aliases must fail closed")
		}
	})

	t.Run("page cache tuple", func(t *testing.T) {
		base := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
		directFilemapAdmittedBody(t, base)
		for _, missing := range []string{"s_dev", "i_ino", "index", "pfn"} {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields = withoutCleanField(fixture.format.Fields, missing)
			directFilemapAssertRejected(t, fixture)
		}
		truncated := cloneDirectFilemapFixture(base)
		truncated.content = truncated.content[:len(truncated.content)-1]
		directFilemapAssertRejected(t, truncated)
		charIndex := cloneDirectFilemapFixture(base)
		filemapFixtureField(t, &charIndex, "index").Type = "char"
		directFilemapAssertRejected(t, charIndex)
	})
}

func TestDonghuDirectCGroupOptionalAuthority(t *testing.T) {
	payload := []byte("group_a\x00")
	content := donghuStandardSchedSwitchContent(80 + len(payload))
	binary.LittleEndian.PutUint32(content[60:64], uint32(len(payload))<<16|80)
	copy(content[80:], payload)
	format := donghuStandardSchedSwitchFormat(eventField{Type: "__data_loc char[]", Name: "cg", Offset: 60, Size: 4})
	body, known := renderEventBody(decodeEvent(format, content), content, 0)
	if !known || body != syntheticSchedBody+" cg=group_a" {
		t.Fatalf("valid data_loc cgroup mismatch: known=%v body=%q", known, body)
	}

	noNUL := []byte("group_without_nul")
	badContent := donghuStandardSchedSwitchContent(80 + len(noNUL))
	binary.LittleEndian.PutUint32(badContent[60:64], uint32(len(noNUL))<<16|80)
	copy(badContent[80:], noNUL)
	body, known = renderEventBody(decodeEvent(format, badContent), badContent, 0)
	if !known || body != syntheticSchedBody {
		t.Fatalf("unterminated cgroup must omit only the optional dimension: known=%v body=%q", known, body)
	}

	relFormat := donghuStandardSchedSwitchFormat(eventField{Type: "__rel_loc char[]", Name: "cg", Offset: 60, Size: 4})
	relContent := donghuStandardSchedSwitchContent(64)
	copy(relContent[60:64], "ABCD")
	body, known = renderEventBody(decodeEvent(relFormat, relContent), relContent, 0)
	if !known || body != syntheticSchedBody {
		t.Fatalf("unsupported relative cgroup locator must be omitted: known=%v body=%q", known, body)
	}

	dualFormat := donghuStandardSchedSwitchFormat(
		eventField{Type: "char", Name: "cg[16]", Offset: 60, Size: 16},
		eventField{Type: "char", Name: "cgroup[16]", Offset: 76, Size: 16},
	)
	dualContent := donghuStandardSchedSwitchContent(92)
	copy(dualContent[60:76], "group_a")
	copy(dualContent[76:92], "group_a")
	body, known = renderEventBody(decodeEvent(dualFormat, dualContent), dualContent, 0)
	if !known || body != syntheticSchedBody {
		t.Fatalf("dual cgroup aliases must omit the ambiguous optional dimension: known=%v body=%q", known, body)
	}

	truncatedAliasFormat := donghuStandardSchedSwitchFormat(
		eventField{Type: "char", Name: "cg[16]", Offset: 60, Size: 16},
		eventField{Type: "char", Name: "cgroup[16]", Offset: 100, Size: 16},
	)
	truncatedAliasContent := donghuStandardSchedSwitchContent(76)
	copy(truncatedAliasContent[60:76], "group_a")
	body, known = renderEventBody(decodeEvent(truncatedAliasFormat, truncatedAliasContent), truncatedAliasContent, 0)
	if !known || body != syntheticSchedBody {
		t.Fatalf("declared truncated alias must not be rescued: known=%v body=%q", known, body)
	}
}

func TestDonghuHarmonySchedSwitchAliasesAndTextClosedSet(t *testing.T) {
	remaining := uint64(5) | uint64(2<<10) | uint64(1<<12) | uint64(3<<13) | uint64(17<<16)
	packed := uint64(0xf) | (remaining << 32)
	for _, alias := range []string{"ninfo[8]", "next_info"} {
		format := donghuHarmonySchedSwitchFormat(eventField{Type: "unsigned long long", Name: alias, Offset: 60, Size: 8})
		content := donghuHarmonySchedSwitchContent(68)
		binary.LittleEndian.PutUint64(content[60:68], packed)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != syntheticSchedBody+" next_info=f,10,2,1,3,17" {
			t.Fatalf("Harmony alias %s mismatch: known=%v body=%q", alias, known, body)
		}
		binary.LittleEndian.PutUint64(content[60:68], math.MaxUint64)
		body, known = renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != syntheticSchedBody {
			t.Fatalf("Harmony alias %s sentinel mismatch: known=%v body=%q", alias, known, body)
		}
	}
	dual := donghuHarmonySchedSwitchFormat(
		eventField{Type: "unsigned long long", Name: "ninfo[8]", Offset: 60, Size: 8},
		eventField{Type: "unsigned long long", Name: "next_info", Offset: 68, Size: 8},
	)
	dualContent := donghuHarmonySchedSwitchContent(76)
	binary.LittleEndian.PutUint64(dualContent[60:68], packed)
	binary.LittleEndian.PutUint64(dualContent[68:76], packed)
	body, known := renderEventBody(decodeEvent(dual, dualContent), dualContent, 0)
	if !known || body != syntheticSchedBody {
		t.Fatalf("dual next_info aliases must degrade only the optional dimension: known=%v body=%q", known, body)
	}

	tests := []struct {
		value string
		want  string
	}{
		{value: "f,10,2,1,3,17", want: "f,10,2,1,3,17"},
		{value: "f,11,2,1,3,17", want: "f,11,2,1,3,17"},
		{value: "f,10,2,1,3"},
		// NEXTINFO P1 (硬伤C, 2026-07-25): next_info is an incremental format
		// per the customer semantics doc — validated decimal tails pass
		// through verbatim instead of dropping the whole lane.
		{value: "f,10,2,1,3,17,0", want: "f,10,2,1,3,17,0"},
		{value: "f,10,2,1,3,17,x"},
		{value: " f,10,2,1,3,17"},
		// AUD-05(3) (§14.6, 2026-07-25): the text lane stopped enforcing the
		// packed-bit-field ranges — cgid=32 is a doc-legitimate extension
		// value the direct-parse lane keeps (unknown_cgroup_32), so the
		// converter preserves it losslessly instead of dropping the token
		// (converter/direct parity pinned in next_info_differential_test.go).
		{value: "f,10,2,1,3,32", want: "f,10,2,1,3,32"},
		{value: "0x0f,10,2,1,3,17"},
	}
	for _, tt := range tests {
		content := make([]byte, 32)
		copy(content, tt.value)
		ev := decodeEvent(eventFormat{Fields: []eventField{{Type: "char", Name: "next_info[32]", Offset: 0, Size: 32}}}, content)
		if got := harmonySchedInfo(ev); got != tt.want {
			t.Fatalf("text next_info %q: got=%q want=%q", tt.value, got, tt.want)
		}
	}
	cgroupContent := make([]byte, 48)
	copy(cgroupContent[0:32], "f,11,2,1,3")
	copy(cgroupContent[32:48], "group_a")
	cgroupEvent := decodeEvent(eventFormat{Fields: []eventField{
		{Type: "char", Name: "next_info[32]", Offset: 0, Size: 32},
		{Type: "char", Name: "cg[16]", Offset: 32, Size: 16},
	}}, cgroupContent)
	if got := harmonySchedInfo(cgroupEvent); got != "f,11,2,1,3" {
		t.Fatalf("external cgroup requires exact five-field next_info: %q", got)
	}
}

func TestDonghuProfilerCoreWireFailuresRejectWholeRow(t *testing.T) {
	validSched := protoPayload(
		protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
		protoBytes(5, []byte("next")), protoVarint(6, 101), protoVarint(7, 118),
	)
	tests := []struct {
		name   string
		field  int
		data   []byte
		reason string
	}{
		{name: "clk name wrong wire", field: 410, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2)), reason: "core_field1_wrong_wire"},
		{name: "clk name token injection", field: 410, data: protoPayload(protoBytes(1, []byte("clk cpu_id=7")), protoVarint(2, 2)), reason: "core_field1_missing_or_invalid"},
		{name: "power name token injection", field: 2002, data: protoPayload(protoBytes(1, []byte("clk|cpu_id=7")), protoVarint(2, 2)), reason: "core_field1_missing_or_invalid"},
		{name: "power state duplicate", field: 2002, data: protoPayload(protoBytes(1, []byte("clk")), protoVarint(2, 2), protoVarint(2, 3)), reason: "core_field2_duplicate"},
		{name: "sched comm missing", field: 2417, data: protoPayload(protoVarint(2, 100), protoBytes(5, []byte("next"))), reason: "core_field1_missing_or_invalid"},
		{name: "sched pid wrong wire", field: 2417, data: append(append([]byte(nil), validSched...), protoBytes(2, []byte{1})...), reason: "core_field2_wrong_wire"},
		{name: "page pfn wrong wire", field: 1000, data: protoPayload(protoBytes(1, []byte{1}), protoVarint(2, 2), protoVarint(3, 3), protoVarint(4, 4)), reason: "filemap_pfn_invalid"},
		{name: "page inode duplicate", field: 1001, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(2, 3), protoVarint(3, 3), protoVarint(4, 4)), reason: "filemap_inode_invalid"},
		{name: "page offset overflow", field: 1000, data: protoPayload(protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, (uint64(math.MaxInt64)>>12)+1), protoVarint(4, 4)), reason: "filemap_index_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, known, degradations := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: tt.field, Payload: tt.data})
			if known || len(degradations) != 1 || degradations[0] != tt.reason {
				t.Fatalf("core failure mismatch: known=%v degradations=%v want=%q", known, degradations, tt.reason)
			}
		})
	}
}

func TestDonghuProfilerOptionalDegradationIsCountedInCoverage(t *testing.T) {
	schedBase := protoPayload(
		protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
		protoBytes(5, []byte("next")), protoVarint(6, 101), protoVarint(7, 118),
	)
	clockBase := protoPayload(protoBytes(1, []byte("synthetic_clk")), protoVarint(2, 123_456))
	structured := protoMessage(2,
		protoVarint(1, 3),
		syntheticTracePluginFtraceEvent(1_000_000_000, 100, 100, "clock", 2002, append(append([]byte(nil), clockBase...), protoVarint(3, 4096)...)),
		syntheticTracePluginFtraceEvent(1_100_000_000, 100, 100, "clock", 2002, append(append([]byte(nil), clockBase...), protoBytes(3, []byte{7})...)),
		syntheticTracePluginFtraceEvent(1_200_000_000, 100, 100, "sched", 2417, append(append([]byte(nil), schedBase...), protoBytes(8, []byte{1})...)),
		syntheticTracePluginFtraceEvent(1_300_000_000, 100, 100, "page", 1000, protoPayload(
			protoVarint(1, 1), protoVarint(2, 2), protoVarint(3, (uint64(math.MaxInt64)>>12)+1), protoVarint(4, 4),
		)),
	)
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 3 {
		t.Fatalf("render degraded rows: rows=%d err=%v coverage=%+v", rows, err, coverage)
	}
	clock := coverageForTable(coverage, "clock_set_rate")
	if clock == nil || clock.RowsRead != 2 || clock.RowsEmitted != 2 ||
		clock.FieldSources["degraded_cpu_id_out_of_range_rows"] != "1" ||
		clock.FieldSources["degraded_cpu_id_wrong_wire_rows"] != "1" ||
		!strings.Contains(clock.Skipped, "cpu_id_out_of_range=1") || !strings.Contains(clock.Skipped, "cpu_id_wrong_wire=1") {
		t.Fatalf("clock degradation coverage mismatch: %+v", clock)
	}
	sched := coverageForTable(coverage, "sched_switch")
	if sched == nil || sched.FieldSources["degraded_next_info_wrong_wire_rows"] != "1" || sched.Skipped != "next_info_wrong_wire=1" {
		t.Fatalf("sched degradation coverage mismatch: %+v", sched)
	}
	page := coverageForTable(coverage, "mm_filemap_add_to_page_cache")
	if page == nil || page.RowsRead != 1 || page.RowsEmitted != 0 ||
		page.FieldSources["degraded_filemap_index_invalid_rows"] != "1" || page.Skipped != "filemap_index_invalid=1" {
		t.Fatalf("page overflow coverage mismatch: %+v", page)
	}
}

func TestDonghuProfilerStructuredConversionRoundTripsTypedFields(t *testing.T) {
	remaining := uint64(5) | uint64(2<<10) | uint64(1<<12) | uint64(3<<13) | uint64(17<<16)
	packed := uint64(0xf) | (remaining << 32)
	structured := protoMessage(2,
		protoVarint(1, 3),
		syntheticTracePluginFtraceEvent(1_000_000_000, 100, 100, "sched", 2417, protoPayload(
			protoBytes(1, []byte("prev")), protoVarint(2, 100), protoVarint(3, 120), protoVarint(4, 1),
			protoBytes(5, []byte("next")), protoVarint(6, 101), protoVarint(7, 118), protoVarint(8, packed),
		)),
		syntheticTracePluginFtraceEvent(1_100_000_000, 100, 100, "clk", 410, protoPayload(
			protoBytes(1, []byte("synthetic_clk_410")), protoVarint(2, 111),
		)),
		syntheticTracePluginFtraceEvent(1_150_000_000, 100, 100, "clk", 410, protoPayload(
			protoBytes(1, []byte("injected cpu_id=7")), protoVarint(2, 111),
		)),
		syntheticTracePluginFtraceEvent(1_200_000_000, 100, 100, "power", 2002, protoPayload(
			protoBytes(1, []byte("synthetic_clk_2002")), protoVarint(2, 222), protoVarint(3, 7),
		)),
		syntheticTracePluginFtraceEvent(1_300_000_000, 100, 100, "page", 1000, protoPayload(
			protoVarint(1, 77), protoVarint(2, 0x1234), protoVarint(3, 1), protoVarint(4, uint64((12<<20)|48)),
		)),
		syntheticTracePluginFtraceEvent(1_400_000_000, 100, 100, "page", 1001, protoPayload(
			protoVarint(1, 77), protoVarint(2, 0x1234), protoVarint(3, 1), protoVarint(4, uint64((12<<20)|48)),
		)),
	)
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic-profiler.htrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", structured)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert structured profiler: %v", err)
	}
	if result.EventsWritten != 5 {
		t.Fatalf("structured event count: %+v", result)
	}
	converted, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(converted)
	if strings.Contains(text, "page=0x0") || strings.Contains(text, "injected") || strings.Contains(lineContaining(text, "synthetic_clk_410"), "cpu_id=") {
		t.Fatalf("structured output fabricated unavailable dimensions:\n%s", text)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery roundtrip: %v", err)
	}
	if len(idx.Events) != 5 {
		t.Fatalf("roundtrip event count: %+v", idx.Events)
	}
	var sched, clk410, clk2002, page *tracequery.Event
	for i := range idx.Events {
		ev := &idx.Events[i]
		switch {
		case ev.Name == "sched_switch":
			sched = ev
		case ev.ClockName == "synthetic_clk_410":
			clk410 = ev
		case ev.ClockName == "synthetic_clk_2002":
			clk2002 = ev
		case ev.Name == "mm_filemap_add_to_page_cache":
			page = ev
		}
	}
	if sched == nil || sched.NextInfo != "f,10,2,1,3,17" || sched.NextInfoAffinity != "f" ||
		sched.NextInfoLoad != 10 || sched.NextInfoGroup != 2 || !sched.NextInfoRestricted || sched.NextInfoExpel != 3 || sched.NextInfoCGID != 17 {
		t.Fatalf("sched next_info roundtrip mismatch: %+v", sched)
	}
	if clk410 == nil || clk410.CPUForFieldPresent || clk410.CPUForFieldValid {
		t.Fatalf("field410 must have no CPU dimension: %+v", clk410)
	}
	if clk2002 == nil || clk2002.CPU != 3 || !clk2002.CPUForFieldPresent || !clk2002.CPUForFieldValid || clk2002.CPUForField != 7 {
		t.Fatalf("field2002 payload CPU must override header ownership only in typed field: %+v", clk2002)
	}
	if page == nil || page.FileFields == nil || page.FileFields.Dev != "12:48" || page.FileFields.Ino != "0x1234" || page.FileFields.Offset != 4096 {
		t.Fatalf("page tuple roundtrip mismatch: %+v", page)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1.0, TimeEnd: 1.5})
	if len(stats.PageCacheByInode) != 1 || stats.PageCacheByInode[0].Dev != "12:48" || stats.PageCacheByInode[0].Inode != "0x1234" ||
		stats.PageCacheByInode[0].Adds != 1 || stats.PageCacheByInode[0].Deletes != 1 ||
		stats.PageCacheByInode[0].MinOffset != 4096 || stats.PageCacheByInode[0].MaxOffset != 4096 {
		t.Fatalf("page-cache churn roundtrip mismatch: %+v", stats.PageCacheByInode)
	}
	for _, table := range []string{"clock_set_rate", "sched_switch", "mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache"} {
		coverage := coverageForTable(result.TraceCoverage, table)
		if coverage == nil || coverage.FieldSources["schema_profile"] == "" {
			t.Fatalf("schema coverage missing for %s: %+v", table, result.TraceCoverage)
		}
	}
	clk410Coverage := coverageForTable(result.TraceCoverage, "clock_set_rate")
	if clk410Coverage == nil || clk410Coverage.RowsRead != 2 || clk410Coverage.RowsEmitted != 1 ||
		!strings.Contains(clk410Coverage.Skipped, "core_field1_missing_or_invalid=1") {
		t.Fatalf("malicious field410 name must remain coverage-only: %+v", clk410Coverage)
	}
	clockProfiles := map[string]bool{}
	for _, coverage := range result.TraceCoverage {
		if coverage.Table == "clock_set_rate" {
			clockProfiles[coverage.FieldSources["schema_profile"]] = true
		}
	}
	if len(clockProfiles) != 2 {
		t.Fatalf("field410 and field2002 need distinct schema disclosures: %+v", result.TraceCoverage)
	}
}

func withoutCleanField(fields []eventField, missing string) []eventField {
	out := make([]eventField, 0, len(fields)-1)
	for _, field := range fields {
		if cleanFieldName(field.Name) != missing {
			out = append(out, field)
		}
	}
	return out
}

func coverageForTable(coverage []TraceDBCoverage, table string) *TraceDBCoverage {
	for i := range coverage {
		if coverage[i].Table == table {
			return &coverage[i]
		}
	}
	return nil
}

func lineContaining(text, token string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, token) {
			return line
		}
	}
	return ""
}

func donghuStandardSchedSwitchFormat(extraFields ...eventField) eventFormat {
	fields := []eventField{
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
		{Type: "char", Name: "prev_comm[16]", Offset: 8, Size: 16},
		{Type: "int", Name: "prev_pid", Offset: 24, Size: 4, Signed: true},
		{Type: "int", Name: "prev_prio", Offset: 28, Size: 4, Signed: true},
		{Type: "unsigned long", Name: "prev_state", Offset: 32, Size: 4},
		{Type: "char", Name: "next_comm[16]", Offset: 36, Size: 16},
		{Type: "int", Name: "next_pid", Offset: 52, Size: 4, Signed: true},
		{Type: "int", Name: "next_prio", Offset: 56, Size: 4, Signed: true},
	}
	fields = append(fields, extraFields...)
	return eventFormat{ID: 90, Name: "sched_switch", Fields: fields}
}

func donghuStandardSchedSwitchContent(length int) []byte {
	content := make([]byte, length)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	copy(content[8:24], []byte("prev"))
	binary.LittleEndian.PutUint32(content[24:28], 100)
	binary.LittleEndian.PutUint32(content[28:32], 120)
	binary.LittleEndian.PutUint32(content[32:36], 1)
	copy(content[36:52], []byte("next"))
	binary.LittleEndian.PutUint32(content[52:56], 101)
	binary.LittleEndian.PutUint32(content[56:60], 118)
	return content
}

func donghuHarmonySchedSwitchFormat(extraFields ...eventField) eventFormat {
	fields := []eventField{
		{Type: "char", Name: "pname[16]", Offset: 0, Size: 16},
		{Type: "int", Name: "prev_tid", Offset: 16, Size: 4, Signed: true},
		{Type: "int", Name: "pprio", Offset: 20, Size: 4, Signed: true},
		{Type: "unsigned int", Name: "pstate", Offset: 24, Size: 4},
		{Type: "char", Name: "nname[16]", Offset: 28, Size: 16},
		{Type: "int", Name: "next_tid", Offset: 44, Size: 4, Signed: true},
		{Type: "int", Name: "nprio", Offset: 48, Size: 4, Signed: true},
	}
	// Leave 52..59 as synthetic padding so optional ninfo/next_info fixtures
	// exercise the same 8-byte alignment as production without copying data.
	fields = append(fields, extraFields...)
	return eventFormat{ID: 91, Name: "sched_switch", Fields: fields}
}

func donghuHarmonySchedSwitchContent(length int) []byte {
	content := make([]byte, length)
	copy(content[0:16], []byte("prev"))
	binary.LittleEndian.PutUint32(content[16:20], 100)
	binary.LittleEndian.PutUint32(content[20:24], 120)
	binary.LittleEndian.PutUint32(content[24:28], 1)
	copy(content[28:44], []byte("next"))
	binary.LittleEndian.PutUint32(content[44:48], 101)
	binary.LittleEndian.PutUint32(content[48:52], 118)
	return content
}
