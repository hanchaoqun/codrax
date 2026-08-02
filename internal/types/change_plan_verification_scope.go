package types

import (
	"sort"
	"strings"
)

// ChangePlanVerificationTargetPaths returns the active plan/slice paths plus
// controller-owned paths retained from earlier plans. Apply callers must keep
// using ActiveChangePlanApplyTargetPaths; this helper is verify-only.
func ChangePlanVerificationTargetPaths(plan *ChangePlan, run *WriteWorkflowRun) []string {
	current, _ := ActiveChangePlanApplyTargetPaths(plan, run)
	if plan == nil || plan.CumulativeVerificationScope == nil {
		return dedupVerificationScopeStrings(current)
	}
	return dedupVerificationScopeStrings(append(current, plan.CumulativeVerificationScope.TargetPaths...))
}

// ChangePlanVerificationBehaviorContracts returns the active plan contracts
// plus retained contracts from earlier still-applied plans.
func ChangePlanVerificationBehaviorContracts(plan *ChangePlan) []WriteBehaviorContract {
	if plan == nil {
		return nil
	}
	out := make([]WriteBehaviorContract, 0, len(plan.BehaviorContracts))
	seen := map[string]bool{}
	add := func(contract WriteBehaviorContract) {
		key := strings.TrimSpace(contract.ID)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, contract)
	}
	for _, contract := range plan.BehaviorContracts {
		add(contract)
	}
	if plan.CumulativeVerificationScope != nil {
		for _, contract := range plan.CumulativeVerificationScope.BehaviorContracts {
			add(contract)
		}
	}
	return out
}

// ChangePlanVerificationProbes returns the active plan probes plus retained
// probes from earlier still-applied plans.
func ChangePlanVerificationProbes(plan *ChangePlan) []VerificationProbe {
	if plan == nil {
		return nil
	}
	out := make([]VerificationProbe, 0, len(plan.VerificationProbes))
	seen := map[string]bool{}
	add := func(probe VerificationProbe) {
		key := strings.TrimSpace(probe.ID)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, probe)
	}
	for _, probe := range plan.VerificationProbes {
		add(probe)
	}
	if plan.CumulativeVerificationScope != nil {
		for _, probe := range plan.CumulativeVerificationScope.VerificationProbes {
			add(probe)
		}
	}
	return out
}

func dedupVerificationScopeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		item := strings.TrimSpace(raw)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
