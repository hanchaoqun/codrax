package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
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
		!strings.Contains(out, "refine_action=`soft_narrow_if_answer_critical_else_caveat`") ||
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

func TestRenderAnswerDocToolHandoffCarriersSuppressesAcceptedSourceInventoryDisplayDebt(t *testing.T) {
	ctx := sourceInventoryMechanicalLandingContextForExplorerTest(false)
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.Complete = false
	obs.Execution = &types.SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true}
	obs.Page = &types.SourceInventoryObservationPage{Offset: 0, Emitted: 3, Total: 128, NextCursor: "3", Complete: false}
	obs.SourceClasses = append(obs.SourceClasses, types.SourceInventorySourceClassCount{
		Role:     types.SourcePathRoleThirdParty,
		Count:    1,
		Complete: true,
		Languages: []types.SourceInventoryLanguageCount{{
			Language: "python",
			Count:    1,
			InScope:  true,
			Samples:  []string{"internal/thirdparty/pkg/entry.py"},
		}},
		Samples: []string{"internal/thirdparty/pkg/entry.py"},
	})
	obs.Sets[0].Members = append(obs.Sets[0].Members, types.SourceInventoryObservationMember{
		Name:          "VendorEntry",
		Key:           "VendorEntry",
		SupportRef:    "VendorEntry: internal/thirdparty/pkg/entry.py:3",
		Provenance:    []string{"repo_lens:direct_children"},
		Role:          types.AnswerCandidateRoleFunction,
		File:          "internal/thirdparty/pkg/entry.py",
		Line:          3,
		Language:      "python",
		CoverageState: types.SourceInventoryCoverageObserved,
	})
	obs.Sets[0].Complete = false
	obs.Sets[0].Count = len(obs.Sets[0].Members)
	obs.Sets[0].Total = len(obs.Sets[0].Members)
	ctx.Mutable.SetSourceInventoryObservation(obs)
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "requested functions",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Run", "Serve", "VendorEntry"},
		SupportRefs: []string{
			"Run: src/run.py:7",
			"Serve: src/serve.py:12",
			"VendorEntry: internal/thirdparty/pkg/entry.py:3",
		},
	}}
	ctx.Mutable.SetInvestigationAggregateFacts(facts)
	ctx.Mutable.SetInvestigationComplete("requested function inventory closed")
	ctx.Mutable.SetInvestigationResultKind("resolved")
	ctx.Mutable.RetainInvestigationAggregateFacts()
	bus := &types.BusContext{RepoRoot: ctx.RepoRoot, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}
	if !tool.SourceInventoryAcceptedClosureCoversRequestedUniverse(bus, facts) {
		t.Fatalf("fixture should prove accepted requested source-inventory universe")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		HandoffCarriers: []types.ToolHandoffCarrier{{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "repo_map",
			ReasonCode: types.SourceInventoryRefinementReasonCandidateBudgetTruncated,
			Refinement: &types.ToolRefinementHint{
				ReasonCode:               types.SourceInventoryRefinementReasonCandidateBudgetTruncated,
				CandidateBudgetTruncated: true,
				PreferredNextTool:        "repo_map",
				PreferredParams: map[string]string{
					"view":               "source_inventory",
					"include_attributes": "false",
				},
				RequiredFields: []string{"scope"},
				NextCursor:     "50",
			},
			ObservationRefs: []types.ToolObservationRef{{
				ID:       "repo_map:source_inventory#navigation",
				Producer: "repo_map",
				ClaimKey: "source_inventory",
			}},
		}},
	})

	out := renderAnswerDocToolHandoffCarriers(ctx)
	if !strings.Contains(out, "tool_observation_handoff") ||
		!strings.Contains(out, "observation=`repo_map:source_inventory#navigation`") {
		t.Fatalf("source-inventory observations should remain visible after stripping stale refinement:\n%s", out)
	}
	for _, forbidden := range []string{
		"candidate_budget_truncated",
		"soft_narrow_if_answer_critical_else_caveat",
		"preferred_tool=`repo_map`",
		"next_cursor=`50`",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("accepted source-inventory universe leaked stale refinement %q:\n%s", forbidden, out)
		}
	}
}

func TestRenderAnswerDocAcceptedClosureTreatsAcceptedSourceInventoryBoundariesAsAuditOnly(t *testing.T) {
	ctx := sourceInventoryMechanicalLandingContextForExplorerTest(false)
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "requested functions",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Run", "Serve"},
		SupportRefs: []string{"Run: src/run.py:7", "Serve: src/serve.py:12"},
	}})
	ctx.Mutable.SetInvestigationComplete("function source-inventory rows are closed")
	ctx.Mutable.SetInvestigationResultKind("resolved")
	ctx.Mutable.RetainInvestigationAggregateFacts()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ValidationBoundaryNotes: []string{"source=pre_dispatch_auto_complete; criterion=evidence_count; downstream=\"disclose the scope/evidence boundary\""},
	})

	out := renderAnswerDocAcceptedClosure(ctx)
	for _, want := range []string{
		"post-closure validation audit notes",
		"Source-inventory authority has accepted the requested/exact universe",
		"Do not convert stale broad-lens budget",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("accepted closure status missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "disclose the boundary in summary/caveat text") {
		t.Fatalf("accepted source-inventory closure should not prompt user-visible caveats:\n%s", out)
	}
}

func TestRenderTypedToolHandoffCarriersHonorsProjectionBudget(t *testing.T) {
	carriers := []types.ToolHandoffCarrier{}
	for i := 0; i < 3; i++ {
		carriers = append(carriers, types.ToolHandoffCarrier{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   fmt.Sprintf("emit_evidence_%d", i),
			ReasonCode: fmt.Sprintf("accepted_evidence_handoff_%d", i),
			AcceptedEvidence: []types.AcceptedEvidenceRef{
				{ID: fmt.Sprintf("ev-a-%d", i), Source: "src/a.go", LineStart: 1},
				{ID: fmt.Sprintf("ev-b-%d", i), Source: "src/b.go", LineStart: 2},
			},
		})
	}
	out := renderTypedToolHandoffCarriers("### Typed handoff", carriers, toolHandoffRenderOptions{
		MaxCarriers: 2,
		MaxRefs:     1,
	})
	if strings.Count(out, "tool=`emit_evidence_") != 2 {
		t.Fatalf("expected exactly two rendered carriers:\n%s", out)
	}
	if strings.Count(out, "evidence=`ev-a-") != 2 {
		t.Fatalf("expected one evidence ref per rendered carrier:\n%s", out)
	}
	if strings.Contains(out, "evidence=`ev-b-") {
		t.Fatalf("handoff projection ignored MaxRefs:\n%s", out)
	}
}

func TestRenderAnswerDocToolHandoffCarriersMixedRuntimeSourceUsesCompactBudget(t *testing.T) {
	mut := types.NewMutableState("q")
	carriers := []types.ToolHandoffCarrier{}
	for i := 0; i < 5; i++ {
		carriers = append(carriers, types.ToolHandoffCarrier{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   fmt.Sprintf("emit_evidence_%d", i),
			ReasonCode: fmt.Sprintf("accepted_evidence_handoff_%d", i),
			AcceptedEvidence: []types.AcceptedEvidenceRef{
				{ID: fmt.Sprintf("ev-a-%d", i), Source: "src/a.go", LineStart: 1},
				{ID: fmt.Sprintf("ev-b-%d", i), Source: "src/b.go", LineStart: 2},
				{ID: fmt.Sprintf("ev-c-%d", i), Source: "src/c.go", LineStart: 3},
				{ID: fmt.Sprintf("ev-d-%d", i), Source: "src/d.go", LineStart: 4},
				{ID: fmt.Sprintf("ev-e-%d", i), Source: "src/e.go", LineStart: 5},
				{ID: fmt.Sprintf("ev-f-%d", i), Source: "src/f.go", LineStart: 6},
				{ID: fmt.Sprintf("ev-g-%d", i), Source: "src/g.go", LineStart: 7},
			},
		})
	}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{HandoffCarriers: carriers})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			LogTriage: &types.LogBundle{
				Observations: []types.LogObservation{{
					Kind:    types.LogObservationRetryCycle,
					Summary: "retry loop",
				}},
			},
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
				IsCrossComponent:     true,
			},
			DiagnosticProfile: types.DiagnosticIntentProfile{
				IsDiagnostic:        true,
				CurrentVersionCheck: true,
				Confidence:          0.9,
			},
		}},
	}
	out := renderAnswerDocToolHandoffCarriers(ctx)
	if strings.Count(out, "tool=`emit_evidence_") != 4 {
		t.Fatalf("mixed answer-doc handoff should render four carriers:\n%s", out)
	}
	if strings.Count(out, "evidence=`ev-f-") != 4 {
		t.Fatalf("mixed answer-doc handoff should keep six refs per rendered carrier:\n%s", out)
	}
	if strings.Contains(out, "evidence=`ev-g-") {
		t.Fatalf("mixed answer-doc handoff projection ignored compact MaxRefs:\n%s", out)
	}
}
