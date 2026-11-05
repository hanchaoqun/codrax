package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC354NativeSupplementReusePublicBoundary(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_identity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx, results := hmc346QueryContext(t, path, 100, .0005, .0055)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results[1:]})
	ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{}, results[:1])
	ctx = ctx.ShallowClone()
	var tables []types.RuntimeMeasurementTable
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementTimeline && strings.HasSuffix(table.ObservationID, "#thread_timeline_observation") {
			tables = append(tables, table)
			basis, current := table.NativeSourceTitleBasis()
			t.Logf("selector=%s key=%q source_current=%t basis=%+v", table.ObservationID, table.NativeDisplayDataKey(), current, basis)
		}
	}
	if len(tables) != 2 {
		t.Fatalf("two actual query publications required, got %d", len(tables))
	}
	raw, _ := json.Marshal(map[string]any{"blocks": []any{
		map[string]any{"id": "lead", "kind": "summary", "text": "以下为本次查询的原生线程状态。"},
		map[string]any{"id": "chosen", "kind": "table", "runtime_measurement": map[string]any{"observation_id": tables[0].ObservationID, "view": "timeline"}},
	}})
	result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !result.Success {
		t.Fatalf("public answer: %v / %s", err, result.Summary)
	}
	text := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh")
	if count := strings.Count(text, "| 线程 | 状态 | 开始"); count != 1 {
		t.Fatalf("same complete native partition rendered %d times across ordinary/supplement lanes", count)
	}
}
