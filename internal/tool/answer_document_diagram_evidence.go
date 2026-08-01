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
	FromNode   string
	ToNode     string
	FromSymbol string
	ToSymbol   string
}

// DiagramCallEdgeEvidenceMismatches cross-checks the directed call edges in a
// call-chain diagram against the accepted evidence pool. The family check is
// intentionally narrow: runtime/root-cause trace diagrams, including explicit
// time-window causal projections and their automatic supplements, do not enter
// this source-code call-edge contract.
func DiagramCallEdgeEvidenceMismatches(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, evidence []types.EvidenceItem) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family != types.QFCallChain || len(evidence) == 0 {
		return nil
	}
	var out []DiagramCallEdgeEvidenceMismatch
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil || len(block.EdgeAnchors) == 0 {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body)
		for _, anchor := range block.EdgeAnchors {
			relation := anchor.RelationKind
			if !relation.IsValid() {
				relation = types.RelationForClaimForm(anchor.ClaimForm)
			}
			if relation != types.DiagramRelCall {
				continue
			}
			fromSymbol := diagramEvidenceEndpointSymbol(anchor.FromNode, labels)
			toSymbol := diagramEvidenceEndpointSymbol(anchor.ToNode, labels)
			if diagramCallEdgeHasTypedEvidence(evidence, fromSymbol, toSymbol) {
				continue
			}
			out = append(out, DiagramCallEdgeEvidenceMismatch{
				BlockID:    block.ID,
				FromNode:   strings.TrimSpace(anchor.FromNode),
				ToNode:     strings.TrimSpace(anchor.ToNode),
				FromSymbol: fromSymbol,
				ToSymbol:   toSymbol,
			})
		}
	}
	return out
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
		return label
	}
	return node
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
