package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocRegisteredExportHandoff struct {
	callTarget            string
	exportSurface         string
	registeredCallable    string
	callEvidenceID        string
	bindingEvidenceID     string
	callLocation          string
	bindingLocation       string
	bindingEndpointStatus string
	downstreamCallStatus  string
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
		fmt.Fprintf(&b, "- registered_export_handoff[%d]: call_target=`%s`; registered_export=`%s`; registered_callable=`%s`; status=`call_targets_registered_export`; binding_endpoint_status=`%s`; downstream_execution_status=`%s`; call_evidence=`%s` @ `%s`; binding_evidence=`%s` @ `%s`.\n",
			i+1, answerDocCallChainInline(bridge.callTarget), answerDocCallChainInline(bridge.exportSurface), answerDocCallChainInline(bridge.registeredCallable),
			bridge.bindingEndpointStatus, bridge.downstreamCallStatus, answerDocCallChainInline(bridge.callEvidenceID), answerDocCallChainInline(bridge.callLocation),
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
			callableSurface := strings.TrimSpace(binding.Object)
			bindingStatus := "explicit_registration_endpoints"
			export := slot + "." + answerDocChainIdentityTail(callable)
			if callTarget == export && !strings.Contains(callable, ".") {
				// A direct export spelling can still carry an unqualified
				// registered callable. When the same exact owner/reference join
				// used by expression-style registrations identifies one unique
				// same-file wrapper, retain that qualified identity. Otherwise
				// leave the short callable unresolved; tail similarity alone is
				// never enough to choose among wrappers/core functions.
				if refinedSlot, refinedCallable, ok := answerDocOwnerReferenceRegistrationEndpoints(callTarget, binding, entries); ok {
					slot = refinedSlot
					callable = answerDocExactChainIdentity(refinedCallable)
					callableSurface = refinedCallable
					export = slot + "." + answerDocChainIdentityTail(callable)
					bindingStatus = "exact_export_plus_owner_reference_join"
				}
			} else if callTarget != export {
				var ok bool
				slot, callable, ok = answerDocOwnerReferenceRegistrationEndpoints(callTarget, binding, entries)
				if !ok {
					continue
				}
				callableSurface = callable
				callable = answerDocExactChainIdentity(callable)
				export = slot + "." + answerDocChainIdentityTail(callable)
				bindingStatus = "exact_owner_reference_join"
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
				callTarget: call.Object, exportSurface: export, registeredCallable: callableSurface,
				callEvidenceID: call.EvidenceID, bindingEvidenceID: binding.EvidenceID,
				callLocation: call.Location, bindingLocation: binding.Location,
				bindingEndpointStatus: bindingStatus,
				downstreamCallStatus:  downstream,
			})
		}
	}
	return out
}

// answerDocOwnerReferenceRegistrationEndpoints recovers a semantic export
// binding only from three already-typed facts: the exact external call target,
// the system-stamped enclosing owner of a grounded registration call, and one
// unique same-file callable owner referenced as a complete token by that
// registration expression. This is advisory handoff authority only; it does
// not create a call edge or mutate the source EvidenceItem.
func answerDocOwnerReferenceRegistrationEndpoints(callTarget string, binding types.AnswerSupportEntry, entries []types.AnswerSupportEntry) (string, string, bool) {
	if binding.AnchorKind != types.AnchorCall || strings.TrimSpace(binding.Source) == "" ||
		binding.Producer != types.EvidenceProducerExplorerEmitEvidence ||
		strings.TrimSpace(binding.OwnerSymbol) == "" || strings.TrimSpace(binding.Object) == "" {
		return "", "", false
	}
	target := answerDocExactChainIdentity(callTarget)
	cut := strings.LastIndex(target, ".")
	if cut <= 0 || cut == len(target)-1 {
		return "", "", false
	}
	exportOwner, exportMember := target[:cut], target[cut+1:]
	bindingOwner := answerDocExactChainIdentity(binding.OwnerSymbol)
	if bindingOwner != exportOwner && answerDocChainIdentityTail(bindingOwner) != exportOwner {
		return "", "", false
	}
	bindingNamespace := answerDocChainIdentityParent(bindingOwner)

	var candidateKey, candidateSurface string
	for _, entry := range entries {
		if entry.EvidenceID == binding.EvidenceID || entry.ClaimForm != types.ClaimCallEdge ||
			!answerDocRegisteredExportCallableProducerEligible(entry.Producer) ||
			strings.TrimSpace(entry.Source) != strings.TrimSpace(binding.Source) {
			continue
		}
		owner := answerDocExactChainIdentity(entry.Subject)
		if owner == "" || answerDocChainIdentityTail(owner) != exportMember ||
			!types.AnswerCodeSurfaceAppearsInText(binding.Object, exportMember) {
			continue
		}
		if candidateNamespace := answerDocChainIdentityParent(owner); bindingNamespace != "" && candidateNamespace != bindingNamespace {
			continue
		}
		if candidateKey != "" && candidateKey != owner {
			return "", "", false
		}
		candidateKey = owner
		candidateSurface = strings.TrimSpace(entry.Subject)
	}
	if candidateKey == "" {
		return "", "", false
	}
	return exportOwner, candidateSurface, true
}

// answerDocRegisteredExportCallableProducerEligible keeps the semantic join on
// exact, already-read operation evidence. A model-emitted call and the
// parser-owned call extracted from a model-selected callable body have the
// same citable source/direction authority here. The latter explicitly does not
// choose a path or conclusion; it only prevents an exact wrapper call from
// disappearing merely because the model selected the enclosing definition
// instead of redundantly emitting every body call itself.
func answerDocRegisteredExportCallableProducerEligible(producer string) bool {
	switch strings.TrimSpace(producer) {
	case types.EvidenceProducerExplorerEmitEvidence,
		types.EvidenceProducerRepoMapSelectedCallableBodyCall:
		return true
	default:
		return false
	}
}

func answerDocChainIdentityParent(raw string) string {
	raw = answerDocExactChainIdentity(raw)
	if idx := strings.LastIndex(raw, "."); idx > 0 {
		return raw[:idx]
	}
	return ""
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
