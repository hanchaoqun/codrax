package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc271RuntimeDomainAdapter struct {
	hmc264NamedZIPAdapter
	step int
}

func (a *hmc271RuntimeDomainAdapter) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	if err := ctx.Err(); err != nil {
		return llm.Response{}, err
	}
	available := map[string]bool{}
	for _, schema := range schemas {
		available[schema.Name] = true
	}
	a.step++
	switch a.step {
	case 1:
		// Discovery is a real prerequisite: a named directory does not make
		// trace_query available until a supported member has been found.
		if !available["trace_catalog"] {
			return llm.Response{}, fmt.Errorf("runtime discovery was displaced before catalog: %v", available)
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "domain-discover", Name: "trace_catalog", Params: json.RawMessage(`{"root":"captures","queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`)}}}, nil
	case 2:
		if !available["trace_query"] {
			return llm.Response{}, fmt.Errorf("catalog did not expose native query: %v", available)
		}
		page, err := hmc270ToolPage(messages, "domain-discover")
		if err != nil {
			return llm.Response{}, err
		}
		var calls map[string]json.RawMessage
		if err := json.Unmarshal(page["query_calls"], &calls); err != nil || len(calls) != 1 {
			return llm.Response{}, fmt.Errorf("actual catalog did not produce one current query: %d %v", len(calls), err)
		}
		for _, params := range calls {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "domain-query", Name: "trace_query", Params: params}}}, nil
		}
	}
	// The source-location obligation may legitimately schedule more work.
	// This fixture supplies no source proof and never claims that work done.
	return llm.Response{Content: "Recorded events are acquired; current implementation locations still require source evidence."}, nil
}

// Classification/action decisions are scripted; EmitAnalysis normalization,
// compilation, Run, the Explorer/BaseAgent surface, catalog and native query
// are real. This checks entry/dispatch, not natural-model acceptance or source
// completion. A source-location obligation must remain unfulfilled here:
// successful runtime acquisition does not prove current implementation.
func TestHMC271NormalRunRuntimeMembersKeepNativeEntryAndSourceLocation(t *testing.T) {
	for _, declaration := range []string{"omitted", "locate"} {
		t.Run(declaration, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			original, err := os.ReadFile("../../eval/fixtures/hmosperf_catalog_objects/captures/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(repo, "captures", "capture.data")
			if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, original, 0400); err != nil {
				t.Fatal(err)
			}
			const request = "List recorded events and current implementation locations from captures, with inspection progress, for thread 101 in 10.000s to 10.050s."
			model := &hmc271RuntimeDomainAdapter{}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceCatalog{})
			registry.Register(&tool.TraceQuery{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{
				LLM: model, Tools: registry, MaxIterations: 2,
				AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {},
			})
			defaultSkills := skill.NewRegistry()
			skill.RegisterDefaults(defaultSkills)
			exploreSkill, err := defaultSkills.Get("explore-skill")
			if err != nil {
				t.Fatal(err)
			}
			var o *Orchestrator
			var output *agent.StageOutput
			var explorerErr error
			explorerCalls := 0
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					params := softSourcePublicAnalysis("")
					params["intent"], params["scenario"], params["question_kind"], params["predicate_axis"] = "enumerate", "generic", "mechanism", "flow"
					params["keywords"], params["entities"] = []string{"capture", "thread", "allocation", "release", "callsites", "progress", "record", "location"}, []string{"captures"}
					params["predicates"].(map[string]any)["is_diagnostic_question"] = false
					params["predicates"].(map[string]any)["has_per_member_table"] = true
					params["diagnostic_profile"].(map[string]any)["is_diagnostic"] = false
					params["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "thread 101", "confidence": 1}
					params["runtime_targets"] = []any{map[string]any{"kind": "thread", "pid": 101, "source": "user_explicit", "confidence": 1}}
					params["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "source_quote": "10.000s to 10.050s", "time_start": 10, "time_end": 10.05, "confidence": 1}
					params["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "fact_families": []string{"other_observed_value"}, "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": 1}
					params["source_scope_profile"] = map[string]any{"requested_scope": "all", "source_quotes": []string{"captures"}, "confidence": 1}
					params["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 1, "dimensions": []any{
						map[string]any{"index": 1, "label": "recorded events", "role": "member_set", "source_quote": "recorded events", "required": true},
						map[string]any{"index": 2, "label": "current implementation locations", "role": "source_location", "source_quote": "current implementation locations", "required": true},
						map[string]any{"index": 3, "label": "inspection progress", "role": "stage_or_workflow", "source_quote": "inspection progress", "required": true},
					}}
					if declaration == "locate" {
						params["current_source_explanation_profile"] = map[string]any{"is_current_source_explanation_requested": true, "modes": []string{"locate_current_code"}, "source_quotes": []string{"current implementation locations"}, "confidence": 1}
					}
					raw, _ := json.Marshal(params)
					result, err := (&tool.EmitAnalysis{}).Execute(o.busCtx, raw)
					if err != nil || !result.Success {
						t.Fatalf("public analysis failed: %v / %s", err, result.Summary)
					}
					rm := ctx.Mutable.RequestModel()
					compiled := compiler.Compile(*rm, budget.BudgetSignals{})
					compiledIR := &types.AnalysisIR{RequestModel: *rm, TaskGraph: compiled.TaskGraph, EvidencePlan: compiled.EvidencePlan, AnswerContract: compiled.AnswerContract}
					if sourceInventoryLensNavigationActive(compiledIR) || rm.SourceInventoryProfile.Active() {
						t.Error("runtime member-location attributes created a principal source inventory")
					}
					// Isolate one ordinary evidence dispatch from unrelated
					// answer rendering/source-completion work. Its request and
					// evidence plan are the public compiler's actual products.
					ir := dagIR(types.AnswerContract{Language: "en"})
					ir.RequestModel, ir.EvidencePlan = *rm, compiled.EvidencePlan
					ir.EvidencePlan.StopConditions = nil
					ir.EvidencePlan.Budget.MaxReactIters = 2
					ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native-entry", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
					return &agent.StageOutput{AnalysisIR: ir, ToolResults: []types.ToolResult{result}}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					explorerCalls++
					if sourceInventoryProfileActive(o.busCtx) || ctx.ExploreToolSurface.IsSourceInventoryLens() {
						t.Error("scheduler revived source inventory before native acquisition")
					}
					if got := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(&ctx.AnalysisIR.RequestModel, ctx.TurnRouteHint); got != types.RuntimeSourceRequirementPrecise {
						t.Errorf("source location lost its precise obligation: %s", got)
					}
					current, err := explorer.Execute(ctx, exploreSkill)
					if output == nil {
						output, explorerErr = current, err
					}
					return current, err
				},
			})
			o = New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(t.TempDir())
			o.SetTurnRouteHint(types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation})
			bus, runErr := o.Run(request, repo, "main")
			if bus == nil || output == nil || model.step < 2 || explorerCalls < 1 || explorerErr != nil {
				t.Fatalf("native entry not reached: steps=%d dispatches=%d explorer=%v run=%v", model.step, explorerCalls, explorerErr, runErr)
			}
			queries := 0
			for _, result := range output.ToolResults {
				if result.ToolName != "trace_query" {
					continue
				}
				if !result.Success {
					t.Fatalf("ordinary native query failed: %s", result.Summary)
				}
				for _, observation := range result.Observations {
					if stack, ok := tool.DecodeTraceResourceStack(observation); ok {
						queries++
						if stack.TargetPID != 101 || len(stack.Events) != 3 {
							t.Fatalf("native query lost the source-local requested events: %+v", stack)
						}
					}
				}
			}
			if queries != 1 || len(bus.Mutable.TraceCatalogs()) != 1 {
				t.Fatalf("actual catalog/native payload not delivered: queries=%d catalogs=%d", queries, len(bus.Mutable.TraceCatalogs()))
			}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, types.ObservationLedger{})
			if !authority.CurrentSourceRequired || authority.CurrentSourceRequirement != types.RuntimeSourceRequirementPrecise || authority.CurrentSourceSatisfied {
				t.Fatalf("runtime query discharged or weakened independent source location: %+v", authority)
			}
			if declaration == "omitted" && bus.AnalysisIR.RequestModel.CurrentSourceExplanationProfile != nil {
				t.Fatal("omitted source declaration was rewritten")
			}
			if after, err := os.ReadFile(source); err != nil || !bytes.Equal(original, after) {
				t.Fatal("runtime original changed", err)
			}
		})
	}
}
