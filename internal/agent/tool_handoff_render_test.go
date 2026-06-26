package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderTypedToolHandoffCarriersRendersTypedFieldsOnly(t *testing.T) {
	out := renderTypedToolHandoffCarriers("### Typed handoff", []types.ToolHandoffCarrier{{
		Version:    types.ToolHandoffCarrierVersion,
		ToolName:   "emit_change_plan",
		ReasonCode: "invalid_enum",
		Repair: &types.ToolRepair{
			Code: "write_plan_repair_pack",
			Hint: "run repo_map then rewrite the whole answer",
		},
		SupportedJSON: &types.ToolJSONSurfaceDescriptor{
			ToolName:          "emit_change_plan",
			ReasonCode:        "invalid_enum",
			FailingFieldPaths: []string{"$.changes[0].edits[0].kind"},
			AcceptedEnums: map[string][]string{
				"$.changes[].edits[].kind": {"replace"},
			},
		},
		Refinement: &types.ToolRefinementHint{
			ReasonCode:               "list_files_result_truncated",
			ResultTruncated:          true,
			CandidateBudgetTruncated: true,
			PreferredNextTool:        "repo_map",
			PreferredParams: map[string]string{
				"scope": "internal/agent",
				"view":  "relation_map",
			},
			RequiredFields:         []string{"scope", "sources"},
			NextCursor:             "50",
			SkippedLargeCandidates: []string{"logs/big.trace"},
			ExcludedRoots:          []string{".codrax", "node_modules"},
			TopSourceClasses:       []types.SourcePathRole{types.SourcePathRoleProduction},
		},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:             "ev-1",
			Source:         "internal/app/main.py",
			LineStart:      12,
			OwnerSymbol:    "main",
			AnchorSymbol:   "run",
			SourcePathRole: types.SourcePathRoleProduction,
		}},
	}})
	for _, want := range []string{
		"tool=`emit_change_plan`",
		"reason=`invalid_enum`",
		"json_fields=`$.changes[0].edits[0].kind`",
		"enum_fields=`$.changes[].edits[].kind`",
		"refine_flags=`result_truncated,candidate_budget_truncated`",
		"preferred_tool=`repo_map`",
		"preferred_params=`scope=internal/agent,view=relation_map`",
		"required_fields=`scope,sources`",
		"next_cursor=`50`",
		"skipped_large=`logs/big.trace`",
		"excluded_roots=`.codrax,node_modules`",
		"top_source_classes=`production`",
		"evidence=`ev-1` @ `internal/app/main.py:12`",
		"owner=`main`",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered handoff missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rewrite the whole answer") || strings.Contains(out, "run repo_map") {
		t.Fatalf("renderer leaked repair hint prose:\n%s", out)
	}
}

func TestRenderTypedToolHandoffCarriersKeepsRefinementBeforePlainObservations(t *testing.T) {
	carriers := make([]types.ToolHandoffCarrier, 0, 10)
	for i := 0; i < 9; i++ {
		carriers = append(carriers, types.ToolHandoffCarrier{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "trace_query",
			ReasonCode: "tool_observation_handoff",
			ObservationRefs: []types.ToolObservationRef{{
				ID:       "obs-" + string(rune('a'+i)),
				Producer: "trace_query",
				ClaimKey: "runtime",
			}},
		})
	}
	carriers = append(carriers, types.ToolHandoffCarrier{
		Version:    types.ToolHandoffCarrierVersion,
		ToolName:   "grep",
		ReasonCode: "grep_result_truncated",
		Refinement: &types.ToolRefinementHint{
			ReasonCode:        "grep_result_truncated",
			ResultTruncated:   true,
			PreferredNextTool: "repo_map",
			PreferredParams: map[string]string{
				"query": "Owner",
				"view":  "task_map",
			},
		},
	})

	out := renderTypedToolHandoffCarriers("### Typed handoff", carriers)
	if !strings.Contains(out, "tool=`grep`") ||
		!strings.Contains(out, "preferred_tool=`repo_map`") ||
		!strings.Contains(out, "preferred_params=`query=Owner,view=task_map`") {
		t.Fatalf("actionable refinement was dropped behind plain observations:\n%s", out)
	}
}

func TestRenderTypedToolHandoffCarriersExtractorHistoricalToolSurface(t *testing.T) {
	out := renderTypedToolHandoffCarriers("### Typed handoff", []types.ToolHandoffCarrier{{
		Version:    types.ToolHandoffCarrierVersion,
		ToolName:   "read_file",
		ReasonCode: "tool_observation_handoff",
		Refinement: &types.ToolRefinementHint{
			ReasonCode:        "read_file_result_truncated",
			ResultTruncated:   true,
			PreferredNextTool: "read_file",
			PreferredParams: map[string]string{
				"line_offset": "40",
				"path":        "src/app.ets",
			},
			RequiredFields: []string{"path", "line_offset"},
			NextCursor:     "40",
		},
		ObservationRefs: []types.ToolObservationRef{{
			ID:        "read_file:src/app.ets:1:40",
			Source:    "src/app.ets",
			LineStart: 1,
			Producer:  "read_file",
		}},
	}}, toolHandoffRenderOptions{
		HistoricalProducerLabels: true,
		CurrentStageAllowedTools: []string{"emit_answer_symbol", "emit_hypothesis_verdict"},
	})
	for _, want := range []string{
		"Tool names below identify prior-stage producers",
		"Current callable tools here: `emit_answer_symbol`, `emit_hypothesis_verdict`",
		"producer_tool=`read_file`",
		"prior_stage_preferred_tool=`read_file`",
		"prior_stage_preferred_params=`line_offset=40,path=src/app.ets`",
		"prior_stage_required_fields=`path,line_offset`",
		"prior_stage_next_cursor=`40`",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("historical extractor handoff missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{
		"- tool=`read_file`",
		" · tool=`read_file`",
		" · preferred_tool=`read_file`",
		" · preferred_params=`line_offset=40,path=src/app.ets`",
		" · required_fields=`path,line_offset`",
		" · next_cursor=`40`",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("historical extractor handoff leaked current-action field %q:\n%s", forbidden, out)
		}
	}
}

func TestRenderAnswerDocToolHandoffCarriersConsumesTurnA(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		HandoffCarriers: []types.ToolHandoffCarrier{{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "emit_evidence",
			ReasonCode: "accepted_evidence_handoff",
			AcceptedEvidence: []types.AcceptedEvidenceRef{{
				ID:          "ev-accepted",
				Source:      "src/owner.ts",
				LineStart:   5,
				OwnerSymbol: "Owner",
			}},
		}},
	})
	ctx := &types.AgentContext{Mutable: mut}
	out := renderAnswerDocToolHandoffCarriers(ctx)
	if !strings.Contains(out, "## Typed Repair And Evidence Handoff") ||
		!strings.Contains(out, "evidence=`ev-accepted` @ `src/owner.ts:5`") {
		t.Fatalf("finalizer handoff not rendered:\n%s", out)
	}
}
