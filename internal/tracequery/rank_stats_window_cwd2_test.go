package tracequery

// rank_stats_window_cwd2_test.go — §21.1 CWD-2 ② engine-half pins (cmp_01 C7
// witness, real_trace_campaign_20260705.md, 2026-07-07): every rank row minted
// from a window_stats summary carries the typed query-window identity the
// backing stats were computed over (StatsWindowStartTs/EndTs), so the tool
// observation face can emit the row-level selected_window note even when the
// result envelope carries no window of its own. Identity carriage ONLY —
// rank 排序/权重/Value 零触碰 is pinned by the stamped-vs-unstamped equality
// leg below.

import "testing"

func cwd2StatsFixture(window TimeWindow) WindowStats {
	return WindowStats{
		Window: window,
		IOPressureSummary: &IOPressureSummary{
			Score: 12.5, LineStart: 99303, LineEnd: 124384, Summary: "io pressure summary",
		},
		SupplyPressureSummary: &SupplyPressureSummary{
			CPUPressureMs: 13821.784, LineStart: 99303, LineEnd: 124384, Summary: "supply pressure summary",
		},
	}
}

func cwd2ChainFixture() ChainResult {
	return ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		RootEvidence: []RootEvidence{{
			Type:       "binder_wait",
			Thread:     ThreadRef{Comm: "dep", PID: 200},
			DurationMs: 4.577,
			LineStart:  1017021,
			LineEnd:    1031685,
			Confidence: 0.92,
			Summary:    "binder wait on dep",
		}},
	}
}

// TestCWD2WindowStatsRankRowsCarryStatsWindow: positive stamp on the C7
// witness family (supply_pressure / io_pressure summaries), negative on the
// wakeup_chain-sourced row (only the typed window_stats Source family carries
// the stats window), and the zero-window control (absence never guesses).
func TestCWD2WindowStatsRankRowsCarryStatsWindow(t *testing.T) {
	window := TimeWindow{StartTs: 3680.569, EndTs: 3680.719}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: window.StartTs, TimeEnd: window.EndTs},
		cwd2ChainFixture(), cwd2StatsFixture(window))
	byType := map[string]RootCauseRankItem{}
	for _, item := range rank.Items {
		byType[item.Type] = item
	}
	for _, typ := range []string{"supply_pressure", "io_pressure"} {
		item, ok := byType[typ]
		if !ok {
			t.Fatalf("fixture must mint a %s rank row: %+v", typ, rank.Items)
		}
		if item.StatsWindowStartTs != window.StartTs || item.StatsWindowEndTs != window.EndTs {
			t.Fatalf("§21.1 CWD-2 ②: %s row must carry the stats query window %+v: %+v", typ, window, item)
		}
	}
	chainRow, ok := byType["binder_wait"]
	if !ok {
		t.Fatalf("fixture must mint the wakeup_chain binder_wait row: %+v", rank.Items)
	}
	if chainRow.StatsWindowStartTs != 0 || chainRow.StatsWindowEndTs != 0 {
		t.Fatalf("a wakeup_chain-sourced row must NOT carry the stats-window stamp: %+v", chainRow)
	}

	// Zero-window control: an unbounded stats window stamps nothing.
	bare := buildRootCauseRankFrom(nil, Query{}, cwd2ChainFixture(), cwd2StatsFixture(TimeWindow{}))
	for _, item := range bare.Items {
		if item.StatsWindowStartTs != 0 || item.StatsWindowEndTs != 0 {
			t.Fatalf("unbounded stats window must stamp nothing (absence never guesses): %+v", item)
		}
	}
}

// TestCWD2StatsWindowStampNeverTouchesRanking pins the 零触碰 red line: the
// stamped and unstamped builds publish identical impact/score/rank/order —
// the ONLY delta is the identity stamp itself.
func TestCWD2StatsWindowStampNeverTouchesRanking(t *testing.T) {
	window := TimeWindow{StartTs: 3680.569, EndTs: 3680.719}
	q := Query{TimeStart: window.StartTs, TimeEnd: window.EndTs}
	stamped := buildRootCauseRankFrom(nil, q, cwd2ChainFixture(), cwd2StatsFixture(window))
	bare := buildRootCauseRankFrom(nil, q, cwd2ChainFixture(), cwd2StatsFixture(TimeWindow{}))
	if len(stamped.Items) != len(bare.Items) {
		t.Fatalf("stamp must not change the item set: %d vs %d", len(stamped.Items), len(bare.Items))
	}
	for i := range stamped.Items {
		a, b := stamped.Items[i], bare.Items[i]
		a.StatsWindowStartTs, a.StatsWindowEndTs = 0, 0
		if a.Type != b.Type || a.ImpactMs != b.ImpactMs || a.Score != b.Score ||
			a.Rank != b.Rank || a.CumulativeImpactMs != b.CumulativeImpactMs {
			t.Fatalf("stamp leaked into a ranking lane at index %d:\nstamped: %+v\nbare:    %+v", i, stamped.Items[i], b)
		}
	}
}

// TestCWD2StampHelperScopesOnSourceFamily pins the helper's typed-source scope
// directly (the enrich pass calls the same helper for its compute_supply /
// cpu_constraints additions).
func TestCWD2StampHelperScopesOnSourceFamily(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "compute_supply", Source: "window_stats.compute_supply"},
		{Type: "scheduler_latency", Source: "scheduler_latency_stats"},
		{Type: "binder_wait", Source: "wakeup_chain"},
	}
	stampWindowStatsRankQueryWindow(items, TimeWindow{StartTs: 1.0, EndTs: 2.0})
	if items[0].StatsWindowStartTs != 1.0 || items[0].StatsWindowEndTs != 2.0 {
		t.Fatalf("window_stats-family row must be stamped: %+v", items[0])
	}
	for _, item := range items[1:] {
		if item.StatsWindowStartTs != 0 || item.StatsWindowEndTs != 0 {
			t.Fatalf("non-window_stats sources must stay unstamped: %+v", item)
		}
	}
	// Degenerate windows stamp nothing.
	stampWindowStatsRankQueryWindow(items, TimeWindow{StartTs: 2.0, EndTs: 2.0})
	stampWindowStatsRankQueryWindow(items, TimeWindow{StartTs: -1.0, EndTs: 2.0})
	if items[1].StatsWindowStartTs != 0 || items[2].StatsWindowStartTs != 0 {
		t.Fatalf("degenerate windows must stamp nothing: %+v", items)
	}
}
