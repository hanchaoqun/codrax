package context

import (
	"reflect"
	"sort"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// typedRelationCarriersFromBus returns the authoritative carrier sequence
// for typed relation probes.
//
// Historical note: the codebase has two graph-shaped fields:
// Mutable.SearchGraph() is the legacy single-repo graph cache, while
// BusContext.MultiGraph is the newer multi-repo carrier that is also
// used for single-repo runs when multi_repo is enabled. Relation probes
// must not read only one of them directly; otherwise single-repo evals
// running through MultiGraph lose typed_graph hints, while older tests
// that seed only Mutable.SearchGraph lose coverage. This helper encodes
// the precedence in one place: try generic relation providers first,
// then fall back to the legacy graph. Callers merge/dedup the outputs
// instead of assuming the first carrier is always complete; MultiGraph
// can legitimately return no active rows in early prompt assembly.
func typedRelationCarriersFromBus(bus *types.BusContext) []any {
	if bus == nil {
		return nil
	}
	var out []any
	add := func(carrier any) {
		if carrier == nil {
			return
		}
		if typedRelationCarrierAlreadyPresent(out, carrier) {
			return
		}
		out = append(out, carrier)
	}
	multiGraphAdded := false
	if provider, ok := bus.MultiGraph.(types.TypedRelationCandidateSource); ok && provider != nil {
		add(provider)
		multiGraphAdded = true
	}
	if bus.Mutable != nil {
		if graph := bus.Mutable.SearchGraph(); graph != nil {
			add(graph)
		}
	}
	if !multiGraphAdded {
		add(bus.MultiGraph)
	}
	return out
}

// typedRelationCarrierFromBus returns the first carrier for legacy callers.
// New code that needs robust prompt hints should use
// typedRelationCarriersFromBus and merge all carriers.
func typedRelationCarrierFromBus(bus *types.BusContext) any {
	carriers := typedRelationCarriersFromBus(bus)
	if len(carriers) == 0 {
		return nil
	}
	return carriers[0]
}

func typedRelationCarrierAlreadyPresent(existing []any, candidate any) bool {
	if candidate == nil {
		return true
	}
	candidateType := reflect.TypeOf(candidate)
	if candidateType == nil || !candidateType.Comparable() {
		return false
	}
	for _, item := range existing {
		if item == nil || reflect.TypeOf(item) != candidateType {
			continue
		}
		if item == candidate {
			return true
		}
	}
	return false
}

// analyzerGraphFromBus is kept as a compatibility wrapper for older
// call sites/tests. New typed relation code should call
// typedRelationCarrierFromBus so the MultiGraph/SearchGraph precedence
// stays centralized.
func analyzerGraphFromBus(bus *types.BusContext) any {
	return typedRelationCarrierFromBus(bus)
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
//   - the analyzer's typed fields do NOT indicate a structural
//     relation surface. This is broader than enumeration: diagrams,
//     architecture explanations, comparisons, and mechanism answers
//     can all need the same typed relation facts when the analyzer
//     has emitted a relation axis such as predicate_axis=implement.
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
	if graph == nil {
		return nil
	}
	candidates := candidateEntityNames(rm)
	if len(candidates) == 0 {
		return nil
	}
	if hints := probeTypedRelationCandidateSource(graph, candidates); len(hints) > 0 {
		return hints
	}
	var hints []types.TypedRelationHint
	seenSource := make(map[string]bool)
	for _, name := range candidates {
		if seenSource[name] {
			continue
		}
		seenSource[name] = true
		// implements probe: typed Symbol.Implements relation.
		if hint := probeImplements(graph, name); hint != nil {
			hints = append(hints, *hint)
		}
	}
	return hints
}

func appendTypedRelationHints(dst []types.TypedRelationHint, src ...types.TypedRelationHint) []types.TypedRelationHint {
	if len(src) == 0 {
		return dst
	}
	seen := map[string]bool{}
	for _, hint := range dst {
		for _, member := range hint.Members {
			seen[typedRelationHintMemberKey(hint, member)] = true
		}
	}
	for _, hint := range src {
		if hint.Relation == "" || strings.TrimSpace(hint.SourceName) == "" || len(hint.Members) == 0 {
			continue
		}
		var members []types.TypedRelationMember
		for _, member := range hint.Members {
			key := typedRelationHintMemberKey(hint, member)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			members = append(members, member)
		}
		if len(members) == 0 {
			continue
		}
		hint.Members = members
		dst = append(dst, hint)
	}
	return dst
}

func typedRelationHintMemberKey(hint types.TypedRelationHint, member types.TypedRelationMember) string {
	if hint.Relation == "" || strings.TrimSpace(hint.SourceName) == "" || strings.TrimSpace(member.Name) == "" {
		return ""
	}
	return strings.ToLower(string(hint.Relation)) + "|" +
		strings.ToLower(strings.TrimSpace(hint.SourceName)) + "|" +
		strings.ToLower(strings.TrimSpace(member.Name)) + "|" +
		strings.TrimSpace(member.File)
}

func probeTypedRelationCandidateSource(graph any, candidates []string) []types.TypedRelationHint {
	provider, ok := graph.(types.TypedRelationCandidateSource)
	if !ok || provider == nil || len(candidates) == 0 {
		return nil
	}
	rows := provider.TypedRelationCandidates(types.TypedRelationQuery{
		Kinds:      []types.TypedRelationKind{types.TypedRelationImplements},
		Sources:    candidates,
		MaxMembers: typedRelationProbeMaxMembers,
		Purpose:    types.TypedRelationPurposePromptHint,
	})
	if len(rows) == 0 {
		return nil
	}
	type groupKey struct {
		relation types.TypedRelationKind
		source   string
	}
	groups := make(map[groupKey][]types.TypedRelationMember)
	sourceKind := make(map[groupKey]string)
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.Relation == "" || strings.TrimSpace(row.SourceName) == "" || strings.TrimSpace(row.Member.Name) == "" {
			continue
		}
		key := groupKey{relation: row.Relation, source: strings.TrimSpace(row.SourceName)}
		member := row.Member
		member.Name = strings.TrimSpace(member.Name)
		member.File = strings.TrimSpace(member.File)
		dedupKey := string(key.relation) + "|" + strings.ToLower(key.source) + "|" + strings.ToLower(member.Name) + "|" + member.File
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		groups[key] = append(groups[key], member)
		if sourceKind[key] == "" {
			sourceKind[key] = strings.TrimSpace(row.SourceKind)
		}
	}
	if len(groups) == 0 {
		return nil
	}
	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].relation != keys[j].relation {
			return keys[i].relation < keys[j].relation
		}
		return keys[i].source < keys[j].source
	})
	hints := make([]types.TypedRelationHint, 0, len(keys))
	for _, key := range keys {
		members := groups[key]
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].Name != members[j].Name {
				return members[i].Name < members[j].Name
			}
			return members[i].File < members[j].File
		})
		if len(members) > typedRelationProbeMaxMembers {
			members = members[:typedRelationProbeMaxMembers]
		}
		hints = append(hints, types.TypedRelationHint{
			Relation:   key.relation,
			SourceName: key.source,
			SourceKind: sourceKind[key],
			Members:    members,
		})
	}
	return hints
}

// shouldProbeTypedRelations gates the probe on the analyzer's typed
// predicates. A typed-relation hint helps three families of
// answer shapes plus relation-diagram / relation-explanation shapes:
//
//	IsCategoryEnumeration  — "list all X of Y"
//	IsRelationalLookup     — "which X have Y"
//	IsCountQuestion        — "how many X" when X is a typed set
//	SubjectInterface+Diagram — "draw/type-relationship view for interface X"
//	PredicateAxis=implement — "show/explain the implementation relation"
//
// The diagram branch is still typed: answer_subject and diagram_hint are
// analyzer fields, not localized prose. A Mermaid type-relation diagram, an
// architecture explanation, a comparison, and an enumeration can all depend on
// the same interface→implementer relation. The probe itself remains precise
// because it only emits rows when the typed graph resolves the named entity to
// an interface / trait / protocol with concrete implementers. No raw
// user/model prose is parsed here.
func shouldProbeTypedRelations(rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	return types.ShouldSurfaceTypedRelationHints(*rm)
}

// candidateEntityNames returns the narrow, provenance-carrying entity
// tokens the probe should attempt to resolve. Broad analyzer Entities
// can include repo-map expansion/context helpers; those remain search
// hints and must not seed hard typed relation gates. This prompt-only
// relation hint is softer: if no provenance lane exists, fall back to
// Entities even when DerivedEntities is present. The subsequent graph
// probe is still exact and emits rows only for entities that resolve
// to an interface / trait / protocol with concrete implementers.
func candidateEntityNames(rm *types.RequestModel) []string {
	out := types.StructuralRelationScopeCandidates(*rm)
	if len(out) > 0 || rm == nil || !types.ShouldSurfaceTypedRelationHints(*rm) {
		return out
	}
	seen := make(map[string]bool, len(rm.AnalyzerHints.Entities))
	for _, value := range rm.AnalyzerHints.Entities {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// probeImplements walks Graph.ImplementersOf for one entity name.
// Returns nil when the entity does not name an interface / trait /
// protocol OR no concrete type implements it.
func probeImplements(graph any, name string) *types.TypedRelationHint {
	if graph == nil || name == "" {
		return nil
	}
	g, ok := graph.(*repotypes.Graph)
	if !ok || g == nil {
		return nil
	}
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
		Relation:   types.TypedRelationImplements,
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
