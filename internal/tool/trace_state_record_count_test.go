package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Distinct public views publish independent state records for the same two
// requested windows. Record count must not be presented as distinct-window
// count; neither authority deduplication nor the four-row preview is changed.
func TestTraceStateRecordCountPublicTwoWindows(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, count := range []int{4, 5, 11} {
			t.Run(fmt.Sprintf("%s/%d_records", lang, count), func(t *testing.T) {
				// This helper calls EmitAnalysis.Execute, retaining both ordered
				// explicit windows and the user-explicit worker target.
				ctx := b1626PublicMemberBus(t, "bounded_fact_set")
				ctx.Language = lang
				ctx.AnalysisIR.AnswerContract.Language = lang
				views := []string{"window_stats", "thread_timeline", "root_cause_rank", "scheduler_latency_stats", "critical_blocking_calls", "wakeup_chain"}
				windows := [][2]float64{{3, 3.035}, {3.04, 3.08}}
				for member, window := range windows {
					n := count / 2
					if member == 0 {
						n = (count + 1) / 2
					}
					// Issue all A queries before B: the public display must still
					// retain B ahead of repeated A records, without merging them.
					for _, view := range views[:n] {
						raw, err := json.Marshal(map[string]any{"view": view, "pid": 200, "time_start": window[0], "time_end": window[1]})
						if err != nil {
							t.Fatal(err)
						}
						suppCoreModelCall(t, ctx, string(raw))
					}
				}
				ledger := suppCoreLedger(ctx)
				states := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
				if len(states) != count {
					t.Fatalf("public queries must mint %d independent state records, got %d: %+v", count, len(states), states)
				}
				seenWindows := map[[2]float64]int{}
				seenIDs := map[string]bool{}
				seenArtifacts := map[string]bool{}
				for _, state := range states {
					key := [2]float64{state.WindowStartTs, state.WindowEndTs}
					if key != windows[0] && key != windows[1] {
						t.Fatalf("unexpected native query window: %+v", state)
					}
					seenWindows[key]++
					seenArtifacts[state.ArtifactKey] = true
					if state.EvidenceID == "" || seenIDs[state.EvidenceID] || state.ArtifactKey == "" || state.Subject != "worker-200" {
						t.Fatalf("records must retain distinct native evidence for the same capture/target: %+v", state)
					}
					seenIDs[state.EvidenceID] = true
					if state.WindowScope.RequestedWindowCount != 2 || state.WindowScope.Role != types.TraceQueryWindowScopeRequestedPrincipal {
						t.Fatalf("public record lost requested-member authority: %+v", state)
					}
				}
				if len(seenWindows) != 2 || len(seenArtifacts) != 1 || seenWindows[windows[0]] != (count+1)/2 || seenWindows[windows[1]] != count/2 {
					t.Fatalf("expected one capture, %d records and only two windows: artifacts=%v windows=%v", count, seenArtifacts, seenWindows)
				}
				t.Logf("public producer: state_records=%d distinct_windows=%d A_records=%d B_records=%d", len(states), len(seenWindows), seenWindows[windows[0]], seenWindows[windows[1]])
				beforeResults, _ := json.Marshal(ctx.ToolResults)
				beforeLedger, _ := json.Marshal(ledger)
				beforeStates, _ := json.Marshal(states)
				const modelText = "Model-authored comparison remains unchanged."
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: modelText}}}
				wire, err := modelOwnedAnswerBlockWire(doc)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]any{"blocks": doc.Blocks})
				result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Unix(1, 0))
				if err != nil || !result.Success {
					t.Fatalf("public answer emission failed: %v %+v", err, result)
				}
				stored := ctx.Mutable.AnswerDocumentV2()
				if err := requireModelOwnedAnswerBlockWirePreserved(wire, stored); err != nil {
					t.Fatal(err)
				}
				if ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("bounded state comparison acquired a root-cause report")
				}
				block := answerDocumentTestBlockByID(t, stored, runtimeTraceTargetStateAuthorityBlockID)
				text := render.RenderAnswerDocument(stored, lang)
				if !strings.Contains(text, block.Text) || !strings.Contains(text, modelText) {
					t.Fatal("actual renderer lost the system state card or model prose")
				}
				prefix := "Target-thread state:"
				if lang == "zh" {
					prefix = "目标线程状态："
				}
				var rows []string
				for _, paragraph := range strings.Split(block.Text, "\n\n") {
					if strings.HasPrefix(paragraph, prefix) {
						rows = append(rows, paragraph)
					}
				}
				if len(rows) != 4 {
					t.Fatalf("four-row state preview changed: got %d in %s", len(rows), block.Text)
				}
				ordered := runtimeTraceMemberStateDisplayOrder(states)
				for i, row := range rows {
					state := ordered[i]
					for _, want := range []string{
						state.Subject,
						types.FormatTraceRuntimeAccountWindow(state.WindowStartTs, state.WindowEndTs, lang),
						fmt.Sprintf("running %.3fms", state.RunningMS),
						fmt.Sprintf("runnable %.3fms", state.RunnableMS),
						fmt.Sprintf("sleep %.3fms", state.SleepMS),
						fmt.Sprintf("%s %.3fms", TraceStateNonIODStateWord(lang == "zh"), state.DStateMS),
						fmt.Sprintf("io_wait %.3fms", state.IOWaitMS),
						fmt.Sprintf("%.3fms", state.TotalMS),
						fmt.Sprintf("%.3fms", state.WindowMS),
					} {
						if !strings.Contains(row, want) {
							t.Errorf("preview row %d lost its native value/scope %q: %s", i, want, row)
						}
					}
				}
				for i, window := range windows {
					if !strings.Contains(rows[i], types.FormatTraceRuntimeAccountWindow(window[0], window[1], lang)) {
						t.Errorf("A and B must precede repeated records: row %d=%s", i, rows[i])
					}
				}
				afterLedger := suppCoreLedger(ctx)
				afterResults, _ := json.Marshal(ctx.ToolResults)
				afterLedgerJSON, _ := json.Marshal(afterLedger)
				afterStates, _ := json.Marshal(types.BuildTraceTargetStateScopeAuthoritiesFromLedger(afterLedger))
				if !bytes.Equal(beforeResults, afterResults) || !bytes.Equal(beforeLedger, afterLedgerJSON) || !bytes.Equal(beforeStates, afterStates) {
					t.Fatal("publication changed native tool results, ledger or state authority/order")
				}
				if strings.Contains(block.Text, "独立范围的状态统计") || strings.Contains(block.Text, "independently scoped state accounts") {
					t.Error("record count is mislabeled as independent-window count")
				}
				if count == 4 {
					if strings.Contains(block.Text, "条状态统计记录未在此展开") || strings.Contains(block.Text, "additional state-statistics records") || strings.Contains(block.Text, "均已完成分析") || strings.Contains(block.Text, "complete analysis of all requested windows") {
						t.Error("an exactly full preview must not invent omitted records")
					}
					return
				}
				want := fmt.Sprintf("%d additional state-statistics records are not expanded here; the record count is not the number of distinct time windows, and this display does not establish complete analysis of all requested windows.", count-4)
				if lang == "zh" {
					want = fmt.Sprintf("另有 %d 条状态统计记录未在此展开；记录条数不代表不同时间窗的数量，以上展示不表示所有时间窗均已完成分析。", count-4)
				}
				if strings.Count(block.Text, want) != 1 || strings.Count(text, want) != 1 {
					t.Errorf("actual rendered omission must describe %d records, not extra windows; want %q in %s", count-4, want, block.Text)
				}
			})
		}
	}
}
