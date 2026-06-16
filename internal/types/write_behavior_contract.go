package types

import (
	"fmt"
	"strings"
)

// WriteBehaviorContract is a typed observable that the write workflow should
// preserve or satisfy. The write_analyzer emits these atoms through
// emit_write_analysis; downstream validators only check IDs/enums/coverage
// relationships and never infer contract semantics from prose.
type WriteBehaviorContract struct {
	ID          string                    `json:"id"`
	Kind        WriteBehaviorContractKind `json:"kind"`
	Subject     string                    `json:"subject,omitempty"`
	Operator    WriteBehaviorOperator     `json:"operator,omitempty"`
	Expected    string                    `json:"expected,omitempty"`
	EvidenceRef string                    `json:"evidence_ref,omitempty"`
	Required    bool                      `json:"required,omitempty"`
	Source      string                    `json:"source,omitempty"`
}

type WriteBehaviorContractKind string

const (
	WriteBehaviorObservable    WriteBehaviorContractKind = "observable"
	WriteBehaviorException     WriteBehaviorContractKind = "exception"
	WriteBehaviorOutputPath    WriteBehaviorContractKind = "output_path"
	WriteBehaviorStdout        WriteBehaviorContractKind = "stdout"
	WriteBehaviorStatusCode    WriteBehaviorContractKind = "status_code"
	WriteBehaviorFileLayout    WriteBehaviorContractKind = "file_layout"
	WriteBehaviorCommandResult WriteBehaviorContractKind = "command_result"
	WriteBehaviorInvariant     WriteBehaviorContractKind = "invariant"
)

type WriteBehaviorOperator string

const (
	WriteBehaviorOpSatisfies WriteBehaviorOperator = "satisfies"
	WriteBehaviorOpEquals    WriteBehaviorOperator = "equals"
	WriteBehaviorOpContains  WriteBehaviorOperator = "contains"
	WriteBehaviorOpExists    WriteBehaviorOperator = "exists"
	WriteBehaviorOpNotExists WriteBehaviorOperator = "not_exists"
	WriteBehaviorOpRaises    WriteBehaviorOperator = "raises"
	WriteBehaviorOpReturns   WriteBehaviorOperator = "returns"
)

func IsKnownWriteBehaviorContractKind(v string) bool {
	switch WriteBehaviorContractKind(v) {
	case WriteBehaviorObservable, WriteBehaviorException, WriteBehaviorOutputPath,
		WriteBehaviorStdout, WriteBehaviorStatusCode, WriteBehaviorFileLayout,
		WriteBehaviorCommandResult, WriteBehaviorInvariant:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorOperator(v string) bool {
	switch WriteBehaviorOperator(v) {
	case WriteBehaviorOpSatisfies, WriteBehaviorOpEquals, WriteBehaviorOpContains,
		WriteBehaviorOpExists, WriteBehaviorOpNotExists, WriteBehaviorOpRaises,
		WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

// NormalizeWriteBehaviorContracts validates and normalizes analyzer-emitted
// contract atoms. When no structured atoms are emitted but expected_outcomes
// exist, it creates generic observable atoms that preserve the outcome text
// without attempting semantic parsing.
func NormalizeWriteBehaviorContracts(in []WriteBehaviorContract, expectedOutcomes []string) []WriteBehaviorContract {
	out := make([]WriteBehaviorContract, 0, len(in)+len(expectedOutcomes))
	seen := map[string]struct{}{}
	for i, c := range in {
		c.ID = strings.TrimSpace(c.ID)
		if c.ID == "" {
			c.ID = fmt.Sprintf("contract-%d", i+1)
		}
		c.Kind = WriteBehaviorContractKind(strings.ToLower(strings.TrimSpace(string(c.Kind))))
		if !IsKnownWriteBehaviorContractKind(string(c.Kind)) {
			c.Kind = WriteBehaviorObservable
		}
		c.Operator = WriteBehaviorOperator(strings.ToLower(strings.TrimSpace(string(c.Operator))))
		if !IsKnownWriteBehaviorOperator(string(c.Operator)) {
			c.Operator = WriteBehaviorOpSatisfies
		}
		c.Subject = strings.TrimSpace(c.Subject)
		c.Expected = strings.TrimSpace(c.Expected)
		c.EvidenceRef = strings.TrimSpace(c.EvidenceRef)
		c.Source = strings.TrimSpace(c.Source)
		if c.Source == "" {
			c.Source = "write_analyzer"
		}
		if c.Expected == "" && c.Subject == "" {
			continue
		}
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		for i, outcome := range expectedOutcomes {
			outcome = strings.TrimSpace(outcome)
			if outcome == "" {
				continue
			}
			id := fmt.Sprintf("outcome-%d", i+1)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, WriteBehaviorContract{
				ID:       id,
				Kind:     WriteBehaviorObservable,
				Operator: WriteBehaviorOpSatisfies,
				Expected: outcome,
				Required: true,
				Source:   "expected_outcome_fallback",
			})
		}
	}
	const maxContracts = 12
	if len(out) > maxContracts {
		out = out[:maxContracts]
	}
	return out
}

func WriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if id := strings.TrimSpace(c.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}
