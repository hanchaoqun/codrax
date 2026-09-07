package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A display/workset budget limits new identities, not amendments to evidence
// already admitted into that workset. Exercise the conjunction of full pools
// and append-tail corrections, not only the two cases independently.
func TestEnrichmentFullPoolKeepsAdmittedIdentityCorrections(t *testing.T) {
	for _, limit := range []int{1, 128, 256, 1024} {
		for _, tail := range []string{"turn_a", "mutable", "both"} {
			t.Run(fmt.Sprintf("%d/%s", limit, tail), func(t *testing.T) {
				ctx, correction := enrichmentCapacityFixture(limit, tail)
				before, _ := json.Marshal(ctx.EvidenceItems)
				got := answerDocTypedEnrichmentEvidencePool(ctx, limit)
				if len(got) != limit {
					t.Fatalf("correction changed the identity budget: got %d want %d", len(got), limit)
				}
				want := types.MergeEvidenceItemByStableID(ctx.EvidenceItems[0], correction)
				if !reflect.DeepEqual(got[0], want) || types.ClaimFormOf(got[0]) != types.ClaimCallEdge {
					t.Fatalf("full pool lost canonical same-ID amendment: got=%+v want=%+v", got[0], want)
				}
				for i := range got {
					if got[i].ID != ctx.EvidenceItems[i].ID {
						t.Fatalf("late new identity displaced ranked member %d", i)
					}
				}
				after, _ := json.Marshal(ctx.EvidenceItems)
				if string(before) != string(after) {
					t.Fatal("pool reconciliation mutated the upstream truth set")
				}
			})
		}
	}
}

func TestEnrichmentFullPoolCorrectionReachesRelationAuthority(t *testing.T) {
	ctx, _ := enrichmentCapacityFixture(1024, "mutable")
	_, edges, _, _ := answerDocCurrentSourceMechanismRelations(ctx)
	for _, edge := range edges {
		if edge.relation == types.DiagramRelCall && edge.from == "Service.Handle" && edge.to == "Store.Save" {
			return
		}
	}
	t.Fatalf("an already-admitted corrected call disappeared before relation compilation: %+v", edges)
}

func enrichmentCapacityFixture(limit int, tail string) (*types.AgentContext, types.EvidenceItem) {
	prior := types.EvidenceItem{
		ID: "corrected-call", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		Source: "service.go", LineStart: 30, LineEnd: 30,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Store.Save",
		GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence,
	}
	correction := prior
	correction.Kind, correction.AnchorKind = types.EvidenceRelationship, types.AnchorCall
	correction.Subject, correction.Predicate, correction.Object = "Service.Handle", "calls", "Store.Save"
	items := []types.EvidenceItem{prior}
	for i := 1; i < limit; i++ {
		items = append(items, types.EvidenceItem{ID: fmt.Sprintf("filler-%d", i), Kind: types.EvidenceDirect})
	}
	mu := types.NewMutableState("")
	// New identities precede the correction to catch an early break after cap.
	late := []types.EvidenceItem{{ID: "not-admitted", Kind: types.EvidenceDirect}, correction}
	if tail == "turn_a" || tail == "both" {
		mu.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: late})
	}
	if tail == "mutable" || tail == "both" {
		mu.AppendEvidence(late)
	}
	return &types.AgentContext{
		AnalysisIR:    &types.AnalysisIR{RequestModel: types.RequestModel{PredicateAxis: types.AxisFlow}},
		EvidenceItems: items, Mutable: mu,
	}, correction
}
