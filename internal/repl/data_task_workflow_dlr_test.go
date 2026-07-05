package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

// DLR review pins (ledger §7.12 as-built ruling): seed artifact
// freshness (same-key later-seen wins + typed producing rounds), the
// shared terminal-answer selection authority, and the completion path
// consuming it.

func dlrPayloadSeedArtifact(id, sample string) dataquery.DataArtifact {
	return dataquery.DataArtifact{
		ID:       id,
		Kind:     dataquery.ArtifactKindCustomPayload,
		Summary:  "script emitted extra payload field(s)",
		Sample:   []string{sample},
		RowCount: 1,
		Fields: map[string]string{
			"payload_keys": "ids",
			"json_shape":   "object(keys=ids)",
		},
	}
}

// Mutation witness (dropping the later-seen-wins dedup turns this
// red): the constant-ID "emitted_payload" envelope from an old round
// must not permanently suppress the repair batch's fresh payload, and
// the surviving entry moves to the tail so seed order tracks
// production freshness.
func TestDataTaskActionRunnerSeedSameKeyLaterSeenWins(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			dlrPayloadSeedArtifact(dataquery.ArtifactIDEmittedPayload, `{"ids":["old"]}`),
			{ID: "other_artifact", Kind: "extract_records/csv"},
		}}},
		{Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			dlrPayloadSeedArtifact(dataquery.ArtifactIDEmittedPayload, `{"ids":["new"]}`),
		}}},
	}
	seed := dataTaskActionRunnerSeed(records)
	if len(seed.Artifacts) != 2 {
		t.Fatalf("Artifacts=%+v, want same-key collapse to the newest copy", seed.Artifacts)
	}
	if seed.Artifacts[0].ID != "other_artifact" {
		t.Fatalf("Artifacts[0].ID=%q, want the superseded entry removed from its old position", seed.Artifacts[0].ID)
	}
	last := seed.Artifacts[1]
	if last.ID != dataquery.ArtifactIDEmittedPayload || len(last.Sample) == 0 || last.Sample[0] != `{"ids":["new"]}` {
		t.Fatalf("Artifacts[1]=%+v, want the newest emitted_payload copy at the freshest position", last)
	}

	rounds := dataTaskSeedArtifactRounds(records)
	if rounds[dataquery.ArtifactIDEmittedPayload] != 2 || rounds["other_artifact"] != 1 {
		t.Fatalf("rounds=%+v, want producing rounds of the surviving copies", rounds)
	}
}

func dlrAnswerRecord(answer string, output dataquery.OutputContract) dataTaskWorkflowRecord {
	plan := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: output,
		Actions: []dataquery.DataAction{{
			ID:     "final_sum_amount",
			Kind:   dataquery.DataActionCustomTransform,
			Script: "emit_result('" + answer + "')",
		}},
	}
	result := dataquery.Result{
		Answer:         answer,
		OutputContract: output,
		Reconcile: &dataquery.ReconcileReport{
			Status:         dataquery.LooseText("pass"),
			ExpectedAnswer: dataquery.LooseText(answer),
			ActualAnswer:   dataquery.LooseText(answer),
		},
		Artifacts: []dataquery.DataArtifact{{
			ID:   "final_answer",
			Kind: string(dataquery.DataActionAssembleAnswer),
		}},
	}
	return dataTaskWorkflowRecord{Plan: plan, Result: &result}
}

func dlrHelperRecord(output dataquery.OutputContract) dataTaskWorkflowRecord {
	plan := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: output,
		Actions: []dataquery.DataAction{{
			ID:   "inspect_generated_artifact_schema",
			Kind: dataquery.DataActionInspectMaterial,
		}},
	}
	result := dataquery.Result{OutputContract: output}
	return dataTaskWorkflowRecord{Plan: plan, Result: &result}
}

func dlrContestingEvaluation() *dataquery.Evaluation {
	return &dataquery.Evaluation{
		Status:     dataquery.EvalRepairNode,
		ActionID:   "final_answer",
		ActionKind: string(dataquery.DataActionAssembleAnswer),
		Confidence: "high",
		Reason:     "assembled answer contradicts the declared output schema",
	}
}

// dlrContinueEvaluation is a routine helper-round evaluation with no
// answer-face anchor: under the sticky-contest ruling it must neither
// open nor clear a contest.
func dlrContinueEvaluation() *dataquery.Evaluation {
	return &dataquery.Evaluation{
		Status: dataquery.EvalContinueData,
		Reason: "materialize more records before reassembling",
	}
}

// dlrAnswerFaceClearingEvaluation explicitly re-evaluates the answer
// face (typed assemble_answer anchor) without requesting repair — the
// only evaluation shape that clears a sticky contest.
func dlrAnswerFaceClearingEvaluation() *dataquery.Evaluation {
	return &dataquery.Evaluation{
		Status:     dataquery.EvalComplete,
		ActionID:   "final_answer",
		ActionKind: string(dataquery.DataActionAssembleAnswer),
		Confidence: "high",
		Reason:     "re-evaluated the assembled answer; it satisfies the declared output schema",
	}
}

// Shared-authority selection pins: the fallback record, the contest
// gate, and the post-contest repair shape.
func TestSelectDataTaskTerminalAnswerContestGating(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	answer := dlrAnswerRecord("17", output)
	helper := dlrHelperRecord(output)

	// Fallback shape: latest is an answerless helper round, the earlier
	// answer-bearing record is selected.
	records := []dataTaskWorkflowRecord{answer, helper}
	contract := dataTaskWorkflowCoverageContract(records, helper.Plan)
	sel := selectDataTaskTerminalAnswer(records, helper.Plan, *helper.Result, contract, output)
	if !sel.FromFallback || sel.Contested || strings.TrimSpace(sel.Result.Answer) != "17" {
		t.Fatalf("sel=%+v, want uncontested fallback onto the answer-bearing record", sel)
	}

	// Contested shape: the latest evaluation actively contests the
	// answer face and does not pre-date the selected record.
	contestedHelper := helper
	contestedHelper.Evaluation = dlrContestingEvaluation()
	records = []dataTaskWorkflowRecord{answer, contestedHelper}
	sel = selectDataTaskTerminalAnswer(records, helper.Plan, *helper.Result, contract, output)
	if !sel.FromFallback || !sel.Contested {
		t.Fatalf("sel=%+v, want contested fallback selection", sel)
	}

	// Post-contest repair shape: an answer-bearing record produced
	// AFTER the contesting evaluation is the repair output and is not
	// contested.
	records = []dataTaskWorkflowRecord{contestedHelper, answer, helper}
	sel = selectDataTaskTerminalAnswer(records, helper.Plan, *helper.Result, contract, output)
	if !sel.FromFallback || sel.Contested || strings.TrimSpace(sel.Result.Answer) != "17" {
		t.Fatalf("sel=%+v, want post-contest repair answer publishable", sel)
	}

	// Sticky-contest witness (second-review P1; mutation witness:
	// reverting to a latest-evaluation-only contest view turns this
	// red): round N assembles X and is contested, round N+1 is an
	// answerless materialization helper whose routine continue
	// evaluation carries no answer-face anchor — the contest survives.
	contestedAnswer := answer
	contestedAnswer.Evaluation = dlrContestingEvaluation()
	continueHelper := helper
	continueHelper.Evaluation = dlrContinueEvaluation()
	records = []dataTaskWorkflowRecord{contestedAnswer, continueHelper}
	sel = selectDataTaskTerminalAnswer(records, helper.Plan, *helper.Result, contract, output)
	if !sel.FromFallback || !sel.Contested {
		t.Fatalf("sel=%+v, want the helper round's continue evaluation to leave the contest sticky", sel)
	}

	// Explicit clearance shape: a later evaluation anchored at the
	// answer face that is not a repair request clears the contest.
	clearedHelper := helper
	clearedHelper.Evaluation = dlrAnswerFaceClearingEvaluation()
	records = []dataTaskWorkflowRecord{contestedAnswer, clearedHelper}
	sel = selectDataTaskTerminalAnswer(records, helper.Plan, *helper.Result, contract, output)
	if !sel.FromFallback || sel.Contested || strings.TrimSpace(sel.Result.Answer) != "17" {
		t.Fatalf("sel=%+v, want explicit answer-face re-evaluation to clear the contest", sel)
	}
}

// Sticky lane activation (second-review P1, same single authority): a
// two-batch repair's intervening helper round must not deactivate the
// answer-repair lane — its later assemble round would otherwise run
// bare against the contested lineage. Only an explicit answer-face
// re-evaluation deactivates it.
func TestDataTaskAssembleAnswerRepairContextStickyAcrossHelperEvaluations(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	contestedAnswer := dlrAnswerRecord("17", output)
	contestedAnswer.Evaluation = dlrContestingEvaluation()
	continueHelper := dlrHelperRecord(output)
	continueHelper.Evaluation = dlrContinueEvaluation()

	ctx := dataTaskAssembleAnswerRepairContext([]dataTaskWorkflowRecord{contestedAnswer, continueHelper})
	if !ctx.Active || ctx.ActionID != "final_answer" {
		t.Fatalf("ctx=%+v, want the lane to stay active across the helper round's continue evaluation", ctx)
	}

	clearedHelper := dlrHelperRecord(output)
	clearedHelper.Evaluation = dlrAnswerFaceClearingEvaluation()
	ctx = dataTaskAssembleAnswerRepairContext([]dataTaskWorkflowRecord{contestedAnswer, clearedHelper})
	if ctx.Active {
		t.Fatalf("ctx=%+v, want explicit answer-face re-evaluation to deactivate the lane", ctx)
	}
}

// Completion single authority (mutation witness: reverting the
// completion check to the literal latest batch turns this red): when
// the latest batch is an answerless helper round while an earlier
// record satisfies the whole contract, the evaluation decision returns
// the answer instead of burning another repair/continuation round.
func TestDataTaskEvaluationDecisionConsumesTerminalAnswerSelection(t *testing.T) {
	output := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	answer := dlrAnswerRecord("17", output)
	helper := dlrHelperRecord(output)
	records := []dataTaskWorkflowRecord{answer, helper}
	decision := dataTaskEvaluationDecisionWithRepo("", records, helper.Plan, *helper.Result,
		dataquery.Evaluation{Status: dataquery.EvalComplete}, false, false, 0, 3)
	if decision.Action != dataworkflow.EvaluationDecisionReturnAnswer || decision.Status != "complete" {
		t.Fatalf("decision=%+v, want completion satisfied through the shared terminal answer selection", decision)
	}

	// Contested fallback never satisfies completion: the same records
	// with a latest contesting evaluation keep completion unsatisfied
	// (the decision must not silently return the contested answer).
	contestedHelper := helper
	contestedHelper.Evaluation = dlrContestingEvaluation()
	records = []dataTaskWorkflowRecord{answer, contestedHelper}
	decision = dataTaskEvaluationDecisionWithRepo("", records, helper.Plan, *helper.Result,
		dataquery.Evaluation{Status: dataquery.EvalComplete}, false, false, 0, 3)
	if decision.Action == dataworkflow.EvaluationDecisionReturnAnswer && decision.Status == "complete" {
		t.Fatalf("decision=%+v, want contested fallback answer refused for completion", decision)
	}

	// Sticky-contest wash-out witness (second-review P1; mutation
	// witness: a latest-evaluation-only contest view turns this red by
	// completing with the contested answer X): contest on the answer
	// round, then an answerless helper round whose continue evaluation
	// is the one being decided — completion must stay unsatisfied and
	// the workflow must continue, not return_answer/complete.
	contestedAnswer := answer
	contestedAnswer.Evaluation = dlrContestingEvaluation()
	continueHelper := helper
	continueHelper.Evaluation = dlrContinueEvaluation()
	records = []dataTaskWorkflowRecord{contestedAnswer, continueHelper}
	decision = dataTaskEvaluationDecisionWithRepo("", records, helper.Plan, *helper.Result,
		*dlrContinueEvaluation(), true, false, 0, 3)
	if decision.Action == dataworkflow.EvaluationDecisionReturnAnswer && decision.Status == "complete" {
		t.Fatalf("decision=%+v, want the helper round's continue evaluation not to launder the contest into completion", decision)
	}
	if decision.Action != dataworkflow.EvaluationDecisionContinuePlan {
		t.Fatalf("decision=%+v, want the contested workflow to continue toward repair", decision)
	}
}
