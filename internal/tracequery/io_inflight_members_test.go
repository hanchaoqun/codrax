package tracequery

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func ioInFlightTestPair(issueLine, completeLine int) ioInFlightInterval {
	return ioInFlightInterval{
		key: ioInFlightGroupKey{"capture", "scsi", "scsi_dispatch_cmd", "8,0", "read"}, start: 1.001, end: 1.004,
		endpoints: ioInFlightEndpoints{ThreadRef{PID: 40, Comm: "issuer"}, ThreadRef{PID: 40, Comm: "completion"}, issueLine, completeLine},
	}
}

func ioInFlightWithoutMembers(stats *IOInFlightStats) []byte {
	copyStats := *stats
	copyStats.Groups = append([]IOInFlightGroup(nil), stats.Groups...)
	for i := range copyStats.Groups {
		copyStats.Groups[i].Members = nil
		copyStats.Groups[i].OmittedMembers = 0
		copyStats.Groups[i].MemberWitnessUnavailableCount = 0
	}
	out, _ := json.Marshal(copyStats)
	return out
}

func TestIOInFlightMembersProvenanceFailurePreservesStatistics(t *testing.T) {
	pair := ioInFlightTestPair(1, 2)
	q := Query{TimeStart: 1, TimeEnd: 1.01}
	storage := storagePairingResult{intervals: []ioInFlightInterval{pair}, starts: ioInFlightStarts{pair.key: 1}}
	baseline := buildIOInFlightStats(q, blockPairingResult{}, storage)
	source := TraceArtifactSource{SourcePath: "capture", CausalCompatible: true, LocalLineCount: 2}
	for _, tc := range []struct {
		name            string
		idx             *Index
		wantUnavailable int
	}{
		{"no_index", nil, 1},
		{"pathless_index", &Index{LineCount: 2}, 1},
		{"compatibility_extent_missing", &Index{Path: "capture"}, 1},
		{"compatibility_extent_proven", &Index{Path: "capture", LineCount: 2}, 0},
		{"physical_ledger", &Index{TraceArtifacts: []TraceArtifactSource{source}}, 0},
		{"source_mismatch", &Index{Path: "capture", LineCount: 2, TraceArtifacts: []TraceArtifactSource{{SourcePath: "different", CausalCompatible: true, LocalLineCount: 2}}}, 1},
		{"ambiguous_ledger", &Index{TraceArtifacts: []TraceArtifactSource{source, source}}, 1},
		{"isolated_ledger", &Index{TraceArtifacts: []TraceArtifactSource{{SourcePath: "capture", CausalCompatible: false, LocalLineCount: 2}}}, 1},
		{"one_endpoint_outside_source", &Index{TraceArtifacts: []TraceArtifactSource{{SourcePath: "capture", CausalCompatible: true, LocalLineCount: 1}}}, 1},
		{"split_endpoint_sources", &Index{TraceArtifacts: []TraceArtifactSource{{SourcePath: "capture", CausalCompatible: true, LocalLineCount: 1}, {SourcePath: "sibling", CausalCompatible: true, LocalLineCount: 1, VirtualLineBase: 1}}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildIOInFlightStats(q, blockPairingResult{}, storage, tc.idx)
			g := got.Groups[0]
			ioInFlightAssertMemberPartition(t, g)
			if g.MemberWitnessUnavailableCount != tc.wantUnavailable || g.OmittedMembers != 0 || !reflect.DeepEqual(ioInFlightWithoutMembers(got), ioInFlightWithoutMembers(baseline)) {
				t.Fatalf("provenance failure changed accepted statistics or capacity count: %+v", got)
			}
			ioInflightPublicValues(t, g, 1, 1, 1, .3, 3, 3)
		})
	}
}

func TestIOInFlightMembersDisjointCountsAndOrder(t *testing.T) {
	var pairs []ioInFlightInterval
	for i := 0; i < 25; i++ {
		pairs = append(pairs, ioInFlightTestPair(2*i+1, 2*i+2))
	}
	// Eighteen pairs have provable endpoint source coordinates; the final
	// seven retain their statistical contribution without a public witness.
	idx := &Index{Path: "capture", LineCount: 36}
	q := Query{TimeStart: 1, TimeEnd: 1.01}
	forward := buildIOInFlightStats(q, blockPairingResult{}, storagePairingResult{intervals: pairs}, idx)
	for i, j := 0, len(pairs)-1; i < j; i, j = i+1, j-1 {
		pairs[i], pairs[j] = pairs[j], pairs[i]
	}
	reverse := buildIOInFlightStats(q, blockPairingResult{}, storagePairingResult{intervals: pairs}, idx)
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatal("latency/source traversal order changed retained witnesses")
	}
	g := forward.Groups[0]
	ioInFlightAssertMemberPartition(t, g)
	if g.AcceptedPairCount != 25 || len(g.Members) != 16 || g.OmittedMembers != 2 || g.MemberWitnessUnavailableCount != 7 || g.Values.PeakRequests != 25 || math.Abs(g.Values.RequestMs-75) > 1e-6 {
		t.Fatalf("unavailable provenance was counted twice or removed from statistics: %+v", g)
	}
}

func TestIOInFlightMembersIdentityIsSourceEndpointAndFamilyBound(t *testing.T) {
	pair := ioInFlightTestPair(1, 2)
	idx := &Index{Path: "capture", LineCount: 6}
	m, ok := ioInFlightMemberForPair(idx, pair, nil)
	if !ok {
		t.Fatal("complete compatibility source unexpectedly unavailable")
	}
	seen := map[string]bool{m.ID: true}
	for _, change := range []func(*ioInFlightInterval, *Index){
		func(p *ioInFlightInterval, idx *Index) { p.endpoints.issueLine = 3; p.endpoints.completeLine = 4 },
		func(p *ioInFlightInterval, idx *Index) { p.key.family = "other_family" },
		func(p *ioInFlightInterval, idx *Index) { p.key.layer = "other_layer" },
		func(p *ioInFlightInterval, idx *Index) { p.key.source = "other_source"; idx.Path = "other_source" },
		func(p *ioInFlightInterval, idx *Index) { p.start = 1.002 },
	} {
		candidate, candidateIndex := pair, *idx
		change(&candidate, &candidateIndex)
		member, ok := ioInFlightMemberForPair(&candidateIndex, candidate, nil)
		if !ok || seen[member.ID] {
			t.Fatalf("distinct source/endpoint/family instance collided: %+v", member)
		}
		seen[member.ID] = true
	}
	// Display names are observations, never identity or pairing evidence.
	pair.endpoints.issue.Comm = "renamed"
	renamed, _ := ioInFlightMemberForPair(idx, pair, nil)
	if renamed.ID != m.ID {
		t.Fatal("thread display name changed physical request identity")
	}
}

func TestIOInFlightMembersUnknownAndDisjointContribution(t *testing.T) {
	pair := ioInFlightTestPair(1, 2)
	idx := &Index{Path: "capture", LineCount: 2}
	unknown, _ := ioInFlightMemberForPair(idx, pair, nil)
	if unknown.WindowContributionMs != nil || unknown.WindowContribution != nil {
		t.Fatal("unknown window invented zero")
	}
	// The reducer normally sees only query-intersecting accepted pairs;
	// still pin a total projection function without fabricating an interval.
	disjoint, _ := ioInFlightMemberForPair(idx, pair, &IOInFlightWindow{StartTs: 2, EndTs: 3})
	if disjoint.WindowContribution != nil || disjoint.WindowContributionMs == nil || *disjoint.WindowContributionMs != 0 {
		t.Fatalf("known empty intersection not distinguished from unknown: %+v", disjoint)
	}
}

type ioInFlightCancelAfterBoundary struct {
	context.Context
	checks int
}

func (c *ioInFlightCancelAfterBoundary) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestIOInFlightMembersCancellationDiscardsPartialWitnesses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	progressive := &ioInFlightCancelAfterBoundary{Context: ctx}
	q := Query{TimeStart: 1, TimeEnd: 1.01, runCancel: newRunCancelState(progressive)}
	// Force the second context probe during member accumulation, not merely
	// at entry, without timing-sensitive goroutines or a huge fixture.
	q.runCancel.units = runCancelSampleMask - 2
	pairs := []ioInFlightInterval{ioInFlightTestPair(1, 2), ioInFlightTestPair(3, 4), ioInFlightTestPair(5, 6)}
	got := buildIOInFlightStats(q, blockPairingResult{}, storagePairingResult{intervals: pairs}, &Index{Path: "capture", LineCount: 6})
	if got != nil || !q.runCancel.fired() || progressive.checks != 2 {
		t.Fatalf("cancelled member accumulation published partial numeric/witness census: %+v checks=%d", got, progressive.checks)
	}
}

func TestIOInFlightMemberCapabilityAndIndependentBudget(t *testing.T) {
	if IOInFlightMemberLimit != 16 || ioInFlightGroupLimit != 8 || ioInFlightSegmentLimit != 16 || sharedDefaultResultLimit != 40 {
		t.Fatal("member display budget requires independent explicit migration")
	}
	catalog, err := TraceCapabilities("window_stats", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range catalog.Metrics {
		if m.ID != "io_inflight" {
			continue
		}
		wire, _ := json.Marshal(m)
		for _, field := range []string{"members", "id", "source_path", "issue_thread", "complete_thread", "issue_line", "complete_line", "issue_local_line", "complete_local_line", "actual_start_ts", "actual_end_ts", "window_contribution", "window_contribution_ms", "omitted_members", "member_witness_unavailable_count"} {
			if !strings.Contains(string(wire), field) {
				t.Errorf("public capability lost member field %q", field)
			}
		}
		for _, bound := range []string{"not the full numeric population", "not a population filter", "unavailable witnesses do not retract", "not issuer-blocked time"} {
			if !strings.Contains(string(wire), bound) {
				t.Errorf("public capability lost measurement boundary %q", bound)
			}
		}
		return
	}
	t.Fatal("missing in-flight capability")
}
