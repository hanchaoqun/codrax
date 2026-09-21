package tool

// Exercise the optional output flag through public parsing, engine execution,
// publication and run memo; changing it must never change causal evidence.

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func windowStatsOptionPayload(t *testing.T, result types.ToolResult) tracequery.Result {
	t.Helper()
	var payload tracequery.Result
	if err := json.Unmarshal(windowStatsOptionPayloadBytes(t, result), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func windowStatsOptionPayloadBytes(t *testing.T, result types.ToolResult) []byte {
	t.Helper()
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef == "" {
			continue
		}
		body, err := os.ReadFile(observation.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	t.Fatal("public query has no full result payload")
	return nil
}

func TestTraceQueryWindowStatsOptionPublicAndMemo(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	var baseline *tracequery.ChainResult
	seenRefs := map[string]bool{}
	for _, tc := range []struct {
		name      string
		value     any
		wantStats bool
		wantMemo  bool
	}{
		{"omitted", nil, true, false},
		{"false", false, false, false},
		{"true", true, true, false},
		{"false_cached", false, false, true},
		{"true_fresh_navigation", true, true, false},
		{"omitted_fresh_navigation", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			overrides := map[string]any{"view": "wakeup_chain"}
			if tc.value != nil {
				overrides["include_window_stats"] = tc.value
			}
			result, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, overrides))
			if err != nil || !result.Success {
				t.Fatalf("query failed: %v %s", err, result.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			if result.ReusedFromRunMemo != tc.wantMemo {
				t.Errorf("memo hit=%v, want %v", result.ReusedFromRunMemo, tc.wantMemo)
			}
			payload := windowStatsOptionPayload(t, result)
			if (payload.WindowStats != nil) != tc.wantStats {
				t.Errorf("WindowStats present=%v, want %v", payload.WindowStats != nil, tc.wantStats)
			}
			// Actual complete sync pairs require current read-bound references;
			// no-stats results remain cacheable without changing causal content.
			wantRefs := 0
			if tc.wantStats {
				wantRefs = 1
				if len(payload.WindowStats.TraceSpans) != wantRefs {
					t.Fatal("fixture must exercise one native complete pair")
				}
			}
			if len(result.TraceBusinessSpanRefs) != wantRefs {
				t.Fatalf("navigation refs=%d want=%d", len(result.TraceBusinessSpanRefs), wantRefs)
			}
			for _, ref := range result.TraceBusinessSpanRefs {
				if seenRefs[ref.Token()] {
					t.Fatal("fresh native read reused an old navigation token")
				}
				seenRefs[ref.Token()] = true
				if _, ok := ctx.Mutable.ResolveTraceBusinessSpanRef(ref.Token()); !ok {
					t.Fatal("published navigation token is not current")
				}
			}
			if strings.Contains(result.Summary, "## Window stats") != tc.wantStats {
				t.Errorf("window statistics summary does not respect the optional output flag")
			}
			foundRunning := false
			for _, o := range result.Observations {
				foundRunning = foundRunning || o.Predicate == "running_time"
			}
			if foundRunning != tc.wantStats {
				t.Errorf("typed statistics observations present=%v, want %v", foundRunning, tc.wantStats)
			}
			if payload.WakeupChain == nil || len(payload.WakeupChain.Edges) == 0 || len(payload.WakeupChain.CausalImpacts) == 0 {
				t.Fatal("optional statistics switch removed the nonempty wakeup chain")
			}
			for _, predicate := range []string{"wakeup_chain_edge", "wakeup_causal_impact"} {
				found := false
				for _, o := range result.Observations {
					found = found || o.Predicate == predicate
				}
				if !found {
					t.Errorf("optional statistics removed causal observation %s", predicate)
				}
			}
			if baseline == nil {
				baseline = payload.WakeupChain
			} else if !reflect.DeepEqual(baseline, payload.WakeupChain) {
				t.Error("optional statistics changed causal chain content")
			}
		})
	}
}

func TestTraceQueryWindowStatsOptionDoesNotDisableOtherViewStatistics(t *testing.T) {
	for _, view := range []string{"window_stats", "root_cause_rank", "frame_root_cause_bundle", "recipe"} {
		t.Run(view, func(t *testing.T) {
			ctx, _ := pureMemoFixtureCtx(t)
			var baseline tracequery.Result
			for _, include := range []bool{true, false} {
				overrides := map[string]any{"view": view, "include_window_stats": include}
				if view == "recipe" {
					overrides["recipe_name"] = "jank"
				}
				result, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, overrides))
				if err != nil || !result.Success {
					t.Fatalf("%s query failed: %v %s", view, err, result.Summary)
				}
				payload := windowStatsOptionPayload(t, result)
				if payload.WindowStats == nil {
					t.Fatal("fixture did not exercise required window statistics")
				}
				if view != "window_stats" && (payload.RootCauseRank == nil || len(payload.RootCauseRank.Items) == 0 || payload.RootCauseRank.BoardParamsFingerprint == "") {
					t.Fatal("fixture did not exercise a nonempty root-cause ranking with board identity")
				}
				if include {
					baseline = payload
				} else {
					if !reflect.DeepEqual(baseline.WindowStats, payload.WindowStats) || !reflect.DeepEqual(baseline.RootCauseRank, payload.RootCauseRank) || !reflect.DeepEqual(baseline.WakeupChain, payload.WakeupChain) || !reflect.DeepEqual(baseline.FrameRootCauseBundle, payload.FrameRootCauseBundle) || !reflect.DeepEqual(baseline.Recipe, payload.Recipe) {
						t.Error("wakeup_chain-only output option changed another view's required statistics, causal evidence or ranking")
					}
				}
			}
		})
	}
}

func TestTraceQueryWindowStatsOptionAliasAndStatsSidecars(t *testing.T) {
	ctx, path := pureMemoFixtureCtx(t)
	extra := " worker-200 (200) [002] .... 5.002000: print: B|200|LoadDocumentIndex\n" +
		" policy-10 (10) [002] .... 5.002200: cpu_frequency_limits: min=558000 max=2270000 cpu_id=2\n" +
		" worker-200 (200) [002] .... 5.003000: print: E|200\n"
	body := strings.Replace(pureMemoFixtureTrace, " worker-200   (  200) [002] .... 5.005000:", extra+" worker-200   (  200) [002] .... 5.005000:", 1)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{true, false} {
		value := "false"
		if include {
			value = "true"
		}
		result, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, map[string]any{"view": "causal_impact", "include_window_stats": value}))
		if err != nil || !result.Success {
			t.Fatalf("alias query failed: %v %s", err, result.Summary)
		}
		payload := windowStatsOptionPayload(t, result)
		if payload.View != "wakeup_chain" || (payload.WindowStats != nil) != include {
			t.Errorf("alias/string-bool option lost: view=%s stats=%v", payload.View, payload.WindowStats != nil)
		}
		for _, predicate := range []string{"running_time", "trace_semantic_span", types.TraceBusinessSpanPredicate} {
			found := false
			for _, o := range result.Observations {
				found = found || o.Predicate == predicate
			}
			if found != include {
				t.Errorf("stats sidecar %s present=%v, want %v", predicate, found, include)
			}
		}
		if result.TraceEvidenceAuthority == nil {
			t.Fatal("missing causal authority")
		}
		if (len(result.TraceEvidenceAuthority.FrequencyLimitWitnesses) > 0) != include {
			t.Errorf("stats-only frequency-limit witnesses ignored option")
		}
		if payload.WakeupChain == nil || len(payload.WakeupChain.Edges) == 0 {
			t.Fatal("alias lost causal edges")
		}
	}
}

func TestTraceQueryWindowStatsOptionAutoWindowCopies(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()
	for _, count := range []int{1, 2} {
		ctx, path := pureMemoFixtureCtx(t)
		body := "app-100 (100) [001] .... 4.999600: print: B|100|ResponseWindow\n" + pureMemoFixtureTrace +
			"app-100 (100) [001] .... 5.007200: print: E|100\n"
		if count == 2 {
			body += strings.ReplaceAll(strings.ReplaceAll(body, "4.999600", "7.999600"), "5.0", "8.0")
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, include := range []bool{true, false} {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "pattern": "ResponseWindow", "include_window_stats": include})
			result, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("auto-window %d failed: %v %s", count, err, result.Summary)
			}
			var results []tracequery.Result
			if count == 1 {
				results = []tracequery.Result{windowStatsOptionPayload(t, result)}
			} else {
				var payload struct {
					Results []traceQueryAutoWindowChild `json:"results"`
				}
				if err := json.Unmarshal(windowStatsOptionPayloadBytes(t, result), &payload); err != nil {
					t.Fatal(err)
				}
				for _, child := range payload.Results {
					if child.Error != "" {
						t.Fatalf("child failed: %s", child.Error)
					}
					results = append(results, child.Result)
				}
			}
			if len(results) != count {
				t.Fatalf("auto-window has %d results, want %d", len(results), count)
			}
			for _, child := range results {
				if child.WakeupChain == nil || len(child.WakeupChain.Edges) == 0 || child.TimeEnd <= child.TimeStart {
					t.Fatal("auto-window lost bounded nonempty chain")
				}
				if (child.WindowStats != nil) != include {
					t.Errorf("stats option did not survive automatic window copy: include=%v stats=%v", include, child.WindowStats != nil)
				}
				if strings.Contains(strings.Join(child.Caveats, "\n"), "relation_scoped_window_stats_unavailable") {
					t.Error("complete tiny window should not fall back to relation pruning")
				}
			}
		}
	}
}
