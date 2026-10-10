package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

type hmc221PlanOnly struct {
	plan dataquery.TaskPlan
	err  error
}

func (p *hmc221PlanOnly) PlanDataTask(context.Context, string, string, TurnPolicy, []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	return p.plan, p.err
}

type hmc221Evaluator struct {
	hmc221PlanOnly
	eval    dataquery.Evaluation
	evalErr error
	calls   int
}

func (p *hmc221Evaluator) EvaluateDataTask(context.Context, string, string, []dataTaskWorkflowRecord, string) (dataquery.Evaluation, error) {
	p.calls++
	return p.eval, p.evalErr
}

type hmc221Continuer struct {
	hmc221Evaluator
	next dataquery.TaskPlan
}

func (p *hmc221Continuer) ContinueDataTask(context.Context, string, string, TurnPolicy, []dataquery.CandidateFile, []dataTaskWorkflowRecord) (dataquery.TaskPlan, error) {
	return p.next, nil
}

func hmc221DataPlan() dataquery.TaskPlan {
	return dataquery.TaskPlan{
		Status: "ready", InputPaths: []string{"values.csv"},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
		Script: `rows = csv_rows("values.csv")
emit({"answer": str(sum(int(row["value"]) for row in rows)), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`,
	}
}

// The planner is scripted; execution, completion gates, return value, defer,
// terminal journal serialization and progress rendering are all production.
func hmc221RunDataCLI(t *testing.T, planner DataTaskPlanner, rounds int) (string, error, dataworkflow.WorkflowJournal, string, int) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root, anchor := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "values.csv"), []byte("value\n10\n7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var progress bytes.Buffer
	answer, runErr := RunDataTaskCLI(context.Background(), "Read the recorded values", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner: planner, RepoRoot: root, RuntimeAnchor: anchor, Language: "en", MaxDataRounds: rounds, MaxRepairRounds: 1, Progress: &progress,
	})
	paths, err := filepath.Glob(filepath.Join(anchor, "data-audit", "*-terminal.json"))
	if err != nil || len(paths) > 1 {
		t.Fatalf("terminal files=%v err=%v", paths, err)
	}
	var terminal dataworkflow.WorkflowJournal
	if len(paths) == 1 {
		raw, err := os.ReadFile(paths[0])
		if err != nil || json.Unmarshal(raw, &terminal) != nil {
			t.Fatalf("terminal decode err=%v raw=%s", err, raw)
		}
	}
	return answer, runErr, terminal, progress.String(), len(paths)
}

func hmc221AssertTerminal(t *testing.T, answer string, err error, terminal dataworkflow.WorkflowJournal, progress, status string, count int) {
	t.Helper()
	if err != nil || count != 1 {
		t.Fatalf("run err=%v terminal files=%d progress=%s", err, count, progress)
	}
	if terminal.Status != status || terminal.Decision.Status != status {
		t.Errorf("terminal=%q decision=%q, want %q", terminal.Status, terminal.Decision.Status, status)
	}
	if terminal.PublishedAnswer != strings.TrimSpace(answer) {
		t.Errorf("published=%q return=%q", terminal.PublishedAnswer, answer)
	}
	if !strings.Contains(progress, "◇ data audit · "+status+" · terminal ") {
		t.Errorf("progress lacks terminal status %q: %s", status, progress)
	}
}

func TestHMC221DataCLITerminalPreservesEvaluation(t *testing.T) {
	for _, status := range []dataquery.EvaluationStatus{dataquery.EvalComplete, dataquery.EvalPartialAnswerPossible, dataquery.EvalBudgetExhausted, dataquery.EvalBlocked, dataquery.EvalNeedsClarification} {
		t.Run(string(status), func(t *testing.T) {
			planner := &hmc221Evaluator{hmc221PlanOnly: hmc221PlanOnly{plan: hmc221DataPlan()}, eval: dataquery.Evaluation{Status: status, Reason: "typed evaluator boundary", Confidence: "high"}}
			answer, err, terminal, progress, count := hmc221RunDataCLI(t, planner, 3)
			hmc221AssertTerminal(t, answer, err, terminal, progress, string(status), count)
			if planner.calls != 1 {
				t.Errorf("evaluator calls=%d, want 1", planner.calls)
			}
			if status == dataquery.EvalComplete || status == dataquery.EvalPartialAnswerPossible || status == dataquery.EvalBudgetExhausted {
				if strings.TrimSpace(answer) != "17" {
					t.Errorf("retained answer=%q, want unchanged 17", answer)
				}
			}
			if terminal.Reason != "typed evaluator boundary" {
				t.Errorf("terminal reason=%q", terminal.Reason)
			}
		})
	}
}

func TestHMC221DataCLITerminalPreservesPlanAndBudget(t *testing.T) {
	for _, status := range []string{"complete", "completed", "partial_answer_possible", "budget_exhausted", "blocked", "needs_clarification", "ready"} {
		t.Run(status, func(t *testing.T) {
			next := hmc221DataPlan()
			next.Status, next.BlockReason = status, "typed next-plan boundary"
			if status != "ready" {
				next.Script = ""
			}
			planner := &hmc221Continuer{hmc221Evaluator: hmc221Evaluator{hmc221PlanOnly: hmc221PlanOnly{plan: hmc221DataPlan()}, eval: dataquery.Evaluation{Status: dataquery.EvalContinueData, Reason: "additional work", Confidence: "high"}}, next: next}
			answer, err, terminal, progress, count := hmc221RunDataCLI(t, planner, 1)
			want := status
			if status == "ready" {
				want = "budget_exhausted"
			}
			hmc221AssertTerminal(t, answer, err, terminal, progress, want, count)
			if status != "ready" && terminal.Reason != "typed next-plan boundary" {
				t.Errorf("terminal reason=%q", terminal.Reason)
			}
		})
	}
}

func TestHMC221DataCLITerminalKeepsErrorAndNoEvaluatorBehavior(t *testing.T) {
	t.Run("no_evaluator", func(t *testing.T) {
		answer, err, terminal, progress, count := hmc221RunDataCLI(t, &hmc221PlanOnly{plan: hmc221DataPlan()}, 3)
		hmc221AssertTerminal(t, answer, err, terminal, progress, "complete", count)
		if strings.TrimSpace(answer) != "17" {
			t.Errorf("answer=%q", answer)
		}
	})
	for _, failure := range []error{errors.New("evaluation transport unavailable"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			planner := &hmc221Evaluator{hmc221PlanOnly: hmc221PlanOnly{plan: hmc221DataPlan()}, eval: dataquery.Evaluation{Status: dataquery.EvalPartialAnswerPossible}, evalErr: failure}
			answer, err, terminal, progress, count := hmc221RunDataCLI(t, planner, 3)
			if !errors.Is(err, failure) || answer != "" || count != 1 || terminal.Status != "failed" || terminal.Decision.Status != "failed" || terminal.PublishedAnswer != "" || !strings.Contains(progress, "◇ data audit · failed · terminal ") {
				t.Fatalf("answer=%q err=%v count=%d terminal=%+v", answer, err, count, terminal)
			}
		})
	}
	t.Run("no_answer_result", func(t *testing.T) {
		plan := hmc221DataPlan()
		plan.Script = `rows = csv_rows("values.csv")
emit({"answer": "", "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`
		answer, err, terminal, _, count := hmc221RunDataCLI(t, &hmc221PlanOnly{plan: plan}, 1)
		if err == nil || answer != "" || count != 1 || terminal.Status != "failed" || terminal.PublishedAnswer != "" {
			t.Fatalf("answer=%q err=%v count=%d terminal=%+v", answer, err, count, terminal)
		}
	})
	t.Run("zero_round_plan_error", func(t *testing.T) {
		failure := errors.New("planning unavailable")
		answer, err, _, _, count := hmc221RunDataCLI(t, &hmc221PlanOnly{err: failure}, 3)
		if !errors.Is(err, failure) || answer != "" || count != 0 {
			t.Fatalf("answer=%q err=%v count=%d", answer, err, count)
		}
	})
}
