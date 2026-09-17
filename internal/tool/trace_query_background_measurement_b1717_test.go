package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Start with the recorded scheduler events from the public regression case.
// Neither the engine result nor an observation is hand-authored: the 7 ms
// private ranking cap must arise naturally from the selected 20 ms window.
func b1717BackgroundTrace(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("fixture is missing its trace carrier")
	}
	body, _, ok = strings.Cut(body, "\n'\n")
	if !ok {
		t.Fatal("fixture trace carrier is not closed")
	}
	return body + "\n"
}

func TestB1717BackgroundMeasurementUsesNativeStateNotRankingCap(t *testing.T) {
	base := b1717BackgroundTrace(t)
	const loggerStart = "2.000500: sched_switch: prev_comm=logger prev_pid=900 prev_prio=20 prev_state=D"
	for _, tc := range []struct {
		name, kind string
		body       string
		measuredMS float64
		capped     bool
	}{
		{name: "recorded_D_state", kind: "d_state_or_io_wait", body: base, measuredMS: 19.5, capped: true},
		{name: "explicit_IO_wait", kind: "io_wait", body: strings.Replace(base,
			"     cookie-200", "         irq-2 (2) [005] .... 2.000750: sched_blocked_reason: pid=900 iowait=1 caller=io_schedule\n     cookie-200", 1), measuredMS: 19.5, capped: true},
		{name: "runnable", kind: "runnable_wait", body: strings.Replace(base, loggerStart,
			strings.TrimSuffix(loggerStart, "D")+"R", 1), measuredMS: 19.5, capped: true},
		{name: "short_D_state_positive", kind: "d_state_or_io_wait", body: strings.Replace(base, loggerStart,
			strings.Replace(loggerStart, "2.000500", "2.017500", 1), 1), measuredMS: 2.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "background.ftrace")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			path, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			native := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100,
				TimeStart: 2, TimeEnd: 2.02, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace})
			if native.RootCauseRank == nil {
				t.Fatal("fixture produced no native rank")
			}
			var owner *tracequery.RootCauseRankItem
			for i := range native.RootCauseRank.Items {
				item := &native.RootCauseRank.Items[i]
				if item.Thread.PID == 900 && item.Type == tc.kind && item.ChainRelevance == "background" {
					owner = item
					break
				}
			}
			if owner == nil {
				t.Fatalf("native background state owner absent: %+v", native.RootCauseRank.Items)
			}
			measured := owner.DStateMs + owner.IOWaitMs
			if tc.kind == "runnable_wait" {
				measured = owner.RunnableMs
			}
			if math.Abs(measured-tc.measuredMS) > 1e-6 || owner.MeasurementSources == nil {
				t.Fatalf("native measurement/source fixture drift: %+v", owner)
			}
			if tc.capped && (math.Abs(owner.ImpactMs-7) > 1e-6 || owner.ImpactMs >= measured) {
				t.Fatalf("fixture no longer exercises private rank cap versus measured state: %+v", owner)
			}
			if !tc.capped && math.Abs(owner.ImpactMs-measured) > 1e-6 {
				t.Fatalf("short-state positive must not exercise cap: %+v", owner)
			}

			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("background state measurement")}
			params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank",
				"trace_flavor": "harmony_hitrace", "pid": 100, "time_start": 2, "time_end": 2.02})
			if err != nil {
				t.Fatal(err)
			}
			published, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !published.Success {
				t.Fatalf("public TraceQuery failed: err=%v result=%+v", err, published)
			}
			var record *types.ObservationRecord
			for i := range published.Observations {
				candidate := &published.Observations[i]
				if candidate.Subject == "logger-900" && candidate.Object == tc.kind && candidate.Predicate == "root_cause_background" {
					record = candidate
					break
				}
			}
			if record == nil {
				t.Fatalf("public background measurement absent: %+v", published.Observations)
			}
			t.Logf("native state=%.3fms private ranking impact=%.3fms; public value=%s unit=%s producer=%s source=%+v",
				measured, owner.ImpactMs, record.Value, record.Unit, record.Producer, record.SourceRef)
			if record.Role != types.AnswerAggregateRoleSupportingCoverage || record.ProvenanceLane != types.ObservationProvenanceArtifactSpan ||
				record.GroundingPolicy != types.ClaimGroundingHard || record.Producer != "trace_query" ||
				record.SourceRef.QueryScopeID == "" || record.SourceRef.Path != path || record.MeasurementSources == nil {
				t.Errorf("background publication changed causal eligibility or lost exact query/source: %+v", record)
			}
			if record.Value != fmt.Sprintf("%.3f", measured) || record.Unit != "ms" {
				t.Errorf("private ordering cap was published as measured scheduler time: measured=%.3fms internal_impact=%.3fms record=%+v", measured, owner.ImpactMs, record)
			}
			if !strings.Contains(strings.Join(record.RichNotes, "\n"), "effective_impact_ms=0.000") {
				t.Error("background observation must not acquire effective target attribution")
			}
			wireBytes, err := os.ReadFile(record.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var wire tracequery.Result
			if err := json.Unmarshal(wireBytes, &wire); err != nil || wire.RootCauseRank == nil {
				t.Fatalf("published query JSON missing: %v", err)
			}
			wireFound := false
			onChainFound := false
			for _, item := range wire.RootCauseRank.Items {
				if item.Thread.PID == 400 && item.Type == "io_wait" && item.ChainRelevance == "on_chain" {
					onChainFound = true
					if math.Abs(item.ImpactMs-11) > 1e-6 {
						t.Errorf("independent on-chain IO measurement changed: %+v", item)
					}
				}
				if item.Thread.PID != 900 || item.Type != tc.kind {
					continue
				}
				wireFound = true
				if math.Abs(item.ImpactMs-measured) > 1e-6 || math.Abs(item.ProjectedImpactMs-measured) > 1e-6 {
					t.Errorf("wire observation scalars retain private rank cap: measured=%.3fms wire=%+v", measured, item)
				}
				if item.EffectiveImpactMs != 0 || item.ChainRelevance != "background" || item.Rank != 0 || math.Abs(item.Score-owner.Score) > 1e-6 {
					t.Errorf("wire copy changed background qualification/private score: native=%+v wire=%+v", owner, item)
				}
			}
			if !wireFound || !onChainFound {
				t.Errorf("wire lost independent background/on-chain row: background=%t on_chain=%t", wireFound, onChainFound)
			}
			ctx.ToolResults = append(ctx.ToolResults, published)
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			projection := types.CompileTraceCausalProjection(ledger)
			projectedFound := false
			for _, node := range projection.BackgroundCauses {
				if node.Subject != "logger-900" || node.Predicate != "root_cause_background" {
					continue
				}
				projectedFound = true
				if math.Abs(node.ImpactMS-measured) > 1e-6 {
					t.Errorf("published notes/projection still label rank cap as observed state: measured=%.3fms node=%+v", measured, node)
				}
				if occupancy, ok := node.PublishedStateOccupancy(); !ok || math.Abs(occupancy-measured) > 1e-6 {
					t.Errorf("original typed state account was lost: occupancy=%v known=%t node=%+v", occupancy, ok, node)
				}
			}
			if !projectedFound {
				t.Error("projection lost measured background row")
			}
		})
	}
}

// Two independently published queries now carry the same original 150 ms
// state, not a 52.5 ms ordering cap beside 150 ms. Keep the existing one-seat
// rule: one measured occurrence with both references, never a two-occurrence
// SUM. The full ledger may converge exact state-account twins before V4.
func TestB1717CalibratedBackgroundQueriesKeepOnePhysicalMeasurement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same-measurement.ftrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("independent background queries")}
	ids := map[string]bool{}
	for _, pid := range []int{100, 0} {
		params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank",
			"pid": pid, "time_start": 1.0, "time_end": 1.15, "min_duration_ms": 0.05})
		if err != nil {
			t.Fatal(err)
		}
		queryCtx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("independent query")}
		result, err := (&TraceQuery{}).Execute(queryCtx, params)
		if err != nil || !result.Success {
			t.Fatalf("public query pid=%d: err=%v result=%+v", pid, err, result)
		}
		found := false
		wantPredicate := "root_cause_background"
		if pid == 0 {
			wantPredicate = "root_cause_unattributed"
		}
		for _, record := range result.Observations {
			// The untargeted producer has no chain qualification: its
			// unattributed row must remain in background when combined.
			if record.Subject == "isplogd-1300" && record.Object == "d_state_or_io_wait" &&
				strings.Contains(record.ID, "#root_cause_rank:") &&
				record.Predicate == wantPredicate {
				found = true
				if record.Value != "150.000" {
					t.Fatalf("both query lanes must expose the original state, pid=%d record=%+v", pid, record)
				}
				if record.Role != types.AnswerAggregateRoleSupportingCoverage || record.ProvenanceLane != types.ObservationProvenanceArtifactSpan {
					t.Fatalf("query must not mint principal/direct-cause authority: %+v", record)
				}
				if pid == 100 && !strings.Contains(strings.Join(record.RichNotes, "\n"), "effective_impact_ms=0.000") {
					t.Fatal("explicit background publication must retain zero effective attribution")
				}
				ids[record.ID] = true
			}
		}
		if !found {
			t.Fatalf("pid=%d lost the background record", pid)
		}
		ctx.ToolResults = append(ctx.ToolResults, result)
	}
	if len(ids) != 2 {
		t.Fatalf("fixture needs two independent publication ids, got %v", ids)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("expected one selected window, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	for _, group := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses} {
		for _, node := range group {
			if node.Subject == "isplogd-1300" {
				t.Fatalf("background measurement must not acquire chain qualification: %+v", node)
			}
		}
	}
	seats := 0
	for _, node := range projection.BackgroundCauses {
		if node.Subject != "isplogd-1300" {
			continue
		}
		seats++
		if math.Abs(node.ImpactMS-150) > 1e-6 || math.Abs(node.CumulativeImpactMS-150) > 1e-6 ||
			node.ChainRelevance != "background" || node.Role != types.TraceCausalRoleRootCauseContext || node.MergedCount != 0 {
			t.Fatalf("republication must keep one noncausal 150ms occurrence, not a SUM: %+v", node)
		}
		for _, id := range append([]string{node.EvidenceID}, node.MergedEvidenceIDs...) {
			delete(ids, id)
		}
	}
	if seats != 1 || len(ids) != 0 {
		t.Fatalf("physical seat or evidence union lost: seats=%d missing_ids=%v", seats, ids)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	rendered := 0
	for _, row := range model.Background {
		if row.Node.Subject != "isplogd-1300" {
			continue
		}
		rendered++
		if !strings.Contains(runtimeTraceProjCauseEvidenceRef(row), "(+") {
			t.Fatalf("second publication reference lost at display: %+v", row)
		}
	}
	if rendered != 1 {
		t.Fatalf("one physical background state must render once, got %d", rendered)
	}
}
