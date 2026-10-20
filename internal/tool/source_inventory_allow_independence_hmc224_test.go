package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

const sourceInventoryAllowRequest224 = "List the recorded events and their process/thread identities; assess their causal relationship. Also inspect src/owner.go when requested."

func sourceInventoryAllowAnalysis224(t *testing.T, carrier string, independent bool) *types.BusContext {
	t.Helper()
	ctx := sourceInventoryRuntimeAnalysisContext(t)
	ctx.Mutable = types.NewMutableState(sourceInventoryAllowRequest224)
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true,
		CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: types.TurnOutcomeSourceExplanation}
	switch carrier {
	case "log_bundle":
		ctx.Mutable.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "ObservedFailure"}}})
	case "trace_bundle":
		ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "recorded-event"}}})
	default:
		kind := strings.TrimSuffix(carrier, "_preflight")
		ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: kind, Source: "attached"}}}
	}
	var payload map[string]any
	if err := json.Unmarshal(sourceInventoryRuntimeAnalysisPayload(t, nil), &payload); err != nil {
		t.Fatal(err)
	}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "full_artifact", "source_quote": "recorded events", "confidence": 0.9}
	payload["source_scope_profile"] = map[string]any{"requested_scope": "all", "confidence": 0.9}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 0.9,
		"dimensions": []any{
			map[string]any{"index": 1, "role": "member_set", "label": "events", "required": true, "source_quote": "recorded events"},
			map[string]any{"index": 2, "role": "source_attribute", "label": "recorders", "required": true, "source_quote": "process/thread identities"},
			map[string]any{"index": 3, "role": "function_or_purpose", "label": "relation", "required": true, "source_quote": "causal relationship"},
		}}
	if independent {
		if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "src", "owner.go"), []byte("package src\nfunc Owner() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		payload["required_files"] = []any{map[string]any{"path": "src/owner.go", "confidence": 1, "rationale": "independent source request"}}
	}
	raw, _ := json.Marshal(payload)
	out, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !out.Success {
		t.Fatalf("analysis: %v / %s", err, out.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("runtime members acquired a source universe from permission or a source attribute (independent=%v): %s", independent, out.Summary)
	}
	if independent && (len(rm.AnalyzerHints.RequiredFileHints) != 1 ||
		rm.AnalyzerHints.RequiredFileHints[0].Path != "src/owner.go" ||
		types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, ctx.TurnRouteHint) != types.RuntimeSourceRequirementPrecise) {
		t.Fatal("independent current-source read lost its precise obligation")
	}
	// The applicability projection does not rewrite route permissions, artifact
	// scopes, or the separately requested causal/source dimensions.
	if !ctx.TurnRouteHint.RequiredOutcomes.Has(types.TurnOutcomeSourceExplanation) ||
		rm.RuntimeArtifactScopeProfile.RequestedScope != types.RuntimeArtifactScopeFullArtifact ||
		len(rm.RequestedAnswerDimensions.Dimensions) != 3 {
		t.Fatal("source-inventory applicability rewrote independent obligations")
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Language: "en"}}
	return ctx
}

func TestHMC224SourceInventoryAllowRequiresIndependentScopePublic(t *testing.T) {
	for _, carrier := range []string{"log_bundle", "trace_bundle", "log_preflight", "trace_preflight"} {
		for _, independent := range []bool{false, true} {
			t.Run(carrier+map[bool]string{false: "/runtime", true: "/mixed"}[independent], func(t *testing.T) {
				sourceInventoryAllowAnalysis224(t, carrier, independent)
			})
		}
	}
}

func TestHMC224SourceInventoryAllowPublicNativeLogDisplay(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime", true: "mixed"}[independent], func(t *testing.T) {
			ctx := sourceInventoryAllowAnalysis224(t, "log_bundle", independent)
			var inputs []loginput.Input
			for _, relative := range []string{"app/session.log", "kernel/session.log.gz"} {
				path, err := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/" + relative)
				if err != nil {
					t.Fatal(err)
				}
				inputs = append(inputs, loginput.Input{Path: path})
			}
			prepared, _ := logQueryPublicBus(t, inputs)
			ctx.AttachedLogCatalog, ctx.AttachedLog = prepared.AttachedLogCatalog, prepared.AttachedLog
			query, err := (&LogQuery{}).Execute(ctx, json.RawMessage(`{}`))
			if err != nil || !query.Success {
				t.Fatalf("query: %v / %s", err, query.Summary)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
			view := types.BuildAnswerSemanticViewForBusContext(ctx)
			indices := view.RuntimeMeasurementContract.NativeDisplayDimensionChoices()
			// The independent source read is not a source-member inventory.
			// Native events retain this runtime dimension beside that obligation.
			if !reflect.DeepEqual(indices, []int{1}) {
				t.Fatalf("runtime member display missing or consumed other dimensions: %v", indices)
			}
			choices := view.RuntimeMeasurementContract.Choices()
			if len(choices) != 1 || len(choices[0].Rows) != 9 || choices[0].CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
				t.Fatalf("producer members or completion boundary changed: %+v", choices)
			}
			block := map[string]any{"id": "records", "kind": "table", "requested_dimension_indices": []int{1},
				"runtime_measurement": map[string]any{"observation_id": choices[0].ObservationID, "view": choices[0].View}}
			payload, _ := json.Marshal(map[string]any{"blocks": []any{
				map[string]any{"id": "lead", "kind": "summary", "text": "These rows retain the source identities; no causal link is established."}, block}})
			err = toolparam.Validate(payload, BuildAnswerDocumentParametersFor(view))
			if err != nil {
				t.Fatalf("projected schema source/display separation: %v", err)
			}
			out, err := (&EmitAnswerDocument{}).Execute(ctx, payload)
			if err != nil || !out.Success {
				t.Fatalf("public native binding: %v / %s", err, out.Summary)
			}
			rows := 0
			for _, b := range ctx.Mutable.AnswerDocumentV2().Blocks {
				if b.RuntimeMeasurement != nil {
					rows += len(b.RuntimeMeasurement.BoundTable.Rows)
					if len(b.Items) != 0 || b.Text != "" || !reflect.DeepEqual(b.RuntimeMeasurement.BoundTable.Rows, choices[0].Rows) {
						t.Fatal("display copied or changed producer facts")
					}
				}
			}
			if rows != 9 || ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("native display lost rows or gained completion/cause authority")
			}
		})
	}
}
