package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc271LocationPayload(t *testing.T) (*types.BusContext, map[string]any) {
	t.Helper()
	const request = "List recorded events, current implementation locations and inspection progress from captures; also list all public functions when requested."
	bus := sourceInventoryRuntimeAnalysisContext(t)
	bus.Mutable = types.NewMutableState(request)
	bus.TurnRouteHint = types.TurnRouteHint{}
	p := hmc257SourceDomainPayload(t, request)
	delete(p, "current_source_explanation_profile")
	p["question_kind"], p["predicate_axis"] = "mechanism", "flow"
	p["source_scope_profile"] = map[string]any{"requested_scope": "all", "source_quotes": []string{"captures"}, "confidence": .9}
	p["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .9, "dimensions": []any{
		map[string]any{"label": "events", "role": "member_set", "source_quote": "recorded events", "required": true, "index": 1},
		map[string]any{"label": "locations", "role": "source_location", "source_quote": "current implementation locations", "required": true, "index": 2},
		map[string]any{"label": "progress", "role": "stage_or_workflow", "source_quote": "inspection progress", "required": true, "index": 3},
	}}
	return bus, p
}

func TestHMC271PublicAnalysisLocationDoesNotCreateSourceUniverse(t *testing.T) {
	for _, declaration := range []string{"omitted", "false", "locate"} {
		t.Run(declaration, func(t *testing.T) {
			bus, p := hmc271LocationPayload(t)
			if declaration != "omitted" {
				p["current_source_explanation_profile"] = map[string]any{
					"is_current_source_explanation_requested": declaration == "locate",
					"modes": []string{"locate_current_code"}, "source_quotes": []string{"current implementation locations"}, "confidence": .9,
				}
			}
			out := hmc256EmitIdentity(t, bus, p)
			if !out.Success {
				t.Fatalf("location obligation rejected instead of preserved: %s", out.Summary)
			}
			rm := bus.Mutable.RequestModel()
			if rm.SourceInventoryProfile.Active() {
				t.Errorf("location attribute expanded runtime members into source declarations: %+v", rm.SourceInventoryProfile)
			}
			if got := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, types.TurnRouteHint{}); got != types.RuntimeSourceRequirementPrecise {
				t.Fatalf("required current location lost its precise contract: %s", got)
			}
			if declaration == "omitted" && rm.CurrentSourceExplanationProfile != nil {
				t.Fatal("omitted declaration was silently made false")
			}
			if len(rm.RequestedAnswerDimensions.Dimensions) != 3 || !rm.RuntimeQuestionProfile.BoundedFactSet() {
				t.Fatal("requested runtime scope or visible dimensions changed")
			}
		})
	}
}

func TestHMC271PublicAnalysisExplicitSourcePrincipalSurvivesRuntime(t *testing.T) {
	bus, p := hmc271LocationPayload(t)
	p["question_kind"] = "enumeration"
	delete(p, "predicate_axis")
	p["source_inventory_profile"] = map[string]any{
		"is_source_inventory": true, "target_roles": []string{"function"},
		"requested_fields": []string{"name", "location"}, "source_quotes": []string{"all public functions"}, "confidence": .95,
	}
	out := hmc256EmitIdentity(t, bus, p)
	if !out.Success {
		t.Fatalf("explicit mixed source inventory rejected: %s", out.Summary)
	}
	rm := bus.Mutable.RequestModel()
	if !rm.SourceInventoryProfile.Active() || rm.SourceInventoryProfile.DeclarationOrigin != types.SourceInventoryDeclarationModelProvided ||
		!types.SourceInventoryPrincipalNavigationActive(*rm) || !types.SourceInventoryPrincipalAuthorityActive(*rm) {
		t.Fatalf("explicit source members lost their own authority: %+v", rm.SourceInventoryProfile)
	}
}

func TestHMC271PublicCompletionLocationDoesNotRequireSourceTransfer(t *testing.T) {
	for _, declaration := range []string{"omitted", "false", "locate", "flow"} {
		t.Run(declaration, func(t *testing.T) {
			ctx := flowOperationCompletionContext(nil)
			rm := &ctx.AnalysisIR.RequestModel
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}
			rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{Index: 1, Label: "current location", Role: types.RequestedAnswerDimensionSourceLocation, Required: true}}}
			if declaration != "omitted" {
				mode := types.CurrentSourceExplanationLocateCurrentCode
				if declaration == "flow" {
					mode = types.CurrentSourceExplanationTraceCurrentFlow
				}
				rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: declaration != "false", Modes: []types.CurrentSourceExplanationMode{mode},
					SourceQuotes: []string{"current implementation"}, Confidence: 1,
				}
			}
			before, _ := json.Marshal(rm)
			out, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
			if err != nil {
				t.Fatal(err)
			}
			flowRepair := out.Repair != nil && out.Repair.Code == "flow_operation_carrier_evidence"
			if flowRepair != (declaration == "flow") {
				t.Fatalf("source location and source transfer were conflated: declaration=%s summary=%s repair=%+v", declaration, out.Summary, out.Repair)
			}
			if declaration != "flow" && strings.Contains(out.Summary, "operation-level transfer") {
				t.Fatalf("location-only question received transfer teaching: %s", out.Summary)
			}
			if got := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, types.TurnRouteHint{}); got != types.RuntimeSourceRequirementPrecise {
				t.Fatalf("flow exception waived current source evidence: %s", got)
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("completion changed the model-owned request")
			}
		})
	}
}

func TestHMC271RuntimeLocationCannotReviveInventoryConsumers(t *testing.T) {
	for _, origin := range []string{"absent", "inactive", "synthesized", "model_provided"} {
		t.Run(origin, func(t *testing.T) {
			ctx := sourceInventoryRequestedUniverseTestContext(nil, nil)
			rm := &ctx.AnalysisIR.RequestModel
			rm.Intent, rm.Predicates.HasPerMemberTable = types.IntentEnumerate, true
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}
			rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionSourceLocation, Required: true}}}
			rm.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "all recorded members"}
			rm.SourceInventoryProfile = nil
			if origin != "absent" {
				rm.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true,
					DeclarationOrigin: types.SourceInventoryDeclarationOrigin(origin), TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					RequestedFields: []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
					SourceQuotes:    []string{"all current source types"}, Confidence: 1}
				if origin == "inactive" {
					rm.SourceInventoryProfile.IsSourceInventory = false
					rm.SourceInventoryProfile.DeclarationOrigin = types.SourceInventoryDeclarationModelProvided
				}
			}
			rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "internal/types/evidence.go", Confidence: 1}}
			ctx.AnalysisIR.EvidencePlan.RequiredFiles = []string{"internal/types/evidence.go"}
			wantInventory := origin == "model_provided"
			before, _ := json.Marshal(rm)
			if got := types.SourceInventoryRequiredFileCoverageShape(*rm); got != wantInventory {
				t.Errorf("required-file universe=%v want %v", got, wantInventory)
			}
			ctx.Mutable.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{Active: true, AdvisoryOnly: true,
				Provenance: []string{"request_traits:typed_source_enumeration_query"},
				Sets:       []types.SourceInventoryAdvisorySet{{Role: types.AnswerCandidateRoleType}}})
			if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking != wantInventory {
				t.Errorf("advisory recreated a source-universe execution gate: %+v", gap)
			}
			ctx.Mutable.SetSourceInventoryObservation(sourceInventoryExactRequiredFileObservation())
			facts := sourceInventoryExactRequiredFileFacts(true)
			gap := SourceInventoryRequestedFileCoverageGap(ctx, facts)
			if gap.Blocking != wantInventory {
				t.Errorf("complete file lens recreated a source-universe completion gate: %+v", gap)
			}
			authority := BuildSourceInventoryAnswerPreEmitAuthority(ctx, facts)
			if !wantInventory && (authority.RequestedFileCoverageGap.Blocking || authority.BestUniverseGap.Blocking) {
				t.Errorf("answer authority promoted support-only rows: %+v", authority)
			}
			if got := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, types.TurnRouteHint{}); got != types.RuntimeSourceRequirementPrecise ||
				!rm.CurrentSourceLaneDecision().RequiresCurrentSource() || len(ctx.AnalysisIR.EvidencePlan.RequiredFiles) != 1 {
				t.Fatal("inventory boundary removed independent current-source location/read responsibility")
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("consumer boundary changed the accepted request")
			}
		})
	}
}
