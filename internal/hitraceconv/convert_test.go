package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDefaultOutputPathAppendsSystraceSuffix(t *testing.T) {
	if got, want := DefaultOutputPath("foo.htrace.bin"), "foo.htrace.bin.systrace"; got != want {
		t.Fatalf("default output: got %q, want %q", got, want)
	}
}

func TestConvertFileWritesTextSystraceAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.htrace.bin")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.OutputPath != input+".systrace" {
		t.Fatalf("output path: got %q", result.OutputPath)
	}
	if result.EventsWritten != 1 {
		t.Fatalf("events written: got %d", result.EventsWritten)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=000",
		"2942.124416",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted trace missing %q:\n%s", want, text)
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery parse converted output: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].WakeePID != 36379 {
		t.Fatalf("converted output did not round-trip through tracequery: %+v", idx.Events)
	}

	if _, err := ConvertFile(context.Background(), Options{InputPath: input}); err == nil ||
		!strings.Contains(err.Error(), "output file already exists") {
		t.Fatalf("expected existing output refusal, got %v", err)
	}
}

func TestConvertFileSkipsMissingFormatRows(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "missing-format.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageForEventID(99))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.EventsWritten != 0 || result.MissingFormatCount != 1 || result.UnknownEventCount != 0 {
		t.Fatalf("missing/unknown counts: %+v", result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "unknown_event") || strings.Contains(text, "payload_hex") || strings.Contains(text, "raw_event=unparsed") {
		t.Fatalf("missing-format rows must not be written into official-compatible systrace output:\n%s", text)
	}
	if len(result.Caveats) == 0 || !strings.Contains(result.Caveats[0], "skipped") {
		t.Fatalf("missing-format skip should be surfaced as caveat: %+v", result.Caveats)
	}
}

func TestConvertFileWritesHeaderOnlyRowsWithoutOfficialRenderer(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "unsupported-format.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticUnsupportedEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageForEventID(20))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.EventsWritten != 1 || result.MissingFormatCount != 0 || result.UnknownEventCount != 1 {
		t.Fatalf("unsupported row counts: %+v", result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "vendor_numeric") || strings.Contains(text, "raw_event=unparsed") || strings.Contains(text, "foo=") {
		t.Fatalf("unsupported known-format rows must be header-only, not generically rendered:\n%s", text)
	}
	if !strings.Contains(text, "2942.124416:") {
		t.Fatalf("unsupported known-format row should preserve official-style header and timestamp:\n%s", text)
	}
	if len(result.Caveats) == 0 || !strings.Contains(result.Caveats[0], "header-only") {
		t.Fatalf("unsupported renderer skip should be surfaced as caveat: %+v", result.Caveats)
	}
}

func TestRenderHarmonySchedSwitchKeepsNextInfoAndCGroup(t *testing.T) {
	format := eventFormat{
		ID:   10,
		Name: "sched_switch",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pname[16]", Offset: 8, Size: 16},
			{Name: "prev_tid", Offset: 24, Size: 4, Signed: true},
			{Name: "pprio", Offset: 28, Size: 4, Signed: true},
			{Name: "pstate", Offset: 32, Size: 4},
			{Name: "nname[16]", Offset: 36, Size: 16},
			{Name: "next_tid", Offset: 52, Size: 4, Signed: true},
			{Name: "nprio", Offset: 56, Size: 4, Signed: true},
			{Name: "ninfo[8]", Offset: 60, Size: 8},
			{Name: "cg[16]", Offset: 68, Size: 16},
		},
	}
	content := make([]byte, 84)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:24], []byte("app"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(100))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	binary.LittleEndian.PutUint32(content[32:36], uint32(1))
	copy(content[36:52], []byte("worker"))
	binary.LittleEndian.PutUint32(content[52:56], uint32(200))
	binary.LittleEndian.PutUint32(content[56:60], uint32(80))
	binary.LittleEndian.PutUint32(content[60:64], uint32(0x0000000f))
	remaining := uint32(5) | uint32(2<<10) | uint32(1<<12) | uint32(3<<13) | uint32(17<<16)
	binary.LittleEndian.PutUint32(content[64:68], remaining)
	copy(content[68:84], []byte("top-app"))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "app"}, tgids: map[int]int{100: 100}}, 1_234_567_000, 2, format, content)
	if !known {
		t.Fatalf("sched_switch should be known: %s", line)
	}
	for _, want := range []string{"next_info=f,10,2,1,3", "cg=top-app"} {
		if !strings.Contains(line, want) {
			t.Fatalf("rendered sched_switch missing %q:\n%s", want, line)
		}
	}
}

func TestRenderHarmonySchedSwitchNinfoIncludesCGIDWhenNoCGroup(t *testing.T) {
	format := eventFormat{
		ID:   11,
		Name: "sched_switch",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pname[16]", Offset: 8, Size: 16},
			{Name: "prev_tid", Offset: 24, Size: 4, Signed: true},
			{Name: "pprio", Offset: 28, Size: 4, Signed: true},
			{Name: "pstate", Offset: 32, Size: 4},
			{Name: "nname[16]", Offset: 36, Size: 16},
			{Name: "next_tid", Offset: 52, Size: 4, Signed: true},
			{Name: "nprio", Offset: 56, Size: 4, Signed: true},
			{Name: "ninfo[8]", Offset: 60, Size: 8},
		},
	}
	content := make([]byte, 68)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:24], []byte("app"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(100))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	copy(content[36:52], []byte("worker"))
	binary.LittleEndian.PutUint32(content[52:56], uint32(200))
	binary.LittleEndian.PutUint32(content[56:60], uint32(80))
	binary.LittleEndian.PutUint32(content[60:64], uint32(0x0000000f))
	remaining := uint32(5) | uint32(2<<10) | uint32(1<<12) | uint32(3<<13) | uint32(17<<16)
	binary.LittleEndian.PutUint32(content[64:68], remaining)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "app"}, tgids: map[int]int{100: 100}}, 1_234_567_000, 2, format, content)
	if !known || !strings.Contains(line, "next_info=f,10,2,1,3,17") {
		t.Fatalf("ninfo with cgid not rendered: known=%v line=%s", known, line)
	}
}

func TestRenderMMFilemapPageCacheUsesNumericFields(t *testing.T) {
	format := eventFormat{
		ID:   30,
		Name: "mm_filemap_add_to_page_cache",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "s_dev", Offset: 8, Size: 8},
			{Name: "i_ino", Offset: 16, Size: 8},
			{Name: "index", Offset: 24, Size: 8},
			{Name: "pfn", Offset: 32, Size: 8},
			{Name: "pg", Offset: 40, Size: 8},
		},
	}
	content := make([]byte, 48)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint64(content[8:16], uint64((12<<20)|48))
	binary.LittleEndian.PutUint64(content[16:24], uint64(0x60ffe))
	binary.LittleEndian.PutUint64(content[24:32], uint64(42))
	binary.LittleEndian.PutUint64(content[32:40], uint64(3062260))
	binary.LittleEndian.PutUint64(content[40:48], uint64(0))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	for _, want := range []string{"dev 12:48", "ino 0x60ffe", "page=0x0", "pfn=3062260", "ofs=172032"} {
		if !known || !strings.Contains(line, want) {
			t.Fatalf("mm_filemap page cache missing %q: known=%v line=%s", want, known, line)
		}
	}
}

func TestGenericIntegerFieldsDoNotBecomePrintableStrings(t *testing.T) {
	format := eventFormat{
		ID:   31,
		Name: "vendor_numeric",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "pfn", Offset: 8, Size: 4},
		},
	}
	content := make([]byte, 12)
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	copy(content[8:12], []byte("ABCD"))

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if known || strings.Contains(line, "pfn=ABCD") || !strings.Contains(line, "pfn=1145258561") {
		t.Fatalf("generic integer field rendered unsafely: known=%v line=%s", known, line)
	}
}

func TestRenderDataLocStrings(t *testing.T) {
	format := eventFormat{
		ID:   20,
		Name: "tracing_mark_write",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "buf", Offset: 8, Size: 4},
		},
	}
	payload := []byte("B|100|RenderFrame\x00")
	content := make([]byte, 12+len(payload))
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload)<<16)|12))
	copy(content[12:], payload)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "render"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if !known || !strings.Contains(line, "tracing_mark_write: B|100|RenderFrame") {
		t.Fatalf("data_loc trace payload not rendered: known=%v line=%s", known, line)
	}
}

func TestGenericDataLocStringIsPreserved(t *testing.T) {
	format := eventFormat{
		ID:   21,
		Name: "vendor_dynamic",
		Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "message", Offset: 8, Size: 4},
		},
	}
	payload := []byte("alpha=1 beta=needle\x00")
	content := make([]byte, 12+len(payload))
	binary.LittleEndian.PutUint32(content[4:8], uint32(100))
	binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload)<<16)|12))
	copy(content[12:], payload)

	line, known := renderEventLine(renderContext{cmdlines: map[int]string{100: "worker"}, tgids: map[int]int{100: 100}}, 2_000_000_000, 1, format, content)
	if known || !strings.Contains(line, "vendor_dynamic: message=alpha=1 beta=needle") {
		t.Fatalf("generic data_loc payload not preserved: known=%v line=%s", known, line)
	}
}

func TestOpenHarmonyPrintFmtCoverageManifest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "openharmony_print_fmt_coverage.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 87 {
		t.Fatalf("coverage manifest should contain header + 86 OpenHarmony PRINT_FMT rows, got %d", len(lines))
	}
	seen := map[string]string{}
	for _, line := range lines[1:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("bad manifest row %q", line)
		}
		if parts[1] != "strong" {
			t.Fatalf("converter support for %s should be strong after official-format parity work, got %q", parts[0], parts[1])
		}
		seen[parts[0]] = parts[2]
		if parts[1] == "" || parts[2] == "" {
			t.Fatalf("coverage row must declare converter and trace_query support: %q", line)
		}
	}
	for name, lane := range map[string]string{
		"PRINT_FMT_SCHED_SWITCH_HM_NINFO_CG": "sched_switch",
		"PRINT_FMT_CPU_FREQUENCY_LIMITS":     "cpu_frequency_limits",
		"PRINT_FMT_UFSHCD_COMMAND":           "storage",
		"PRINT_FMT_EROFS_LOOKUP_START":       "filesystem",
		"PRINT_FMT_TRACING_MARK_WRITE":       "trace_mark",
	} {
		if seen[name] != lane {
			t.Fatalf("coverage manifest lane for %s: got %q want %q", name, seen[name], lane)
		}
	}
}

func TestOfficialSystraceLineFormatUsesCommonFlagsAndIdleName(t *testing.T) {
	format := eventFormat{
		ID:   50,
		Name: "irq_handler_exit",
		Fields: []eventField{
			{Name: "common_type", Offset: 0, Size: 2},
			{Name: "common_flags", Offset: 2, Size: 1},
			{Name: "common_preempt_count", Offset: 3, Size: 1},
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "irq", Offset: 8, Size: 4, Signed: true},
			{Name: "ret", Offset: 12, Size: 4, Signed: true},
		},
	}
	content := make([]byte, 16)
	content[2] = 0x0d // irqs-off + need-resched + hardirq
	content[3] = 2
	binary.LittleEndian.PutUint32(content[8:12], 7)
	binary.LittleEndian.PutUint32(content[12:16], 1)

	line, known := renderEventLine(renderContext{}, 1_000_001_000, 3, format, content)
	for _, want := range []string{"<idle>-0", "(-----)", "[003] dnh2", "1.000001: irq_handler_exit: irq=7 ret=handled"} {
		if !known || !strings.Contains(line, want) {
			t.Fatalf("official systrace line missing %q: known=%v line=%s", want, known, line)
		}
	}
}

func TestOfficialSubsystemRenderersMatchOpenHarmonyShapes(t *testing.T) {
	t.Run("cpu_frequency_limits", func(t *testing.T) {
		format := eventFormat{ID: 60, Name: "cpu_frequency_limits", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "min_freq", Offset: 8, Size: 4},
			{Name: "max_freq", Offset: 12, Size: 4},
			{Name: "cpu_id", Offset: 16, Size: 4},
		}}
		content := make([]byte, 20)
		binary.LittleEndian.PutUint32(content[8:12], 1000000)
		binary.LittleEndian.PutUint32(content[12:16], 2200000)
		binary.LittleEndian.PutUint32(content[16:20], 11)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "min=1000000 max=2200000 cpu_id=11" {
			t.Fatalf("cpu limits: known=%v body=%q", known, body)
		}
	})

	t.Run("i2c_write_dynamic_array", func(t *testing.T) {
		format := eventFormat{ID: 61, Name: "i2c_write", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Name: "adapter_nr", Offset: 8, Size: 4, Signed: true},
			{Name: "msg_nr", Offset: 12, Size: 4},
			{Name: "addr", Offset: 16, Size: 4},
			{Name: "flags", Offset: 20, Size: 4},
			{Name: "len", Offset: 24, Size: 4},
			{Type: "__data_loc u8[]", Name: "__data_loc_buf", Offset: 28, Size: 4},
		}}
		content := make([]byte, 34)
		binary.LittleEndian.PutUint32(content[8:12], 2)
		binary.LittleEndian.PutUint32(content[12:16], 3)
		binary.LittleEndian.PutUint32(content[16:20], 0x5a)
		binary.LittleEndian.PutUint32(content[20:24], 1)
		binary.LittleEndian.PutUint32(content[24:28], 2)
		binary.LittleEndian.PutUint32(content[28:32], uint32((2<<16)|32))
		copy(content[32:34], []byte{0xab, 0xcd})
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		if !known || body != "i2c-2 #3 a=05a f=0001 l=2 ab-cd]" {
			t.Fatalf("i2c write: known=%v body=%q", known, body)
		}
	})

	t.Run("ufshcd_command", func(t *testing.T) {
		format := eventFormat{ID: 62, Name: "ufshcd_command", Fields: []eventField{
			{Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "__data_loc char[]", Name: "__data_loc_str", Offset: 8, Size: 4},
			{Type: "__data_loc char[]", Name: "__data_loc_dev_name", Offset: 12, Size: 4},
			{Name: "tag", Offset: 16, Size: 4},
			{Name: "doorbell", Offset: 20, Size: 4},
			{Name: "transfer_len", Offset: 24, Size: 4, Signed: true},
			{Name: "intr", Offset: 28, Size: 4},
			{Name: "lba", Offset: 32, Size: 8},
			{Name: "opcode", Offset: 40, Size: 4},
			{Name: "group_id", Offset: 44, Size: 4},
		}}
		payload1 := []byte("send\x00")
		payload2 := []byte("ufs0\x00")
		content := make([]byte, 48+len(payload1)+len(payload2))
		binary.LittleEndian.PutUint32(content[8:12], uint32((len(payload1)<<16)|48))
		binary.LittleEndian.PutUint32(content[12:16], uint32((len(payload2)<<16)|(48+len(payload1))))
		binary.LittleEndian.PutUint32(content[16:20], 4)
		binary.LittleEndian.PutUint32(content[20:24], 0xff)
		binary.LittleEndian.PutUint32(content[24:28], 4096)
		binary.LittleEndian.PutUint32(content[28:32], 1)
		binary.LittleEndian.PutUint64(content[32:40], 123)
		binary.LittleEndian.PutUint32(content[40:44], 0x28)
		binary.LittleEndian.PutUint32(content[44:48], 7)
		copy(content[48:], payload1)
		copy(content[48+len(payload1):], payload2)
		body, known := renderEventBody(decodeEvent(format, content), content, 0)
		want := "send: ufs0: tag: 4, DB: 0xff, size: 4096, IS: 1, LBA: 123, opcode: 0x28 (READ_10), group_id: 0x7"
		if !known || body != want {
			t.Fatalf("ufshcd command: known=%v body=%q", known, body)
		}
	})
}

func syntheticBinaryHitrace(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPage())
	return b.Bytes()
}

func syntheticEventFormat() string {
	return strings.Join([]string{
		"name: sched_wakeup",
		"ID: 10",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;",
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;",
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;",
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;",
		`print fmt: "comm=%s pid=%d prio=%d target_cpu=%03d"`,
		"",
	}, "\n")
}

func syntheticUnsupportedEventFormat() string {
	return strings.Join([]string{
		"name: vendor_numeric",
		"ID: 20",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:int foo;\toffset:8;\tsize:4;\tsigned:1;",
		`print fmt: "foo=%d"`,
		"",
	}, "\n")
}

func syntheticRawPage() []byte {
	return syntheticRawPageForEventID(10)
}

func syntheticRawPageForEventID(eventID uint16) []byte {
	page := make([]byte, tracePageSize)
	binary.LittleEndian.PutUint64(page[0:8], 2942124416000) // ns, renders 2942.124416
	binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
	page[16] = 4

	content := make([]byte, 36)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	content[2] = 0
	content[3] = 0
	binary.LittleEndian.PutUint32(content[4:8], uint32(36379))
	copy(content[8:24], []byte("com.tencent.mm"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(36379))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	binary.LittleEndian.PutUint32(content[32:36], uint32(0))

	off := pageHeaderSize
	binary.LittleEndian.PutUint32(page[off:off+4], 0)
	binary.LittleEndian.PutUint16(page[off+4:off+6], uint16(len(content)))
	copy(page[off+eventHeaderSize:], content)
	return page
}

func writeFileHeader(b *bytes.Buffer, cpuNum int) {
	buf := make([]byte, fileHeaderSize)
	binary.LittleEndian.PutUint16(buf[0:2], 0xace)
	buf[2] = 1
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(cpuNum<<1))
	b.Write(buf)
}

func writeSegment(b *bytes.Buffer, typ int, payload []byte) {
	var hdr [segmentHdrSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(typ))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	b.Write(hdr[:])
	b.Write(payload)
}
