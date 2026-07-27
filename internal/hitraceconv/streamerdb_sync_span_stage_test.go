package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type traceDBSyncSpanStageCaseResult struct {
	report    traceDBSyncSpanReport
	coverage  TraceDBCoverage
	body      string
	workspace string
}

func renderTraceDBSyncSpanStageCase(t *testing.T, options traceDBSyncSpanStageOptions,
	candidates []traceDBSyncSpanCandidate, poisons []traceDBSyncSpanLanePoison, keepControl bool,
) traceDBSyncSpanStageCaseResult {
	t.Helper()
	return renderTraceDBSyncSpanStageFenceCase(t, options, candidates, poisons, nil, keepControl)
}

func renderTraceDBSyncSpanStageFenceCase(t *testing.T, options traceDBSyncSpanStageOptions,
	candidates []traceDBSyncSpanCandidate, poisons []traceDBSyncSpanLanePoison,
	fences []traceDBSyncSpanLaneFence, keepControl bool,
) traceDBSyncSpanStageCaseResult {
	t.Helper()
	if options.TempRoot == "" {
		options.TempRoot = t.TempDir()
	}
	authority, err := newTraceDBSyncSpanAuthorityWithOptions(
		context.Background(), filepath.Join(t.TempDir(), "out.systrace"), options,
	)
	if err != nil {
		t.Fatalf("construct staged sync-span authority: %v", err)
	}
	workspace := authority.stage.workspace
	defer func() {
		if err := authority.cleanup(); err != nil {
			t.Errorf("deferred staged authority cleanup: %v", err)
		}
	}()
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if keepControl {
		if err := addTraceDBInstantRow(sink, 500_000, "stage-control", 99, 99, 0, "print: stage-control"); err != nil {
			t.Fatalf("add unrelated control row: %v", err)
		}
	}
	for _, candidate := range candidates {
		if err := authority.submit(context.Background(), candidate); err != nil {
			t.Fatalf("submit staged candidate %+v: %v", candidate, err)
		}
	}
	for _, poison := range poisons {
		if err := authority.poisonExactLane(context.Background(), poison); err != nil {
			t.Fatalf("submit staged poison %+v: %v", poison, err)
		}
	}
	for _, fence := range fences {
		if err := authority.fenceExactLane(context.Background(), fence); err != nil {
			t.Fatalf("submit staged fence %+v: %v", fence, err)
		}
	}
	report, coverage, err := authority.finalize(context.Background(), sink)
	if err != nil {
		t.Fatalf("finalize staged sync spans: %v coverage=%+v", err, coverage)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("finalize left private workspace %q: %v", workspace, err)
	}
	if err := authority.cleanup(); err != nil {
		t.Fatalf("second staged authority cleanup was not idempotent: %v", err)
	}
	var output bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(context.Background(), &output); err != nil {
		t.Fatalf("write staged sync-span rows: %v", err)
	}
	return traceDBSyncSpanStageCaseResult{
		report: report, coverage: coverage, body: output.String(), workspace: workspace,
	}
}

func traceDBSyncSpanParityCandidates() []traceDBSyncSpanCandidate {
	pointStart := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerRegistration, 10, 10, 100, 1_000_000, 1_000_000, "point-start",
	)
	outer := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 1, 10, 100, 1_000_000, 5_000_000, "outer",
	)
	inner := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 2, 10, 100, 1_000_000, 5_000_000, "inner",
	)
	outer.DepthKnown, outer.Depth, outer.DepthProvenance = true, 0, traceDBSyncSpanDepthCallstack
	inner.DepthKnown, inner.Depth, inner.DepthProvenance = true, 1, traceDBSyncSpanDepthCallstack
	nested := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, -1, 10, 100, 2_000_000, 3_000_000, "nested",
	)
	pointEnd := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 3, 10, 100, 5_000_000, 5_000_000, "point-end",
	)
	adjacent := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 0, 10, 100, 5_000_000, 6_000_000, "adjacent",
	)
	otherLane := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 1, 20, 200, 1_500_000, 2_500_000, "other-lane",
	)
	namespace := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 4, 30, 300, 1_250_000, 1_350_000, "namespace|pipe",
	)
	namespace.MarkerPID, namespace.MarkerPIDKnown = 500, true
	unavailable := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 5, 40, 400, 1_100_000, 1_200_000, "cpu|unavailable",
	)
	unavailable.StartCPU, unavailable.EndCPU = 0, 0
	unavailable.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownEnd
	unavailable.StartCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	unavailable.EndCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	return []traceDBSyncSpanCandidate{unavailable, namespace, otherLane, pointEnd, nested, inner, adjacent, outer, pointStart}
}

func TestTraceDBSyncSpanStageMemorySQLiteBodyAndReportParity(t *testing.T) {
	candidates := traceDBSyncSpanParityCandidates()
	forward := append([]traceDBSyncSpanCandidate(nil), candidates...)
	reverse := append([]traceDBSyncSpanCandidate(nil), candidates...)
	for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}

	var reference traceDBSyncSpanStageCaseResult
	for index, test := range []struct {
		name       string
		resident   int64
		candidates []traceDBSyncSpanCandidate
		backend    string
	}{
		{name: "memory-forward", resident: 1 << 20, candidates: forward, backend: "memory"},
		{name: "memory-reverse", resident: 1 << 20, candidates: reverse, backend: "memory"},
		{name: "sqlite-forward", resident: 1, candidates: forward, backend: "sqlite"},
		{name: "sqlite-reverse", resident: 1, candidates: reverse, backend: "sqlite"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{
				ResidentBytes: test.resident,
			}, test.candidates, nil, false)
			if !strings.HasPrefix(result.coverage.FieldSources["stage_backend"], test.backend+";") {
				t.Fatalf("stage backend mismatch: %+v", result.coverage)
			}
			if test.backend == "memory" {
				if result.coverage.SpillChunks != 0 || result.coverage.TempBytes != 0 {
					t.Fatalf("memory stage claimed external artifacts: %+v", result.coverage)
				}
			} else if result.coverage.SpillChunks == 0 || result.coverage.TempBytes == 0 ||
				!strings.Contains(result.coverage.FieldSources["stage_backend"], "indexed_lane_plan=true") {
				t.Fatalf("SQLite stage did not expose bounded indexed spill: %+v", result.coverage)
			}
			if !strings.Contains(result.body, "# codrax_trace_mark_exact/v1") ||
				!strings.Contains(result.body, "span_pid=500") ||
				strings.Contains(result.body, "tracing_mark_write: B|500|namespace|pipe") {
				t.Fatalf("namespace marker PID did not survive %s stage: %q", test.backend, result.body)
			}
			if !strings.Contains(result.body, "# codrax_trace_mark_cpu_unavailable/v1") ||
				!strings.Contains(result.body, "reason=unknown_end_cpu") {
				t.Fatalf("typed CPU-unavailable marker did not survive %s stage: %q", test.backend, result.body)
			}
			if index == 0 {
				reference = result
				return
			}
			if !reflect.DeepEqual(result.report, reference.report) || result.body != reference.body ||
				result.coverage.RowsRead != reference.coverage.RowsRead ||
				result.coverage.RowsEmitted != reference.coverage.RowsEmitted ||
				result.coverage.Skipped != reference.coverage.Skipped {
				t.Fatalf("memory/SQLite or submission-order parity drifted:\nreport=%+v\nwant=%+v\ncoverage=%+v\nwant coverage=%+v\n--- body\n%s\n--- want\n%s",
					result.report, reference.report, result.coverage, reference.coverage, result.body, reference.body)
			}
		})
	}
}

func TestTraceDBSyncSpanStageLocalizedFenceMemorySQLiteParity(t *testing.T) {
	candidates := []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 1000, 1500, "prefix-kept"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 2200, 2500, "interval-suppressed"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 3, 10, 100, 2200, 2400, "other-producer-kept"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 4, 10, 100, 3000, 3900, "between-kept"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 5, 10, 100, 4100, 4200, "suffix-suppressed"),
	}
	fences := []traceDBSyncSpanLaneFence{
		{
			Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 10,
			CanonicalITID: 10, CanonicalITIDKnown: true,
			Start: 2000, End: 3000, Kind: traceDBSyncSpanFenceInterval,
			Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
		},
		{
			Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 10,
			CanonicalITID: 10, CanonicalITIDKnown: true,
			Start: 4000, Kind: traceDBSyncSpanFenceSuffix,
			Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
		},
	}
	var reference traceDBSyncSpanStageCaseResult
	for index, test := range []struct {
		name     string
		resident int64
		backend  string
	}{
		{name: "memory", resident: 1 << 20, backend: "memory"},
		{name: "sqlite", resident: 1, backend: "sqlite"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := renderTraceDBSyncSpanStageFenceCase(t,
				traceDBSyncSpanStageOptions{ResidentBytes: test.resident},
				candidates, nil, fences, false)
			callstack := result.report.ByProducer[traceDBSyncSpanProducerCallstack]
			syscall := result.report.ByProducer[traceDBSyncSpanProducerSyscall]
			if result.report.LocalizedFenceLanes != 1 || result.report.PoisonedLanes != 0 ||
				result.report.SuppressedSpans != 2 || result.report.EmittedEndpoints != 6 ||
				callstack.FenceDeclarations != 2 || callstack.FenceSuppressedSpans != 2 ||
				callstack.SuppressedSpans != 2 || syscall.SuppressedSpans != 0 ||
				!strings.Contains(result.body, "prefix-kept") ||
				!strings.Contains(result.body, "between-kept") ||
				!strings.Contains(result.body, "other-producer-kept") ||
				strings.Contains(result.body, "interval-suppressed") ||
				strings.Contains(result.body, "suffix-suppressed") ||
				!strings.Contains(result.coverage.Skipped, "localized_fence_lanes=1") {
				t.Fatalf("localized fence scope drifted: report=%+v coverage=%+v body=%q",
					result.report, result.coverage, result.body)
			}
			if !strings.HasPrefix(result.coverage.FieldSources["stage_backend"], test.backend+";") {
				t.Fatalf("localized fence backend mismatch: %+v", result.coverage)
			}
			if index == 0 {
				reference = result
				return
			}
			if !reflect.DeepEqual(result.report, reference.report) ||
				result.body != reference.body ||
				result.coverage.Skipped != reference.coverage.Skipped {
				t.Fatalf("localized fence memory/SQLite parity drifted:\nreport=%+v\nwant=%+v\nbody=%q\nwant=%q",
					result.report, reference.report, result.body, reference.body)
			}
		})
	}
}

func TestTraceDBSyncSpanStageLocalizedFenceBudgetFailsClosed(t *testing.T) {
	candidate := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 1, 20, 200, 2000, 2100, "must-not-publish",
	)
	fences := []traceDBSyncSpanLaneFence{
		{
			Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 10,
			CanonicalITID: 10, CanonicalITIDKnown: true,
			Start: 1000, End: 1100, Kind: traceDBSyncSpanFenceInterval,
			Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
		},
		{
			Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 10,
			CanonicalITID: 10, CanonicalITIDKnown: true,
			Start: 1200, End: 1300, Kind: traceDBSyncSpanFenceInterval,
			Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
		},
	}
	for _, test := range []struct {
		name     string
		resident int64
	}{
		{name: "memory", resident: 1 << 20},
		{name: "sqlite", resident: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := renderTraceDBSyncSpanStageFenceCase(t,
				traceDBSyncSpanStageOptions{
					ResidentBytes:  test.resident,
					MaxActiveDepth: 1,
				},
				[]traceDBSyncSpanCandidate{candidate}, nil, fences, true)
			if result.report.BudgetFailClosedReason != traceDBSyncSpanStageBudgetActiveDepthCap ||
				result.report.EmittedEndpoints != 0 || result.report.SuppressedSpans != 1 ||
				strings.Contains(result.body, "must-not-publish") ||
				!strings.Contains(result.body, "stage-control") ||
				!strings.Contains(result.coverage.Skipped, "sync_family_budget_fail_closed=active_depth_cap") {
				t.Fatalf("localized fence budget did not fail closed: report=%+v coverage=%+v body=%q",
					result.report, result.coverage, result.body)
			}
		})
	}
}

func TestTraceDBSyncSpanStagePromotionPreservesOrdinalAndZeroBoundaryOrder(t *testing.T) {
	pointStart := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerRegistration, 10, 10, 100, 1_000_000, 1_000_000, "point-start",
	)
	outer := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 1, 10, 100, 1_000_000, 3_000_000, "outer",
	)
	pointEnd := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 2, 10, 100, 3_000_000, 3_000_000, "point-end",
	)
	resident := traceDBSyncSpanCandidateResidentBytes(pointStart) + 64 +
		traceDBSyncSpanCandidateResidentBytes(outer) + 64
	stage, err := newTraceDBSyncSpanStage(context.Background(), traceDBSyncSpanStageOptions{
		TempRoot: t.TempDir(), ResidentBytes: resident,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := stage.workspace
	defer func() {
		if err := stage.cleanup(); err != nil {
			t.Errorf("cleanup promotion stage: %v", err)
		}
	}()
	for _, candidate := range []traceDBSyncSpanCandidate{pointStart, outer} {
		if err := stage.addCandidate(context.Background(), candidate); err != nil {
			t.Fatal(err)
		}
	}
	if stage.external || len(stage.memoryCandidates) != 2 {
		t.Fatalf("stage promoted before exact resident boundary: external=%t candidates=%d", stage.external, len(stage.memoryCandidates))
	}
	if err := stage.addCandidate(context.Background(), pointEnd); err != nil {
		t.Fatalf("promotion with existing ordinals failed: %v", err)
	}
	if !stage.external || len(stage.memoryCandidates) != 0 {
		t.Fatalf("third candidate did not promote exactly once: external=%t memory=%d", stage.external, len(stage.memoryCandidates))
	}
	if err := stage.seal(context.Background()); err != nil {
		t.Fatalf("seal promoted stage: %v", err)
	}
	iterator, err := stage.candidateIterator(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer iterator.close()
	var names []string
	var ordinals []int64
	for {
		item, ok, err := iterator.next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		names = append(names, item.Candidate.Name)
		ordinals = append(ordinals, item.Ordinal)
	}
	if !reflect.DeepEqual(names, []string{"point-start", "outer", "point-end"}) ||
		!reflect.DeepEqual(ordinals, []int64{1, 2, 3}) {
		t.Fatalf("promotion lost ordinal or zero/start/end ordering: names=%v ordinals=%v", names, ordinals)
	}
	stats := stage.snapshotStats()
	if stats.Backend != "sqlite" || stats.ExternalArtifacts != 1 || !stats.LanePlanVerified {
		t.Fatalf("promoted stage stats mismatch: %+v", stats)
	}
	if err := stage.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("promoted stage workspace survived cleanup: %v", err)
	}
	if err := stage.cleanup(); err != nil {
		t.Fatalf("promoted stage cleanup is not idempotent: %v", err)
	}
}

func TestTraceDBSyncSpanStageZeroStartInteriorEndParity(t *testing.T) {
	for _, test := range []struct {
		name              string
		pointTS           int64
		differentIdentity bool
		wantIdentityLanes int
		wantEndpoints     int
	}{
		{name: "different-at-start", pointTS: 1_000_000, differentIdentity: true, wantEndpoints: 4},
		{name: "different-in-interior", pointTS: 2_000_000, differentIdentity: true, wantIdentityLanes: 1},
		{name: "same-in-interior", pointTS: 2_000_000, wantEndpoints: 4},
		{name: "different-at-end", pointTS: 3_000_000, differentIdentity: true, wantEndpoints: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			outer := traceDBTestSyncSpanCandidate(
				traceDBSyncSpanProducerCallstack, 1, 10, 100, 1_000_000, 3_000_000, "outer",
			)
			pointTGID := int64(100)
			if test.differentIdentity {
				pointTGID = 200
			}
			point := traceDBTestSyncSpanCandidate(
				traceDBSyncSpanProducerCallstack, 2, 10, pointTGID, test.pointTS, test.pointTS, "point",
			)
			if test.differentIdentity {
				point.CanonicalITID, point.OwnerIPID = 20, 2
			}

			memory := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{
				ResidentBytes: 1 << 20,
			}, []traceDBSyncSpanCandidate{point, outer}, nil, false)
			sqlite := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{
				ResidentBytes: 1,
			}, []traceDBSyncSpanCandidate{outer, point}, nil, false)
			if !reflect.DeepEqual(memory.report, sqlite.report) || memory.body != sqlite.body ||
				memory.coverage.Skipped != sqlite.coverage.Skipped {
				t.Fatalf("zero boundary changed across stage backends: memory=%+v/%q sqlite=%+v/%q",
					memory.report, memory.coverage.Skipped, sqlite.report, sqlite.coverage.Skipped)
			}
			if memory.report.IdentityLanes != test.wantIdentityLanes ||
				memory.report.EmittedEndpoints != test.wantEndpoints {
				t.Fatalf("zero boundary semantics mismatch: report=%+v\n%s", memory.report, memory.body)
			}
			if test.wantEndpoints == 0 {
				if strings.Contains(memory.body, "tracing_mark_write") {
					t.Fatalf("interior identity conflict leaked endpoints:\n%s", memory.body)
				}
				return
			}
			pointToken := "B|100|point"
			if pointTGID == 200 {
				pointToken = "B|200|point"
			}
			pointBegin := strings.Index(memory.body, pointToken)
			outerBegin := strings.Index(memory.body, "B|100|outer")
			outerEnd := strings.Index(memory.body, "E|100|")
			if pointBegin < 0 || outerBegin < 0 || outerEnd < 0 {
				t.Fatalf("zero/positive endpoints missing:\n%s", memory.body)
			}
			if test.pointTS == outer.Start && pointBegin >= outerBegin {
				t.Fatalf("exact-start zero pair did not precede positive begin:\n%s", memory.body)
			}
			if test.pointTS == outer.End && pointBegin <= outerEnd {
				t.Fatalf("exact-end zero pair did not follow positive end:\n%s", memory.body)
			}
		})
	}
}

func TestTraceDBSyncSpanStageDuplicatePoisonPromotionParity(t *testing.T) {
	duplicateA := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 7, 30, 300, 1_000_000, 2_000_000, "duplicate-a",
	)
	duplicateB := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 7, 40, 400, 1_000_000, 2_000_000, "duplicate-b",
	)
	poisoned := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 8, 50, 500, 1_000_000, 2_000_000, "poisoned",
	)
	sameLaneIndependent := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 10, 50, 500, 2_500_000, 3_000_000, "same-lane-independent",
	)
	control := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 9, 60, 600, 1_000_000, 2_000_000, "control",
	)
	candidates := []traceDBSyncSpanCandidate{duplicateA, duplicateB, poisoned, sameLaneIndependent, control}
	poisons := []traceDBSyncSpanLanePoison{
		{Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 50, CanonicalITID: 50, CanonicalITIDKnown: true, Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate},
		{Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 70, CanonicalITID: 70, CanonicalITIDKnown: true, Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate},
	}
	promotionBoundary := traceDBSyncSpanCandidateResidentBytes(duplicateA) + 64

	var reference traceDBSyncSpanStageCaseResult
	for index, test := range []struct {
		name     string
		resident int64
	}{
		{name: "memory", resident: 1 << 20},
		{name: "promotion-boundary", resident: promotionBoundary},
		{name: "forced-sqlite", resident: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{
				ResidentBytes: test.resident,
			}, candidates, poisons, false)
			if result.report.PoisonedLanes != 4 || result.report.DuplicateLanes != 2 ||
				result.report.SuppressedSpans != 3 || result.report.EmittedEndpoints != 4 ||
				strings.Contains(result.body, "duplicate-a") || strings.Contains(result.body, "duplicate-b") ||
				strings.Contains(result.body, "poisoned") || !strings.Contains(result.body, "same-lane-independent") ||
				!strings.Contains(result.body, "B|600|control") {
				t.Fatalf("duplicate/poison locality mismatch: report=%+v\n%s", result.report, result.body)
			}
			if stats := result.report.ByProducer[traceDBSyncSpanProducerCallstack]; stats.PoisonDeclarations != 2 {
				t.Fatalf("poison declaration accounting mismatch: %+v", result.report.ByProducer)
			}
			if index == 0 {
				reference = result
				return
			}
			if !reflect.DeepEqual(result.report, reference.report) || result.body != reference.body ||
				result.coverage.Skipped != reference.coverage.Skipped {
				t.Fatalf("promotion changed duplicate/poison result: got=%+v/%q want=%+v/%q",
					result.report, result.coverage.Skipped, reference.report, reference.coverage.Skipped)
			}
		})
	}
}

func TestTraceDBSyncSpanStageDuplicateIdentityCanSuppressTypedIdleLaneZero(t *testing.T) {
	idle := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerRegistration, 0, 0, 0, 1_000_000, 1_000_000, "idle",
	)
	borrowed := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerRegistration, 0, 1, 1, 2_000_000, 2_000_000, "borrowed-idle-identity",
	)
	for _, resident := range []int64{1 << 20, 1} {
		result := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: resident},
			[]traceDBSyncSpanCandidate{borrowed, idle}, nil, false)
		if result.report.DuplicateLanes != 2 || result.report.PoisonedLanes != 2 ||
			result.report.SuppressedSpans != 2 || result.report.EmittedEndpoints != 0 ||
			strings.Contains(result.body, "tracing_mark_write") {
			t.Fatalf("duplicate typed idle lane zero was not journaled/suppressed locally: %+v\n%s",
				result.report, result.body)
		}
	}
}

func TestTraceDBSyncSpanStageBudgetFailCloseKeepsUnrelatedRows(t *testing.T) {
	cleanA := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 1, 20, 200, 1_000_000, 2_000_000, "clean-a",
	)
	cleanB := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerSyscall, 2, 30, 300, 3_000_000, 4_000_000, "clean-b",
	)
	outer := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 1, 10, 100, 1_000_000, 5_000_000, "outer",
	)
	middle := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 2, 10, 100, 2_000_000, 4_000_000, "middle",
	)
	inner := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 3, 10, 100, 2_500_000, 3_500_000, "inner",
	)
	outer.DepthKnown, outer.Depth, outer.DepthProvenance = true, 0, traceDBSyncSpanDepthCallstack
	middle.DepthKnown, middle.Depth, middle.DepthProvenance = true, 1, traceDBSyncSpanDepthCallstack
	inner.DepthKnown, inner.Depth, inner.DepthProvenance = true, 2, traceDBSyncSpanDepthCallstack

	tests := []struct {
		name       string
		options    traceDBSyncSpanStageOptions
		candidates []traceDBSyncSpanCandidate
		poisons    []traceDBSyncSpanLanePoison
		reason     string
	}{
		{
			name: "record-cap", options: traceDBSyncSpanStageOptions{MaxRecords: 1},
			candidates: []traceDBSyncSpanCandidate{cleanA, cleanB}, reason: traceDBSyncSpanStageBudgetRecordCap,
		},
		{
			name: "audit-comparison-cap", options: traceDBSyncSpanStageOptions{MaxAuditComparisons: 1},
			candidates: []traceDBSyncSpanCandidate{cleanA, outer, middle, inner, cleanB}, reason: traceDBSyncSpanStageBudgetAuditCompareCap,
		},
		{
			name: "active-depth-cap", options: traceDBSyncSpanStageOptions{MaxActiveDepth: 1},
			candidates: []traceDBSyncSpanCandidate{outer, middle}, reason: traceDBSyncSpanStageBudgetActiveDepthCap,
		},
		{
			name: "sqlite-page-cap", options: traceDBSyncSpanStageOptions{
				ResidentBytes: 1, MaxTempBytes: traceDBSyncSpanSQLitePageBytes,
			},
			candidates: []traceDBSyncSpanCandidate{cleanA}, reason: traceDBSyncSpanStageBudgetSQLitePageCap,
		},
	}
	wide := cleanA
	wide.Name = strings.Repeat("wide", 128)
	tests = append(tests, struct {
		name       string
		options    traceDBSyncSpanStageOptions
		candidates []traceDBSyncSpanCandidate
		poisons    []traceDBSyncSpanLanePoison
		reason     string
	}{
		name: "active-byte-cap", options: traceDBSyncSpanStageOptions{MaxActiveBytes: 512},
		candidates: []traceDBSyncSpanCandidate{wide}, reason: traceDBSyncSpanStageBudgetActiveByteCap,
	})
	var tempDuplicateCandidates []traceDBSyncSpanCandidate
	for index := 0; index < 510; index++ {
		tid := int64(1_000 + index)
		tempDuplicateCandidates = append(tempDuplicateCandidates, traceDBTestSyncSpanCandidate(
			traceDBSyncSpanProducerSyscall, 999, tid, tid,
			1_000_000, 1_000_005, fmt.Sprintf("duplicate-%03d", index),
		))
	}
	var sqlitePageCandidates []traceDBSyncSpanCandidate
	for index := 0; index < 100; index++ {
		tid := int64(10_000 + index)
		candidate := traceDBTestSyncSpanCandidate(
			traceDBSyncSpanProducerSyscall, int64(index+1), tid, tid,
			1_000_000+int64(index)*10, 1_000_005+int64(index)*10,
			fmt.Sprintf("page-%03d-%s", index, strings.Repeat("x", 512)),
		)
		sqlitePageCandidates = append(sqlitePageCandidates, candidate)
	}
	tests = append(tests, struct {
		name       string
		options    traceDBSyncSpanStageOptions
		candidates []traceDBSyncSpanCandidate
		poisons    []traceDBSyncSpanLanePoison
		reason     string
	}{
		name: "sqlite-live-page-cap", options: traceDBSyncSpanStageOptions{
			ResidentBytes: 1, MaxTempBytes: 2 * traceDBSyncSpanSQLiteMinimumPages * traceDBSyncSpanSQLitePageBytes,
		},
		candidates: sqlitePageCandidates, reason: traceDBSyncSpanStageBudgetSQLitePageCap,
	})
	tests = append(tests, struct {
		name       string
		options    traceDBSyncSpanStageOptions
		candidates []traceDBSyncSpanCandidate
		poisons    []traceDBSyncSpanLanePoison
		reason     string
	}{
		name: "temp-byte-cap", options: traceDBSyncSpanStageOptions{
			ResidentBytes: 1 << 20, MaxTempBytes: traceDBSyncSpanSQLitePageBytes,
		},
		candidates: append([]traceDBSyncSpanCandidate{cleanA}, tempDuplicateCandidates...),
		reason:     traceDBSyncSpanStageBudgetTempByteCap,
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := renderTraceDBSyncSpanStageCase(t, test.options, test.candidates, test.poisons, true)
			if result.report.BudgetFailClosedReason != test.reason ||
				result.report.SuppressedSpans != result.report.SubmittedSpans ||
				result.report.EmittedEndpoints != 0 || result.coverage.RowsEmitted != 0 || result.coverage.Error != "" ||
				!strings.Contains(result.coverage.Skipped, "sync_family_budget_fail_closed="+test.reason) {
				t.Fatalf("budget did not fail-close the whole sync family: report=%+v coverage=%+v",
					result.report, result.coverage)
			}
			if !strings.Contains(result.body, "print: stage-control") || strings.Contains(result.body, "tracing_mark_write: B|") {
				t.Fatalf("budget fail-close lost unrelated row or leaked sync endpoint:\n%s", result.body)
			}
		})
	}
}

func TestTraceDBSyncSpanStagePositiveAuditPrecedesTentativeZeroConflict(t *testing.T) {
	outer := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 1, 10, 100, 1_000_000, 3_000_000, "outer",
	)
	point := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 2, 10, 200, 2_000_000, 2_000_000, "identity-point",
	)
	point.CanonicalITID, point.OwnerIPID = 20, 2
	crossing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 3, 10, 100, 2_500_000, 4_000_000, "crossing",
	)
	for _, resident := range []int64{1 << 20, 1} {
		result := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: resident},
			[]traceDBSyncSpanCandidate{point, crossing, outer}, nil, false)
		if result.report.CrossingLanes != 1 || result.report.IdentityLanes != 0 ||
			result.report.SuppressedSpans != 3 || result.report.EmittedEndpoints != 0 {
			t.Fatalf("positive audit did not retain B1-b precedence over a tentative zero conflict: %+v", result.report)
		}
	}
}

func TestTraceDBSyncSpanStageBadLaneJournalRejectsCorruptionBeforeReader(t *testing.T) {
	stage, err := newTraceDBSyncSpanStage(context.Background(), traceDBSyncSpanStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	if err := stage.seal(context.Background()); err != nil {
		t.Fatal(err)
	}
	journal, err := stage.newBadLaneJournal()
	if err != nil {
		t.Fatal(err)
	}
	defer journal.abort()
	if err := journal.add(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if err := journal.seal(context.Background()); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(journal.path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	var checksumByte [1]byte
	if _, err := file.ReadAt(checksumByte[:], info.Size()-1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	checksumByte[0] ^= 0xff
	if _, err := file.WriteAt(checksumByte[:], info.Size()-1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.reader(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "sync_span_bad_lane_journal_checksum") {
		t.Fatalf("corrupt bad-lane journal was accepted before publication: %v", err)
	}
}

func TestTraceDBSyncSpanStageCleanupIsIdempotent(t *testing.T) {
	for _, test := range []struct {
		name     string
		resident int64
	}{
		{name: "memory", resident: 1 << 20},
		{name: "sqlite", resident: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stage, err := newTraceDBSyncSpanStage(context.Background(), traceDBSyncSpanStageOptions{
				TempRoot: t.TempDir(), ResidentBytes: test.resident,
			})
			if err != nil {
				t.Fatal(err)
			}
			workspace := stage.workspace
			info, err := os.Stat(workspace)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("private workspace mode mismatch: info=%v err=%v", info, err)
			}
			candidate := traceDBTestSyncSpanCandidate(
				traceDBSyncSpanProducerSyscall, 1, 10, 100, 1_000_000, 2_000_000, "cleanup",
			)
			if err := stage.addCandidate(context.Background(), candidate); err != nil {
				t.Fatal(err)
			}
			if stage.external {
				dbInfo, err := os.Stat(stage.dbPath)
				if err != nil || dbInfo.Mode().Perm() != 0o600 {
					t.Fatalf("private SQLite mode mismatch: info=%v err=%v", dbInfo, err)
				}
			}
			if err := stage.cleanup(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(workspace); !os.IsNotExist(err) {
				t.Fatalf("workspace survived cleanup: %v", err)
			}
			if err := stage.cleanup(); err != nil {
				t.Fatalf("second cleanup failed: %v", err)
			}
		})
	}
}

func TestTraceDBSyncSpanStageSQLiteLaneQueryPlanUsesIndex(t *testing.T) {
	stage, err := newTraceDBSyncSpanStage(context.Background(), traceDBSyncSpanStageOptions{
		TempRoot: t.TempDir(), ResidentBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stage.cleanup(); err != nil {
			t.Errorf("cleanup query-plan stage: %v", err)
		}
	}()
	for _, candidate := range traceDBSyncSpanParityCandidates() {
		if err := stage.addCandidate(context.Background(), candidate); err != nil {
			t.Fatal(err)
		}
	}
	if err := stage.seal(context.Background()); err != nil {
		t.Fatalf("seal SQLite query-plan fixture: %v", err)
	}
	rows, err := stage.conn.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+traceDBSyncSpanSelectCandidatesSQL)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int64
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, strings.ToUpper(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, "CANDIDATE_LANE_IDX") || strings.Contains(plan, "TEMP B-TREE") ||
		!stage.snapshotStats().LanePlanVerified {
		t.Fatalf("SQLite lane scan is not mechanically index-bounded:\n%s\nstats=%+v", plan, stage.snapshotStats())
	}
}
