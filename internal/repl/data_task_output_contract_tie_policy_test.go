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

// data_task_output_contract_tie_policy_test.go — G6-data-contract #0/#1
// (colleague_merge_audit §40.56 合流复核收编): the data lane has ONE
// output-contract tie policy. Before this pin the seed fold
// (dataTaskWorkflowOutputContract) kept the FIRST equally-specific
// declaration while the carry resolver (dataTaskCarryDurableOutputContract)
// let the LATEST win, so after an explicit equal-specificity format revision
// every reader of the fold — a system-built continuation, the CLI/REPL
// resume seed, the completion authorities — silently reverted the user's
// revision. Both now resolve through dataworkflow.ResolveOutputContract in
// ascending (chronological) order.

func tiePolicyContractMatrix() []dataquery.OutputContract {
	formats := append([]dataquery.OutputFormat{""}, dataquery.DataActionOutputProjectionFormats()...)
	var out []dataquery.OutputContract
	for _, format := range formats {
		for _, explanation := range []bool{false, true} {
			out = append(out, dataquery.OutputContract{Format: format, ExplanationAllowed: explanation})
		}
		out = append(out, dataquery.OutputContract{Format: format, CompleteReference: true, ReferencePath: "targets.csv", ReferenceKeyField: "canonical_label"})
	}
	return out
}

func tiePolicyExecutedRecord(contract dataquery.OutputContract) dataTaskWorkflowRecord {
	plan := dataquery.TaskPlan{Status: "ready", OutputContract: contract}
	return dataTaskWorkflowRecord{Plan: plan, Result: &dataquery.Result{OutputContract: plan.OutputContract}}
}

// TestDataTaskOutputContractFoldEqualsCarryChain is the tie-policy pin: for
// every ordered pair and triple of declarations from the format matrix, the
// seed fold over the executed records equals the live carry chain the loop's
// protectPlan closure would have produced over the same declarations. The
// equal-specificity pairs (json_only → plain_single_line, …) are red on the
// first-wins fold.
func TestDataTaskOutputContractFoldEqualsCarryChain(t *testing.T) {
	matrix := tiePolicyContractMatrix()
	check := func(sequence []dataquery.OutputContract) {
		t.Helper()
		var records []dataTaskWorkflowRecord
		var durable dataquery.OutputContract
		for i, contract := range sequence {
			plan := dataquery.TaskPlan{Status: "ready", OutputContract: contract}
			if i == 0 {
				durable = dataTaskWorkflowOutputContract(nil, plan)
			}
			var carried dataquery.TaskPlan
			carried, durable = dataTaskCarryDurableOutputContract(plan, durable)
			records = append(records, dataTaskWorkflowRecord{Plan: carried, Result: &dataquery.Result{OutputContract: carried.OutputContract}})
		}
		current := records[len(records)-1].Plan
		if fold := dataTaskWorkflowOutputContract(records, current); fold != durable {
			t.Fatalf("sequence %+v: fold=%+v carried=%+v — two tie policies", sequence, fold, durable)
		}
	}
	for _, a := range matrix {
		for _, b := range matrix {
			check([]dataquery.OutputContract{a, b})
			for _, c := range matrix {
				check([]dataquery.OutputContract{a, b, c})
			}
		}
	}
	// The explicit revision case named by the finding, spelled out.
	jsonOnly := dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false}
	plain := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	records := []dataTaskWorkflowRecord{tiePolicyExecutedRecord(jsonOnly), tiePolicyExecutedRecord(plain)}
	if got := dataTaskWorkflowOutputContract(records, records[1].Plan); got.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("fold=%+v, want the later equally-specific revision plain_single_line", got)
	}
	if got := dataworkflow.ResolveOutputContract(jsonOnly, plain); got.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("resolve=%+v, want the later equally-specific declaration", got)
	}
	if got := dataworkflow.ResolveOutputContract(plain, jsonOnly); got.Format != dataquery.OutputJSONOnly {
		t.Fatalf("resolve=%+v, want the later equally-specific declaration", got)
	}
	stronger := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, CompleteReference: true, ReferencePath: "targets.csv", ReferenceKeyField: "canonical_label"}
	if got := dataworkflow.ResolveOutputContract(stronger, jsonOnly); got != stronger.Normalize() {
		t.Fatalf("resolve=%+v, want the more specific earlier declaration to survive a weaker later one", got)
	}
}

func tiePolicyContinuationRecords() ([]dataTaskWorkflowRecord, dataquery.TaskPlan, dataquery.OutputContract) {
	coverage := referenceGroundingContract()
	coverage.ContributionLedgerRequired = true
	coverage.ReconcileRequired = true
	jsonOnly := dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false}
	plain := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	contributions := func(contract dataquery.OutputContract) *dataquery.Result {
		res := referenceGroundingResult("", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
		res.OutputContract = contract
		return &res
	}
	first := dataquery.TaskPlan{
		Status:           "ready",
		Goal:             "sum per target",
		CoverageContract: coverage,
		OutputContract:   jsonOnly,
		Actions: []dataquery.DataAction{{
			ID:         "ledger",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"observations.csv", "labels.csv"},
		}},
	}
	revised := first
	revised.OutputContract = plain
	revised.Status = "complete"
	records := []dataTaskWorkflowRecord{
		{Plan: first, Result: contributions(jsonOnly)},
		{Plan: revised, Result: contributions(plain)},
	}
	return records, revised, plain
}

// TestDataTaskSystemContinuationKeepsRevisedOutputContract is the
// continuation scenario: json_only executed, then an explicit equally
// specific plain_single_line revision executed; the live carried contract
// is plain. A system-built next-stage continuation takes its contract from
// the seed fold and passes protectPlan's carry — it must keep plain, and
// the carry must leave the durable contract untouched (red on the
// first-wins fold: the continuation flipped both back to json_only).
func TestDataTaskSystemContinuationKeepsRevisedOutputContract(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current, plain := tiePolicyContinuationRecords()
	durable := dataTaskWorkflowOutputContract(nil, records[0].Plan)
	for _, rec := range records {
		_, durable = dataTaskCarryDurableOutputContract(rec.Plan, durable)
	}
	if durable.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("live carried contract=%+v, want the explicit revision", durable)
	}
	continuation, reason, ok := dataTaskWorkflowNextStageFallbackWithRepo(root, records, current, "terminal plan ended")
	if !ok || len(continuation.Actions) == 0 {
		t.Fatalf("next-stage continuation not built: ok=%v reason=%q plan=%+v", ok, reason, continuation)
	}
	if continuation.OutputContract != plain.Normalize() {
		t.Fatalf("system continuation contract=%+v, want the revised plain_single_line from the fold (reason=%q)", continuation.OutputContract, reason)
	}
	carried, after := dataTaskCarryDurableOutputContract(continuation, durable)
	if carried.OutputContract.Format != dataquery.OutputPlainSingleLine || after.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("carry flipped the contract: plan=%+v durable=%+v", carried.OutputContract, after)
	}
	if got := dataTaskExecutionOutputContractBaseline(dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: current}); got != durable {
		t.Fatalf("out-of-loop gate baseline=%+v != live carried %+v", got, durable)
	}
}

// TestRunDataTaskCLIResumeKeepsRevisedOutputContract is the CLI-resume
// scenario on the §29.142 witness chain: an earlier answer round declared
// json_only with the same complete-reference fields (equal specificity),
// then the model explicitly revised the format to plain_single_line and
// that round executed. The resumed run re-seeds durableOutputContract from
// the fold, the proposal lane executes under it and the terminal completion
// gate judges the grounded answer under it — every reader must see the
// revision, so the run publishes "17,0,5" (red on the first-wins fold: the
// seed and the completion gate read json_only and rejected the plain
// answer as not satisfying the workflow output_contract).
func TestRunDataTaskCLIResumeKeepsRevisedOutputContract(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	jsonOnly := records[0].Plan.OutputContract
	jsonOnly.Format = dataquery.OutputJSONOnly
	if dataworkflow.ResolveOutputContract(jsonOnly) == dataworkflow.ResolveOutputContract(records[0].Plan.OutputContract) {
		t.Fatal("fixture: the two declarations must differ in format")
	}
	earlier := records[0]
	earlier.Plan.OutputContract = jsonOnly
	earlierResult := *records[0].Result
	earlierResult.OutputContract = jsonOnly
	earlierResult.Answer = `{"GroupA":17,"GroupX":4,"GroupC":5}`
	earlier.Result = &earlierResult
	records = append([]dataTaskWorkflowRecord{earlier}, records...)
	if fold := dataTaskWorkflowOutputContract(records, current); fold.Format != dataquery.OutputPlainSingleLine {
		t.Fatalf("resume seed fold=%+v, want the explicit plain_single_line revision", fold)
	}
	checkpoint := writeDataTaskWorkflowCheckpointFile(t.TempDir(), root, records, current, dataquery.TaskPlan{}, 2, 0, "revised contract checkpoint", "test")
	if checkpoint == "" {
		t.Fatal("checkpoint path empty")
	}
	planner := &stubDataTaskPlanner{
		repairErr:    newDataTaskPlannerNoToolError("data task planner", nil),
		continuePlan: dataquery.TaskPlan{},
		eval:         dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "grounded projection satisfies the contract", Confidence: "high"},
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
		t.Fatalf("RunDataTaskCLI resume: %v\n%s", err, progress.String())
	}
	if strings.TrimSpace(answer) != "17,0,5" {
		t.Fatalf("answer=%q, want the grounded plain_single_line answer under the revised contract\n%s", answer, progress.String())
	}
	proposalPlans, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-validator_proposal-r*.plan.json"))
	if err != nil || len(proposalPlans) == 0 {
		t.Fatalf("proposal plan audit files=%v err=%v", proposalPlans, err)
	}
	rawProposal, err := os.ReadFile(proposalPlans[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawProposal), `"format": "plain_single_line"`) || strings.Contains(string(rawProposal), `"format": "json_only"`) {
		t.Fatalf("proposal plan executed under a reverted contract:\n%s", rawProposal)
	}
}

// liveGateRecordingPlanner routes repair drafts through the REAL structured
// planner (llmDataTaskPlanner → planDataTaskWithTool gate) while recording
// the ExecutionOutputContract the live loop handed it; every other planner
// face is the stub.
type liveGateRecordingPlanner struct {
	*stubDataTaskPlanner
	repair       *llmDataTaskPlanner
	gateBaseline []dataquery.OutputContract
}

func (p *liveGateRecordingPlanner) RepairDataTaskWithRuntimeView(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, previous dataquery.TaskPlan, executionError string, violation dataquery.DataTaskViolation, view dataTaskWorkflowRuntimeView) (dataquery.TaskPlan, error) {
	p.stubDataTaskPlanner.repairCalls++
	p.stubDataTaskPlanner.repairErrors = append(p.stubDataTaskPlanner.repairErrors, executionError)
	p.gateBaseline = append(p.gateBaseline, view.ExecutionOutputContract)
	return p.repair.RepairDataTaskWithRuntimeView(ctx, userLine, repoRoot, policy, candidates, previous, executionError, violation, view)
}

// TestRunDataTaskCLILiveLoopGateReadsCarriedOutputContract is the live-loop
// composition pin (G6-data-contract #1 (a)): durableOutputContract →
// runtimeView().ExecutionOutputContract → planDataTaskWithTool through the
// real gate, inside RunDataTaskCLI. The deferred remainder drifts
// (json_object under a plain_single_line workflow), the typed staging guard
// hands the repair to the real planner, whose first draft drifts again: the
// gate must reject it in-tool against the loop's carried contract (the view
// carried a declared baseline equal to the durable contract; the bounded
// repair prompt names that format), and the repaired plan then executes to
// the grounded answer with no execution-time failure.
func TestRunDataTaskCLILiveLoopGateReadsCarriedOutputContract(t *testing.T) {
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
	rank := dataTaskExecutableRankContractFromRuntimeView(root, dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: deferred})
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	drift := `[{"id":"project","kind":"assemble_answer","params":{"projection":"json_object","output_field":"total","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]`
	repaired := `[{"id":"project","kind":"assemble_answer","params":{"projection":"values","complete_reference":"true","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]`
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		outputContractDraftPlanResponse(tool, drift),
		outputContractDraftPlanResponse(tool, repaired),
	}}
	planner := &liveGateRecordingPlanner{
		stubDataTaskPlanner: &stubDataTaskPlanner{eval: dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "grounded projection satisfies the contract", Confidence: "high"}},
		repair:              &llmDataTaskPlanner{adapter: adapter},
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
		t.Fatalf("answer=%q, want the grounded projection under the carried contract", answer)
	}
	if len(planner.gateBaseline) != 1 {
		t.Fatalf("repair views=%d, want exactly one typed repair through the live loop; errors=%v", len(planner.gateBaseline), planner.repairErrors)
	}
	if !dataworkflow.OutputContractDeclared(planner.gateBaseline[0]) || planner.gateBaseline[0] != durable.Normalize() {
		t.Fatalf("view.ExecutionOutputContract=%+v, want the loop's carried contract %+v", planner.gateBaseline[0], durable.Normalize())
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner calls=%d, want the gate to reject the drifted draft against the carried contract and take one bounded repair", len(adapter.calls))
	}
	repairPrompt := adapter.calls[1].messages[len(adapter.calls[1].messages)-1].Content
	if !strings.Contains(repairPrompt, "output_contract.format=plain_single_line") {
		t.Fatalf("in-tool repair prompt does not name the carried execution format:\n%s", repairPrompt)
	}
	terminalFiles, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-terminal.json"))
	if err != nil || len(terminalFiles) != 1 {
		t.Fatalf("terminal audit files=%v err=%v", terminalFiles, err)
	}
	rawTerminal, err := os.ReadFile(terminalFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawTerminal), "data action failed action_id") {
		t.Fatalf("terminal audit carries an execution-time action failure:\n%s", rawTerminal)
	}
}
