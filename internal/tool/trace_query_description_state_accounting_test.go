package tool

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These literals pin the one approved HMC §183 teaching replacement. Do not
// derive either side from current production text: the reverse transform must
// detect unrelated edits and preserve the older capability-prefix hashes.
const traceQueryStateAccountingApprovedGuidance = "state_drilldown is a cumulative state measurement scope, not a continuous state interval. Its top_sleep/top_runnable/top_running/top_io_wait/top_d_state rows sum state durations; start/end bound those records and may contain gaps. Typed state_accounting distinguishes observed boundaries, open tails and unknown closure; its times are accounted contributions, not invented actual endpoints. Missing metadata means unknown closure. state_churn keeps each state's account separately. Preserve source, recommended_views, chain_required and recursive; precise occurrences need same-source occurrence evidence, never cumulative duration equated to envelope width. Requested windows and causal eligibility are unchanged."

const traceQueryStateAccountingPriorGuidance = "state_drilldown is a cumulative state measurement scope, not a continuous state interval. Its top_sleep/top_runnable/top_running/top_io_wait/top_d_state rows sum state durations; their start/end, when present, bound those records and may contain gaps. state_churn also aggregates states and may have no timestamp bounds. Preserve source, recommended_views, chain_required and recursive; precise state occurrences require a same-source thread_timeline interval, never an equality between cumulative duration and envelope width. Unknown drilldown sources do not prove a single occurrence. Requested windows and causal eligibility are unchanged."

func traceQueryDescriptionBeforeStateAccountingEvolution(t *testing.T, description string) string {
	t.Helper()
	if types.TraceStateDrilldownWindowGuidance != traceQueryStateAccountingApprovedGuidance ||
		strings.Count(description, traceQueryStateAccountingApprovedGuidance) != 1 ||
		strings.Contains(description, traceQueryStateAccountingPriorGuidance) {
		t.Fatal("state accounting teaching must be exactly the approved one-time replacement")
	}
	return strings.Replace(description, traceQueryStateAccountingApprovedGuidance, traceQueryStateAccountingPriorGuidance, 1)
}

func TestTraceQueryDescriptionStateAccountingOnlyEvolution(t *testing.T) {
	previous := traceQueryDescriptionBeforeStateAccountingEvolution(t, (&TraceQuery{}).Description())
	// Complete golden SHA immediately before e15b10751. This includes all
	// earlier SQLite/event-name and terminal capability contracts unchanged.
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(previous))); got != "a6e517507280c740603ef2654ff04db44719b5efb66065a34b663763d38188f4" {
		t.Fatalf("Description changed outside the approved state accounting guidance: %s", got)
	}
}
