package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Start with actual engine-owned streams, not an ID-to-domain convention or a
// hand-authored observation. No proposed source-set field is needed to compile
// the initial RED: the exact native partition token must survive publication.
func b1638b2bPublishedNativeSources(t *testing.T, view string) (*types.BusContext, types.ToolResult, tracequery.Result) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "native-sources.ftrace")
	var body strings.Builder
	for i := 0; i < 9; i++ {
		start := 1 + float64(i)*.03
		state := "S"
		if i%3 != 0 {
			state = "D"
		}
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120\n", start)
		fmt.Fprintf(&body, "worker-55 (55) [000] .... %.6f: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", start+.01, state)
		if state == "D" {
			fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_blocked_reason: pid=55 iowait=%d caller=io_schedule\n", start+.015, i%3-1)
		}
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0\n", start+.02)
	}
	fmt.Fprintln(&body, "idle-0 (0) [000] .... 1.270000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("native measurement sources")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 55, "time_start": 1, "time_end": 1.27})
	published, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !published.Success {
		t.Fatalf("actual query failed: %v %+v", err, published)
	}
	ctx.Mutable.AppendDispatchToolResult(published)
	for _, record := range published.Observations {
		if record.Predicate != "target_window_states" {
			continue
		}
		if record.SourceRef.Path != path || record.SourceRef.PayloadRef == "" || record.SourceRef.QueryScopeID == "" {
			t.Fatalf("fixture lacks its actual outer source receipt: %+v", record.SourceRef)
		}
		payload, err := os.ReadFile(record.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var native tracequery.Result
		if err := json.Unmarshal(payload, &native); err != nil || native.WindowStats == nil {
			t.Fatalf("actual native JSON missing: %v", err)
		}
		return ctx, published, native
	}
	t.Fatal("fixture did not publish its target account")
	return nil, types.ToolResult{}, tracequery.Result{}
}

func b1638b2bSource(label string) *types.TraceSchedulerMeasurementSources {
	return types.TraceSchedulerMeasurementSourcesFromDomain(&types.TraceSchedulerMeasurementDomain{
		Version: 1, Status: "constructed_partition", Method: "off_cpu_sweep", TargetTID: 55,
		WindowStartTs: 1, WindowEndTs: 2, PartitionID: "scheduler_partition:v1:" + label,
	})
}

func TestB1638B2BToolPreservesFoldedUnknownAndOnlyActualMembers(t *testing.T) {
	a, b, excluded := b1638b2bSource("a"), b1638b2bSource("b"), b1638b2bSource("not-admitted")
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/native.ftrace", PayloadRef: "/query/result.json", QueryScopeID: "actual-child"}
	td := tracequery.ThreadDuration{Thread: tracequery.ThreadRef{PID: 55, Comm: "worker"}, DurationMs: 12, CPU: 0,
		MeasurementDomain: &excluded.Domains[0], MeasurementSources: types.MergeTraceSchedulerMeasurementSources(a, nil, b)}
	before, _ := json.Marshal(td)
	rows := traceQueryTypedThreadDurationObservations([]tracequery.ThreadDuration{td}, tracequery.TimeWindow{StartTs: 1, EndTs: 2}, ref,
		"scope", "time", "top_sleep", "sleep_wait", "sleep", "native bucket", .8)
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].MeasurementSources, td.MeasurementSources) || !rows[0].MeasurementSources.HasUnknown {
		t.Fatalf("folded sources or unknown contributor were replaced by legacy single domain: %+v", rows)
	}
	if !reflect.DeepEqual(rows[0].SourceRef, ref) || rows[0].Value != "12.000" {
		t.Fatal("measurement metadata changed original source or value")
	}
	rows[0].MeasurementSources.Domains[0].PartitionID = "mutated publication"
	after, _ := json.Marshal(td)
	if !bytes.Equal(before, after) {
		t.Fatal("published source inventory aliases native input")
	}
	for _, unknown := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			members := []tracequery.WakeupCausalImpact{
				{Thread: td.Thread, OnChain: true, DominantState: "sleep", DominantImpactMs: 10, MeasurementSources: a},
				{Thread: td.Thread, OnChain: true, DominantState: "sleep", DominantImpactMs: 20, MeasurementSources: b},
				{Thread: td.Thread, OnChain: false, DominantState: "sleep", DominantImpactMs: 100, MeasurementSources: excluded},
			}
			if unknown {
				members = append(members, tracequery.WakeupCausalImpact{Thread: td.Thread, OnChain: true, DominantState: "sleep", DominantImpactMs: 5})
			}
			if reverse {
				for i, j := 0, len(members)-1; i < j; i, j = i+1, j-1 {
					members[i], members[j] = members[j], members[i]
				}
			}
			before, _ := json.Marshal(members)
			fold, ok := traceQueryWakeupCausalImpactFoldRecord("scope", ref, "time", members, tracequery.TimeWindow{StartTs: 1, EndTs: 2})
			want := types.MergeTraceSchedulerMeasurementSources(a, b)
			want.HasUnknown = unknown
			if !ok || fold.Value != "20.000" || !reflect.DeepEqual(fold.MeasurementSources, want) || !reflect.DeepEqual(fold.SourceRef, ref) {
				t.Fatalf("overflow fold lost a real source, added excluded source, or changed its existing max ruler: unknown=%t reverse=%t fold=%+v", unknown, reverse, fold)
			}
			fold.MeasurementSources.Domains[0].PartitionID = "mutated fold"
			after, _ := json.Marshal(members)
			if !bytes.Equal(before, after) {
				t.Fatal("overflow publication mutated its source members")
			}
		}
	}
}

func TestB1638B2BPublicationViewsCloneOnlySourceMetadata(t *testing.T) {
	sources := types.MergeTraceSchedulerMeasurementSources(b1638b2bSource("a"), nil)
	impact := tracequery.WakeupCausalImpact{Thread: tracequery.ThreadRef{PID: 55}, DominantState: "sleep", DominantImpactMs: 10, MeasurementSources: sources}
	aggregate := tracequery.WakeupCausalAggregate{Thread: impact.Thread, DominantState: "sleep", DominantImpactMs: 10, MeasurementSources: sources}
	item := tracequery.RootCauseRankItem{Thread: impact.Thread, Type: "sleep_wait", ImpactMs: 10, MeasurementSources: sources}
	for name, publish := range map[string]func() *types.TraceSchedulerMeasurementSources{
		"rank": func() *types.TraceSchedulerMeasurementSources {
			return traceQueryRootCauseForPublication(item).MeasurementSources
		},
		"impact": func() *types.TraceSchedulerMeasurementSources {
			return traceQueryPriorityCausalImpactForPublication(impact).MeasurementSources
		},
		"aggregate": func() *types.TraceSchedulerMeasurementSources {
			return traceQueryPriorityCausalAggregateForPublication(aggregate).MeasurementSources
		},
	} {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(sources)
			copy := publish()
			if copy == sources || !reflect.DeepEqual(copy, sources) {
				t.Fatal("publication lost source inventory or shares its mutable carrier")
			}
			copy.Domains[0].PartitionID = "mutated publication"
			after, _ := json.Marshal(sources)
			if !bytes.Equal(before, after) {
				t.Fatal("publication aliases native source entries")
			}
		})
	}
}

func TestB1638B2BActualNativeSourcePublicationAndLedger(t *testing.T) {
	ctx, published, native := b1638b2bPublishedNativeSources(t, "window_stats")
	stats := native.WindowStats
	wanted := map[string]*types.TraceSchedulerMeasurementDomain{}
	for _, lane := range []struct {
		predicate string
		rows      []tracequery.ThreadDuration
	}{
		{"running_time", stats.TopRunning}, {"runnable_wait", stats.RunnableTop}, {"sleep_wait", stats.SleepTop},
		{"d_state_or_io_wait", stats.DStateTop}, {"io_wait", stats.IOWaitTop},
	} {
		for _, row := range lane.rows {
			if row.Thread.PID == 55 && row.DurationMs > 0 {
				wanted[lane.predicate] = row.MeasurementDomain
			}
		}
	}
	for _, row := range stats.StateChurn {
		if row.Thread.PID == 55 && row.TotalMs > 0 {
			wanted["state_churn"] = row.MeasurementDomain
		}
	}
	if native.TargetWindowStates != nil {
		wanted["target_window_states"] = native.TargetWindowStates.MeasurementDomain
	}
	if len(wanted) != 7 {
		t.Fatalf("fixture did not exercise all native publication paths: %v", wanted)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	for _, stage := range []struct {
		name    string
		records []types.ObservationRecord
	}{{"actual_tool_result", published.Observations}, {"dispatch_to_ledger", ledger.Records}} {
		t.Run(stage.name, func(t *testing.T) {
			for predicate, domain := range wanted {
				if domain == nil || domain.PartitionID == "" {
					t.Fatalf("native %s fixture has no known source", predicate)
				}
				found := false
				for _, record := range stage.records {
					// EvidencePack also has deliberately valueless mirrors with
					// this predicate. They may not borrow a stat row's source.
					if record.Predicate != predicate || record.Subject != "worker-55" || record.Value == "" {
						continue
					}
					found = true
					wire, err := json.Marshal(record)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(wire, []byte(domain.PartitionID)) {
						t.Errorf("%s lost native %s source %s while retaining value=%s and outer query=%s", stage.name, predicate, domain.PartitionID, record.Value, record.SourceRef.QueryScopeID)
					}
				}
				if !found {
					t.Errorf("fixture lost its existing %s observation", predicate)
				}
			}
		})
	}
}

func TestB1638B2BActualNativeSourceReachesProjection(t *testing.T) {
	ctx, _, native := b1638b2bPublishedNativeSources(t, "root_cause_rank")
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	projection := types.CompileTraceCausalProjection(ledger)
	records := map[string]types.ObservationRecord{}
	for _, record := range ledger.Records {
		records[record.ID] = record // exact linkage only, never parse ID bytes
	}
	var nodes []types.TraceCausalProjectionNode
	nodes = append(nodes, projection.RankedSeats...)
	nodes = append(nodes, projection.OnChainCauses...)
	nodes = append(nodes, projection.AdjacentCauses...)
	nodes = append(nodes, projection.BackgroundCauses...)
	checked := 0
	for _, node := range nodes {
		record, ok := records[node.EvidenceID]
		if !ok || record.MeasurementSources == nil || len(record.MeasurementSources.Domains) == 0 {
			continue
		}
		checked++
		b1638b2bAssertFinalOrigin(t, node.MeasurementOrigins, record)
		wire, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		for _, domain := range record.MeasurementSources.Domains {
			if !bytes.Contains(wire, []byte(domain.PartitionID)) {
				t.Errorf("actual projection node lost its input measurement source: predicate=%s object=%s value=%s source=%s", node.Predicate, node.Object, node.Value, domain.PartitionID)
			}
		}
	}
	if checked == 0 {
		t.Fatal("fixture must exercise an existing admitted node with a known native source; a stat row alone is not node admission")
	}
	if native.TargetWindowStates == nil || native.TargetWindowStates.MeasurementDomain == nil || projection.TargetStateAccount == nil {
		t.Fatal("fixture must retain its independently selected target account")
	}
	wire, err := json.Marshal(projection.TargetStateAccount)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(native.TargetWindowStates.MeasurementDomain.PartitionID)) {
		t.Error("selected target account lost its own native timeline source at projection handoff")
	}
	b1638b2bAssertFinalOrigin(t, projection.TargetStateAccount.MeasurementOrigins, records[projection.TargetStateAccount.EvidenceID])
}

func b1638b2bAssertFinalOrigin(t *testing.T, origins []types.TraceSchedulerMeasurementOrigin, record types.ObservationRecord) {
	t.Helper()
	for _, origin := range origins {
		if reflect.DeepEqual(origin.SourceRef, record.SourceRef) && origin.ObservedAt == record.ObservedAt &&
			reflect.DeepEqual(origin.MeasurementSources, record.MeasurementSources) {
			return
		}
	}
	t.Errorf("projection origin did not preserve final qualified parent and native source together: record=%+v sources=%+v origins=%+v", record.SourceRef, record.MeasurementSources, origins)
}

func TestB1638B2BDonghuActualLocalTimelineSource(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	var mainScope string
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprintf("line_filtered=%t", filtered), func(t *testing.T) {
			dir := t.TempDir()
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("native source regression")}
			params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "thread": "CompThread_0-2955",
				"time_start": 13762.791708, "time_end": 13763.024898}
			if filtered {
				params["line_start"], params["line_end"] = 1, 400
			}
			raw, _ := json.Marshal(params)
			published, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !published.Success {
				t.Fatalf("actual query failed: %v %s", err, published.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(published)
			var native tracequery.Result
			var account types.ObservationRecord
			for _, record := range published.Observations {
				if record.Predicate == "target_window_states" && record.Subject == "CompThread_0-2955" {
					account = record
				}
			}
			if account.SourceRef.Path != path || account.SourceRef.PayloadRef == "" || account.SourceRef.QueryScopeID == "" {
				t.Fatal("fixture lost its actual result identity")
			}
			payload, err := os.ReadFile(account.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload, &native); err != nil || native.WakeupChain == nil || native.TargetWindowStates == nil || native.TargetWindowStates.MeasurementDomain == nil {
				t.Fatalf("fixture lost complete native result: %v", err)
			}
			if !filtered {
				mainScope = account.SourceRef.QueryScopeID
			} else if mainScope == account.SourceRef.QueryScopeID {
				t.Fatal("same time, different line query cannot reuse parent result scope")
			}
			count, total, localNotTarget := 0, 0.0, false
			for _, record := range published.Observations {
				if record.Predicate != "wakeup_causal_impact" || record.Subject != "CompThread_0-2955" || record.Object != "s_sleep" {
					continue
				}
				count++
				var matched *tracequery.WakeupCausalImpact
				for i := range native.WakeupChain.CausalImpacts {
					impact := &native.WakeupChain.CausalImpacts[i]
					if impact.Thread.PID == 2955 && impact.DominantState == record.Object && impact.LineStart == record.Span.LineStart &&
						impact.LineEnd == record.Span.LineEnd && impact.Window.StartTs == record.Span.StartTs && impact.Window.EndTs == record.Span.EndTs {
						if matched != nil {
							t.Fatal("fixture has ambiguous physical native impact; cannot infer source from ID")
						}
						matched = impact
					}
				}
				if matched == nil || matched.MeasurementSources == nil || len(matched.MeasurementSources.Domains) == 0 {
					t.Errorf("native sleep impact lacks its own calculated local timeline source: %+v", matched)
					continue
				}
				total += matched.DominantImpactMs
				if !reflect.DeepEqual(record.MeasurementSources, matched.MeasurementSources) || !reflect.DeepEqual(record.SourceRef, account.SourceRef) {
					t.Error("publication changed local source inventory or parent result identity")
				}
				if record.MeasurementSources == nil {
					continue
				}
				for _, domain := range record.MeasurementSources.Domains {
					if domain.Method != "thread_timeline" || domain.TargetTID != 2955 {
						t.Errorf("impact borrowed a different native method or target: %+v", domain)
					}
					if domain.PartitionID != native.TargetWindowStates.MeasurementDomain.PartitionID {
						localNotTarget = true
					}
				}
			}
			wantCount, wantTotal := 8, 78.630
			if filtered {
				wantCount, wantTotal = 1, 230.286
			}
			if count != wantCount || math.Abs(total-wantTotal) > .0005 || (!filtered && !localNotTarget) {
				t.Fatalf("actual source-only query changed original sleep contributions or borrowed target receipt: count=%d total=%.6f distinct_local=%t", count, total, localNotTarget)
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			projection := types.CompileTraceCausalProjection(ledger)
			byID := map[string]types.ObservationRecord{}
			for _, record := range ledger.Records {
				byID[record.ID] = record
			}
			var nodes []types.TraceCausalProjectionNode
			nodes = append(nodes, projection.RankedSeats...)
			nodes = append(nodes, projection.OnChainCauses...)
			nodes = append(nodes, projection.BackgroundCauses...)
			checked := 0
			for _, node := range nodes {
				if record, ok := byID[node.EvidenceID]; ok && record.Subject == "CompThread_0-2955" && record.MeasurementSources != nil {
					checked++
					b1638b2bAssertFinalOrigin(t, node.MeasurementOrigins, record)
				}
			}
			if checked == 0 {
				t.Fatal("real query must carry known sources through admitted nodes, not just native JSON")
			}
		})
	}
}
