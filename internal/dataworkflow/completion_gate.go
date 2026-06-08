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
	ledgerGuard, ledgerDep, ledgerBlocked := completionLedgerGuard(input.LedgerGraph)
	if input.ValidationErr != nil {
		if ledgerBlocked && validationErrorShouldYieldToLedger(input.ValidationErr) {
			return ledgerGuard
		}
		return validationErrorGuardResult(input.ValidationErr)
	}
	if ledgerBlocked && strings.TrimSpace(ledgerDep.Ledger) != string(LedgerFinalProjection) {
		return ledgerGuard
	}
	switch input.OutputGraph.Status {
	case OutputProjectionStatusIncompleteReference:
		return outputProjectionGuardResult(input.OutputGraph, input.ReferenceGap)
	case OutputProjectionStatusMissingProjection:
		return outputProjectionGuardResult(input.OutputGraph, input.ReferenceGap)
	case OutputProjectionStatusMissingAnswer:
		return outputProjectionGuardResult(input.OutputGraph, input.ReferenceGap)
	default:
		return GuardResult{}
	}
}

func completionLedgerGuard(graph LedgerGraph) (GuardResult, LedgerDependency, bool) {
	dep, ok := FirstIncompleteRequiredLedger(graph)
	if !ok {
		return GuardResult{}, LedgerDependency{}, false
	}
	guard := LedgerGraphCompletionGuardResult(graph)
	if guard.Empty() {
		return GuardResult{}, LedgerDependency{}, false
	}
	return guard, dep, true
}

func validationErrorShouldYieldToLedger(err error) bool {
	if validationErrorHasCode(err, "missing_required_ledger") {
		return true
	}
	return validationErrorHasAnyCode(err,
		"unlinked_rule_coverage",
		"unlinked_source_rule_coverage",
	)
}

func validationErrorHasCode(err error, code string) bool {
	return validationErrorHasAnyCode(err, code)
}

func validationErrorHasAnyCode(err error, codes ...string) bool {
	allowed := map[string]bool{}
	for _, code := range codes {
		if code = strings.TrimSpace(code); code != "" {
			allowed[code] = true
		}
	}
	if len(allowed) == 0 {
		return false
	}
	if err == nil {
		return false
	}
	var validationErr dataquery.DataValidationError
	if errors.As(err, &validationErr) {
		for _, violation := range validationErr.Violations {
			if allowed[strings.TrimSpace(violation.Code)] {
				return true
			}
		}
	}
	var resultErr *dataquery.DataResultValidationError
	if errors.As(err, &resultErr) {
		for _, violation := range resultErr.Violations {
			if allowed[strings.TrimSpace(violation.Code)] {
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
			Field:                strings.TrimSpace(violation.Field),
			Operation:            strings.TrimSpace(violation.Operation),
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
	case OutputProjectionStatusMissingAnswer:
		return "validate data workflow completion: data output incomplete: workflow has not produced a final answer candidate"
	default:
		return outputProjectionDecisionReason(graph)
	}
}
