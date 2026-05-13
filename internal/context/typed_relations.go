package context

import (
	"sort"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzerGraphFromBus extracts the typed repomap.Graph from a
// BusContext via Mutable.SearchGraph(). Returns nil when Mutable is
// nil or no graph has been seeded yet (analyzer-only paths, sub-
// agent paths that intentionally skip the graph). Callers MUST
// nil-check; the probe gracefully returns no hints.
func analyzerGraphFromBus(bus *types.BusContext) any {
	if bus == nil || bus.Mutable == nil {
		return nil
	}
	return bus.Mutable.SearchGraph()
}

// typedRelationProbeMaxMembers caps the per-relation member count
// surfaced in TypedRelationHint to prevent prompt bloat on huge
// relations (massive interfaces / large packages). 50 covers all
// realistic enumeration cases observed in eval.
const typedRelationProbeMaxMembers = 50

// ProbeTypedRelations is the entry point that builds the
// TypedRelationHint slice for an AgentContext. Returns nil when:
//   - graph is nil (no repomap available — analyzer-only paths)
//   - rm is nil (no analyzer signal yet)
//   - the analyzer's predicates do NOT indicate a structural
//     enumeration / relational lookup question (Intent=trace and
//     free-form explanation paths skip this entirely; their answer
//     shape is sequence / prose, not a typed set)
//
// Probe table (priority order — first non-empty wins per entity):
//
//	implements / overrides → Graph.ImplementersOf
//	extends                → walk Relation{Kind:"inheritance"}
//	called-by / references → Graph.CallersOfID + reference walk
//	scoped-to              → walk FileInfo.Package / FileInfo.RelPath
//	registers              → walk Symbol.AnchorKind=Assignment + name
//
// This first commit ships ONLY the implements probe (covers the s5a
// regression class). The other relation tags are reserved in
// AllTypedRelations() so their AnchorKind mappings + structural
// tests are in place; their probe rows can be added incrementally
// without touching the channel or the render path.
func ProbeTypedRelations(graph any, rm *types.RequestModel) []types.TypedRelationHint {
	if rm == nil {
		return nil
	}
	if !shouldProbeTypedRelations(rm) {
		return nil
	}
	g, ok := graph.(*repotypes.Graph)
	if !ok || g == nil {
		return nil
	}
	candidates := candidateEntityNames(rm)
	if len(candidates) == 0 {
		return nil
	}
	var hints []types.TypedRelationHint
	seenSource := make(map[string]bool)
	for _, name := range candidates {
		if seenSource[name] {
			continue
		}
		seenSource[name] = true
		// implements probe: typed Symbol.Implements relation.
		if hint := probeImplements(g, name); hint != nil {
			hints = append(hints, *hint)
		}
	}
	return hints
}

// shouldProbeTypedRelations gates the probe on the analyzer's typed
// predicates. A typed-relation hint helps three families of
// question:
//
//	IsCategoryEnumeration  — "list all X of Y"
//	IsRelationalLookup     — "which X have Y"
//	IsCountQuestion        — "how many X" when X is a typed set
//
// For free-form explanation / trace / measurement-scalar questions
// the answer set isn't a structural relation, so the hint would be
// noise. We use the analyzer's typed predicates directly (no
// keyword tables — see feedback_no_custom_keyword_matching).
func shouldProbeTypedRelations(rm *types.RequestModel) bool {
	preds := rm.Predicates
	if preds.IsCategoryEnumeration {
		return true
	}
	if preds.IsRelationalLookup {
		return true
	}
	if preds.IsCountQuestion {
		return true
	}
	return false
}

// candidateEntityNames returns the narrow, provenance-carrying entity
// tokens the probe should attempt to resolve. Broad analyzer Entities
// can include repo-map expansion/context helpers; those remain search
// hints and must not seed typed_graph relation gates unless no newer
// provenance lane exists.
func candidateEntityNames(rm *types.RequestModel) []string {
	return types.StructuralRelationScopeCandidates(*rm)
}

// probeImplements walks Graph.ImplementersOf for one entity name.
// Returns nil when the entity does not name an interface / trait /
// protocol OR no concrete type implements it.
func probeImplements(g *repotypes.Graph, name string) *types.TypedRelationHint {
	if g == nil || name == "" {
		return nil
	}
	defs, ok := g.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		return nil
	}
	var sourceKind string
	for _, d := range defs {
		if d == nil {
			continue
		}
		switch d.Kind {
		case "interface", "trait", "protocol":
			sourceKind = d.Kind
		}
		if sourceKind != "" {
			break
		}
	}
	if sourceKind == "" {
		return nil
	}
	ids := g.ImplementersOf(name)
	if len(ids) == 0 {
		return nil
	}
	hint := types.TypedRelationHint{
		Relation:   "implements",
		SourceName: name,
		SourceKind: sourceKind,
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		sym, ok := g.SymbolByID[id]
		if !ok || sym == nil || sym.Name == "" {
			continue
		}
		key := sym.Name + "|" + sym.File
		if seen[key] {
			continue
		}
		seen[key] = true
		hint.Members = append(hint.Members, types.TypedRelationMember{
			Name:     sym.Name,
			File:     sym.File,
			Line:     sym.Line,
			Kind:     sym.Kind,
			Distance: 1,
		})
	}
	if len(hint.Members) == 0 {
		return nil
	}
	sort.SliceStable(hint.Members, func(i, j int) bool {
		return hint.Members[i].Name < hint.Members[j].Name
	})
	if len(hint.Members) > typedRelationProbeMaxMembers {
		hint.Members = hint.Members[:typedRelationProbeMaxMembers]
	}
	return &hint
}
