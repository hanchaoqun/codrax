package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativePresentationReplacesOnlyCurrentSynthesisHMC240(t *testing.T) {
	for _, view := range []string{"perf_sampling_tree", "window_stats", "search_then_window"} {
		t.Run(view, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			queryView := view
			if view == "search_then_window" {
				queryView = "window_stats"
			}
			query, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": queryView, "pid": 11, "time_start": 1, "time_end": 2})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), query)
			if err != nil || !result.Success {
				t.Fatalf("query: %v %+v", err, result)
			}
			const synthesis = "STALE_CURRENT_SYNTHESIS_merges_independent_denominators"
			const independent = "Independent investigation caveat remains advisory."
			results := []types.ToolResult{result}
			if view == "search_then_window" {
				navigation, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"document-worker","time_start":1,"time_end":2}`))
				if err != nil || !navigation.Success {
					t.Fatalf("navigation: %v %+v", err, navigation)
				}
				results = append([]types.ToolResult{navigation}, results...)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, InvestigationNotes: []string{synthesis, independent}})
			ctx.Mutable.SetInvestigationComplete(synthesis)
			ctx.Mutable.SetInvestigationResultKind("resolved")
			before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			prompt := dependencyObservationMessages(t, ctx)
			if strings.Contains(prompt, synthesis) {
				t.Error("fully published native query replayed current model synthesis")
			}
			for _, want := range []string{independent, "total_weight=35", "total_weight=7", "total_weight=100", "unavailable_weight=5", "sys;root"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lost %q", want)
				}
			}
			after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if !bytes.Equal(before, after) || ctx.Mutable.StableInvestigationCompleteReason() != synthesis {
				t.Fatal("presentation mutated accepted audit")
			}
		})
	}
}

func TestNativePresentationSynthesisBoundariesHMC240(t *testing.T) {
	for _, name := range []string{"collapsed", "unknown_target", "other_window", "other_query", "source_explanation", "causal", "relation", "absence", "no_receipt"} {
		t.Run(name, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			args := map[string]any{"source": "attached_trace", "view": "perf_sampling_tree", "pid": 11, "time_start": 1, "time_end": 2}
			if name == "collapsed" {
				args["sampling_depth"] = 1
			}
			if name == "unknown_target" {
				args["pid"] = 99
			}
			params, _ := json.Marshal(args)
			r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !r.Success {
				t.Fatalf("query: %v %+v", err, r)
			}
			results := []types.ToolResult{r}
			if name == "other_query" {
				x, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"perf_sample","limit":1,"time_start":1,"time_end":2}`))
				if err != nil || !x.Success {
					t.Fatalf("other query: %v %+v", err, x)
				}
				results = append(results, x)
			}
			const synthesis = "CURRENT_ADVISORY_MUST_SURVIVE"
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, InvestigationNotes: []string{"independent note"}})
			ctx.Mutable.SetInvestigationComplete(synthesis)
			ctx.Mutable.SetInvestigationResultKind("resolved")
			switch name {
			case "other_window":
				end := 1.5
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "source_explanation":
				ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"runtime.go"}, Confidence: 1}
			case "causal":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "relation":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
			case "absence":
				ctx.Mutable.SetInvestigationResultKind("absence")
			}
			prompt := ""
			if name == "no_receipt" {
				prompt = renderAnswerDocInvestigationNarrativeHandoff(ctx)
			} else {
				prompt = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			}
			if !strings.Contains(prompt, synthesis) {
				t.Fatal("partial/unqualified/independent-scope handoff removed current advisory")
			}
		})
	}
}
