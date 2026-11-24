package tool

// HMC377 round 13 — batch-owner ruling R-377-13 ⑤⑥ (2026-11-22), the tool's
// half, end to end through production (the trace_query publication path:
// traceQueryPriorityResultForPublication / traceQueryPriorityRankForPublication;
// the typed observations -> projection -> tree model -> fences):
//   - R12N-1 / R12H-1: the publisher's re-run owns the publication's rows (no
//     caller assignment can drop a detail it returns to the board — review 12
//     U25 reverted seven universes), and a gate-withdrawn absorber ends with
//     no family key and no absorbed rows;
//   - R12H-5: the reader's partial [E#] form is the engine's held-in-part +
//     cap-cut form only; a share held in part and passed back in part is a
//     mixed disposition and takes the see-detail form (UR1_w, E1satMix_w,
//     QSHR_w, QSHRC_w of fix12/review-holders/evidence/F4_*.txt);
//   - R12H-6: a CPU-restriction row whose whole carved share passed back to it
//     states so with the existing returned-share words (fault injection
//     w24w2_w482, F7_*.txt);
//   - U24 (R-377-13 ⑦ pin): the share counted here rides on the note wire
//     beside a share counted elsewhere, 0.000 included.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R13WithdrawnAbsorberResult: 6326's chain inversion seat (its priority
// proof fails at the gate: proven lower 0.5 < effective 1.903) owns a claimless
// scheduler-latency exact twin as its B4 lossless detail.
func hmc377R13WithdrawnAbsorberResult() tracequery.Result {
	const key = "rank_pair:priority_inversion_candidate>scheduler_latency|pid:6326|on_chain|window:10.000000..11.000000|interval:10.200000..10.201903|lines:10-20"
	inv := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
		GatedRunnableMs: 1.903, PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 0.5,
		PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.201903, LineStart: 10, LineEnd: 20,
		RankFamilyKey: key, AbsorbedRankRows: 1}
	twin := tracequery.RootCauseRankItem{Tier: tracequery.RootCauseTierAbsorbed, Type: "scheduler_latency", Source: "scheduler_latency_stats", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.6, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
		StartTs: 10.2, EndTs: 10.201903, LineStart: 10, LineEnd: 20, AbsorbedByRankFamily: true, AbsorbedIntoFamily: key,
		Summary: "binder:458_5-6326 scheduler latency"}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true}},
		WakeupChain: &tracequery.ChainResult{Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{{Thread: hmc377R9Other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 30, TotalMs: 30,
				RunnableMs: 30, OnChain: true, LineStart: 5, LineEnd: 6, Window: tracequery.TimeWindow{StartTs: 10.1, EndTs: 10.13}}}},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{hmc377R9OtherSeat(), inv}, AbsorbedItems: []tracequery.RootCauseRankItem{twin}},
	}
}

func TestHMC377R13PublicationReRunOwnsThePublishedRows(t *testing.T) {
	result := hmc377R13WithdrawnAbsorberResult()
	for _, name := range []string{"rank publication", "result publication"} {
		var rank *tracequery.RootCauseRankResult
		if name == "rank publication" {
			rank = traceQueryPriorityRankForPublication(result.RootCauseRank)
		} else {
			rank = traceQueryPriorityResultForPublication(result).RootCauseRank
		}
		inv := rank.Items[1]
		if inv.Type == "priority_inversion_candidate" || inv.Tier != tracequery.RootCauseTierContextOnly {
			t.Fatalf("%s: precondition: the gate withdraws the inversion seat (type %s tier %s)", name, inv.Type, inv.Tier)
		}
		if inv.RankFamilyKey != "" || inv.AbsorbedRankRows != 0 {
			t.Errorf("%s: the withdrawn row still owns a family: key=%q rows=%d", name, inv.RankFamilyKey, inv.AbsorbedRankRows)
		}
		if len(rank.AbsorbedItems) != 0 || len(rank.Items) != 3 || rank.Items[2].Type != "scheduler_latency" || rank.Items[2].AbsorbedByRankFamily {
			t.Errorf("%s: the twin (no published row holds the withdrawn seat's account) is published on the board: board %d detail %d", name, len(rank.Items), len(rank.AbsorbedItems))
		}
	}
	if len(result.RootCauseRank.AbsorbedItems) != 1 || result.RootCauseRank.Items[1].RankFamilyKey == "" {
		t.Errorf("the engine result changed")
	}
	// The face hosts no absorbed row under the context-only row. The
	// published twin is its own priced row beside it (HMC377 R-377-14 ⑤,
	// TestHMC377R14ReleasedTwinIsItsOwnRowBesideTheContextRow).
	f := hmc377R9Run(t, result)
	if strings.Contains(f.zh, "链上并入") || strings.Contains(f.en, "chain-lane absorbed") {
		t.Errorf("the face still hosts an absorbed row under the context-only row:\n%s", f.dump())
	}
}

func TestHMC377R13ReaderPartialFormNamesNoHolderForHeldAndReturned(t *testing.T) {
	anchored, chainSeat := tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat
	run := hmc377R12ChainRun(0.853, 21, "seat-3")
	for _, tc := range []struct {
		name     string
		returned float64
		// otherReturned: a share of another sentence of the row (no part of
		// the represented share) that passed back to it.
		otherReturned float64
		seeAll        bool
	}{
		{"held + passed back (UR1_w): see-detail", 1.05, 0, true},
		{"held + cap-cut (UR1cut_c): the engine's partial form names the holder", 0, 0, false},
		{"held + cap-cut beside another share of the row that passed back: the represented share's holder is still named", 0, 0.2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := []tracequery.RootCauseShareClaim{hmc377R11Claim(anchored, chainSeat, 1.903, false, true, 0.853, tc.returned, false, "seat-3")}
			if tc.otherReturned > 0 {
				claims = append(claims, hmc377R11Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, tc.otherReturned, false, false, 0, tc.otherReturned, false))
			}
			sat := hmc377R12Satellite(1.903, 1.05, 0.853, false, claims...)
			if tc.returned+tc.otherReturned > 0 {
				hmc377R11SetReturned(&sat, tc.returned+tc.otherReturned)
			}
			f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.05, 10, 20), run, sat))
			zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
			named := strings.Contains(zh, hmc377R7Squash("锚定份部分由链席[")) || strings.Contains(en, hmc377R7Squash("anchored share partly represented by the chain seat ["))
			seeDetail := strings.Contains(zh, hmc377R7Squash("锚定份部分由本线程另一席持有(见明细)")) &&
				strings.Contains(en, hmc377R7Squash("anchored share partly held by another seat of this thread (see the detail index)"))
			if named == tc.seeAll || seeDetail != tc.seeAll {
				t.Errorf("named=%t see-detail=%t\n%s", named, seeDetail, f.dump())
			}
		})
	}
}

func TestHMC377R13RestrictionRowStatesItsReturnedShare(t *testing.T) {
	anchored, chainSeat := tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat
	cpu := func(returned float64, resolvedMs float64) tracequery.RootCauseRankItem {
		it := tracequery.RootCauseRankItem{Rank: 5, Tier: "tertiary", Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: hmc377R9Dep,
			ImpactMs: 15.422, CumulativeImpactMs: 15.422, EffectiveImpactMs: 15.422, ProjectedImpactMs: 15.422, RunnableMs: 15.422, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.3, EndTs: 10.4, LineStart: 90, LineEnd: 92,
			ShareClaims: []tracequery.RootCauseShareClaim{hmc377R11Claim(anchored, chainSeat, 0.606, false, false, resolvedMs, returned, false)},
			Summary:     "binder:458_5-6326 runnable under a CPU affinity/cpuset restriction"}
		hmc377R11SetReturned(&it, returned)
		return it
	}
	t.Run("the whole carved share passed back", func(t *testing.T) {
		result := hmc377R11Result(hmc377R11Context(0.606, 10, 20), cpu(0.606, 0))
		f := hmc377R9Run(t, result)
		if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("锚定份中 0.606ms 的持有行仅作上下文,本行以自身数值计入,本行共计 15.422ms")) ||
			!strings.Contains(hmc377R7Squash(f.en), hmc377R7Squash("0.606ms of the anchored share is held by a row that is context only, so this row counts it in its own account: 15.422ms in all")) {
			t.Errorf("returned-share words missing\n%s", f.dump())
		}
		if strings.Contains(f.zh, "全窗15.422") {
			t.Errorf("the restriction row states a whole-window account\n%s", f.zh)
		}
		// The words are explained: the row lights their legend entry (it is
		// the only row of this board that speaks them).
		if zh, en := hmc377R13Legend(t, result, true), hmc377R13Legend(t, result, false); !strings.Contains(zh, "`持有行仅作上下文,本行以自身数值计入` =") ||
			!strings.Contains(en, "`the row holding it is context only, so this row counts it in its own account` =") {
			t.Errorf("the returned-share words have no legend entry:\n%s\n%s", zh, en)
		}
	})
	t.Run("a share inside the thread's unwalked seat that passed back whole is stated the same way", func(t *testing.T) {
		// The restriction seat's successor claim covers its own restricted
		// instants in either on-chain seat of its thread (anchored or
		// unwalked part).
		it := cpu(0, 0)
		it.ShareClaims = []tracequery.RootCauseShareClaim{hmc377R11Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 0.606, false, false, 0, 0.606, false)}
		hmc377R11SetReturned(&it, 0.606)
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(0.606, 10, 20), it))
		if !strings.Contains(hmc377R7Squash(f.zh), hmc377R7Squash("0.606ms 的持有行仅作上下文,本行以自身数值计入,本行共计 15.422ms")) {
			t.Errorf("returned-share words missing\n%s", f.dump())
		}
	})
	t.Run("a ◇ remainder states its returned share once, on its split line", func(t *testing.T) {
		rem := hmc377R12Remainder(1.903, 2.903, 2.903, hmc377R11Claim(anchored, chainSeat, 1.903, false, false, 0, 1.903, false))
		hmc377R11SetReturned(&rem, 1.903)
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(1.903, 10, 20), rem))
		if n := strings.Count(hmc377R7Squash(f.zh), hmc377R7Squash("的持有行仅作上下文,本行以自身数值计入")); n != 1 {
			t.Errorf("returned-share words %d times, want once\n%s", n, f.zh)
		}
	})
	t.Run("a share passed back only in part is no whole return", func(t *testing.T) {
		f := hmc377R9Run(t, hmc377R11Result(hmc377R11Context(0.3, 10, 20), cpu(0.3, 0.306)))
		if strings.Contains(f.zh, "的持有行仅作上下文,本行以自身数值计入") {
			t.Errorf("a share returned in part is worded as returned whole\n%s", f.zh)
		}
	})
}

func TestHMC377R13ShareCountedHereRidesBesideTheShareCountedElsewhere(t *testing.T) {
	// U24: a ◇ remainder whose whole rest the restriction seat counts: the
	// share counted here is 0 and rides as 0.000 beside the share counted
	// elsewhere (the three subset fields ride together).
	rem := tracequery.RootCauseRankItem{Rank: 0, Tier: tracequery.RootCauseTierAbsorbed, Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 0, CumulativeImpactMs: 0, RunnableMs: 0, DominantState: "runnable", ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		StartTs: 10.5, EndTs: 10.501, LineStart: 70, LineEnd: 80, ChainAnchoredMs: 1.903, ChainAnchorFullMs: 2.903, ChainAnchorRemainderSeat: true,
		ShareCountedElsewhereMs: 1.0, ShareCountedHereMs: 0}
	records := traceQueryTypedObservations(hmc377R11Result(rem), "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	found := false
	for _, r := range records {
		for _, n := range r.RichNotes {
			if strings.HasPrefix(n, types.TraceNoteKeyShareCountedElsewhere+"=") {
				found = true
				if !hmc377R13HasNote(r.RichNotes, types.TraceNoteKeyShareCountedHere+"=0.000") {
					t.Errorf("the share counted here does not ride beside the share counted elsewhere: %v", r.RichNotes)
				}
			}
		}
	}
	if !found {
		t.Fatalf("precondition: the share counted elsewhere rides on the note wire")
	}
}

func hmc377R13HasNote(notes []string, want string) bool {
	for _, n := range notes {
		if n == want {
			return true
		}
	}
	return false
}

// hmc377R13Legend renders result's tree and returns the legend entries its
// marks light.
func hmc377R13Legend(t *testing.T, result tracequery.Result, zh bool) string {
	t.Helper()
	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("projections %d", len(set.Projections))
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	_ = runtimeTraceProjTreeFence(model, zh)
	return strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, zh), "\n")
}

// hmc377R13WithdrawnDetailResult: 6326's chain inversion seat owns the
// R-377-4 inversion ready account as its B4 lossless detail, and the priority
// proof of both fails at the gate (proven lower 0.5 < effective 1.903); no
// other row of the thread is published.
func hmc377R13WithdrawnDetailResult() tracequery.Result {
	result := hmc377R13WithdrawnAbsorberResult()
	const key = "rank_pair:priority_inversion_candidate>priority_inversion_runnable_wait|pid:6326|on_chain|window:10.000000..11.000000|interval:10.200000..10.201903|lines:10-20"
	rank := *result.RootCauseRank
	inv := rank.Items[1]
	inv.RankFamilyKey = key
	det := tracequery.RootCauseRankItem{Tier: tracequery.RootCauseTierAbsorbed, Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 0.5, PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.6, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
		StartTs: 10.2, EndTs: 10.201903, LineStart: 30, LineEnd: 40, AbsorbedByRankFamily: true, AbsorbedIntoFamily: key,
		Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
	rank.Items = []tracequery.RootCauseRankItem{rank.Items[0], inv}
	rank.AbsorbedItems = []tracequery.RootCauseRankItem{det}
	result.RootCauseRank = &rank
	return result
}

// R-377-13 ⑤, continuation: a lossless detail the gate itself withdrew is a
// withdrawn row too. Its withdrawn absorber hosts nothing, so it is published
// on its own — as the context row the gate wrote, never as a seat carrying its
// ready account (commit a6be6a95e published it as an adjacent ◎ carrier of
// 1.903ms: ~/hmc-work/hmc377/fix13/hold/cont/exp/tool_head.out).
func TestHMC377R13GateWithdrawnDetailIsPublishedAsContext(t *testing.T) {
	result := hmc377R13WithdrawnDetailResult()
	rank := traceQueryPriorityRankForPublication(result.RootCauseRank)
	if len(rank.AbsorbedItems) != 0 || len(rank.Items) != 3 {
		t.Fatalf("published board %d detail %d, want the withdrawn detail on the board (a withdrawn row absorbs nothing)", len(rank.Items), len(rank.AbsorbedItems))
	}
	for _, it := range rank.Items[1:] {
		if it.Type == "priority_inversion_candidate" || it.Type == "priority_inversion_runnable_wait" || it.Tier != tracequery.RootCauseTierContextOnly || it.Rank != 0 {
			t.Errorf("row %d-%d is published as type %s tier %q rank %d, want the context row the gate wrote", it.LineStart, it.LineEnd, it.Type, it.Tier, it.Rank)
		}
		if it.RankFamilyKey != "" || it.AbsorbedRankRows != 0 || it.AbsorbedByRankFamily {
			t.Errorf("row %d-%d wears a family: key=%q rows=%d absorbed=%t", it.LineStart, it.LineEnd, it.RankFamilyKey, it.AbsorbedRankRows, it.AbsorbedByRankFamily)
		}
	}
	f := hmc377R9Run(t, result)
	if len(f.depCarriers) != 0 {
		t.Errorf("a row the gate published as context carries the thread's ready time: %v\n%s", f.depCarriers, f.dump())
	}
	if n := strings.Count(f.zh, "上下文·不参与根因排序"); n != 2 {
		t.Errorf("%d context rows drawn, want the withdrawn seat and its withdrawn detail\n%s", n, f.zh)
	}
	if strings.Contains(f.zh, "链上并入") || strings.Contains(f.en, "chain-lane absorbed") {
		t.Errorf("the face hosts an absorbed row under a context-only row:\n%s", f.dump())
	}
}
