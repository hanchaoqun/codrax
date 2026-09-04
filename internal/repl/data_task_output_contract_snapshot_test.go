package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
	"github.com/hanchaoqun/codrax/internal/llm"
)

// data_task_output_contract_snapshot_test.go — V9-4 (colleague_merge_audit
// §40.27 / §40.56): the planner's pre-dispatch projection gate and the
// execution judge read ONE output-contract snapshot. Before this pin the
// gate validated every action against the model's DRAFT contract while
// `protectPlan` later carried the stronger durable workflow contract into
// the executed plan, so a plan the planner was told was valid failed at
// `ActionRunner.Run` and burned repair/continue rounds.

// outputContractSnapshotRecords is the witness state shared by the pins: a
// reconcile ledger already exists under a strict plain_single_line contract
// with a declared complete reference, so assemble_answer is the executable
// next rank and the durable contract is far more specific than a draft that
// omits its output contract.
func outputContractSnapshotRecords() ([]dataTaskWorkflowRecord, dataquery.OutputContract) {
	records, _ := validatorProposalWitnessRecords()
	return records, records[0].Plan.OutputContract
}

func outputContractDraftPlanResponse(tool llm.ToolSchema, actions string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name:   tool.Name,
			Params: []byte(`{"status":"ready","actions":` + actions + `,"continue_after":false}`),
		}},
		StopReason: "tool_use",
	}
}

// TestDataTaskPlannerGateJudgesExecutionOutputContract is pin ①: the draft
// omits output_contract, declares assemble_answer projection=json_object, and
// the workflow's durable contract is plain_single_line. The gate must reject
// while the tool call is still open (one bounded same-tool repair naming the
// effective format) instead of accepting a plan that the executor will
// reject under the carried contract.
func TestDataTaskPlannerGateJudgesExecutionOutputContract(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, durable := outputContractSnapshotRecords()
	if durable.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("fixture durable contract=%+v, want plain_single_line", durable)
	}
	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: records[0].Plan}
	rank := dataTaskExecutableRankContractFromRuntimeView(root, view)
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	drift := `[{"id":"project","kind":"assemble_answer","params":{"projection":"json_object","output_field":"total","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]`
	repaired := `[{"id":"project","kind":"assemble_answer","params":{"projection":"values","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]`
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		outputContractDraftPlanResponse(tool, drift),
		outputContractDraftPlanResponse(tool, repaired),
	}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	plan, err := planner.RepairDataTaskWithRuntimeView(context.Background(), "sum per target", root, TurnPolicy{Route: RouteData}, nil, view.CurrentPlan, "grounding failed", dataquery.DataTaskViolation{}, view)
	if err != nil {
		t.Fatalf("RepairDataTaskWithRuntimeView: %v", err)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want the gate to reject the drafted projection under the execution contract and take one bounded repair; plan=%+v", len(adapter.calls), plan)
	}
	repairPrompt := adapter.calls[1].messages[len(adapter.calls[1].messages)-1].Content
	if !strings.Contains(repairPrompt, "output_contract.format=plain_single_line") {
		t.Fatalf("repair prompt does not name the effective execution format:\n%s", repairPrompt)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Params["projection"] != "values" {
		t.Fatalf("plan=%+v, want the repaired compatible projection", plan)
	}
	// Identity: the contract the gate judged is the contract the resolver
	// carries into execution — carrying again is a no-op.
	carried, _ := dataTaskCarryDurableOutputContract(plan, durable)
	if carried.OutputContract != plan.OutputContract {
		t.Fatalf("gate contract %+v != execution contract %+v", plan.OutputContract, carried.OutputContract)
	}
	if plan.OutputContract.Format != dataquery.OutputPlainSingleLine || !plan.OutputContract.CompleteReference {
		t.Fatalf("gate did not judge against the durable snapshot: %+v", plan.OutputContract)
	}
}

// TestDataTaskStagingGuardRejectsActionOutputContractDrift is pin ③ at the
// admission lane: a plan that never passed the planner gate (deferred
// remainder, fallback, resume) and whose action cannot satisfy the contract
// it will execute under is stopped by a typed staging guard — not by an
// execution failure.
func TestDataTaskStagingGuardRejectsActionOutputContractDrift(t *testing.T) {
	records, durable := outputContractSnapshotRecords()
	plan := dataquery.TaskPlan{
		Status:           "ready",
		Goal:             "project per-target totals",
		OutputContract:   durable,
		CoverageContract: records[0].Plan.CoverageContract,
		Actions: []dataquery.DataAction{{
			ID:   "project",
			Kind: dataquery.DataActionAssembleAnswer,
			Params: map[string]string{
				"projection":          "json_object",
				"reference_path":      "targets.csv",
				"reference_key_field": "canonical_label",
			},
		}},
	}
	guard := dataTaskWorkflowStagingGuardResult("", records, plan)
	if guard.Code != "action_output_contract_drift" {
		t.Fatalf("guard=%+v, want typed action_output_contract_drift before execution", guard)
	}
	if guard.Repairability != dataworkflow.RepairNeedsTypedAction || !strings.Contains(guard.ErrorText(), "plain_single_line") {
		t.Fatalf("guard=%+v, want a typed-action repair lane that names the effective format", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].Param != "projection/output_contract.format" || guard.Violations[0].ActionID != "project" {
		t.Fatalf("violations=%+v, want the executor's own typed locus on the offending action", guard.Violations)
	}
	// The same plan under a compatible projection passes the same guard
	// unchanged: system-built projection plans keep their params.
	compatible := plan
	compatible.Actions = []dataquery.DataAction{{
		ID:   "project",
		Kind: dataquery.DataActionAssembleAnswer,
		Params: map[string]string{
			"projection":          "values",
			"reference_path":      "targets.csv",
			"reference_key_field": "canonical_label",
		},
	}}
	if guard := dataTaskWorkflowStagingGuardResult("", records, compatible); !guard.Empty() {
		t.Fatalf("compatible plan blocked: %+v", guard)
	}
}

// TestRunDataTaskCLIDeferredDriftIsCaughtBeforeExecution is pin ③ end to
// end: a deferred remainder queued under json_only carries
// assemble_answer projection=json_object; by dispatch time the durable
// workflow contract is plain_single_line. Dispatch must stop at the typed
// drift guard and hand the model a repair — no execution failure record.
func TestRunDataTaskCLIDeferredDriftIsCaughtBeforeExecution(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, durable := outputContractSnapshotRecords()
	deferred := dataquery.TaskPlan{
		Status:         "ready",
		Goal:           "project per-target totals",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false},
		Actions: []dataquery.DataAction{{
			ID:   "project",
			Kind: dataquery.DataActionAssembleAnswer,
			Params: map[string]string{
				"projection":          "json_object",
				"reference_path":      "targets.csv",
				"reference_key_field": "canonical_label",
			},
		}},
	}
	checkpoint := writeDataTaskWorkflowCheckpointFile(t.TempDir(), root, records, dataquery.TaskPlan{}, deferred, 1, 0, "deferred drift checkpoint", "test")
	if checkpoint == "" {
		t.Fatal("checkpoint path empty")
	}
	planner := &stubDataTaskPlanner{
		repairPlan: dataquery.TaskPlan{
			Status:         "ready",
			Goal:           "project per-target totals",
			OutputContract: durable,
			Actions: []dataquery.DataAction{{
				ID:   "project",
				Kind: dataquery.DataActionAssembleAnswer,
				Params: map[string]string{
					"projection":          "values",
					"complete_reference":  "true",
					"reference_path":      "targets.csv",
					"reference_key_field": "canonical_label",
				},
			}},
		},
		eval: dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "grounded projection satisfies the contract", Confidence: "high"},
	}
	var progress bytes.Buffer
	runtimeAnchor := t.TempDir()
	answer, err := RunDataTaskCLI(context.Background(), "sum per target", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner:         planner,
		RepoRoot:        root,
		RuntimeAnchor:   runtimeAnchor,
		Language:        "en",
		MaxRepairRounds: 2,
		MaxDataRounds:   6,
		Progress:        &progress,
		ResumePath:      checkpoint,
	})
	if err != nil {
		t.Fatalf("RunDataTaskCLI: %v\n%s", err, progress.String())
	}
	if !strings.Contains(answer, "17,0,5") {
		t.Fatalf("answer=%q, want the grounded projection under the durable contract", answer)
	}
	if planner.repairCalls != 1 {
		t.Fatalf("repairCalls=%d, want exactly one typed repair; errors=%v", planner.repairCalls, planner.repairErrors)
	}
	if !strings.Contains(planner.repairErrors[0], "output contract in effect for this workflow") || !strings.Contains(planner.repairErrors[0], "output_contract.format=plain_single_line") {
		t.Fatalf("repair error=%q, want the typed drift guard naming the effective format (not an execution failure)", planner.repairErrors[0])
	}
	if strings.HasPrefix(planner.repairErrors[0], "execute data task") {
		t.Fatalf("repair error=%q, drift must be caught before execution", planner.repairErrors[0])
	}
	terminalFiles, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-terminal.json"))
	if err != nil || len(terminalFiles) != 1 {
		t.Fatalf("terminal audit files=%v err=%v", terminalFiles, err)
	}
	rawTerminal, err := os.ReadFile(terminalFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	terminalText := string(rawTerminal)
	if !strings.Contains(terminalText, `"code": "`+dataworkflow.GuardCodeActionOutputContractDrift+`"`) {
		t.Fatalf("terminal audit does not carry the typed drift guard code:\n%s", terminalText)
	}
	if strings.Count(terminalText, `"kind": "execute"`) != 1 {
		t.Fatalf("execute events=%d, want the drifted deferred batch never dispatched:\n%s", strings.Count(terminalText, `"kind": "execute"`), terminalText)
	}
	if strings.Contains(terminalText, "data action failed action_id") {
		t.Fatalf("terminal audit carries an execution-time action failure:\n%s", terminalText)
	}
}

// TestDataTaskCarryDurableOutputContractIsIdempotent is pin ⑤ (resolver
// half): carrying the durable contract twice equals carrying it once for
// every declared-format pair, which is what lets the planner gate carry
// first and protectPlan carry again without the executed contract moving.
func TestDataTaskCarryDurableOutputContractIsIdempotent(t *testing.T) {
	formats := append([]dataquery.OutputFormat{""}, dataquery.DataActionOutputProjectionFormats()...)
	for _, candidateFormat := range formats {
		for _, durableFormat := range formats {
			for _, explanation := range []bool{false, true} {
				candidate := dataquery.TaskPlan{OutputContract: dataquery.OutputContract{Format: candidateFormat, ExplanationAllowed: explanation}}
				durable := dataquery.OutputContract{Format: durableFormat, ExplanationAllowed: !explanation, CompleteReference: durableFormat != "", ReferencePath: "targets.csv", ReferenceKeyField: "canonical_label"}
				once, onceDurable := dataTaskCarryDurableOutputContract(candidate, durable)
				twice, twiceDurable := dataTaskCarryDurableOutputContract(once, onceDurable)
				if once.OutputContract != twice.OutputContract || onceDurable != twiceDurable {
					t.Fatalf("carry not idempotent for candidate=%+v durable=%+v: once=%+v twice=%+v", candidate.OutputContract, durable, once.OutputContract, twice.OutputContract)
				}
			}
		}
	}
}

// TestDataTaskExecutionOutputContractBaselinePrecedence pins the single
// reader of the gate baseline: a live loop's carried value wins; outside a
// loop the seed fold is used; a stale freeform Result never weakens a strict
// plan contract (ResolveOutputContract specificity).
func TestDataTaskExecutionOutputContractBaselinePrecedence(t *testing.T) {
	strict := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	records := []dataTaskWorkflowRecord{{
		Plan:   dataquery.TaskPlan{Status: "ready", OutputContract: strict},
		Result: &dataquery.Result{OutputContract: dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}},
	}}
	live := dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false}
	if got := dataTaskExecutionOutputContractBaseline(dataTaskWorkflowRuntimeView{Records: records, ExecutionOutputContract: live}); got != live {
		t.Fatalf("baseline=%+v, want the live loop's carried contract %+v", got, live)
	}
	if got := dataTaskExecutionOutputContractBaseline(dataTaskWorkflowRuntimeView{Records: records}); got != strict {
		t.Fatalf("baseline=%+v, want the seed fold %+v (stale freeform result must not weaken it)", got, strict)
	}
	if got := dataTaskExecutionOutputContractBaseline(dataTaskWorkflowRuntimeView{}); dataworkflow.OutputContractDeclared(got) && got.Format != dataquery.OutputFreeform {
		t.Fatalf("empty view baseline=%+v, want the undeclared/freeform seed", got)
	}
}
