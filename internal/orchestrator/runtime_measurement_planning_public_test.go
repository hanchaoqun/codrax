package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/amplifier"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Model decisions are scripted; Run, compilation, native query, accepted
// completion and the finalizer's scoped measurement view are real. The exact
// agent material-to-planning adapter is covered separately by its IR test.
func TestRuntimeMeasurementPlanningPublicRun(t *testing.T) {
	for _, tc := range []struct {
		name     string
		names    []string
		first    string
		explicit bool
	}{
		{"shared prefix", []string{"capture_alpha", "capture_beta", "capture_gamma", "capture_delta"}, "", false},
		{"shared suffix", []string{"alpha_capture", "beta_capture", "gamma_capture", "delta_capture"}, "", false},
		{"no common affix", []string{"Alpha", "Beta", "Gamma", "Delta"}, "", false},
		{"navigation is not completion", []string{"capture_alpha", "capture_beta"}, "recipe", false},
		{"failed query is not completion", []string{"capture_alpha", "capture_beta"}, "failed", false},
		{"partial search is not completion", []string{"capture_alpha", "capture_beta"}, "event_search", false},
		{"cancelled query is not completion", []string{"capture_alpha", "capture_beta"}, "cancelled", false},
		{"explicit topics still required", []string{"capture_alpha", "capture_beta"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
			if err != nil {
				t.Fatal(err)
			}
			repo := t.TempDir()
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			path = filepath.Join(repo, "capture.systrace")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 640})
			if err != nil {
				t.Fatal(err)
			}
			start, end := 2.0, 2.25
			rm := types.RequestModel{RawRequest: "Report measured values between 2 and 2.25 seconds.", Language: "en", Intent: types.IntentEnumerate, Scenario: types.ScenarioGeneric, Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{Entities: tc.names}, Predicates: types.SemanticPredicates{IsCrossComponent: true},
				PerfTrace:                   &types.PerfBundle{Observations: []types.PerfObservation{{}}},
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOtherObservedValue}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "between 2 and 2.25 seconds"},
				ExternalObservationPolicy:   &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"runtime observations only"}},
			}
			if tc.explicit {
				rm.SubTopics = []types.SubTopic{{Summary: "first independent measurement", Entities: []string{tc.names[0]}}, {Summary: "second independent measurement", Entities: []string{tc.names[1]}}}
			}
			amplified, _ := amplifier.AmplifyWithPlanningFacts(rm, amplifier.PlanningFacts{SinglePhysicalRuntimeArtifact: material.SinglePhysicalSource()})
			compiled := compiler.Compile(amplified, budget.BudgetSignals{})
			// Isolate planning/closure from presentation-only citation checks. The
			// generated DAG, required topics and native completion checks are intact.
			ir := &types.AnalysisIR{Version: types.AnalysisIRVersion, RequestModel: amplified, TaskGraph: compiled.TaskGraph, EvidencePlan: compiled.EvidencePlan, AnswerContract: types.AnswerContract{Language: "en"}}
			ir.EvidencePlan.StopConditions = nil // no hypotheses are installed by this fixture
			var dispatches []string
			finalCalls, queries, accepted := 0, 0, 0
			var o *Orchestrator
			fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentPerfTriager: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					ctx.Mutable.SetPerfTrace(rm.PerfTrace)
					return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
				},
				types.AgentAnalyzer: dagAnalyzerFn(ir),
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					dispatches = append(dispatches, ctx.ExploreDispatchKey)
					view := "window_stats"
					queryPath := path
					bus := o.busCtx
					firstIncomplete := len(dispatches) == 1 && tc.first != ""
					if firstIncomplete {
						switch tc.first {
						case "recipe", "event_search":
							view = tc.first
						case "failed":
							queryPath = filepath.Join(t.TempDir(), "missing.systrace")
						case "cancelled":
							copyBus := bus.ShallowClone()
							cancelled, cancel := context.WithCancel(context.Background())
							cancel()
							copyBus.Ctx = cancelled
							bus = copyBus
						}
					}
					params, _ := json.Marshal(map[string]any{"source": "path", "path": queryPath, "view": view, "time_start": start, "time_end": end, "limit": 1})
					query, queryErr := (&tool.TraceQuery{}).Execute(bus, params)
					queries++
					if tc.first == "failed" || tc.first == "cancelled" {
						if firstIncomplete && queryErr == nil && query.Success {
							t.Fatal("failed/cancelled query succeeded")
						}
					} else if queryErr != nil || !query.Success {
						t.Fatalf("native query: %v / %s", queryErr, query.Summary)
					}
					ctx.Mutable.AppendDispatchToolResult(query)
					if firstIncomplete {
						if ctx.Mutable.IsInvestigationComplete() {
							t.Fatal("query or planning facts manufactured completion")
						}
						// No model-owned complete declaration is made on incomplete
						// investigation. The remaining evidence node must still run.
						return &agent.StageOutput{MissingPiece: types.MissingFacts, ToolResults: []types.ToolResult{query}}, nil
					}
					complete, err := (&tool.EmitInvestigationComplete{}).Execute(o.busCtx, json.RawMessage(`{"reason":"Accepted native measurements cover the requested source and window; use those scoped values.","confidence":"high","result_kind":"resolved"}`))
					if err != nil || !complete.Success || !ctx.Mutable.IsInvestigationComplete() {
						t.Fatalf("actual completion rejected: %v / %s", err, complete.Summary)
					}
					accepted++
					results := []types.ToolResult{query, complete}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, AcceptedClosureReason: ctx.Mutable.StableInvestigationCompleteReason(), AcceptedResultKind: ctx.Mutable.StableInvestigationResultKind(), RuntimeObservationOnlyCompletion: true})
					return &agent.StageOutput{MissingPiece: types.MissingNone, ToolResults: results}, nil
				},
				types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					finalCalls++
					if accepted == 0 {
						t.Fatal("finalizer received no real accepted completion")
					}
					choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
					groups := map[string]bool{}
					for _, choice := range choices {
						if choice.View == types.RuntimeMeasurementSummary {
							groups[choice.ObservationID] = true
						}
					}
					if len(groups) < 8 {
						t.Fatalf("finalizer lost eight independently measured endpoint populations: %d", len(groups))
					}
					if !reflect.DeepEqual(ctx.AnalysisIR.RequestModel.SubTopics, rm.SubTopics) {
						t.Fatal("completion rewrote explicit topics")
					}
					return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Measured values remain separated by source, endpoint and byte caliber."}, nil
				},
			}
			ar, sr, sar := buildRegistries(fns)
			sr.Register(&skill.Config{Name: "perf-triage-skill"})
			o = New(types.PipelineSettings{MaxParallelism: 1}, ar, sr, sar)
			o.SetMaxSteps(20)
			o.SetAttachedHitrace(material.Preview())
			o.SetAttachedHitraceSource(path)
			o.SetAttachedTraceMaterial(material)
			_, err = o.Run(rm.RawRequest, repo, "main")
			if err != nil {
				t.Fatal(err)
			}
			if finalCalls != 1 {
				t.Fatalf("finalizer calls=%d dispatches=%v state=%+v graph=%+v", finalCalls, dispatches, o.busCtx.TaskState, ir.TaskGraph)
			}
			want := 1
			if tc.first != "" {
				want = 2
			}
			if tc.explicit {
				want = 2
				if len(dispatches) != 2 || dispatches[1] != "n1_evidence_t0+n1_evidence_t1" {
					t.Fatalf("required topic units were skipped: %v", dispatches)
				}
			}
			if queries != want || len(dispatches) != want {
				t.Fatalf("queries=%d dispatches=%v want=%d", queries, dispatches, want)
			}
		})
	}
}
