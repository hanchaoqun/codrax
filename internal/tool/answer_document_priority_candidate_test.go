package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPriorityInversionTypedAuthorityDoesNotRewriteModelConclusionAtPersist(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID: "edge", Subject: "worker-200", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
			Value: "7.000", Unit: "ms",
			RichNotes: []string{
				"priority_inversion_candidate=true", "priority_relation=lower_priority_waker",
				"rank=1", "tier=primary", "effective_impact_ms=7.000", "chain_relevance=on_chain",
			},
		}},
	}}
	observationsBefore, err := json.Marshal(bus.ToolResults[0].Observations)
	if err != nil {
		t.Fatalf("marshal observations before persist: %v", err)
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID: "summary", Kind: types.BlockSummary,
				Text: "worker-200 唤醒 app-100，存在优先级反转（lower_priority_waker）。",
			},
			{
				ID: "steps", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{
					ID: "s1", Label: "优先级反转：低优先级唤醒者",
					Text: "There is a priority inversion on this lower_priority_waker edge.", CitationRef: -1,
				}},
			},
			{
				ID: "already-bounded", Kind: types.BlockSummary,
				Text: "存在优先级反转候选（typed）。",
			},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist: err=%v success=%v summary=%s", err, res.Success, res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("missing persisted document")
	}
	observationsAfter, err := json.Marshal(bus.ToolResults[0].Observations)
	if err != nil {
		t.Fatalf("marshal observations after persist: %v", err)
	}
	if string(observationsAfter) != string(observationsBefore) {
		t.Fatalf("persistence must not mutate rank/tier/impact observation wire:\nbefore=%s\nafter=%s", observationsBefore, observationsAfter)
	}
	visible := answerDocumentVisibleSurfaceForRuntimeTrace(got)
	for _, want := range []string{
		"存在优先级反转（lower_priority_waker）",
		"优先级反转：低优先级唤醒者",
		"There is a priority inversion on this lower_priority_waker edge.",
		"存在优先级反转候选（typed）",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("persist-time system changed or dropped model wording %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "候选候选") {
		t.Fatalf("final surface created duplicated candidate wording:\n%s", visible)
	}
}

func TestPriorityInversionCandidateNormalizerRequiresExactTypedTrue(t *testing.T) {
	for _, notes := range [][]string{
		nil,
		{"priority_inversion_candidate=false"},
		{"priority_inversion_candidate=true-ish"},
	} {
		ctx := &types.BusContext{ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true,
			Observations: []types.ObservationRecord{{ID: "edge", RichNotes: notes}},
		}}}
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "summary", Kind: types.BlockSummary, Text: "存在优先级反转（unproven）。",
		}}}
		if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 0 {
			t.Fatalf("non-exact typed gate must not rewrite notes=%v fixed=%d doc=%+v", notes, fixed, doc)
		}
		if doc.Blocks[0].Text != "存在优先级反转（unproven）。" {
			t.Fatalf("non-exact typed gate changed prose: notes=%v text=%q", notes, doc.Blocks[0].Text)
		}
	}
}

func TestPriorityInversionCandidateNormalizerDoesNotRewriteSystemOrDiagramBlocks(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "edge", Subject: "worker-200", RichNotes: []string{"priority_inversion_candidate=true"},
		}},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "system", Kind: types.BlockSummary, Text: "worker-200 存在优先级反转（system fixture）。",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		},
		{
			ID: "diagram", Kind: types.BlockDiagram, Title: "worker-200 存在优先级反转（diagram label）。",
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "text", Body: "A --> B"},
		},
	}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 0 {
		t.Fatalf("system/diagram blocks must remain byte-identical, fixed=%d doc=%+v", fixed, doc)
	}
}

func TestPriorityInversionCandidateNormalizerDoesNotTouchUnrelatedDefiniteClaim(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "edge", Subject: "worker-200", RichNotes: []string{"priority_inversion_candidate=true"},
		}},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary,
		Text: "另一个锁持有者存在优先级反转（由独立 holder/waiter 证据确认）。",
	}}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 0 {
		t.Fatalf("candidate on another edge must not rewrite an unrelated claim, fixed=%d doc=%+v", fixed, doc)
	}
}

func TestPriorityInversionCandidateNormalizerBindsProductionClaimsToTypedSubject(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "worker-candidate", Subject: "shadowhook-task-64305", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
			RichNotes: []string{"priority_inversion_candidate=true", "effective_impact_ms=4.250"},
		}},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary,
		Text: strings.Join([]string{
			"核心原因是 shadowhook-task线程发生优先级反转。",
			"shadowhook-task线程的依赖从而产生优先级反转。",
			"shadowhook-task线程的优先级反转是on-chain根因。",
			"Priority inversion caused the frame for shadowhook-task.",
		}, "\n"),
	}}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed == 0 {
		t.Fatalf("typed subject candidate should weaken the bound definite claims: %+v", doc)
	}
	got := doc.Blocks[0].Text
	for _, want := range []string{
		"核心原因是 shadowhook-task线程出现优先级反转候选。",
		"shadowhook-task线程的依赖从而产生优先级反转候选。",
		"shadowhook-task线程的优先级反转候选是on-chain根因。",
		"Priority-inversion candidate may explain the frame for shadowhook-task.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("bound production wording missing %q:\n%s", want, got)
		}
	}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 0 {
		t.Fatalf("candidate normalization must be idempotent, second fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
}

func TestPriorityInversionCandidateNormalizerKeepsIndependentConfirmedEdge(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			{
				ID: "candidate", Subject: "worker-200", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
				RichNotes: []string{"priority_inversion_candidate=true", "effective_impact_ms=1.300"},
			},
			{
				ID: "confirmed", Subject: "holder-300", Predicate: "priority_inversion_confirmed", Object: "app-100",
				RichNotes: []string{"priority_inversion_authority=confirmed_holder_waiter"},
			},
		},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary,
		Text: "worker-200 存在优先级反转。\nholder-300 对 app-100 存在优先级反转。",
	}}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 1 {
		t.Fatalf("want exactly the candidate-bound line repaired, fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
	if !strings.Contains(doc.Blocks[0].Text, "worker-200 存在优先级反转候选。") ||
		!strings.Contains(doc.Blocks[0].Text, "holder-300 对 app-100 存在优先级反转。") {
		t.Fatalf("candidate/confirmed edge scoping failed: %q", doc.Blocks[0].Text)
	}
}

func TestPriorityInversionCandidateEdgeBindingRequiresExactPair(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "edge", Subject: "worker-20", Predicate: "wakeup_chain_edge", Object: "app-100",
			RichNotes: []string{"priority_inversion_candidate=true"},
		}},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary,
		Text: "worker-20 对 app-100 存在优先级反转。\nworker-20 对 app-101 存在优先级反转。\nworker-200 对 app-100 存在优先级反转。",
	}}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 1 {
		t.Fatalf("only the exact subject/peer line may change, fixed=%d text=%q", fixed, doc.Blocks[0].Text)
	}
	lines := strings.Split(doc.Blocks[0].Text, "\n")
	if !strings.Contains(lines[0], "低优先级依赖候选") || !strings.Contains(lines[0], "本窗未测得优先级反转影响") ||
		strings.Contains(lines[1], "候选") || strings.Contains(lines[2], "候选") {
		t.Fatalf("edge identity binding is not exact: %#v", lines)
	}
}

func TestPriorityInversionStructuralEdgeRepairsCustomerSummaryAndListWithoutErasingMeasuredSemantics(t *testing.T) {
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "edge", Subject: "worker-200", Predicate: "wakeup_chain_edge", Object: "app-100",
			RichNotes: []string{"priority_inversion_candidate=true", "priority_relation=lower_priority_waker"},
		}},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "summary", Kind: types.BlockSummary,
			Text: "根因是低优先级 CFS 线程 worker-200（prio=20）执行 VerifyClass，阻塞了高优先级的 app-100 被调度——存在优先级反转（lower_priority_waker）。",
		},
		{
			ID: "timeline", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID: "edge-item", Label: "优先级反转：低优先级唤醒者",
				Text: "worker-200 唤醒 app-100，存在优先级反转候选，app-100 被间接阻塞。",
			}},
		},
	}}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 3 {
		t.Fatalf("want summary + label + list text repaired, fixed=%d doc=%+v", fixed, doc)
	}
	visible := answerDocumentVisibleSurfaceForRuntimeTrace(doc)
	for _, want := range []string{
		"根因是低优先级 CFS 线程 worker-200（prio=20）执行 VerifyClass；该低优先级关系仅构成依赖候选，本窗未测得优先级反转影响。",
		"低优先级依赖候选：低优先级唤醒者（本窗未测得反转影响）",
		"worker-200 唤醒 app-100，存在低优先级依赖候选，但本窗未测得优先级反转影响。",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("structural-only surface missing %q:\n%s", want, visible)
		}
	}
	for _, forbidden := range []string{"阻塞了高优先级", "被间接阻塞", "存在优先级反转候选"} {
		if strings.Contains(visible, forbidden) {
			t.Fatalf("structural-only surface retained overclaim %q:\n%s", forbidden, visible)
		}
	}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(doc, ctx); fixed != 0 {
		t.Fatalf("structural normalization must be idempotent, fixed=%d doc=%+v", fixed, doc)
	}
}

func TestPriorityInversionMeasuredGateIsPositiveFiniteRootCauseImpactAndNotSameCPU(t *testing.T) {
	base := types.ObservationRecord{
		ID: "rank", Subject: "dependency-200", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
		RichNotes: []string{
			"priority_inversion_candidate=true", "effective_impact_ms=2.750",
			"gated_runnable_ms=1.500", "gated_running_deficit_ms=1.250",
			"dependency_cpu=1", "consumer_cpu=5",
		},
	}
	if !runtimeTracePriorityInversionMeasuredRecord(base) {
		t.Fatalf("positive cross-CPU gated impact must remain a measured candidate: %+v", base)
	}
	for _, bad := range []types.ObservationRecord{
		{Predicate: "wakeup_chain_edge", Object: "priority_inversion_candidate", RichNotes: []string{"effective_impact_ms=2.750"}},
		{Predicate: "root_cause_primary", Object: "priority_inversion_candidate", RichNotes: []string{"effective_impact_ms=0"}},
		{Predicate: "root_cause_primary", Object: "priority_inversion_candidate", RichNotes: []string{"effective_impact_ms=NaN"}},
		{Predicate: "root_cause_primary", Object: "running", RichNotes: []string{"priority_inversion_candidate=true", "effective_impact_ms=2.750"}},
	} {
		if runtimeTracePriorityInversionMeasuredRecord(bad) {
			t.Fatalf("non-measured shape passed exact gate: %+v", bad)
		}
	}
}

func TestCausalTreeTargetSelfRunnableLabelCannotBeReadAsDependencyState(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowSelf, HasData: true, EvidenceTag: "E1",
		Node: types.TraceCausalProjectionNode{
			Subject: "app-100", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ImpactMS: 1.3,
		},
	}
	zh := strings.Join(runtimeTraceProjSelfRowLines(row, 7.0, true), "\n")
	en := strings.Join(runtimeTraceProjSelfRowLines(row, 7.0, false), "\n")
	if !strings.Contains(zh, "⧖ 自身·runnable 1.300ms") {
		t.Fatalf("ZH target self row is still ambiguous: %q", zh)
	}
	if !strings.Contains(en, "⧖ own·runnable 1.300ms") {
		t.Fatalf("EN target self row is still ambiguous: %q", en)
	}
	for _, forbidden := range []string{"worker-200", "dependency-200"} {
		if strings.Contains(zh, forbidden) || strings.Contains(en, forbidden) {
			t.Fatalf("self row fabricated dependency identity %q: zh=%q en=%q", forbidden, zh, en)
		}
	}
}

func TestPriorityInversionImpactLegendKeepsCandidateCaliber(t *testing.T) {
	spec, ok := runtimeTraceProjImpactFormSpecFor(runtimeTraceProjImpactFormInversion)
	if !ok {
		t.Fatal("missing priority-inversion impact-form spec")
	}
	if !strings.Contains(spec.CategoryZH, "候选") || !strings.Contains(spec.SemanticsZH, "候选") ||
		!strings.Contains(spec.CategoryEN, "candidate") || !strings.Contains(spec.SemanticsEN, "candidate") {
		t.Fatalf("inversion form must remain candidate-strength on every generated face: %+v", spec)
	}
	if strings.Contains(spec.SemanticsZH, "优先级反转(低优先级") ||
		strings.Contains(spec.SemanticsEN, "priority inversion (a low-priority holder blocks") {
		t.Fatalf("legend retained the retired confirmed-inversion semantics: %+v", spec)
	}
}

func TestPriorityInversionCandidateTypedPublicationKeepsOnChainMainOffChainBackground(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank", SourcePath: "/trace/customer.systrace", TimeStart: 5.0, TimeEnd: 5.007,
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.007},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
					Thread:   tracequery.ThreadRef{Comm: "onchain", PID: 200},
					ImpactMs: 40, CumulativeImpactMs: 40, EffectiveImpactMs: 7, RunnableMs: 7,
					PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 7,
					PriorityRelationArtifactSources: []string{"compat:index"},
					ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
				},
				{
					Rank: 2, Tier: "tertiary", Type: "priority_inversion_candidate",
					Thread:   tracequery.ThreadRef{Comm: "offchain", PID: 900},
					ImpactMs: 100, CumulativeImpactMs: 100, EffectiveImpactMs: 100, RunnableMs: 100,
					PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 100,
					PriorityRelationArtifactSources: []string{"compat:index"},
					ChainRelevance:                  "background", Causality: "background", Confidence: 0.8,
				},
			},
		},
	}
	records := traceQueryTypedObservations(result, "customer.systrace", "payload-ref", "raw-ref", "", time.Unix(0, 0).UTC())
	bySubject := map[string]types.ObservationRecord{}
	for _, record := range records {
		bySubject[record.Subject] = record
	}
	onChain := bySubject["onchain-200"]
	offChain := bySubject["offchain-900"]
	if onChain.Predicate != "root_cause_primary" || onChain.Role != types.AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("typed on-chain candidate lost its main-board publication: %+v", onChain)
	}
	if offChain.Predicate != "root_cause_background" || offChain.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("typed off-chain candidate must publish only as background: %+v", offChain)
	}
	onNotes := strings.Join(onChain.RichNotes, "\n")
	for _, want := range []string{"rank=1", "tier=primary", "effective_impact_ms=7.000", "chain_relevance=on_chain"} {
		if !strings.Contains(onNotes, want) {
			t.Fatalf("on-chain candidate lost typed ranking field %q:\n%s", want, onNotes)
		}
	}
	offNotes := strings.Join(offChain.RichNotes, "\n")
	if !strings.Contains(offNotes, "effective_impact_ms=0.000") || strings.Contains(offNotes, "effective_impact_ms=100.000") {
		t.Fatalf("off-chain candidate published a positive effective attribution:\n%s", offNotes)
	}
	if onChain.Value != "40.000" || offChain.Value != "100.000" {
		t.Fatalf("publication rewrote raw impact values: on=%+v off=%+v", onChain, offChain)
	}
}
