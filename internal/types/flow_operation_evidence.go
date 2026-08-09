package types

import "strings"

// FlowOperationEvidenceEmissionGuide is the single semantic teaching source
// shared by the explorer's completion repair and tests. The emit_evidence JSON
// schema remains the authority for field names and enum values.
const FlowOperationEvidenceEmissionGuide = "Read the operation sites that actually move the requested value/state/control: producer, transfer or merge boundary, and consumer. Treat analyzer-provided named participants as a soft exploration checklist: when relevant, inspect an operation incident to each participant; if no such site is proved, keep that participant independent and explicitly unproven rather than inventing a bridge. Emit one citable row per verified directed operation with exact subject/object and the matching anchor_kind (call, callback, assignment, initializer, return, or precedence). Definitions, type/field rosters, and nearby helper paths remain context only. If the inspected source cannot prove a transfer, preserve that as an unproven boundary instead of inventing an edge."

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

// MissingExplorerFlowIncidentParticipants returns schema-validated flow
// participants whose requested incident relation is still absent from the
// Explorer-authored operation handoff. Participant hints are planning
// obligations, never evidence: this helper only checks them against citable
// typed relations and never manufactures an edge from the hint itself.
//
// An empty result is intentionally ambiguous between "all covered" and "no
// typed participant obligations"; callers that need to distinguish those
// states can inspect DiagramHint.Participants. Context-only participants are
// excluded because their connection was not requested. Identity comparison
// uses the shared cross-language identity segmentation plus a unique
// short-to-qualified bridge rather than label text, request prose, or fuzzy
// similarity.
func MissingExplorerFlowIncidentParticipants(evidence []EvidenceItem, rm RequestModel) []string {
	if rm.DiagramHint == nil || len(rm.DiagramHint.Participants) == 0 {
		return nil
	}
	operations := ExplorerAuthoredFlowOperationEvidenceForRequest(evidence, rm)
	seen := make(map[string]bool, len(rm.DiagramHint.Participants))
	missing := make([]string, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		identity := strings.TrimSpace(participant.Identity)
		key := strings.ToLower(identity)
		if identity == "" || seen[key] || participant.Role != DiagramParticipantIncidentRequired {
			continue
		}
		seen[key] = true
		covered := explorerFlowIncidentParticipantCovered(identity, operations)
		if !covered {
			missing = append(missing, identity)
		}
	}
	return missing
}

func explorerFlowIncidentParticipantCovered(identity string, operations []EvidenceItem) bool {
	participantSegments := answerCodeIdentitySegments(identity)
	if len(participantSegments) == 0 {
		return false
	}
	candidates := map[string]struct{}{}
	for _, operation := range operations {
		for _, endpoint := range []string{operation.Subject, operation.Object} {
			endpointSegments := answerCodeIdentitySegments(endpoint)
			if len(endpointSegments) == 0 {
				continue
			}
			if answerCodeIdentitySegmentSlicesEqual(participantSegments, endpointSegments) {
				candidates[strings.Join(endpointSegments, ".")] = struct{}{}
				continue
			}
			// A source relation may qualify a short request identity when that
			// qualification is unique in the selected operation set. The reverse
			// is unsafe: an unqualified relation endpoint cannot prove the owner
			// of an explicitly qualified participant.
			if len(participantSegments) == 1 && len(endpointSegments) > 1 &&
				AnswerCodeIdentitySurfacesCompatible(identity, endpoint) {
				candidates[strings.Join(endpointSegments, ".")] = struct{}{}
			}
		}
	}
	return len(candidates) == 1
}
