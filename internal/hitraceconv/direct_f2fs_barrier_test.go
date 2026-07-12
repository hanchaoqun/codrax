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

func TestDirectF2FSCompleteCaptureBarrierPreventsCrossHolePairing(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
	if err != nil {
		t.Fatal(err)
	}
	start := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8))
	done := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8))
	badFixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
	directF2FSPutUint(directF2FSFixtureField(t, &badFixture, "len"), badFixture.content, uint64(^uint64(0)>>1)+1)
	badEvent := decodeEvent(badFixture.format, badFixture.content)
	badPayload, admission, reason := decodeDirectF2FSPayload(badEvent)
	if admission != bodyRejected || reason == "" || !badPayload.IdentityKnown {
		t.Fatalf("bad physical F2FS endpoint did not retain exact lane identity: admission=%d reason=%q payload=%+v", admission, reason, badPayload)
	}
	bad := directF2FSAudit(badEvent, badPayload)
	work := directPairAdmittedAudit(t, directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111))

	barrier.observe(start)
	barrier.addPublishedRow(1, start)
	barrier.observe(bad)
	barrier.observe(done)
	barrier.addPublishedRow(3, done)
	barrier.observe(work)
	barrier.addPublishedRow(4, work)
	filtered := barrier.filter([]renderedRow{
		{seq: 1, pairKind: pairRenderF2FS, line: "start"},
		{seq: 3, pairKind: pairRenderF2FS, line: "done"},
		{seq: 4, pairKind: pairRenderWorkqueue, line: "work"},
		{seq: 5, line: "inventory"},
	})
	if len(filtered) != 2 || filtered[0].seq != 4 || filtered[1].seq != 5 ||
		barrier.poisonedKinds[pairRenderF2FS] || barrier.poisonedKinds[pairRenderWorkqueue] ||
		barrier.poisonedLaneCount() != 1 || barrier.poisonedRows != 2 {
		t.Fatalf("F2FS cross-hole barrier scope drifted: filtered=%+v poisoned=%v lanes=%v rows=%d", filtered, barrier.poisonedKinds, barrier.poisonedLanes, barrier.poisonedRows)
	}
}

func TestDirectF2FSUnknownHardKeyClosesOnlyF2FSFamily(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
	if err != nil {
		t.Fatal(err)
	}
	badFixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
	directF2FSPutUint(directF2FSFixtureField(t, &badFixture, "ino"), badFixture.content, 0)
	badEvent := decodeEvent(badFixture.format, badFixture.content)
	badPayload, admission, _ := decodeDirectF2FSPayload(badEvent)
	if admission != bodyRejected || badPayload.IdentityKnown {
		t.Fatalf("bad F2FS hard key unexpectedly admitted: %+v admission=%d", badPayload, admission)
	}
	bad := directF2FSAudit(badEvent, badPayload)
	cleanWrite := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8))
	mmc := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
	barrier.observe(bad)
	barrier.observe(cleanWrite)
	barrier.addPublishedRow(2, cleanWrite)
	barrier.observe(mmc)
	barrier.addPublishedRow(3, mmc)
	filtered := barrier.filter([]renderedRow{
		{seq: 2, pairKind: pairRenderF2FS, line: "f2fs"},
		{seq: 3, pairKind: pairRenderMMC, line: "mmc"},
	})
	if len(filtered) != 1 || filtered[0].seq != 3 || !barrier.poisonedKinds[pairRenderF2FS] || barrier.poisonedKinds[pairRenderMMC] {
		t.Fatalf("unknown F2FS identity did not stay family-local: filtered=%+v kinds=%v", filtered, barrier.poisonedKinds)
	}
}

func TestDirectF2FSNonKeyDescriptorFailureQuarantinesOnlyExactLane(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
	if err != nil {
		t.Fatal(err)
	}
	start := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8))
	badFixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
	directF2FSFixtureField(t, &badFixture, "len").Type = "unsigned long long"
	badEvent := decodeEvent(badFixture.format, badFixture.content)
	badPayload, admission, reason := decodeDirectF2FSPayload(badEvent)
	if admission != bodyRejected || reason != "invalid_f2fs_descriptor_profile" || !badPayload.IdentityKnown {
		t.Fatalf("non-key descriptor failure lost independently proven F2FS lane: admission=%d reason=%q payload=%+v", admission, reason, badPayload)
	}
	bad := directF2FSAudit(badEvent, badPayload)
	cleanSibling := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8))

	barrier.observe(start)
	barrier.addPublishedRow(1, start)
	barrier.observe(bad)
	barrier.observe(cleanSibling)
	barrier.addPublishedRow(3, cleanSibling)
	filtered := barrier.filter([]renderedRow{
		{seq: 1, pairKind: pairRenderF2FS, line: "poisoned-lane-start"},
		{seq: 3, pairKind: pairRenderF2FS, line: "clean-sibling"},
	})
	if len(filtered) != 1 || filtered[0].seq != 3 || barrier.poisonedKinds[pairRenderF2FS] ||
		barrier.poisonedLaneCount() != 1 || barrier.poisonedRows != 1 {
		t.Fatalf("known-key non-key failure widened beyond its exact lane: filtered=%+v kinds=%v lanes=%v rows=%d",
			filtered, barrier.poisonedKinds, barrier.poisonedLanes, barrier.poisonedRows)
	}
}

func TestDirectF2FSUnknownOwnerClosesFamilyAndMMCReverseIsolationHolds(t *testing.T) {
	makeBarrier := func(t *testing.T) *directPairCaptureBarrier {
		t.Helper()
		barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
		if err != nil {
			t.Fatal(err)
		}
		return barrier
	}
	t.Run("unknown F2FS owner", func(t *testing.T) {
		barrier := makeBarrier(t)
		fixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
		binary.LittleEndian.PutUint32(fixture.content[4:8], ^uint32(0))
		ev := decodeEvent(fixture.format, fixture.content)
		payload, admission, reason := decodeDirectF2FSPayload(ev)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("payload fields should remain independently valid: admission=%d reason=%q", admission, reason)
		}
		bad := directF2FSAudit(ev, payload)
		if bad.HeaderOwnerKnown || bad.EndpointAdmitted {
			t.Fatalf("negative header owner gained F2FS endpoint authority: %+v", bad)
		}
		mmc := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
		barrier.observe(bad)
		barrier.observe(mmc)
		barrier.addPublishedRow(2, mmc)
		filtered := barrier.filter([]renderedRow{{seq: 2, pairKind: pairRenderMMC, line: "mmc"}})
		if len(filtered) != 1 || !barrier.poisonedKinds[pairRenderF2FS] || barrier.poisonedKinds[pairRenderMMC] {
			t.Fatalf("unknown F2FS owner damaged independent MMC: filtered=%+v kinds=%v", filtered, barrier.poisonedKinds)
		}
	})
	t.Run("bad MMC does not suppress F2FS", func(t *testing.T) {
		barrier := makeBarrier(t)
		badMMC := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
		badMMC.HeaderOwnerKnown = false
		badMMC.EndpointAdmitted = false
		f2fs := directF2FSAdmittedAudit(t, directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8))
		barrier.observe(badMMC)
		barrier.observe(f2fs)
		barrier.addPublishedRow(2, f2fs)
		filtered := barrier.filter([]renderedRow{{seq: 2, pairKind: pairRenderF2FS, line: "f2fs"}})
		if len(filtered) != 1 || !barrier.poisonedKinds[pairRenderMMC] || barrier.poisonedKinds[pairRenderF2FS] {
			t.Fatalf("bad MMC damaged independent F2FS: filtered=%+v kinds=%v", filtered, barrier.poisonedKinds)
		}
	})
}

func TestDirectF2FSFormatFamilyProvenanceIsExact(t *testing.T) {
	for _, name := range []string{"f2fs_sync_file_enter", "f2fs_sync_file_exit", "f2fs_direct_IO_enter", "f2fs_direct_IO_exit", "f2fs_write_begin", "f2fs_write_end"} {
		if got := pairCriticalFormatFamilyForName(name); got != pairCriticalFormatFamilyF2FS {
			t.Fatalf("exact F2FS format lost family provenance: name=%q mask=%d", name, got)
		}
	}
	for _, name := range []string{"f2fs_direct_io_enter", "F2FS_direct_IO_enter", "f2fs_write_end_vendor"} {
		if got := pairCriticalFormatFamilyForName(name); got != 0 {
			t.Fatalf("near F2FS format gained family provenance: name=%q mask=%d", name, got)
		}
	}
	start := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
	done := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
	start.format.ID, done.format.ID = 777, 777
	text := strings.Join(append(directF2FSSyntheticFormatBlock(start.format), directF2FSSyntheticFormatBlock(done.format)...), "\n")
	catalog, err := parseEventFormats([]byte(text))
	if err != nil || !catalog.Poisoned[777] || catalog.PoisonedFamilies[777]&pairCriticalFormatFamilyF2FS == 0 {
		t.Fatalf("conflicting F2FS descriptor ID lost family provenance: poisoned=%v families=%v err=%v", catalog.Poisoned, catalog.PoisonedFamilies, err)
	}
}

func TestDirectF2FSConversionWithholdsEndpointsAcrossBadPhysicalRow(t *testing.T) {
	start := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
	done := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
	start.format.ID, done.format.ID = 701, 702
	badDone := directF2FSCloneFixture(done)
	directF2FSPutUint(directF2FSFixtureField(t, &badDone, "len"), badDone.content, uint64(^uint64(0)>>1)+1)
	formatText := strings.Join(append(directF2FSSyntheticFormatBlock(start.format), directF2FSSyntheticFormatBlock(done.format)...), "\n")
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formatText))
	writeSegment(&capture, segmentCmdlines, []byte("100 f2fs-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 701, OffsetNS: 1_000, Content: start.content},
		{EventID: 702, OffsetNS: 2_000, Content: badDone.content},
		{EventID: 702, OffsetNS: 3_000, Content: done.content},
	}))
	dir := t.TempDir()
	input, output := filepath.Join(dir, "f2fs-source.htrace"), filepath.Join(dir, "f2fs-output.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(result.Caveats, "\n")
	if result.EventsWritten != 0 || strings.Contains(string(text), "f2fs_direct_IO_") ||
		!strings.Contains(joined, "withheld_rows=2") || !strings.Contains(joined, "poisoned_lanes=1") ||
		!strings.Contains(joined, "f2fs_direct_IO_exit_invalid_f2fs_payload_range=1") {
		t.Fatalf("direct F2FS conversion failed complete-capture closure: result=%+v\n%s", result, text)
	}
}

func TestDirectF2FSCleanConversionPairsExactlyOnceAndCountsBytesOnce(t *testing.T) {
	start := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter66, 8)
	done := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
	start.format.ID, done.format.ID = 711, 712
	formatText := strings.Join(append(directF2FSSyntheticFormatBlock(start.format), directF2FSSyntheticFormatBlock(done.format)...), "\n")
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formatText))
	writeSegment(&capture, segmentCmdlines, []byte("100 f2fs-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 711, OffsetNS: 1_000_000, Content: start.content},
		{EventID: 712, OffsetNS: 3_000_000, Content: done.content},
	}))
	dir := t.TempDir()
	input, output := filepath.Join(dir, "f2fs-clean.htrace"), filepath.Join(dir, "f2fs-clean.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil || result.EventsWritten != 2 {
		t.Fatalf("clean direct F2FS conversion failed: result=%+v err=%v", result, err)
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	found := false
	for _, item := range stats.StorageLatencyByLayer {
		if item.Layer == "f2fs" && item.Event == "f2fs_direct_io" {
			found = item.PairedCount == 1 && item.Bytes == 4096 && item.MaxLatencyMs > 1.999 && item.MaxLatencyMs < 2.001
		}
	}
	if !found {
		t.Fatalf("clean direct F2FS did not pair/count once: %+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
	}
}

func directF2FSSyntheticFormatBlock(format eventFormat) []string {
	fields := make([]string, 0, len(format.Fields))
	for _, field := range format.Fields {
		fields = append(fields, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
	}
	lines := syntheticFormatBlock(format.Name, format.ID, fields)
	lines[len(lines)-2] = "print fmt: " + format.PrintFmt
	return lines
}

func directF2FSAdmittedAudit(t *testing.T, fixture directF2FSTestFixture) directPairLineAudit {
	t.Helper()
	ev := decodeEvent(fixture.format, fixture.content)
	payload, admission, reason := decodeDirectF2FSPayload(ev)
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("seed F2FS payload rejected: admission=%d reason=%q", admission, reason)
	}
	body, ok := renderCanonicalF2FSPayload(payload)
	if !ok {
		t.Fatal("seed F2FS payload did not render")
	}
	audit := directF2FSAudit(ev, payload)
	if !audit.Governed || audit.Kind != pairRenderF2FS || !audit.HeaderOwnerKnown ||
		!audit.Verdict.KeyKnown || !audit.Verdict.PayloadAdmitted ||
		!audit.Verdict.EmitterKnown || !audit.Verdict.EmitterAdmitted ||
		!directF2FSWireParity(payload, body, audit.Verdict) {
		t.Fatalf("seed F2FS audit incomplete: %+v body=%q", audit, body)
	}
	audit.EndpointAdmitted = true
	return audit
}
