package tracequery

// HMC377 round 14 — batch-owner ruling R-377-14 ⑥ (2026-11-24).
//
// ⑥ scopes R-377-13 ⑤ ("a withdrawn row absorbs nothing") by the gate's
// closed type set: the publication gate withdraws priority-inversion rows
// only, and only the B4 reconcile gives such a row a family key — always
// together with its absorbed-row count. The other two reconciliation lanes
// key their own family rows: G1 a D/IO family row, the IO-facet lane the
// family row it synthesizes. Two conjuncts that restated this in the
// withdrawal (`AbsorbedRankRows > 0` on the withdrawn row's key, `successor`
// beside the role test of a hold passed whole) are deleted; the disjointness
// of the closed sets is pinned here, so widening either set fails this test
// and needs a new ruling.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC377R14GateTypesOwnNoOtherLanesFamily(t *testing.T) {
	// The gate's closed set, as the tool's publication boundary reads it.
	gate := []string{"priority_inversion_candidate", "priority_inversion_runnable_wait"}
	for _, typ := range gate {
		if !types.TraceRootCauseTypeIsPriorityInversion(typ) {
			t.Fatalf("precondition: %s is a type the publication gate can withdraw", typ)
		}
	}
	withdrawable := 0
	for _, typ := range CausalTokenUniverse() {
		if types.TraceRootCauseTypeIsPriorityInversion(typ) {
			withdrawable++
		}
	}
	if withdrawable != len(gate) {
		t.Errorf("the gate can withdraw %d registered types, want exactly %v — a wider gate needs a ruling on which lane's family a withdrawn row can own", withdrawable, gate)
	}
	// G1 keys a family row of one of these types (the matched family's own
	// token) and reads the critical row's token to find it.
	g1 := map[string]bool{}
	for critical, families := range criticalBlockingRankFamilyReconTypes {
		g1[critical] = true
		for _, fam := range families {
			g1[fam] = true
		}
	}
	// The IO-facet lane keys the family row it synthesizes; its carrying type
	// is one of the lead facets, each a member of the closed facet set.
	facet := map[string]bool{}
	for typ := range adjacentIOFacetFamilyTypes {
		facet[typ] = true
	}
	for _, lead := range adjacentIOFacetLeadOrder {
		if !adjacentIOFacetFamilyTypes[lead] {
			t.Errorf("IO-facet lead type %s is outside the closed facet set", lead)
		}
		facet[lead] = true
	}
	if len(g1) == 0 || len(facet) == 0 {
		t.Fatalf("precondition: both lanes have a closed family-row type set (G1 %d, IO facet %d)", len(g1), len(facet))
	}
	for lane, set := range map[string]map[string]bool{"G1": g1, "IO facet": facet} {
		for typ := range set {
			if types.TraceRootCauseTypeIsPriorityInversion(typ) {
				t.Errorf("%s family-row type %s is a type the publication gate can withdraw: a withdrawn row could then wear that lane's family key without a B4 absorbed-row count", lane, typ)
			}
		}
		for _, typ := range gate {
			if set[typ] {
				t.Errorf("gate type %s is a %s family-row type", typ, lane)
			}
		}
	}
}
