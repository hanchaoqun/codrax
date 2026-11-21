package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 fix round 4 (review round 3, R4-3): the wakeup-anchored seat of a
// three-way split is found by typed identity — same thread, same state
// family, the owner marker — never by value. The seat publishes the thread's
// whole share inside its unwalked waits; a row whose account holds only part
// of it (a satellite, one of several census-group seats) carries its own part.
// Round 3 matched by exact value: such a row said the seat was not on the
// board (it was), and its pair sentence claimed the three seats restore an
// account they do not sum to.

// hmc377PartialSatellite is a scheduler-latency satellite of
// Executor$Worker-10894 whose inventory holds 0.006 of the thread's 0.012 ms
// inside the unwalked waits: 206.856 = 29.904 walked + 0.006 + 176.946.
func hmc377PartialSatellite() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hbr-sat",
		Subject: "Executor$Worker-10894", Object: "scheduler_latency", TypeToken: "scheduler_latency",
		StateKind: "runnable", ChainRelevance: "adjacent",
		ImpactMS: 176.946, CumulativeImpactMS: 176.946,
		ChainAnchoredMS: 29.904, ChainAnchorFullMS: 206.856, ChainAnchorUnwalkedMS: 0.006, ChainAnchorOutsideMS: 176.946,
		ChainAnchorRemainderSeat: true,
		// HMC377 fix round 5: the engine-resolved typed claims — the seat
		// holding the unwalked part publishes more than this row's part
		// (not exact).
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 29.904, true, "hbr-chain"),
			hmc377Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 0.006, false, "hbr-own")},
		Rank: 2, Confidence: 0.78, LineStart: 10, LineEnd: 40,
	}
}

func hmc377PartialRows(model runtimeTraceProjTreeModel) map[string]*runtimeTraceProjTreeRow {
	out := map[string]*runtimeTraceProjTreeRow{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent} {
		for i := range rows {
			out[rows[i].Node.EvidenceID] = &rows[i]
		}
	}
	return out
}

func TestHMC377PartialAccountNamesTheWakeupAnchoredSeatByIdentity(t *testing.T) {
	p := hmc377ThreeWayProjection(true, true)
	p.AdjacentCauses = append(p.AdjacentCauses, hmc377PartialSatellite())
	model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	rows := hmc377PartialRows(model)
	chain, own, rem, sat := rows["hbr-chain"], rows["hbr-own"], rows["hbr-remainder"], rows["hbr-sat"]
	if chain == nil || own == nil || rem == nil || sat == nil {
		t.Fatalf("fixture drifted: chain=%v own=%v rem=%v sat=%v", chain != nil, own != nil, rem != nil, sat != nil)
	}
	ownTag, chainTag := strings.TrimSpace(own.EvidenceTag), strings.TrimSpace(chain.EvidenceTag)
	if sat.ChainAnchorUnwalkedInvisible || sat.ChainAnchorUnwalkedRef != ownTag || sat.ChainAnchorUnwalkedSeatMS != 0.012 {
		t.Fatalf("the satellite finds the seat holding the thread's share by identity (ref %q, invisible %t, seat %.3f)",
			sat.ChainAnchorUnwalkedRef, sat.ChainAnchorUnwalkedInvisible, sat.ChainAnchorUnwalkedSeatMS)
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	for _, pin := range []string{
		"同源三分:全窗206.856ms=锚定29.904ms(⛓链上席)+唤醒锚定0.006ms(本行这份;唤醒锚定席[" + ownTag + "]持本线程全部0.012ms)+本行其余176.946ms",
		"同源三分对席:⛓席=[" + chainTag + "],唤醒锚定席=[" + ownTag + "],◇席=本行;唤醒锚定席持本线程全部0.012ms,本行账只含其中0.006ms——本行账三份合计还原全窗账 206.856ms,三席之和不还原(规则见图例)",
		// The formal remainder holds the thread's whole share: its pair
		// still restores the account with the three seats.
		"同源三分:全窗206.862ms=锚定29.904ms(⛓链上席)+唤醒锚定0.012ms(⛓唤醒锚定席)+本行其余176.946ms",
	} {
		if !rspaFenceContains(fence, pin) {
			t.Fatalf("missing %q:\n%s", pin, fence)
		}
	}
	if hmc377R7RetiredAbsenceWord(fence) != "" {
		t.Fatalf("the wakeup-anchored seat is on this board — no row may say otherwise:\n%s", fence)
	}
	if strings.Contains(strings.ReplaceAll(fence, " ", ""), "三席合计还原全窗账206.856ms") {
		t.Fatalf("the three seats (29.904 + 0.012 + 176.946) do not sum to the satellite's 206.856 account:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	for _, pin := range []string{
		"0.006ms wakeup-anchored (this row's part; the wakeup-anchored seat [" + ownTag + "] holds the thread's whole 0.012ms)",
		"the wakeup-anchored seat holds the thread's whole 0.012ms and this row's account holds 0.006ms of it — the three parts of this row's account restore the whole-window account 206.856ms, the three seats' sum does not (rule in the legend)",
	} {
		if !rspaFenceContains(fenceEN, pin) {
			t.Fatalf("en missing %q:\n%s", pin, fenceEN)
		}
	}
	if hmc377R7RetiredAbsenceWord(fenceEN) != "" {
		t.Fatalf("en: the wakeup-anchored seat is on this board:\n%s", fenceEN)
	}
}

// The ⛓ side of a pair whose ◇ account holds only part of the thread's share
// names the same parts (the pair arm carries the seat's value to it).
func TestHMC377PartialAccountPairReadsTheSameFromTheChainSide(t *testing.T) {
	p := hmc377ThreeWayProjection(true, true)
	p.AdjacentCauses = []types.TraceCausalProjectionNode{hmc377PartialSatellite()}
	model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	rows := hmc377PartialRows(model)
	chain, own, sat := rows["hbr-chain"], rows["hbr-own"], rows["hbr-sat"]
	if chain == nil || own == nil || sat == nil {
		t.Fatalf("fixture drifted")
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	pin := "同源三分对席:◇席=[" + strings.TrimSpace(sat.EvidenceTag) + "],唤醒锚定席=[" + strings.TrimSpace(own.EvidenceTag) +
		"],⛓席=本行;唤醒锚定席持本线程全部0.012ms,◇席账只含其中0.006ms——◇席账三份合计还原全窗账 206.856ms,三席之和不还原(规则见图例)"
	if !rspaFenceContains(fence, pin) {
		t.Fatalf("missing %q:\n%s", pin, fence)
	}
}

// The review's render evidence (zz_rv3_render_test.go) as a real test: the
// delivered projection of the batch's synthetic unwalked capture plus one
// partial satellite row — a copy of the delivered ◇ three-way remainder whose
// account holds half of dep.worker's unwalked share. The satellite names the
// delivered wakeup-anchored seat by its [E#] and never claims the three seats
// restore its account.
func TestHMC377DeliveredProjectionPartialSatelliteNamesItsSeat(t *testing.T) {
	d := hmc377UnwalkedDeliver(t)
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = d.results
	set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
	if len(set.Projections) == 0 {
		t.Fatalf("no delivered projection")
	}
	p := set.Projections[0]
	var remainder, seat *types.TraceCausalProjectionNode
	for _, nodes := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses} {
		for i := range nodes {
			n := &nodes[i]
			if !strings.HasPrefix(n.Subject, "dep.worker") {
				continue
			}
			switch {
			case n.ChainAnchorUnwalkedSeat:
				seat = n
			case n.ChainAnchorRemainderSeat && n.ChainAnchorUnwalkedMS > 0:
				remainder = n
			}
		}
	}
	if seat == nil || remainder == nil {
		t.Fatalf("the delivered projection carries dep.worker's wakeup-anchored seat and three-way remainder (seat=%v remainder=%v)", seat != nil, remainder != nil)
	}
	sat := *remainder
	sat.EvidenceID += "#partial-satellite"
	sat.Object, sat.TypeToken = "scheduler_latency", "scheduler_latency"
	sat.ChainAnchorUnwalkedMS = remainder.ChainAnchorUnwalkedMS / 2
	sat.ChainAnchorFullMS = sat.ChainAnchoredMS + sat.ChainAnchorUnwalkedMS + sat.ChainAnchorOutsideMS
	// HMC377 fix round 5: the forged satellite's typed claims are forged with
	// it — the engine claims this row's own part, which the seat (holding the
	// thread's whole share) does not publish exactly.
	sat.ShareClaims = append([]types.TraceCausalProjectionShareClaim(nil), remainder.ShareClaims...)
	for i := range sat.ShareClaims {
		if sat.ShareClaims[i].Part == string(tracequery.RootCauseShareClaimUnwalked) {
			sat.ShareClaims[i].ShareMS, sat.ShareClaims[i].Exact = sat.ChainAnchorUnwalkedMS, false
		}
	}
	p.AdjacentCauses = append(p.AdjacentCauses, sat)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		var satRow, seatRow *runtimeTraceProjTreeRow
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent} {
			for i := range rows {
				switch rows[i].Node.EvidenceID {
				case sat.EvidenceID:
					satRow = &rows[i]
				case seat.EvidenceID:
					seatRow = &rows[i]
				}
			}
		}
		if satRow == nil || seatRow == nil {
			t.Fatalf("the satellite and the seat render (sat=%v seat=%v)", satRow != nil, seatRow != nil)
		}
		if satRow.ChainAnchorUnwalkedInvisible || satRow.ChainAnchorUnwalkedRef != strings.TrimSpace(seatRow.EvidenceTag) {
			t.Fatalf("zh=%t: the satellite names the delivered wakeup-anchored seat %q, got ref %q invisible %t",
				zh, seatRow.EvidenceTag, satRow.ChainAnchorUnwalkedRef, satRow.ChainAnchorUnwalkedInvisible)
		}
		fence := strings.ReplaceAll(rspaFenceJoined(runtimeTraceProjTreeFence(model, zh)), " ", "")
		claim := "三席合计还原全窗账" + formatThreeDecimals(sat.ChainAnchorFullMS) + "ms"
		partial := "唤醒锚定席持本线程全部" + formatThreeDecimals(runtimeTraceProjNodeDisplayImpact(*seat)) + "ms,本行账只含其中" + formatThreeDecimals(sat.ChainAnchorUnwalkedMS) + "ms"
		if !zh {
			claim = "thethreeseats'sumrestoresthewhole-windowaccount" + formatThreeDecimals(sat.ChainAnchorFullMS) + "ms"
			partial = "thewakeup-anchoredseatholdsthethread'swhole" + formatThreeDecimals(runtimeTraceProjNodeDisplayImpact(*seat)) + "msandthisrow'saccountholds" + formatThreeDecimals(sat.ChainAnchorUnwalkedMS) + "msofit"
		}
		if strings.Contains(fence, claim) {
			t.Fatalf("zh=%t: the three seats do not sum to the satellite's account:\n%s", zh, fence)
		}
		if !strings.Contains(fence, partial) {
			t.Fatalf("zh=%t: the satellite says which part of the seat's share it covers (%q):\n%s", zh, partial, fence)
		}
	}
}

func formatThreeDecimals(v float64) string { return fmt.Sprintf("%.3f", v) }

// Review round 3, note 1: the relation legend entry said the ⛓ + ◇ pair alone
// restores the account; on a three-way split it takes the three parts, and the
// three-way entry says what a partial account's row states.
func TestHMC377RelationLegendCoversTheThreeWaySplit(t *testing.T) {
	var relation, threeWay *runtimeTraceProjLegendEntry
	catalog := runtimeTraceProjLegendCatalog()
	for i := range catalog {
		switch catalog[i].Mark {
		case runtimeTraceProjMarkChainAnchorRelation:
			relation = &catalog[i]
		case runtimeTraceProjMarkChainAnchorThreeWay:
			threeWay = &catalog[i]
		}
	}
	if relation == nil || threeWay == nil {
		t.Fatalf("both legend entries are in the catalog")
	}
	for _, pin := range []string{"同源三分时还原全窗值的是三份", "两席单独相加不还原"} {
		if !strings.Contains(relation.ZH, pin) {
			t.Fatalf("the relation entry covers the three-way split (%q): %s", pin, relation.ZH)
		}
	}
	for _, pin := range []string{"on a three-way split the account is restored by three parts", "the ⛓ and ◇ seats alone do not sum back to it"} {
		if !strings.Contains(relation.EN, pin) {
			t.Fatalf("en: the relation entry covers the three-way split (%q): %s", pin, relation.EN)
		}
	}
	if !strings.Contains(threeWay.ZH, "三席之和不还原") || !strings.Contains(threeWay.EN, "the three seats' sum does not") {
		t.Fatalf("the three-way entry says what a partial account's row states:\n%s\n%s", threeWay.ZH, threeWay.EN)
	}
}
