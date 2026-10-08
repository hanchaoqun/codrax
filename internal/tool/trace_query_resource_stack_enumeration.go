package tool

import (
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the original query payload, before the independently bounded model
// handoff. A shortened handoff does not mean the original result lost rows.
// This adapter only adds precise negative enumeration boundaries; it neither
// changes matched counts nor grants full-capture or causal authority.
func traceQueryResultEnumerationCompactions(result tracequery.Result) []tracequery.ViewCompaction {
	compactions := result.Compactions
	p := result.ResourceStack
	if tracequery.CanonicalViewName(result.View) != tracequery.ViewResourceStack || p == nil || p.Status != "available" || !tracequery.ValidResourceStack(*p) {
		return compactions
	}
	compactions = append([]tracequery.ViewCompaction(nil), compactions...)
	if p.OmittedEvents > 0 {
		compactions = append(compactions, tracequery.ViewCompaction{View: tracequery.ViewResourceStack, Dimension: "events", Emitted: len(p.Events), Total: p.MatchedEvents})
	}
	for _, e := range p.Events {
		if e.OmittedFrames > 0 {
			compactions = append(compactions, tracequery.ViewCompaction{View: tracequery.ViewResourceStack, Dimension: "event_row_" + strconv.FormatInt(e.RowID, 10) + "_frames", Emitted: len(e.Frames), Total: e.Source.FrameCount})
		}
	}
	return compactions
}

// This stream historically had no enumeration grant. Publish only new
// incomplete-result boundaries, not a "complete" certificate for unavailable
// queries, capture coverage, or source unwinding.
func traceQueryResourceStackIncompleteEnumeration(result tracequery.Result) *types.ToolEnumerationAuthority {
	authority := traceQueryEnumerationAuthority(result)
	if authority.Status != "incomplete" {
		return nil
	}
	return authority
}
