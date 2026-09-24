package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMeasurementActualFinalizerHandoff(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx, result, native := ioInFlightPublicContext(t, lang, scope, "")
				before, _ := json.Marshal(result)
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				choices := view.RuntimeMeasurementContract.Choices()
				if len(choices) != 3*len(native.Groups) || len(choices) != 12 {
					t.Fatalf("actual producer -> accepted TurnA -> fresh contract lost views: %d", len(choices))
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, table := range choices {
					if !strings.Contains(prompt, "observation_id=\""+table.ObservationID+"\" view=\""+string(table.View)+"\"") {
						t.Fatalf("selector missing from actual finalizer instruction: %s/%s", table.ObservationID, table.View)
					}
				}
				for _, token := range []string{"Peak concurrent requests", "Starts inside query", "Actual start (s)", "0.998000", "1.012000", "preview_omitted_rows=", "No source evidence_items are needed"} {
					if !strings.Contains(prompt, token) {
						t.Errorf("same-population selector teaching/preview lost %q", token)
					}
				}
				if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
					t.Error("raw publication JSON leaked into the model prompt")
				}
				after, _ := json.Marshal(result)
				if string(before) != string(after) || ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("display selector altered native result or acquired causal authority")
				}
			})
		}
	}
}
