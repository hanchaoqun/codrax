package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	projection := got.Blocks[1]
	if projection.ID != "runtime_trace_causal_projection" ||
		projection.Kind != types.BlockOrderedList ||
		projection.SurfaceRole != types.SurfacePrincipal {
		t.Fatalf("missing principal trace causal projection block: %+v", projection)
	}
	if len(projection.ClaimUses) != 1 || projection.ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("projection must stay in external-observation lane: %+v", projection.ClaimUses)
	}
	if len(projection.Items) < 2 {
		t.Fatalf("projection items missing: %+v", projection.Items)
	}
	if projection.Items[0].Label != "主根因" ||
		!strings.Contains(projection.Items[0].Text, "threadpool-400 -> io_wait") ||
		!strings.Contains(projection.Items[0].Text, "cumulative_impact=11.000ms") {
		t.Fatalf("projection should select highest impact root cause first: %+v", projection.Items[0])
	}
	if projection.Items[1].Label != "因果链路" ||
		!strings.Contains(projection.Items[1].Text, "threadpool-400 -> network-300 -> cookie-200 -> app-100") {
		t.Fatalf("projection should preserve wakeup path: %+v", projection.Items[1])
	}
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
