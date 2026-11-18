package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// One account published by two ranked seats is shown on the better-ranked seat,
// whichever seat the pool lists first.
func TestHMC376MeasuredFactShowsTheBestRankedSeatInEitherPoolOrder(t *testing.T) {
	const key = "state_account:v2:account"
	worse := measuredFactSeatHMC376("seat-rank-3", 3, 30, 40, key)
	better := measuredFactSeatHMC376("seat-rank-1", 1, 30, 40, key)
	for name, pool := range map[string][]types.TraceCausalProjectionNode{
		"worse_first":  {worse, better},
		"better_first": {better, worse},
	} {
		projection := types.TraceCausalProjection{OnChainCauses: pool}
		if got := measuredFactEvidenceHMC376(traceDecisionActualOccupancyCandidates(projection, 8)); got != "seat-rank-1" {
			t.Errorf("%s: measured list=%q, want the best-ranked seat", name, got)
		}
	}
}
