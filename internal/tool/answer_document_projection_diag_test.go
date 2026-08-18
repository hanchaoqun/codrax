package tool

// answer_document_projection_diag_test.go — DIAG batch display pins
// (§28.11-3, real_trace_campaign_20260705.md, 2026-07-09).
//
// A1 audit-token face: an E# entry standing for a cross-thread take-MAX fold
// with µs-tied members carries same_value_members= + same_value_lines= audit
// tokens (merged_ids family), seated early enough — and with the widened
// ceiling — that a worst-case prefix cannot push them off the audit cell.
//
// A2 实际口径 stanza line: the typed producer note + both values render the
// two-caliber line in both languages; either half missing renders nothing.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func diagFoldAuditNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:                types.TraceCausalRoleRootCauseContext,
		Predicate:           "critical_blocking",
		Subject:             "",
		Tier:                "deterministic_optimization",
		Causality:           "adjacent_to_wakeup_chain",
		Rank:                7,
		Confidence:          0.82,
		ChainRelevance:      "on_chain",
		OnChainOverflowFold: true,
		ImpactMS:            14.272,
		CumulativeImpactMS:  14.272,
		MergedCount:         2,
		MergedMinMS:         14.272,
		MergedMaxMS:         14.272,
		MergedSubjects:      []string{"hmfs_discard-1234", "com.example.app-42"},
		EvidenceID:          "fold-ev",
		LineStart:           5001,
		LineEnd:             6180,
		StartTs:             100.0,
		EndTs:               100.2,
		SameValueMembers: []types.TraceCausalProjectionSameValueMember{
			{Subject: "hmfs_discard-1234", LineStart: 5001, LineEnd: 5040},
			{Subject: "com.example.app-42", LineStart: 6100, LineEnd: 6180},
		},
	}
}

// TestDiagSameValueAuditTokensOnEvidenceIndex: the E# entry renders BOTH
// same_value tokens (subjects + per-member line intervals) on the audit face
// — through the real evidence-index item pipeline, worst-case prefix
// included, so the widened SameValueAudit ceiling is what this pin proves.
func TestDiagSameValueAuditTokensOnEvidenceIndex(t *testing.T) {
	idx := newRuntimeTraceCausalProjectionEvidenceIndex()
	if id := idx.add(diagFoldAuditNode(), true); id == "" {
		t.Fatal("fold node must join the evidence index")
	}
	_, items := runtimeTraceProjEvidenceBlockParts(idx, true)
	if len(items) != 1 {
		t.Fatalf("expected one index item, got %d", len(items))
	}
	text := items[0].Text
	if !strings.Contains(text, "取最大值时并列的同值成员：hmfs_discard-1234、com.example.app-42") {
		t.Fatalf("evidence explanation must carry the µs-tie member subjects:\n%s", text)
	}
	if !strings.Contains(text, "行 5001–5040、6100–6180") {
		t.Fatalf("evidence explanation must carry the per-member line intervals:\n%s", text)
	}
	// The merged accounting family keeps its seat beside the new tokens.
	if !strings.Contains(text, "合并 2 条同类观测") {
		t.Fatalf("merged observation count must survive beside the tie detail:\n%s", text)
	}
	for _, raw := range []string{"same_value_members=", "same_value_lines=", "merged_count="} {
		if strings.Contains(text, raw) {
			t.Fatalf("visible evidence explanation leaked raw metadata %q:\n%s", raw, text)
		}
	}
}

// TestDiagSameValueAuditWorstCasePrefix (RCM-2 D4 precedent): the worst REAL
// token prefix plus a two-member roster of LONG thread labels — the E23
// customer shape at its widest — still keeps BOTH same_value tokens inside
// the widened 280-rune ceiling (复核 P3-2: kept in lockstep with the
// SameValueAudit ceiling literal in the render arm).
func TestDiagSameValueAuditWorstCasePrefix(t *testing.T) {
	node := diagFoldAuditNode()
	node.SameValueMembers = []types.TraceCausalProjectionSameValueMember{
		{Subject: "com.example.superlongapp:render-4310789", LineStart: 9999901, LineEnd: 9999940},
		{Subject: "hmfs_discard_worker_pool-9876543", LineStart: 8888801, LineEnd: 8888840},
	}
	node.MergedSubjects = []string{"com.example.superlongapp:render-4310789", "hmfs_discard_worker_pool-9876543"}
	idx := newRuntimeTraceCausalProjectionEvidenceIndex()
	idx.add(node, true)
	_, items := runtimeTraceProjEvidenceBlockParts(idx, true)
	text := items[0].Text
	if !strings.Contains(text, "com.example.superlongapp:render-4310789、hmfs_discard_worker_pool-9876543") ||
		!strings.Contains(text, "9999901–9999940、8888801–8888840") {
		t.Fatalf("worst-case prefix must not push the tie detail off the evidence ceiling:\n%s", text)
	}
}

// TestDiagSameValueAuditTokensAbsentWithoutTie: no disclosure → no token
// (absence never fabricates a tie).
func TestDiagSameValueAuditTokensAbsentWithoutTie(t *testing.T) {
	node := diagFoldAuditNode()
	node.SameValueMembers = nil
	node.MergedMinMS = 13.900
	idx := newRuntimeTraceCausalProjectionEvidenceIndex()
	idx.add(node, true)
	_, items := runtimeTraceProjEvidenceBlockParts(idx, true)
	if len(items) != 1 {
		t.Fatalf("expected one index item, got %d", len(items))
	}
	if strings.Contains(items[0].Text, "取最大值时并列的同值成员") || strings.Contains(items[0].Text, "same_value_") {
		t.Fatalf("no tie → no same-value disclosure:\n%s", items[0].Text)
	}
}

// --- A2: 实际口径 two-caliber stanza line -----------------------------------

func diagActualCaliberProjection(note string, actualImpact, actualTotal float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "act-1",
			Subject: "OS_FFRT_2_2-43037", Object: "sleep_wait", TypeToken: "sleep_wait",
			ChainRelevance: "on_chain", ImpactMS: 12.0, CumulativeImpactMS: 12.0,
			ActualImpactMS: actualImpact, ActualTotalMS: actualTotal,
			ActualCaliberNote: note,
			Confidence:        0.8,
		}},
	}
}

// TestDiagActualCaliberDetailLineBothDirections (D-10 双向 pin): the typed
// note + both values render the two-caliber line (zh + EN); no note — even
// with both values present — renders nothing, and a note missing one half
// renders nothing (fail-safe, never a half-claim).
func TestDiagActualCaliberDetailLineBothDirections(t *testing.T) {
	projection := diagActualCaliberProjection(types.TraceActualCaliberStateSegmentVsThreadTotal, 59.050, 112.234)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "- 实际口径: 状态段 59.050ms/线程合计 112.234ms(两口径,来源不同)") {
		t.Fatalf("zh 实际口径 line must state both calibers:\n%s", detail)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enDetail := runtimeTraceProjDetailFullText(enModel, false)
	if !strings.Contains(enDetail, "- actual calibers: state segment 59.050ms / thread-level total 112.234ms (two calibers, different sources)") {
		t.Fatalf("EN actual-calibers line missing:\n%s", enDetail)
	}

	// Direction 2: no producer note (≤10% divergence upstream) → no line even
	// though both typed values are on the node.
	silent := diagActualCaliberProjection("", 59.050, 62.000)
	silentModel := buildRuntimeTraceProjTreeModel(silent, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if text := runtimeTraceProjDetailFullText(silentModel, true); strings.Contains(text, "实际口径") {
		t.Fatalf("no typed note → no 实际口径 line:\n%s", text)
	}

	// Fail-safe: note present but the thread-total half missing → nothing.
	half := diagActualCaliberProjection(types.TraceActualCaliberStateSegmentVsThreadTotal, 59.050, 0)
	halfModel := buildRuntimeTraceProjTreeModel(half, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if text := runtimeTraceProjDetailFullText(halfModel, true); strings.Contains(text, "实际口径") {
		t.Fatalf("a missing caliber half must render nothing:\n%s", text)
	}
}

// TestDiagActualCaliberProducerNote pins the single divergence judgment both
// ways: >10% of the larger → the closed enum; ≤10% or a missing half → "".
func TestDiagActualCaliberProducerNote(t *testing.T) {
	if got := traceQueryActualCaliberNote(59.050, 112.234); got != types.TraceActualCaliberStateSegmentVsThreadTotal {
		t.Fatalf("D-10 shape must disclose, got %q", got)
	}
	if got := traceQueryActualCaliberNote(100, 105); got != "" {
		t.Fatalf("5%% apart must stay silent, got %q", got)
	}
	// Boundary: exactly 10% of the larger is NOT >10% — stays silent.
	if got := traceQueryActualCaliberNote(90, 100); got != "" {
		t.Fatalf("exactly 10%% must stay silent, got %q", got)
	}
	if got := traceQueryActualCaliberNote(0, 100); got != "" {
		t.Fatalf("missing segment half must stay silent, got %q", got)
	}
	if got := traceQueryActualCaliberNote(100, 0); got != "" {
		t.Fatalf("missing total half must stay silent, got %q", got)
	}
	// Symmetric: a state segment LARGER than the thread total (defensive
	// direction) still discloses when >10% apart.
	if got := traceQueryActualCaliberNote(112.234, 59.050); got != types.TraceActualCaliberStateSegmentVsThreadTotal {
		t.Fatalf("reverse divergence must disclose too, got %q", got)
	}
}

// TestDiagSameValueMemberWireNote pins the wire-side tie collector both ways
// (the producer half of the E23 replica).
func TestDiagSameValueMemberWireNote(t *testing.T) {
	members := traceNoteKeysEmitFixtureOverflowImpacts(traceNoteKeysEmitFixtureResult().WakeupChain.CausalImpacts[0])
	note := traceQuerySameValueMemberNote(members, 14.272)
	if note != "tievictim-500@200-205,tietwin-501@210-215" {
		t.Fatalf("tie roster must name both members with their line ranges: %q", note)
	}
	// Distinct values → "" (zero-drop).
	if got := traceQuerySameValueMemberNote(members[:3], 14.272); got != "" {
		t.Fatalf("no tie at max → no note, got %q", got)
	}
}
