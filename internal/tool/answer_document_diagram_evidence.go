package tool

import (
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// DiagramCallEdgeEvidenceMismatch identifies one structured source relation
// whose direction is not backed by the matching citable typed EvidenceItem.
// The historical name is retained for the public validator call site; issues
// distinguish invocation from registration/binding failures.
//
// This authority deliberately consumes only typed answer/evidence fields and
// Mermaid syntax. It never scans the raw request, model prose, edge-message
// vocabulary, or rendered final text for hard-gate keywords.
type DiagramCallEdgeEvidenceMismatch struct {
	BlockID        string
	Issue          string
	FromNode       string
	ToNode         string
	FromSymbol     string
	ToSymbol       string
	FromNodeSymbol string
	ToNodeSymbol   string
	Relation       types.DiagramRelationKind
	BodyOccurrence int
}

// IsRequestedStagePrecedenceSpineIncomplete exposes the typed completeness
// diagnosis to the post-finalizer validator without exporting the internal
// issue vocabulary as another mutable string contract.
func (m DiagramCallEdgeEvidenceMismatch) IsRequestedStagePrecedenceSpineIncomplete() bool {
	return m.Issue == diagramRequestedStageSpineIncomplete
}

const (
	diagramCallEdgeIssueDuplicateParticipant  = "duplicate_participant_identity"
	diagramCallEdgeIssueMissingAnchor         = "missing_call_anchor"
	diagramCallEdgeIssueMissingGroundedAnchor = types.DiagramRelationFailureMissingGroundedCallAnchor
	diagramCallEdgeIssueMissingRelationAnchor = "missing_relation_anchor"
	diagramCallEdgeIssueAnchorWithoutBodyEdge = "typed_anchor_without_visible_edge"
	// diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge names the exact
	// sub-shape of a stale anchor where the body shows the same endpoint pair
	// once, in the opposite direction (colleague_merge_audit §40.57): the
	// model's typed claim and its own arrow disagree on direction. It is
	// reported instead of the generic stale-anchor issue so the retry teaches
	// alignment (swap the anchor endpoints or reverse the arrow) rather than
	// deletion; the system never realigns the anchor on the model's behalf.
	diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge = "typed_anchor_reversed_against_visible_edge"
	diagramStandaloneRelationIdentityMissing             = "standalone_relation_endpoint_identity_missing"
	diagramCallEdgeIssueNoEvidence                       = "call_edge_unproven"
	diagramCallEdgeIssueOccurrenceUnproven               = "call_edge_occurrence_unproven"
	diagramTypedRelationTupleEndpointReused              = "typed_relation_tuple_reused_across_visible_endpoints"
	diagramCallEdgeIssueReplyOperatorConflict            = "call_reply_operator_conflict"
	diagramSequenceRelationReplyConflict                 = "sequence_relation_reply_operator_conflict"
	diagramRegistrationEdgeIssueNoEvidence               = "registration_edge_unproven"
	diagramTypeRelationEdgeIssueNoEvidence               = "type_relation_edge_unproven"
	diagramAssignmentEdgeIssueNoEvidence                 = "assignment_edge_unproven"
	diagramDataFlowEdgeIssueNoEvidence                   = "data_flow_edge_unproven"
	diagramReturnEdgeIssueNoEvidence                     = "return_edge_unproven"
	diagramCallbackEdgeIssueNoEvidence                   = "callback_handoff_unproven"
	diagramArgumentFlowEdgeIssueNoEvidence               = "argument_flow_unproven"
	diagramTypedEndpointsCollapsedToSelfEdge             = "typed_endpoints_collapsed_to_self_edge"
	diagramEdgeAnchorNodeIdentityConflict                = "edge_anchor_node_identity_conflict"
	diagramSemanticRelationIssueNoEvidence               = "semantic_relation_edge_unproven"
	diagramRequestedStageSpineIncomplete                 = "requested_stage_precedence_spine_incomplete"
)

// DiagramCallEdgeEvidenceMismatches cross-checks model-authored typed call
// edges against the accepted evidence pool. An explicit relation_kind is an
// evidence claim regardless of the surrounding answer family: call and every
// logical guard/import/precedence/contain/observe relation always need their
// same-direction citable typed authority. QFCallChain additionally
// gets strict typed-relation ownership for every source-diagram body edge and
// visible-edge ownership. An optional diagram may be a faithful subset of
// the grounded relations carried by sibling prose/list blocks: omitting a
// true edge from a visual is not a false relation claim. Keeping completeness
// out of this hard gate also ensures that a system-authored copy-ready diagram
// capsule is accepted unchanged by the same validator.
//
// Runtime/root-cause trace diagrams, including explicit time-window causal
// projections and their automatic supplements, use a separate runtime
// relation authority and deliberately do not enter this source-code contract.
func DiagramCallEdgeEvidenceMismatches(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, evidence []types.EvidenceItem, stagePrecedenceOpt ...[]stageauthority.PrecedenceRelation) []DiagramCallEdgeEvidenceMismatch {
	var stagePrecedence []stageauthority.PrecedenceRelation
	if len(stagePrecedenceOpt) > 0 {
		stagePrecedence = stagePrecedenceOpt[0]
	}
	return diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, stagePrecedence, nil)
}

// diagramCallEdgeEvidenceMismatchesWithRequestModel keeps the exported,
// context-free validator strict while allowing the production pre/post emit
// paths to consume one exact request-scoped participant carrier permission.
// The permission is display-only; every relation below still passes through
// the unchanged typed evidence checks.
func diagramCallEdgeEvidenceMismatchesWithRequestModel(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	rm *types.RequestModel,
) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family == types.QFRootCauseTrace {
		return nil
	}
	var participantCarrierAuthorities []diagramParticipantEndpointCarrierAuthority
	if rm != nil {
		participantCarrierAuthorities = diagramParticipantEndpointCarrierAuthorities(*rm, evidence, stagePrecedence)
	}
	strictSourceCallChain := view.Family == types.QFCallChain
	// A schema-validated source relation axis, not diagram vocabulary, decides
	// whether a visible arrow is itself the requested relation claim. Deleting
	// edge_anchors therefore cannot turn the same factual topology into a
	// metadata-free presentation escape. Ordinary definition/architecture
	// diagrams retain the legacy presentation-only lane. Runtime trace was
	// excluded above and keeps its independent causal projection authority.
	strictSourceRelationBody := strictSourceCallChain || types.PredicateAxisRequiresDiagramEdgeOwnership(view.RelationAxis)
	// Identity ambiguity is diagnosed before edge authority. Otherwise the
	// same typed endpoint declared under multiple aliases can turn every valid
	// edge into a misleading missing-anchor report and send the model through
	// endpoint/direction rewrites. This check consumes parsed declarations and
	// exact typed call endpoints only; class/actor presentation labels that are
	// not exact call endpoints remain available to the message-operation lane.
	if duplicates := diagramDuplicateTypedParticipantIdentities(doc, evidence, strictSourceCallChain); len(duplicates) > 0 {
		return duplicates
	}
	requiredAnchors := view.RequiredMechanismAnchors
	documentLabels := diagramEvidenceDocumentNodeLabels(doc)
	documentEdges := diagramEvidenceDocumentEdges(doc)
	strictBodyEdgeKeys := diagramEvidenceStrictBodyEdgeKeys(doc, strictSourceRelationBody)
	bodyEdgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	var out []DiagramCallEdgeEvidenceMismatch
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		labels := documentLabels
		strictBodyCoverage := false
		if block.Kind == types.BlockDiagram && block.Diagram != nil {
			labels = diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
			// QFCallChain is the precise typed signal that any chosen diagram is
			// explaining source relations; DiagramCallDAG is independently a
			// precise source-call declaration in every non-runtime family. Every
			// body edge in either lane needs one schema-validated relation owner.
			// Runtime trace diagrams were excluded above and retain their own
			// authority.
			strictBodyCoverage = diagramRequiresTypedBodyOwnership(block.Diagram.Kind, strictSourceRelationBody)
		}
		parsedEdges := documentEdges
		if block.Diagram != nil {
			parsedEdges = mermaidcompat.ParseEdges(block.Diagram.Body)
		}
		effectiveAnchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, bodyEdgeBlockCounts)
		typedAnchorRelations := diagramTypedAnchorRelationSet(effectiveAnchors)
		// Mermaid's -->> sequence operator is a response/return presentation,
		// not a forward invocation, precedence, callback, binding, or another
		// forward semantic relation. A model-authored relation_kind cannot
		// redefine that visible syntax. Keep this operator/relation consistency
		// check independent from strict family coverage so an ordinary sequence
		// diagram cannot escape it merely by being classified QFGeneric. An
		// explicit return owner and an unanchored structural reverse reply remain
		// legal. Runtime trace diagrams returned above retain their separate
		// causal authority.
		if block.Diagram != nil && block.Diagram.Kind == types.DiagramSequence {
			operatorPairOccurrence := make(map[string]int)
			for _, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				occurrence := operatorPairOccurrence[key]
				operatorPairOccurrence[key] = occurrence + 1
				if mermaidcompat.SequenceArrowBase(edge.Operator) != "-->>" {
					continue
				}
				relation, conflict := diagramSequenceReplyOperatorRelationConflict(typedAnchorRelations[key])
				if !conflict {
					continue
				}
				fromSymbol, toSymbol := diagramEvidenceEdgeEndpointSymbolsAtOccurrence(
					edge.From, edge.To, occurrence, effectiveAnchors, labels, evidence,
				)
				issue := diagramSequenceRelationReplyConflict
				if relation == types.DiagramRelCall {
					// Preserve the established issue identity for downstream pins and
					// historical diagnostics while the closed matrix covers every
					// other forward relation through the generic issue.
					issue = diagramCallEdgeIssueReplyOperatorConflict
				}
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: issue,
					FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
					FromSymbol: fromSymbol, ToSymbol: toSymbol,
					Relation: relation, BodyOccurrence: occurrence + 1,
				})
			}
		}
		visibleBodyEdgeKeys := make(map[string]bool, len(parsedEdges))
		visibleBodyPairCounts := make(map[string]int, len(parsedEdges))
		if block.Kind == types.BlockDiagram && block.Diagram != nil {
			for _, edge := range parsedEdges {
				visibleBodyEdgeKeys[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
				visibleBodyPairCounts[diagramEvidenceUnorderedEdgeKey(edge.From, edge.To)]++
			}
			// A typed endpoint tuple describes one exact relation. It may own
			// repeated occurrences on the same visible actor pair (whose separate
			// occurrence budget is checked below), but it cannot be cloned onto
			// several different reader-visible endpoint pairs. Otherwise one
			// o.busCtx -> BuildAgentContext fact can be displayed as BusContext
			// feeding Analyzer, Explorer, Extractor, and Finalizer at once. This
			// exact schema-to-Mermaid cardinality check reads no request, message
			// label, reasoning, or answer prose.
			out = append(out, diagramTypedRelationTupleEndpointReuseMismatches(
				block.ID, effectiveAnchors, visibleBodyEdgeKeys,
			)...)
		}
		if strictBodyCoverage {
			// The same visible actor/component pair may legitimately carry
			// several operation-level calls. In that shape each distinct exact
			// identity pair needs its own visible occurrence. Otherwise a second
			// typed anchor could survive as a hidden graph after the model removed
			// the corresponding message. This is an exact structured cardinality
			// check; labels and message prose are deliberately irrelevant. An
			// anchor whose own pair is absent from the body is owned by the
			// anchor loop below (reversed / stale), never by this arm.
			for _, anchor := range diagramEvidenceExcessCallIdentityAnchors(parsedEdges, effectiveAnchors) {
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
					FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					FromSymbol: strings.TrimSpace(anchor.FromIdentity),
					ToSymbol:   strings.TrimSpace(anchor.ToIdentity),
					Relation:   types.DiagramRelCall,
				})
			}
			callAnchorKeys := diagramCallAnchorKeySet(effectiveAnchors)
			structuralReplies := diagramSequenceStructuralReplyIndexSet(block.Diagram.Kind, parsedEdges, typedAnchorRelations)
			callOccurrenceUse := make(map[string]int)
			bodyPairOccurrence := make(map[string]int)
			for edgeIndex, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				occurrence := bodyPairOccurrence[key]
				bodyPairOccurrence[key] = occurrence + 1
				fromSymbol, toSymbol := diagramEvidenceEdgeEndpointSymbolsAtOccurrence(
					edge.From, edge.To, occurrence, effectiveAnchors, labels, evidence,
				)
				hasTypedCallEvidence := diagramCallEdgeHasTypedEvidence(evidence, requiredAnchors, fromSymbol, toSymbol, edge.Label)
				// In a sequence diagram Mermaid's dashed -->> operator is a
				// response/return lane, not a second source-code invocation in
				// the reverse direction. It therefore needs no call anchor and
				// cannot be used to satisfy one. A call-DAG may also mix typed
				// control/dependency edges with invocation edges. An exact
				// non-call edge_anchor owns those edges; the separate relation
				// legality validator checks that typed relation. However, a
				// non-call anchor cannot hide a SAME-ENDPOINT call already proved
				// by typed evidence: in a call_dag that visible edge must retain
				// relation_kind=call (it may carry an additional guard anchor).
				// Unanchored sequence/call-DAG edges retain the fail-closed call
				// default; flow/architecture edges must at least declare their
				// canonical relation owner.
				if structuralReplies[edgeIndex] {
					continue
				}
				relations := typedAnchorRelations[key]
				if !diagramHasValidTypedRelation(relations) {
					issue := diagramCallEdgeIssueMissingRelationAnchor
					if block.Diagram.Kind == types.DiagramSequence || block.Diagram.Kind == types.DiagramCallDAG {
						issue = diagramCallEdgeIssueMissingAnchor
						if hasTypedCallEvidence {
							issue = diagramCallEdgeIssueMissingGroundedAnchor
						}
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: issue,
						FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						BodyOccurrence: occurrence + 1,
					})
					continue
				}
				requiresCallAuthority := diagramParsedEdgeRequiresCallAuthority(block.Diagram.Kind, relations, hasTypedCallEvidence)
				// A schema-valid non-call enum is an assertion, not evidence.
				// Require same-direction typed support for every logical relation
				// that can otherwise take a call-DAG edge out of call authority.
				// This closes the relabel escape (call -> precedence/guard/etc.)
				// without inspecting the edge label, request, or answer prose.
				for _, relation := range types.AllDiagramRelationKinds() {
					if !relations[relation] || !diagramStrictLogicalRelationNeedsEvidence(relation) {
						continue
					}
					// When the edge is an invocation surface but relation_kind=call
					// is absent, the missing-call diagnosis is the single executable
					// repair. Validate any additional logical relation after the
					// invocation retains its honest owner.
					if requiresCallAuthority && !callAnchorKeys[key] {
						continue
					}
					stageAuthority := relation == types.DiagramRelPrecedence &&
						diagramStagePrecedenceEdgeHasTypedAuthority(
							stagePrecedence, edge.From, edge.To, effectiveAnchors, labels,
						)
					genericAuthority := diagramLogicalRelationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, relation)
					// A generic source extractor may summarize one ordered stage
					// membership list as first -> last. Once the checkout-verified
					// read-stage provider is active, that lossy tuple must not expand
					// the three adjacent stage edges into a transitive message. This
					// branch reads only typed endpoint identities and the verified
					// provider; non-stage workflow precedence remains on the generic
					// evidence lane.
					if relation == types.DiagramRelPrecedence &&
						diagramVerifiedStageEndpointPair(stagePrecedence, fromSymbol, toSymbol) {
						genericAuthority = false
					}
					if genericAuthority || stageAuthority {
						continue
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramSemanticRelationIssueNoEvidence,
						FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						Relation: relation, BodyOccurrence: occurrence + 1,
					})
				}
				if !requiresCallAuthority {
					continue
				}
				if !callAnchorKeys[key] {
					issue := diagramCallEdgeIssueMissingAnchor
					if hasTypedCallEvidence {
						issue = diagramCallEdgeIssueMissingGroundedAnchor
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID:        block.ID,
						Issue:          issue,
						FromNode:       strings.TrimSpace(edge.From),
						ToNode:         strings.TrimSpace(edge.To),
						FromSymbol:     fromSymbol,
						ToSymbol:       toSymbol,
						BodyOccurrence: occurrence + 1,
					})
					continue
				}
				if hasTypedCallEvidence {
					// One static call-site row proves one concrete visible call
					// occurrence. It must not silently authorize the same arrow
					// four times with four different message payloads. Distinct
					// grounded call sites increase the budget; duplicated evidence
					// records do not. This is a structural Mermaid-occurrence vs
					// typed-evidence comparison and never reads the edge label,
					// model prose, or request text as authority.
					occurrenceKey, occurrenceBudget := diagramCallEdgeTypedEvidenceOccurrenceAuthority(
						evidence, requiredAnchors, fromSymbol, toSymbol, edge.Label,
					)
					callOccurrenceUse[occurrenceKey]++
					if callOccurrenceUse[occurrenceKey] > occurrenceBudget {
						out = append(out, DiagramCallEdgeEvidenceMismatch{
							BlockID: block.ID, Issue: diagramCallEdgeIssueOccurrenceUnproven,
							FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
							FromSymbol: fromSymbol, ToSymbol: toSymbol,
							Relation: types.DiagramRelCall, BodyOccurrence: occurrence + 1,
						})
					}
					continue
				}
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID:        block.ID,
					Issue:          diagramCallEdgeIssueNoEvidence,
					FromNode:       strings.TrimSpace(edge.From),
					ToNode:         strings.TrimSpace(edge.To),
					FromSymbol:     fromSymbol,
					ToSymbol:       toSymbol,
					BodyOccurrence: occurrence + 1,
				})
			}
		}
		for _, anchor := range block.EdgeAnchors {
			fromSymbol, toSymbol := diagramEvidenceAnchorEndpointSymbols(anchor, labels, evidence)
			if block.Kind != types.BlockDiagram && block.Diagram == nil &&
				answerBlockCarriesStandaloneTypedRelations(*block) && !anchor.HasEndpointIdentityPair() {
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: diagramStandaloneRelationIdentityMissing,
					FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					FromSymbol: strings.TrimSpace(anchor.FromIdentity),
					ToSymbol:   strings.TrimSpace(anchor.ToIdentity),
					Relation:   diagramAnchorRelation(anchor),
				})
				continue
			}
			// Diagram-local edge metadata describes the visible body, not a
			// hidden replacement graph. Keep optional visuals free to show any
			// faithful subset of the evidence, including a node-only subset with
			// no anchors, while rejecting the inverse drift where typed anchors
			// survive after every corresponding arrow was deleted. Sibling
			// structured carriers remain legal and are checked through the unique
			// body-edge ownership lane above.
			if block.Kind == types.BlockDiagram && block.Diagram != nil &&
				!visibleBodyEdgeKeys[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] {
				// The exact sub-shape "same endpoint pair, visible exactly once,
				// in the opposite direction" is the model contradicting its own
				// arrow (UML `Base <|.. Impl` with anchor Base -> Impl is the
				// production witness). Name it precisely so the retry teaches
				// alignment instead of deletion; ambiguous pairs (repeated or
				// absent reverse arrows) stay on the generic stale-anchor issue.
				// Relation-agnostic by class: any relation_kind, any diagram
				// family. The anchor itself is never rewritten (§40.57).
				issue := diagramCallEdgeIssueAnchorWithoutBodyEdge
				if diagramAnchorReversedAgainstUniqueVisibleEdge(anchor, visibleBodyEdgeKeys, visibleBodyPairCounts) {
					issue = diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge
				}
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: issue,
					FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					FromSymbol: fromSymbol,
					ToSymbol:   toSymbol,
					Relation:   diagramAnchorRelation(anchor),
				})
				continue
			}
			relation := diagramAnchorRelation(anchor)
			// A complete typed identity pair selects relation evidence; it must
			// not be attached to the opposite reader-visible Mermaid entities.
			// Resolve a node only when its parsed declaration contains one unique
			// citable code identity. Business/domain labels therefore remain
			// presentation-only, while an exact visible A paired with typed B is
			// a schema-level contradiction. A component/type may still host one
			// of its exact operation endpoints, preserving actor-oriented call and
			// sequence diagrams. This check reads neither edge messages, request
			// text, reasoning, nor rendered prose, and it never rewrites the graph.
			if block.Kind == types.BlockDiagram && block.Diagram != nil && anchor.HasEndpointIdentityPair() {
				conflict, fromNodeSymbol, toNodeSymbol := diagramEdgeAnchorNodeIdentityConflictDetails(
					anchor, labels, evidence, stagePrecedence, participantCarrierAuthorities,
				)
				if conflict {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramEdgeAnchorNodeIdentityConflict,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol:     strings.TrimSpace(anchor.FromIdentity),
						ToSymbol:       strings.TrimSpace(anchor.ToIdentity),
						FromNodeSymbol: fromNodeSymbol,
						ToNodeSymbol:   toNodeSymbol,
						Relation:       relation,
					})
					continue
				}
			}
			// One Mermaid participant may intentionally host several exact
			// invocation operations, so actor self-messages remain legal for
			// call/callback/return. A value, binding, type, or logical relation is
			// different: collapsing two distinct typed endpoints onto the same
			// visible alias turns a proved A -> B direction into a reader-visible
			// A -> A self-loop. Reject only that schema-carried contradiction.
			// Message text, display labels, request prose, and answer prose never
			// participate in this decision.
			if diagramTypedEndpointsCollapseToNonInvocationSelfEdge(anchor, relation) {
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: diagramTypedEndpointsCollapsedToSelfEdge,
					FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					FromSymbol: fromSymbol, ToSymbol: toSymbol,
					Relation: relation,
				})
				continue
			}
			anchorKey := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
			if relation == types.DiagramRelCallback {
				if !diagramCallbackEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramCallbackEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
					})
				}
				continue
			}
			if relation == types.DiagramRelArgumentFlow {
				if !diagramArgumentFlowEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramArgumentFlowEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						Relation: relation,
					})
				}
				continue
			}
			if relation == types.DiagramRelTypeRelation {
				if !diagramTypeRelationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramTypeRelationEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
					})
				}
				continue
			}
			if relation == types.DiagramRelRegister {
				if !diagramRegistrationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramRegistrationEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
					})
				}
				continue
			}
			if relation == types.DiagramRelAssignment || relation == types.DiagramRelDataFlow || relation == types.DiagramRelReturn {
				if !diagramValueFlowEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, relation) {
					issue := diagramAssignmentEdgeIssueNoEvidence
					if relation == types.DiagramRelDataFlow {
						issue = diagramDataFlowEdgeIssueNoEvidence
					} else if relation == types.DiagramRelReturn {
						issue = diagramReturnEdgeIssueNoEvidence
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: issue,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						Relation: relation,
					})
				}
				continue
			}
			if diagramStrictLogicalRelationNeedsEvidence(relation) {
				// Strict source diagrams validate the same typed anchor while
				// walking the visible body edge above. All other non-runtime
				// families still treat an explicit relation_kind as a factual
				// assertion: a schema-valid enum is not its own evidence.
				if strictBodyEdgeKeys[anchorKey] {
					continue
				}
				if !diagramLogicalRelationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, relation) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramSemanticRelationIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						Relation: relation,
					})
				}
				continue
			}
			if relation != types.DiagramRelCall {
				continue
			}
			if strictBodyEdgeKeys[anchorKey] {
				continue
			}
			if diagramCallAnchorHasTypedEvidence(evidence, requiredAnchors, fromSymbol, toSymbol, anchor, parsedEdges) {
				continue
			}
			out = append(out, DiagramCallEdgeEvidenceMismatch{
				BlockID:    block.ID,
				Issue:      diagramCallEdgeIssueNoEvidence,
				FromNode:   strings.TrimSpace(anchor.FromNode),
				ToNode:     strings.TrimSpace(anchor.ToNode),
				FromSymbol: fromSymbol,
				ToSymbol:   toSymbol,
				Relation:   relation,
			})
		}
	}
	return out
}

// diagramTypedRelationTupleEndpointReuseMismatches rejects only an exact typed
// identity tuple that the model mapped to more than one distinct visible
// endpoint pair in the same diagram. Repeated metadata for the same body pair
// stays in the existing occurrence/identity lanes. Anchors without both exact
// identities also stay in their existing evidence resolver, where display
// labels cannot mint this new authority.
func diagramTypedRelationTupleEndpointReuseMismatches(
	blockID string,
	anchors []types.DiagramEdgeAnchor,
	visibleBodyEdgeKeys map[string]bool,
) []DiagramCallEdgeEvidenceMismatch {
	type tupleUse struct {
		anchor  types.DiagramEdgeAnchor
		edgeKey string
	}
	usesByTuple := make(map[string][]tupleUse)
	var tupleOrder []string
	for _, anchor := range anchors {
		relation := diagramAnchorRelation(anchor)
		edgeKey := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
		if !relation.IsValid() || !anchor.HasEndpointIdentityPair() || !visibleBodyEdgeKeys[edgeKey] {
			continue
		}
		tupleKey := string(relation) + "\x00" + strings.TrimSpace(anchor.FromIdentity) + "\x00" + strings.TrimSpace(anchor.ToIdentity)
		if _, seen := usesByTuple[tupleKey]; !seen {
			tupleOrder = append(tupleOrder, tupleKey)
		}
		seenEdge := false
		for _, existing := range usesByTuple[tupleKey] {
			if existing.edgeKey == edgeKey {
				seenEdge = true
				break
			}
		}
		if !seenEdge {
			usesByTuple[tupleKey] = append(usesByTuple[tupleKey], tupleUse{anchor: anchor, edgeKey: edgeKey})
		}
	}
	var out []DiagramCallEdgeEvidenceMismatch
	for _, tupleKey := range tupleOrder {
		uses := usesByTuple[tupleKey]
		if len(uses) < 2 {
			continue
		}
		for _, use := range uses {
			anchor := use.anchor
			out = append(out, DiagramCallEdgeEvidenceMismatch{
				BlockID: blockID, Issue: diagramTypedRelationTupleEndpointReused,
				FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
				FromSymbol: strings.TrimSpace(anchor.FromIdentity), ToSymbol: strings.TrimSpace(anchor.ToIdentity),
				Relation: diagramAnchorRelation(anchor),
			})
		}
	}
	return out
}

func diagramTypedEndpointsCollapseToNonInvocationSelfEdge(anchor types.DiagramEdgeAnchor, relation types.DiagramRelationKind) bool {
	if !anchor.HasEndpointIdentityPair() ||
		!strings.EqualFold(strings.TrimSpace(anchor.FromNode), strings.TrimSpace(anchor.ToNode)) ||
		types.AnswerCodeIdentitySurfacesEquivalent(anchor.FromIdentity, anchor.ToIdentity) {
		return false
	}
	switch relation {
	case types.DiagramRelCall, types.DiagramRelCallback, types.DiagramRelReturn:
		return false
	default:
		return relation.IsValid()
	}
}

// diagramSequenceReplyOperatorRelationConflict is the closed semantic matrix
// for an explicitly owned sequence edge rendered with Mermaid's response
// operator. Relation ownership is schema-validated, so this helper never reads
// message labels, request text, model prose, or rendered answer text. Return is
// the only typed relation compatible with -->>; no owner is handled separately
// as a possible structural reverse reply.
func diagramSequenceReplyOperatorRelationConflict(relations map[types.DiagramRelationKind]bool) (types.DiagramRelationKind, bool) {
	for _, relation := range types.AllDiagramRelationKinds() {
		if relations[relation] && relation != types.DiagramRelReturn {
			return relation, true
		}
	}
	return types.DiagramRelUnknown, false
}

// diagramEvidenceEdgeEndpointSymbols resolves endpoint identity from the
// structured edge anchor before consulting visible Mermaid labels. The
// identity pair is only a selector into typed evidence; it cannot manufacture
// relation authority. Conflicting structured pairs fail closed instead of
// allowing display order to choose a fact.
func diagramEvidenceEdgeEndpointSymbols(
	fromNode, toNode string,
	anchors []types.DiagramEdgeAnchor,
	labels map[string]string,
	evidence []types.EvidenceItem,
) (string, string) {
	fromIdentity, toIdentity, present, conflict := diagramEvidenceEdgeIdentityPair(fromNode, toNode, anchors)
	if present {
		if conflict {
			return "", ""
		}
		return fromIdentity, toIdentity
	}
	return diagramEvidenceEndpointSymbol(fromNode, labels, evidence),
		diagramEvidenceEndpointSymbol(toNode, labels, evidence)
}

// diagramEvidenceEdgeEndpointSymbolsAtOccurrence preserves operation identity
// when a sequence diagram intentionally reuses one actor pair for multiple
// messages (including actor self-messages). In that Mermaid shape the visible
// participant ids describe carriers, while the ordered edge_anchors entries
// describe the exact operation endpoints. One unique identity pair retains the
// legacy reusable behavior; multiple distinct pairs bind to visible edge
// occurrences in first-appearance order. The labels remain display-only and
// cannot choose, reverse, or authorize a relation. Missing/extra identity pairs
// fail closed, and every selected pair is still checked against typed evidence
// by the caller.
func diagramEvidenceEdgeEndpointSymbolsAtOccurrence(
	fromNode, toNode string,
	occurrence int,
	anchors []types.DiagramEdgeAnchor,
	labels map[string]string,
	evidence []types.EvidenceItem,
) (string, string) {
	wantKey := diagramEvidenceEdgeKey(fromNode, toNode)
	type identityPair struct{ from, to string }
	var pairs []identityPair
	for i := range anchors {
		anchor := anchors[i]
		if diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode) != wantKey || !anchor.HasEndpointIdentityPair() {
			continue
		}
		candidate := identityPair{
			from: strings.TrimSpace(anchor.FromIdentity),
			to:   strings.TrimSpace(anchor.ToIdentity),
		}
		seen := false
		for _, existing := range pairs {
			if existing == candidate {
				seen = true
				break
			}
		}
		if !seen {
			pairs = append(pairs, candidate)
		}
	}
	switch len(pairs) {
	case 0:
		return diagramEvidenceEndpointSymbol(fromNode, labels, evidence),
			diagramEvidenceEndpointSymbol(toNode, labels, evidence)
	case 1:
		return pairs[0].from, pairs[0].to
	default:
		if occurrence < 0 || occurrence >= len(pairs) {
			return "", ""
		}
		return pairs[occurrence].from, pairs[occurrence].to
	}
}

// diagramEvidenceExcessCallIdentityAnchors returns distinct exact call
// identity pairs that exceed the visible occurrences of their own endpoint
// pair — the hidden second operation on one visible arrow. Anchor order and
// Mermaid occurrence order are both schema-carried order, so the result is
// deterministic even when one actor pair hosts many operations.
//
// Ownership split (colleague_merge_audit §40.57 合流复核收编): an anchor whose
// own endpoint pair is absent from the body is the anchor loop's row —
// typed_anchor_reversed_against_visible_edge for the exact reversed sub-shape,
// typed_anchor_without_visible_edge otherwise — so this arm never re-mints a
// second row (once under the generic stale issue) for the same anchor. Only
// block-local anchors can be absent here: a sibling-carrier anchor enters the
// effective set solely through a visible key.
func diagramEvidenceExcessCallIdentityAnchors(edges []mermaidcompat.Edge, anchors []types.DiagramEdgeAnchor) []types.DiagramEdgeAnchor {
	bodyCount := make(map[string]int)
	for _, edge := range edges {
		bodyCount[diagramEvidenceEdgeKey(edge.From, edge.To)]++
	}
	seenPairs := make(map[string]map[string]bool)
	var out []types.DiagramEdgeAnchor
	for _, anchor := range anchors {
		if diagramAnchorRelation(anchor) != types.DiagramRelCall || !anchor.HasEndpointIdentityPair() {
			continue
		}
		key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
		if bodyCount[key] == 0 {
			continue
		}
		pairKey := strings.TrimSpace(anchor.FromIdentity) + "\x00" + strings.TrimSpace(anchor.ToIdentity)
		if seenPairs[key] == nil {
			seenPairs[key] = make(map[string]bool)
		}
		if seenPairs[key][pairKey] {
			continue
		}
		seenPairs[key][pairKey] = true
		if len(seenPairs[key]) > bodyCount[key] {
			out = append(out, anchor)
		}
	}
	return out
}

func diagramEvidenceAnchorEndpointSymbols(anchor types.DiagramEdgeAnchor, labels map[string]string, evidence []types.EvidenceItem) (string, string) {
	if anchor.HasEndpointIdentityPair() {
		return strings.TrimSpace(anchor.FromIdentity), strings.TrimSpace(anchor.ToIdentity)
	}
	return diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence),
		diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
}

// diagramEdgeAnchorNodeIdentityConflictDetails rejects only a precise binding
// contradiction. An unresolved or business-only visible label cannot create a
// hard gate. A uniquely evidence-backed code identity must, however, denote
// the same endpoint selected by that side of the anchor (or an exact owner of
// that operation); otherwise correct typed relation metadata can be used to
// sign a visibly reversed graph.
func diagramEdgeAnchorNodeIdentityConflictDetails(
	anchor types.DiagramEdgeAnchor,
	labels map[string]string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	participantCarrierAuthorities []diagramParticipantEndpointCarrierAuthority,
) (bool, string, string) {
	if !anchor.HasEndpointIdentityPair() {
		return false, "", ""
	}
	fromIdentity, fromOK := diagramEvidenceExactNodeIdentity(anchor.FromNode, labels, evidence)
	toIdentity, toOK := diagramEvidenceExactNodeIdentity(anchor.ToNode, labels, evidence)
	// A read-stage row has four checkout-verified declaration/value aliases.
	// The generic code-identity comparator intentionally does not equate those
	// spellings globally. For an explicitly typed precedence edge only, accept
	// the visible and anchor pairs when both select the same one verified row.
	// This is pair-scoped and cannot authorize a call/data-flow relation, a
	// reversed edge, or any relation outside the selected read-stage provider.
	if diagramAnchorRelation(anchor) == types.DiagramRelPrecedence &&
		diagramStagePrecedenceAnchorBindsVisibleNodes(stagePrecedence, anchor, labels) {
		return false, fromIdentity, toIdentity
	}
	if fromOK && !diagramEvidenceNodeIdentityBindsEndpoint(fromIdentity, anchor.FromIdentity) &&
		!diagramParticipantEndpointCarrierAuthorizes(
			participantCarrierAuthorities, anchor, "from", anchor.FromNode, fromIdentity, labels,
		) {
		return true, fromIdentity, toIdentity
	}
	if toOK && !diagramEvidenceNodeIdentityBindsEndpoint(toIdentity, anchor.ToIdentity) &&
		!diagramParticipantEndpointCarrierAuthorizes(
			participantCarrierAuthorities, anchor, "to", anchor.ToNode, toIdentity, labels,
		) {
		return true, fromIdentity, toIdentity
	}
	return false, fromIdentity, toIdentity
}

func diagramStagePrecedenceAnchorBindsVisibleNodes(
	relations []stageauthority.PrecedenceRelation,
	anchor types.DiagramEdgeAnchor,
	labels map[string]string,
) bool {
	if len(relations) == 0 || !anchor.HasEndpointIdentityPair() {
		return false
	}
	fromStage, fromOK := diagramVisibleEndpointUniqueStage(relations, anchor.FromNode, labels)
	toStage, toOK := diagramVisibleEndpointUniqueStage(relations, anchor.ToNode, labels)
	if !fromOK || !toOK {
		return false
	}
	matches := 0
	for _, relation := range relations {
		if !diagramStageRowIdentityMatches(relation.From, fromStage) ||
			!diagramStageRowIdentityMatches(relation.To, toStage) ||
			!diagramStageRowIdentityMatches(relation.From, anchor.FromIdentity) ||
			!diagramStageRowIdentityMatches(relation.To, anchor.ToIdentity) {
			continue
		}
		matches++
	}
	return matches == 1
}

func diagramEvidenceExactNodeIdentity(node string, labels map[string]string, evidence []types.EvidenceItem) (string, bool) {
	node = strings.TrimSpace(node)
	if node == "" {
		return "", false
	}
	label := strings.TrimSpace(labels[strings.ToLower(node)])
	if label == "" {
		return "", false
	}
	candidates := diagramEvidenceLabelIdentityCandidates(label)
	matched := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if !diagramEvidenceIdentityAppearsExactly(evidence, candidate) {
			continue
		}
		duplicate := false
		for _, existing := range matched {
			if types.AnswerCodeIdentitySurfacesEquivalent(existing, candidate) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			matched = append(matched, candidate)
		}
	}
	if len(matched) != 1 {
		return "", false
	}
	return matched[0], true
}

func diagramEvidenceNodeIdentityBindsEndpoint(nodeIdentity, endpointIdentity string) bool {
	return types.AnswerCodeIdentitySurfacesEquivalent(nodeIdentity, endpointIdentity) ||
		types.AnswerCodeIdentitySurfacesCompatible(nodeIdentity, endpointIdentity) ||
		types.AnswerCodeIdentityOwnsEndpoint(nodeIdentity, endpointIdentity)
}

func diagramEvidenceEdgeIdentityPair(fromNode, toNode string, anchors []types.DiagramEdgeAnchor) (string, string, bool, bool) {
	wantKey := diagramEvidenceEdgeKey(fromNode, toNode)
	var fromIdentity, toIdentity string
	present := false
	for i := range anchors {
		anchor := &anchors[i]
		if diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode) != wantKey || !anchor.HasEndpointIdentityPair() {
			continue
		}
		candidateFrom := strings.TrimSpace(anchor.FromIdentity)
		candidateTo := strings.TrimSpace(anchor.ToIdentity)
		if !present {
			fromIdentity, toIdentity, present = candidateFrom, candidateTo, true
			continue
		}
		if fromIdentity != candidateFrom || toIdentity != candidateTo {
			return "", "", true, true
		}
	}
	return fromIdentity, toIdentity, present, false
}

// diagramStagePrecedenceHasTypedVisibleAuthority resolves a Mermaid endpoint
// label against the same checkout-verified stage rows when the generic source
// identity resolver cannot choose one label line. A display label may carry
// two declaration-backed spellings for the same stage, for example
// `extractor\nStageExtract`; treating those spellings as two unrelated source
// identities made the generic resolver fail closed and then caused the answer
// repair loop to delete a valid precedence edge.
//
// This bridge stays exact and fail-closed: it considers only complete identity
// lines parsed from the node declaration, every matching line must resolve to
// one unique stage row, and a label whose lines name different stages is
// rejected. Mermaid node IDs are considered only for a genuinely unlabeled
// node, so presentation aliases such as n1 never acquire authority by
// themselves.
func diagramStagePrecedenceEdgeHasTypedAuthority(
	relations []stageauthority.PrecedenceRelation,
	fromNode, toNode string,
	anchors []types.DiagramEdgeAnchor,
	labels map[string]string,
) bool {
	fromIdentity, toIdentity, present, conflict := diagramEvidenceEdgeIdentityPair(fromNode, toNode, anchors)
	if present {
		return !conflict && diagramStagePrecedenceHasTypedAuthority(relations, fromIdentity, toIdentity)
	}
	return diagramStagePrecedenceHasTypedVisibleAuthority(relations, fromNode, toNode, labels)
}

func diagramStagePrecedenceHasTypedVisibleAuthority(relations []stageauthority.PrecedenceRelation, fromNode, toNode string, labels map[string]string) bool {
	fromStage, fromOK := diagramVisibleEndpointUniqueStage(relations, fromNode, labels)
	toStage, toOK := diagramVisibleEndpointUniqueStage(relations, toNode, labels)
	if !fromOK || !toOK {
		return false
	}
	return diagramStagePrecedenceHasTypedAuthority(relations, fromStage, toStage)
}

func diagramStagePrecedenceHasTypedAuthority(relations []stageauthority.PrecedenceRelation, fromIdentity, toIdentity string) bool {
	matched := false
	for _, relation := range relations {
		if !diagramStageRowIdentityMatches(relation.From, fromIdentity) || !diagramStageRowIdentityMatches(relation.To, toIdentity) {
			continue
		}
		if matched {
			return false
		}
		matched = true
	}
	return matched
}

func diagramVisibleEndpointUniqueStage(relations []stageauthority.PrecedenceRelation, node string, labels map[string]string) (string, bool) {
	node = strings.TrimSpace(node)
	if node == "" {
		return "", false
	}
	var surfaces []string
	if label := strings.TrimSpace(labels[strings.ToLower(node)]); label != "" {
		surfaces = diagramEvidenceLabelIdentityCandidates(label)
	} else {
		surfaces = []string{node}
	}
	matched := make(map[string]bool)
	visit := func(row stageauthority.StageRow) {
		for _, surface := range surfaces {
			if diagramStageRowIdentityMatches(row, surface) {
				matched[row.StageIdent] = true
				return
			}
		}
	}
	for _, relation := range relations {
		visit(relation.From)
		visit(relation.To)
	}
	if len(matched) != 1 {
		return "", false
	}
	for stage := range matched {
		return stage, true
	}
	return "", false
}

func diagramStageRowIdentityMatches(row stageauthority.StageRow, surface string) bool {
	for _, alias := range row.IdentityAliases() {
		if types.AnswerCodeIdentitySurfacesEquivalent(alias, surface) ||
			types.AnswerCodeIdentitySurfacesCompatible(alias, surface) {
			return true
		}
	}
	return false
}

// diagramVerifiedStageEndpointPair reports whether both endpoint identities
// resolve to exactly one checkout-verified read-stage row. It does not assert
// that the pair is an adjacent edge; callers use the result to keep generic
// precedence evidence from widening the provider's closed adjacent-edge set.
func diagramVerifiedStageEndpointPair(relations []stageauthority.PrecedenceRelation, from, to string) bool {
	_, fromOK := diagramUniqueVerifiedStageRow(relations, from)
	_, toOK := diagramUniqueVerifiedStageRow(relations, to)
	return fromOK && toOK
}

func diagramUniqueVerifiedStageRow(relations []stageauthority.PrecedenceRelation, surface string) (stageauthority.StageRow, bool) {
	surface = strings.TrimSpace(surface)
	if surface == "" || len(relations) == 0 {
		return stageauthority.StageRow{}, false
	}
	matched := make(map[string]stageauthority.StageRow)
	visit := func(row stageauthority.StageRow) {
		if !diagramStageRowIdentityMatches(row, surface) {
			return
		}
		key := strings.ToLower(strings.Join([]string{
			strings.TrimSpace(row.StageIdent), strings.TrimSpace(row.StageValue),
			strings.TrimSpace(row.AgentIdent), strings.TrimSpace(row.AgentValue),
		}, "\x00"))
		matched[key] = row
	}
	for _, relation := range relations {
		visit(relation.From)
		visit(relation.To)
	}
	if len(matched) != 1 {
		return stageauthority.StageRow{}, false
	}
	for _, row := range matched {
		return row, true
	}
	return stageauthority.StageRow{}, false
}

func diagramCallbackEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimCallbackHandoff },
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object} },
	)
}

func diagramArgumentFlowEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimArgumentFlow },
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object} },
	)
}

func diagramTypeRelationEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		types.IsRepoMapTypeRelationEvidence,
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object} },
	)
}

func diagramRegistrationEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimRegistrationEdge },
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object} },
	)
}

func diagramValueFlowEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string, relation types.DiagramRelationKind) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	wantForm := types.ClaimFormForRelation(relation)
	if fromSymbol == "" || toSymbol == "" ||
		(relation != types.DiagramRelAssignment && relation != types.DiagramRelDataFlow && relation != types.DiagramRelReturn) ||
		(wantForm != types.ClaimAssignmentFact && wantForm != types.ClaimReturnFact) {
		return false
	}
	assignmentEndpoint := func(ev types.EvidenceItem, receiver bool) []string {
		lhs, rhs, ok := types.AssignmentEvidenceEndpoints(ev)
		if ok {
			if receiver {
				// Subject/Object were already checked against this exact tuple.
				// Retain both the source spelling and its safe model-authored local
				// qualification so `o.field` may be displayed as `field` without
				// letting an unrelated owner mint authority.
				return []string{lhs, ev.Subject}
			}
			return []string{rhs, ev.Object}
		}
		indexedReceiver, container, value, indexed := types.IndexedAssignmentEvidenceEndpoints(ev)
		if !indexed {
			return nil
		}
		if receiver {
			return []string{container, indexedReceiver}
		}
		return []string{value, ev.Object}
	}
	sourceCandidates := func(ev types.EvidenceItem) []string { return []string{ev.Subject} }
	targetCandidates := func(ev types.EvidenceItem) []string { return []string{ev.Object} }
	if relation == types.DiagramRelAssignment {
		sourceCandidates = func(ev types.EvidenceItem) []string { return assignmentEndpoint(ev, true) }
		targetCandidates = func(ev types.EvidenceItem) []string { return assignmentEndpoint(ev, false) }
	} else if relation == types.DiagramRelDataFlow {
		// data_flow is the execution-direction view of the same exact source
		// statement: RHS value/source -> LHS assigned receiver.
		sourceCandidates = func(ev types.EvidenceItem) []string { return assignmentEndpoint(ev, false) }
		targetCandidates = func(ev types.EvidenceItem) []string { return assignmentEndpoint(ev, true) }
	}
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool {
			if types.ClaimFormOf(ev) != wantForm {
				return false
			}
			return wantForm != types.ClaimAssignmentFact ||
				types.AssignmentEvidenceEndpointsMatch(ev) || types.IndexedAssignmentEvidenceEndpointsMatch(ev)
		},
		sourceCandidates,
		targetCandidates,
	)
}

// diagramStrictLogicalRelationNeedsEvidence names the logical relations that
// used to be accepted from relation_kind alone inside a grounded source
// diagram. Containment deliberately joins this list even though it has no
// edge-level ClaimForm: strict source diagrams must express hierarchy with a
// Mermaid subgraph (or omit the edge) until a precise directed containment
// carrier exists. A model-authored enum cannot manufacture that carrier.
func diagramStrictLogicalRelationNeedsEvidence(relation types.DiagramRelationKind) bool {
	switch relation {
	case types.DiagramRelGuard,
		types.DiagramRelControlFlow,
		types.DiagramRelImport,
		types.DiagramRelPrecedence,
		types.DiagramRelContain,
		types.DiagramRelObserve,
		types.DiagramRelTemporal:
		return true
	default:
		return false
	}
}

// diagramLogicalRelationEdgeHasTypedEvidence verifies only structured,
// citable evidence fields. It never derives relation authority from a Mermaid
// label, the raw request, model reasoning, or rendered answer text.
//
// Guard rows use the parser-grounded enclosing callable (OwnerSymbol) or the
// typed Subject as their source and the condition token (AnchorSymbol) or an
// explicitly supplied Object as their destination. Other logical relations
// preserve the typed Subject -> Object/AnchorSymbol direction. Containment has
// no edge-level claim form and therefore cannot pass this strict edge gate.
func diagramLogicalRelationEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string, relation types.DiagramRelationKind) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	wantForm := types.ClaimFormForRelation(relation)
	if fromSymbol == "" || toSymbol == "" || wantForm == types.ClaimUnknown {
		return false
	}
	return diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == wantForm },
		func(ev types.EvidenceItem) []string {
			if relation == types.DiagramRelGuard {
				return []string{ev.Subject, ev.OwnerSymbol}
			}
			return []string{ev.Subject}
		},
		func(ev types.EvidenceItem) []string { return []string{ev.Object, ev.AnchorSymbol} },
	)
}

// diagramRelationEdgeHasExactOrUniqueShortProjection is the single endpoint
// identity authority shared by every strict diagram-relation lane. The typed
// evidence row still owns the relation and its direction; this helper only
// reconciles presentation identity on the source and destination sides.
//
// A qualified diagram endpoint requires an exact typed qualified identity. A
// short endpoint may project one qualified typed identity family, but two
// owners with the same tail fail closed. Both endpoints must match the same
// evidence row, so independently matching source/target rows cannot mint a
// relation. Request text, Mermaid labels, model prose, paths, and language
// names never participate.
func diagramRelationEdgeHasExactOrUniqueShortProjection(
	evidence []types.EvidenceItem,
	fromSymbol, toSymbol string,
	accept func(types.EvidenceItem) bool,
	sourceCandidates, targetCandidates func(types.EvidenceItem) []string,
) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" || accept == nil || sourceCandidates == nil || targetCandidates == nil {
		return false
	}
	if !diagramRelationEndpointHasExactOrUniqueShortProjection(evidence, fromSymbol, accept, sourceCandidates) ||
		!diagramRelationEndpointHasExactOrUniqueShortProjection(evidence, toSymbol, accept, targetCandidates) {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || !accept(ev) {
			continue
		}
		if diagramRelationEndpointCandidateSetMatches(sourceCandidates(ev), fromSymbol) &&
			diagramRelationEndpointCandidateSetMatches(targetCandidates(ev), toSymbol) {
			return true
		}
	}
	return false
}

func diagramRelationEndpointHasExactOrUniqueShortProjection(
	evidence []types.EvidenceItem,
	surface string,
	accept func(types.EvidenceItem) bool,
	candidateSet func(types.EvidenceItem) []string,
) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	qualified := diagramEvidenceQualifiedOwner(surface) != ""
	var candidates []string
	for _, ev := range evidence {
		if !ev.IsCitable() || !accept(ev) {
			continue
		}
		for _, raw := range candidateSet(ev) {
			raw = strings.TrimSpace(raw)
			if raw == "" || !diagramRelationEndpointCandidateMatches(raw, surface) {
				continue
			}
			if qualified {
				return true
			}
			duplicate := false
			for _, existing := range candidates {
				if types.AnswerCodeIdentitySurfacesEquivalent(existing, raw) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				candidates = append(candidates, raw)
			}
		}
	}
	if len(candidates) == 0 {
		return false
	}
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if !types.AnswerCodeIdentitySurfacesCompatible(candidates[i], candidates[j]) {
				return false
			}
		}
	}
	return true
}

func diagramRelationEndpointCandidateSetMatches(candidates []string, surface string) bool {
	for _, candidate := range candidates {
		if diagramRelationEndpointCandidateMatches(candidate, surface) {
			return true
		}
	}
	return false
}

func diagramRelationEndpointCandidateMatches(candidate, surface string) bool {
	candidate = strings.TrimSpace(candidate)
	surface = strings.TrimSpace(surface)
	if candidate == "" || surface == "" {
		return false
	}
	if candidate == surface || types.AnswerCodeIdentitySurfacesEquivalent(candidate, surface) {
		return true
	}
	if diagramEvidenceQualifiedOwner(surface) != "" {
		// A short evidence endpoint cannot mint a model-authored owner.
		return false
	}
	return types.AnswerCodeIdentitySurfacesCompatible(candidate, surface)
}

func diagramParsedEdgeRequiresCallAuthority(kind types.DiagramKind, relations map[types.DiagramRelationKind]bool, hasTypedCallEvidence bool) bool {
	if relations[types.DiagramRelCall] {
		return true
	}
	switch kind {
	case types.DiagramSequence:
		// Mermaid sequence arrows are presentation syntax shared by calls,
		// callbacks, value flow, registration, declared-type relationships, and
		// other explicitly typed relations. A canonical non-call edge_anchor is
		// therefore the relation owner; its own exact evidence gate below decides
		// whether the edge is legal. Requiring call authority as well would make a
		// producer-owned assignment/return/etc. recipe impossible to satisfy.
		// With no explicit typed owner the historical fail-closed call default is
		// preserved, and an explicit call owner always wins above.
		for relation := range relations {
			if relation.IsValid() && relation != types.DiagramRelCall {
				return false
			}
		}
		return true
	case types.DiagramCallDAG:
		// A call-DAG may contain explicitly typed control/dependency edges, but
		// a same-direction call already proven by typed evidence cannot hide
		// behind one of them.
		return hasTypedCallEvidence
	default:
		// Flow and architecture diagrams may legitimately draw any canonical
		// relation. Once a precise non-call owner exists, do not reinterpret
		// that edge from its label, operator, request text, or model prose.
		return false
	}
}

func diagramHasValidTypedRelation(relations map[types.DiagramRelationKind]bool) bool {
	for relation := range relations {
		if relation.IsValid() {
			return true
		}
	}
	return false
}

func diagramRequiresTypedBodyOwnership(kind types.DiagramKind, sourceCallChainFamily bool) bool {
	return kind == types.DiagramCallDAG || sourceCallChainFamily
}

// diagramSequenceStructuralReplyIndexSet recognizes only a dashed reverse edge
// that consumes a preceding, still-unpaired forward sequence invocation. A
// later invocation cannot retroactively authorize an earlier reply, and one
// invocation cannot authorize several replies. Pairing is LIFO per exact node
// pair so nested calls and repeated calls between the same actors retain their
// visible temporal order.
//
// A standalone -->> edge is not sufficient to self-declare as a reply, an
// explicit reverse call anchor keeps that edge inside the call-evidence
// contract, and a typed non-call forward edge cannot be borrowed as an
// invocation. Message labels and endpoint evidence do not participate in this
// structural syntax classification.
func diagramSequenceStructuralReplyIndexSet(kind types.DiagramKind, edges []mermaidcompat.Edge, typedRelations map[string]map[types.DiagramRelationKind]bool) map[int]bool {
	out := make(map[int]bool)
	if kind != types.DiagramSequence {
		return out
	}
	pending := make(map[string][]int)
	for index, edge := range edges {
		key := diagramEvidenceEdgeKey(edge.From, edge.To)
		if mermaidcompat.SequenceArrowBase(edge.Operator) != "-->>" {
			if diagramParsedEdgeRequiresCallAuthority(kind, typedRelations[key], false) {
				pending[key] = append(pending[key], index)
			}
			continue
		}
		if typedRelations[key][types.DiagramRelCall] {
			continue
		}
		reverseKey := diagramEvidenceEdgeKey(edge.To, edge.From)
		stack := pending[reverseKey]
		if len(stack) == 0 {
			continue
		}
		out[index] = true
		pending[reverseKey] = stack[:len(stack)-1]
	}
	return out
}

func diagramTypedAnchorRelationSet(anchors []types.DiagramEdgeAnchor) map[string]map[types.DiagramRelationKind]bool {
	out := make(map[string]map[types.DiagramRelationKind]bool)
	for _, anchor := range anchors {
		relation := diagramAnchorRelation(anchor)
		if !relation.IsValid() {
			continue
		}
		key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
		if key == "\x00" {
			continue
		}
		if out[key] == nil {
			out[key] = make(map[types.DiagramRelationKind]bool)
		}
		out[key][relation] = true
	}
	return out
}

func diagramCallAnchorKeySet(anchors []types.DiagramEdgeAnchor) map[string]bool {
	out := make(map[string]bool, len(anchors))
	for _, anchor := range anchors {
		if diagramAnchorRelation(anchor) != types.DiagramRelCall {
			continue
		}
		if key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode); key != "\x00" {
			out[key] = true
		}
	}
	return out
}

func diagramAnchorRelation(anchor types.DiagramEdgeAnchor) types.DiagramRelationKind {
	relation := anchor.RelationKind
	if !relation.IsValid() {
		relation = types.RelationForClaimForm(anchor.ClaimForm)
	}
	return relation
}

func diagramEvidenceEdgeKey(from, to string) string {
	return strings.ToLower(strings.TrimSpace(from)) + "\x00" + strings.ToLower(strings.TrimSpace(to))
}

// diagramEvidenceUnorderedEdgeKey keys one visible endpoint pair regardless
// of arrow direction, using the same exact normalization as
// diagramEvidenceEdgeKey so the two lanes cannot disagree on identity.
func diagramEvidenceUnorderedEdgeKey(a, b string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}

// diagramAnchorReversedAgainstUniqueVisibleEdge reports the exact stale-anchor
// sub-shape in which the anchor's own endpoint pair is drawn exactly once in
// the body, in the opposite direction. Three precise signals only: the
// anchor's direction is absent from the body, the reverse direction is
// present, and the unordered pair occurs once. Duplicate or bidirectional
// pairs and self-edges never qualify; labels, evidence, request text and
// prose are not read. Pure predicate — it never edits the anchor.
func diagramAnchorReversedAgainstUniqueVisibleEdge(
	anchor types.DiagramEdgeAnchor,
	visibleBodyEdgeKeys map[string]bool,
	visibleBodyPairCounts map[string]int,
) bool {
	from, to := strings.TrimSpace(anchor.FromNode), strings.TrimSpace(anchor.ToNode)
	if from == "" || to == "" || strings.EqualFold(from, to) {
		return false
	}
	return !visibleBodyEdgeKeys[diagramEvidenceEdgeKey(from, to)] &&
		visibleBodyEdgeKeys[diagramEvidenceEdgeKey(to, from)] &&
		visibleBodyPairCounts[diagramEvidenceUnorderedEdgeKey(from, to)] == 1
}

func diagramEvidenceNodeLabels(body string, kind types.DiagramKind) map[string]string {
	candidates := make(map[string]map[string]bool)
	bareReferences := make(map[string]map[string]bool)
	sequenceSyntax := diagramEvidenceUsesSequenceSyntax(body, kind)
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			diagramEvidenceAddNodeLabelCandidate(candidates, decl)
		}
		// Parentheses/brackets in a sequence message are payload, not node
		// declarations: `A->>B: resolve("json")` must not mint a document
		// alias `resolve -> json`. Sequence identities come exclusively from
		// participant/actor declarations; flow-family diagrams keep the node
		// declaration parser.
		if !sequenceSyntax {
			for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
				if diagramEvidenceBareNodeReference(line, decl) {
					diagramEvidenceAddNodeLabelCandidate(bareReferences, decl)
					continue
				}
				diagramEvidenceAddNodeLabelCandidate(candidates, decl)
			}
		}
	}
	return diagramEvidenceUniqueNodeLabels(candidates, bareReferences)
}

// diagramEvidenceDocumentNodeLabels supplies a unique document-level label
// registry for edge anchors carried by sibling structured blocks. Local
// diagram blocks still use their own labels. Reused node IDs with conflicting
// labels are omitted so a cross-block carrier cannot guess an identity.
func diagramEvidenceDocumentNodeLabels(doc *types.AnswerDocumentV2) map[string]string {
	candidates := make(map[string]map[string]bool)
	bareReferences := make(map[string]map[string]bool)
	if doc == nil {
		return nil
	}
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Diagram == nil {
			continue
		}
		sequenceSyntax := diagramEvidenceUsesSequenceSyntax(block.Diagram.Body, block.Diagram.Kind)
		for _, line := range strings.Split(block.Diagram.Body, "\n") {
			for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
				diagramEvidenceAddNodeLabelCandidate(candidates, decl)
			}
			if !sequenceSyntax {
				for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
					if diagramEvidenceBareNodeReference(line, decl) {
						diagramEvidenceAddNodeLabelCandidate(bareReferences, decl)
						continue
					}
					diagramEvidenceAddNodeLabelCandidate(candidates, decl)
				}
			}
		}
	}
	return diagramEvidenceUniqueNodeLabels(candidates, bareReferences)
}

// diagramEvidenceBareNodeReference distinguishes a standalone flowchart node
// reference (`AE`) from a shaped declaration (`AE[analyzerEvaluator]`). A
// subgraph commonly repeats an already declared node by bare ID solely to
// place it in the group. That reference must inherit the one explicit display
// identity rather than becoming a competing label. When no explicit label
// exists, the bare node remains a valid visible declaration.
func diagramEvidenceBareNodeReference(line string, decl mermaidcompat.NodeDecl) bool {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ";"))
	return trimmed != "" && trimmed == strings.TrimSpace(decl.Ident) &&
		strings.TrimSpace(decl.Label) == strings.TrimSpace(decl.Ident)
}

func diagramEvidenceUniqueNodeLabels(explicit, bareReferences map[string]map[string]bool) map[string]string {
	out := make(map[string]string)
	for key, labels := range explicit {
		if len(labels) != 1 {
			continue
		}
		for label := range labels {
			out[key] = label
		}
	}
	for key, labels := range bareReferences {
		if _, hasExplicit := explicit[key]; hasExplicit || len(labels) != 1 {
			continue
		}
		for label := range labels {
			out[key] = label
		}
	}
	return out
}

// diagramEvidenceUsesSequenceSyntax gives the exact Mermaid body declaration
// precedence over the semantic diagram family. The two normally agree, but a
// recovered/malformed document may carry kind=sequence with a flowchart body.
// Falling back to the typed kind keeps declaration-less partial sequence
// bodies conservative without letting message payloads become node labels.
func diagramEvidenceUsesSequenceSyntax(body string, kind types.DiagramKind) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if keyword := mermaidcompat.FirstKeywordIn(line); keyword != "" {
			return keyword == "sequenceDiagram"
		}
		break
	}
	return kind == types.DiagramSequence
}

func diagramEvidenceDocumentEdges(doc *types.AnswerDocumentV2) []mermaidcompat.Edge {
	if doc == nil {
		return nil
	}
	var out []mermaidcompat.Edge
	for i := range doc.Blocks {
		if doc.Blocks[i].Diagram == nil {
			continue
		}
		out = append(out, mermaidcompat.ParseEdges(doc.Blocks[i].Diagram.Body)...)
	}
	return out
}

// diagramEvidenceEffectiveAnchorsForBlock keeps diagram-local ownership exact
// while preserving the documented sibling-carrier lane. A sibling anchor may
// own a body edge only when that raw endpoint pair occurs in exactly one
// diagram block; reused short aliases such as A->B cannot silently authorize
// multiple unrelated diagrams.
func diagramEvidenceEffectiveAnchorsForBlock(doc *types.AnswerDocumentV2, blockIndex int, edgeBlockCounts map[string]int) []types.DiagramEdgeAnchor {
	if doc == nil || blockIndex < 0 || blockIndex >= len(doc.Blocks) || doc.Blocks[blockIndex].Diagram == nil {
		return nil
	}
	bodyKeys := make(map[string]bool)
	for _, edge := range mermaidcompat.ParseEdges(doc.Blocks[blockIndex].Diagram.Body) {
		bodyKeys[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
	}
	out := append([]types.DiagramEdgeAnchor(nil), doc.Blocks[blockIndex].EdgeAnchors...)
	for i := range doc.Blocks {
		if i == blockIndex {
			continue
		}
		for _, anchor := range doc.Blocks[i].EdgeAnchors {
			key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
			if bodyKeys[key] && edgeBlockCounts[key] == 1 {
				out = append(out, anchor)
				continue
			}
			// A standalone relation carrier keeps reader-facing endpoint labels,
			// while an optional sibling diagram may use short Mermaid aliases for
			// those exact visible labels. Resolve an ephemeral validation copy only
			// when that label pair maps to one visible directed edge in exactly one
			// diagram. Never mutate the carrier: if the diagram is later removed,
			// the final relation list must retain the model-authored labels.
			if !answerBlockCarriesStandaloneTypedRelations(doc.Blocks[i]) {
				continue
			}
			from, to, ok := diagramUniqueVisibleAliasPair(doc, anchor.FromNode, anchor.ToNode)
			if !ok {
				continue
			}
			key = diagramEvidenceEdgeKey(from, to)
			if !bodyKeys[key] || edgeBlockCounts[key] != 1 {
				continue
			}
			resolved := anchor
			resolved.FromNode = from
			resolved.ToNode = to
			out = append(out, resolved)
		}
	}
	return out
}

func diagramEvidenceBodyEdgeBlockCounts(doc *types.AnswerDocumentV2) map[string]int {
	out := make(map[string]int)
	if doc == nil {
		return out
	}
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Diagram == nil {
			continue
		}
		seen := make(map[string]bool)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			key := diagramEvidenceEdgeKey(edge.From, edge.To)
			if seen[key] {
				continue
			}
			seen[key] = true
			out[key]++
		}
	}
	return out
}

func diagramEvidenceStrictBodyEdgeKeys(doc *types.AnswerDocumentV2, sourceCallChainFamily bool) map[string]bool {
	out := make(map[string]bool)
	if doc == nil {
		return out
	}
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Diagram == nil || !diagramRequiresTypedBodyOwnership(block.Diagram.Kind, sourceCallChainFamily) {
			continue
		}
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			out[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
		}
	}
	return out
}

func diagramEvidenceAddNodeLabelCandidate(dst map[string]map[string]bool, decl mermaidcompat.NodeDecl) {
	ident := strings.TrimSpace(decl.Ident)
	if ident == "" {
		return
	}
	label := strings.TrimSpace(decl.Label)
	if label == "" {
		label = ident
	}
	key := strings.ToLower(ident)
	if dst[key] == nil {
		dst[key] = make(map[string]bool)
	}
	dst[key][label] = true
}

func diagramEvidenceEndpointSymbol(node string, labels map[string]string, evidenceOpt ...[]types.EvidenceItem) string {
	node = strings.TrimSpace(node)
	if node == "" {
		return ""
	}
	if label := strings.TrimSpace(labels[strings.ToLower(node)]); label != "" {
		fallback := diagramEvidenceLabelSymbol(label)
		// A compact participant label may put one source file in parentheses
		// after the exact code identity (`run (main.rs)`). Treat that suffix like
		// the already-supported newline file/line display metadata. This is a
		// closed source-path grammar, not a prose/keyword projection; the prefix
		// still has to be a complete cross-language code identity and the suffix
		// a known source/config path. It therefore cannot turn call-shaped text or
		// an arbitrary parenthetical into endpoint authority.
		if identity, ok := diagramEvidenceIdentityBeforeSourceFileQualifier(label); ok {
			fallback = identity
		}
		if len(evidenceOpt) == 0 {
			return fallback
		}
		candidates := diagramEvidenceLabelIdentityCandidates(label)
		matched := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			if !diagramEvidenceIdentityAppearsExactly(evidenceOpt[0], candidate) {
				continue
			}
			equivalent := false
			for _, existing := range matched {
				if types.AnswerCodeIdentitySurfacesEquivalent(existing, candidate) {
					equivalent = true
					break
				}
			}
			if !equivalent {
				matched = append(matched, candidate)
			}
		}
		if len(matched) == 1 {
			return matched[0]
		}
		if len(matched) > 1 {
			// More than one distinct typed identity in the same label is an
			// ambiguity, not permission to let display order choose authority.
			return ""
		}
		return fallback
	}
	return node
}

// diagramEvidenceLabelIdentityCandidates preserves every exact code identity
// carried by a Mermaid presentation label. A label may put a human-facing
// value before the canonical source identity (`analyze<br/>StageAnalyze`) or a
// source location after it (`buildAnalysisIR<br/>analyzer.go:1820`). The
// candidates remain inert until one unique identity is present in citable
// typed evidence; neither position, casing, language, nor prose similarity
// chooses the endpoint.
func diagramEvidenceLabelIdentityCandidates(label string) []string {
	parts := []string{strings.TrimSpace(label)}
	normalized := strings.NewReplacer("<br/>", "\n", "<br>", "\n", `\n`, "\n").Replace(label)
	lines := strings.Split(normalized, "\n")
	parts = append(parts, lines...)
	parts = append(parts, diagramEvidenceJoinedLabelIdentityCandidates(lines)...)
	for _, line := range lines {
		if identity, ok := diagramEvidenceIdentityBeforeDisplayQualifier(line); ok {
			parts = append(parts, identity)
		}
		if identity, ok := diagramEvidenceIdentityBeforeSourceFileQualifier(line); ok {
			parts = append(parts, identity)
		}
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if symbol, _, ok := types.ParseAnswerSupportRefMemberLocation(candidate); ok && strings.TrimSpace(symbol) != "" {
			candidate = strings.TrimSpace(symbol)
		}
		candidate = diagramEvidenceExactInlineCodeIdentity(candidate)
		if candidate == "" || types.HasCodeOrConfigPathSuffix(candidate) ||
			!types.AnswerCodeIdentitySurfacesEquivalent(candidate, candidate) {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == candidate {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, candidate)
		}
	}
	return out
}

// diagramEvidenceIdentityBeforeDisplayQualifier separates one exact code
// endpoint from a compact human-facing qualifier such as
// `tokenize_bytes (Rust)` or `Service.handle (ArkTS)`. The qualifier is
// presentation metadata, just like a file/line suffix; it must not change the
// typed endpoint identity.
//
// This is intentionally a closed syntactic projection, not fuzzy matching:
// the parenthesis must be terminal and whitespace-delimited, its content must
// be one simple token, and the prefix must already be an exact cross-language
// code-identity surface. Thus call-shaped labels such as `resolve(json)`,
// prose, and a second endpoint hidden in `(Other.Run)` remain fail-closed. The
// returned candidate is still inert until diagramEvidenceEndpointSymbol finds
// it uniquely in citable typed evidence.
func diagramEvidenceIdentityBeforeDisplayQualifier(label string) (string, bool) {
	label = strings.TrimSpace(label)
	if !strings.HasSuffix(label, ")") {
		return "", false
	}
	open := strings.LastIndex(label, " (")
	if open <= 0 {
		return "", false
	}
	identity := diagramEvidenceExactInlineCodeIdentity(strings.TrimSpace(label[:open]))
	qualifier := strings.TrimSpace(label[open+2 : len(label)-1])
	if identity == "" || qualifier == "" || strings.IndexFunc(qualifier, func(r rune) bool {
		return r <= ' ' || strings.ContainsRune("()[]{}<>/\\.:;,'\"`", r)
	}) >= 0 || !types.AnswerCodeIdentitySurfacesEquivalent(identity, identity) {
		return "", false
	}
	return identity, true
}

// diagramEvidenceIdentityBeforeSourceFileQualifier separates one exact code
// endpoint from a compact source-location display suffix such as
// `run (main.rs)` or `Service::handle (src/service.cpp)`. It is deliberately
// separate from the language/display qualifier helper above: only a known
// source/config path (optionally with a numeric line range) is accepted.
//
// The returned prefix is presentation identity only. Callers still require a
// unique citable typed endpoint and the ordinary relation evidence; this
// helper never reads message text, request prose, answer prose, or repository
// keywords and never creates an edge or relation.
func diagramEvidenceIdentityBeforeSourceFileQualifier(label string) (string, bool) {
	label = strings.TrimSpace(label)
	if !strings.HasSuffix(label, ")") {
		return "", false
	}
	open := strings.LastIndex(label, " (")
	if open <= 0 {
		return "", false
	}
	identity := diagramEvidenceExactInlineCodeIdentity(strings.TrimSpace(label[:open]))
	location := strings.TrimSpace(label[open+2 : len(label)-1])
	if identity == "" || location == "" || strings.ContainsAny(location, "\n\r") ||
		types.HasCodeOrConfigPathSuffix(identity) ||
		types.AnswerCodeIdentitySurfaceKey(identity) == "" {
		return "", false
	}
	if types.HasCodeOrConfigPathSuffix(strings.ReplaceAll(location, `\\`, "/")) {
		return identity, true
	}
	if _, ok := types.ParseAnswerSourceLocationSurface(location); ok {
		return identity, true
	}
	return "", false
}

// diagramEvidenceJoinedLabelIdentityCandidates restores a code identity that
// was split only for Mermaid presentation width, for example
// `explorerEvaluator\n.ParseOutput` or `Service\n::run`. Joining is deliberately
// narrower than general text normalization: every continuation line must
// begin with a language-native member separator and the resulting surface must
// itself be one exact code identity. The candidate remains inert until the
// caller proves that one unique citable typed evidence endpoint owns it.
func diagramEvidenceJoinedLabelIdentityCandidates(lines []string) []string {
	out := make([]string, 0, len(lines))
	for start := 0; start < len(lines)-1; start++ {
		joined := strings.TrimSpace(lines[start])
		if joined == "" {
			continue
		}
		for end := start + 1; end < len(lines); end++ {
			continuation := strings.TrimSpace(lines[end])
			if !diagramEvidenceMemberContinuation(continuation) {
				break
			}
			joined += continuation
			candidate := diagramEvidenceExactInlineCodeIdentity(joined)
			if candidate == "" || types.HasCodeOrConfigPathSuffix(candidate) ||
				!types.AnswerCodeIdentitySurfacesEquivalent(candidate, candidate) {
				continue
			}
			out = append(out, candidate)
		}
	}
	return out
}

func diagramEvidenceMemberContinuation(value string) bool {
	for _, separator := range []string{"::", "->", ".", "#", "/"} {
		if strings.HasPrefix(value, separator) && len(value) > len(separator) {
			return true
		}
	}
	return false
}

func diagramEvidenceIdentityAppearsExactly(evidence []types.EvidenceItem, candidate string) bool {
	for _, item := range evidence {
		if !item.IsCitable() {
			continue
		}
		for _, endpoint := range []string{item.Subject, item.Object, item.AnchorSymbol, item.OwnerSymbol} {
			if types.AnswerCodeIdentitySurfacesEquivalent(endpoint, candidate) {
				return true
			}
		}
	}
	return false
}

// diagramEvidenceLabelSymbol removes only the deterministic presentation
// suffix commonly carried by Mermaid node labels (for example
// `buildAnalysisIR<br/>analyzer.go:1820`). The first line is the typed endpoint
// identity; later lines are file/line display metadata. This is deliberately
// not a fuzzy symbol matcher: no prefix, token-overlap, or prose inference is
// accepted after the exact first-line projection.
func diagramEvidenceLabelSymbol(label string) string {
	return diagramEvidenceExactInlineCodeIdentity(types.DiagramPrimaryVisibleIdentity(label))
}

// diagramEvidenceExactInlineCodeIdentity removes one Markdown presentation
// wrapper only when the complete label is exactly one cross-language code
// identity. Malformed wrappers, prose, multiple tokens, and nested backticks
// stay byte-visible to the existing fail-closed evidence resolver.
func diagramEvidenceExactInlineCodeIdentity(label string) string {
	if len(label) < 3 || label[0] != '`' || label[len(label)-1] != '`' || strings.Count(label, "`") != 2 {
		return label
	}
	identity := label[1 : len(label)-1]
	if identity != strings.TrimSpace(identity) || !types.AnswerCodeIdentitySurfacesCompatible(identity, identity) {
		return label
	}
	return identity
}

func diagramCallEdgeHasTypedEvidence(evidence []types.EvidenceItem, requiredAnchors []types.AnswerRequiredAnchor, fromSymbol, toSymbol, edgeLabel string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" {
		return false
	}
	// Exact endpoint surfaces remain the strongest and cheapest lane.
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if !diagramCallEvidenceEndpointMatches(ev, ev.Subject, fromSymbol) {
			continue
		}
		// Object is the typed fully-qualified callee surface, while
		// AnchorSymbol is the exact callee identifier verified on the call
		// line (for example Object=normalizer.Normalize and
		// AnchorSymbol=Normalize). Both are closed typed fields of the SAME
		// grounded call-site record, so either exact surface may label the
		// destination node without introducing fuzzy/prefix matching.
		if diagramCallEvidenceEndpointMatches(ev, ev.Object, toSymbol) ||
			diagramCallEvidenceEndpointMatches(ev, ev.AnchorSymbol, toSymbol) {
			return true
		}
	}

	// Grounding may preserve an enclosing owner on a call row while a compact
	// architecture/flow diagram uses the operation name alone on either side
	// (for example Orchestrator.runAnalyzePhase -> Orchestrator.dispatchStage is
	// displayed as runAnalyzePhase -> Orchestrator.dispatchStage). The call row
	// already proves the direction; this lane only reconciles each endpoint's
	// presentation identity. Accept a short spelling when every citable endpoint
	// on that side that is compatible with it belongs to one identity family.
	// Two qualified owners with the same operation tail remain incompatible and
	// fail closed.
	//
	// This is intentionally side-aware and pair-preserving.  It does not infer
	// owners from source paths, labels, request text, or prose, and a source-side
	// match plus an unrelated target-side match cannot mint a new edge.
	if diagramRelationEdgeHasExactOrUniqueShortProjection(
		evidence, fromSymbol, toSymbol,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimCallEdge },
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object, ev.AnchorSymbol} },
	) {
		return true
	}

	// A same-language call site can be normalized to short in-file names
	// (`Run -> RunWith`) while the answer uses the exact user endpoint owner
	// (`gate.Run -> gate.RunWith`). Preserve that qualified presentation only
	// when the caller is an exact required mechanism anchor and one of two
	// parser-owned bindings is unique: either a citable definition binds the
	// owner/operation and source file while the qualified callee is already
	// citable, or the call record itself carries the exact grounded OwnerSymbol
	// and targets an unqualified operation inside that same owner. This closes
	// an encoder mismatch without guessing from paths, Mermaid prose, or user
	// text.
	if diagramCallEdgeHasRequiredQualifiedCaller(
		evidence, requiredAnchors, fromSymbol, toSymbol,
	) {
		return true
	}

	// A module/class-qualified display label can name a callable whose own
	// body evidence is necessarily short (for example an inbound Rust call
	// targets walker::collect_files while the call inside walker.rs is recorded
	// as collect_files -> walk). Join those two typed surfaces only when the
	// accepted evidence itself closes the identity: the exact qualified caller
	// is a citable call endpoint, one unique citable definition binds the short
	// operation to the inner call's source file, and no same-named inner caller
	// appears in another source. This preserves reader-friendly qualification
	// without inferring ownership from a file path, Mermaid prose, or language.
	if diagramCallEdgeHasUniqueInboundQualifiedCaller(evidence, fromSymbol, toSymbol) {
		return true
	}

	// A grounded call site can carry the exact callee operation as a short
	// symbol (Object=schedule / AnchorSymbol=schedule) while a diagram uses the
	// definition-qualified endpoint VisitService.schedule. Accept that lossless
	// presentation projection only when one citable typed definition uniquely
	// binds the requested owner+operation. The call record still proves the
	// direction; the definition record only resolves its short destination.
	// Missing, conflicting, or overloaded definition identities fail closed.
	if diagramCallEdgeHasUniqueDefinitionBackedCallee(evidence, fromSymbol, toSymbol) {
		return true
	}

	// Sequence participants are commonly actor/class labels while the typed
	// call evidence is method-qualified (VisitService vs
	// VisitService.schedule). Resolve that presentation alias only when the
	// structured message starts with the exact typed callee operation and the
	// resulting call-edge identity is unique. This is not fuzzy prefix
	// matching: both owners and the operation are exact typed projections; an
	// absent/ambiguous message fails closed.
	operation := diagramEvidenceCallLabelOperation(edgeLabel)
	if operation == "" {
		return false
	}
	candidates := make(map[string]bool)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		subject := strings.TrimSpace(ev.Subject)
		object := strings.TrimSpace(ev.Object)
		if !diagramEvidenceEndpointMatchesQualifiedOwner(fromSymbol, subject) ||
			!diagramEvidenceEndpointMatchesQualifiedOwner(toSymbol, object) {
			continue
		}
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		if anchor == "" {
			anchor = diagramEvidenceQualifiedOperation(object)
		}
		// Grounding canonicalizes AnchorCall to the resolved callee surface, so
		// production evidence commonly carries `VisitService.schedule` here
		// while a sequence message carries the exact operation `schedule(...)`.
		// Compare the lossless operation projection of that SAME typed anchor;
		// owner compatibility above still prevents a same-tail symbol in another
		// class/package from authorizing the edge.
		anchorOperation := diagramEvidenceQualifiedOperation(anchor)
		if anchorOperation == "" || operation != anchorOperation {
			continue
		}
		candidates[subject+"\x00"+object+"\x00"+anchor] = true
	}
	return len(candidates) == 1
}

// diagramCallEdgeTypedEvidenceOccurrenceAuthority returns a stable group key
// and budget for one already-proven visible call. Exact endpoint rows win. For
// class/actor participants, the existing exact message-operation resolver may
// select a different call row for each operation; those rows receive distinct
// keys. A bridge that needs definition/owner context but cannot be attributed
// to one exact row retains a conservative one-occurrence budget.
func diagramCallEdgeTypedEvidenceOccurrenceAuthority(
	evidence []types.EvidenceItem,
	requiredAnchors []types.AnswerRequiredAnchor,
	fromSymbol, toSymbol, edgeLabel string,
) (string, int) {
	if !diagramCallEdgeHasTypedEvidence(evidence, requiredAnchors, fromSymbol, toSymbol, edgeLabel) {
		return "unproven", 0
	}
	seen := make(map[string]struct{})
	add := func(ev types.EvidenceItem) {
		identity := types.StableEvidenceID(ev)
		if strings.TrimSpace(identity) == "" {
			identity = ev.Source + "\x00" + strconv.Itoa(ev.LineStart) + "\x00" + strconv.Itoa(ev.LineEnd) +
				"\x00" + ev.Subject + "\x00" + ev.Object + "\x00" + ev.AnchorSymbol
		}
		seen[identity] = struct{}{}
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		fromMatches := diagramCallEvidenceEndpointMatches(ev, ev.Subject, fromSymbol) ||
			types.AnswerCodeIdentitySurfacesCompatible(ev.Subject, fromSymbol)
		toMatches := diagramCallEvidenceEndpointMatches(ev, ev.Object, toSymbol) ||
			diagramCallEvidenceEndpointMatches(ev, ev.AnchorSymbol, toSymbol) ||
			types.AnswerCodeIdentitySurfacesCompatible(ev.Object, toSymbol) ||
			types.AnswerCodeIdentitySurfacesCompatible(ev.AnchorSymbol, toSymbol)
		if !fromMatches || !toMatches {
			continue
		}
		add(ev)
	}
	// Class/actor participant labels are not method endpoints. When exact
	// endpoint matching found no row, reuse the same exact operation projection
	// that made the edge valid above. The structured message is only an identity
	// discriminator here; it does not create relation authority.
	if len(seen) == 0 {
		operation := diagramEvidenceCallLabelOperation(edgeLabel)
		if operation != "" {
			for _, ev := range evidence {
				if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
					!diagramEvidenceEndpointMatchesQualifiedOwner(fromSymbol, strings.TrimSpace(ev.Subject)) ||
					!diagramEvidenceEndpointMatchesQualifiedOwner(toSymbol, strings.TrimSpace(ev.Object)) {
					continue
				}
				anchor := strings.TrimSpace(ev.AnchorSymbol)
				if anchor == "" {
					anchor = diagramEvidenceQualifiedOperation(strings.TrimSpace(ev.Object))
				}
				if diagramEvidenceQualifiedOperation(anchor) == operation {
					add(ev)
				}
			}
		}
	}
	if len(seen) == 0 {
		key := "bridge\x00" + strings.ToLower(strings.TrimSpace(fromSymbol)) + "\x00" +
			strings.ToLower(strings.TrimSpace(toSymbol))
		return key, 1
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "evidence\x00" + strings.Join(keys, "\x01"), len(keys)
}

func diagramCallEvidenceEndpointMatches(item types.EvidenceItem, raw, surface string) bool {
	if types.AnswerCodeIdentitySurfacesEquivalent(raw, surface) {
		return true
	}
	projected, ok := types.DeterministicSourceQualifiedEvidenceSymbol(item, raw)
	return ok && types.AnswerCodeIdentitySurfacesEquivalent(projected, surface)
}

func diagramCallEdgeHasUniqueInboundQualifiedCaller(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	fromOperation := diagramEvidenceQualifiedOperation(fromSymbol)
	if diagramEvidenceQualifiedOwner(fromSymbol) == "" || fromOperation == "" || toSymbol == "" {
		return false
	}
	// Resolve the presentation identity through the same fail-closed typed
	// bridge consumed by the relation-authority renderer. A model-authored
	// prefix, source path, or language-specific separator cannot mint it.
	qualified, ok := types.ResolveUniqueQualifiedCallEndpoint(evidence, fromOperation)
	if !ok || !types.AnswerCodeIdentitySurfacesEquivalent(qualified, fromSymbol) {
		return false
	}

	found := false
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
			strings.TrimSpace(ev.Subject) != fromOperation {
			continue
		}
		if !diagramEvidenceExactCallTargetMatches(ev, toSymbol) {
			continue
		}
		found = true
	}
	return found
}

func diagramEvidenceExactCallTargetMatches(ev types.EvidenceItem, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	return types.AnswerCodeIdentitySurfacesEquivalent(strings.TrimSpace(ev.Object), target) ||
		types.AnswerCodeIdentitySurfacesEquivalent(strings.TrimSpace(ev.AnchorSymbol), target)
}

// diagramCallAnchorHasTypedEvidence routes explicit edge anchors through the
// same message-operation resolver as parsed diagram edges. The anchor's node
// direction remains the authority; a body label is only a precise alias
// discriminator. Multiple distinct labels that each resolve remain ambiguous.
func diagramCallAnchorHasTypedEvidence(
	evidence []types.EvidenceItem,
	requiredAnchors []types.AnswerRequiredAnchor,
	fromSymbol, toSymbol string,
	anchor types.DiagramEdgeAnchor,
	parsedEdges []mermaidcompat.Edge,
) bool {
	if diagramCallEdgeHasTypedEvidence(evidence, requiredAnchors, fromSymbol, toSymbol, "") {
		return true
	}
	wantKey := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
	labels := make(map[string]bool)
	for _, edge := range parsedEdges {
		if diagramEvidenceEdgeKey(edge.From, edge.To) != wantKey {
			continue
		}
		if label := strings.TrimSpace(edge.Label); label != "" {
			labels[label] = true
		}
	}
	if len(labels) != 1 {
		return false
	}
	for label := range labels {
		return diagramCallEdgeHasTypedEvidence(evidence, requiredAnchors, fromSymbol, toSymbol, label)
	}
	return false
}

func diagramCallEdgeHasRequiredQualifiedCaller(
	evidence []types.EvidenceItem,
	requiredAnchors []types.AnswerRequiredAnchor,
	fromSymbol, toSymbol string,
) bool {
	fromOwner := diagramEvidenceQualifiedOwner(fromSymbol)
	fromOperation := diagramEvidenceQualifiedOperation(fromSymbol)
	toOwner := diagramEvidenceQualifiedOwner(toSymbol)
	toOperation := diagramEvidenceQualifiedOperation(toSymbol)
	callerDefinitionSource, _, callerDefinitionOK := diagramEvidenceUniqueDefinitionLocation(evidence, fromOwner, fromOperation)
	if fromOwner == "" || fromOperation == "" || toOperation == "" {
		return false
	}
	definitionBound := toOwner != "" && callerDefinitionOK &&
		diagramRequiredMechanismAnchorContainsExactSymbol(requiredAnchors, fromSymbol) &&
		diagramEvidenceContainsExactCallEndpoint(evidence, toSymbol)
	// A system-stamped OwnerSymbol can also bind the natural presentation
	// `package.Caller -> Callee`: the caller owner is exact parser metadata and
	// the callee remains the exact Object/AnchorSymbol on that same call-site
	// row. A differently-qualified target still needs definition-backed proof.
	ownerContextBound := toOwner == "" || fromOwner == toOwner
	if !definitionBound && !ownerContextBound {
		return false
	}

	locations := make(map[string]bool)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
			strings.TrimSpace(ev.Subject) != fromOperation {
			continue
		}
		ownerBound := strings.TrimSpace(ev.OwnerSymbol) == fromSymbol
		if definitionBound {
			if strings.TrimSpace(ev.Source) != callerDefinitionSource {
				continue
			}
		} else if !ownerBound {
			// OwnerSymbol is stamped from the parsed enclosing callable after
			// grounding. An exact qualified owner on the call record is therefore
			// a definition-equivalent binding; a short/model-authored owner cannot
			// qualify the endpoint by itself.
			continue
		}
		object := strings.TrimSpace(ev.Object)
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		if object != "" && object != toOperation && object != toSymbol {
			continue
		}
		if object != toOperation && object != toSymbol && anchor != toOperation {
			continue
		}
		key := strings.TrimSpace(ev.Source) + "\x00" + strconv.Itoa(ev.LineStart) + "\x00" + fromOperation + "\x00" + toOperation
		locations[key] = true
	}
	return len(locations) == 1
}

func diagramRequiredMechanismAnchorContainsExactSymbol(required []types.AnswerRequiredAnchor, symbol string) bool {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return false
	}
	for _, anchor := range required {
		if anchor.Kind == types.ContractTermSymbol && strings.TrimSpace(anchor.Text) == symbol {
			return true
		}
	}
	return false
}

func diagramEvidenceContainsExactCallEndpoint(evidence []types.EvidenceItem, symbol string) bool {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if strings.TrimSpace(ev.Subject) == symbol ||
			strings.TrimSpace(ev.Object) == symbol ||
			strings.TrimSpace(ev.AnchorSymbol) == symbol {
			return true
		}
	}
	return false
}

func diagramCallEdgeHasUniqueDefinitionBackedCallee(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	owner := diagramEvidenceQualifiedOwner(toSymbol)
	operation := diagramEvidenceQualifiedOperation(toSymbol)
	if owner == "" || operation == "" {
		return false
	}

	hasDirectedCall := false
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
			strings.TrimSpace(ev.Subject) != fromSymbol {
			continue
		}
		object := strings.TrimSpace(ev.Object)
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		// A differently-qualified Object is contrary evidence, even when its
		// short operation happens to match. Only a genuinely short typed
		// surface can be completed by the definition lane.
		if object != "" && object != operation {
			continue
		}
		if object == operation || (object == "" && anchor == operation) {
			hasDirectedCall = true
			break
		}
	}
	if !hasDirectedCall {
		return false
	}

	return diagramEvidenceHasUniqueDefinition(evidence, owner, operation)
}

func diagramEvidenceHasUniqueDefinition(evidence []types.EvidenceItem, owner, operation string) bool {
	_, _, ok := diagramEvidenceUniqueDefinitionLocation(evidence, owner, operation)
	return ok
}

func diagramEvidenceUniqueDefinitionLocation(evidence []types.EvidenceItem, owner, operation string) (string, int, bool) {
	type definitionLocation struct {
		source string
		line   int
	}
	definitions := make(map[definitionLocation]bool)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimDefinitionFact ||
			strings.TrimSpace(ev.Subject) != owner || strings.TrimSpace(ev.AnchorSymbol) != operation {
			continue
		}
		definitions[definitionLocation{source: strings.TrimSpace(ev.Source), line: ev.LineStart}] = true
	}
	if len(definitions) != 1 {
		return "", 0, false
	}
	for location := range definitions {
		return location.source, location.line, location.source != "" && location.line > 0
	}
	return "", 0, false
}

func diagramEvidenceCallLabelOperation(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if idx := strings.Index(label, "("); idx >= 0 {
		label = strings.TrimSpace(label[:idx])
	}
	fields := strings.Fields(label)
	if len(fields) != 1 {
		return ""
	}
	return diagramEvidenceQualifiedOperation(strings.Trim(fields[0], "`"))
}

func diagramEvidenceEndpointMatchesQualifiedOwner(surface, qualified string) bool {
	surface = strings.TrimSpace(surface)
	qualified = strings.TrimSpace(qualified)
	if surface == "" || qualified == "" {
		return false
	}
	if surface == qualified {
		return true
	}
	return surface == diagramEvidenceQualifiedOwner(qualified)
}

func diagramEvidenceQualifiedOwner(symbol string) string {
	symbol, cut, width := diagramEvidenceQualifiedSplit(symbol)
	if cut <= 0 || cut+width >= len(symbol) {
		return ""
	}
	return strings.TrimSpace(symbol[:cut])
}

func diagramEvidenceQualifiedOperation(symbol string) string {
	symbol, cut, width := diagramEvidenceQualifiedSplit(symbol)
	if cut < 0 {
		return symbol
	}
	if cut+width >= len(symbol) {
		return ""
	}
	return strings.TrimSpace(symbol[cut+width:])
}

func diagramEvidenceQualifiedSplit(symbol string) (string, int, int) {
	symbol = strings.TrimSpace(symbol)
	cut, width := -1, 0
	for _, separator := range []string{"::", "->", ".", "#", "/"} {
		if idx := strings.LastIndex(symbol, separator); idx > cut {
			cut, width = idx, len(separator)
		}
	}
	return symbol, cut, width
}
