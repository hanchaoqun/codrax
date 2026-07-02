package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_mutation_runtime_test.go — B4 v3 (2026-05-04).
// Tests that both full and patch paths converge on
// ApplyAndPersistMutation: shared merged-doc validation, shared
// setter, shared telemetry summary.

func newBusForMutationTest() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("test"),
	}
}

// TestApplyAndPersistMutation_ReplaceAllPersistsDocAndClearsPatchFlag
// — happy path: ReplaceAll → SetAnswerDocumentV2WithMutation called
// with MutationReplaceAll → LastEmitFromPatch=false.
func TestApplyAndPersistMutation_ReplaceAllPersistsDocAndClearsPatchFlag(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, err := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks) != 1 {
		t.Fatalf("merged doc not persisted; got %+v", got)
	}
	if bus.Mutable.LastEmitFromPatch() {
		t.Errorf("ReplaceAll must clear LastEmitFromPatch")
	}
}

func TestApplyAndPersistMutation_NormalizesHarmonyPriorityClassSurface(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Mutable.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics_normalized",
			Summary: "Harmony priority class normalized from raw prio fields: prio=98/ohos_rt, prio=120/ohos_rt. Rule: 数值越大优先级越高; 1-40=CFS, 41-139=RT.",
			Tags:    []string{"harmony_priority_normalized", "prio=98/ohos_rt", "prio=120/ohos_rt"},
		}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "Harmony priority semantics: 数值越大优先级越高; 1-40=CFS, 41-139=RT. Observed classes: prio=98/ohos_rt, prio=120/ohos_rt. rcu_preempt 处于 CFS 类（prio=98）。窗口内观察到 prio=98（CFS，rcu_preempt）。prio=98 落入 CFS 1–40 波段。",
		}, {
			ID:          "prio",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:    "p98",
				Label: "prio=98/ohos_rt (rcu_preempt)",
				Text:  "规则为 1-40=CFS、41-139=RT；rcu_preempt 线程在 CPU0 上运行，属于 CFS 区间（1-40）。",
			}, {
				ID:    "p120",
				Label: "prio=120/ohos_rt (ACCS0)",
				Text:  "ACCS0 线程处于 RT 区间（41-139）。",
			}},
		}, {
			ID:   "table",
			Kind: types.BlockTable,
			Text: "| 线程 | 优先级值 | 调度类 |\n|---|---|---|\n| rcu_preempt | prio=98/ohos_cfs | ohos_cfs（CFS） |",
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected mutation success, got %+v", res)
	}
	stored := bus.Mutable.AnswerDocumentV2()
	if stored == nil || len(stored.Blocks) < 2 || len(stored.Blocks[1].Items) < 2 {
		t.Fatalf("stored answer missing priority rows: %+v", stored)
	}
	if got := stored.Blocks[1].Items[0].Text; !strings.Contains(got, "属于 RT 区间（41-139）") || strings.Contains(got, "属于 CFS 区间（1-40）") {
		t.Fatalf("prio=98/ohos_rt row was not normalized from typed class: %q", got)
	}
	if got := stored.Blocks[0].Text; !strings.Contains(got, "1-40=CFS, 41-139=RT") {
		t.Fatalf("rule text should not be rewritten: %q", got)
	}
	if got := stored.Blocks[0].Text; !strings.Contains(got, "处于 RT 类（prio=98）") || strings.Contains(got, "处于 CFS 类（prio=98）") {
		t.Fatalf("bare prio=98 summary class should be normalized from typed map while preserving rule text: %q", got)
	}
	if got := stored.Blocks[0].Text; !strings.Contains(got, "prio=98（RT，rcu_preempt）") || strings.Contains(got, "prio=98（CFS，rcu_preempt）") {
		t.Fatalf("paren class after bare prio should be normalized from typed map: %q", got)
	}
	if got := stored.Blocks[0].Text; !strings.Contains(got, "prio=98 落入 RT 41-139 波段") || strings.Contains(got, "prio=98 落入 CFS 1–40 波段") {
		t.Fatalf("class/range phrase after bare prio should be normalized from typed map: %q", got)
	}
	if got := stored.Blocks[1].Items[1].Text; !strings.Contains(got, "RT 区间（41-139）") {
		t.Fatalf("already-correct row should be preserved: %q", got)
	}
	if got := stored.Blocks[2].Text; !strings.Contains(got, "prio=98/ohos_rt") || !strings.Contains(got, "ohos_rt（RT）") || strings.Contains(got, "ohos_cfs") {
		t.Fatalf("table row should be normalized from typed priority map: %q", got)
	}
}

func TestApplyAndPersistMutation_StampsReadOwnerAnchorsFromTurnA(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/observed.py", "pkg/owner.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/observed.py",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}, {
				Path:        "pkg/owner.py",
				Kind:        types.SourceLocalizationAnchorGroundedEvidence,
				Strength:    types.SourceLocalizationAnchorOwner,
				OwnerSymbol: "Owner.Handle",
				EvidenceRef: &types.WriteExplorationEvidenceRef{
					ID:          "ev-owner",
					Source:      "pkg/owner.py",
					LineStart:   12,
					OwnerSymbol: "Owner.Handle",
				},
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.ReadOwnerAnchors) != 1 {
		t.Fatalf("read owner anchors not stamped: %+v", got)
	}
	if got.ReadSourceLocalization == nil || got.ReadSourceLocalization.Status != types.SourceLocalizationObserved {
		t.Fatalf("read source localization not stamped: %+v", got.ReadSourceLocalization)
	}
	if len(got.ReadSourceLocalization.SourcePaths) == 0 || got.ReadSourceLocalization.SourcePaths[0] != "pkg/observed.py" {
		t.Fatalf("read source localization lost observed path: %+v", got.ReadSourceLocalization)
	}
	if got.ReadOwnerAnchors[0].Path != "pkg/owner.py" || got.ReadOwnerAnchors[0].OwnerSymbol != "Owner.Handle" {
		t.Fatalf("wrong stamped anchor: %+v", got.ReadOwnerAnchors[0])
	}
	if got.ReadOwnerAnchors[0].EvidenceRef == nil || got.ReadOwnerAnchors[0].EvidenceRef.ID != "ev-owner" {
		t.Fatalf("stamped anchor lost evidence ref: %+v", got.ReadOwnerAnchors[0])
	}
}

func TestApplyAndPersistMutation_SoftMixedRuntimeWithSourceProofKeepsSourceAuditSupplements(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AttachedLog = "WARN request timed out at artifact line 42"
	logBundle := &types.LogBundle{Errors: []types.LogError{{Type: "timeout"}}}
	bus.Mutable.SetLogTriage(logBundle)
	bus.TurnRouteHint = types.TurnRouteHint{
		Route:           "repo",
		Source:          "mixed",
		NeedsRepoAccess: true,
		Confidence:      0.9,
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:    types.IntentRootCause,
		Scenario:  types.ScenarioRootCause,
		LogTriage: logBundle,
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
	}}
	bus.EvidenceItems = []types.EvidenceItem{{
		ID:        "ev-owner",
		Source:    "pkg/owner.py",
		LineStart: 12,
		Summary:   "current source defines the timeout owner",
		Origin:    types.ClaimOriginCurrentRepo,
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationSupported,
			SourcePaths: []string{"pkg/owner.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:        "pkg/owner.py",
				Kind:        types.SourceLocalizationAnchorGroundedEvidence,
				Strength:    types.SourceLocalizationAnchorOwner,
				OwnerSymbol: "Owner.Handle",
				EvidenceRef: &types.WriteExplorationEvidenceRef{
					ID:          "ev-owner",
					Source:      "pkg/owner.py",
					LineStart:   12,
					OwnerSymbol: "Owner.Handle",
				},
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "runtime answer with current-source proof"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadSourceLocalization == nil || got.ReadSourceLocalization.Status != types.SourceLocalizationSupported {
		t.Fatalf("accepted current-source proof should keep localization supplement: %+v", got.ReadSourceLocalization)
	}
	if len(got.ReadOwnerAnchors) != 1 || got.ReadOwnerAnchors[0].Path != "pkg/owner.py" {
		t.Fatalf("accepted current-source proof should keep owner anchor supplement: %+v", got.ReadOwnerAnchors)
	}
}

func TestApplyAndPersistMutation_StampsReadNavigationCoverageFromTurnA(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqCallChain),
				Entities: []string{"dispatch"},
			},
		},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "repo_map",
			Success:  true,
			RawRef:   "blob://repo-map-task",
			Observations: []types.ObservationRecord{{
				ID:        "repo_map:task#navigation:task_map",
				Origin:    types.AnswerEvidenceOriginCrossRepoIndex,
				Producer:  "repo_map",
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCrossRepoIndex, RawRef: "blob://repo-map-task"},
				Predicate: types.RepoMapNavigationObservationPredicate,
				Object:    string(types.RepoMapNavigationRouteTaskMap),
			}},
		}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.ReadNavigationCoverage == nil {
		t.Fatalf("read navigation coverage not stamped: %+v", got)
	}
	coverage := got.ReadNavigationCoverage
	if coverage.State != types.RepoMapNavigationCoveragePartial {
		t.Fatalf("coverage state = %s, want partial: %+v", coverage.State, coverage)
	}
	if !testRepoMapRoutePresent(coverage.CoveredRoutes, types.RepoMapNavigationRouteTaskMap) {
		t.Fatalf("task_map should be covered: %+v", coverage)
	}
	if !testRepoMapRoutePresent(coverage.MissingRoutes, types.RepoMapNavigationRouteRelationMap) ||
		!testRepoMapRoutePresent(coverage.MissingRoutes, types.RepoMapNavigationRouteCallPath) {
		t.Fatalf("relation_map and call_path should stay missing for call-chain policy: %+v", coverage)
	}
	if len(coverage.EvidenceRefs) == 0 || coverage.EvidenceRefs[0] != "blob://repo-map-task" {
		t.Fatalf("typed evidence ref not preserved: %+v", coverage.EvidenceRefs)
	}
}

func TestApplyAndPersistMutation_StampsReadLocalizerFollowupFromTypedAuthorities(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Intent: types.IntentTrace},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.ReadLocalizerFollowup == nil {
		t.Fatalf("read localizer follow-up not stamped: %+v", got)
	}
	followup := got.ReadLocalizerFollowup
	if followup.State != types.ReadLocalizerFollowupNeeded {
		t.Fatalf("follow-up state = %s, want needed: %+v", followup.State, followup)
	}
	if len(followup.CandidatePaths) == 0 || followup.CandidatePaths[0] != "pkg/handler.py" {
		t.Fatalf("candidate paths not preserved: %+v", followup.CandidatePaths)
	}
	requirements := strings.Join(followup.EvidenceRequirements, "\n")
	if !strings.Contains(requirements, "typed_owner_localization_anchor") ||
		!strings.Contains(requirements, "repo_map_navigation_requirement") {
		t.Fatalf("follow-up requirements incomplete:\n%s", requirements)
	}
}

func TestApplyAndPersistMutation_DemotesNavigationOnlyFollowupAfterOwnerEvidence(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Intent: types.IntentTrace},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py", "pkg/nearby.py"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-owner",
			Kind:            types.EvidenceMechanism,
			Scope:           types.ScopeLine,
			Source:          "pkg/handler.py",
			LineStart:       12,
			Subject:         "Handler.Dispatch",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			OwnerSymbol:     "Handler.Dispatch",
		}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadNavigationCoverage == nil || got.ReadNavigationCoverage.State == types.RepoMapNavigationCoverageNotRequired {
		t.Fatalf("navigation coverage should still be stamped for audit: %+v", got.ReadNavigationCoverage)
	}
	if got.ReadLocalizerFollowup != nil {
		t.Fatalf("navigation-only follow-up should be advisory after owner evidence: %+v", got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_RuntimeTraceObservationOnlySkipsReadNavigationSupplement(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AttachedHitrace = "app-100 sched_switch prev_state=S"
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"app-100"},
		},
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadNavigationCoverage != nil {
		t.Fatalf("runtime-only trace should not stamp source navigation coverage: %+v", got.ReadNavigationCoverage)
	}
	if got.ReadLocalizerFollowup != nil {
		t.Fatalf("runtime-only trace should not stamp source localizer follow-up: %+v", got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_TraceQueryObservationSkipsReadNavigationSupplementWithoutAttachment(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"app-100"},
		},
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	bus.Mutable.AppendDispatchToolResult(mutationTraceQueryRuntimeToolResult())
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadNavigationCoverage != nil || got.ReadLocalizerFollowup != nil {
		t.Fatalf("trace_query runtime answer should not stamp navigation/localizer supplements: coverage=%+v followup=%+v", got.ReadNavigationCoverage, got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_RuntimeLogObservationOnlySkipsReadNavigationSupplement(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AttachedLog = "WARN request timed out at artifact line 42"
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioRootCause,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"request timeout"},
		},
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadNavigationCoverage != nil || got.ReadLocalizerFollowup != nil {
		t.Fatalf("runtime log answer should not stamp navigation/localizer supplements: coverage=%+v followup=%+v", got.ReadNavigationCoverage, got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_RuntimeTraceSourceOptionalSuppressesSourceAuditSupplements(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AttachedHitrace = "app-100 sched_switch prev_state=S"
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"app-100"},
		},
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:              "read_turn_a",
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"internal/tracequery/query.go"},
			SupportedPaths:      []string{"internal/tracequery/query.go"},
			OwnerSupportedPaths: []string{"internal/tracequery/query.go"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:        "internal/tracequery/query.go",
				Kind:        types.SourceLocalizationAnchorGroundedEvidence,
				Strength:    types.SourceLocalizationAnchorOwner,
				OwnerSymbol: "expandChain",
			}},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "repo_map",
			Success:  true,
			RawRef:   "blob://repo-map-task",
			Observations: []types.ObservationRecord{{
				ID:        "repo_map:task#navigation:task_map",
				Origin:    types.AnswerEvidenceOriginCrossRepoIndex,
				Producer:  "repo_map",
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCrossRepoIndex, RawRef: "blob://repo-map-task"},
				Predicate: types.RepoMapNavigationObservationPredicate,
				Object:    string(types.RepoMapNavigationRouteTaskMap),
			}},
		}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "runtime observation answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadSourceLocalization != nil || len(got.ReadOwnerAnchors) != 0 {
		t.Fatalf("runtime source-optional answer should not stamp source localization supplements: localization=%+v anchors=%+v", got.ReadSourceLocalization, got.ReadOwnerAnchors)
	}
	if got.ReadNavigationCoverage != nil || got.ReadLocalizerFollowup != nil {
		t.Fatalf("runtime source-optional answer should not stamp navigation/localizer supplements: coverage=%+v followup=%+v", got.ReadNavigationCoverage, got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_RuntimeTraceCurrentSourceRequirementStampsReadNavigation(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AttachedHitrace = "app-100 sched_switch prev_state=S"
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"app-100"},
		},
		SourceScopeProfile: &types.SourceScopeProfile{
			RequestedScope: types.SourceScopeProduction,
			Confidence:     0.8,
		},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Label:    "current implementation",
				Role:     types.RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	if got.ReadNavigationCoverage == nil || got.ReadNavigationCoverage.State != types.RepoMapNavigationCoverageMissing {
		t.Fatalf("current-source trace should keep navigation coverage: %+v", got.ReadNavigationCoverage)
	}
	if got.ReadLocalizerFollowup == nil || got.ReadLocalizerFollowup.State != types.ReadLocalizerFollowupNeeded {
		t.Fatalf("current-source trace should keep localizer follow-up: %+v", got.ReadLocalizerFollowup)
	}
}

func TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalProjection(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-app", "app-100", "compute_supply", "0.020", "0.020", 1),
			traceProjectionObservation("root-threadpool", "threadpool-400", "io_wait", "11.000", "11.000", 4),
			{
				ID:              "path",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "wakeup_chain",
				ClaimKey:        "wakeup_chain:path",
				Object:          "threadpool-400 -> network-300 -> cookie-200 -> app-100",
			},
			{
				ID:              "semantic",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "trace_semantic_span",
				ClaimKey:        "trace_semantic_span:class_verification",
				Subject:         "threadpool-400",
				Object:          "class_verification",
				Value:           "2.000",
				Unit:            "ms",
				RichNotes: []string{
					"span_name=VerifyClass com.example.Foo",
					"semantic_class=class_verification",
					"chain_relevance=on_chain",
					"causality=on_wakeup_chain",
					"chain_depth=1",
				},
				Confidence: 0.82,
			},
			{
				ID:              "bg-unresolved",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "root_cause_context",
				ClaimKey:        "root_cause_context:background",
				Subject:         "CodecLooper-17604",
				Object:          "unknown-thread",
				Value:           "6.886",
				Unit:            "ms",
				RichNotes: []string{
					"chain_relevance=background",
					"causality=background",
					"cumulative_impact_ms=6.886",
				},
				Confidence: 0.80,
			},
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "app-100 direct wait was observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 2 {
		t.Fatalf("answer document not persisted with projection: %+v", got)
	}
	// v3 lead block: fact-only conclusion + window/fallback declaration + tree
	// reading note + the MAIN monospace tree fence (no card list, no columns).
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil ||
		projection.Kind != types.BlockSection ||
		projection.SurfaceRole != types.SurfacePrincipal ||
		len(projection.Columns) != 0 || len(projection.Items) != 0 {
		t.Fatalf("missing principal v3 projection lead block: %+v", projection)
	}
	if len(projection.ClaimUses) != 1 || projection.ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("projection must stay in external-observation lane: %+v", projection.ClaimUses)
	}
	// Target-anchored tree: 🎯 root = user-focused thread, real branches, four
	// edge kinds, and the co-primary target row surfaces as a self-state line.
	for _, want := range []string{"主根因", "```text", "🎯 app-100", "└─下钻─", "─唤醒─", "compute_supply"} {
		if !strings.Contains(projection.Text, want) {
			t.Fatalf("v3 lead tree missing %q:\n%s", want, projection.Text)
		}
	}
	// No window anchor in this fixture → deterministic fallback scale, never a
	// fabricated window percentage.
	if !strings.Contains(projection.Text, "窗口起止未采集") {
		t.Fatalf("missing-window fixture must render the fallback scale declaration:\n%s", projection.Text)
	}
	// Bare path transit nodes stay visible so the chain is unbroken.
	if !strings.Contains(projection.Text, "链路中转") {
		t.Fatalf("transit chain nodes without rows must stay visible:\n%s", projection.Text)
	}

	text := projectionClusterText(got.Blocks)
	// v3 detail table: the single lossless surface.
	detail := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_detail")
	if detail == nil || detail.Kind != types.BlockTable {
		t.Fatalf("missing v3 lossless detail table:\n%s", text)
	}
	assertProjectionRowsCitationFree(t, detail)
	for _, want := range []string{"层级", "因果位置·优先级", "关系 ▸ 影响点", "影响形态", "窗口投影", "链上累计", "有效归因", "实际状态", "证据·置信"} {
		if !stringSliceContains(detail.Columns, want) {
			t.Fatalf("detail table missing column %q: %+v", want, detail.Columns)
		}
	}
	for _, want := range []string{"threadpool-400 / io_wait", "11.000ms", "阻塞/IO", "确定性优化点", "VerifyClass com.example.Foo", "class_verification"} {
		if !strings.Contains(text, want) {
			t.Fatalf("projection should surface fact %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "P0") {
		t.Fatalf("projection should not expose ambiguous P0/P1/P2 labels:\n%s", text)
	}
	if !strings.Contains(text, "CodecLooper-17604") || !strings.Contains(text, "未定位线程") || strings.Contains(text, "unknown-thread") {
		t.Fatalf("projection should translate unknown-thread sentinel for users without losing the unresolved-thread caveat:\n%s", text)
	}
	// 0 mermaid: no diagram blocks, no mermaid fences — the tree carries the
	// topology losslessly on all three surfaces.
	for _, blockID := range []string{
		"runtime_trace_causal_projection_wakeup",
		"runtime_trace_causal_projection_sleep",
		"runtime_trace_causal_projection_on_chain",
		"runtime_trace_causal_projection_on_chain_tree",
		"runtime_trace_causal_projection_impact",
		"runtime_trace_causal_projection_background",
	} {
		if projectionClusterBlock(got.Blocks, blockID) != nil {
			t.Fatalf("v3 must not render legacy multi-view block %s:\n%s", blockID, text)
		}
	}
	if strings.Contains(text, "mermaid") {
		t.Fatalf("v3 projection is zero-mermaid:\n%s", text)
	}
}

func TestApplyAndPersistMutation_TraceCausalProjectionBoundsLongWakeupChainDisplay(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	var path []string
	for i := 0; i < 8; i++ {
		path = append(path, "ThreadPoolForeg-60555", "NetworkService-60595")
	}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-threadpool", "ThreadPoolForeg-60555", "io_wait", "11.000", "11.000", 1),
			{
				ID:              "path",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "wakeup_chain",
				ClaimKey:        "wakeup_chain:path",
				Object:          strings.Join(path, " -> "),
			},
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "runtime chain observed."},
		},
	}

	if _, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	got := bus.Mutable.AnswerDocumentV2()
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil {
		t.Fatalf("missing projection block: %+v", got.Blocks)
	}
	// v3: the long trunk compresses its middle into ONE omitted marker row with
	// the counts + detected-cycle audit note; the full chain never renders raw.
	for _, want := range []string{"🎯 NetworkService-60595", "省略7个链路节点", "检测到2节点循环约8轮", "完整链路见原始 trace_query 记录"} {
		if !strings.Contains(projection.Text, want) {
			t.Fatalf("long wakeup chain tree should carry bounded audit note %q:\n%s", want, projection.Text)
		}
	}
	// The tree stays bounded: trunk display ≤ 8 chain rows + 1 omitted marker.
	if n := strings.Count(projection.Text, "─唤醒─") + strings.Count(projection.Text, "─下钻─"); n > 9 {
		t.Fatalf("long trunk must stay bounded, got %d chain rows:\n%s", n, projection.Text)
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_wakeup") != nil {
		t.Fatalf("v3 renders no wakeup diagram block: %+v", got.Blocks)
	}
}

// TestApplyAndPersistMutation_TraceCausalProjectionSleepDrilldownAndTriad pins the
// full presentation-gap coverage (§7 c/d/e) end-to-end in the rendered markdown:
// a sleep-dominant node is marked a symptom and drilled to its direct typed
// waker (gap d), an undrillable missing_wakeup sleep is explicitly flagged
// (gap e), and the duration triad (cum/proj/eff/act) renders with magnitude
// bars (gap c).
func TestApplyAndPersistMutation_TraceCausalProjectionSleepDrilldownAndTriad(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	mkRoot := func(id, subj, obj, cum, state string, rank int, notes ...string) types.ObservationRecord {
		rn := append([]string{
			fmt.Sprintf("rank=%d", rank), "tier=primary", "impact_ms=" + cum, "cumulative_impact_ms=" + cum,
			"causality=on_wakeup_chain", "chain_relevance=on_chain", "chain_depth=1", "dominant_state=" + state,
		}, notes...)
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "root_cause_primary", ClaimKey: "root_cause_primary",
			Subject: subj, Object: obj, Value: cum, Unit: "ms",
			Span: types.ObservationSpan{LineStart: 4, LineEnd: 7}, RichNotes: rn, Confidence: 0.9,
		}
	}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			mkRoot("root-run", "worker-200", "running", "4.600", "running", 1),
			mkRoot("root-sleep", "app-100", "sleep_wait", "5.000", "s_sleep", 2, "effective_impact_ms=11.040", "actual_impact_ms=4.600"),
			{
				ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
				Object: "worker-200 -> app-100",
			},
			{
				ID: "edge-worker-app", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain_edge", ClaimKey: "wakeup_chain_edge:worker-200->app-100",
				Subject: "worker-200", Object: "app-100",
			},
			{
				ID: "tool:1#trace_query:root_evidence:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard, ClaimKey: "root_evidence:missing_wakeup", Predicate: "missing_wakeup",
				Subject: "app-100", Value: "2.100", Unit: "ms", Span: types.ObservationSpan{LineStart: 88, LineEnd: 96},
				Summary: "sleep interval has no matching sched_wakeup row in the selected trace window",
			},
		},
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "observed."}}}
	if _, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	rendered := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")

	// gap c: the duration triad renders losslessly in the v3 detail table and
	// the tree carries magnitude bars.
	for _, want := range []string{"█", "| 层级 | 因果位置·优先级 | 节点/原因 | 关系 ▸ 影响点 | 影响形态 | 窗口投影 | 链上累计 | 有效归因 | 实际状态 | 证据·置信 |", "sleep / 等待唤醒", "running / CPU执行", "11.040ms", "4.600ms"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("duration triad + bar must render (gap c): missing %q:\n%s", want, rendered)
		}
	}
	// gap d: the sleeping target drills to its direct typed waker — the fact-only
	// conclusion line names the drilldown target, and the tree anchors at 🎯.
	for _, want := range []string{"💤", "🎯 app-100", "worker-200", "下钻到 worker-200"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("sleep drilldown must render (gap d): missing %q:\n%s", want, rendered)
		}
	}
	// gap e: the undrillable sleep is explicitly flagged with the typed reason
	// inline (self row ⛔) and stays auditable via the evidence locator.
	for _, want := range []string{"⛔", "无法下钻", "missing_wakeup", "lines=88-96"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("undrillable sleep must render (gap e): missing %q:\n%s", want, rendered)
		}
	}
	// v3: zero mermaid, no legacy multi-view tables, no ambiguous P0 labels.
	if !strings.Contains(rendered, "```text") || strings.Contains(rendered, "```mermaid") {
		t.Fatalf("section must render the monospace tree with zero mermaid:\n%s", rendered)
	}
	if strings.Contains(rendered, "P0") {
		t.Fatalf("section must not render ambiguous P0/P1/P2 labels:\n%s", rendered)
	}
	if strings.Contains(rendered, "| 链路深度 | 关系 | 上游原因 |") ||
		strings.Contains(rendered, "| 因果位置 | 关注点 | 关系 | 影响形态 |") {
		t.Fatalf("legacy multi-view tables must not render in v3:\n%s", rendered)
	}
}

func TestApplyAndPersistMutation_TraceCausalProjectionCoverageBoundaryWhenGuarded(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Refinement: &types.ToolRefinementHint{
			ReasonCode:      "trace_query_heavy_view_requires_scope",
			ResultTruncated: true,
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "trace_query was guarded."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	coverage := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_coverage")
	if coverage == nil || coverage.Kind != types.BlockCaveat {
		t.Fatalf("guarded trace_query without causal rows should render coverage caveat: %+v", got.Blocks)
	}
	for _, want := range []string{"未生成分层因果表", "heavy view", "不是“没有背景影响”的结论"} {
		if !strings.Contains(coverage.Text, want) {
			t.Fatalf("coverage caveat missing %q:\n%s", want, coverage.Text)
		}
	}
}

func TestApplyAndPersistMutation_TraceCausalProjectionNoBackgroundAndLongNodePresentation(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	longSubject := "com.example.deep.package.with.very.long.MainThreadRenderer~~UI~~-42591"
	longObject := "blocked_on_extremely_long_worker_chain_with_extra_context"
	longRef := "/very/long/customer/path/hiprofiler_data_20260702_very_long_name.systrace:123-130"
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			func() types.ObservationRecord {
				r := traceProjectionObservation("root-long", longSubject, longObject, "13.000", "13.000", 1)
				r.SupportRefs = []string{longRef}
				return r
			}(),
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "long node observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil || projection.Kind != types.BlockSection {
		t.Fatalf("missing v3 projection lead block: %+v", got.Blocks)
	}
	text := projectionClusterText(got.Blocks)
	if !strings.Contains(projection.Text, "没有产出可承重的 off-chain/background 行") {
		t.Fatalf("projection lead should explain missing background statistics:\n%s", projection.Text)
	}
	// No wakeup path in this fixture → the flat-layer fallback header renders
	// instead of an invented 🎯 anchor.
	if !strings.Contains(projection.Text, "唤醒链路径未解析") || strings.Contains(projection.Text, "🎯") {
		t.Fatalf("path-less fixture should render the flat fallback header:\n%s", projection.Text)
	}
	evidenceIndex := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_evidence")
	if evidenceIndex == nil || evidenceIndex.Kind != types.BlockBulletList || len(evidenceIndex.Columns) != 0 {
		t.Fatalf("evidence index should render as a bullet list: %+v", evidenceIndex)
	}
	if !strings.Contains(text, "完整定位见原始 trace_query 记录") ||
		!strings.Contains(text, "hiprofiler_data_20260702_very_long_name.systrace") ||
		!strings.Contains(text, ":123-130") {
		t.Fatalf("evidence index should show short locator and point to original trace_query record:\n%s", text)
	}
	// The detail table (markdown surface) must escape ~~; the monospace fence is
	// a literal <pre> surface where the raw name renders without strikethrough
	// hazard, so only inspect the table/evidence markdown surfaces.
	if detailIdx := strings.Index(text, "因果投影明细"); detailIdx >= 0 {
		if strings.Contains(text[detailIdx:], "~~UI~~") {
			t.Fatalf("detail table must escape markdown strikethrough markers:\n%s", text[detailIdx:])
		}
	} else {
		t.Fatalf("missing v3 detail table:\n%s", text)
	}
	if strings.Contains(text, "/very/long/customer/path/") ||
		strings.Contains(text, longRef) {
		t.Fatalf("user-facing projection should not render full local absolute trace paths:\n%s", text)
	}
	// The long identity is compacted in the tree label (display-width budget)
	// while the detail row keeps the subject/cause split readable.
	if !strings.Contains(projection.Text, "…") {
		t.Fatalf("long node should be compacted in the tree label: %s", projection.Text)
	}
}

func TestApplyAndPersistMutation_LowImpactSemanticSpanSurvivesToRenderedText(t *testing.T) {
	// O5 end-to-end: a low-impact (2ms) semantic span must survive from the
	// trace observation, through the auto-injected projection block, all the
	// way to the final rendered markdown — not be swallowed by summary/cap
	// truncation. Also exercises the O4 within-requested-window tag, since a
	// frame_target_resolution anchor is present.
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			{
				ID:              "trace_query:window#frame_target_resolution",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "frame_target_resolution",
				Subject:         "app-100",
				Object:          "frame_timeline_ui_unique",
				Span:            types.ObservationSpan{StartTs: 100.0, EndTs: 200.0},
				RichNotes:       []string{"window_source=query_window", "window=100.000000..200.000000"},
				Confidence:      0.86,
			},
			traceProjectionObservation("root-app", "app-100", "compute_supply", "0.020", "0.020", 1),
			{
				ID:              "semantic",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "trace_semantic_span",
				ClaimKey:        "trace_semantic_span:class_verification",
				Subject:         "app-100",
				Object:          "class_verification",
				Value:           "2.000",
				Unit:            "ms",
				Span:            types.ObservationSpan{StartTs: 120.0, EndTs: 122.0},
				RichNotes: []string{
					"span_name=VerifyClass com.example.Foo",
					"semantic_class=class_verification",
					"chain_relevance=on_chain",
					"causality=on_wakeup_chain",
					"chain_depth=1",
					"window=120.000000..122.000000",
				},
				Confidence: 0.82,
			},
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "app-100 direct wait was observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("answer document not persisted")
	}
	rendered := render.RenderAnswerDocument(got, "zh")
	if !strings.Contains(rendered, "确定性优化点") {
		t.Fatalf("semantic span optimization block must survive to rendered text: %q", rendered)
	}
	if !strings.Contains(rendered, "VerifyClass com.example.Foo") {
		t.Fatalf("the concrete low-impact semantic span name must survive to rendered text: %q", rendered)
	}
	if !strings.Contains(rendered, "class_verification") {
		t.Fatalf("the semantic class must survive to rendered text: %q", rendered)
	}
	// v3: the anchor window renders as the lead's explicit window line (in-window
	// nodes carry no marker; only outside/crossing nodes get ⚠).
	if !strings.Contains(rendered, "关注窗口 100.000s → 200.000s") {
		t.Fatalf("the anchor window line must render when the precise window anchor exists: %q", rendered)
	}
	if strings.Contains(rendered, "⚠跨窗") {
		t.Fatalf("a fully in-window node must not carry the crossing marker: %q", rendered)
	}
}

func TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalProjectionInEnglish(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Language = "en"
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
		},
		AnswerContract: types.AnswerContract{Language: "en"},
	}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-threadpool", "threadpool-400", "io_wait", "11.000", "11.000", 1),
			{
				ID:              "path",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "wakeup_chain",
				ClaimKey:        "wakeup_chain:path",
				Object:          "threadpool-400 -> network-300 -> app-100",
			},
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "app-100 direct wait was observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 2 {
		t.Fatalf("answer document not persisted with projection: %+v", got)
	}
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil || projection.Kind != types.BlockSection {
		t.Fatalf("missing English v3 projection lead block: %+v", projection)
	}
	if projection.Title != "Trace Causal Projection" {
		t.Fatalf("projection title should follow language: %+v", projection)
	}
	// English lead: fact-only conclusion, tree-reading note, target-anchored tree
	// with localized edge labels — zero mermaid.
	for _, want := range []string{"Primary root cause: threadpool-400 io_wait 11.000ms", "Tree reading", "```text", "🎯 app-100", "<user-focused thread>", "─wakes─"} {
		if !strings.Contains(projection.Text, want) {
			t.Fatalf("English v3 lead missing %q:\n%s", want, projection.Text)
		}
	}
	text := projectionClusterText(got.Blocks)
	detail := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_detail")
	if detail == nil {
		t.Fatalf("missing English v3 detail table:\n%s", text)
	}
	assertProjectionRowsCitationFree(t, detail)
	for _, want := range []string{"Layer", "Relation ▸ impact point", "Window projection", "Chain total", "Evidence · confidence"} {
		if !stringSliceContains(detail.Columns, want) {
			t.Fatalf("English detail table missing column %q: %+v", want, detail.Columns)
		}
	}
	for _, want := range []string{"threadpool-400 / io_wait", "11.000ms"} {
		if !strings.Contains(text, want) {
			t.Fatalf("English projection should surface %q:\n%s", want, text)
		}
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_wakeup") != nil ||
		strings.Contains(text, "mermaid") {
		t.Fatalf("English v3 is zero-mermaid:\n%s", text)
	}
}

func TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalHopDepth(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-worker", "worker-200", "sleep_wait", "9.000", "9.000", 1),
			{
				ID:              "path",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "wakeup_chain",
				ClaimKey:        "wakeup_chain:path",
				Object:          "io-500 -> net-400 -> binder-300 -> worker-200 -> app-100",
			},
			{
				ID:              "hop-depth",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "wakeup_causal_impact",
				ClaimKey:        "wakeup_causal_impact:io-500",
				Subject:         "io-500",
				Object:          "io_wait",
				Value:           "7.000",
				Unit:            "ms",
				RichNotes: []string{
					"causality=on_wakeup_chain",
					"depth=4",
					"impact=7.000ms",
				},
				Confidence: 0.82,
			},
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "app-100 direct wait was observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 2 {
		t.Fatalf("answer document not persisted with projection: %+v", got)
	}
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil || projection.Kind != types.BlockSection {
		t.Fatalf("missing v3 projection lead block: %+v", projection)
	}
	text := projectionClusterText(got.Blocks)
	// The on-chain io-500 hop keeps its typed depth-4 chain position (gaps a/b):
	// it sits at trunk depth 4 in both the tree and the detail table.
	for _, want := range []string{"io-500 / io_wait", "深度4"} {
		if !strings.Contains(text, want) {
			t.Fatalf("projection should preserve on-chain hop depth %q:\n%s", want, text)
		}
	}
	// gap d: the sleep-state primary (worker-200 -> sleep_wait) is marked a
	// symptom in the tree; its upstream trunk child IS the drilldown lane, so no
	// separate diagram is needed (v3 is zero-mermaid).
	if !strings.Contains(projection.Text, "💤 worker-200") {
		t.Fatalf("sleep-state node should be marked as a symptom in the tree:\n%s", projection.Text)
	}
	if !strings.Contains(text, "sleep症状") {
		t.Fatalf("sleep symptom action label should render:\n%s", text)
	}
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_sleep") != nil {
		t.Fatalf("v3 renders no sleep diagram block: %+v", got.Blocks)
	}
}

func TestApplyAndPersistMutation_ExpandsRuntimeTraceCausalProjectionCapacity(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	records := []types.ObservationRecord{{
		ID:              "path",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "wakeup_chain",
		ClaimKey:        "wakeup_chain:path",
		Object:          "dep-1 -> dep-2 -> dep-3 -> dep-4 -> dep-5 -> dep-6 -> dep-7 -> dep-8 -> dep-9 -> dep-10 -> app-100",
	}}
	for i := 1; i <= 10; i++ {
		records = append(records, traceProjectionObservation(
			fmt.Sprintf("root-%d", i),
			fmt.Sprintf("dep-%d", i),
			"sleep_wait",
			fmt.Sprintf("%d.000", 30-i),
			fmt.Sprintf("%d.000", 30-i),
			i,
		))
	}
	semanticClasses := []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile"}
	for i := 1; i <= 16; i++ {
		class := semanticClasses[(i-1)%len(semanticClasses)]
		records = append(records, types.ObservationRecord{
			ID:              fmt.Sprintf("semantic-%02d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "trace_semantic_span",
			ClaimKey:        "trace_semantic_span:" + class,
			Subject:         fmt.Sprintf("dep-%d", i),
			Object:          class,
			Value:           "1.500",
			Unit:            "ms",
			RichNotes: []string{
				fmt.Sprintf("span_name=%s-%02d", class, i),
				"semantic_class=" + class,
				"chain_relevance=on_chain",
				"causality=on_wakeup_chain",
				fmt.Sprintf("chain_depth=%d", i),
			},
			Confidence: 0.82,
		})
	}
	for depth := 1; depth <= 10; depth++ {
		records = append(records, types.ObservationRecord{
			ID:              fmt.Sprintf("hop-%02d", depth),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			ClaimKey:        fmt.Sprintf("wakeup_causal_impact:dep-%d", depth),
			Subject:         fmt.Sprintf("dep-%d", depth),
			Object:          "sleep_wait",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes: []string{
				"causality=on_wakeup_chain",
				fmt.Sprintf("depth=%d", depth),
				"impact=1.000ms",
			},
			Confidence: 0.80,
		})
	}
	bus.ToolResults = []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: records,
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "app-100 causal path was observed."},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	projection := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection")
	if projection == nil || projection.Kind != types.BlockSection {
		t.Fatalf("missing v3 projection lead block: %+v", projection)
	}
	text := projectionClusterText(got.Blocks)
	// Deep co-primary layers survive: dep-1 sits at trunk depth 10 in the detail
	// table (the lossless surface — rows are never capped).
	for _, want := range []string{"dep-1 / sleep_wait", "深度10"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expanded projection should keep deep typed evidence %q:\n%s", want, text)
		}
	}
	// The tree trunk display stays bounded via the omitted marker while every
	// row remains in the detail table.
	if !strings.Contains(projection.Text, "省略2个链路节点") {
		t.Fatalf("deep trunk should compress its middle with an omitted marker:\n%s", projection.Text)
	}
	// Semantic classes across the 16-span bucket survive.
	for _, class := range []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile"} {
		if !strings.Contains(text, class) {
			t.Fatalf("expanded projection should keep semantic class %q:\n%s", class, text)
		}
	}
	detail := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_detail")
	if detail == nil || len(detail.Items) < 24 {
		t.Fatalf("detail table should preserve a wide typed trace surface, got %d rows: %+v", len(detail.Items), detail)
	}
	assertProjectionRowsCitationFree(t, detail)
}

func traceProjectionObservation(id, subject, object, value, cumulative string, rank int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Subject:         subject,
		Object:          object,
		Value:           value,
		Unit:            "ms",
		RichNotes: []string{
			fmt.Sprintf("rank=%d", rank),
			"tier=primary",
			"impact_ms=" + value,
			"cumulative_impact_ms=" + cumulative,
			"causality=on_wakeup_chain",
		},
		Confidence: 0.9,
	}
}

func answerBlockItemByID(items []types.AnswerBlockItem, id string) *types.AnswerBlockItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// projectionClusterBlock returns the block with the given id from the projection
// cluster (runtime_trace_causal_projection[/_wakeup/_sleep]).
func projectionClusterBlock(blocks []types.AnswerBlock, id string) *types.AnswerBlock {
	for i := range blocks {
		if blocks[i].ID == id {
			return &blocks[i]
		}
	}
	return nil
}

// projectionClusterText flattens every cell of the lead table plus the diagram
// bodies of the whole projection cluster into one string, so tests can assert on
// the block-cluster content without depending on exact row/column positions.
func projectionClusterText(blocks []types.AnswerBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if !strings.HasPrefix(blk.ID, "runtime_trace_causal_projection") {
			continue
		}
		b.WriteString(blk.Title)
		b.WriteByte('\n')
		b.WriteString(blk.Text)
		b.WriteByte('\n')
		for _, it := range blk.Items {
			if it.Label != "" {
				b.WriteString(it.Label)
				b.WriteString(": ")
			}
			if it.Text != "" {
				b.WriteString(it.Text)
				b.WriteByte('\n')
			}
			b.WriteString(strings.Join(it.Cells, " | "))
			b.WriteByte('\n')
		}
		if blk.Diagram != nil {
			b.WriteString(blk.Diagram.Body)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// assertProjectionRowsCitationFree checks the red-line invariant that every
// system-injected projection table row carries CitationRef=-1.
func assertProjectionRowsCitationFree(t *testing.T, blk *types.AnswerBlock) {
	t.Helper()
	for _, it := range blk.Items {
		if it.CitationRef != -1 {
			t.Fatalf("projection row must keep CitationRef=-1, got %d: %+v", it.CitationRef, it)
		}
	}
}

func mutationTraceQueryRuntimeToolResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:window#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact},
			Subject:         "app-100",
			Predicate:       "root_cause_primary",
			Object:          "runnable",
		}},
	}
}

func TestApplyAndPersistMutation_StampsReadReasoningGraphSummary(t *testing.T) {
	bus := newBusForMutationTest()
	bus.TraceID = "trace-read-summary"
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Intent: types.IntentTrace},
		EvidencePlan: types.EvidencePlan{
			RequiredFiles: []string{"pkg/handler.py"},
		},
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{ID: "n1", Type: types.NodeEvidence}}},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-handler",
			Kind:            types.EvidenceMechanism,
			Scope:           types.ScopeLine,
			Source:          "pkg/handler.py",
			LineStart:       12,
			Subject:         "Handler",
			Predicate:       "dispatches",
			Object:          "SubAgent",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"pkg/handler.py"},
			SupportedPaths:      []string{"pkg/handler.py"},
			OwnerSupportedPaths: []string{"pkg/handler.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:         "pkg/handler.py",
				Kind:         types.SourceLocalizationAnchorGroundedEvidence,
				Strength:     types.SourceLocalizationAnchorOwner,
				OwnerSymbol:  "Handler",
				AnchorSymbol: "Handler.dispatch",
			}},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "repo_map",
			Success:  true,
			RawRef:   "blob://repo-map-task",
			Observations: []types.ObservationRecord{{
				ID:        "repo_map:task#navigation:task_map",
				Origin:    types.AnswerEvidenceOriginCrossRepoIndex,
				Producer:  "repo_map",
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCrossRepoIndex, RawRef: "blob://repo-map-task"},
				Predicate: types.RepoMapNavigationObservationPredicate,
				Object:    string(types.RepoMapNavigationRouteTaskMap),
			}},
		}},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
		Citations: []types.Citation{{File: "pkg/handler.py", Line: 12}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.ReadReasoningGraph == nil {
		t.Fatalf("read reasoning graph not stamped: %+v", got)
	}
	graph := got.ReadReasoningGraph
	if graph.GraphID != "trace-read-summary" ||
		graph.EventCount == 0 ||
		graph.ReadEventCount == 0 ||
		graph.EvidenceRefCount == 0 ||
		graph.AnswerBlockCount != 1 ||
		graph.CitationCount != 1 ||
		len(graph.EventRefs) == 0 {
		t.Fatalf("unexpected read reasoning graph summary: %+v", graph)
	}
}

func testRepoMapRoutePresent(routes []types.RepoMapNavigationRoute, want types.RepoMapNavigationRoute) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}

func TestApplyAndPersistMutation_StampsStructuralReadOwnerAnchorsFromLineEvidence(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := []byte(`class Documenter:
    def filter_members(self):
        has_doc = bool(doc)
`)
	if err := os.WriteFile(filepath.Join(repo, "pkg", "owner.py"), src, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	bus := newBusForMutationTest()
	bus.RepoRoot = repo
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			SourcePaths: []string{"pkg/owner.py"},
			EvidenceRefs: []types.WriteExplorationEvidenceRef{{
				ID:        "ev-line",
				Kind:      "relationship",
				Source:    "pkg/owner.py",
				LineStart: 3,
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "answer"}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.ReadOwnerAnchors) != 1 {
		t.Fatalf("read owner anchors not stamped from structural evidence: %+v", got)
	}
	if got.ReadOwnerAnchors[0].OwnerSymbol != "Documenter.filter_members" ||
		got.ReadOwnerAnchors[0].Strength != types.SourceLocalizationAnchorOwner {
		t.Fatalf("wrong structural owner anchor: %+v", got.ReadOwnerAnchors[0])
	}
}

// TestApplyAndPersistMutation_PartialPersistsDocAndSetsPatchFlag
// — patch path: NewPartialMutation → LastEmitFromPatch=true.
func TestApplyAndPersistMutation_PartialPersistsDocAndSetsPatchFlag(t *testing.T) {
	bus := newBusForMutationTest()
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
	}
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "new"},
		},
	}
	mutation := types.NewPartialMutation(patch)
	res, err := ApplyAndPersistMutation(bus, "test_patch", mutation, prev, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || got.Blocks[0].Text != "new" {
		t.Fatalf("patched doc not persisted correctly; got %+v", got)
	}
	if !bus.Mutable.LastEmitFromPatch() {
		t.Errorf("Partial mutation must set LastEmitFromPatch=true")
	}
}

func TestApplyAndPersistMutation_AcceptedDocDropsRejectedTextAttachments(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{
		{
			Kind:   types.AnswerDisplayAttachmentMarkdown,
			Body:   "stale rejected prose",
			Source: "emit_answer_document.rejected_payload",
			Reason: "rejected structured answer draft contained user-visible text",
		},
		{
			Kind:     types.AnswerDisplayAttachmentDiagram,
			Language: "mermaid",
			Body:     "flowchart TD\n  A --> B",
			Source:   "emit_answer_document.rejected_payload",
		},
	})
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
	}
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "accepted"},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_patch", types.NewPartialMutation(patch), prev, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 1 || attachments[0].Kind != types.AnswerDisplayAttachmentDiagram {
		t.Fatalf("accepted structured doc should drop stale text but preserve diagram attachments, got %+v", attachments)
	}
}

// TestApplyAndPersistMutation_DuplicateBlockIDRejected — merged-doc
// validation enforces unique block ids. Both paths get the same
// rejection message.
func TestApplyAndPersistMutation_DuplicateBlockIDRejected(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "x", Kind: types.BlockSummary, Text: "a"},
			{ID: "x", Kind: types.BlockSection, Text: "b"},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if res.Success {
		t.Fatalf("expected rejection on duplicate id; got Success=true")
	}
	if !strings.Contains(res.Summary, "duplicate id") {
		t.Errorf("rejection should name 'duplicate id'; got %q", res.Summary)
	}
}

func TestApplyAndPersistMutation_DedupesExactVisibleDuplicateBlocks(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/agent/agent.go", Line: 3502}},
		Blocks: []types.AnswerBlock{{
			ID:          "aggregate-member-set-subagent_agent_1",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "baseagent_explorer",
				Label:       "BaseAgent (explorer)",
				CitationRef: 0,
			}},
		}, {
			ID:          "aggregate-member-set-subagent_agent_1-2",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "baseagent_explorer_retry",
				Label:       "BaseAgent (explorer)",
				CitationRef: 0,
			}},
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected duplicate visible block normalization to accept doc; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 {
		t.Fatalf("expected exact visible duplicate block to be dropped, got %+v", got)
	}
	if got.Blocks[0].ID != "aggregate-member-set-subagent_agent_1" {
		t.Fatalf("dedupe should keep first block for stable ordering, got %+v", got.Blocks)
	}
}

func TestApplyAndPersistMutation_DedupesSemanticEquivalentPrincipalBlocks(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/agent/subagent_runtime.go", Line: 218}},
		Blocks: []types.AnswerBlock{{
			ID:          "aggregate-member-set-subagent_agent_1",
			Kind:        types.BlockOrderedList,
			Title:       "可调用 subagent 的 agent（1）",
			Text:        "主体答案已经完整。",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:            "orchestrator",
				Label:         "Orchestrator",
				Text:          "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CandidateRole: types.AnswerCandidateRoleAgent,
				CitationRef:   0,
			}},
		}, {
			ID:          "aggregate-member-set-subagent_agent_1_retry",
			Kind:        types.BlockOrderedList,
			Title:       "可调用 SubAgent 的组件",
			Text:        "修复 citation 后重新追加的同一载体。",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:            "orchestrator_retry",
				Label:         "`Orchestrator`",
				Text:          "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CandidateRole: types.AnswerCandidateRoleAgent,
				CitationRef:   0,
			}},
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected semantic duplicate block normalization to accept doc; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 {
		t.Fatalf("expected semantic-equivalent principal block to be dropped, got %+v", got)
	}
	if got.Blocks[0].ID != "aggregate-member-set-subagent_agent_1" {
		t.Fatalf("dedupe should keep first block for stable ordering, got %+v", got.Blocks)
	}
}

func TestApplyAndPersistMutation_DedupesSemanticEquivalentPrincipalBlocksFromPatch(t *testing.T) {
	bus := newBusForMutationTest()
	initial := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/agent/subagent_runtime.go", Line: 218}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "answer",
		}, {
			ID:          "aggregate-member-set-subagent_agent_1",
			Kind:        types.BlockOrderedList,
			Title:       "可调用 subagent 的 agent（1）",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:            "orchestrator",
				Label:         "Orchestrator",
				Text:          "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CandidateRole: types.AnswerCandidateRoleAgent,
				CitationRef:   0,
			}},
		}},
	}
	if res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(initial), nil, time.Now()); err != nil || !res.Success {
		t.Fatalf("initial persist failed: res=%+v err=%v", res, err)
	}
	prev := bus.Mutable.AnswerDocumentV2()
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"summary", "aggregate-member-set-subagent_agent_1"},
		AddBlocks: []types.AnswerBlock{{
			ID:          "aggregate-member-set-subagent_agent_1_retry",
			Kind:        types.BlockOrderedList,
			Title:       "重复的可调用 subagent agent",
			Text:        "不同标题和说明，但同一 principal item/citation carrier。",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:            "orchestrator_retry",
				Label:         "Orchestrator",
				Text:          "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CandidateRole: types.AnswerCandidateRoleAgent,
				CitationRef:   0,
			}},
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_patch", types.NewPartialMutation(patch), prev, time.Now())
	if err != nil {
		t.Fatalf("patch apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected semantic duplicate patch normalization to accept doc; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("expected summary + first principal block only, got %+v", got)
	}
	if got.Blocks[1].ID != "aggregate-member-set-subagent_agent_1" {
		t.Fatalf("patch dedupe should keep original principal block, got %+v", got.Blocks)
	}
}

func TestApplyAndPersistMutation_DoesNotDedupeSameLabelDifferentCitation(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "pkg/first.go", Line: 12},
			{File: "pkg/second.go", Line: 12},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "first",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "native_add_first",
				Label:       "native_add",
				CitationRef: 0,
			}},
		}, {
			ID:          "second",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "native_add_second",
				Label:       "native_add",
				CitationRef: 1,
			}},
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected distinct source locations to be accepted; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("same label at different citation anchors must remain distinct, got %+v", got)
	}
}

func TestApplyAndPersistMutation_DoesNotSemanticallyDedupeNonPrincipalBlocks(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/agent/subagent_runtime.go", Line: 218}},
		Blocks: []types.AnswerBlock{{
			ID:    "support_a",
			Kind:  types.BlockBulletList,
			Title: "support a",
			Text:  "first support block",
			Items: []types.AnswerBlockItem{{
				Label:       "Orchestrator",
				Text:        "support detail",
				CitationRef: 0,
			}},
		}, {
			ID:    "support_b",
			Kind:  types.BlockBulletList,
			Title: "support b",
			Text:  "second support block",
			Items: []types.AnswerBlockItem{{
				Label:       "Orchestrator",
				Text:        "support detail",
				CitationRef: 0,
			}},
		}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected non-principal blocks to be accepted; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("non-principal blocks must not be semantically deduped, got %+v", got)
	}
}

func TestRememberRejectedAnswerDocumentDraft_DedupesSemanticEquivalentPrincipalBlocks(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "internal/agent/subagent_runtime.go", Line: 218}},
		Blocks: []types.AnswerBlock{{
			ID:          "aggregate-member-set-subagent_agent_1",
			Kind:        types.BlockOrderedList,
			Title:       "可调用 subagent 的 agent（1）",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				Label:       "Orchestrator",
				Text:        "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CitationRef: 0,
			}},
		}, {
			ID:          "aggregate-member-set-subagent_agent_1_retry",
			Kind:        types.BlockOrderedList,
			Title:       "repaired duplicate carrier",
			Text:        "不同说明，同一载体。",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				Label:       "Orchestrator",
				Text:        "通过 SubAgentRuntime.Run 调度 SubAgent。",
				CitationRef: 0,
			}},
		}},
	}

	rememberRejectedAnswerDocumentDraft(bus, doc)
	got := bus.Mutable.LastRejectedAnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 {
		t.Fatalf("expected rejected draft to share semantic duplicate normalization, got %+v", got)
	}
	if got.Blocks[0].ID != "aggregate-member-set-subagent_agent_1" {
		t.Fatalf("rejected draft dedupe should keep first block, got %+v", got.Blocks)
	}
}

// TestApplyAndPersistMutation_DiagramWithNilPayloadRejected — diagram
// kind requires a non-nil Diagram payload.
func TestApplyAndPersistMutation_DiagramWithNilPayloadRejected(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "d", Kind: types.BlockDiagram, Diagram: nil},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if res.Success {
		t.Fatalf("expected rejection; got Success=true")
	}
	if !strings.Contains(res.Summary, "diagram") {
		t.Errorf("rejection should name 'diagram'; got %q", res.Summary)
	}
}

func TestApplyAndPersistMutation_DiagramPayloadOnSectionNormalizesKind(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "s",
			Kind: types.BlockSection,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: "flowchart TD\n  A --> B",
			},
		}},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if !res.Success {
		t.Fatalf("expected diagram discriminator repair to succeed; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 {
		t.Fatalf("persisted doc missing: %+v", got)
	}
	if got.Blocks[0].Kind != types.BlockDiagram {
		t.Fatalf("persisted kind = %q, want diagram", got.Blocks[0].Kind)
	}
	if got.Blocks[0].Diagram == nil || !strings.Contains(got.Blocks[0].Diagram.Body, "A --> B") {
		t.Fatalf("diagram payload should be preserved, got %+v", got.Blocks[0].Diagram)
	}
}

func TestApplyAndPersistMutation_CanonicalizesSummaryLeadBlock(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:    "items",
				Kind:  types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "i1", Label: "A"}},
			},
			{ID: "caveat", Kind: types.BlockCaveat, Text: "scope"},
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit",
		types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected summary-order canonicalization to accept doc; got %+v", res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 3 {
		t.Fatalf("merged doc not persisted; got %+v", got)
	}
	if got.Blocks[0].ID != "summary" || got.Blocks[1].ID != "items" || got.Blocks[2].ID != "caveat" {
		t.Fatalf("summary should move to lead while preserving detail order, got block ids: %v",
			[]string{got.Blocks[0].ID, got.Blocks[1].ID, got.Blocks[2].ID})
	}
}

// TestApplyAndPersistMutation_FullAndPatchProduceByteIdenticalMerged
// — same logical doc reached via full vs patch paths produces an
// identical merged AnswerDocumentV2 in MutableState.
func TestApplyAndPersistMutation_FullAndPatchProduceByteIdenticalMerged(t *testing.T) {
	target := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "the answer"},
		},
		Citations: []types.Citation{{File: "foo.go", Line: 1}},
	}

	// Full path.
	busFull := newBusForMutationTest()
	if _, err := ApplyAndPersistMutation(busFull, "full",
		types.NewReplaceAllMutation(target), nil, time.Now()); err != nil {
		t.Fatalf("full apply: %v", err)
	}

	// Patch path: prev has different text; patch replaces.
	busPatch := newBusForMutationTest()
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
		Citations: []types.Citation{{File: "foo.go", Line: 1}},
	}
	busPatch.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "the answer"},
		},
	}
	if _, err := ApplyAndPersistMutation(busPatch, "patch",
		types.NewPartialMutation(patch), prev, time.Now()); err != nil {
		t.Fatalf("patch apply: %v", err)
	}

	full := busFull.Mutable.AnswerDocumentV2()
	patched := busPatch.Mutable.AnswerDocumentV2()
	if full == nil || patched == nil {
		t.Fatalf("docs not persisted")
	}
	if full.Blocks[0].Text != patched.Blocks[0].Text {
		t.Errorf("merged doc text differs:\n  full=%q\n  patch=%q",
			full.Blocks[0].Text, patched.Blocks[0].Text)
	}
}

// TestBuildAnswerDocumentSemanticContractDescription_SharedBetweenTools
// — sanity check the SST helper renders content both Description()
// outputs include.
func TestBuildAnswerDocumentSemanticContractDescription_SharedBetweenTools(t *testing.T) {
	body := BuildAnswerDocumentSemanticContractDescription()
	if !strings.Contains(body, "Block kinds") || !strings.Contains(body, "summary") || !strings.Contains(body, "ordered_list") {
		t.Errorf("body missing canonical block-kind list; got: %.200s...", body)
	}
	if !strings.Contains(body, "claim_form values") {
		t.Errorf("body missing claim_form enum guidance")
	}
	full := (&EmitAnswerDocument{}).Description()
	patch := (&EmitAnswerDocumentPatch{}).Description()
	if !strings.Contains(full, body) {
		t.Errorf("full Description() missing the shared SST body")
	}
	if !strings.Contains(patch, body) {
		t.Errorf("patch Description() missing the shared SST body")
	}
	for _, surface := range []struct {
		name string
		text string
	}{
		{name: "shared description", text: body},
		{name: "full description", text: full},
		{name: "patch description", text: patch},
		{name: "full parameters", text: string((&EmitAnswerDocument{}).Parameters())},
	} {
		for _, forbidden := range []string{"citation_ref=-1", "citation_ref = -1", "-1 / omitted", "or -1 / omitted"} {
			if strings.Contains(surface.text, forbidden) {
				t.Fatalf("%s should not teach no-citation sentinel %q:\n%s", surface.name, forbidden, surface.text)
			}
		}
	}
}

// TestApplyAndPersistMutation_SummaryReportsMutationKind — ToolResult
// Summary names the mutation surface so operators can grep telemetry.
func TestApplyAndPersistMutation_SummaryReportsMutationKind(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	res, _ := ApplyAndPersistMutation(bus, "tool",
		types.NewReplaceAllMutation(doc), nil, time.Now())
	if !strings.Contains(res.Summary, "replace_all") {
		t.Errorf("Summary should name mutation kind; got %q", res.Summary)
	}
}

func TestAnswerDocumentMutationRepair_MapsDeterministicPatchRejects(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantFields []string
		wantHint   string
	}{
		{
			name:       "citation mode conflict",
			err:        fmt.Errorf("patch: replace_citations and append_citations are mutually exclusive (contract invariant 5); set exactly one"),
			wantCode:   "answer_doc_patch_citation_mode_conflict",
			wantFields: []string{"replace_citations", "append_citations"},
			wantHint:   "exactly one citation-pool operation",
		},
		{
			name:       "existing block added",
			err:        fmt.Errorf("patch: add_blocks[%q] already exists in previous emit (use replace_blocks to modify)", "s1"),
			wantCode:   "answer_doc_patch_existing_block",
			wantFields: []string{"add_blocks", "replace_blocks", "unchanged_block_ids"},
			wantHint:   "already exists",
		},
		{
			name:       "replace citations preserves old cited block",
			err:        fmt.Errorf("patch: replace_citations cannot preserve citation-bearing block %q; replace/remove that block too, use append_citations, or re-emit a full emit_answer_document so every citation_ref is renumbered against the new pool", "list1"),
			wantCode:   "answer_doc_patch_replace_citations_with_preserved_blocks",
			wantFields: []string{"replace_citations", "append_citations", "replace_blocks", "remove_block_ids"},
			wantHint:   "citation_ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repair := answerDocumentMutationRepair(tt.err)
			if repair == nil {
				t.Fatalf("expected structured repair for %v", tt.err)
			}
			if repair.Code != tt.wantCode {
				t.Fatalf("repair code = %q, want %q", repair.Code, tt.wantCode)
			}
			for _, field := range tt.wantFields {
				if !mutationRepairStringSliceContains(repair.Fields, field) {
					t.Fatalf("repair fields missing %q: %+v", field, repair.Fields)
				}
			}
			if !strings.Contains(repair.Hint, tt.wantHint) {
				t.Fatalf("repair hint %q does not contain %q", repair.Hint, tt.wantHint)
			}
		})
	}
}

func TestAnswerDocumentMutationRepair_UnknownErrorStaysUnstructured(t *testing.T) {
	if repair := answerDocumentMutationRepair(fmt.Errorf("patch: unrelated validation failure")); repair != nil {
		t.Fatalf("unknown mutation error should not fabricate repair metadata: %+v", repair)
	}
}

func mutationRepairStringSliceContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
