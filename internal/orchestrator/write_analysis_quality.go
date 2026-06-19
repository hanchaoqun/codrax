package orchestrator

import (
	"fmt"
	"sort"
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
		if rejection := writeAnalysisPlacementQualityRejection(i, contract, raw); rejection != "" {
			return rejection
		}
		if !writeAnalysisContractNeedsExactGrounding(contract) {
			continue
		}
		expected := strings.TrimSpace(contract.Expected)
		if expected == "" || strings.Contains(raw, expected) {
			continue
		}
		if writeBehaviorContractGroundsExactExpected(contract, raw) {
			continue
		}
		return fmt.Sprintf(
			"behavior_contracts[%d] id=%q uses exact operator %q with expected=%q, but that exact expected value is not present in raw_request and no grounded comparator is attached",
			i, contract.ID, contract.Operator, expected,
		)
	}
	return ""
}

func repairWriteAnalysisIRQuality(ir *types.WriteAnalysisIR) (*types.WriteAnalysisIR, []string) {
	if ir == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(ir.Request.RawRequest)
	if raw == "" || len(ir.Request.BehaviorContracts) == 0 {
		return ir, nil
	}
	repaired := cloneWriteAnalysisIRForQualityRepair(ir)
	var repairs []string
	for i := range repaired.Request.BehaviorContracts {
		contract := &repaired.Request.BehaviorContracts[i]
		if rejection := writeAnalysisPlacementQualityRejection(i, *contract, raw); rejection != "" {
			oldOperator := contract.Operator
			contract.Placement = nil
			if contract.Required && contract.Polarity != types.WriteBehaviorPolarityObserved &&
				contract.Operator != types.WriteBehaviorOpSatisfies {
				contract.Operator = types.WriteBehaviorOpSatisfies
			}
			contract.Source = appendWriteAnalysisContractSource(contract.Source, "quality_repaired:dropped_invalid_placement")
			repairs = append(repairs, fmt.Sprintf("behavior_contracts[%d] id=%q dropped invalid placement and operator=%s->%s", i, contract.ID, oldOperator, contract.Operator))
			continue
		}
		if !writeAnalysisContractNeedsExactGrounding(*contract) {
			continue
		}
		expected := strings.TrimSpace(contract.Expected)
		if expected == "" || strings.Contains(raw, expected) {
			continue
		}
		if writeBehaviorContractGroundsExactExpected(*contract, raw) {
			continue
		}
		oldOperator := contract.Operator
		contract.Operator = types.WriteBehaviorOpSatisfies
		contract.Source = appendWriteAnalysisContractSource(contract.Source, "quality_repaired:softened_ungrounded_exact")
		repairs = append(repairs, fmt.Sprintf("behavior_contracts[%d] id=%q operator=%s->%s", i, contract.ID, oldOperator, contract.Operator))
	}
	if len(repairs) == 0 {
		return ir, nil
	}
	sort.Strings(repairs)
	return repaired, repairs
}

func writeAnalysisPlacementQualityRejection(index int, contract types.WriteBehaviorContract, raw string) string {
	if contract.Placement == nil {
		return ""
	}
	p := contract.Placement
	var missing []string
	if strings.TrimSpace(string(p.Surface)) == "" {
		missing = append(missing, "placement.surface")
	}
	if strings.TrimSpace(p.Anchor) == "" {
		missing = append(missing, "placement.anchor")
	}
	if writeAnalysisPlacementExpected(contract) == "" {
		missing = append(missing, "placement.expected")
	}
	if strings.TrimSpace(string(p.Relation)) == "" {
		missing = append(missing, "placement.relation")
	}
	if writeRenderedTextRelationRequiresDelimiter(p.Relation) && strings.TrimSpace(p.Delimiter) == "" {
		missing = append(missing, "placement.delimiter")
	}
	if len(missing) > 0 {
		return fmt.Sprintf(
			"behavior_contracts[%d] id=%q carries rendered-text placement but is missing %s; placement contracts require typed surface, anchor, expected, relation, and delimiter for boundary relations",
			index, contract.ID, strings.Join(missing, ", "),
		)
	}
	if !writeAnalysisPlacementGrounded(contract, raw) {
		return fmt.Sprintf(
			"behavior_contracts[%d] id=%q carries rendered-text placement but anchor=%q and expected=%q are not both present in raw_request and no placement/contract evidence_ref is attached",
			index, contract.ID, strings.TrimSpace(p.Anchor), writeAnalysisPlacementExpected(contract),
		)
	}
	return ""
}

func writeAnalysisPlacementExpected(contract types.WriteBehaviorContract) string {
	if contract.Placement != nil {
		if expected := strings.TrimSpace(contract.Placement.Expected); expected != "" {
			return expected
		}
	}
	return strings.TrimSpace(contract.Expected)
}

func writeRenderedTextRelationRequiresDelimiter(relation types.WriteRenderedTextRelation) bool {
	switch relation {
	case types.WriteRenderedTextSuffixBeforeDelimiter,
		types.WriteRenderedTextPrefixAfterDelimiter,
		types.WriteRenderedTextBetweenAnchorAndDelimiter:
		return true
	default:
		return false
	}
}

func writeAnalysisPlacementGrounded(contract types.WriteBehaviorContract, raw string) bool {
	if strings.TrimSpace(contract.EvidenceRef) != "" {
		return true
	}
	if contract.Placement == nil {
		return true
	}
	if strings.TrimSpace(contract.Placement.EvidenceRef) != "" {
		return true
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	anchor := strings.TrimSpace(contract.Placement.Anchor)
	expected := writeAnalysisPlacementExpected(contract)
	return anchor != "" && expected != "" && strings.Contains(raw, anchor) && strings.Contains(raw, expected)
}

func writeAnalysisContractNeedsExactGrounding(contract types.WriteBehaviorContract) bool {
	if !contract.Required || contract.Polarity == types.WriteBehaviorPolarityObserved {
		return false
	}
	switch contract.Operator {
	case types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals,
		types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains,
		types.WriteBehaviorOpExists, types.WriteBehaviorOpNotExists,
		types.WriteBehaviorOpRaises, types.WriteBehaviorOpNotRaises,
		types.WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

func writeBehaviorContractGroundsExactExpected(contract types.WriteBehaviorContract, rawRequest string) bool {
	if strings.TrimSpace(contract.EvidenceRef) != "" {
		return true
	}
	if contract.Placement != nil && strings.TrimSpace(contract.Placement.EvidenceRef) != "" {
		return true
	}
	return writeBehaviorComparatorGroundsExactExpected(contract.Comparator, rawRequest)
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

func cloneWriteAnalysisIRForQualityRepair(ir *types.WriteAnalysisIR) *types.WriteAnalysisIR {
	if ir == nil {
		return nil
	}
	repaired := *ir
	repaired.Request = ir.Request
	if len(ir.Request.BehaviorContracts) > 0 {
		repaired.Request.BehaviorContracts = make([]types.WriteBehaviorContract, len(ir.Request.BehaviorContracts))
		for i, contract := range ir.Request.BehaviorContracts {
			repaired.Request.BehaviorContracts[i] = contract
			if contract.Placement != nil {
				placement := *contract.Placement
				repaired.Request.BehaviorContracts[i].Placement = &placement
			}
			if contract.Comparator != nil {
				comparator := *contract.Comparator
				repaired.Request.BehaviorContracts[i].Comparator = &comparator
			}
		}
	}
	return &repaired
}

func appendWriteAnalysisContractSource(source, marker string) string {
	source = strings.TrimSpace(source)
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return source
	}
	if source == "" {
		return marker
	}
	for _, part := range strings.Split(source, ";") {
		if strings.TrimSpace(part) == marker {
			return source
		}
	}
	return source + ";" + marker
}
