//go:build !race

package hitraceconv

import (
	"context"
	"errors"
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

const profilerCompactStorageResourceChildEnv = "CODRAX_PROFILER_COMPACT_STORAGE_RESOURCE_CHILD"

type profilerCompactStorageResourceMetric struct {
	rows         int
	baselineHeap uint64
	preparedHeap uint64
	heapGrowth   uint64
	sidecarBytes uint64
	peakFDs      int
}

func TestProfilerCompactStoredRowResourceBound(t *testing.T) {
	if os.Getenv(profilerCompactStorageResourceChildEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0],
			"-test.run=^TestProfilerCompactStoredRowResourceBound$", "-test.count=1", "-test.v")
		command.Env = profilerCompactStorageResourceChildEnvironment()
		output, err := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("compact storage resource subprocess timed out: %v\n%s", ctx.Err(), output)
		}
		if err != nil {
			t.Fatalf("compact storage resource subprocess failed: %v\n%s", err, output)
		}
		t.Logf("compact storage resource subprocess:\n%s", output)
		return
	}

	previousLimit := debug.SetMemoryLimit(128 << 20)
	defer debug.SetMemoryLimit(previousLimit)
	previousGC := debug.SetGCPercent(20)
	defer debug.SetGCPercent(previousGC)

	small := runProfilerCompactStorageResourceCase(t, 70_000)
	large := runProfilerCompactStorageResourceCase(t, 120_000)
	const maximumIncrementalGrowth = uint64(2 << 20)
	if large.heapGrowth > small.heapGrowth+maximumIncrementalGrowth {
		t.Fatalf("compact stored-row heap regained payload-proportional retention: small=%+v large=%+v allowed_increment=%d",
			small, large, maximumIncrementalGrowth)
	}
	t.Logf("compact stored-row resource proof: small=%+v large=%+v allowed_increment=%d",
		small, large, maximumIncrementalGrowth)
}

func profilerCompactStorageResourceChildEnvironment() []string {
	prefixes := [...]string{
		profilerCompactStorageResourceChildEnv + "=", "GOMEMLIMIT=", "GOGC=", "GOMAXPROCS=",
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
		profilerCompactStorageResourceChildEnv+"=1",
		"GOMEMLIMIT=128MiB", "GOGC=20", "GOMAXPROCS=1",
	)
}

func runProfilerCompactStorageResourceCase(t *testing.T, rows int) profilerCompactStorageResourceMetric {
	t.Helper()
	root, err := os.MkdirTemp("", "codrax-profiler-compact-storage-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove compact storage root: %v", err)
		}
	}()
	source := filepath.Join(root, "capture.htrace")
	if err := os.WriteFile(source, []byte("compact-storage-resource-source"), 0o600); err != nil {
		t.Fatal(err)
	}
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
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin compact storage text publisher")
	}
	baseline := profilerCompactStorageLiveHeap()

	for index := 0; index < rows; index++ {
		row := renderedRow{
			tsNS: uint64(index + 1), seq: index,
			pairKind: pairRenderF2FS,
		}
		if index%2 == 0 {
			row.pairLane = "compact-storage-lane-a"
			row.line = "compact-storage-f2fs-begin"
			row.pairTable = "f2fs_write_begin"
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteBegin
		} else {
			row.pairLane = "compact-storage-lane-b"
			row.line = "compact-storage-f2fs-end"
			row.pairTable = "f2fs_write_end"
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteEnd
		}
		if err := sink.add(row); err != nil {
			t.Fatalf("add compact storage row %d/%d: %v", index, rows, err)
		}
	}
	if err := sink.endProfilerTextMessage(rows); err != nil {
		t.Fatalf("end compact storage text message rows=%d: %v", rows, err)
	}
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "compact-storage-lane-a")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatalf("seal compact storage capture rows=%d: %v", rows, err)
	}
	prepared := profilerCompactStorageLiveHeap()
	runtime.KeepAlive(sink)
	heapGrowth := uint64(0)
	if prepared > baseline {
		heapGrowth = prepared - baseline
	}

	wantSidecarBytes := uint64(96) + uint64(rows)*uint64(56)
	manifest := sink.sourceOrderSidecar
	registry := sink.pairLaneRegistries[pairRenderF2FS]
	if sink.captureLifecycle != profilerCaptureSealed || !sink.prepared ||
		len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(rows) ||
		sink.stats.RowsAccepted != rows || sink.stats.SpillChunks != 1 || sink.stats.MergePasses != 0 ||
		sink.stats.PeakBufferedRows != rows || !manifest.present() || manifest.rowCount != uint64(rows) ||
		manifest.size != wantSidecarBytes || sink.stats.SourceSidecarLogicalBytes != wantSidecarBytes ||
		sink.stats.SourceSidecarPhysicalBytes != wantSidecarBytes || sink.openRunFDs != 0 ||
		sink.stats.PeakOpenRunFDs > 2 || len(registry.byKey) != 2 || len(registry.keys) != 2 ||
		len(registry.states) != 2 || registry.keys[0] != "compact-storage-lane-a" ||
		registry.keys[1] != "compact-storage-lane-b" || !registry.states[0].poisoned ||
		registry.states[1].poisoned {
		t.Fatalf("prepared compact storage resource shape drifted rows=%d sidecar=%+v runs=%+v stats=%+v open=%d",
			rows, manifest, sink.runs, sink.stats, sink.openRunFDs)
	}
	assertProfilerCompactStorageSidecarSamples(t, manifest, rows)
	info, err := os.Stat(manifest.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(wantSidecarBytes) {
		t.Fatalf("compact storage sidecar physical size rows=%d info=%v err=%v want=%d",
			rows, info, err, wantSidecarBytes)
	}
	runPath := sink.runs[0].path
	sidecarPath := manifest.path
	stats, err := sink.writeTo(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("publish compact storage resource rows=%d: %v", rows, err)
	}
	cleaned = true
	if stats.RowsAccepted != rows || stats.RowsWritten != rows/2 || stats.RowsWithheld != rows/2 ||
		stats.PeakOpenRunFDs > 2 || stats.CurrentLiveTempBytes != 0 ||
		sink.openRunFDs != 0 || sink.activeTempBytes != 0 || sink.liveTempBytes != 0 ||
		sink.sourceOrderSidecar.present() || sink.runs != nil || sink.rows != nil ||
		sink.rowIngestOrdinals != nil || sink.artifacts != nil ||
		!sink.profilerSourceProof.retired || sink.profilerSourceProof.workspace != nil ||
		sink.profilerSourceProof.scratch != nil || sink.profilerSourceProof.hasher != nil {
		t.Fatalf("compact storage cleanup state drifted rows=%d stats=%+v sink=%+v proof=%+v",
			rows, stats, sink, sink.profilerSourceProof)
	}
	for _, path := range []string{runPath, sidecarPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("compact storage cleanup retained artifact %q: %v", path, statErr)
		}
	}
	entries, err := os.ReadDir(sinkDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("compact storage cleanup retained directory entries rows=%d entries=%v err=%v",
			rows, entries, err)
	}
	return profilerCompactStorageResourceMetric{
		rows: rows, baselineHeap: baseline, preparedHeap: prepared, heapGrowth: heapGrowth,
		sidecarBytes: wantSidecarBytes, peakFDs: stats.PeakOpenRunFDs,
	}
}

func profilerCompactStorageLiveHeap() uint64 {
	runtime.GC()
	debug.FreeOSMemory()
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return memory.HeapAlloc
}
