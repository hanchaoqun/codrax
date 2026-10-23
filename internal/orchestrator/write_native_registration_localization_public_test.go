package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Reproduce the real planner's legal source + test reads, followed by an
// offered assertion_ref. The old test-only adapter did not exercise the
// production read anchor that incorrectly reopened source-owner obligations.
type registrationSourceReadLLM struct{ controllerRegistrationLLM }

func (l *registrationSourceReadLLM) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	response, err := l.controllerRegistrationLLM.Chat(ctx, messages, schemas, opts)
	if err != nil {
		return response, err
	}
	if l.round == 1 {
		response.ToolCalls = append([]llm.ToolCall{{ID: "read-source", Name: "read_file", Params: json.RawMessage(`{"path":"value.py"}`)}}, response.ToolCalls...)
		return response, nil
	}
	delivered := false
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == "read-source" && strings.Contains(message.Content, "showing lines 1-2 of 2 total") && strings.Contains(message.Content, "return value + 1") {
			delivered = true
		}
	}
	if !delivered {
		l.t.Fatal("complete current source read did not reach the next real planner request")
	}
	ref := ""
	for _, schema := range schemas {
		if schema.Name != l.entry {
			continue
		}
		var shape struct {
			Properties map[string]struct {
				Items struct {
					Properties map[string]struct {
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema.Parameters, &shape); err != nil {
			l.t.Fatal(err)
		}
		choices := shape.Properties["project_test_observations"].Items.Properties["assertion_ref"].Enum
		if len(choices) != 1 {
			l.t.Fatalf("expected one current source assertion choice, got %v", choices)
		}
		ref = choices[0]
	}
	if ref == "" {
		l.t.Fatal("actual model schema did not offer the retained assertion")
	}
	params, err := json.Marshal(map[string]any{
		"request": "Verify retained source without another edit", "summary": "Register the fully read existing assertion and execute it again.", "changes": []any{},
		"project_test_observations": []map[string]any{{"id": "existing-increment", "test_path": "test_value.py", "assertion_ref": ref, "contract_refs": []string{"increment-result"}}},
	})
	if err != nil {
		l.t.Fatal(err)
	}
	response.ToolCalls[0].Params = params
	return response, nil
}

func registrationSourceReadFixture(t *testing.T, entry string) controllerRegistrationFixture {
	t.Helper()
	f := newControllerRegistrationFixture(t)
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	registry.Register(&tool.EmitPlanSkeleton{})
	adapter := &registrationSourceReadLLM{controllerRegistrationLLM{t: t, entry: entry}}
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: adapter, MaxIterations: 12, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	planningSkill, err := skills.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	selectedSkill := *planningSkill
	selectedSkill.ToolSuggestions = []string{"read_file", entry}
	runNativeRegistrationTerminalPlanning(t, f, planner, &selectedSkill, nil)
	if adapter.round != 2 {
		t.Fatalf("unexpected planner turn count %d", adapter.round)
	}
	return f
}

func TestNativeRegistrationSourceReadRestoredTerminalPublic(t *testing.T) {
	// Match the actual live planner entry. The existing controller public
	// round-trip separately retains both emitter compatibility controls.
	f := registrationSourceReadFixture(t, "emit_change_plan")
	f.restartExpectingSourceTerminal(t, true)
}

func TestNativeRegistrationSourceReadLocalizationBoundariesPublic(t *testing.T) {
	f := registrationSourceReadFixture(t, "emit_change_plan")
	mu := f.o.busCtx.Mutable
	original, err := json.Marshal(struct {
		Run  *types.WriteWorkflowRun
		Plan *types.ChangePlan
	}{mu.WriteWorkflowRun(), mu.ChangePlan()})
	if err != nil {
		t.Fatal(err)
	}
	for _, condition := range []string{
		"valid", "ordinary_verify_only", "missing_receipt", "forged_receipt", "duplicate_receipt", "different_run", "different_root", "missing_identity", "different_batch",
		"wrong_source", "wrong_sha", "wrong_apply_receipt", "missing_source_terminal", "unverified_source_owner", "non_native_terminal", "missing_verify_ref", "missing_verify_attempt", "wrong_verify_plan", "wrong_verify_report", "later_failed_verify",
		"extra_applied_source", "conflicting_source_generation", "duplicate_source_batch", "extra_expected_owner", "extra_scope_owner", "explicit_current_owner_gap", "new_source_change", "changed_contract", "changed_test_version", "unapplied_active_slice", "dangling_active_slice", "dangling_source_slice", "extra_slice_apply_started", "extra_slice_apply_complete", "slice_same_source_ref_without_attempt",
	} {
		t.Run(condition, func(t *testing.T) {
			var fixture struct {
				Run  *types.WriteWorkflowRun
				Plan *types.ChangePlan
			}
			if err := json.Unmarshal(original, &fixture); err != nil {
				t.Fatal(err)
			}
			run, plan := fixture.Run, fixture.Plan
			var source, active *types.WriteWorkflowBatch
			for i := range run.Batches {
				batch := &run.Batches[i]
				if batch.ID == run.ActiveBatchID {
					active = batch
				}
				if batch.ID == "source" {
					source = batch
				}
			}
			if source == nil || active == nil {
				t.Fatal("public source/registration batches missing")
			}
			switch condition {
			case "ordinary_verify_only":
				plan.PersistenceKind, plan.NativeTestRegistration = "", nil
			case "missing_receipt":
				run.NativeTestRegistrations = nil
			case "forged_receipt":
				plan.NativeTestRegistration.Digest = strings.Repeat("a", 64)
				run.NativeTestRegistrations[0].Digest = plan.NativeTestRegistration.Digest
			case "duplicate_receipt":
				run.NativeTestRegistrations = append(run.NativeTestRegistrations, run.NativeTestRegistrations[0])
			case "different_run":
				run.RunID = "another-run"
			case "different_root":
				run.Identity.CanonicalRepoRoot += "/other"
			case "missing_identity":
				run.Identity = nil
			case "different_batch":
				active.ID, run.ActiveBatchID = "another-batch", "another-batch"
			case "wrong_source":
				source.PlanID = "foreign-source"
			case "wrong_sha":
				source.ApplyRef = strings.Repeat("a", 40)
			case "wrong_apply_receipt":
				source.Attempts[0].ArtifactRef = strings.Repeat("b", 40)
			case "missing_source_terminal":
				source.Completion = nil
			case "unverified_source_owner":
				source.Completion.Verdict, source.Completion.ReasonCode = types.WriteWorkflowCompletionUnverified, "localization_observed_without_owner"
			case "non_native_terminal":
				source.Completion.Source = "model_claim"
			case "missing_verify_ref":
				source.VerifyRef = ""
			case "missing_verify_attempt":
				source.Attempts = source.Attempts[:1]
			case "wrong_verify_plan":
				source.Attempts[len(source.Attempts)-1].PlanID = "another-plan"
			case "wrong_verify_report":
				source.Attempts[len(source.Attempts)-1].ReportID = "another-report"
			case "later_failed_verify":
				source.Attempts = append(source.Attempts, types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", PlanID: source.PlanID, ReportID: source.VerifyRef})
			case "extra_applied_source":
				run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "other-source", PlanID: "other-plan", ApplyRef: f.head, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "other-plan", ArtifactRef: f.head}}})
			case "conflicting_source_generation":
				source.Attempts = append(source.Attempts, types.WriteWorkflowAttempt{Kind: "apply", Status: "applied", PlanID: source.PlanID, ArtifactRef: strings.Repeat("b", 40)})
			case "duplicate_source_batch":
				duplicate := *source
				duplicate.ID = "duplicate-source"
				run.Batches = append(run.Batches, duplicate)
			case "extra_expected_owner":
				active.ExpectedPaths = append(active.ExpectedPaths, "other_owner.py")
			case "extra_scope_owner":
				run.ContextPacks = append(run.ContextPacks, types.WriteContextPack{BatchID: run.ActiveBatchID, Items: []types.WriteContextItem{{Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{Path: "other_owner.py", Role: types.SourcePathRoleProduction, Kind: types.SourceLocalizationAnchorScope, Strength: types.SourceLocalizationAnchorSupporting}}}})
			case "explicit_current_owner_gap":
				plan.LocalizationReview = &types.SourceLocalizationReview{PlanID: plan.ID, BatchID: run.ActiveBatchID, SourcePaths: []string{"value.py"}, OwnerMissingPaths: []string{"value.py"}}
			case "new_source_change":
				plan.Changes = []types.FileChange{{Path: "value.py", Kind: "modify", NewContent: "changed"}}
			case "changed_contract":
				plan.BehaviorContracts[0].Expected = "increment(4) == 99"
			case "changed_test_version":
				plan.NativeTestRegistration.Tests[0].SHA256 = strings.Repeat("a", 64)
			case "unapplied_active_slice":
				active.Slices = []types.WriteWorkflowSlice{{ID: "extra-slice", PlanID: plan.ID, Paths: []string{"other_owner.py"}}}
			case "dangling_active_slice":
				active.ActiveSliceID = "missing-slice"
			case "dangling_source_slice":
				source.ActiveSliceID = "missing-slice"
			case "extra_slice_apply_started", "extra_slice_apply_complete":
				status := "started"
				if condition == "extra_slice_apply_complete" {
					status = "applied"
				}
				run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "other-source", Slices: []types.WriteWorkflowSlice{{ID: "unsettled-slice", Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: status, PlanID: "other-source", ArtifactRef: f.head}}}}})
			case "slice_same_source_ref_without_attempt":
				run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "other-slice", Slices: []types.WriteWorkflowSlice{{ID: "same-source-ref", PlanID: source.PlanID, ApplyRef: f.head}}})
			}
			before, _ := json.Marshal(fixture)
			got := loopkernel.WorkflowRegistrationReusesVerifiedSource(*run, plan)
			if got != (condition == "valid") {
				t.Fatalf("reused source=%t for %s", got, condition)
			}
			view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, plan)
			if condition == "valid" || condition == "missing_source_terminal" {
				if view.Localization.State != "" || view.Localization.RequiresMoreContext || view.LocalizationGateEligible {
					t.Fatalf("read-only verification acquired a new source-owner obligation: %+v", view.Localization)
				}
				legacy := loopkernel.LocalizationReviewFromWriteWorkflowRun(*run, run.ActiveBatchID)
				if legacy == nil || len(legacy.Anchors) == 0 || loopkernel.DeriveLocalizationAuthority(legacy).State != loopkernel.LocalizationAuthorityObservedOnly {
					t.Fatalf("actual read anchors disappeared or were promoted to owner: %+v", legacy)
				}
			} else if condition != "different_batch" && !view.Localization.RequiresMoreContext {
				t.Fatalf("real or unauthenticated owner obligation disappeared: %+v", view.Localization)
			}
			after, _ := json.Marshal(fixture)
			if string(before) != string(after) {
				t.Fatal("projection rewrote registration/source completion or retained read anchors")
			}
		})
	}
}

func TestNativeRegistrationSourceReadFailedChildAuditPublic(t *testing.T) {
	f := registrationSourceReadFixture(t, "emit_change_plan")
	o, mu := f.o, f.o.busCtx.Mutable
	run, plan := mu.WriteWorkflowRun(), mu.ChangePlan()
	// A real unresolved source-owner verdict must not be laundered by the
	// later registered test pass. Keep the actual report after this child Fatal.
	for i := range run.Batches {
		if run.Batches[i].ID == "source" {
			run.Batches[i].Completion.Verdict = types.WriteWorkflowCompletionUnverified
			run.Batches[i].Completion.ReasonCode = "localization_observed_without_owner"
		}
	}
	if _, err := repl.NewWriteWorkflowRunStore(o.ensureChangeReportDir()).Save(run); err != nil {
		t.Fatal(err)
	}
	input := registrationRestartInput{Root: o.busCtx.RepoRoot, WorkDir: o.busCtx.WorkDir, Objective: mu.Objective(), PlanID: plan.ID, SourceID: f.source.ID, Head: f.head, OldInvocation: f.oldInvocation, ParentPID: os.Getpid(), SourceTerminal: true}
	if prior := o.loadWriteFinalReportChangeReport(f.source.ID); prior != nil {
		for _, command := range prior.ExecutedCommands {
			if command.InvocationID != "" {
				input.PriorInvocations = append(input.PriorInvocations, command.InvocationID)
			}
		}
	}
	body, _ := json.Marshal(input)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestNativeRegistrationFreshProcessHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "CODRAX_REGISTRATION_RESTART_INPUT="+string(body))
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "new proof batch not verified") || !strings.Contains(string(output), "localization_observed_without_owner") {
		t.Fatalf("source owner gap did not remain a real child failure: %v\n%s", err, output)
	}
	out := t.TempDir()
	preserveRegistrationRestartAudit(t, input.WorkDir, out)
	var childPlan types.ChangePlan
	var childReport types.ChangeReport
	var childRun types.WriteWorkflowRun
	var audit struct {
		PID                int      `json:"pid"`
		ParentPID          int      `json:"parent_pid"`
		Failed             bool     `json:"test_failed"`
		Independent        bool     `json:"independent_process"`
		ReadonlyChecked    bool     `json:"readonly_checked"`
		Readonly           bool     `json:"readonly"`
		CurrentInvocations []string `json:"current_invocations"`
	}
	for i, target := range []any{&childPlan, &childReport, &childRun, &audit} {
		body, err := os.ReadFile(filepath.Join(out, registrationRestartAuditFiles[i]))
		if err != nil || json.Unmarshal(body, target) != nil {
			t.Fatalf("parent did not retain actual child artifact %s: %v", registrationRestartAuditFiles[i], err)
		}
	}
	if childPlan.ID != plan.ID || childReport.PlanID != plan.ID || !childReport.Passed || childRun.Completion == nil || childRun.Completion.Verdict != types.WriteWorkflowCompletionUnverified ||
		!audit.Failed || !audit.Independent || audit.PID == os.Getpid() || audit.ParentPID != os.Getpid() || !audit.ReadonlyChecked || !audit.Readonly || len(audit.CurrentInvocations) != 1 {
		t.Fatalf("failed child audit was stale, incomplete or mislabeled: report=%+v completion=%+v audit=%+v", childReport, childRun.Completion, audit)
	}
	for _, previous := range append(input.PriorInvocations, f.oldInvocation) {
		if audit.CurrentInvocations[0] == previous {
			t.Fatal("failed child audit contains an old invocation")
		}
	}
	if types.BuildVerificationProofLedger(&childPlan, &childReport, nil).State != types.VerificationProofLedgerVerified {
		t.Fatal("expected retained fresh native proof, independently from source-owner completion")
	}
}
