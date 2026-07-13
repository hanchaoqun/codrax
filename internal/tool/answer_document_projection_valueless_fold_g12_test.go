package tool

// answer_document_projection_valueless_fold_g12_test.go — G12-ENG display
// pins (§29.1, real_trace_campaign_20260705.md, 2026-07-09).
//
// The huadong_79_01 E23 witness: an on-chain overflow fold of ONE real
// 14.272ms member (the target's ×5 binder-wait aggregate) plus ONE
// zero-duration member (hmfs_discard's ×4 blocked_reason marker aggregate)
// rendered "2线程取最大(单项14.272~14.272ms)" — the min–max ranged over positive
// displays only while ×N counted every member, fabricating a second 14.272ms
// observation under the valueless member's subject. The customer read it as
// same-segment double attribution and audited the raw trace (g12_report.txt).
//
// MUTATION self-check (修根臂关闭→伪双计回归, mandated by the batch order):
// TestG12MixedFoldMutationRevert zeroes the typed MergedValuelessCount on the
// SAME node and asserts the legacy fabricated form comes back — reverting the
// accounting arm cannot pass this file.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// g12MixedFoldNode is the E23-shaped fold row: two members, one valued at
// 14.272ms, one valueless (typed MergedValuelessCount=1).
func g12MixedFoldNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:                 types.TraceCausalRoleRootCauseContext,
		Predicate:            "critical_blocking",
		ChainRelevance:       "on_chain",
		OnChainOverflowFold:  true,
		ImpactMS:             14.272,
		CumulativeImpactMS:   14.272,
		MergedCount:          2,
		MergedMinMS:          14.272,
		MergedMaxMS:          14.272,
		MergedValuelessCount: 1,
		MergedSubjects:       []string{"hmfs_discard-26-562", "oney.hmn.berlin-42591"},
		EvidenceID:           "g12-fold-ev",
		Confidence:           0.82,
		LineStart:            1017021,
		LineEnd:              1625582,
	}
}

func g12MixedFoldProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"hmfs_discard-26-562", "oney.hmn.berlin-42591"},
		WindowStartTs: 6793224.9,
		WindowEndTs:   6793225.0,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "hmfs_discard-26-562",
				Object: "io_latency", StateKind: "io_wait", ChainRelevance: "on_chain",
				ImpactMS: 11.506, CumulativeImpactMS: 11.506, Confidence: 0.86,
				EvidenceID: "g12-io-ev", LineStart: 1619193, LineEnd: 1638234},
			g12MixedFoldNode(),
		},
	}
}

// TestG12MixedFoldHonestRangeFaces pins the honest mixed-fold wording on all
// three faces: the fence tag, the (a) table token (shared helper) and the (b)
// 合并明细 line — plus the gated legend entry (bidirectional via the mark).
func TestG12MixedFoldHonestRangeFaces(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(g12MixedFoldProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	t.Logf("mixed fold render (zh fence):\n%s", fence)
	if !strings.Contains(fence, "2线程取最大(有值1项 单项14.272ms,1项无时长值)") {
		t.Fatalf("the mixed fold must bind the range to the valued member only:\n%s", fence)
	}
	// The fabricated legacy claim — both members wearing the max — must be gone.
	if strings.Contains(fence, "2线程取最大(单项14.272~14.272ms)") {
		t.Fatalf("the fabricated n=2 same-value form must not render on a mixed fold:\n%s", fence)
	}
	// Legend: the 无时长值 entry teaches the wording exactly when it renders
	// (full NEW-7 bidirectional contract over every mark on this fixture).
	marks := revisit76AssertLegendBidirectional(t, "g12_mixed_fold_zh", g12MixedFoldProjection(), true)
	if !marks.has(runtimeTraceProjMarkValuelessFoldMembers) {
		t.Fatalf("the mixed fold must emit the valueless-members mark")
	}
	revisit76AssertLegendBidirectional(t, "g12_mixed_fold_en", g12MixedFoldProjection(), false)
	// (b) detail block mirrors the honest form.
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "2线程取最大(墙钟跨线程不可加和),有值1项各 14.272ms,另1项无时长值") {
		t.Fatalf("(b) block must mirror the honest mixed form:\n%s", detail)
	}
	if strings.Contains(detail, "各 14.272~14.272ms") {
		t.Fatalf("(b) block must not claim the range over every member:\n%s", detail)
	}
	// EN face.
	enModel := buildRuntimeTraceProjTreeModel(g12MixedFoldProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "2-thread max(1 valued 14.272ms, 1 without measurable duration)") {
		t.Fatalf("EN mixed fold must carry the honest form:\n%s", enFence)
	}
}

// TestG12MixedFoldMutationRevert is the mandated strongest-mutation pin:
// zeroing the typed valueless count (the exact effect of reverting the
// constructor accounting arm) re-fabricates the E23 byte-form on the same
// node. If the accounting arm is removed, TestG12MixedFoldHonestRangeFaces
// and the types-side reproduction pin red together with this witness.
func TestG12MixedFoldMutationRevert(t *testing.T) {
	node := g12MixedFoldNode()
	honest := runtimeTraceProjMergedMaxTagText(node, true)
	node.MergedValuelessCount = 0 // the reverted-arm simulation
	legacy := runtimeTraceProjMergedMaxTagText(node, true)
	if legacy != "2线程取最大(单项14.272~14.272ms)" {
		t.Fatalf("mutation witness drifted — expected the legacy fabricated form, got %q", legacy)
	}
	if honest == legacy {
		t.Fatalf("the honest form must differ from the fabricated legacy form: %q", honest)
	}
	if !strings.Contains(honest, "1项无时长值") {
		t.Fatalf("the honest form must disclose the valueless member: %q", honest)
	}
}

// TestG12AllValuedFoldByteIdentity is the 负向 pin: an all-valued fold keeps
// the legacy tag byte-identically on every face (zh + en), and the mixed-fold
// legend entry never renders for it.
func TestG12AllValuedFoldByteIdentity(t *testing.T) {
	node := g12MixedFoldNode()
	node.MergedValuelessCount = 0
	node.MergedMinMS = 1.843
	if got := runtimeTraceProjMergedMaxTagText(node, true); got != "2线程取最大(单项1.843~14.272ms)" {
		t.Fatalf("all-valued zh tag must stay byte-identical: %q", got)
	}
	if got := runtimeTraceProjMergedMaxTagText(node, false); got != "2-thread max(each 1.843~14.272ms)" {
		t.Fatalf("all-valued en tag must stay byte-identical: %q", got)
	}
	projection := g12MixedFoldProjection()
	projection.OnChainCauses[1].MergedValuelessCount = 0
	projection.OnChainCauses[1].MergedMinMS = 1.843
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "2线程取最大(单项1.843~14.272ms)") {
		t.Fatalf("all-valued fold keeps the legacy form:\n%s", fence)
	}
	if strings.Contains(fence, "无时长值") {
		t.Fatalf("the mixed-fold wording must not leak onto all-valued folds:\n%s", fence)
	}
}

// g12MixedCWDNode is a subject-bearing mixed CWD merged row (复核 P1-1/P2-1):
// three members from overlapping query windows, one valueless.
func g12MixedCWDNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: "cwd-thread-7",
		Object: "supply_pressure", ChainRelevance: "on_chain",
		ImpactMS: 60.0, CumulativeImpactMS: 60.0,
		MergedCount: 3, MergedMinMS: 20.0, MergedMaxMS: 60.0,
		MergedValuelessCount: 1, MergedSumMS: 80.0, MergedCrossWindowMax: true,
		MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
			{StartTs: 6793224.9, EndTs: 6793225.0}, {StartTs: 6793224.95, EndTs: 6793225.05},
		},
		MergedSubjects: []string{"cwd-thread-7", "cwd-thread-8"},
		EvidenceID:     "g12-cwd-ev", Confidence: 0.7, LineStart: 100, LineEnd: 900,
	}
}

func g12MixedCWDProjection() types.TraceCausalProjection {
	p := g12MixedFoldProjection()
	p.OnChainCauses = append(p.OnChainCauses, g12MixedCWDNode())
	return p
}

// TestG12MixedCWDFoldFaces (复核 P1-1 + P2-1): the mixed CWD row's fence tag
// binds the range to the valued members, the 无时长值 legend entry rides along
// (词条-图例双向契约 — the P1-1 lesion was the missing mark on the
// RowMetricParts CWD arm), and the (b) 合并明细 line says the SAME split (the
// P2-1 lesion was (b) claiming 单次 a~b over every member on the same row).
func TestG12MixedCWDFoldFaces(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(g12MixedCWDProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "3次跨窗取最大(有值2项 单项20.000~60.000ms,1项无时长值)") {
		t.Fatalf("mixed CWD fence tag must bind the range to valued members:\n%s", fence)
	}
	marks := revisit76AssertLegendBidirectional(t, "g12_mixed_cwd_zh", g12MixedCWDProjection(), true)
	if !marks.has(runtimeTraceProjMarkValuelessFoldMembers) {
		t.Fatalf("the mixed CWD row must emit the valueless-members mark (P1-1)")
	}
	revisit76AssertLegendBidirectional(t, "g12_mixed_cwd_en", g12MixedCWDProjection(), false)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "原始和 80.000ms 供对照,有值2项单次 20.000~60.000ms,另1项无时长值") {
		t.Fatalf("(b) CWD arm must carry the valued split (P2-1):\n%s", detail)
	}
	if strings.Contains(detail, "供对照,单次 20.000~60.000ms") {
		t.Fatalf("(b) CWD arm must not claim the range over every member:\n%s", detail)
	}
}

// TestG12MixedUnionAndSumDetailFaces (复核 P2-1 union 连带 + P2-2① sum): the
// union and plain-SUM merged rows' fence tags and (b) per-instance segments
// ride the same valued split on mixed rows; all-valued forms stay
// byte-identical.
func TestG12MixedUnionAndSumDetailFaces(t *testing.T) {
	union := g12MixedCWDNode()
	union.MergedCrossWindowMax = false
	union.MergedIntervalUnion = true
	union.EvidenceID = "g12-union-ev"
	if got := runtimeTraceProjMergedUnionTagText(union, true); got != "3次(有值2项 20.000~60.000ms,1项无时长值)union" {
		t.Fatalf("mixed union tag must carry the valued split: %q", got)
	}
	if got := runtimeTraceProjMergedPerInstanceText(union, true); got != "有值2项单次 20.000~60.000ms,另1项无时长值" {
		t.Fatalf("mixed per-instance segment must carry the valued split: %q", got)
	}
	sum := g12MixedCWDNode()
	sum.MergedCrossWindowMax = false
	sum.MergedQueryWindows = nil
	if got := runtimeTraceProjMergedSumTagText(sum, true); got != "3次(有值2项 20.000~60.000ms,1项无时长值)" {
		t.Fatalf("mixed sum tag must carry the valued split (P2-2①): %q", got)
	}
	// 负向: all-valued rows keep the legacy neutral forms byte-identically.
	sum.MergedValuelessCount = 0
	if got := runtimeTraceProjMergedSumTagText(sum, true); got != "3次(20.000~60.000ms)" {
		t.Fatalf("all-valued sum tag must stay byte-identical: %q", got)
	}
	union.MergedValuelessCount = 0
	if got := runtimeTraceProjMergedUnionTagText(union, false); got != "n=3(20.000~60.000ms)union" {
		t.Fatalf("all-valued union tag must keep the en count form: %q", got)
	}
}

// g12StandaloneAllZeroR2Projection is the 复核 P2-2② witness shape: the hmfs
// ×4 zero-duration blocked_reason R2 aggregate rendered STANDALONE (no
// overflow) — previously 4次(0.000~0.000ms) + (b) 单次 0.000~0.000ms 伪值.
func g12StandaloneAllZeroR2Projection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"hmfs_discard-26-562", "oney.hmn.berlin-42591"},
		WindowStartTs: 6793224.9,
		WindowEndTs:   6793225.0,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "hmfs_discard-26-562",
				Object: "io_latency", StateKind: "io_wait", ChainRelevance: "on_chain",
				ImpactMS: 11.506, CumulativeImpactMS: 11.506, Confidence: 0.86,
				EvidenceID: "g12-io-ev", LineStart: 1619193, LineEnd: 1638234},
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "hmfs_discard-26-562",
				Object: "blocked_reason", ChainRelevance: "on_chain",
				MergedCount: 4, MergedValuelessCount: 4,
				MergedSubjects: []string{"hmfs_discard-26-562"},
				EvidenceID:     "g12-br-ev", Confidence: 0.82, LineStart: 1484314, LineEnd: 1625582},
		},
	}
}

// TestG12StandaloneAllZeroR2Faces (复核 P2-2②): the standalone all-zero R2
// row speaks the 无时长值 family word on BOTH faces — never a 0.000 pseudo
// range — and the legend entry rides along.
func TestG12StandaloneAllZeroR2Faces(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(g12StandaloneAllZeroR2Projection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "4次(全部无时长值)") {
		t.Fatalf("standalone all-zero R2 row must wear the honest tag:\n%s", fence)
	}
	if strings.Contains(fence, "0.000–0.000") {
		t.Fatalf("the 0.000 pseudo range is banned on the all-zero shape:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "同一线程 4 次实例合并,全部无时长值") {
		t.Fatalf("(b) sum arm must speak the honest all-zero form:\n%s", detail)
	}
	if strings.Contains(detail, "单次 0.000~0.000ms") {
		t.Fatalf("(b) sum arm must not mint the 0.000 pseudo range:\n%s", detail)
	}
	marks := revisit76AssertLegendBidirectional(t, "g12_all_zero_r2_zh", g12StandaloneAllZeroR2Projection(), true)
	if !marks.has(runtimeTraceProjMarkValuelessFoldMembers) {
		t.Fatalf("the all-zero R2 row must emit the valueless-members mark")
	}
	revisit76AssertLegendBidirectional(t, "g12_all_zero_r2_en", g12StandaloneAllZeroR2Projection(), false)
	enModel := buildRuntimeTraceProjTreeModel(g12StandaloneAllZeroR2Projection(), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	if enFence := runtimeTraceProjTreeFence(enModel, false); !strings.Contains(enFence, "n=4 (all without measurable duration)") {
		t.Fatalf("EN all-zero tag expected:\n%s", enFence)
	}
}

// TestG12MixedFoldAuditToken pins the evidence-index audit face: a mixed
// merged row carries merged_valueless=N beside merged_count=N; an all-valued
// row never mints the token.
func TestG12MixedFoldAuditToken(t *testing.T) {
	mixed := runtimeTraceCausalProjectionAuditDetail(g12MixedFoldNode(), true, false)
	if !strings.Contains(mixed, "merged_count=2") || !strings.Contains(mixed, "merged_valueless=1") {
		t.Fatalf("audit face must carry the valueless accounting beside merged_count:\n%s", mixed)
	}
	allValued := g12MixedFoldNode()
	allValued.MergedValuelessCount = 0
	if detail := runtimeTraceCausalProjectionAuditDetail(allValued, true, false); strings.Contains(detail, "merged_valueless") {
		t.Fatalf("all-valued rows must not mint the valueless token:\n%s", detail)
	}
}
