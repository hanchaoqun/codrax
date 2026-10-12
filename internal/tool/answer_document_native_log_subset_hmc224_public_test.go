package tool

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reproduce the real all-sources query followed by per-source confirmation.
// Presentation can hide redundant defaults, never the original selectors.
func TestHMC224NativeLogSubsetPublicEmitPatchRender(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}} {
		var inputs []loginput.Input
		for _, relative := range []string{"app/session.log", "kernel/session.log.gz"} {
			path, _ := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/" + relative)
			inputs = append(inputs, loginput.Input{Path: path})
		}
		bus, _ := logQueryPublicBus(t, inputs)
		rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionMemberSet)
		rm.LogTriage = &types.LogBundle{}
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
		bus.Mutable.SetRequestModel(rm)
		sources := bus.AttachedLogCatalog.Sources()
		queries := []map[string]any{{"limit": 50}, {"source_ids": []string{sources[0].ID}, "limit": 50}, {"source_ids": []string{sources[1].ID}, "limit": 50}}
		var results []types.ToolResult
		for _, i := range order {
			args, _ := json.Marshal(queries[i])
			result, err := (&LogQuery{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("query: %v / %s", err, result.Summary)
			}
			results = append(results, result)
		}
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
		ledger := func() []byte {
			raw, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			return raw
		}
		before := ledger()
		lead := map[string]any{"id": "lead", "kind": "summary", "text": "原始记录的来源和时钟保持独立；不推断因果。"}
		if out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead}}, false); !out.Success {
			t.Fatal(out.Summary)
		}
		doc := bus.Mutable.AnswerDocumentV2()
		rows, tables := 0, 0
		var systemIDs []string
		for _, block := range doc.Blocks {
			if block.RuntimeMeasurement != nil {
				rows += len(block.RuntimeMeasurement.BoundTable.Rows)
				tables++
				systemIDs = append(systemIDs, block.ID)
				if block.RuntimeMeasurement.BoundTable.MemberSet != nil {
					t.Fatal("display acquired member-completion authority")
				}
			}
		}
		if rows != 9 || tables != 1 {
			t.Fatalf("order %v: default display kept %d rows / %d tables; want 9 / 1", order, rows, tables)
		}
		visible := render.RenderAnswerDocument(doc, "zh")
		for _, text := range []string{"9007199254741001", "99-09", "orphan stack detail, owner not established"} {
			if strings.Count(visible, text) != 1 {
				t.Fatalf("native field %q missing or duplicated", text)
			}
		}
		if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{lead}, "unchanged_block_ids": systemIDs}, true); !out.Success {
			t.Fatal(out.Summary)
		}
		if render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh") != visible {
			t.Fatal("patch reintroduced duplicate defaults")
		}
		contract := types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract
		choices := contract.Choices()
		if len(choices) != 3 {
			t.Fatalf("suppression removed original selectors: %d", len(choices))
		}
		blocks := []any{lead}
		for i, choice := range choices {
			blocks = append(blocks, map[string]any{"id": []string{"all", "app", "kernel"}[i], "kind": "table", "runtime_measurement": map[string]any{"observation_id": choice.ObservationID, "view": choice.View}})
		}
		if out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false); !out.Success {
			t.Fatal(out.Summary)
		}
		rows, tables = 0, 0
		for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
			if block.RuntimeMeasurement != nil {
				rows += len(block.RuntimeMeasurement.BoundTable.Rows)
				tables++
			}
		}
		if rows != 18 || tables != 3 || !bytes.Equal(before, ledger()) || bus.Mutable.TraceRootCauseReport() != nil {
			t.Fatal("explicit selectors, raw facts or causal boundary changed")
		}
	}
}
