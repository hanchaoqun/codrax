package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

const (
	profilerHostileResourceChildEnv = "CODRAX_PROFILER_HOSTILE_RESOURCE_CHILD"
	profilerHostileMemoryLimit      = int64(64 << 20)
	profilerHostileFixedHeapSlack   = uint64(24 << 20)
	profilerHostileSampleEvery      = uint64(1 << 18)
)

type profilerHostileResourceFixture struct {
	Name        string
	Occurrences uint64
	DataBytes   uint64
	FrameBytes  uint64
	FileBytes   int64
}

type profilerHostileHeapContext struct {
	context.Context
	Polls       uint64
	sampleEvery uint64
	Baseline    uint64
	Peak        uint64
	Limit       uint64
	Samples     uint64
	Cancel      context.CancelFunc
	Exceeded    bool
}

func (ctx *profilerHostileHeapContext) Err() error {
	ctx.Polls++
	if ctx.sampleEvery > 0 && ctx.Polls%ctx.sampleEvery == 0 {
		ctx.sample()
	}
	if ctx.Context == nil {
		return nil
	}
	return ctx.Context.Err()
}

func (ctx *profilerHostileHeapContext) sample() {
	// This gate measures live parser-owned Go heap. Collecting before the
	// sample intentionally excludes short-lived allocation churn and makes the
	// assertion independent of GC scheduling differences between normal/race.
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	ctx.Samples++
	if stats.HeapAlloc > ctx.Peak {
		ctx.Peak = stats.HeapAlloc
	}
	if ctx.Limit > 0 && ctx.peakDelta() > ctx.Limit && !ctx.Exceeded {
		ctx.Exceeded = true
		if ctx.Cancel != nil {
			ctx.Cancel()
		}
	}
}

func profilerHostilePeakDeltaLimit(frameBytes uint64) uint64 {
	const mib = uint64(1 << 20)
	rounded := ((frameBytes + mib - 1) / mib) * mib
	return rounded + profilerHostileFixedHeapSlack
}

func (ctx *profilerHostileHeapContext) peakDelta() uint64 {
	if ctx.Peak <= ctx.Baseline {
		return 0
	}
	return ctx.Peak - ctx.Baseline
}

func profilerHostileProtoVarint(value uint64) []byte {
	var out [10]byte
	used := 0
	for value >= 0x80 {
		out[used] = byte(value) | 0x80
		value >>= 7
		used++
	}
	out[used] = byte(value)
	return append([]byte(nil), out[:used+1]...)
}

func profilerHostileLengthDelimitedPrefix(field int, length uint64) []byte {
	return append(profilerHostileProtoVarint(uint64(field<<3|2)), profilerHostileProtoVarint(length)...)
}

func profilerHostileVarintField(field int, value uint64) []byte {
	return append(profilerHostileProtoVarint(uint64(field<<3)), profilerHostileProtoVarint(value)...)
}

func writeProfilerHostileBytes(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func writeProfilerHostileRepeated(writer io.Writer, token []byte, count int) error {
	if len(token) == 0 || count < 0 {
		return fmt.Errorf("invalid hostile repeat token=%d count=%d", len(token), count)
	}
	if count == 0 {
		return nil
	}
	const blockBytes = 64 << 10
	perBlock := blockBytes / len(token)
	if perBlock == 0 {
		perBlock = 1
	}
	block := bytes.Repeat(token, perBlock)
	for count > 0 {
		items := perBlock
		if items > count {
			items = count
		}
		if err := writeProfilerHostileBytes(writer, block[:items*len(token)]); err != nil {
			return err
		}
		count -= items
	}
	return nil
}

func writeProfilerHostileFrame(t *testing.T, path, name string, occurrences, dataBytes uint64,
	writeData func(io.Writer) error,
) profilerHostileResourceFixture {
	t.Helper()
	var pluginPrefix bytes.Buffer
	pluginPrefix.Write(profilerHostileLengthDelimitedPrefix(1, uint64(len("ftrace-plugin"))))
	pluginPrefix.WriteString("ftrace-plugin")
	pluginPrefix.Write(profilerHostileVarintField(2, 0))
	pluginPrefix.Write(profilerHostileLengthDelimitedPrefix(3, dataBytes))
	frameBytes := uint64(pluginPrefix.Len()) + dataBytes
	if frameBytes > uint64(^uint32(0)) || frameBytes > maxProfilerPluginFrameBytes {
		t.Fatalf("hostile fixture frame escaped product bounds: data=%d frame=%d", dataBytes, frameBytes)
	}
	fileBytes := int64(profilerTraceHeaderSize) + 4 + int64(frameBytes)
	header := make([]byte, profilerTraceHeaderSize)
	binary.LittleEndian.PutUint64(header[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(header[8:16], uint64(fileBytes))
	binary.LittleEndian.PutUint32(header[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(header[20:24], 2)
	binary.LittleEndian.PutUint32(header[56:60], profilerDataTypeProtobuf)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	var frameLength [4]byte
	binary.LittleEndian.PutUint32(frameLength[:], uint32(frameBytes))
	for _, data := range [][]byte{header, frameLength[:], pluginPrefix.Bytes()} {
		if err := writeProfilerHostileBytes(file, data); err != nil {
			t.Fatal(err)
		}
	}
	if writeData == nil {
		t.Fatal("hostile frame writer is nil")
	}
	if err := writeData(file); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != fileBytes || !info.Mode().IsRegular() {
		t.Fatalf("hostile fixture seal drifted: size=%d want=%d mode=%v", info.Size(), fileBytes, info.Mode())
	}
	committed = true
	return profilerHostileResourceFixture{
		Name: name, Occurrences: occurrences, DataBytes: dataBytes, FrameBytes: frameBytes, FileBytes: fileBytes,
	}
}

func writeProfilerHostileNestedFixture(t *testing.T, path string) profilerHostileResourceFixture {
	t.Helper()
	const (
		perCPUOccurrences = 2 << 20
		eventOccurrences  = 2 << 20
	)
	perCPUToken := []byte{0x12, 0x00} // FtraceCpuStatsMsg.per_cpu_stats = empty message.
	eventToken := []byte{0x12, 0x00}  // FtraceCpuDetailMsg.event = empty message.
	cpuField := profilerHostileVarintField(1, 2)

	statsPayloadBytes := uint64(perCPUOccurrences * len(perCPUToken))
	detailPayloadBytes := uint64(len(cpuField) + eventOccurrences*len(eventToken))
	statsPrefix := profilerHostileLengthDelimitedPrefix(1, statsPayloadBytes)
	detailPrefix := profilerHostileLengthDelimitedPrefix(2, detailPayloadBytes)
	dataBytes := uint64(len(statsPrefix)) + statsPayloadBytes + uint64(len(detailPrefix)) + detailPayloadBytes
	return writeProfilerHostileFrame(t, path, "nested", perCPUOccurrences, dataBytes, func(writer io.Writer) error {
		if err := writeProfilerHostileBytes(writer, statsPrefix); err != nil {
			return err
		}
		if err := writeProfilerHostileRepeated(writer, perCPUToken, perCPUOccurrences); err != nil {
			return err
		}
		if err := writeProfilerHostileBytes(writer, detailPrefix); err != nil {
			return err
		}
		if err := writeProfilerHostileBytes(writer, cpuField); err != nil {
			return err
		}
		return writeProfilerHostileRepeated(writer, eventToken, eventOccurrences)
	})
}

func writeProfilerHostileTopFixture(t *testing.T, path string) profilerHostileResourceFixture {
	t.Helper()
	// Every token is a correct-wire empty embedded message. Interleaving keeps
	// the five repeated top fields on one physical-order authority path and
	// collectively exposes the historical top-level [][]byte materializer.
	cycle := []byte{
		0x0a, 0x00, // field 1: FtraceCpuStatsMsg
		0x12, 0x00, // field 2: FtraceCpuDetailMsg
		0x2a, 0x00, // field 5: SymbolDetail
		0x32, 0x00, // field 6: ClockDetail
		0x42, 0x00, // field 8: CommDictMsg
	}
	const targetBytes = 8 << 20
	occurrences := (targetBytes + len(cycle) - 1) / len(cycle)
	dataBytes := uint64(occurrences * len(cycle))
	return writeProfilerHostileFrame(t, path, "top", uint64(occurrences), dataBytes, func(writer io.Writer) error {
		return writeProfilerHostileRepeated(writer, cycle, occurrences)
	})
}

func profilerHostileResourceChildEnvironment() []string {
	prefixes := [...]string{
		profilerHostileResourceChildEnv + "=",
		"GOMEMLIMIT=",
		"GOGC=",
		"GOMAXPROCS=",
	}
	environment := make([]string, 0, len(os.Environ())+4)
	for _, item := range os.Environ() {
		skip := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(item, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			environment = append(environment, item)
		}
	}
	return append(environment,
		profilerHostileResourceChildEnv+"=1",
		"GOMEMLIMIT=64MiB",
		"GOGC=20",
		"GOMAXPROCS=2",
	)
}

func profilerHostileCaveatHasToken(caveats []string, token string) bool {
	const prefix = "ftrace-plugin structured metadata: "
	for _, caveat := range caveats {
		if !strings.HasPrefix(caveat, prefix) {
			continue
		}
		for _, item := range strings.Split(strings.TrimPrefix(caveat, prefix), "; ") {
			if item == token {
				return true
			}
		}
	}
	return false
}

func profilerHostileCoverageByTable(t *testing.T, coverage []TraceDBCoverage, table string) TraceDBCoverage {
	t.Helper()
	var found *TraceDBCoverage
	for index := range coverage {
		if coverage[index].Table != table {
			continue
		}
		if found != nil {
			t.Fatalf("hostile fixture minted duplicate coverage table %s: %+v", table, coverage)
		}
		found = &coverage[index]
	}
	if found == nil {
		t.Fatalf("hostile fixture omitted coverage table %s: %+v", table, coverage)
	}
	return *found
}

func assertProfilerHostileNestedDiagnostics(t *testing.T, extracted profilerContainerExtraction) {
	t.Helper()
	if len(extracted.TraceCoverage) != 2 || len(extracted.Caveats) != 4 {
		t.Fatalf("nested hostile returned diagnostics shape drifted: coverage=%d caveats=%d",
			len(extracted.TraceCoverage), len(extracted.Caveats))
	}
	plugin := profilerHostileCoverageByTable(t, extracted.TraceCoverage, "plugin:ftrace-plugin")
	if plugin.Family != "builtin_modern_profiler" || plugin.Role != "query_ready_export" || !plugin.Found ||
		plugin.RowsRead != 1 || plugin.RowsEmitted != 0 || plugin.Skipped != "structured_degraded=1" ||
		len(plugin.FieldSources) != 13 {
		t.Fatalf("nested hostile plugin coverage drifted: %+v", plugin)
	}
	event := profilerHostileCoverageByTable(t, extracted.TraceCoverage, "__event_envelope__")
	if event.Family != "builtin_modern_ftrace:envelope" || event.Role != "unsupported_input" || !event.Found ||
		event.RowsRead != 2<<20 || event.RowsEmitted != 0 || len(event.FieldSources) != 11 ||
		event.FieldSources["degraded_envelope_occurrences"] != "4194304" ||
		event.FieldSources["degraded_envelope_affected_frames"] != "1" ||
		event.FieldSources["degraded_envelope_oneof_missing_occurrences"] != "2097152" ||
		event.FieldSources["degraded_envelope_oneof_missing_affected_frames"] != "1" ||
		event.FieldSources["degraded_envelope_common_fields_missing_occurrences"] != "2097152" ||
		event.FieldSources["degraded_envelope_common_fields_missing_affected_frames"] != "1" {
		t.Fatalf("nested hostile event degradation drifted: %+v", event)
	}
	for _, token := range []string{
		"summary_frames=1", "stats_messages=1", "stats_start=1", "stats_end=0", "stats_cpus=1",
		"detail_messages=1", "detail_cpus=1", "structured_event_records=0", "detail_overwrite=0",
	} {
		if !profilerHostileCaveatHasToken(extracted.Caveats, token) {
			t.Fatalf("nested hostile metadata omitted exact token %q: %+v", token, extracted.Caveats)
		}
	}
}

func assertProfilerHostileTopDiagnostics(t *testing.T, fixture profilerHostileResourceFixture,
	extracted profilerContainerExtraction,
) {
	t.Helper()
	if fixture.Occurrences == 0 || len(extracted.TraceCoverage) != 1 || len(extracted.Caveats) != 4 {
		t.Fatalf("top hostile returned diagnostics shape drifted: fixture=%+v coverage=%d caveats=%d",
			fixture, len(extracted.TraceCoverage), len(extracted.Caveats))
	}
	plugin := profilerHostileCoverageByTable(t, extracted.TraceCoverage, "plugin:ftrace-plugin")
	if plugin.Family != "builtin_modern_profiler" || plugin.Role != "query_ready_export" || !plugin.Found ||
		plugin.RowsRead != 1 || plugin.RowsEmitted != 0 || plugin.Skipped != "" ||
		len(plugin.FieldSources) != 13 || plugin.FieldSources["outcome_structured_frames"] != "1" {
		t.Fatalf("top hostile plugin coverage drifted: %+v", plugin)
	}
	count := fmt.Sprintf("%d", fixture.Occurrences)
	for _, token := range []string{
		"summary_frames=1",
		"stats_messages=" + count,
		"stats_start=" + count,
		"stats_end=0",
		"detail_messages=" + count,
		"detail_cpus=1",
		"structured_event_records=0",
		"detail_overwrite=0",
		"symbols=" + count,
		"clock_detail_records=" + count,
		"clock_details=UNKNOWN(0)",
		"clock_detail_truncated_frames=1",
	} {
		if !profilerHostileCaveatHasToken(extracted.Caveats, token) {
			t.Fatalf("top hostile metadata omitted exact token %q: %+v", token, extracted.Caveats)
		}
	}
}

func runProfilerHostileResourceCase(t *testing.T, writeFixture func(*testing.T, string) profilerHostileResourceFixture) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "hostile-structured.htrace")
	fixture := writeFixture(t, input)
	if fixture.DataBytes < 8<<20 || fixture.DataBytes > 16<<20 {
		t.Fatalf("%s hostile data bytes=%d, want 8..16 MiB", fixture.Name, fixture.DataBytes)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
	if err != nil || !ok {
		t.Fatalf("read %s hostile profiler header: ok=%t err=%v", fixture.Name, ok, err)
	}
	sink, err := newTraceDBRowSink(filepath.Join(dir, "sink"), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup %s hostile sink: %v", fixture.Name, err)
		}
	}()

	runtime.GC()
	debug.FreeOSMemory()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	resourceCtx, cancelResource := context.WithCancel(context.Background())
	defer cancelResource()
	limit := profilerHostilePeakDeltaLimit(fixture.FrameBytes)
	heapCtx := &profilerHostileHeapContext{
		Context: resourceCtx, Cancel: cancelResource, sampleEvery: profilerHostileSampleEvery,
		Baseline: baseline.HeapAlloc, Peak: baseline.HeapAlloc, Limit: limit,
	}
	extracted, err := extractProfilerTraceFileWithFrameLimit(
		heapCtx, input, info.Size(), header, sink, maxProfilerPluginFrameBytes)
	heapCtx.sample()
	if err != nil {
		if heapCtx.Exceeded {
			t.Fatalf("%s hostile structured live heap crossed the fixed resource gate during parsing: baseline=%d peak=%d delta=%d limit=%d",
				fixture.Name, heapCtx.Baseline, heapCtx.Peak, heapCtx.peakDelta(), heapCtx.Limit)
		}
		t.Fatal(err)
	}
	wantUnsupported := 0
	if fixture.Name == "nested" {
		wantUnsupported = 1
	}
	if !extracted.Detected || extracted.Kind != "openharmony_profiler_trace_file" || extracted.Messages != 1 ||
		len(extracted.PluginMessages) != 1 || extracted.PluginMessages["ftrace-plugin"] != 1 ||
		extracted.StructuredFtrace != 1 || extracted.MalformedFtrace != 0 || extracted.RejectedMessages != 0 ||
		extracted.StructuredRows != 0 || extracted.UnsupportedFtrace != wantUnsupported ||
		extracted.TextPluginMessages != 0 || extracted.TextRows != 0 || extracted.StandaloneDetected ||
		extracted.SourceFailClosed || len(extracted.textMessages) != 0 || len(extracted.pairPublishers) != 0 ||
		sink.stats.RowsAccepted != 0 || sink.publishableRows() != 0 {
		t.Fatalf("%s hostile structured return shape drifted: extracted=%+v sink=%+v", fixture.Name, extracted, sink.stats)
	}
	for _, item := range extracted.TraceCoverage {
		t.Logf("%s hostile coverage: table=%s read=%d emitted=%d skipped=%q fields=%v",
			fixture.Name, item.Table, item.RowsRead, item.RowsEmitted, item.Skipped, item.FieldSources)
	}
	for _, caveat := range extracted.Caveats {
		t.Logf("%s hostile caveat: %s", fixture.Name, caveat)
	}
	switch fixture.Name {
	case "nested":
		assertProfilerHostileNestedDiagnostics(t, extracted)
	case "top":
		assertProfilerHostileTopDiagnostics(t, fixture, extracted)
	default:
		t.Fatalf("unknown hostile fixture %q", fixture.Name)
	}
	peakDelta := heapCtx.peakDelta()
	if heapCtx.Polls <= 1000 || heapCtx.Samples < 2 || peakDelta > limit {
		t.Fatalf("%s hostile structured live heap escaped fixed resource gate: data=%d frame=%d polls=%d samples=%d baseline=%d peak=%d delta=%d limit=%d",
			fixture.Name, fixture.DataBytes, fixture.FrameBytes, heapCtx.Polls, heapCtx.Samples,
			heapCtx.Baseline, heapCtx.Peak, peakDelta, limit)
	}
	t.Logf("%s hostile structured resource proof: data=%d frame=%d file=%d occurrences=%d polls=%d samples=%d baseline=%d peak=%d delta=%d limit=%d memory_limit=%d",
		fixture.Name, fixture.DataBytes, fixture.FrameBytes, fixture.FileBytes, fixture.Occurrences,
		heapCtx.Polls, heapCtx.Samples, heapCtx.Baseline, heapCtx.Peak, peakDelta, limit, profilerHostileMemoryLimit)
}

func TestProfilerStructuredHostileFrameStaysWithinMemoryBudget(t *testing.T) {
	if os.Getenv(profilerHostileResourceChildEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0],
			"-test.run=^TestProfilerStructuredHostileFrameStaysWithinMemoryBudget$", "-test.count=1", "-test.v")
		command.Env = profilerHostileResourceChildEnvironment()
		output, err := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("hostile-frame resource subprocess timed out: %v\n%s", ctx.Err(), output)
		}
		if err != nil {
			t.Fatalf("hostile-frame resource subprocess failed: %v\n%s", err, output)
		}
		t.Logf("hostile-frame resource subprocess:\n%s", output)
		return
	}

	previousLimit := debug.SetMemoryLimit(profilerHostileMemoryLimit)
	defer debug.SetMemoryLimit(previousLimit)
	previousGC := debug.SetGCPercent(20)
	defer debug.SetGCPercent(previousGC)

	t.Run("nested", func(t *testing.T) {
		runProfilerHostileResourceCase(t, writeProfilerHostileNestedFixture)
	})
	t.Run("top", func(t *testing.T) {
		runProfilerHostileResourceCase(t, writeProfilerHostileTopFixture)
	})
}
