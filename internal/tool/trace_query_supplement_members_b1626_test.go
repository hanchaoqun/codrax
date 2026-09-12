package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The public runner, not a helper-false assertion: the model sampled A, while
// the accepted typed request asks for A and B on the same capture and target.
// A request member never denotes the enclosing interval or the last query.
func b1626SupplementMembers(t *testing.T, windows [][2]float64) *types.BusContext {
	t.Helper()
	ctx := suppCoreContext(t)
	members := make([]map[string]any, 0, len(windows))
	for _, w := range windows {
		members = append(members, map[string]any{"time_start": w[0], "time_end": w[1], "source_quote": fmt.Sprintf("%.6f..%.6f", w[0], w[1])})
	}
	raw, _ := json.Marshal(map[string]any{"requested_scope": "explicit_time_window", "time_windows": members})
	var scope types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal(raw, &scope); err != nil {
		t.Fatal(err)
	}
	rm := &ctx.AnalysisIR.RequestModel
	rm.RuntimeArtifactScopeProfile = &scope
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactRecordedReason}}
	return ctx
}

func b1626SupplementStateWindows(results []types.ToolResult) [][2]float64 {
	var out [][2]float64
	for _, result := range results {
		for _, record := range result.Observations {
			if record.Predicate != "target_window_states" || !traceSupplementTargetLabelMatches(record.Subject, traceQueryRequestTarget{PID: 200}) {
				continue
			}
			start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
			if ok {
				out = append(out, [2]float64{start, end})
			}
		}
	}
	return out
}

func TestB1626SupplementOnlyMissingMemberPublic(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
			windows := [][2]float64{a, b}
			if reverse {
				windows = [][2]float64{b, a}
			}
			ctx := b1626SupplementMembers(t, windows)
			suppCoreModelCall(t, ctx, `{"view":"window_stats","pid":200,"time_start":3,"time_end":3.035}`)
			before, _ := json.Marshal(ctx.ToolResults)
			request, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			out := RunTraceQuerySystemSupplement(ctx)
			if !reflect.DeepEqual(out.Executed, []string{"window_stats"}) || !reflect.DeepEqual(b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()), [][2]float64{b}) {
				for _, record := range suppCoreLedger(ctx).Records {
					if !record.SystemSupplement && record.Predicate == "target_window_states" {
						t.Logf("original state source: %+v", record.SourceRef)
					}
				}
				t.Fatalf("only missing B must be supplemented, never last/envelope: out=%+v states=%v", out, b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()))
			}
			ledger := suppCoreLedger(ctx)
			seen := map[[2]float64]bool{}
			for _, record := range ledger.Records {
				if record.Predicate == "target_window_states" {
					start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
					if ok {
						seen[[2]float64{start, end}] = true
					}
				}
			}
			if !seen[a] || !seen[b] {
				t.Fatalf("both original A and supplemented B must reach ledger: %v", seen)
			}
			stored, _ := json.Marshal(ctx.Mutable.SystemTraceSupplementResults())
			if again := RunTraceQuerySystemSupplement(ctx); again.Attempted {
				t.Fatalf("one task attempt only: %+v", again)
			}
			after, _ := json.Marshal(ctx.ToolResults)
			afterRequest, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			afterStored, _ := json.Marshal(ctx.Mutable.SystemTraceSupplementResults())
			if string(before) != string(after) || string(request) != string(afterRequest) || string(stored) != string(afterStored) {
				t.Fatal("supplement altered original inputs or second attempt replaced results")
			}
		})
	}
}

func TestB1626SupplementForeignScopeCannotSatisfyMemberPublic(t *testing.T) {
	for _, shape := range []string{"capture", "target", "line_filter", "missing_source", "unknown_lines", "unknown_window", "parent_window", "target_scope"} {
		t.Run(shape, func(t *testing.T) {
			a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
			ctx := b1626SupplementMembers(t, [][2]float64{a, b})
			params := map[string]any{"view": "window_stats", "pid": 200, "time_start": a[0], "time_end": a[1]}
			switch shape {
			case "capture":
				path := filepath.Join(t.TempDir(), "other.ftrace")
				if err := os.WriteFile(path, []byte(suppCoreTrace), 0600); err != nil {
					t.Fatal(err)
				}
				params["source"], params["path"] = "path", path
			case "target":
				params["pid"] = 300
			case "line_filter":
				params["line_start"], params["line_end"] = 1, 6
			}
			raw, _ := json.Marshal(params)
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("actual foreign query: %v %+v", err, result)
			}
			if shape == "missing_source" {
				for i := range result.Observations {
					result.Observations[i].SourceRef = types.ObservationSourceRef{}
				}
			}
			for i := range result.Observations {
				source := &result.Observations[i].SourceRef
				switch shape {
				case "unknown_lines":
					source.QueryLineRangeKnown = false
				case "unknown_window":
					source.QueryWindowKnown = false
				case "parent_window":
					source.QueryWindowStartTs, source.QueryWindowEndTs = 3, 3.2
				case "target_scope":
					source.QueryTargetScope = "process"
				}
			}
			ctx.ToolResults = append(ctx.ToolResults, result)
			out := RunTraceQuerySystemSupplement(ctx)
			got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults())
			if !reflect.DeepEqual(out.Executed, []string{"window_stats", "window_stats"}) || !reflect.DeepEqual(got, [][2]float64{a, b}) {
				t.Fatalf("foreign/unknown facts cannot waive either member: out=%+v states=%v", out, got)
			}
		})
	}
}

func TestB1626SupplementDuplicateMembersShareWorkPublic(t *testing.T) {
	a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
	ctx := b1626SupplementMembers(t, [][2]float64{a, a, b})
	before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	out := RunTraceQuerySystemSupplement(ctx)
	if got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()); !reflect.DeepEqual(got, [][2]float64{a, b}) {
		t.Fatalf("duplicate endpoint member must share work, not consume another budget: %+v %v", out, got)
	}
	after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(before) != string(after) {
		t.Fatal("work deduplication deleted a requested member")
	}
}

func TestB1626SupplementCancellationKeepsCompletedMemberPublic(t *testing.T) {
	a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
	ctx := b1626SupplementMembers(t, [][2]float64{a, b})
	dctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx.Ctx = dctx
	old := traceSupplementAfterViewHook
	traceSupplementAfterViewHook = func(string) { cancel() }
	t.Cleanup(func() { traceSupplementAfterViewHook = old })
	out := RunTraceQuerySystemSupplement(ctx)
	if got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()); !reflect.DeepEqual(got, [][2]float64{a}) {
		t.Fatalf("cancel B must preserve completed A: %+v %v", out, got)
	}
	if out.SkipReason != types.TraceSupplementReasonCanceledByCaller {
		t.Fatalf("caller cancel is not a duration overrun: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 2 || len(meta.MemberWindows[0].Views) != 1 || len(meta.MemberWindows[1].Views) != 0 || len(meta.MemberWindows[1].CanceledViews) != 0 || len(meta.MemberWindows[1].SkippedViews) != 1 || meta.MemberWindows[1].SkipReason != types.TraceSupplementReasonCanceledByCaller {
		t.Fatalf("B canceled before dispatch must keep its exact no-execution disclosure: %+v", meta)
	}
}

func TestB1626SupplementOneTotalDeadlinePublic(t *testing.T) {
	a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
	ctx := b1626SupplementMembers(t, [][2]float64{a, b})
	// Once A completes, wait on the actual caller deadline. B must share it,
	// not create another full allowance at the next member boundary.
	dctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1500*time.Millisecond))
	defer cancel()
	ctx.Ctx = dctx
	old := traceSupplementAfterViewHook
	traceSupplementAfterViewHook = func(string) { <-dctx.Done() }
	t.Cleanup(func() { traceSupplementAfterViewHook = old })
	out := RunTraceQuerySystemSupplement(ctx)
	if got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()); !reflect.DeepEqual(got, [][2]float64{a}) || out.SkipReason != types.TraceSupplementReasonDurationBudgetExceeded {
		t.Fatalf("one deadline keeps A, skips B without resetting: %+v %v", out, got)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 2 || meta.MemberWindows[1].SkipReason != types.TraceSupplementReasonDurationBudgetExceeded || len(meta.MemberWindows[1].Views) != 0 || len(meta.MemberWindows[1].CanceledViews) != 0 {
		t.Fatalf("undispatched B must disclose its member-level budget skip: %+v", meta)
	}
}

func TestB1626SupplementSpanBudgetAndOverlapPublic(t *testing.T) {
	for _, shape := range []string{"wide_second", "wide_first", "overlap", "nested"} {
		t.Run(shape, func(t *testing.T) {
			a, b := [2]float64{3, 3.035}, [2]float64{3.025, 3.08}
			want := [][2]float64{a, b}
			switch shape {
			case "wide_second":
				b = [2]float64{3, 130}
				want = [][2]float64{a}
			case "wide_first":
				a, b = [2]float64{3, 130}, a
				want = [][2]float64{b}
			case "nested":
				b = [2]float64{3.01, 3.03}
				want = [][2]float64{a, b}
			}
			ctx := b1626SupplementMembers(t, [][2]float64{a, b})
			out := RunTraceQuerySystemSupplement(ctx)
			if got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()); !reflect.DeepEqual(got, want) {
				t.Fatalf("members must never merge, clip, or let a wide skip swallow another member: %+v got=%v want=%v", out, got, want)
			}
		})
	}
}

func TestB1626SupplementMemberMetadataOwnCopyAndJSON(t *testing.T) {
	ctx := b1626SupplementMembers(t, [][2]float64{{3, 3.035}, {3.04, 3.08}})
	RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 2 || len(meta.Views) != 2 || meta.WindowStart != 0 || meta.WindowEnd != 0 {
		t.Fatalf("member presence, not a fake 0..0 window: %+v", meta)
	}
	before, _ := json.Marshal(meta)
	var restored types.SystemTraceSupplementMeta
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	other := types.NewMutableState("metadata snapshot")
	other.SetSystemTraceSupplement(restored, nil)
	for _, input := range []*types.SystemTraceSupplementMeta{meta, &restored} {
		input.MemberWindows[0].Views[0] = "mutated"
		input.MemberWindows[0].ViewValueObservations[0] = -1
		input.MemberWindows[0].ViewObservationFamilies[0].TargetStateRows = -1
		input.Views[0], input.ViewValueObservations[0] = "mutated", -1
	}
	for _, state := range []*types.MutableState{ctx.Mutable, other} {
		after, _ := json.Marshal(state.SystemTraceSupplementMeta())
		if string(before) != string(after) {
			t.Fatalf("meta setter/getter aliases member or total slices: before=%s after=%s", before, after)
		}
	}
}

func TestB1626SupplementCausalViewsPerMemberPublic(t *testing.T) {
	a, b := [2]float64{3, 3.2}, [2]float64{3.04, 3.08}
	ctx := b1626SupplementMembers(t, [][2]float64{a, b})
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
	suppCoreModelCall(t, ctx, `{"view":"root_cause_rank","pid":200,"time_start":3,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"critical_blocking_calls","pid":200,"time_start":3,"time_end":3.2}`)
	if f := traceSupplementFamilies(suppCoreLedger(ctx)); !f.Rank || !f.Chain || !f.WindowStates || !f.Critical || !f.BlockedReasonCensus || !f.WakeupEdgeCensus {
		t.Fatalf("actual A fixture must already contain all core families: %+v", f)
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if !reflect.DeepEqual(out.Executed, []string{"root_cause_rank", "critical_blocking_calls"}) {
		t.Fatalf("existing A core views cannot waive B nor be replayed themselves: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 2 || len(meta.MemberWindows[0].Views) != 0 || len(meta.MemberWindows[1].Views) != 2 {
		t.Fatalf("keep distinct A no-op/B actual core execution: %+v", meta)
	}
	for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
		for _, record := range result.Observations {
			if !record.SourceRef.QueryWindowKnown || record.SourceRef.QueryWindowStartTs != b[0] || record.SourceRef.QueryWindowEndTs != b[1] {
				t.Fatalf("native recursive row must retain its own B parent source: %+v", record.SourceRef)
			}
		}
	}
}

func TestB1626SupplementMemberExecutionFailureKeepsEarlierPublic(t *testing.T) {
	a, b := [2]float64{3, 3.035}, [2]float64{3.04, 3.08}
	ctx := b1626SupplementMembers(t, [][2]float64{a, b})
	old := traceSupplementAfterViewHook
	traceSupplementAfterViewHook = func(string) {
		path := filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename)
		if err := os.Rename(path, path+".unavailable"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { traceSupplementAfterViewHook = old })
	out := RunTraceQuerySystemSupplement(ctx)
	if got := b1626SupplementStateWindows(ctx.Mutable.SystemTraceSupplementResults()); !reflect.DeepEqual(got, [][2]float64{a}) || out.SkipReason != types.TraceSupplementReasonExecutionFailed {
		t.Fatalf("source unavailable for B cannot erase completed A: %+v %v", out, got)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 2 || meta.MemberWindows[1].SkipReason != types.TraceSupplementReasonExecutionFailed || len(meta.MemberWindows[1].Views) != 0 {
		t.Fatalf("failed B must not claim completion: %+v", meta)
	}
}

func TestB1626SupplementMissingTargetAndInvalidMembersDoNotFallback(t *testing.T) {
	for _, shape := range []string{"missing_target", "invalid_member"} {
		t.Run(shape, func(t *testing.T) {
			ctx := b1626SupplementMembers(t, [][2]float64{{3, 3.035}, {3.04, 3.08}})
			if shape == "missing_target" {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
			}
			suppCoreModelCall(t, ctx, `{"view":"event_search","pattern":"dma_fence","time_start":3,"time_end":3.2}`)
			if shape == "invalid_member" {
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeWindows[1].TimeEnd = nil
			}
			out := RunTraceQuerySystemSupplement(ctx)
			if len(out.Executed) != 0 || len(ctx.Mutable.SystemTraceSupplementResults()) != 0 {
				t.Fatalf("missing typed member/target cannot fall back to last or whole capture: %+v", out)
			}
		})
	}
}
