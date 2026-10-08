package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func measurementMemberPublicContext(t *testing.T) (*types.AgentContext, types.RuntimeMeasurementTable) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "measured.systrace")
	body := "idle-0 (0) [000] .... 0.000000: cpu_idle: state=0 cpu_id=0\n" +
		"idle-0 (0) [000] .... 0.000000: cpu_frequency: state=1000000 cpu_id=0\n" +
		"idle-0 (0) [001] .... 0.000000: cpu_idle: state=1 cpu_id=1\n" +
		"idle-0 (0) [001] .... 0.000000: cpu_frequency: state=800000 cpu_id=1\n" +
		"idle-0 (0) [000] .... 0.040000: cpu_frequency: state=2000000 cpu_id=0\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	// Run-entry resolution and the query must refer to the same physical
	// source (macOS TempDir may otherwise spell /private/var as /var).
	var err error
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 0.0, .04
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		Predicates:                  types.SemanticPredicates{HasPerMemberTable: true},
		ExternalObservationPolicy:   &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"no code"}},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0..40ms"},
		RequestedAnswerDimensions:   &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Label: "combinations", Required: true}}},
	}
	mu := types.NewMutableState("Show combinations within 0..40ms, no code")
	mu.SetRequestModel(rm)
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("native query: %v %s", err, result.Summary)
	}
	mu.AppendDispatchToolResult(result)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: mu.DispatchToolResults()})
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementDistribution {
			return ctx, table
		}
	}
	t.Fatal("native producer did not publish distribution")
	return nil, types.RuntimeMeasurementTable{}
}

func replaceMeasurementMemberPublication(t *testing.T, ctx *types.AgentContext, mutate func(*types.ObservationRecord, *types.RuntimeMeasurementPublication)) {
	t.Helper()
	results := ctx.Mutable.DispatchToolResults()
	for ri := range results {
		for oi := range results[ri].Observations {
			r := &results[ri].Observations[oi]
			p, ok := types.DecodeRuntimeMeasurementPublication(*r)
			if !ok {
				continue
			}
			mutate(r, &p)
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			for ni, note := range r.RichNotes {
				if strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
					r.RichNotes[ni] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
				}
			}
		}
	}
	mu := types.NewMutableState("member coverage negative")
	mu.SetRequestModel(ctx.AnalysisIR.RequestModel)
	for _, result := range results {
		mu.AppendDispatchToolResult(result)
	}
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.Mutable = mu
}

func TestRuntimeMeasurementMemberSetPublicRejectsUnsupportedCoverage(t *testing.T) {
	for _, name := range []string{"not_selected", "summary", "timeline", "stale", "partial", "model_inference", "no_preflight", "narrow_window", "named_target", "independent_source", "low_score_bound_source", "duplicate_for_two", "missing_facet"} {
		t.Run(name, func(t *testing.T) {
			ctx, table := measurementMemberPublicContext(t)
			selector := map[string]any{"observation_id": table.ObservationID, "view": string(table.View)}
			selected := []any{selector}
			switch name {
			case "not_selected":
				selected = nil
			case "no_preflight":
				ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{}
			case "summary", "timeline":
				selector["view"] = name
			case "stale":
				selector["observation_id"] = "unpublished"
			case "partial":
				replaceMeasurementMemberPublication(t, ctx, func(_ *types.ObservationRecord, p *types.RuntimeMeasurementPublication) {
					for i := range p.Tables {
						if p.Tables[i].MemberSet != nil {
							p.Tables[i].MemberSet.Complete = false
							p.Tables[i].MemberSet.TotalRows++
						}
					}
				})
			case "model_inference":
				replaceMeasurementMemberPublication(t, ctx, func(r *types.ObservationRecord, _ *types.RuntimeMeasurementPublication) {
					r.ClaimAuthority = types.ObservationClaimAuthorityModelInference
				})
			case "narrow_window":
				replaceMeasurementMemberPublication(t, ctx, func(r *types.ObservationRecord, p *types.RuntimeMeasurementPublication) {
					r.SourceRef.QueryWindowEndTs = .03
					p.Source = r.SourceRef
				})
			case "named_target":
				ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "target process"}
			case "independent_source", "low_score_bound_source":
				ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{1}}}
				if name == "low_score_bound_source" {
					ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints[0].Confidence = 0
				}
			case "duplicate_for_two":
				selected = append(selected, selector)
				ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions = append(ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionMemberSet, Label: "another population", Required: true})
			}
			ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			// Agent plans are immutable within a stage; this is a new request
			// snapshot after the negative's typed contract change.
			ctx = &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight}
			params, _ := json.Marshal(map[string]any{"reason": "Selected measurement", "confidence": "high", "result_kind": "resolved", "runtime_measurement_member_sets": selected})
			result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil {
				t.Fatal(err)
			}
			if name == "missing_facet" {
				if !result.Success || !ctx.Mutable.IsInvestigationComplete() {
					t.Fatalf("valid completion: %s", result.Summary)
				}
			} else if ctx.Mutable.IsInvestigationComplete() || len(ctx.Mutable.InvestigationMeasurementMemberSets()) != 0 {
				t.Fatalf("unsupported coverage completed: %s", result.Summary)
			}
			if name == "stale" || name == "not_selected" {
				return
			}
			block := map[string]any{"id": "measured", "kind": "table", "runtime_measurement": selector, "facet_ids": []string{"member_set"}}
			if name == "missing_facet" {
				delete(block, "facet_ids")
			}
			blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "Partial display remains available."}, block}
			if name == "duplicate_for_two" {
				blocks = append(blocks, map[string]any{"id": "duplicate", "kind": "table", "runtime_measurement": selector, "facet_ids": []string{"member_set"}})
			}
			params, _ = json.Marshal(map[string]any{"blocks": blocks})
			result, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
			if err != nil || !result.Success {
				t.Fatalf("display capability regressed: %v %s", err, result.Summary)
			}
			missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2())
			if len(missing) == 0 {
				t.Fatalf("unsupported/duplicate population claimed requested coverage: dims=%+v count=%d blocks=%+v", types.BuildAnswerSemanticViewForAgentContext(ctx).Presentation.RequestedDimensions, answerDocumentMemberSetOrExactSourceInventoryPayloadBlockCount(ctx, ctx.Mutable.AnswerDocumentV2()), ctx.Mutable.AnswerDocumentV2().Blocks)
			}
			hint := requestedAnswerDimensionCoverageHint(ctx, missing, "en")
			if name != "missing_facet" && !strings.Contains(hint, "already carries the member marker") {
				t.Fatalf("advisory requests repeated no-op marker: %s", hint)
			}
			if name == "missing_facet" {
				block["facet_ids"] = []string{"member_set"}
				params, _ = json.Marshal(map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead"}})
				result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
				if err != nil || !result.Success || len(missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2())) != 0 {
					t.Fatalf("facet patch: %v %s", err, result.Summary)
				}
			}
		})
	}
}

func TestRuntimeMeasurementMemberSetPublicSchemaPublishesExactChoices(t *testing.T) {
	ctx, table := measurementMemberPublicContext(t)
	ctx.Stage = types.StageExplore
	initial := llm.ToolSchema{Name: "emit_investigation_complete", Description: (&tool.EmitInvestigationComplete{}).Description(), Parameters: (&tool.EmitInvestigationComplete{}).Parameters()}
	schemas := (&explorerEvaluator{}).FilterToolSchemas(ctx, []llm.ToolSchema{initial})
	if len(schemas) != 1 || schemas[0].Description != initial.Description {
		t.Fatal("schema routing/description changed")
	}
	var parsed struct {
		Properties map[string]struct {
			Items struct {
				Enum []types.AnswerRuntimeMeasurementReceipt `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemas[0].Parameters, &parsed); err != nil {
		t.Fatal(err)
	}
	choices := parsed.Properties["runtime_measurement_member_sets"].Items.Enum
	if len(choices) != 1 || choices[0].ObservationID != table.ObservationID || choices[0].View != table.View {
		t.Fatalf("unqualified/cross-product choices: %+v", choices)
	}
	for _, invalid := range []bool{false, true} {
		choice := choices[0]
		if invalid {
			choice.View = types.RuntimeMeasurementTimeline
		}
		payload, _ := json.Marshal(map[string]any{"reason": "Measured population", "confidence": "high", "result_kind": "resolved", "runtime_measurement_member_sets": []types.AnswerRuntimeMeasurementReceipt{choice}})
		err := toolparam.Validate(payload, schemas[0].Parameters)
		if (err != nil) != invalid {
			t.Fatalf("schema exact-choice validation invalid=%t err=%v", invalid, err)
		}
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.RequestedScope = types.RuntimeArtifactScopeFullArtifact
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = nil
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = nil
	schemas = (&explorerEvaluator{}).FilterToolSchemas(ctx, []llm.ToolSchema{initial})
	if explorerSchemaTopLevelPropertyForTest(t, schemas[0], "runtime_measurement_member_sets") {
		t.Fatal("unknown full-capture coverage published as complete choice")
	}
}

func TestRuntimeMeasurementMemberSetPublicMultiWindow(t *testing.T) {
	ctx, existing := measurementMemberPublicContext(t)
	start, middle, end := 0.0, .02, .04
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{
		{TimeStart: &start, TimeEnd: &middle, SourceQuote: "first"}, {TimeStart: &middle, TimeEnd: &end, SourceQuote: "second"},
	}}
	ctx = &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	path := filepath.Join(ctx.RepoRoot, "measured.systrace")
	for _, bounds := range [][2]float64{{start, middle}, {middle, end}} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": bounds[0], "time_end": bounds[1]})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("query: %v %s", err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	var choices []types.AnswerRuntimeMeasurementReceipt
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementDistribution {
			if table.ObservationID == existing.ObservationID {
				t.Fatal("whole envelope entered disjoint exact window choices")
			}
			choices = append(choices, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		}
	}
	if len(choices) != 2 {
		t.Fatalf("choices=%d", len(choices))
	}
	complete := func(selected []types.AnswerRuntimeMeasurementReceipt) types.ToolResult {
		params, _ := json.Marshal(map[string]any{"reason": "Both requested ranges have selected populations", "confidence": "high", "result_kind": "resolved", "runtime_measurement_member_sets": selected})
		result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if r := complete(choices[:1]); r.Success || ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("single requested window waived its sibling")
	}
	if r := complete(choices); !r.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("exact two-window handoff: %s", r.Summary)
	}
	if len(ctx.Mutable.InvestigationMeasurementMemberSets()) != 2 {
		t.Fatal("multi-window choices not retained")
	}
}

func TestRuntimeMeasurementMemberSetPublicEnumeration(t *testing.T) {
	ctx, table := measurementMemberPublicContext(t)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentEnumerate
	ctx.AnalysisIR.RequestModel.Predicates.IsCategoryEnumeration = true
	ctx = &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	params, _ := json.Marshal(map[string]any{"reason": "Enumerated measured combinations", "confidence": "high", "result_kind": "resolved", "runtime_measurement_member_sets": []types.AnswerRuntimeMeasurementReceipt{{ObservationID: table.ObservationID, View: table.View}}})
	result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("enumeration completion: %v %s", err, result.Summary)
	}
}

func TestRuntimeMeasurementMemberSetPublicCompletionEmitAdvisoryPatch(t *testing.T) {
	ctx, table := measurementMemberPublicContext(t)
	selector := map[string]any{"observation_id": table.ObservationID, "view": string(table.View)}
	params, _ := json.Marshal(map[string]any{"reason": "Measured combinations are in the selected native table", "confidence": "high", "result_kind": "resolved", "runtime_measurement_member_sets": []any{selector}})
	result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("native member handoff: %v %s", err, result.Summary)
	}
	if len(ctx.Mutable.StableInvestigationAggregateFacts()) != 0 {
		t.Fatal("member receipt copied into model aggregate")
	}
	block := map[string]any{"id": "measured", "kind": "table", "facet_ids": []string{"member_set"}, "runtime_measurement": selector}
	params, _ = json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "The selected table lists measured combinations."}, block}})
	result, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
	if err != nil || !result.Success {
		t.Fatalf("emit: %v %s", err, result.Summary)
	}
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 0 {
		t.Fatalf("bound visible member set ignored: %+v", missing)
	}
	e := &answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); sig.HintRequested {
		t.Fatalf("already-covered answer got redundant patch: %s", sig.Hint)
	}
	params, _ = json.Marshal(map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead"}})
	result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
	if err != nil || !result.Success {
		t.Fatalf("patch: %v %s", err, result.Summary)
	}
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 0 {
		t.Fatal("patch dropped member coverage")
	}
	prompt := renderAnswerDocRuntimeMeasurementChoices(ctx)
	if !strings.Contains(prompt, "Selected during investigation") {
		t.Fatal("completion selector choice lost before finalizer")
	}
}
