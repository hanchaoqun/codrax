package agent

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The real interval publisher already has exact query endpoints. Registration
// must not silently discard them while allowing the same native rows to display.
func TestHMC348PublicMemberEndpointsReachAllFactSurfaces(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			ctx, result := b1706SemanticFactContext(t)
			original, _ := json.Marshal(result)
			projection := hmc218CausalProjection(t, ctx)
			tables := hmc347MemberTables(ctx)
			if len(tables) != 2 {
				t.Fatal("actual native member fixtures absent")
			}
			for _, table := range tables {
				basis, owned := table.NativeSourceTitleBasis()
				if !owned || basis.QueryWindow != "[10,10.02) s" {
					t.Fatalf("native interval endpoint lost between publisher and shared title: %+v / owned=%v", basis, owned)
				}
				if !table.HasNativeMeasurementFacts() || table.HasNativeRawFacts() || table.MemberSet != nil || table.DisplayPopulation != nil {
					t.Fatal("endpoint display acquired another fact or coverage authority")
				}
			}
			_, messages := finalizerReaderFieldMessages(t, ctx)
			if !strings.Contains(messages, `"query_window":"[10,10.02) s"`) {
				t.Fatal("actual finalizer lost native query ruler")
			}
			table := tables[0]
			blocks := []any{}
			if kind != types.BlockSummary {
				blocks = append(blocks, map[string]any{"id": "lead", "kind": "summary", "text": "以下保留逐成员业务区间，不据集合包络推断执行或依赖。"})
			}
			blocks = append(blocks, map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
			args, _ := json.Marshal(map[string]any{"blocks": blocks})
			out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
			if err != nil || !out.Success {
				t.Fatalf("native fact surface: %v / %s", err, out.Summary)
			}
			bound := ctx.Mutable.AnswerDocumentV2().Blocks[len(blocks)-1].RuntimeMeasurement
			if bound == nil || !bound.IsBound() || !reflect.DeepEqual(bound.BoundTable.Rows, table.Rows) || !reflect.DeepEqual(bound.BoundTable.Notes, table.Notes) {
				t.Fatal("title contract rewrote member facts")
			}
			if basis, ok := bound.BoundTable.NativeSourceTitleBasis(); !ok || basis.QueryWindow != "[10,10.02) s" {
				t.Fatal("final carrier lost exact endpoint")
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(original, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
				t.Fatal("display endpoint changed native query or causal projection")
			}
		})
	}
}
