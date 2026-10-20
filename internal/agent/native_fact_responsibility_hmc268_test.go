package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC268NativeFactResponsibilityPublicAdapter(t *testing.T) {
	result := readerFieldScopePublicResult(t)
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeBoundedEffectVerdict, types.RuntimeQuestionScopeCausalDiagnosis} {
		t.Run(string(scope), func(t *testing.T) {
			ctx := readerFieldScopePublicContext("en", scope, []types.ToolResult{result})
			before := readerFieldScopePublicSnapshot(t, ctx)
			prompt := hmc266NegativePrompt(t, ctx, "finalize")
			if before != readerFieldScopePublicSnapshot(t, ctx) {
				t.Fatal("teaching changed original facts or causal projection")
			}
			switch scope {
			case types.RuntimeQuestionScopeBoundedFactSet:
				if strings.Contains(prompt, "Present the model-owned bounded conclusion first") {
					t.Error("pure fact request is instructed to invent an additional bounded verdict")
				}
				if !strings.Contains(prompt, "does not require an additional business interpretation") {
					t.Error("pure fact responsibility is not separated from interpretation")
				}
			case types.RuntimeQuestionScopeBoundedEffectVerdict:
				if !strings.Contains(prompt, "Present the model-owned bounded conclusion first") || !strings.Contains(prompt, "does not decide yes/no/mixed/unproven") {
					t.Error("bounded effect lost independent verdict responsibility")
				}
			case types.RuntimeQuestionScopeCausalDiagnosis:
				if !strings.Contains(prompt, "Keep every typed on-chain candidate eligible") {
					t.Error("causal task lost on-chain business/root-cause responsibility")
				}
			}
		})
	}
}
