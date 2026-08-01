package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func lowCoverageProjectionFixture(attributedMS, windowMS float64) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
	lead := types.TraceCausalProjectionNode{
		EvidenceID: "lead", Role: types.TraceCausalRolePrimaryRootCause,
		Subject: "shadowhook-task-64305", Predicate: "root_cause_primary", Object: "runnable_wait",
		Rank: 1, Tier: "primary", Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: attributedMS, CumulativeImpactMS: attributedMS, EffectiveImpactMS: attributedMS,
	}
	projection := types.TraceCausalProjection{
		PrimaryRootCause: &lead,
		WindowStartTs:    5.0,
		WindowEndTs:      5.0 + windowMS/1000,
	}
	model := runtimeTraceProjTreeModel{
		Target:   "app-100",
		WindowMS: windowMS,
		TrunkLen: 2,
		TreeRows: []runtimeTraceProjTreeRow{{
			Node: lead, Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
		}},
	}
	return projection, model
}

func lowCoverageOverclaimDocument() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "model-summary", Kind: types.BlockSummary,
			Text: strings.Join([]string{
				"shadowhook-task线程是整帧核心原因。",
				"主根因是 shadowhook-task线程。",
				// CROWNPOS-1: the definitional-prefix head form a model may echo
				// verbatim — the demotion sweep needs its own long-form pair
				// (the bare 主根因: pair never matches through the parenthetical).
				"主根因(=已证链上单项最大可消除量): shadowhook-task线程。",
				"shadowhook-task caused the frame drop.",
			}, "\n"),
		},
		{
			ID: "system-summary", Kind: types.BlockSummary,
			Text:                "shadowhook-task线程是整帧核心原因。",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		},
		{
			ID: "diagram", Kind: types.BlockDiagram, Title: "shadowhook-task线程是整帧核心原因。",
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "text", Body: "A --> B"},
		},
	}}
}

func TestLowCoverageSixPercentWeakensOnlyBoundModelClaims(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(6, 100)
	verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
	if !verdict.Comparable || !verdict.LowCoverage() || verdict.AttributedMS != 6 || verdict.DenominatorMS != 100 {
		t.Fatalf("6%% verdict mismatch: %+v", verdict)
	}
	if line := runtimeTraceProjWindowLine(projection, model, true); !strings.Contains(line, "链上已归因 6.000ms(6%)") {
		t.Fatalf("renderer did not consume the same 6/100 coverage arithmetic:\n%s", line)
	}

	doc := lowCoverageOverclaimDocument()
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 1 {
		t.Fatalf("want one model-authored field repaired, fixed=%d doc=%+v", fixed, doc)
	}
	for _, want := range []string{
		"shadowhook-task线程是当前已解释部分中最大候选。",
		"当前已解释部分中最大候选是 shadowhook-task线程。",
		// CROWNPOS-1: the echoed definitional-prefix form demotes too — and the
		// parenthetical itself must not survive the rewrite.
		"当前已解释部分中最大候选: shadowhook-task线程。",
		"shadowhook-task is the largest candidate within the currently explained portion.",
	} {
		if !strings.Contains(doc.Blocks[0].Text, want) {
			t.Fatalf("low-coverage model surface missing %q:\n%s", want, doc.Blocks[0].Text)
		}
	}
	if strings.Contains(doc.Blocks[0].Text, "已证链上单项最大可消除量") {
		t.Fatalf("the definitional parenthetical must not survive the demotion sweep:\n%s", doc.Blocks[0].Text)
	}
	if doc.Blocks[1].Text != "shadowhook-task线程是整帧核心原因。" ||
		doc.Blocks[2].Title != "shadowhook-task线程是整帧核心原因。" {
		t.Fatalf("system/diagram surfaces must remain byte-identical: %+v", doc.Blocks)
	}
}

func TestLowCoverageTwentyPercentBoundaryStillWeakens(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(20, 100)
	verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
	if !verdict.Comparable || !verdict.LowCoverage() {
		t.Fatalf("20%% inclusive boundary must be low coverage: %+v", verdict)
	}
	doc := lowCoverageOverclaimDocument()
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed == 0 {
		t.Fatalf("20%% boundary should weaken the bound model claim: %+v", doc)
	}
}

func TestLowCoverageAboveTwentyPercentDoesNotWeaken(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(21, 100)
	verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
	if !verdict.Comparable || verdict.LowCoverage() {
		t.Fatalf(">20%% must stay outside the guard: %+v", verdict)
	}
	doc := lowCoverageOverclaimDocument()
	want := doc.Blocks[0].Text
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 || doc.Blocks[0].Text != want {
		t.Fatalf(">20%% changed model prose: fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
}

func TestLowCoverageUnknownDoesNotWeaken(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(6, 0)
	verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
	if verdict.Comparable || verdict.LowCoverage() {
		t.Fatalf("unknown denominator must be incomparable: %+v", verdict)
	}
	doc := lowCoverageOverclaimDocument()
	want := doc.Blocks[0].Text
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 || doc.Blocks[0].Text != want {
		t.Fatalf("unknown coverage changed model prose: fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
}

func TestLowCoverageUnrelatedSubjectDoesNotWeaken(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(6, 100)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "other-task线程是整帧核心原因。",
	}}}
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 {
		t.Fatalf("unrelated subject must not be changed, fixed=%d doc=%+v", fixed, doc)
	}
}

func TestLowCoverageNegatedClaimDoesNotWeaken(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(6, 100)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary,
		Text: "shadowhook-task线程并未导致整帧丢帧，也不是主根因。\nshadowhook-task did not cause the frame drop and is not the primary root cause.",
	}}}
	want := doc.Blocks[0].Text
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 || doc.Blocks[0].Text != want {
		t.Fatalf("negated claim must stay byte-identical: fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
}

func TestLowCoverageNormalizationIsIdempotent(t *testing.T) {
	projection, model := lowCoverageProjectionFixture(6, 100)
	doc := lowCoverageOverclaimDocument()
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed == 0 {
		t.Fatal("fixture premise: first normalization did not run")
	}
	want := doc.Blocks[0].Text
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 || doc.Blocks[0].Text != want {
		t.Fatalf("second normalization must be byte-idempotent: fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
}

func TestLowCoverageIncomparableTypedShapesDoNotWeaken(t *testing.T) {
	tests := []struct {
		name  string
		shape func(types.TraceCausalProjection, runtimeTraceProjTreeModel) (types.TraceCausalProjection, runtimeTraceProjTreeModel)
	}{
		{
			name: "cross_base",
			shape: func(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
				model.TreeRows[0].Node.QueryWindowStartTs, model.TreeRows[0].Node.QueryWindowEndTs = 5.0, 5.1
				model.SelfRows = []runtimeTraceProjTreeRow{{
					Node: types.TraceCausalProjectionNode{
						Subject: "app-100", Role: types.TraceCausalRoleRootCauseContext, StateKind: "s_sleep", ImpactMS: 100,
						QueryWindowStartTs: 6.0, QueryWindowEndTs: 6.1,
					},
					Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				}}
				return projection, model
			},
		},
		{
			name: "census_collapse",
			shape: func(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
				model.SelfRows = []runtimeTraceProjTreeRow{
					{
						Node: types.TraceCausalProjectionNode{Subject: "app-100", Role: types.TraceCausalRoleRootCauseContext, StateKind: "s_sleep", ImpactMS: 10},
						Kind: runtimeTraceProjTreeRowSelf, HasData: true,
					},
					{
						Node: types.TraceCausalProjectionNode{Subject: "app-100", Role: types.TraceCausalRoleCausalHop, StateKind: "s_sleep", Object: "sleep", ImpactMS: 30},
						Kind: runtimeTraceProjTreeRowSelf, HasData: true,
					},
				}
				return projection, model
			},
		},
		{
			name: "overshoot",
			shape: func(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
				model.SelfRows = []runtimeTraceProjTreeRow{{
					Node: types.TraceCausalProjectionNode{Subject: "app-100", Role: types.TraceCausalRoleRootCauseContext, StateKind: "s_sleep", ImpactMS: 5},
					Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				}}
				return projection, model
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projection, model := lowCoverageProjectionFixture(6, 100)
			projection, model = tc.shape(projection, model)
			verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
			if verdict.Comparable || verdict.LowCoverage() {
				t.Fatalf("shape must stay incomparable: %+v", verdict)
			}
			doc := lowCoverageOverclaimDocument()
			want := doc.Blocks[0].Text
			if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc, projection, model); fixed != 0 || doc.Blocks[0].Text != want {
				t.Fatalf("incomparable shape changed prose: fixed=%d text=%q", fixed, doc.Blocks[0].Text)
			}
		})
	}
}

func TestLowCoveragePersistPipelinePublishesBoundaryWithoutRewritingModelConclusion(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			{
				ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Subject:         "app-100", Predicate: "wakeup_chain", Object: "shadowhook-task-64305 -> app-100",
				ClaimKey: "wakeup_chain:path", RichNotes: []string{
					"path=shadowhook-task-64305 -> app-100", "window=5.000000..5.100000", "selected_window=5.000000..5.100000",
				},
			},
			projV3Obs("lead", "root_cause_primary", "root_cause_primary:shadowhook-task-64305",
				"shadowhook-task-64305", "runnable_wait", "6.000", 6, 10, 20,
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
				"effective_impact_ms=6.000", "dominant_state=runnable", "selected_window=5.000000..5.100000"),
		},
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "model-summary", Kind: types.BlockSummary, Text: "shadowhook-task线程是整帧核心原因。",
	}}}
	wantModelWire, err := modelOwnedAnswerBlockWire(doc)
	if err != nil {
		t.Fatalf("snapshot model blocks: %v", err)
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist: err=%v success=%v summary=%s", err, res.Success, res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("missing persisted document")
	}
	if err := requireModelOwnedAnswerBlockWirePreserved(wantModelWire, got); err != nil {
		t.Fatalf("typed low-coverage authority changed the model answer: %v", err)
	}
	projection := projectionClusterBlock(got.Blocks, runtimeTraceCausalProjectionBlockIDBase)
	if projection == nil || !RuntimeTraceSystemBlock(*projection) {
		t.Fatalf("typed low-coverage projection boundary was not published as a sibling system block: %+v", got.Blocks)
	}
}
