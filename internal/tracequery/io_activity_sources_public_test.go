package tracequery

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestIOActivityPublicBundleSourceIsolation(t *testing.T) {
	dir := t.TempDir()
	trace, perf := filepath.Join(dir, "capture.systrace"), filepath.Join(dir, "sample.perftrace")
	body := ioActivityRQLine("1.01", "R") + ioActivityRQLine("1.02", "W")
	writeBundleProvenanceFixture(t, trace, body)
	writeBundleProvenanceFixture(t, perf, "app-41 (41) [000] .... 1.015: perf_sample: cpu=0 pid=41 tid=41 period=1 event=cpu-cycles symbol=App dso=libapp.so source=test\n"+body)
	manifest := filepath.Join(dir, "capture.tracebundle.json")
	writeTraceBundleV2ForTest(t, manifest, []byte(`{"version":"test","systrace":"capture.systrace","artifacts":[{"type":"perftrace","path":"sample.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}},{"type":"systrace","path":"capture.systrace"}],"perf_clock_alignments":[{"artifact_path":"sample.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":0,"slope":1,"calibrated":true}]}`))
	idx, err := BuildIndex(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TimeStart: 1, TimeEnd: 1.1}
	s := ioActivityPublicRun(t, idx, q)
	if s.GroupCount != 1 || s.Groups[0].SourcePath != canonicalTraceIndexPath(trace) || s.Groups[0].Values.EventCount != 2 {
		t.Fatalf("companion events gained native activity authority: %+v", s)
	}
	// The native replay layer can place the perf source before the primary;
	// do not mislabel this lower-level path as the public manifest contract.
	rebased, err := parseTraceArtifactPathList(context.Background(), filepath.Join(dir, "replay"), 0, 0, BuildOptions{}, []string{perf, trace})
	if err != nil {
		t.Fatal(err)
	}
	r := ioActivityPublicRun(t, rebased, q)
	if !reflect.DeepEqual(s.Groups, r.Groups) {
		t.Fatalf("physical population changed with virtual-line placement: direct=%+v rebased=%+v", s.Groups, r.Groups)
	}
	otherPath := filepath.Join(dir, "other.systrace")
	writeBundleProvenanceFixture(t, otherPath, body)
	other, err := BuildIndex(context.Background(), otherPath)
	if err != nil {
		t.Fatal(err)
	}
	if ioActivityPublicRun(t, other, q).Groups[0].SourcePath == s.Groups[0].SourcePath {
		t.Fatal("same payload in distinct physical sources shared identity")
	}
}

func TestIOActivityPublicFullWireAndSixFilesystemEndpoints(t *testing.T) {
	body := "io-40 (40) [001] .... 1.01: mmc_request_start: " + mmcExactStartBody + "\n" +
		"io-40 (40) [001] .... 1.02: mmc_request_done: " + mmcDirectDoneBody + "\n" +
		"io-40 (40) [001] .... 1.03: f2fs_direct_IO_enter: dev=8:0 ino=0x9 pos=0 len=8192 rw=read\n" +
		"io-40 (40) [001] .... 1.04: f2fs_direct_IO_exit: dev=8:0 ino=0x9 pos=0 len=8192 rw=read ret=2048\n" +
		"io-40 (40) [001] .... 1.05: f2fs_write_begin: dev=8:0 ino=0x9 pos=0 len=64\n" +
		"io-40 (40) [001] .... 1.06: f2fs_write_end: dev=8:0 ino=0x9 pos=0 len=64 copied=32\n" +
		"io-40 (40) [001] .... 1.07: f2fs_sync_file_enter: dev=8:0 ino=0x9 pino=0x1 i_mode=0x81a4 i_size=8192 i_nlink=1 i_blocks=16 i_advise=0x0\n" +
		"io-40 (40) [001] .... 1.08: f2fs_sync_file_exit: dev=8:0 ino=0x9 cp_reason=0 datasync=1 ret=0\n"
	idx := buildTraceIndex(t, "full-wire.systrace", body)
	if len(idx.Events[0].FieldText) != 300 || len(idx.Events[1].FieldText) != 300 {
		t.Fatal("fixture no longer exercises clipped previews")
	}
	q := Query{TimeStart: 1, TimeEnd: 1.1}
	s := ioActivityPublicRun(t, idx, q)
	if s.GroupCount != 8 {
		t.Fatalf("supported endpoint families missing: %+v", s)
	}
	for _, want := range []struct {
		family, phase, caliber string
		bytes                  uint64
	}{
		{"mmc_request", "start", "request_bytes", 4096}, {"mmc_request", "done", "transferred_bytes", 4096},
		{"f2fs_direct_io", "start", "request_bytes", 8192}, {"f2fs_direct_io", "done", "transferred_bytes", 2048},
		{"f2fs_write", "start", "request_bytes", 64}, {"f2fs_write", "done", "copied_bytes", 32},
	} {
		g := ioActivityPublicGroup(t, s, want.family, want.phase)
		ioActivityPublicValues(t, g.Values, 1, want.bytes)
		if g.ByteCaliber != want.caliber {
			t.Fatalf("request and actual endpoint bytes collapsed: %+v", g)
		}
	}
	// A display string is not a second measurement parser; an adversarial
	// preview edit cannot change the immutable full-wire typed handoff.
	clone := *idx
	clone.Events = append([]Event(nil), idx.Events...)
	for i := range clone.Events {
		clone.Events[i].FieldText = "bytes=0 request_bytes=999999"
	}
	if got := ioActivityPublicRun(t, &clone, q); !reflect.DeepEqual(s, got) {
		t.Fatal("activity re-parsed clipped display text")
	}
	// Pre-activity/manual verdicts retain pairing authority, but never gain
	// a size claim from a zero-default field or from the display fallback.
	for i := range clone.Events {
		rf := *clone.Events[i].ResourceFields
		if rf.mmcPairing != nil {
			admission := *rf.mmcPairing
			admission.activityParsed = false
			rf.mmcPairing = &admission
		}
		if rf.f2fsPairing != nil {
			admission := *rf.f2fsPairing
			admission.activityParsed = false
			rf.f2fsPairing = &admission
		}
		clone.Events[i].ResourceFields = &rf
	}
	cloneResult := Run(&clone, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1})
	if cloneResult.WindowStats == nil || cloneResult.WindowStats.IOActivity != nil {
		t.Fatal("pre-activity carrier acquired endpoint measurement authority")
	}
}

func TestIOActivityPublicCompactStorageCarrier(t *testing.T) {
	// Keep the new scalars within the pre-existing 64-bit allocator classes
	// (MMC 48, F2FS 64), without adding a ResourceFields pointer/heap object.
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(ResourceFields{}) != 96 || unsafe.Sizeof(mmcPairingAdmission{}) != 48 || unsafe.Sizeof(f2fsPairingAdmission{}) != 64) {
		t.Fatalf("storage activity allocated another carrier: resource=%d mmc=%d f2fs=%d", unsafe.Sizeof(ResourceFields{}), unsafe.Sizeof(mmcPairingAdmission{}), unsafe.Sizeof(f2fsPairingAdmission{}))
	}
	idx := buildTraceIndex(t, "rejected-storage.systrace", "io-40 (40) [001] .... 1.01: mmc_request_start: mmc0 tag=-1 opcode=17 blocks=1 block_size=512 blk_addr=1 unexpected=1\n"+
		"io-40 (40) [001] .... 1.02: f2fs_write_begin: dev=8:0 ino=0x9 pos=0 len=64 len=128\n")
	s := ioActivityPublicRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if s.Coverage.RejectedEndpointCount != 2 || s.Coverage.SupportedEndpointCount != 0 || s.GroupCount != 0 {
		t.Fatalf("compact rejected carriers became absent or admitted: %+v", s)
	}
}

func TestIOActivityPublicCacheEpochColdWarmAndWindowed(t *testing.T) {
	body := ioActivityRQLine("0", "R") + ioActivityRQLine("0.01", "W") + ioActivityRQLine("1", "R")
	path := filepath.Join(t.TempDir(), "cache.systrace")
	writeBundleProvenanceFixture(t, path, body)
	var key parseCacheKey
	idx, err := buildIndexWithObserver(context.Background(), path, BuildOptions{}, func(phase traceIndexBuildPhase, k parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			key = k
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if ParserVersion != "tracequery-v45" || key.version != ParserVersion {
		t.Fatalf("IO parse-time presence carrier reused old cache epoch: %+v", key)
	}
	oldKey := key
	oldKey.version = "tracequery-v44"
	indexCache.Store(oldKey, &Index{Path: path})
	indexCache.Delete(key)
	t.Cleanup(func() { indexCache.Delete(oldKey) })
	cold, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TimeStart: 0, TimeEnd: .1, TimeStartSet: true, TimeEndSet: true}
	want := ioActivityPublicRun(t, idx, q)
	if !reflect.DeepEqual(want, ioActivityPublicRun(t, cold, q)) || !reflect.DeepEqual(want, ioActivityPublicRun(t, warm, q)) {
		t.Fatal("cold/warm parse lost byte presence")
	}
	opts := BuildOptions{TimeStart: 0, TimeEnd: .1, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true}
	derived, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "cold-window.systrace")
	writeBundleProvenanceFixture(t, other, body)
	windowCold, err := BuildIndexWithOptions(context.Background(), other, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, windowed := range []*Index{derived, windowCold} {
		if !windowed.Windowed {
			t.Fatal("test did not use actual windowed parser/derive path")
		}
		s := ioActivityPublicRun(t, windowed, q)
		if s.Coverage.SupportedEndpointCount != 2 || s.Groups[0].Values.EventCount != 2 || !strings.Contains(strings.Join(s.Coverage.Reasons, ";"), "windowed_index_observed_endpoint_population_only") {
			t.Fatalf("windowed activity borrowed pairing topology requirement or omitted disclosure: %+v", s)
		}
		ioActivityPublicValues(t, s.Groups[0].Values, 2, 8192)
	}
}

func TestIOActivityPublicCancellationKeepsOnlyCompleteFace(t *testing.T) {
	idx := buildTraceIndex(t, "cancel.systrace", strings.Repeat(ioActivityRQLine("1.01", "R"), 100))
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1}
	full := ioActivityPublicRun(t, idx, q)
	want, _ := json.Marshal(full)
	sawCancel, sawComplete := false, false
	for k := 0; k < 64; k++ {
		res := Run(idx, q.WithRunContext(newRunCancelAfterN(k)))
		if res.ViewCancellation != nil {
			sawCancel = true
		}
		if res.WindowStats != nil && res.WindowStats.IOActivity != nil {
			sawComplete = true
			got, _ := json.Marshal(res.WindowStats.IOActivity)
			if string(got) != string(want) {
				t.Fatal("interrupted work published a partial activity aggregate")
			}
		}
	}
	if !sawCancel || !sawComplete {
		t.Fatalf("cancel sweep did not exercise both outcomes: %v/%v", sawCancel, sawComplete)
	}
}
