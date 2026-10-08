package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func currentBatchPromptFixture() *types.MutableState {
	mu := types.NewMutableState("repair source and verify")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "original project objective"}},
		PhaseProposal: types.PhaseProposal{Split: "sequential", Phases: []types.PhaseSeed{
			{Goal: "obsolete initial implementation", RoughTargetPaths: []string{"old.py"}},
		}},
	})
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID: "current-run", Goal: "durable project objective", Status: types.WriteWorkflowRunInProgress,
		ActiveBatchID: "proof-2", Batches: []types.WriteWorkflowBatch{
			{ID: "batch-1", Goal: "historical implementation", Status: types.WriteWorkflowBatchComplete},
			{ID: "proof-2", Goal: "check current delivery", Purpose: "verification_proof_followup",
				Status: types.WriteWorkflowBatchReadyToPlan, ExpectedPaths: []string{"current.py"},
				SuccessCriteria: []string{"criterion-current"}, ActiveSliceID: "slice-current"},
		},
	})
	mu.SetWriteContextPack(&types.WriteContextPack{PackID: "old-pack", BatchID: "batch-1", Goal: "obsolete pack goal",
		Items: []types.WriteContextItem{{ID: "shared", Kind: "constraint", Priority: types.WriteContextP0,
			Text: "preserve independent evidence", SourceStage: "analysis"}},
	})
	return mu
}

func TestCurrentWriteBatchPromptReplacesInitialSeed(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "restored"}[restore], func(t *testing.T) {
			mu := currentBatchPromptFixture()
			if restore {
				body, _ := json.Marshal(mu.WriteWorkflowRun())
				var run types.WriteWorkflowRun
				if err := json.Unmarshal(body, &run); err != nil {
					t.Fatal(err)
				}
				mu.SetWriteWorkflowRun(&run)
			}
			before, _ := json.Marshal(mu.WriteWorkflowRun())
			ctx := &types.AgentContext{Mutable: mu}
			got := newPlannerEvaluatorForTest(t).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"## Current write batch", "current-run", "proof-2", "check current delivery", "phase: ready_to_plan", "current.py", "criterion-current"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in prompt:\n%s", want, got)
				}
			}
			for _, stale := range []string{"next_batch:", "obsolete initial implementation", "obsolete pack goal"} {
				if strings.Contains(got, stale) {
					t.Errorf("stale instruction %q in prompt", stale)
				}
			}
			after, _ := json.Marshal(mu.WriteWorkflowRun())
			if string(before) != string(after) {
				t.Fatal("presentation mutated durable run")
			}
		})
	}
}

func TestCurrentWriteContextPackHeadersUseCurrentScope(t *testing.T) {
	mu := currentBatchPromptFixture()
	ctx := &types.AgentContext{Mutable: mu}
	before, _ := json.Marshal(mu.WriteContextPack())
	for _, consumer := range []types.WriteContextConsumer{types.WriteConsumerPlanner, types.WriteConsumerVerifier, types.WriteConsumerController} {
		got := buildWriteContextPackPromptSection(ctx, consumer, "", 20)
		for _, want := range []string{"batch_id: proof-2", "check current delivery", "source_pack_batch_id: batch-1", "preserve independent evidence"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s missing %q: %s", consumer, want, got)
			}
		}
		if strings.Contains(got, "obsolete pack goal") {
			t.Errorf("%s revives stale goal", consumer)
		}
	}
	after, _ := json.Marshal(mu.WriteContextPack())
	if string(before) != string(after) {
		t.Fatal("presentation mutated stored context pack")
	}
}

func TestCurrentWriteBatchPromptCanonicalStateAndMissingActive(t *testing.T) {
	for _, tc := range []struct {
		name, active string
		status       types.WriteWorkflowBatchStatus
		failed       bool
		want         string
	}{
		{"replan", "proof-2", types.WriteWorkflowBatchReadyToPlan, true, "phase: needs_replan"},
		{"complete", "proof-2", types.WriteWorkflowBatchComplete, false, "phase: complete"},
		{"blocked", "proof-2", types.WriteWorkflowBatchBlocked, false, "phase: blocked"},
		{"missing", "absent", types.WriteWorkflowBatchReadyToPlan, false, "active_batch_unavailable"},
		{"empty", "", types.WriteWorkflowBatchReadyToPlan, false, "active_batch_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := currentBatchPromptFixture()
			run := mu.WriteWorkflowRun()
			run.ActiveBatchID = tc.active
			run.Batches[1].Status = tc.status
			if tc.failed {
				run.Batches[1].Attempts = []types.WriteWorkflowAttempt{{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"}}
			}
			mu.SetWriteWorkflowRun(run)
			got := (&plannerEvaluator{}).buildWorkflowSeedSection(&types.AgentContext{Mutable: mu})
			if !strings.Contains(got, tc.want) || strings.Contains(got, "next_batch:") {
				t.Fatalf("wrong current view: %s", got)
			}
		})
	}
}

func TestCurrentWriteProofTeachingFollowsDispatchAuthorization(t *testing.T) {
	mu := currentBatchPromptFixture()
	run := mu.WriteWorkflowRun()
	run.ProgressLedger = []types.WriteWorkflowProgress{{BatchID: "batch-1", ReasonCode: "verification_proof_followup_requested"}}
	mu.SetWriteWorkflowRun(run)
	e := newPlannerEvaluatorForTest(t)
	ctx := &types.AgentContext{Mutable: mu}
	section := func() string { return e.buildProofFollowupMaterializationSection(ctx) }
	if got := section(); !strings.Contains(got, "verification_probes[]") || strings.Contains(got, "permits read-only existing-test registration") {
		t.Fatalf("unauthorized teaching: %s", got)
	}
	sha := strings.Repeat("a", 40)
	delivery := types.VerificationDeliverySnapshot{SourcePlanID: "applied", AppliedCommitSHA: sha, PatchEffect: &types.PatchEffectRecord{PlanID: "applied", RecordID: "effect", Source: "applied_commit", HeadRef: sha, DiffFingerprint: strings.Repeat("b", 64)}}
	if err := mu.AuthorizeNativeTestRegistration(t.TempDir(), delivery, []types.WriteBehaviorContract{{ID: "criterion-current"}}, []string{"current.py"}); err != nil {
		t.Fatal(err)
	}
	got := section()
	for _, want := range []string{"permits read-only existing-test registration", "test_path, existing contract_refs, and an offered assertion_ref or the exact assertion_suite/assertion_id pair", "verification must execute these exact tests again", "do not combine these two plan shapes", "changes: []"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	// Persisted run/context prose must never recreate the dispatch-only grant.
	encoded, _ := json.Marshal(run)
	var restored types.WriteWorkflowRun
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable = types.NewMutableState("restored")
	ctx.Mutable.SetWriteWorkflowRun(&restored)
	if strings.Contains(section(), "permits read-only existing-test registration") || ctx.Mutable.NativeTestRegistrationAuthorization() != nil {
		t.Fatal("restored prose acquired authority")
	}
	mu.RevokeNativeTestRegistrationAuthorization()
	ctx.Mutable = mu
	if strings.Contains(section(), "permits read-only existing-test registration") {
		t.Fatal("revoked grant still taught")
	}
	// A typed repair handoff is a different lane; presentation cannot overrule it.
	run.Batches[1].Purpose = "impact_and_verification_proof_followup"
	mu.SetWriteWorkflowRun(run)
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{BatchID: "proof-2", PlanID: "failed-plan"})
	if section() != "" {
		t.Fatal("proof-only teaching masked typed repair")
	}
}
