package dataworkflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// output_contract_admission.go — V9-4 (colleague_merge_audit §40.27 /
// §40.56): the output contract is ONE identifier space resolved from ONE
// snapshot. The planner's pre-dispatch gate judges every action against the
// contract the plan will execute under (the REPL/CLI resolver carries the
// durable workflow contract into the draft before the gate runs), and every
// plan that never passed that gate — deferred remainders, deterministic
// fallbacks, resumed checkpoints — is judged here, at admission and again
// before dispatch, by the same executor normalizer under the same carried
// contract. The executor's own check stays as the fail-closed backstop.

// GuardCodeActionOutputContractDrift is the typed admission code for an
// action whose executor verdict depends on the output contract in effect and
// is negative under that contract. It is a typed escape lane, not a stop:
// admission routes it through the ordinary repair/fallback recovery.
const GuardCodeActionOutputContractDrift = "action_output_contract_drift"

// OutputContractDeclared reports whether a contract carries any typed
// declaration (ResolveOutputContract treats an undeclared contract as absent).
func OutputContractDeclared(contract dataquery.OutputContract) bool {
	return outputContractSpecificity(contract) >= 0
}

// ActionOutputContractGuardResult judges every action of plan against
// plan.OutputContract with the executor's admission normalizer. Only
// contract-dependent rejections (dataquery.DataActionParamError with a
// non-empty OutputFormat) become the drift guard; contract-independent
// parameter defects keep their existing lanes. Plans without actions and
// plans whose actions normalize cleanly return an empty guard, so system
// built projection plans pass unchanged.
func ActionOutputContractGuardResult(plan dataquery.TaskPlan) GuardResult {
	for i, action := range plan.Actions {
		if _, err := dataquery.NormalizeDataActionForOutputContract(action, plan.OutputContract); err != nil {
			var paramErr dataquery.DataActionParamError
			if !errors.As(err, &paramErr) || !paramErr.OutputContractDependent() {
				continue
			}
			return actionOutputContractDriftGuard(plan, action, i, paramErr)
		}
	}
	return GuardResult{}
}

func actionOutputContractDriftGuard(plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int, paramErr dataquery.DataActionParamError) GuardResult {
	format := strings.TrimSpace(string(paramErr.OutputFormat))
	if format == "" {
		format = strings.TrimSpace(string(plan.OutputContract.Normalize().Format))
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) cannot satisfy the output contract in effect for this workflow (output_contract.format=%s): %s. Choose a projection for that format, or revise output_contract explicitly; the executor judges every action against the same contract.",
		guardActionNumber(actionIndex),
		guardActionLabel(action),
		format,
		strings.TrimSpace(paramErr.Error()))
	violation := WorkflowViolationFromDataTaskViolation(paramErr.Violation(), action)
	violation.Reason = message
	if strings.TrimSpace(violation.ActionID) == "" {
		violation.ActionID = strings.TrimSpace(action.ID)
	}
	return NewGuardResult(GuardCodeActionOutputContractDrift, "error", RepairNeedsTypedAction, message, violation)
}
