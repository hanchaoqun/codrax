package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDirectMMCCompleteCaptureBarrierPreventsCrossHolePairing(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
	if err != nil {
		t.Fatal(err)
	}
	start := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
	done := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_done", 8))
	badFixture := directMMCTestFixtureFor("mmc_request_done", 8)
	mrq := directMMCFixtureField(t, &badFixture, "mrq")
	for index := 0; index < mrq.Size; index++ {
		badFixture.content[mrq.Offset+index] = 0
	}
	badEvent := decodeEvent(badFixture.format, badFixture.content)
	badPayload, admission, reason := decodeDirectMMCPayload(badEvent, badFixture.content)
	if admission != bodyRejected || reason == "" {
		t.Fatalf("bad physical endpoint was not rejected: admission=%d reason=%q", admission, reason)
	}
	bad := directMMCAudit(badEvent, badPayload)

	work := directPairAdmittedAudit(t,
		directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111))
	barrier.observe(start)
	barrier.addPublishedRow(1, start)
	barrier.observe(bad)
	barrier.observe(done)
	barrier.addPublishedRow(3, done)
	barrier.observe(work)
	barrier.addPublishedRow(4, work)

	rows := []renderedRow{
		{seq: 1, pairKind: pairRenderMMC, line: "start"},
		{seq: 3, pairKind: pairRenderMMC, line: "done"},
		{seq: 4, pairKind: pairRenderWorkqueue, line: "work"},
		{seq: 5, line: "inventory"},
	}
	filtered := barrier.filter(rows)
	if len(filtered) != 2 || filtered[0].seq != 4 || filtered[1].seq != 5 ||
		barrier.poisonedKinds[pairRenderMMC] || barrier.poisonedKinds[pairRenderWorkqueue] ||
		len(barrier.poisonedLanes) != 1 || barrier.poisonedRows != 2 {
		t.Fatalf("MMC cross-hole barrier scope drifted: filtered=%+v poisoned=%v lanes=%v rows=%d", filtered, barrier.poisonedKinds, barrier.poisonedLanes, barrier.poisonedRows)
	}
}

func TestDirectMMCBadEmitterLaneDoesNotSuppressCleanSibling(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "source.htrace"))
	if err != nil {
		t.Fatal(err)
	}
	bad := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
	bad.EndpointAdmitted = false
	cleanStart := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_start", 8))
	cleanDone := directMMCAdmittedAudit(t, directMMCTestFixtureFor("mmc_request_done", 8))
	cleanStart.HeaderTID, cleanDone.HeaderTID = bad.HeaderTID+1, bad.HeaderTID+1

	barrier.observe(bad)
	barrier.observe(cleanStart)
	barrier.addPublishedRow(2, cleanStart)
	barrier.observe(cleanDone)
	barrier.addPublishedRow(3, cleanDone)
	filtered := barrier.filter([]renderedRow{
		{seq: 2, pairKind: pairRenderMMC, line: "clean-start"},
		{seq: 3, pairKind: pairRenderMMC, line: "clean-done"},
	})
	if len(filtered) != 2 || barrier.poisonedKinds[pairRenderMMC] || len(barrier.poisonedLanes) != 1 || barrier.poisonedRows != 0 {
		t.Fatalf("bad MMC emitter damaged clean sibling locality: filtered=%+v kinds=%v lanes=%v rows=%d", filtered, barrier.poisonedKinds, barrier.poisonedLanes, barrier.poisonedRows)
	}
}

func TestDirectMMCFormatFamilyProvenanceIsExact(t *testing.T) {
	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		if got := pairCriticalFormatFamilyForName(name); got != pairCriticalFormatFamilyMMC {
			t.Fatalf("exact MMC format lost family provenance: name=%q mask=%d", name, got)
		}
	}
	for _, name := range []string{"MMC_request_start", "mmc_request_start_vendor", "vendor_mmc_request_done"} {
		if got := pairCriticalFormatFamilyForName(name); got != 0 {
			t.Fatalf("near MMC format gained family provenance: name=%q mask=%d", name, got)
		}
	}
	start := directMMCTestFixtureFor("mmc_request_start", 8)
	done := directMMCTestFixtureFor("mmc_request_done", 8)
	start.format.ID, done.format.ID = 777, 777
	text := strings.Join(append(directMMCSyntheticFormatBlock(start.format), directMMCSyntheticFormatBlock(done.format)...), "\n")
	catalog, err := parseEventFormats([]byte(text))
	if err != nil || !catalog.Poisoned[777] || catalog.PoisonedFamilies[777]&pairCriticalFormatFamilyMMC == 0 {
		t.Fatalf("conflicting MMC descriptor ID lost family provenance: poisoned=%v families=%v err=%v", catalog.Poisoned, catalog.PoisonedFamilies, err)
	}
}

func TestDirectMMCConversionWithholdsValidEndpointsAcrossBadPhysicalRow(t *testing.T) {
	start := directMMCTestFixtureFor("mmc_request_start", 8)
	done := directMMCTestFixtureFor("mmc_request_done", 8)
	start.format.ID = 701
	done.format.ID = 702
	badDone := directMMCCloneFixture(done)
	mrq := directMMCFixtureField(t, &badDone, "mrq")
	for index := 0; index < mrq.Size; index++ {
		badDone.content[mrq.Offset+index] = 0
	}

	formatText := strings.Join(append(
		directMMCSyntheticFormatBlock(start.format),
		directMMCSyntheticFormatBlock(done.format)...,
	), "\n")
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formatText))
	writeSegment(&capture, segmentCmdlines, []byte("100 mmc-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 701, OffsetNS: 1_000, Content: start.content},
		{EventID: 702, OffsetNS: 2_000, Content: badDone.content},
		{EventID: 702, OffsetNS: 3_000, Content: done.content},
	}))
	dir := t.TempDir()
	input := filepath.Join(dir, "mmc-source.htrace")
	output := filepath.Join(dir, "mmc-output.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	joinedCaveats := strings.Join(result.Caveats, "\n")
	if result.EventsWritten != 0 || strings.Contains(string(text), "mmc_request_") ||
		!strings.Contains(joinedCaveats, "withheld_rows=2") ||
		!strings.Contains(joinedCaveats, "poisoned_lanes=1") ||
		!strings.Contains(joinedCaveats, "mmc_request_done_invalid_mmc_pointer=1") {
		t.Fatalf("direct conversion failed complete-capture closure: result=%+v\n%s", result, text)
	}
}

func TestDirectMMCCleanConversionPairsExactlyOnce(t *testing.T) {
	start := directMMCTestFixtureFor("mmc_request_start", 8)
	done := directMMCTestFixtureFor("mmc_request_done", 8)
	start.format.ID = 711
	done.format.ID = 712
	formatText := strings.Join(append(
		directMMCSyntheticFormatBlock(start.format),
		directMMCSyntheticFormatBlock(done.format)...,
	), "\n")
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formatText))
	writeSegment(&capture, segmentCmdlines, []byte("100 mmc-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 711, OffsetNS: 1_000_000, Content: start.content},
		{EventID: 712, OffsetNS: 3_000_000, Content: done.content},
	}))
	dir := t.TempDir()
	input := filepath.Join(dir, "mmc-clean.htrace")
	output := filepath.Join(dir, "mmc-clean.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil || result.EventsWritten != 2 {
		t.Fatalf("clean direct MMC conversion failed: result=%+v err=%v", result, err)
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	found := false
	for _, item := range stats.StorageLatencyByLayer {
		if item.Layer == "mmc" && item.Event == "mmc_request" {
			found = item.PairedCount == 1 && item.MaxLatencyMs > 1.999 && item.MaxLatencyMs < 2.001
		}
	}
	if !found {
		t.Fatalf("clean direct MMC did not pair exactly once: %+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
	}
}

func directMMCSyntheticFormatBlock(format eventFormat) []string {
	fields := make([]string, 0, len(format.Fields))
	for _, field := range format.Fields {
		fields = append(fields, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
	}
	return syntheticFormatBlock(format.Name, format.ID, fields)
}

func directMMCAdmittedAudit(t *testing.T, fixture directMMCTestFixture) directPairLineAudit {
	t.Helper()
	ev := decodeEvent(fixture.format, fixture.content)
	payload, admission, reason := decodeDirectMMCPayload(ev, fixture.content)
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("seed MMC payload rejected: admission=%d reason=%q", admission, reason)
	}
	body, ok := renderCanonicalMMCPayload(payload)
	if !ok {
		t.Fatal("seed MMC payload did not render")
	}
	audit := directMMCAudit(ev, payload)
	if !audit.Governed || audit.Kind != pairRenderMMC || !audit.HeaderOwnerKnown ||
		!audit.Verdict.KeyKnown || !audit.Verdict.PayloadAdmitted ||
		!audit.Verdict.EmitterKnown || !audit.Verdict.EmitterAdmitted ||
		!directMMCWireParity(payload, body, audit.Verdict) {
		t.Fatalf("seed MMC audit incomplete: %+v body=%q", audit, body)
	}
	audit.EndpointAdmitted = true
	return audit
}
