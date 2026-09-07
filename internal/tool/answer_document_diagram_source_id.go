package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// diagramExplicitSourceNodeDeclarations owns the source-syntax ID inventory
// shared by repair capabilities and their schema. Mermaid IDs are case
// sensitive, unlike the separate code-identity lookup registry: never emit a
// canonical lookup key as a visible node ID. Conflicting labels for the same
// exact ID stay unavailable; API and api remain distinct declarations.
func diagramExplicitSourceNodeDeclarations(body string, kind types.DiagramKind) []explicitDiagramEndpointDeclaration {
	explicit := make(map[string]map[string]bool)
	bare := make(map[string]map[string]bool)
	sequence := diagramEvidenceUsesSequenceSyntax(body, kind)
	for _, line := range strings.Split(body, "\n") {
		var declarations []mermaidcompat.NodeDecl
		if sequence {
			declarations = mermaidcompat.SequenceParticipantDeclarations(line)
		} else {
			declarations = mermaidcompat.NodeDeclarationsAll(line)
		}
		for _, declaration := range declarations {
			id, label := strings.TrimSpace(declaration.Ident), strings.TrimSpace(declaration.Label)
			if id == "" {
				continue
			}
			if label == "" {
				label = id
			}
			registry := explicit
			if !sequence && diagramEvidenceBareNodeReference(line, declaration) {
				registry = bare
			}
			if registry[id] == nil {
				registry[id] = make(map[string]bool)
			}
			registry[id][label] = true
		}
	}
	labels := diagramEvidenceUniqueNodeLabels(explicit, bare)
	out := make([]explicitDiagramEndpointDeclaration, 0, len(labels))
	for id, label := range labels {
		out = append(out, explicitDiagramEndpointDeclaration{ID: id, Label: label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
