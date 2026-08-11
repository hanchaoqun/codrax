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
	typedEndpoints := diagramCanonicalTypedCallEndpointAliases(evidence)
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
		callAliases := diagramCallParticipantAliases(block,
			diagramRequiresTypedBodyOwnership(block.Diagram.Kind, strictSourceCallChain))
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
				canonicalIdentity, typed := typedEndpoints[strings.ToLower(identity)]
				if alias == "" || identity == "" || !typed || canonicalIdentity == "" || !callAliases[alias] {
					continue
				}
				entry := byIdentity[canonicalIdentity]
				if entry == nil {
					entry = &diagramParticipantIdentityAliases{seen: make(map[string]bool)}
					byIdentity[canonicalIdentity] = entry
					order = append(order, canonicalIdentity)
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

func diagramCallParticipantAliases(block *types.AnswerBlock, strictBodyOwnership bool) map[string]bool {
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
	if !strictBodyOwnership {
		return out
	}
	edges := mermaidcompat.ParseEdges(block.Diagram.Body)
	typedRelations := diagramTypedAnchorRelationSet(block.EdgeAnchors)
	structuralReplies := diagramSequenceStructuralReplyKeySet(block.Diagram.Kind, edges, typedRelations)
	for _, edge := range edges {
		key := diagramEvidenceEdgeKey(edge.From, edge.To)
		if structuralReplies[key] ||
			!diagramParsedEdgeRequiresCallAuthority(block.Diagram.Kind, typedRelations[key], false) {
			continue
		}
		out[strings.TrimSpace(edge.From)] = true
		out[strings.TrimSpace(edge.To)] = true
	}
	return out
}

// diagramCanonicalTypedCallEndpointAliases preserves alias families already
// carried by one citable call row. Object and AnchorSymbol are two exact
// source representations of that row's callee, so `gate.RunWith` and
// `RunWith` can canonicalize to the same endpoint without consulting Mermaid
// prose or a source path. An alias that points at multiple canonical owners is
// omitted and therefore remains fail-closed.
func diagramCanonicalTypedCallEndpointAliases(evidence []types.EvidenceItem) map[string]string {
	candidates := make(map[string]map[string]bool)
	add := func(canonical string, aliases ...string) {
		canonical = strings.TrimSpace(canonical)
		if canonical == "" {
			return
		}
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			key := strings.ToLower(alias)
			if candidates[key] == nil {
				candidates[key] = make(map[string]bool)
			}
			candidates[key][canonical] = true
		}
	}
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimCallEdge {
			continue
		}
		add(item.Subject, item.Subject)
		callee := strings.TrimSpace(item.Object)
		if callee == "" {
			callee = strings.TrimSpace(item.AnchorSymbol)
		}
		add(callee, item.Object, item.AnchorSymbol)
	}
	out := make(map[string]string)
	for alias, owners := range candidates {
		if len(owners) != 1 {
			continue
		}
		for owner := range owners {
			out[alias] = owner
		}
	}
	return out
}
