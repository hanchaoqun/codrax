package tool

import (
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
			callAnchorKeys := diagramCallAnchorKeySet(block.EdgeAnchors)
			for _, edge := range parsedEdges {
				key := diagramEvidenceEdgeKey(edge.From, edge.To)
				parsedEdgeKeys[key] = true
				fromSymbol := diagramEvidenceEndpointSymbol(edge.From, labels)
				toSymbol := diagramEvidenceEndpointSymbol(edge.To, labels)
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
				if diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
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
			if diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
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

func diagramCallEdgeHasTypedEvidence(evidence []types.EvidenceItem, fromSymbol, toSymbol string) bool {
	fromSymbol = strings.TrimSpace(fromSymbol)
	toSymbol = strings.TrimSpace(toSymbol)
	if fromSymbol == "" || toSymbol == "" {
		return false
	}
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if strings.TrimSpace(ev.Subject) == fromSymbol && strings.TrimSpace(ev.Object) == toSymbol {
			return true
		}
	}
	return false
}
