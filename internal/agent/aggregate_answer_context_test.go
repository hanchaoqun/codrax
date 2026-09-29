package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAggregateAnswerContextKeepsCoverageWithoutInputRoster(t *testing.T) {
	rm := types.RequestModel{Intent: types.IntentEnumerate,
		CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "every file"},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Role: types.RequestedAnswerDimensionCount, Required: true, Index: 1},
			{Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true, Index: 2},
		}},
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: types.NewMutableState("count and explain")}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "Every grounded match must appear") || !strings.Contains(prompt, "Requested Measurement Coverage") {
		t.Fatalf("actual finalizer message contradicts aggregate shape: %s", prompt)
	}
	if enumerationIntentForContext(ctx) {
		t.Fatal("explorer requests a roster for count+explanation")
	}
	rm.Predicates.IsCategoryEnumeration = true
	ctx.AnalysisIR.RequestModel = rm
	if !enumerationIntentForContext(ctx) {
		t.Fatal("explicit category enumeration lost")
	}
	if p := renderAnswerDocEnumerationBoundary(ctx, nil); !strings.Contains(p, "Every grounded match must appear") {
		t.Fatal("true exhaustive roster teaching lost")
	}
}
