package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func freshNativeDisplayContext224(ctx *types.AgentContext) *types.AgentContext {
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language,
		AgentName: ctx.AgentName, Stage: ctx.Stage, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR,
		AttachedLogCatalog: ctx.AttachedLogCatalog, AttachedLog: ctx.AttachedLog, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight}
}

func nativeDisplayBlock224(table types.RuntimeMeasurementTable, indices ...int) map[string]any {
	return map[string]any{"id": "records", "kind": "table", "requested_dimension_indices": indices,
		"runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
}

func TestHMC224NativeDisplayDimensionSchemaFullPatch(t *testing.T) {
	ctx, table := nativeDisplayBindingContext224(t, `{}`)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if got := view.RuntimeMeasurementContract.NativeDisplayDimensionChoices(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("typed choices: %v", got)
	}
	roster := renderAnswerDocRuntimeMeasurementChoices(ctx)
	if !strings.Contains(roster, "eligible requested_dimension_indices=[1 2]") || !strings.Contains(roster, table.ObservationID) {
		t.Fatalf("table-specific eligibility omitted: %s", roster)
	}
	if !strings.Contains(string(tool.BuildAnswerDocumentParametersFor(view)), `"uniqueItems":true`) {
		t.Fatal("schema omitted unique index teaching")
	}
	for _, patch := range []bool{false, true} {
		for _, name := range []string{"valid", "unknown_index", "duplicate_index", "no_selector", "foreign_selector", "summary", "wrong_kind", "copied_rows"} {
			t.Run(name+map[bool]string{false: "/full", true: "/patch"}[patch], func(t *testing.T) {
				block := nativeDisplayBlock224(table, 1, 2)
				switch name {
				case "unknown_index":
					block["requested_dimension_indices"] = []int{7}
				case "duplicate_index":
					block["requested_dimension_indices"] = []int{1, 1}
				case "no_selector":
					delete(block, "runtime_measurement")
				case "foreign_selector":
					block["runtime_measurement"].(map[string]any)["observation_id"] = "another-query"
				case "summary":
					block["runtime_measurement"].(map[string]any)["view"] = types.RuntimeMeasurementSummary
				case "wrong_kind":
					block["kind"] = "summary"
				case "copied_rows":
					block["text"] = "| record |\n|---|\n| fabricated |"
				}
				schema := tool.BuildAnswerDocumentParametersFor(view)
				payload := map[string]any{"blocks": []any{block}}
				if patch {
					schema = tool.BuildAnswerDocumentPatchParametersFor(view)
					payload = map[string]any{"replace_blocks": []any{block}}
				}
				raw, _ := json.Marshal(payload)
				// The local schema subset does not execute uniqueItems; the
				// precise typed binder below still rejects duplicate indices.
				if err := toolparam.Validate(raw, schema); name != "duplicate_index" && (err == nil) != (name == "valid") {
					t.Fatalf("schema disagrees with exact binding: %v", err)
				}
				if !patch {
					payload["blocks"] = []any{map[string]any{"id": "lead", "kind": "summary", "text": "Recorded events."}, block}
					raw, _ = json.Marshal(payload)
					result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
					if (err == nil && result.Success) != (name == "valid") {
						t.Fatalf("public full emit disagrees with schema: %v %s", err, result.Summary)
					}
				}
			})
		}
	}
}

func TestHMC224NativeDisplayDimensionsRejectUnqualifiedScope(t *testing.T) {
	for _, name := range []string{"partial", "nonrequired", "causal_role", "duplicate_dimension", "source_bound", "scope_unspecified", "scope_not_applicable", "scope_invalid", "scope_missing", "explicit_window"} {
		t.Run(name, func(t *testing.T) {
			query := `{}`
			if name == "partial" {
				query = `{"limit":2}`
			}
			ctx, table := nativeDisplayBindingContext224(t, query)
			rm := &ctx.AnalysisIR.RequestModel
			switch name {
			case "nonrequired":
				rm.RequestedAnswerDimensions.Dimensions[0].Required = false
			case "causal_role":
				rm.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionCausalAttribution
			case "duplicate_dimension":
				rm.RequestedAnswerDimensions.Dimensions[1].Index = 1
			case "source_bound":
				rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", Confidence: 0, RequestedDimensionIndices: []int{1}}}
			case "scope_unspecified":
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeUnspecified
			case "scope_not_applicable":
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeNotApplicable
			case "scope_invalid":
				rm.RuntimeQuestionProfile.Scope = "not_a_scope"
			case "scope_missing":
				rm.RuntimeQuestionProfile = nil
			case "explicit_window":
				start, end := 1.0, 2.0
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..2s"}
			}
			ctx = freshNativeDisplayContext224(ctx)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			block := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}, RequestedDimensionIndices: []int{1}}
			if _, valid := types.BoundNativeDisplayDimensions(block, view.RuntimeMeasurementContract); valid {
				t.Fatal("unqualified member dimension acquired display authority")
			}
			if nativeDisplayDimensionBindingInstruction(ctx, 1, false) != "" {
				t.Fatal("unqualified dimension received binding advice")
			}
			// The same evidence remains selectable as partial/background display.
			block.RequestedDimensionIndices = nil
			if !types.BindRuntimeMeasurementReceipt(block.RuntimeMeasurement, view.RuntimeMeasurementContract) {
				t.Fatal("display eligibility removed usable partial facts")
			}
		})
	}
}

func TestHMC224NativeDisplayDimensionsDoNotDoubleCountLegacySeats(t *testing.T) {
	for _, name := range []string{"duplicate_explicit", "same_table_legacy", "independent_legacy"} {
		t.Run(name, func(t *testing.T) {
			ctx, table := measurementMemberPublicContext(t)
			ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions = append(ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions,
				types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionMemberSet, Label: "independent roster", Required: true})
			ctx = freshNativeDisplayContext224(ctx)
			if !table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
				t.Fatal("test requires old qualified Trace population")
			}
			first := nativeDisplayBlock224(table, 1)
			first["facet_ids"] = []string{"member_set"}
			second := nativeDisplayBlock224(table, 1)
			second["id"], second["facet_ids"] = "second", []string{"member_set"}
			if name == "same_table_legacy" {
				delete(second, "requested_dimension_indices")
			}
			if name == "independent_legacy" {
				second = map[string]any{"id": "second", "kind": "bullet_list", "facet_ids": []string{"member_set"}, "items": []any{map[string]any{"text": "Independent supplied roster"}}}
			}
			raw, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Measured populations."}, first, second}})
			result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || !result.Success {
				t.Fatalf("valid mixed display: %v %s", err, result.Summary)
			}
			missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2())
			if name == "independent_legacy" && len(missing) != 0 || name != "independent_legacy" && (len(missing) != 1 || missing[0].Index != 2) {
				t.Fatalf("explicit/legacy seats are not disjoint: %+v", missing)
			}
			if ctx.Mutable.IsInvestigationComplete() {
				t.Fatal("display binding completed investigation")
			}
		})
	}
}

func TestHMC224NativeDisplayDimensionRecoveryRebindsCurrentAuthority(t *testing.T) {
	ctx, table := nativeDisplayBindingContext224(t, `{}`)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	raw, _ := json.Marshal(map[string]any{"blocks": []any{nativeDisplayBlock224(table, 1, 2)}})
	var doc types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &doc); err != nil || !types.RebindRuntimeAnswerReceipts(&doc, view) {
		t.Fatalf("valid JSON recovery failed: %v", err)
	}
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &doc)
	copy := ctx.Mutable.AnswerDocumentV2()
	copy.Blocks[0].RequestedDimensionIndices[0] = 999
	if got := ctx.Mutable.AnswerDocumentV2().Blocks[0].RequestedDimensionIndices; !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("mutable snapshot aliases indices: %v", got)
	}
	// Current request now assigns dimension 1 to a source obligation. A saved
	// successful binding must neither retain authority nor partially mutate rows.
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", RequestedDimensionIndices: []int{1}}}
	ctx = freshNativeDisplayContext224(ctx)
	current := types.BuildAnswerSemanticViewForAgentContext(ctx)
	before := doc.Blocks[0].RuntimeMeasurement
	if types.RebindRuntimeAnswerReceipts(&doc, current) || doc.Blocks[0].RuntimeMeasurement != before {
		t.Fatal("stale index assignment rebound or partially replaced receipt")
	}
	if answerDocumentNativeDisplayDimensionCovered(ctx, &doc, 1) || answerDocumentNativeDisplayDimensionCovered(ctx, &doc, 2) {
		t.Fatal("stale multi-index binding retained coverage")
	}
	clone := view.RuntimeMeasurementContract.Clone()
	choices := clone.NativeDisplayDimensionChoices()
	choices[0] = 999
	if !reflect.DeepEqual(view.RuntimeMeasurementContract.NativeDisplayDimensionChoices(), []int{1, 2}) {
		t.Fatal("contract choice return aliases private authority")
	}
}
