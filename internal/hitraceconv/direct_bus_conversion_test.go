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

type conversionBusCase struct {
	name    string
	id      int
	fields  []string
	content []byte
	want    string
}

func TestBuiltinBinaryDirectBusCanonicalLinesAndTracequeryInventoryOnly(t *testing.T) {
	cases := conversionBusCases()
	formats := make([]string, 0, len(cases))
	events := make([]syntheticRawEvent, 0, len(cases))
	for index, item := range cases {
		formats = append(formats, strings.Join(syntheticFormatBlock(item.name, item.id, item.fields), "\n"))
		events = append(events, syntheticRawEvent{
			EventID:  uint16(item.id),
			OffsetNS: uint32(index * 1_000),
			Content:  item.content,
		})
	}

	result, text, output := runConversionBusCapture(t, strings.Join(formats, "\n"), events)
	if result.EventsWritten != len(cases) || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 {
		t.Fatalf("exact I2C/SMBus capture was not completely admitted: result=%+v\n%s", result, text)
	}
	for _, item := range cases {
		assertConversionBusLineCount(t, text, item.name+": "+item.want, 1)
	}

	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("build converted direct-bus index: %v", err)
	}
	if len(index.Events) != len(cases) {
		t.Fatalf("direct bus rows must remain lossless inventory: got=%d want=%d events=%+v", len(index.Events), len(cases), index.Events)
	}
	seen := make(map[string]int, len(cases))
	for _, event := range index.Events {
		if event.Type != tracequery.EventStorage {
			t.Fatalf("direct bus event escaped storage inventory: name=%s type=%s", event.Name, event.Type)
		}
		seen[event.Name]++
	}
	for _, item := range cases {
		if seen[item.name] != 1 {
			t.Fatalf("direct bus inventory identity changed: name=%s count=%d all=%v", item.name, seen[item.name], seen)
		}
	}

	query := tracequery.Query{TimeStart: index.FirstTs, TimeEnd: index.LastTs}
	stats := tracequery.ComputeWindowStats(index, query)
	if stats.StorageEventCount != len(cases) || stats.EventCounts[tracequery.EventStorage] != len(cases) {
		t.Fatalf("direct bus inventory count changed: storage=%d event_counts=%v", stats.StorageEventCount, stats.EventCounts)
	}
	if len(stats.IOLatencies) != 0 || len(stats.StorageLatencyByLayer) != 0 || stats.IOPressureSummary != nil ||
		len(stats.IOBurstEpisodes) != 0 {
		t.Fatalf("identity-free direct bus events minted generic storage duration/pressure: io=%+v storage=%+v pressure=%+v burst=%+v",
			stats.IOLatencies, stats.StorageLatencyByLayer, stats.IOPressureSummary, stats.IOBurstEpisodes)
	}
	rank := tracequery.BuildRootCauseRank(index, query)
	if len(rank.Items) != 0 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("inventory-only direct bus events entered root-cause rank: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestBuiltinBinaryDirectBusMalformedRowIsLocalAndSameIDSiblingsSurvive(t *testing.T) {
	item := conversionBusCases()[1] // i2c_write: dynamic data-loc payload.
	item.id = 650
	bad := append([]byte(nil), item.content...)
	// The logical len remains three while the physical data-loc claims four.
	// This is content-local corruption, not a descriptor/ID conflict.
	binary.LittleEndian.PutUint32(bad[20:24], uint32(4<<16|24))

	format := strings.Join(syntheticFormatBlock(item.name, item.id, item.fields), "\n")
	result, text, _ := runConversionBusCapture(t, format, []syntheticRawEvent{
		{EventID: uint16(item.id), OffsetNS: 0, Content: item.content},
		{EventID: uint16(item.id), OffsetNS: 1_000, Content: bad},
		{EventID: uint16(item.id), OffsetNS: 2_000, Content: item.content},
	})
	if result.EventsWritten != 2 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 {
		t.Fatalf("malformed direct-bus row escaped or poisoned its same-ID siblings: result=%+v\n%s", result, text)
	}
	assertConversionBusLineCount(t, text, item.name+": "+item.want, 2)
	if strings.Count(text, item.name+":") != 2 {
		t.Fatalf("malformed governed row degraded to a public header/body instead of being suppressed:\n%s", text)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 governed direct ftrace event row") ||
		!strings.Contains(caveats, "i2c_write_missing_or_invalid_i2c_buffer=1") {
		t.Fatalf("direct-bus content rejection is not auditable: %s", caveats)
	}
}

func TestBuiltinBinaryDirectBusNearAndSuffixNamesHaveNoCanonicalAuthority(t *testing.T) {
	base := conversionBusCases()
	near := []conversionBusCase{
		{name: "i2c_read_start", id: 660, fields: base[0].fields, content: base[0].content},
		{name: "i2c_write_vendor", id: 661, fields: base[1].fields, content: base[1].content},
		{name: "smbus_read_done", id: 662, fields: base[4].fields, content: base[4].content},
		{name: "smbus_result_vendor", id: 663, fields: base[7].fields, content: base[7].content},
	}
	formats := make([]string, 0, len(near))
	events := make([]syntheticRawEvent, 0, len(near))
	for index, item := range near {
		formats = append(formats, strings.Join(syntheticFormatBlock(item.name, item.id, item.fields), "\n"))
		events = append(events, syntheticRawEvent{EventID: uint16(item.id), OffsetNS: uint32(index * 1_000), Content: item.content})
	}
	result, text, _ := runConversionBusCapture(t, strings.Join(formats, "\n"), events)
	if result.UnknownEventCount != len(near) || result.MissingFormatCount != 0 {
		t.Fatalf("near-name rows did not remain compatibility-only unknown inventory: result=%+v\n%s", result, text)
	}
	for _, item := range near {
		if strings.Contains(text, item.name+":") {
			t.Fatalf("near/suffix name acquired a public standard-event body: name=%s\n%s", item.name, text)
		}
	}
	if strings.Contains(text, "i2c-2 #7") || strings.Contains(text, "i2c-3 a=050") {
		t.Fatalf("near/suffix name inherited a canonical I2C/SMBus payload:\n%s", text)
	}
}

func TestDirectBusPublicationAuthorityRemainsSingleAndBackendScoped(t *testing.T) {
	exactNames := []string{
		"i2c_read", "i2c_write", "i2c_reply", "i2c_result",
		"smbus_read", "smbus_write", "smbus_reply", "smbus_result",
	}
	for _, name := range exactNames {
		if !directBusNameGoverned(name) {
			t.Fatalf("exact direct-bus name is outside the single governed closed set: %s", name)
		}
	}
	for _, name := range []string{"i2c_read_start", "i2c_result_done", "smbus_write_vendor", "vendor_smbus_reply"} {
		if directBusNameGoverned(name) {
			t.Fatalf("near/suffix name entered the direct-bus closed set: %s", name)
		}
	}

	legacy := conversionProductionFunction(t, "official_render.go", "renderOfficialOpenHarmonyBody")
	if strings.Contains(legacy, `"i2c_`) || strings.Contains(legacy, `"smbus_`) {
		t.Fatalf("legacy official renderer regained direct-bus publication authority:\n%s", legacy)
	}
	bodyDecision := conversionProductionFunction(t, "render.go", "renderEventBodyDecisionWithPair")
	busAt := strings.Index(bodyDecision, "decodeDirectBusPayload(")
	legacyAt := strings.Index(bodyDecision, "renderLegacyEventBody(")
	if busAt < 0 || legacyAt < 0 || busAt >= legacyAt {
		t.Fatalf("typed direct-bus decoder must remain before the legacy renderer: bus=%d legacy=%d\n%s", busAt, legacyAt, bodyDecision)
	}
	lineDecision := conversionProductionFunction(t, "render.go", "renderEventLineDecisionWithPairAudit")
	gateAt := strings.Index(lineDecision, "directBusNameGoverned(format.Name)")
	lineAt := strings.Index(lineDecision, "traceDBSinglePhysicalLine(line, false)")
	if gateAt < 0 || lineAt < 0 || gateAt >= lineAt {
		t.Fatalf("exact direct-bus closed set must share the canonical single-line publication gate: gate=%d line=%d\n%s", gateAt, lineAt, lineDecision)
	}

	for field := 1300; field <= 1303; field++ {
		if descriptor, ok := profilerFtraceEventDescriptors[field]; ok {
			t.Fatalf("structured ftrace field %d unexpectedly gained direct-bus publication authority: %+v", field, descriptor)
		}
		name, body, ok, _ := renderProfilerFtraceEventBodyWithAudit(profilerFtraceEventRecord{Field: field})
		if ok || name != "" || body != "" {
			t.Fatalf("structured ftrace field %d published without an approved profile: name=%q body=%q ok=%v", field, name, body, ok)
		}
	}
	for field, descriptor := range profilerFtraceEventDescriptors {
		name := strings.ToLower(strings.TrimSpace(descriptor.Name))
		family := strings.ToLower(strings.TrimSpace(descriptor.Family))
		if strings.HasPrefix(name, "i2c_") || strings.HasPrefix(name, "smbus_") || family == "i2c" || family == "smbus" {
			t.Fatalf("structured ftrace descriptor %d gained unapproved bus authority: %+v", field, descriptor)
		}
	}
	for _, name := range exactNames {
		if class := traceDBRawFtraceClass(name); class != "" {
			t.Fatalf("SQL raw ftrace classified unapproved direct-bus authority: name=%s class=%s", name, class)
		}
		if line, ok := traceDBRenderRawFtrace(name, map[string]traceDBValue{}, map[string]bool{}); ok || line != "" {
			t.Fatalf("SQL raw ftrace published unapproved direct-bus row: name=%s line=%q ok=%v", name, line, ok)
		}
	}
}

func conversionBusCases() []conversionBusCase {
	common := func(fields ...string) []string {
		return append([]string{
			syntheticField("unsigned short", "common_type", 0, 2, false),
			syntheticField("unsigned char", "common_flags", 2, 1, false),
			syntheticField("unsigned char", "common_preempt_count", 3, 1, false),
			syntheticField("int", "common_pid", 4, 4, true),
		}, fields...)
	}
	i2cMessageFields := func(withBuffer bool) []string {
		fields := []string{
			syntheticField("int", "adapter_nr", 8, 4, true),
			syntheticField("__u16", "msg_nr", 12, 2, false),
			syntheticField("__u16", "addr", 14, 2, false),
			syntheticField("__u16", "flags", 16, 2, false),
			syntheticField("__u16", "len", 18, 2, false),
		}
		if withBuffer {
			fields = append(fields, syntheticField("__data_loc __u8[]", "buf", 20, 4, false))
		}
		return common(fields...)
	}
	smbusReadFields := common(
		syntheticField("int", "adapter_nr", 8, 4, true),
		syntheticField("__u16", "flags", 12, 2, false),
		syntheticField("__u16", "addr", 14, 2, false),
		syntheticField("__u8", "command", 16, 1, false),
		syntheticField("__u32", "protocol", 20, 4, false),
		// This is the real tracefs C declarator emitted by
		// __array(__u8, buf, I2C_SMBUS_BLOCK_MAX + 2).
		syntheticField("__u8", "buf[32 + 2]", 24, 34, false),
	)
	smbusDataFields := common(
		syntheticField("int", "adapter_nr", 8, 4, true),
		syntheticField("__u16", "addr", 12, 2, false),
		syntheticField("__u16", "flags", 14, 2, false),
		syntheticField("__u8", "command", 16, 1, false),
		syntheticField("__u8", "len", 17, 1, false),
		syntheticField("__u32", "protocol", 20, 4, false),
		syntheticField("__u8", "buf[32 + 2]", 24, 34, false),
	)
	smbusResultFields := common(
		syntheticField("int", "adapter_nr", 8, 4, true),
		syntheticField("__u16", "addr", 12, 2, false),
		syntheticField("__u16", "flags", 14, 2, false),
		syntheticField("__u8", "read_write", 16, 1, false),
		syntheticField("__u8", "command", 17, 1, false),
		syntheticField("__s16", "res", 18, 2, true),
		syntheticField("__u32", "protocol", 20, 4, false),
	)

	return []conversionBusCase{
		{name: "i2c_read", id: 600, fields: i2cMessageFields(false), content: conversionI2CMessage(600, nil), want: "i2c-2 #7 a=02a f=0001 l=3"},
		{name: "i2c_write", id: 601, fields: i2cMessageFields(true), content: conversionI2CMessage(601, []byte{0xab, 0x00, 0xcd}), want: "i2c-2 #7 a=02a f=0001 l=3 [ab-00-cd]"},
		{name: "i2c_reply", id: 602, fields: i2cMessageFields(true), content: conversionI2CMessage(602, []byte{0x01, 0xef}), want: "i2c-2 #7 a=02a f=0001 l=2 [01-ef]"},
		{name: "i2c_result", id: 603, fields: common(
			syntheticField("int", "adapter_nr", 8, 4, true),
			syntheticField("__u16", "nr_msgs", 12, 2, false),
			syntheticField("__s16", "ret", 14, 2, true),
		), content: conversionI2CResult(603, -5), want: "i2c-2 n=7 ret=-5"},
		{name: "smbus_read", id: 604, fields: smbusReadFields, content: conversionSMBusRead(604), want: "i2c-3 a=050 f=0002 c=a BYTE_DATA"},
		{name: "smbus_write", id: 605, fields: smbusDataFields, content: conversionSMBusData(605, 2, []byte{0xab}), want: "i2c-3 a=050 f=0002 c=a BYTE_DATA l=1 [ab]"},
		{name: "smbus_reply", id: 606, fields: smbusDataFields, content: conversionSMBusData(606, 3, []byte{0x00, 0xcd}), want: "i2c-3 a=050 f=0002 c=a WORD_DATA l=2 [00-cd]"},
		{name: "smbus_result", id: 607, fields: smbusResultFields, content: conversionSMBusResult(607, 1, -5), want: "i2c-3 a=050 f=0002 c=a BYTE_DATA rd res=-5"},
	}
}

func conversionBusEnvelope(eventID int, size int) []byte {
	content := make([]byte, size)
	binary.LittleEndian.PutUint16(content[0:2], uint16(eventID))
	content[2] = 0
	content[3] = 0
	binary.LittleEndian.PutUint32(content[4:8], 100)
	return content
}

func conversionI2CMessage(eventID int, data []byte) []byte {
	size := 20
	if data != nil {
		size = 24 + len(data)
	}
	content := conversionBusEnvelope(eventID, size)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 7)
	binary.LittleEndian.PutUint16(content[14:16], 0x2a)
	binary.LittleEndian.PutUint16(content[16:18], 1)
	length := 3
	if data != nil {
		length = len(data)
	}
	binary.LittleEndian.PutUint16(content[18:20], uint16(length))
	if data != nil {
		binary.LittleEndian.PutUint32(content[20:24], uint32(len(data)<<16|24))
		copy(content[24:], data)
	}
	return content
}

func conversionI2CResult(eventID int, result int16) []byte {
	content := conversionBusEnvelope(eventID, 16)
	binary.LittleEndian.PutUint32(content[8:12], 2)
	binary.LittleEndian.PutUint16(content[12:14], 7)
	binary.LittleEndian.PutUint16(content[14:16], uint16(result))
	return content
}

func conversionSMBusRead(eventID int) []byte {
	content := conversionBusEnvelope(eventID, 58)
	binary.LittleEndian.PutUint32(content[8:12], 3)
	binary.LittleEndian.PutUint16(content[12:14], 2)
	binary.LittleEndian.PutUint16(content[14:16], 0x50)
	content[16] = 0x0a
	binary.LittleEndian.PutUint32(content[20:24], 2)
	return content
}

func conversionSMBusData(eventID int, protocol uint32, data []byte) []byte {
	content := conversionBusEnvelope(eventID, 58)
	binary.LittleEndian.PutUint32(content[8:12], 3)
	binary.LittleEndian.PutUint16(content[12:14], 0x50)
	binary.LittleEndian.PutUint16(content[14:16], 2)
	content[16] = 0x0a
	content[17] = byte(len(data))
	binary.LittleEndian.PutUint32(content[20:24], protocol)
	copy(content[24:58], data)
	return content
}

func conversionSMBusResult(eventID int, readWrite uint8, result int16) []byte {
	content := conversionBusEnvelope(eventID, 24)
	binary.LittleEndian.PutUint32(content[8:12], 3)
	binary.LittleEndian.PutUint16(content[12:14], 0x50)
	binary.LittleEndian.PutUint16(content[14:16], 2)
	content[16] = readWrite
	content[17] = 0x0a
	binary.LittleEndian.PutUint16(content[18:20], uint16(result))
	binary.LittleEndian.PutUint32(content[20:24], 2)
	return content
}

func runConversionBusCapture(t *testing.T, formats string, events []syntheticRawEvent) (Result, string, string) {
	t.Helper()
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formats))
	writeSegment(&capture, segmentCmdlines, []byte("100 bus-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents(events))

	dir := t.TempDir()
	input := filepath.Join(dir, "direct-bus.sys")
	output := filepath.Join(dir, "direct-bus.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert direct-bus capture: %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return result, string(body), output
}

func assertConversionBusLineCount(t *testing.T, text string, suffix string, want int) {
	t.Helper()
	got := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasSuffix(line, suffix) {
			got++
		}
	}
	if got != want {
		t.Fatalf("canonical direct-bus line count for %q: got=%d want=%d\n%s", suffix, got, want, text)
	}
}

func conversionProductionFunction(t *testing.T, file string, name string) string {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read production source %s: %v", file, err)
	}
	source := string(body)
	start := strings.Index(source, "func "+name+"(")
	if start < 0 {
		t.Fatalf("production function %s not found in %s", name, file)
	}
	rest := source[start:]
	if next := strings.Index(rest[1:], "\nfunc "); next >= 0 {
		rest = rest[:next+1]
	}
	return rest
}
