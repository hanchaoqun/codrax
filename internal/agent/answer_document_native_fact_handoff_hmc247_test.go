package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeFactHandoffContextHMC247(t *testing.T, sampling bool) *types.AgentContext {
	t.Helper()
	if !sampling {
		ctx, _, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeCausalDiagnosis, "")
		ctx.AnalysisIR.RequestModel.Intent = types.IntentReturnValue
		ctx.AnalysisIR.RequestModel.Predicates.IsScalarAnswer = true
		return ctx
	}
	ctx := samplingSelectionAnalysisContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = false
	ctx.AnalysisIR.RequestModel.Predicates.IsScalarAnswer = true
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	ctx.AnalysisIR.RequestModel.Intent = types.IntentReturnValue
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"perf_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !r.Success {
		t.Fatalf("native query: %v %s", err, r.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	return ctx
}

func TestHMC247NativeFactHandoffActualExplorerSchema(t *testing.T) {
	for _, mode := range []string{"sampling", "io", "no_contract", "stale_source"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, mode != "io")
			if mode == "no_contract" {
				ctx.Mutable = types.NewMutableState("no native observations")
			}
			if mode == "stale_source" {
				ta := ctx.Mutable.TurnAArtifacts()
				for i := range ta.ToolResults {
					for j := range ta.ToolResults[i].Observations {
						ta.ToolResults[i].Observations[j].SourceRef.Path += "-stale"
					}
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(*ta)
			}
			ctx = ctx.ShallowClone()
			ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
			reg := tool.NewRegistry()
			tool.RegisterDefaults(reg)
			reg.Register(&tool.EmitInvestigationComplete{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("capture native handoff schema")}
			sk := traceTeachingSkill(t, "explore-skill")
			sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete") // Orchestrator's evidence-tool injection.
			_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual explorer schema not captured: %v calls=%d", err, capture.calls)
			}
			for _, schema := range capture.tools {
				if schema.Name != "emit_investigation_complete" {
					continue
				}
				var root struct {
					Properties map[string]json.RawMessage `json:"properties"`
				}
				if err := json.Unmarshal(schema.Parameters, &root); err != nil {
					t.Fatal(err)
				}
				var reason struct {
					Description string `json:"description"`
				}
				if err := json.Unmarshal(root.Properties["reason"], &reason); err != nil {
					t.Fatal(err)
				}
				field, present := root.Properties["native_fact_handoff"]
				if mode == "no_contract" || mode == "stale_source" {
					if present {
						t.Fatal("unavailable native choices must not advertise the optional handoff")
					}
					var legacy struct {
						Properties map[string]json.RawMessage `json:"properties"`
					}
					json.Unmarshal((&tool.EmitInvestigationComplete{}).Parameters(), &legacy)
					var oldReason struct {
						Description string `json:"description"`
					}
					json.Unmarshal(legacy.Properties["reason"], &oldReason)
					if reason.Description != oldReason.Description {
						t.Fatal("unavailable handoff changed legacy reason teaching")
					}
					return
				}
				if !strings.Contains(reason.Description, "When native_fact_handoff is selected, reason is lifecycle rationale only") || !strings.Contains(reason.Description, "Otherwise use the following legacy reason contract:") {
					t.Fatal("current native choices lack conditional reason teaching")
				}
				var handoff struct {
					Properties struct {
						Measurements struct {
							Items struct {
								Enum []types.AnswerRuntimeMeasurementReceipt `json:"enum"`
							} `json:"items"`
						} `json:"runtime_measurements"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(field, &handoff); err != nil {
					t.Fatal(err)
				}
				choices := handoff.Properties.Measurements.Items.Enum
				if len(choices) == 0 || len(choices) > 32 {
					t.Fatalf("actual causal explorer schema lacks bounded exact native pairs: %d", len(choices))
				}
				contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
				for _, choice := range choices {
					if !types.BindRuntimeMeasurementReceipt(&choice, contract) || !choice.IsBound() {
						t.Fatalf("schema advertised unavailable pair: %+v", choice)
					}
				}
				return
			}
			t.Fatal("actual explorer omitted completion tool")
		})
	}
}

func TestHMC247NativeFactHandoffPublicFinalizer(t *testing.T) {
	for _, sampling := range []bool{true, false} {
		name := "io"
		if sampling {
			name = "sampling"
		}
		t.Run(name, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, sampling)
			var selections []map[string]any
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
				if table.View == types.RuntimeMeasurementSummary {
					selections = append(selections, map[string]any{"observation_id": table.ObservationID, "view": table.View})
					break
				}
			}
			if len(selections) == 0 {
				t.Fatal("missing native selector")
			}
			const lifecycle = "HMC247_LIFECYCLE_ONLY_COMPLETION_REASON"
			const interpretation = "HMC247_DIAGNOSIS_REMAINS_UNPROVEN; preserve source and on-chain follow-up."
			const independent = "HMC247_INDEPENDENT_NEGATIVE_AND_BUSINESS_CLUE"
			ta := ctx.Mutable.TurnAArtifacts()
			ta.InvestigationNotes = []string{lifecycle, independent}
			ctx.Mutable.SetTurnAArtifacts(*ta)
			raw, _ := json.Marshal(map[string]any{"reason": lifecycle, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": selections, "interpretation": interpretation}})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success || !ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("public typed completion: %v %s", err, r.Summary)
			}
			// Source/diagnostic obligations must not be silently dropped by the
			// new display handoff. The existing artifact-only suppression lane
			// has a separate negative test below.
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			ctx = ctx.ShallowClone() // Real finalizer dispatch starts a fresh local cache.
			extractCtx := ctx.ShallowClone()
			extractCtx.AgentName, extractCtx.Stage = types.AgentExtractor, types.StageExtract
			reg := tool.NewRegistry()
			tool.RegisterDefaults(reg)
			capture := &traceTeachingCaptureLLM{stop: errors.New("capture native handoff extraction")}
			_, err = NewExtractorAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(extractCtx, traceTeachingSkill(t, "extract-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual extractor not captured: %v calls=%d", err, capture.calls)
			}
			var extracted strings.Builder
			for _, m := range capture.messages {
				extracted.WriteString(m.Content)
			}
			for _, want := range []string{"lifecycle rationale only", lifecycle, interpretation, "Native measurement references"} {
				if !strings.Contains(extracted.String(), want) {
					t.Errorf("actual extractor lost %q", want)
				}
			}
			prompt := dependencyObservationMessages(t, ctx)
			if strings.Contains(prompt, lifecycle) {
				at := strings.Index(prompt, lifecycle)
				t.Logf("reason surface: %s", prompt[max(0, at-180):min(len(prompt), at+len(lifecycle)+120)])
				var p answerDocNativePresentationReceipt
				renderAnswerDocRuntimeMeasurementChoices(ctx, &p)
				h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
				bound, valid := types.BindInvestigationNativeFactHandoff(h, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract)
				for _, b := range bound {
					t.Logf("selected %s/%s rows=%d notes=%d complete=%t", b.ObservationID, b.View, len(b.BoundTable.Rows), len(b.BoundTable.Notes), p.measurementViews[nativeFactHandoffViewKey(*b.BoundTable)])
				}
				t.Errorf("lifecycle-only reason replayed as answer synthesis despite exact native publication (valid=%t)", valid)
			}
			for _, want := range []string{interpretation, independent, "Verified measurement tables"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lost independent/native lane %q", want)
				}
			}
		})
	}
}

func TestHMC247NativeFactHandoffConservativeBoundaries(t *testing.T) {
	for _, mode := range []string{"complete", "unpublished", "partial_view", "wrong_source", "wrong_window", "json_restart", "legacy", "long_interpretation", "sanitized_interpretation", "runtime_only_suppression"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, false)
			var publication answerDocNativePresentationReceipt
			renderAnswerDocRuntimeMeasurementChoices(ctx, &publication)
			var choice *types.RuntimeMeasurementTable
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
				complete := publication.measurementViews[nativeFactHandoffViewKey(table)]
				if complete == (mode != "partial_view") {
					copy := table
					choice = &copy
					break
				}
			}
			if choice == nil {
				t.Fatalf("public fixture has no required complete=%t view", mode != "partial_view")
			}
			const reason = "HMC247_CLOSURE_AUDIT_RETAIN_UNTIL_COMPLETE"
			const independent = "HMC247_KEEP_INDEPENDENT_NOTE"
			interpretation := "HMC247_KEEP_INDEPENDENT_INTERPRETATION"
			if mode == "long_interpretation" {
				interpretation += "\n\n" + strings.Repeat(" independent boundary", 100)
			}
			if mode == "sanitized_interpretation" {
				interpretation += "\n\n<think>hidden scratch</think>"
			}
			ta := ctx.Mutable.TurnAArtifacts()
			ta.AcceptedClosureReason, ta.AcceptedResultKind = reason, "resolved"
			ta.InvestigationNotes = []string{reason, independent}
			ctx.Mutable.SetTurnAArtifacts(*ta)
			payload := map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved"}
			if mode != "legacy" {
				payload["native_fact_handoff"] = map[string]any{"runtime_measurements": []any{map[string]any{"observation_id": choice.ObservationID, "view": choice.View}}, "interpretation": interpretation}
			}
			raw, _ := json.Marshal(payload)
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success || !ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("public completion %v %s", err, r.Summary)
			}
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			switch mode {
			case "wrong_source":
				ta = ctx.Mutable.TurnAArtifacts()
				for i := range ta.ToolResults {
					for j := range ta.ToolResults[i].Observations {
						ta.ToolResults[i].Observations[j].SourceRef.Path += "-other"
					}
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(*ta)
			case "wrong_window":
				start, end := 5.0, 6.0
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5..6"}
			case "json_restart":
				wire, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				var restored types.TurnAArtifacts
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState("restart")
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete(reason)
				ctx.Mutable.SetInvestigationResultKind("resolved")
			case "runtime_only_suppression":
				ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
				ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
			}
			ctx = ctx.ShallowClone()
			var p *answerDocNativePresentationReceipt
			if mode != "unpublished" {
				p = &answerDocNativePresentationReceipt{}
				renderAnswerDocRuntimeMeasurementChoices(ctx, p)
			}
			text := renderAnswerDocInvestigationNarrativeHandoffWithPublication(ctx, p)
			if mode == "runtime_only_suppression" {
				// The optional field must not revive an explanation where the
				// existing typed artifact-only lane suppresses model narratives.
				if runtimeObservationOnlyForAnswerDoc(ctx) && answerDocHasDirectRuntimeObservation(ctx) {
					if text != "" {
						t.Fatal("optional interpretation bypassed typed suppression")
					}
					return
				}
				t.Fatal("fixture did not reach old runtime-only suppression")
			}
			wantReason := mode == "wrong_source" || mode == "wrong_window" || mode == "json_restart" || mode == "legacy"
			if strings.Contains(text, reason) != wantReason {
				t.Errorf("reason retained=%t want=%t: %s", strings.Contains(text, reason), wantReason, text)
			}
			if !strings.Contains(text, independent) {
				t.Error("independent note lost")
			}
			if mode != "legacy" && !strings.Contains(text, "HMC247_KEEP_INDEPENDENT_INTERPRETATION") {
				t.Error("independent interpretation lost")
			}
			if mode == "sanitized_interpretation" && strings.Contains(text, "hidden scratch") {
				t.Error("unsanitized scratch leaked")
			}
			if ctx.Mutable.StableInvestigationCompleteReason() != reason {
				t.Error("narrative projection changed original lifecycle audit")
			}
			separated := answerDocSeparatedNativeFactHandoff(ctx, p)
			if (mode == "unpublished" || mode == "partial_view") && (!separated.ownsCurrentNarrative || separated.nativeViewsComplete()) {
				t.Error("current narrative ownership and incomplete native publication were conflated")
			}
		})
	}
}

func TestHMC247NativeFactHandoffTurnAMerge(t *testing.T) {
	prior := &types.TurnAArtifacts{AcceptedClosureReason: "prior", AcceptedNativeFactHandoff: &types.InvestigationNativeFactHandoff{Interpretation: "prior interpretation", RuntimeMeasurements: []types.AnswerRuntimeMeasurementReceipt{{ObservationID: "a", View: types.RuntimeMeasurementSummary}}}}
	got := mergeTurnAArtifactsWithPrior(prior, types.TurnAArtifacts{})
	if got.AcceptedNativeFactHandoff == nil {
		t.Fatal("closure-only retry lost audit intent")
	}
	got.AcceptedNativeFactHandoff.RuntimeMeasurements[0].ObservationID = "mutated"
	if prior.AcceptedNativeFactHandoff.RuntimeMeasurements[0].ObservationID != "a" {
		t.Fatal("merge aliased intent")
	}
	got = mergeTurnAArtifactsWithPrior(prior, types.TurnAArtifacts{AcceptedClosureReason: "new legacy reason"})
	if got.AcceptedNativeFactHandoff != nil {
		t.Fatal("new legacy closure borrowed prior separation")
	}
}
