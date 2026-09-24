package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A missing completion and a missing time denominator are different facts.
// Exercise native parsing/publication, not a hand-made provider table, so the
// renderer cannot turn either kind of unavailable measurement into zero.
func TestRuntimeMeasurementPublicUnpairedIOWindowAvailability(t *testing.T) {
	for _, lineOnly := range []bool{false, true} {
		name := "known_window_no_complete_pairs"
		if lineOnly {
			name = "line_only_no_time_denominator"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "unpaired.systrace")
			const body = "reader-11 (11) [000] .... 1.200000: block_rq_issue: 8,0 R 4096 () 8 + 8 [reader]\n"
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			params := map[string]any{"source": "path", "path": path, "view": "window_stats"}
			if lineOnly {
				params["line_start"], params["line_end"] = 1, 1
			} else {
				params["time_start"], params["time_end"] = 1, 2
			}
			mu := types.NewMutableState("Describe the observed IO measurements")
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: mu}
			raw, _ := json.Marshal(params)
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("native query failed: %v / %s", err, result.Summary)
			}
			var row *types.ObservationRecord
			for i := range result.Observations {
				if result.Observations[i].Predicate == "io_inflight" {
					if row != nil {
						t.Fatal("single request unexpectedly produced multiple IO groups")
					}
					row = &result.Observations[i]
				}
			}
			if row == nil {
				t.Fatal("native unpaired-start group was omitted")
			}
			data, err := os.ReadFile(row.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var native tracequery.Result
			if err := json.Unmarshal(data, &native); err != nil {
				t.Fatal(err)
			}
			if native.WindowStats == nil || native.WindowStats.IOInFlight == nil || len(native.WindowStats.IOInFlight.Groups) != 1 {
				t.Fatal("native IO prerequisite missing")
			}
			stats := native.WindowStats.IOInFlight
			group := stats.Groups[0]
			if group.Values != nil || group.AcceptedPairCount != 0 || group.IssueCount != 1 || len(group.Members) != 0 {
				t.Fatalf("missing completion became a complete pair or measured zero: %+v", group)
			}
			if lineOnly {
				if stats.Window != nil || stats.WindowUnavailableReason != "line_bounds_take_precedence" || group.ValuesUnavailableReason != stats.WindowUnavailableReason {
					t.Fatalf("line-selected population acquired a time denominator: %+v", stats)
				}
			} else if stats.Window == nil || stats.Window.StartTs != 1 || stats.Window.EndTs != 2 || group.ValuesUnavailableReason != "no_accepted_complete_pairs" {
				t.Fatalf("known window/no-pair distinction lost: %+v", stats)
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(*row); !ok {
				t.Fatal("native provider publication failed exact source binding")
			}
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
			receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: row.ID, View: types.RuntimeMeasurementSummary}
			if !types.BindRuntimeMeasurementReceipt(receipt, contract) {
				t.Fatal("native summary was not selectable")
			}
			table := receipt.BoundTable
			if len(table.Rows) != 1 || len(table.Rows[0]) != 6 {
				t.Fatalf("unexpected summary shape: %+v", table)
			}
			for _, cell := range table.Rows[0][:4] {
				if cell != "unavailable" {
					t.Fatalf("unknown concurrent/time measure rendered as %q", cell)
				}
			}
			if table.Rows[0][4] != "0" || table.Rows[0][5] != "1" {
				t.Fatal("exact complete-pair/arrival counts were lost with unavailable durations")
			}
			visible := render.RenderAnswerDocument(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "io", Kind: types.BlockTable, RuntimeMeasurement: receipt}}}, "en")
			if !strings.Contains(visible, "| unavailable | unavailable | unavailable | unavailable | 0 | 1 |") {
				t.Fatalf("visible table conflated unavailable measures and exact counts: %s", visible)
			}
			if lineOnly {
				for _, want := range []string{"Continuous time window unavailable (not a measured zero)", "No continuous time denominator is established"} {
					if !strings.Contains(visible, want) {
						t.Fatalf("line-only disclosure missing %q: %s", want, visible)
					}
				}
				if strings.Contains(visible, "The time window is known") {
					t.Fatal("line-only query claimed a measured continuous window")
				}
			} else {
				for _, want := range []string{"Window &#91;1.000000, 2.000000) seconds", "The time window is known, but no usable complete-pair measurements", "unavailable is not measured zero"} {
					if !strings.Contains(visible, want) {
						t.Fatalf("known-window/no-pair disclosure missing %q: %s", want, visible)
					}
				}
				if strings.Contains(visible, "No continuous time denominator") || strings.Contains(visible, "Continuous time window unavailable") {
					t.Fatal("missing completion incorrectly taught that the known denominator is unavailable")
				}
			}
		})
	}
}
