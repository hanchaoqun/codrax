package tool

// HMC377 fix round 7 (R7-2, review round 6 finding 1; ruling R-377-8 value
// part) — a holder drawn under its causal impact's chain-view record.
//
// The renderer's drawn-holder lookup keyed only by the holder's rank record.
// When the rank cap removed that copy while the chain view drew its own record
// of the same wakeup causal impact (105 蜂鸟 container #1 TID 7912: pool
// priority_inversion_candidate of binder:458_5-6326 = wakeup_causal_impact:31,
// drawn as ⇅ E27(+1) with runnable(全额) 1.903; 105 元宝 container #2 TID
// 50874 net_thread-50925 → E21(+1); 105 网易 container #1 TID 22853
// ease.cloudmusic-22897 → E11(+1)), the holder read as not drawn — and a
// represented satellite in that position would keep its own value in the ◎
// population beside its drawn holder. Round 7 carries the causal impact's
// typed identity from the engine (HolderCausalImpacts → the share_claims
// note's eighth field → HolderImpactEvidenceIDs, the impact's chain-view
// record id on the same publication) and the lookup recognises it.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The record id the codec builds for a holder's causal impact is the id the
// emission gives that impact's chain-view record on the same publication.
func TestHMC377R7HolderImpactNamesTheEmittedChainViewRecord(t *testing.T) {
	dep := tracequery.ThreadRef{Comm: "binder:458_5", PID: 6326}
	other := tracequery.ThreadRef{Comm: "other", PID: 6400}
	result := tracequery.Result{
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "app", PID: 100},
			CausalImpacts: []tracequery.WakeupCausalImpact{
				{Thread: other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 1, TotalMs: 1, OnChain: true, LineStart: 5, LineEnd: 6},
				{Thread: dep, ChainDepth: 2, DominantState: "runnable", DominantImpactMs: 1.903, TotalMs: 1.903, OnChain: true, LineStart: 10, LineEnd: 20},
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{{
				Rank: 6, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: dep,
				ImpactMs: 89.986, CumulativeImpactMs: 89.986, EffectiveImpactMs: 89.986, RunnableMs: 89.986,
				ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76,
				ChainAnchoredMs: 1.903, ChainAnchorFullMs: 91.889, ChainAnchorRemainderSeat: true,
				ShareClaims: []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat,
					ShareMs: 1.903, HolderCausalImpacts: []int{2}}},
			}},
		},
	}
	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	ids := map[string]types.ObservationRecord{}
	var claimant types.ObservationRecord
	for _, r := range records {
		ids[r.ID] = r
		if strings.Contains(r.ID, "#root_cause_rank:") && r.Subject == "binder:458_5-6326" {
			claimant = r
		}
	}
	if claimant.ID == "" {
		t.Fatal("the claimant's rank record is emitted")
	}
	var value string
	for _, note := range claimant.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyShareClaims+"=") {
			value = strings.TrimPrefix(note, types.TraceNoteKeyShareClaims+"=")
		}
	}
	claims := types.ParseTraceShareClaimsNote(value, claimant.ID)
	if len(claims) != 1 || len(claims[0].HolderImpactEvidenceIDs) != 1 {
		t.Fatalf("the unresolved claim names its holder's causal impact: note %q → %+v", value, claims)
	}
	hop, ok := ids[claims[0].HolderImpactEvidenceIDs[0]]
	if !ok || hop.Predicate != "wakeup_causal_impact" || hop.Subject != "binder:458_5-6326" {
		t.Fatalf("the named record %q is the impact's emitted chain-view record: %+v", claims[0].HolderImpactEvidenceIDs[0], hop)
	}
}

// Only impacts whose own chain-view record this publication emits with the
// engine's account stand for a holder: inside the typed row cap, past the
// row guard, and with the inversion composition the engine minted the seat
// from (a record whose composition the priority filter withdrew publishes
// another account). A claim naming any impact without such a record names
// none.
func TestHMC377R7HolderImpactNeedsTheSameAccountOnThePublication(t *testing.T) {
	rowCap := traceQueryWidthTypedFamilyRowCap()
	chain := &tracequery.ChainResult{}
	for i := 0; i < rowCap+2; i++ {
		chain.CausalImpacts = append(chain.CausalImpacts, tracequery.WakeupCausalImpact{
			Thread: tracequery.ThreadRef{Comm: "dep", PID: 300 + i}, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 1, TotalMs: 1, OnChain: true})
	}
	chain.CausalImpacts[1] = tracequery.WakeupCausalImpact{Thread: tracequery.ThreadRef{Comm: "empty", PID: 900}}
	chain.CausalImpacts[2].PriorityInversionCandidate = true // soft caliber: the filter withdraws the candidate role
	chain.CausalImpacts[2].PriorityInversionGatedMs, chain.CausalImpacts[2].GatedRunnableMs = 1, 1
	records := traceQueryShareClaimImpactRecords(chain)
	for ordinal, want := range map[int]bool{1: true, 2: false, 3: false, 4: true, rowCap: true, rowCap + 1: false, rowCap + 2: false} {
		if records[ordinal] != want {
			t.Errorf("impact #%d stands for a holder = %t, want %t", ordinal, records[ordinal], want)
		}
	}
	item := tracequery.RootCauseRankItem{ShareClaims: []tracequery.RootCauseShareClaim{
		{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat, ShareMs: 1, HolderCausalImpacts: []int{1, 4}},
		{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat, ShareMs: 1, HolderCausalImpacts: []int{1, rowCap + 1}},
		{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat, ShareMs: 1, Resolved: true, HolderCausalImpacts: []int{1}},
	}}
	notes := traceQueryShareClaimsNotes(item, nil, records)
	if len(notes) != 1 || notes[0] != types.TraceNoteKeyShareClaims+"=anchored/chain_seat/0/0/0/1.000//1+4;anchored/chain_seat/0/0/0/1.000/;anchored/chain_seat/1/0/0/1.000/" {
		t.Fatalf("the note names an impact set only when every impact has its record: %v", notes)
	}
}

// hmc377R7ImpactClaim is an unresolved claim naming its holder's causal impact
// record.
func hmc377R7ImpactClaim(part tracequery.RootCauseShareClaimPart, represented bool, share float64, impacts ...string) types.TraceCausalProjectionShareClaim {
	return types.TraceCausalProjectionShareClaim{Part: string(part), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: share,
		Represented: represented, HolderImpactEvidenceIDs: impacts}
}

// The lookup and the ◎ eligibility recognise the causal impact's record: a ◇
// remainder names its anchored share's holder in the held form, a represented
// satellite names the drawn row's [E#] and leaves the ◎ population; the
// impact's record drawn nowhere (or only inside the claimant's own row) keeps
// the see-detail form and the satellite's own value.
func TestHMC377R7HolderDrawnAsItsCausalImpactRecord(t *testing.T) {
	const impact = "trace_query:pub#wakeup_causal_impact:31"
	for _, placement := range []string{"own", "merged", "nowhere", "self-rem", "self-sat"} {
		for _, zh := range []bool{true, false} {
			model := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
			hop := hmc377R6Row("trace_query:chain#wakeup_causal_impact:31", "binder:458_5-6326", "E27(+1)", runtimeTraceProjTreeRowChain)
			hop.Node.Predicate = "wakeup_causal_impact"
			switch placement {
			case "own":
				hop.Node.EvidenceID = impact
				model.TreeRows = append(model.TreeRows, hop)
			case "merged":
				hop.Node.MergedEvidenceIDs = []string{impact}
				model.TreeRows = append(model.TreeRows, hop)
			}
			remainder := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "rem", Subject: "binder:458_5-6326",
				TypeToken: "runnable_wait", StateKind: "runnable", ChainRelevance: "adjacent", ImpactMS: 89.986, CumulativeImpactMS: 89.986,
				EffectiveImpactMS: 89.986, Rank: 6, ChainAnchoredMS: 1.903, ChainAnchorFullMS: 91.889, ChainAnchorRemainderSeat: true,
				ShareClaims: []types.TraceCausalProjectionShareClaim{hmc377R7ImpactClaim(tracequery.RootCauseShareClaimAnchored, false, 1.903, impact)}},
				Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E43"}
			satellite := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "sat", Subject: "binder:458_5-6326",
				TypeToken: "scheduler_latency", Predicate: "root_cause_adjacent", StateKind: "runnable", ChainRelevance: "adjacent",
				ImpactMS: 1.903, CumulativeImpactMS: 1.903, EffectiveImpactMS: 1.903, Rank: 9, Confidence: 0.78,
				// The engine dropped the represented marker: the rank board
				// does not hold the share.
				ChainAnchorRepresentedByChainSeat: false,
				ShareClaims:                       []types.TraceCausalProjectionShareClaim{hmc377R7ImpactClaim(tracequery.RootCauseShareClaimAnchored, true, 1.903, impact)}},
				Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E44"}
			// The impact record drawn only inside the claimant's own row (each
			// claimant on its own board: the other claimant's row would draw
			// it for that one).
			switch placement {
			case "self-rem":
				remainder.Node.MergedEvidenceIDs = []string{impact}
				model.Adjacent = append(model.Adjacent, remainder)
			case "self-sat":
				satellite.Node.MergedEvidenceIDs = []string{impact}
				model.Adjacent = append(model.Adjacent, satellite)
			default:
				model.Adjacent = append(model.Adjacent, remainder, satellite)
			}
			runtimeTraceProjMarkAccountRelations(&model, zh)
			rem, sat := hmc377R6RowOf(&model, "rem"), hmc377R6RowOf(&model, "sat")
			fence := runtimeTraceProjTreeFence(model, zh)
			if w := hmc377R7RetiredAbsenceWord(fence); w != "" {
				t.Fatalf("%s zh=%t: absence word %q:\n%s", placement, zh, w, fence)
			}
			held, seeDetail := "(⛓链上席)", "(锚定席见明细)"
			represented := "锚定份由链席[E27(+1)]代表"
			if !zh {
				held, seeDetail = "(⛓ chain seat)", "(anchored seat: see the detail index)"
				represented = "represented by the chain seat [E27(+1)]"
			}
			drawn := placement == "own" || placement == "merged"
			if drawn {
				if rem.ChainAnchorTwinInvisible || rem.ChainAnchorTwinUnreferenced || !rspaFenceContains(fence, held) || rspaFenceContains(fence, seeDetail) {
					t.Fatalf("%s zh=%t: the remainder's holder is drawn as E27(+1): invisible=%t unreferenced=%t\n%s", placement, zh,
						rem.ChainAnchorTwinInvisible, rem.ChainAnchorTwinUnreferenced, fence)
				}
				if sat.RepresentedState != runtimeTraceProjShareRefRendered || strings.Join(sat.RepresentedRefs, ",") != "E27(+1)" || !rspaFenceContains(fence, represented) {
					t.Fatalf("%s zh=%t: the satellite names the drawn holder: state=%d refs=%v\n%s", placement, zh, sat.RepresentedState, sat.RepresentedRefs, fence)
				}
				if runtimeTraceProjElimEligible(*sat) || !runtimeTraceProjElimRepresentedExcluded(*sat) {
					t.Fatalf("%s zh=%t: a share whose holder is drawn never enters the ◎ population a second time", placement, zh)
				}
				if rem.AccountRelRef != "" {
					t.Fatalf("%s zh=%t: no pair sentence borrows the impact row's account (got %q)", placement, zh, rem.AccountRelRef)
				}
				continue
			}
			if rem != nil && (!rspaFenceContains(fence, seeDetail) || rspaFenceContains(fence, held)) {
				t.Fatalf("%s zh=%t: an impact record drawn nowhere else keeps the see-detail form:\n%s", placement, zh, fence)
			}
			if sat != nil && (sat.RepresentedState == runtimeTraceProjShareRefRendered || !runtimeTraceProjElimEligible(*sat)) {
				t.Fatalf("%s zh=%t: the satellite keeps its own value in the population (state=%d)", placement, zh, sat.RepresentedState)
			}
		}
	}
}
