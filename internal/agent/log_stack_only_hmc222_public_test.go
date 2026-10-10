package agent

import (
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC222StackOnlyPublicEmitLedgerFinalizer(t *testing.T) {
	const captured = "cache.(*Store).Get(0x0, ...)"
	for _, withFrame := range []bool{true, false} {
		var frames []any
		if withFrame {
			frames = append(frames, map[string]any{"raw": captured, "func": "cache.(*Store).Get", "confidence": 1})
		}
		bus := emitHMC222Log(t, captured+"\n", []map[string]any{{"type": "UnverifiedDiagnosis", "frames": frames}})
		bundle := bus.Mutable.LogTriage()
		rm := types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause, LogTriage: bundle,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"only attached log"}, Confidence: 1}}
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
		bus.Mutable.SetRequestModel(rm)
		ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
		bindings := types.CompileRuntimeArtifactClaimBindings(&rm, nil)
		want := 0
		if withFrame {
			want = 1
		}
		if len(ledger.Records) != want || len(bindings) != want {
			t.Fatalf("withFrame=%v: ledger=%+v bindings=%+v", withFrame, ledger.Records, bindings)
		}
		if withFrame {
			row := ledger.Records[0]
			if row.Subject != captured || row.Predicate != "stack_frame" || row.ProvenanceLane != types.ObservationProvenanceArtifactSpan || row.ClaimAuthority != types.ObservationClaimAuthorityDirectObservation {
				t.Fatalf("frame either lost or upgraded to an error/cause: %+v", row)
			}
			if row.SourceRef.Path != "" || row.Span.LineStart != 0 || bindings[0].TargetRef != captured {
				t.Fatalf("frame borrowed error/source coordinates: %+v / %+v", row, bindings)
			}
		}
		ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		if withFrame && (!strings.Contains(prompt, captured) || !strings.Contains(prompt, "claim_authority=`direct_observation`")) {
			t.Fatalf("finalizer lost valid stack support: %s", prompt)
		}
		if strings.Contains(prompt, "preserve their verified source-line spelling: `UnverifiedDiagnosis`") {
			t.Fatalf("frame promoted a diagnostic Type: %s", prompt)
		}
	}
}
