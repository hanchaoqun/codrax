package types

import (
	"encoding/json"
	"testing"
)

// Independent expected matrix: adding a producer requires reviewing all its
// routing capabilities. None of these declarations is evidence of completion,
// a physical relationship, a full population, or a root cause.
func hmc348ExpectedProducerContracts() map[string]runtimeMeasurementProducerContract {
	return map[string]runtimeMeasurementProducerContract{
		TraceEventSearchInventoryPredicate:      {views: runtimeMeasurementMembersView, kind: runtimeMeasurementProducerEvent},
		RuntimeSamplingMeasurementPredicate:     {views: runtimeMeasurementSummaryView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerSampling},
		RuntimeResourceMeasurementPredicate:     {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerResource},
		"packet_inventory_observation":          {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerRaw},
		"packet_flows_observation":              {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerRaw},
		TraceBusinessSpanPredicate:              {views: runtimeMeasurementSummaryView, kind: runtimeMeasurementProducerInterval},
		"trace_span_members_observation":        {views: runtimeMeasurementMembersView, kind: runtimeMeasurementProducerInterval},
		"io_inflight":                           {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, kind: runtimeMeasurementProducerInterval},
		"io_activity":                           {views: runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerInterval},
		"scheduler_concurrency":                 {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerAggregate},
		"cpu_state_frequency_observation":       {views: runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerAggregate},
		"process_measurements_observation":      {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, kind: runtimeMeasurementProducerRaw},
		"preferred_frame_rate_observation":      {views: runtimeMeasurementSummaryView | runtimeMeasurementTimelineView | runtimeMeasurementDistributionView, kind: runtimeMeasurementProducerAggregate},
		"measurements_observation":              {views: runtimeMeasurementSummaryView | runtimeMeasurementMembersView | runtimeMeasurementTimelineView, kind: runtimeMeasurementProducerRaw},
		"thread_timeline_observation":           {views: runtimeMeasurementTimelineView, kind: runtimeMeasurementProducerInterval},
		"cpu_frequency_timeline_observation":    {views: runtimeMeasurementTimelineView, kind: runtimeMeasurementProducerInterval},
		"unpositioned_measurements_observation": {views: runtimeMeasurementMembersView, kind: runtimeMeasurementProducerRaw},
	}
}

func TestHMC348ProducerContractTotalityAndRoutingBoundaries(t *testing.T) {
	expected := hmc348ExpectedProducerContracts()
	seen := map[string]bool{}
	views := map[RuntimeMeasurementView]runtimeMeasurementViewMask{
		RuntimeMeasurementSummary: runtimeMeasurementSummaryView, RuntimeMeasurementMembers: runtimeMeasurementMembersView,
		RuntimeMeasurementTimeline: runtimeMeasurementTimelineView, RuntimeMeasurementDistribution: runtimeMeasurementDistributionView,
		"invented": 0, "": 0,
	}
	for _, declaration := range runtimeMeasurementProducerContracts {
		want, exists := expected[declaration.predicate]
		if !exists || declaration.predicate == "" || seen[declaration.predicate] || declaration.kind != want.kind || declaration.views != want.views {
			t.Fatalf("unreviewed, duplicated or drifted producer: %+v", declaration)
		}
		seen[declaration.predicate] = true
		if !RuntimeMeasurementPredicateIsRegistered(declaration.predicate) || rawMeasurementPredicate(declaration.predicate) != (want.kind == runtimeMeasurementProducerRaw) ||
			nativeMeasurementGenericDisplayPredicate(declaration.predicate) != (want.kind == runtimeMeasurementProducerInterval || want.kind == runtimeMeasurementProducerAggregate) {
			t.Fatal("producer routing drifted between consumers")
		}
		for view, mask := range views {
			if runtimeMeasurementPredicateAllowsView(declaration.predicate, view) != (mask != 0 && want.views&mask != 0) {
				t.Fatalf("predicate/view drift: %q / %q", declaration.predicate, view)
			}
		}
	}
	if len(seen) != len(expected) {
		t.Fatal("expected producer absent")
	}
	for _, unknown := range []string{"", "invented", "trace_span_members_observation ", "TRACE_SPAN_MEMBERS_OBSERVATION"} {
		if RuntimeMeasurementPredicateIsRegistered(unknown) || rawMeasurementPredicate(unknown) || nativeMeasurementGenericDisplayPredicate(unknown) {
			t.Fatal("inexact producer acquired routing permission")
		}
		for view := range views {
			if runtimeMeasurementPredicateAllowsView(unknown, view) {
				t.Fatal("unknown producer acquired a view")
			}
		}
	}
}

func hmc348ProducerRecord(t *testing.T, predicate string, view RuntimeMeasurementView) ObservationRecord {
	t.Helper()
	return rewriteRawProjectionHMC268(t, intervalEndpointRecordHMC321(t), func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
		r.Predicate, p.Tables[0].View = predicate, view
	})
}

func TestHMC348EveryProducerEndpointOptInIsExplicit(t *testing.T) {
	views := []struct {
		view RuntimeMeasurementView
		mask runtimeMeasurementViewMask
	}{
		{RuntimeMeasurementSummary, runtimeMeasurementSummaryView}, {RuntimeMeasurementMembers, runtimeMeasurementMembersView},
		{RuntimeMeasurementTimeline, runtimeMeasurementTimelineView}, {RuntimeMeasurementDistribution, runtimeMeasurementDistributionView},
	}
	for predicate, expected := range hmc348ExpectedProducerContracts() {
		for _, v := range views {
			if expected.views&v.mask == 0 {
				continue
			}
			for _, inclusive := range []bool{false, true} {
				t.Run(predicate+"/"+string(v.view)+map[bool]string{false: "/excluded", true: "/included"}[inclusive], func(t *testing.T) {
					r := hmc348ProducerRecord(t, predicate, v.view)
					before, _ := json.Marshal(r)
					r = PublishRuntimeMeasurementQueryWindow(r, 0, 1, inclusive)
					want := runtimeMeasurementQueryEndUnknown
					if expected.kind == runtimeMeasurementProducerInterval {
						want = runtimeMeasurementNativeQueryEnd(inclusive)
					}
					if endpointOfHMC321(r) != want || runtimeRawMeasurementValid(r) {
						t.Fatal("interval publisher crossed a producer-family boundary")
					}
					after, _ := json.Marshal(r)
					if string(before) != string(after) {
						t.Fatal("private endpoint changed wire facts")
					}
				})
			}
		}
	}
}

func TestHMC348MemberEndpointCannotRepairOrRestoreAlteredFacts(t *testing.T) {
	original := PublishRuntimeMeasurementQueryWindow(hmc348ProducerRecord(t, "trace_span_members_observation", RuntimeMeasurementMembers), 0, 1, false)
	if endpointOfHMC321(original) != runtimeMeasurementQueryEndExcluded {
		t.Fatal("current native endpoint absent")
	}
	wire, _ := json.Marshal(original)
	var restored ObservationRecord
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if endpointOfHMC321(FinalizeRuntimeRawMeasurement(restored)) != runtimeMeasurementQueryEndUnknown {
		t.Fatal("JSON regained endpoint authority")
	}
	for name, change := range map[string]func(*ObservationRecord, *RuntimeMeasurementPublication){
		"row":     func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Rows[0][0] = "changed" },
		"unit":    func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Unit = "execution time" },
		"payload": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.PayloadRef += ".other" },
		"query":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryScopeID += ".other" },
		"window":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryWindowEndTs = 2 },
		"view": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].View = RuntimeMeasurementSummary
		},
		"note": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = append(p.Tables[0].Notes, "new boundary")
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := rewriteRawProjectionHMC268(t, original, change)
			r = PublishRuntimeMeasurementQueryWindow(FinalizeRuntimeRawMeasurement(r), 0, 1, false)
			if endpointOfHMC321(r) != runtimeMeasurementQueryEndUnknown || endpointOfHMC321(original) != runtimeMeasurementQueryEndExcluded {
				t.Fatal("mutated member facts were repaired or original seal changed")
			}
		})
	}
}
