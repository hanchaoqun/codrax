package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDirectPairBarrierExactLanePreventsCrossHoleAndPreservesSiblings(t *testing.T) {
	wqStartA := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111)
	wqBadA := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0)
	wqEndA := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xaaa, 0x222)
	wqStartB := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xbbb, 0x333)
	wqEndB := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xbbb, 0x444)
	dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
	dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 7, 9, true)

	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{
			{id: 201, format: wqStartA.format}, {id: 202, format: wqEndA.format},
			{id: 203, format: dmaStart.format}, {id: 204, format: dmaEnd.format},
		},
		[]syntheticRawEvent{
			{EventID: 201, OffsetNS: 1_000, Content: wqStartA.content},
			{EventID: 201, OffsetNS: 2_000, Content: wqBadA.content},
			{EventID: 202, OffsetNS: 3_000, Content: wqEndA.content},
			{EventID: 201, OffsetNS: 4_000, Content: wqStartB.content},
			{EventID: 202, OffsetNS: 5_000, Content: wqEndB.content},
			{EventID: 203, OffsetNS: 6_000, Content: dmaStart.content},
			{EventID: 204, OffsetNS: 7_000, Content: dmaEnd.content},
		},
	)
	if strings.Contains(body, "work struct 0xaaa") || strings.Count(body, "work struct 0xbbb") != 2 ||
		strings.Count(body, "dma_fence_wait_") != 2 {
		t.Fatalf("exact-lane barrier lost locality or bridged the bad endpoint:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result, "withheld_rows=2", "poisoned_lanes=1", "poisoned_families=0")
	stats := directPairWindowStats(t, output)
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 ||
		len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 {
		t.Fatalf("converter -> tracequery exact-lane anti-rescue failed: WQ=%+v DMA=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity)
	}
}

func TestDirectPairBarrierUnknownKeyPoisonsOnlyItsFamily(t *testing.T) {
	wqStart := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xbbb, 0x111)
	wqEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xbbb, 0x222)
	dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
	dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 7, 9, false)
	dmaBad := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
	binary.LittleEndian.PutUint32(dmaBad.content[8:12], binary.LittleEndian.Uint32(dmaBad.content[8:12])&0xffff)

	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{
			{id: 211, format: wqStart.format}, {id: 212, format: wqEnd.format},
			{id: 213, format: dmaStart.format}, {id: 214, format: dmaEnd.format},
		},
		[]syntheticRawEvent{
			{EventID: 213, OffsetNS: 1_000, Content: dmaStart.content},
			{EventID: 213, OffsetNS: 2_000, Content: dmaBad.content},
			{EventID: 214, OffsetNS: 3_000, Content: dmaEnd.content},
			{EventID: 211, OffsetNS: 4_000, Content: wqStart.content},
			{EventID: 212, OffsetNS: 5_000, Content: wqEnd.content},
		},
	)
	if strings.Contains(body, "dma_fence_wait_") || strings.Count(body, "work struct 0xbbb") != 2 {
		t.Fatalf("unknown DMA key did not remain family-local:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result, "withheld_rows=2", "poisoned_families=1")
	stats := directPairWindowStats(t, output)
	if len(stats.DMAFenceActivity) != 0 || len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("family anti-rescue leaked or harmed WQ: WQ=%+v DMA=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity)
	}
}

func TestDirectPairBarrierOverlappingDMAIdentityPoisonsFamilyAndPreservesWorkqueue(t *testing.T) {
	wqStart := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xbbb, 0x111)
	wqEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xbbb, 0x222)
	dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
	dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 7, 9, false)
	dmaOverlap := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
	// Make both hard-key strings claim the same physical bytes. The decoded
	// values would look like driver=display/timeline=display, but a shared byte
	// range cannot independently prove two identity dimensions.
	binary.LittleEndian.PutUint32(dmaOverlap.content[12:16], binary.LittleEndian.Uint32(dmaOverlap.content[8:12]))

	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{
			{id: 215, format: wqStart.format}, {id: 216, format: wqEnd.format},
			{id: 217, format: dmaStart.format}, {id: 218, format: dmaEnd.format},
		},
		[]syntheticRawEvent{
			{EventID: 217, OffsetNS: 1_000, Content: dmaStart.content},
			{EventID: 217, OffsetNS: 2_000, Content: dmaOverlap.content},
			{EventID: 218, OffsetNS: 3_000, Content: dmaEnd.content},
			{EventID: 215, OffsetNS: 4_000, Content: wqStart.content},
			{EventID: 216, OffsetNS: 5_000, Content: wqEnd.content},
		},
	)
	if strings.Contains(body, "dma_fence_wait_") || strings.Count(body, "work struct 0xbbb") != 2 {
		t.Fatalf("overlapping DMA identity poisoned a fake lane instead of its family, or harmed WQ:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result,
		"withheld_rows=2", "poisoned_lanes=0", "poisoned_families=1")
	stats := directPairWindowStats(t, output)
	if len(stats.DMAFenceActivity) != 0 || len(stats.WorkqueueActivity) != 1 ||
		stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("overlap anti-rescue leaked or harmed WQ: WQ=%+v DMA=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity)
	}
}

func TestDirectPairBarrierWorkqueueHardAliasPoisonsFamilyAndPreservesDMA(t *testing.T) {
	for _, alias := range []string{"addr", "address"} {
		t.Run(alias, func(t *testing.T) {
			wqStart := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xbbb, 0x111)
			wqEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xbbb, 0x222)
			wqAlias := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x333)
			wqAlias.format.Fields = append(wqAlias.format.Fields,
				eventField{Type: "void *", Name: alias, Offset: len(wqAlias.content), Size: 8})
			aliasBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(aliasBytes, 0xbbb)
			wqAlias.content = append(wqAlias.content, aliasBytes...)

			dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 7, 9, false)
			dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 7, 9, false)
			result, body, output := convertDirectPairCapture(t,
				[]directPairFormatSpec{
					{id: 219, format: wqStart.format}, {id: 220, format: wqEnd.format},
					{id: 221, format: wqAlias.format},
					{id: 222, format: dmaStart.format}, {id: 223, format: dmaEnd.format},
				},
				[]syntheticRawEvent{
					{EventID: 219, OffsetNS: 1_000, Content: wqStart.content},
					{EventID: 221, OffsetNS: 2_000, Content: wqAlias.content},
					{EventID: 220, OffsetNS: 3_000, Content: wqEnd.content},
					{EventID: 222, OffsetNS: 4_000, Content: dmaStart.content},
					{EventID: 223, OffsetNS: 5_000, Content: dmaEnd.content},
				},
			)
			if strings.Contains(body, "workqueue_execute_") || strings.Count(body, "dma_fence_wait_") != 2 {
				t.Fatalf("hard alias poisoned only canonical work or harmed DMA:\n%s", body)
			}
			assertDirectPairBarrierCaveat(t, result,
				"withheld_rows=2", "poisoned_lanes=0", "poisoned_families=1")
			stats := directPairWindowStats(t, output)
			if len(stats.WorkqueueActivity) != 0 || len(stats.DMAFenceActivity) != 1 ||
				stats.DMAFenceActivity[0].PairedCount != 1 {
				t.Fatalf("hard-alias anti-rescue leaked or harmed DMA: WQ=%+v DMA=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity)
			}
		})
	}
}

func TestDirectPairBarrierInvalidOwnerAndPoisonedDescriptorAreNotInvisible(t *testing.T) {
	t.Run("invalid owner", func(t *testing.T) {
		wqStart := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111)
		wqBad := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111)
		binary.LittleEndian.PutUint32(wqBad.content[4:8], ^uint32(0))
		wqEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xaaa, 0x222)
		dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 1, 2, false)
		dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 1, 2, false)
		result, body, _ := convertDirectPairCapture(t,
			[]directPairFormatSpec{
				{id: 221, format: wqStart.format}, {id: 222, format: wqEnd.format},
				{id: 223, format: dmaStart.format}, {id: 224, format: dmaEnd.format},
			},
			[]syntheticRawEvent{
				{EventID: 221, OffsetNS: 1_000, Content: wqStart.content},
				{EventID: 221, OffsetNS: 2_000, Content: wqBad.content},
				{EventID: 222, OffsetNS: 3_000, Content: wqEnd.content},
				{EventID: 223, OffsetNS: 4_000, Content: dmaStart.content},
				{EventID: 224, OffsetNS: 5_000, Content: dmaEnd.content},
			},
		)
		if strings.Contains(body, "workqueue_execute_") || strings.Count(body, "dma_fence_wait_") != 2 {
			t.Fatalf("invalid owner disappeared before the family barrier:\n%s", body)
		}
		assertDirectPairBarrierCaveat(t, result, "poisoned_families=1")
	})

	t.Run("recoverable poisoned descriptor", func(t *testing.T) {
		wq := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111)
		dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 1, 2, false)
		dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 1, 2, false)
		formats := strings.Join([]string{
			directPairFormatBlock(231, wq.format),
			directPairFormatBlock(231, dmaStart.format),
			directPairFormatBlock(232, wq.format),
			directPairFormatBlock(233, dmaStart.format),
			directPairFormatBlock(234, dmaEnd.format),
		}, "\n")
		result, body, _ := convertDirectPairCaptureText(t, formats, []syntheticRawEvent{
			{EventID: 231, OffsetNS: 1_000, Content: wq.content},
			{EventID: 232, OffsetNS: 2_000, Content: wq.content},
			{EventID: 233, OffsetNS: 3_000, Content: dmaStart.content},
			{EventID: 234, OffsetNS: 4_000, Content: dmaEnd.content},
		})
		if strings.Contains(body, "workqueue_execute_") || strings.Contains(body, "dma_fence_wait_") {
			t.Fatalf("cross-family poisoned descriptor candidates did not close both implicated families:\n%s", body)
		}
		assertDirectPairBarrierCaveat(t, result, "poisoned_families=2")
	})
}

func TestDirectPairBarrierBudgetsFailClosedForBothFamilies(t *testing.T) {
	newBarrier := func(t *testing.T) *directPairCaptureBarrier {
		t.Helper()
		barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "pair.ftrace"))
		if err != nil {
			t.Fatal(err)
		}
		barrier.maxObservations = 10
		barrier.maxRows = 10
		barrier.maxLaneKeys = 10
		return barrier
	}
	workA := directPairAdmittedAudit(t,
		directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111))
	workB := directPairAdmittedAudit(t,
		directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xbbb, 0x222))

	tests := []struct {
		name string
		run  func(*directPairCaptureBarrier)
	}{
		{
			name: "observation cap",
			run: func(barrier *directPairCaptureBarrier) {
				barrier.maxObservations = 1
				barrier.observe(workA)
				barrier.observe(workA)
			},
		},
		{
			name: "published row cap",
			run: func(barrier *directPairCaptureBarrier) {
				barrier.maxRows = 1
				barrier.observe(workA)
				barrier.addPublishedRow(1, workA)
				barrier.observe(workA)
				barrier.addPublishedRow(2, workA)
			},
		},
		{
			name: "lane key cap",
			run: func(barrier *directPairCaptureBarrier) {
				barrier.maxLaneKeys = 1
				barrier.observe(workA)
				barrier.observe(workB)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			barrier := newBarrier(t)
			test.run(barrier)
			if !barrier.budgetFailed || barrier.poisonedFamilyCount() != 2 {
				t.Fatalf("budget did not close both pair families: failed=%t kinds=%v", barrier.budgetFailed, barrier.poisonedKinds)
			}
			rows := []renderedRow{
				{seq: 101, pairKind: pairRenderWorkqueue, line: "work"},
				{seq: 102, pairKind: pairRenderDMAFence, line: "dma"},
				{seq: 103, pairKind: pairRenderUnknown, line: "inventory"},
			}
			filtered := barrier.filter(rows)
			if len(filtered) != 1 || filtered[0].seq != 103 {
				t.Fatalf("budget fail-close leaked pair rows or removed inventory: %+v", filtered)
			}
		})
	}
}

func TestDirectPairBarrierMissingPublishedRowMappingFailsClosedGlobally(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "pair.ftrace"))
	if err != nil {
		t.Fatal(err)
	}
	rows := []renderedRow{
		{seq: 41, pairKind: pairRenderWorkqueue, line: "unmapped-pair"},
		{seq: 43, pairKind: pairRenderDMAFence, line: "otherwise-mapped-pair"},
		{seq: 42, pairKind: pairRenderUnknown, line: "inventory"},
	}
	barrier.rows[43] = directPairBarrierRow{kind: pairRenderDMAFence, lane: "known-lane"}
	filtered := barrier.filter(rows)
	if len(filtered) != 1 || filtered[0].seq != 42 || barrier.poisonedRows != 2 ||
		!barrier.budgetFailed || barrier.poisonedFamilyCount() != 2 {
		t.Fatalf("missing barrier row mapping did not fail closed globally: filtered=%+v poisoned=%d failed=%t kinds=%v",
			filtered, barrier.poisonedRows, barrier.budgetFailed, barrier.poisonedKinds)
	}
}

func TestDirectPairBarrierSourceNamespaceIsAbsoluteAndOutputScoped(t *testing.T) {
	relative := filepath.Join("relative", "pair-a.ftrace")
	wantAbsolute, err := filepath.Abs(relative)
	if err != nil {
		t.Fatal(err)
	}
	first, err := newDirectPairCaptureBarrier(relative)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newDirectPairCaptureBarrier(filepath.Join("relative", "pair-b.ftrace"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(first.source) || first.source != filepath.Clean(wantAbsolute) || first.source == second.source {
		t.Fatalf("output namespace was not canonical and isolated: first=%q second=%q want=%q", first.source, second.source, filepath.Clean(wantAbsolute))
	}

	audit := directPairAdmittedAudit(t,
		directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111))
	firstLane, firstOK := pairingEndpointLaneKey(audit.Verdict, first.source)
	secondLane, secondOK := pairingEndpointLaneKey(audit.Verdict, second.source)
	if !firstOK || !secondOK || firstLane == secondLane {
		t.Fatalf("different output artifacts shared a pair lane: first=%q/%t second=%q/%t", firstLane, firstOK, secondLane, secondOK)
	}

	for _, output := range []string{"", " \t "} {
		if barrier, createErr := newDirectPairCaptureBarrier(output); createErr == nil || barrier != nil {
			t.Fatalf("empty output namespace admitted: output=%q barrier=%+v err=%v", output, barrier, createErr)
		} else if reason, ok := traceDBOutputInvariantReason(createErr); !ok || reason != "invalid_direct_pair_output_namespace" {
			t.Fatalf("empty namespace returned wrong typed error: output=%q reason=%q typed=%t err=%v", output, reason, ok, createErr)
		}
	}
}

func directPairAdmittedAudit(t *testing.T, fixture directPairTestFixture) directPairLineAudit {
	t.Helper()
	ev := decodeEvent(fixture.format, fixture.content)
	payload, admission, reason := decodeDirectPairPayload(ev, fixture.content)
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("seed pair admission=%d reason=%q", admission, reason)
	}
	audit := newDirectPairLineAudit(ev, payload)
	if !audit.Governed || !audit.HeaderOwnerKnown || !audit.Verdict.KeyKnown ||
		!audit.Verdict.PayloadAdmitted || !audit.Verdict.EmitterAdmitted {
		t.Fatalf("seed pair audit incomplete: %+v", audit)
	}
	audit.EndpointAdmitted = true
	return audit
}

type directPairFormatSpec struct {
	id     int
	format eventFormat
}

func convertDirectPairCapture(t *testing.T, formats []directPairFormatSpec, events []syntheticRawEvent) (Result, string, string) {
	t.Helper()
	blocks := make([]string, 0, len(formats))
	for _, item := range formats {
		blocks = append(blocks, directPairFormatBlock(item.id, item.format))
	}
	return convertDirectPairCaptureText(t, strings.Join(blocks, "\n"), events)
}

func convertDirectPairCaptureText(t *testing.T, formats string, events []syntheticRawEvent) (Result, string, string) {
	t.Helper()
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formats))
	writeSegment(&capture, segmentCmdlines, []byte("100 pair-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents(events))
	dir := t.TempDir()
	input := filepath.Join(dir, "pair.sys")
	output := filepath.Join(dir, "pair.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert direct pair fixture: %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return result, string(body), output
}

func directPairFormatBlock(id int, format eventFormat) string {
	lines := []string{"name: " + format.Name, fmt.Sprintf("ID: %d", id), "format:"}
	for _, field := range format.Fields {
		lines = append(lines, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
	}
	printFmt := format.PrintFmt
	if strings.TrimSpace(printFmt) == "" {
		printFmt = `"synthetic"`
	}
	lines = append(lines, "print fmt: "+printFmt, "")
	return strings.Join(lines, "\n")
}

func assertDirectPairBarrierCaveat(t *testing.T, result Result, wants ...string) {
	t.Helper()
	joined := strings.Join(result.Caveats, "\n")
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Fatalf("pair barrier caveat missing %q:\n%s", want, joined)
		}
	}
}

func directPairWindowStats(t *testing.T, output string) tracequery.WindowStats {
	t.Helper()
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("build converted pair index: %v", err)
	}
	return tracequery.ComputeWindowStats(index, tracequery.Query{})
}
