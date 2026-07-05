package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestFinalDataTaskAnswerForCLIRejectsArtifactSummary(t *testing.T) {
	plan := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	result := dataquery.Result{
		Answer:         "4 artifact(s)",
		OutputContract: plan.OutputContract,
	}
	answer, err := finalDataTaskAnswerForCLI("", nil, plan, result, "en")
	if err == nil {
		t.Fatalf("answer=%q, want artifact summary rejected", answer)
	}
	if !strings.Contains(err.Error(), "final answer candidate") {
		t.Fatalf("err=%v, want final answer candidate guard", err)
	}
}

func TestFinalDataTaskAnswerForCLIAcceptsTypedFinalAnswer(t *testing.T) {
	plan := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	result := dataquery.Result{
		Answer:         "42",
		OutputContract: plan.OutputContract,
	}
	answer, err := finalDataTaskAnswerForCLI("", nil, plan, result, "en")
	if err != nil {
		t.Fatalf("finalDataTaskAnswerForCLI err=%v", err)
	}
	if strings.TrimSpace(answer) != "42" {
		t.Fatalf("answer=%q, want 42", answer)
	}
}

// DL-C pin (ledger §7.12, specimen
// data_basic_sum_with_rules-20260705-050649): the terminal answer
// projection must consume the same typed answer-candidate face that
// publishes has_answer=true. When the literal latest batch is an
// answerless helper round (inspect/schema probe), the terminal
// projection selects the latest final-answer-candidate record instead
// of failing a satisfied workflow with "result.answer is empty", and
// the completion gate is evaluated against the plan that produced that
// answer (the specimen's answer came from a custom_transform batch, so
// gating it against the answerless helper plan would still report
// missing_projection).
func TestFinalDataTaskAnswerForCLIConsumesEarlierAnswerBearingRecord(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	answerPlan := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: output,
		Actions: []dataquery.DataAction{{
			ID:     "final_sum_amount",
			Kind:   dataquery.DataActionCustomTransform,
			Script: "emit_result('17')",
		}},
	}
	answerReconcile := &dataquery.ReconcileReport{
		Status:         dataquery.LooseText("pass"),
		ExpectedAnswer: dataquery.LooseText("17"),
		ActualAnswer:   dataquery.LooseText("17"),
	}
	answerResult := dataquery.Result{
		Answer:         "17",
		OutputContract: output,
		Reconcile:      answerReconcile,
	}
	helperPlan := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: output,
		Actions: []dataquery.DataAction{{
			ID:   "inspect_generated_artifact_schema",
			Kind: dataquery.DataActionInspectMaterial,
		}},
	}
	helperResult := dataquery.Result{OutputContract: output}
	records := []dataTaskWorkflowRecord{
		{Plan: answerPlan, Result: &answerResult},
		{Plan: helperPlan, Result: &helperResult},
	}
	answer, err := finalDataTaskAnswerForCLI("", records, helperPlan, helperResult, "en")
	if err != nil {
		t.Fatalf("finalDataTaskAnswerForCLI err=%v, want earlier answer-bearing record consumed", err)
	}
	if strings.TrimSpace(answer) != "17" {
		t.Fatalf("answer=%q, want 17 from the answer-bearing record", answer)
	}
}

// Publish-time consult pin (DL-C review Med; mutation witness:
// removing the contest consult turns this red): when the latest
// evaluation actively contests the answer face (actionable repair_node
// anchored at assemble_answer) and the stored answer does not
// post-date it, the terminal fallback must fail loud instead of
// silently publishing the exact answer under contest. An answer-bearing
// record produced AFTER the contest is the repair output and publishes
// normally.
func TestFinalDataTaskAnswerForCLIRefusesContestedFallbackAnswer(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	answer := dlrAnswerRecord("17", output)
	helper := dlrHelperRecord(output)
	contestedHelper := helper
	contestedHelper.Evaluation = dlrContestingEvaluation()

	records := []dataTaskWorkflowRecord{answer, contestedHelper}
	published, err := finalDataTaskAnswerForCLI("", records, helper.Plan, *helper.Result, "en")
	if err == nil {
		t.Fatalf("answer=%q, want contested fallback publication refused", published)
	}
	if !strings.Contains(err.Error(), "actively contests") {
		t.Fatalf("err=%v, want the contest consult failure", err)
	}

	// Post-contest repair output publishes: the answer record follows
	// the contesting evaluation, so it is the repair result, not the
	// contested answer.
	records = []dataTaskWorkflowRecord{contestedHelper, answer, helper}
	published, err = finalDataTaskAnswerForCLI("", records, helper.Plan, *helper.Result, "en")
	if err != nil {
		t.Fatalf("finalDataTaskAnswerForCLI err=%v, want post-contest repair answer published", err)
	}
	if strings.TrimSpace(published) != "17" {
		t.Fatalf("answer=%q, want 17 from the post-contest repair record", published)
	}

	// Sticky-contest witness (second-review P1; mutation witness: a
	// latest-evaluation-only contest view turns this red by publishing
	// the contested X): the answer round carries the contest, the next
	// round is an answerless materialization helper whose routine
	// continue evaluation has no answer-face anchor — the contest is
	// not laundered and publication still fails loud.
	contestedAnswer := answer
	contestedAnswer.Evaluation = dlrContestingEvaluation()
	continueHelper := helper
	continueHelper.Evaluation = dlrContinueEvaluation()
	records = []dataTaskWorkflowRecord{contestedAnswer, continueHelper}
	published, err = finalDataTaskAnswerForCLI("", records, helper.Plan, *helper.Result, "en")
	if err == nil {
		t.Fatalf("answer=%q, want sticky contest to keep refusing publication across the helper round", published)
	}
	if !strings.Contains(err.Error(), "actively contests") {
		t.Fatalf("err=%v, want the contest consult failure", err)
	}

	// Explicit clearance shape: a later evaluation anchored at the
	// answer face without a repair request clears the contest and the
	// stored answer publishes.
	clearedHelper := helper
	clearedHelper.Evaluation = dlrAnswerFaceClearingEvaluation()
	records = []dataTaskWorkflowRecord{contestedAnswer, clearedHelper}
	published, err = finalDataTaskAnswerForCLI("", records, helper.Plan, *helper.Result, "en")
	if err != nil {
		t.Fatalf("finalDataTaskAnswerForCLI err=%v, want explicit answer-face re-evaluation to clear the contest", err)
	}
	if strings.TrimSpace(published) != "17" {
		t.Fatalf("answer=%q, want 17 after the contest is explicitly cleared", published)
	}
}

// Dual shape: when no record carries a contract-satisfying answer, the
// terminal projection still fails loudly — the fallback never invents
// an answer.
func TestFinalDataTaskAnswerForCLIStillFailsWithoutAnyAnswerBearingRecord(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	helperPlan := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: output,
		Actions: []dataquery.DataAction{{
			ID:   "inspect_generated_artifact_schema",
			Kind: dataquery.DataActionInspectMaterial,
		}},
	}
	helperResult := dataquery.Result{OutputContract: output}
	records := []dataTaskWorkflowRecord{{Plan: helperPlan, Result: &helperResult}}
	answer, err := finalDataTaskAnswerForCLI("", records, helperPlan, helperResult, "en")
	if err == nil {
		t.Fatalf("answer=%q, want terminal failure without any answer-bearing record", answer)
	}
}
