package tracequery

import (
	"fmt"
	"testing"
)

// HMC377 E3-R2: an own phase whose own time is unknown (no synchronous
// marker-tree account — the unseen Douyin doFrame window's async
// "animator" slices, 298 ms inclusive) never takes an own-time slot by its
// inclusive total: real own work (Record View#draw() own 4.525 ms) must not
// be pushed out of the own-time TOP-8.
func TestHMC377UnknownOwnTimeNeverRanksAsOwnTime(t *testing.T) {
	var tier []*BusinessSpanMention
	// Seven large inclusive families with KNOWN own time 5 ms, plus two
	// async families with unknown own time and the largest totals: by the
	// former key the two async families took two of the eight own-time slots
	// and the ninth-largest own time (draw, 4.525 ms) fell out of every
	// criterion.
	for i := 0; i < 7; i++ {
		tier = append(tier, &BusinessSpanMention{Thread: ThreadRef{PID: 100}, Name: fmt.Sprintf("phase-%d", i),
			OnChainBasis: BusinessSpanMentionBasisSelf, Count: 1, TotalMs: float64(200 - i), MaxSingleMs: float64(200 - i),
			StartLine: 10 + i, Phase: &BusinessSpanPhaseAccount{SelfMs: 5}})
	}
	for i := 0; i < 2; i++ {
		tier = append(tier, &BusinessSpanMention{Thread: ThreadRef{PID: 100}, Name: fmt.Sprintf("async-%d", i),
			OnChainBasis: BusinessSpanMentionBasisSelf, Count: 1, TotalMs: float64(300 - i), MaxSingleMs: float64(300 - i), StartLine: 1 + i})
	}
	draw := &BusinessSpanMention{Thread: ThreadRef{PID: 100}, Name: "Record View#draw()", OnChainBasis: BusinessSpanMentionBasisSelf,
		Count: 1, TotalMs: 4.904, MaxSingleMs: 4.904, StartLine: 99, Phase: &BusinessSpanPhaseAccount{SelfMs: 4.525}}
	tier = append(tier, draw)
	selected := selectBusinessSpanMentionTier(tier)
	found := false
	for _, m := range selected {
		if m == draw {
			found = true
		}
	}
	if !found {
		t.Fatalf("the phase with the largest known own time must hold an own-time slot: %v", hmc377MentionNames(selected))
	}
}

func hmc377MentionNames(in []*BusinessSpanMention) []string {
	var out []string
	for _, m := range in {
		out = append(out, m.Name)
	}
	return out
}
