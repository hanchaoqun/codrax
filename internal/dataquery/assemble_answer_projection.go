package dataquery

import (
	"fmt"
	"strings"
)

// assemble_answer projection entry point and answer-level helpers.
// Split out of action_runner.go under the DQA LOC ratchet; the
// answer-repair lane rungs and their decision table live in
// assemble_answer_repair.go, the deterministic reconcile-group
// projection stays in action_runner.go
// (runAssembleAnswerFromReconcileGroups).

// runAssembleAnswer projects the final answer. Outside the answer-repair
// lane it is a plain delegation to the deterministic reconcile-group
// projection — byte-identical to the pre-DL-A behavior only when no
// typed rule-declared output field exists: the declared-field mapping
// (DL-A(b)) is an intentional out-of-lane root fix and renames
// single-key projections everywhere. Inside the lane it walks the
// typed projection priority ladder (as-built ruling, ledger §7.12; full
// decision table in assemble_answer_repair.go):
//
//	rung 1: unique typed rule-declared output field + deterministic
//	        single-key recompute from current reconcile groups → the
//	        fresh recompute wins; payloads are never projected.
//	rung 2: payload fallback (contract-narrowed admission, freshness
//	        ranking, declared-schema guard on every rank).
//	rung 3: declaration present but no usable candidate → contested
//	        lineage projection published WITH a typed advisory trace
//	        (returned advisories + artifact field), never silently.
//	rung 4: no declaration, no candidate → lineage projection as
//	        outside the lane.
func (r ActionRunner) runAssembleAnswer(action DataAction, reconcile *ReconcileReport, contract OutputContract, artifacts []DataArtifact, contributions []ContributionRecord, ruleCoverage []RuleCoverageRecord) (DataArtifact, string, *ReconcileReport, []string, error) {
	if reconcile == nil {
		return DataArtifact{}, "", nil, nil, DataActionDependencyError{
			ActionKind:    DataActionAssembleAnswer,
			Role:          "reconcile",
			Operation:     "answer_projection",
			ExpectedShape: "prior reconcile report produced by an earlier typed action",
			ActualSnippet: "missing reconcile report",
			RepairAction:  DataActionReconcile,
			Message:       "assemble_answer requires prior reconcile report",
		}
	}
	contract = r.effectiveAssembleOutputContract(contract)
	declaredField := ruleDeclaredOutputField(ruleCoverage)
	if !r.AnswerRepair.Active {
		artifact, answer, next, err := r.runAssembleAnswerFromReconcileGroups(action, reconcile, contract, artifacts, contributions, declaredField)
		return artifact, answer, next, nil, err
	}
	// Rung 1 — declared mapping first: the typed declaration plus a
	// deterministic single-key recompute from the CURRENT reconcile
	// groups is strictly stronger evidence than any stored payload
	// (content and lineage agree by fresh construction), so the payload
	// lane must never preempt it.
	var lineageArtifact DataArtifact
	var lineageAnswer string
	var lineageNext *ReconcileReport
	var lineageErr error
	lineageComputed := false
	if declaredField != "" {
		lineageArtifact, lineageAnswer, lineageNext, lineageErr = r.runAssembleAnswerFromReconcileGroups(action, reconcile, contract, artifacts, contributions, declaredField)
		lineageComputed = true
		if lineageErr == nil && jsonObjectHasSingleKey(lineageAnswer, declaredField) {
			return lineageArtifact, lineageAnswer, lineageNext, nil, nil
		}
	}
	// Rung 2 — payload fallback, only when the declared mapping is
	// unavailable or not deterministically computable.
	if artifact, answer, next, ok := r.runAssembleAnswerFromRepairSource(action, reconcile, contract, artifacts, declaredField); ok {
		return artifact, answer, next, nil, nil
	}
	// Rungs 3/4 — no usable payload candidate: fall back to the
	// contested lineage projection. With a declaration present this
	// fallback is never silent (typed advisory trace, rung 3); without
	// one it is the pre-lane behavior (rung 4).
	if !lineageComputed {
		lineageArtifact, lineageAnswer, lineageNext, lineageErr = r.runAssembleAnswerFromReconcileGroups(action, reconcile, contract, artifacts, contributions, declaredField)
	}
	var advisories []string
	if declaredField != "" && lineageErr == nil {
		if lineageArtifact.Fields == nil {
			lineageArtifact.Fields = map[string]string{}
		}
		lineageArtifact.Fields[assembleDeclaredFieldUnprojectedKey] = declaredField
		advisories = append(advisories, answerRepairDeclaredFieldAdvisory(declaredField))
	}
	return lineageArtifact, lineageAnswer, lineageNext, advisories, lineageErr
}

func (r ActionRunner) effectiveAssembleOutputContract(contract OutputContract) OutputContract {
	if contract.Format == "" && r.Seed.OutputContract.Format != "" {
		contract = r.Seed.OutputContract
	}
	return contract.Normalize()
}

func (r ActionRunner) runAssembleAnswerFromAnswerLevelReconcile(action DataAction, reconcile *ReconcileReport, contract OutputContract, contributions []ContributionRecord) (DataArtifact, string, *ReconcileReport, bool, error) {
	if reconcile == nil || len(reconcile.Groups) > 0 {
		return DataArtifact{}, "", nil, false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(reconcile.Status.String()), "pass") {
		return DataArtifact{}, "", nil, false, nil
	}
	if len(reconcileTargetContributions(contributions)) != 0 {
		return DataArtifact{}, "", nil, false, nil
	}
	answer := strings.TrimSpace(r.Seed.Answer)
	if answer == "" || AnswerLooksLikeArtifactSummary(answer) {
		return DataArtifact{}, "", nil, false, nil
	}
	if err := ValidateAnswer(answer, contract); err != nil {
		return DataArtifact{}, "", nil, true, fmt.Errorf("assemble_answer seed answer does not satisfy output contract: %w", err)
	}
	next := *reconcile
	next.ActualAnswer = LooseText(answer)
	next.ExpectedAnswer = LooseText(answer)
	next.Groups = append(withoutFinalAnswerProjectionGroups(reconcile.Groups), ReconcileGroup{
		GroupKey:   LooseText("final_answer"),
		Metric:     LooseText("projection"),
		Scope:      LooseText("final_answer"),
		Role:       LooseText("output"),
		Expected:   LooseText(answer),
		Actual:     LooseText(answer),
		Difference: LooseText("0"),
	})
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "assembled_answer")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionAssembleAnswer),
		SourcePaths: assembleAnswerConsumedPaths(action, contract, assembleReferenceProjection{}),
		Summary:     "assembled final answer from answer-level reconcile",
		Fields: map[string]string{
			"group_count":  "0",
			"projection":   "answer_level_seed",
			"answer_level": "true",
			"answer_len":   fmt.Sprintf("%d", len(answer)),
		},
	}, answer, &next, true, nil
}
