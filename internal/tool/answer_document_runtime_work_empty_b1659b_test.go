package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1659bEmptyWorkBus(profileOnly bool) *types.BusContext {
	ctx := externalPerfBus()
	rm := types.RequestModel{Intent: types.IntentExplain, PerfTrace: ctx.Mutable.PerfTrace()}
	if profileOnly {
		rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeRelationAnalysis, RuntimeWorkRelationRequested: true,
		}
	} else {
		rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{
				Index: 1, Label: "Observed work and target", Required: true,
				Role: types.RequestedAnswerDimensionRuntimeWorkRelation,
			}},
		}
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	return ctx
}

func b1659bModelWorkBoundary() map[string]any {
	return map[string]any{
		"id": "work-boundary", "kind": "caveat", "surface_role": "principal",
		"facet_ids": []string{"runtime_work_relation", "uncertainty_boundary"},
		"text":      "The trace does not identify a measured business operation to relate to the target. Existing scheduler observations remain separate facts.",
	}
}

func b1659bExecuteAnswer(t *testing.T, ctx *types.BusContext, payload map[string]any, patch bool) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	var result types.ToolResult
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("answer submission mutated model-owned input bytes")
	}
	return result
}

func TestB1659bPublicEmptyWorkBoundaryFullAndPatchRemainExecutable(t *testing.T) {
	for _, profileOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_dimension", true: "profile"}[profileOnly], func(t *testing.T) {
			ctx := b1659bEmptyWorkBus(profileOnly)
			view := types.BuildAnswerSemanticViewForBusContext(ctx)
			if view == nil || view.RuntimeWorkRelationContract.Active() {
				t.Fatal("fixture must compile an actual empty semantic-work supply")
			}
			boundary := b1659bModelWorkBoundary()
			payload := map[string]any{"blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Only established trace facts are available."},
				boundary,
			}}
			raw, _ := json.Marshal(payload)
			if err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(view)); err != nil {
				t.Fatalf("teaching's empty-evidence shape is not accepted by the dispatch schema: %v", err)
			}
			if result := b1659bExecuteAnswer(t, ctx, payload, false); !result.Success {
				t.Fatalf("public full emit rejected the taught empty-evidence shape: %+v", result)
			}
			base := ctx.Mutable.AnswerDocumentV2()
			before := blockByID(t, base, "work-boundary")
			if before.RuntimeWorkRelation != nil || len(before.ClaimUses) != 0 || before.Text != boundary["text"] ||
				before.Kind != types.BlockCaveat || before.SurfaceRole != types.SurfacePrincipal ||
				!reflect.DeepEqual(before.FacetIDs, []string{"runtime_work_relation", "uncertainty_boundary"}) {
				t.Fatalf("full emit manufactured evidence or changed model ownership: %+v", before)
			}
			boundary["title"] = "Evidence still needed"
			patch := map[string]any{"replace_blocks": []any{boundary}, "unchanged_block_ids": []string{"summary"}}
			patchRaw, _ := json.Marshal(patch)
			if err := toolparam.Validate(patchRaw, BuildAnswerDocumentPatchParametersFor(view)); err != nil {
				t.Fatalf("complete-block boundary patch is not schema-valid: %v", err)
			}
			if result := b1659bExecuteAnswer(t, ctx, patch, true); !result.Success {
				t.Fatalf("public patch rejected empty-evidence boundary: %+v", result)
			}
			after := ctx.Mutable.AnswerDocumentV2()
			want := before
			want.Title = "Evidence still needed"
			if !reflect.DeepEqual(want, blockByID(t, after, "work-boundary")) {
				t.Fatal("patch changed another field or invented a runtime-work receipt")
			}
			for _, original := range base.Blocks {
				if original.ID != "work-boundary" && !reflect.DeepEqual(original, blockByID(t, after, original.ID)) {
					t.Fatalf("boundary patch changed unrelated block %q", original.ID)
				}
			}
			if !reflect.DeepEqual(base.Citations, after.Citations) {
				t.Fatal("boundary patch changed unrelated citations")
			}
			// Neither the new teaching nor a valid boundary grants a fake
			// observation/conclusion pair through either public submission path.
			for _, patchMode := range []bool{false, true} {
				boundary["runtime_work_relation"] = map[string]any{
					"observation_id": "invented-scheduler-work", "conclusion": "relation_unproven",
				}
				attempt := map[string]any{"blocks": []any{
					map[string]any{"id": "summary", "kind": "summary", "text": "Only established trace facts are available."}, boundary,
				}}
				if patchMode {
					attempt = map[string]any{"replace_blocks": []any{boundary}, "unchanged_block_ids": []string{"summary"}}
				}
				beforeJSON, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
				result := b1659bExecuteAnswer(t, ctx, attempt, patchMode)
				if result.Success || !strings.Contains(result.Summary, "runtime_work_relation") {
					t.Fatalf("empty supply allowed an invented receipt (patch=%t): %+v", patchMode, result)
				}
				afterJSON, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
				if !bytes.Equal(beforeJSON, afterJSON) {
					t.Fatalf("rejected invented receipt mutated accepted answer (patch=%t)", patchMode)
				}
			}
		})
	}
}
