package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func wakeInventoryContext270(t *testing.T) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	var text strings.Builder
	for n := 0; n < 8; n++ {
		ts := 3 + float64(n)/100
		fmt.Fprintf(&text, "target-3051 (1182) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=3051 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", ts)
		waker := 1021
		if n >= 5 {
			waker = 20975
		}
		fmt.Fprintf(&text, "same-comm-%d (908) [001] .... %.6f: sched_wakeup: comm=target pid=3051 prio=120 target_cpu=0\n", waker, ts+.005)
		fmt.Fprintf(&text, "target-3051 (1182) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=3051 next_prio=120\n", ts+.006)
	}
	text.WriteString("target-3051 (1182) [000] .... 3.081000: sched_wakeup: comm=other pid=444 prio=120 target_cpu=0\n")
	text.WriteString("same-comm-999 (909) [000] .... 3.082000: sched_wakeup: comm=target pid=9999 prio=120 target_cpu=0\n")
	text.WriteString("same-comm-20975 (908) [000] .... 3.090000: sched_wakeup: comm=target pid=3051 prio=120 target_cpu=0\n")
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(text.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 3.0, 3.09
	rm := types.RequestModel{
		Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 3051, Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactDirectWaker}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "3.0–3.09"},
	}
	mutable := types.NewMutableState("列出目标线程在 3.0–3.09 内的状态和直接唤醒记录")
	mutable.SetRequestModel(rm)
	return &types.BusContext{Language: "zh", RepoRoot: dir, WorkDir: dir, Mutable: mutable,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
}

func wakeInventoryTable270(t *testing.T, ctx *types.BusContext) types.RuntimeMeasurementTable {
	t.Helper()
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	if contract != nil {
		for _, table := range contract.Tables {
			if table.EventData != nil {
				return table
			}
		}
	}
	t.Fatal("complete wake-event inventory is absent; a causal subset/census cannot replace it")
	return types.RuntimeMeasurementTable{}
}

func TestHMC270PublicWakeInventoryIndependentOfCausalSubset(t *testing.T) {
	ctx := wakeInventoryContext270(t)
	chain := suppCoreModelCall(t, ctx, `{"view":"wakeup_chain","pid":3051,"time_start":3,"time_end":3.09,"max_branches":6,"max_depth":2,"min_duration_ms":0.1}`)
	edges := 0
	for _, record := range chain.Observations {
		if record.Predicate == "wakeup_chain_edge" {
			edges++
		}
	}
	if edges != 6 {
		t.Fatalf("fixture requires a causal subset of six edges, got %d", edges)
	}
	suppCoreModelCall(t, ctx, `{"view":"thread_timeline","pid":3051,"time_start":3,"time_end":3.09}`)
	chainBefore, _ := json.Marshal(chain)
	out := RunTraceQuerySystemSupplement(ctx)
	if !strings.Contains(strings.Join(out.Executed, ","), "event_search") || len(out.Executed) > 2 {
		t.Fatalf("complete state/chain evidence still needs an independent event inventory: %+v", out)
	}
	table := wakeInventoryTable270(t, ctx)
	if len(table.EventData.Rows) != 8 {
		t.Fatalf("receiver inventory=%d, want 8 (not emitter events, same-name targets, end boundary or causal subset)", len(table.EventData.Rows))
	}
	for _, kind := range []string{"summary", "table", "diagram"} {
		blocks := []any{eventBlock236("wake-events", kind, table.ObservationID, map[string]any{
			"field_keys": []string{"event.trace_time_seconds", "emitter.tid", "emitter.tgid", "scheduler.wakee.tid"},
		})}
		if kind != "summary" {
			blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "以下为原始唤醒事件记录，不以因果链子集代替枚举。"}}, blocks...)
		}
		result := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false)
		if !result.Success {
			t.Fatalf("%s rejected: %s", kind, result.Summary)
		}
		block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "wake-events")
		if len(block.RuntimeMeasurement.BoundTable.Rows) != 8 {
			t.Fatalf("%s changed population", kind)
		}
		for n, row := range block.RuntimeMeasurement.BoundTable.Rows {
			waker := "1021"
			if n >= 5 {
				waker = "20975"
			}
			if row[1] != waker || row[2] != "908" || row[3] != "3051" {
				t.Fatalf("%s lost thread/process/direction identity: %v", kind, row)
			}
		}
		visible := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh")
		for _, timestamp := range []string{"3.055", "3.065", "3.075"} {
			if !strings.Contains(visible, timestamp) {
				t.Fatalf("%s omitted %s", kind, timestamp)
			}
		}
		if kind == "diagram" && strings.Contains(visible, "-->") {
			t.Fatal("finite wake-event facts minted a causal edge")
		}
	}
	after, _ := json.Marshal(ctx.ToolResults[0])
	if string(chainBefore) != string(after) || len(ctx.ToolResults) != 2 {
		t.Fatal("supplement changed original causal result or model call history")
	}
}

func TestHMC270PublicWakeInventoryReceiverNotGenericPID(t *testing.T) {
	ctx := wakeInventoryContext270(t)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":3051,"event_types":["sched_wakeup"],"time_start":3,"time_end":3.09}`)
	table := wakeInventoryTable270(t, ctx)
	if table.EventData.Coverage.MatchedTotal <= 8 {
		t.Fatal("fixture must retain the wider generic participation-query population")
	}
	if len(table.EventData.Rows) != 8 {
		t.Fatalf("direct_waker display confused sender participation with receiver: %d rows", len(table.EventData.Rows))
	}
}

func TestHMC270PublicWakeInventoryCurrentSourceAndWindow(t *testing.T) {
	for _, mode := range []string{"complete_current", "other_capture", "narrow_window", "serialized", "display_cap"} {
		t.Run(mode, func(t *testing.T) {
			ctx := wakeInventoryContext270(t)
			params := map[string]any{"view": "event_search", "pid": 3051, "event_types": []string{"sched_wakeup"}, "time_start": 3.0, "time_end": 3.09}
			if mode == "other_capture" {
				bytes, err := os.ReadFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename))
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(ctx.WorkDir, "other.systrace")
				if err := os.WriteFile(path, bytes, 0600); err != nil {
					t.Fatal(err)
				}
				params["source"], params["path"] = "path", path
			}
			if mode == "narrow_window" {
				params["time_end"] = 3.05
			}
			if mode == "display_cap" {
				params["limit"] = 2
			}
			raw, _ := json.Marshal(params)
			result := suppCoreModelCall(t, ctx, string(raw))
			if mode == "serialized" {
				raw, _ = json.Marshal(result)
				var restored types.ToolResult
				if err := json.Unmarshal(raw, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.ToolResults = []types.ToolResult{restored}
				// A current private replay may legitimately recognize an exact
				// copy. This negative is restoration without that native owner.
				ctx.Mutable = types.NewMutableState("restored history")
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			}
			out := RunTraceQuerySystemSupplement(ctx)
			ran := strings.Contains(strings.Join(out.Executed, ","), "event_search")
			want := mode != "complete_current" && mode != "display_cap"
			if ran != want || len(out.Executed) > 2 {
				t.Fatalf("%s inventory suppression=%+v, want event execution=%t", mode, out, want)
			}
			if mode == "display_cap" {
				table := wakeInventoryTable270(t, ctx)
				if table.EventData.RowsComplete || len(table.EventData.Rows) != 2 || table.EventData.Coverage.MatchedTotal <= 2 {
					t.Fatal("preview completeness was promoted or same capped query repeated")
				}
			}
		})
	}
}

func TestHMC270PublicWakeInventoryDisplayBoundaries(t *testing.T) {
	for _, mode := range []string{"name_only", "pid_beats_name", "state_only", "causal", "idle_emitter"} {
		t.Run(mode, func(t *testing.T) {
			ctx := wakeInventoryContext270(t)
			if mode == "idle_emitter" {
				path := filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename)
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				raw = []byte(strings.Replace(string(raw), "same-comm-1021 (908)", "idle-0 (0)", 1))
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			suppCoreModelCall(t, ctx, `{"view":"event_search","pid":3051,"event_types":["sched_wakeup"],"time_start":3,"time_end":3.09}`)
			rm := &ctx.AnalysisIR.RequestModel
			want := 8
			switch mode {
			case "name_only":
				rm.RuntimeTargets[0].PID, rm.RuntimeTargets[0].Thread = 0, "target"
			case "pid_beats_name":
				rm.RuntimeTargets[0].Thread = "other"
			case "state_only":
				rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}
				want = 9
			case "causal":
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
				want = 9 // Existing explicit-window display still excludes its end boundary.
			}
			ctx.Mutable.SetRequestModel(*rm)
			table := wakeInventoryTable270(t, ctx)
			if len(table.EventData.Rows) != want {
				t.Fatalf("%s rows=%d want=%d", mode, len(table.EventData.Rows), want)
			}
		})
	}
}

func TestHMC270PublicWakeInventoryPerRequestedWindow(t *testing.T) {
	ctx := wakeInventoryContext270(t)
	var scope types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","time_windows":[{"time_start":3,"time_end":3.04,"source_quote":"3–3.04"},{"time_start":3.04,"time_end":3.09,"source_quote":"3.04–3.09"}]}`), &scope); err != nil {
		t.Fatal(err)
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &scope
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	out := RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if strings.Count(strings.Join(out.Executed, ","), "event_search") != 2 || len(out.Executed) > 4 || len(meta.MemberWindows) != 2 {
		t.Fatalf("requested members did not retain separate bounded execution: %+v %+v", out, meta)
	}
	for _, member := range meta.MemberWindows {
		if len(member.Views) > 2 {
			t.Fatal("inventory exceeded the existing per-member two-view budget")
		}
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	counts := []int{}
	for _, table := range contract.Tables {
		if table.EventData != nil {
			counts = append(counts, len(table.EventData.Rows))
		}
	}
	if len(counts) != 2 || counts[0] != 4 || counts[1] != 4 {
		t.Fatalf("member populations borrowed another window: %v", counts)
	}
}
