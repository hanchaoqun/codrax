package repl

// E2PROP-1 (§29.150⑥, E-2 ruling "系统可提案不可代答" / the system may propose,
// never answer): when the data-task planner degrades into its typed
// double-empty error while the completion gate's grounding validator has
// already published a repair hint carrying COMPLETE assemble_answer
// parameters (complete_reference=true + reference_path + reference_key_field,
// the §29.142 witness shape where the correct "17,0,5" was computed and then
// withheld), the system may synthesize that projection as one more fallback
// plan CANDIDATE. The candidate enters the existing candidate lane — plan
// admission, execution, the full validator chain, and the evaluator all stay
// in front of any publication — and the planner/model keeps every choice it
// has today (the proposal only fires after the repair planner's bounded
// reprompt AND the continuation planner both came up empty).
//
// Red lines honored (feedback_no_system_backfill):
//   - the proposal is a PLAN candidate, never an answer: zero direct
//     execution, zero emit-face bypass, completion-gate ownership untouched;
//   - parameters come from TYPED validator carriers re-attested live
//     (recomputed guard + verbatim tie to the repair driver), never from
//     prose parsing — prose alone can never mint a proposal;
//   - synthesized parameters inherit the C1/C3 material credential
//     (dataTaskReferencePathIsWorkflowMaterial) BEFORE any proposal exists:
//     a generated-artifact alias (the replay#2 laundering source) is refused
//     with a logged reason, never silently dropped into the lane;
//   - a refused or absent candidate keeps the honest typed failure verbatim.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// dataTaskValidatorProposalSource is the typed provenance marker for fallback
// plan candidates synthesized from a validator repair hint. It rides
// dataTaskRepairPlanResult.Source into plan-transition journal events and
// plan audit scopes so the origin is a greppable typed field, not prose.
const dataTaskValidatorProposalSource = "validator_proposal"

// dataTaskValidatorRepairHint is the typed extraction of a grounding
// validator's complete assemble_answer repair parameters, together with the
// answer lineage the validator judged (needed so synthesis re-resolves the
// reference candidate against the same typed carriers).
type dataTaskValidatorRepairHint struct {
	ReferencePath     string
	ReferenceKeyField string
	AnswerPlan        dataquery.TaskPlan
	AnswerResult      dataquery.Result
}

// dataTaskValidatorAssembleAnswerRepairHint recomputes the reference-set
// grounding guard from the live runtime view (same terminal-answer selection
// the evaluation path consumes — single authority, DL-C) and returns its
// complete typed assemble_answer parameters only when:
//
//	(1) the recomputed grounding guard is red (a live validator attests the
//	    hint right now — a stale or forged prose hint can never qualify);
//	(2) the repair-driver error text carries the guard's message verbatim
//	    (the planner that degraded was actually being driven by THIS hint);
//	(3) the guard violation carries every parameter on typed fields:
//	    code=output_reference_grounding_mismatch, action_kind=assemble_answer,
//	    input_alias=reference_path, field=reference_key_field.
//
// Anything less returns false and the honest typed failure stands.
func dataTaskValidatorAssembleAnswerRepairHint(repoRoot string, view dataTaskWorkflowRuntimeView, repairDriverErrText string) (dataTaskValidatorRepairHint, bool) {
	records := view.Records
	current := view.CurrentPlan
	result, ok := latestDataTaskResult(records)
	if !ok {
		return dataTaskValidatorRepairHint{}, false
	}
	contract := dataTaskWorkflowCoverageContract(repoRoot, records, current)
	output := dataTaskWorkflowOutputContract(records, current)
	answerPlan, answerResult := current, result
	if sel := selectDataTaskTerminalAnswerWithRepo(repoRoot, records, current, result, contract, output); sel.FromFallback && !sel.Contested {
		answerPlan, answerResult = sel.Plan, sel.Result
	}
	guard := dataTaskOutputReferenceGroundingGuardResult(repoRoot, records, answerPlan, answerResult)
	if guard.Empty() || len(guard.Violations) == 0 {
		return dataTaskValidatorRepairHint{}, false
	}
	if !strings.Contains(repairDriverErrText, strings.TrimSpace(guard.Message)) {
		return dataTaskValidatorRepairHint{}, false
	}
	violation := guard.Violations[0]
	if strings.TrimSpace(violation.Code) != "output_reference_grounding_mismatch" ||
		strings.TrimSpace(violation.ActionKind) != string(dataquery.DataActionAssembleAnswer) {
		return dataTaskValidatorRepairHint{}, false
	}
	referencePath := strings.TrimSpace(violation.InputAlias)
	referenceKeyField := strings.TrimSpace(violation.Field)
	if referencePath == "" || referenceKeyField == "" {
		logging.Info("[repl/data] validator proposal inert: grounding hint is live but its typed assemble_answer parameters are incomplete (reference_path=%q reference_key_field=%q)", referencePath, referenceKeyField)
		return dataTaskValidatorRepairHint{}, false
	}
	return dataTaskValidatorRepairHint{
		ReferencePath:     referencePath,
		ReferenceKeyField: referenceKeyField,
		AnswerPlan:        answerPlan,
		AnswerResult:      answerResult,
	}, true
}

// dataTaskSynthesizeValidatorProposalPlan builds the fallback plan candidate
// from validator-provided assemble_answer parameters. The C1/C3 material
// credential gate runs FIRST: a reference_path that fails
// dataTaskReferencePathIsWorkflowMaterial (a workflow-generated artifact
// alias — the laundered self-referential universe of replay#2) is refused
// before any plan exists. Every refusal returns a non-empty reason so the
// caller can disclose it; nothing is dropped silently. The plan shape is the
// EXISTING zero-fill projection machinery (BuildRequiredOutputProjectionPlan
// with a declared reference gap) — the proposal lane mints no new plan
// grammar of its own.
func dataTaskSynthesizeValidatorProposalPlan(repoRoot string, records []dataTaskWorkflowRecord, answerPlan dataquery.TaskPlan, answerResult dataquery.Result, referencePath, referenceKeyField string) (dataquery.TaskPlan, string, bool) {
	referencePath = strings.TrimSpace(referencePath)
	referenceKeyField = strings.TrimSpace(referenceKeyField)
	if referencePath == "" || referenceKeyField == "" {
		return dataquery.TaskPlan{}, "incomplete assemble_answer parameters (reference_path and reference_key_field are both required)", false
	}
	if !dataTaskReferencePathIsWorkflowMaterial(repoRoot, records, answerPlan, answerResult, referencePath) {
		return dataquery.TaskPlan{}, "reference_path " + referencePath + " failed the workflow material credential (workflow-generated artifact alias; generated universes never gain proposal standing)", false
	}
	runner := dataquery.ActionRunner{RepoRoot: strings.TrimSpace(repoRoot), Seed: answerResult}
	groupKeys := dataTaskContributionGroupKeys(answerResult.Contributions)
	candidate, ok := dataTaskExplicitReferenceProjectionCandidate(runner, referencePath, referenceKeyField, groupKeys)
	if !ok {
		return dataquery.TaskPlan{}, "reference candidate " + referencePath + "#" + referenceKeyField + " is not resolvable from the on-disk material", false
	}
	if strings.TrimSpace(candidate.Field) != referenceKeyField {
		return dataquery.TaskPlan{}, "reference candidate resolution drifted from the validator's key field (" + referencePath + "#" + referenceKeyField + " resolved as field " + strings.TrimSpace(candidate.Field) + ")", false
	}
	contract := dataTaskWorkflowCoverageContract(repoRoot, records, answerPlan)
	output := dataTaskWorkflowOutputContract(records, answerPlan)
	plan, ok := dataworkflow.BuildRequiredOutputProjectionPlan(dataworkflow.OutputProjectionPlanInput{
		Current:                answerPlan,
		Coverage:               contract,
		Output:                 output,
		Result:                 answerResult,
		ReferenceGap:           dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: true},
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(answerPlan),
	})
	if !ok {
		return dataquery.TaskPlan{}, "projection plan synthesis unavailable for the validator-parameterized reference set", false
	}
	return plan, "", true
}

// dataTaskValidatorHintFallbackPlanCandidate is the single composition point
// of the proposal lane: typed extraction (live validator re-attestation) then
// credential-gated synthesis. Refusals are logged with their reason — the
// lane never drops a candidate silently — and the function only ever returns
// a CANDIDATE plan for the existing fallback lane; it has no execution or
// publication path (pinned structurally).
func dataTaskValidatorHintFallbackPlanCandidate(repoRoot string, view dataTaskWorkflowRuntimeView, repairDriverErrText string) (dataquery.TaskPlan, bool) {
	hint, ok := dataTaskValidatorAssembleAnswerRepairHint(repoRoot, view, repairDriverErrText)
	if !ok {
		return dataquery.TaskPlan{}, false
	}
	plan, refusal, ok := dataTaskSynthesizeValidatorProposalPlan(repoRoot, view.Records, hint.AnswerPlan, hint.AnswerResult, hint.ReferencePath, hint.ReferenceKeyField)
	if !ok {
		logging.Warning("[repl/data] validator proposal refused: %s", refusal)
		return dataquery.TaskPlan{}, false
	}
	logging.Info("[repl/data] validator proposal candidate synthesized from grounding repair hint reference=%s#%s (source=%s; planner/model retains selection, validator chain re-verifies)",
		hint.ReferencePath, hint.ReferenceKeyField, dataTaskValidatorProposalSource)
	return plan, true
}
