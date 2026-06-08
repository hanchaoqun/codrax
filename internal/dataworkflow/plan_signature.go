package dataworkflow

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func PlanActionSignature(plan dataquery.TaskPlan) string {
	if len(plan.Actions) == 0 {
		return ""
	}
	type actionSig struct {
		Kind           string            `json:"kind,omitempty"`
		InputPaths     []string          `json:"input_paths,omitempty"`
		OutputArtifact string            `json:"output_artifact,omitempty"`
		Params         map[string]string `json:"params,omitempty"`
	}
	sig := struct {
		Actions []actionSig `json:"actions,omitempty"`
	}{Actions: make([]actionSig, 0, len(plan.Actions))}
	for _, action := range plan.Actions {
		sig.Actions = append(sig.Actions, actionSig{
			Kind:           string(NormalizeActionKind(action.Kind)),
			InputPaths:     cleanStrings(action.InputPaths),
			OutputArtifact: normalizeAccessPath(action.OutputArtifact),
			Params:         action.Params,
		})
	}
	raw, err := json.Marshal(sig)
	if err != nil {
		return ""
	}
	return string(raw)
}

func PlanActionSignatureAlreadySeen(seen []dataquery.TaskPlan, candidate dataquery.TaskPlan) bool {
	signature := PlanActionSignature(candidate)
	if signature == "" {
		return false
	}
	for _, plan := range seen {
		if PlanActionSignature(plan) == signature {
			return true
		}
	}
	return false
}

func PlanActionSignatureAlreadySeenInRecords(records []WorkflowRecord, candidate dataquery.TaskPlan) bool {
	signature := PlanActionSignature(candidate)
	if signature == "" {
		return false
	}
	for _, record := range records {
		if PlanActionSignature(record.Plan) == signature {
			return true
		}
	}
	return false
}
