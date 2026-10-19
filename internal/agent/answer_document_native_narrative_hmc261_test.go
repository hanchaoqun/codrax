package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC261NativeNarrativeOwnershipSurvivesPresentationLimits(t *testing.T) {
	for _, mode := range []string{"sampling", "io", "unpublished", "long_tail", "sanitized"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, mode == "sampling")
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			if len(choices) == 0 {
				t.Fatal("missing public native choices")
			}
			const reason = "HMC261_LIFECYCLE_NOT_A_SECOND_DIAGNOSIS"
			const independent = "HMC261_INDEPENDENT_SOURCE_AND_CHAIN_BUSINESS_NOTE"
			interpretation := strings.Repeat("independent source and business boundary; ", 28) + "HMC261_AFTER_900_BYTE_BOUNDARY"
			if mode == "long_tail" {
				interpretation += strings.Repeat("独立业务边界。", 200) + "HMC261_UNPUBLISHED_TAIL"
			}
			if mode == "sanitized" {
				interpretation = "<think>HMC261_PRIVATE_SCRATCH</think>"
			}
			ta := ctx.Mutable.TurnAArtifacts()
			ta.InvestigationNotes = []string{reason, independent}
			ctx.Mutable.SetTurnAArtifacts(*ta)
			raw, _ := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
				"runtime_measurements": choices[:1], "interpretation": interpretation,
			}})
			result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
				t.Fatalf("public accepted completion: %v %s", err, result.Summary)
			}
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			ctx = ctx.ShallowClone()
			var publication *answerDocNativePresentationReceipt
			if mode != "unpublished" {
				publication = &answerDocNativePresentationReceipt{}
				renderAnswerDocRuntimeMeasurementChoices(ctx, publication)
			}
			text := renderAnswerDocInvestigationNarrativeHandoffWithPublication(ctx, publication)
			if strings.Contains(text, reason) {
				t.Error("presentation limit revived lifecycle reason as a second diagnosis")
			}
			if !strings.Contains(text, independent) || !utf8.ValidString(text) {
				t.Error("independent note or valid UTF-8 lost")
			}
			if mode != "sanitized" && !strings.Contains(text, "HMC261_AFTER_900_BYTE_BOUNDARY") {
				t.Error("old reason slot did not preserve interpretation within the shared narrative budget")
			}
			if mode == "long_tail" && (strings.Contains(text, "HMC261_UNPUBLISHED_TAIL") || !strings.Contains(text, "bytes omitted")) {
				t.Error("long interpretation must disclose its bounded view, not pretend complete publication")
			}
			if strings.Contains(text, "HMC261_PRIVATE_SCRATCH") {
				t.Error("sanitized scratch returned through another narrative lane")
			}
			if ctx.Mutable.StableInvestigationCompleteReason() != reason || ctx.Mutable.AcceptedInvestigationNativeFactHandoff().Interpretation != interpretation {
				t.Error("presentation changed original lifecycle or interpretation audit data")
			}
			projection := answerDocSeparatedNativeFactHandoff(ctx, publication)
			if !projection.ownsCurrentNarrative || projection.publishedInterpretationBytes > 1800 {
				t.Error("current owner lost or shared narrative budget enlarged")
			}
			if mode == "long_tail" && projection.interpretationBytes <= projection.publishedInterpretationBytes {
				t.Error("partial interpretation incorrectly marked complete")
			}
			if mode == "sanitized" && (!projection.sanitized || projection.publishedInterpretationBytes != 0) {
				t.Error("safety filtering was not represented independently of ownership")
			}
			if mode == "sampling" || mode == "io" {
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil)
				if strings.Contains(prompt, reason) || !strings.Contains(prompt, interpretation) || !strings.Contains(prompt, independent) {
					t.Error("actual finalizer context lost separated narrative ownership")
				}
			}
		})
	}
}
