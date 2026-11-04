package agent

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A query's exact scheduler target may guide automatic presentation, but it
// must not delete another thread's observed facts or break explicit layouts.
func TestHMC349PublicBusinessDefaultsKeepExactTargetAndOptionalBackground(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			ctx, results := hmc346QueryContext(t, path, 100, .999, 1.051)
			original, _ := json.Marshal(results)
			projection := hmc218CausalProjection(t, ctx)
			contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
			defaults := map[string]bool{}
			for _, r := range types.RuntimeNativeFactDisplaySelections(&ctx.AnalysisIR.RequestModel, contract) {
				defaults[r.ObservationID] = true
			}
			business := map[string]types.ObservationRecord{}
			for _, r := range results[0].Observations {
				if r.Predicate == types.TraceBusinessSpanPredicate {
					business[r.Subject] = r
				}
			}
			if len(business) != 2 {
				t.Fatal("public query lost target or dependent worker business facts")
			}
			main, worker := business["app-main-100"], business["document-worker-200"]
			if main.ID == "" || worker.ID == "" || main.SourceRef.QueryTargetPID != 100 || worker.SourceRef.QueryTargetPID != 100 {
				t.Fatal("fixture has no precise shared query target")
			}
			if !defaults[main.ID] || defaults[worker.ID] {
				t.Fatal("automatic business tables promote an independent worker to the exact target's default presentation")
			}
			var background types.RuntimeMeasurementTable
			for _, table := range contract.Choices() {
				if table.ObservationID == worker.ID {
					background = table
				}
			}
			if background.ObservationID == "" || background.DefaultPresentation || !background.HasNativeMeasurementFacts() {
				t.Fatal("background table disappeared or lost current native facts")
			}
			_, messages := finalizerReaderFieldMessages(t, ctx)
			if !bytes.Contains([]byte(messages), []byte(worker.ID)) {
				t.Fatal("actual finalizer lost observed dependency-thread business information")
			}
			blocks := []any{}
			if kind != types.BlockSummary {
				blocks = append(blocks, map[string]any{"id": "lead", "kind": "summary", "text": "主线程与工作线程的观测区间分别保留，依赖关系以独立链证据为准。"})
			}
			blocks = append(blocks, map[string]any{"id": "background", "kind": kind, "runtime_measurement": map[string]any{"observation_id": worker.ID, "view": background.View}})
			args, _ := json.Marshal(map[string]any{"blocks": blocks})
			out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
			if err != nil || !out.Success {
				t.Fatalf("explicit current background facts became inaccessible: %v / %s", err, out.Summary)
			}
			bound := ctx.Mutable.AnswerDocumentV2().Blocks[len(blocks)-1].RuntimeMeasurement
			if bound == nil || !bound.IsBound() || !reflect.DeepEqual(bound.BoundTable.Rows, background.Rows) || !reflect.DeepEqual(bound.BoundTable.Notes, background.Notes) {
				t.Fatal("optional display changed worker values, source or bounds")
			}
			after, _ := json.Marshal(results)
			if !bytes.Equal(original, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
				t.Fatal("display intent changed query facts or causal projection")
			}
		})
	}
}
