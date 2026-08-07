package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// explorerCallChainDirectCallFrontierLimit bounds advisory parser rows. The
// frontier is navigation context, not a completeness gate: a large orchestration
// function can contain many incidental calls, and the model still owns which
// calls are load-bearing for the user's requested explanation.
const explorerCallChainDirectCallFrontierLimit = 24

// explorerCallChainEndpointBoundaryFrontierLimit keeps exact-endpoint
// adjacency small.  It is only a navigation aid for reading a possible
// reverse/shared-frontier boundary; it is never evidence or a completion
// requirement.
const explorerCallChainEndpointBoundaryFrontierLimit = 8

type explorerCallChainDirectCallFrontierRow struct {
	Source       string
	Line         int
	Caller       string
	Callee       string
	TargetSource string
	Resolved     bool
}

type explorerCallChainEndpointBoundaryRow struct {
	Boundary explorerCallChainDirectCallFrontierRow
	Peer     *explorerCallChainDirectCallFrontierRow
	Kind     string
}

// renderExplorerCallChainDirectCallFrontier gives Explorer a bounded,
// parser-owned view of direct calls inside the uniquely resolved source
// endpoint. It addresses a context gap where the model read a very large
// source function but began its roster hundreds of lines after an early,
// load-bearing helper.
//
// This is deliberately SOFT:
//   - activation comes only from the typed source-code call-chain profile;
//   - rows come only from AST-grade parser relations inside one exact callable;
//   - the text says to inspect and select, and never mints EvidenceItem rows;
//   - ambiguity, missing bounds, multi-repo uncertainty, and runtime artifacts
//     stand down;
//   - no user/model prose or case-specific symbol is scanned.
//
// A frontier row is not proof. The model must still read the exact source line,
// decide whether the call matters, and emit its own grounded call evidence.
func renderExplorerCallChainDirectCallFrontier(ctx *types.AgentContext, graph *repomap.Graph) string {
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil ||
		types.RuntimeArtifactContextActiveFromAgent(ctx) {
		return ""
	}
	if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil && !mg.IsSingle() {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if types.ResolveQuestionFamily(rm) != types.QFCallChain ||
		rm.CallChainEndpointProfile == nil || !rm.CallChainEndpointProfile.Active() {
		return ""
	}
	source := strings.TrimSpace(rm.CallChainEndpointProfile.Source)
	fi, sourceDef, ok := explorerUniqueCallChainSourceDefinition(graph, source)
	if !ok {
		return ""
	}
	sink := strings.TrimSpace(rm.CallChainEndpointProfile.Sink)
	allRows := explorerCallChainASTDirectCallRows(graph, fi, sourceDef, source)
	rows, total := explorerCallChainDirectCallFrontierRowsFromAll(allRows, sink)
	if len(rows) == 0 {
		return ""
	}
	boundaryRows := explorerCallChainEndpointBoundaryRows(graph, allRows, source, sink)
	logging.Debug("[explorer] typed direct-call frontier source=%q sink=%q emitted=%d total=%d", source, sink, len(rows), total)

	var b strings.Builder
	b.WriteString("### Typed Direct-call Frontier (advisory)\n\n")
	fmt.Fprintf(&b, "The repository parser found the following direct-call candidates inside the uniquely resolved source endpoint `%s` at `%s:%d-%d`. This is a bounded navigation frontier, not answer evidence and not a required member list. Read the relevant lines, select only calls that are load-bearing for the requested path/order/mechanism, and emit grounded call-edge evidence for the calls you keep. Rows retain source-line order; sibling edges from the same caller are not concurrent or a callee-to-callee chain unless separate control-flow/concurrency evidence proves that relation. Do not claim every row is important, reachable to the requested sink, or part of one linear chain.\n\n",
		source, canonicalExplorerPath(sourceDef.File), sourceDef.Line, sourceDef.EndLine)
	b.WriteString("| Source line | Parser-owned candidate | Resolved current-repo target |\n")
	b.WriteString("|-------------|------------------------|------------------------------|\n")
	for _, row := range rows {
		target := "unresolved syntax surface; inspect the line"
		if row.Resolved {
			target = "`" + row.TargetSource + "`"
		}
		fmt.Fprintf(&b, "| `%s:%d` | `%s` -> `%s` | %s |\n",
			row.Source, row.Line, row.Caller, row.Callee, target)
	}
	if total > len(rows) {
		fmt.Fprintf(&b, "\nShown %d of %d parser-owned direct calls using a deterministic endpoint-relevant/first/middle/last sample; use the source body or a scoped `repo_map(view=\"relation_map\")` pass when another branch is relevant.\n", len(rows), total)
	}
	if len(boundaryRows) > 0 {
		b.WriteString("\n### Typed Endpoint-boundary Frontier (advisory)\n\n")
		fmt.Fprintf(&b, "The exact requested sink `%s` has the AST-authored adjacent calls below. They are shown only because they either point back to the exact source or share a direct callee with the source frontier. Read the exact boundary line before describing a wrapper, reverse edge, shared implementation, or `no_directed_path` result. These rows are navigation metadata, not answer evidence; do not reverse an arrow, equate endpoints, or omit a load-bearing boundary edge after you have read it.\n\n", sink)
		b.WriteString("| Exact sink boundary row | Why it may matter |\n")
		b.WriteString("|-------------------------|-------------------|\n")
		for _, item := range boundaryRows {
			why := "calls the exact source endpoint"
			if item.Kind == "shared_frontier" && item.Peer != nil {
				why = fmt.Sprintf("shares callee `%s` with `%s:%d` `%s` -> `%s`", item.Boundary.Callee, item.Peer.Source, item.Peer.Line, item.Peer.Caller, item.Peer.Callee)
			}
			fmt.Fprintf(&b, "| `%s:%d` `%s` -> `%s` | %s |\n",
				item.Boundary.Source, item.Boundary.Line, item.Boundary.Caller, item.Boundary.Callee, why)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func explorerUniqueCallChainSourceDefinition(graph *repomap.Graph, source string) (*repomap.FileInfo, *repomap.Symbol, bool) {
	source = strings.TrimSpace(source)
	if graph == nil || source == "" {
		return nil, nil, false
	}
	entities := map[string]string{strings.ToLower(source): source}
	type definition struct {
		fi  *repomap.FileInfo
		sym *repomap.Symbol
	}
	definitions := make(map[string]definition)
	forEachMatchingDef(entities, graph, func(_, _, _ string, sym *repomap.Symbol) bool {
		if sym == nil || (sym.Kind != "function" && sym.Kind != "method") || sym.Line <= 0 || sym.EndLine < sym.Line {
			return true
		}
		file := canonicalExplorerPath(sym.File)
		fi := graph.FileIndex[file]
		if fi == nil {
			fi = graph.FileIndex[sym.File]
		}
		if fi == nil {
			return true
		}
		definitions[fmt.Sprintf("%s:%d", file, sym.Line)] = definition{fi: fi, sym: sym}
		return true
	})
	if len(definitions) != 1 {
		return nil, nil, false
	}
	for _, def := range definitions {
		return def.fi, def.sym, true
	}
	return nil, nil, false
}

func explorerCallChainDirectCallFrontierRows(graph *repomap.Graph, fi *repomap.FileInfo, sourceDef *repomap.Symbol, source, sink string) ([]explorerCallChainDirectCallFrontierRow, int) {
	return explorerCallChainDirectCallFrontierRowsFromAll(explorerCallChainASTDirectCallRows(graph, fi, sourceDef, source), sink)
}

func explorerCallChainDirectCallFrontierRowsFromAll(rows []explorerCallChainDirectCallFrontierRow, sink string) ([]explorerCallChainDirectCallFrontierRow, int) {
	total := len(rows)
	return explorerSampleDirectCallFrontierRows(rows, explorerCallChainDirectCallFrontierLimit, sink), total
}

func explorerCallChainASTDirectCallRows(graph *repomap.Graph, fi *repomap.FileInfo, sourceDef *repomap.Symbol, source string) []explorerCallChainDirectCallFrontierRow {
	if graph == nil || fi == nil || sourceDef == nil || sourceDef.Line <= 0 || sourceDef.EndLine < sourceDef.Line {
		return nil
	}
	rows := make([]explorerCallChainDirectCallFrontierRow, 0)
	seen := make(map[string]bool)
	for i := range fi.Relations {
		rel := fi.Relations[i]
		if rel.Kind != "call" || rel.Line < sourceDef.Line || rel.Line > sourceDef.EndLine ||
			(rel.Provenance != repotypes.ProvenanceTreeSitter && rel.Provenance != repotypes.ProvenanceCangjieParser) {
			continue
		}
		enclosing := runtimeTargetEnclosingCallable(fi, rel.Line)
		if enclosing == nil || enclosing.Line != sourceDef.Line || enclosing.Name != sourceDef.Name {
			continue
		}
		callee := explorerCallChainRelationCalleeSurface(fi, rel, nil)
		if callee == "" {
			continue
		}
		row := explorerCallChainDirectCallFrontierRow{
			Source: canonicalExplorerPath(fi.RelPath), Line: rel.Line,
			Caller: source, Callee: callee,
		}
		if target := graph.ResolveCallTarget(fi, rel); target != nil {
			row.Resolved = true
			row.TargetSource = canonicalExplorerPath(target.File)
			row.Callee = explorerCallChainRelationCalleeSurface(fi, rel, target)
		}
		key := strings.ToLower(fmt.Sprintf("%s:%d:%s", row.Source, row.Line, row.Callee))
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Line != rows[j].Line {
			return rows[i].Line < rows[j].Line
		}
		return rows[i].Callee < rows[j].Callee
	})
	return rows
}

// explorerCallChainEndpointBoundaryRows identifies a narrow, parser-owned
// navigation seam around the exact sink.  Unlike the source frontier it does
// not publish arbitrary sink-body calls: a row must either call the exact
// source (a reverse edge) or share an exact direct callee with the source
// frontier.  This is a soft hint only.  Explorer must read the callsite before
// it can become typed evidence, at which point the existing read/parser
// handoff contract applies.
func explorerCallChainEndpointBoundaryRows(graph *repomap.Graph, sourceRows []explorerCallChainDirectCallFrontierRow, source, sink string) []explorerCallChainEndpointBoundaryRow {
	if graph == nil || len(sourceRows) == 0 || strings.TrimSpace(source) == "" || strings.TrimSpace(sink) == "" {
		return nil
	}
	for _, row := range sourceRows {
		if explorerEndpointSurfacesCompatible(row.Callee, sink) {
			return nil
		}
	}
	fi, sinkDef, ok := explorerUniqueCallChainSourceDefinition(graph, sink)
	if !ok {
		return nil
	}
	sinkRows := explorerCallChainASTDirectCallRows(graph, fi, sinkDef, sink)
	if len(sinkRows) == 0 {
		return nil
	}
	out := make([]explorerCallChainEndpointBoundaryRow, 0, explorerCallChainEndpointBoundaryFrontierLimit)
	seen := make(map[string]bool)
	for i := range sinkRows {
		boundary := sinkRows[i]
		item := explorerCallChainEndpointBoundaryRow{Boundary: boundary}
		if explorerEndpointSurfacesCompatible(boundary.Callee, source) {
			item.Kind = "reverse_endpoint"
		} else {
			for j := range sourceRows {
				if explorerEndpointSurfacesCompatible(boundary.Callee, sourceRows[j].Callee) {
					peer := sourceRows[j]
					item.Kind = "shared_frontier"
					item.Peer = &peer
					break
				}
			}
		}
		if item.Kind == "" {
			continue
		}
		key := strings.ToLower(fmt.Sprintf("%s:%d:%s:%s", boundary.Source, boundary.Line, boundary.Caller, boundary.Callee))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= explorerCallChainEndpointBoundaryFrontierLimit {
			break
		}
	}
	return out
}

func explorerEndpointSurfacesCompatible(left, right string) bool {
	left = explorerNormalizeEndpointSurface(left)
	right = explorerNormalizeEndpointSurface(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(left, right) || types.CallChainEndpointCompatible(left, right)
}

func explorerCallChainRelationCalleeSurface(fi *repomap.FileInfo, rel repomap.Relation, target *repomap.Symbol) string {
	name := strings.TrimSpace(rel.ToEP.Name)
	if target != nil && strings.TrimSpace(target.Name) != "" {
		name = strings.TrimSpace(target.Name)
	}
	if name == "" {
		return ""
	}
	owner := ""
	if target != nil {
		owner = strings.TrimSpace(target.Receiver)
		if owner == "" {
			owner = strings.TrimSpace(target.Parent)
		}
	}
	if owner == "" {
		owner = strings.TrimSpace(rel.ToEP.Receiver)
	}
	if owner == "" {
		return name
	}
	separator := "."
	if fi != nil {
		switch fi.Language {
		case repomap.LangRust, repomap.LangCpp, repomap.LangCangjie:
			separator = "::"
		}
	}
	if strings.HasSuffix(owner, ".") || strings.HasSuffix(owner, "::") || strings.HasSuffix(owner, "#") || strings.HasSuffix(owner, "->") {
		return owner + name
	}
	return owner + separator + name
}

// explorerSampleDirectCallFrontierRows preserves source order after selecting
// a deterministic first/middle/last sample. Resolved current-repo calls are
// selected first; unresolved syntax surfaces only fill spare capacity.
func explorerSampleDirectCallFrontierRows(rows []explorerCallChainDirectCallFrontierRow, limit int, sink string) []explorerCallChainDirectCallFrontierRow {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	if len(rows) <= limit {
		return append([]explorerCallChainDirectCallFrontierRow(nil), rows...)
	}
	resolved := make([]int, 0, len(rows))
	unresolved := make([]int, 0, len(rows))
	for i := range rows {
		if rows[i].Resolved {
			resolved = append(resolved, i)
		} else {
			unresolved = append(unresolved, i)
		}
	}
	selected := make(map[int]bool, limit)
	add := func(index int) {
		if index >= 0 && index < len(rows) && len(selected) < limit {
			selected[index] = true
		}
	}
	type relevantIndex struct {
		index int
		score int
	}
	relevant := make([]relevantIndex, 0)
	for i := range rows {
		if score := explorerEndpointVicinityScore(rows[i].Callee, sink); score > 0 {
			relevant = append(relevant, relevantIndex{index: i, score: score})
		}
	}
	sort.SliceStable(relevant, func(i, j int) bool {
		if relevant[i].score != relevant[j].score {
			return relevant[i].score > relevant[j].score
		}
		return relevant[i].index < relevant[j].index
	})
	for _, item := range relevant {
		add(item.index)
	}
	// Early helpers are the common omission, while tail calls carry the named
	// sink/boundary. Typed sink-vicinity rows above survive before this generic
	// sample; vicinity is navigation only and never mints endpoint equivalence.
	// Middle samples prevent a long source body from becoming a head/tail-only
	// view.
	for i := 0; i < len(resolved) && i < 10; i++ {
		add(resolved[i])
	}
	for i := len(resolved) - 1; i >= 0 && i >= len(resolved)-8; i-- {
		add(resolved[i])
	}
	for slot := 1; slot <= 6 && len(resolved) > 0; slot++ {
		add(resolved[(slot*(len(resolved)-1))/7])
	}
	for _, index := range resolved {
		add(index)
	}
	for _, index := range unresolved {
		add(index)
	}
	indices := make([]int, 0, len(selected))
	for index := range selected {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	out := make([]explorerCallChainDirectCallFrontierRow, 0, len(indices))
	for _, index := range indices {
		out = append(out, rows[index])
	}
	return out
}

func explorerEndpointVicinityScore(candidate, sink string) int {
	candidate = explorerNormalizeEndpointSurface(candidate)
	sink = explorerNormalizeEndpointSurface(sink)
	if candidate == "" || sink == "" {
		return 0
	}
	if strings.EqualFold(candidate, sink) {
		return 4
	}
	cOwner, cLeaf := explorerEndpointOwnerLeaf(candidate)
	sOwner, sLeaf := explorerEndpointOwnerLeaf(sink)
	if cLeaf == "" || sLeaf == "" {
		return 0
	}
	if cOwner != "" && strings.EqualFold(cOwner, sOwner) {
		if strings.EqualFold(cLeaf, sLeaf) {
			return 4
		}
		cLower, sLower := strings.ToLower(cLeaf), strings.ToLower(sLeaf)
		if strings.HasPrefix(cLower, sLower) || strings.HasPrefix(sLower, cLower) {
			return 3
		}
	}
	if strings.EqualFold(cLeaf, sLeaf) {
		return 2
	}
	return 0
}

func explorerNormalizeEndpointSurface(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	for _, separator := range []string{"::", "->", "#"} {
		endpoint = strings.ReplaceAll(endpoint, separator, ".")
	}
	return strings.Trim(endpoint, ".")
}

func explorerEndpointOwnerLeaf(endpoint string) (string, string) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", ""
	}
	if at := strings.LastIndex(endpoint, "."); at >= 0 {
		return strings.TrimSpace(endpoint[:at]), strings.TrimSpace(endpoint[at+1:])
	}
	return "", endpoint
}
