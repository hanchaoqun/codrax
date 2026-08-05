package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

type diagramParticipantIdentityAliases struct {
	aliases []string
	seen    map[string]bool
}

// diagramDuplicateTypedParticipantIdentities reports only exact typed call
// endpoints declared under multiple Mermaid node IDs. It deliberately does
// not merge class/actor presentation carriers: those can legitimately use the
// same visible owner while distinct message operations disambiguate calls.
func diagramDuplicateTypedParticipantIdentities(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, strictSourceCallChain bool) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil {
		return nil
	}
	typedEndpoints := diagramExactTypedCallEndpoints(evidence)
	if len(typedEndpoints) == 0 {
		return nil
	}
	var out []DiagramCallEdgeEvidenceMismatch
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Diagram == nil ||
			(block.Diagram.Kind != types.DiagramSequence && block.Diagram.Kind != types.DiagramCallDAG) {
			continue
		}
		callAliases := diagramCallParticipantAliases(block, strictSourceCallChain)
		if len(callAliases) == 0 {
			continue
		}
		byIdentity := make(map[string]*diagramParticipantIdentityAliases)
		var order []string
		for _, line := range strings.Split(block.Diagram.Body, "\n") {
			decls := mermaidcompat.SequenceParticipantDeclarations(line)
			if block.Diagram.Kind == types.DiagramCallDAG {
				decls = mermaidcompat.NodeDeclarationsAll(line)
			}
			for _, decl := range decls {
				alias := strings.TrimSpace(decl.Ident)
				identity := diagramEvidenceLabelSymbol(decl.Label)
				if identity == "" {
					identity = alias
				}
				if alias == "" || identity == "" || !typedEndpoints[identity] || !callAliases[alias] {
					continue
				}
				entry := byIdentity[identity]
				if entry == nil {
					entry = &diagramParticipantIdentityAliases{seen: make(map[string]bool)}
					byIdentity[identity] = entry
					order = append(order, identity)
				}
				if !entry.seen[alias] {
					entry.seen[alias] = true
					entry.aliases = append(entry.aliases, alias)
				}
			}
		}
		for _, identity := range order {
			aliases := byIdentity[identity].aliases
			if len(aliases) < 2 {
				continue
			}
			out = append(out, DiagramCallEdgeEvidenceMismatch{
				BlockID:    block.ID,
				Issue:      diagramCallEdgeIssueDuplicateParticipant,
				FromNode:   aliases[0],
				ToNode:     aliases[1],
				FromSymbol: identity,
				ToSymbol:   identity,
			})
		}
	}
	return out
}

func diagramCallParticipantAliases(block *types.AnswerBlock, strictSourceCallChain bool) map[string]bool {
	out := make(map[string]bool)
	if block == nil || block.Diagram == nil {
		return out
	}
	for _, anchor := range block.EdgeAnchors {
		if diagramAnchorRelation(anchor) != types.DiagramRelCall {
			continue
		}
		out[strings.TrimSpace(anchor.FromNode)] = true
		out[strings.TrimSpace(anchor.ToNode)] = true
	}
	if !strictSourceCallChain {
		return out
	}
	edges := mermaidcompat.ParseEdges(block.Diagram.Body)
	typedRelations := diagramTypedAnchorRelationSet(block.EdgeAnchors)
	structuralReplies := diagramSequenceStructuralReplyKeySet(block.Diagram.Kind, edges, typedRelations)
	for _, edge := range edges {
		if structuralReplies[diagramEvidenceEdgeKey(edge.From, edge.To)] ||
			!diagramParsedEdgeRequiresCallAuthority(block.Diagram.Kind, edge, typedRelations) {
			continue
		}
		out[strings.TrimSpace(edge.From)] = true
		out[strings.TrimSpace(edge.To)] = true
	}
	return out
}

func diagramExactTypedCallEndpoints(evidence []types.EvidenceItem) map[string]bool {
	out := make(map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimCallEdge {
			continue
		}
		if subject := strings.TrimSpace(item.Subject); subject != "" {
			out[subject] = true
		}
		if object := strings.TrimSpace(item.Object); object != "" {
			out[object] = true
		} else if anchor := strings.TrimSpace(item.AnchorSymbol); anchor != "" {
			out[anchor] = true
		}
	}
	return out
}
