package dataworkflow

import "github.com/hanchaoqun/codrax/internal/dataquery"

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
