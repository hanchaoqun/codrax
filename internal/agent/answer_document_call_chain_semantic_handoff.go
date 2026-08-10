package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocRegisteredExportHandoff struct {
	callTarget           string
	exportSurface        string
	registeredCallable   string
	callEvidenceID       string
	bindingEvidenceID    string
	callLocation         string
	bindingLocation      string
	downstreamCallStatus string
}

// renderAnswerDocCallChainSemanticHandoffs publishes only exact, typed joins
// already present in the current-code-path support lane. It is soft synthesis
// context: it neither validates answer prose nor creates an answer edge or
// conclusion. In particular, lexical proximity and short-name similarity do
// not close a native/FFI execution bridge.
func renderAnswerDocCallChainSemanticHandoffs(plan *types.AnswerSupportPlan) string {
	if plan == nil || plan.Family != types.QFCallChain {
		return ""
	}
	var entries []types.AnswerSupportEntry
	for _, lane := range plan.Lanes {
		if lane.Kind == types.SupportLaneCurrentCodePath {
			entries = append(entries, lane.Entries...)
		}
	}
	if len(entries) == 0 {
		return ""
	}

	bridges := answerDocRegisteredExportHandoffs(entries)
	guardGroups := answerDocGuardStateGroups(entries)
	if len(bridges) == 0 && len(guardGroups) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Typed bridge and branch handoff (advisory)\n\n")
	b.WriteString("- These rows are exact joins over typed support entries. They reduce ambiguity for the model but do not replace its explanation, create a call edge, or authorize a conclusion.\n")
	for i, bridge := range bridges {
		fmt.Fprintf(&b, "- registered_export_handoff[%d]: call_target=`%s`; registered_export=`%s`; registered_callable=`%s`; status=`call_targets_registered_export`; downstream_execution_status=`%s`; call_evidence=`%s` @ `%s`; binding_evidence=`%s` @ `%s`.\n",
			i+1, answerDocCallChainInline(bridge.callTarget), answerDocCallChainInline(bridge.exportSurface), answerDocCallChainInline(bridge.registeredCallable),
			bridge.downstreamCallStatus, answerDocCallChainInline(bridge.callEvidenceID), answerDocCallChainInline(bridge.callLocation),
			answerDocCallChainInline(bridge.bindingEvidenceID), answerDocCallChainInline(bridge.bindingLocation))
	}
	for i, group := range guardGroups {
		values := make([]string, 0, len(group.assignments))
		for _, assignment := range group.assignments {
			values = append(values, fmt.Sprintf("%s @ %s [%s]", answerDocCallChainInline(assignment.Object), answerDocCallChainInline(assignment.Location), answerDocCallChainInline(assignment.EvidenceID)))
		}
		state := "single_grounded_state_only"
		if len(group.assignments) > 1 {
			state = "multiple_grounded_states"
		}
		fmt.Fprintf(&b, "- guard_state_handoff[%d]: guard_owner=`%s`; guard_symbol=`%s`; grounded_state_writes=`%s`; state_coverage=`%s`; branch_call_mapping_status=`unproven_without_typed_branch_ownership`.\n",
			i+1, answerDocCallChainInline(group.guard.OwnerSymbol), answerDocCallChainInline(group.guard.AnchorSymbol), strings.Join(values, " | "), state)
	}
	if len(guardGroups) > 0 {
		b.WriteString("- Multiple grounded writes to one guard symbol are alternatives or updates, not one unconditional hard-coded value. A single observed write also does not prove a repository-wide constant. Keep each branch call separate unless a typed branch-ownership carrier proves its scope; do not infer containment from line order alone.\n")
	}
	b.WriteString("\n")
	return b.String()
}

func answerDocRegisteredExportHandoffs(entries []types.AnswerSupportEntry) []answerDocRegisteredExportHandoff {
	var calls, bindings []types.AnswerSupportEntry
	for _, entry := range entries {
		switch entry.ClaimForm {
		case types.ClaimCallEdge:
			calls = append(calls, entry)
		case types.ClaimRegistrationEdge:
			bindings = append(bindings, entry)
		}
	}
	var out []answerDocRegisteredExportHandoff
	seen := make(map[string]bool)
	for _, call := range calls {
		callTarget := answerDocExactChainIdentity(call.Object)
		if callTarget == "" {
			continue
		}
		for _, binding := range bindings {
			slot := answerDocExactChainIdentity(binding.Subject)
			callable := answerDocExactChainIdentity(binding.Object)
			if slot == "" || callable == "" {
				continue
			}
			export := slot + "." + answerDocChainIdentityTail(callable)
			if callTarget != export {
				continue
			}
			key := call.EvidenceID + "\x00" + binding.EvidenceID + "\x00" + export
			if seen[key] {
				continue
			}
			seen[key] = true
			downstream := "unproven"
			if strings.Contains(callable, ".") {
				for _, next := range calls {
					if answerDocExactChainIdentity(next.Subject) == callable {
						downstream = "proved_by_exact_registered_callable_call"
						break
					}
				}
			} else {
				downstream = "unresolved_from_unqualified_registered_callable"
			}
			out = append(out, answerDocRegisteredExportHandoff{
				callTarget: call.Object, exportSurface: export, registeredCallable: binding.Object,
				callEvidenceID: call.EvidenceID, bindingEvidenceID: binding.EvidenceID,
				callLocation: call.Location, bindingLocation: binding.Location,
				downstreamCallStatus: downstream,
			})
		}
	}
	return out
}

type answerDocGuardStateGroup struct {
	guard       types.AnswerSupportEntry
	assignments []types.AnswerSupportEntry
}

func answerDocGuardStateGroups(entries []types.AnswerSupportEntry) []answerDocGuardStateGroup {
	var out []answerDocGuardStateGroup
	for _, guard := range entries {
		if guard.ClaimForm != types.ClaimGuardCondition || guard.Source == "" || guard.AnchorSymbol == "" {
			continue
		}
		symbol := answerDocExactChainIdentity(guard.AnchorSymbol)
		var assignments []types.AnswerSupportEntry
		seen := make(map[string]bool)
		for _, entry := range entries {
			if entry.ClaimForm != types.ClaimAssignmentFact || entry.Source != guard.Source ||
				answerDocExactChainIdentity(entry.Subject) != symbol {
				continue
			}
			key := entry.EvidenceID + "\x00" + entry.Location + "\x00" + entry.Object
			if seen[key] {
				continue
			}
			seen[key] = true
			assignments = append(assignments, entry)
		}
		if len(assignments) > 0 {
			out = append(out, answerDocGuardStateGroup{guard: guard, assignments: assignments})
		}
	}
	return out
}

func answerDocExactChainIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "->", ".")
	raw = strings.ReplaceAll(raw, "::", ".")
	raw = strings.ReplaceAll(raw, "#", ".")
	return strings.Trim(raw, ".")
}

func answerDocChainIdentityTail(raw string) string {
	raw = answerDocExactChainIdentity(raw)
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		return raw[idx+1:]
	}
	return raw
}
