package tool

import "github.com/hanchaoqun/codrax/internal/tracequery"

// traceQueryRootCauseRankWireItemForPublicationInUniverse publishes one rank
// row and drops the gate's decision — a test helper (HMC377 round 11,
// register R10T-5: it had no production caller left once the rank publisher
// read the decision, so it moved out of trace_query.go). The production path
// is traceQueryRootCauseRankWireItemPublicationInUniverse.
func traceQueryRootCauseRankWireItemForPublicationInUniverse(item tracequery.RootCauseRankItem, universe traceQueryPriorityArtifactUniverse) tracequery.RootCauseRankItem {
	published, _ := traceQueryRootCauseRankWireItemPublicationInUniverse(item, universe)
	return published
}
