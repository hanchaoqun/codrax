package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// writeAnalysisIRQualityRejection returns a retry-worthy structural rejection
// for WriteAnalysisIR facts that are too precise to become P0 planning
// contracts without typed grounding. The check reads only the structured IR and
// verbatim raw-request evidence. It deliberately does not classify user intent
// by keywords or inspect model prose/rationale.
func writeAnalysisIRQualityRejection(ir *types.WriteAnalysisIR) string {
	if ir == nil {
		return ""
	}
	raw := strings.TrimSpace(ir.Request.RawRequest)
	if raw == "" {
		return ""
	}
	for i, contract := range ir.Request.BehaviorContracts {
		if !writeAnalysisContractNeedsExactGrounding(contract) {
			continue
		}
		expected := strings.TrimSpace(contract.Expected)
		if expected == "" || strings.Contains(raw, expected) {
			continue
		}
		if writeBehaviorComparatorGroundsExactExpected(contract.Comparator, raw) {
			continue
		}
		return fmt.Sprintf(
			"behavior_contracts[%d] id=%q uses exact operator %q with expected=%q, but that exact expected value is not present in raw_request and no grounded comparator is attached",
			i, contract.ID, contract.Operator, expected,
		)
	}
	return ""
}

func writeAnalysisContractNeedsExactGrounding(contract types.WriteBehaviorContract) bool {
	if !contract.Required || contract.Polarity == types.WriteBehaviorPolarityObserved {
		return false
	}
	switch contract.Operator {
	case types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals, types.WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

func writeBehaviorComparatorGroundsExactExpected(c *types.WriteBehaviorComparator, rawRequest string) bool {
	if c == nil {
		return false
	}
	if evidenceRef := strings.TrimSpace(c.EvidenceRef); evidenceRef != "" {
		return true
	}
	expected := strings.TrimSpace(c.Expected)
	return expected != "" && strings.Contains(rawRequest, expected)
}
