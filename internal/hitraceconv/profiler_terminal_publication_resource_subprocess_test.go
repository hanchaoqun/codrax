//go:build !race

package hitraceconv

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	profilerTerminalPublicationResourceChildEnv = "CODRAX_PROFILER_TERMINAL_PUBLICATION_RESOURCE_CHILD"
	profilerTerminalPublicationResourceRowsEnv  = "CODRAX_PROFILER_TERMINAL_PUBLICATION_RESOURCE_ROWS"
	profilerTerminalPublicationResourceMetric   = "CODRAX_PROFILER_TERMINAL_PUBLICATION_RESOURCE_METRIC="
)

type profilerTerminalPublicationResourceMeasurement struct {
	Rows           int    `json:"rows"`
	PreparedGrowth uint64 `json:"prepared_growth"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
	SidecarBytes   uint64 `json:"sidecar_bytes"`
	PeakFDs        int    `json:"peak_fds"`
}

// TestProfilerTerminalPublicationResourceBound is the C-b3 acceptance gate.
// It exercises the production TraceFile frame loop with one canonical text row
// per physical ProfilerPluginData message. The input is streamed to disk so
// the fixture itself cannot recreate the retired O(message-count) shadow.
func TestProfilerTerminalPublicationResourceBound(t *testing.T) {
	if os.Getenv(profilerTerminalPublicationResourceChildEnv) == "1" {
		rows, err := strconv.Atoi(os.Getenv(profilerTerminalPublicationResourceRowsEnv))
		if err != nil || rows <= 0 || rows%2 != 0 {
			t.Fatalf("invalid terminal-publication child rows: %q", os.Getenv(profilerTerminalPublicationResourceRowsEnv))
		}
		previousLimit := debug.SetMemoryLimit(128 << 20)
		defer debug.SetMemoryLimit(previousLimit)
		previousGC := debug.SetGCPercent(20)
		defer debug.SetGCPercent(previousGC)

		measurement := runProfilerTerminalPublicationResourceCase(t, rows)
		encoded, err := json.Marshal(measurement)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s%s\n", profilerTerminalPublicationResourceMetric, encoded)
		return
	}

	small := runProfilerTerminalPublicationResourceChild(t, 70_000)
	large := runProfilerTerminalPublicationResourceChild(t, 120_000)
	const retainedSlopeAllowance = uint64(2 << 20)
	const allocatedSlopeAllowance = uint64(16*1024*50_000 + 64<<20)
	// Q3 deterministically classifies each production row through tracequery at
	// admission. Keep that incremental churn explicit and linear instead of
	// hiding it in the pre-existing renderer/sorter allowance.
	const traceClassifierSlopeAllowance = uint64(512 * 50_000)
	if large.PreparedGrowth > small.PreparedGrowth+retainedSlopeAllowance {
		t.Fatalf("terminal publication regained message-proportional retained state: small=%+v large=%+v allowance=%d",
			small, large, retainedSlopeAllowance)
	}
	if large.AllocatedBytes > small.AllocatedBytes+allocatedSlopeAllowance+traceClassifierSlopeAllowance {
		t.Fatalf("terminal publication allocation churn exceeded the linear guard: small=%+v large=%+v allowance=%d+%d",
			small, large, allocatedSlopeAllowance, traceClassifierSlopeAllowance)
	}
	t.Logf("terminal publication resource proof: small=%+v large=%+v retained_allowance=%d allocated_allowance=%d+%d",
		small, large, retainedSlopeAllowance, allocatedSlopeAllowance, traceClassifierSlopeAllowance)
}

func runProfilerTerminalPublicationResourceChild(
	t *testing.T,
	rows int,
) profilerTerminalPublicationResourceMeasurement {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^TestProfilerTerminalPublicationResourceBound$", "-test.count=1", "-test.v")
	command.Env = profilerTerminalPublicationResourceChildEnvironment(rows)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("terminal publication resource child rows=%d timed out: %v\n%s", rows, ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("terminal publication resource child rows=%d failed: %v\n%s", rows, err, output)
	}
	marker := []byte(profilerTerminalPublicationResourceMetric)
	index := strings.LastIndex(string(output), string(marker))
	if index < 0 {
		t.Fatalf("terminal publication resource child rows=%d omitted metric:\n%s", rows, output)
	}
	line := string(output[index+len(marker):])
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	var measurement profilerTerminalPublicationResourceMeasurement
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &measurement); err != nil || measurement.Rows != rows {
		t.Fatalf("terminal publication resource child rows=%d invalid metric=%q parsed=%+v err=%v\n%s",
			rows, line, measurement, err, output)
	}
	t.Logf("terminal publication child rows=%d:\n%s", rows, output)
	return measurement
}

func profilerTerminalPublicationResourceChildEnvironment(rows int) []string {
	prefixes := [...]string{
		profilerTerminalPublicationResourceChildEnv + "=",
		profilerTerminalPublicationResourceRowsEnv + "=",
		"GOMEMLIMIT=", "GOGC=", "GOMAXPROCS=",
	}
	environment := make([]string, 0, len(os.Environ())+5)
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
		profilerTerminalPublicationResourceChildEnv+"=1",
		profilerTerminalPublicationResourceRowsEnv+"="+strconv.Itoa(rows),
		"GOMEMLIMIT=128MiB", "GOGC=20", "GOMAXPROCS=1",
	)
}

func runProfilerTerminalPublicationResourceCase(
	t *testing.T,
	rows int,
) profilerTerminalPublicationResourceMeasurement {
	t.Helper()
	root, err := os.MkdirTemp("", "codrax-profiler-terminal-publication-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove terminal publication resource root: %v", err)
		}
	}()

	source := filepath.Join(root, "capture.htrace")
	inputSize := writeProfilerTerminalPublicationResourceFixture(t, source, rows)
	sinkDir := filepath.Join(root, "sink")
	if err := os.Mkdir(sinkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(sinkDir, defaultTraceDBRowSinkThreshold)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = sink.cleanup()
		}
	}()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}

	// Fixture construction is intentionally outside both measurements. The
	// measured region starts at the production extraction boundary.
	baselineHeap := profilerCompactStorageLiveHeap()
	allocationBaseline := profilerPairTotalAllocated()
	extracted, err := extractProfilerContainerSystraceRows(
		context.Background(), source, inputSize, sink,
	)
	if err != nil {
		t.Fatalf("extract terminal publication resource rows=%d: %v", rows, err)
	}
	assertProfilerTerminalPublicationStagedShape(t, extracted, sink, rows)
	if err := sink.sealProfilerCaptureContext(context.Background()); err != nil {
		t.Fatalf("seal terminal publication resource rows=%d: %v", rows, err)
	}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		t.Fatalf("project terminal publication source state rows=%d: %v", rows, err)
	}
	terminal, err := applyProfilerTerminalPublication(&extracted, sink)
	if err != nil {
		t.Fatalf("apply terminal publication rows=%d: %v", rows, err)
	}
	assertProfilerTerminalPublicationPreparedShape(t, extracted, terminal, sink, rows)

	preparedHeap := profilerCompactStorageLiveHeap()
	runtime.KeepAlive(extracted)
	runtime.KeepAlive(terminal)
	runtime.KeepAlive(sink)
	preparedGrowth := uint64(0)
	if preparedHeap > baselineHeap {
		preparedGrowth = preparedHeap - baselineHeap
	}

	manifest := sink.sourceOrderSidecar
	runPath, sidecarPath := sink.runs[0].path, manifest.path
	stats, err := sink.writeTo(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("publish terminal publication resource rows=%d: %v", rows, err)
	}
	cleaned = true
	if stats.RowsAccepted != rows || stats.RowsWritten != rows || stats.RowsWithheld != 0 ||
		stats.PeakOpenRunFDs > 2 || stats.CurrentLiveTempBytes != 0 || sink.openRunFDs != 0 ||
		sink.activeTempBytes != 0 || sink.liveTempBytes != 0 || sink.sourceOrderSidecar.present() ||
		sink.runs != nil || sink.rows != nil || sink.rowIngestOrdinals != nil || sink.artifacts != nil ||
		!sink.profilerSourceProof.retired || sink.profilerSourceProof.workspace != nil ||
		sink.profilerSourceProof.scratch != nil || sink.profilerSourceProof.hasher != nil {
		t.Fatalf("terminal publication cleanup drifted rows=%d stats=%+v sink=%+v proof=%+v",
			rows, stats, sink, sink.profilerSourceProof)
	}
	for _, path := range []string{runPath, sidecarPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("terminal publication cleanup retained %q: %v", path, statErr)
		}
	}
	entries, err := os.ReadDir(sinkDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("terminal publication cleanup retained entries rows=%d entries=%v err=%v",
			rows, entries, err)
	}
	allocated := profilerPairTotalAllocated()
	allocatedBytes := uint64(0)
	if allocated > allocationBaseline {
		allocatedBytes = allocated - allocationBaseline
	}
	return profilerTerminalPublicationResourceMeasurement{
		Rows: rows, PreparedGrowth: preparedGrowth, AllocatedBytes: allocatedBytes,
		SidecarBytes: manifest.size, PeakFDs: stats.PeakOpenRunFDs,
	}
}

func writeProfilerTerminalPublicationResourceFixture(t *testing.T, path string, rows int) int64 {
	t.Helper()
	if rows <= 0 || uint64(rows) > uint64(math.MaxUint32)/2 {
		t.Fatalf("terminal publication fixture rows out of range: %d", rows)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	writer := bufio.NewWriterSize(file, 256<<10)
	var header [profilerTraceHeaderSize]byte
	if _, err := writer.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	fileBytes := uint64(profilerTraceHeaderSize)
	for index := 0; index < rows; index++ {
		event := "f2fs_write_begin: " + directF2FSExpectedBody(directF2FSProfileWriteBegin66)
		if index%2 == 1 {
			event = "f2fs_write_end: " + directF2FSExpectedBody(directF2FSProfileWriteEnd)
		}
		line := traceDBFormatLine("io", 100, 100, 2,
			1_000_000_000+int64(index)*1_000, 0, 0, event) + "\n"
		message := syntheticProfilerPluginData("bytrace_plugin", []byte(line))
		if len(message) == 0 || uint64(len(message)) > math.MaxUint32 {
			t.Fatalf("terminal publication frame %d has invalid size %d", index, len(message))
		}
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(message)))
		if _, err := writer.Write(length[:]); err != nil {
			t.Fatalf("write terminal publication frame length %d: %v", index, err)
		}
		if _, err := writer.Write(message); err != nil {
			t.Fatalf("write terminal publication frame body %d: %v", index, err)
		}
		fileBytes += uint64(len(length) + len(message))
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(header[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(header[8:16], fileBytes)
	binary.LittleEndian.PutUint32(header[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(header[20:24], uint32(rows))
	emptyDigest := sha256.Sum256(nil)
	copy(header[24:56], emptyDigest[:])
	binary.LittleEndian.PutUint32(header[56:60], profilerDataTypeProtobuf)
	if n, err := file.WriteAt(header[:], 0); err != nil || n != len(header) {
		t.Fatalf("write terminal publication header: bytes=%d err=%v", n, err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	info, err := os.Stat(path)
	if err != nil || uint64(info.Size()) != fileBytes {
		t.Fatalf("terminal publication fixture size drifted: info=%v err=%v want=%d", info, err, fileBytes)
	}
	return int64(fileBytes)
}

func assertProfilerTerminalPublicationStagedShape(
	t *testing.T,
	extracted profilerContainerExtraction,
	sink *traceDBRowSink,
	rows int,
) {
	t.Helper()
	if !extracted.Detected || extracted.Kind != "openharmony_profiler_trace_file" ||
		extracted.Messages != rows || extracted.TextPluginMessages != rows || extracted.TextRows != rows ||
		extracted.StructuredRows != 0 || extracted.StructuredFtrace != 0 || extracted.MalformedFtrace != 0 ||
		extracted.UnsupportedFtrace != 0 || extracted.RejectedMessages != 0 || extracted.SourceFailClosed ||
		!extracted.publicationCaveatPending || extracted.terminalPublicationApplied ||
		len(extracted.Caveats) != 2 ||
		len(extracted.PluginMessages) != 1 || extracted.PluginMessages["bytrace_plugin"] != rows ||
		sink.stats.RowsAccepted != rows {
		t.Fatalf("terminal publication staged extraction drifted rows=%d extracted=%+v stats=%+v",
			rows, extracted, sink.stats)
	}
	coverageIndex, present := extracted.profilerPublisherCoverage.coverageIndex(profilerPairPublisherBytrace)
	if !present || coverageIndex < 0 || coverageIndex >= len(extracted.TraceCoverage) ||
		len(extracted.TraceCoverage) != 1 {
		t.Fatalf("terminal publication publisher coverage drifted rows=%d indexes=%+v coverage=%+v",
			rows, extracted.profilerPublisherCoverage, extracted.TraceCoverage)
	}
	for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
		if publisher != profilerPairPublisherBytrace && extracted.profilerPublisherCoverage.Present[publisher] {
			t.Fatalf("terminal publication gained publisher %d rows=%d indexes=%+v",
				publisher, rows, extracted.profilerPublisherCoverage)
		}
	}
	for slot, present := range extracted.profilerEventCoverage.Present {
		if present {
			t.Fatalf("terminal publication text fixture gained structured event slot %d rows=%d indexes=%+v",
				slot, rows, extracted.profilerEventCoverage)
		}
	}
	coverage := extracted.TraceCoverage[coverageIndex]
	if coverage.Family != "builtin_modern_profiler" || coverage.Table != "plugin:bytrace_plugin" ||
		coverage.Role != "query_ready_export" || !coverage.Found || coverage.RowsRead != rows ||
		coverage.RowsEmitted != rows || coverage.Skipped != "" || len(coverage.FieldSources) != 22 ||
		coverage.FieldSources["observed_messages"] != strconv.Itoa(rows) ||
		coverage.FieldSources["outcome_text_rows_frames"] != strconv.Itoa(rows) ||
		coverage.FieldSources[profilerCoverageStagedRowsKey(pairRenderF2FS)] != strconv.Itoa(rows) {
		t.Fatalf("terminal publication route coverage drifted rows=%d coverage=%+v", rows, coverage)
	}
	registry := sink.pairLaneRegistries[pairRenderF2FS]
	family := sink.pairFixedLedger.families[pairRenderF2FS]
	half := rows / 2
	if len(registry.byKey) != 1 || len(registry.keys) != 1 || len(registry.states) != 1 ||
		registry.keys[0] == "" || registry.states[0].poisoned ||
		family != (profilerPairFixedFamilyLedger{profilerPairFixedCounts: profilerPairFixedCounts{staged: rows}}) ||
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin] !=
			(profilerPairFixedCounts{staged: half}) ||
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd] !=
			(profilerPairFixedCounts{staged: half}) {
		t.Fatalf("terminal publication staged pair shape drifted rows=%d registry=%+v family=%+v begin=%+v end=%+v",
			rows, registry, family,
			sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin],
			sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd])
	}
}

func assertProfilerTerminalPublicationPreparedShape(
	t *testing.T,
	extracted profilerContainerExtraction,
	terminal profilerTerminalPublicationLedger,
	sink *traceDBRowSink,
	rows int,
) {
	t.Helper()
	wantSidecarBytes := uint64(profilerSourceOrderSidecarHeaderBytes) +
		uint64(rows)*uint64(profilerSourceOrderSidecarRecordBytes)
	manifest := sink.sourceOrderSidecar
	if sink.captureLifecycle != profilerCaptureSealed || !sink.prepared ||
		!manifest.present() || manifest.rowCount != uint64(rows) || manifest.size != wantSidecarBytes ||
		len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(rows) ||
		sink.stats.RowsAccepted != rows || sink.stats.SpillChunks != 1 || sink.stats.MergePasses != 0 ||
		sink.stats.PeakBufferedRows != rows || sink.openRunFDs != 0 || sink.stats.PeakOpenRunFDs > 2 ||
		sink.activeTempBytes != sink.runs[0].size ||
		sink.liveTempBytes != sink.runs[0].size+wantSidecarBytes ||
		sink.stats.CurrentLiveTempBytes != sink.liveTempBytes ||
		sink.stats.SourceSidecarLogicalBytes != wantSidecarBytes ||
		sink.stats.SourceSidecarPhysicalBytes != wantSidecarBytes {
		t.Fatalf("terminal publication prepared storage drifted rows=%d manifest=%+v runs=%+v active=%d live=%d stats=%+v",
			rows, manifest, sink.runs, sink.activeTempBytes, sink.liveTempBytes, sink.stats)
	}
	if extracted.TextPluginMessages != rows || extracted.TextRows != rows || extracted.StructuredRows != 0 ||
		extracted.publicationCaveatPending || !extracted.terminalPublicationApplied ||
		len(extracted.Caveats) != 3 || len(extracted.TraceCoverage) != 1 ||
		len(extracted.TraceCoverage[0].FieldSources) != 22 {
		t.Fatalf("terminal publication projected extraction drifted rows=%d extracted=%+v", rows, extracted)
	}
	projectedCoverage := extracted.TraceCoverage[0]
	if projectedCoverage.Table != "plugin:bytrace_plugin" || projectedCoverage.RowsRead != rows ||
		projectedCoverage.RowsEmitted != rows || projectedCoverage.Skipped != "" ||
		projectedCoverage.FieldSources[profilerCoverageStagedRowsKey(pairRenderF2FS)] != strconv.Itoa(rows) {
		t.Fatalf("terminal publication projected coverage drifted rows=%d coverage=%+v",
			rows, projectedCoverage)
	}
	want := profilerTerminalPublicationCounts{staged: uint64(rows), published: uint64(rows)}
	half := uint64(rows / 2)
	if terminal.rows != want || terminal.textRows != want ||
		terminal.pairFamilies[pairRenderF2FS] != want ||
		terminal.publishers[profilerPairPublisherBytrace] != want ||
		terminal.publisherFamilies[profilerPairPublisherBytrace][pairRenderF2FS] != want ||
		terminal.endpoints[profilerPairEndpointF2FSWriteBegin] !=
			(profilerTerminalPublicationCounts{staged: half, published: half}) ||
		terminal.endpoints[profilerPairEndpointF2FSWriteEnd] !=
			(profilerTerminalPublicationCounts{staged: half, published: half}) ||
		terminal.textMessages != (profilerTerminalTextMessageLedger{
			staged: uint64(rows), published: uint64(rows), pairBearing: uint64(rows),
		}) {
		t.Fatalf("terminal publication ledger drifted rows=%d terminal=%+v", rows, terminal)
	}
	if terminal.structuredRows != (profilerTerminalPublicationCounts{}) ||
		terminal.sourceNeutralRows != (profilerTerminalPublicationCounts{}) {
		t.Fatalf("terminal publication gained non-text authority rows=%d terminal=%+v", rows, terminal)
	}
	for kind := pairRenderKind(0); kind < pairRenderKindCount; kind++ {
		if kind != pairRenderF2FS && terminal.pairFamilies[kind] != (profilerTerminalPublicationCounts{}) ||
			terminal.structuredPairFamilies[kind] != (profilerTerminalPublicationCounts{}) {
			t.Fatalf("terminal publication gained family %d rows=%d terminal=%+v", kind, rows, terminal)
		}
	}
	for endpoint := profilerPairEndpointSlot(0); endpoint < profilerPairEndpointSlotCount; endpoint++ {
		if endpoint != profilerPairEndpointF2FSWriteBegin && endpoint != profilerPairEndpointF2FSWriteEnd &&
			terminal.endpoints[endpoint] != (profilerTerminalPublicationCounts{}) ||
			terminal.structuredEndpoints[endpoint] != (profilerTerminalPublicationCounts{}) {
			t.Fatalf("terminal publication gained endpoint %d rows=%d terminal=%+v", endpoint, rows, terminal)
		}
	}
	for publisher := profilerPairPublisherSlot(0); publisher < profilerPairPublisherSlotCount; publisher++ {
		if publisher != profilerPairPublisherBytrace &&
			terminal.publishers[publisher] != (profilerTerminalPublicationCounts{}) {
			t.Fatalf("terminal publication gained publisher %d rows=%d terminal=%+v", publisher, rows, terminal)
		}
		for kind := pairRenderKind(0); kind < pairRenderKindCount; kind++ {
			if (publisher != profilerPairPublisherBytrace || kind != pairRenderF2FS) &&
				terminal.publisherFamilies[publisher][kind] != (profilerTerminalPublicationCounts{}) {
				t.Fatalf("terminal publication gained publisher-family %d/%d rows=%d terminal=%+v",
					publisher, kind, rows, terminal)
			}
		}
	}
	assertProfilerTerminalPublicationSidecarSamples(t, manifest, rows)
}

func assertProfilerTerminalPublicationSidecarSamples(
	t *testing.T,
	manifest profilerSourceOrderSidecarManifest,
	rows int,
) {
	t.Helper()
	file, err := os.Open(manifest.path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, ordinal := range []uint64{0, uint64(rows / 2), uint64(rows - 1)} {
		offset, err := profilerSourceOrderSidecarRecordOffset(ordinal, uint64(rows))
		if err != nil {
			t.Fatal(err)
		}
		var wire [profilerSourceOrderSidecarRecordBytes]byte
		if _, err := file.ReadAt(wire[:], offset); err != nil {
			t.Fatal(err)
		}
		record, err := decodeProfilerSourceOrderSidecarRecord(wire[:])
		if err != nil {
			t.Fatal(err)
		}
		wantEndpoint := profilerPairEndpointF2FSWriteBegin
		if ordinal%2 == 1 {
			wantEndpoint = profilerPairEndpointF2FSWriteEnd
		}
		if record.ordinalPlusOne != ordinal+1 || record.provenance.LaneID != 1 ||
			record.provenance.TextMessageOrdinal != uint32(ordinal+1) ||
			record.provenance.PairKind != pairRenderF2FS || record.provenance.EndpointSlot != wantEndpoint ||
			record.provenance.PublisherSlot != profilerPairPublisherBytrace ||
			record.provenance.Flags != profilerPairRowProvenanceText ||
			record.disposition != profilerSourceOrderDispositionPublish {
			t.Fatalf("terminal publication sidecar sample ordinal=%d drifted: record=%+v endpoint=%d",
				ordinal, record, wantEndpoint)
		}
	}
}
