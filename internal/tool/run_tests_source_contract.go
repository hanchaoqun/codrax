package tool

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const postApplySourceContractMaxLineBytes = 1 << 20

// postApplySourceContractConfidenceRecords observes only source-value
// contracts whose typed evidence_ref identifies one exact line in a
// plan-owned repository file. It is deliberately language-neutral and does
// not parse request text, model prose, runtime output, or symbol names.
// Runtime/exception/state/placement/transition contracts remain executable
// probe or project-test obligations.
func postApplySourceContractConfidenceRecords(repoRoot string, plan *types.ChangePlan) []types.VerificationConfidenceRecord {
	if plan == nil || strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	owned := postApplySourceContractOwnedPaths(plan)
	if len(owned) == 0 {
		return nil
	}
	var out []types.VerificationConfidenceRecord
	for _, contract := range types.ChangePlanVerificationBehaviorContracts(plan) {
		if !postApplySourceContractEligible(contract) {
			continue
		}
		surface, ok := types.ParseAnswerSourceLocationSurface(contract.EvidenceRef)
		if !ok || surface.LineStart <= 0 || surface.LineStart != surface.LineEnd {
			continue
		}
		rel, ok := postApplySourceContractRepoPath(surface.File)
		if !ok || !owned[rel] {
			continue
		}
		line, ok := readPostApplySourceContractLine(repoRoot, rel, surface.LineStart)
		if !ok {
			continue
		}
		matched := postApplySourceContractMatches(contract.Operator, line, contract.Expected)
		status := "satisfied"
		severity := "info"
		reasonCode := "post_apply_source_contract_observed"
		detail := "the plan-owned post-apply source line satisfied the exact typed source-value contract"
		if !matched {
			status = "missing"
			severity = "warning"
			reasonCode = "post_apply_source_contract_value_mismatch"
			detail = "the plan-owned post-apply source line did not satisfy the exact typed source-value contract"
		}
		out = append(out, types.VerificationConfidenceRecord{
			Source:       "post_apply_source_observation",
			Category:     "source_contract_refs",
			Status:       status,
			Severity:     severity,
			ReasonCode:   reasonCode,
			ContractRefs: []string{strings.TrimSpace(contract.ID)},
			Detail:       detail,
		})
	}
	return out
}

func postApplySourceContractEligible(contract types.WriteBehaviorContract) bool {
	if !types.IsHardRequiredWriteBehaviorContract(contract) ||
		contract.Placement != nil || contract.Transition != nil || contract.Comparator != nil ||
		strings.TrimSpace(contract.Expected) == "" {
		return false
	}
	// These kinds can describe a source value. Output, exception, command and
	// status contracts inherently require execution and cannot be discharged by
	// finding similar bytes in a file.
	switch contract.Kind {
	case types.WriteBehaviorObservable, types.WriteBehaviorInvariant, types.WriteBehaviorFileLayout:
	default:
		return false
	}
	switch contract.Operator {
	case types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals,
		types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains:
		return true
	default:
		return false
	}
}

func postApplySourceContractOwnedPaths(plan *types.ChangePlan) map[string]bool {
	out := map[string]bool{}
	for _, raw := range types.ChangePlanVerificationTargetPaths(plan, nil) {
		if rel, ok := postApplySourceContractRepoPath(raw); ok {
			out[rel] = true
		}
	}
	return out
}

func postApplySourceContractRepoPath(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	if raw == "" || strings.HasPrefix(raw, "/") || filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", false
	}
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}
	return rel, true
}

func readPostApplySourceContractLine(repoRoot, rel string, lineNumber int) (string, bool) {
	if lineNumber <= 0 {
		return "", false
	}
	rootAbs, err := filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", false
	}
	resolvedTarget, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filepath.FromSlash(rel)))
	if err != nil || !pathWithinRoot(resolvedRoot, resolvedTarget) || samePath(resolvedRoot, resolvedTarget) {
		return "", false
	}
	file, err := os.Open(resolvedTarget)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), postApplySourceContractMaxLineBytes)
	for current := 1; scanner.Scan(); current++ {
		if current == lineNumber {
			return scanner.Text(), true
		}
	}
	return "", false
}

func postApplySourceContractMatches(operator types.WriteBehaviorOperator, actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	switch operator {
	case types.WriteBehaviorOpEquals:
		return actual == expected
	case types.WriteBehaviorOpNotEquals:
		return actual != expected
	case types.WriteBehaviorOpContains:
		return strings.Contains(actual, expected)
	case types.WriteBehaviorOpNotContains:
		return !strings.Contains(actual, expected)
	default:
		return false
	}
}
