package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTurnOutcomesReconcileSecondarySourceWithoutProseMatching(t *testing.T) {
	p := TurnPolicy{RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation, Route: RouteOperation, Operation: "computer_operation", OperationKind: "computer_operation", NeedsOperationAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, Source: "current_message", TargetSurface: "desktop", Confidence: .95}
	got := ApplyTurnPolicyGuards(p, false, false)
	if got.Route != RouteRepo || got.NeedsOperationAccess || !got.NeedsRepoAccess || got.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceRequired {
		t.Fatalf("secondary obligation dropped: %+v", got)
	}
	if got.WriteIntent == WriteIntentExplicitChange {
		t.Fatal("read outcomes authorized write")
	}
	for _, concrete := range []types.TurnOutcomeSet{types.TurnOutcomeExternalArtifact, types.TurnOutcomeComputerAction} {
		p.RequiredOutcomes |= concrete
		got = ApplyTurnPolicyGuards(p, false, false)
		if got.Route != RouteOperation || !got.NeedsOperationAccess || !got.NeedsRepoAccess {
			t.Fatalf("concrete action lost: %+v", got)
		}
		p.RequiredOutcomes &^= concrete
	}
	p.RequiredOutcomes = types.TurnOutcomeSourceChange
	if ApplyTurnPolicyGuards(p, false, false).Route == RouteWrite {
		t.Fatal("outcome bit granted write route")
	}
}

func TestTurnOutcomesClassifierAndRouteHint(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"required_outcomes":["measurement","source_explanation"],"route":"operation","operation":"computer_operation","source":"mixed","confidence":0.95,"reason":"two results","requires_diagram":false}`)}}
	p, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicy(context.Background(), "汇总数量，并说明当前实现", "", false)
	if err != nil {
		t.Fatal(err)
	}
	hint := TurnRouteHintFromPolicy(p)
	if !hint.RequiredOutcomes.Has(types.TurnOutcomeMeasurement) || !hint.RequiresCurrentSourceEvidence() {
		t.Fatalf("handoff: %+v", hint)
	}
	if strings.Count(adapter.calls[0].messages[0].Content, turnOutcomesTeaching) != 1 {
		t.Fatal("outcome teaching repeated or missing")
	}
	if !strings.Contains(string(turnPolicyTool.Parameters), `"required_outcomes"`) {
		t.Fatal("missing wire teaching")
	}
}

type shortOutcomeCLIPlanner struct {
	fakeCLICommandPlanner
	evaluations int
	outcomes    types.TurnOutcomeSet
	evalErr     error
}

func (p *shortOutcomeCLIPlanner) EvaluateCommandOperation(_ context.Context, _ string, records []commandOperationResultRecord, _ string) (operation.OperationEvaluation, error) {
	p.evaluations++
	p.outcomes = records[len(records)-1].Plan.RequiredOutcomes
	if p.evalErr != nil {
		return operation.OperationEvaluation{}, p.evalErr
	}
	return operation.OperationEvaluation{Status: operation.EvalPartialAnswerPossible, Confidence: "high", Reason: "measurement ready; source explanation remains unverified"}, nil
}

func TestTurnOutcomesUnavailableEvaluationCannotClaimCompletion(t *testing.T) {
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = t.TempDir()
	planner := &shortOutcomeCLIPlanner{evalErr: errors.New("evaluation unavailable"), fakeCLICommandPlanner: fakeCLICommandPlanner{req: operation.CommandOperationRequest{WorkDir: policy.DefaultWorkDir, RiskLevel: "low", Steps: []operation.CommandStep{{ID: "read", Shell: "printf 7", RiskLevel: "low"}}}}}
	answer, err := RunCommandOperationCLI(context.Background(), "request", TurnPolicy{Route: RouteOperation, Operation: "computer_operation", NeedsOperationAccess: true, RequiredOutcomes: types.TurnOutcomeSourceExplanation}, CommandOperationCLIConfig{Planner: planner, Policy: policy, RepoRoot: policy.DefaultWorkDir, RuntimeAnchor: t.TempDir()})
	if err != nil || !strings.Contains(answer, "was not verified") {
		t.Fatalf("unverified results became completion: %v %s", err, answer)
	}
	plan := operation.CommandOperationPlan{RequiredOutcomes: types.TurnOutcomeSourceExplanation}
	for _, status := range []operation.OperationStatus{operation.StatusReady, operation.StatusFailed, operation.StatusBlocked} {
		if _, ok := commandOperationUnevaluatedOutcomes(plan, operation.CommandOperationResult{Status: status}, nil); ok {
			t.Fatalf("non-executed %s misclassified", status)
		}
	}
	if _, ok := commandOperationUnevaluatedOutcomes(plan, operation.CommandOperationResult{Status: operation.StatusExecuted}, &operation.OperationEvaluation{Status: operation.EvalComplete}); ok {
		t.Fatal("verified result downgraded")
	}
}

func TestTurnOutcomesShortCommandStillChecksIndependentResults(t *testing.T) {
	for _, outcomes := range []types.TurnOutcomeSet{types.TurnOutcomeMeasurement, types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation} {
		policy := operation.DefaultCommandPolicy()
		policy.DefaultWorkDir = t.TempDir()
		planner := &shortOutcomeCLIPlanner{fakeCLICommandPlanner: fakeCLICommandPlanner{req: operation.CommandOperationRequest{Text: "two requested results", WorkDir: policy.DefaultWorkDir, RiskLevel: "low", Steps: []operation.CommandStep{{ID: "count", Shell: "printf 7", RiskLevel: "low"}}}}}
		var progress bytes.Buffer
		answer, err := RunCommandOperationCLI(context.Background(), "two requested results", TurnPolicy{Route: RouteOperation, Operation: "computer_operation", NeedsOperationAccess: true, RequiredOutcomes: outcomes, RiskLevel: "low"}, CommandOperationCLIConfig{Planner: planner, Policy: policy, RepoRoot: policy.DefaultWorkDir, RuntimeAnchor: t.TempDir(), Progress: &progress})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes.NeedsGoalEvaluation() {
			if planner.evaluations != 1 || planner.outcomes != outcomes || !strings.Contains(answer, "remains unverified") {
				t.Fatalf("short stdout bypassed completion: %d %v %s", planner.evaluations, planner.outcomes, answer)
			}
		} else if planner.evaluations != 0 {
			t.Fatal("simple measurement gained model call")
		}
	}
}

func TestTurnOutcomesPersistThroughCommandPlanAndRepairContext(t *testing.T) {
	outcomes := types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation
	p := operation.BuildCommandOperationPlan(operation.CommandOperationRequest{Text: "request", RequiredOutcomes: outcomes}, operation.DefaultCommandPolicy())
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored operation.CommandOperationPlan
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if commandOperationPolicyFromPlan(restored).RequiredOutcomes != outcomes {
		t.Fatal("resume lost goals")
	}
	if !strings.Contains(renderCommandPlanForPrompt(restored), "source_explanation") {
		t.Fatal("goal checker/answerer cannot see goals")
	}
}
