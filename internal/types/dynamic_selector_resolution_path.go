package types

import (
	"fmt"
	"sort"
	"strings"
)

const DynamicSelectorResolutionPathVersion = 1

type EvidenceSelectorApplication struct {
	Owner   string `json:"owner"`
	Literal string `json:"literal"`
}

type DynamicSelectorResolutionStatus string

const DynamicSelectorResolutionCandidateOnly DynamicSelectorResolutionStatus = "candidate_only"

type DynamicSelectorResolutionHopRole string

const (
	DynamicSelectorHopEntryCall           DynamicSelectorResolutionHopRole = "entry_call"
	DynamicSelectorHopSelectorArgument    DynamicSelectorResolutionHopRole = "selector_argument_flow"
	DynamicSelectorHopSelectorApplication DynamicSelectorResolutionHopRole = "selector_application"
	DynamicSelectorHopRegistration        DynamicSelectorResolutionHopRole = "registration"
	DynamicSelectorHopLookupAssignment    DynamicSelectorResolutionHopRole = "lookup_assignment"
	DynamicSelectorHopFactoryReturn       DynamicSelectorResolutionHopRole = "factory_return"
	DynamicSelectorHopCallbackHandoff     DynamicSelectorResolutionHopRole = "callback_handoff"
	DynamicSelectorHopTypeRelation        DynamicSelectorResolutionHopRole = "type_relation"
)

type DynamicSelectorResolutionHop struct {
	Role         DynamicSelectorResolutionHopRole `json:"role"`
	RelationKind DiagramRelationKind              `json:"relation_kind,omitempty"`
	ClaimForm    ClaimForm                        `json:"claim_form"`
	Predicate    string                           `json:"predicate,omitempty"`
	FromIdentity string                           `json:"from_identity"`
	ToIdentity   string                           `json:"to_identity"`
	EvidenceID   string                           `json:"evidence_id"`
}

type DynamicSelectorResolutionPath struct {
	Version           int                             `json:"version"`
	Status            DynamicSelectorResolutionStatus `json:"status"`
	EntryIdentity     string                          `json:"entry_identity"`
	SelectorArgument  string                          `json:"selector_argument"`
	SelectorOwner     string                          `json:"selector_owner"`
	SelectorLiteral   string                          `json:"selector_literal"`
	ContainerIdentity string                          `json:"container_identity"`
	LookupIdentity    string                          `json:"lookup_identity"`
	CandidateIdentity string                          `json:"candidate_identity"`
	Hops              []DynamicSelectorResolutionHop  `json:"hops"`
	CallbackHops      []DynamicSelectorResolutionHop  `json:"callback_hops,omitempty"`
	TypeRoster        []DynamicSelectorResolutionHop  `json:"type_roster,omitempty"`
}

type DynamicSelectorResolutionRejectionReason string

const (
	DynamicSelectorRejectInvalidApplication  DynamicSelectorResolutionRejectionReason = "invalid_selector_application"
	DynamicSelectorRejectAmbiguousCandidate  DynamicSelectorResolutionRejectionReason = "ambiguous_candidate"
	DynamicSelectorRejectBindingUnavailable  DynamicSelectorResolutionRejectionReason = "binding_unavailable"
	DynamicSelectorRejectAmbiguousContainer  DynamicSelectorResolutionRejectionReason = "ambiguous_container"
	DynamicSelectorRejectLookupUnavailable   DynamicSelectorResolutionRejectionReason = "lookup_unavailable"
	DynamicSelectorRejectAmbiguousLookup     DynamicSelectorResolutionRejectionReason = "ambiguous_lookup"
	DynamicSelectorRejectReturnUnavailable   DynamicSelectorResolutionRejectionReason = "return_unavailable"
	DynamicSelectorRejectAmbiguousReturn     DynamicSelectorResolutionRejectionReason = "ambiguous_return"
	DynamicSelectorRejectEntryUnavailable    DynamicSelectorResolutionRejectionReason = "entry_unavailable"
	DynamicSelectorRejectAmbiguousEntry      DynamicSelectorResolutionRejectionReason = "ambiguous_entry"
	DynamicSelectorRejectArgumentUnavailable DynamicSelectorResolutionRejectionReason = "selector_argument_unavailable"
	DynamicSelectorRejectAmbiguousArgument   DynamicSelectorResolutionRejectionReason = "ambiguous_selector_argument"
)

type DynamicSelectorResolutionRejection struct {
	SelectorLiteral string                                   `json:"selector_literal,omitempty"`
	SelectorOwner   string                                   `json:"selector_owner,omitempty"`
	Reason          DynamicSelectorResolutionRejectionReason `json:"reason"`
	EvidenceIDs     []string                                 `json:"evidence_ids,omitempty"`
}

type DynamicSelectorResolutionCompilation struct {
	Version    int                                  `json:"version"`
	Candidates []DynamicSelectorResolutionPath      `json:"candidates,omitempty"`
	Rejected   []DynamicSelectorResolutionRejection `json:"rejected,omitempty"`
}

type dynamicSelectorApplicationCandidate struct {
	item      EvidenceItem
	ownerKey  string
	selector  string
	candidate string
}

type dynamicSelectorBindingCandidate struct {
	item         EvidenceItem
	container    string
	containerKey string
	value        string
}

type dynamicSelectorLookupCandidate struct {
	item      EvidenceItem
	owner     string
	receiver  string
	container string
}

type dynamicSelectorReturnCandidate struct {
	item       EvidenceItem
	expression string
}

type dynamicSelectorEntryCandidate struct {
	item EvidenceItem
	from string
}

type dynamicSelectorArgumentCandidate struct {
	item     EvidenceItem
	argument string
}

// CompileDynamicSelectorResolutionPaths joins only citable typed evidence.
// It does not parse source snippets, summaries, request prose, model output,
// diagram text, language names, or file extensions. Every emitted hop keeps
// the original relation kind and Evidence ID; the result is candidate-only and
// never creates a synthetic direct-call edge.
func CompileDynamicSelectorResolutionPaths(evidence []EvidenceItem, entryIdentity string) DynamicSelectorResolutionCompilation {
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
		path, reason, ids := compileOneDynamicSelectorResolutionPath(evidence, app.item, entryIdentity)
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

func compileOneDynamicSelectorResolutionPath(evidence []EvidenceItem, app EvidenceItem, requestedEntry string) (DynamicSelectorResolutionPath, DynamicSelectorResolutionRejectionReason, []string) {
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
	path.CallbackHops = compileDynamicSelectorCallbackHops(evidence, entry.from)
	path.TypeRoster = compileDynamicSelectorTypeRoster(evidence, path.CandidateIdentity)
	return path, "", nil
}

func dynamicSelectorHop(role DynamicSelectorResolutionHopRole, relation DiagramRelationKind, item EvidenceItem, from, to string) DynamicSelectorResolutionHop {
	return DynamicSelectorResolutionHop{
		Role: role, RelationKind: relation, ClaimForm: ClaimFormOf(item), Predicate: strings.TrimSpace(item.Predicate),
		FromIdentity: strings.TrimSpace(from), ToIdentity: strings.TrimSpace(to), EvidenceID: dynamicSelectorEvidenceID(item),
	}
}

func compileDynamicSelectorCallbackHops(evidence []EvidenceItem, entry string) []DynamicSelectorResolutionHop {
	var calls []EvidenceItem
	for _, item := range evidence {
		if item.IsCitable() && ClaimFormOf(item) == ClaimCallEdge &&
			dynamicSelectorIdentityEquivalent(firstNonEmptyDynamicSelectorIdentity(item.OwnerSymbol, item.Subject), entry) {
			calls = append(calls, item)
		}
	}
	var out []DynamicSelectorResolutionHop
	seen := make(map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimCallbackHandoff {
			continue
		}
		for _, call := range calls {
			if !dynamicSelectorIdentityEquivalent(call.Object, item.Subject) {
				continue
			}
			id := dynamicSelectorEvidenceID(item)
			if !seen[id] {
				seen[id] = true
				out = append(out, dynamicSelectorHop(DynamicSelectorHopCallbackHandoff, DiagramRelCallback, item, item.Subject, item.Object))
			}
		}
	}
	return out
}

func compileDynamicSelectorTypeRoster(evidence []EvidenceItem, candidate string) []DynamicSelectorResolutionHop {
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
		return dynamicSelectorRelationOrdinal(evidence, out[i].EvidenceID) < dynamicSelectorRelationOrdinal(evidence, out[j].EvidenceID)
	})
	return out
}

func dynamicSelectorRelationOrdinal(evidence []EvidenceItem, id string) int {
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

func dynamicSelectorContainerIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, '['); idx > 0 && strings.HasSuffix(raw, "]") {
		raw = strings.TrimSpace(raw[:idx])
	}
	return dynamicSelectorValueIdentity(raw)
}

func dynamicSelectorBindingRelationKind(item EvidenceItem) DiagramRelationKind {
	if ClaimFormOf(item) == ClaimAssignmentFact {
		return DiagramRelAssignment
	}
	return DiagramRelRegister
}

func dynamicSelectorValueIdentity(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), "`'\"")
	raw = strings.TrimLeft(raw, "*&")
	if AnswerCodeIdentitySurfaceKey(raw) == "" {
		return ""
	}
	return raw
}

func dynamicSelectorInvocationCallee(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, prefix := range []string{"await ", "try ", "try? ", "try! ", "new "} {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	}
	idx := strings.IndexByte(raw, '(')
	if idx <= 0 || !strings.HasSuffix(raw, ")") {
		return ""
	}
	return dynamicSelectorValueIdentity(raw[:idx])
}

func dynamicSelectorIdentityEquivalent(left, right string) bool {
	return AnswerCodeIdentitySurfacesEquivalent(strings.TrimSpace(left), strings.TrimSpace(right))
}

func dynamicSelectorEvidenceID(item EvidenceItem) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return StableEvidenceID(item)
}

func dynamicSelectorEvidenceOccurrenceKey(item EvidenceItem) string {
	return strings.Join([]string{
		strings.TrimSpace(item.Source),
		fmt.Sprintf("%d:%d", item.LineStart, item.LineEnd),
		string(ClaimFormOf(item)),
		strings.TrimSpace(item.Predicate),
		AnswerCodeIdentitySurfaceKey(item.OwnerSymbol),
		AnswerCodeIdentitySurfaceKey(item.Subject),
		AnswerCodeIdentitySurfaceKey(item.Object),
	}, "\x00")
}

func firstNonEmptyDynamicSelectorIdentity(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// The helpers below deduplicate byte-identical evidence while preserving
// semantic multiplicity. Distinct candidate/container/lookup/return/entry
// shapes remain visible to the ambiguity checks and therefore fail closed.
func dynamicSelectorUniqueApplications(in []dynamicSelectorApplicationCandidate) []dynamicSelectorApplicationCandidate {
	out := make([]dynamicSelectorApplicationCandidate, 0, len(in))
	seen := make(map[string]bool)
	for _, candidate := range in {
		key := candidate.ownerKey + "\x00" + candidate.selector + "\x00" +
			AnswerCodeIdentitySurfaceKey(candidate.candidate) + "\x00" + dynamicSelectorEvidenceOccurrenceKey(candidate.item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorApplicationCandidates(in []dynamicSelectorApplicationCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		if key := AnswerCodeIdentitySurfaceKey(item.candidate) + "\x00" + dynamicSelectorEvidenceOccurrenceKey(item.item); key != "\x00" {
			seen[key] = true
		}
	}
	return len(seen)
}

func dynamicSelectorApplicationEvidenceIDs(in []dynamicSelectorApplicationCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func uniqueDynamicSelectorBindings(in []dynamicSelectorBindingCandidate) []dynamicSelectorBindingCandidate {
	out := make([]dynamicSelectorBindingCandidate, 0, len(in))
	seen := make(map[string]int)
	for _, candidate := range in {
		key := dynamicSelectorBindingShapeKey(candidate)
		if idx, ok := seen[key]; ok {
			// The same exact source assignment may be present twice: once as
			// the parser-authored assignment row and once as a grounded model
			// registration row. They agree on the endpoint tuple and source
			// occurrence, so they are corroborating carriers, not two possible
			// bindings. Prefer the stronger registration relation without
			// relabeling an assignment when no such row exists.
			if ClaimFormOf(out[idx].item) == ClaimAssignmentFact && ClaimFormOf(candidate.item) == ClaimRegistrationEdge {
				out[idx] = candidate
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorBindingShapes(in []dynamicSelectorBindingCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		seen[dynamicSelectorBindingShapeKey(item)] = true
	}
	return len(seen)
}

func dynamicSelectorBindingShapeKey(candidate dynamicSelectorBindingCandidate) string {
	end := candidate.item.LineEnd
	if end <= 0 {
		end = candidate.item.LineStart
	}
	return strings.Join([]string{
		candidate.containerKey,
		AnswerCodeIdentitySurfaceKey(candidate.value),
		strings.TrimSpace(candidate.item.Source),
		fmt.Sprintf("%d:%d", candidate.item.LineStart, end),
		AnswerCodeIdentitySurfaceKey(candidate.item.OwnerSymbol),
	}, "\x00")
}

func dynamicSelectorBindingEvidenceIDs(in []dynamicSelectorBindingCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func uniqueDynamicSelectorLookups(in []dynamicSelectorLookupCandidate) []dynamicSelectorLookupCandidate {
	out := make([]dynamicSelectorLookupCandidate, 0, len(in))
	seen := make(map[string]bool)
	for _, candidate := range in {
		key := AnswerCodeIdentitySurfaceKey(candidate.owner) + "\x00" +
			AnswerCodeIdentitySurfaceKey(candidate.receiver) + "\x00" +
			AnswerCodeIdentitySurfaceKey(candidate.container) + "\x00" + dynamicSelectorEvidenceOccurrenceKey(candidate.item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorLookupShapes(in []dynamicSelectorLookupCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		seen[AnswerCodeIdentitySurfaceKey(item.owner)+"\x00"+
			AnswerCodeIdentitySurfaceKey(item.receiver)+"\x00"+
			AnswerCodeIdentitySurfaceKey(item.container)+"\x00"+dynamicSelectorEvidenceOccurrenceKey(item.item)] = true
	}
	return len(seen)
}

func dynamicSelectorLookupEvidenceIDs(in []dynamicSelectorLookupCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func uniqueDynamicSelectorReturns(in []dynamicSelectorReturnCandidate) []dynamicSelectorReturnCandidate {
	out := make([]dynamicSelectorReturnCandidate, 0, len(in))
	seen := make(map[string]bool)
	for _, candidate := range in {
		key := AnswerCodeIdentitySurfaceKey(firstNonEmptyDynamicSelectorIdentity(candidate.item.OwnerSymbol, candidate.item.Subject)) +
			"\x00" + candidate.expression + "\x00" + dynamicSelectorEvidenceOccurrenceKey(candidate.item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorReturnShapes(in []dynamicSelectorReturnCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		seen[AnswerCodeIdentitySurfaceKey(firstNonEmptyDynamicSelectorIdentity(item.item.OwnerSymbol, item.item.Subject))+"\x00"+item.expression+"\x00"+dynamicSelectorEvidenceOccurrenceKey(item.item)] = true
	}
	return len(seen)
}

func dynamicSelectorReturnEvidenceIDs(in []dynamicSelectorReturnCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func uniqueDynamicSelectorEntries(in []dynamicSelectorEntryCandidate) []dynamicSelectorEntryCandidate {
	out := make([]dynamicSelectorEntryCandidate, 0, len(in))
	seen := make(map[string]bool)
	for _, candidate := range in {
		key := AnswerCodeIdentitySurfaceKey(candidate.from) + "\x00" + dynamicSelectorEvidenceOccurrenceKey(candidate.item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorEntryShapes(in []dynamicSelectorEntryCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		seen[AnswerCodeIdentitySurfaceKey(item.from)+"\x00"+dynamicSelectorEvidenceOccurrenceKey(item.item)] = true
	}
	return len(seen)
}

func dynamicSelectorEntryEvidenceIDs(in []dynamicSelectorEntryCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func uniqueDynamicSelectorArguments(in []dynamicSelectorArgumentCandidate) []dynamicSelectorArgumentCandidate {
	out := make([]dynamicSelectorArgumentCandidate, 0, len(in))
	seen := make(map[string]bool)
	for _, candidate := range in {
		key := AnswerCodeIdentitySurfaceKey(candidate.argument) + "\x00" + dynamicSelectorEvidenceOccurrenceKey(candidate.item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func distinctDynamicSelectorArgumentShapes(in []dynamicSelectorArgumentCandidate) int {
	seen := make(map[string]bool)
	for _, item := range in {
		seen[AnswerCodeIdentitySurfaceKey(item.argument)+"\x00"+dynamicSelectorEvidenceOccurrenceKey(item.item)] = true
	}
	return len(seen)
}

func dynamicSelectorArgumentEvidenceIDs(in []dynamicSelectorArgumentCandidate) []string {
	ids := make([]string, 0, len(in))
	for _, item := range in {
		ids = appendUniqueDynamicSelectorID(ids, dynamicSelectorEvidenceID(item.item))
	}
	return ids
}

func appendUniqueDynamicSelectorID(ids []string, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}
