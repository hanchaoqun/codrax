package tool

import (
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
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
	BlockID    string
	Issue      string
	FromNode   string
	ToNode     string
	FromSymbol string
	ToSymbol   string
	Relation   types.DiagramRelationKind
}

const (
	diagramCallEdgeIssueDuplicateParticipant  = "duplicate_participant_identity"
	diagramCallEdgeIssueMissingAnchor         = "missing_call_anchor"
	diagramCallEdgeIssueMissingRelationAnchor = "missing_relation_anchor"
	diagramCallEdgeIssueAnchorWithoutBodyEdge = "typed_anchor_without_visible_edge"
	diagramCallEdgeIssueNoEvidence            = "call_edge_unproven"
	diagramCallEdgeIssueOccurrenceUnproven    = "call_edge_occurrence_unproven"
	diagramRegistrationEdgeIssueNoEvidence    = "registration_edge_unproven"
	diagramTypeRelationEdgeIssueNoEvidence    = "type_relation_edge_unproven"
	diagramAssignmentEdgeIssueNoEvidence      = "assignment_edge_unproven"
	diagramReturnEdgeIssueNoEvidence          = "return_edge_unproven"
	diagramCallbackEdgeIssueNoEvidence        = "callback_handoff_unproven"
	diagramSemanticRelationIssueNoEvidence    = "semantic_relation_edge_unproven"
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
func DiagramCallEdgeEvidenceMismatches(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, evidence []types.EvidenceItem) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family == types.QFRootCauseTrace {
		return nil
	}
	strictSourceCallChain := view.Family == types.QFCallChain
	// AxisFlow is a schema-validated request relation, not a keyword guess.
	// In that lane a visible arrow is itself the requested transfer claim, so
	// deleting edge_anchors cannot turn the same factual topology into a
	// metadata-free presentation escape. Ordinary architecture/definition
	// diagrams retain the legacy presentation-only lane. Runtime trace was
	// excluded above and keeps its independent causal projection authority.
	strictSourceRelationBody := strictSourceCallChain || view.RelationAxis == types.AxisFlow
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
		visibleBodyEdgeKeys := make(map[string]bool, len(parsedEdges))
		if block.Kind == types.BlockDiagram && block.Diagram != nil {
			for _, edge := range parsedEdges {
				visibleBodyEdgeKeys[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
			}
		}
		if strictBodyCoverage {
			effectiveAnchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, bodyEdgeBlockCounts)
			callAnchorKeys := diagramCallAnchorKeySet(effectiveAnchors)
			typedAnchorRelations := diagramTypedAnchorRelationSet(effectiveAnchors)
			structuralReplies := diagramSequenceStructuralReplyKeySet(block.Diagram.Kind, parsedEdges, typedAnchorRelations)
			callOccurrenceUse := make(map[string]int)
			for _, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				fromSymbol := diagramEvidenceEndpointSymbol(edge.From, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(edge.To, labels, evidence)
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
				if structuralReplies[key] {
					continue
				}
				relations := typedAnchorRelations[key]
				if !diagramHasValidTypedRelation(relations) {
					issue := diagramCallEdgeIssueMissingRelationAnchor
					if block.Diagram.Kind == types.DiagramSequence || block.Diagram.Kind == types.DiagramCallDAG {
						issue = diagramCallEdgeIssueMissingAnchor
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: issue,
						FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
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
					if diagramLogicalRelationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, relation) {
						continue
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramSemanticRelationIssueNoEvidence,
						FromNode: strings.TrimSpace(edge.From), ToNode: strings.TrimSpace(edge.To),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
						Relation: relation,
					})
				}
				if !requiresCallAuthority {
					continue
				}
				if !callAnchorKeys[key] {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID:    block.ID,
						Issue:      diagramCallEdgeIssueMissingAnchor,
						FromNode:   strings.TrimSpace(edge.From),
						ToNode:     strings.TrimSpace(edge.To),
						FromSymbol: fromSymbol,
						ToSymbol:   toSymbol,
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
							Relation: types.DiagramRelCall,
						})
					}
					continue
				}
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID:    block.ID,
					Issue:      diagramCallEdgeIssueNoEvidence,
					FromNode:   strings.TrimSpace(edge.From),
					ToNode:     strings.TrimSpace(edge.To),
					FromSymbol: fromSymbol,
					ToSymbol:   toSymbol,
				})
			}
		}
		for _, anchor := range block.EdgeAnchors {
			// Diagram-local edge metadata describes the visible body, not a
			// hidden replacement graph. Keep optional visuals free to show any
			// faithful subset of the evidence, including a node-only subset with
			// no anchors, while rejecting the inverse drift where typed anchors
			// survive after every corresponding arrow was deleted. Sibling
			// structured carriers remain legal and are checked through the unique
			// body-edge ownership lane above.
			if block.Kind == types.BlockDiagram && block.Diagram != nil &&
				!visibleBodyEdgeKeys[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] {
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID: block.ID, Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
					FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					FromSymbol: diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence),
					ToSymbol:   diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence),
					Relation:   diagramAnchorRelation(anchor),
				})
				continue
			}
			relation := diagramAnchorRelation(anchor)
			anchorKey := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
			if relation == types.DiagramRelCallback {
				fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
				if !diagramCallbackEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramCallbackEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
					})
				}
				continue
			}
			if relation == types.DiagramRelTypeRelation {
				fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
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
				fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
				if !diagramRegistrationEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: diagramRegistrationEdgeIssueNoEvidence,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
					})
				}
				continue
			}
			if relation == types.DiagramRelAssignment || relation == types.DiagramRelReturn {
				fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
				if !diagramValueFlowEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, relation) {
					issue := diagramAssignmentEdgeIssueNoEvidence
					if relation == types.DiagramRelReturn {
						issue = diagramReturnEdgeIssueNoEvidence
					}
					out = append(out, DiagramCallEdgeEvidenceMismatch{
						BlockID: block.ID, Issue: issue,
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
						FromSymbol: fromSymbol, ToSymbol: toSymbol,
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
				fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
				toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
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
			fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels, evidence)
			toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels, evidence)
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
			})
		}
	}
	return out
}

func diagramCallbackEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallbackHandoff {
			continue
		}
		if diagramTypedValueEndpointMatches(ev.Subject, fromSymbol) &&
			diagramTypedValueEndpointMatches(ev.Object, toSymbol) {
			return true
		}
	}
	return false
}

func diagramTypeRelationEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" {
		return false
	}
	for _, item := range evidence {
		if !item.IsCitable() || !types.IsRepoMapTypeRelationEvidence(item) {
			continue
		}
		if types.AnswerCodeIdentitySurfacesEquivalent(item.Subject, fromSymbol) &&
			types.AnswerCodeIdentitySurfacesEquivalent(item.Object, toSymbol) {
			return true
		}
	}
	return false
}

func diagramRegistrationEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimRegistrationEdge {
			continue
		}
		if types.AnswerCodeIdentitySurfacesEquivalent(ev.Subject, fromSymbol) &&
			types.AnswerCodeIdentitySurfacesEquivalent(ev.Object, toSymbol) {
			return true
		}
	}
	return false
}

func diagramValueFlowEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string, relation types.DiagramRelationKind) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	wantForm := types.ClaimFormForRelation(relation)
	if fromSymbol == "" || toSymbol == "" || (wantForm != types.ClaimAssignmentFact && wantForm != types.ClaimReturnFact) {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != wantForm {
			continue
		}
		if diagramTypedValueEndpointMatches(ev.Subject, fromSymbol) &&
			diagramTypedValueEndpointMatches(ev.Object, toSymbol) {
			return true
		}
	}
	return false
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
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != wantForm {
			continue
		}
		fromMatches := diagramTypedValueEndpointMatches(ev.Subject, fromSymbol)
		if relation == types.DiagramRelGuard {
			fromMatches = fromMatches || diagramTypedValueEndpointMatches(ev.OwnerSymbol, fromSymbol)
		}
		if !fromMatches {
			continue
		}
		if diagramTypedValueEndpointMatches(ev.Object, toSymbol) ||
			diagramTypedValueEndpointMatches(ev.AnchorSymbol, toSymbol) {
			return true
		}
	}
	return false
}

func diagramTypedValueEndpointMatches(evidenceEndpoint, diagramEndpoint string) bool {
	evidenceEndpoint = strings.TrimSpace(evidenceEndpoint)
	diagramEndpoint = strings.TrimSpace(diagramEndpoint)
	if evidenceEndpoint == "" || diagramEndpoint == "" {
		return false
	}
	return evidenceEndpoint == diagramEndpoint ||
		types.CallChainEndpointCompatible(evidenceEndpoint, diagramEndpoint) ||
		types.CallChainEndpointCompatible(diagramEndpoint, evidenceEndpoint)
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

// diagramSequenceStructuralReplyKeySet recognizes only a dashed reverse edge
// paired with a visible forward sequence invocation. A standalone -->> edge is
// not sufficient to self-declare as a reply, and an explicit call anchor keeps
// the reverse edge inside the call-evidence contract. Typed evidence elsewhere
// in the session cannot change the syntax role of a structurally paired reply.
func diagramSequenceStructuralReplyKeySet(kind types.DiagramKind, edges []mermaidcompat.Edge, typedRelations map[string]map[types.DiagramRelationKind]bool) map[string]bool {
	out := make(map[string]bool)
	if kind != types.DiagramSequence {
		return out
	}
	forward := make(map[string]bool)
	for _, edge := range edges {
		if mermaidcompat.SequenceArrowBase(edge.Operator) == "-->>" {
			continue
		}
		forward[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
	}
	for _, edge := range edges {
		if mermaidcompat.SequenceArrowBase(edge.Operator) != "-->>" {
			continue
		}
		key := diagramEvidenceEdgeKey(edge.From, edge.To)
		if typedRelations[key][types.DiagramRelCall] {
			continue
		}
		if forward[diagramEvidenceEdgeKey(edge.To, edge.From)] {
			out[key] = true
		}
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

func diagramEvidenceNodeLabels(body string, kind types.DiagramKind) map[string]string {
	candidates := make(map[string]map[string]bool)
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
				diagramEvidenceAddNodeLabelCandidate(candidates, decl)
			}
		}
	}
	out := make(map[string]string)
	for key, labels := range candidates {
		if len(labels) != 1 {
			continue
		}
		for label := range labels {
			out[key] = label
		}
	}
	return out
}

// diagramEvidenceDocumentNodeLabels supplies a unique document-level label
// registry for edge anchors carried by sibling structured blocks. Local
// diagram blocks still use their own labels. Reused node IDs with conflicting
// labels are omitted so a cross-block carrier cannot guess an identity.
func diagramEvidenceDocumentNodeLabels(doc *types.AnswerDocumentV2) map[string]string {
	candidates := make(map[string]map[string]bool)
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
					diagramEvidenceAddNodeLabelCandidate(candidates, decl)
				}
			}
		}
	}
	out := make(map[string]string)
	for key, labels := range candidates {
		if len(labels) != 1 {
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
			}
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
	parts = append(parts, strings.Split(normalized, "\n")...)
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
	label = strings.TrimSpace(label)
	cut := len(label)
	for _, separator := range []string{"<br/>", "<br>", `\n`, "\n"} {
		if idx := strings.Index(label, separator); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	label = strings.TrimSpace(label[:cut])
	if symbol, _, ok := types.ParseAnswerSupportRefMemberLocation(label); ok && strings.TrimSpace(symbol) != "" {
		label = strings.TrimSpace(symbol)
	}
	return diagramEvidenceExactInlineCodeIdentity(label)
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
	if diagramCallEndpointHasExactOrUniqueShortProjection(evidence, fromSymbol, true) &&
		diagramCallEndpointHasExactOrUniqueShortProjection(evidence, toSymbol, false) {
		for _, ev := range evidence {
			if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
				!diagramCallEndpointMatchesExactOrShortProjection(ev, fromSymbol, true) {
				continue
			}
			if diagramCallEndpointMatchesExactOrShortProjection(ev, toSymbol, false) {
				return true
			}
		}
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

// diagramCallEndpointHasExactOrUniqueShortProjection preserves a qualified
// diagram endpoint only when citable call evidence carries that exact owner.
// A bare operation may use the existing unique typed projection. This keeps a
// mixed short/qualified presentation usable without letting a short evidence
// endpoint mint a model-authored owner qualification.
func diagramCallEndpointHasExactOrUniqueShortProjection(evidence []types.EvidenceItem, surface string, sourceSide bool) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	if diagramEvidenceQualifiedOwner(surface) == "" {
		return diagramCallEndpointHasUniqueTypedProjection(evidence, surface, sourceSide)
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if diagramCallEndpointMatchesExactOrShortProjection(ev, surface, sourceSide) {
			return true
		}
	}
	return false
}

func diagramCallEndpointMatchesExactOrShortProjection(ev types.EvidenceItem, surface string, sourceSide bool) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	qualified := diagramEvidenceQualifiedOwner(surface) != ""
	match := func(candidate string) bool {
		if qualified {
			return types.AnswerCodeIdentitySurfacesEquivalent(candidate, surface)
		}
		return types.AnswerCodeIdentitySurfacesCompatible(candidate, surface)
	}
	if sourceSide {
		return match(ev.Subject)
	}
	return match(ev.Object) || match(ev.AnchorSymbol)
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

// diagramCallEndpointHasUniqueTypedProjection reports whether one diagram
// endpoint has exactly one lossless identity family on the selected side of
// the citable call graph. Qualified spellings with language-native separators
// are one family when they are equivalent; a qualified spelling and its short
// final operation are also one family. Distinct qualified owners are not.
func diagramCallEndpointHasUniqueTypedProjection(evidence []types.EvidenceItem, surface string, sourceSide bool) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	candidates := make([]string, 0, 2)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || !types.AnswerCodeIdentitySurfacesCompatible(raw, surface) {
			return
		}
		for _, existing := range candidates {
			if types.AnswerCodeIdentitySurfacesEquivalent(existing, raw) {
				return
			}
		}
		candidates = append(candidates, raw)
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if sourceSide {
			add(ev.Subject)
			continue
		}
		add(ev.Object)
		add(ev.AnchorSymbol)
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
