package hitraceconv

import (
	"encoding/binary"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDirectBlockBarrierRejectsSameLaneRescueAndPreservesCrossEmitterPairs(t *testing.T) {
	const (
		dev     = uint64(8 << 20)
		badLBA  = uint64(123)
		goodLBA = uint64(456)
	)
	rqIssueA := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	rqBadA := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 8, rwbs: "R",
	})
	directSetField(&rqBadA.format, "error", func(field *eventField) { field.Signed = false })
	rqDoneA := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 8, rwbs: "R",
	})
	rqIssueB := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: goodLBA, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	rqDoneB := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: goodLBA, nrSector: 8, rwbs: "R",
	})

	bioQueueA := directBlockPairFixture("block_bio_queue", 100, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 16, rwbs: "W", comm: "io",
	})
	bioBadA := directBlockPairFixture("block_bio_complete", 2, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 16, rwbs: "W",
	})
	directSetField(&bioBadA.format, "error", func(field *eventField) { field.Signed = false })
	bioDoneA := directBlockPairFixture("block_bio_complete", 2, directBlockFixtureValues{
		dev: dev, sector: badLBA, nrSector: 16, rwbs: "W",
	})
	bioQueueB := directBlockPairFixture("block_bio_queue", 100, directBlockFixtureValues{
		dev: dev, sector: goodLBA, nrSector: 16, rwbs: "W", comm: "io",
	})
	bioDoneB := directBlockPairFixture("block_bio_complete", 2, directBlockFixtureValues{
		dev: dev, sector: goodLBA, nrSector: 16, rwbs: "W",
	})

	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{
			{id: 301, format: rqIssueA.format}, {id: 302, format: rqBadA.format}, {id: 303, format: rqDoneA.format},
			{id: 304, format: bioQueueA.format}, {id: 305, format: bioBadA.format}, {id: 306, format: bioDoneA.format},
		},
		[]syntheticRawEvent{
			{EventID: 301, OffsetNS: 1_000, Content: rqIssueA.content},
			{EventID: 302, OffsetNS: 2_000, Content: rqBadA.content},
			{EventID: 303, OffsetNS: 3_000, Content: rqDoneA.content},
			{EventID: 301, OffsetNS: 4_000, Content: rqIssueB.content},
			{EventID: 303, OffsetNS: 5_000, Content: rqDoneB.content},
			{EventID: 304, OffsetNS: 6_000, Content: bioQueueA.content},
			{EventID: 305, OffsetNS: 7_000, Content: bioBadA.content},
			{EventID: 306, OffsetNS: 8_000, Content: bioDoneA.content},
			{EventID: 304, OffsetNS: 9_000, Content: bioQueueB.content},
			{EventID: 306, OffsetNS: 10_000, Content: bioDoneB.content},
		},
	)
	if strings.Contains(body, " 123 + ") || strings.Count(body, "block_rq_issue:") != 1 ||
		strings.Count(body, "block_rq_complete:") != 1 || strings.Count(body, "block_bio_queue:") != 1 ||
		strings.Count(body, "block_bio_complete:") != 1 {
		t.Fatalf("same-lane rejected endpoint was rescued or clean sibling was lost:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result,
		"withheld_rows=4", "poisoned_lanes=2", "poisoned_families=0",
		"legacy_budget_reason=none", "block_budget_reason=none", "shared_authority_reason=none")
	assertDirectBlockBarrierCoverage(t, result, 10, 8, 4, 4)

	stats := directPairWindowStats(t, output)
	if len(stats.IOLatencies) != 2 {
		t.Fatalf("converter -> tracequery block pairs mismatch: %+v caveats=%v", stats.IOLatencies, stats.Caveats)
	}
	for _, latency := range stats.IOLatencies {
		if latency.Sector != int64(goodLBA) || latency.IssueThread.PID != 100 || latency.CompleteThread.PID != 2 {
			t.Fatalf("Block owner leaked into request identity or bad lane survived: %+v", latency)
		}
	}
	if rq := directBlockStorageLatency(stats, "block_rq"); rq == nil || rq.PairedCount != 1 {
		t.Fatalf("RQ pair not published exactly once: %+v", stats.StorageLatencyByLayer)
	}
	if bio := directBlockStorageLatency(stats, "block_bio"); bio == nil || bio.PairedCount != 1 {
		t.Fatalf("BIO pair not published exactly once: %+v", stats.StorageLatencyByLayer)
	}
}

func TestDirectBlockBarrierAuditsPhysicalTimestampOrderBeforeOutputSort(t *testing.T) {
	const dev = uint64(8 << 20)
	issueA := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	doneA := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: 123, nrSector: 8, rwbs: "R",
	})
	issueB := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: 456, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	doneB := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: 456, nrSector: 8, rwbs: "R",
	})

	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{{id: 311, format: issueA.format}, {id: 312, format: doneA.format}},
		[]syntheticRawEvent{
			// Physical order is intentionally not timestamp order. Sorting first
			// would turn lane A into a plausible positive-duration request.
			{EventID: 311, OffsetNS: 3_000, Content: issueA.content},
			{EventID: 312, OffsetNS: 2_000, Content: doneA.content},
			{EventID: 311, OffsetNS: 1_000, Content: issueB.content},
			{EventID: 312, OffsetNS: 4_000, Content: doneB.content},
		},
	)
	if strings.Contains(body, " 123 + ") || strings.Count(body, "block_rq_issue:") != 1 ||
		strings.Count(body, "block_rq_complete:") != 1 {
		t.Fatalf("timestamp rollback crossed output sort or harmed sibling:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result, "withheld_rows=2", "poisoned_lanes=1", "poisoned_families=0")
	assertDirectBlockBarrierCoverage(t, result, 4, 4, 2, 2)
	stats := directPairWindowStats(t, output)
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].Sector != 456 {
		t.Fatalf("physical rollback lane was rescued or sibling pair lost: %+v caveats=%v", stats.IOLatencies, stats.Caveats)
	}
}

func TestDirectBlockBarrierSameTimestampUsesStablePhysicalSequence(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(t.TempDir() + "/same-ts.sys")
	if err != nil {
		t.Fatal(err)
	}
	issue := directBlockAdmittedAudit(t, "block_bio_queue", 0, directBlockFixtureValues{
		dev: 8 << 20, sector: 77, nrSector: 8, rwbs: "R", comm: "io",
	})
	done := directBlockAdmittedAudit(t, "block_bio_complete", 2, directBlockFixtureValues{
		dev: 8 << 20, sector: 77, nrSector: 8, rwbs: "R",
	})
	barrier.observe(issue)
	barrier.addPublishedRowAt(1, 9_000, issue)
	barrier.observe(done)
	barrier.addPublishedRowAt(2, 9_000, done)
	rows := barrier.filter([]renderedRow{
		{seq: 1, pairKind: pairRenderBlock, line: "start"},
		{seq: 2, pairKind: pairRenderBlock, line: "done"},
	})
	if err := barrier.validateAccounting(rows); err != nil || len(rows) != 2 ||
		barrier.poisonedLaneCount() != 0 || barrier.authorityFailure != "" {
		t.Fatalf("same timestamp lost stable-sequence admission: rows=%+v barrier=%+v err=%v", rows, barrier, err)
	}
}

func TestDirectBlockBarrierKeepsKnownOwnerAndKeyWhenNonOwnerEnvelopeFails(t *testing.T) {
	fixture := directBlockPairFixture("block_bio_queue", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 77, nrSector: 8, rwbs: "R", comm: "io",
	})
	for index := range fixture.format.Fields {
		if fixture.format.Fields[index].Name == "common_flags" {
			fixture.format.Fields[index].Type = "unsigned short"
		}
	}
	_, _, _, envelopeOK, audit := renderEventLineDecisionWithPairAudit(
		renderContext{}, 1_000, 0, fixture.format, fixture.content,
	)
	if envelopeOK || !audit.Governed || !audit.HeaderOwnerKnown || !audit.Verdict.KeyKnown || audit.EndpointAdmitted {
		t.Fatalf("non-owner envelope failure erased exact Block provenance: envelope=%t audit=%+v", envelopeOK, audit)
	}
	barrier, err := newDirectPairCaptureBarrier(t.TempDir() + "/non-owner-envelope.sys")
	if err != nil {
		t.Fatal(err)
	}
	barrier.observe(audit)
	if barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 1 {
		t.Fatalf("non-owner envelope failure widened beyond its known Block lane: kinds=%v lanes=%v",
			barrier.poisonedKinds, barrier.poisonedLanes)
	}
}

func TestDirectPairBarrierAccountingRejectsCorruptionBeforePublication(t *testing.T) {
	barrier, err := newDirectPairCaptureBarrier(t.TempDir() + "/accounting.sys")
	if err != nil {
		t.Fatal(err)
	}
	barrier.stagedRows[pairRenderBlock] = 1
	barrier.withheldRows[pairRenderBlock] = 2
	barrier.poisonedRows = 2
	if err := barrier.validateAccounting(nil); err == nil {
		t.Fatal("withheld>staged corruption reached the output boundary")
	}
}

func TestDirectBlockBarrierFormatConflictHasExactProvenanceAndCoverage(t *testing.T) {
	issue := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	inventory := directBlockPairFixture("block_rq_insert", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	formats := strings.Join([]string{
		directPairFormatBlock(321, issue.format),
		directPairFormatBlock(321, inventory.format),
	}, "\n")
	result, body, _ := convertDirectPairCaptureText(t, formats, []syntheticRawEvent{
		{EventID: 321, OffsetNS: 1_000, Content: issue.content},
	})
	if strings.Contains(body, "block_rq_") {
		t.Fatalf("conflicting Block descriptor published a row:\n%s", body)
	}
	assertDirectPairBarrierCaveat(t, result, "withheld_rows=0", "poisoned_families=1")
	assertDirectBlockBarrierCoverage(t, result, 1, 0, 0, 0)
}

func TestDirectBlockInventoryFormatConflictDoesNotClaimEndpointProvenance(t *testing.T) {
	insert := directBlockPairFixture("block_rq_insert", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	remap := directBlockPairFixture("block_rq_remap", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, oldDev: 7 << 20, oldSector: 99, nrBios: 1, rwbs: "R",
	})
	formats := strings.Join([]string{
		directPairFormatBlock(322, insert.format),
		directPairFormatBlock(322, remap.format),
	}, "\n")
	result, _, _ := convertDirectPairCaptureText(t, formats, []syntheticRawEvent{
		{EventID: 322, OffsetNS: 1_000, Content: insert.content},
	})
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_raw_ftrace:block_capture" {
			t.Fatalf("inventory-only descriptor conflict claimed Block endpoint provenance: %+v", coverage)
		}
	}
	joined := strings.Join(result.Caveats, "\n")
	if strings.Contains(joined, "rejected Workqueue/DMA/MMC/F2FS/Block endpoints") {
		t.Fatalf("inventory-only descriptor conflict poisoned the Block pair family: %s", joined)
	}
}

func directBlockPairFixture(name string, tid int32, values directBlockFixtureValues) directPairTestFixture {
	format, payload := directBlockFixture(name, values)
	content := make([]byte, 8+len(payload))
	copy(content[8:], payload)
	format.Fields = append(directPairCommonFields(), shiftedEventFields(format.Fields, 8)...)
	directPairFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[4:8], uint32(tid))
	return directPairTestFixture{format: format, content: content}
}

func assertDirectBlockBarrierCoverage(t *testing.T, result Result, read, staged, withheld, emitted int) {
	t.Helper()
	for _, coverage := range result.TraceCoverage {
		if coverage.Family != "builtin_raw_ftrace:block_capture" || coverage.Table != "__complete_capture_barrier__" {
			continue
		}
		if coverage.Role != "unsupported_input" || coverage.RowsRead != read || coverage.RowsEmitted != emitted ||
			coverage.FieldSources["scope"] != "source_local" || coverage.FieldSources["proof_domain"] != "block" ||
			coverage.FieldSources["rows_staged"] != strconv.Itoa(staged) ||
			coverage.FieldSources["rows_withheld"] != strconv.Itoa(withheld) {
			t.Fatalf("direct Block barrier coverage mismatch: %+v", coverage)
		}
		return
	}
	t.Fatalf("direct Block barrier coverage missing: %+v", result.TraceCoverage)
}

func directBlockStorageLatency(stats tracequery.WindowStats, event string) *tracequery.StorageLatencySummary {
	for index := range stats.StorageLatencyByLayer {
		row := &stats.StorageLatencyByLayer[index]
		if row.Layer == "block" && row.Event == event {
			return row
		}
	}
	return nil
}
