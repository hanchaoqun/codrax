package tool

// HMC377 round 14 — batch-owner ruling R-377-14 ⑥ (2026-11-24), the tool's
// half: "a withdrawn row absorbs nothing" is scoped by the publication gate's
// closed type set. The engine's withdrawal reads a withdrawn row's family key
// as a B4 absorber's (tracequery pins that no other reconciliation lane keys a
// row of a gate type: TestHMC377R14GateTypesOwnNoOtherLanesFamily); this test
// pins the other side — the gate withdraws rows of exactly the types that
// predicate names, over the whole registered token universe. A gate that
// learns to withdraw another type fails here and needs a ruling on which
// lane's family such a row can own.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC377R14PublicationGateWithdrawsOnlyPriorityInversionTypes(t *testing.T) {
	withdrawn := map[string]bool{}
	for _, typ := range tracequery.CausalTokenUniverse() {
		// A row that proves nothing: no priority relation, no provenance.
		row := tracequery.RootCauseRankItem{Rank: 1, Tier: "primary", Type: typ, Source: "wakeup_chain.causal_impacts",
			Thread: hmc377R9Dep, ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903,
			DominantState: "runnable", ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
			StartTs: 10.2, EndTs: 10.201903, LineStart: 10, LineEnd: 20}
		published, gone := traceQueryRootCauseRankWireItemPublicationInUniverse(row, traceQueryPriorityOpenArtifactUniverse())
		if gone != types.TraceRootCauseTypeIsPriorityInversion(typ) {
			t.Errorf("type %s: gate withdrew=%v, the closed set says %v", typ, gone, types.TraceRootCauseTypeIsPriorityInversion(typ))
		}
		if gone {
			withdrawn[typ] = true
			if published.Tier != tracequery.RootCauseTierContextOnly {
				t.Errorf("type %s: a withdrawn row is published as %q, want context only", typ, published.Tier)
			}
		}
	}
	if len(withdrawn) != 2 || !withdrawn["priority_inversion_candidate"] || !withdrawn["priority_inversion_runnable_wait"] {
		t.Errorf("the gate withdrew proof-less rows of %v, want exactly the two priority-inversion types", withdrawn)
	}
}
