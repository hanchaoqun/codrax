package tool

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc300DisplayDomainNode(id string, start, end, value float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		EvidenceID: id, Subject: "worker-42", Object: "s_sleep", StateKind: "s_sleep", TypeToken: "s_sleep",
		Predicate: "wakeup_causal_impact", Value: strconv.FormatFloat(value, 'g', -1, 64), Unit: "ms",
		ImpactMS: value, CumulativeImpactMS: value, StartTs: start, EndTs: end, LineStart: 100, LineEnd: 110,
		MeasurementOrigins: []types.TraceSchedulerMeasurementOrigin{{
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/original.trace",
				PayloadRef: "/results/" + id + ".json", QueryScopeID: "query-" + id,
				TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", ClockAlignment: "identity",
			},
			ObservedAt: "2026-10-27T00:00:00Z",
			MeasurementSources: types.TraceSchedulerMeasurementSourcesFromDomain(&types.TraceSchedulerMeasurementDomain{
				Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 42,
				WindowStartTs: start, WindowEndTs: end, PartitionID: "native-" + id,
			}),
		}},
	}
}

func TestHMC300DisplayAdjacentKeepsDistinctMeasurementDomains(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(strconv.FormatBool(reverse), func(t *testing.T) {
			wide := hmc300DisplayDomainNode("wide", 1, 4, 31)
			narrow := hmc300DisplayDomainNode("closed", 2, 3, 31)
			rows := []types.TraceCausalProjectionNode{wide, narrow}
			if reverse {
				rows[0], rows[1] = rows[1], rows[0]
			}
			before, _ := json.Marshal(rows)
			got := runtimeTraceProjAdjacentNodesForDisplay(rows)
			if !reflect.DeepEqual(got, rows) {
				t.Fatalf("equal-valued overlapping measurements from distinct native scopes folded: %+v", got)
			}
			after, _ := json.Marshal(rows)
			if string(before) != string(after) {
				t.Fatal("display mutated native projection inputs")
			}
		})
	}
	// Different opaque publication/partition IDs alone are not a scope conflict.
	a, b := hmc300DisplayDomainNode("a", 1, 4, 31), hmc300DisplayDomainNode("b", 1, 4, 31)
	got := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{a, b})
	if len(got) != 1 || got[0].ImpactMS != 31 || got[0].DuplicatePublications != 2 || len(got[0].MeasurementOrigins) != 2 {
		t.Fatalf("same-domain publication fold lost value or provenance: %+v", got)
	}
}

func TestHMC300DisplayAdjacentUnknownCannotBridgeConflictingDomains(t *testing.T) {
	unknown := hmc300DisplayDomainNode("legacy", 1, 4, 31)
	unknown.MeasurementOrigins = nil
	wide := hmc300DisplayDomainNode("wide", 1, 4, 31)
	narrow := hmc300DisplayDomainNode("closed", 2, 3, 31)
	for _, knownFirst := range []bool{false, true} {
		t.Run(strconv.FormatBool(knownFirst), func(t *testing.T) {
			rows := []types.TraceCausalProjectionNode{unknown, wide, narrow}
			if knownFirst {
				rows[0], rows[1] = rows[1], rows[0]
			}
			got := runtimeTraceProjAdjacentNodesForDisplay(rows)
			if len(got) != 2 || got[0].DuplicatePublications != 2 || got[0].ImpactMS != 31 ||
				got[1].EvidenceID != narrow.EvidenceID || got[1].DuplicatePublications > 1 {
				t.Fatalf("unknown publication bridged distinct known measurements: %+v", got)
			}
			if !types.TraceCausalProjectionMeasurementDomainsConflict(got[0].MeasurementOrigins, narrow.MeasurementOrigins) {
				t.Fatal("absorbed known native provenance was discarded")
			}
		})
	}
}

func TestHMC300DisplayTrunkOccurrenceDomains(t *testing.T) {
	t.Run("overlapping distinct domains are not summed", func(t *testing.T) {
		wide, narrow := hmc300DisplayDomainNode("wide", 1, 4, 31), hmc300DisplayDomainNode("closed", 2, 3, 31)
		for _, rows := range [][]types.TraceCausalProjectionNode{{wide, narrow}, {narrow, wide}} {
			got, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(rows[0], rows[1:])
			if !reflect.DeepEqual(got, rows[0]) || !reflect.DeepEqual(kept, rows[1:]) {
				t.Fatalf("display guessed a SUM/union of overlapping query populations: %+v; %+v", got, kept)
			}
		}
	})
	t.Run("every accepted member is checked", func(t *testing.T) {
		main := hmc300DisplayDomainNode("main", 1, 2, 5)
		a, b := hmc300DisplayDomainNode("a", 3, 5, 7), hmc300DisplayDomainNode("b", 4, 6, 11)
		for _, tail := range [][]types.TraceCausalProjectionNode{{a, b}, {b, a}} {
			got, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(main, tail)
			if got.MergedCount != 2 || got.ImpactMS != main.ImpactMS+tail[0].ImpactMS ||
				len(kept) != 1 || !reflect.DeepEqual(kept[0], tail[1]) {
				t.Fatalf("later members conflict even though each is disjoint from main: %+v; %+v", got, kept)
			}
		}
	})
	t.Run("disjoint and touching occurrences remain additive", func(t *testing.T) {
		for _, start := range []float64{2, 3} {
			main, extra := hmc300DisplayDomainNode("main", 1, 2, 5), hmc300DisplayDomainNode("extra", start, start+1, 7)
			got, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(main, []types.TraceCausalProjectionNode{extra})
			if got.MergedCount != 2 || got.ImpactMS != 12 || len(kept) != 0 {
				t.Fatalf("legitimate independent occurrences no longer add: %+v; %+v", got, kept)
			}
		}
	})
	t.Run("unknown provenance does not manufacture a conflict", func(t *testing.T) {
		main, extra := hmc300DisplayDomainNode("main", 1, 4, 5), hmc300DisplayDomainNode("legacy", 2, 3, 7)
		extra.MeasurementOrigins = nil
		if types.TraceCausalProjectionOccurrenceDomainsConflict([]types.TraceCausalProjectionNode{main, extra}) {
			t.Fatal("new domain guard manufactured a contradiction from unknown provenance")
		}
		got, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(main, []types.TraceCausalProjectionNode{extra})
		// The pre-existing restriction gate still keeps partially known inputs
		// apart: absence of a new conflict is not positive permission to merge.
		if !reflect.DeepEqual(got, main) || len(kept) != 1 || !reflect.DeepEqual(kept[0], extra) {
			t.Fatalf("legacy input borrowed the known restriction: %+v; %+v", got, kept)
		}
		main.MeasurementOrigins = nil
		got, kept = runtimeTraceProjTrunkFoldSameStateOccurrences(main, []types.TraceCausalProjectionNode{extra})
		if got.MergedCount != 2 || got.ImpactMS != 12 || len(kept) != 0 {
			t.Fatalf("new negative guard changed wholly legacy behavior: %+v; %+v", got, kept)
		}
	})
}

func TestHMC300DisplayTrunkCompleteStateCoverStillAuthoritative(t *testing.T) {
	total, child := hmc300DisplayDomainNode("total", 1, 2, 100), hmc300DisplayDomainNode("child", 1.1, 1.2, 10)
	for _, n := range []*types.TraceCausalProjectionNode{&total, &child} {
		n.StateAccountKey, n.StateAccountComplete = "physical:"+n.EvidenceID, true
		n.QueryWindowStartTs, n.QueryWindowEndTs = 1, 2
	}
	for _, rows := range [][]types.TraceCausalProjectionNode{{total, child}, {child, total}} {
		got, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(rows[0], rows[1:])
		if len(kept) != 0 || got.ImpactMS != 100 || !got.MergedIntervalUnion || len(got.MergedEvidenceIDs) != 1 {
			t.Fatalf("independent complete-state-cover proof was overridden by generic domain veto: %+v; %+v", got, kept)
		}
	}
}
