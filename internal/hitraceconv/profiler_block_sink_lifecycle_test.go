package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilerCaptureSourceNamespaceResolvesPhysicalSymlink(t *testing.T) {
	dir := t.TempDir()
	realSource := filepath.Join(dir, "capture.htrace")
	linkSource := filepath.Join(dir, "capture-link.htrace")
	if err := os.WriteFile(realSource, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSource, linkSource); err != nil {
		t.Fatal(err)
	}

	open := func(path string) *traceDBRowSink {
		t.Helper()
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := sink.cleanup(); err != nil {
				t.Errorf("cleanup row sink: %v", err)
			}
		})
		if err := sink.openProfilerCapture(path); err != nil {
			t.Fatal(err)
		}
		return sink
	}
	realSink := open(realSource)
	linkSink := open(linkSource)
	resolved, err := filepath.EvalSymlinks(realSource)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved = filepath.Clean(resolved)
	if realSink.captureSource != resolved || linkSink.captureSource != resolved ||
		realSink.captureSource != linkSink.captureSource {
		t.Fatalf("one physical Profiler source split across symlink namespaces: real=%q link=%q want=%q",
			realSink.captureSource, linkSink.captureSource, resolved)
	}
}

func TestProfilerCaptureLifecycleRequiresSealAndRejectsPostSealMutation(t *testing.T) {
	source := filepath.Join(t.TempDir(), "capture.htrace")
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("open write rejected then sealed write succeeds", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(source); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "ordinary-row"}); err != nil {
			t.Fatal(err)
		}
		if _, err := sink.writeTo(context.Background(), &bytes.Buffer{}); traceDBInvariantReason(err) != "profiler_capture_write_before_seal" {
			t.Fatalf("open capture reached publication: %v", err)
		}
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsAccepted != 1 || stats.RowsWritten != 1 || stats.RowsWithheld != 0 || !strings.HasSuffix(output.String(), "ordinary-row\n") {
			t.Fatalf("sealed capture accounting drifted: stats=%+v output=%q", stats, output.String())
		}
	})

	t.Run("sealed add records breach and blocks publication", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(source); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "prefix"}); err != nil {
			t.Fatal(err)
		}
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 2, seq: 2, line: "late"}); traceDBInvariantReason(err) != "profiler_capture_add_after_seal" {
			t.Fatalf("post-seal add did not breach: err=%v breach=%q", err, sink.captureBreach)
		}
		if _, err := sink.writeTo(context.Background(), &bytes.Buffer{}); traceDBInvariantReason(err) != "profiler_capture_add_after_seal" {
			t.Fatalf("breached capture reached publication: %v", err)
		}
	})
}

func TestProfilerBlockAndLegacyProofBudgetsAreIndependent(t *testing.T) {
	t.Run("block cap preserves legacy", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		sink.blockPairProof.maxObservations = 1
		for _, row := range []renderedRow{
			{tsNS: 1, seq: 1, line: "block-start", pairKind: pairRenderBlock, pairLane: "block-a", pairTable: "block_bio_queue"},
			{tsNS: 2, seq: 2, line: "block-done", pairKind: pairRenderBlock, pairLane: "block-a", pairTable: "block_bio_complete"},
			{tsNS: 3, seq: 3, line: "mmc-survives", pairKind: pairRenderMMC, pairLane: "mmc-a", pairTable: "mmc_request_start"},
		} {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if sink.blockPairProof.failureReason != "observations" || !sink.poisoned[pairRenderBlock] ||
			sink.poisoned[pairRenderMMC] || sink.poisoned[pairRenderF2FS] || sink.legacyPairProof.failureReason != "" ||
			sink.publishableRows() != 1 {
			t.Fatalf("Block cap crossed proof domains: block=%+v legacy=%+v poisoned=%v publishable=%d",
				sink.blockPairProof, sink.legacyPairProof, sink.poisoned, sink.publishableRows())
		}
	})

	t.Run("legacy cap preserves block", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		sink.legacyPairProof.maxObservations = 1
		for _, row := range []renderedRow{
			{tsNS: 1, seq: 1, line: "mmc-start", pairKind: pairRenderMMC, pairLane: "mmc-a", pairTable: "mmc_request_start"},
			{tsNS: 2, seq: 2, line: "f2fs-cap", pairKind: pairRenderF2FS, pairLane: "f2fs-a", pairTable: "f2fs_write_begin"},
			{tsNS: 3, seq: 3, line: "block-survives", pairKind: pairRenderBlock, pairLane: "block-a", pairTable: "block_bio_queue"},
		} {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if sink.legacyPairProof.failureReason != "observations" || !sink.poisoned[pairRenderMMC] ||
			!sink.poisoned[pairRenderF2FS] || sink.poisoned[pairRenderBlock] || sink.blockPairProof.failureReason != "" ||
			sink.publishableRows() != 1 {
			t.Fatalf("legacy cap crossed proof domains: block=%+v legacy=%+v poisoned=%v publishable=%d",
				sink.blockPairProof, sink.legacyPairProof, sink.poisoned, sink.publishableRows())
		}
	})
}

func TestProfilerPairCensusUsesFixedCaptureKindSet(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if !sink.beginPairRowCensus() {
		t.Fatal("pair census did not start")
	}
	for index, kind := range profilerCaptureKinds {
		pairTable := map[pairRenderKind]string{
			pairRenderMMC: "mmc_request_start", pairRenderF2FS: "f2fs_write_begin", pairRenderBlock: "block_bio_queue",
		}[kind]
		if err := sink.add(renderedRow{
			tsNS: uint64(index + 1), seq: index + 1, line: "capture-row", pairKind: kind,
			pairLane: "lane-" + string(rune('a'+index)), pairTable: pairTable,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.add(renderedRow{tsNS: 9, seq: 9, line: "ordinary-row"}); err != nil {
		t.Fatal(err)
	}
	staged := sink.endPairRowCensus()
	if len(staged) != int(pairRenderKindCount) {
		t.Fatalf("pair census is not enum-sized: len=%d count=%d", len(staged), pairRenderKindCount)
	}
	for _, kind := range profilerCaptureKinds {
		if staged[kind].total != 1 {
			t.Fatalf("fixed census lost kind %d: %+v", kind, staged[kind])
		}
	}
	if staged[pairRenderWorkqueue].total != 0 || staged[pairRenderDMAFence].total != 0 {
		t.Fatalf("non-profiler pair families entered capture census: %+v", staged)
	}

	ledger := newProfilerContainerDiagnosticLedger()
	extracted := profilerContainerExtraction{PluginMessages: map[string]int{}}
	if _, ok := ledger.observeAccepted(&extracted, profilerPluginRouteOtherText, "other", profilerPluginData{},
		profilerPluginIssueCensus{}, 7, profilerPluginOutcomeTextRows, 3, staged); !ok || !ledger.materialize(&extracted) {
		t.Fatal("fixed plugin staged census did not materialize")
	}
	if len(extracted.TraceCoverage) != 1 || extracted.TraceCoverage[0].FieldSources[profilerCoverageBlockStagedRows] != "1" ||
		extracted.TraceCoverage[0].FieldSources[profilerCoverageMMCStagedRows] != "1" ||
		extracted.TraceCoverage[0].FieldSources[profilerCoverageF2FSStagedRows] != "1" {
		t.Fatalf("plugin staged fixed set was not disclosed: %+v", extracted.TraceCoverage)
	}
}

func TestProfilerBlockPhysicalLaneClockAuditedBeforeSort(t *testing.T) {
	t.Run("rollback poisons exact lane only", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		for _, row := range []renderedRow{
			{tsNS: 20, seq: 1, line: "lane-a-start", pairKind: pairRenderBlock, pairLane: "lane-a", pairTable: "block_bio_queue"},
			{tsNS: 5, seq: 2, line: "lane-b", pairKind: pairRenderBlock, pairLane: "lane-b", pairTable: "block_bio_queue"},
			{tsNS: 10, seq: 3, line: "lane-a-done", pairKind: pairRenderBlock, pairLane: "lane-a", pairTable: "block_bio_complete"},
		} {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if sink.poisoned[pairRenderBlock] || !profilerTestPoisonedLanes(sink)[pairRenderBlock]["lane-a"] ||
			profilerTestPoisonedLanes(sink)[pairRenderBlock]["lane-b"] || sink.withheldPairRowsForKind(pairRenderBlock) != 2 {
			t.Fatalf("physical rollback quarantine scope drifted: poisoned=%v lanes=%v", sink.poisoned, profilerTestPoisonedLanes(sink))
		}
		var output bytes.Buffer
		stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsAccepted != 3 || stats.RowsWritten != 1 || stats.RowsWithheld != 2 || !strings.HasSuffix(output.String(), "lane-b\n") {
			t.Fatalf("rollback crossed sort/publication: stats=%+v output=%q", stats, output.String())
		}
	})

	t.Run("same timestamp stable sequence is legal", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		for seq := 1; seq <= 2; seq++ {
			if err := sink.add(renderedRow{tsNS: 10, seq: seq, line: "same-ts", pairKind: pairRenderBlock, pairLane: "lane", pairTable: "block_bio_queue"}); err != nil {
				t.Fatal(err)
			}
		}
		if sink.pairKindPoisoned(pairRenderBlock) || sink.publishableRows() != 2 {
			t.Fatalf("same timestamp was treated as rollback: poisoned=%v lanes=%v", sink.poisoned, profilerTestPoisonedLanes(sink))
		}
	})
}

func TestProfilerCaptureAccountingRejectsWithheldAboveStagedWithoutClamp(t *testing.T) {
	source := filepath.Join(t.TempDir(), "capture.htrace")
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "block", pairKind: pairRenderBlock, pairLane: "lane", pairTable: "block_bio_queue"}); err != nil {
		t.Fatal(err)
	}
	sink.pairFixedLedger.families[pairRenderBlock].withheld = 2
	sink.pairFixedLedger.endpoints[profilerPairEndpointBlockBIOQueue].withheld = 2
	if _, err := sink.withheldPairRowsForKindChecked(pairRenderBlock); traceDBInvariantReason(err) != "profiler_pair_fixed_ledger_invalid" {
		t.Fatalf("withheld>staged was clamped instead of rejected by the scalar guard: err=%v", err)
	}
	if err := sink.sealProfilerCapture(); traceDBInvariantReason(err) != "profiler_pair_fixed_ledger_invalid" {
		t.Fatalf("invalid fixed accounting escaped the seal guard: err=%v breach=%q", err, sink.captureBreach)
	}
	if _, err := sink.writeTo(context.Background(), &bytes.Buffer{}); traceDBInvariantReason(err) != "profiler_capture_accounting_invalid" {
		t.Fatalf("invalid accounting reached publication: %v", err)
	}
}

func TestProfilerBlockTextBarrierIsSourceWideForContainerAndSession(t *testing.T) {
	rows := []string{
		profilerBlockBarrierTextLine(5_002_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []"),
		profilerBlockBarrierTextLine(5_001_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]"),
		profilerBlockBarrierTextLine(5_003_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]"),
		profilerBlockBarrierTextLine(5_004_000_000, "block_rq_issue: 0,1 R 4 () 10 + 3 []"),
		profilerBlockBarrierTextLine(5_005_000_000, "block_rq_complete: 0,1 R (READ) 10 + 3 [0]"),
	}
	for _, row := range rows {
		if pair := profilerTextPairAdmission(row); !pair.Governed || pair.Kind != pairRenderBlock {
			t.Fatalf("fixture row lost exact Block provenance: pair=%+v row=%q", pair, row)
		} else if !pair.Admitted || !pair.LaneKnown || !pair.HeaderOwnerKnown {
			t.Fatalf("fixture row was not a fully proven Block endpoint: pair=%+v row=%q", pair, row)
		}
	}

	t.Run("binary container messages share one barrier", func(t *testing.T) {
		frames := make([]profilerResourceFrame, 0, len(rows))
		for _, row := range rows {
			message := syntheticProfilerPluginData("bytrace_plugin", []byte(row))
			frames = append(frames, profilerResourceFrame{declared: uint32(len(message)), payload: message})
		}
		profilerAssertBlockTextSourceWideResult(t, profilerResourceTraceFile(frames...))
	})

	t.Run("session records share one barrier", func(t *testing.T) {
		body := []byte(profilerSessionJSONTag + "\n" + strings.Join(rows, "\n") + "\n")
		profilerAssertBlockTextSourceWideResult(t, body)
	})
}

func profilerBlockBarrierTextLine(tsNS int64, body string) string {
	return traceDBFormatLine("worker", 40, 40, 2, tsNS, 0, 0, body)
}

func profilerAssertBlockTextSourceWideResult(t *testing.T, input []byte) {
	t.Helper()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.trace")
	outputPath := filepath.Join(dir, "output.ftrace")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: inputPath},
		int64(len(input)), outputPath, nil, nil, nil, nil, nil)
	if err != nil || !handled {
		t.Fatalf("profiler source-wide conversion failed: handled=%t err=%v result=%+v", handled, err, result)
	}
	if result.OutputPath == "" {
		t.Fatalf("profiler source-wide barrier suppressed every lane: result=%+v", result)
	}
	body, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if result.EventsWritten != 2 || strings.Contains(text, " 2 + 3") ||
		strings.Count(text, " 10 + 3") != 2 {
		t.Fatalf("same-key Block hole was rescued or unrelated lane lost: events=%d\n%s", result.EventsWritten, text)
	}
	barriers := 0
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
			barriers++
			if coverage.RowsRead != 3 || coverage.FieldSources["budget_failure"] != "none" {
				t.Fatalf("Block barrier coverage drifted: %+v", coverage)
			}
		}
	}
	if barriers != 1 {
		t.Fatalf("Block barrier coverage count=%d result=%+v", barriers, result.TraceCoverage)
	}
}

func traceDBInvariantReason(err error) string {
	if typed, ok := err.(*traceDBOutputInvariantError); ok {
		return typed.Reason
	}
	return ""
}
