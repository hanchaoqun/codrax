package tool

import (
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Setters of the HMC377 round-11 published-state pins
// (trace_query_withdrawn_segments_r11_hmc377_test.go). The red run on
// b2325eba6 replaces this file through a build overlay with no-op setters
// (the fields do not exist there); the test file itself is unchanged.

func hmc377R11SetClaim(c *tracequery.RootCauseShareClaim, resolvedMs, returnedMs float64, successor bool) {
	c.ResolvedMs, c.ReturnedMs, c.Successor = resolvedMs, returnedMs, successor
}

func hmc377R11SetReturned(item *tracequery.RootCauseRankItem, ms float64) { item.ShareReturnedMs = ms }

func hmc377R11GatedPart() tracequery.RootCauseShareClaimPart {
	return tracequery.RootCauseShareClaimGatedShare
}

func hmc377R11GatedRole() tracequery.RootCauseShareHolderRole {
	return tracequery.RootCauseShareHolderGatedShareSeat
}

const hmc377R11ShareReturnedNoteKey = types.TraceNoteKeyShareReturned
