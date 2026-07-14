package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CR-2 组② P5 同段多车道收敛门 (ledger §29.42 P5; witnesses: 14704 双席
// 20260712-131531 E1/E2 — one running segment 54.599ms/55% published as a
// candidate-lane row AND a bare raw-state (root_evidence, context_only) row;
// donghu 20260712-133933 E8/E9 — the critical_blocking ×4 twin of the D-state
// family row re-publishing the same segments with a fake 「单次 3.774~16.064ms」
// per-instance claim (F-1 残口移交)。 Display-layer forerunner of the v5 P1
// engine one-seat-per-segment batch (EVOLUTION RECORD in the fold lane): the
// engine wire is untouched — fingerprint-equal rows converge at render.

// cr2P5EqualityArmRecords is the 14704 archetype at the PRODUCTION emission
// level (v5 P1 件① retirement form): the candidate wakeup_causal_impact
// record and its bare root_evidence copy — same subject, same value, same
// exact line span. The display equality arm is retired; the ENGINE one-seat
// mint (R1 + arm A) converges the pair before any render layer sees it.
func cr2P5EqualityArmRecords(rawValue string) []types.ObservationRecord {
	return []types.ObservationRecord{
		{
			ID:              "trace_query:t#wakeup_causal_impact:9",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Span:            types.ObservationSpan{LineStart: 50, LineEnd: 60},
			ClaimKey:        "wakeup_causal_impact:waker-1",
			Subject:         "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
			Value: "20.000", Unit: "ms",
			RichNotes:  []string{"causality=on_wakeup_chain", "dominant_state=s_sleep"},
			Confidence: 0.78,
		},
		{
			ID:              "trace_query:t#wakeup_causal_impact:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Span:            types.ObservationSpan{LineStart: 101, LineEnd: 101},
			ClaimKey:        "wakeup_causal_impact:coldpool-1",
			Subject:         "coldpool-1", Predicate: "wakeup_causal_impact", Object: "running",
			Value: "54.599", Unit: "ms",
			RichNotes:  []string{"causality=on_wakeup_chain", "dominant_state=running"},
			Confidence: 0.78,
		},
		{
			ID:              "trace_query:t#root_evidence:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Span:            types.ObservationSpan{LineStart: 101, LineEnd: 101},
			ClaimKey:        "root_evidence:running",
			Subject:         "coldpool-1", Predicate: "running",
			Value: rawValue, Unit: "ms",
			RichNotes:  []string{"tier=context_only", "effective_impact_ms=0.000", "dominant_state=running"},
			Confidence: 0.75,
		},
	}
}

// 14704 形 pin — 退役等价证明 (v5 P1 件①, 2026-07-13): with the display
// equality arm retired, the ENGINE-converged pair renders exactly one row
// carrying the highest-lane word and the design-sanctioned merged_ids face
// ([E#(+N)], B.2 「peer 只并 E#」) — the interim 同段镜像已并入 tag is gone
// with its arm.
func TestCR2P5SameSegmentEqualityConvergesAtEngine(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(cr2P5EqualityArmRecords("54.599"))
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var seats []runtimeTraceProjTreeRow
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if !row.HasData {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) ==
				runtimeTraceCausalProjectionCanonicalNode("coldpool-1") {
				seats = append(seats, row)
			}
		}
	}
	if len(seats) != 1 {
		t.Fatalf("one physical segment must render exactly one row (B.2 一段一席), got %d", len(seats))
	}
	row := seats[0]
	if row.Node.StateKind != "running" {
		t.Fatalf("the surviving row must keep the highest-lane word: %+v", row.Node)
	}
	ref := runtimeTraceProjCauseEvidenceRef(row)
	if !strings.Contains(ref, "(+") {
		t.Fatalf("the raw copy's E# must ride the merged_ids bracket: %q", ref)
	}
}

// 件3 (修复轮, 对抗复核 F4, 2026-07-13) — the arm-A EXCLUSIVE carriers'
// display equivalence: the VALUELESS raw copy and the eff-lane-valued keeper
// shapes (both structurally unreachable by R1's value-keyed merge) also
// render exactly one seat with the merged_ids bracket. Detaching arm A from
// the aggregation order reds BOTH variants (red-green 实录 in the batch
// report; the valued variant above stays green under R1 alone).
func TestCR2P5SameSegmentEqualityArmAVariantsConvergeAtEngine(t *testing.T) {
	countSeats := func(projection types.TraceCausalProjection) (int, []runtimeTraceProjTreeRow) {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		var seats []runtimeTraceProjTreeRow
		for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
			for _, row := range rows {
				if row.HasData && runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) ==
					runtimeTraceCausalProjectionCanonicalNode("coldpool-1") {
					seats = append(seats, row)
				}
			}
		}
		return len(seats), seats
	}
	// Variant 1: valueless raw copy.
	if n, seats := countSeats(types.TraceCausalProjectionFromObservationRecords(
		cr2P5EqualityArmRecords("0.000"))); n != 1 {
		t.Fatalf("valueless raw twin must converge to one rendered seat (arm A), got %d", n)
	} else if ref := runtimeTraceProjCauseEvidenceRef(seats[0]); !strings.Contains(ref, "(+") {
		t.Fatalf("the valueless raw's E# must ride the merged_ids bracket: %q", ref)
	}
	// Variant 2: keeper valued on the effective lane only.
	records := cr2P5EqualityArmRecords("54.599")
	records[1].Value = "0.000"
	records[1].RichNotes = []string{"causality=on_wakeup_chain", "dominant_state=running", "effective_impact_ms=54.599"}
	if n, _ := countSeats(types.TraceCausalProjectionFromObservationRecords(records)); n != 1 {
		t.Fatalf("eff-lane-valued keeper must absorb its raw twin (arm A), got %d seats", n)
	}
}

// Fail-open control — 退役等价证明: a differing value is a different account;
// the engine keeps two seats and the display renders both (宁两行勿假并).
func TestCR2P5SameSegmentEqualityFailsOpenOnValueMismatch(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(cr2P5EqualityArmRecords("51.000"))
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.HasData && runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) ==
				runtimeTraceCausalProjectionCanonicalNode("coldpool-1") {
				count++
			}
		}
	}
	if count != 2 {
		t.Fatalf("value-mismatched lanes are different accounts and must stay two rows, got %d", count)
	}
}

// cr2P5CensusMirrorProjection is the legend-census fixture for the 同段镜像
// mark since the equality arm's retirement (v5 P1 件①): the MEMBER arm's
// LEGACY-EXCLUSIVE carrier (修复轮 件4, 对抗复核 F5) — raw re-issues whose
// Predicate is an UNREGISTERED causal token (d_state_or_io_wait, legacy
// records without the dominant_state note) folding into a cross-refinement
// io keeper. The engine arm B provably cannot fold this shape (no registered
// state word on the raw + token mismatch across the refinement — the
// registry lookup this display arm performs is the only family proof), so
// the fixture is production-real FOR THE DISPLAY ARM: a registered-word raw
// (the pre-件4 fixture form) would already converge at the engine and never
// reach this layer. Values are probe-aligned (3次(10.000~30.000ms)).
func cr2P5CensusMirrorProjection() types.TraceCausalProjection {
	self := "census-thread-1"
	raw := func(id string, ms float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:" + id,
			Subject: self, Predicate: "d_state_or_io_wait",
			Tier: types.TraceCausalTierContextOnly, ChainRelevance: "on_chain",
			ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.75,
			LineStart: line, LineEnd: line + 40,
		}
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", self},
		WindowStartTs: 1000.000,
		WindowEndTs:   1000.100,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#state_drilldown:1",
				Subject: self, Object: "io_wait", TypeToken: "io_wait", StateKind: "io_wait",
				ChainRelevance: "on_chain",
				ImpactMS:       60.000, CumulativeImpactMS: 60.000,
				MergedCount: 3, MergedMinMS: 10.000, MergedMaxMS: 30.000, MergedSumMS: 60.000,
				Confidence: 0.9, LineStart: 100, LineEnd: 900},
			raw("1", 10.000, 200),
			raw("2", 20.000, 400),
			raw("3", 30.000, 600),
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#wakeup_causal_impact:5",
				Subject: "waker-1", Predicate: "wakeup_causal_impact",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 13.898, CumulativeImpactMS: 13.898,
				Confidence: 0.8, LineStart: 950, LineEnd: 980},
		},
	}
}

func cr2P5FamilyMirrorProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target-9"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E8 shape: the D/IO family row with CAL-1 segment truth (true
			// single-segment extrema; roster carries per-CPU group sums).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-family",
				Subject: "compthread-1", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
				StateKind: "d_sleep", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
				ImpactMS: 36.757, CumulativeImpactMS: 36.757, EffectiveImpactMS: 36.757,
				FamilyMemberCount: 4, FamilyMemberMinMS: 2.197, FamilyMemberMaxMS: 3.853,
				FamilyFoldCaliber: "sum_disjoint",
				ChainRelevance:    "on_chain", Confidence: 0.82, LineStart: 2260, LineEnd: 25862},
			// E9 shape: the critical_blocking ×4 twin of the SAME segments —
			// its Merged min–max are the per-CPU group sums, not segments.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-twin",
				Subject: "compthread-1", Predicate: "critical_blocking", Object: "d_state_or_io_wait",
				Tier: "", ChainRelevance: "on_chain", CumulativeImpactMS: 36.757, ImpactMS: 36.757,
				MergedCount: 4, MergedMinMS: 3.774, MergedMaxMS: 16.064,
				Confidence: 0.80, LineStart: 3157, LineEnd: 21524},
		},
	}
}

// donghu E8/E9 形 pin (F-1 残口): the merged critical_blocking twin of a family
// row wears the typed mirror mark, and its per-instance range speaks the
// group-sum truth (段 inventory 传播) instead of the fake 「单次 a~b」.
func TestCR2P5FamilyMirrorTwinRangeHonesty(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(cr2P5FamilyMirrorProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "单次 3.774~16.064ms") {
		t.Fatalf("the twin's per-instance claim is per-CPU group sums, not single segments — 「单次」 must not render:\n%s", detail)
	}
	if !strings.Contains(detail, "各成员 3.774~16.064ms(按CPU分组合计,非单段") {
		t.Fatalf("the twin must disclose the group-sum caliber of its member range:\n%s", detail)
	}
	if !strings.Contains(detail, "单段真值 2.197~3.853ms") {
		t.Fatalf("the family row's segment truth must propagate to the twin lane:\n%s", detail)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段镜像·与家族行同源") {
		t.Fatalf("the twin's 行2 must wear the family-mirror tag:\n%s", fence)
	}
}

// Fail-open control: a member-count mismatch is NOT the fingerprint — the twin
// keeps its own legacy wording and no mirror mark appears.
func TestCR2P5FamilyMirrorFailsOpenOnCountMismatch(t *testing.T) {
	projection := cr2P5FamilyMirrorProjection()
	projection.OnChainCauses[1].MergedCount = 3
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "同段镜像") || strings.Contains(detail, "各成员 3.774") {
		t.Fatalf("count-mismatched rows must fail open to the legacy render:\n%s", detail)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "同段镜像") {
		t.Fatalf("count-mismatched rows must not wear the mirror tag:\n%s", fence)
	}
}

// --- CR-2 修复轮 C-2 (冷读 A1/A2, 2026-07-12) --------------------------------

// A1 形 pin (tieba E6/E18): an un-merged aggregate-lane row whose typed
// fingerprint (subject + state + µs-equal display AND cumulative + same query
// window) matches exactly one ×N candidate row wears the value-mirror tag —
// the two 23.748ms rows stop reading additively. Values/accounts untouched.
func TestCR2FixValueMirrorTagsAggregateTwin(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "tieba-1"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E6 shape: the ×5 merged candidate row.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-cand",
				Subject: "cookiemonster-1", Object: "runnable_wait", TypeToken: "runnable_wait",
				StateKind: "runnable", Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
				ImpactMS: 23.748, CumulativeImpactMS: 23.748, EffectiveImpactMS: 8.307,
				MergedCount: 5, MergedMinMS: 1.661, MergedMaxMS: 8.307,
				QueryWindowStartTs: 34579.473, QueryWindowEndTs: 34579.572,
				ChainRelevance: "on_chain", Confidence: 0.78, LineStart: 7705, LineEnd: 8547},
			// E18 shape: the aggregate reference row — same five physical
			// segments, different line span, its own eff caliber (全额).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-agg",
				Subject: "cookiemonster-1", Object: "runnable_wait",
				StateKind: "runnable", Predicate: "wakeup_causal_aggregate",
				ImpactMS: 23.748, CumulativeImpactMS: 23.748, EffectiveImpactMS: 23.748,
				QueryWindowStartTs: 34579.473, QueryWindowEndTs: 34579.572,
				ChainRelevance: "on_chain", Confidence: 0.80, LineStart: 2892, LineEnd: 13060},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段镜像·与[") || !strings.Contains(fence, "]同一物理时间,不可相加") {
		t.Fatalf("the aggregate twin must wear the value-mirror tag:\n%s", fence)
	}
	// Both rows keep their own values (mark only, never a merge).
	if strings.Count(fence, "23.748ms") < 2 {
		t.Fatalf("both rows keep rendering their own value:\n%s", fence)
	}
}

// A1 fail-open control: a display-value mismatch (different physical time) is
// not the fingerprint — no tag.
func TestCR2FixValueMirrorFailsOpenOnValueMismatch(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "tieba-1"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-cand",
				Subject: "cookiemonster-1", Object: "runnable_wait", TypeToken: "runnable_wait",
				StateKind: "runnable", Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
				ImpactMS: 23.748, CumulativeImpactMS: 23.748, EffectiveImpactMS: 8.307,
				MergedCount: 5, MergedMinMS: 1.661, MergedMaxMS: 8.307,
				ChainRelevance: "on_chain", Confidence: 0.78, LineStart: 7705, LineEnd: 8547},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-agg",
				Subject: "cookiemonster-1", Object: "runnable_wait",
				StateKind: "runnable", Predicate: "wakeup_causal_aggregate",
				ImpactMS: 20.342, CumulativeImpactMS: 20.342, EffectiveImpactMS: 20.342,
				ChainRelevance: "on_chain", Confidence: 0.80, LineStart: 2892, LineEnd: 13060},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(model, true); strings.Contains(fence, "同一物理时间") {
		t.Fatalf("value-mismatched lanes must fail open:\n%s", fence)
	}
}

// A2 形 pin (donghu E13/E23, 冷读 A2) — DELIBERATE fail-open, recorded: the
// rank row and the merged chain row share the exact evidence line span, but
// their CUMULATIVE accounts diverge (4.710 enclosing-chain scope vs 2.181 own
// scope) — the pinned W-A rule (RNB 收尾 2026-07-07, cmp_01 E7/E8 precedent:
// 「不同账目绝不折」) refuses the fold: merging would delete or misattribute
// the enclosing-scope account. The two-row render is the honest terminal form
// until the v5 P1 engine one-seat-per-segment batch re-mints the lanes.
func TestCR2FixRankChainCumDivergenceStaysTwoRows(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"keva-1-17437", "aweme-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E13 shape: merged chain row (runnable + running views).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-chain",
				Subject: "keva-1-17437", Object: "runnable", StateKind: "runnable",
				Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
				Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 2.181, CumulativeImpactMS: 2.181, EffectiveImpactMS: 3.399,
				MergedCount: 2, MergedMinMS: 1.218, MergedMaxMS: 2.181,
				PriorityInversionCandidate: true,
				Confidence:                 0.78, LineStart: 20817, LineEnd: 21412},
			// E23 shape: the rank row — same line span, enclosing-scope cum.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-rank",
				Subject: "keva-1-17437", Object: "runnable_wait", TypeToken: "runnable_wait",
				StateKind: "runnable", Predicate: "root_cause_secondary", Rank: 5, Tier: "secondary",
				ImpactMS: 3.399, CumulativeImpactMS: 4.710, EffectiveImpactMS: 3.399,
				PriorityInversionCandidate: true,
				ChainRelevance:             "on_chain", Confidence: 0.91, LineStart: 20817, LineEnd: 21412},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.HasData && runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) ==
				runtimeTraceCausalProjectionCanonicalNode("keva-1-17437") {
				count++
			}
		}
	}
	if count != 2 {
		t.Fatalf("diverging cumulative accounts must stay two rows (W-A 不同账目绝不折), got %d", count)
	}
}

// D1 形 pin (冷读 donghu r2 「证据 E39」悬空, 2026-07-12): a semantic span the
// model walk never rendered must NOT mint a fresh E# on the optimization
// table (the printed evidence index would end before it) — the cell falls
// back to the span's own inline trace locator.
func TestCR2FixOptimizationTableNeverDanglesEvidence(t *testing.T) {
	span := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-jit",
		Subject: "jitpool-1", Object: "jit_compilation", TypeToken: "jit_compilation",
		Predicate: "trace_semantic_span", SemanticClass: "jit_compilation",
		SpanName: "JIT compiling something", EffectiveImpactMS: 2.388,
		CumulativeImpactMS: 2.388, ImpactMS: 2.388,
		ChainRelevance: "background", Confidence: 0.8, LineStart: 5968, LineEnd: 6112,
	}
	projection := types.TraceCausalProjection{SemanticSpans: []types.TraceCausalProjectionNode{span}}
	// The walk-rebuilt index deliberately does NOT contain the span (the r2
	// witness shape: the tree rendered no seat for it).
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	_, rows := runtimeTraceSemanticOptimizationParts(projection, evidence, 0, true)
	if len(rows) == 0 {
		t.Fatalf("the optimization table must render the span row")
	}
	tagCell := rows[0].Cells[len(rows[0].Cells)-1]
	if strings.HasPrefix(tagCell, "E") {
		t.Fatalf("an un-walked span must not mint a fresh E# (dangling pointer): %q", tagCell)
	}
	if !strings.Contains(tagCell, "行 5968–6112") {
		t.Fatalf("the cell must carry the inline trace locator: %q", tagCell)
	}
	// Control: a walk-allocated span keeps its E#.
	walked := newRuntimeTraceCausalProjectionEvidenceIndex()
	walked.add(span, true)
	_, rows = runtimeTraceSemanticOptimizationParts(projection, walked, 0, true)
	if tag := rows[0].Cells[len(rows[0].Cells)-1]; tag != "E1" {
		t.Fatalf("a walk-allocated span keeps its E#: %q", tag)
	}
}
