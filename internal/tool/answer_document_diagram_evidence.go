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

const (
	diagramCallEdgeIssueMissingAnchor = "missing_call_anchor"
	diagramCallEdgeIssueNoEvidence    = "call_edge_unproven"
	diagramCallEdgeIssuePrincipalMiss = "principal_call_edge_missing"
)

// DiagramCallEdgeEvidenceMismatches cross-checks the directed call edges in a
// call-chain diagram against the accepted evidence pool. The family check is
// intentionally narrow: runtime/root-cause trace diagrams, including explicit
// time-window causal projections and their automatic supplements, do not enter
// this source-code call-edge contract.
func DiagramCallEdgeEvidenceMismatches(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, evidence []types.EvidenceItem) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family != types.QFCallChain {
		return nil
	}
	var out []DiagramCallEdgeEvidenceMismatch
	visibleCallSymbolPairs := make(map[string]bool)
	hasStrictCallDiagram := false
	strictCallDiagramID := ""
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body)
		parsedEdges := mermaidcompat.ParseEdges(block.Diagram.Body)
		parsedEdgeKeys := make(map[string]bool, len(parsedEdges))
		strictBodyCoverage := block.Diagram.Kind == types.DiagramSequence || block.Diagram.Kind == types.DiagramCallDAG
		if strictBodyCoverage {
			hasStrictCallDiagram = true
			if strictCallDiagramID == "" {
				strictCallDiagramID = block.ID
			}
			callAnchorKeys := diagramCallAnchorKeySet(block.EdgeAnchors)
			typedAnchorRelations := diagramTypedAnchorRelationSet(block.EdgeAnchors)
			for _, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				fromSymbol := diagramEvidenceEndpointSymbol(edge.From, labels)
				toSymbol := diagramEvidenceEndpointSymbol(edge.To, labels)
				hasTypedCallEvidence := diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, edge.Label)
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
				if !diagramParsedEdgeRequiresCallAuthority(block.Diagram.Kind, edge, typedAnchorRelations) &&
					!hasTypedCallEvidence {
					continue
				}
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
				visibleCallSymbolPairs[diagramEvidenceEdgeKey(fromSymbol, toSymbol)] = true
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
			if diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, "") {
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
	if hasStrictCallDiagram {
		principalSymbols := diagramPrincipalPathItemSymbols(doc)
		for _, fromSymbol := range principalSymbols {
			for _, toSymbol := range principalSymbols {
				if fromSymbol == toSymbol || visibleCallSymbolPairs[diagramEvidenceEdgeKey(fromSymbol, toSymbol)] {
					continue
				}
				if !diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol, "") {
					continue
				}
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
	if kind == types.DiagramSequence && strings.TrimSpace(edge.Operator) == "-->>" {
		return false
	}
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
	out := make(map[string]string)
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			diagramEvidenceAddNodeLabel(out, decl)
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			diagramEvidenceAddNodeLabel(out, decl)
		}
	}
	return out
}

func diagramEvidenceAddNodeLabel(dst map[string]string, decl mermaidcompat.NodeDecl) {
	ident := strings.TrimSpace(decl.Ident)
	if ident == "" {
		return
	}
	label := strings.TrimSpace(decl.Label)
	if label == "" {
		label = ident
	}
	dst[strings.ToLower(ident)] = label
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

func diagramCallEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol, edgeLabel string) bool {
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

	definitions := make(map[string]bool)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimDefinitionFact ||
			strings.TrimSpace(ev.Subject) != owner || strings.TrimSpace(ev.AnchorSymbol) != operation {
			continue
		}
		definitions[strings.TrimSpace(ev.Source)+"\x00"+strconv.Itoa(ev.LineStart)+"\x00"+owner+"\x00"+operation] = true
	}
	return len(definitions) == 1
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
