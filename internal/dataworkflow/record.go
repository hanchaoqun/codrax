package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// WorkflowRecord is the package-neutral execution record for a data workflow
// batch. Field names intentionally preserve the existing checkpoint JSON shape.
type WorkflowRecord struct {
	Plan       dataquery.TaskPlan
	Result     *dataquery.Result
	Err        string
	Violations []dataquery.DataTaskViolation
	Evaluation *dataquery.Evaluation
	Admission  *ActionDAGAdmissionDecision
}

// ActiveEvaluationFromRecords returns the evaluator judgment that is still
// authoritative for the live workflow state. An evaluation is attached to the
// result record it judged. Once a newer execution outcome arrives without its
// own evaluation, the older judgment becomes audit history and must not
// override facts derived from that newer result or failure.
//
// Answer-face contests intentionally do not use this projection: their sticky
// open/clear semantics are owned by the dedicated terminal-answer authority.
func ActiveEvaluationFromRecords(records []WorkflowRecord) (dataquery.Evaluation, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Evaluation != nil {
			return *rec.Evaluation, true
		}
		if workflowRecordHasExecutionOutcome(rec) {
			return dataquery.Evaluation{}, false
		}
	}
	return dataquery.Evaluation{}, false
}

func workflowRecordHasExecutionOutcome(rec WorkflowRecord) bool {
	return rec.Result != nil || strings.TrimSpace(rec.Err) != "" || len(rec.Violations) > 0
}

// ReconcileFailureStreak counts consecutive reconcile-action rounds
// that ended in a runtime failure, with no successful reconcile in
// between (EVALFIX-1 Gap A). Records for OTHER action kinds neither
// extend nor reset the streak: a refused escape attempt between two
// reconcile failures must not hide the streak, and only an actual
// reconcile SUCCESS (a record whose result carries a reconcile report
// and no error) closes it.
func ReconcileFailureStreak(records []WorkflowRecord) int {
	streak := 0
	for _, rec := range records {
		if rec.Result != nil && rec.Result.Reconcile != nil && strings.TrimSpace(rec.Err) == "" {
			streak = 0
			continue
		}
		if !planIsSingleReconcileAction(rec.Plan) {
			continue
		}
		if strings.TrimSpace(rec.Err) != "" {
			streak++
		}
	}
	return streak
}

func planIsSingleReconcileAction(plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if NormalizeActionKind(action.Kind) != dataquery.DataActionReconcile {
			return false
		}
	}
	return true
}
