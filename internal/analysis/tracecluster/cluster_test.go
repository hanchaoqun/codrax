package tracecluster

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFingerprintDropsInstanceNoise(t *testing.T) {
	a := decision("pid=38291 tid=59321", "Binder:59321 completed at 100.125ms 0xabc")
	b := decision("pid=40122 tid=61107", "Binder:61107 completed at 210.010ms 0xdef")
	if got, want := Fingerprint(a), Fingerprint(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("instance noise changed fingerprint:\n%#v\n%#v", got, want)
	}
}

func TestExactIsOrderIndependentAndConservesMembers(t *testing.T) {
	resolvedA := finding("f-a", decision("ui", "binder:100 completed"))
	resolvedB := finding("f-b", decision("ui", "binder:200 completed"))
	unresolved := types.TraceFindingV1{SchemaVersion: types.TraceFindingSchemaVersion, FindingID: "f-u", AnalysisKey: "a-u", Unresolved: &types.TraceUnresolvedDecision{Reason: "证据不足"}}
	a := []Input{{UnitID: "u2", Finding: resolvedB}, {UnitID: "u3", Finding: unresolved}, {UnitID: "u1", Finding: resolvedA}}
	b := []Input{{UnitID: "u1", Finding: resolvedA}, {UnitID: "u2", Finding: resolvedB}, {UnitID: "u3", Finding: unresolved}}
	gotA := Exact("batch", a, nil)
	gotB := Exact("batch", b, nil)
	if !gotA.Invariants.Valid || !gotB.Invariants.Valid {
		t.Fatalf("invalid invariant reports: %#v %#v", gotA.Invariants, gotB.Invariants)
	}
	if len(gotA.Clusters) != 1 || gotA.Clusters[0].PrimaryCount != 2 || gotA.UnresolvedCount != 1 {
		t.Fatalf("unexpected cluster result: %#v", gotA)
	}
	if !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("input ordering changed deterministic result")
	}
}

func TestExactTracksContributorsSeparately(t *testing.T) {
	primary := decision("ui", "binder")
	contributor := decision("ui", "scheduler")
	contributor.CandidateID = "candidate-2"
	contributor.Token = types.TraceCausalTokenSnapshot{Token: "runnable_wait", Lane: "scheduling_demand", Additivity: "wall_clock_per_thread", SubjectKind: "per_thread", FixDirection: "scheduling_supply", RegistryHash: "registry-v1"}
	f := finding("f1", primary)
	f.Contributors = []types.TraceCauseDecision{contributor}
	got := Exact("batch", []Input{{UnitID: "u1", Finding: f}}, nil)
	if !got.Invariants.Valid || len(got.Clusters) != 2 {
		t.Fatalf("unexpected contributor result: %#v", got)
	}
	var contributorCount int
	for _, cluster := range got.Clusters {
		contributorCount += len(cluster.ContributorMembers)
	}
	if contributorCount != 1 || got.ResolvedCount != 1 {
		t.Fatalf("contributor changed primary conservation: %#v", got)
	}
}

func TestMetricCalibersStayInSeparateBuckets(t *testing.T) {
	a := decision("ui", "binder")
	b := decision("ui", "binder")
	a.Magnitude = &types.TypedMagnitude{Value: 10, Unit: "ms", Additivity: "wall_clock", Caliber: "selected_window"}
	b.Magnitude = &types.TypedMagnitude{Value: 20, Unit: "ms", Additivity: "wall_clock", Caliber: "whole_trace"}
	got := Exact("batch", []Input{{UnitID: "u1", Finding: finding("f1", a)}, {UnitID: "u2", Finding: finding("f2", b)}}, nil)
	if len(got.Clusters) != 1 || len(got.Clusters[0].MetricBuckets) != 2 {
		t.Fatalf("incompatible calibers were not separated: %#v", got.Clusters)
	}
}

func decision(subject, event string) types.TraceCauseDecision {
	return types.TraceCauseDecision{
		CandidateID: "candidate-1", Status: types.TraceCausalSupportedCandidate,
		Token:       types.TraceCausalTokenSnapshot{Token: "binder_wait", Lane: "wakeup_chain", Additivity: "wall_clock_per_thread", SubjectKind: "per_thread", FixDirection: "io_dependency", RegistryHash: "registry-v1"},
		SubjectRole: "ui_thread", UpstreamRole: "binder_server", CausalShape: "upstream_completion_wakes_target",
		Phase: "pre_wakeup_dependency", NormalizedEventKey: event, NormalizedStackKey: subject,
		EvidenceRefs: []string{"e1"},
	}
}

func finding(id string, d types.TraceCauseDecision) types.TraceFindingV1 {
	return types.TraceFindingV1{SchemaVersion: types.TraceFindingSchemaVersion, FindingID: id, AnalysisKey: "analysis-" + id, PrimaryCause: &d}
}
