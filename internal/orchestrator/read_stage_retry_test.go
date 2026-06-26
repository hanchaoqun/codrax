package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildExploreTransientRetryCheckpointHintIncludesTypedObservationOrigins(t *testing.T) {
	mut := types.NewMutableState("count generated tool files")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary: "[exec_command: $ find internal/tool -name '*.go' | wc -l]\n" +
			"[exec_command: evidence_origin=command_measurement measurement=count]\n" +
			"140\n",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	for _, want := range []string{
		"Checkpoint summary",
		"remaining-objective context only",
		"typed observation origins=command_measurement:1",
		"successful tool results=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint hint missing %q:\n%s", want, got)
		}
	}
}

func TestBuildExploreTransientRetryCheckpointHintCarriesReadProofGuidance(t *testing.T) {
	mut := types.NewMutableState("weak read proof")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/maybe.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-observed",
			Source:          "pkg/maybe.py",
			LineStart:       3,
			Subject:         "Maybe",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/maybe.py"},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	for _, want := range []string{
		"proof authority=weak",
		"reason=proof_weak",
		"action=add_proof",
		"mode=advisory",
		"loop next-action=add_proof source=proof_authority reason=proof_weak",
		"route_surface=verification",
		"route_reason=loop_tool_route_verification",
		"route_tools=run_tests",
		"loop shadow recommended=add_proof imperative=add_proof match=true reason=proof_weak",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint hint missing proof guidance %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "mode=hard") {
		t.Fatalf("weak read proof must not be rendered as a hard block:\n%s", got)
	}
}

func TestBuildExploreTransientRetryCheckpointHintDedupsEvidenceViews(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:      types.EvidenceConcrete,
		Subject:   "A",
		Predicate: "binds",
		Object:    "B",
		Source:    "a.go",
		LineStart: 1,
		Scope:     types.ScopeLine,
	}
	ev.ID = types.StableEvidenceID(ev)
	mut := types.NewMutableState("dedupe checkpoint evidence")
	mut.AppendEvidence([]types.EvidenceItem{ev})
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{ev}})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{ev},
	}}

	got := o.buildExploreTransientRetryCheckpointHint()
	if !strings.Contains(got, "structured evidence rows=1") {
		t.Fatalf("checkpoint hint should count one merged evidence row, got:\n%s", got)
	}
	if strings.Contains(got, "structured evidence rows=3") {
		t.Fatalf("checkpoint hint double-counted evidence views:\n%s", got)
	}
}

func TestReadLoopNextActionDecisionFromGuidanceWeakProof(t *testing.T) {
	decision, ok := readLoopNextActionDecisionFromGuidance(loopkernel.ReadProofGuidance{
		Active:            true,
		State:             loopkernel.ProofCoverageWeak,
		ReasonCode:        "proof_weak",
		RecommendedAction: loopkernel.LoopActionAddProof,
		TruthState:        types.TruthLedgerWeak,
		TruthAction:       types.TruthActionAddProof,
		Advisory:          true,
	}, true)
	if !ok || !decision.Active {
		t.Fatalf("weak add-proof guidance should produce active decision: %+v ok=%t", decision, ok)
	}
	if decision.Action != loopkernel.LoopActionAddProof ||
		decision.RouteSurface != loopkernel.LoopToolSurfaceVerification ||
		decision.RouteReasonCode != "loop_tool_route_verification" ||
		!containsString(decision.ToolSuggestions, "run_tests") {
		t.Fatalf("unexpected decision route: %+v", decision)
	}
	if !decision.Policy.IsActive() ||
		!containsString(decision.Policy.AllowedTools, "read_file") ||
		!containsString(decision.Policy.AllowedTools, "run_tests") ||
		!containsString(decision.Policy.DeniedTools, "exec_command") {
		t.Fatalf("decision should carry executable read dispatch policy: %+v", decision.Policy)
	}
	summary := readLoopNextActionDecisionSummary(decision)
	for _, want := range []string{
		"loop next-action=add_proof",
		"source=proof_authority",
		"reason=proof_weak",
		"route_surface=verification",
		"route_tools=run_tests",
		"policy_allowed_tools=",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("decision summary missing %q: %s", want, summary)
		}
	}
}

func TestReadLoopNextActionDecisionSkipsNonAddProofGuidance(t *testing.T) {
	if decision, ok := readLoopNextActionDecisionFromGuidance(loopkernel.ReadProofGuidance{
		Active:            true,
		State:             loopkernel.ProofCoverageCovered,
		RecommendedAction: loopkernel.LoopActionContinue,
		TruthState:        types.TruthLedgerCovered,
		TruthAction:       types.TruthActionContinue,
	}, true); ok || decision.Active {
		t.Fatalf("covered/continue guidance should not produce add-proof decision: %+v ok=%t", decision, ok)
	}
	if decision, ok := readLoopNextActionDecisionFromGuidance(loopkernel.ReadProofGuidance{
		Active:            true,
		State:             loopkernel.ProofCoverageFailed,
		RecommendedAction: loopkernel.LoopActionRepair,
		HardBlock:         true,
		TruthState:        types.TruthLedgerFailed,
		TruthAction:       types.TruthActionRepair,
	}, true); ok || decision.Active {
		t.Fatalf("hard-block guidance should not produce add-proof decision: %+v ok=%t", decision, ok)
	}
}

func TestGraphStateReadLoopNextActionIsOneShot(t *testing.T) {
	state := &graphState{}
	decision := readLoopNextActionDecision{
		Active:          true,
		Action:          loopkernel.LoopActionAddProof,
		ReasonCode:      "proof_weak",
		RouteSurface:    loopkernel.LoopToolSurfaceVerification,
		RouteReasonCode: "loop_tool_route_verification",
		ToolSuggestions: []string{"run_tests"},
	}
	state.setReadLoopNextAction(decision)
	got, ok := state.consumeReadLoopNextAction()
	if !ok || got.Action != loopkernel.LoopActionAddProof {
		t.Fatalf("consume decision = %+v ok=%t", got, ok)
	}
	if got, ok := state.consumeReadLoopNextAction(); ok || got.Active {
		t.Fatalf("read loop next-action must be one-shot, got %+v ok=%t", got, ok)
	}
	directive := renderReadLoopNextActionDirective(decision)
	for _, want := range []string{
		"Read loop typed next action",
		"loop next-action=add_proof",
		"narrow proof-collection continuation",
	} {
		if !strings.Contains(directive, want) {
			t.Fatalf("directive missing %q:\n%s", want, directive)
		}
	}
}

func TestReadRunActiveStateFromGraphStateOmitsRawRetryHint(t *testing.T) {
	state := &graphState{}
	rawHint := "raw retry hint with model-facing checkpoint body"
	state.setTransientRetryHint(rawHint)
	state.setReadLoopNextAction(readLoopNextActionDecision{
		Active:          true,
		Action:          loopkernel.LoopActionAddProof,
		ReasonCode:      "proof_weak",
		ProofState:      loopkernel.ProofCoverageWeak,
		TruthAction:     types.TruthActionAddProof,
		RouteSurface:    loopkernel.LoopToolSurfaceVerification,
		RouteReasonCode: "loop_tool_route_verification",
		ToolSuggestions: []string{"run_tests"},
		Policy: types.ReadDispatchPolicy{
			Active:       true,
			Action:       types.ReadDispatchPolicyActionAddProof,
			AllowedTools: []string{"read_file", "emit_evidence"},
			ScopePaths:   []string{"pkg/a.py"},
			MaxToolCalls: 2,
			OneShot:      true,
		},
	})
	active := readRunActiveStateFromGraphState(state)
	if !active.IsActive() || !active.TransientRetryPending || active.TransientRetryHintHash == "" || active.TransientRetryHintBytes != len([]byte(rawHint)) {
		t.Fatalf("active state missing typed retry audit: %+v", active)
	}
	if strings.Contains(active.TransientRetryHintHash, "raw retry") {
		t.Fatalf("active state must not store raw retry hint: %+v", active)
	}
	if active.ReadLoopNextAction.Action != types.ReadDispatchPolicyActionAddProof ||
		active.ReadDispatchPolicy.Action != types.ReadDispatchPolicyActionAddProof {
		t.Fatalf("active next-action/policy = %+v", active)
	}
}

func TestReadDispatchPolicyForNextActionCarriesAcceptedEvidenceScopes(t *testing.T) {
	mut := types.NewMutableState("weak proof with accepted evidence")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{ID: "ev-a", Source: "./pkg/a.py", GroundingStatus: types.GroundingGrounded},
	}})
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-b", Source: `pkg\b.py`, GroundingStatus: types.GroundingGrounded},
		{ID: "ev-a2", Source: "pkg/a.py", GroundingStatus: types.GroundingGrounded},
	})
	decision := readLoopNextActionDecision{
		Active:          true,
		Action:          loopkernel.LoopActionAddProof,
		ReasonCode:      "proof_weak",
		RouteSurface:    loopkernel.LoopToolSurfaceVerification,
		RouteReasonCode: "loop_tool_route_verification",
		ToolSuggestions: []string{"run_tests"},
	}
	policy := readDispatchPolicyForNextAction(decision, mut)
	if !policy.IsActive() || policy.Action != types.ReadDispatchPolicyActionAddProof {
		t.Fatalf("policy inactive: %+v", policy)
	}
	for _, want := range []string{"run_tests", "repo_map", "read_file", "grep", "emit_evidence", "emit_investigation_complete"} {
		if !containsString(policy.AllowedTools, want) {
			t.Fatalf("policy missing allowed tool %q: %+v", want, policy)
		}
	}
	for _, deny := range []string{"exec_command", "list_files"} {
		if !containsString(policy.DeniedTools, deny) {
			t.Fatalf("policy missing denied tool %q: %+v", deny, policy)
		}
	}
	if got, want := strings.Join(policy.ScopePaths, ","), "pkg/a.py,pkg/b.py"; got != want {
		t.Fatalf("policy scope paths = %s, want %s", got, want)
	}
}

func TestInstallReadDispatchPolicyForExploreTightensAndRestoresBudget(t *testing.T) {
	mut := types.NewMutableState("install policy")
	mut.SetExploreBudget(&types.ExploreBudget{
		PerToolCap:  map[string]int{"read_file": 8, "exec_command": 8},
		PerToolUsed: map[string]int{},
		OverallCap:  10,
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	restore := o.installReadDispatchPolicyForExplore(types.ReadDispatchPolicy{
		Active:       true,
		Action:       types.ReadDispatchPolicyActionAddProof,
		AllowedTools: []string{"read_file", "emit_evidence"},
		MaxToolCalls: 2,
		OneShot:      true,
	}, true)
	if !o.busCtx.ReadDispatchPolicy.IsActive() {
		t.Fatalf("policy not installed: %+v", o.busCtx.ReadDispatchPolicy)
	}
	if rem := mut.BudgetRemaining("read_file"); rem != 2 {
		t.Fatalf("read_file budget remaining = %d, want 2", rem)
	}
	restore()
	if o.busCtx.ReadDispatchPolicy.IsActive() {
		t.Fatalf("policy should restore to inactive: %+v", o.busCtx.ReadDispatchPolicy)
	}
	if rem := mut.BudgetRemaining("read_file"); rem != 8 {
		t.Fatalf("read_file budget after restore = %d, want 8", rem)
	}
}

func TestReadLoopAddProofActionSkipsCoveredProof(t *testing.T) {
	mut := types.NewMutableState("covered runtime proof")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedResultKind:               "resolved",
		RuntimeObservationOnlyCompletion: true,
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "answer",
			Value: "covered",
		}},
	})
	if got := readLoopAddProofActionSummaryFromMutable(mut); got != "" {
		t.Fatalf("covered proof should not request add_proof action, got %q", got)
	}
}

func TestBuildExploreFactRetryContinuationHintCarriesRuntimeFrontier(t *testing.T) {
	mut := types.NewMutableState("trace root cause")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: "[grep retrieval governor]\n" +
			"line_windows=record_trace.systrace:1102600-1102640(matches=4); record_trace.systrace:1139160-1139200(matches=3)\n" +
			"next_shape=single large runtime artifact matched too broadly; narrow with one exact timestamp/literal/thread id\n",
		Refinement: &types.ToolRefinementHint{
			ReasonCode:        "grep_result_truncated",
			ResultTruncated:   true,
			PreferredNextTool: "grep",
			PreferredParams: map[string]string{
				"read_file_path":        "record_trace.systrace",
				"read_file_line_offset": "1102602",
				"read_file_limit":       "41",
			},
		},
	})
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"record_trace.systrace"},
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "frontier",
			Value: "[GT]ColdPool#5-36624",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentRootCause,
			Scenario:   types.ScenarioRootCause,
			Complexity: types.ComplexityComplex,
		}},
	}}

	got := o.buildExploreFactRetryContinuationHint(&agent.StageOutput{
		ToolResults: []types.ToolResult{{
			ToolName: "grep",
			Success:  true,
			Summary:  "line_window_hint=first returned match is record_trace.systrace:1102623; next use read_file\n",
			Refinement: &types.ToolRefinementHint{
				ReasonCode:        "grep_result_truncated",
				ResultTruncated:   true,
				PreferredNextTool: "grep",
				PreferredParams: map[string]string{
					"read_file_path":        "record_trace.systrace",
					"read_file_line_offset": "1102602",
					"read_file_limit":       "41",
					"grep_line_start":       "1102603",
					"grep_line_end":         "1102643",
				},
			},
		}},
	})
	for _, want := range []string{
		exploreFactRetryCheckpointPrefix,
		"not a fresh investigation",
		"Runtime/log/trace continuation",
		"read_file_path=record_trace.systrace",
		"read_file_line_offset=1102602",
		"read_file_limit=41",
		"frontier=[GT]ColdPool#5-36624",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fact retry checkpoint missing %q:\n%s", want, got)
		}
	}
}

func TestBuildExploreFactRetryContinuationHintPrefersTypedRefinement(t *testing.T) {
	mut := types.NewMutableState("find owner")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreFactRetryContinuationHint(&agent.StageOutput{
		ToolResults: []types.ToolResult{{
			ToolName: "grep",
			Success:  true,
			Summary:  "line_window_hint=summary fallback should not render for this result\n",
			Refinement: &types.ToolRefinementHint{
				ReasonCode:        "grep_result_truncated",
				ResultTruncated:   true,
				PreferredNextTool: "repo_map",
				PreferredParams: map[string]string{
					"grep_line_end":         "1020",
					"grep_line_start":       "980",
					"query":                 "Owner",
					"read_file_limit":       "41",
					"read_file_line_offset": "979",
					"read_file_path":        "record_trace.systrace",
					"view":                  "task_map",
				},
				RequiredFields: []string{"query"},
			},
		}},
	})
	for _, want := range []string{
		"Useful preserved typed tool refinement hints:",
		"tool_refinement tool=grep reason=grep_result_truncated",
		"flags=result_truncated",
		"preferred_tool=repo_map",
		"query=Owner",
		"read_file_path=record_trace.systrace",
		"read_file_line_offset=979",
		"read_file_limit=41",
		"grep_line_start=980",
		"grep_line_end=1020",
		"required_fields=query",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed refinement checkpoint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "summary fallback should not render") {
		t.Fatalf("typed refinement should suppress same-result summary-prefix fallback:\n%s", got)
	}
}

func TestContinuationToolResultHintsIgnoresSummaryOnlyPrefixes(t *testing.T) {
	hints := continuationToolResultHints([]types.ToolResult{{
		ToolName: "grep",
		Success:  true,
		Summary:  "line_window_hint=first returned match is trace.systrace:42; next use read_file\n",
	}}, 4)
	if len(hints) != 0 {
		t.Fatalf("summary-only prefix should not drive retry continuation hints: %+v", hints)
	}
}

func TestContinuationToolResultHintsReadsHandoffRefinement(t *testing.T) {
	hints := continuationToolResultHints([]types.ToolResult{{
		ToolName: "list_files",
		Success:  true,
		Handoff: &types.ToolHandoffCarrier{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "list_files",
			ReasonCode: "list_files_result_truncated",
			Refinement: &types.ToolRefinementHint{
				ReasonCode:        "list_files_result_truncated",
				ResultTruncated:   true,
				PreferredNextTool: "list_files",
				PreferredParams: map[string]string{
					"include": "*.go",
					"path":    "internal",
				},
			},
		},
	}}, 4)
	if len(hints) != 1 ||
		!strings.Contains(hints[0], "tool_refinement tool=list_files") ||
		!strings.Contains(hints[0], "preferred_params=include=*.go,path=internal") {
		t.Fatalf("handoff refinement not rendered: %+v", hints)
	}
}

func TestSoftAgentOutputRetryMessageRuntimeContinuationLocalized(t *testing.T) {
	bus := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentRootCause,
			Scenario:   types.ScenarioRootCause,
			Complexity: types.ComplexityComplex,
		}},
	}
	got := softAgentOutputRetryMessage(bus, "zh", types.StageExplore, types.MissingFacts)
	if !strings.Contains(got, "继续上次调查") {
		t.Fatalf("runtime retry UX should indicate continuation, got %q", got)
	}
	if strings.Contains(got, "↻") {
		t.Fatalf("missing-facts continuation should use a muted glyph, got %q", got)
	}
	if kind := softAgentOutputRetryNoticeKind(types.StageExplore, types.MissingFacts); kind != render.NoticeInvestigationSupplement {
		t.Fatalf("missing-facts continuation notice kind = %v, want NoticeInvestigationSupplement", kind)
	}
}

func TestExploreRuntimeTraceContinuationLikelyUsesTypedToolSignals(t *testing.T) {
	bus := &types.BusContext{}
	if !exploreRuntimeTraceContinuationLikely(bus, []types.ToolResult{{
		ToolName: "grep",
		Success:  true,
		Refinement: &types.ToolRefinementHint{
			PreferredParams: map[string]string{
				"read_file_path": "record_trace.systrace",
			},
		},
	}}) {
		t.Fatal("typed runtime refinement path should trigger runtime continuation")
	}
	if !exploreRuntimeTraceContinuationLikely(bus, []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		}},
	}}) {
		t.Fatal("typed runtime observation should trigger runtime continuation")
	}
}

func TestExploreRuntimeTraceContinuationLikelyIgnoresSummaryOnlyPaths(t *testing.T) {
	bus := &types.BusContext{}
	if exploreRuntimeTraceContinuationLikely(bus, []types.ToolResult{{
		ToolName: "custom_tool",
		Success:  true,
		Summary:  "runtime/log/trace result mentions record_trace.systrace and file.log",
	}}) {
		t.Fatal("summary-only runtime path text must not trigger runtime continuation")
	}
}
