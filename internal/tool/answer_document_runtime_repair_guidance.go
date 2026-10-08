package tool

import "github.com/hanchaoqun/codrax/internal/types"

// This is a teaching selector, not relation authority. Even a valid census or
// empty query can explain which evidence namespace to use without authorizing
// any endpoint, arrow, chain membership, or root cause. An attachment alone,
// model notes, and soft observations do not select this capsule.
func runtimeDiagramHasQueryObservations(ledger types.ObservationLedger) bool {
	for _, record := range ledger.Records {
		ref := record.SourceRef
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact && types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) &&
			record.GroundingPolicy == types.ClaimGroundingHard && ref.Kind == types.ObservationSourceRuntimeArtifact &&
			ref.Path != "" && ref.QueryScopeID != "" && ref.PayloadRef != "" {
			return true
		}
	}
	return false
}

const runtimeDiagramRelationOwnershipTeaching = "Every visible arrow and its edge_anchors must agree on endpoints, direction, and the honest relation_kind. A sequence/flow diagram does not turn runtime events into source calls. Runtime wakeup needs one producer-issued waker→wakee event pair; runtime contain needs a direct synchronous parent→child pair. Copy the exact pair from the available runtime anchors or use a schema-published local repair candidate; names, state boundaries, counts, adjacent events, and source definitions do not supply that pair. Source call still requires its own grounded call-site evidence; other source relations retain their own typed evidence. Runtime event authority does not establish chain membership, attributed waiting, priority/capacity conclusions, or a root cause."

const runtimeDiagramMissingRelationTeaching = "No eligible runtime relation pair is published in this context; this means missing current relation credentials, not that the trace contains no such events. If trace_query is available in the current stage, collect the same source, exact target, and requested half-open window: wakeup_chain for dependencies, event_search for individual recorded wakeups, or window_stats for synchronous business nesting. A broad census or target-not-found result is not a substitute. If only answer/patch tools are available, do not reopen collection: retain independently evidenced state intervals as notes and disclose the missing relation; remove unsupported arrows, not unrelated supported content. Do not replace an event timestamp with a state boundary."

func runtimeDiagramRelationRepairTeaching(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	if !runtimeDiagramHasQueryObservations(ledger) {
		return ""
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	text := runtimeDiagramRelationOwnershipTeaching
	if len(RuntimeDiagramRelations(ledger, rm)) == 0 {
		return text + " " + runtimeDiagramMissingRelationTeaching
	}
	return text + " Missing or invalid metadata is not a prohibition on runtime diagrams: repair with the published exact pairs. Keep supported portions and disclose gaps; do not relabel an unsupported arrow as call or invent a bridge merely to keep a diagram connected."
}
