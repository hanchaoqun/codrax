package repl

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const hmc274ContradictoryPolicy = `{"required_outcomes":["measurement","source_explanation"],"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"optional","operation":"investigate","source":"artifact","confidence":0.95,"reason":"recorded facts","requires_diagram":false}`

func TestHMC274SourceOutcomeConflictPublicRepair(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "artifact", true: "mixed"}[mixed], func(t *testing.T) {
			corrected := strings.Replace(hmc274ContradictoryPolicy, `,"source_explanation"`, "", 1)
			if mixed {
				corrected = strings.Replace(hmc274ContradictoryPolicy, `"optional"`, `"required"`, 1)
				corrected = strings.Replace(corrected, `"artifact"`, `"mixed"`, 1)
			}
			adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(hmc274ContradictoryPolicy), turnPolicyResp(corrected)}}
			policy, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicy(context.Background(), "List observed durations and their trend; inspect the current implementation if independently requested.", "", false)
			if err != nil || len(adapter.calls) != 2 {
				t.Fatalf("conflicting source declarations were not repaired once: calls=%d err=%v policy=%+v", len(adapter.calls), err, policy)
			}
			guarded := ApplyTurnPolicyGuards(policy, false, false)
			hint := TurnRouteHintFromPolicy(guarded)
			if hint.RequiresCurrentSourceEvidence() != mixed || hint.RequiredOutcomes.Has(types.TurnOutcomeSourceExplanation) != mixed {
				t.Fatalf("system overrode the model's coherent repair: %+v", hint)
			}
			for _, call := range adapter.calls {
				if len(call.tools) != 1 || string(call.tools[0].Parameters) != string(turnPolicyTool.Parameters) {
					t.Fatal("repair changed the classifier schema")
				}
			}
			last := adapter.calls[1].messages[len(adapter.calls[1].messages)-1].Content
			for _, want := range []string{"source_explanation", "optional", "current", "Reclassify", "not yet accepted"} {
				if !strings.Contains(last, want) {
					t.Errorf("repair omitted neutral consistency guidance %q: %s", want, last)
				}
			}
			hmc274AssertAnalysisAndFinalizer(t, hint, mixed)
		})
	}
}

func TestHMC274SourceOutcomeConflictSharesOneRepairBudget(t *testing.T) {
	for _, first := range []string{hmc274ContradictoryPolicy, strings.Replace(hmc274ContradictoryPolicy, `,"requires_diagram":false`, "", 1)} {
		adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(first), turnPolicyResp(hmc274ContradictoryPolicy)}}
		policy, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicySingleShot(context.Background(), "Inspect recorded values.", "", false)
		if err == nil || len(adapter.calls) != 2 || policy.RequiredOutcomes != 0 {
			t.Fatalf("invalid second response escaped or received another budget: calls=%d err=%v policy=%+v", len(adapter.calls), err, policy)
		}
	}
}

func TestHMC274CoherentAndLegacySourceOutcomesStaySingleShot(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		required      bool
	}{
		{"optional", strings.Replace(hmc274ContradictoryPolicy, `,"source_explanation"`, "", 1), false},
		{"required", strings.Replace(hmc274ContradictoryPolicy, `"optional"`, `"required"`, 1), true},
		{"legacy", strings.Replace(hmc274ContradictoryPolicy, `,"current_source_evidence_mode":"optional"`, "", 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(tc.payload)}}
			policy, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicy(context.Background(), "Current request.", "", false)
			if err != nil || len(adapter.calls) != 1 || TurnRouteHintFromPolicy(ApplyTurnPolicyGuards(policy, false, false)).RequiresCurrentSourceEvidence() != tc.required {
				t.Fatalf("coherent or legacy classification changed: calls=%d err=%v policy=%+v", len(adapter.calls), err, policy)
			}
		})
	}
}

func TestHMC274RepairDoesNotInheritPriorAttemptFields(t *testing.T) {
	for _, tc := range []struct {
		name, repaired string
		mode           types.TurnRouteCurrentSourceEvidenceMode
		outcomes       types.TurnOutcomeSet
	}{
		{"legacy_mode", strings.Replace(hmc274ContradictoryPolicy, `,"current_source_evidence_mode":"optional"`, "", 1), types.TurnRouteCurrentSourceEvidenceUnspecified, types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation},
		{"legacy_outcomes", strings.Replace(hmc274ContradictoryPolicy, `"required_outcomes":["measurement","source_explanation"],`, "", 1), types.TurnRouteCurrentSourceEvidenceOptional, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(hmc274ContradictoryPolicy), turnPolicyResp(tc.repaired)}}
			policy, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicy(context.Background(), "Inspect recorded values.", "", false)
			if err != nil || len(adapter.calls) != 2 || policy.CurrentSourceEvidenceMode != tc.mode || policy.RequiredOutcomes != tc.outcomes {
				t.Fatalf("replacement attempt inherited an earlier declaration: calls=%d err=%v policy=%+v", len(adapter.calls), err, policy)
			}
		})
	}
}

// Both public boundaries are exercised: a scripted model repairs the route;
// EmitAnalysis consumes that exact returned hint, and the production Finalizer
// sends the compiled authority to its actual adapter. No facts are fabricated
// from the route and no private authority helper is used as the only witness.
func hmc274AssertAnalysisAndFinalizer(t *testing.T, hint types.TurnRouteHint, mixed bool) {
	t.Helper()
	const request = "List observed durations and their trend; inspect the current implementation if independently requested."
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{
		"intent":"explain","scenario":"generic","complexity":"moderate","question_kind":"mechanism","predicate_axis":"flow",
		"keywords":["durations","trend"],"entities":[],"intent_confidence":0.9,"complexity_confidence":0.9,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.9},
		"runtime_artifact_scope_profile":{"requested_scope":"full_artifact","source_quote":"observed durations","confidence":0.9},
		"runtime_target_profile":{"declaration":"no_named_target","confidence":0.9},
		"runtime_question_profile":{"scope":"bounded_fact_set","fact_families":["target_scheduler_state","count_or_duration"],"runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.9},
		"source_scope_profile":{"requested_scope":"all","source_quotes":[],"confidence":0.9},
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.9},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[{"index":1,"label":"durations","role":"observed_value","source_quote":"observed durations","required":true},{"index":2,"label":"trend","role":"stage_or_workflow","source_quote":"their trend","required":true}]},
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`), &payload); err != nil {
		t.Fatal(err)
	}
	if mixed {
		payload["current_source_explanation_profile"] = map[string]any{"is_current_source_explanation_requested": true, "modes": []string{"trace_current_flow"}, "source_quotes": []string{"current implementation"}, "confidence": 0.9}
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState(request), TurnRouteHint: hint, AttachedHitrace: "# tracer: nop\n", Language: "en"}
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "attached"}}}
	raw, _ := json.Marshal(payload)
	result, err := (&tool.EmitAnalysis{}).Execute(bus, raw)
	if err != nil || !result.Success || bus.Mutable.RequestModel() == nil {
		t.Fatalf("public analyzer: %v %s", err, result.Summary)
	}
	rm := bus.Mutable.RequestModel()
	if !rm.RuntimeQuestionProfile.BoundedFactSet() || rm.PredicateAxis != types.AxisFlow || len(rm.RequestedAnswerDimensions.Dimensions) != 2 {
		t.Fatal("coherent runtime scope or requested trend was rewritten")
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Language: "en"}}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_scheduler_concurrency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	queryParams, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 1.01})
	query, err := (&tool.TraceQuery{}).Execute(bus, queryParams)
	if err != nil || !query.Success || len(query.Observations) == 0 {
		t.Fatalf("real native producer: %v %s", err, query.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
	ctx := promptctx.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	stop := errors.New("captured actual finalizer request")
	capture := &scriptedChatAdapter{responses: []llm.Response{{}}, errors: []error{stop}}
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	sk, err := skills.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.NewFinalizerAgent(&agent.Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, stop) || len(capture.calls) != 1 {
		t.Fatalf("actual finalizer boundary: calls=%d err=%v", len(capture.calls), err)
	}
	var messages strings.Builder
	for _, m := range capture.calls[0].messages {
		messages.WriteString(m.Content)
		messages.WriteByte('\n')
	}
	want := "current_source_required=false"
	if mixed {
		want = "current_source_required=true"
	}
	if !strings.Contains(messages.String(), "### Runtime Source Authority") || !strings.Contains(messages.String(), want) {
		t.Fatalf("actual finalizer authority missing %q: %s", want, messages.String())
	}
	if mixed && !rm.CurrentSourceExplanationProfile.Active() {
		t.Fatal("explicit mixed source request was dropped")
	}
}
