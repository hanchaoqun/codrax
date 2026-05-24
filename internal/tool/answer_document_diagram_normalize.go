package tool

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

type diagramSymbolSourceKey struct {
	symbol string
	source string
}

type diagramDefinitionTarget struct {
	source string
	line   int
}

type diagramDefinitionIndex struct {
	definitions    map[string]diagramDefinitionTarget
	ambiguous      map[string]bool
	nonDefinitions map[diagramSymbolSourceKey]bool
}

func normalizeDiagramDefinitionLabelsByEvidence(doc *types.AnswerDocumentV2, pctx *preEmitCheckContext) int {
	if doc == nil || pctx == nil {
		return 0
	}
	index := buildDiagramDefinitionIndex(pctx)
	if len(index.definitions) == 0 {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		replacements := diagramDefinitionLabelReplacements(block.Diagram.Body, index, pctx)
		if len(replacements) == 0 {
			continue
		}
		body := block.Diagram.Body
		for oldLabel, newLabel := range replacements {
			if oldLabel == newLabel {
				continue
			}
			var changed int
			body, changed = replaceMermaidVisibleLabel(body, oldLabel, newLabel)
			fixed += changed
		}
		block.Diagram.Body = body
	}
	return fixed
}

func buildDiagramDefinitionIndex(pctx *preEmitCheckContext) diagramDefinitionIndex {
	index := diagramDefinitionIndex{
		definitions:    make(map[string]diagramDefinitionTarget),
		ambiguous:      make(map[string]bool),
		nonDefinitions: make(map[diagramSymbolSourceKey]bool),
	}
	for _, ev := range pctx.evidenceItems() {
		if ev.GroundingStatus == types.GroundingUngrounded || strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 {
			continue
		}
		source := pctx.canonicalPath(ev.Source)
		if source == "" {
			continue
		}
		keys := diagramEvidenceSymbolKeys(ev)
		if len(keys) == 0 {
			continue
		}
		if ev.AnchorKind == types.AnchorDefinition {
			target := diagramDefinitionTarget{source: source, line: ev.LineStart}
			for _, key := range keys {
				addDiagramDefinitionTarget(index, key, target)
			}
			continue
		}
		for _, key := range keys {
			index.nonDefinitions[diagramSymbolSourceKey{symbol: key, source: source}] = true
		}
	}
	return index
}

func addDiagramDefinitionTarget(index diagramDefinitionIndex, key string, target diagramDefinitionTarget) {
	key = strings.TrimSpace(key)
	if key == "" || target.source == "" || target.line <= 0 || index.ambiguous[key] {
		return
	}
	if existing, ok := index.definitions[key]; ok {
		if existing.source != target.source || existing.line != target.line {
			delete(index.definitions, key)
			index.ambiguous[key] = true
		}
		return
	}
	index.definitions[key] = target
}

func diagramDefinitionLabelReplacements(body string, index diagramDefinitionIndex, pctx *preEmitCheckContext) map[string]string {
	replacements := map[string]string{}
	visitLabel := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" {
			return
		}
		if _, seen := replacements[label]; seen {
			return
		}
		if replacement, ok := diagramDefinitionReplacementForLabel(label, index, pctx); ok {
			replacements[label] = replacement
		}
	}
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			visitLabel(decl.Label)
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			visitLabel(decl.Label)
		}
	}
	return replacements
}

func diagramDefinitionReplacementForLabel(label string, index diagramDefinitionIndex, pctx *preEmitCheckContext) (string, bool) {
	symbol, source, hadLine, ok := splitDiagramSymbolPathLabel(label, pctx)
	if !ok {
		return "", false
	}
	for _, key := range diagramSymbolKeyVariants(symbol) {
		if index.ambiguous[key] {
			continue
		}
		target, found := index.definitions[key]
		if !found || target.source == "" || target.line <= 0 {
			continue
		}
		if target.source == source {
			if hadLine {
				return "", false
			}
			return fmt.Sprintf("%s @ %s:%d", symbol, target.source, target.line), true
		}
		if index.nonDefinitions[diagramSymbolSourceKey{symbol: key, source: source}] {
			return fmt.Sprintf("%s @ %s:%d", symbol, target.source, target.line), true
		}
	}
	return "", false
}

func splitDiagramSymbolPathLabel(label string, pctx *preEmitCheckContext) (symbol, source string, hadLine bool, ok bool) {
	parts := strings.Split(label, " @ ")
	if len(parts) != 2 {
		return "", "", false, false
	}
	symbol = strings.TrimSpace(parts[0])
	rawSource := strings.TrimSpace(parts[1])
	if !diagramSymbolLabelLooksAtomic(symbol) || rawSource == "" {
		return "", "", false, false
	}
	rawSource, hadLine = stripDiagramLineSuffix(rawSource)
	if rawSource == "" || (!strings.Contains(rawSource, "/") && filepath.Ext(rawSource) == "") {
		return "", "", false, false
	}
	if pctx != nil {
		source = pctx.canonicalPath(rawSource)
	} else {
		source = strings.TrimSpace(strings.ReplaceAll(rawSource, `\`, `/`))
	}
	return symbol, source, hadLine, source != ""
}

func stripDiagramLineSuffix(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	idx := strings.LastIndex(raw, ":")
	if idx <= 0 || idx >= len(raw)-1 {
		return raw, false
	}
	suffix := raw[idx+1:]
	if dash := strings.Index(suffix, "-"); dash >= 0 {
		suffix = suffix[:dash]
	}
	if suffix == "" {
		return raw, false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return raw, false
		}
	}
	return strings.TrimSpace(raw[:idx]), true
}

func diagramSymbolLabelLooksAtomic(symbol string) bool {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" || strings.Contains(symbol, "/") {
		return false
	}
	hasLetter := false
	for _, r := range symbol {
		if unicode.IsSpace(r) {
			return false
		}
		if unicode.IsLetter(r) || r == '_' || r == '@' {
			hasLetter = true
		}
	}
	return hasLetter
}

func diagramEvidenceSymbolKeys(ev types.EvidenceItem) []string {
	var keys []string
	seen := map[string]bool{}
	add := func(raw string) {
		for _, key := range diagramSymbolKeyVariants(raw) {
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	add(ev.AnchorSymbol)
	add(ev.Subject)
	add(ev.Object)
	if ev.OwnerSymbol != "" && ev.AnchorSymbol != "" {
		add(ev.OwnerSymbol + "." + ev.AnchorSymbol)
	}
	for _, term := range ev.SurfaceTerms {
		add(term)
	}
	return keys
}

func diagramSymbolKeyVariants(raw string) []string {
	base := diagramSymbolKey(raw)
	if base == "" {
		return nil
	}
	out := []string{base}
	seen := map[string]bool{base: true}
	for _, sep := range []string{"::", "->", ".", "#"} {
		if idx := strings.LastIndex(base, sep); idx > 0 && idx+len(sep) < len(base) {
			tail := strings.TrimSpace(base[idx+len(sep):])
			if tail != "" && !seen[tail] {
				seen[tail] = true
				out = append(out, tail)
			}
		}
	}
	return out
}

func diagramSymbolKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`\"'")
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "("); idx > 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	raw = strings.Trim(raw, " \t\r\n:;,")
	if raw == "" {
		return ""
	}
	return strings.ToLower(raw)
}

func replaceMermaidVisibleLabel(body, oldLabel, newLabel string) (string, int) {
	if oldLabel == "" || newLabel == "" || oldLabel == newLabel {
		return body, 0
	}
	quotedOld := strconv.Quote(oldLabel)
	quotedNew := strconv.Quote(newLabel)
	replacements := []struct {
		old string
		new string
	}{
		{quotedOld, quotedNew},
		{"'" + oldLabel + "'", quotedNew},
		{"[" + oldLabel + "]", "[" + quotedNew + "]"},
		{"(" + oldLabel + ")", "(" + quotedNew + ")"},
		{"{" + oldLabel + "}", "{" + quotedNew + "}"},
		{">" + oldLabel + "]", ">" + quotedNew + "]"},
		{" as " + oldLabel, " as " + quotedNew},
	}
	total := 0
	for _, repl := range replacements {
		if repl.old == "" || repl.old == repl.new {
			continue
		}
		count := strings.Count(body, repl.old)
		if count == 0 {
			continue
		}
		body = strings.ReplaceAll(body, repl.old, repl.new)
		total += count
	}
	return body, total
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
		existing := diagramEdgeAnchorKeySet(block.EdgeAnchors)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			rel := types.InferRelationFromLabel(edge.Label)
			if rel == types.DiagramRelUnknown {
				continue
			}
			key := diagramEdgeAnchorKey(edge.From, edge.To, rel)
			if existing[key] {
				continue
			}
			block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{
				FromNode:     edge.From,
				ToNode:       edge.To,
				RelationKind: rel,
				ClaimForm:    types.ClaimFormForRelation(rel),
			})
			existing[key] = true
			fixed++
		}
	}
	return fixed
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

func diagramEdgeAnchorKeySet(anchors []types.DiagramEdgeAnchor) map[string]bool {
	out := make(map[string]bool, len(anchors))
	for _, anchor := range anchors {
		rel := anchor.RelationKind
		if !rel.IsValid() {
			rel = types.RelationForClaimForm(anchor.ClaimForm)
		}
		if rel == types.DiagramRelUnknown {
			continue
		}
		out[diagramEdgeAnchorKey(anchor.FromNode, anchor.ToNode, rel)] = true
	}
	return out
}

func diagramEdgeAnchorKey(from, to string, rel types.DiagramRelationKind) string {
	return strings.ToLower(strings.TrimSpace(from)) + "\x00" +
		strings.ToLower(strings.TrimSpace(to)) + "\x00" +
		string(rel)
}
