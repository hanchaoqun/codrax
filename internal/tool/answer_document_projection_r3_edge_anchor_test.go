package tool

// answer_document_projection_r3_edge_anchor_test.go — R3-IMPL display pins
// (§29.88.1 user ruling 2026-07-14; SCAN-3 sentinel pair §29.88.8):
//
//	件a  the host-edge-anchored semantic seat renders the 行2 边锚定(宿主→
//	     目标) credential sentence (typed OnChainBasis single-field fork) with
//	     the µs boundary + via word, zh 主 + EN 槽;
//	件b  the legend entry renders exactly when the mark fires (词条-图例双向 —
//	     the representative-shape sweep in the revisit76 harness also carries
//	     this fixture);
//	件c  the bisected span's ◇ remainder clone rides the EXISTING 同源二分
//	     行2 disclosure (RSPA typed trio reused — no second sentence family)
//	     and the existing adjacent semantic row form (确定性优化·候选);
//	件d  ENGINE-REAL sentinel witnesses (donghu_tieba_frame.systrace, zero
//	     LLM): the 正臂 window renders the 61839 VerifyClass seat ON the
//	     chain tier at 0.285ms with the credential sentence; the 负臂 window
//	     renders NO chain-tier semantic row (§29.53 产线实铸形 red line).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// r3HostEdgeAnchoredProjection is the SCAN-3 positive sentinel geometry
// (tieba 61839 VerifyClass 0.285ms before the 34579.496810 裸边) plus a
// synthetic bisected pair (件c; no natural straddle witness in-repo — 如实注,
// engine half pinned in tracequery TestR3BisectedSpanMintsSeatPlusRemainder).
func r3HostEdgeAnchoredProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"CookieMonsterCl-59843", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.490,
		WindowEndTs:   34579.500,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "r3-inversion",
			Subject: "NetworkService-60595", Object: "priority_inversion_candidate",
			TypeToken: "priority_inversion_candidate", StateKind: "runnable",
			// ChainDepth 1: keeps the depth chip out of the parent-unconfirmed
			// wording (the width governor may split that chip's EN words —
			// the bidirectional sweep owns that shape in its own fixtures).
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 5.951, CumulativeImpactMS: 6.406, EffectiveImpactMS: 5.951,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 4831, LineEnd: 6197,
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			// 件a: the sentinel seat — typed basis + credential pair.
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "r3-verifyclass",
			Subject: "T7@ZeusThreadPo-61839", Predicate: "trace_semantic_span",
			Object: "class_verification", SemanticClass: "class_verification",
			SpanName:       "VerifyClass com.baidu.zeus.mml.lac.LacUtils",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			OnChainBasis:            "host_wakeup_edge_pre_span",
			HostWakeupEdgeAnchorTS:  34579.496810,
			HostWakeupEdgeAnchorVia: "direct",
			ImpactMS:                0.285, CumulativeImpactMS: 0.285, EffectiveImpactMS: 0.285,
			Rank: 2, Tier: "secondary", Confidence: 0.82, LineStart: 5552, LineEnd: 5572,
		}, {
			// 件c ⛓ half: the bisected span's pre-edge seat (synthetic).
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "r3-straddle-seat",
			Subject: "worker-61900", Predicate: "trace_semantic_span",
			Object: "jit_compile", SemanticClass: "jit_compile",
			SpanName:       "JIT compiling void com.example.Straddle.run()",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			OnChainBasis:            "host_wakeup_edge_pre_span",
			HostWakeupEdgeAnchorTS:  34579.498000,
			HostWakeupEdgeAnchorVia: "chain_hop",
			ImpactMS:                1.0, CumulativeImpactMS: 1.0, EffectiveImpactMS: 1.0,
			ChainAnchoredMS: 1.0, ChainAnchorFullMS: 3.0,
			Rank: 3, Tier: "tertiary", Confidence: 0.82, LineStart: 5700, LineEnd: 5720,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			// 件c ◇ half: the post-edge remainder clone (RSPA trio reused).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "r3-straddle-remainder",
			Subject: "worker-61900", Object: "jit_compile", TypeToken: "jit_compile",
			SemanticClass:  "jit_compile",
			SpanName:       "JIT compiling void com.example.Straddle.run()",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
			ImpactMS: 2.0, CumulativeImpactMS: 2.0, EffectiveImpactMS: 2.0,
			ChainAnchoredMS: 1.0, ChainAnchorFullMS: 3.0, ChainAnchorRemainderSeat: true,
			Rank: 1, Confidence: 0.82, LineStart: 5700, LineEnd: 5720,
		}},
	}
}

// 件a/件b: the credential sentence + legend entry, zh and EN.
func TestR3EdgeAnchorSentenceAndLegend(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(r3HostEdgeAnchoredProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	// 修复轮件1/件2 (冷读 P3-1/P3-2): 「最晚相关边」 names the implementation
	// (latest in-window credential edge); the zh via word speaks the zh
	// inventory word 直接裸边 (图例同词), the EN slot keeps the wire token.
	if !strings.Contains(fence, "边锚定(宿主→目标):本席凭宿主线程自身对目标的窗内 typed 唤醒边入链上(边=凭证,边前=有效,边后=解除),计入值=span 边前段窗内投影(最晚相关边 34579.496810s,凭证=直接裸边)") {
		t.Fatalf("件a: the credential sentence must render with the µs boundary + zh via word:\n%s", fence)
	}
	if strings.Contains(fence, "凭证=direct") || strings.Contains(fence, "最近相关边") {
		t.Fatalf("件a: the zh sentence must not leak the EN wire token or the retired 最近 word:\n%s", fence)
	}
	// The chain-hop form wears its own zh word (straddle seat, via=chain_hop).
	if !strings.Contains(fence, "凭证=链上跳边") {
		t.Fatalf("件a: the chain-hop via zh word must render:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkHostEdgeAnchored) {
		t.Fatalf("件b: the mark must record at the emission site")
	}
	// 件c: the ◇ remainder rides the EXISTING 同源二分 disclosure and the
	// existing ◇ semantic row qualifier family.
	if !strings.Contains(fence, "同源二分:全窗3.000ms=锚定1.000ms") {
		t.Fatalf("件c: the remainder must ride the existing bipartition sentence:\n%s", fence)
	}
	// 修复轮件4 (对抗 P2-2): the ownership word follows the twin's actual
	// family — a SEMANTIC pair wears (✦链上席), never the state form's
	// (⛓链上席) (typed SemanticClass fork; the state form's ⛓ word is pinned
	// by the RSPA fixture below).
	if !strings.Contains(fence, "(✦链上席)") {
		t.Fatalf("件4: the semantic remainder's ownership word must wear the ✦ family glyph:\n%s", fence)
	}
	if strings.Contains(fence, "(⛓链上席)") {
		t.Fatalf("件4: the semantic pair must not borrow the state-family ⛓ ownership word:\n%s", fence)
	}
	// Negative arm: the STATE pair keeps (⛓链上席) byte-identically (the
	// RSPA donghu D-IO fixture, SemanticClass empty).
	rspaFence := rspaFenceJoined(runtimeTraceProjTreeFence(
		buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
			newRuntimeTraceCausalProjectionEvidenceIndex(), true), true))
	if !strings.Contains(rspaFence, "(⛓链上席)") {
		t.Fatalf("件4 负向: the state pair must keep the ⛓ ownership word:\n%s", rspaFence)
	}
	// EN mirror.
	modelEN := buildRuntimeTraceProjTreeModel(r3HostEdgeAnchoredProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "edge-anchored (host→target): this seat rides the chain tier on the HOST thread's own in-window typed wakeup edge toward the analysis target (edge=credential, pre-edge=effective, post-edge=released); the counted value is the span's pre-edge in-window projection (latest credential edge 34579.496810s, via=direct)") {
		t.Fatalf("件a EN: the credential sentence must render:\n%s", fenceEN)
	}
}

// r3RealTreeFence renders the whole zh answer document from ENGINE-REAL
// records (elimSemanticRealFence sibling; zero LLM, zero dispatch variance)
// and returns the full markdown.
func r3RealTreeFence(t *testing.T, pid int, start, end float64) string {
	t.Helper()
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: pid, TimeStart: start, TimeEnd: end,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "R3-IMPL sentinel witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
}

// 件d: the engine-real sentinel pair on the rendered answer face.
func TestR3SentinelWindowsRenderRealFence(t *testing.T) {
	// 正臂: the seat renders on the chain tier with the credential sentence.
	pos := r3RealTreeFence(t, 59566, 34579.490, 34579.500)
	if !strings.Contains(pos, "VerifyClass com.baidu.zeus.mml.lac.LacUtils") {
		t.Fatalf("正臂 must render the VerifyClass seat:\n%s", pos)
	}
	if !strings.Contains(pos, "0.285ms") {
		t.Fatalf("正臂 must render the 0.285ms sentinel value:\n%s", pos)
	}
	if !strings.Contains(pos, "边锚定(宿主→目标)") || !strings.Contains(pos, "34579.496810") {
		t.Fatalf("正臂 must render the credential sentence with the µs boundary:\n%s", pos)
	}
	// 负臂: no chain-tier semantic row and no credential sentence.
	neg := r3RealTreeFence(t, 59566, 34579.466, 34579.4965)
	if strings.Contains(neg, "边锚定(宿主→目标)") {
		t.Fatalf("负臂 must not render any credential sentence:\n%s", neg)
	}
}

// --- ONCHAIN-3c: the state-seat sibling basis display pins (2026-07-19) -------

// o3cStateEdgeProjection — a bare-census-edge host's bisected runnable state
// pair on the state sibling basis (the SCAN-3 61839 geometry at seat grain:
// engine halves pinned in tracequery rank_state_edge_anchor_test.go and the
// live TestRSPATiebaWitnessBoard witness).
func o3cStateEdgeProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"CookieMonsterCl-59843", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.490,
		WindowEndTs:   34579.500,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "o3c-anchor",
			Subject: "NetworkService-60595", Object: "priority_inversion_candidate",
			TypeToken: "priority_inversion_candidate", StateKind: "runnable",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 5.951, CumulativeImpactMS: 6.406, EffectiveImpactMS: 5.951,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 4831, LineEnd: 6197,
		}, {
			// The ⛓ pre-edge state half (state sibling basis).
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "o3c-state-seat",
			Subject: "T7@ZeusThreadPo-61839", Object: "runnable_wait",
			TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			OnChainBasis:            "host_wakeup_edge_pre_state",
			HostWakeupEdgeAnchorTS:  34579.496810,
			HostWakeupEdgeAnchorVia: "direct",
			ImpactMS:                0.370, CumulativeImpactMS: 0.370, EffectiveImpactMS: 0.370,
			ChainAnchoredMS: 0.370, ChainAnchorFullMS: 0.445,
			Rank: 2, Tier: "secondary", Confidence: 0.76, LineStart: 5600, LineEnd: 5620,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			// The ◇ post-edge remainder clone (RSPA trio reused; state family).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "o3c-state-rem",
			Subject: "T7@ZeusThreadPo-61839", Object: "runnable_wait",
			TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
			HostWakeupEdgeAnchorTS:  34579.496810,
			HostWakeupEdgeAnchorVia: "direct",
			ImpactMS:                0.075, CumulativeImpactMS: 0.075, EffectiveImpactMS: 0.075,
			ChainAnchoredMS: 0.370, ChainAnchorFullMS: 0.445, ChainAnchorRemainderSeat: true,
			Rank: 1, Confidence: 0.76, LineStart: 5600, LineEnd: 5620,
		}},
	}
}

// ONCHAIN-3c 件a: the state sibling forks the 行2 value clause on the SAME
// single OnChainBasis field (span=投影; state=状态段清单边前份合计) with the
// shared boundary detail; the span wording stays byte-identical (negative
// pin lives in TestR3EdgeAnchorSentenceAndLegend above).
func TestO3CStateEdgeAnchorSentence(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(o3cStateEdgeProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "边锚定(宿主→目标):本席凭宿主线程自身对目标的窗内 typed 唤醒边入链上(边=凭证,边前=有效,边后=解除),计入值=状态段清单边前份合计(最晚相关边 34579.496810s,凭证=直接裸边)") {
		t.Fatalf("件a: the state-seat credential sentence must render its own value clause:\n%s", fence)
	}
	if strings.Contains(fence, "计入值=span 边前段窗内投影") {
		t.Fatalf("件a: the state fixture must not wear the span value clause:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkHostEdgeAnchored) {
		t.Fatalf("件b: the shared mark must record at the emission site")
	}
	// The ◇ state remainder rides the EXISTING 同源二分 disclosure with the
	// state-family ⛓ ownership glyph (never the semantic ✦ word).
	if !strings.Contains(fence, "同源二分:全窗0.445ms=锚定0.370ms") {
		t.Fatalf("件c: the state remainder must ride the existing bipartition sentence:\n%s", fence)
	}
	// EN face: the state value clause renders in the EN slot too.
	modelEN := buildRuntimeTraceProjTreeModel(o3cStateEdgeProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !strings.Contains(fenceEN, "the counted value is the state-segment inventory's pre-edge share sum") {
		t.Fatalf("件a: the EN state value clause must render:\n%s", fenceEN)
	}
}

// ONCHAIN-3c 件e: the state basis wears the same 边锚定 chip on the ◎ board
// (single chip word, two basis tokens — the value-form difference lives on
// the 行2 sentence).
func TestO3CStateEdgeElimBoardChip(t *testing.T) {
	projection := rnb5bMicroAnchorFoldProjection()
	edge := elimChainNode("E-sedge", "hosthread-99", "runnable_wait", "runnable", 2, 0.370, 210)
	edge.OnChainBasis = "host_wakeup_edge_pre_state"
	projection.OnChainCauses = append(projection.OnChainCauses, edge)
	_, elim := elimRenderOverview(t, projection, true)
	found := false
	for _, line := range elimOverviewMemberLines(elim) {
		if strings.Contains(line, "hosthread-99") {
			found = true
			if !strings.Contains(line, "·边锚定") {
				t.Fatalf("the state edge-anchored seat must wear the 边锚定 chip: %q", line)
			}
		}
	}
	if !found {
		t.Fatalf("fixture drifted: the state edge-anchored seat must sit on the board:\n%s", elim)
	}
}
