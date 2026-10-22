package agent

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC279ResourceStatisticsActualFinalizerAndDelivery(t *testing.T) {
	ctx := resourceFactsContextHMC249(t)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if string(before) != string(after) {
		t.Fatal("context changed native evidence")
	}
	for _, want := range []string{"distribution", "complete-query event-flow statistics", "原查询全集事件=3", "768", "512", "bytes", "事件次数", "大小状态次数", "已知非负=2", "原深度"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual Finalizer lost %q", want)
		}
	}
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
		t.Fatal("duplicated raw publication JSON")
	}
	groups, views := 0, 0
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if !table.HasNativeResourceFacts() || table.View != types.RuntimeMeasurementDistribution {
			continue
		}
		views++
		groups += len(table.Rows)
		for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable} {
			t.Run(table.ObservationID+"/"+string(kind), func(t *testing.T) {
				ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
				blocks := []types.AnswerBlock{{ID: "statistics", Kind: kind, RuntimeMeasurement: ref}}
				if kind == types.BlockTable {
					blocks = append([]types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "以下统计按独立来源列出。"}}, blocks...)
				}
				raw, _ := json.Marshal(map[string]any{"blocks": blocks})
				out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
				if err != nil || !out.Success {
					t.Fatalf("native delivery %v %s", err, out.Summary)
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh"))
				for _, row := range table.Rows {
					for _, cell := range row {
						if !strings.Contains(visible, cell) {
							t.Fatalf("same-payload cell omitted %q", cell)
						}
					}
				}
				if !strings.Contains(visible, table.Notes[1]) {
					t.Fatal("full query/omission denominator not delivered")
				}
			})
		}
	}
	if views != 4 || groups != 4 {
		t.Fatalf("independent sources/true zero: views=%d groups=%d", views, groups)
	}
}

func TestHMC279ResourceStatisticsBudgetDispositionActualFinalizer(t *testing.T) {
	ctx := resourceFactsContextHMC249(t)
	results := ctx.Mutable.TurnAArtifacts().ToolResults
	marked := 0
	for i := range results {
		for j, r := range results[i].Observations {
			pub, ok := types.DecodeRuntimeMeasurementPublication(r)
			if !ok || r.Predicate != types.RuntimeResourceMeasurementPredicate {
				continue
			}
			notes := []string{}
			for _, n := range r.RichNotes {
				if !strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
					notes = append(notes, n)
				}
			}
			r.RichNotes = notes
			legacy, ok := types.PublishRuntimeResourceMeasurement(r, pub.Tables[:2], false)
			if !ok {
				t.Fatal("legacy fixture")
			}
			results[i].Observations[j] = types.WithRuntimeResourceStatisticsDisplayOmission(legacy)
			marked++
		}
	}
	if marked != 4 {
		t.Fatal("missing native source fixtures", marked)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "资源事件分组统计表因独立展示预算未发布") || !strings.Contains(prompt, "不是未查询或零结果") || !strings.Contains(prompt, "原深度") {
		t.Fatal("typed budget disposition disappeared or old facts were evicted")
	}
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.HasNativeResourceFacts() && table.View == types.RuntimeMeasurementDistribution {
			t.Fatal("unpublished table advertised")
		}
	}
}
