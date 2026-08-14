package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeOrphanDiagramEdgeAnchors removes diagram-only metadata after the
// model intentionally removes every typed diagram block. Edge anchors may live
// on a sibling prose/list block while they own endpoints in another diagram,
// but with no diagram anywhere in the document there is no body alias or
// visible relation left to own. Keeping those orphan anchors would turn a
// successful optional-diagram removal into a second, invisible relation claim
// and an avoidable hard retry.
//
// This is a structural JSON repair only: it never edits answer prose, diagram
// source, citations, conclusions, or relation metadata while any typed diagram
// remains present.
func normalizeOrphanDiagramEdgeAnchors(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockDiagram && doc.Blocks[i].Diagram != nil {
			return 0
		}
	}
	removed := 0
	for i := range doc.Blocks {
		removed += len(doc.Blocks[i].EdgeAnchors)
		doc.Blocks[i].EdgeAnchors = nil
	}
	return removed
}

func normalizeDiagramEdgeAnchorMetadata(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		aliases := diagramNodeAliasIndex(block.Diagram.Body)
		for j := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[j]
			if resolved := aliases[diagramSurfaceKey(anchor.FromNode)]; resolved != "" && anchor.FromNode != resolved {
				anchor.FromNode = resolved
				fixed++
			}
			if resolved := aliases[diagramSurfaceKey(anchor.ToNode)]; resolved != "" && anchor.ToNode != resolved {
				anchor.ToNode = resolved
				fixed++
			}
			rel := anchor.RelationKind
			if !rel.IsValid() {
				rel = types.RelationForClaimForm(anchor.ClaimForm)
				if rel.IsValid() {
					anchor.RelationKind = rel
					fixed++
				}
			}
			if rel.IsValid() {
				wantClaim := types.ClaimFormForRelation(rel)
				if wantClaim != types.ClaimUnknown && anchor.ClaimForm != wantClaim {
					anchor.ClaimForm = wantClaim
					fixed++
				}
			}
		}
	}
	// Edge anchors may intentionally live on a sibling prose/list block. The
	// Mermaid syntax normalizer can replace a nonportable code identity in the
	// diagram body with codraxNodeN while preserving that identity as the
	// visible label. Resolve such sibling metadata only when the exact label
	// pair maps to one visible directed edge in exactly one diagram block.
	// Ambiguous/reused labels remain untouched and fail through the ordinary
	// ownership gate; no edge, direction, relation kind, or evidence is minted.
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		for j := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[j]
			from, to, ok := diagramUniqueVisibleAliasPair(doc, anchor.FromNode, anchor.ToNode)
			if !ok {
				continue
			}
			if anchor.FromNode != from {
				anchor.FromNode = from
				fixed++
			}
			if anchor.ToNode != to {
				anchor.ToNode = to
				fixed++
			}
		}
	}
	return fixed
}

// normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes restores only endpoint
// identity metadata that the model omitted while copying an exact authoring
// recipe. The model must already have authored the visible edge and an anchor
// with the same direction and relation kind. The fast path uses the recipe's
// stable node IDs directly. A second path permits business-facing node IDs only
// when the complete model-authored connected component has a unique
// relation-labelled graph mapping onto one typed recipe component. Ambiguity,
// a partial model identity pair, or any topology/relation mismatch stays
// fail-closed for the ordinary validator.
//
// This repair never reads visible labels and never creates an edge, anchor, or
// relation. Business-facing labels therefore cannot overwrite typed identity,
// while an unproved model-authored relation gains no authority.
func normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc *types.AnswerDocumentV2, recipes []types.DiagramEdgeAnchor) int {
	if doc == nil || len(recipes) == 0 {
		return 0
	}
	type pair struct{ from, to string }
	candidates := make(map[string]map[pair]bool)
	for _, recipe := range recipes {
		if !recipe.HasEndpointIdentityPair() || !recipe.RelationKind.IsValid() {
			continue
		}
		key := diagramEvidenceEdgeKey(recipe.FromNode, recipe.ToNode) + "\x00" + string(recipe.RelationKind)
		if candidates[key] == nil {
			candidates[key] = make(map[pair]bool)
		}
		candidates[key][pair{
			from: strings.TrimSpace(recipe.FromIdentity),
			to:   strings.TrimSpace(recipe.ToIdentity),
		}] = true
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		visible := make(map[string]bool)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			visible[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
		}
		for j := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[j]
			// A partial pair is structurally suspicious. Do not silently replace
			// model-authored identity metadata with a different typed pair.
			if strings.TrimSpace(anchor.FromIdentity) != "" || strings.TrimSpace(anchor.ToIdentity) != "" ||
				!anchor.RelationKind.IsValid() || !visible[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] {
				continue
			}
			key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode) + "\x00" + string(anchor.RelationKind)
			if len(candidates[key]) != 1 {
				continue
			}
			for candidate := range candidates[key] {
				anchor.FromIdentity = candidate.from
				anchor.ToIdentity = candidate.to
				fixed++
			}
		}
	}
	fixed += normalizeDiagramEdgeAnchorIdentitiesByUniqueTypedTopology(doc, recipes)
	return fixed
}

type diagramTypedRecipeEdge struct {
	from, to                 string
	fromIdentity, toIdentity string
	relation                 types.DiagramRelationKind
}

type diagramModelAnchorEdge struct {
	from, to string
	relation types.DiagramRelationKind
	anchor   *types.DiagramEdgeAnchor
}

type diagramRelationComponent struct {
	nodes     []string
	edgeIndex []int
}

type diagramIdentityPair struct{ from, to string }

// normalizeDiagramEdgeAnchorIdentitiesByUniqueTypedTopology is the alias-safe
// continuation of the exact-node fast path above. It reads only model-authored
// edge anchors plus parsed visible edge topology and the dispatch-scoped typed
// receipt. Visible labels, request prose, answer prose, citations, and source
// locations are deliberately absent from the decision. It therefore cannot
// turn display similarity into authority.
func normalizeDiagramEdgeAnchorIdentitiesByUniqueTypedTopology(doc *types.AnswerDocumentV2, recipes []types.DiagramEdgeAnchor) int {
	recipeEdges := diagramTypedRecipeEdges(recipes)
	if len(recipeEdges) == 0 {
		return 0
	}
	recipeComponents := diagramTypedRecipeComponents(recipeEdges)
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if block.Kind != types.BlockDiagram || block.Diagram == nil || len(block.EdgeAnchors) == 0 {
			continue
		}
		visible := make(map[string]bool)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			visible[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
		}
		modelEdges := make([]diagramModelAnchorEdge, 0, len(block.EdgeAnchors))
		for ai := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[ai]
			if !anchor.RelationKind.IsValid() || !visible[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] {
				continue
			}
			modelEdges = append(modelEdges, diagramModelAnchorEdge{
				from: strings.TrimSpace(anchor.FromNode), to: strings.TrimSpace(anchor.ToNode),
				relation: anchor.RelationKind, anchor: anchor,
			})
		}
		for _, component := range diagramModelAnchorComponents(modelEdges) {
			// One-sided identity is a model-authored conflict, not an omission.
			partial := false
			for _, edgeIndex := range component.edgeIndex {
				anchor := modelEdges[edgeIndex].anchor
				fromSet := strings.TrimSpace(anchor.FromIdentity) != ""
				toSet := strings.TrimSpace(anchor.ToIdentity) != ""
				if fromSet != toSet {
					partial = true
					break
				}
			}
			if partial {
				continue
			}
			candidates := make(map[int]map[diagramIdentityPair]bool)
			mappingCount := 0
			for _, recipeComponent := range recipeComponents {
				if len(component.nodes) != len(recipeComponent.nodes) || len(component.edgeIndex) != len(recipeComponent.edgeIndex) {
					continue
				}
				for _, mapping := range diagramComponentIsomorphisms(component, modelEdges, recipeComponent, recipeEdges) {
					mappingCount++
					for _, edgeIndex := range component.edgeIndex {
						modelEdge := modelEdges[edgeIndex]
						recipeEdge, ok := diagramUniqueMappedRecipeEdge(mapping[modelEdge.from], mapping[modelEdge.to], modelEdge.relation, recipeComponent, recipeEdges)
						if !ok {
							continue
						}
						if candidates[edgeIndex] == nil {
							candidates[edgeIndex] = make(map[diagramIdentityPair]bool)
						}
						candidates[edgeIndex][diagramIdentityPair{from: recipeEdge.fromIdentity, to: recipeEdge.toIdentity}] = true
					}
				}
			}
			if mappingCount == 0 {
				continue
			}
			for _, edgeIndex := range component.edgeIndex {
				anchor := modelEdges[edgeIndex].anchor
				if anchor.HasEndpointIdentityPair() || len(candidates[edgeIndex]) != 1 {
					continue
				}
				for pair := range candidates[edgeIndex] {
					anchor.FromIdentity = pair.from
					anchor.ToIdentity = pair.to
					fixed++
				}
			}
		}
	}
	return fixed
}

func diagramTypedRecipeEdges(recipes []types.DiagramEdgeAnchor) []diagramTypedRecipeEdge {
	out := make([]diagramTypedRecipeEdge, 0, len(recipes))
	seen := make(map[string]bool)
	for _, recipe := range recipes {
		if !recipe.HasEndpointIdentityPair() || !recipe.RelationKind.IsValid() {
			continue
		}
		edge := diagramTypedRecipeEdge{
			from: strings.TrimSpace(recipe.FromNode), to: strings.TrimSpace(recipe.ToNode),
			fromIdentity: strings.TrimSpace(recipe.FromIdentity), toIdentity: strings.TrimSpace(recipe.ToIdentity),
			relation: recipe.RelationKind,
		}
		key := edge.from + "\x00" + edge.to + "\x00" + string(edge.relation) + "\x00" + edge.fromIdentity + "\x00" + edge.toIdentity
		if edge.from == "" || edge.to == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, edge)
	}
	return out
}

func diagramTypedRecipeComponents(edges []diagramTypedRecipeEdge) []diagramRelationComponent {
	pairs := make([][2]string, len(edges))
	for i, edge := range edges {
		pairs[i] = [2]string{edge.from, edge.to}
	}
	return diagramRelationComponents(pairs)
}

func diagramModelAnchorComponents(edges []diagramModelAnchorEdge) []diagramRelationComponent {
	pairs := make([][2]string, len(edges))
	for i, edge := range edges {
		pairs[i] = [2]string{edge.from, edge.to}
	}
	return diagramRelationComponents(pairs)
}

func diagramRelationComponents(edges [][2]string) []diagramRelationComponent {
	var out []diagramRelationComponent
	seenNode := make(map[string]bool)
	for _, edge := range edges {
		for _, seed := range edge {
			if seed == "" || seenNode[seed] {
				continue
			}
			component := diagramRelationComponent{}
			queue := []string{seed}
			seenNode[seed] = true
			for len(queue) > 0 {
				node := queue[0]
				queue = queue[1:]
				component.nodes = append(component.nodes, node)
				for edgeIndex, pair := range edges {
					if pair[0] != node && pair[1] != node {
						continue
					}
					if !diagramIntSliceContains(component.edgeIndex, edgeIndex) {
						component.edgeIndex = append(component.edgeIndex, edgeIndex)
					}
					for _, next := range pair {
						if next != "" && !seenNode[next] {
							seenNode[next] = true
							queue = append(queue, next)
						}
					}
				}
			}
			out = append(out, component)
		}
	}
	return out
}

func diagramIntSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func diagramComponentIsomorphisms(model diagramRelationComponent, modelEdges []diagramModelAnchorEdge, recipe diagramRelationComponent, recipeEdges []diagramTypedRecipeEdge) []map[string]string {
	var out []map[string]string
	mapping := make(map[string]string, len(model.nodes))
	used := make(map[string]bool, len(recipe.nodes))
	var visit func(int)
	visit = func(at int) {
		if at == len(model.nodes) {
			if diagramMappedComponentMatches(model, modelEdges, recipe, recipeEdges, mapping) {
				copyMapping := make(map[string]string, len(mapping))
				for from, to := range mapping {
					copyMapping[from] = to
				}
				out = append(out, copyMapping)
			}
			return
		}
		modelNode := model.nodes[at]
		for _, recipeNode := range recipe.nodes {
			if used[recipeNode] || !diagramNodeRelationSignatureEqual(modelNode, model, modelEdges, recipeNode, recipe, recipeEdges) {
				continue
			}
			mapping[modelNode] = recipeNode
			used[recipeNode] = true
			visit(at + 1)
			delete(mapping, modelNode)
			delete(used, recipeNode)
		}
	}
	visit(0)
	return out
}

func diagramNodeRelationSignatureEqual(modelNode string, model diagramRelationComponent, modelEdges []diagramModelAnchorEdge, recipeNode string, recipe diagramRelationComponent, recipeEdges []diagramTypedRecipeEdge) bool {
	modelSignature := make(map[string]int)
	for _, index := range model.edgeIndex {
		edge := modelEdges[index]
		diagramAddRelationSignature(modelSignature, modelNode, edge.from, edge.to, edge.relation)
	}
	recipeSignature := make(map[string]int)
	for _, index := range recipe.edgeIndex {
		edge := recipeEdges[index]
		diagramAddRelationSignature(recipeSignature, recipeNode, edge.from, edge.to, edge.relation)
	}
	if len(modelSignature) != len(recipeSignature) {
		return false
	}
	for key, count := range modelSignature {
		if recipeSignature[key] != count {
			return false
		}
	}
	return true
}

func diagramAddRelationSignature(signature map[string]int, node, from, to string, relation types.DiagramRelationKind) {
	if from == node && to == node {
		signature["self\x00"+string(relation)]++
		return
	}
	if from == node {
		signature["out\x00"+string(relation)]++
	}
	if to == node {
		signature["in\x00"+string(relation)]++
	}
}

func diagramMappedComponentMatches(model diagramRelationComponent, modelEdges []diagramModelAnchorEdge, recipe diagramRelationComponent, recipeEdges []diagramTypedRecipeEdge, mapping map[string]string) bool {
	modelCounts := make(map[string]int)
	for _, index := range model.edgeIndex {
		edge := modelEdges[index]
		key := diagramEvidenceEdgeKey(mapping[edge.from], mapping[edge.to]) + "\x00" + string(edge.relation)
		modelCounts[key]++
		recipeEdge, ok := diagramUniqueMappedRecipeEdge(mapping[edge.from], mapping[edge.to], edge.relation, recipe, recipeEdges)
		if !ok {
			return false
		}
		if edge.anchor.HasEndpointIdentityPair() &&
			(strings.TrimSpace(edge.anchor.FromIdentity) != recipeEdge.fromIdentity || strings.TrimSpace(edge.anchor.ToIdentity) != recipeEdge.toIdentity) {
			return false
		}
	}
	recipeCounts := make(map[string]int)
	for _, index := range recipe.edgeIndex {
		edge := recipeEdges[index]
		key := diagramEvidenceEdgeKey(edge.from, edge.to) + "\x00" + string(edge.relation)
		recipeCounts[key]++
	}
	if len(modelCounts) != len(recipeCounts) {
		return false
	}
	for key, count := range modelCounts {
		if recipeCounts[key] != count {
			return false
		}
	}
	return true
}

func diagramUniqueMappedRecipeEdge(from, to string, relation types.DiagramRelationKind, component diagramRelationComponent, edges []diagramTypedRecipeEdge) (diagramTypedRecipeEdge, bool) {
	var found diagramTypedRecipeEdge
	matches := 0
	for _, index := range component.edgeIndex {
		edge := edges[index]
		if edge.from == from && edge.to == to && edge.relation == relation {
			found = edge
			matches++
		}
	}
	return found, matches == 1
}

func diagramUniqueVisibleAliasPair(doc *types.AnswerDocumentV2, rawFrom, rawTo string) (string, string, bool) {
	if doc == nil || strings.TrimSpace(rawFrom) == "" || strings.TrimSpace(rawTo) == "" {
		return "", "", false
	}
	var resolvedFrom, resolvedTo string
	matches := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		aliases := diagramNodeAliasIndex(block.Diagram.Body)
		from := aliases[diagramSurfaceKey(rawFrom)]
		to := aliases[diagramSurfaceKey(rawTo)]
		if from == "" || to == "" {
			continue
		}
		visible := false
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if diagramEvidenceEdgeKey(edge.From, edge.To) == diagramEvidenceEdgeKey(from, to) {
				visible = true
				break
			}
		}
		if !visible {
			continue
		}
		matches++
		if matches > 1 {
			return "", "", false
		}
		resolvedFrom, resolvedTo = from, to
	}
	return resolvedFrom, resolvedTo, matches == 1
}

func diagramNodeAliasIndex(body string) map[string]string {
	values := map[string]string{}
	ambiguous := map[string]bool{}
	add := func(surface, ident string) {
		key := diagramSurfaceKey(surface)
		ident = strings.TrimSpace(ident)
		if key == "" || ident == "" || ambiguous[key] {
			return
		}
		if existing := values[key]; existing != "" && existing != ident {
			delete(values, key)
			ambiguous[key] = true
			return
		}
		values[key] = ident
	}
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			add(decl.Ident, decl.Ident)
			add(decl.Label, decl.Ident)
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			add(decl.Ident, decl.Ident)
			add(decl.Label, decl.Ident)
		}
	}
	return values
}

func diagramSurfaceKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
