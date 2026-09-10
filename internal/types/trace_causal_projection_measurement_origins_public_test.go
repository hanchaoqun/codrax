package types_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real parsed native partitions and final ledger receipts, without a stage tap
// or a fabricated source. This tests preservation, NOT permission to add the
// main-query and line-filtered values (that independent B3 issue remains open).
func TestB1638B3ActualQueriesPreserveEverySleepOriginInProjection(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	run := func(filtered bool) types.ToolResult {
		params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "thread": "CompThread_0-2955", "time_start": 13762.791708, "time_end": 13763.024898}
		if filtered {
			params["line_start"], params["line_end"] = 1, 400
		}
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
		if err != nil || !result.Success {
			t.Fatalf("actual query failed: %v %s", err, result.Summary)
		}
		return result
	}
	main, filtered := run(false), run(true)
	for _, tc := range []struct {
		name    string
		results []types.ToolResult
		count   int
	}{
		{"main_only", []types.ToolResult{main}, 8},
		{"main_then_filtered", []types.ToolResult{main, filtered}, 9},
		{"filtered_then_main", []types.ToolResult{filtered, main}, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := 13762.791708, 13763.024898
			rm := types.RequestModel{Scenario: types.ScenarioRootCause, Intent: types.IntentRootCause,
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, Confidence: .9},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
				RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 2955, Thread: "CompThread_0-2955", Source: "user_explicit"}}}
			mut := types.NewMutableState("分析目标线程在明确窗口内的等待")
			mut.SetRequestModel(rm)
			for _, r := range tc.results {
				mut.AppendDispatchToolResult(r)
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, ToolResults: tc.results}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			want := map[string]bool{}
			for _, r := range ledger.Records {
				if r.Subject != "CompThread_0-2955" || r.Predicate != "wakeup_causal_impact" || r.Object != "s_sleep" {
					continue
				}
				if r.MeasurementSources == nil || len(r.MeasurementSources.Domains) == 0 || r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" || r.ObservedAt == "" {
					t.Fatal("actual source premise missing")
				}
				for _, o := range types.TraceSchedulerMeasurementOriginsFromRecord(r) {
					raw, _ := json.Marshal(o)
					want[string(raw)] = true
				}
			}
			if len(want) != tc.count {
				t.Fatalf("unexpected native input fixture: %d distinct origins, want %d", len(want), tc.count)
			}
			projection := types.CompileTraceCausalProjection(ledger)
			// A logical row can appear in more than one presentation bucket.
			// Compare exact bound-source coverage across those public views;
			// the focused merge tests separately pin per-member multiplicity.
			got, seats := map[string]bool{}, 0
			for _, bucket := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses, projection.SupportingHops} {
				for _, n := range bucket {
					if n.Subject != "CompThread_0-2955" || n.Predicate != "wakeup_causal_impact" || n.Object != "s_sleep" {
						continue
					}
					seats++
					for _, o := range n.MeasurementOrigins {
						raw, _ := json.Marshal(o)
						got[string(raw)] = true
					}
				}
			}
			if seats == 0 {
				t.Fatal("no published sleep seats reached")
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("published projection lost or invented actual member origins: got %d distinct origins, want %d (seats=%d)", len(got), len(want), seats)
			}
		})
	}
}
