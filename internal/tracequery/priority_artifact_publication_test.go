package tracequery

import (
	"context"
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPriorityArtifactProvenanceMapsBundleTokenToPhysicalChild(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "scheduler.systrace")
	perftrace := filepath.Join(dir, "samples.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       helper-9 (9) [003] .... 5.001000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002
       idle-0 (0) [002] .... 5.006000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 5.007000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.007500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.008000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`)
	writeBundleProvenanceFixture(t, perftrace, `
     worker-200 (200) [002] .... 5.004000: perf_sample: cpu=2 pid=200 tid=200 period=9000 event=cpu-cycles symbol=Worker::run dso=libworker.so source=test
`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"scheduler.systrace",
  "artifacts":[
    {"type":"systrace","path":"scheduler.systrace"},
    {"type":"perftrace","path":"samples.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"samples.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.TraceArtifacts) != 2 || canonicalTraceIndexPath(idx.TraceArtifacts[0].SourcePath) != canonicalTraceIndexPath(systrace) {
		t.Fatalf("bundle child ordering no longer maps artifact:0 to the scheduler trace: %+v", idx.TraceArtifacts)
	}
	q := Query{View: "root_cause_rank", PID: 100, TimeStart: 4.998, TimeEnd: 5.012, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.001, Limit: 16, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, q)
	var edge *WakeupEdge
	for i := range chain.Edges {
		if chain.Edges[i].Waker.PID == 200 && chain.Edges[i].Wakee.PID == 100 {
			edge = &chain.Edges[i]
			break
		}
	}
	if edge == nil {
		t.Fatalf("bundle fixture lost its worker-to-app edge: %+v", chain.Edges)
	}
	if edge.WakerPriorityArtifactSource != "artifact:0" || edge.WakeePriorityArtifactSource != "artifact:0" {
		t.Fatalf("edge caliber aliases were not separated from physical child provenance: %+v", *edge)
	}

	var impact *WakeupCausalImpact
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID == 200 {
			impact = &chain.CausalImpacts[i]
			break
		}
	}
	if impact == nil || !impact.PriorityInversionCandidate {
		t.Fatalf("bundle fixture lost its hard dependency impact: %+v", chain.CausalImpacts)
	}
	if impact.PriorityArtifactSource != "artifact:0" || impact.TargetPriorityArtifactSource != "artifact:0" ||
		!reflect.DeepEqual(impact.PriorityRelationArtifactSources, []string{"artifact:0"}) {
		t.Fatalf("impact omitted or misbound physical priority provenance: %+v", *impact)
	}

	published := Run(idx, q)
	if len(published.TraceArtifacts) != 2 || canonicalTraceIndexPath(published.TraceArtifacts[0].SourcePath) != canonicalTraceIndexPath(systrace) {
		t.Fatalf("Result.TraceArtifacts cannot resolve artifact:0 back to the scheduler child: %+v", published.TraceArtifacts)
	}
	if published.RootCauseRank == nil {
		t.Fatalf("root-cause view omitted its rank payload: %+v", published)
	}
	rank := *published.RootCauseRank
	foundRank := false
	for _, item := range append(append([]RootCauseRankItem(nil), rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID != 200 || item.Type != "priority_inversion_candidate" {
			continue
		}
		foundRank = true
		if !reflect.DeepEqual(item.PriorityRelationArtifactSources, []string{"artifact:0"}) {
			t.Fatalf("rank row dropped its relation artifact source: %+v", item)
		}
	}
	if !foundRank {
		t.Fatalf("bundle fixture produced no inversion rank row: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestPriorityArtifactSourceUnionSurvivesAggregateRootAndFamilyFold(t *testing.T) {
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		{
			Window: TimeWindow{StartTs: 1, EndTs: 2}, PriorityRelationCaliber: string(priorityCaliberClosedRangeStable),
			PriorityRelationProvenLowerMs: 1, PriorityRelationArtifactSources: []string{"artifact:2", "artifact:0"},
		},
		{
			Window: TimeWindow{StartTs: 3, EndTs: 4}, PriorityRelationCaliber: string(priorityCaliberClosedRangeStable),
			PriorityRelationProvenLowerMs: 1, PriorityRelationArtifactSources: []string{"artifact:1", "artifact:2"},
		},
	}}
	aggregate := WakeupCausalAggregate{
		Thread: ThreadRef{Comm: "worker", PID: 200}, DominantState: string(StateRunnable), PriorityInversion: true,
		PriorityInversionGatedMs: 2, GatedRunnableMs: 2,
	}
	applyAggregatePriorityRelationCoverage(&chain, &aggregate, []int{0, 1})
	want := []string{"artifact:0", "artifact:1", "artifact:2"}
	if !reflect.DeepEqual(aggregate.PriorityRelationArtifactSources, want) {
		t.Fatalf("aggregate relation provenance is not a deterministic union: got=%v want=%v", aggregate.PriorityRelationArtifactSources, want)
	}
	root := rootCauseItemFromCausalAggregate(aggregate)
	if !reflect.DeepEqual(root.PriorityRelationArtifactSources, want) {
		t.Fatalf("aggregate-to-root publication dropped relation provenance: got=%v want=%v", root.PriorityRelationArtifactSources, want)
	}

	items := []RootCauseRankItem{
		{
			Type: "priority_inversion_runnable_wait", Thread: ThreadRef{Comm: "worker", PID: 200},
			StartTs: 1, EndTs: 2, ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1,
			GatedRunnableMs: 1, PriorityRelationProvenLowerMs: 1, PriorityRelationCaliber: string(priorityCaliberClosedRangeStable),
			PriorityRelationArtifactSources: []string{"artifact:2", "artifact:0"},
		},
		{
			Type: "priority_inversion_runnable_wait", Thread: ThreadRef{Comm: "worker", PID: 200},
			StartTs: 3, EndTs: 4, ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1,
			GatedRunnableMs: 1, PriorityRelationProvenLowerMs: 1, PriorityRelationCaliber: string(priorityCaliberClosedRangeStable),
			PriorityRelationArtifactSources: []string{"artifact:1", "artifact:2"},
		},
	}
	merged := mergeSameThreadTypeRankFamily(Query{}, false, items, []int{0, 1})
	if !reflect.DeepEqual(merged.PriorityRelationArtifactSources, want) {
		t.Fatalf("rank-family fold dropped member physical sources: got=%v want=%v", merged.PriorityRelationArtifactSources, want)
	}
}

func TestPriorityArtifactSourceStampedOnDirectRunnableInversion(t *testing.T) {
	idx := buildTraceIndex(t, "priority-direct-artifact.ftrace", `
       idle-0 (0) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=R+ ==> next_comm=rival next_pid=300 next_prio=20
      rival-300 (300) [001] .... 5.005000: sched_switch: prev_comm=rival prev_pid=300 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=159
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`)
	q := Query{View: "root_cause_rank", PID: 100, TimeStart: 4.998, TimeEnd: 5.012, MinDurationMs: 0.001, Limit: 16, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	rank := BuildRootCauseRank(idx, q)
	for _, item := range append(append([]RootCauseRankItem(nil), rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID == 100 && item.Type == "priority_inversion_runnable_wait" {
			if !reflect.DeepEqual(item.PriorityRelationArtifactSources, []string{"artifact:0"}) {
				t.Fatalf("direct inversion row omitted scheduler artifact provenance: %+v", item)
			}
			return
		}
	}
	t.Fatalf("fixture produced no direct runnable inversion: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
}

func TestPriorityDerivativeFoldsCannotMintHardCaliberFromAdvisoryInputs(t *testing.T) {
	t.Run("causal aggregate", func(t *testing.T) {
		chain := ChainResult{CausalImpacts: []WakeupCausalImpact{{
			Window:                        TimeWindow{StartTs: 1, EndTs: 2},
			PriorityRelationCaliber:       string(priorityCaliberAdvisoryNearest),
			PriorityRelationProvenLowerMs: 2, PriorityRelationUnknownOrNonLowerMs: 3,
			PriorityInversionCandidate: true, PriorityInversionGatedMs: 2, GatedRunnableMs: 2,
		}}}
		aggregate := WakeupCausalAggregate{}
		applyAggregatePriorityRelationCoverage(&chain, &aggregate, []int{0})
		applyAggregateGatedInversion(&chain, &aggregate, []int{0})
		if priorityEvidenceCaliberIsHard(aggregate.PriorityRelationCaliber) ||
			aggregate.PriorityRelationProvenLowerMs != 0 || aggregate.PriorityRelationUnknownOrNonLowerMs != 5 ||
			aggregate.PriorityInversionGatedMs != 0 || aggregate.GatedRunnableMs != 0 {
			t.Fatalf("aggregate arithmetic re-minted advisory priority proof: %+v", aggregate)
		}
	})

	for _, tc := range []struct {
		name   string
		second TimeWindow
	}{
		{name: "sum disjoint", second: TimeWindow{StartTs: 1.002, EndTs: 1.003}},
		{name: "interval union", second: TimeWindow{StartTs: 1.0005, EndTs: 1.0015}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			windows := []TimeWindow{{StartTs: 1, EndTs: 1.001}, tc.second}
			items := make([]RootCauseRankItem, 0, len(windows))
			for _, window := range windows {
				items = append(items, RootCauseRankItem{
					Type: "priority_inversion_runnable_wait", Thread: ThreadRef{PID: 200},
					StartTs: window.StartTs, EndTs: window.EndTs,
					ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1,
					GatedRunnableMs:               1,
					PriorityRelationCaliber:       string(priorityCaliberAdvisoryNearest),
					PriorityRelationProvenLowerMs: 1,
				})
			}
			merged := mergeSameThreadTypeRankFamily(Query{}, false, items, []int{0, 1})
			if priorityEvidenceCaliberIsHard(merged.PriorityRelationCaliber) || merged.PriorityRelationProvenLowerMs != 0 ||
				merged.PriorityRelationUnknownOrNonLowerMs != 2 {
				t.Fatalf("%s arithmetic re-minted advisory priority proof: %+v", tc.name, merged)
			}
		})
	}
}

func TestPriorityFamilyFoldRequiresAuthorityClosurePerEffectiveMember(t *testing.T) {
	member := func(start, end float64, source string) RootCauseRankItem {
		return RootCauseRankItem{
			Type: "priority_inversion_runnable_wait", Thread: ThreadRef{Comm: "worker", PID: 200},
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			StartTs: start, EndTs: end, ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1,
			GatedRunnableMs: 1, Confidence: 0.88,
			PriorityRelationCaliber:         string(priorityCaliberClosedRangeStable),
			PriorityRelationProvenLowerMs:   1,
			PriorityRelationArtifactSources: []string{source},
		}
	}

	valid := member(1, 1.001, "artifact:0")
	for _, tc := range []struct {
		name       string
		mutate     func(*RootCauseRankItem)
		unknownNaN bool
	}{
		{
			name:   "missing member source",
			mutate: func(item *RootCauseRankItem) { item.PriorityRelationArtifactSources = nil },
		},
		{
			name:   "insufficient proven coverage",
			mutate: func(item *RootCauseRankItem) { item.PriorityRelationProvenLowerMs = 0.5 },
		},
		{
			name:       "nonfinite proven coverage",
			mutate:     func(item *RootCauseRankItem) { item.PriorityRelationProvenLowerMs = math.NaN() },
			unknownNaN: true,
		},
		{
			name:   "advisory caliber",
			mutate: func(item *RootCauseRankItem) { item.PriorityRelationCaliber = string(priorityCaliberAdvisoryNearest) },
		},
		{
			name:   "noncanonical artifact token",
			mutate: func(item *RootCauseRankItem) { item.PriorityRelationArtifactSources = []string{"artifact:01"} },
		},
		{
			name: "mixed compatibility and artifact tokens",
			mutate: func(item *RootCauseRankItem) {
				item.PriorityRelationArtifactSources = []string{"artifact:1", "compat:index"}
			},
		},
		{
			name:   "logical attachment token",
			mutate: func(item *RootCauseRankItem) { item.PriorityRelationArtifactSources = []string{"runtime_artifact:1"} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := member(1.0005, 1.0015, "artifact:1")
			tc.mutate(&bad)
			merged := mergeSameThreadTypeRankFamily(Query{}, true, []RootCauseRankItem{valid, bad}, []int{0, 1})
			if merged.MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
				t.Fatalf("fixture no longer exercises overlapping interval-union fold: %+v", merged)
			}
			if priorityEvidenceCaliberIsHard(merged.PriorityRelationCaliber) ||
				merged.PriorityRelationProvenLowerMs != 0 || merged.EffectiveImpactMs != 0 ||
				merged.GatedRunnableMs != 0 || merged.GatedRunningDeficitMs != 0 ||
				merged.Score != 0 || merged.RankSortBoostedEffectiveMs != 0 || rootCauseEffectiveImpactMs(merged) != 0 {
				t.Fatalf("invalid member authority was laundered by family arithmetic/sibling provenance: %+v", merged)
			}
			if tc.unknownNaN != math.IsNaN(merged.PriorityRelationUnknownOrNonLowerMs) {
				t.Fatalf("malformed coverage account was silently rewritten: %+v", merged)
			}
			board := []RootCauseRankItem{merged}
			assignRootCauseRankOrdinalsAndTiers(board)
			if board[0].Rank != 0 || board[0].Tier != RootCauseTierContextOnly {
				t.Fatalf("failed priority family retained a crownable rank seat: %+v", board[0])
			}
		})
	}

	// Positive control: two independently closed members may fold and compete;
	// this proves the guard is per-member authority, not a blanket family ban.
	second := member(1.0005, 1.0015, "artifact:1")
	merged := mergeSameThreadTypeRankFamily(Query{}, true, []RootCauseRankItem{valid, second}, []int{0, 1})
	if merged.PriorityRelationCaliber != string(priorityCaliberClosedRangeStable) ||
		math.Abs(merged.PriorityRelationProvenLowerMs-1.5) > 1e-9 ||
		math.Abs(merged.EffectiveImpactMs-1.5) > 1e-9 ||
		!reflect.DeepEqual(merged.PriorityRelationArtifactSources, []string{"artifact:0", "artifact:1"}) {
		t.Fatalf("fully authorized priority family was over-demoted: %+v", merged)
	}
	board := []RootCauseRankItem{merged}
	assignRootCauseRankOrdinalsAndTiers(board)
	if board[0].Rank != 1 || board[0].Tier == RootCauseTierContextOnly {
		t.Fatalf("fully authorized family lost its legitimate rank seat: %+v", board[0])
	}
}

func TestPriorityFamilyFoldRelationSourceLexicalClosure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources []string
		want    bool
	}{
		{name: "compat", sources: []string{"compat:index"}, want: true},
		{name: "artifacts", sources: []string{"artifact:0", "artifact:12"}, want: true},
		{name: "empty", sources: nil},
		{name: "blank", sources: []string{""}},
		{name: "whitespace", sources: []string{" artifact:0"}},
		{name: "leading zero", sources: []string{"artifact:00"}},
		{name: "negative", sources: []string{"artifact:-1"}},
		{name: "logical attachment", sources: []string{"runtime_artifact:0"}},
		{name: "mixed universes", sources: []string{"compat:index", "artifact:0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := priorityFoldRelationSourcesLexicallyClosed(tc.sources); got != tc.want {
				t.Fatalf("lexical closure=%t want=%t for %v", got, tc.want, tc.sources)
			}
		})
	}
}
