package tracequery

import (
	"math"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 14 (batch-owner ruling R-377-14 ⑨, 2026-11-24; review pins
// A25a / A25b): the two case-B arms of the chain re-anchor that record a
// clipped ready-time seat's inside segments as its measured inventory — the
// plain ready-time arm and the R-377-4 inversion arm — were pinned only by
// the opt-in whole-original test
// (TestHMC377R13WholeOriginalClippedSeatShowsItsHopsAccount). This drives both
// arms in the default suite through the production state passes on the F3
// universe (rank_chain_anchor_unwalked_wait_hmc377_test.go): thread 200 has
// 12 ms of ready time, [1.010, 1.015] inside the walked wait, [1.052, 1.056]
// inside the wait its own wakeup ended, [1.070, 1.073] outside both, and no
// chain-lane seat of its own.
//
// The clipped seat holds the 9 ms inside. Its measured identity must name
// exactly those two segments: the seat's own census inventory is the whole
// 12 ms account, which does not reproduce the published 9 ms, so without the
// recorded inside segments the seat shows no account at all and the tree
// cannot relate it to a hop row priced on the same instants.
func TestHMC377R14CaseBClippedSeatShowsItsInsideSegments(t *testing.T) {
	inside := [][2]float64{{1.010, 1.015}, {1.052, 1.056}}
	want := types.TraceSchedulerStateAccountIdentity(200, string(StateRunnable), 1.0, 1.1, inside, 9)
	if want == "" {
		t.Fatal("fixture: the inside segments reproduce 9 ms and mint an identity")
	}
	for _, arm := range []struct {
		name string
		seat func(RootCauseRankItem) RootCauseRankItem
	}{
		{"plain ready-time seat", func(seat RootCauseRankItem) RootCauseRankItem { return seat }},
		{"inversion account", func(seat RootCauseRankItem) RootCauseRankItem {
			seat.Type = "priority_inversion_runnable_wait"
			seat.EffectiveImpactMs, seat.GatedRunnableMs, seat.PriorityRelationProvenLowerMs = 2.0, 2.0, 2.0
			return seat
		}},
	} {
		t.Run(arm.name, func(t *testing.T) {
			chain, stats, segments := hmc377UnwalkedChain()
			chain.CausalImpacts = nil
			items := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{arm.seat(hmc377UnwalkedWindowSeat(segments))})
			rank := RootCauseRankResult{Items: items, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}}
			stampStateAccountPublicationKeys(nil, &rank)
			var clipped, remainders []RootCauseRankItem
			for _, item := range rank.Items {
				switch {
				case item.Type != "runnable_wait" || item.ChainAnchorFullMs <= 0:
				case item.ChainAnchorRemainderSeat:
					remainders = append(remainders, item)
				default:
					clipped = append(clipped, item)
				}
			}
			if len(clipped) != 1 || len(remainders) != 1 || clipped[0].ChainRelevance != "on_chain" ||
				math.Abs(clipped[0].RunnableMs-9) > 0.0005 || math.Abs(remainders[0].RunnableMs-3) > 0.0005 {
				t.Fatalf("fixture: one clipped seat with the 9 ms inside and one remainder with the 3 ms outside: %+v", rank.Items)
			}
			if clipped[0].MeasuredStateAccountKey != want {
				t.Fatalf("the clipped seat shows exactly its inside segments: got %q want %q", clipped[0].MeasuredStateAccountKey, want)
			}
			if remainders[0].MeasuredStateAccountKey == want {
				t.Fatalf("the remainder holds other instants and never shows the inside account: %q", remainders[0].MeasuredStateAccountKey)
			}
		})
	}
}
