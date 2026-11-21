package tool

// HMC377 fix round 9 (R9-1, register R8H-1) — a share claim is valid only
// against the record actually published. The engine resolves every share
// claim against its own final board (tracequery resolveRankShareClaims); the
// tool's publication gate runs later and can withdraw a holder row: a
// priority-inversion seat whose relation coverage does not authorize its
// effective impact (proven lower < effective), or whose relation provenance is
// an artifact that is not causal-compatible, is published as context only
// (Rank 0, unknown_state, no gated runnable share). Before this round the
// emitter copied the engine's Resolved / Exact / HolderKeys unchanged, so the
// face named the withdrawn context row as the holder of the share
// (「锚定1.903ms(⛓链上席)」, 「⛓席=[E3]」, 「锚定份由链席[E3]代表…」), and a
// represented satellite left ◎ while the withdrawn row is no ◎ member: the
// share had 0 carriers.
//
// Review round 8 universe (fix8/review-holders/univ, TestZZRv8U3), ported
// end to end through production: traceQueryTypedObservations (publication
// inside) -> CompileTraceCausalProjectionSet -> tree model -> ◎ board ->
// fences. The hand oracle is computed from the universe definition, never
// from the renderer's index.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	hmc377R9Target = tracequery.ThreadRef{Comm: "app", PID: 100}
	hmc377R9Other  = tracequery.ThreadRef{Comm: "other", PID: 6400}
	hmc377R9Dep    = tracequery.ThreadRef{Comm: "binder:458_5", PID: 6326}
)

const hmc377R9DepSubject = "binder:458_5-6326"

type hmc377R9Face struct {
	zh, en         string
	elimZH, elimEN string
	notes          []string
	// depCarriers: the ◎ members of the dependency thread (tag/predicate/value).
	depCarriers []string
	rows        []string
}

func (f hmc377R9Face) dump() string {
	return fmt.Sprintf("notes=%v\nrows:\n  %s\n◎ dep=%v\n%s\n%s\n%s\n%s", f.notes, strings.Join(f.rows, "\n  "), f.depCarriers, f.elimZH, f.elimEN, f.zh, f.en)
}

func hmc377R9Run(t *testing.T, result tracequery.Result) hmc377R9Face {
	t.Helper()
	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	var face hmc377R9Face
	for _, r := range records {
		for _, n := range r.RichNotes {
			if strings.HasPrefix(n, types.TraceNoteKeyShareClaims+"=") {
				face.notes = append(face.notes, n)
			}
		}
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("projections %d", len(set.Projections))
	}
	projection := set.Projections[0]
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		elim := runtimeTraceProjElimOverviewFence(projection, model, zh)
		if !zh {
			face.en, face.elimEN = fence, elim
			continue
		}
		face.zh, face.elimZH = fence, elim
		for _, e := range runtimeTraceProjElimBoard(model) {
			if e.row.Node.Subject == hmc377R9DepSubject {
				face.depCarriers = append(face.depCarriers, fmt.Sprintf("%s/%s/%.3f", e.row.EvidenceTag, e.row.Node.Predicate, runtimeTraceProjNodeDisplayImpact(e.row.Node)))
			}
		}
		for _, r := range runtimeTraceProjSMR1AllRows(&model) {
			if r.HasData {
				face.rows = append(face.rows, fmt.Sprintf("%s %s %s rank=%d val=%.3f repr=%t state=%d refs=%v elim=%t", r.EvidenceTag, r.Node.Subject, r.Node.Predicate, r.Node.Rank,
					runtimeTraceProjNodeDisplayImpact(r.Node), r.Node.ChainAnchorRepresentedByChainSeat, r.RepresentedState, r.RepresentedRefs, runtimeTraceProjElimEligible(*r)))
			}
		}
	}
	return face
}

func hmc377R9OtherSeat() tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{Rank: 1, Tier: "primary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Other,
		ImpactMs: 30, CumulativeImpactMs: 30, EffectiveImpactMs: 30, RunnableMs: 30, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.9, StartTs: 10.1, EndTs: 10.13, LineStart: 5, LineEnd: 6}
}

// hmc377R9Universe: the dependency thread's priority-inversion seat (rank 2,
// key seat-2) holds the 1.903ms runnable share of a ◇ claimant (a split
// remainder, or a represented satellite) whose claim the engine resolved to
// it. mode picks the publication outcome of that holder: "compatible" (both
// artifacts causal-compatible, proven lower 3 >= effective 2.409: published),
// "incompatible" (the relation provenance artifact is not causal-compatible:
// withdrawn), "coverage" (proven lower 2.0 < effective 2.409: withdrawn).
func hmc377R9Universe(mode, claimant string) tracequery.Result {
	provenLower := 3.0
	if mode == "coverage" {
		provenLower = 2.0
	}
	holder := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 4.622, CumulativeImpactMs: 4.622, EffectiveImpactMs: 2.409, RunnableMs: 1.903, RunningMs: 2.719, DominantState: "running",
		GatedRunnableMs: 1.903, GatedRunningDeficitMs: 0.506,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: provenLower,
		PriorityRelationArtifactSources: []string{"artifact:1"},
		ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.2046, LineStart: 10, LineEnd: 20,
		ShareHolderKey: "seat-2"}
	impact := tracequery.WakeupCausalImpact{Thread: hmc377R9Dep, ChainDepth: 2, DominantState: "running", DominantImpactMs: 2.719, TotalMs: 4.622,
		RunningMs: 2.719, RunnableMs: 1.903, OnChain: true, LineStart: 10, LineEnd: 20,
		Window:                     tracequery.TimeWindow{StartTs: 10.2, EndTs: 10.2046},
		PriorityInversionCandidate: true, PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: provenLower, PriorityRelationArtifactSources: []string{"artifact:1"},
		PriorityInversionGatedMs: 2.409, GatedRunnableMs: 1.903, GatedRunningDeficitMs: 0.506}
	var sat tracequery.RootCauseRankItem
	if claimant == "remainder" {
		sat = tracequery.RootCauseRankItem{Rank: 6, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
			ImpactMs: 89.986, CumulativeImpactMs: 89.986, EffectiveImpactMs: 89.986, RunnableMs: 89.986, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: 10.3, EndTs: 10.4, LineStart: 30, LineEnd: 90,
			ChainAnchoredMs: 1.903, ChainAnchorFullMs: 91.889, ChainAnchorRemainderSeat: true,
			ShareClaims: []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat,
				ShareMs: 1.903, Resolved: true, Exact: true, HolderKeys: []string{"seat-2"}}}}
	} else {
		sat = tracequery.RootCauseRankItem{Rank: 3, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
			ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.2019, LineStart: 30, LineEnd: 40,
			ChainAnchorRepresentedByChainSeat: true,
			ShareClaims: []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat,
				ShareMs: 1.903, Represented: true, Resolved: true, Exact: true, HolderKeys: []string{"seat-2"}}}}
	}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true},
			{SourcePath: "other.htrace", Kind: "hitrace", CausalCompatible: mode != "incompatible"}},
		WakeupChain: &tracequery.ChainResult{
			Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{
				{Thread: hmc377R9Other, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 30, TotalMs: 30, RunnableMs: 30, OnChain: true, LineStart: 5, LineEnd: 6,
					Window: tracequery.TimeWindow{StartTs: 10.1, EndTs: 10.13}},
				impact,
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{hmc377R9OtherSeat(), holder, sat}},
	}
}

func TestHMC377R9WithdrawnHolderIsNeverNamedAndTheShareKeepsOneCarrier(t *testing.T) {
	for _, mode := range []string{"compatible", "incompatible", "coverage"} {
		// Hand oracle from the universe definition: the gate publishes the
		// holder only in the compatible universe.
		withdrawn := mode != "compatible"
		for _, claimant := range []string{"remainder", "represented"} {
			name := mode + "/" + claimant
			result := hmc377R9Universe(mode, claimant)
			published := traceQueryPriorityResultForPublication(result)
			ph, sat := published.RootCauseRank.Items[1], published.RootCauseRank.Items[2]
			if gateWithdrew := ph.Type != "priority_inversion_candidate" || ph.GatedRunnableMs <= 0; gateWithdrew != withdrawn {
				t.Fatalf("%s: the universe's gate outcome changed (published holder type=%s gated=%.3f, hand oracle withdrawn=%t)", name, ph.Type, ph.GatedRunnableMs, withdrawn)
			}
			// The published record itself: the claim is resolved only while
			// its holder is published; a withdrawn holder is named by no key.
			claim := sat.ShareClaims[0]
			if claim.Resolved == withdrawn || claim.Exact == withdrawn || (len(claim.HolderKeys) == 0) != withdrawn {
				t.Errorf("%s: published claim %+v (withdrawn holder=%t)", name, claim, withdrawn)
			}
			if withdrawn && ph.ShareHolderKey != "" {
				t.Errorf("%s: the withdrawn holder still carries the holder key %q", name, ph.ShareHolderKey)
			}
			if claimant == "represented" && sat.ChainAnchorRepresentedByChainSeat == withdrawn {
				t.Errorf("%s: published represented marker %t (withdrawn holder=%t)", name, sat.ChainAnchorRepresentedByChainSeat, withdrawn)
			}
			// The engine result is never mutated by publication.
			if c := result.RootCauseRank.Items[2].ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 1 || result.RootCauseRank.Items[1].ShareHolderKey != "seat-2" {
				t.Errorf("%s: publication mutated the engine result: %+v key=%q", name, c, result.RootCauseRank.Items[1].ShareHolderKey)
			}

			f := hmc377R9Run(t, result)
			if w := hmc377R7RetiredAbsenceWord(f.zh + f.en + f.elimZH + f.elimEN); w != "" {
				t.Errorf("%s: absence word %q\n%s", name, w, f.dump())
			}
			represented := "0"
			if claimant == "represented" {
				represented = "1"
			}
			wantNote := types.TraceNoteKeyShareClaims + "=anchored/chain_seat/1/1/" + represented + "/1.903/2"
			if withdrawn {
				wantNote = types.TraceNoteKeyShareClaims + "=anchored/chain_seat/0/0/" + represented + "/1.903/"
			}
			if len(f.notes) != 1 || f.notes[0] != wantNote {
				t.Errorf("%s: claims note %v, want [%s]", name, f.notes, wantNote)
			}
			zh, en := hmc377R7Squash(f.zh), hmc377R7Squash(f.en)
			has := func(face, s string) bool { return strings.Contains(face, hmc377R7Squash(s)) }
			namesHolder := has(zh, "锚定1.903ms(⛓链上席)") || has(zh, "⛓席=[E3]") || has(zh, "◇席=[E") || has(zh, "锚定份由链席[") ||
				has(en, "1.903ms anchored (⛓ chain seat)") || has(en, "⛓ seat = [E3]") || has(en, "◇ seat = [E") || has(en, "represented by the chain seat [")
			if namesHolder == withdrawn {
				t.Errorf("%s: the face names the holder=%t, hand oracle %t (withdrawn holder=%t)\n%s", name, namesHolder, !withdrawn, withdrawn, f.dump())
			}
			switch {
			case claimant == "remainder" && withdrawn:
				if !has(zh, "同源二分:全窗91.889ms=锚定1.903ms(锚定席见明细)+本行其余89.986ms(无链上凭证)") ||
					!has(en, "full-window 91.889ms=1.903ms anchored (anchored seat: see the detail index) + this remainder 89.986ms") {
					t.Errorf("%s: the split takes the see-detail form\n%s", name, f.dump())
				}
			case claimant == "remainder":
				if !has(zh, "同源二分对席:⛓席=[E3],◇席=本行") || !has(zh, "同源二分对席:◇席=[E4],⛓席=本行") {
					t.Errorf("%s: control: the published holder pairs with the remainder\n%s", name, f.dump())
				}
			case withdrawn:
				if !has(zh, "锚定份由本线程另一席持有(见明细)") || !has(en, "anchored share held by another seat of this thread (see the detail index)") {
					t.Errorf("%s: the represented sentence takes the see-detail form\n%s", name, f.dump())
				}
			default:
				if !has(zh, "锚定份由链席[E3]代表") {
					t.Errorf("%s: control: the represented sentence names the published holder\n%s", name, f.dump())
				}
			}
			if claimant != "represented" {
				continue
			}
			// The dependency thread's 1.903ms share has exactly one ◎
			// carrier: the published holder E3, or — when it is withdrawn
			// (context only, never a ◎ member) — the satellite with its own
			// value.
			want := "E3/root_cause_secondary/4.622"
			if withdrawn {
				want = "E4/root_cause_adjacent/1.903"
			}
			if len(f.depCarriers) != 1 || f.depCarriers[0] != want {
				t.Errorf("%s: ◎ carriers of the dependency thread %v, hand oracle [%s]\n%s", name, f.depCarriers, want, f.dump())
			}
		}
	}
}
