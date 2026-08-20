package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteControllerPromptConsumesTypedArtifactsAndAvoidsProseRouting(t *testing.T) {
	mut := types.NewMutableState("implement workflow")
	mut.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeCross,
				Summary: "ship workflow controller",
			},
			Risk: types.WriteRiskProfile{Overall: types.RiskBandMedium},
		},
	})
	mut.SetWriteContextPack(&types.WriteContextPack{
		PackID:  "pack-1",
		BatchID: "batch-1",
		Goal:    "stale plan summary that must not become remaining work",
		Items: []types.WriteContextItem{{
			Priority:  types.WriteContextP0,
			Kind:      "constraint",
			Text:      "preserve legacy write path",
			Consumers: []types.WriteContextConsumer{types.WriteConsumerController},
		}},
	})
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "first batch",
			Status: types.WriteWorkflowBatchNeedsExploration,
		}},
		Budget: types.WriteWorkflowBudget{MaxBatches: 5, MaxExplorationRounds: 2},
	})
	eval := &writeControllerEvaluator{}
	got := eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut, Mode: types.ModePlan}, nil)
	for _, want := range []string{
		"## Typed write task",
		"ship workflow controller",
		"## Workflow run state",
		"wf-1",
		"## Priority write context pack",
		"durable_batch_goal: first batch",
		"not proof of remaining work",
		"emit_write_workflow_decision",
		"typed action enum",
		"## Available controller actions",
		"mode: plan",
		"action enum: explore_code, plan_batch",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "- goal: stale plan summary") {
		t.Fatalf("controller prompt exposed plan summary as remaining-work goal:\n%s", got)
	}
	for _, unavailable := range []string{"apply_plan", "verify_batch"} {
		if strings.Contains(got, "action enum: "+unavailable) || strings.Contains(actionContractSection(got), unavailable) {
			t.Fatalf("controller prompt advertises mode-masked action %q:\n%s", unavailable, got)
		}
	}
	for _, banned := range []string{
		"keyword",
		"if the request says",
		"if the user says",
		"summary contains",
		"rationale contains",
		"parse prose",
	} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("controller prompt contains prose-routing smell %q:\n%s", banned, got)
		}
	}
}

func actionContractSection(prompt string) string {
	start := strings.Index(prompt, "## Available controller actions")
	if start < 0 {
		return ""
	}
	section := prompt[start:]
	if end := strings.Index(section[len("## Available controller actions"):], "\n\n## "); end >= 0 {
		return section[:len("## Available controller actions")+end]
	}
	return section
}

func TestWriteControllerActionContractUsesModeProjectedSchemaAuthority(t *testing.T) {
	plan := renderWriteControllerActionContract(&types.AgentContext{Mode: types.ModePlan})
	for _, want := range []string{"mode: plan", "explore_code", "plan_batch", "finish", "block"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan action contract missing %q:\n%s", want, plan)
		}
	}
	for _, banned := range []string{"apply_plan", "verify_batch"} {
		if strings.Contains(plan, banned) {
			t.Fatalf("plan action contract contains masked %q:\n%s", banned, plan)
		}
	}

	apply := renderWriteControllerActionContract(&types.AgentContext{Mode: types.ModeApply})
	for _, want := range []string{"mode: apply", "plan_batch", "apply_plan", "verify_batch"} {
		if !strings.Contains(apply, want) {
			t.Fatalf("apply action contract missing %q:\n%s", want, apply)
		}
	}
}

func TestWriteControllerActionContractUsesReadyToPlanStateProjection(t *testing.T) {
	mut := types.NewMutableState("plan a proof")
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID: "wf-ready", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-proof",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-proof", Purpose: "verification_proof_followup",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	})
	section := renderWriteControllerActionContract(&types.AgentContext{Mode: types.ModeApply, Mutable: mut})
	for _, want := range []string{"mode: apply", "explore_code", "plan_batch"} {
		if !strings.Contains(section, want) {
			t.Fatalf("ready_to_plan action contract missing %q:\n%s", want, section)
		}
	}
	for _, masked := range []string{"apply_plan", "verify_batch"} {
		if strings.Contains(section, masked) {
			t.Fatalf("ready_to_plan action contract exposed impossible %q:\n%s", masked, section)
		}
	}
}

func TestWriteControllerTaskSectionUsesActiveReplanContractGeneration(t *testing.T) {
	mut := types.NewMutableState("repair remaining expression")
	mut.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task:             types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "repair callback handling"},
		ExpectedOutcomes: []string{"only one expression changes"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Operator: types.WriteBehaviorOpSatisfies,
			Expected: "only one expression changes",
			Required: true,
			Source:   types.WriteBehaviorContractSourceExpectedOutcomeFallback,
		}},
	}})
	mut.SetChangePlan(&types.ChangePlan{
		ID:                         "plan-repair",
		AcceptanceTests:            []string{"both negative paths pass", "only repository.c changes"},
		BehaviorContractGeneration: types.WriteBehaviorContractGenerationPlanAcceptanceRebase,
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     types.WriteBehaviorObservable,
			Operator: types.WriteBehaviorOpSatisfies,
			Expected: "both negative paths pass",
			Required: true,
			Source:   types.WriteBehaviorContractSourcePlanAcceptanceFallback,
		}},
	})

	got := renderWriteControllerTaskSection(&types.AgentContext{Mutable: mut})
	for _, want := range []string{
		"behavior_contract_generation: plan_acceptance_rebase",
		"analyzer_proposed_outcomes (planning_only; request/preserve constraints/source/tests take precedence): both negative paths pass | only repository.c changes",
		"expected=both negative paths pass",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller task section missing current replan authority %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "only one expression changes") {
		t.Fatalf("controller task section retained superseded analyzer fallback:\n%s", got)
	}
}

func TestWriteControllerParseOutputReadsStoredDecisionJSON(t *testing.T) {
	mut := types.NewMutableState("implement workflow")
	mut.SetWriteWorkflowDecisionJSON([]byte(`{"action":"finish","reason_code":"done"}`))
	eval := &writeControllerEvaluator{}
	out, err := eval.ParseOutput(&types.AgentContext{Mutable: mut}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.Error != "" {
		t.Fatalf("ParseOutput returned error: %s", out.Error)
	}
	if !strings.Contains(out.StageReport, "Action: finish") {
		t.Fatalf("StageReport missing action: %s", out.StageReport)
	}
	if string(out.Data) != `{"action":"finish","reason_code":"done"}` {
		t.Fatalf("Data should preserve stored normalized JSON, got %s", out.Data)
	}
}

func TestWriteControllerSoftStopRetainsTypedContextForRequiredToolCorrection(t *testing.T) {
	eval := &writeControllerEvaluator{}
	sig := eval.Observe(nil, LoopObservation{
		Phase: PhaseSoftStop,
		Response: llm.Response{
			Content:    strings.Repeat("discarded draft ", 100),
			StopReason: "length",
		},
	})
	if !sig.HintRequested || sig.HintKey != "write-controller.required-tool.length-no-tool" {
		t.Fatalf("length/no-tool response must request the typed correction, got %+v", sig)
	}
	if sig.IsolateNextPrompt || !sig.BypassThrottle || !sig.BypassBudget {
		t.Fatalf("length/no-tool correction must retain typed context and bypass ordinary hint budgets, got %+v", sig)
	}
	for _, want := range []string{"current typed workflow state", "available action enum", "emit_write_workflow_decision exactly once"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("length/no-tool correction missing %q: %s", want, sig.Hint)
		}
	}
	if strings.Contains(sig.Hint, "discarded draft discarded draft") {
		t.Fatalf("isolated correction must not copy response prose: %s", sig.Hint)
	}

	ordinary := eval.Observe(nil, LoopObservation{Phase: PhaseSoftStop, Response: llm.Response{Content: "analysis", StopReason: "end_turn"}})
	if ordinary.HintKey != "write-controller.required-tool.no-tool" || ordinary.IsolateNextPrompt {
		t.Fatalf("ordinary no-tool controller response must retain the same typed-context recovery boundary, got %+v", ordinary)
	}
}

type writeControllerRetryCaptureLLM struct {
	calls    int
	messages [][]llm.Message
}

func (l *writeControllerRetryCaptureLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.messages = append(l.messages, append([]llm.Message(nil), messages...))
	if l.calls == 1 {
		return llm.Response{Content: strings.Repeat("discarded controller reasoning ", 200)}, nil
	}
	return llm.Response{}, nil
}

func (*writeControllerRetryCaptureLLM) ModelID() string               { return "write-controller-retry-capture" }
func (*writeControllerRetryCaptureLLM) MaxContextTokens() int         { return 128000 }
func (*writeControllerRetryCaptureLLM) MaxOutputTokens() int          { return 4096 }
func (*writeControllerRetryCaptureLLM) RequestTimeout() time.Duration { return 0 }
func (*writeControllerRetryCaptureLLM) RetryMaxAttempts() int         { return 0 }

func TestWriteControllerNoToolRetryKeepsTypedStateAndCompactsDiscardedReasoning(t *testing.T) {
	mut := types.NewMutableState("retain exact typed task marker")
	mut.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "repair typed retry context"},
	}})
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-retry-context",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1", Goal: "plan from retained state", Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	})
	llmStub := &writeControllerRetryCaptureLLM{}
	agent := NewBaseAgent(types.AgentWriteController, &Dependencies{
		LLM: llmStub, MaxIterations: 2,
	}, &writeControllerEvaluator{})
	_, _ = agent.Execute(&types.AgentContext{
		Stage: types.StageWriteController, Mode: types.ModeApply, Objective: "retain exact typed task marker", Mutable: mut,
	}, &skill.Config{})
	if len(llmStub.messages) != 2 {
		t.Fatalf("expected one no-tool retry, got calls=%d", len(llmStub.messages))
	}
	var second strings.Builder
	for _, message := range llmStub.messages[1] {
		second.WriteString(message.Content)
		second.WriteByte('\n')
	}
	got := second.String()
	for _, want := range []string{
		"retain exact typed task marker",
		"wf-retry-context",
		"action enum:",
		"emit_write_workflow_decision exactly once",
		"protocol-only text omitted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retry lost typed context or protocol marker %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "discarded controller reasoning discarded controller reasoning") {
		t.Fatalf("retry replayed discarded controller prose instead of the bounded marker:\n%s", got)
	}
}

func TestWriteControllerPromptHasBoundedExplorationHandoffReceipt(t *testing.T) {
	mut := types.NewMutableState("plan from explored files")
	mut.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:         "batch-7",
		TargetFiles:     []string{"a.py", "b.py", "c.py", "d.py", "e.py"},
		RelevantSymbols: []string{"build_delta", "normalize"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{
			{ID: "E1", Source: "a.py", LineStart: 10},
			{ID: "E2", Source: "b.py", LineStart: 20},
		},
		Unknowns:   []string{"runtime version"},
		Confidence: "high",
	})
	got := renderWriteControllerArtifactSection(&types.AgentContext{Mutable: mut})
	for _, want := range []string{
		"exploration_handoff: status=present batch_id=batch-7 target_files=5 symbols=2 evidence_refs=2 unknowns=1 confidence=high",
		"exploration_target_file: a.py",
		"exploration_target_file: d.py",
		"exploration_target_file: ... +1 more",
		"context-pack compaction does not mean exploration is absent",
		"controller must still choose the next typed action itself",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed handoff receipt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "exploration_target_file: e.py") {
		t.Fatalf("handoff receipt must stay bounded to four concrete target rows:\n%s", got)
	}
}

func TestWriteControllerPromptFiltersPlannerProbeReports(t *testing.T) {
	mut := types.NewMutableState("stale probe")
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-probe",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-current",
		}},
	})
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-current", Status: types.PlanStatusPending})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID:         "plan-current",
		Channel:        types.ChangeReportChannelPlannerProbe,
		Passed:         false,
		FailureSummary: "stale dry-run failure",
	})
	eval := &writeControllerEvaluator{}
	got := eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	if strings.Contains(got, "stale dry-run failure") || strings.Contains(got, "change_report:") {
		t.Fatalf("planner probe report must not render as authoritative controller artifact:\n%s", got)
	}

	mut.SetChangeReport(&types.ChangeReport{
		PlanID:         "plan-current",
		Channel:        types.ChangeReportChannelPostApplyVerify,
		Passed:         true,
		FailureSummary: "",
	})
	got = eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	if !strings.Contains(got, "change_report: plan_id=plan-current channel=post_apply_verify passed=true") {
		t.Fatalf("post-apply report should render as authoritative controller artifact:\n%s", got)
	}
}

func TestWriteControllerPromptPreservesPartialVerificationEvidence(t *testing.T) {
	mut := types.NewMutableState("partial verification")
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-partial",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-partial",
		}},
	})
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-partial", Status: types.PlanStatusUnverified})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID:             "plan-partial",
		Channel:            types.ChangeReportChannelPostApplyVerify,
		Passed:             false,
		VerificationStatus: types.VerificationStatusUnavailable,
		FailureKind:        types.FailureKindRunnerMissing,
		FailureSummary:     "Java runtime unavailable",
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindUnit,
			AssertionID: "make-test",
			Suite:       "check",
			Passed:      true,
		}},
	})

	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		"verification_evidence: status=unavailable passed_results=1 failed_results=0 total_results=1",
		"passed_results are retained partial evidence",
		"Java runtime unavailable",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost partial verification evidence %q:\n%s", want, got)
		}
	}
}

func TestWriteControllerPromptScopesAllVerifiedToTypedObligations(t *testing.T) {
	mut := types.NewMutableState("verify exact behavior without upgrading planning prose")
	mut.SetChangePlan(&types.ChangePlan{
		ID:              "plan-proof-scope",
		Status:          types.PlanStatusApplied,
		AcceptanceTests: []string{"single boundary remains unchanged", "five values collapse"},
		BehaviorContracts: []types.WriteBehaviorContract{
			{
				ID: "exact-collapse", Kind: types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpEquals, Expected: "five values collapse",
				Required: true, Source: "write_analyzer",
			},
			{
				ID: "candidate-single", Kind: types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpSatisfies, Expected: "single boundary remains unchanged",
				Source: "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded,
			},
		},
	})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-proof-scope", Passed: true, VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
			ContractRefs: []string{"exact-collapse"},
		}},
	})

	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	want := "verification_completion_scope: required_typed_contracts=1 covered_required_typed_contracts=1 planning_only_contracts=1 natural_language_acceptance_items=2 all_verified_applies_to=required_typed_obligations acceptance_items_authority=planning_guidance_only"
	if !strings.Contains(got, want) {
		t.Fatalf("controller prompt lost exact verification authority scope %q:\n%s", want, got)
	}
}

func TestWriteControllerPromptDisclosesChangedPathCapabilityBoundary(t *testing.T) {
	mut := types.NewMutableState("static path verification")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-static", Status: types.PlanStatusApplied})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-static", Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "make-check", Passed: true}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
			Path: "src/widget.ts", Status: types.ChangedPathVerificationCovered,
			Caliber:    types.ChangedPathVerificationDeclaredProjectCheck,
			Capability: types.VerificationCapabilitySourceStatic,
		}},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		"changed_path_verification: path=src/widget.ts status=covered caliber=declared_project_check capability=source_static",
		"source_static/syntax_only coverage proves source shape only",
		"do not select all_verified from report passed status alone",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost typed path capability %q:\n%s", want, got)
		}
	}
}

func TestWriteControllerPromptCarriesExecutedCommandWithoutUpgradingCapability(t *testing.T) {
	mut := types.NewMutableState("aggregate verification command")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-command", Status: types.PlanStatusApplied})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-command", Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "make-test", Passed: true}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check", Command: "make check", ExitCode: 0,
			Outcome: "executed", Source: "declared_coverage_test_surface",
		}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
			Path: "src/widget.ts", Status: types.ChangedPathVerificationCovered,
			Caliber:    types.ChangedPathVerificationDeclaredProjectCheck,
			Capability: types.VerificationCapabilitySourceStatic,
		}},
	})

	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		`verification_command: runner=make cwd=. suite=check outcome=executed exit_code=0 source=declared_coverage_test_surface command="make check"`,
		"total_results counts top-level runner results, not nested Make/Gradle/npm subcommands",
		"do not infer that a declared sub-check was skipped solely because total_results is smaller",
		"source_static/syntax_only coverage proves source shape only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost typed command accounting boundary %q:\n%s", want, got)
		}
	}
}

func TestRenderWriteControllerRunSection_CanonicalAttemptState(t *testing.T) {
	mu := types.NewMutableState("canonical state")
	run := types.WriteWorkflowRun{
		RunID:         "wf-canon",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "repair verify",
			Status: types.WriteWorkflowBatchReadyToPlan,
			PlanID: "plan-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", ReportID: "plan-1.report.json"},
			},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			Status:     "progress",
			ReasonCode: "verify_failed",
			Message:    "verify failed: red tests",
		}},
	}
	mu.SetWriteWorkflowRun(&run)
	ctx := &types.AgentContext{Mutable: mu}
	section := renderWriteControllerRunSection(ctx)
	if !strings.Contains(section, "state=needs_replan") {
		t.Fatalf("active batch must render the canonical derived phase, got:\n%s", section)
	}
	if !strings.Contains(section, "cause=tests_failed") {
		t.Fatalf("derived phase must carry the typed cause, got:\n%s", section)
	}
	if !strings.Contains(section, "failed_verify_attempts=1") {
		t.Fatalf("failed verify attempt count missing:\n%s", section)
	}
	if strings.Contains(section, "state=ready_to_plan") {
		t.Fatalf("contradictory ready_to_plan reading must not render for a failed-verify batch:\n%s", section)
	}
	if !strings.Contains(section, "last_event: batch=batch-1 event=verify_failed") {
		t.Fatalf("progress should render as a labeled event, got:\n%s", section)
	}
}
