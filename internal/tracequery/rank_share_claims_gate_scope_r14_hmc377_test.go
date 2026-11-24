package tracequery

// HMC377 round 14 — batch-owner ruling R-377-14 ⑥⑨ (2026-11-24).
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
//
// ⑨ pins what review 13 found unpinned on the placement of a withdrawn row's
// claimless detail: a hold without instants carries the withdrawn account only
// when the successor rule passed it (the role test — a successor hold alone
// carries roles); and on the one-successor form of a claim's words: it counts
// the rows that hold the successor-held part.

import (
	"math"
	"strings"
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

// R-377-14 ⑨ (M09 / A15f): a hold without instants carries a withdrawn
// account only when the successor rule passed it. A published row of the same
// thread whose own hold has no instants (a seat no segment inventory
// reproduced) holds its own account, never the withdrawn seat's.
func TestHMC377R14OnlyAPassedHoldWithoutInstantsCarriesTheWithdrawnAccount(t *testing.T) {
	inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
	// The thread's ◇ remainder holds 1.000 without instants and makes no
	// claim on the chain seat's windows: it is no successor of the seat.
	rem := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1, RunnableMs: 1, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: 10.5, EndTs: 10.501, LineStart: 70, LineEnd: 80,
		ChainAnchorRemainderSeat: true, Summary: "binder:458_5-6326 remainder"}
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, nil, 1)
	rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, rem, hmc377R13SchedTwin(inv)})
	if len(rank.AbsorbedItems) != 1 {
		t.Fatalf("precondition: the chain seat absorbs its twin, got %d details", len(rank.AbsorbedItems))
	}
	pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
	for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
		t.Errorf("%s", f)
	}
	priced := hmc377R14Priced(pub)
	if len(pub.AbsorbedItems) != 0 || len(priced) != 2 || priced[1].Type != "scheduler_latency" || math.Abs(priced[1].RunnableMs-1.903) > 0.0005 {
		t.Errorf("priced %s details %s; want the ◇ remainder and the twin on its own row at 1.903",
			hmc377R14Describe(priced), hmc377R14Describe(pub.AbsorbedItems))
	}
	if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.903) > 0.0015 {
		t.Errorf("Σ carriers %.3f, want raw 2.903", got)
	}
}

// R-377-14 ⑨ (A03): the one-successor form is decided by the rows that hold
// the successor-held part, not by every row the claim names. A chain seat of
// the thread no segment inventory reproduced is a holder by value and places
// no instant: the whole share stays with the one successor row.
func TestHMC377R14SuccessorFormCountsTheRowsHoldingTheSuccessorPart(t *testing.T) {
	// The second chain seat of 6326 holds its role without instants.
	seat := hmc377R11ChainRun(1.0, []foldInterval{{start: 10.6, end: 10.601}}, 50, 55)
	seat.rspaHeld = nil
	rspaHold(&seat, RootCauseShareHolderChainSeat, false, nil, 1.0)
	pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
		seat, hmc377R13Rem(2.0, hmc377R11W(), 70), hmc377R11Satellite1()}), hmc377R13IsChainInversion, nil)
	found := false
	for _, it := range pub.Items {
		if !strings.Contains(it.Summary, "runnable satellite") {
			continue
		}
		found = true
		if len(it.ShareClaims) != 1 || len(it.ShareClaims[0].holders()) != 2 {
			t.Fatalf("precondition: the satellite's claim names the successor row and the seat without instants: %+v", it.ShareClaims)
		}
		if !strings.Contains(it.Summary, rspaRepresentedSuccessor(1.903, 1.903)) {
			t.Errorf("the whole share is held by one successor row; got %q", it.Summary)
		}
	}
	if !found {
		t.Fatal("no satellite published")
	}
	// Raw: W 1.903 + the seat 1.000 + the ◇ rest 2.000.
	if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-4.903) > 0.0015 {
		t.Errorf("Σ carriers %.3f, want raw 4.903", got)
	}
}
