package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Real TraceQuery observations enter the actual finalizer initial instruction.
// The policy must distinguish overlapping rulers from disjoint state partitions
// without changing any source measurement, completion proof or model text.
func TestCausalIOAdditionTeachingPublicInitialInstruction(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, state := range []string{"S", "D"} {
			for _, closed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/closed=%t", lang, state, closed), func(t *testing.T) {
					ctx := b1645ActualCausalIOContext(t, state, lang, closed)
					causalIOInstructionContext(ctx)
					ledger := answerDocObservationLedger(ctx)
					snapshot := func() string {
						current := answerDocObservationLedger(ctx)
						return b1645Snapshot(t, []any{ctx.AnalysisIR, ctx.Mutable.TurnAArtifacts(), current,
							types.CompileTraceCausalProjectionSet(current), types.BuildTraceBlockingWallClockAuthorities(current, &ctx.AnalysisIR.RequestModel), ctx.Mutable.AnswerDocumentV2()})
					}
					before := snapshot()
					prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
					if len(ledger.Records) == 0 || strings.Count(prompt, "### IO Measurements For Causal Interpretation") != 1 {
						t.Fatal("actual native observations did not reach the causal initial instruction")
					}
					for _, want := range []string{
						"Do not blindly add overlapping request-residence and issuer-wait intervals or totals from different threads or requests.",
						"For the same thread and window, mutually exclusive adjacent scheduler states may be summed using the published native state account",
						"sleep plus runnable waiting can describe off-CPU time",
						"This does not make request residence equivalent to issuer blocking or establish a cross-thread response contribution",
						"attributing it to another thread's response still requires an independent causal chain",
						"Missing closure is unproven, not proof of zero IO",
						"request_residence=`0.100`", fmt.Sprintf("completion_woke_issuer=`%t`", closed),
					} {
						if !strings.Contains(prompt, want) {
							t.Errorf("actual initial instruction missing %q", want)
						}
					}
					if strings.Contains(prompt, "Request residence, issuer blocking, scheduler delay, and cross-request aggregates are not additive.") {
						t.Error("blanket non-addition rule still contradicts the native state partition")
					}
					if closed != strings.Contains(prompt, "issuer_blocked=`0.090`") {
						t.Fatal("teaching lost or invented the native completion-closed wait")
					}
					if before != snapshot() {
						t.Fatal("teaching mutated source evidence, window, causal eligibility or model answer")
					}
				})
			}
		}
	}
}
