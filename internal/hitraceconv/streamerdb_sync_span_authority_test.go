package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func traceDBTestSyncSpanCandidate(producer traceDBSyncSpanProducer, stableID, tid, tgid, start, end int64, name string) traceDBSyncSpanCandidate {
	candidate := traceDBSyncSpanCandidate{
		Producer:           producer,
		StableID:           stableID,
		HeaderTID:          tid,
		HeaderTGID:         tgid,
		CanonicalITID:      tid,
		CanonicalITIDKnown: true,
		OwnerIPID:          1,
		OwnerIPIDKnown:     true,
		Start:              start,
		End:                end,
		StartCPU:           1,
		EndCPU:             2,
		Task:               fmt.Sprintf("tid-%d", tid),
		Name:               name,
		DepthProvenance:    traceDBSyncSpanDepthUnknown,
	}
	switch producer {
	case traceDBSyncSpanProducerRegistration:
		candidate.StableKind = traceDBSyncSpanStableRegistrationITID
		candidate.CanonicalITID = stableID
		candidate.StartCPU, candidate.EndCPU = 0, 0
		candidate.StartCPUProvenance = traceDBSyncSpanCPURegistrationMetadata
		candidate.EndCPUProvenance = traceDBSyncSpanCPURegistrationMetadata
		candidate.NameProvenance = traceDBSyncSpanNameRegistration
	case traceDBSyncSpanProducerCallstack:
		candidate.StableKind = traceDBSyncSpanStableCallstackRowID
		candidate.StartCPUProvenance = traceDBSyncSpanCPUCallstackTypedRunning
		candidate.EndCPUProvenance = traceDBSyncSpanCPUCallstackTypedRunning
		candidate.NameProvenance = traceDBSyncSpanNameCallstack
	case traceDBSyncSpanProducerSyscall:
		candidate.StableKind = traceDBSyncSpanStableSyscallRowID
		candidate.StartCPU, candidate.EndCPU = 0, 0
		candidate.StartCPUProvenance = traceDBSyncSpanCPUSyscallTypedRunning
		candidate.EndCPUProvenance = traceDBSyncSpanCPUSyscallTypedRunning
		candidate.NameProvenance = traceDBSyncSpanNameSyscallNumber
	case traceDBSyncSpanProducerAppStartup:
		candidate.StableKind = traceDBSyncSpanStableAppStartupRowID
		candidate.CanonicalITID, candidate.CanonicalITIDKnown = 0, false
		candidate.StartCPU, candidate.EndCPU = 0, 0
		candidate.StartCPUProvenance = traceDBSyncSpanCPULegacyUnverified
		candidate.EndCPUProvenance = traceDBSyncSpanCPULegacyUnverified
		candidate.NameProvenance = traceDBSyncSpanNameAppStartupDictionary
	case traceDBSyncSpanProducerStaticInitialize:
		candidate.StableKind = traceDBSyncSpanStableStaticInitializeRowID
		candidate.CanonicalITID, candidate.CanonicalITIDKnown = 0, false
		candidate.StartCPU, candidate.EndCPU = 0, 0
		candidate.StartCPUProvenance = traceDBSyncSpanCPULegacyUnverified
		candidate.EndCPUProvenance = traceDBSyncSpanCPULegacyUnverified
		candidate.NameProvenance = traceDBSyncSpanNameStaticObject
	}
	return candidate
}

func TestTraceDBSyncSpanMarkerPIDAuthorityIsClosed(t *testing.T) {
	valid := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 17267, 17267, 1000, 2000, "namespace")
	valid.MarkerPID, valid.MarkerPIDKnown = 37722, true
	if err := validateTraceDBSyncSpanCandidate(valid); err != nil {
		t.Fatalf("valid callstack namespace PID rejected: %v", err)
	}
	for _, mutate := range []func(*traceDBSyncSpanCandidate){
		func(candidate *traceDBSyncSpanCandidate) { candidate.Producer = traceDBSyncSpanProducerSyscall },
		func(candidate *traceDBSyncSpanCandidate) { candidate.MarkerPID = 0 },
		func(candidate *traceDBSyncSpanCandidate) { candidate.MarkerPID = int64(math.MaxInt32) + 1 },
		func(candidate *traceDBSyncSpanCandidate) { candidate.MarkerPIDKnown = false },
	} {
		candidate := valid
		mutate(&candidate)
		if err := validateTraceDBSyncSpanCandidate(candidate); err == nil {
			t.Fatalf("invalid marker PID authority accepted: %+v", candidate)
		}
	}
}

func TestTraceDBSyncSpanCPUUnavailableAuthorityIsClosed(t *testing.T) {
	valid := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 101, 100, 1000, 2000, "retained")
	valid.StartCPU, valid.EndCPU = 0, 0
	valid.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownEnd
	valid.StartCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	valid.EndCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	if err := validateTraceDBSyncSpanCandidate(valid); err != nil {
		t.Fatalf("valid CPU-unavailable callstack span rejected: %v", err)
	}
	for _, mutate := range []func(*traceDBSyncSpanCandidate){
		func(candidate *traceDBSyncSpanCandidate) { candidate.Producer = traceDBSyncSpanProducerSyscall },
		func(candidate *traceDBSyncSpanCandidate) { candidate.StartCPU = 1 },
		func(candidate *traceDBSyncSpanCandidate) {
			candidate.StartCPUProvenance = traceDBSyncSpanCPUCallstackTypedRunning
		},
		func(candidate *traceDBSyncSpanCandidate) {
			candidate.CPUPlacement = traceDBSyncSpanCPUPlacement(traceDBSyncSpanCPUPlacementAliasAmbiguous + 1)
		},
	} {
		candidate := valid
		mutate(&candidate)
		if err := validateTraceDBSyncSpanCandidate(candidate); err == nil {
			t.Fatalf("invalid CPU-unavailable authority accepted: %+v", candidate)
		}
	}
}

func TestTraceDBSyncSpanGlobalSourcePoisonIsConstantStateAndKeepsBudgetDisclosure(t *testing.T) {
	ctx := context.Background()
	authority, err := newTraceDBSyncSpanAuthorityWithOptions(ctx, filepath.Join(t.TempDir(), "out.systrace"),
		traceDBSyncSpanStageOptions{MaxRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer authority.cleanup()
	for _, candidate := range []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 101, 100, 1000, 1100, "first"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 202, 200, 1200, 1300, "second"),
	} {
		if err := authority.submit(ctx, candidate); err != nil {
			t.Fatalf("submit global/budget candidate: %v", err)
		}
	}
	poison := traceDBSyncSpanGlobalPoison{
		Producer: traceDBSyncSpanProducerSyscall,
		Reason:   traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate,
	}
	if err := authority.poisonGlobally(ctx, poison); err != nil {
		t.Fatal(err)
	}
	if err := authority.poisonGlobally(ctx, poison); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	report, coverage, err := authority.finalize(ctx, sink)
	if err != nil {
		t.Fatalf("finalize global/budget poison: %v coverage=%+v", err, coverage)
	}
	if !report.GlobalPoisoned || report.SourceFailClosedReason != "unlocalizable_syscall_candidate" ||
		report.BudgetFailClosedReason != traceDBSyncSpanStageBudgetRecordCap || report.SubmittedSpans != 2 ||
		report.SuppressedSpans != 2 || report.EmittedEndpoints != 0 || report.PoisonedLanes != 0 ||
		report.ByProducer[traceDBSyncSpanProducerSyscall].GlobalPoisonDeclarations != 1 ||
		!strings.Contains(coverage.Skipped, "sync_family_source_fail_closed=unlocalizable_syscall_candidate") ||
		!strings.Contains(coverage.Skipped, "sync_family_budget_fail_closed=record_cap") {
		t.Fatalf("global source/budget disclosure drifted: report=%+v coverage=%+v", report, coverage)
	}
	items := []TraceDBCoverage{
		{Family: "slice", Table: "callstack", Found: true},
		{Family: "slice", Table: "syscall", Found: true},
	}
	if err := reconcileTraceDBSyncSpanCoverage(items, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(items[0].Skipped, "suppressed_spans=2") ||
		!strings.Contains(items[0].Skipped, "sync_family_source_fail_closed=unlocalizable_syscall_candidate") ||
		!strings.Contains(items[0].Skipped, "sync_family_budget_fail_closed=record_cap") ||
		!strings.Contains(items[1].Skipped, "global_poison_declarations=1") {
		t.Fatalf("global poison reconciliation lost split accounting: %+v", items)
	}
}

func TestTraceDBSyncSpanSyscallClosedEnumsRejectMismatch(t *testing.T) {
	baseline := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 101, 100, 1000, 1100, "sys_9")
	cpuProvenances := []traceDBSyncSpanCPUProvenance{
		traceDBSyncSpanCPUUnknown,
		traceDBSyncSpanCPURegistrationMetadata,
		traceDBSyncSpanCPUCallstackTypedRunning,
		traceDBSyncSpanCPUSyscallTypedRunning,
		traceDBSyncSpanCPULegacyUnverified,
		traceDBSyncSpanCPUProvenance(uint8(traceDBSyncSpanCPULegacyUnverified) + 1),
	}
	for _, provenance := range cpuProvenances {
		start := baseline
		start.StartCPUProvenance = provenance
		startAccepted := validateTraceDBSyncSpanCandidate(start) == nil
		if startAccepted != (provenance == traceDBSyncSpanCPUSyscallTypedRunning) {
			t.Fatalf("syscall start CPU provenance=%d accepted=%t", provenance, startAccepted)
		}
		end := baseline
		end.EndCPUProvenance = provenance
		endAccepted := validateTraceDBSyncSpanCandidate(end) == nil
		if endAccepted != (provenance == traceDBSyncSpanCPUSyscallTypedRunning) {
			t.Fatalf("syscall end CPU provenance=%d accepted=%t", provenance, endAccepted)
		}
	}

	ctx := context.Background()
	authority := newTraceDBTestSyncSpanAuthority(t)
	exactBase := traceDBSyncSpanLanePoison{
		Producer: traceDBSyncSpanProducerSyscall, HeaderTID: 101,
		CanonicalITID: 1, CanonicalITIDKnown: true,
		Reason: traceDBSyncSpanLanePoisonRejectedSyscallCandidate,
	}
	exactProducers := []traceDBSyncSpanProducer{
		traceDBSyncSpanProducerUnknown, traceDBSyncSpanProducerRegistration, traceDBSyncSpanProducerCallstack,
		traceDBSyncSpanProducerSyscall, traceDBSyncSpanProducerAppStartup, traceDBSyncSpanProducerStaticInitialize,
		traceDBSyncSpanProducer(traceDBSyncSpanProducerStaticInitialize + 1),
	}
	exactReasons := []traceDBSyncSpanLanePoisonReason{
		traceDBSyncSpanLanePoisonUnknown, traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
		traceDBSyncSpanLanePoisonRejectedSyscallCandidate,
		traceDBSyncSpanLanePoisonReason(traceDBSyncSpanLanePoisonRejectedSyscallCandidate + 1),
	}
	for _, producer := range exactProducers {
		for _, reason := range exactReasons {
			poison := exactBase
			poison.Producer, poison.Reason = producer, reason
			accepted := validateTraceDBSyncSpanLanePoison(poison) == nil
			want := producer == traceDBSyncSpanProducerCallstack && reason == traceDBSyncSpanLanePoisonRejectedCallstackCandidate ||
				producer == traceDBSyncSpanProducerSyscall && reason == traceDBSyncSpanLanePoisonRejectedSyscallCandidate
			if accepted != want {
				t.Fatalf("exact poison producer=%d reason=%d accepted=%t want=%t", producer, reason, accepted, want)
			}
		}
	}
	invalidExact := exactBase
	invalidExact.Producer = traceDBSyncSpanProducerAppStartup
	if err := authority.poisonExactLane(ctx, invalidExact); err == nil {
		t.Fatal("exact poison method accepted a closed-enum mismatch")
	}
	if authority.poisonedTotal != 0 || authority.stage.records != 0 {
		t.Fatalf("rejected exact poison mutated authority: poisoned=%d records=%d", authority.poisonedTotal, authority.stage.records)
	}
	if err := authority.poisonExactLane(ctx, exactBase); err != nil {
		t.Fatalf("valid syscall exact poison rejected: %v", err)
	}
	if authority.poisonedTotal != 1 || authority.stage.records != 1 {
		t.Fatalf("valid exact poison not recorded once: poisoned=%d records=%d", authority.poisonedTotal, authority.stage.records)
	}

	globalReasons := []traceDBSyncSpanGlobalPoisonReason{
		traceDBSyncSpanGlobalPoisonUnknown, traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate,
		traceDBSyncSpanGlobalPoisonReason(traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate + 1),
	}
	for _, producer := range exactProducers {
		for _, reason := range globalReasons {
			poison := traceDBSyncSpanGlobalPoison{Producer: producer, Reason: reason}
			accepted := validateTraceDBSyncSpanGlobalPoison(poison) == nil
			want := producer == traceDBSyncSpanProducerSyscall && reason == traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate
			if accepted != want {
				t.Fatalf("global poison producer=%d reason=%d accepted=%t want=%t", producer, reason, accepted, want)
			}
		}
	}
	invalidGlobal := traceDBSyncSpanGlobalPoison{
		Producer: traceDBSyncSpanProducerAppStartup,
		Reason:   traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate,
	}
	if err := authority.poisonGlobally(ctx, invalidGlobal); err == nil {
		t.Fatal("global poison method accepted a closed-enum mismatch")
	}
	if authority.globalPoisonedTotal != 0 || authority.poisonedTotal != 1 || authority.stage.records != 1 {
		t.Fatalf("rejected global poison mutated authority: global=%+v poisoned=%d records=%d",
			authority.globalPoisoned, authority.poisonedTotal, authority.stage.records)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	validGlobal := traceDBSyncSpanGlobalPoison{
		Producer: traceDBSyncSpanProducerSyscall,
		Reason:   traceDBSyncSpanGlobalPoisonUnlocalizableSyscallCandidate,
	}
	if err := authority.poisonGlobally(canceled, validGlobal); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled global poison error=%v, want context.Canceled", err)
	}
	if authority.globalPoisonedTotal != 0 {
		t.Fatal("cancelled global poison mutated authority")
	}
	if err := authority.poisonGlobally(ctx, validGlobal); err != nil {
		t.Fatalf("valid global poison rejected: %v", err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if _, _, err := authority.finalize(ctx, sink); err != nil {
		t.Fatal(err)
	}
	if err := authority.poisonGlobally(ctx, validGlobal); err == nil || authority.globalPoisonedTotal != 1 {
		t.Fatalf("finalized authority accepted or mutated global poison: err=%v total=%d", err, authority.globalPoisonedTotal)
	}
}

func renderTraceDBSyncSpanAuthority(t *testing.T, candidates []traceDBSyncSpanCandidate,
	poisons []traceDBSyncSpanLanePoison, threshold int,
) (traceDBSyncSpanReport, TraceDBCoverage, string, traceDBRowSortStats) {
	t.Helper()
	authority := newTraceDBTestSyncSpanAuthority(t)
	sink, err := newTraceDBRowSink(t.TempDir(), threshold)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if err := authority.submit(context.Background(), candidate); err != nil {
			t.Fatalf("submit %+v: %v", candidate, err)
		}
	}
	for _, poison := range poisons {
		if err := authority.poisonExactLane(context.Background(), poison); err != nil {
			t.Fatalf("poison %+v: %v", poison, err)
		}
	}
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.runs) != 0 {
		t.Fatalf("Submit/Poison published before Finalize: stats=%+v rows=%d chunks=%d",
			sink.stats, len(sink.rows), len(sink.runs))
	}
	report, coverage, err := authority.finalize(context.Background(), sink)
	if err != nil {
		t.Fatalf("finalize: %v coverage=%+v", err, coverage)
	}
	var out bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	return report, coverage, out.String(), stats
}

func TestTraceDBSyncSpanAuthorityOrdersContainmentAdjacentAndZero(t *testing.T) {
	candidates := []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 3, 10, 100, 40, 50, "adjacent"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 1, 10, 100, 40, 40, "registration"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 2, 10, 100, 20, 30, "inner"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 40, "outer"),
	}
	report, coverage, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
	if report.EmittedEndpoints != 8 || report.SuppressedSpans != 0 || coverage.RowsEmitted != 8 || coverage.Skipped != "" {
		t.Fatalf("clean authority accounting mismatch: report=%+v coverage=%+v", report, coverage)
	}
	order := []string{
		"B|100|outer", "B|100|inner", "E|100|", "E|100|",
		"B|100|registration", "E|100|", "B|100|adjacent", "E|100|",
	}
	position := -1
	for _, token := range order {
		next := strings.Index(body[position+1:], token)
		if next < 0 {
			t.Fatalf("wire order missing %q after byte %d:\n%s", token, position, body)
		}
		position += next + 1
	}
}

func TestTraceDBSyncSpanAuthorityCrossingPoisonsWholePhysicalLaneOrderIndependently(t *testing.T) {
	base := []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 1, 10, 100, 5, 5, "registration"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "cross-a"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 20, 40, "cross-b"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerAppStartup, 1, 20, 100, 15, 25, "other-lane"),
	}
	var reference string
	for _, reverse := range []bool{false, true} {
		candidates := append([]traceDBSyncSpanCandidate(nil), base...)
		if reverse {
			for left, right := 0, len(candidates)-1; left < right; left, right = left+1, right-1 {
				candidates[left], candidates[right] = candidates[right], candidates[left]
			}
		}
		report, coverage, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
		if report.CrossingLanes != 1 || report.PoisonedLanes != 1 || report.SuppressedSpans != 3 ||
			report.EmittedEndpoints != 2 || !strings.Contains(coverage.Skipped, "crossing_lanes=1") ||
			strings.Contains(body, "cross-a") || strings.Contains(body, "cross-b") || strings.Contains(body, "registration") ||
			!strings.Contains(body, "other-lane") {
			t.Fatalf("crossing lane was not atomic/local: report=%+v coverage=%+v\n%s", report, coverage, body)
		}
		if reference == "" {
			reference = body
		} else if body != reference {
			t.Fatalf("physical submission order changed output:\n--- reference\n%s\n--- reverse\n%s", reference, body)
		}
	}
}

func TestTraceDBSyncSpanAuthorityIdenticalPositiveRequiresComparableDepth(t *testing.T) {
	t.Run("cross producer has no depth proof", func(t *testing.T) {
		candidates := []traceDBSyncSpanCandidate{
			traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 20, "a"),
			traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 10, 20, "b"),
		}
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
		if report.IdenticalLanes != 1 || report.SuppressedSpans != 2 || strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("unproven identical interval survived: report=%+v body=%q", report, body)
		}
	})

	t.Run("exact callstack depth proves nesting", func(t *testing.T) {
		outer := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 20, "outer")
		inner := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 10, 20, "inner")
		outer.DepthKnown, outer.Depth, outer.DepthProvenance = true, 0, traceDBSyncSpanDepthCallstack
		inner.DepthKnown, inner.Depth, inner.DepthProvenance = true, 1, traceDBSyncSpanDepthCallstack
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
			[]traceDBSyncSpanCandidate{inner, outer}, nil, 128)
		if report.EmittedEndpoints != 4 || report.SuppressedSpans != 0 ||
			strings.Index(body, "B|100|outer") > strings.Index(body, "B|100|inner") {
			t.Fatalf("exact depth nesting rejected/reordered: report=%+v\n%s", report, body)
		}
	})

	t.Run("equal exact depth is still ambiguous", func(t *testing.T) {
		left := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 20, "left")
		right := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 10, 20, "right")
		left.DepthKnown, left.DepthProvenance = true, traceDBSyncSpanDepthCallstack
		right.DepthKnown, right.DepthProvenance = true, traceDBSyncSpanDepthCallstack
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
			[]traceDBSyncSpanCandidate{left, right}, nil, 128)
		if report.IdenticalLanes != 1 || report.SuppressedSpans != 2 || strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("equal-depth identical interval survived: report=%+v body=%q", report, body)
		}
	})
}

func TestTraceDBSyncSpanAuthorityUsesHeaderTIDNotPayloadTGID(t *testing.T) {
	t.Run("same TGID different TID are different lanes", func(t *testing.T) {
		candidates := []traceDBSyncSpanCandidate{
			traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "tid10"),
			traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 20, 100, 20, 40, "tid20"),
		}
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
		if report.EmittedEndpoints != 4 || report.PoisonedLanes != 0 ||
			!strings.Contains(body, "tid10") || !strings.Contains(body, "tid20") {
			t.Fatalf("payload TGID incorrectly merged physical lanes: report=%+v\n%s", report, body)
		}
	})

	t.Run("same TID different TGID conflicts in one lane", func(t *testing.T) {
		left := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "tgid100")
		right := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 200, 15, 20, "tgid200")
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t, []traceDBSyncSpanCandidate{left, right}, nil, 128)
		if report.IdentityLanes != 1 || report.SuppressedSpans != 2 || strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("payload TGID split one physical lane: report=%+v body=%q", report, body)
		}
	})

	t.Run("overlapping owner generations conflict even when TGID matches", func(t *testing.T) {
		left := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "owner-one")
		right := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 15, 20, "owner-two")
		right.OwnerIPID = 2
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t, []traceDBSyncSpanCandidate{left, right}, nil, 128)
		if report.IdentityLanes != 1 || report.SuppressedSpans != 2 || strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("precise owner generation mismatch survived: report=%+v body=%q", report, body)
		}
	})
}

func TestTraceDBSyncSpanAuthorityChecksComparableDepthAcrossEveryOpenAncestor(t *testing.T) {
	outer := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 50, "outer")
	middle := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 20, 40, "middle")
	inner := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 25, 35, "inner")
	outer.DepthKnown, outer.Depth, outer.DepthProvenance = true, 3, traceDBSyncSpanDepthCallstack
	inner.DepthKnown, inner.Depth, inner.DepthProvenance = true, 3, traceDBSyncSpanDepthCallstack
	report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
		[]traceDBSyncSpanCandidate{inner, middle, outer}, nil, 128)
	if report.DepthLanes != 1 || report.SuppressedSpans != 3 || strings.Contains(body, "tracing_mark_write") {
		t.Fatalf("interposed producer hid exact callstack depth conflict: report=%+v body=%q", report, body)
	}
}

func TestTraceDBSyncSpanAuthorityAuditOrderIsTotalAndSubmissionIndependent(t *testing.T) {
	outer := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 3, 10, 100, 10, 20, "outer")
	inner := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 20, "inner")
	conflict := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 10, 20, "conflict")
	outer.DepthKnown, outer.Depth, outer.DepthProvenance = true, 0, traceDBSyncSpanDepthCallstack
	inner.DepthKnown, inner.Depth, inner.DepthProvenance = true, 1, traceDBSyncSpanDepthCallstack
	conflict.CanonicalITID, conflict.OwnerIPID = 20, 2
	conflict.DepthKnown, conflict.Depth, conflict.DepthProvenance = true, 2, traceDBSyncSpanDepthCallstack
	base := []traceDBSyncSpanCandidate{outer, inner, conflict}
	permutations := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	var referenceReport traceDBSyncSpanReport
	var referenceCoverage string
	for iteration, order := range permutations {
		candidates := []traceDBSyncSpanCandidate{base[order[0]], base[order[1]], base[order[2]]}
		report, coverage, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
		if report.IdentityLanes != 1 || report.PoisonedLanes != 1 || report.SuppressedSpans != 3 ||
			strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("permutation %v changed fail-close semantics: report=%+v coverage=%+v body=%q", order, report, coverage, body)
		}
		if iteration == 0 {
			referenceReport, referenceCoverage = report, coverage.Skipped
		} else if !reflect.DeepEqual(report, referenceReport) || coverage.Skipped != referenceCoverage {
			t.Fatalf("permutation %v changed deterministic audit result: got=%+v/%q want=%+v/%q",
				order, report, coverage.Skipped, referenceReport, referenceCoverage)
		}
	}
}

func TestTraceDBSyncSpanAuthorityRejectsGhostIdentityAndForgedDepthProvenance(t *testing.T) {
	base := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 10, 20, "sys_1")
	for _, test := range []struct {
		name   string
		mutate func(*traceDBSyncSpanCandidate)
		reason string
	}{
		{
			name: "missing required owner proof",
			mutate: func(candidate *traceDBSyncSpanCandidate) {
				candidate.OwnerIPIDKnown = false
				candidate.OwnerIPID = 0
			},
			reason: "sync_span_candidate_provenance_mismatch",
		},
		{
			name: "ghost owner value",
			mutate: func(candidate *traceDBSyncSpanCandidate) {
				candidate.OwnerIPIDKnown = false
			},
			reason: "unproven_sync_span_owner_ipid",
		},
		{
			name: "ghost canonical value",
			mutate: func(candidate *traceDBSyncSpanCandidate) {
				candidate.Producer = traceDBSyncSpanProducerAppStartup
				candidate.StableKind = traceDBSyncSpanStableAppStartupRowID
				candidate.Name = "AppStartup:cold"
				candidate.NameProvenance = traceDBSyncSpanNameAppStartupDictionary
				candidate.CanonicalITIDKnown = false
			},
			reason: "unproven_sync_span_canonical_itid",
		},
		{
			name: "non-callstack forged exact depth",
			mutate: func(candidate *traceDBSyncSpanCandidate) {
				candidate.DepthKnown = true
				candidate.Depth = 1
				candidate.DepthProvenance = traceDBSyncSpanDepthCallstack
			},
			reason: "sync_span_candidate_provenance_mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			authority := newTraceDBTestSyncSpanAuthority(t)
			reason, typed := traceDBOutputInvariantReason(authority.submit(context.Background(), candidate))
			if !typed || reason != test.reason {
				t.Fatalf("ghost/forged provenance reason=%q typed=%t want=%q", reason, typed, test.reason)
			}
		})
	}
}

func TestTraceDBSyncSpanAuthorityExactPoisonAndDuplicateIdentityAreLocal(t *testing.T) {
	poison := traceDBSyncSpanLanePoison{
		Producer: traceDBSyncSpanProducerCallstack, HeaderTID: 10,
		CanonicalITID: 10, CanonicalITIDKnown: true,
		Reason: traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
	}
	candidates := []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 1, 10, 100, 10, 20, "poisoned-syscall"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 2, 20, 100, 10, 20, "unrelated"),
	}
	report, _, body, _ := renderTraceDBSyncSpanAuthority(t, candidates, []traceDBSyncSpanLanePoison{poison}, 128)
	if report.PoisonedLanes != 1 || report.SuppressedSpans != 1 ||
		strings.Contains(body, "poisoned-syscall") || !strings.Contains(body, "unrelated") {
		t.Fatalf("exact poison crossed/missed its physical lane: report=%+v\n%s", report, body)
	}

	duplicate := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 7, 30, 100, 10, 20, "duplicate-a")
	duplicateOther := duplicate
	duplicateOther.Name = "duplicate-b"
	report, coverage, body, _ := renderTraceDBSyncSpanAuthority(t,
		[]traceDBSyncSpanCandidate{duplicate, duplicateOther,
			traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 8, 40, 100, 10, 20, "control")}, nil, 128)
	if report.DuplicateLanes != 1 || report.SuppressedSpans != 2 ||
		!strings.Contains(coverage.Skipped, "duplicate_stable_identity_lanes=1") ||
		strings.Contains(body, "duplicate-a") || strings.Contains(body, "duplicate-b") || !strings.Contains(body, "control") {
		t.Fatalf("duplicate stable identity was silently rescued/globalized: report=%+v coverage=%+v\n%s", report, coverage, body)
	}
}

func TestTraceDBSyncSpanAuthorityZeroSpansAreAtomicAndIdleIsTyped(t *testing.T) {
	first := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 1, 10, 100, 10, 10, "first")
	second := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 2, 10, 100, 10, 10, "second")
	idle := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 0, 0, 0, 11, 11, "idle")
	report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
		[]traceDBSyncSpanCandidate{second, idle, first}, nil, 128)
	if report.EmittedEndpoints != 6 {
		t.Fatalf("zero spans were rejected: %+v", report)
	}
	firstBegin, firstEnd := strings.Index(body, "B|100|first"), strings.Index(body, "E|100|")
	secondBegin := strings.Index(body, "B|100|second")
	if firstBegin < 0 || firstEnd <= firstBegin || secondBegin <= firstEnd {
		t.Fatalf("same-point zero spans were not emitted as stable atomic pairs:\n%s", body)
	}

	nonRegistrationIdle := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, 9, 0, 0, 1, 2, "bad-idle")
	authority := newTraceDBTestSyncSpanAuthority(t)
	if reason, ok := traceDBOutputInvariantReason(authority.submit(context.Background(), nonRegistrationIdle)); !ok || reason != "unproven_sync_span_idle_subject" {
		t.Fatalf("default zero borrowed canonical idle authority: reason=%q typed=%t", reason, ok)
	}
}

func TestTraceDBSyncSpanAuthorityZeroSpanIdentityOnlyConflictsInsideOpenInterval(t *testing.T) {
	outer := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "outer")

	t.Run("interior generation change poisons lane", func(t *testing.T) {
		point := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 20, 20, "changed-owner")
		point.CanonicalITID, point.OwnerIPID = 20, 2
		report, coverage, body, _ := renderTraceDBSyncSpanAuthority(t,
			[]traceDBSyncSpanCandidate{point, outer}, nil, 128)
		if report.IdentityLanes != 1 || report.SuppressedSpans != 2 ||
			!strings.Contains(coverage.Skipped, "identity_conflict_lanes=1") ||
			strings.Contains(body, "tracing_mark_write") {
			t.Fatalf("interior zero-span identity change survived: report=%+v coverage=%+v body=%q", report, coverage, body)
		}
	})

	t.Run("same identity interior point remains atomic", func(t *testing.T) {
		point := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 100, 20, 20, "same-owner")
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
			[]traceDBSyncSpanCandidate{point, outer}, nil, 128)
		outerBegin := strings.Index(body, "B|100|outer")
		pointBegin := strings.Index(body, "B|100|same-owner")
		pointEnd := -1
		if pointBegin >= 0 {
			if relative := strings.Index(body[pointBegin:], "E|100|"); relative >= 0 {
				pointEnd = pointBegin + relative
			}
		}
		outerEnd := strings.LastIndex(body, "E|100|")
		if report.EmittedEndpoints != 4 || report.PoisonedLanes != 0 ||
			outerBegin < 0 || pointBegin <= outerBegin || pointEnd <= pointBegin || outerEnd <= pointEnd {
			t.Fatalf("same-identity interior zero span rejected: report=%+v\n%s", report, body)
		}
	})

	t.Run("different identity at exact boundaries is outside stack", func(t *testing.T) {
		atStart := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 2, 10, 200, 10, 10, "before-begin")
		atEnd := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 3, 10, 200, 30, 30, "after-end")
		atStart.CanonicalITID, atStart.OwnerIPID = 20, 2
		atEnd.CanonicalITID, atEnd.OwnerIPID = 20, 2
		report, _, body, _ := renderTraceDBSyncSpanAuthority(t,
			[]traceDBSyncSpanCandidate{atEnd, outer, atStart}, nil, 128)
		startPoint := strings.Index(body, "B|200|before-begin")
		outerBegin := strings.Index(body, "B|100|outer")
		endPoint := strings.Index(body, "B|200|after-end")
		outerEnd := -1
		if endPoint >= 0 {
			outerEnd = strings.LastIndex(body[:endPoint], "E|100|")
		}
		if report.EmittedEndpoints != 6 || report.PoisonedLanes != 0 ||
			startPoint < 0 || outerBegin <= startPoint || outerEnd < outerBegin || endPoint <= outerEnd {
			t.Fatalf("boundary zero-span phase/identity semantics drifted: report=%+v\n%s", report, body)
		}
	})
}

func TestTraceDBSyncSpanAuthorityStateCoverageAndSpillParity(t *testing.T) {
	candidates := []traceDBSyncSpanCandidate{
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerCallstack, 1, 10, 100, 10, 30, "outer"),
		traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSyscall, -1, 10, 100, 15, 20, "inner"),
	}
	largeReport, largeCoverage, largeBody, largeStats := renderTraceDBSyncSpanAuthority(t, candidates, nil, 128)
	smallReport, smallCoverage, smallBody, smallStats := renderTraceDBSyncSpanAuthority(t, candidates, nil, 1)
	if largeBody != smallBody || !reflect.DeepEqual(largeReport, smallReport) ||
		largeCoverage.RowsEmitted != smallCoverage.RowsEmitted || largeStats.SpillChunks != 1 || smallStats.SpillChunks <= 1 {
		t.Fatalf("row-sink spill threshold changed authority result: large=%+v/%+v small=%+v/%+v",
			largeReport, largeStats, smallReport, smallStats)
	}
	if !strings.Contains(largeCoverage.FieldSources["buffering"], "candidate-byte-bounded") ||
		!strings.Contains(largeCoverage.FieldSources["buffering"], "final generic row sorter bounded") ||
		strings.Contains(largeCoverage.FieldSources["buffering"], "remains separately open") || largeCoverage.SpillChunks != 0 {
		t.Fatalf("B1-c bounded stage / generic sorter boundary drifted: %+v", largeCoverage)
	}

	authority := newTraceDBTestSyncSpanAuthority(t)
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.submit(context.Background(), candidates[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := authority.finalize(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if reason, ok := traceDBOutputInvariantReason(authority.submit(context.Background(), candidates[1])); !ok || reason != "sync_span_authority_not_open" {
		t.Fatalf("late Submit did not fail loud: reason=%q typed=%t", reason, ok)
	}
	if _, _, err := authority.finalize(context.Background(), sink); err == nil {
		t.Fatal("double Finalize did not fail loud")
	}
}
