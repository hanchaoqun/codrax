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
