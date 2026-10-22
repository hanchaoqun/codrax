package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real artifact-only route, unlike the mixed/current-source
// fixtures for the original units handoff. Completion acceptance and native
// table delivery are independent of advisory-unit delivery.
func TestHMC280RuntimeInterpretationUnitsActualFinalizer(t *testing.T) {
	for _, producer := range []string{"sampling", "io", "resource"} {
		t.Run(producer, func(t *testing.T) {
			var ctx *types.AgentContext
			if producer == "resource" {
				ctx = resourceFactsContextHMC249(t)
				// Match the attached-artifact triage lane of the actual run;
				// resource facts still come only from the public native queries.
				ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{Subject: "resource observation navigation", Summary: "Use the source-bound resource query rows."}}})
				ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = false
				ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
			} else {
				ctx = nativeFactHandoffContextHMC247(t, producer == "sampling")
			}
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			if len(choices) == 0 {
				t.Fatal("public query has no current native references")
			}
			var units []types.InvestigationInterpretationUnit
			for i := 0; i < 7; i++ {
				units = append(units, types.InvestigationInterpretationUnit{
					Text:        fmt.Sprintf("HMC280 independent clue %d", i),
					Boundaries:  []string{fmt.Sprintf("HMC280 not a proved cause %d", i)},
					SupportRefs: []string{fmt.Sprintf("source-A/query-A:%d", i)},
				})
			}
			const reason = "HMC280_LIFECYCLE_NOT_A_CONCLUSION"
			raw, _ := json.Marshal(map[string]any{
				"reason": reason, "confidence": "high", "result_kind": "resolved",
				"evidence_floor_waiver": map[string]any{"reason": "external_only_trace", "rationale": "Native runtime observations do not require current-repository citations."},
				"native_fact_handoff": map[string]any{
					"runtime_measurements": choices[:1], "interpretation_role": "independent_analysis", "interpretation_units": units,
				},
			})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
				t.Fatalf("public completion not accepted: %v %s", err, r.Summary)
			}
			ctx = ctx.ShallowClone()
			if !runtimeObservationOnlyForAnswerDoc(ctx) || !answerDocHasDirectRuntimeObservation(ctx) || !answerDocHasDeterministicRuntimeQueryObservation(ctx) {
				t.Fatal("fixture did not reach artifact-only deterministic suppression")
			}
			before, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
			actual := dependencyObservationMessages(t, ctx)
			for i, unit := range units {
				encoded, _ := json.Marshal(unit)
				if !strings.Contains(actual, string(encoded)) {
					t.Errorf("accepted unit %d absent or split in actual Finalizer INIT", i)
				}
			}
			for _, want := range []string{"advisory only", "not native measurements or proof of a causal relation", choices[0].ObservationID} {
				if !strings.Contains(actual, want) {
					t.Errorf("missing advisory/native identity boundary %q", want)
				}
			}
			if strings.Contains(actual, reason) {
				t.Fatal("structured units revived lifecycle prose")
			}
			after, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
			if string(before) != string(after) {
				t.Fatal("Finalizer altered accepted advisory handoff")
			}
		})
	}
}

func TestHMC280RuntimeInterpretationUnitsBoundaries(t *testing.T) {
	for _, mode := range []string{"budget", "all_omitted", "sanitized", "restatement", "legacy", "missing_role", "retired", "restored", "source", "query", "payload", "target", "clock", "window", "no_directed_path", "log_peers"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, true)
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			const reason = "HMC280_SUPPRESSED_LIFECYCLE"
			const legacy = "HMC280_SUPPRESSED_LEGACY"
			const note = "HMC280_SUPPRESSED_NOTE"
			units := []types.InvestigationInterpretationUnit{
				{Text: "HMC280_FIRST_CLAIM", Boundaries: []string{"HMC280_FIRST_BOUNDARY"}, SupportRefs: []string{"unverified-source/query-A:1"}},
				{Text: "HMC280_SECOND_CLAIM", Boundaries: []string{"HMC280_SECOND_BOUNDARY"}, SupportRefs: []string{"unverified-source/query-B:2"}},
			}
			role := types.InvestigationIndependentAnalysis
			switch mode {
			case "budget":
				units[0].Boundaries[0] += strings.Repeat("long declared boundary; ", 100)
			case "all_omitted":
				for i := range units {
					units[i].Boundaries[0] += strings.Repeat("long declared boundary; ", 100)
				}
			case "sanitized":
				units[0].Boundaries[0] = "<think>HMC280_PRIVATE_BOUNDARY</think>"
			case "restatement":
				role = types.InvestigationNativeRestatement
			case "missing_role":
				role = ""
			case "legacy":
				units = nil
			}
			raw, _ := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
				"runtime_measurements": choices[:1], "interpretation": legacy, "interpretation_role": role, "interpretation_units": units,
			}})
			result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
				t.Fatalf("public completion not accepted: %v %s", err, result.Summary)
			}
			ta := ctx.Mutable.TurnAArtifacts()
			ta.AcceptedClosureReason = reason
			ta.AcceptedNativeFactHandoff = ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
			ta.InvestigationNotes = []string{reason, legacy, note}
			ctx.Mutable.SetTurnAArtifacts(*ta)
			switch mode {
			case "retired":
				ctx.Mutable.SetInvestigationComplete(reason)
			case "restored":
				wire, _ := json.Marshal(ta)
				var restored types.TurnAArtifacts
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState("restored advisory only")
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete(reason)
			case "source", "query", "payload", "target", "clock":
				for i := range ta.ToolResults {
					for j := range ta.ToolResults[i].Observations {
						ref := &ta.ToolResults[i].Observations[j].SourceRef
						switch mode {
						case "source":
							ref.Path += "-other"
						case "query":
							ref.QueryScopeID += "-other"
						case "payload":
							ref.PayloadRef += "-other"
						case "target":
							ref.QueryTargetPID++
						case "clock":
							ref.TimeDomain += "-other"
						}
					}
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(*ta)
			case "window":
				start, end := 5.0, 6.0
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5..6"}
			case "no_directed_path":
				rm := &ctx.AnalysisIR.RequestModel
				rm.PerfTrace, rm.LogTriage = nil, nil
				rm.Intent, rm.PredicateAxis = types.IntentTrace, types.AxisCall
				rm.CallChainEndpointProfile = &types.CallChainEndpointProfile{Source: "callerA", Sink: "calleeB"}
				rm.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"callerA", "calleeB"}}
				ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{Reason: types.PrincipalSpanWaiverNoDirectedPath, Rationale: "independent endpoint branches"})
				if types.CompileCallChainEndpointBoundary(*rm, ctx.Mutable.PrincipalSpanWaiver()) == nil {
					t.Fatal("missing typed no-path boundary")
				}
			case "log_peers":
				ctx.Mutable.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{}, {}}})
			}
			ctx = ctx.ShallowClone()
			switch mode {
			case "retired", "restored", "source", "query", "payload", "target", "clock", "window":
				if answerDocSeparatedNativeFactHandoff(ctx, nil).ownsCurrentNarrative {
					t.Fatal("invalid completion/source/query identity retained narrative ownership")
				}
			}
			actual := dependencyObservationMessages(t, ctx)
			wantSecond := mode == "budget" || mode == "sanitized"
			if strings.Contains(actual, "HMC280_SECOND_CLAIM") != wantSecond || strings.Contains(actual, "HMC280_FIRST_CLAIM") {
				t.Errorf("typed eligibility/whole-unit boundary failed: second=%t want=%t", strings.Contains(actual, "HMC280_SECOND_CLAIM"), wantSecond)
			}
			for _, forbidden := range []string{reason, legacy, note, "HMC280_PRIVATE_BOUNDARY", "unverified-source/query-A:1"} {
				if strings.Contains(actual, forbidden) {
					t.Errorf("ineligible narrative or split unit escaped: %s", forbidden)
				}
			}
			if wantSecond {
				encoded, _ := json.Marshal(units[1])
				if !strings.Contains(actual, string(encoded)) || !strings.Contains(actual, "Interpretation is partial: 1 of 2 declared units shown whole") || !strings.Contains(actual, "support_refs are proposed locators, not verified evidence") {
					t.Fatal("complete remaining unit or honest omission status missing")
				}
				projection := answerDocSeparatedNativeFactHandoff(ctx, nil)
				if projection.publishedInterpretationBytes > 1800 || projection.nativeViewsComplete() || !projection.ownsCurrentNarrative {
					t.Fatal("budget, independent native publication or ownership changed")
				}
			}
			if mode == "all_omitted" && !strings.Contains(actual, "Interpretation is partial: 0 of 2 declared units shown whole") {
				t.Fatal("all-omitted handoff pretended complete or revived lifecycle prose")
			}
		})
	}
}
