//go:build !race

package hitraceconv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	profilerPairUniqueLaneChildEnv = "CODRAX_PROFILER_PAIR_UNIQUE_LANE_CHILD"
	profilerPairUniqueLaneRowsEnv  = "CODRAX_PROFILER_PAIR_UNIQUE_LANE_ROWS"
	profilerPairUniqueLaneMetric   = "CODRAX_PROFILER_PAIR_UNIQUE_LANE_METRIC="
)

type profilerPairUniqueLaneResourceMetric struct {
	Rows           int    `json:"rows"`
	PreparedGrowth uint64 `json:"prepared_growth"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
	SidecarBytes   uint64 `json:"sidecar_bytes"`
	PeakFDs        int    `json:"peak_fds"`
}

func TestProfilerPairUniqueLaneResourceBound(t *testing.T) {
	if os.Getenv(profilerPairUniqueLaneChildEnv) == "1" {
		rows, err := strconv.Atoi(os.Getenv(profilerPairUniqueLaneRowsEnv))
		if err != nil || rows <= 0 {
			t.Fatalf("invalid unique-lane child rows: %q", os.Getenv(profilerPairUniqueLaneRowsEnv))
		}
		previousLimit := debug.SetMemoryLimit(128 << 20)
		defer debug.SetMemoryLimit(previousLimit)
		previousGC := debug.SetGCPercent(20)
		defer debug.SetGCPercent(previousGC)
		metric := runProfilerPairUniqueLaneResourceCase(t, rows)
		encoded, err := json.Marshal(metric)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s%s\n", profilerPairUniqueLaneMetric, encoded)
		return
	}

	small := runProfilerPairUniqueLaneResourceChild(t, 70_000)
	large := runProfilerPairUniqueLaneResourceChild(t, 120_000)
	const retainedSlopeAllowance = uint64(256*50_000 + 2<<20)
	// TotalAlloc is monotonic within each isolated child. Unlike a sparsely
	// sampled HeapAlloc peak, its slope cannot move with the GC phase while it
	// still catches accidental per-lane allocation amplification. The retained
	// slope plus the exact sink-structure pin remains the duplicate-shadow proof.
	const allocatedSlopeAllowance = uint64(16*1024*50_000 + 64<<20)
	if large.PreparedGrowth > small.PreparedGrowth+retainedSlopeAllowance {
		t.Fatalf("unique-lane retained heap suggests a duplicate shadow: small=%+v large=%+v allowance=%d",
			small, large, retainedSlopeAllowance)
	}
	if large.AllocatedBytes > small.AllocatedBytes+allocatedSlopeAllowance {
		t.Fatalf("unique-lane allocation churn exceeded the linear guard: small=%+v large=%+v allowance=%d",
			small, large, allocatedSlopeAllowance)
	}
	t.Logf("unique-lane fixed-ledger resource proof: small=%+v large=%+v retained_allowance=%d allocated_allowance=%d",
		small, large, retainedSlopeAllowance, allocatedSlopeAllowance)
}

func runProfilerPairUniqueLaneResourceChild(t *testing.T, rows int) profilerPairUniqueLaneResourceMetric {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^TestProfilerPairUniqueLaneResourceBound$", "-test.count=1", "-test.v")
	command.Env = profilerPairUniqueLaneChildEnvironment(rows)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("unique-lane resource child rows=%d timed out: %v\n%s", rows, ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("unique-lane resource child rows=%d failed: %v\n%s", rows, err, output)
	}
	marker := []byte(profilerPairUniqueLaneMetric)
	index := strings.LastIndex(string(output), string(marker))
	if index < 0 {
		t.Fatalf("unique-lane resource child rows=%d omitted metric:\n%s", rows, output)
	}
	line := string(output[index+len(marker):])
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	var metric profilerPairUniqueLaneResourceMetric
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &metric); err != nil || metric.Rows != rows {
		t.Fatalf("unique-lane resource child rows=%d invalid metric=%q parsed=%+v err=%v\n%s",
			rows, line, metric, err, output)
	}
	t.Logf("unique-lane child rows=%d:\n%s", rows, output)
	return metric
}

func profilerPairUniqueLaneChildEnvironment(rows int) []string {
	prefixes := [...]string{
		profilerPairUniqueLaneChildEnv + "=", profilerPairUniqueLaneRowsEnv + "=",
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
		profilerPairUniqueLaneChildEnv+"=1",
		profilerPairUniqueLaneRowsEnv+"="+strconv.Itoa(rows),
		"GOMEMLIMIT=128MiB", "GOGC=20", "GOMAXPROCS=1",
	)
}

func runProfilerPairUniqueLaneResourceCase(t *testing.T, rows int) profilerPairUniqueLaneResourceMetric {
	t.Helper()
	root, err := os.MkdirTemp("", "codrax-profiler-unique-lane-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove unique-lane root: %v", err)
		}
	}()
	source := filepath.Join(root, "capture.htrace")
	if err := os.WriteFile(source, []byte("unique-lane-resource-source"), 0o600); err != nil {
		t.Fatal(err)
	}
	sinkDir := filepath.Join(root, "sink")
	if err := os.Mkdir(sinkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Use production defaults: the authenticated reader now borrows short
	// records directly from bufio and retains at most one lazy fragmented-row
	// scratch per reader, so its configured 24 MiB hard cap no longer creates a
	// fresh 256 KiB allocation for every row/pass.
	sink, err := newTraceDBRowSink(sinkDir, rows+1)
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
	baseline := profilerCompactStorageLiveHeap()
	allocationBaseline := profilerPairTotalAllocated()

	for index := 0; index < rows; index++ {
		lane := fmt.Sprintf("lane-%08x", index)
		row := renderedRow{
			tsNS: uint64(index + 1), seq: index + 1, pairKind: pairRenderF2FS,
			pairLane: lane, structuredPair: true,
		}
		if index%2 == 0 {
			row.line = "unique-lane-f2fs-begin"
			row.pairTable = "f2fs_write_begin"
			row.profilerEventField = 4011
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteBegin
		} else {
			row.line = "unique-lane-f2fs-end"
			row.pairTable = "f2fs_write_end"
			row.profilerEventField = 4012
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteEnd
		}
		if err := sink.add(row); err != nil {
			t.Fatalf("add unique lane %d/%d: %v", index, rows, err)
		}
		if index%2 == 0 {
			sink.poisonPairLane(pairRenderF2FS, lane)
		}
	}
	registry := &sink.pairLaneRegistries[pairRenderF2FS]
	family := sink.pairFixedLedger.families[pairRenderF2FS]
	begin := sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin]
	end := sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd]
	half := rows / 2
	if len(registry.byKey) != rows || len(registry.keys) != rows || len(registry.states) != rows ||
		cap(registry.keys) >= 2*rows || cap(registry.states) >= 2*rows ||
		family.profilerPairFixedCounts != (profilerPairFixedCounts{
			staged: rows, structured: rows, withheld: half, structuredWithheld: half,
		}) || begin != (profilerPairFixedCounts{
		staged: half, structured: half, withheld: half, structuredWithheld: half,
	}) || end != (profilerPairFixedCounts{staged: half, structured: half}) {
		t.Fatalf("unique-lane fixed shape drifted rows=%d registry=%d/%d/%d caps=%d/%d family=%+v begin=%+v end=%+v",
			rows, len(registry.byKey), len(registry.keys), len(registry.states),
			cap(registry.keys), cap(registry.states), family, begin, end)
	}
	for _, ordinal := range []int{0, rows / 2, rows - 1} {
		if sink.rows[ordinal].provenance.LaneID != uint32(ordinal+1) {
			t.Fatalf("dense lane id[%d]=%d", ordinal, sink.rows[ordinal].provenance.LaneID)
		}
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatalf("seal unique-lane capture rows=%d: %v", rows, err)
	}
	prepared := profilerCompactStorageLiveHeap()
	runtime.KeepAlive(sink)
	preparedGrowth := uint64(0)
	if prepared > baseline {
		preparedGrowth = prepared - baseline
	}
	wantSidecarBytes := uint64(96) + uint64(rows)*uint64(56)
	manifest := sink.sourceOrderSidecar
	if !manifest.present() || manifest.rowCount != uint64(rows) || manifest.size != wantSidecarBytes ||
		len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(rows) ||
		sink.stats.PeakOpenRunFDs > 2 || sink.openRunFDs != 0 {
		t.Fatalf("unique-lane prepared shape drifted rows=%d sidecar=%+v runs=%+v stats=%+v",
			rows, manifest, sink.runs, sink.stats)
	}
	assertProfilerPairUniqueLaneSidecarSamples(t, manifest, rows)
	runPath, sidecarPath := sink.runs[0].path, manifest.path
	stats, err := sink.writeTo(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("publish unique-lane capture rows=%d: %v", rows, err)
	}
	cleaned = true
	if stats.RowsAccepted != rows || stats.RowsWritten != half || stats.RowsWithheld != half ||
		stats.PeakOpenRunFDs > 2 || stats.CurrentLiveTempBytes != 0 || sink.openRunFDs != 0 ||
		sink.activeTempBytes != 0 || sink.liveTempBytes != 0 || sink.sourceOrderSidecar.present() ||
		sink.runs != nil || sink.rows != nil || sink.rowIngestOrdinals != nil || sink.artifacts != nil {
		t.Fatalf("unique-lane cleanup drifted rows=%d stats=%+v sink=%+v", rows, stats, sink)
	}
	for _, path := range []string{runPath, sidecarPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("unique-lane cleanup retained %q: %v", path, statErr)
		}
	}
	entries, err := os.ReadDir(sinkDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unique-lane cleanup retained entries rows=%d entries=%v err=%v", rows, entries, err)
	}
	allocated := profilerPairTotalAllocated()
	allocatedBytes := uint64(0)
	if allocated > allocationBaseline {
		allocatedBytes = allocated - allocationBaseline
	}
	return profilerPairUniqueLaneResourceMetric{
		Rows: rows, PreparedGrowth: preparedGrowth, AllocatedBytes: allocatedBytes,
		SidecarBytes: wantSidecarBytes, PeakFDs: stats.PeakOpenRunFDs,
	}
}

func assertProfilerPairUniqueLaneSidecarSamples(
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
		wantDisposition := profilerSourceOrderDispositionWithhold
		if ordinal%2 == 1 {
			wantEndpoint = profilerPairEndpointF2FSWriteEnd
			wantDisposition = profilerSourceOrderDispositionPublish
		}
		if record.ordinalPlusOne != ordinal+1 || record.provenance.LaneID != uint32(ordinal+1) ||
			record.provenance.TextMessageOrdinal != 0 || record.provenance.PairKind != pairRenderF2FS ||
			record.provenance.EndpointSlot != wantEndpoint ||
			record.provenance.PublisherSlot != profilerPairPublisherNone ||
			record.provenance.Flags != profilerPairRowProvenanceStructured ||
			record.disposition != wantDisposition {
			t.Fatalf("unique-lane sidecar sample ordinal=%d drifted: record=%+v endpoint=%d disposition=%d",
				ordinal, record, wantEndpoint, wantDisposition)
		}
	}
}

func profilerPairTotalAllocated() uint64 {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return memory.TotalAlloc
}
