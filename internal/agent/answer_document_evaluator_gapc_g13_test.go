package agent

// Wave-3.1 GAP-C G13 advisory pins (§27.4 G13 + §28.1 ruling,
// docs/design/real_trace_campaign_20260705.md, 2026-07-09; huadong_79_01
// witness: the answer prose led with a cadence-discounted periodic chain
// while the projection's elected primary root cause was an 11.5ms IO row).
//
// Contract under pin:
//   - entity mentioned in the summary/principal prose → NO advisory, the
//     accepted emit stops normally;
//   - entity missing → ONE advisory hint (HintRequested, never
//     StopRequested on the same observation), and the very next
//     observation accepts unconditionally (latch = non-blocking by
//     construction; entity mention matching is a noisy signal → soft
//     guidance only, red line);
//   - target-self symptom rows and aggregate-metric rows never elect the
//     headline entity (mirrors the rendered rank board's typed skips);
//   - the tid-suffix-stripped thread label counts as a mention (loose
//     normalization).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func gapcG13RankRecord(subject string, rank int, impactMS float64, extraNotes ...string) types.ObservationRecord {
	notes := append([]string{
		fmt.Sprintf("rank=%d", rank),
		"chain_relevance=on_chain",
		fmt.Sprintf("impact=%.3fms", impactMS),
		fmt.Sprintf("effective_impact_ms=%.3f", impactMS),
	}, extraNotes...)
	return types.ObservationRecord{
		ID:              "trace_query:rank:" + subject,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "huadong.systrace"},
		Span:            types.ObservationSpan{LineStart: 100, LineEnd: 105},
		ClaimKey:        "root_cause_primary:" + subject,
		Subject:         subject,
		Predicate:       "root_cause_primary",
		Object:          "io_wait",
		Value:           fmt.Sprintf("%.3f", impactMS),
		Unit:            "ms",
		RichNotes:       notes,
		SupportRefs:     []string{"huadong.systrace:100-105"},
	}
}

func gapcG13EvaluatorContext(records []types.ObservationRecord) (*answerDocumentEvaluator, *types.AgentContext, *types.MutableState) {
	mu := types.NewMutableState("卡顿主因是什么")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: records,
	}}})
	ctx := &types.AgentContext{
		Language:   "zh",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}},
		Mutable:    mu,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	return e, ctx, mu
}

func gapcG13SummaryDoc(text string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: text,
	}}}
}

// TestG13AdvisoryMentionedEntityNoHint: the summary names the elected
// entity (tid suffix stripped — loose normalization) → no advisory, the
// accepted emit stops normally.
func TestG13AdvisoryMentionedEntityNoHint(t *testing.T) {
	e, ctx, mu := gapcG13EvaluatorContext([]types.ObservationRecord{
		gapcG13RankRecord("hmfs_discard-5876", 1, 11.506),
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll,
		gapcG13SummaryDoc("主要原因是 hmfs_discard 线程的 IO 等待,窗口内归因 11.506ms。"))

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if sig.HintRequested {
		t.Fatalf("entity mentioned (tid-stripped) must not fire the advisory, got %+v", sig)
	}
	if !sig.StopRequested {
		t.Fatalf("accepted emit with mentioned entity must stop normally, got %+v", sig)
	}
}

// TestG13AdvisoryMissingEntityFiresOnceAndNeverBlocks: the summary pushes a
// different entity → exactly one advisory hint naming the elected entity;
// the same observation never stops AND never rejects; the next observation
// accepts unconditionally even when the prose stays unchanged.
func TestG13AdvisoryMissingEntityFiresOnceAndNeverBlocks(t *testing.T) {
	e, ctx, mu := gapcG13EvaluatorContext([]types.ObservationRecord{
		gapcG13RankRecord("hmfs_discard-5876", 1, 11.506),
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll,
		gapcG13SummaryDoc("VSyncGenerator 延迟链是主要原因。"))

	first := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !first.HintRequested || first.StopRequested {
		t.Fatalf("missing headline entity must fire ONE advisory without stopping, got %+v", first)
	}
	if first.HintKey != "answer_doc.trace_primary_cause_entity" {
		t.Fatalf("advisory must ride its own hint key, got %q", first.HintKey)
	}
	for _, want := range []string{"hmfs_discard-5876", "emit_answer_document_patch", "显式说明与该排序的差异"} {
		if !strings.Contains(first.Hint, want) {
			t.Fatalf("advisory hint missing %q:\n%s", want, first.Hint)
		}
	}
	// Advisory, not a gate: the unchanged document is accepted on the very
	// next observation — the latch guarantees the lane can never block.
	second := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if second.HintRequested || !second.StopRequested {
		t.Fatalf("the G13 advisory must be one-shot and non-blocking, got %+v", second)
	}
}

// TestG13AdvisoryLatchResetsPerDispatch (复核 P1-1-2, 2026-07-09): the
// one-shot latch is DISPATCH-scoped, not process-scoped — the evaluator is
// a long-lived registry singleton, so without the per-dispatch reset the
// advisory would go permanently silent from the second turn of a session
// onward. BuildInitialInstruction (the dispatch entry) must re-arm the lane
// exactly like its sister coverage latches.
func TestG13AdvisoryLatchResetsPerDispatch(t *testing.T) {
	e, ctx, mu := gapcG13EvaluatorContext([]types.ObservationRecord{
		gapcG13RankRecord("hmfs_discard-5876", 1, 11.506),
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll,
		gapcG13SummaryDoc("VSyncGenerator 延迟链是主要原因。"))
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested {
		t.Fatalf("dispatch 1 must fire the advisory, got %+v", sig)
	}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.StopRequested {
		t.Fatalf("dispatch 1 second observation must accept, got %+v", sig)
	}
	// New dispatch on the same evaluator instance (the REPL second-turn
	// shape): the entity is still missing → the advisory must fire again.
	_ = e.BuildInitialInstruction(ctx, nil)
	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.HintRequested || sig.StopRequested {
		t.Fatalf("a fresh dispatch must re-arm the G13 advisory latch, got %+v", sig)
	}
}

// TestG13AdvisorySilentWhenOnlySymptomAndAggregateRows: rows the rendered
// rank board skips (the target's own symptom tier, aggregate metrics) never
// elect a headline entity — the honest "no on-chain primary located" lane
// stays advisory-silent.
func TestG13AdvisorySilentWhenOnlySymptomAndAggregateRows(t *testing.T) {
	self := gapcG13RankRecord("com.example.app-1234", 1, 6.357, "tier=target_self_state")
	aggregate := gapcG13RankRecord("cpu_pressure", 2, 40.0, "subject_kind=aggregate_metric")
	e, ctx, mu := gapcG13EvaluatorContext([]types.ObservationRecord{self, aggregate})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll,
		gapcG13SummaryDoc("窗口内未定位到链上主根因。"))

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if sig.HintRequested {
		t.Fatalf("symptom/aggregate-only boards must not elect a headline entity, got %+v", sig)
	}
	if !sig.StopRequested {
		t.Fatalf("accepted emit must stop normally on the silent lane, got %+v", sig)
	}
}

// TestG13HeadlineElectionPrefersDiscountedAttribution: the election judges a
// periodic source by its discounted attribution — the cadence-discounted row
// (eff 0.166) loses to the IO row (eff 11.506) even when its raw window sum
// is larger (the huadong shape: raw 38.996 vs discounted 0.166).
//
// SYNTHETIC ANTI-REGRESSION SHAPE (复核 P3 注记, 2026-07-09): after the G9
// rank re-numbering the engine demotes a periodic source below the rank
// board, so a periodic row wearing Rank=1 is a pre-G9 form the engine no
// longer produces. The shape is KEPT deliberately as a defensive pin: if a
// periodic row ever re-enters the rank lane, the election must still judge
// it by its discounted attribution, never by its raw window sum.
func TestG13HeadlineElectionPrefersDiscountedAttribution(t *testing.T) {
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRolePrimaryRootCause, Subject: "VSyncGenerator-2270",
				Rank: 1, ImpactMS: 38.996, EffectiveImpactMS: 0.166, PeriodicSource: true,
			},
			{
				Role: types.TraceCausalRolePrimaryRootCause, Subject: "hmfs_discard-5876",
				Rank: 2, ImpactMS: 11.506, EffectiveImpactMS: 11.506,
			},
		},
	}
	node, ok := traceProjectionHeadlineNode(projection)
	if !ok {
		t.Fatalf("election must find a headline candidate")
	}
	if node.Subject != "hmfs_discard-5876" {
		t.Fatalf("election must follow the discounted attribution (11.506 > 0.166), got %q", node.Subject)
	}
}

// TestG13HeadlineElectionHonestFallbackIsSilent: no candidate with a
// POSITIVE DISCOUNTED attribution → no election (the report's honest
// fallback lane) — and a subject-less or rank-less row never elects either.
// 复核 P2-2 conservative-arm extension (2026-07-09): a ranked row with a
// positive RAW window value but zero discounted attribution stays
// non-electable — there is no raw-value fallback lane, because on an
// all-eff≤0 board it would re-admit exactly the values the discounts
// retired and diverge from the rendered lead selection.
func TestG13HeadlineElectionHonestFallbackIsSilent(t *testing.T) {
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, Subject: "t1-1", Rank: 1}, // zero attribution
			{Role: types.TraceCausalRolePrimaryRootCause, Subject: "", Rank: 2, ImpactMS: 5},
			{Role: types.TraceCausalRolePrimaryRootCause, Subject: "t3-3", ImpactMS: 5}, // no rank
			// P2-2: raw window value present, discounted attribution absent —
			// the conservative arm keeps the advisory silent.
			{Role: types.TraceCausalRolePrimaryRootCause, Subject: "t4-4", Rank: 3, ImpactMS: 9.5, CumulativeImpactMS: 9.5},
		},
	}
	if node, ok := traceProjectionHeadlineNode(projection); ok {
		t.Fatalf("zero-discounted-attribution / unranked / subject-less rows must not elect, got %+v", node)
	}
}

// TestG13HeadlineElectionSkipsSemanticSpanRows (复核 P1-1, 2026-07-09): the
// semantic optimization-span lane never claims the primary root cause (the
// LEAD-SEM negative pin), and after the rank re-numbering a semantic row's
// classified copy in the on-chain bucket carries a REAL rank — the election
// must skip it on the same typed pair the display board's exclusion reads
// (Role enum + trace_semantic_span predicate) and seat the next candidate.
func TestG13HeadlineElectionSkipsSemanticSpanRows(t *testing.T) {
	projection := types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleSemanticSpan, Subject: "jit-worker-77",
				Predicate: "trace_semantic_span", Rank: 1,
				ImpactMS: 50.0, EffectiveImpactMS: 50.0,
			},
			{
				Role: types.TraceCausalRoleCausalHop, Subject: "hmfs_discard-5876",
				Predicate: "root_cause_tertiary", Rank: 2,
				ImpactMS: 11.506, EffectiveImpactMS: 11.506,
			},
		},
	}
	node, ok := traceProjectionHeadlineNode(projection)
	if !ok {
		t.Fatalf("a non-semantic candidate exists; the election must seat it")
	}
	if node.Subject != "hmfs_discard-5876" {
		t.Fatalf("the semantic span must never elect the headline entity, got %q", node.Subject)
	}
	// Predicate-token arm alone (Role drifted) must also skip.
	projection.OnChainCauses[0].Role = types.TraceCausalRoleCausalHop
	if node, _ := traceProjectionHeadlineNode(projection); node.Subject == "jit-worker-77" {
		t.Fatalf("the trace_semantic_span predicate arm must also exclude, got %q", node.Subject)
	}
	// All-semantic board → silent (the LEAD-SEM lane never wears the
	// primary-cause claim, so the advisory has nothing to demand).
	projection.OnChainCauses = projection.OnChainCauses[:1]
	projection.OnChainCauses[0].Predicate = "trace_semantic_span"
	if node, ok := traceProjectionHeadlineNode(projection); ok {
		t.Fatalf("an all-semantic board must stay silent, got %+v", node)
	}
}

// TestG13HeadlineElectionSkipsUnknownThreadSentinel (复核 P2-1, 2026-07-09):
// §7.30 裁定1 — an unresolved-subject sentinel row never seats on the board;
// its label is a sentinel, not an entity the prose could meaningfully name.
// The election mirrors the display layer's canonical-subject arm.
func TestG13HeadlineElectionSkipsUnknownThreadSentinel(t *testing.T) {
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRolePrimaryRootCause, Subject: "Unknown-Thread",
				Rank: 1, ImpactMS: 99.0, EffectiveImpactMS: 99.0,
			},
			{
				Role: types.TraceCausalRolePrimaryRootCause, Subject: "hmfs_discard-5876",
				Rank: 2, ImpactMS: 11.506, EffectiveImpactMS: 11.506,
			},
		},
	}
	node, ok := traceProjectionHeadlineNode(projection)
	if !ok || node.Subject != "hmfs_discard-5876" {
		t.Fatalf("the unknown-thread sentinel must never elect; want the resolved row, got ok=%v node=%+v", ok, node)
	}
	// Sentinel-only board → silent.
	projection.PrimaryRootCauses = projection.PrimaryRootCauses[:1]
	if node, ok := traceProjectionHeadlineNode(projection); ok {
		t.Fatalf("a sentinel-only board must stay silent, got %+v", node)
	}
}

// TestG13EntityMentionLooseNormalization pins the matcher's loose lanes:
// case-insensitive full token, tid-suffix-stripped base, and the negative
// side (an unrelated thread never matches; ultra-short bases never match).
func TestG13EntityMentionLooseNormalization(t *testing.T) {
	visible := normalizedRequestedDimensionText("主要原因是 HMFS_discard 线程的 IO 等待。")
	if !traceProjectionHeadlineEntityMentioned("hmfs_discard-5876", visible) {
		t.Fatalf("tid-stripped case-insensitive base must count as a mention")
	}
	if !traceProjectionHeadlineEntityMentioned("HMFS_DISCARD", visible) {
		t.Fatalf("case-insensitive full token must count as a mention")
	}
	if traceProjectionHeadlineEntityMentioned("VSyncGenerator-2270", visible) {
		t.Fatalf("an unmentioned thread must not match")
	}
	if traceProjectionHeadlineEntityMentioned("ab-123", visible) {
		t.Fatalf("a sub-3-rune base must never substring-match")
	}
}
