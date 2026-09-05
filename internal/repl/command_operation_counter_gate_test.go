package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_counter_gate_test.go — the top-of-loop command-round
// counter gate (colleague_merge_audit §40.52, batch six fold-in review
// round eight #0). executeCommandOperationPlanAttempt and
// runCommandOperationCLIPlan accept an arbitrary resume state; the gate
// `commandRounds >= commandOperationCommandRoundLimit(ownRecords)` is the
// executor-side invariant over that input contract: a ready plan resumed
// with the counter already at the operation's limit mints the command-round
// budget terminal before any executor call, in the REPL lane (a direct
// resume and the /approve carry) and the CLI lane alike. The limit is the
// function of the own rounds, not the base constant: at the base limit
// with an evaluation-granted extension, and below the limit, the plan
// executes.

// counterGateOwnRecords returns n executed own rounds; with material the
// rounds carry a payload ref and the last one the continue_command /
// partial evaluation that grants the extended limit.
func counterGateOwnRecords(n int, material bool) commandOperationOwnRecords {
	records := make(commandOperationOwnRecords, 0, n)
	for i := 0; i < n; i++ {
		record := commandOperationResultRecord{
			Plan:   operation.CommandOperationPlan{ID: "prior"},
			Result: operation.CommandOperationResult{PlanID: "prior", Status: operation.StatusExecuted},
		}
		if material {
			record.Result.PayloadRef = "/tmp/material-page.txt"
			if i == n-1 {
				record.Evaluation = &operation.OperationEvaluation{Status: operation.EvalContinueCommand, MaterialCoverageStatus: operation.MaterialCoveragePartial}
			}
		}
		records = append(records, record)
	}
	return records
}

// counterGatePlan is a ready low-risk plan whose only step writes marker:
// the marker's existence is the executor witness.
func counterGatePlan(workDir, marker string) operation.CommandOperationPlan {
	return operation.CommandOperationPlan{
		ID:          "resumed-at-the-limit",
		RequestText: "write the marker file",
		Status:      operation.StatusReady,
		RiskLevel:   "low",
		WorkDir:     workDir,
		Steps: []operation.CommandStep{{
			ID:        "write",
			Title:     "write the marker",
			Program:   "touch",
			Args:      []string{marker},
			RiskLevel: "low",
		}},
	}
}

// counterGateArm is one resume state: the own rounds, the carried counter
// and whether the gate must refuse the round.
type counterGateArm struct {
	name    string
	own     commandOperationOwnRecords
	rounds  int
	refused bool
}

func counterGateArms() []counterGateArm {
	return []counterGateArm{
		{"at_the_limit", counterGateOwnRecords(commandOperationMaxCommandRounds, false), commandOperationMaxCommandRounds, true},
		{"at_the_base_limit_with_extension_granted", counterGateOwnRecords(commandOperationMaxCommandRounds, true), commandOperationMaxCommandRounds, false},
		{"below_the_limit", counterGateOwnRecords(commandOperationMaxCommandRounds-1, false), commandOperationMaxCommandRounds - 1, false},
	}
}

func requireCounterGateWindow(t *testing.T, lane string, arm counterGateArm, plan operation.CommandOperationPlan, records []commandOperationResultRecord, marker string) {
	t.Helper()
	if len(records) != 1 || records[0].Plan.ID != plan.ID {
		t.Fatalf("%s/%s: records=%d, want exactly the resumed plan's one record", lane, arm.name, len(records))
	}
	got := records[0].Result
	_, err := os.Stat(marker)
	if arm.refused {
		if got.Status != operation.StatusBudgetExhausted || got.FailureClass != "budget_exhausted" || !strings.Contains(got.OutputPreview, "command-round budget exhausted") {
			t.Fatalf("%s/%s: status=%s class=%q preview=%q, want the command-round budget terminal", lane, arm.name, got.Status, got.FailureClass, got.OutputPreview)
		}
		if err == nil {
			t.Fatalf("%s/%s: the marker exists — the executor ran a round resumed at the command-round limit", lane, arm.name)
		}
		return
	}
	if got.Status != operation.StatusExecuted {
		t.Fatalf("%s/%s: status=%s preview=%q, want executed", lane, arm.name, got.Status, got.OutputPreview)
	}
	if err != nil {
		t.Fatalf("%s/%s: the marker is missing — the round below the limit did not execute: %v", lane, arm.name, err)
	}
}

// requireCounterGatePanel reads the round card: the budget terminal renders
// through the step-less Note shape with the reason exactly once.
func requireCounterGatePanel(t *testing.T, lane string, printed string) {
	t.Helper()
	compact := strings.Join(strings.Fields(strings.ReplaceAll(printed, "│", "")), "")
	if strings.Contains(compact, "step``") {
		t.Fatalf("%s: panel renders a synthetic step:\n%s", lane, printed)
	}
	if got := strings.Count(compact, "command-roundbudgetexhaustedbeforetheusergoalwasfullysatisfied"); got != 1 {
		t.Fatalf("%s: the budget reason must appear exactly once on the panel, got %d:\n%s", lane, got, printed)
	}
	if !strings.Contains(compact, "Note:commandoperationcommand-roundbudgetexhausted") {
		t.Fatalf("%s: the budget terminal must render through the step-less Note shape:\n%s", lane, printed)
	}
}

func counterGateREPL(t *testing.T, input string) (*REPL, *scriptedChatAdapter, *bytes.Buffer, string) {
	t.Helper()
	adapter := &scriptedChatAdapter{responses: []llm.Response{{Content: "answered over the records.", StopReason: "end_turn"}}}
	r, out := newApproveBudgetREPL(t, adapter, input)
	workDir := t.TempDir()
	r.operationPolicy.DefaultWorkDir = workDir
	return r, adapter, out, workDir
}

// TestCommandOperationCounterGateRefusesResumeAtTheLimit pins the gate on
// both REPL entries: a direct resume of executeCommandOperationPlanAttempt
// and the /approve carry. One adapter call in every arm — the answerer —
// so no planner, evaluator or replanner ran; the answerer's prompt carries
// the budget terminal (refused) or the executed round.
//
// EVOLUTION RECORD (review round eight #0): after the round-seven replan
// gate no production caller reaches this gate — every fresh entry passes a
// zero state and every park sits under `commandRounds < limit` — so a
// scratch overlay deleting both gates (`if false {` at repl.go 4874 and
// command_operation_cli.go 125, not committed) kept the whole repl package
// green on 42d2017e4. Under that mutation this pin and its CLI twin go
// red (the at_the_limit arm executes: marker written, status executed);
// the live tree is green.
func TestCommandOperationCounterGateRefusesResumeAtTheLimit(t *testing.T) {
	for _, arm := range counterGateArms() {
		t.Run("direct_resume/"+arm.name, func(t *testing.T) {
			r, adapter, out, workDir := counterGateREPL(t, "/exit\n")
			marker := filepath.Join(workDir, "marker")
			plan := counterGatePlan(workDir, marker)
			r.executeCommandOperationPlanAttempt(plan, plan.RequestText, plan.RequestText, commandOperationAttemptState{
				Own:           append(commandOperationOwnRecords(nil), arm.own...),
				CommandRounds: arm.rounds,
			})
			requireCounterGateWindow(t, "repl direct", arm, plan, r.operationResults, marker)
			if r.pendingOperation != nil || r.pendingOperationCarry != nil {
				t.Fatalf("nothing may park: plan=%+v carry=%+v", r.pendingOperation, r.pendingOperationCarry)
			}
			if len(adapter.calls) != 1 {
				t.Fatalf("adapter calls=%d, want 1 (the answerer only)\n%s", len(adapter.calls), out.String())
			}
			if arm.refused {
				requireCounterGatePanel(t, "repl direct", out.String())
				if !strings.Contains(answererUserPrompt(t, adapter), "status=budget_exhausted failure_class=budget_exhausted") {
					t.Fatalf("the answerer must see the budget terminal:\n%s", answererUserPrompt(t, adapter))
				}
			}
		})
	}
	t.Run("approve_carry/at_the_limit", func(t *testing.T) {
		arm := counterGateArms()[0]
		r, adapter, out, workDir := counterGateREPL(t, "/approve\n/exit\n")
		marker := filepath.Join(workDir, "marker")
		plan := counterGatePlan(workDir, marker)
		r.parkCommandOperationPlan(plan, commandOperationAttemptState{Own: arm.own, CommandRounds: arm.rounds})
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		requireCounterGateWindow(t, "repl /approve", arm, plan, r.operationResults, marker)
		if r.pendingOperation != nil || r.pendingOperationCarry != nil {
			t.Fatalf("the approved plan must not stay parked: plan=%+v carry=%+v", r.pendingOperation, r.pendingOperationCarry)
		}
		if len(adapter.calls) != 1 {
			t.Fatalf("adapter calls=%d, want 1 (the answerer only)\n%s", len(adapter.calls), out.String())
		}
		requireCounterGatePanel(t, "repl /approve", out.String())
	})
}

func answererUserPrompt(t *testing.T, adapter *scriptedChatAdapter) string {
	t.Helper()
	user := ""
	for _, msg := range adapter.calls[len(adapter.calls)-1].messages {
		if msg.Role == "user" {
			user = msg.Content
		}
	}
	return user
}

// TestCommandOperationCLICounterGateRefusesResumeAtTheLimit is the CLI
// twin: runCommandOperationCLIPlan resumed at the limit hands the answerer
// the own rounds plus the budget terminal and never calls the executor;
// with the extension granted or below the limit the round executes.
func TestCommandOperationCLICounterGateRefusesResumeAtTheLimit(t *testing.T) {
	t.Parallel()
	for _, arm := range counterGateArms() {
		t.Run(arm.name, func(t *testing.T) {
			workDir := t.TempDir()
			policy := operation.DefaultCommandPolicy()
			policy.DefaultWorkDir = workDir
			planner := &replanCountingCLIPlanner{}
			progress := &bytes.Buffer{}
			marker := filepath.Join(workDir, "marker")
			plan := counterGatePlan(workDir, marker)
			answer, err := runCommandOperationCLIPlan(context.Background(), CommandOperationCLIConfig{
				Planner:       planner,
				Policy:        policy,
				RepoRoot:      workDir,
				RuntimeAnchor: t.TempDir(),
				Language:      "en",
				Progress:      progress,
			}, plan.RequestText, plan, commandOperationAttemptState{
				Own:           append(commandOperationOwnRecords(nil), arm.own...),
				CommandRounds: arm.rounds,
			})
			if err != nil {
				t.Fatalf("runCommandOperationCLIPlan: %v", err)
			}
			if planner.replanCalls != 0 || planner.continuationCalls != 0 {
				t.Fatalf("replan_calls=%d continuation_calls=%d, want 0/0", planner.replanCalls, planner.continuationCalls)
			}
			if len(planner.answered) != len(arm.own)+1 {
				t.Fatalf("answerer records=%d, want the %d own rounds plus one", len(planner.answered), len(arm.own))
			}
			requireCounterGateWindow(t, "cli", arm, plan, planner.answered[len(arm.own):], marker)
			if arm.refused {
				requireCounterGatePanel(t, "cli", progress.String())
				if !strings.Contains(answer, "final: command operation command-round budget exhausted") {
					t.Fatalf("answer must be synthesised over the budget terminal:\n%s", answer)
				}
			}
		})
	}
}
