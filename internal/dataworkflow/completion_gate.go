package dataworkflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type CompletionGateGuardInput struct {
	ValidationErr error
	LedgerGraph   LedgerGraph
	OutputGraph   OutputProjectionGraph
	ReferenceGap  ReferenceProjectionGap
}

func CompletionGateGuardResult(input CompletionGateGuardInput) GuardResult {
	if input.ValidationErr != nil {
		if validationErrorHasCode(input.ValidationErr, "missing_required_ledger") {
			if guard := LedgerGraphCompletionGuardResult(input.LedgerGraph); !guard.Empty() {
				return guard
			}
		}
		return validationErrorGuardResult(input.ValidationErr)
	}
	switch input.OutputGraph.Status {
	case OutputProjectionStatusIncompleteReference:
		return outputProjectionGuardResult(input.OutputGraph, input.ReferenceGap)
	case OutputProjectionStatusMissingProjection:
		return outputProjectionGuardResult(input.OutputGraph, input.ReferenceGap)
	default:
		return GuardResult{}
	}
}

func validationErrorHasCode(err error, code string) bool {
	code = strings.TrimSpace(code)
	if err == nil || code == "" {
		return false
	}
	var validationErr dataquery.DataValidationError
	if errors.As(err, &validationErr) {
		for _, violation := range validationErr.Violations {
			if strings.TrimSpace(violation.Code) == code {
				return true
			}
		}
	}
	var resultErr *dataquery.DataResultValidationError
	if errors.As(err, &resultErr) {
		for _, violation := range resultErr.Violations {
			if strings.TrimSpace(violation.Code) == code {
				return true
			}
		}
	}
	return false
}

func validationErrorGuardResult(err error) GuardResult {
	violations := validationErrorWorkflowViolations(err)
	message := fmt.Sprintf("validate data workflow completion: %v", err)
	code := "data_validation_incomplete"
	if len(violations) > 0 && strings.TrimSpace(violations[0].Code) != "" {
		code = violations[0].Code
	}
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violations...)
}

func validationErrorWorkflowViolations(err error) []WorkflowViolation {
	var dataViolations []dataquery.DataTaskViolation
	var validationErr dataquery.DataValidationError
	if errors.As(err, &validationErr) {
		dataViolations = append(dataViolations, validationErr.Violations...)
	}
	var resultErr *dataquery.DataResultValidationError
	if errors.As(err, &resultErr) {
		dataViolations = append(dataViolations, resultErr.Violations...)
	}
	if len(dataViolations) == 0 {
		return []WorkflowViolation{{
			Code:          "data_validation_incomplete",
			Severity:      "error",
			Repairability: RepairNeedsTypedAction,
			Reason:        strings.TrimSpace(err.Error()),
		}}
	}
	out := make([]WorkflowViolation, 0, len(dataViolations))
	for _, violation := range dataViolations {
		code := strings.TrimSpace(violation.Code)
		if code == "" {
			code = "data_validation_incomplete"
		}
		out = append(out, WorkflowViolation{
			Code:                 code,
			Severity:             "error",
			Repairability:        RepairNeedsTypedAction,
			ActionID:             strings.TrimSpace(violation.ActionID),
			ActionKind:           strings.TrimSpace(violation.ActionKind),
			InputAlias:           strings.TrimSpace(violation.InputAlias),
			InputAliases:         cleanStrings(violation.InputAliases),
			RepairActionHints:    cleanStrings([]string{violation.RepairHint}),
			MissingFields:        cleanStrings(violation.MissingFields),
			AvailableFieldSample: cleanStrings(violation.AvailableFieldSample),
			Reason:               strings.TrimSpace(violation.Summary),
		})
	}
	return out
}

func outputProjectionGuardResult(graph OutputProjectionGraph, gap ReferenceProjectionGap) GuardResult {
	code := "output_projection_" + strings.TrimSpace(graph.Status)
	message := outputProjectionCompletionMessage(graph, gap)
	action := dataquery.DataAction{
		ID:   "complete_output_projection",
		Kind: dataquery.DataActionAssembleAnswer,
	}
	violation := NewActionInputViolation(
		code,
		"error",
		RepairNeedsTypedAction,
		action,
		strings.TrimSpace(gap.Candidate.Path),
		nil,
		message,
		graph.ProducesActions,
	)
	if gap.Present {
		violation.InputAlias = strings.TrimSpace(gap.Candidate.Path)
		violation.MissingFields = cleanStrings([]string{gap.Candidate.Field})
	}
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func outputProjectionCompletionMessage(graph OutputProjectionGraph, gap ReferenceProjectionGap) string {
	switch graph.Status {
	case OutputProjectionStatusIncompleteReference:
		candidate := gap.Candidate
		return fmt.Sprintf("validate data workflow completion: data output incomplete: final answer has %d item(s), but reference field %q in %q defines %d output key(s); run assemble_answer with complete_reference=true, reference_path, and reference_key_field to project missing zero/empty values without changing contribution records",
			graph.AnswerItemCount, candidate.Field, candidate.Path, candidate.KeyCount)
	case OutputProjectionStatusMissingProjection:
		return "validate data workflow completion: data output incomplete: output_contract requires final answer projection but result.answer is empty while reconcile groups are available"
	default:
		return outputProjectionDecisionReason(graph)
	}
}
