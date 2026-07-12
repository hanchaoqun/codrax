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
// family row re-publishing the same segments with a fake 「单次 3.774–16.064ms」
// per-instance claim (F-1 残口移交)。 Display-layer forerunner of the v5 P1
// engine one-seat-per-segment batch (EVOLUTION RECORD in the fold lane): the
// engine wire is untouched — fingerprint-equal rows converge at render.

func cr2P5EqualityArmProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "coldpool-1"},
		WindowStartTs: 2942.200,
		WindowEndTs:   2942.300,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// A plain data-bearing chain row so the fence draws its bar scale
			// (keeps the legend-bidirectional fixture representative).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#wakeup_causal_impact:9",
				Subject: "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 20.000, CumulativeImpactMS: 20.000,
				Confidence: 0.78, LineStart: 50, LineEnd: 60},
			// Candidate lane: the richer word (state kind + drill identity).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#wakeup_causal_impact:1",
				Subject: "coldpool-1", Predicate: "wakeup_causal_impact", Object: "running",
				StateKind: "running", Tier: types.TraceCausalTierContextOnly,
				ChainRelevance: "on_chain", ImpactMS: 54.599, CumulativeImpactMS: 54.599,
				Confidence: 0.78, LineStart: 101, LineEnd: 101},
			// Raw-state lane: the bare root_evidence copy of the SAME segment
			// (same subject, same exact line span, same value to the µs).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:1",
				Subject: "coldpool-1", Predicate: "running", Object: "",
				Tier: types.TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				ImpactMS: 54.599, Confidence: 0.75, LineStart: 101, LineEnd: 101},
		},
	}
}

// 14704 形 pin: fingerprint-equal candidate/raw-state rows converge to ONE row;
// the surviving row keeps the highest-lane word, merges the mirror's E# into
// its bracket, and wears the typed mirror tag on 行2.
func TestCR2P5SameSegmentContextMirrorMerges(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(cr2P5EqualityArmProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
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
		t.Fatalf("one physical segment must render exactly one row (P5 一段一席), got %d", len(seats))
	}
	row := seats[0]
	if row.Node.StateKind != "running" {
		t.Fatalf("the surviving row must be the candidate lane (highest word position): %+v", row.Node)
	}
	if len(row.SameSegMirrorPeers) != 1 {
		t.Fatalf("the raw-state copy must ride the mirror peer carrier: %+v", row)
	}
	ref := runtimeTraceProjCauseEvidenceRef(row)
	if !strings.Contains(ref, "+") {
		t.Fatalf("the mirror row's E# must merge into the surviving bracket: %q", ref)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段镜像已并入") {
		t.Fatalf("行2 must wear the typed mirror tag:\n%s", fence)
	}
}

// Fail-open control: a differing value (not the µs-equal fingerprint) keeps the
// two-row render byte-for-byte — the gate never merges on subject alone.
func TestCR2P5SameSegmentContextMirrorFailsOpenOnValueMismatch(t *testing.T) {
	projection := cr2P5EqualityArmProjection()
	projection.OnChainCauses[1].ImpactMS = 51.000
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
// group-sum truth (段 inventory 传播) instead of the fake 「单次 a–b」.
func TestCR2P5FamilyMirrorTwinRangeHonesty(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(cr2P5FamilyMirrorProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "单次 3.774–16.064ms") {
		t.Fatalf("the twin's per-instance claim is per-CPU group sums, not single segments — 「单次」 must not render:\n%s", detail)
	}
	if !strings.Contains(detail, "各成员 3.774–16.064ms(按CPU分组合计,非单段") {
		t.Fatalf("the twin must disclose the group-sum caliber of its member range:\n%s", detail)
	}
	if !strings.Contains(detail, "单段真值 2.197–3.853ms") {
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
	_, rows := runtimeTraceSemanticOptimizationParts(projection, evidence, true)
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
	_, rows = runtimeTraceSemanticOptimizationParts(projection, walked, true)
	if tag := rows[0].Cells[len(rows[0].Cells)-1]; tag != "E1" {
		t.Fatalf("a walk-allocated span keeps its E#: %q", tag)
	}
}
