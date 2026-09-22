package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual instruction, not a global substring: the detailed reader
// already carries these descriptions, while the two compact row-local recaps
// must identify the components of their own attribution value independently.
func TestFinalValueComponentPublicInstruction(t *testing.T) {
	const runnable = "lower-priority dependency scheduling/compute-supply candidate; an actual inversion or lock blocking is not proved; composition: runnable wait counted in full 1.000 ms + folded running-supply deficit 0.000 ms"
	const supply = "folded supply deficit (estimated lower bound, not total running time); frequency known for 10.000 ms, unknown for 2.000 ms; uses default capability ratios"
	for _, tc := range []struct {
		name, kind, state, direction, description string
		measured, effective                       float64
		notes                                     []string
	}{
		{"sleep_vs_runnable", "priority_inversion_candidate", "s_sleep", "scheduling_priority", runnable, 17, 1, []string{"gated_runnable=1.000", "gated_running_deficit=0.000"}},
		{"io_vs_runnable", "priority_inversion_candidate", "io_wait", "scheduling_priority", runnable, 11, 1, []string{"gated_runnable=1.000", "gated_running_deficit=0.000"}},
		{"running_deficit", "running", "running", "scheduling_supply", supply, 12, 3, []string{"supply_fold_deficit_ms=3.000", "supply_fold_ideal_ms=9.000", "fold_basis=known=10.000ms,unknown=2.000ms", "fold_capability=default_table"}},
		{"pure_io", "io_wait", "io_wait", "io_dependency", "", 11, 11, []string{"gated_runnable=1.000", "gated_running_deficit=0.000"}},
		{"semantic_work", "jit_compile", "running", "self_workload", "", 12, 3, []string{"supply_fold_deficit_ms=3.000", "supply_fold_ideal_ms=9.000", "fold_basis=known=10.000ms,unknown=2.000ms"}},
		{"unknown_family", "future_family", "io_wait", "io_dependency", "", 11, 1, []string{"gated_runnable=1.000", "gated_running_deficit=0.000"}},
		{"missing_parts", "priority_inversion_candidate", "s_sleep", "scheduling_priority", "lower-priority dependency scheduling/compute-supply candidate; an actual inversion or lock blocking is not proved", 17, 1, nil},
		{"mismatched_parts", "priority_inversion_candidate", "s_sleep", "scheduling_priority", "lower-priority dependency scheduling/compute-supply candidate; an actual inversion or lock blocking is not proved", 17, 1, []string{"gated_runnable=1.000", "gated_running_deficit=3.000"}},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				record := finalValueComponentRecord(tc.name, tc.kind, tc.state, tc.direction, 2, tc.measured, tc.effective, tc.notes...)
				ctx := finalValueComponentContext(t, lang, []types.ObservationRecord{record})
				before := finalValueComponentSnapshot(t, ctx)
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if after := finalValueComponentSnapshot(t, ctx); after != before {
					t.Fatal("instruction changed input observations, ledger, projection, candidate contract, request, or model answer")
				}
				projection, node := finalValueComponentNode(t, ctx, record.ID)
				if got := tracefinding.RootCauseNodeValueDescription(projection, node, "en"); got != tc.description {
					t.Fatalf("fixture does not carry the intended existing value ruler: %q != %q", got, tc.description)
				}
				stateRows := finalValueComponentLines(instruction, "- state_value_authority ", record.ID)
				if tc.measured == tc.effective {
					if len(stateRows) != 0 {
						t.Fatalf("equal values gained a previously omitted state recap: %v", stateRows)
					}
				} else {
					if len(stateRows) != 1 {
						t.Fatalf("expected one exact state row, got %v", stateRows)
					}
					finalValueComponentAssertRow(t, stateRows[0], tc.description,
						"state_kind=`"+tc.state+"`", fmt.Sprintf("measured_state_occupancy=%.3fms", tc.measured),
						fmt.Sprintf("effective_attribution=%.3fms", tc.effective), "relation=`distinct_do_not_substitute`", "locator_range=`2.000000..2.020000`")
				}
				compact := finalValueComponentLines(instruction, "  - ", record.ID)
				if len(compact) != 1 {
					t.Fatalf("expected one exact compact leader, got %v", compact)
				}
				finalValueComponentAssertRow(t, compact[0], tc.description,
					"leader_rank=#1", "leader_subject=`worker-200`", fmt.Sprintf("leader_effective_attribution=%.3fms", tc.effective),
					"leader_state_kind=`"+tc.state+"`", fmt.Sprintf("leader_measured_state_occupancy=%.3fms", tc.measured),
					"query_window=`2.000000..2.020000`", "direction_subtotal_authority=`not_provided_without_exact_fold`")
			})
		}
	}
}

func TestFinalValueComponentPublicInstructionKeepsSameSubjectWindowsSeparate(t *testing.T) {
	first := finalValueComponentRecord("window-a", "priority_inversion_candidate", "s_sleep", "scheduling_priority", 2, 17, 1,
		"gated_runnable=1.000", "gated_running_deficit=0.000")
	// Overlapping query windows keep both rows eligible in the elected window,
	// but their distinct query identities must never exchange components.
	second := finalValueComponentRecord("window-b", "priority_inversion_candidate", "s_sleep", "scheduling_priority", 2.01, 14, 5,
		"gated_runnable=2.000", "gated_running_deficit=3.000", "gated_capability=freq_only")
	for _, reverse := range []bool{false, true} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(fmt.Sprintf("reverse=%t/%s", reverse, lang), func(t *testing.T) {
				records := []types.ObservationRecord{first, second}
				if reverse {
					records[0], records[1] = records[1], records[0]
				}
				ctx := finalValueComponentContext(t, lang, records)
				before := finalValueComponentSnapshot(t, ctx)
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, record := range records {
					projection, node := finalValueComponentNode(t, ctx, record.ID)
					description := tracefinding.RootCauseNodeValueDescription(projection, node, "en")
					if description == "" {
						t.Fatal("fixture lost its own component description")
					}
					for _, prefix := range []string{"- state_value_authority ", "  - "} {
						rows := finalValueComponentLines(instruction, prefix, record.ID)
						if len(rows) != 1 {
							t.Fatalf("missing or merged same-subject/rank window row: %s %v", record.ID, rows)
						}
						finalValueComponentAssertRow(t, rows[0], description,
							fmt.Sprintf("effective_attribution=%.3fms", node.EffectiveImpactMS),
							fmt.Sprintf("locator_range=`%.6f..%.6f`", record.Span.StartTs, record.Span.EndTs))
						other := "full 2.000 ms"
						if record.ID == second.ID {
							other = "full 1.000 ms"
						}
						if strings.Contains(rows[0], other) {
							t.Errorf("row borrowed the other query's components: %s", rows[0])
						}
					}
				}
				if finalValueComponentSnapshot(t, ctx) != before {
					t.Fatal("cross-window display mutated input or ranking authority")
				}
			})
		}
	}
}

func finalValueComponentRecord(id, kind, state, direction string, start, measured, effective float64, notes ...string) types.ObservationRecord {
	measurementKey := state
	if state == "s_sleep" {
		measurementKey = "sleep"
	}
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/components.trace", ArtifactID: "components.trace", ArtifactKind: "trace", QueryScopeID: id},
		Span:      types.ObservationSpan{StartTs: start, EndTs: start + .02, LineStart: 1, LineEnd: 2},
		Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:worker-200", Subject: "worker-200", Object: kind,
		Value: fmt.Sprintf("%.3f", effective), Unit: "ms",
		RichNotes: append([]string{"rank=1", "tier=primary", "chain_relevance=on_chain", "type=" + kind, "dominant_state=" + state,
			fmt.Sprintf("impact_ms=%.3f", effective), fmt.Sprintf("effective_impact_ms=%.3f", effective),
			fmt.Sprintf("%s=%.3f", measurementKey, measured), fmt.Sprintf("selected_window=%.6f..%.6f", start, start+.02),
			"rank_board_target=app-100", "rank_board_params_fingerprint=component-board", "fix_direction=" + direction}, notes...),
	}
}

func finalValueComponentContext(t *testing.T, lang string, records []types.ObservationRecord) *types.AgentContext {
	t.Helper()
	const request = "Diagnose the observed runtime dependency."
	rm := types.RequestModel{RawRequest: request, Language: lang, Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, Confidence: 1}}
	mu := types.NewMutableState(request)
	mu.SetRequestModel(rm)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true,
		Observations: records, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}}}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned explanation. 模型原文。"}}})
	bus := &types.BusContext{Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "/captures/components.trace", Carrier: "path"}}}}
	return ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
}

func finalValueComponentSnapshot(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), ledger, set, contract, ctx.AnalysisIR.RequestModel, ctx.Mutable.AnswerDocumentV2()})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func finalValueComponentNode(t *testing.T, ctx *types.AgentContext, id string) (types.TraceCausalProjection, types.TraceCausalProjectionNode) {
	t.Helper()
	for _, projection := range types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)).Projections {
		for _, node := range projection.RankedSeats {
			if node.EvidenceID == id {
				return projection, node
			}
		}
	}
	t.Fatalf("published rank row missing: %s", id)
	return types.TraceCausalProjection{}, types.TraceCausalProjectionNode{}
}

func finalValueComponentLines(instruction, prefix, id string) []string {
	var rows []string
	for _, line := range strings.Split(instruction, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, "row_identity=`"+id+"`") &&
			(prefix != "  - " || strings.Contains(line, "direction_subtotal_authority=")) {
			rows = append(rows, line)
		}
	}
	return rows
}

func finalValueComponentAssertRow(t *testing.T, row, description string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if !strings.Contains(row, field) {
			t.Errorf("original field %q changed or missing in %s", field, row)
		}
	}
	if description == "" {
		if strings.Contains(row, "effective_attribution_description=") {
			t.Errorf("unrelated or unknown row borrowed a component description: %s", row)
		}
	} else if want := fmt.Sprintf("effective_attribution_description=%q", description); !strings.Contains(row, want) {
		t.Errorf("exact attribution component identity missing %q in %s", want, row)
	}
}
