package types

// NegativeObservationTeaching describes the model's declaration carrier, not
// an execution receipt. It is shared by tool-schema and answer-side teaching.
// Query-specific native publications retain their own scope/completeness
// validators; this helper neither creates a new proof protocol nor changes a
// source, execution, or completion gate.
const NegativeObservationTeaching = "negative_observation retains a scoped absence claim over a non-repo source (logs, traces, VCS, commands, external resources or MCP). The kind, value=0, origin, provenance and support-ref strings do not themselves prove that an operation completed or returned zero matches. Use a completed observation's own source, query/filters, window and completeness to describe a bounded empty result. Pending selection, failed/canceled operations, an unavailable tool and an unread page are not empty results; retain their operation/repair status instead. Native results remain independently usable in their own scope; a successful raw read or MCP response is not automatically a complete query."

// UnverifiedNegativeObservationPresentation is an answer-context qualifier,
// not a model schema field or durable authority. The raw model declaration
// remains in its original aggregate/ledger for audit.
const UnverifiedNegativeObservationPresentation = "fact_authority=`advisory_model_inference`; zero_result_authority=`not_established`; operation_completion=`not_established_by_this_claim`; claimed absence only, not measured zero, unreadability or decoder failure; consult the independent operation result and its exact scope"

// AnswerAggregateHasUnverifiedNegativeObservation identifies a model-authored
// absence declaration. Even a valid reference locates evidence, not proof of
// the model's selected predicate or completeness. The kind selects only this
// conservative presentation rule; it can never certify a real zero. Actual
// producer facts are separate ObservationRecords, never upgraded copies of
// this aggregate. Repository negative_search retains its separate contract.
func AnswerAggregateHasUnverifiedNegativeObservation(fact AnswerAggregateFact) bool {
	return fact.Kind == AnswerAggregateNegativeObservation
}

// ObservationHasUnverifiedNegativeClaim uses compiled claim ownership, not
// producer names, model origins, values, or text matching. Native rows remain
// intact, including empty external responses and query-specific coverage.
func ObservationHasUnverifiedNegativeClaim(record ObservationRecord) bool {
	if record.aggregateKind != "" {
		return record.aggregateKind == AnswerAggregateNegativeObservation
	}
	return record.Negative && record.ClaimAuthority == ObservationClaimAuthorityModelInference
}
