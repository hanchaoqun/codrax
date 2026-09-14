package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise both public tools: the producer publishes several separated
// occurrences, while the answer projection carries only their start/end
// envelope. The renderer must not upgrade that envelope to one occurrence.
func TestB1688PublicTraceQueryAnswerEnvelope(t *testing.T) {
	path, err := filepath.Abs(elimSemanticTiebaTrace)
	if err != nil {
		t.Fatal(err)
	}
	queryBus := &types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir(), Mutable: types.NewMutableState("b1688-query")}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "frame_root_cause_bundle",
		"pid": 59566, "time_start": 34579.472865, "time_end": 34579.587805,
		"trace_flavor": "harmony_hitrace", "platform": "donghu",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(queryBus, params)
	if err != nil || !result.Success {
		t.Fatalf("real trace_query failed: %v %s", err, result.Summary)
	}
	var payloadRef string
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef != "" {
			payloadRef = observation.SourceRef.PayloadRef
			break
		}
	}
	payload, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatalf("read real producer payload %q: %v", payloadRef, err)
	}
	var produced tracequery.Result
	if err := json.Unmarshal(payload, &produced); err != nil {
		t.Fatal(err)
	}
	if produced.FrameRootCauseBundle == nil || produced.FrameRootCauseBundle.RootCauseRank == nil {
		t.Fatal("fixture must publish the real root-cause board")
	}
	var ranked *tracequery.RootCauseRankItem
	for i := range produced.FrameRootCauseBundle.RootCauseRank.Items {
		item := &produced.FrameRootCauseBundle.RootCauseRank.Items[i]
		if item.Rank == 1 {
			ranked = item
			break
		}
	}
	if ranked == nil || len(ranked.OccurrenceWindows) < 2 {
		t.Fatalf("fixture must provide multiple occurrences, got %+v", ranked)
	}
	hasGap := false
	for i, occurrence := range ranked.OccurrenceWindows {
		if occurrence.Window.EndTs <= occurrence.Window.StartTs || occurrence.Window.StartTs < ranked.StartTs || occurrence.Window.EndTs > ranked.EndTs {
			t.Fatalf("occurrence must lie within its published envelope: %+v", occurrence)
		}
		if i > 0 && ranked.OccurrenceWindows[i-1].Window.EndTs < occurrence.Window.StartTs {
			hasGap = true
		}
	}
	if !hasGap || ranked.StartTs != ranked.OccurrenceWindows[0].Window.StartTs || ranked.EndTs != ranked.OccurrenceWindows[len(ranked.OccurrenceWindows)-1].Window.EndTs {
		t.Fatalf("fixture must distinguish a multi-occurrence envelope from a continuous episode: %+v", ranked.OccurrenceWindows)
	}
	t.Logf("producer premise: rank=%d occurrences=%d envelope=%.6f..%.6f first=%.6f..%.6f", ranked.Rank, len(ranked.OccurrenceWindows), ranked.StartTs, ranked.EndTs, ranked.OccurrenceWindows[0].Window.StartTs, ranked.OccurrenceWindows[0].Window.EndTs)
	resultBefore, _ := json.Marshal(result)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := &types.BusContext{RepoRoot: queryBus.RepoRoot, WorkDir: t.TempDir(), Language: lang, Mutable: types.NewMutableState("b1688-answer-" + lang), ToolResults: []types.ToolResult{result}, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}}
			compile := func() types.TraceCausalProjectionSet {
				return types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: bus.ToolResults, RequestModel: &bus.AnalysisIR.RequestModel, RepoRoot: bus.RepoRoot}))
			}
			projectionBefore, _ := json.Marshal(compile())
			modelText := "The reported intervals are preserved for independent inspection."
			if lang == "zh" {
				modelText = "已保留查询区间，供独立核对。"
			}
			answer, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model-summary", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": modelText}}})
			emitted, err := (&EmitAnswerDocument{}).Execute(bus, answer)
			if err != nil || !emitted.Success {
				t.Fatalf("public answer emit failed: %v %s", err, emitted.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			if doc == nil {
				t.Fatal("accepted answer missing")
			}
			var windowBlock *types.AnswerBlock
			var modelSeen, occupancySeen, causeSeen bool
			for i := range doc.Blocks {
				block := &doc.Blocks[i]
				switch block.ID {
				case "model-summary":
					modelSeen = block.Text == modelText
				case runtimeTraceCausalProjectionBlockIDBase + runtimeTraceCausalProjectionOccupancySuffix:
					occupancySeen = true
				case runtimeTraceCausalProjectionBlockIDBase:
					causeSeen = true
				case runtimeTraceCausalProjectionBlockIDBase + runtimeTraceCausalProjectionRepresentativeSuffix:
					windowBlock = block
				}
			}
			if !modelSeen || !occupancySeen || !causeSeen || windowBlock == nil || len(windowBlock.Items) == 0 {
				t.Fatalf("model text and both system axes must survive: model=%t occupancy=%t cause=%t windows=%+v", modelSeen, occupancySeen, causeSeen, windowBlock)
			}
			cells := windowBlock.Items[0].Cells
			if len(cells) != 4 || cells[0] != "#1" || cells[2] != fmt.Sprintf("%.6f..%.6f", ranked.StartTs, ranked.EndTs) {
				t.Fatalf("publication must retain the exact rank and producer envelope: %+v", cells)
			}
			projectionAfter, _ := json.Marshal(compile())
			resultAfter, _ := json.Marshal(bus.ToolResults[0])
			if !bytes.Equal(projectionBefore, projectionAfter) || !bytes.Equal(resultBefore, resultAfter) {
				t.Fatal("display must not modify typed observations, root selection, pricing, or projection")
			}
			assertB1688EnvelopeCaliber(t, cells[3], lang == "zh")
		})
	}
}

func assertB1688EnvelopeCaliber(t *testing.T, caliber string, zh bool) {
	t.Helper()
	wants := []string{"envelope", "does not establish", "continuous", "single occurrence", "overlap", "must not be summed", "full query window", "not this single-window duration"}
	forbidden := "one representative occurrence"
	if zh {
		wants = []string{"包络", "不证明", "持续", "一次", "重叠", "不可相加", "完整查询窗", "不能把它当作此单窗时长"}
		forbidden = "一处代表性发生片段"
	}
	for _, want := range wants {
		if !strings.Contains(caliber, want) {
			t.Errorf("system interval caliber must preserve %q: %s", want, caliber)
		}
	}
	if strings.Contains(caliber, forbidden) {
		t.Errorf("an envelope must not assert an independently identified occurrence: %s", caliber)
	}
}

func TestB1688BareIntervalDoesNotInventOccurrenceIdentity(t *testing.T) {
	projection := types.TraceCausalProjection{RankedSeats: []types.TraceCausalProjectionNode{
		{Subject: "worker-101", ChainRelevance: "on_chain", Rank: 1, StartTs: 1.001, EndTs: 1.009},
		{Subject: "neighbor-102", ChainRelevance: "adjacent", Rank: 2, StartTs: 1.001, EndTs: 1.009},
		{Subject: "background-103", ChainRelevance: "background", Rank: 3, StartTs: 1.001, EndTs: 1.009},
		{Subject: "unclassified-104", Rank: 4, StartTs: 1.001, EndTs: 1.009},
	}}
	before, _ := json.Marshal(projection)
	for _, zh := range []bool{true, false} {
		block := runtimeTraceCausalProjectionRepresentativeWindowsBlock(projection, zh, runtimeTraceCausalProjectionBlockIDBase, "", nil, nil)
		if block == nil || len(block.Items) != 1 || block.Items[0].Cells[2] != "1.001000..1.009000" {
			t.Fatalf("a single published interval remains visible without invented occurrence metadata: %+v", block)
		}
		assertB1688EnvelopeCaliber(t, block.Items[0].Cells[3], zh)
	}
	after, _ := json.Marshal(projection)
	if !bytes.Equal(before, after) {
		t.Fatal("display modified its typed input")
	}
}
