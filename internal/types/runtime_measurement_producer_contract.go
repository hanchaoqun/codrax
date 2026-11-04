package types

// One closed producer declaration owns registration, views and publication
// family. These are routing capabilities, never source/coverage/causal proof.
// Dedicated event/sampling/resource seals keep their existing validators.
type runtimeMeasurementProducerKind uint8

const (
	runtimeMeasurementProducerUnknown runtimeMeasurementProducerKind = iota
	runtimeMeasurementProducerInterval
	runtimeMeasurementProducerAggregate
	runtimeMeasurementProducerRaw
	runtimeMeasurementProducerEvent
	runtimeMeasurementProducerSampling
	runtimeMeasurementProducerResource
)

type runtimeMeasurementViewMask uint8

const (
	runtimeMeasurementSummaryView runtimeMeasurementViewMask = 1 << iota
	runtimeMeasurementMembersView
	runtimeMeasurementTimelineView
	runtimeMeasurementDistributionView
)

type runtimeMeasurementProducerContract struct {
	predicate string
	views     runtimeMeasurementViewMask
	kind      runtimeMeasurementProducerKind
}

// Private, read-only declarations; lookup returns a value, never a mutable
// entry or a public JSON permission field. New producers must choose their
// family explicitly instead of silently inheriting a generic interval seal.
var runtimeMeasurementProducerContracts = [...]runtimeMeasurementProducerContract{
	{TraceEventSearchInventoryPredicate, runtimeMeasurementMembersView, runtimeMeasurementProducerEvent},
	{RuntimeSamplingMeasurementPredicate, runtimeMeasurementSummaryView | runtimeMeasurementDistributionView, runtimeMeasurementProducerSampling},
	{RuntimeResourceMeasurementPredicate, runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerResource},
	{"packet_inventory_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerRaw},
	{"packet_flows_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerRaw},
	{TraceBusinessSpanPredicate, runtimeMeasurementSummaryView, runtimeMeasurementProducerInterval},
	{"trace_span_members_observation", runtimeMeasurementMembersView, runtimeMeasurementProducerInterval},
	{"io_inflight", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval},
	{"io_activity", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerInterval},
	{"scheduler_concurrency", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate},
	{"cpu_state_frequency_observation", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate},
	{"process_measurements_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerRaw},
	{"preferred_frame_rate_observation", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate},
	{"measurements_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerRaw},
	{"thread_timeline_observation", runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval},
	{"cpu_frequency_timeline_observation", runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval},
	{"unpositioned_measurements_observation", runtimeMeasurementMembersView, runtimeMeasurementProducerRaw},
}

func runtimeMeasurementProducerContractFor(predicate string) runtimeMeasurementProducerContract {
	for _, contract := range runtimeMeasurementProducerContracts {
		if contract.predicate == predicate {
			return contract
		}
	}
	return runtimeMeasurementProducerContract{}
}

func (c runtimeMeasurementProducerContract) allows(view RuntimeMeasurementView) bool {
	if c.kind == runtimeMeasurementProducerUnknown {
		return false
	}
	switch view {
	case RuntimeMeasurementSummary:
		return c.views&runtimeMeasurementSummaryView != 0
	case RuntimeMeasurementMembers:
		return c.views&runtimeMeasurementMembersView != 0
	case RuntimeMeasurementTimeline:
		return c.views&runtimeMeasurementTimelineView != 0
	case RuntimeMeasurementDistribution:
		return c.views&runtimeMeasurementDistributionView != 0
	default:
		return false
	}
}
