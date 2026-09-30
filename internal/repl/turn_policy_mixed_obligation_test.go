package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestClassifyPolicyMixedObligationTeaching(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"required","operation":"investigate","source":"mixed","confidence":0.9,"reason":"all requested dimensions need evidence","requires_diagram":false}`)}}
	c := &llmChitchatClassifier{adapter: adapter}
	if _, err := c.ClassifyPolicy(context.Background(), "测量构建耗时并解释当前代码的构建路径", "", false); err != nil {
		t.Fatal(err)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{"Do not choose a route by the first or dominant subtask", "a requested source explanation remains required even when secondary"} {
		if !strings.Contains(system, want) || !strings.Contains(string(turnPolicyTool.Parameters), want) {
			t.Fatalf("prompt/schema contract drift: missing %q", want)
		}
	}
	if strings.Contains(system, "keep route=operation so the dispatcher can use") {
		t.Fatal("retired generic exception still overrides source obligations")
	}
}

func TestMixedObligationDoesNotInferSourceFromOperationDirectory(t *testing.T) {
	for _, mode := range []types.TurnRouteCurrentSourceEvidenceMode{types.TurnRouteCurrentSourceEvidenceRequired, types.TurnRouteCurrentSourceEvidenceOptional} {
		p := TurnPolicy{Route: RouteOperation, Operation: "computer_operation", OperationKind: "computer_operation", NeedsRepoAccess: true, NeedsOperationAccess: true, Source: "mixed", CurrentSourceEvidenceMode: mode, TargetSurface: "desktop", RiskLevel: "low", Confidence: 0.9}
		got := ApplyTurnPolicyGuards(p, false, false)
		if mode == types.TurnRouteCurrentSourceEvidenceOptional && got.Route != RouteOperation {
			t.Fatal("working directory alone was promoted to source proof")
		}
		if mode == types.TurnRouteCurrentSourceEvidenceRequired && got.Route != RouteRepo {
			t.Fatalf("typed source investigation lost: %+v", got)
		}
	}
}

func TestOperationGoalCompletionTeachingAgreesWithJSONSchema(t *testing.T) {
	if strings.Count(operationEvaluationSystemPrompt, operationGoalCompletionContract) != 1 || !strings.Contains(operationGoalCompletionContract, "describe it in reason") || !strings.Contains(operationGoalCompletionContract, "Keep missing_inputs empty unless user-owned") {
		t.Fatal("unfulfilled source obligation must not contradict user-owned missing_inputs schema")
	}
}
