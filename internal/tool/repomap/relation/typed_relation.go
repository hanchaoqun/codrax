package relation

import (
	"path"
	"sort"
	"strings"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TypedRelationCandidates adapts a repomap graph into the shared typed
// relation provider contract. It consumes only graph-resolved structural data:
// symbol implements edges and file-to-file import edges. It does not inspect
// raw user prose, model prose, or source text.
func TypedRelationCandidates(g *rmtypes.Graph, q types.TypedRelationQuery) []types.TypedRelationCandidate {
	if g == nil || len(q.Sources) == 0 {
		return nil
	}
	var out []types.TypedRelationCandidate
	if q.AllowsKind(types.TypedRelationImplements) {
		out = append(out, implementerCandidates(g, q)...)
	}
	if q.AllowsKind(types.TypedRelationImports) {
		out = append(out, importEdgeCandidates(g, q, types.TypedRelationImports)...)
	}
	if q.AllowsKind(types.TypedRelationExports) {
		out = append(out, importEdgeCandidates(g, q, types.TypedRelationExports)...)
	}
	if len(out) == 0 {
		return nil
	}
	sortTypedRelationCandidates(out)
	if q.MaxMembers > 0 && len(out) > q.MaxMembers {
		out = out[:q.MaxMembers]
	}
	return out
}

func implementerCandidates(g *rmtypes.Graph, q types.TypedRelationQuery) []types.TypedRelationCandidate {
	var out []types.TypedRelationCandidate
	seen := map[string]bool{}
	for _, source := range q.Sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		sourceKind := relationSourceSymbolKind(g, source)
		for _, id := range g.ImplementersOf(source) {
			sym, ok := g.SymbolByID[id]
			if !ok || sym == nil || strings.TrimSpace(sym.Name) == "" {
				continue
			}
			file := cleanRelationPath(sym.File)
			if file == "" {
				continue
			}
			key := strings.ToLower(source) + "|implements|" + strings.ToLower(sym.Name) + "|" + file
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, types.TypedRelationCandidate{
				Relation:   types.TypedRelationImplements,
				SourceName: source,
				SourceKind: sourceKind,
				SourceFile: cleanRelationPath(relationSourceSymbolFile(g, source)),
				SourceLine: relationSourceSymbolLine(g, source),
				Member: types.TypedRelationMember{
					Name:     strings.TrimSpace(sym.Name),
					File:     file,
					Line:     sym.Line,
					Kind:     strings.TrimSpace(sym.Kind),
					Distance: 1,
				},
				Carrier:   types.TypedRelationCarrierGraph,
				Precision: types.TypedRelationPrecisionExactSymbolID,
			})
		}
	}
	return out
}

func importEdgeCandidates(g *rmtypes.Graph, q types.TypedRelationQuery, kind types.TypedRelationKind) []types.TypedRelationCandidate {
	var out []types.TypedRelationCandidate
	seen := map[string]bool{}
	for _, source := range q.Sources {
		for _, ref := range relationSourceFileRefs(g, source, q.Purpose) {
			if q.Purpose == types.TypedRelationPurposeCoverageGate && !ref.precision.CoverageGateEligible() {
				continue
			}
			var members []string
			switch kind {
			case types.TypedRelationImports:
				members = g.ImportGraph[ref.file]
			case types.TypedRelationExports:
				members = g.ReverseImports[ref.file]
			default:
				continue
			}
			for _, memberFile := range members {
				memberFile = cleanRelationPath(memberFile)
				if memberFile == "" {
					continue
				}
				observedFile := ref.file
				if kind == types.TypedRelationExports {
					// Reverse import evidence is observed in the importing file.
					observedFile = memberFile
				}
				key := strings.ToLower(string(kind)) + "|" + strings.ToLower(ref.display) + "|" + memberFile
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, types.TypedRelationCandidate{
					Relation:   kind,
					SourceName: ref.display,
					SourceKind: ref.kind,
					SourceFile: observedFile,
					Member: types.TypedRelationMember{
						Name:     memberFile,
						File:     memberFile,
						Line:     relationFileLine(g, memberFile),
						Kind:     "file",
						Distance: 1,
					},
					Carrier:   types.TypedRelationCarrierGraph,
					Precision: ref.precision,
				})
			}
		}
	}
	return out
}

type relationSourceFileRef struct {
	display   string
	file      string
	kind      string
	precision types.TypedRelationPrecision
}

func relationSourceFileRefs(g *rmtypes.Graph, raw string, purpose types.TypedRelationPurpose) []relationSourceFileRef {
	if g == nil {
		return nil
	}
	source := cleanRelationPath(raw)
	if source == "" {
		return nil
	}
	var out []relationSourceFileRef
	seen := map[string]bool{}
	add := func(display, file, kind string, precision types.TypedRelationPrecision) {
		file = cleanRelationPath(file)
		display = strings.TrimSpace(display)
		if display == "" || file == "" {
			return
		}
		if _, ok := g.FileIndex[file]; !ok {
			return
		}
		key := strings.ToLower(display) + "|" + file + "|" + string(precision)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, relationSourceFileRef{display: display, file: file, kind: kind, precision: precision})
	}
	if _, ok := g.FileIndex[source]; ok {
		add(source, source, "file", types.TypedRelationPrecisionExactFile)
		return out
	}
	if purpose != types.TypedRelationPurposePromptHint {
		return nil
	}
	prefix := source
	if prefix != "" && prefix != "." && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if prefix != "" && prefix != "./" {
		for _, fi := range g.Files {
			if fi == nil {
				continue
			}
			file := cleanRelationPath(fi.RelPath)
			if strings.HasPrefix(file, prefix) {
				add(source, file, "directory", types.TypedRelationPrecisionNameOnly)
			}
		}
	}
	for _, fi := range g.Files {
		if fi == nil || strings.TrimSpace(fi.Package) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fi.Package), source) {
			add(source, fi.RelPath, "package", types.TypedRelationPrecisionNameOnly)
		}
	}
	return out
}

func relationSourceSymbolKind(g *rmtypes.Graph, name string) string {
	if def := firstRelationSourceSymbol(g, name); def != nil {
		return strings.TrimSpace(def.Kind)
	}
	return ""
}

func relationSourceSymbolFile(g *rmtypes.Graph, name string) string {
	if def := firstRelationSourceSymbol(g, name); def != nil {
		return def.File
	}
	return ""
}

func relationSourceSymbolLine(g *rmtypes.Graph, name string) int {
	if def := firstRelationSourceSymbol(g, name); def != nil {
		return def.Line
	}
	return 0
}

func firstRelationSourceSymbol(g *rmtypes.Graph, name string) *rmtypes.Symbol {
	if g == nil {
		return nil
	}
	defs := g.SymbolDefs[name]
	if len(defs) == 0 {
		for candidate, values := range g.SymbolDefs {
			if strings.EqualFold(candidate, name) {
				defs = values
				break
			}
		}
	}
	for _, def := range defs {
		if def != nil && strings.TrimSpace(def.Name) != "" {
			return def
		}
	}
	return nil
}

func relationFileLine(g *rmtypes.Graph, file string) int {
	if g == nil {
		return 0
	}
	fi, ok := g.FileIndex[cleanRelationPath(file)]
	if !ok || fi == nil {
		return 0
	}
	line := 0
	for _, sym := range fi.Symbols {
		if sym.Line <= 0 {
			continue
		}
		if line == 0 || sym.Line < line {
			line = sym.Line
		}
	}
	if line > 0 {
		return line
	}
	return 1
}

func cleanRelationPath(raw string) string {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`)
	raw = strings.TrimPrefix(raw, "./")
	if raw == "" || raw == "." || raw == "/" {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func sortTypedRelationCandidates(rows []types.TypedRelationCandidate) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if a.SourceName != b.SourceName {
			return a.SourceName < b.SourceName
		}
		if a.Member.Name != b.Member.Name {
			return a.Member.Name < b.Member.Name
		}
		return a.Member.File < b.Member.File
	})
}
