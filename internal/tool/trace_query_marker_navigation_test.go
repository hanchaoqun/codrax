package tool

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func markerNavigationCursorContext(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer("OpenDocument", "DispatchWork", "LoadDocumentIndex", "PrepareRecord").Replace(string(fixture))
	ctx, path := businessRefTestContext(t, body)
	ctx.AnalysisIR = &types.AnalysisIR{}
	ctx.Mutable.SetRequestModel(types.RequestModel{})
	seed := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 100, "time_start": 1, "time_end": 1.051})
	if !seed.Success {
		t.Fatal(seed.Summary)
	}
	for _, rm := range []*types.RequestModel{&ctx.AnalysisIR.RequestModel, ctx.Mutable.RequestModel()} {
		if rm == nil || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 100 || !types.RuntimeTargetIsExplorationCursorSource(rm.RuntimeTargets[0].Source) {
			t.Fatalf("public window_stats did not seed only the exact exploration cursor: %+v", rm)
		}
	}
	return ctx, path
}

func markerNavigationPayload(t *testing.T, result types.ToolResult) tracequery.Result {
	t.Helper()
	if !result.Success {
		t.Fatal(result.Summary)
	}
	data, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestTraceQueryMarkerNavigationPublicDoesNotInheritCursor(t *testing.T) {
	for _, mode := range []string{"named_span", "trace_mark", "canonical_alias_and_action", "inactive_focus"} {
		t.Run(mode, func(t *testing.T) {
			ctx, path := markerNavigationCursorContext(t)
			if mode == "inactive_focus" {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets, types.RuntimeTarget{Source: "user_explicit"})
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			}
			before, _ := json.Marshal([]any{ctx.AnalysisIR.RequestModel, ctx.Mutable.RequestModel()})
			params := map[string]any{"path": path, "view": "span_window", "span_name": "PrepareRecord", "time_start": 1.004, "time_end": 1.045, "line_start": 7, "line_end": 14}
			if mode != "named_span" && mode != "inactive_focus" {
				delete(params, "span_name")
				params["view"], params["event_types"], params["pattern"] = "event_search", []string{"trace_mark"}, "PrepareRecord"
			}
			if mode == "canonical_alias_and_action" {
				delete(params, "pattern")
				params["event_types"], params["trace_mark_actions"], params["patterns"] = []string{"print", "traceMark"}, []string{"B"}, []string{"PrepareRecord"}
			}
			result := businessRefTestQuery(t, ctx, params)
			payload := markerNavigationPayload(t, result)
			if mode == "named_span" || mode == "inactive_focus" {
				if len(payload.SpanWindows) != 1 || payload.SpanWindows[0].Thread.PID != 200 || payload.SpanWindows[0].SpanPID != 100 ||
					payload.SpanWindows[0].Name != "PrepareRecord" || payload.SpanWindows[0].StartTs != 1.0045 || payload.SpanWindows[0].EndTs != 1.0445 {
					t.Errorf("discovery borrowed cursor TID or marker payload PID: %+v", payload.SpanWindows)
				}
			} else if len(payload.Events) != 1 || payload.Events[0].PID != 200 || payload.Events[0].SpanPID != 100 || payload.Events[0].SpanName != "PrepareRecord" {
				t.Errorf("structured marker inventory lost the other emitter: %+v", payload.Events)
			}
			for _, want := range []string{"pid= target_scope=thread", "time_start=1.004000 time_end=1.045000", "line_start=7 line_end=14", "trace_query_target_inheritance_skipped=marker_navigation"} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("public navigation lost %q", want)
				}
			}
			after, _ := json.Marshal([]any{ctx.AnalysisIR.RequestModel, ctx.Mutable.RequestModel()})
			if string(before) != string(after) {
				t.Fatal("discovery cleared the cursor or elected a new focus")
			}
		})
	}
}

func TestTraceQueryMarkerNavigationDescriptionKeepsScopeBoundaries(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"except cursor-only marker discovery (span_window with nonempty span_name or event_search with only normalized trace_mark types)",
		"neither request-model copy has an active non-cursor target",
		"explicit selectors, user/unknown-source focus, process scope and system supplementation retain their scopes",
	} {
		if strings.Count(description, want) != 1 {
			t.Errorf("inheritance teaching must carry exactly one scoped exception %q", want)
		}
	}
}

func TestTraceQueryMarkerNavigationPublicPreservesFocusAndExplicitSelectors(t *testing.T) {
	for _, mode := range []string{"user_focus", "unknown_source", "blank_source", "mixed_same_identity", "mutable_focus", "analysis_focus", "explicit_tid", "explicit_thread", "explicit_process", "process_without_pid", "system_supplement"} {
		t.Run(mode, func(t *testing.T) {
			ctx, path := markerNavigationCursorContext(t)
			focus := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}
			switch mode {
			case "user_focus", "unknown_source", "blank_source":
				if mode == "unknown_source" {
					focus.Source = "legacy_unknown"
				} else if mode == "blank_source" {
					focus.Source = ""
				}
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{focus}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			case "mixed_same_identity":
				// The existing cardinality map keeps the last entry's source.
				// A same-identity cursor must not erase the earlier user focus.
				ctx.AnalysisIR.RequestModel.RuntimeTargets = append([]types.RuntimeTarget{focus}, ctx.AnalysisIR.RequestModel.RuntimeTargets...)
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			case "mutable_focus":
				ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeTargets: []types.RuntimeTarget{focus}})
			case "analysis_focus":
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{focus, ctx.AnalysisIR.RequestModel.RuntimeTargets[0]}
			case "system_supplement":
				ctx.Mutable.BeginSystemTraceSupplementExecution()
				defer ctx.Mutable.EndSystemTraceSupplementExecution()
			}
			params := map[string]any{"path": path, "view": "span_window", "span_name": "PrepareRecord", "time_start": 1.004, "time_end": 1.045}
			switch mode {
			case "explicit_tid":
				params["pid"] = 100
			case "explicit_thread":
				params["thread"] = "app-main-100"
			case "explicit_process":
				params["pid"], params["target_scope"] = 100, "process"
			case "process_without_pid":
				params["target_scope"] = "process"
			}
			result := businessRefTestQuery(t, ctx, params)
			if mode == "process_without_pid" {
				// Execute already requires an explicit process ID before the
				// inheritance seam; navigation must not bypass that admission.
				if result.Success || !strings.Contains(result.Summary, "explicit positive pid=<process_id>") {
					t.Fatal("unscoped explicit process query was widened")
				}
				return
			}
			payload := markerNavigationPayload(t, result)
			if mode == "explicit_process" {
				if len(payload.SpanWindows) != 1 || payload.SpanWindows[0].Thread.PID != 200 || payload.SpanWindows[0].SpanPID != 100 {
					t.Fatal("explicit process membership no longer discovers its exact member")
				}
			} else if len(payload.SpanWindows) != 0 {
				t.Fatal("user focus, explicit selector or system supplement was widened")
			}
			if strings.Contains(result.Summary, "trace_query_target_inheritance_skipped=marker_navigation") {
				t.Fatal("protected focus/selector/supplement skipped inheritance")
			}
		})
	}
}

func TestTraceQueryMarkerNavigationPublicKeepsOtherInheritance(t *testing.T) {
	for _, mode := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "unnamed_span", "mixed_types", "no_types", "action_without_types", "unknown_type"} {
		t.Run(mode, func(t *testing.T) {
			ctx, path := markerNavigationCursorContext(t)
			params := map[string]any{"path": path, "view": mode, "time_start": 1, "time_end": 1.051}
			switch mode {
			case "unnamed_span":
				params["view"] = "span_window"
			case "mixed_types", "no_types", "action_without_types", "unknown_type":
				params["view"] = "event_search"
				if mode == "mixed_types" {
					params["event_types"] = []string{"trace_mark", "sched_switch"}
				} else if mode == "action_without_types" {
					params["trace_mark_actions"] = []string{"B"}
				} else if mode == "unknown_type" {
					params["event_types"] = []string{"trace_mark", "invented"}
				}
			}
			result := businessRefTestQuery(t, ctx, params)
			if !result.Success || !strings.Contains(result.Summary, "pid=100 target_scope=thread") || strings.Contains(result.Summary, "trace_query_target_inheritance_skipped=marker_navigation") {
				t.Fatalf("unrelated view/type family lost its exact cursor: %s", result.Summary)
			}
		})
	}
}
