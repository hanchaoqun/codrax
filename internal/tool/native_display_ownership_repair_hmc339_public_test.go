package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC339NativeOwnershipRepairPublicFullAndPatch(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "patch"}[patch], func(t *testing.T) {
			ctx, record := eventDisplayPublicContext236(t)
			valid := eventBlock236("facts", "table", record.ID, map[string]any{"field_keys": []string{"jank.appid"}})
			lead := map[string]any{"id": "lead", "kind": "summary", "text": "Observed application records; grouping is separate from causal interpretation."}
			if patch {
				if r := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{lead, valid}}, false); !r.Success {
					t.Fatal(r.Summary)
				}
			}
			before, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			bad := eventBlock236("facts", "table", record.ID, map[string]any{"field_keys": []string{"jank.appid"}})
			bad["requested_dimension_indices"] = []int{999}
			operation := "blocks"
			input := map[string]any{operation: []any{lead, bad}}
			if patch {
				operation = "replace_blocks"
				input = map[string]any{operation: []any{bad}}
			}
			result := b1659bExecuteAnswer(t, ctx, input, patch)
			if result.Success || result.Repair == nil {
				t.Fatal("unpublished display ownership was accepted")
			}
			if !strings.Contains(result.Summary, "requested_dimension_indices") || !strings.Contains(result.Summary, "Keep the current observation_id/view and block kind") || strings.Contains(result.Summary, "This measurement selector is unavailable, stale") {
				t.Fatalf("a valid native selector was blamed for the separate ownership field: %s", result.Summary)
			}
			if patch {
				after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
				if string(before) != string(after) {
					t.Fatal("rejected ownership patch changed the accepted answer")
				}
			}
			// The same bad selection remains rejected. Feedback cannot manufacture
			// a native population credential or silently remove optional metadata.
			if retry := b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{bad}}, true); retry.Success {
				t.Fatal("unmodified bad ownership became valid after feedback")
			}
			fixed := b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{valid}}, true)
			if !fixed.Success {
				t.Fatal(fixed.Summary)
			}
			block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
			if block.Kind != types.BlockTable || !block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement.ObservationID != record.ID || len(block.RequestedDimensionIndices) != 0 {
				t.Fatal("explicit repair changed the native kind, source or selection")
			}
		})
	}
}

func TestHMC339OriginalResourceSelectionOwnershipPublicReproduction(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_identity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:    &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Label: "资源操作明细", Role: types.RequestedAnswerDimensionMemberSet, Required: true}}}}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Mutable: types.NewMutableState("整理资源操作的原始字段"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": .0005, "time_end": .0055, "limit": 30})
	r, err := (&TraceQuery{}).Execute(ctx, query)
	if err != nil || !r.Success {
		t.Fatalf("public native query: %v %s", err, r.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	inv := requireEventSearchInventory(t, r)
	selection := map[string]any{"field_keys": []string{"event.trace_time_seconds", "event.type", "resource.operation", "marker.name", "resource.size", "resource.address_i64", "resource.address_bits_hex", "resource.sub_type_id", "resource.sub_type_name", "resource.end_ts_ns", "resource.callchain_id", "counter.value", "counter.owner_scope", "emitter.tid"}, "row_ids": []string{"event-1", "event-2", "event-3", "event-4", "event-5", "event-6", "event-7", "event-8", "event-9", "event-10"}}
	block := eventBlock236("facts", "table", inv.ID, selection)
	block["requested_dimension_indices"] = []int{1}
	lead := map[string]any{"id": "lead", "kind": "summary", "text": "以下保留原生记录，累计量和资源操作分别计数；不能据此确定响应根因。"}
	rejected := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{lead, block}}, false)
	if rejected.Success || !strings.Contains(rejected.Summary, "blocks[1].requested_dimension_indices") || !strings.Contains(rejected.Summary, "Keep the current observation_id/view and block kind") {
		t.Fatalf("original valid selector and optional ownership were not separated: %s", rejected.Summary)
	}
	delete(block, "requested_dimension_indices")
	fixed := b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{block}}, true)
	if !fixed.Success {
		t.Fatal(fixed.Summary)
	}
	accepted := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
	if accepted.Kind != types.BlockTable || !accepted.RuntimeMeasurement.IsBound() || len(accepted.RuntimeMeasurement.BoundTable.Rows) != 10 {
		t.Fatal("repair lost the original ten-row native table or changed kind")
	}
}

func TestHMC339OwnershipHintDoesNotMaskInvalidNativeSelector(t *testing.T) {
	ctx, record := eventDisplayPublicContext236(t)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	for _, mode := range []string{"wrong_id", "wrong_view", "wrong_field", "competing_text", "causal_metadata"} {
		t.Run(mode, func(t *testing.T) {
			b := types.AnswerBlock{ID: "facts", Kind: types.BlockTable, RequestedDimensionIndices: []int{999}, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: record.ID, View: types.RuntimeMeasurementMembers, EventSelection: &types.RuntimeEventSelection{FieldKeys: []string{"jank.appid"}}}}
			switch mode {
			case "wrong_id":
				b.RuntimeMeasurement.ObservationID = "unknown"
			case "wrong_view":
				b.RuntimeMeasurement.View = types.RuntimeMeasurementSummary
			case "wrong_field":
				b.RuntimeMeasurement.EventSelection.FieldKeys = []string{"unknown"}
			case "competing_text":
				b.Text = "copied facts"
			case "causal_metadata":
				b.TraceCausalClaimCaliber = types.TraceCausalClaimTypedChain
			}
			hints := preCheckRuntimeMeasurementBindings(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{b}}, view)
			if len(hints) == 0 || strings.Contains(formatEmitFixHints(hints), "Keep the current observation_id/view and block kind") {
				t.Fatal("an invalid native selector was relabeled as only optional ownership")
			}
		})
	}
	for _, phrase := range []string{"Aggregate counts and generic timing labels do not prove", "Explicit native state fields describe only their published owner and measured interval set", "self wall time is not running time"} {
		if !strings.Contains(types.RuntimeMeasurementFactDisplayTeaching, phrase) {
			t.Fatal("native state authority is contradicted by generic timing teaching")
		}
	}
}
