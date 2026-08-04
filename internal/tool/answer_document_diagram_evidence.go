package tool

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// DiagramCallEdgeEvidenceMismatch identifies one structured call edge whose
// direction is not backed by any citable typed call-edge EvidenceItem.
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
}

type diagramVisibleCallEdge struct {
	fromSymbol string
	toSymbol   string
	label      string
}

const (
	diagramCallEdgeIssueMissingAnchor = "missing_call_anchor"
	diagramCallEdgeIssueNoEvidence    = "call_edge_unproven"
	diagramCallEdgeIssuePrincipalMiss = "principal_call_edge_missing"
)

// DiagramCallEdgeEvidenceMismatches cross-checks model-authored typed call
// edges against the accepted evidence pool. An explicit relation_kind=call is
// an evidence claim regardless of the surrounding answer family, so it always
// needs one same-direction citable call-site record. QFCallChain additionally
// gets strict sequence/call-DAG body coverage and principal-path completeness.
//
// Runtime/root-cause trace diagrams, including explicit time-window causal
// projections and their automatic supplements, use a separate runtime
// relation authority and deliberately do not enter this source-code contract.
func DiagramCallEdgeEvidenceMismatches(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, evidence []types.EvidenceItem) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family == types.QFRootCauseTrace {
		return nil
	}
	strictSourceCallChain := view.Family == types.QFCallChain
	requiredAnchors := view.RequiredMechanismAnchors
	documentLabels := diagramEvidenceDocumentNodeLabels(doc)
	documentEdges := diagramEvidenceDocumentEdges(doc)
	var out []DiagramCallEdgeEvidenceMismatch
	var visibleCallEdges []diagramVisibleCallEdge
	hasStrictCallDiagram := false
	strictCallDiagramID := ""
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		labels := documentLabels
		strictBodyCoverage := false
		if block.Kind == types.BlockDiagram && block.Diagram != nil {
			labels = diagramEvidenceNodeLabels(block.Diagram.Body)
			strictBodyCoverage = strictSourceCallChain &&
				(block.Diagram.Kind == types.DiagramSequence || block.Diagram.Kind == types.DiagramCallDAG)
		}
		parsedEdges := documentEdges
		if block.Diagram != nil {
			parsedEdges = mermaidcompat.ParseEdges(block.Diagram.Body)
		}
		parsedEdgeKeys := make(map[string]bool, len(parsedEdges))
		if strictBodyCoverage {
			hasStrictCallDiagram = true
			if strictCallDiagramID == "" {
				strictCallDiagramID = block.ID
			}
			callAnchorKeys := diagramCallAnchorKeySet(block.EdgeAnchors)
			typedAnchorRelations := diagramTypedAnchorRelationSet(block.EdgeAnchors)
			structuralReplies := diagramSequenceStructuralReplyKeySet(block.Diagram.Kind, parsedEdges, typedAnchorRelations)
			for _, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				fromSymbol := diagramEvidenceEndpointSymbol(edge.From, labels)
				toSymbol := diagramEvidenceEndpointSymbol(edge.To, labels)
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
				// Unanchored DAG edges retain the fail-closed call default.
				if structuralReplies[key] {
					continue
				}
				if !diagramParsedEdgeRequiresCallAuthority(block.Diagram.Kind, edge, typedAnchorRelations) &&
					!hasTypedCallEvidence {
					continue
				}
				visibleCallEdges = append(visibleCallEdges, diagramVisibleCallEdge{
					fromSymbol: fromSymbol,
					toSymbol:   toSymbol,
					label:      edge.Label,
				})
				parsedEdgeKeys[key] = true
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
			if diagramAnchorRelation(anchor) != types.DiagramRelCall {
				continue
			}
			if strictBodyCoverage && parsedEdgeKeys[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] {
				continue
			}
			fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels)
			toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels)
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
	if strictSourceCallChain && hasStrictCallDiagram {
		principalMissing := make(map[string]bool)
		for _, required := range diagramPrincipalPathCitationCallEvidence(doc, evidence) {
			if diagramVisibleEdgesContainSpecificCall(visibleCallEdges, evidence, requiredAnchors, required) {
				continue
			}
			fromSymbol := strings.TrimSpace(required.Subject)
			toSymbol := strings.TrimSpace(required.Object)
			if toSymbol == "" {
				toSymbol = strings.TrimSpace(required.AnchorSymbol)
			}
			key := diagramEvidenceEdgeKey(fromSymbol, toSymbol)
			if key == "\x00" || principalMissing[key] {
				continue
			}
			principalMissing[key] = true
			out = append(out, DiagramCallEdgeEvidenceMismatch{
				BlockID:    strictCallDiagramID,
				Issue:      diagramCallEdgeIssuePrincipalMiss,
				FromNode:   fromSymbol,
				ToNode:     toSymbol,
				FromSymbol: fromSymbol,
				ToSymbol:   toSymbol,
			})
		}
		principalSymbols := diagramPrincipalPathItemSymbols(doc)
		for _, fromSymbol := range principalSymbols {
			for _, toSymbol := range principalSymbols {
				key := diagramEvidenceEdgeKey(fromSymbol, toSymbol)
				if fromSymbol == toSymbol || principalMissing[key] {
					continue
				}
				requiredCalls := diagramCallEvidenceForEndpoints(evidence, requiredAnchors, fromSymbol, toSymbol)
				if len(requiredCalls) == 0 {
					continue
				}
				visible := false
				for _, required := range requiredCalls {
					if diagramVisibleEdgesContainSpecificCall(visibleCallEdges, evidence, requiredAnchors, required) {
						visible = true
						break
					}
				}
				if visible {
					continue
				}
				principalMissing[key] = true
				out = append(out, DiagramCallEdgeEvidenceMismatch{
					BlockID:    strictCallDiagramID,
					Issue:      diagramCallEdgeIssuePrincipalMiss,
					FromNode:   fromSymbol,
					ToNode:     toSymbol,
					FromSymbol: fromSymbol,
					ToSymbol:   toSymbol,
				})
			}
		}
	}
	return out
}

// diagramCallEvidenceForEndpoints resolves a model-selected principal pair to
// the same typed call records used by the visible-edge lane. Returning the
// records (rather than comparing raw labels) lets class/actor presentations be
// checked through diagramVisibleEdgesContainSpecificCall without inventing a
// second alias policy.
func diagramCallEvidenceForEndpoints(evidence []types.EvidenceItem, requiredAnchors []types.AnswerRequiredAnchor, fromSymbol, toSymbol string) []types.EvidenceItem {
	definitions := make([]types.EvidenceItem, 0, len(evidence))
	for _, ev := range evidence {
		if types.ClaimFormOf(ev) == types.ClaimDefinitionFact {
			definitions = append(definitions, ev)
		}
	}
	var out []types.EvidenceItem
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		proof := make([]types.EvidenceItem, 0, len(definitions)+1)
		proof = append(proof, ev)
		proof = append(proof, definitions...)
		if diagramCallEdgeHasTypedEvidence(proof, requiredAnchors, fromSymbol, toSymbol, "") {
			out = append(out, ev)
		}
	}
	return out
}

// diagramPrincipalPathCitationCallEvidence resolves the citations selected by
// model-owned principal path rows back to their citable typed call records.
// This is the citation-carrier completeness lane: a row label may be an exact
// node identity or a presentation such as `caller -> callee`, but neither the
// label nor item prose is parsed to make a hard-gate decision. File + call-site
// line are precise typed fields. If one citation names multiple distinct call
// directions, the row is ambiguous and this lane deliberately declines to
// guess.
func diagramPrincipalPathCitationCallEvidence(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem) []types.EvidenceItem {
	if doc == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []types.EvidenceItem
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.SurfaceRole != types.SurfacePrincipal || block.Kind == types.BlockDiagram ||
			!types.AnswerBlockKindRendersStructuredItems(block.Kind) ||
			!diagramBlockCarriesFacet(block, types.FacetPrincipalPathEdge) ||
			!diagramBlockCarriesClaimForm(block, types.ClaimCallEdge) {
			continue
		}
		for _, item := range block.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			citation := doc.Citations[item.CitationRef]
			matches := diagramCitationCallEvidenceMatches(citation, evidence)
			if len(matches) != 1 {
				continue
			}
			match := matches[0]
			key := diagramTypedCallEvidenceKey(match)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, match)
		}
	}
	return out
}

func diagramCitationCallEvidenceMatches(citation types.Citation, evidence []types.EvidenceItem) []types.EvidenceItem {
	if strings.TrimSpace(citation.File) == "" || citation.Line <= 0 {
		return nil
	}
	byDirection := make(map[string]types.EvidenceItem)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
			ev.LineStart != citation.Line || !preEmitPathMatches(ev.Source, citation.File) {
			continue
		}
		key := diagramTypedCallEvidenceKey(ev)
		if key != "" {
			byDirection[key] = ev
		}
	}
	if len(byDirection) != 1 {
		return nil
	}
	for _, ev := range byDirection {
		return []types.EvidenceItem{ev}
	}
	return nil
}

func diagramTypedCallEvidenceKey(ev types.EvidenceItem) string {
	fromSymbol := strings.TrimSpace(ev.Subject)
	toSymbol := strings.TrimSpace(ev.Object)
	if toSymbol == "" {
		toSymbol = strings.TrimSpace(ev.AnchorSymbol)
	}
	if fromSymbol == "" || toSymbol == "" {
		return ""
	}
	return diagramEvidenceEdgeKey(fromSymbol, toSymbol)
}

func diagramVisibleEdgesContainSpecificCall(edges []diagramVisibleCallEdge, evidence []types.EvidenceItem, requiredAnchors []types.AnswerRequiredAnchor, required types.EvidenceItem) bool {
	proof := make([]types.EvidenceItem, 0, len(evidence)+1)
	proof = append(proof, required)
	for _, ev := range evidence {
		if types.ClaimFormOf(ev) == types.ClaimDefinitionFact {
			proof = append(proof, ev)
		}
	}
	for _, edge := range edges {
		if diagramCallEdgeHasTypedEvidence(proof, requiredAnchors, edge.fromSymbol, edge.toSymbol, edge.label) {
			return true
		}
	}
	return false
}

// diagramPrincipalPathItemSymbols returns only model-selected, structured
// principal hop identities. This is the narrow completeness boundary: a
// citable call edge is required in the diagram only when BOTH endpoints were
// already selected by the model into a principal_path_edge item carrier.
// Summary/item prose and the raw request are deliberately ignored.
func diagramPrincipalPathItemSymbols(doc *types.AnswerDocumentV2) []string {
	if doc == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.SurfaceRole != types.SurfacePrincipal || block.Kind == types.BlockDiagram ||
			!types.AnswerBlockKindRendersStructuredItems(block.Kind) ||
			!diagramBlockCarriesFacet(block, types.FacetPrincipalPathEdge) ||
			!diagramBlockCarriesClaimForm(block, types.ClaimCallEdge) {
			continue
		}
		for _, item := range block.Items {
			symbol := strings.TrimSpace(item.Label)
			key := strings.ToLower(symbol)
			if symbol == "" || seen[key] || !types.IsCodeIdentitySurface(symbol) {
				continue
			}
			seen[key] = true
			out = append(out, symbol)
		}
	}
	return out
}

func diagramBlockCarriesFacet(block *types.AnswerBlock, facet types.AnswerFacetKind) bool {
	if block == nil {
		return false
	}
	want := string(facet)
	for _, got := range block.FacetIDs {
		if strings.TrimSpace(got) == want {
			return true
		}
	}
	for _, use := range block.ClaimUses {
		if strings.TrimSpace(use.FacetID) == want {
			return true
		}
	}
	return false
}

func diagramBlockCarriesClaimForm(block *types.AnswerBlock, claim types.ClaimForm) bool {
	if block == nil {
		return false
	}
	for _, use := range block.ClaimUses {
		if use.ClaimForm == claim {
			return true
		}
	}
	return false
}

func diagramParsedEdgeRequiresCallAuthority(kind types.DiagramKind, edge mermaidcompat.Edge, typedRelations map[string]map[types.DiagramRelationKind]bool) bool {
	if kind != types.DiagramCallDAG {
		return true
	}
	relations := typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]
	if relations[types.DiagramRelCall] {
		return true
	}
	// Only an explicit, schema-validated non-call relation can take a
	// call-DAG edge out of the call-evidence contract. Labels and prose are
	// intentionally ignored here; an absent/unknown relation stays fail-closed.
	for relation := range relations {
		if relation.IsValid() {
			return false
		}
	}
	return true
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

func diagramEvidenceNodeLabels(body string) map[string]string {
	candidates := make(map[string]map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			diagramEvidenceAddNodeLabelCandidate(candidates, decl)
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			diagramEvidenceAddNodeLabelCandidate(candidates, decl)
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
		for _, line := range strings.Split(block.Diagram.Body, "\n") {
			for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
				diagramEvidenceAddNodeLabelCandidate(candidates, decl)
			}
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

func diagramEvidenceEndpointSymbol(node string, labels map[string]string) string {
	node = strings.TrimSpace(node)
	if node == "" {
		return ""
	}
	if label := strings.TrimSpace(labels[strings.ToLower(node)]); label != "" {
		return diagramEvidenceLabelSymbol(label)
	}
	return node
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
		return strings.TrimSpace(symbol)
	}
	return label
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
		if strings.TrimSpace(ev.Subject) != fromSymbol {
			continue
		}
		// Object is the typed fully-qualified callee surface, while
		// AnchorSymbol is the exact callee identifier verified on the call
		// line (for example Object=normalizer.Normalize and
		// AnchorSymbol=Normalize). Both are closed typed fields of the SAME
		// grounded call-site record, so either exact surface may label the
		// destination node without introducing fuzzy/prefix matching.
		if strings.TrimSpace(ev.Object) == toSymbol || strings.TrimSpace(ev.AnchorSymbol) == toSymbol {
			return true
		}
	}

	// A same-language call site can be normalized to short in-file names
	// (`Run -> RunWith`) while the answer uses the exact user endpoint owner
	// (`gate.Run -> gate.RunWith`). Preserve that qualified presentation only
	// when four typed authorities agree: the caller is an exact required
	// mechanism anchor, one citable definition binds its owner+operation, the
	// short call site is in that same typed source file, and the callee's
	// qualified surface occurs verbatim in a citable call record. This closes an
	// encoder mismatch without guessing from paths, Mermaid prose, or user text.
	if diagramCallEdgeHasRequiredQualifiedCaller(
		evidence, requiredAnchors, fromSymbol, toSymbol,
	) {
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
		if anchor == "" || operation != anchor {
			continue
		}
		candidates[subject+"\x00"+object+"\x00"+anchor] = true
	}
	return len(candidates) == 1
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
	toOperation := diagramEvidenceQualifiedOperation(toSymbol)
	callerDefinitionSource, _, callerDefinitionOK := diagramEvidenceUniqueDefinitionLocation(evidence, fromOwner, fromOperation)
	if fromOwner == "" || fromOperation == "" || toOperation == "" ||
		!diagramRequiredMechanismAnchorContainsExactSymbol(requiredAnchors, fromSymbol) ||
		!callerDefinitionOK ||
		!diagramEvidenceContainsExactCallEndpoint(evidence, toSymbol) {
		return false
	}

	locations := make(map[string]bool)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge ||
			strings.TrimSpace(ev.Subject) != fromOperation ||
			strings.TrimSpace(ev.Source) != callerDefinitionSource {
			continue
		}
		object := strings.TrimSpace(ev.Object)
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		if object != toOperation && anchor != toOperation {
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
	for _, separator := range []string{"::", ".", "#"} {
		if idx := strings.LastIndex(symbol, separator); idx > cut {
			cut, width = idx, len(separator)
		}
	}
	return symbol, cut, width
}
