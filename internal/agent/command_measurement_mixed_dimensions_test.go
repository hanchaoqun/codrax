package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMeasurementMechanismGuidancePreservesRequiredSecondaryDimensions(t *testing.T) {
	for _, intent := range []types.Intent{types.IntentReturnValue, types.IntentExplain, types.IntentEnumerate} {
		for _, role := range []types.RequestedAnswerDimensionRole{types.RequestedAnswerDimensionFunctionOrPurpose, types.RequestedAnswerDimensionBranchBehavior, types.RequestedAnswerDimensionRelationPath, types.RequestedAnswerDimensionStageWorkflow} {
			ctx := commandMeasurementEvidencePathTestContext(false)
			ctx.TurnRouteHint = types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			rm := &ctx.AnalysisIR.RequestModel
			rm.Intent = intent
			rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionCount, Required: true}, {Index: 2, Role: role, Required: true}}}
			if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx); !strings.Contains(got, "independent evidence carriers") {
				t.Fatalf("secondary dimension lost: %s/%s", intent, role)
			}
			ctx.Stage = types.StageFinalize
			if got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); !strings.Contains(got, "Current-source measurement context") {
				t.Fatalf("finalizer lost %s/%s", intent, role)
			}
			rm.RequestedAnswerDimensions.Dimensions[1].Required = false
			if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx); got != "" {
				t.Fatal("optional dimension promoted")
			}
		}
	}
}
