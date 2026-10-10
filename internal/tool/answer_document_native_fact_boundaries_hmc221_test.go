package tool

import (
	"encoding/json"
	"html"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC221MixedRuntimeNativeFactsPublicBoundaries(t *testing.T) {
	for _, domain := range []string{"log", "measurement"} {
		for _, negative := range []string{"causal_only", "relation_only", "contributors_only", "verdict_only", "optional", "unspecified", "not_applicable", "inventory", "window"} {
			t.Run(domain+"/"+negative, func(t *testing.T) {
				bus, _ := nativeMixedPublicContext(t, domain)
				rm := &bus.AnalysisIR.RequestModel
				roles := map[string]types.RequestedAnswerDimensionRole{"causal_only": types.RequestedAnswerDimensionCausalAttribution,
					"relation_only": types.RequestedAnswerDimensionRelationPath, "contributors_only": types.RequestedAnswerDimensionCausalContributorSet,
					"verdict_only": types.RequestedAnswerDimensionTargetEffectVerdict}
				if role, ok := roles[negative]; ok {
					rm.RequestedAnswerDimensions.Dimensions = []types.RequestedAnswerDimension{{Role: role, Required: true}}
				}
				switch negative {
				case "optional":
					rm.RequestedAnswerDimensions.Dimensions[0].Required = false
				case "unspecified":
					rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeUnspecified
				case "not_applicable":
					rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeNotApplicable
				case "inventory":
					rm.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}}
				case "window":
					start, end := 8.0, 9.0
					rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[8,9) 秒", Confidence: 1}
				}
				bus.Mutable.SetRequestModel(*rm)
				out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "事实与解释仍受各自范围限制。"}}}, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
					if block.RuntimeMeasurement != nil {
						t.Fatalf("%s imported unrelated default facts", negative)
					}
				}
			})
		}
	}
}

func TestHMC221NativeMeasurementUnverifiedWindowPublic(t *testing.T) {
	for _, pair := range []bool{false, true} {
		bus, _ := nativeMixedPublicContext(t, "measurement")
		path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2})
		result := bus.Mutable.TurnAArtifacts().ToolResults[0]
		// Start from the successful public query and erase only its time
		// receipt to exercise a producer with unknown continuous coverage.
		// Do not construct a choice or manufacture any measured value.
		for i := range result.Observations {
			publication, ok := types.DecodeRuntimeMeasurementPublication(result.Observations[i])
			if !ok {
				continue
			}
			result.Observations[i].SourceRef.QueryWindowKnown = false
			result.Observations[i].SourceRef.QueryWindowStartTs, result.Observations[i].SourceRef.QueryWindowEndTs = 0, 0
			publication.Source = result.Observations[i].SourceRef
			encoded, _ := json.Marshal(publication)
			result.Observations[i].RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)}
		}
		if pair {
			receipt := types.NewNativeMeasurementPair(bus.Mutable, [2]types.NativeMeasurementPairSide{{Request: args, Result: result, Material: traceMeasurementPairMaterial(bus, result)}})
			result = types.ToolResult{ToolName: "trace_query", Success: true, RuntimeMeasurementPair: receipt}
		}
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
		choices := types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Choices()
		var supplementary *types.RuntimeMeasurementTable
		for i, table := range choices {
			if table.View == types.RuntimeMeasurementMembers {
				if table.DefaultPresentation || !strings.Contains(table.Label, "time scope unverified") {
					t.Fatalf("unverified time table may replace requested-window facts: %+v", table)
				}
				supplementary = &choices[i]
			}
		}
		if supplementary == nil {
			t.Fatalf("explicit supplementary selection lost (pair=%t): choices=%+v", pair, choices)
		}
		lead := map[string]any{"id": "lead", "kind": "summary", "text": "行范围补充查询没有证明完整时间窗覆盖。"}
		out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead}}, false)
		if !out.Success {
			t.Fatal(out.Summary)
		}
		for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
			if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.ObservationID == supplementary.ObservationID {
				t.Fatal("summary-only emit auto-selected unknown-window measurement")
			}
		}
		selector := map[string]any{"id": "supplementary", "kind": "table", "runtime_measurement": map[string]any{"observation_id": supplementary.ObservationID, "view": supplementary.View}}
		out = b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead, selector}}, false)
		visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
		if !out.Success || !strings.Contains(visible, "9007199254740993") || !strings.Contains(visible, "cannot substitute for the requested-window statistics") {
			t.Fatalf("manual supplementary selector lost original values/boundary: %s", out.Summary)
		}
	}
}

func TestHMC221NativeFactsCannotAuthorizeLogRelationPublic(t *testing.T) {
	bus, _ := nativeMixedPublicContext(t, "log")
	lead := map[string]any{"id": "lead", "kind": "summary", "text": "跨来源关系仍未证明。"}
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	before := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	records := bus.Mutable.TurnAArtifacts().ToolResults[0].Observations
	lead["relation_claims"] = []any{map[string]any{"authority_id": records[0].ID, "member_refs": []string{records[1].ID}, "physical_relation": "unresolved", "addition": "forbidden"}}
	out = b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{lead}}, true)
	if out.Success || !strings.Contains(out.Summary, "no typed relation authority") || render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh") != before {
		t.Fatalf("native table upgraded ordinary records into relation authority or mutated accepted answer: %s", out.Summary)
	}
}
