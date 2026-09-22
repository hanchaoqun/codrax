package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The finalizer already honors a present but empty answer projection. Pin its
// public path while emit-time consumers are brought into agreement: a durable
// audit fact is not a fallback answer fact merely because none survived.
func TestAnswerAggregateEmptyProjectionPublicFinalInstruction(t *testing.T) {
	testAnswerAggregateEmptyProjectionFinalMessages(t, false)
}

// Explicit relation members exercise the independently rendered relation
// dossier. Scalar-only facts cannot expose that consumer's raw-state fallback.
func TestAnswerAggregateEmptyProjectionPublicRelationDossier(t *testing.T) {
	testAnswerAggregateEmptyProjectionFinalMessages(t, true)
}

func testAnswerAggregateEmptyProjectionFinalMessages(t *testing.T, relations bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "native.systrace")
	if err := os.WriteFile(path, []byte(traceWaitRawStateAgentCycle(1, "D", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.004
	params, _ := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "window_stats", "pid": 77,
		"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace",
	})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("native public query: %v; %s", err, result.Summary)
	}
	const unsupportedLabel = "unsupported_runtime_duration_from_model"
	const sourceLabel = "independent_source_retry_budget"
	for _, mixed := range []bool{false, true} {
		name := "empty"
		if mixed {
			name = "mixed_source"
		}
		for _, lang := range []string{"zh", "en"} {
			t.Run(name+"/"+lang, func(t *testing.T) {
				rm := types.RequestModel{
					RawRequest: "Report the target's observed wait and any separately grounded source budget.",
					Language:   lang, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
					RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Thread: "reader-77", Source: "user_explicit", Confidence: 1}},
					RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "reader-77", Confidence: 1},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
						TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000000..1.004000", Confidence: 1},
					RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
						FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences},
						SourceQuote:  "observed wait", Confidence: 1},
				}
				facts := []types.AnswerAggregateFact{{
					Kind: types.AnswerAggregateScalar, Role: types.AnswerAggregateRolePrincipalAnswer,
					Label: unsupportedLabel, Value: "19.671", Unit: "ms",
					SupportRefs: []string{"trace_query:window_stats:model-recalculation"},
					Provenance:  "model_emitted",
				}}
				if relations {
					facts[0].Kind, facts[0].Value, facts[0].Unit = types.AnswerAggregateMemberSet, "1", ""
					facts[0].Members = []string{"UnprovenWaitOwner -> UnprovenCompletion"}
				}
				mu := types.NewMutableState(rm.RawRequest)
				if mixed {
					facts = append(facts, types.AnswerAggregateFact{
						Kind: types.AnswerAggregateScalar, Role: types.AnswerAggregateRolePrincipalAnswer,
						Label: sourceLabel, Value: "7", SupportRefs: []string{"budget.go:2"}, Provenance: "model_emitted",
					})
					if relations {
						facts[1].Kind, facts[1].Value = types.AnswerAggregateMemberSet, "1"
						facts[1].Members = []string{"RetryBudget -> RetryLimit"}
					}
					mu.AppendEvidence([]types.EvidenceItem{{ID: "source-budget", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
						Source: "budget.go", LineStart: 2, LineEnd: 2, AnchorKind: types.AnchorDefinition,
						Subject: "RetryBudget", GroundingStatus: types.GroundingGrounded, Summary: "RetryBudget = 7"}})
				}
				mu.SetRequestModel(rm)
				mu.SetInvestigationAggregateFacts(facts)
				mu.RetainInvestigationAggregateFacts()
				mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}, AcceptedAggregateFacts: facts})
				mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
					DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned business judgment."}},
				})
				bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
					AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
					RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
				}
				snapshot := func() string {
					data, _ := json.Marshal([]any{result, mu.StableInvestigationAggregateFacts(), mu.TurnAArtifacts(), mu.EmittedEvidence(), mu.AnswerDocumentV2(), bus.AnalysisIR.RequestModel})
					return string(data)
				}
				before := snapshot()
				ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
				plan := types.BuildAnswerSurfacePlanForAgentContext(ctx)
				wantCount := 0
				if mixed {
					wantCount = 1
				}
				if plan == nil || len(plan.StableAggregateFacts) != wantCount {
					t.Fatalf("fixture did not produce the requested empty/mixed answer projection: %+v", plan)
				}
				if mixed && (plan.StableAggregateFacts[0].Label != sourceLabel || plan.StableAggregateFacts[0].Value != facts[1].Value) {
					t.Fatal("independently supported source fact was discarded or changed")
				}
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				var contextMessages strings.Builder
				for _, message := range ctxbuilder.ToMessages(ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "projection-public-test"})) {
					contextMessages.WriteString(message.Content)
					contextMessages.WriteByte('\n')
				}
				for face, text := range map[string]string{"context_messages": contextMessages.String(), "final_instruction": instruction} {
					for _, forbidden := range []string{unsupportedLabel, "UnprovenWaitOwner", "UnprovenCompletion"} {
						if strings.Contains(text, forbidden) {
							t.Errorf("%s replayed excluded aggregate relation %q", face, forbidden)
						}
					}
					if relations && strings.Contains(text, sourceLabel) != mixed {
						t.Errorf("%s lost independent source relation or invented one", face)
					}
				}
				for _, forbidden := range []string{unsupportedLabel, "19.671"} {
					if strings.Contains(instruction, forbidden) {
						t.Errorf("filtered runtime synthesis replayed in actual finalizer instruction: %q", forbidden)
					}
				}
				if strings.Contains(instruction, sourceLabel) != mixed || strings.Contains(instruction, "## Structured Aggregate Facts") != mixed {
					t.Error("empty projection gained aggregate facts or mixed projection lost its independent source fact")
				}
				for _, wanted := range []string{"occurrence_count=1", "wall_clock_sum=1.000ms", "prev_state_raw=D"} {
					if !strings.Contains(instruction, wanted) {
						t.Errorf("authoritative native trace evidence was lost with the model restatement: %q", wanted)
					}
				}
				if after := snapshot(); before != after {
					t.Fatal("answer projection mutated the original aggregate payload, evidence, request or model document")
				}
				if len(mu.StableInvestigationAggregateFacts()) != len(facts) || len(mu.TurnAArtifacts().AcceptedAggregateFacts) != len(facts) {
					t.Fatal("withheld aggregate facts must remain in both durable audit carriers")
				}
			})
		}
	}
}
