package tracequery

import (
	"strings"
	"testing"
)

func b4RankPair() (RootCauseRankItem, RootCauseRankItem) {
	thread := ThreadRef{Comm: "app-100", PID: 100, TGID: 10}
	absorber := RootCauseRankItem{
		Type: "d_state_or_io_wait", Thread: thread,
		StartTs: 5.001000, EndTs: 5.002062,
		LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.000000, StatsWindowEndTs: 5.007000,
		Source: "window_stats", ChainRelevance: "on_chain",
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
	}
	absorbed := RootCauseRankItem{
		Type: "io_burst_episode", Thread: thread,
		StartTs: 5.001000, EndTs: 5.002062,
		LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.000000, StatsWindowEndTs: 5.007000,
		Source: "window_stats.io_burst_episodes", ChainRelevance: "on_chain",
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
	}
	return absorber, absorbed
}

func TestB4ExactCrossTypeSeatReconciliation(t *testing.T) {
	absorber, absorbed := b4RankPair()
	other := RootCauseRankItem{
		Type: "running", Thread: ThreadRef{Comm: "worker-200", PID: 200},
		ImpactMs: 0.5, CumulativeImpactMs: 0.5, EffectiveImpactMs: 0.5,
		ChainRelevance: "on_chain",
	}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{absorber, absorbed, other}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 1 {
		t.Fatalf("one exact duplicate seat must move to the lossless carrier: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	if rank.Items[0].Type != "d_state_or_io_wait" || rank.Items[0].AbsorbedRankRows != 1 || rank.Items[0].RankFamilyKey == "" {
		t.Fatalf("D-state row must own the seat and exact family key: %+v", rank.Items[0])
	}
	got := rank.AbsorbedItems[0]
	if got.Type != "io_burst_episode" || got.Tier != RootCauseTierAbsorbed || got.Rank != 0 ||
		!got.AbsorbedByRankFamily || got.AbsorbedIntoFamily != rank.Items[0].RankFamilyKey {
		t.Fatalf("IO-burst observation must retain typed absorption provenance: %+v", got)
	}
	if !strings.Contains(got.AbsorbedIntoFamily, "pid:100") ||
		!strings.Contains(got.AbsorbedIntoFamily, "interval:5.001000..5.002062") ||
		!strings.Contains(got.AbsorbedIntoFamily, "lines:757-758") {
		t.Fatalf("canonical key must expose the exact identity dimensions: %q", got.AbsorbedIntoFamily)
	}
	assignRootCauseRanksAndTiers(rank.Items)
	if rank.Items[0].Rank != 1 || rank.Items[1].Rank != 2 || rank.AbsorbedItems[0].Rank != 0 {
		t.Fatalf("absorbed row must consume no ordinal: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}

	// Reset-first idempotency: rejoin the lossless row, recompute, converge.
	key := rank.Items[0].RankFamilyKey
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 1 ||
		rank.Items[0].AbsorbedRankRows != 1 || rank.Items[0].RankFamilyKey != key ||
		rank.AbsorbedItems[0].AbsorbedIntoFamily != key {
		t.Fatalf("double reconcile must converge: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4CrossTypeSeatNearMissesFailOpen(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RootCauseRankItem, *RootCauseRankItem)
	}{
		{"numeric thread differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Thread.PID = 101 }},
		{"comm only is insufficient", func(a, b *RootCauseRankItem) { a.Thread.PID, b.Thread.PID = 0, 0 }},
		{"type pair not adjudicated", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Type = "io_wait" }},
		{"absorber producer differs", func(a, _ *RootCauseRankItem) { a.Source = "wakeup_chain" }},
		{"absorbed producer differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Source = "window_stats" }},
		{"query start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StatsWindowStartTs += 0.000001 }},
		{"query end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StatsWindowEndTs += 0.000001 }},
		{"query endpoints absent", func(a, b *RootCauseRankItem) {
			a.StatsWindowStartTs, b.StatsWindowStartTs = 0, 0
			a.StatsWindowEndTs, b.StatsWindowEndTs = 0, 0
		}},
		{"chain lane differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.ChainRelevance = "background" }},
		{"interval start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StartTs += 0.000001 }},
		{"interval end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.EndTs += 0.000001 }},
		{"line start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.LineStart++ }},
		{"line end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.LineEnd++ }},
		{"line span absent", func(a, b *RootCauseRankItem) { a.LineStart, b.LineStart = 0, 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := b4RankPair()
			tc.mutate(&a, &b)
			rank := RootCauseRankResult{Items: []RootCauseRankItem{a, b}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 ||
				rank.Items[0].RankFamilyKey != "" || rank.Items[1].AbsorbedByRankFamily {
				t.Fatalf("near miss must keep both honest seats: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

func TestB4ExactCrossTypeSeatAtZeroTraceEpoch(t *testing.T) {
	absorber, absorbed := b4RankPair()
	absorber.StatsWindowStartTs, absorber.StatsWindowEndTs = 0, 0.007
	absorbed.StatsWindowStartTs, absorbed.StatsWindowEndTs = 0, 0.007
	absorber.StartTs, absorber.EndTs = 0, 0.001062
	absorbed.StartTs, absorbed.EndTs = 0, 0.001062
	rank := RootCauseRankResult{Items: []RootCauseRankItem{absorber, absorbed}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 || rank.Items[0].Type != "d_state_or_io_wait" {
		t.Fatalf("a valid t=0 window/interval must still reconcile the exact physical seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4ReconTypeUniversePinned(t *testing.T) {
	if len(crossTypeRankSeatReconPairs) != 1 {
		t.Fatalf("cross-type rank-seat adjudication universe changed: %+v", crossTypeRankSeatReconPairs)
	}
	spec, ok := crossTypeRankSeatReconPairs["io_burst_episode"]
	if !ok || spec.absorberType != "d_state_or_io_wait" || spec.absorbedType != "io_burst_episode" ||
		spec.absorberSource != "window_stats" || spec.absorbedSource != "window_stats.io_burst_episodes" {
		t.Fatalf("B4 exact pair changed without a ruling: %+v", crossTypeRankSeatReconPairs)
	}
}

func TestB4AmbiguousAbsorberFailsOpen(t *testing.T) {
	a, b := b4RankPair()
	secondOwner := a
	secondOwner.MemberKey = "second-owner"
	rank := RootCauseRankResult{Items: []RootCauseRankItem{a, secondOwner, b}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("two exact possible owners must not trigger arbitrary absorption: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4FamilyKeySurvivesLaterG1Reconciliation(t *testing.T) {
	a, b := b4RankPair()
	rank := RootCauseRankResult{Items: []RootCauseRankItem{a, b}}
	reconcileExactCrossTypeRankSeats(&rank)
	b4Key := rank.Items[0].RankFamilyKey

	g1 := g1UnitFamily()
	rank.Items = append(rank.Items, g1)
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if rank.Items[0].RankFamilyKey != b4Key || rank.Items[0].AbsorbedRankRows != 1 {
		t.Fatalf("G1 reset must preserve B4-owned marker: %+v", rank.Items[0])
	}
	if rank.Items[1].RankFamilyKey == "" || rank.Items[1].AbsorbedChainRows != 1 ||
		!blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("G1 must still reconcile its own independent family: %+v / %+v", rank.Items[1], blocking.Items[0])
	}
}

func TestB4BuildRootCauseRankProductionShape(t *testing.T) {
	thread := ThreadRef{Comm: "app-100", PID: 100, TGID: 10}
	window := TimeWindow{StartTs: 5.000000, EndTs: 5.007000}
	stats := WindowStats{
		Window: window,
		DStateTop: []ThreadDuration{{
			Thread: thread, DurationMs: 1.062, CPU: 2,
			StartTs: 5.001000, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
		}},
		IOBurstEpisodes: []IOBurstEpisodeSummary{{
			Thread: thread, DominantSignal: "d_state_or_io_wait",
			DurationMs: 1.062, DStateMs: 1.062,
			StartTs: 5.001000, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
			Confidence: 0.74,
		}},
	}
	rank := buildRootCauseRankFrom(nil, Query{PID: 100, TimeStart: window.StartTs, TimeEnd: window.EndTs}, ChainResult{}, stats)
	activeD, activeBurst := 0, 0
	for _, item := range rank.Items {
		switch item.Type {
		case "d_state_or_io_wait":
			activeD++
			if item.AbsorbedRankRows != 1 || item.RankFamilyKey == "" {
				t.Fatalf("production D-state row missing B4 marker: %+v", item)
			}
		case "io_burst_episode":
			activeBurst++
		}
	}
	if activeD != 1 || activeBurst != 0 || len(rank.AbsorbedItems) != 1 ||
		rank.AbsorbedItems[0].Type != "io_burst_episode" {
		t.Fatalf("production rank path must seat the physical wait once: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}
