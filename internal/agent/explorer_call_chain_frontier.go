package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// explorerCallChainDirectCallFrontierLimit bounds advisory parser rows. The
// frontier is navigation context, not a completeness gate: a large orchestration
// function can contain many incidental calls, and the model still owns which
// calls are load-bearing for the user's requested explanation.
const explorerCallChainDirectCallFrontierLimit = 24

type explorerCallChainDirectCallFrontierRow struct {
	Source       string
	Line         int
	Caller       string
	Callee       string
	TargetSource string
	Resolved     bool
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
	rows, total := explorerCallChainDirectCallFrontierRows(graph, fi, sourceDef, source)
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Typed Direct-call Frontier (advisory)\n\n")
	fmt.Fprintf(&b, "The repository parser found the following direct-call candidates inside the uniquely resolved source endpoint `%s` at `%s:%d-%d`. This is a bounded navigation frontier, not answer evidence and not a required member list. Read the relevant lines, select only calls that are load-bearing for the requested path/order/mechanism, and emit grounded call-edge evidence for the calls you keep. Do not claim every row is important, reachable to the requested sink, or part of one linear chain.\n\n",
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
		fmt.Fprintf(&b, "\nShown %d of %d parser-owned direct calls using a deterministic first/middle/last sample; use the source body or a scoped `repo_map(view=\"relation_map\")` pass when another branch is relevant.\n", len(rows), total)
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

func explorerCallChainDirectCallFrontierRows(graph *repomap.Graph, fi *repomap.FileInfo, sourceDef *repomap.Symbol, source string) ([]explorerCallChainDirectCallFrontierRow, int) {
	if graph == nil || fi == nil || sourceDef == nil || sourceDef.Line <= 0 || sourceDef.EndLine < sourceDef.Line {
		return nil, 0
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
	total := len(rows)
	return explorerSampleDirectCallFrontierRows(rows, explorerCallChainDirectCallFrontierLimit), total
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
func explorerSampleDirectCallFrontierRows(rows []explorerCallChainDirectCallFrontierRow, limit int) []explorerCallChainDirectCallFrontierRow {
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
	// Early helpers are the common omission, while tail calls carry the named
	// sink/boundary. Middle samples prevent a long source body from becoming a
	// head/tail-only view.
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
