package types

import (
	"sort"
	"strings"
)

// Frozen pre-index scan oracle for B1580. Only the five scan functions are
// copied; relation shape/dedup helpers remain unchanged production semantics.
// Keep this independent of index construction so a lost bucket cannot make
// both the optimized result and its oracle agree on the same missing fact.

func b1580ScanReferenceCompileDynamicSelectorResolutionPaths(evidence []EvidenceItem, entryIdentity string) DynamicSelectorResolutionCompilation {
	out := DynamicSelectorResolutionCompilation{Version: DynamicSelectorResolutionPathVersion}
	entryIdentity = strings.TrimSpace(entryIdentity)

	groups := make(map[string][]dynamicSelectorApplicationCandidate)
	groupOrder := make([]string, 0)
	for _, item := range evidence {
		if !item.IsCitable() || item.SelectorApplication == nil ||
			strings.TrimSpace(item.Predicate) != "decorator_selector_application" {
			continue
		}
		owner := strings.TrimSpace(item.SelectorApplication.Owner)
		selector := strings.TrimSpace(item.SelectorApplication.Literal)
		candidate := strings.TrimSpace(item.Object)
		ownerKey := AnswerCodeIdentitySurfaceKey(owner)
		if ownerKey == "" || selector == "" || candidate == "" || strings.ContainsAny(selector, "\x00\r\n") {
			out.Rejected = append(out.Rejected, DynamicSelectorResolutionRejection{
				SelectorLiteral: selector,
				SelectorOwner:   owner,
				Reason:          DynamicSelectorRejectInvalidApplication,
				EvidenceIDs:     []string{dynamicSelectorEvidenceID(item)},
			})
			continue
		}
		groupKey := ownerKey + "\x00" + selector
		if _, exists := groups[groupKey]; !exists {
			groupOrder = append(groupOrder, groupKey)
		}
		groups[groupKey] = append(groups[groupKey], dynamicSelectorApplicationCandidate{
			item: item, ownerKey: ownerKey, selector: selector, candidate: candidate,
		})
	}

	for _, groupKey := range groupOrder {
		apps := dynamicSelectorUniqueApplications(groups[groupKey])
		if len(apps) == 0 {
			continue
		}
		if distinctDynamicSelectorApplicationCandidates(apps) != 1 {
			out.Rejected = append(out.Rejected, DynamicSelectorResolutionRejection{
				SelectorLiteral: apps[0].selector,
				SelectorOwner:   strings.TrimSpace(apps[0].item.SelectorApplication.Owner),
				Reason:          DynamicSelectorRejectAmbiguousCandidate,
				EvidenceIDs:     dynamicSelectorApplicationEvidenceIDs(apps),
			})
			continue
		}
		app := apps[0]
		path, reason, ids := b1580ScanReferencecompileOneDynamicSelectorResolutionPath(evidence, app.item, entryIdentity)
		if reason != "" {
			out.Rejected = append(out.Rejected, DynamicSelectorResolutionRejection{
				SelectorLiteral: app.selector,
				SelectorOwner:   strings.TrimSpace(app.item.SelectorApplication.Owner),
				Reason:          reason,
				EvidenceIDs:     ids,
			})
			continue
		}
		out.Candidates = append(out.Candidates, path)
	}

	sort.SliceStable(out.Candidates, func(i, j int) bool {
		if out.Candidates[i].SelectorLiteral != out.Candidates[j].SelectorLiteral {
			return out.Candidates[i].SelectorLiteral < out.Candidates[j].SelectorLiteral
		}
		return out.Candidates[i].CandidateIdentity < out.Candidates[j].CandidateIdentity
	})
	return out
}

func b1580ScanReferencecompileOneDynamicSelectorResolutionPath(evidence []EvidenceItem, app EvidenceItem, requestedEntry string) (DynamicSelectorResolutionPath, DynamicSelectorResolutionRejectionReason, []string) {
	selector := app.SelectorApplication
	appID := dynamicSelectorEvidenceID(app)

	var bindings []dynamicSelectorBindingCandidate
	for _, item := range evidence {
		if !item.IsCitable() || !dynamicSelectorIdentityEquivalent(item.OwnerSymbol, selector.Owner) {
			continue
		}
		var container, value string
		switch ClaimFormOf(item) {
		case ClaimRegistrationEdge:
			container = dynamicSelectorContainerIdentity(item.Subject)
			value = dynamicSelectorValueIdentity(item.Object)
		case ClaimAssignmentFact:
			_, indexedContainer, assigned, ok := IndexedAssignmentEvidenceEndpoints(item)
			if !ok {
				continue
			}
			// An ordinary property/local assignment inside a selector helper
			// is not a selector binding. The exact indexed receiver is the
			// minimum source fact that supplies a keyed container without
			// upgrading assignment semantics to registration.
			container = dynamicSelectorValueIdentity(indexedContainer)
			value = dynamicSelectorValueIdentity(assigned)
		default:
			continue
		}
		if container == "" || value == "" {
			continue
		}
		bindings = append(bindings, dynamicSelectorBindingCandidate{item: item, container: container, containerKey: AnswerCodeIdentitySurfaceKey(container), value: value})
	}
	bindings = uniqueDynamicSelectorBindings(bindings)
	if len(bindings) == 0 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectBindingUnavailable, []string{appID}
	}
	if distinctDynamicSelectorBindingShapes(bindings) != 1 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectAmbiguousContainer, append([]string{appID}, dynamicSelectorBindingEvidenceIDs(bindings)...)
	}
	bindingRow := bindings[0]

	var lookups []dynamicSelectorLookupCandidate
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimAssignmentFact || !AssignmentEvidenceEndpointsMatch(item) {
			continue
		}
		receiver, _, ok := AssignmentEvidenceEndpoints(item)
		indexedContainer, indexed := IndexedAssignmentValueContainer(item)
		if !ok || !indexed || !dynamicSelectorIdentityEquivalent(receiver, bindingRow.value) ||
			AnswerCodeIdentitySurfaceKey(indexedContainer) != bindingRow.containerKey {
			continue
		}
		owner := strings.TrimSpace(item.OwnerSymbol)
		if AnswerCodeIdentitySurfaceKey(owner) == "" {
			continue
		}
		lookups = append(lookups, dynamicSelectorLookupCandidate{item: item, owner: owner, receiver: receiver, container: indexedContainer})
	}
	lookups = uniqueDynamicSelectorLookups(lookups)
	if len(lookups) == 0 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectLookupUnavailable, []string{appID, dynamicSelectorEvidenceID(bindingRow.item)}
	}
	if distinctDynamicSelectorLookupShapes(lookups) != 1 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectAmbiguousLookup, append([]string{appID, dynamicSelectorEvidenceID(bindingRow.item)}, dynamicSelectorLookupEvidenceIDs(lookups)...)
	}
	lookupRow := lookups[0]

	var returns []dynamicSelectorReturnCandidate
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimReturnFact ||
			!dynamicSelectorIdentityEquivalent(firstNonEmptyDynamicSelectorIdentity(item.OwnerSymbol, item.Subject), lookupRow.owner) {
			continue
		}
		callee := dynamicSelectorInvocationCallee(item.Object)
		if callee == "" || !dynamicSelectorIdentityEquivalent(callee, lookupRow.receiver) {
			continue
		}
		returns = append(returns, dynamicSelectorReturnCandidate{item: item, expression: strings.TrimSpace(item.Object)})
	}
	returns = uniqueDynamicSelectorReturns(returns)
	if len(returns) == 0 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectReturnUnavailable, []string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item)}
	}
	if distinctDynamicSelectorReturnShapes(returns) != 1 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectAmbiguousReturn, append([]string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item)}, dynamicSelectorReturnEvidenceIDs(returns)...)
	}
	returnRow := returns[0]

	var entries []dynamicSelectorEntryCandidate
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimCallEdge ||
			!dynamicSelectorIdentityEquivalent(item.Object, lookupRow.owner) {
			continue
		}
		// A call edge's Subject is its typed source endpoint. OwnerSymbol is
		// enclosing/source qualification and can legitimately be more qualified
		// (for example pipeline.runner.run_pipeline); it is only a fallback for
		// deterministic legacy rows whose Subject is absent. Preferring the owner
		// here makes an otherwise exact run_pipeline -> resolve edge fail entry
		// matching without adding any evidence or ambiguity.
		from := strings.TrimSpace(firstNonEmptyDynamicSelectorIdentity(item.Subject, item.OwnerSymbol))
		if from == "" || (requestedEntry != "" && !dynamicSelectorIdentityEquivalent(from, requestedEntry)) {
			continue
		}
		entries = append(entries, dynamicSelectorEntryCandidate{item: item, from: from})
	}
	entries = uniqueDynamicSelectorEntries(entries)
	if len(entries) == 0 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectEntryUnavailable, []string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item), dynamicSelectorEvidenceID(returnRow.item)}
	}
	if distinctDynamicSelectorEntryShapes(entries) != 1 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectAmbiguousEntry, append([]string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item), dynamicSelectorEvidenceID(returnRow.item)}, dynamicSelectorEntryEvidenceIDs(entries)...)
	}
	entry := entries[0]

	var arguments []dynamicSelectorArgumentCandidate
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimArgumentFlow ||
			!dynamicSelectorIdentityEquivalent(item.Object, lookupRow.owner) ||
			strings.TrimSpace(item.Source) != strings.TrimSpace(entry.item.Source) ||
			item.LineStart != entry.item.LineStart {
			continue
		}
		argument := strings.TrimSpace(item.Subject)
		if argument == "" {
			continue
		}
		arguments = append(arguments, dynamicSelectorArgumentCandidate{item: item, argument: argument})
	}
	arguments = uniqueDynamicSelectorArguments(arguments)
	if len(arguments) == 0 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectArgumentUnavailable, []string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item), dynamicSelectorEvidenceID(returnRow.item), dynamicSelectorEvidenceID(entry.item)}
	}
	if distinctDynamicSelectorArgumentShapes(arguments) != 1 {
		return DynamicSelectorResolutionPath{}, DynamicSelectorRejectAmbiguousArgument, append([]string{appID, dynamicSelectorEvidenceID(bindingRow.item), dynamicSelectorEvidenceID(lookupRow.item), dynamicSelectorEvidenceID(returnRow.item), dynamicSelectorEvidenceID(entry.item)}, dynamicSelectorArgumentEvidenceIDs(arguments)...)
	}
	argument := arguments[0]

	path := DynamicSelectorResolutionPath{
		Version:           DynamicSelectorResolutionPathVersion,
		Status:            DynamicSelectorResolutionCandidateOnly,
		EntryIdentity:     entry.from,
		SelectorArgument:  argument.argument,
		SelectorOwner:     strings.TrimSpace(selector.Owner),
		SelectorLiteral:   strings.TrimSpace(selector.Literal),
		ContainerIdentity: bindingRow.container,
		LookupIdentity:    lookupRow.owner,
		CandidateIdentity: strings.TrimSpace(app.Object),
		Hops: []DynamicSelectorResolutionHop{
			dynamicSelectorHop(DynamicSelectorHopEntryCall, DiagramRelCall, entry.item, entry.from, lookupRow.owner),
			dynamicSelectorHop(DynamicSelectorHopSelectorArgument, DiagramRelArgumentFlow, argument.item, argument.argument, lookupRow.owner),
			dynamicSelectorHop(DynamicSelectorHopSelectorApplication, DiagramRelUnknown, app, strings.TrimSpace(selector.Owner), strings.TrimSpace(app.Object)),
			dynamicSelectorHop(DynamicSelectorHopRegistration, dynamicSelectorBindingRelationKind(bindingRow.item), bindingRow.item, bindingRow.container, bindingRow.value),
			dynamicSelectorHop(DynamicSelectorHopLookupAssignment, DiagramRelAssignment, lookupRow.item, lookupRow.container, lookupRow.receiver),
			dynamicSelectorHop(DynamicSelectorHopFactoryReturn, DiagramRelReturn, returnRow.item, lookupRow.owner, returnRow.expression),
		},
	}
	path.CallbackHops = b1580ScanReferencecompileDynamicSelectorCallbackHops(evidence, entry.from)
	path.TypeRoster = b1580ScanReferencecompileDynamicSelectorTypeRoster(evidence, path.CandidateIdentity)
	return path, "", nil
}

func b1580ScanReferencecompileDynamicSelectorCallbackHops(evidence []EvidenceItem, entry string) []DynamicSelectorResolutionHop {
	var calls []EvidenceItem
	for _, item := range evidence {
		if item.IsCitable() && ClaimFormOf(item) == ClaimCallEdge &&
			dynamicSelectorIdentityEquivalent(firstNonEmptyDynamicSelectorIdentity(item.OwnerSymbol, item.Subject), entry) {
			calls = append(calls, item)
		}
	}
	var out []DynamicSelectorResolutionHop
	seenCalls := make(map[string]bool)
	seenHandoffs := make(map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimCallbackHandoff {
			continue
		}
		for _, call := range calls {
			if !dynamicSelectorIdentityEquivalent(call.Object, item.Subject) {
				continue
			}
			callID := dynamicSelectorEvidenceID(call)
			if !seenCalls[callID] {
				seenCalls[callID] = true
				out = append(out, dynamicSelectorHop(
					DynamicSelectorHopCallbackReceiverCall,
					DiagramRelCall,
					call,
					firstNonEmptyDynamicSelectorIdentity(call.Subject, call.OwnerSymbol),
					call.Object,
				))
			}
			handoffID := dynamicSelectorEvidenceID(item)
			if !seenHandoffs[handoffID] {
				seenHandoffs[handoffID] = true
				out = append(out, dynamicSelectorHop(DynamicSelectorHopCallbackHandoff, DiagramRelCallback, item, item.Subject, item.Object))
			}
		}
	}
	return out
}

func b1580ScanReferencecompileDynamicSelectorTypeRoster(evidence []EvidenceItem, candidate string) []DynamicSelectorResolutionHop {
	var out []DynamicSelectorResolutionHop
	seen := make(map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || !IsRepoMapTypeRelationEvidence(item) || !dynamicSelectorIdentityEquivalent(item.Subject, candidate) {
			continue
		}
		id := dynamicSelectorEvidenceID(item)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, dynamicSelectorHop(DynamicSelectorHopTypeRelation, DiagramRelTypeRelation, item, item.Subject, item.Object))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return b1580ScanReferencedynamicSelectorRelationOrdinal(evidence, out[i].EvidenceID) < b1580ScanReferencedynamicSelectorRelationOrdinal(evidence, out[j].EvidenceID)
	})
	return out
}

func b1580ScanReferencedynamicSelectorRelationOrdinal(evidence []EvidenceItem, id string) int {
	for _, item := range evidence {
		if dynamicSelectorEvidenceID(item) == id {
			if item.RelationOrdinal > 0 {
				return item.RelationOrdinal
			}
			return int(^uint(0) >> 1)
		}
	}
	return int(^uint(0) >> 1)
}
