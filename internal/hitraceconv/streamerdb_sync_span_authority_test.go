package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
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
		candidate.StartCPUProvenance = traceDBSyncSpanCPULegacyUnverified
		candidate.EndCPUProvenance = traceDBSyncSpanCPULegacyUnverified
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
		if err := authority.submit(candidate); err != nil {
			t.Fatalf("submit %+v: %v", candidate, err)
		}
	}
	for _, poison := range poisons {
		if err := authority.poisonExactLane(poison); err != nil {
			t.Fatalf("poison %+v: %v", poison, err)
		}
	}
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.chunks) != 0 {
		t.Fatalf("Submit/Poison published before Finalize: stats=%+v rows=%d chunks=%d",
			sink.stats, len(sink.rows), len(sink.chunks))
	}
	report, coverage, err := authority.finalize(context.Background(), sink)
	if err != nil {
		t.Fatalf("finalize: %v coverage=%+v", err, coverage)
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
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
			reason, typed := traceDBOutputInvariantReason(authority.submit(candidate))
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
	if reason, ok := traceDBOutputInvariantReason(authority.submit(nonRegistrationIdle)); !ok || reason != "unproven_sync_span_idle_subject" {
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
		largeCoverage.RowsEmitted != smallCoverage.RowsEmitted || largeStats.SpillChunks != 0 || smallStats.SpillChunks == 0 {
		t.Fatalf("row-sink spill threshold changed authority result: large=%+v/%+v small=%+v/%+v",
			largeReport, largeStats, smallReport, smallStats)
	}
	if !strings.Contains(largeCoverage.FieldSources["buffering"], "B1-c") || largeCoverage.SpillChunks != 0 {
		t.Fatalf("B1-b overclaimed bounded staging: %+v", largeCoverage)
	}

	authority := newTraceDBTestSyncSpanAuthority(t)
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.submit(candidates[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := authority.finalize(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if reason, ok := traceDBOutputInvariantReason(authority.submit(candidates[1])); !ok || reason != "sync_span_authority_not_open" {
		t.Fatalf("late Submit did not fail loud: reason=%q typed=%t", reason, ok)
	}
	if _, _, err := authority.finalize(context.Background(), sink); err == nil {
		t.Fatal("double Finalize did not fail loud")
	}
}
