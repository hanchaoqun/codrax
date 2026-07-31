package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// WO-G1 (SMR-1 批, smr_audit_report §② SMR-S12a, 2026-07-12):
//   - 91869 witness: the SAME thread seated TWO ◌ rows — the tree chain-stop
//     (root_evidence lane, kind lost → false 「窗内无调度数据·链止」 beside a
//     valued row) AND the ◇ stanza copy (rank lane, true 「窗内无≥阈值等待
//     区间·链止」). One expandChain mint = one fact = one seat.
//   - 52774 witness: the false no_sched_data wording on the chain-stop row.
//
// Mechanism: G2 双发布去重扩臂 (the existing arm covers ◇席 vs 溢出折叠;
// this adds 树链止 vs ◇席) + the typed TraceGapKind now rides the
// root_evidence lane emission (same note key as the rank lane — no second
// signal).

func smr1G1DoubleGapProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target-1"},
		WindowStartTs: 1000.000,
		WindowEndTs:   1000.002,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// A valued chain row right beside the gap marker (the 52774 shape:
			// the gap wording must not claim "no scheduler data" next to it).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#wakeup_causal_impact:1",
				Subject: "coldpool-6", Predicate: "wakeup_causal_impact", Object: "running",
				StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 0.966, CumulativeImpactMS: 0.966,
				Confidence: 0.78, LineStart: 50, LineEnd: 60,
				QueryWindowStartTs: 1000.000, QueryWindowEndTs: 1000.002},
			// The chain-stop ◌ (root_evidence lane) — with the typed kind now
			// propagated (WO-G1 emission half).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:1",
				Subject: "coldpool-6", Predicate: "trace_gap", Object: "",
				Tier: types.TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				TraceGapKind: tracequery.TraceGapKindNoEligibleWait,
				Confidence:   0.6, LineStart: 70, LineEnd: 70,
				QueryWindowStartTs: 1000.000, QueryWindowEndTs: 1000.002},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			// The ◇ stanza copy of the SAME gap fact (rank lane, kind present).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#root_cause:gap",
				Subject: "coldpool-6", Predicate: "root_cause_context", Object: "trace_gap",
				TypeToken: "trace_gap", Tier: "data_gap", ChainRelevance: "adjacent",
				TraceGapKind: tracequery.TraceGapKindNoEligibleWait,
				Confidence:   0.6, LineStart: 71, LineEnd: 71,
				QueryWindowStartTs: 1000.000, QueryWindowEndTs: 1000.002},
		},
	}
}

func smr1CountGapSeats(model runtimeTraceProjTreeModel, subject string) int {
	count := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.HasData && runtimeTraceProjTraceGapNode(row.Node) &&
				runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) ==
					runtimeTraceCausalProjectionCanonicalNode(subject) {
				count++
			}
		}
	}
	return count
}

// 91869 pin: the same thread's compatible-window trace_gap fact holds exactly
// ONE ◌ seat, and the merged seat speaks the ONE true criterion word.
func TestSMR1G1DoubleGapMarkerConvergesToOneSeat(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1G1DoubleGapProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if got := smr1CountGapSeats(model, "coldpool-6"); got != 1 {
		t.Fatalf("one trace_gap fact must hold exactly one ◌ seat (树链止 vs ◇席 dedup), got %d", got)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "窗内无≥阈值等待区间·链止") {
		t.Fatalf("the surviving seat must speak the typed true criterion:\n%s", fence)
	}
	if strings.Contains(fence, "窗内无调度数据·链止") {
		t.Fatalf("52774 假词面: the false no_sched_data wording must not render when the typed kind says no_eligible_wait:\n%s", fence)
	}
	// 禁令 (vnote 修正): the merged seat carries ONE reason — the two words are
	// mutually exclusive faces of a closed enum, never a dual-reason line.
	if strings.Count(fence, "·链止") != 1 {
		t.Fatalf("the merged ◌ seat must state exactly one 链止 criterion:\n%s", fence)
	}
}

// Distinct query windows are two facts — both seats stay (fail-open guard).
func TestSMR1G1DistinctWindowGapSeatsStay(t *testing.T) {
	projection := smr1G1DoubleGapProjection()
	projection.AdjacentCauses[0].QueryWindowStartTs = 1000.500
	projection.AdjacentCauses[0].QueryWindowEndTs = 1000.502
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if got := smr1CountGapSeats(model, "coldpool-6"); got != 2 {
		t.Fatalf("distinct query windows are two facts and keep two seats, got %d", got)
	}
}

// WO-G1 emission half: the root_evidence trace_gap observation carries the
// typed kind note (the chain lane stops dropping the engine's criterion).
func TestSMR1G1RootEvidenceCarriesTraceGapKind(t *testing.T) {
	result := tracequery.Result{
		WakeupChain: &tracequery.ChainResult{
			RootEvidence: []tracequery.RootEvidence{{
				Type:      "trace_gap",
				Thread:    tracequery.ThreadRef{PID: 36644, Comm: "coldpool-6"},
				Summary:   "scheduler intervals exist in the aligned window but all sit below min_duration_ms (no eligible wait candidate)",
				LineStart: 70, LineEnd: 70,
				GapKind:    tracequery.TraceGapKindNoEligibleWait,
				Confidence: 0.6,
			}},
		},
	}
	records := traceQueryTypedObservations(result, "t.systrace", "payload", "raw", "t", time.Now())
	found := false
	for _, record := range records {
		if !strings.Contains(record.ID, "#root_evidence:") {
			continue
		}
		found = true
		note := types.TraceNoteKeyTraceGapKind + "=" + tracequery.TraceGapKindNoEligibleWait
		if !strings.Contains(strings.Join(record.RichNotes, "\n"), note) {
			t.Fatalf("root_evidence trace_gap record must carry the typed kind note %q, got %v", note, record.RichNotes)
		}
		if record.ProvenanceLane != types.ObservationProvenanceArtifactSpan {
			t.Fatalf("root_evidence trace_gap is a coverage boundary, not a direct cause: %+v", record)
		}
	}
	if !found {
		t.Fatalf("expected a root_evidence observation, got %d records", len(records))
	}
}

// P1-1 (SMR-1 修复轮, 2026-07-13; 冷读 40422 回退形 + 8411/64414 折叠面饿死):
// the critical_blocking and root_cause_rank emissions carry the row's own
// typed wall-clock segment on the ObservationSpan — the display wall-clock
// gates (NEW-3 / G2 / B1) starve without it (行区间 alone must never fold:
// the negative arm is TestSMR1N1RowNumberEnvelopeAloneNeverFolds).
func TestSMR1P11EmissionsCarryWallClockSpan(t *testing.T) {
	result := tracequery.Result{
		CriticalBlocking: &tracequery.CriticalBlockingResult{
			Items: []tracequery.CriticalBlockingCandidate{{
				Type: "io_latency", Thread: tracequery.ThreadRef{PID: 60555, Comm: "threadpoolforeg"},
				DurationMs: 4.262, LineStart: 8806, LineEnd: 9070,
				StartTs: 34579.526000, EndTs: 34579.530262, Confidence: 0.8,
			}},
		},
	}
	records := traceQueryTypedObservations(result, "t.systrace", "payload", "raw", "t", time.Now())
	found := false
	for _, record := range records {
		if !strings.Contains(record.ID, "#critical_blocking:") {
			continue
		}
		found = true
		if record.Span.StartTs != 34579.526000 || record.Span.EndTs != 34579.530262 {
			t.Fatalf("critical_blocking span must carry the typed segment, got %+v", record.Span)
		}
	}
	if !found {
		t.Fatalf("expected a critical_blocking observation")
	}
}
