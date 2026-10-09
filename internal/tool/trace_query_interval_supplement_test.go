package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

func intervalSupplementPublicContext(t *testing.T) (*types.BusContext, string, []byte) {
	t.Helper()
	ctx, prep, _ := hmc17NamedPathContext(t)
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	return intervalSupplementContextForSource(t, ctx, prep.Prepare, path, 1, 2)
}

func intervalSupplementContextForSource(t *testing.T, ctx *types.BusContext, prepare func(context.Context, string) (*attachment.TraceMaterial, error), path string, start, end float64) (*types.BusContext, string, []byte) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = m, m.Preview()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeArtifactScopeProfile: requestWindowTestProfile(start, end),
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
	}}
	return ctx, path, before
}

func intervalSupplementPublicPublication(t *testing.T, result types.ToolResult) {
	t.Helper()
	for _, obs := range result.Observations {
		if obs.Predicate != TraceMeasurementsPredicate {
			continue
		}
		pub, ok := types.DecodeRuntimeMeasurementPublication(obs)
		if !ok || len(pub.Tables) != 3 {
			t.Fatalf("not a valid same-source native three-table publication: %+v", pub)
		}
		for _, table := range pub.Tables {
			if table.MemberSet != nil {
				t.Fatal("raw values acquired member-set authority")
			}
			if table.View == types.RuntimeMeasurementMembers && len(table.Rows) != 13 {
				t.Fatalf("missing carry-in or native population: rows=%d", len(table.Rows))
			}
		}
		return
	}
	t.Fatal("no native measurement publication")
}

func TestIntervalSupplementPublicRawRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		already       bool
	}{
		{name: "unfiltered_native_family_needs_recovery"},
		{name: "pattern_must_not_be_silently_removed", pattern: "gpufreq"},
		{name: "explicit_native_query_is_not_repeated", already: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, source, sourceBefore := intervalSupplementPublicContext(t)
			args := map[string]any{"source": "attached_trace", "view": "event_search", "time_start": 1, "time_end": 2}
			if tc.pattern != "" {
				args["pattern"] = tc.pattern
			}
			discovery := hmc17NamedQuery(t, ctx, args)
			inv := requireEventSearchInventory(t, discovery).EventSearchInventory
			if inv == nil || len(inv.Rows) == 0 {
				t.Fatal("no real native family discovered")
			}
			physical, _, valid := ctx.Mutable.ResolveTraceQueryWindowReplay(discovery.TraceQueryWindowReplay)
			payload := hmc17NamedPayload(t, discovery)
			t.Logf("native source=%s source_count=%d legacy_replay_current=%v path=%q", payload.SourcePath, len(payload.TraceArtifacts), valid, physical)
			ctx.ToolResults = append(ctx.ToolResults, discovery)
			if tc.already {
				native := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "measurements", "time_start": 1, "time_end": 2})
				intervalSupplementPublicPublication(t, native)
				ctx.ToolResults = append(ctx.ToolResults, native)
			}
			modelBefore, _ := json.Marshal(ctx.ToolResults)
			out := RunTraceQuerySystemSupplement(ctx)
			modelAfter, _ := json.Marshal(ctx.ToolResults)
			if !bytes.Equal(modelBefore, modelAfter) {
				t.Fatal("system recovery mutated model transcript")
			}
			sourceAfter, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(sourceBefore, sourceAfter) {
				t.Fatal("source bytes changed")
			}
			results := ctx.Mutable.SystemTraceSupplementResults()
			t.Logf("actual parsed_rows=%d outcome=%+v system_results=%d", len(inv.Rows), out, len(results))
			if tc.pattern != "" || tc.already {
				if len(out.Executed) != 0 || len(results) != 0 {
					t.Fatal("widened selector or duplicated complete native query")
				}
				return
			}
			if len(out.Executed) != 1 || out.Executed[0] != "measurements" || len(results) != 1 {
				t.Fatalf("native raw-family discovery did not restore same-window interval publication: %+v", out)
			}
			intervalSupplementPublicPublication(t, results[0])
			if next := RunTraceQuerySystemSupplement(ctx); next.Attempted {
				t.Fatal("repeated supplement")
			}
		})
	}
}

func TestIntervalSupplementPublicAuthorityBoundaries(t *testing.T) {
	for _, name := range []string{"historical JSON", "new run", "member changed", "manifest changed", "wrong window", "missing window", "multiple windows", "named owner", "user owner without profile", "missing target declaration", "count only", "scheduler family", "causal", "independent relation", "relation dimension"} {
		t.Run(name, func(t *testing.T) {
			ctx, _, _ := intervalSupplementPublicContext(t)
			args := map[string]any{"source": "attached_trace", "view": "event_search", "time_start": 1, "time_end": 2}
			if name == "wrong window" {
				args["time_end"] = 2.1
			}
			r := hmc17NamedQuery(t, ctx, args)
			if !r.Success {
				t.Fatal(r.Summary)
			}
			ctx.ToolResults = []types.ToolResult{r}
			rm := &ctx.AnalysisIR.RequestModel
			switch name {
			case "historical JSON":
				b, _ := json.Marshal(ctx.ToolResults)
				ctx.ToolResults = nil
				if err := json.Unmarshal(b, &ctx.ToolResults); err != nil {
					t.Fatal(err)
				}
			case "new run":
				ctx.Mutable = types.NewMutableState("new turn")
			case "member changed":
				p := hmc17NamedPayload(t, r)
				if len(p.TraceArtifacts) != 1 {
					t.Fatal("not a single-member source")
				}
				if err := os.WriteFile(p.TraceArtifacts[0].SourcePath, []byte("replacement source\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "manifest changed":
				p := hmc17NamedPayload(t, r)
				if filepath.Ext(p.SourcePath) != ".json" {
					t.Fatal("fixture not using a prepared manifest")
				}
				if err := os.WriteFile(p.SourcePath, []byte("{}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing window":
				rm.RuntimeArtifactScopeProfile = nil
			case "multiple windows":
				a, b, c, d := 1.0, 2.0, 3.0, 4.0
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{{TimeStart: &a, TimeEnd: &b, SourceQuote: "first"}, {TimeStart: &c, TimeEnd: &d, SourceQuote: "second"}}}
			case "named owner":
				rm.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "owner"}
				rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 12, Source: "user_explicit"}}
			case "user owner without profile":
				rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 12, Source: "user_explicit"}}
			case "missing target declaration":
				rm.RuntimeTargetProfile = nil
			case "count only":
				rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}
			case "scheduler family":
				rm.RuntimeQuestionProfile.FactFamilies = append(rm.RuntimeQuestionProfile.FactFamilies, types.RuntimeQuestionFactTargetSchedulerState)
			case "causal":
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "independent relation":
				rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
			case "relation dimension":
				rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionRelationPath, Required: true}}}
			}
			out := RunTraceQuerySystemSupplement(ctx)
			for _, view := range out.Executed {
				if view == "measurements" {
					t.Fatalf("broadened request or borrowed stale authorization: %+v", out)
				}
			}
		})
	}
}

func TestIntervalSupplementParameterAllowlist(t *testing.T) {
	base := map[string]any{"source": "path", "path": "/capture", "view": "event_search", "time_start": 1, "time_end": 2, "pid": 0, "thread": "", "target_scope": "", "limit": 3}
	raw, _ := json.Marshal(base)
	params, _, ok := traceIntervalSupplementParams(raw, 1, 2)
	if !ok || len(params) != 6 {
		t.Fatalf("lost unfiltered precise query: %s %+v", raw, params)
	}
	for key, value := range map[string]any{"pattern": "freq", "patterns": []string{"freq"}, "pid": 9, "thread": "worker", "target_scope": "thread", "line_start": 1, "line_end": 5, "business_span_ref": "selected", "event_types": []string{"measure_interval"}, "event_names": []string{"gpufreq"}, "event_field_filters": []any{}, "span_name": "frame", "min_duration_ms": 5, "future_selector": true} {
		t.Run(key, func(t *testing.T) {
			var changed map[string]any
			_ = json.Unmarshal(raw, &changed)
			changed[key] = value
			encoded, _ := json.Marshal(changed)
			if _, _, allowed := traceIntervalSupplementParams(encoded, 1, 2); allowed {
				t.Fatal("silently dropped a selector", string(encoded))
			}
		})
	}
}

func TestIntervalSupplementPublicBudget(t *testing.T) {
	ctx, _, _ := intervalSupplementPublicContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "event_search", "time_start": 1, "time_end": 2})
	ctx.ToolResults = []types.ToolResult{r}
	old := traceSupplementMaxWindowSpanS
	traceSupplementMaxWindowSpanS = .5
	t.Cleanup(func() { traceSupplementMaxWindowSpanS = old })
	out := RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if len(out.Executed) != 0 || out.SkipReason != types.TraceSupplementReasonWindowSpanExceeded || meta == nil || len(meta.SkippedViews) != 1 || meta.SkippedViews[0] != "measurements" {
		t.Fatalf("missing budget disclosure: %+v meta=%+v", out, meta)
	}
}

const intervalSupplementThreeFamiliesSQL = `
CREATE TABLE measure_filter(id,name,type,source_arg_set_id);
INSERT INTO measure_filter VALUES (10,'vendor value','measure',NULL);
CREATE TABLE cpu_measure_filter(id,name,cpu);
INSERT INTO cpu_measure_filter VALUES (1,'cpu_idle',0),(2,'cpu_frequency',0);
CREATE TABLE measure(ts,dur,value,filter_id,type);
INSERT INTO measure VALUES (1000000000,100000000,7,10,'measure'),(1000000000,100000000,0,1,'cpu_measure'),(1000000000,100000000,1000000,2,'cpu_measure');
CREATE TABLE process(ipid,pid,name,start_ts,end_ts);
INSERT INTO process VALUES (1,100,'sample',0,3000000000);
CREATE TABLE process_measure_filter(id,name,ipid);
INSERT INTO process_measure_filter VALUES (1,'Resident',1);
CREATE TABLE process_measure(type,ts,dur,value,filter_id);
INSERT INTO process_measure VALUES ('process_measure',1000000000,100000000,11,1);
`

func TestIntervalSupplementPublicSharedCallBudget(t *testing.T) {
	path := measurementIndependentSQLiteFixture(t, intervalSupplementThreeFamiliesSQL)
	ctx, prep, _ := hmc17NamedPathContext(t)
	ctx, _, before := intervalSupplementContextForSource(t, ctx, prep.Prepare, path, 1, 2)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "event_search", "time_start": 1, "time_end": 2})
	_, _, views, ok := ctx.Mutable.ResolveTraceIntervalNavigation(context.Background(), r.TraceQueryWindowReplay)
	if !ok || len(views) != 3 {
		t.Fatalf("fixture did not produce three native families: %v %s", views, r.Summary)
	}
	ctx.ToolResults = []types.ToolResult{r}
	out := RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if len(out.Executed) != 2 || len(ctx.Mutable.SystemTraceSupplementResults()) != 2 || meta == nil || len(meta.SkippedViews) != 1 || meta.SkipReason != types.TraceSupplementReasonQueryBudgetExceeded {
		t.Fatalf("native discoveries bypassed shared two-call budget: %+v meta=%+v", out, meta)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}

func TestIntervalSupplementPublicEmptyNativeAlreadyCompleted(t *testing.T) {
	path := measurementIndependentSQLiteFixture(t, `CREATE TABLE measure_filter(id,name,type,source_arg_set_id); INSERT INTO measure_filter VALUES (1,'raw','vendor',NULL); CREATE TABLE measure(ts,dur,value,filter_id,type); INSERT INTO measure VALUES (2000000000,100000000,7,1,'vendor');`)
	ctx, prep, _ := hmc17NamedPathContext(t)
	ctx, _, _ = intervalSupplementContextForSource(t, ctx, prep.Prepare, path, 1, 2)
	native := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "measurements", "time_start": 1, "time_end": 2})
	payload := hmc17NamedPayload(t, native)
	if payload.Measurements == nil || payload.Measurements.Status != "available" || len(payload.Measurements.Rows) != 0 || payload.Measurements.UnpositionedRows != 0 {
		t.Fatalf("not a valid right-boundary empty response: %+v", payload.Measurements)
	}
	_, _, views, ok := ctx.Mutable.ResolveTraceIntervalNavigation(context.Background(), native.TraceQueryWindowReplay)
	if !ok || len(views) != 1 || views[0] != "measurements" {
		t.Fatalf("valid empty query lost native completion receipt: %v %s", views, native.Summary)
	}
	ctx.ToolResults = []types.ToolResult{native}
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 0 {
		t.Fatalf("valid empty native response repeated: %+v", out)
	}
}

func TestIntervalSupplementPublicProcessDefaultScope(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "default_all_owners_is_complete", true: "explicit_scope_is_not_dropped"}[explicit], func(t *testing.T) {
			path := measurementIndependentSQLiteFixture(t, `CREATE TABLE process(ipid,pid,name,start_ts,end_ts); INSERT INTO process VALUES (1,100,'sample',0,3000000000); CREATE TABLE process_measure_filter(id,name,ipid); INSERT INTO process_measure_filter VALUES (1,'Resident',1); CREATE TABLE process_measure(type,ts,dur,value,filter_id); INSERT INTO process_measure VALUES ('process_measure',1000000000,100000000,11,1);`)
			ctx, prep, _ := hmc17NamedPathContext(t)
			ctx, _, _ = intervalSupplementContextForSource(t, ctx, prep.Prepare, path, 1, 2)
			args := map[string]any{"source": "attached_trace", "view": "event_search", "time_start": 1, "time_end": 2}
			if explicit {
				args["target_scope"] = "process"
			}
			discovery := hmc17NamedQuery(t, ctx, args)
			ctx.ToolResults = []types.ToolResult{discovery}
			if !explicit {
				native := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "process_measurements", "time_start": 1, "time_end": 2})
				payload := hmc17NamedPayload(t, native)
				if payload.ProcessMeasurements == nil || payload.ProcessMeasurements.Status != "available" || payload.ProcessMeasurements.TotalRows != 1 {
					t.Fatalf("missing process-owned raw value: %+v", payload.ProcessMeasurements)
				}
				ctx.ToolResults = append(ctx.ToolResults, native)
			}
			if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 0 {
				t.Fatalf("repeated completed native query or removed explicit scope: %+v", out)
			}
		})
	}
}

func TestIntervalSupplementPublicCausalPriority(t *testing.T) {
	ctx := suppCoreContext(t)
	rm := &ctx.AnalysisIR.RequestModel
	rm.RuntimeArtifactScopeProfile = requestWindowTestProfile(3, 3.2)
	rm.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "worker"}
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}}
	rm.Intent, rm.PredicateAxis = types.IntentTrace, types.AxisCall
	rm.AnalyzerHints.Kind = string(types.ReqCallChain)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 2 || out.Executed[0] != "root_cause_rank" || out.Executed[1] != "critical_blocking_calls" {
		t.Fatalf("causal lane lost priority: %+v", out)
	}
}
