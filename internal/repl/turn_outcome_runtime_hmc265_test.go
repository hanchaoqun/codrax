package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC265OutcomeTeachingSeparatesRecordedStacksFromCurrentSource(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"required_outcomes":["measurement"],"route":"repo","operation":"investigate","source":"artifact","current_source_evidence_mode":"optional","confidence":0.95,"reason":"recorded frames","requires_diagram":false}`)}}
	p, err := (&llmChitchatClassifier{adapter: adapter}).ClassifyPolicy(context.Background(), "查看采样记录里的调用栈", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if TurnRouteHintFromPolicy(p).RequiresCurrentSourceEvidence() {
		t.Fatal("measurement gained independent source obligation")
	}
	prompt := adapter.calls[0].messages[0].Content
	if strings.Count(prompt, turnOutcomesTeaching) != 1 || !strings.Contains(turnOutcomesTeaching, "Recorded runtime stacks") {
		t.Fatal("sole teaching lacks recorded-stack contrast")
	}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(turnPolicyTool.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema.Properties["required_outcomes"].Description, "current implementation") {
		t.Fatal("outcome field leaves source meaning implicit")
	}
	p.RequiredOutcomes |= types.TurnOutcomeSourceExplanation
	if !TurnRouteHintFromPolicy(ApplyTurnPolicyGuards(p, false, false)).RequiresCurrentSourceEvidence() {
		t.Fatal("real source obligation was removed")
	}
}
