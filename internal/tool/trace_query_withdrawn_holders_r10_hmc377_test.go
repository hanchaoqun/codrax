package tool

// HMC377 round 10 — batch-owner ruling R-377-10 (2026-11-21): a row the
// trace_query publication gate withdraws (publishes as context only) holds
// nothing and claims nothing, and every share keeps exactly one ◎ carrier.
//
// End to end through production (traceQueryTypedObservations, publication
// inside -> CompileTraceCausalProjectionSet -> tree model -> ◎ board ->
// fences), hand oracles from the universe definitions:
//   - U4 (review 9 R9H-2, univ/oracle9_univ.log wt-head_U4_w: ◎ Σ 1.097 of raw
//     3.000): a LEVELMERGE-1 constituent whose claim seat is withdrawn returns
//     to ◎ with its own value, and neither it nor the residual says the share
//     is counted in that seat;
//   - F5 (review 9 R9H-5, perturb/out/nec_w11098.fence_zh.txt E5): the
//     withdrawn row prints no split of its own in held form, and its twin has
//     no held partner.
// Projection-level shapes pin the renderer's half of R-377-10 ③ (a partly
// represented satellite, already published less the share its resolved
// holders count) and the claim-seat pointer reading the same typed decision.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R10U4Universe — review 9 U4: the (6326, runnable) aggregate account
// 3.000 ms = 1.903 ms claimed by the inversion seat's gated composite (the
// constituent row A, lines 50..60) + 1.097 ms residual seat B. withdraw=true:
// the inversion seat's proven lower (1.2) is below its effective impact
// (2.409), so the gate publishes it as context only.
//
// EVOLUTION RECORD (HMC377 round 11, batch-owner ruling R-377-11 ①,
// 2026-11-21): old → new. Old: the pair was hand-built without claims and the
// withdrawal was decided by the claim-seat line span
// (levelMergeWithdrawClaimSeats, removed). New: the LEVELMERGE-1 relation is a
// typed claim of the one resolver, which a tool-package hand row cannot carry
// (no engine instants — register R10H-9), so the universe is the published
// state the resolver leaves (engine pin: TestHMC377R10GatedShareSplitWithdrawnClaimSeat
// and TestHMC377R11LevelMergeIsAClaimOfTheOneResolver): withdraw=false — both
// claims resolved to the published inversion seat (seat-2); withdraw=true — the
// inversion seat published as context only, the constituent (seat-4) its
// successor that took the share back, the residual's claim resolved to it.
// The hand oracle and every pinned word face are unchanged.
func hmc377R10U4Universe(withdraw bool) tracequery.Result {
	inv := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 2.409, CumulativeImpactMs: 2.409, EffectiveImpactMs: 2.409, RunnableMs: 1.903, RunningMs: 0.506, DominantState: "runnable",
		GatedRunnableMs: 1.903, GatedRunningDeficitMs: 0.506,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 3.0,
		PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.2019, LineStart: 10, LineEnd: 20,
		ShareHolderKey: "seat-2"}
	gated := func(resolved bool, holders ...string) []tracequery.RootCauseShareClaim {
		return []tracequery.RootCauseShareClaim{{Part: hmc377R11GatedPart(), Role: hmc377R11GatedRole(),
			ShareMs: 1.903, Resolved: resolved, Exact: resolved, HolderKeys: holders}}
	}
	b := tracequery.RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R9Dep,
		ImpactMs: 1.097, CumulativeImpactMs: 1.097, EffectiveImpactMs: 1.097, RunnableMs: 1.097, DominantState: "runnable", ChainDepth: 2,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.4, LineStart: 50, LineEnd: 60,
		GatedShareClaimedMs: 1.903, GatedShareFullMs: 3.0, GatedShareClaimSeats: []string{"10..20"},
		ShareClaims: gated(true, "seat-2"),
		Summary:     "binder:458_5-6326 runnable aggregate residual 1.097ms after the interval-accounting split"}
	a := b
	a.Rank, a.Tier = 0, "adjacent"
	a.ImpactMs, a.CumulativeImpactMs, a.EffectiveImpactMs, a.RunnableMs, a.ProjectedImpactMs = 1.903, 1.903, 1.903, 1.903, 1.903
	a.ChainRelevance, a.Causality = "adjacent", "adjacent_to_wakeup_chain"
	a.GatedShareConstituentSeat = true
	a.Summary = "binder:458_5-6326 runnable share 1.903ms of the 3.000ms aggregate account (constituent share)"
	if withdraw {
		// The published copy: the gate's context row; the constituent took
		// the carved share back and the residual's claim resolves to it.
		inv.Rank, inv.Tier, inv.Type, inv.EffectiveImpactMs, inv.GatedRunnableMs, inv.GatedRunningDeficitMs = 0, tracequery.RootCauseTierContextOnly, "unknown_state", 0, 0, 0
		inv.ShareHolderKey = ""
		inv.Summary = "scheduler state retained as context: priority relation caliber \"closed_range_stable\" does not authorize a finite positive inversion root"
		b.GatedShareClaimWithdrawn, a.GatedShareClaimWithdrawn = true, true
		b.ShareClaims = gated(true, "seat-4")
		hmc377R11SetClaim(&b.ShareClaims[0], 1.903, 0, true)
		a.ShareClaims = gated(false)
		hmc377R11SetClaim(&a.ShareClaims[0], 0, 1.903, false)
		a.ShareHolderKey = "seat-4"
		hmc377R11SetReturned(&a, 1.903)
	} else {
		a.ShareClaims = gated(true, "seat-2")
	}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true}},
		WakeupChain: &tracequery.ChainResult{
			Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{
				{Thread: hmc377R9Other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 30, TotalMs: 30, RunnableMs: 30, OnChain: true, LineStart: 5, LineEnd: 6,
					Window: tracequery.TimeWindow{StartTs: 10.1, EndTs: 10.13}},
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{hmc377R9OtherSeat(), inv, b, a}},
	}
}

func TestHMC377R10GatedShareConstituentCarriesItsShareWhenItsClaimSeatIsWithdrawn(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		name := map[bool]string{false: "published claim seat", true: "withdrawn claim seat"}[withdraw]
		f := hmc377R9Run(t, hmc377R10U4Universe(withdraw))
		zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
		elimZH := hmc377R7Squash(f.elimZH)
		if w := hmc377R7RetiredAbsenceWord(f.zh + f.en + f.elimZH + f.elimEN); w != "" {
			t.Errorf("%s: absence word %q\n%s", name, w, f.dump())
		}
		// Hand oracle: raw runnable of 6326 = 3.000 ms = 1.903 + 1.097.
		var carriers []string
		for _, c := range f.depCarriers {
			carriers = append(carriers, c[strings.Index(c, "/")+1:])
		}
		if !withdraw {
			// Control: the inversion seat counts the 1.903 share in its gated
			// composite; the constituent stays off ◎ and points at it.
			want := []string{"root_cause_secondary/2.409", "root_cause_secondary/1.097"}
			if strings.Join(carriers, ",") != strings.Join(want, ",") {
				t.Errorf("%s: ◎ carriers %v, hand oracle %v\n%s", name, carriers, want, f.dump())
			}
			if !strings.Contains(zh, hmc377R7Squash("分账构成份·归因已计入反转席[")) || !strings.Contains(elimZH, hmc377R7Squash("分账构成份")) {
				t.Errorf("%s: control wording\n%s", name, f.dump())
			}
			continue
		}
		want := []string{"root_cause_secondary/1.097", "root_cause_adjacent/1.903"}
		if strings.Join(carriers, ",") != strings.Join(want, ",") {
			t.Errorf("%s: ◎ carriers %v, hand oracle %v (one carrier per share; Σ = raw 3.000)\n%s", name, carriers, want, f.dump())
		}
		for _, banned := range []string{"已计入本线程反转席", "已计入反转席", "归因已计入", "已由其gated复合计入"} {
			if strings.Contains(zh, hmc377R7Squash(banned)) {
				t.Errorf("%s: the pair still says the share is counted in the withdrawn seat (%q)\n%s", name, banned, f.dump())
			}
		}
		if strings.Contains(elimZH, hmc377R7Squash("分账构成份")) {
			t.Errorf("%s: the ◎ footnote still counts the constituent as excluded\n%s", name, f.dump())
		}
		if !strings.Contains(zh, hmc377R7Squash("分账构成份:本行 1.903ms 为全账 3.000ms 中与本线程反转席分支窗重叠的份额;该反转席仅作上下文(不参与根因排序),此份额由本行自计")) ||
			!strings.Contains(zh, hmc377R7Squash("分账残余席:全账 3.000ms = 构成份 1.903ms(与本线程反转席分支窗重叠,该席仅作上下文,由本线程构成份行自计) + 本席残余 1.097ms")) ||
			!strings.Contains(en, hmc377R7Squash("so this row counts the share with its own value")) ||
			!strings.Contains(en, hmc377R7Squash("this thread's constituent row counts it with its own value")) {
			t.Errorf("%s: the withdrawn-seat wording of the pair\n%s", name, f.dump())
		}
		var gatedNote bool
		for _, n := range hmc377R10Notes(t, hmc377R10U4Universe(true), types.TraceNoteKeyGatedShareClaimWithdrawn) {
			gatedNote = gatedNote || strings.HasSuffix(n, "=true")
		}
		if !gatedNote {
			t.Errorf("%s: the typed withdrawal decision must ride the record notes", name)
		}
	}
}

// hmc377R10F5Universe — the withdrawn inversion seat carries the chain
// seat's split stamp (1.130 = 0.820 anchored + 0.310 remainder) and claims
// its ◇ remainder twin; the twin claims the anchored share back (NetEase
// reboot5 fault-injection shape, HeapTaskDaemon-11098).
func hmc377R10F5Universe() tracequery.Result {
	inv := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 5.793, CumulativeImpactMs: 5.793, EffectiveImpactMs: 5.793, RunnableMs: 0.820, RunningMs: 4.973, DominantState: "running",
		GatedRunnableMs: 0.820, GatedRunningDeficitMs: 4.973,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 2.896,
		PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainAnchoredMs:                 0.820, ChainAnchorFullMs: 1.130,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.3, LineStart: 10, LineEnd: 20,
		ShareHolderKey: "seat-2",
		ShareClaims: []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimRemainder, Role: tracequery.RootCauseShareHolderRemainderSeat,
			ShareMs: 0.310, Resolved: true, Exact: true, HolderKeys: []string{"seat-3"}}}}
	twin := tracequery.RootCauseRankItem{Rank: 1, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 0.310, CumulativeImpactMs: 0.310, EffectiveImpactMs: 0.310, RunnableMs: 0.310, DominantState: "runnable",
		ChainAnchoredMs: 0.820, ChainAnchorFullMs: 1.130, ChainAnchorRemainderSeat: true,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: 10.5, EndTs: 10.6, LineStart: 30, LineEnd: 90,
		ShareHolderKey: "seat-3",
		ShareClaims: []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat,
			ShareMs: 0.820, Resolved: true, Exact: true, HolderKeys: []string{"seat-2"}}}}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true}},
		WakeupChain: &tracequery.ChainResult{
			Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{
				{Thread: hmc377R9Other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 30, TotalMs: 30, RunnableMs: 30, OnChain: true, LineStart: 5, LineEnd: 6,
					Window: tracequery.TimeWindow{StartTs: 10.1, EndTs: 10.13}},
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{hmc377R9OtherSeat(), inv, twin}},
	}
}

func TestHMC377R10WithdrawnRowPrintsNoSplitOfItsOwn(t *testing.T) {
	result := hmc377R10F5Universe()
	published := traceQueryPriorityResultForPublication(result)
	inv, twin := published.RootCauseRank.Items[1], published.RootCauseRank.Items[2]
	if inv.Type == "priority_inversion_candidate" {
		t.Fatalf("precondition: the gate withdraws the inversion seat (proven lower 2.896 < effective 5.793)")
	}
	if inv.ChainAnchoredMs != 0 || inv.ChainAnchorFullMs != 0 || len(inv.ShareClaims) != 0 || inv.ShareHolderKey != "" {
		t.Errorf("the withdrawn row keeps its split/claims: anchored=%.3f full=%.3f claims=%+v key=%q",
			inv.ChainAnchoredMs, inv.ChainAnchorFullMs, inv.ShareClaims, inv.ShareHolderKey)
	}
	if twin.ShareHolderKey != "" || twin.ShareClaims[0].Resolved {
		t.Errorf("the twin keeps a held partner: key=%q claim=%+v", twin.ShareHolderKey, twin.ShareClaims[0])
	}
	if r := result.RootCauseRank.Items[1]; r.ChainAnchorFullMs != 1.130 || len(r.ShareClaims) != 1 || r.ShareHolderKey != "seat-2" {
		t.Errorf("publication mutated the engine result")
	}
	f := hmc377R9Run(t, result)
	zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
	if strings.Contains(zh, hmc377R7Squash("本行锚定0.820ms+其余0.310ms")) || strings.Contains(zh, hmc377R7Squash("(◇余段席)")) ||
		strings.Contains(zh, hmc377R7Squash("同源二分对席")) || strings.Contains(en, hmc377R7Squash("(◇ remainder seat)")) {
		t.Errorf("the withdrawn row still prints its split in held form\n%s", f.dump())
	}
	if !strings.Contains(zh, hmc377R7Squash("同源二分:全窗1.130ms=锚定0.820ms(锚定席见明细)+本行其余0.310ms(无链上凭证)")) {
		t.Errorf("the twin keeps its own split in the see-detail form\n%s", f.dump())
	}
	if w := hmc377R7RetiredAbsenceWord(f.zh + f.en + f.elimZH + f.elimEN); w != "" {
		t.Errorf("absence word %q\n%s", w, f.dump())
	}
}

func hmc377R10Notes(t *testing.T, result tracequery.Result, key string) []string {
	t.Helper()
	var out []string
	for _, r := range traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC()) {
		for _, n := range r.RichNotes {
			if strings.HasPrefix(n, key+"=") {
				out = append(out, n)
			}
		}
	}
	return out
}

// hmc377R10PartialRepresentedProjection — R-377-10 ③ renderer half: the
// satellite's represented sentence resolved only in part (its walked 1.903
// lost its holder, its unwalked 1.000 is counted at the thread's unwalked
// seat). The publication already wrote the satellite's value as its own 2.903
// less that 1.000; holderDrawn=false leaves the holder record undrawn.
func hmc377R10PartialRepresentedProjection(holderDrawn bool) types.TraceCausalProjection {
	unw := elimChainNode("r10-unw", "binder:458_5-6326", "runnable_wait", "runnable", 3, 1.0, 41)
	unw.ChainAnchorUnwalkedSeat = true
	holder := "r10-unw"
	if !holderDrawn {
		holder = "r10-undrawn"
	}
	sat := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "r10-sat",
		Subject: "binder:458_5-6326", Predicate: "root_cause_tertiary", Object: "runnable_wait", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 4, Tier: "tertiary",
		ImpactMS: 1.903, CumulativeImpactMS: 1.903, EffectiveImpactMS: 1.903, RunnableMS: 1.903,
		ChainAnchorRepresentedHeldMS: 1.0,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 1.903, Represented: true},
			{Part: string(tracequery.RootCauseShareClaimUnwalked), Role: string(tracequery.RootCauseShareHolderUnwalkedSeat), ShareMS: 1.0, Represented: true,
				Resolved: true, Exact: true, HolderEvidenceIDs: []string{holder}},
		},
		LineStart: 30, LineEnd: 40, Confidence: 0.78,
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"app-100"},
		WindowStartTs:           10.0,
		WindowEndTs:             11.0,
		// The D-state chain row keeps the ⛓ glyph's icon legend lit beside the
		// ◎ channel word (the representative-fixture convention, xlane2 同款).
		OnChainCauses: []types.TraceCausalProjectionNode{elimChainNode("r10-other", "other-6400", "runnable_wait", "runnable", 1, 30, 5), unw,
			elimChainNode("r10-dio", "worker-300", "d_state_or_io_wait", "d_sleep", 6, 3.0, 600)},
		AdjacentCauses: []types.TraceCausalProjectionNode{sat},
	}
}

func TestHMC377R10PartlyRepresentedSatelliteEntersTheOverviewWithTheRestOnly(t *testing.T) {
	for _, drawn := range []bool{true, false} {
		projection := hmc377R10PartialRepresentedProjection(drawn)
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		var carriers []string
		sum := 0.0
		for _, e := range runtimeTraceProjElimBoard(model) {
			if e.row.Node.Subject == "binder:458_5-6326" {
				carriers = append(carriers, e.row.Node.EvidenceID)
				sum += e.row.Node.EffectiveImpactMS
			}
		}
		// Hand oracle: raw runnable of 6326 = 2.903 = 1.903 (the satellite's
		// own carrier) + 1.000 (the unwalked seat).
		if len(carriers) != 2 || sum < 2.9025 || sum > 2.9035 {
			t.Errorf("drawn=%t: ◎ carriers of 6326 %v Σ %.3f, hand oracle [r10-unw r10-sat] Σ 2.903", drawn, carriers, sum)
		}
		sat := xlane2FindRowByEvidenceID(&model, "r10-sat")
		if sat == nil || sat.RepresentedState != runtimeTraceProjShareRefPartial {
			t.Fatalf("drawn=%t: the satellite must read its partial state: %+v", drawn, sat)
		}
		_ = runtimeTraceProjTreeFence(model, true)
		if !model.Marks.has(runtimeTraceProjMarkChainAnchorRepresentedPartial) {
			t.Errorf("drawn=%t: the partial-representation mark must be set", drawn)
		}
		for _, zh := range []bool{true, false} {
			m := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := hmc377R7Squash(runtimeTraceProjTreeFence(m, zh))
			unwTag := xlane2FindRowByEvidenceID(&m, "r10-unw").EvidenceTag
			var want string
			switch {
			case drawn && zh:
				want = "锚定份部分由链席[" + unwTag + "]代表:本席全账 2.903ms 锚定于 typed 唤醒依赖窗内(有凭证),其中 1.000ms 已计入[" + unwTag + "],本行只计其余 1.903ms,记 ◇ 邻近"
			case drawn:
				want = "anchored share partly represented by the chain seat [" + unwTag + "]: this seat's whole 2.903ms account is anchored inside typed wakeup-dependency windows (it HAS credential); 1.000ms of it is counted at [" + unwTag + "] and this row counts only the other 1.903ms, on the ◇ adjacent channel"
			case zh:
				want = "锚定份部分由本线程另一席持有(见明细):本席全账 2.903ms 锚定于 typed 唤醒依赖窗内(有凭证),其中 1.000ms 由该席计入,本行只计其余 1.903ms,记 ◇ 邻近"
			default:
				want = "anchored share partly held by another seat of this thread (see the detail index): this seat's whole 2.903ms account is anchored inside typed wakeup-dependency windows (it HAS credential); 1.000ms of it is counted by that seat and this row counts only the other 1.903ms, on the ◇ adjacent channel"
			}
			if !strings.Contains(fence, hmc377R7Squash(want)) {
				t.Errorf("drawn=%t zh=%t: want %q in\n%s", drawn, zh, want, runtimeTraceProjTreeFence(m, zh))
			}
			if w := hmc377R7RetiredAbsenceWord(runtimeTraceProjTreeFence(m, zh)); w != "" {
				t.Errorf("drawn=%t zh=%t: absence word %q", drawn, zh, w)
			}
		}
	}
}

// hmc377R10WithdrawnSplitProjection — R-377-10 ② renderer half: a published
// split whose claim seat the gate withdrew. A priority-inversion row with the
// claim seat's line span is still on the projection (inconsistent input on
// purpose): the pointer reads the typed decision, never the lookup.
func hmc377R10WithdrawnSplitProjection() types.TraceCausalProjection {
	inv := elimChainNode("r10-inv", "binder:458_5-6326", "priority_inversion_candidate", "running", 2, 2.409, 10)
	inv.LineStart, inv.LineEnd = 10, 20
	inv.GatedRunnableMS, inv.GatedRunningDeficitMS = 1.903, 0.506
	b := elimChainNode("r10-b", "binder:458_5-6326", "runnable_wait", "runnable", 3, 1.097, 50)
	b.GatedShareClaimedMS, b.GatedShareFullMS, b.GatedShareClaimSeats, b.GatedShareClaimWithdrawn = 1.903, 3.0, []string{"10..20"}, true
	a := elimChainNode("r10-a", "binder:458_5-6326", "runnable_wait", "runnable", 0, 1.903, 50)
	a.Rank, a.ChainRelevance, a.Causality = 0, "adjacent", "adjacent_to_wakeup_chain"
	a.Predicate, a.Tier = "root_cause_adjacent", "adjacent"
	a.GatedShareClaimedMS, a.GatedShareFullMS, a.GatedShareClaimSeats, a.GatedShareConstituentSeat, a.GatedShareClaimWithdrawn = 1.903, 3.0, []string{"10..20"}, true, true
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"app-100"},
		WindowStartTs:           10.0,
		WindowEndTs:             11.0,
		OnChainCauses: []types.TraceCausalProjectionNode{elimChainNode("r10-other", "other-6400", "runnable_wait", "runnable", 1, 30, 5), inv, b,
			elimChainNode("r10-dio", "worker-300", "d_state_or_io_wait", "d_sleep", 6, 3.0, 600)},
		AdjacentCauses: []types.TraceCausalProjectionNode{a},
	}
}

func TestHMC377R10WithdrawnSplitPointerAndExclusionReadOneDecision(t *testing.T) {
	projection := hmc377R10WithdrawnSplitProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	a, b := xlane2FindRowByEvidenceID(&model, "r10-a"), xlane2FindRowByEvidenceID(&model, "r10-b")
	if len(a.GatedShareClaimRefs) != 0 || len(b.GatedShareClaimRefs) != 0 {
		t.Errorf("the claim-seat pointer names a seat the gate withdrew: a=%v b=%v", a.GatedShareClaimRefs, b.GatedShareClaimRefs)
	}
	if !runtimeTraceProjElimEligible(*a) || runtimeTraceProjElimGatedConstituentExcluded(*a) {
		t.Errorf("the constituent must enter ◎ with its own value (exclusion and footnote census read the same decision)")
	}
	if _, ok := runtimeTraceProjElimGatedConstituentFootnote(model, true); ok {
		t.Errorf("the constituent footnote still counts the constituent")
	}
	fence := hmc377R7Squash(runtimeTraceProjTreeFence(model, true))
	if !model.Marks.has(runtimeTraceProjMarkGatedShareClaimWithdrawn) {
		t.Errorf("the withdrawn-split mark must be set")
	}
	if strings.Contains(fence, hmc377R7Squash("反转席[")) || strings.Contains(fence, hmc377R7Squash("已计入")) {
		t.Errorf("the pair names the withdrawn seat or says it counts the share\n%s", runtimeTraceProjTreeFence(model, true))
	}
}

// The typed partial share rides rank item -> record note -> projection node
// -> renderer (R-377-10 ③ sync pin): an engine-published satellite whose
// represented sentence resolved only in part (here the engine cap cut the
// walked holder, so nothing is re-resolved at publication) reaches the face in
// its partial state and enters ◎ with the published rest.
func TestHMC377R10PartialShareRidesTheRecordToTheFace(t *testing.T) {
	unw := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 1.0, CumulativeImpactMs: 1.0, EffectiveImpactMs: 1.0, RunnableMs: 1.0, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.3, EndTs: 10.301, LineStart: 41, LineEnd: 45,
		ChainAnchorUnwalkedSeat: true, ShareHolderKey: "seat-2"}
	sat := tracequery.RootCauseRankItem{Rank: 3, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.301, LineStart: 30, LineEnd: 40,
		ChainAnchorRepresentedHeldMs: 1.0,
		ShareClaims: []tracequery.RootCauseShareClaim{
			{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat, ShareMs: 1.903, Represented: true},
			{Part: tracequery.RootCauseShareClaimUnwalked, Role: tracequery.RootCauseShareHolderUnwalkedSeat, ShareMs: 1.0, Represented: true,
				Resolved: true, Exact: true, HolderKeys: []string{"seat-2"}}}}
	result := tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true}},
		WakeupChain:    &tracequery.ChainResult{Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11}},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{hmc377R9OtherSeat(), unw, sat}},
	}
	if notes := hmc377R10Notes(t, result, types.TraceNoteKeyChainAnchorRepresentedHeld); len(notes) != 1 || notes[0] != types.TraceNoteKeyChainAnchorRepresentedHeld+"=1.000" {
		t.Fatalf("the typed partial share must ride the record note: %v", notes)
	}
	f := hmc377R9Run(t, result)
	if strings.Join(f.depCarriers, ",") != "E2/root_cause_secondary/1.000,E3/root_cause_adjacent/1.903" {
		t.Errorf("◎ carriers %v, hand oracle [unwalked seat 1.000, satellite rest 1.903] (Σ = raw 2.903)\n%s", f.depCarriers, f.dump())
	}
	if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("锚定份部分由链席[E2]代表:本席全账 2.903ms 锚定于 typed 唤醒依赖窗内(有凭证),其中 1.000ms 已计入[E2],本行只计其余 1.903ms,记 ◇ 邻近")) {
		t.Errorf("the partial form on the face\n%s", f.dump())
	}
}
