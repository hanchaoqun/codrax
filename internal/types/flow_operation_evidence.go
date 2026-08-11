package types

import "strings"

// FlowOperationEvidenceEmissionGuide is the single semantic teaching source
// shared by the explorer's completion repair and tests. The emit_evidence JSON
// schema remains the authority for field names and enum values.
const FlowOperationEvidenceEmissionGuide = "Read the operation sites that actually move the requested value/state/control: producer, transfer or merge boundary, and consumer. Treat analyzer-provided named participants as a soft exploration checklist: when relevant, inspect an operation incident to each participant; if no such site is proved, keep that participant independent and explicitly unproven rather than inventing a bridge. For a state carrier, container, or context object, inspect the exact writer and reader operation sites, such as setter/getter or member calls, direct assignments, and returns; its field/type declaration does not prove that data entered or left it. Emit one citable row per verified directed operation with exact syntax-authored subject/object and the matching anchor_kind (call, callback, assignment, initializer, return, or precedence): a call/callback uses the exact enclosing callable and invoked callee, an assignment/initializer uses the exact LHS receiver and RHS value/source, and a return uses the exact returning callable and returned value/source. A declaration-only field/member line is a definition, never an initializer; initializer requires an exact value-bearing member binding such as a struct/object/config literal entry. Do not rename an endpoint to a semantic role or result label such as a carrier field, component, or final answer; participant names guide navigation only and never become edge authority. The final diagram may choose typed relation_kind=data_flow to render a proved assignment tuple in execution direction RHS -> LHS. Definitions, type/field rosters, and nearby helper paths remain context only. If the inspected source cannot prove a transfer, preserve that as an unproven boundary instead of inventing an edge."

// FlowOperationEvidence returns citable current-source directed operations.
// It is intentionally relevance-neutral: the hard completion safeguard may
// test whether *any* operation-level carrier exists, but must not hard-gate on
// noisy entity/ranker overlap. Current-question relevance remains a separate
// typed support/ranking concern downstream.
func FlowOperationEvidence(evidence []EvidenceItem) []EvidenceItem {
	out := make([]EvidenceItem, 0)
	for _, item := range evidence {
		if !item.IsCitable() ||
			RuntimeArtifactPathKind(item.Source) != "" ||
			strings.TrimSpace(item.Source) == "" ||
			strings.TrimSpace(item.Subject) == "" ||
			strings.TrimSpace(item.Object) == "" ||
			!PredicateAxisHasMatchingAnchor(AxisFlow, item) {
			continue
		}
		if (item.AnchorKind == AnchorAssignment || item.AnchorKind == AnchorInitializer) &&
			!AssignmentEvidenceEndpointsMatch(item) {
			// An assignment-shaped line without exact LHS/RHS ownership may
			// remain ordinary evidence, but cannot close a directed flow lane.
			continue
		}
		switch item.Origin {
		case ClaimOriginLog, ClaimOriginPerf:
			continue
		}
		out = append(out, item)
	}
	return out
}

func HasFlowOperationEvidence(evidence []EvidenceItem) bool {
	return len(FlowOperationEvidence(evidence)) > 0
}

// FlowOperationEvidenceForRequest additionally applies the analyzer's typed
// principal source scope. A test/example operation may be useful context for a
// production question, but cannot satisfy its operation-carrier safeguard.
func FlowOperationEvidenceForRequest(evidence []EvidenceItem, rm RequestModel) []EvidenceItem {
	items := FlowOperationEvidence(evidence)
	scope := SourceScopeProduction
	if rm.SourceScopeProfile != nil {
		if rm.SourceScopeProfile.RequestedScope != "" {
			scope = rm.SourceScopeProfile.RequestedScope
		}
		if rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal {
			scope = SourceScopeAll
		}
	}
	out := items[:0]
	for _, item := range items {
		if SourceScopeAllowsPathRole(scope, ClassifySourcePathRole(item.Source)) {
			out = append(out, item)
		}
	}
	return out
}

func HasFlowOperationEvidenceForRequest(evidence []EvidenceItem, rm RequestModel) bool {
	return len(FlowOperationEvidenceForRequest(evidence, rm)) > 0
}

// ExplorerAuthoredFlowOperationEvidenceForRequest returns the operation rows
// the Explorer model explicitly selected for this investigation. Parser and
// repo-wide dataflow expansions remain valuable background evidence, but they
// must not independently expand principal ordered-path authority merely by
// entering a broad support plan.
func ExplorerAuthoredFlowOperationEvidenceForRequest(evidence []EvidenceItem, rm RequestModel) []EvidenceItem {
	items := FlowOperationEvidenceForRequest(evidence, rm)
	out := items[:0]
	for _, item := range items {
		if strings.TrimSpace(item.Producer) == EvidenceProducerExplorerEmitEvidence {
			out = append(out, item)
		}
	}
	return out
}

// AnswerCodeIdentityIncidentViaDeclaredBinding aligns a user-facing static
// type participant with an exact operation endpoint through a parser-stamped
// declaration row. It is an identity/coverage bridge only: operation remains
// the sole authority for relation kind, direction, endpoints, and source line.
//
// The join is deliberately strict. The declaration and operation must be in
// the same source file; the endpoint must contain the exact declared binding;
// and a member declaration must belong to the same typed callable owner as the
// operation. Dynamic/untyped/ambiguous declarations publish no DeclaredType
// and therefore fail closed.
func AnswerCodeIdentityIncidentViaDeclaredBinding(participant, endpoint string, operation EvidenceItem, evidence []EvidenceItem) bool {
	participant = strings.TrimSpace(participant)
	endpointSegments := answerCodeIdentitySegments(endpoint)
	if participant == "" || len(endpointSegments) == 0 || !operation.IsCitable() {
		return false
	}
	operationSource := strings.TrimSpace(strings.ReplaceAll(operation.Source, `\`, "/"))
	operationOwner := strings.TrimSpace(operation.OwnerIdentity)
	if operationOwner == "" {
		operationOwner = strings.TrimSpace(operation.OwnerSymbol)
	}
	for _, declaration := range evidence {
		if !declaration.IsCitable() || declaration.AnchorKind != AnchorDefinition ||
			strings.TrimSpace(declaration.DeclaredBinding) == "" ||
			strings.TrimSpace(declaration.DeclaredType) == "" {
			continue
		}
		declarationSource := strings.TrimSpace(strings.ReplaceAll(declaration.Source, `\`, "/"))
		if declarationSource == "" || declarationSource != operationSource ||
			RuntimeArtifactPathKind(declaration.Source) != "" {
			continue
		}
		if !AnswerCodeIdentitySurfacesCompatible(participant, declaration.DeclaredType) {
			continue
		}
		bindingSegments := answerCodeIdentitySegments(declaration.DeclaredBinding)
		if len(bindingSegments) == 0 || !answerCodeIdentityContainsSegment(endpointSegments, bindingSegments[len(bindingSegments)-1]) {
			continue
		}
		declaredOwner := strings.TrimSpace(declaration.DeclaredOwner)
		if declaredOwner != "" && (operationOwner == "" ||
			(!AnswerCodeIdentityOwnsEndpoint(declaredOwner, operationOwner) &&
				!AnswerCodeIdentitySurfacesCompatible(declaredOwner, operationOwner))) {
			continue
		}
		return true
	}
	return false
}

func answerCodeIdentityContainsSegment(segments []string, want string) bool {
	for _, segment := range segments {
		if segment == want {
			return true
		}
	}
	return false
}
