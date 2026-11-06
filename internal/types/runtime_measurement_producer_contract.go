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

// Query endpoint authority is independent of the output family's shape. A
// continuous aggregate may prove a fixed half-open ruler without acquiring
// the ability to declare inclusive bounds (or to seal other aggregates).
type runtimeMeasurementProducerWindow uint8

const (
	runtimeMeasurementProducerWindowUnspecified runtimeMeasurementProducerWindow = iota
	runtimeMeasurementProducerWindowVariable
	runtimeMeasurementProducerWindowHalfOpen
)

type runtimeMeasurementProducerContract struct {
	predicate string
	views     runtimeMeasurementViewMask
	kind      runtimeMeasurementProducerKind
	window    runtimeMeasurementProducerWindow
}

// Private, read-only declarations; lookup returns a value, never a mutable
// entry or a public JSON permission field. New producers must choose their
// family explicitly instead of silently inheriting a generic interval seal.
var runtimeMeasurementProducerContracts = [...]runtimeMeasurementProducerContract{
	{TraceEventSearchInventoryPredicate, runtimeMeasurementMembersView, runtimeMeasurementProducerEvent, runtimeMeasurementProducerWindowUnspecified},
	{RuntimeSamplingMeasurementPredicate, runtimeMeasurementSummaryView | runtimeMeasurementDistributionView, runtimeMeasurementProducerSampling, runtimeMeasurementProducerWindowUnspecified},
	{RuntimeResourceMeasurementPredicate, runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerResource, runtimeMeasurementProducerWindowUnspecified},
	{"packet_inventory_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerRaw, runtimeMeasurementProducerWindowUnspecified},
	{"packet_flows_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, runtimeMeasurementProducerRaw, runtimeMeasurementProducerWindowUnspecified},
	{TraceBusinessSpanPredicate, runtimeMeasurementSummaryView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"trace_span_members_observation", runtimeMeasurementMembersView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"io_inflight", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"io_activity", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"scheduler_concurrency", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate, runtimeMeasurementProducerWindowHalfOpen},
	{"cpu_state_frequency_observation", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate, runtimeMeasurementProducerWindowUnspecified},
	{"process_measurements_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerRaw, runtimeMeasurementProducerWindowUnspecified},
	{"preferred_frame_rate_observation", runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, runtimeMeasurementProducerAggregate, runtimeMeasurementProducerWindowUnspecified},
	{"measurements_observation", runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, runtimeMeasurementProducerRaw, runtimeMeasurementProducerWindowUnspecified},
	{"thread_timeline_observation", runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"cpu_frequency_timeline_observation", runtimeMeasurementTimelineView, runtimeMeasurementProducerInterval, runtimeMeasurementProducerWindowVariable},
	{"unpositioned_measurements_observation", runtimeMeasurementMembersView, runtimeMeasurementProducerRaw, runtimeMeasurementProducerWindowUnspecified},
}

func (c runtimeMeasurementProducerContract) allowsQueryEnd(inclusive bool) bool {
	return c.window == runtimeMeasurementProducerWindowVariable ||
		(c.window == runtimeMeasurementProducerWindowHalfOpen && !inclusive)
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
