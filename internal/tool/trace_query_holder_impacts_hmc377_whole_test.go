package tool

// HMC377 fix round 7 (R7-2) — whole-original public-entry regression on the
// three review-6 windows (opt-in: each skips without its variable and fails
// when the file is not the pinned original). On each, the rank cap cut the
// seat minted from one wakeup causal impact while the chain view draws its own
// record of that impact; the claim naming the seat now names the impact's
// chain-view record, and the face names the holder in its held form:
//   - 105 蜂鸟 container #1 TID 7912: ◇ binder:458_5-6326 runnable remainder
//     89.986 (full 91.906 = 1.903 anchored + 0.017 wakeup-anchored + 89.986)
//     hands its anchored 1.903 to impact #31, drawn as ⇅ binder:458_5-6326
//     running [E27(+1)] (runnable(全额) 1.903 + running(折算) 0.506 = 2.409);
//   - 105 元宝 container #2 TID 50874: ◇ net_thread-50925 (divergent: chain
//     seat Σ 3.784 vs anchored Σ 4.455) — impact #32 drawn as [E21(+1)];
//   - 105 网易 container #1 TID 22853: ◇ ease.cloudmusic-22897 (divergent:
//     chain seat Σ 0.734 vs anchored Σ 20.076) — impact #9 drawn as [E11(+1)].
// Values, ranks, lanes and structure do not move. A represented satellite
// injected beside each claimant naming the same impact (unresolved on the rank
// board, the engine's represented marker dropped) is named under the drawn
// row's [E#] and leaves the ◎ population — never summed beside its own drawn
// holder.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC377R7WholeOriginalHolderDrawnAsItsCausalImpactRecord(t *testing.T) {
	cases := []struct {
		window   hmc377R7Window
		subject  string
		full     string
		value    string
		part     string
		holder   string // the drawn impact row's subject
		zh, en   string
		holderMS string
	}{
		{hmc377R7Windows()[7], "binder:458_5-6326", "91.906", "89.986", "anchored", "binder:458_5-6326",
			"同源三分:全窗91.906ms=锚定1.903ms(⛓链上席)+唤醒锚定0.017ms(唤醒锚定席见明细)+本行其余89.986ms",
			"same-source three-way split: whole-window 91.906ms=1.903ms anchored (⛓ chain seat) + 0.017ms wakeup-anchored (wakeup-anchored seat: see the detail index) + this remainder 89.986ms",
			"2.719"},
		{hmc377R7Windows()[5], "net_thread-50925", "92.174", "87.054", "chain_account", "net_thread-50925",
			"链席自账Σ3.784ms 与锚定账Σ4.455ms 失合(差0.671ms),链席另列自账,两席不可相加",
			"the chain seat's own Σ 3.784ms diverges from the anchored-ledger Σ 4.455ms (delta 0.671ms) — the chain seat keeps its own separate account, the two seats are never additive",
			""},
		{hmc377R7Windows()[6], "ease.cloudmusic-22897", "298.506", "278.430", "chain_account", "ease.cloudmusic-22897",
			"失合(差19.342ms),链席另列自账,两席不可相加",
			"(delta 19.342ms) — the chain seat keeps its own separate account, the two seats are never additive",
			""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.window.name, func(t *testing.T) {
			w := c.window
			path := hmc376Original(t, w.variable, w.digest, w.size)
			d := hmc377DeliverWindow(t, path, w.tid, w.start, w.end)
			for _, face := range []string{d.zh, d.en} {
				if word := hmc377R7RetiredAbsenceWord(face); word != "" {
					t.Fatalf("the delivered face asserts a holder's absence (%q)", word)
				}
			}
			if !rspaFenceContains(d.zh, c.zh) {
				t.Errorf("zh: the holder drawn as its causal impact's record is named in its held form %q:\n%s", c.zh, d.zh)
			}
			if !rspaFenceContains(d.en, c.en) {
				t.Errorf("en: the holder drawn as its causal impact's record is named in its held form %q:\n%s", c.en, d.en)
			}
			bus := newBusForMutationTest()
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
			bus.ToolResults = d.results
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
			set := types.CompileTraceCausalProjectionSet(ledger)
			if len(set.Projections) == 0 {
				t.Fatal("no projection")
			}
			p := set.Projections[0]
			var claimant *types.TraceCausalProjectionNode
			for i := range p.AdjacentCauses {
				n := &p.AdjacentCauses[i]
				if n.Subject == c.subject && n.ChainAnchorRemainderSeat && hmc377R6Ms3(n.ChainAnchorFullMS) == c.full {
					claimant = n
				}
			}
			if claimant == nil {
				t.Fatalf("the ◇ %s remainder (full %s) is on the projection", c.subject, c.full)
			}
			if hmc377R6Ms3(runtimeTraceProjNodeDisplayImpact(*claimant)) != c.value || claimant.ChainRelevance != "adjacent" {
				t.Fatalf("the claimant's value or lane moved: %s %s", hmc377R6Ms3(runtimeTraceProjNodeDisplayImpact(*claimant)), claimant.ChainRelevance)
			}
			var impacts []string
			for _, cl := range claimant.ShareClaims {
				if cl.Part == c.part && !cl.Resolved {
					impacts = cl.HolderImpactEvidenceIDs
				}
			}
			if len(impacts) != 1 || !strings.Contains(impacts[0], types.TraceShareClaimCausalImpactRecordMarker) {
				t.Fatalf("the unresolved %s claim names its holder's causal impact record: %+v", c.part, claimant.ShareClaims)
			}
			for _, zh := range []bool{true, false} {
				model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				index := runtimeTraceProjDrawnRecordIndex(&model, runtimeTraceProjSMR1AllRows(&model))
				drawn, ok := index[impacts[0]]
				if !ok || drawn.row == nil || drawn.row.Node.Subject != c.holder || drawn.row.Node.ChainAnchorRemainderSeat {
					t.Fatalf("zh=%t: the impact record %s is drawn in the %s chain row: %+v", zh, impacts[0], c.holder, drawn)
				}
				if c.holderMS != "" && hmc377R6Ms3(runtimeTraceProjNodeDisplayImpact(drawn.row.Node)) != c.holderMS {
					t.Fatalf("zh=%t: the drawn impact row's value moved: %s", zh, hmc377R6Ms3(runtimeTraceProjNodeDisplayImpact(drawn.row.Node)))
				}
				for _, row := range runtimeTraceProjSMR1AllRows(&model) {
					if row.Node.EvidenceID == claimant.EvidenceID && (row.ChainAnchorTwinInvisible || row.ChainAnchorTwinUnreferenced) {
						t.Fatalf("zh=%t: the holder is drawn as [%s], the claimant reads invisible=%t unreferenced=%t",
							zh, drawn.tag, row.ChainAnchorTwinInvisible, row.ChainAnchorTwinUnreferenced)
					}
				}
				// The injected represented satellite naming the same impact.
				q := p
				sat := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hmc377-r7-sat",
					Subject: c.subject, Object: "scheduler_latency", TypeToken: "scheduler_latency", StateKind: "runnable", ChainRelevance: "adjacent",
					ImpactMS: 0.5, CumulativeImpactMS: 0.5, EffectiveImpactMS: 0.5, Rank: 40, Confidence: 0.78, LineStart: claimant.LineStart, LineEnd: claimant.LineStart,
					ShareClaims: []types.TraceCausalProjectionShareClaim{{Part: "anchored", Role: "chain_seat", ShareMS: 0.5, Represented: true, HolderImpactEvidenceIDs: impacts}}}
				q.AdjacentCauses = append(append([]types.TraceCausalProjectionNode(nil), q.AdjacentCauses...), sat)
				satModel := buildRuntimeTraceProjTreeModel(q, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				var got *runtimeTraceProjTreeRow
				for _, row := range runtimeTraceProjSMR1AllRows(&satModel) {
					if row.Node.EvidenceID == "hmc377-r7-sat" {
						got = row
					}
				}
				if got == nil {
					t.Fatalf("zh=%t: the injected satellite renders", zh)
				}
				if got.RepresentedState != runtimeTraceProjShareRefRendered || len(got.RepresentedRefs) != 1 || got.RepresentedRefs[0] != drawn.tag {
					t.Fatalf("zh=%t: the satellite names the drawn holder [%s]: state=%d refs=%v", zh, drawn.tag, got.RepresentedState, got.RepresentedRefs)
				}
				if runtimeTraceProjElimEligible(*got) || !runtimeTraceProjElimRepresentedExcluded(*got) {
					t.Fatalf("zh=%t: the satellite's share is drawn as [%s] — it never enters the ◎ population a second time", zh, drawn.tag)
				}
			}
		})
	}
}
