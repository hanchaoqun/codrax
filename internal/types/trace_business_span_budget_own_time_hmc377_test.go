package types

import "testing"

// HMC377 E3-R2: the budgeted handoff/decision-card selection orders the
// analysed thread's own phases by their own time; a phase whose own time is
// unknown (an async slice: SelfKnown=false) ranks after every known own time,
// never by its inclusive total (unseen Douyin doFrame: animator 298.005 ms
// inclusive led the "own phases first, by own time" list).
func TestHMC377BudgetedSelectionRanksUnknownOwnTimeLast(t *testing.T) {
	in := []TraceCausalProjectionBusinessSpanMention{
		{Subject: "app-1", Name: "animator", Count: 1, TotalMS: 298.005, MaxMS: 298.005, Basis: "self"},
		{Subject: "app-1", Name: "measure", Count: 1, TotalMS: 200, MaxMS: 200, Basis: "self", SelfKnown: true, SelfMS: 179.919},
		{Subject: "app-1", Name: "traversal", Count: 1, TotalMS: 250, MaxMS: 250, Basis: "self", SelfKnown: true, SelfMS: 94.295},
	}
	got := TraceBusinessSpanMentionBudgetedSelection(in, 2)
	if len(got) != 2 || got[0].Name != "measure" || got[1].Name != "traversal" {
		t.Fatalf("known own time must lead; unknown own time ranks last: %+v", got)
	}
}
