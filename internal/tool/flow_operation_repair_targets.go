package tool

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	maxFlowOperationRepairFiles    = 6
	maxFlowOperationRepairKeywords = 8
	flowOperationRepairReadRadius  = 12
	flowNavigationIndexCacheKey    = "flow_navigation_index:v6"
)

type flowOperationRepairReadTarget struct {
	file      string
	lineRange types.LineRange
	// focusIdentity is one parser-owned operation surface at this coordinate
	// (for example the exact complete argument that remains to be emitted).
	// It is navigation/progress identity only: it never becomes evidence or an
	// answer edge without a later grounded model-authored emit_evidence row.
	focusIdentity string
	// alreadyRead distinguishes "open this source" from "extract the typed
	// operation from source that is already present in the read closure". A
	// high-quality already-read cross-participant site must not disappear from
	// navigation merely because the model has not emitted its relation row yet.
	alreadyRead bool
	// receivingCallableBody means this coordinate continues an already-grounded
	// argument handoff into the uniquely resolved receiving callable. It changes
	// only the SOFT extraction hint; it never authorizes an evidence row or edge.
	receivingCallableBody bool
	// callerHandoff means this coordinate was reached by walking backward from
	// a grounded callee-body operation to an exact parser caller. It affects
	// guidance only; the model still owns the argument-flow evidence.
	callerHandoff bool
}

type flowParserRelationSite struct {
	file          string
	relation      *repotypes.Relation
	ownerSurfaces []string
	// carrierOwnerBridgeRank is set only on a binding-scoped copy returned by
	// flowNavigationBindingRelationSites. A positive value means the parser
	// proved that the original member's declaring owner is itself carried by a
	// statically typed binding on this relation owner's type. It is a SOFT
	// navigation preference, never relation or answer authority.
	carrierOwnerBridgeRank int
}

type flowParserSymbolSite struct {
	file   string
	symbol *repotypes.Symbol
}

type flowDeclaredBindingSite struct {
	file         string
	alias        string
	declaredType string
	owner        string
}

type flowNavigationCallableOwner struct {
	name         string
	receiver     string
	line         int
	endLine      int
	prefixMaxEnd int
}

// flowNavigationIndex is a graph-derived, navigation-only index. It replaces
// repeated whole-repository symbol/relation scans during completion retries.
// Candidate lookup is exact after language-neutral case/separator folding;
// the existing typed compatibility checks still decide whether each candidate
// is usable. Fuzzy substring matching remains available to ordinary SOFT
// repo_map/grep guidance, but is intentionally not paid as an O(repository)
// synchronous completion tax.
type flowNavigationIndex struct {
	symbolsByKey         map[string][]flowParserSymbolSite
	relationsByKey       map[string][]flowParserRelationSite
	relationsByToken     map[string][]flowParserRelationSite
	relationsByFile      map[string][]flowParserRelationSite
	relationsByOwnerKey  map[string][]flowParserRelationSite
	relationsByTarget    map[*repotypes.Symbol][]flowParserRelationSite
	sourceLinesMu        sync.Mutex
	sourceLinesByFile    map[string]map[int]string
	sourceLinesAttempted map[string]bool
}

func flowNavigationIndexForContext(ctx *types.BusContext) *flowNavigationIndex {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	if cached, ok := ctx.Mutable.SearchGraphDerived(flowNavigationIndexCacheKey).(*flowNavigationIndex); ok && cached != nil {
		return cached
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || len(graph.FileIndex) == 0 {
		return nil
	}
	index := &flowNavigationIndex{
		symbolsByKey:         make(map[string][]flowParserSymbolSite),
		relationsByKey:       make(map[string][]flowParserRelationSite),
		relationsByToken:     make(map[string][]flowParserRelationSite),
		relationsByFile:      make(map[string][]flowParserRelationSite),
		relationsByOwnerKey:  make(map[string][]flowParserRelationSite),
		relationsByTarget:    make(map[*repotypes.Symbol][]flowParserRelationSite),
		sourceLinesByFile:    make(map[string]map[int]string),
		sourceLinesAttempted: make(map[string]bool),
	}
	paths := make([]string, 0, len(graph.FileIndex))
	for path := range graph.FileIndex {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file := graph.FileIndex[path]
		if file == nil {
			continue
		}
		for i := range file.Symbols {
			symbol := &file.Symbols[i]
			site := flowParserSymbolSite{
				file:   canonicalRelationSourcePath(firstNonEmptyFlowRepairString(symbol.File, file.RelPath, path)),
				symbol: symbol,
			}
			for _, key := range flowNavigationSurfaceKeys(symbol.Name, symbol.DeclaredType) {
				index.symbolsByKey[key] = append(index.symbolsByKey[key], site)
			}
		}
		callableOwners := flowNavigationCallableOwners(file.Symbols)
		for i := range file.Relations {
			relation := &file.Relations[i]
			switch relation.Kind {
			case "call", "reference", "type_usage":
			default:
				continue
			}
			line := relation.Line
			if line <= 0 {
				line = max(relation.FromEP.Line, relation.ToEP.Line)
			}
			site := flowParserRelationSite{
				file:          canonicalRelationSourcePath(firstNonEmptyFlowRepairString(relation.File, file.RelPath, path)),
				relation:      relation,
				ownerSurfaces: flowNavigationEnclosingCallableSurfaces(callableOwners, line),
			}
			if site.file != "" {
				index.relationsByFile[site.file] = append(index.relationsByFile[site.file], site)
			}
			if relation.Kind == "call" {
				if target := graph.ResolveCallTarget(file, *relation); target != nil {
					index.relationsByTarget[target] = append(index.relationsByTarget[target], site)
				}
			}
			for _, key := range flowNavigationSurfaceKeys(site.ownerSurfaces...) {
				index.relationsByOwnerKey[key] = append(index.relationsByOwnerKey[key], site)
			}
			seenKeys := make(map[string]bool)
			operationSurfaces := append(
				flowRepairRelationEndpointSurfaces(relation.FromEP),
				flowRepairRelationEndpointSurfaces(relation.ToEP)...,
			)
			operationSurfaces = append(operationSurfaces, site.ownerSurfaces...)
			for _, key := range flowNavigationSurfaceKeys(operationSurfaces...) {
				if seenKeys[key] {
					continue
				}
				seenKeys[key] = true
				index.relationsByKey[key] = append(index.relationsByKey[key], site)
			}
			seenTokens := make(map[string]bool)
			for _, surface := range site.ownerSurfaces {
				for _, token := range flowNavigationIdentityTokens(surface) {
					if len(token) < 4 || seenTokens[token] {
						continue
					}
					seenTokens[token] = true
					index.relationsByToken[token] = append(index.relationsByToken[token], site)
				}
			}
		}
	}
	ctx.Mutable.SetSearchGraphDerived(flowNavigationIndexCacheKey, index)
	return index
}

func flowNavigationSurfaceKeys(surfaces ...string) []string {
	var keys []string
	for _, surface := range surfaces {
		for _, identity := range flowParticipantSymbolLookupKeys([]string{surface}) {
			if key := flowRepairPlanningKey(identity); key != "" {
				keys = appendUniqueBounded(keys, []string{key}, maxFlowOperationRepairKeywords)
			}
		}
	}
	return keys
}

func flowNavigationSymbols(index *flowNavigationIndex, surfaces []string) []flowParserSymbolSite {
	if index == nil {
		return nil
	}
	var out []flowParserSymbolSite
	seen := make(map[*repotypes.Symbol]bool)
	for _, key := range flowNavigationSurfaceKeys(surfaces...) {
		for _, site := range index.symbolsByKey[key] {
			if site.symbol == nil || seen[site.symbol] {
				continue
			}
			seen[site.symbol] = true
			out = append(out, site)
		}
	}
	return out
}

func flowNavigationRelationSites(index *flowNavigationIndex, surfaces []string) []flowParserRelationSite {
	if index == nil {
		return nil
	}
	var out []flowParserRelationSite
	seen := make(map[*repotypes.Relation]bool)
	for _, key := range flowNavigationSurfaceKeys(surfaces...) {
		for _, site := range index.relationsByKey[key] {
			if site.relation == nil || seen[site.relation] {
				continue
			}
			seen[site.relation] = true
			out = append(out, site)
		}
	}
	// A request-visible stage/component label may be a whole lexical token
	// inside its implementation type (`extractor` -> `extractorEvaluator`).
	// Query only when the requested surface itself is exactly one token; a
	// compound identity such as BusContext never fans out through `context`.
	// This is a bounded navigation alias, not identity or relation authority.
	for _, surface := range surfaces {
		tokens := flowNavigationIdentityTokens(surface)
		if len(tokens) != 1 || len(tokens[0]) < 4 || flowRepairPlanningKey(surface) != tokens[0] {
			continue
		}
		for _, site := range index.relationsByToken[tokens[0]] {
			if site.relation == nil || seen[site.relation] {
				continue
			}
			seen[site.relation] = true
			out = append(out, site)
		}
	}
	return out
}

// flowNavigationCallableOwners builds a per-file interval index for the
// universal function/method symbols emitted by every supported parser.
func flowNavigationCallableOwners(symbols []repotypes.Symbol) []flowNavigationCallableOwner {
	owners := make([]flowNavigationCallableOwner, 0, len(symbols))
	for _, symbol := range symbols {
		if (symbol.Kind != "function" && symbol.Kind != "method") || symbol.Line <= 0 || symbol.EndLine < symbol.Line {
			continue
		}
		owners = append(owners, flowNavigationCallableOwner{
			name: strings.TrimSpace(symbol.Name), receiver: strings.TrimSpace(symbol.Receiver),
			line: symbol.Line, endLine: symbol.EndLine,
		})
	}
	sort.SliceStable(owners, func(i, j int) bool {
		if owners[i].line != owners[j].line {
			return owners[i].line < owners[j].line
		}
		return owners[i].endLine > owners[j].endLine
	})
	prefixMaxEnd := 0
	for i := range owners {
		prefixMaxEnd = max(prefixMaxEnd, owners[i].endLine)
		owners[i].prefixMaxEnd = prefixMaxEnd
	}
	return owners
}

// flowNavigationEnclosingCallableSurfaces recovers the parser-owned operation
// owner when a language extractor leaves Relation.FromEP empty. The innermost
// enclosing function/method range wins; no filename, request prose, or model
// answer text participates. These surfaces guide source navigation only.
func flowNavigationEnclosingCallableSurfaces(owners []flowNavigationCallableOwner, line int) []string {
	if line <= 0 || len(owners) == 0 {
		return nil
	}
	idx := sort.Search(len(owners), func(i int) bool { return owners[i].line > line }) - 1
	var selected flowNavigationCallableOwner
	for idx >= 0 {
		candidate := owners[idx]
		if candidate.endLine >= line {
			selected = candidate
			break
		}
		if candidate.prefixMaxEnd < line {
			break
		}
		idx--
	}
	if selected.name == "" {
		return nil
	}
	owner := selected.name
	if selected.receiver != "" {
		owner = selected.receiver + "." + selected.name
	}
	out := []string{owner, selected.receiver, selected.name}
	return appendUniqueBounded(nil, out, maxFlowOperationRepairKeywords)
}

// flowNavigationIdentityTokens splits language-neutral identifier boundaries
// for the SOFT relation index. It handles separators plus lower/digit→upper
// camel transitions; exact compound identities remain queried by their full
// key and are never broadened to individual tokens.
func flowNavigationIdentityTokens(raw string) []string {
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		token := strings.ToLower(string(current))
		tokens = appendUniqueBounded(tokens, []string{token}, maxFlowOperationRepairKeywords)
		current = current[:0]
	}
	var previous rune
	for _, r := range strings.TrimSpace(raw) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			previous = 0
			continue
		}
		if len(current) > 0 && unicode.IsUpper(r) && (unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			flush()
		}
		current = append(current, r)
		previous = r
	}
	flush()
	return tokens
}

// flowNavigationSourceLines loads one parser-indexed source file only after a
// typed declared binding has made that file relevant to a repair. The cache is
// navigation-only and run-local; source text never becomes evidence merely by
// being present here. repoRelativePathWithinRoot prevents graph/cache paths
// from escaping the active repository.
func flowNavigationSourceLines(ctx *types.BusContext, index *flowNavigationIndex, source string) map[int]string {
	if ctx == nil || index == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	source = canonicalRelationSourcePath(source)
	if source == "" {
		return nil
	}
	index.sourceLinesMu.Lock()
	defer index.sourceLinesMu.Unlock()
	if index.sourceLinesAttempted[source] {
		return index.sourceLinesByFile[source]
	}
	index.sourceLinesAttempted[source] = true
	rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, source)
	if !ok || rel == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	rows := strings.Split(text, "\n")
	lines := make(map[int]string, len(rows))
	for idx, row := range rows {
		lines[idx+1] = strings.TrimSuffix(row, "\r")
	}
	index.sourceLinesByFile[source] = lines
	return lines
}

func flowNavigationCallReceiver(relation *repotypes.Relation) string {
	if relation == nil || strings.TrimSpace(relation.Kind) != "call" {
		return ""
	}
	name := strings.TrimSpace(relation.ToEP.Name)
	receiver := strings.TrimSpace(relation.ToEP.Receiver)
	if name != "" && receiver != "" {
		return receiver + "." + name
	}
	return firstNonEmptyFlowRepairString(name, receiver)
}

// flowNavigationArgumentMatchesBinding accepts only a complete identifier-like
// argument containing the parser-declared binding as one exact identity
// segment. The binding may be the final value (`ctx.Mutable`) or an outer typed
// carrier whose member is handed off (`o.busCtx.Mutable`). Quoted literals are
// rejected before identity normalization so a display string such as "busCtx"
// cannot steer source navigation as though it were the variable. This is only
// a read-coordinate signal; the complete argument still needs separately
// grounded evidence before it can authorize any relation.
func flowNavigationArgumentMatchesBinding(argument, alias string) bool {
	argument = strings.TrimSpace(argument)
	alias = strings.TrimSpace(alias)
	if argument == "" || alias == "" {
		return false
	}
	switch argument[0] {
	case '\'', '"', '`':
		return false
	}
	return types.AnswerCodeIdentitySurfacesCompatible(alias, argument) ||
		types.AnswerCodeIdentityContainsExactSegment(argument, alias)
}

// flowResolvedParticipantIdentity is a parser-owned navigation projection for
// one request-visible participant. Surfaces may close a hard *search*
// obligation, but never create relation authority; only a separately grounded
// operation can do that.
type flowResolvedParticipantIdentity struct {
	// surfaces are precise hard-coverage identities. planningSurfaces are
	// additional parser-owned navigation aliases and must never be passed to
	// relation/answer authority.
	surfaces         []string
	planningSurfaces []string
	files            []string
}

func (r flowResolvedParticipantIdentity) softNavigationSurfaces() []string {
	out := appendUniqueBounded(nil, r.surfaces, maxFlowOperationRepairKeywords)
	return appendUniqueBounded(out, r.planningSurfaces, maxFlowOperationRepairKeywords)
}

// flowOperationPlanningParticipants returns the identities that may steer a
// SOFT operation-site search. An explicit diagram participant slate remains
// authoritative when present. When a required flow visual intentionally has
// no explicit slate, the repair lane would otherwise have no parser identity
// to search even though entity resolution already proved exact source
// symbols. In that narrow shape, reuse only resolver-confirmed symbol
// provenance that was admitted for search or shape.
//
// The derived rows are navigation-only. They are not written back to
// DiagramHint, do not create participant coverage obligations, and never mint
// an evidence row or relation edge. Ambiguous symbols, concepts, scopes,
// files, and unresolved names remain excluded.
func flowOperationPlanningParticipants(rm types.RequestModel) []types.DiagramParticipantHint {
	if rm.DiagramHint != nil && len(rm.DiagramHint.Participants) > 0 {
		return rm.DiagramHint.Participants
	}
	if rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace {
		return nil
	}

	seen := make(map[string]bool)
	out := make([]types.DiagramParticipantHint, 0, maxFlowOperationRepairKeywords)
	for _, provenance := range rm.AnalyzerHints.EntityProvenance {
		if !provenance.Resolved || provenance.Resolution != types.EntityResolutionSymbol ||
			(!provenance.UseForSearch && !provenance.UseForShape) {
			continue
		}
		identity := strings.TrimSpace(provenance.ResolvedAs)
		if identity == "" {
			identity = strings.TrimSpace(provenance.Surface)
		}
		key := strings.ToLower(identity)
		if identity == "" || key == "" || seen[key] || !types.IsCodeIdentitySurface(identity) {
			continue
		}
		seen[key] = true
		out = append(out, types.DiagramParticipantHint{
			Identity: identity,
			Role:     types.DiagramParticipantIncidentRequired,
		})
		if len(out) == maxFlowOperationRepairKeywords {
			break
		}
	}
	return out
}

// flowResolveParticipantIdentity keeps the analyzer's unique-symbol lane as
// the first authority, then permits one narrow late upgrade from the complete
// parser graph. The upgrade exists for request labels that name a static member
// (for example a carrier field) while the analyzer prescan resolved only its
// owner/type.
//
// A member upgrade is accepted only when:
//   - the member name is an exact identity match;
//   - its parser-owned static type is present;
//   - its owner is another uniquely resolved participant in the same requested
//     diagram; and
//   - exactly one principal-source declaration survives.
//
// This is deliberately stricter than soft navigation. It cannot upgrade a
// homonym, a concept label, an untyped/dynamic binding, or a member under an
// unrequested/ambiguous owner.
func flowResolveParticipantIdentity(ctx *types.BusContext, rm types.RequestModel, participant types.DiagramParticipantHint) flowResolvedParticipantIdentity {
	surfaces := types.DiagramParticipantIdentitySurfaces(rm, participant)
	if types.DiagramParticipantHasPreciseSourceOperationIdentity(rm, participant) {
		return flowResolvedParticipantIdentity{surfaces: appendUniqueBounded(nil, surfaces, maxFlowOperationRepairKeywords)}
	}
	if ctx == nil || ctx.Mutable == nil || len(surfaces) == 0 || rm.DiagramHint == nil {
		return flowResolvedParticipantIdentity{}
	}
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		return flowResolvedParticipantIdentity{}
	}

	var ownerSurfaces []string
	for _, owner := range flowOperationPlanningParticipants(rm) {
		if strings.EqualFold(strings.TrimSpace(owner.Identity), strings.TrimSpace(participant.Identity)) ||
			!owner.Role.IsValid() ||
			!types.DiagramParticipantHasPreciseSourceOperationIdentity(rm, owner) {
			continue
		}
		ownerSurfaces = appendUniqueBounded(ownerSurfaces,
			types.DiagramParticipantIdentitySurfaces(rm, owner), maxFlowOperationRepairKeywords)
	}
	if len(ownerSurfaces) == 0 {
		return flowResolvedParticipantIdentity{}
	}

	type candidate struct {
		file         string
		name         string
		parent       string
		declaredType string
	}
	var candidates []candidate
	for _, site := range flowNavigationSymbols(index, surfaces) {
		symbol := site.symbol
		if symbol == nil {
			continue
		}
		path := canonicalRelationSourcePath(firstNonEmptyFlowRepairString(symbol.File, site.file))
		name := strings.TrimSpace(symbol.Name)
		parent := strings.TrimSpace(symbol.Parent)
		declaredType := strings.TrimSpace(symbol.DeclaredType)
		if path == "" || !relationSourceInRequestedScope(path, rm) ||
			name == "" || parent == "" || declaredType == "" ||
			!flowAnyIdentitySurfaceMatches(surfaces, name) ||
			!flowAnyIdentitySurfaceMatches(ownerSurfaces, parent) {
			continue
		}
		candidates = append(candidates, candidate{
			file: path, name: name, parent: parent, declaredType: declaredType,
		})
	}
	if len(candidates) != 1 {
		return flowResolvedParticipantIdentity{}
	}
	c := candidates[0]
	resolved := appendUniqueBounded(nil, surfaces, maxFlowOperationRepairKeywords)
	resolved = appendUniqueBounded(resolved,
		[]string{c.parent + "." + c.name, c.declaredType}, maxFlowOperationRepairKeywords)
	return flowResolvedParticipantIdentity{
		surfaces: resolved,
		// The requested member remains the only hard identity. Its exact
		// parser-owned parent is a SOFT navigation bridge so a later repair can
		// follow a value handoff through the owner without treating every owner
		// operation as an incident edge for the member itself.
		planningSurfaces: appendUniqueBounded(nil, []string{c.parent}, maxFlowOperationRepairKeywords),
		files:            appendUniqueBounded(nil, []string{c.file}, maxFlowOperationRepairFiles),
	}
}

// flowParticipantSymbolLookupKeys projects already-typed identity surfaces to
// exact SymbolDefs keys. It is a lookup accelerator, not an identity resolver:
// compatibility and owner checks still run on every returned parser symbol,
// and a late upgrade still requires exactly one surviving declaration.
//
// Keeping the original case is intentional because SymbolDefs follows source
// language spelling. Qualified identities across the supported languages use
// one of these parser/display separators; only their final declaration segment
// can be a name-keyed SymbolDefs key.
func flowParticipantSymbolLookupKeys(surfaces []string) []string {
	var keys []string
	for _, surface := range surfaces {
		raw := strings.Trim(strings.TrimSpace(surface), "`'\"")
		raw = strings.TrimSuffix(raw, "()")
		if raw == "" || strings.ContainsAny(raw, "\n\r\t ") {
			continue
		}
		raw = strings.NewReplacer(
			"::", ".", "->", ".", "#", ".", "/", ".", `\`, ".",
		).Replace(raw)
		parts := strings.Split(raw, ".")
		key := strings.Trim(strings.TrimSpace(parts[len(parts)-1]), "*&( )")
		if key == "" || !types.IsCodeIdentitySurface(key) {
			continue
		}
		keys = appendUniqueBounded(keys, []string{key}, maxFlowOperationRepairKeywords)
	}
	return keys
}

func flowAnyIdentitySurfaceMatches(surfaces []string, identity string) bool {
	for _, surface := range surfaces {
		if types.AnswerCodeIdentitySurfacesCompatible(surface, identity) ||
			types.AnswerCodeIdentitySurfacesEquivalent(surface, identity) {
			return true
		}
	}
	return false
}

// flowOperationRepairTargets builds a bounded navigation plan exclusively
// from typed analyzer participants, citable evidence, and the read closure.
// Its fuzzy compatibility is intentionally planning-only: these targets may
// help find an operation site, but can never authorize a relation edge.
func flowOperationRepairTargets(ctx *types.BusContext, missing []string, evidence []types.EvidenceItem) ([]string, []string) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil {
		return nil, nil
	}
	rm := ctx.AnalysisIR.RequestModel
	wanted := make(map[string]bool, len(missing))
	for _, identity := range missing {
		if key := strings.ToLower(strings.TrimSpace(identity)); key != "" {
			wanted[key] = true
		}
	}

	var surfaces []string
	var resolvedFiles []string
	participants := flowOperationPlanningParticipants(rm)
	for _, participant := range participants {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		if len(wanted) > 0 && !wanted[strings.ToLower(strings.TrimSpace(participant.Identity))] {
			continue
		}
		resolved := flowResolveParticipantIdentity(ctx, rm, participant)
		surfaces = appendUniqueBounded(surfaces, resolved.softNavigationSurfaces(), maxFlowOperationRepairKeywords)
		resolvedFiles = appendUniqueBounded(resolvedFiles, resolved.files, maxFlowOperationRepairFiles)
	}
	// A typed context-only participant is not an edge obligation. It becomes a
	// useful SOFT search scope only after structured evidence proves that every
	// missing incident participant already has a separate local operation and
	// the remaining deficit is solely their relationship inside a named
	// container/stage/subsystem. Initial operation discovery therefore remains
	// tightly scoped to the missing incident participant. This context never
	// enters the missing roster or relation authority.
	if flowMissingParticipantsHaveLocalOperations(ctx, evidence, missing) {
		for _, participant := range participants {
			if participant.Role != types.DiagramParticipantContextOnly {
				continue
			}
			contextSurfaces := []string{strings.TrimSpace(participant.Identity)}
			contextSurfaces = append(contextSurfaces, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
			surfaces = appendUniqueBounded(surfaces, contextSurfaces, maxFlowOperationRepairKeywords)
			resolved := flowResolveParticipantIdentity(ctx, rm, participant)
			resolvedFiles = appendUniqueBounded(resolvedFiles, resolved.files, maxFlowOperationRepairFiles)
			surfaces = appendUniqueBounded(surfaces, resolved.softNavigationSurfaces(), maxFlowOperationRepairKeywords)
		}
	}

	keywords := append([]string(nil), surfaces...)
	declaredFiles, declaredAliases := flowRepairDeclaredBindingTargets(ctx, rm, surfaces)
	keywords = appendUniqueBounded(keywords, declaredAliases, maxFlowOperationRepairKeywords)
	relationFiles, relationAliases := flowRepairParserRelationTargets(ctx, rm, surfaces)
	keywords = appendUniqueBounded(keywords, relationAliases, maxFlowOperationRepairKeywords)
	// A request identity may name a field or conceptual carrier (`Mutable`)
	// while grounded current-source evidence exposes its declared type or
	// operation identity (`MutableState`, `applyStageOutput`). Carry those exact
	// related surfaces into the bounded search plan even when the participant
	// keyword list is non-empty. This is navigation-only: relation authority
	// still requires a separately emitted citable operation row.
	for _, item := range evidence {
		if !item.IsCitable() || !relationSourceInRequestedScope(item.Source, rm) ||
			(len(surfaces) > 0 && !flowRepairItemMatchesAnySurface(item, surfaces)) {
			continue
		}
		keywords = appendUniqueBounded(keywords,
			[]string{item.Subject, item.Object, item.AnchorSymbol, item.OwnerSymbol},
			maxFlowOperationRepairKeywords)
		if len(keywords) == maxFlowOperationRepairKeywords {
			break
		}
	}

	var related, other []string
	for _, item := range evidence {
		if !item.IsCitable() || !relationSourceInRequestedScope(item.Source, rm) {
			continue
		}
		file := canonicalRelationSourcePath(item.Source)
		if file == "" {
			continue
		}
		if flowRepairItemMatchesAnySurface(item, surfaces) {
			related = appendUniqueBounded(related, []string{file}, maxFlowOperationRepairFiles)
		} else {
			other = appendUniqueBounded(other, []string{file}, maxFlowOperationRepairFiles)
		}
	}
	sort.Strings(related)
	sort.Strings(other)
	files := appendUniqueBounded(nil, resolvedFiles, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, declaredFiles, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, relationFiles, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, related, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, other, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, completionMaterializationReadFiles(ctx.Mutable.EvidenceClosure()), maxFlowOperationRepairFiles)
	return files, keywords
}

// flowOperationRepairReadTargetForMissing returns one bounded, parser-owned
// operation occurrence. It is a navigation target only: an unread occurrence
// may be put on the lazy-read queue, while an already-read occurrence asks the
// model to extract the exact operation from its existing source context. The
// occurrence never becomes EvidenceItem authority and never closes a
// participant or relation gate by itself. The explorer must still inspect the
// source and emit the exact syntax-owned operation through emit_evidence.
//
// Selecting one target at a time keeps the repair cross-language and cheap.
// If the first occurrence is only a declaration-adjacent reference, the next
// identical completion downgrade advances to the next unread parser site
// rather than bulk-reading every fuzzy candidate.
func flowOperationRepairReadTargetForMissing(ctx *types.BusContext, missing []string) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil {
		return flowOperationRepairReadTarget{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	wanted := make(map[string]bool, len(missing))
	for _, identity := range missing {
		if key := strings.ToLower(strings.TrimSpace(identity)); key != "" {
			wanted[key] = true
		}
	}
	var surfaces []string
	var participantSurfaceGroups [][]string
	var missingParticipantSurfaceGroups [][]string
	if rm.DiagramHint != nil {
		for _, participant := range flowOperationPlanningParticipants(rm) {
			if participant.Role != types.DiagramParticipantIncidentRequired {
				continue
			}
			resolved := flowResolveParticipantIdentity(ctx, rm, participant)
			planningSurfaces := resolved.softNavigationSurfaces()
			if len(planningSurfaces) > 0 {
				participantSurfaceGroups = append(participantSurfaceGroups, planningSurfaces)
			}
			// Candidate discovery remains scoped to the participant(s) that
			// still lack incidence. Candidate QUALITY, however, must compare
			// that operation with every independently requested participant.
			// A stage/component may already be covered by another edge while
			// still being the exact opposite endpoint carried as a sibling
			// argument at the missing carrier's real handoff. Filtering both
			// sets by missing status made that useful endpoint disappear and
			// left unrelated same-type helper calls tied for first place.
			if len(wanted) > 0 && !wanted[strings.ToLower(strings.TrimSpace(participant.Identity))] {
				continue
			}
			missingParticipantSurfaceGroups = append(missingParticipantSurfaceGroups, planningSurfaces)
			surfaces = appendUniqueBounded(surfaces, planningSurfaces, maxFlowOperationRepairKeywords)
		}
	}
	if len(surfaces) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		return flowOperationRepairReadTarget{}, false
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if target, ok := flowNavigationGroundedBodyOperationCallerHandoffReadTarget(
		ctx, index, participantSurfaceGroups, missingParticipantSurfaceGroups, evidence,
	); ok {
		return target, true
	}
	if target, ok := flowNavigationGroundedHandoffCalleeOperationReadTarget(
		ctx, index, missingParticipantSurfaceGroups, evidence,
	); ok {
		return target, true
	}
	type rankedTarget struct {
		target        flowOperationRepairReadTarget
		relation      *repotypes.Relation
		ownerSurfaces []string
		// participantTouchRank counts distinct requested participant groups
		// touched by this one parser-owned operation coordinate. It ranks a
		// real cross-participant receiver/caller site ahead of a ubiquitous
		// context argument used by an unrelated helper.
		participantTouchRank   int
		carrierOwnerBridgeRank int
		// carrierValueRank distinguishes handing off the carrier itself from
		// handing off one derived member. Both are useful navigation, but an
		// exact whole-carrier transfer is the stronger next place to inspect.
		carrierValueRank int
		carrierRank      int
		handoffRank      int
		matchRank        int
		kindRank         int
		line             int
	}
	var candidates []rankedTarget
	closure := ctx.Mutable.EvidenceClosure()
	for _, site := range flowNavigationRelationSites(index, surfaces) {
		path := site.file
		if path == "" || !relationSourceInRequestedScope(path, rm) || site.relation == nil {
			continue
		}
		relation := site.relation
		from := append(flowRepairRelationEndpointSurfaces(relation.FromEP), site.ownerSurfaces...)
		to := flowRepairRelationEndpointSurfaces(relation.ToEP)
		matchRank := max(
			flowRepairPlanningSurfaceMatchRank(surfaces, from),
			flowRepairPlanningSurfaceMatchRank(surfaces, to),
		)
		if matchRank == 0 {
			continue
		}
		file := path
		line := relation.Line
		if line <= 0 {
			line = relation.FromEP.Line
		}
		if line <= 0 {
			line = relation.ToEP.Line
		}
		if file == "" || line <= 0 {
			continue
		}
		start := line - flowOperationRepairReadRadius
		if start < 1 {
			start = 1
		}
		candidates = append(candidates, rankedTarget{
			target: flowOperationRepairReadTarget{
				file:        file,
				lineRange:   types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
				alreadyRead: closure.HasReadLine(file, line),
				focusIdentity: firstNonEmptyFlowRepairString(
					relation.ToEP.Name, relation.ToEP.Receiver, relation.FromEP.Name, relation.FromEP.Receiver,
				),
			},
			relation:      relation,
			ownerSurfaces: append([]string(nil), site.ownerSurfaces...),
			participantTouchRank: flowNavigationRequestedParticipantTouchRank(
				relation, flowDeclaredBindingSite{}, site.ownerSurfaces, participantSurfaceGroups,
			),
			matchRank: matchRank,
			kindRank:  flowOperationRepairRelationKindRank(relation.Kind),
			line:      line,
		})
	}
	// A static participant is often named by its type while its operation site
	// mentions only a field/local binding. Parser relation endpoints cannot
	// carry call arguments, so endpoint-only lookup repeatedly lands on local
	// receiver calls and misses the actual handoff. Join the exact declared
	// binding to parser-owned calls in the same file and rank a complete
	// argument occurrence ahead of those local sites. This remains a read
	// coordinate: DetectArgumentFlowsAtLine does not create evidence, and the
	// explorer must still submit the source-owned argument row.
	// Allocate the declared-binding search budget per missing participant.
	// Flattening every identity into one globally bounded list allowed a
	// high-frequency carrier (for example Mutable) to consume every file slot
	// before another independently requested carrier (for example BusContext)
	// reached candidate scoring. That made lexical graph order, rather than
	// operation quality, choose the repair. Per-participant quotas stay bounded
	// while preventing cross-participant starvation.
	var bindingSites []flowDeclaredBindingSite
	seenBindingSites := make(map[string]bool)
	for _, group := range missingParticipantSurfaceGroups {
		for _, binding := range flowRepairDeclaredBindingSites(index, rm, group) {
			key := strings.ToLower(binding.file + "\x00" + binding.alias + "\x00" + binding.owner)
			if seenBindingSites[key] {
				continue
			}
			seenBindingSites[key] = true
			bindingSites = append(bindingSites, binding)
		}
	}
	for _, binding := range bindingSites {
		graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
		for _, site := range flowNavigationBindingRelationSites(index, graph, binding) {
			lines := flowNavigationSourceLines(ctx, index, site.file)
			if len(lines) == 0 {
				continue
			}
			gc := &ground.Context{
				Graph: graph, RepoRoot: ctx.RepoRoot,
				LineIndex: map[string]map[int]string{site.file: lines},
			}
			relation := site.relation
			callee := flowNavigationCallReceiver(relation)
			if relation == nil || callee == "" {
				continue
			}
			line := relation.Line
			if line <= 0 {
				line = relation.ToEP.Line
			}
			if line <= 0 {
				continue
			}
			argumentRank := 0
			var argumentSurfaces []string
			for _, flow := range ground.DetectArgumentFlowsAtLine(gc, site.file, line, callee) {
				argumentSurfaces = append(argumentSurfaces, flow.Argument)
				if flowNavigationArgumentMatchesBinding(flow.Argument, binding.alias) {
					argumentRank = max(argumentRank, flowRepairPlanningSurfaceMatchRank(
						[]string{binding.alias}, []string{flow.Argument},
					))
				}
			}
			if argumentRank == 0 {
				continue
			}
			start := line - flowOperationRepairReadRadius
			if start < 1 {
				start = 1
			}
			candidates = append(candidates, rankedTarget{
				target: flowOperationRepairReadTarget{
					file:          site.file,
					lineRange:     types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
					alreadyRead:   closure.HasReadLine(site.file, line),
					focusIdentity: binding.alias,
				},
				relation:      relation,
				ownerSurfaces: append([]string(nil), site.ownerSurfaces...),
				participantTouchRank: flowNavigationRequestedParticipantTouchRank(
					relation, binding,
					append(append(append([]string(nil), site.ownerSurfaces...), binding.owner), argumentSurfaces...),
					participantSurfaceGroups,
				),
				carrierOwnerBridgeRank: site.carrierOwnerBridgeRank,
				carrierValueRank:       flowNavigationCarrierArgumentValueRank(argumentSurfaces, binding.alias),
				carrierRank:            1,
				handoffRank:            flowNavigationCarrierHandoffRank(relation, binding.alias),
				matchRank:              argumentRank,
				kindRank:               flowOperationRepairRelationKindRank(relation.Kind),
				line:                   line,
			})
		}
	}
	if len(candidates) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		// A parser-owned call is the only relation kind in this candidate
		// pool that can expose a complete value argument. Prefer that
		// operation coordinate before counting lexical participant touches:
		// a type_usage/reference beside two requested labels still cannot
		// produce the missing transfer row, while the call can be inspected
		// for its exact argument/receiver handoff. This remains navigation
		// only; no call, argument, participant incidence, or diagram edge is
		// minted here. Within the same parser relation kind the existing
		// multi-participant and whole-carrier ranks retain their ordering.
		if candidates[i].kindRank != candidates[j].kindRank {
			return candidates[i].kindRank > candidates[j].kindRank
		}
		if candidates[i].participantTouchRank != candidates[j].participantTouchRank {
			return candidates[i].participantTouchRank > candidates[j].participantTouchRank
		}
		if candidates[i].carrierValueRank != candidates[j].carrierValueRank {
			return candidates[i].carrierValueRank > candidates[j].carrierValueRank
		}
		if candidates[i].carrierOwnerBridgeRank != candidates[j].carrierOwnerBridgeRank {
			return candidates[i].carrierOwnerBridgeRank > candidates[j].carrierOwnerBridgeRank
		}
		if candidates[i].carrierRank != candidates[j].carrierRank {
			return candidates[i].carrierRank > candidates[j].carrierRank
		}
		if candidates[i].handoffRank != candidates[j].handoffRank {
			return candidates[i].handoffRank > candidates[j].handoffRank
		}
		if candidates[i].matchRank != candidates[j].matchRank {
			return candidates[i].matchRank > candidates[j].matchRank
		}
		// When two coordinates are equally useful, spend the repair on fresh
		// source. Crucially, read state is below semantic quality: a direct
		// cross-participant operation already in context still outranks an
		// unrelated unread carrier use.
		if candidates[i].target.alreadyRead != candidates[j].target.alreadyRead {
			return !candidates[i].target.alreadyRead
		}
		if candidates[i].target.file != candidates[j].target.file {
			return candidates[i].target.file < candidates[j].target.file
		}
		return candidates[i].line < candidates[j].line
	})
	selected := candidates[0]
	if selected.target.alreadyRead && selected.carrierRank > 0 {
		// A carrier-producing call inside a dispatcher is often only the
		// beginning of the requested path: its result is consumed by the agent
		// invocation, whose result is then applied back to shared state. Follow
		// that exact source-owned result/argument chain before diving into the
		// callee body. This only chooses the next read coordinate.
		if next, ok := flowNavigationCallResultContinuationReadTarget(
			ctx, index, selected.relation, selected.ownerSurfaces,
		); ok {
			return next, true
		}
		if next, ok := flowNavigationCalleeMutationReadTarget(
			ctx, index, selected.relation, participantSurfaceGroups,
		); ok {
			return next, true
		}
	}
	return selected.target, true
}

// flowNavigationGroundedBodyOperationCallerHandoffReadTarget is the reverse
// half of the relation frontier. A model may first discover a precise
// assignment/member call inside a receiving callable (for example a context
// builder copying one carrier field) without having emitted the call-site
// argument that brought the outer carrier into that callable. Starting a new
// repository-wide search at that point loses the proven component.
//
// This helper walks only parser identities: the grounded item's stamped
// enclosing callable, graph-resolved callers of that exact symbol, complete
// source arguments, and parser declarations whose static type matches a still
// missing participant. It chooses a bounded extraction coordinate but does
// not infer formal-parameter binding, create evidence, or authorize an edge.
// A caller handoff already present in evidence is skipped so the forward
// handoff->callee-body frontier can take the next step.
func flowNavigationGroundedBodyOperationCallerHandoffReadTarget(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	participantSurfaceGroups [][]string,
	missingParticipantSurfaceGroups [][]string,
	evidence []types.EvidenceItem,
) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || index == nil ||
		len(participantSurfaceGroups) == 0 || len(missingParticipantSurfaceGroups) == 0 || len(evidence) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return flowOperationRepairReadTarget{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	type candidate struct {
		target          flowOperationRepairReadTarget
		bodyRank        int
		missingRank     int
		participantRank int
		callerLine      int
	}
	var candidates []candidate
	seen := make(map[string]bool)
	closure := ctx.Mutable.EvidenceClosure()
	for _, bodyItem := range evidence {
		if !bodyItem.IsCitable() || strings.TrimSpace(bodyItem.OwnerIdentity) == "" || bodyItem.LineStart <= 0 ||
			!relationSourceInRequestedScope(bodyItem.Source, rm) {
			continue
		}
		bodyRank := 0
		switch types.ClaimFormOf(bodyItem) {
		case types.ClaimAssignmentFact, types.ClaimReturnFact, types.ClaimArgumentFlow:
			bodyRank = 3
		case types.ClaimCallEdge, types.ClaimCallbackHandoff:
			bodyRank = 2
		default:
			continue
		}
		bodyTouchesRequestedParticipant := false
		for _, group := range participantSurfaceGroups {
			for _, endpoint := range []string{bodyItem.Subject, bodyItem.Object} {
				if diagramParticipantCandidateEndpointMatches(group, endpoint, bodyItem, evidence) {
					bodyTouchesRequestedParticipant = true
					break
				}
			}
			if bodyTouchesRequestedParticipant {
				break
			}
		}
		if !bodyTouchesRequestedParticipant {
			continue
		}
		callable := flowNavigationExactEvidenceOwnerCallable(graph, bodyItem)
		if callable == nil {
			continue
		}
		for _, site := range index.relationsByTarget[callable] {
			relation := site.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" ||
				!relationSourceInRequestedScope(site.file, rm) {
				continue
			}
			line := max(relation.Line, relation.FromEP.Line, relation.ToEP.Line)
			if line <= 0 {
				continue
			}
			lines := flowNavigationSourceLines(ctx, index, site.file)
			callee := flowNavigationCallReceiver(relation)
			if len(lines) == 0 || callee == "" {
				continue
			}
			callerInfo := graph.FileIndex[site.file]
			if callerInfo == nil {
				continue
			}
			gc := &ground.Context{
				Graph: graph, RepoRoot: ctx.RepoRoot,
				LineIndex: map[string]map[int]string{site.file: lines},
			}
			for _, argument := range ground.DetectArgumentFlowsAtLine(gc, site.file, line, callee) {
				missingRank := 0
				for _, group := range missingParticipantSurfaceGroups {
					for _, binding := range flowRepairDeclaredBindingSites(index, rm, group) {
						if !flowNavigationBindingCanOwnCallerArgument(graph, callerInfo, site, binding) ||
							!flowNavigationArgumentMatchesBinding(argument.Argument, binding.alias) {
							continue
						}
						missingRank = max(missingRank, flowRepairPlanningSurfaceMatchRank(
							[]string{binding.alias}, []string{argument.Argument},
						))
					}
				}
				participantRank := 0
				for _, group := range participantSurfaceGroups {
					participantRank = max(participantRank, flowRepairPlanningSurfaceMatchRank(
						group, []string{argument.Argument},
					))
				}
				// The missing carrier may already have been emitted at this exact
				// call. Continue across a different request-owned sibling argument
				// instead of restarting at another local carrier use. This is the
				// precise bridge for generic dispatch APIs whose component/stage is
				// selected by an enum argument rather than the callee name.
				if missingRank == 0 && participantRank == 0 {
					continue
				}
				if flowNavigationArgumentFlowAlreadyEmitted(
					evidence, site.file, line, argument.Argument, callRelationTargetName(graph, callerInfo, relation),
				) {
					continue
				}
				key := strings.ToLower(site.file + "\x00" + strconv.Itoa(line) + "\x00" + argument.Argument)
				if seen[key] {
					continue
				}
				seen[key] = true
				start := line - flowOperationRepairReadRadius
				if start < 1 {
					start = 1
				}
				candidates = append(candidates, candidate{
					target: flowOperationRepairReadTarget{
						file: site.file, lineRange: types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
						alreadyRead: closure.HasReadLine(site.file, line), callerHandoff: true,
						focusIdentity: argument.Argument,
					},
					bodyRank: bodyRank, missingRank: missingRank, participantRank: participantRank, callerLine: line,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].bodyRank != candidates[j].bodyRank {
			return candidates[i].bodyRank > candidates[j].bodyRank
		}
		if candidates[i].missingRank != candidates[j].missingRank {
			return candidates[i].missingRank > candidates[j].missingRank
		}
		if candidates[i].participantRank != candidates[j].participantRank {
			return candidates[i].participantRank > candidates[j].participantRank
		}
		if candidates[i].target.alreadyRead != candidates[j].target.alreadyRead {
			return candidates[i].target.alreadyRead
		}
		if candidates[i].target.file != candidates[j].target.file {
			return candidates[i].target.file < candidates[j].target.file
		}
		return candidates[i].callerLine < candidates[j].callerLine
	})
	return candidates[0].target, true
}

func flowNavigationExactEvidenceOwnerCallable(graph *repotypes.Graph, item types.EvidenceItem) *repotypes.Symbol {
	if graph == nil || strings.TrimSpace(item.OwnerIdentity) == "" || item.LineStart <= 0 {
		return nil
	}
	path := canonicalRelationSourcePath(item.Source)
	file := graph.FileIndex[path]
	if file == nil {
		return nil
	}
	var matched *repotypes.Symbol
	for index := range file.Symbols {
		symbol := &file.Symbols[index]
		if (symbol.Kind != "function" && symbol.Kind != "method") || symbol.Line <= 0 ||
			symbol.EndLine < item.LineStart || symbol.Line > item.LineStart {
			continue
		}
		qualified := qualifiedEvidenceSymbolNameInFile(file, symbol)
		if !types.AnswerCodeIdentitySurfacesEquivalent(item.OwnerIdentity, qualified) &&
			!types.AnswerCodeIdentitySurfacesCompatible(item.OwnerIdentity, qualified) {
			continue
		}
		if matched != nil {
			return nil
		}
		matched = symbol
	}
	return matched
}

func flowNavigationBindingCanOwnCallerArgument(
	graph *repotypes.Graph,
	callerInfo *repotypes.FileInfo,
	callerSite flowParserRelationSite,
	binding flowDeclaredBindingSite,
) bool {
	if graph == nil || callerInfo == nil || strings.TrimSpace(binding.alias) == "" {
		return false
	}
	bindingInfo := graph.FileIndex[binding.file]
	if bindingInfo == nil || callerInfo.Language != bindingInfo.Language {
		return false
	}
	samePackage := strings.TrimSpace(callerInfo.Package) != "" &&
		strings.TrimSpace(callerInfo.Package) == strings.TrimSpace(bindingInfo.Package)
	sameDirectory := filepath.ToSlash(filepath.Dir(callerInfo.RelPath)) ==
		filepath.ToSlash(filepath.Dir(bindingInfo.RelPath))
	if !samePackage && !sameDirectory {
		return false
	}
	owner := strings.TrimSpace(binding.owner)
	if owner == "" {
		return true
	}
	for _, callerOwner := range callerSite.ownerSurfaces {
		if types.AnswerCodeIdentityOwnsEndpoint(owner, callerOwner) ||
			types.AnswerCodeIdentitySurfacesCompatible(owner, callerOwner) {
			return true
		}
	}
	return false
}

func flowNavigationArgumentFlowAlreadyEmitted(
	evidence []types.EvidenceItem,
	source string,
	line int,
	argument string,
	callee string,
) bool {
	source = canonicalRelationSourcePath(source)
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimArgumentFlow ||
			canonicalRelationSourcePath(item.Source) != source || item.LineStart != line {
			continue
		}
		if types.AnswerCodeIdentitySurfacesCompatible(item.Subject, argument) &&
			(types.AnswerCodeIdentitySurfacesCompatible(item.Object, callee) ||
				types.AnswerCodeIdentitySurfacesEquivalent(item.Object, callee)) {
			return true
		}
	}
	return false
}

// flowNavigationGroundedHandoffCalleeOperationReadTarget continues an exact,
// model-authored argument handoff into the body of its uniquely graph-resolved
// receiver. This closes a common relation-frontier gap: after proving
// `carrier -> BuildContext`, a participant repair must inspect
// `BuildContext`'s already-read body before restarting at an unrelated local
// operation that merely shares the participant's type/name.
//
// Every input to this preference is precise and typed: one citable
// argument-flow row, the parser call relation at the same source/line, one
// ResolveCallTarget result, the target callable span, read-closure coverage,
// and a parser-owned body call incident to a still-missing participant. The
// returned coordinate remains navigation only. The model must inspect the
// source and emit the exact operation; this function never creates evidence,
// a diagram edge, or a conclusion. Ambiguous or unresolved callees fail
// closed and fall back to the ordinary bounded repair search.
func flowNavigationGroundedHandoffCalleeOperationReadTarget(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	missingParticipantSurfaceGroups [][]string,
	evidence []types.EvidenceItem,
) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || index == nil ||
		len(missingParticipantSurfaceGroups) == 0 || len(evidence) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return flowOperationRepairReadTarget{}, false
	}
	type candidate struct {
		target    flowOperationRepairReadTarget
		matchRank int
		line      int
	}
	var candidates []candidate
	seen := make(map[string]bool)
	closure := ctx.Mutable.EvidenceClosure()
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimArgumentFlow || item.LineStart <= 0 ||
			!relationSourceInRequestedScope(item.Source, ctx.AnalysisIR.RequestModel) {
			continue
		}
		callerFile := canonicalRelationSourcePath(item.Source)
		callerInfo := graph.FileIndex[callerFile]
		if callerInfo == nil {
			continue
		}
		var resolved *repotypes.Symbol
		ambiguous := false
		for _, site := range index.relationsByFile[callerFile] {
			relation := site.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" ||
				max(relation.Line, relation.FromEP.Line, relation.ToEP.Line) != item.LineStart {
				continue
			}
			visibleTarget := callRelationTargetName(graph, callerInfo, relation)
			if !flowAnyIdentitySurfaceMatches([]string{item.Object, item.AnchorSymbol}, visibleTarget) &&
				!flowAnyIdentitySurfaceMatches([]string{item.Object, item.AnchorSymbol}, flowNavigationCallReceiver(relation)) {
				continue
			}
			target := graph.ResolveCallTarget(callerInfo, *relation)
			if target == nil {
				continue
			}
			if resolved != nil && resolved != target {
				ambiguous = true
				break
			}
			resolved = target
		}
		if ambiguous || resolved == nil || resolved.Line <= 0 || resolved.EndLine < resolved.Line {
			continue
		}
		calleeFile := canonicalRelationSourcePath(resolved.File)
		calleeInfo := graph.FileIndex[calleeFile]
		if calleeInfo == nil || !relationSourceInRequestedScope(calleeFile, ctx.AnalysisIR.RequestModel) {
			continue
		}
		for _, site := range index.relationsByFile[calleeFile] {
			relation := site.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" {
				continue
			}
			line := max(relation.Line, relation.FromEP.Line, relation.ToEP.Line)
			if line < resolved.Line || line > resolved.EndLine {
				continue
			}
			surfaces := append(flowRepairRelationEndpointSurfaces(relation.FromEP),
				flowRepairRelationEndpointSurfaces(relation.ToEP)...)
			surfaces = append(surfaces, site.ownerSurfaces...)
			matchRank := 0
			for _, group := range missingParticipantSurfaceGroups {
				// Rank 1 is planning-only substring affinity. Continuing a
				// grounded frontier requires at least a typed-compatible endpoint.
				matchRank = max(matchRank, flowRepairPlanningSurfaceMatchRank(group, surfaces))
			}
			if matchRank < 2 {
				continue
			}
			key := strings.ToLower(calleeFile + "\x00" + strings.TrimSpace(resolved.Name) + "\x00" + strconv.Itoa(line))
			if seen[key] {
				continue
			}
			seen[key] = true
			start := line - flowOperationRepairReadRadius
			if start < 1 {
				start = 1
			}
			candidates = append(candidates, candidate{
				target: flowOperationRepairReadTarget{
					file: calleeFile, lineRange: types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
					alreadyRead: closure.HasReadLine(calleeFile, line), receivingCallableBody: true,
				},
				matchRank: matchRank,
				line:      line,
			})
		}
	}
	if len(candidates) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].matchRank != candidates[j].matchRank {
			return candidates[i].matchRank > candidates[j].matchRank
		}
		if candidates[i].target.alreadyRead != candidates[j].target.alreadyRead {
			return candidates[i].target.alreadyRead
		}
		if candidates[i].target.file != candidates[j].target.file {
			return candidates[i].target.file < candidates[j].target.file
		}
		return candidates[i].line < candidates[j].line
	})
	return candidates[0].target, true
}

// flowNavigationCarrierArgumentValueRank is a SOFT preference among already
// parser-owned complete arguments. A binding used as the whole value
// (`owner.busCtx`) is a more direct carrier handoff than a value derived from
// it (`owner.busCtx.Language`). The latter remains eligible because nested
// member projection is a real and common flow shape. This rank selects only a
// read coordinate and never proves either transfer.
func flowNavigationCarrierArgumentValueRank(arguments []string, binding string) int {
	best := 0
	for _, argument := range arguments {
		argument = strings.TrimSpace(argument)
		if !flowNavigationArgumentMatchesBinding(argument, binding) {
			continue
		}
		if types.AnswerCodeIdentitySurfacesCompatible(binding, argument) {
			best = max(best, 2)
			continue
		}
		best = max(best, 1)
	}
	return best
}

const flowNavigationCallResultContinuationMaxHops = 4

// flowNavigationCallResultContinuationReadTarget follows an already-read
// call-assignment result to a later call that consumes that exact receiver in
// the same parser-owned enclosing callable. Repeating the step covers common
// dispatcher pipelines such as build context -> execute agent -> apply output
// without reading an entire large function or stopping at its first carrier
// occurrence. All supported language extractors publish the same callable and
// call relation shapes; source parsing is limited to the shared conservative
// assignment/complete-argument helpers.
//
// This is navigation only. The receiver roster may contain every LHS of a
// multi-result assignment, and neither it nor a selected read coordinate is
// evidence of transfer. The model must inspect and emit every relation row.
func flowNavigationCallResultContinuationReadTarget(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	relation *repotypes.Relation,
	ownerSurfaces []string,
) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.Mutable == nil || index == nil || relation == nil ||
		strings.TrimSpace(relation.Kind) != "call" {
		return flowOperationRepairReadTarget{}, false
	}
	file := canonicalRelationSourcePath(firstNonEmptyFlowRepairString(
		relation.File,
	))
	if file == "" {
		for candidateFile, sites := range index.relationsByFile {
			for _, site := range sites {
				if site.relation == relation {
					file = candidateFile
					break
				}
			}
			if file != "" {
				break
			}
		}
	}
	line := max(relation.Line, relation.FromEP.Line, relation.ToEP.Line)
	lines := flowNavigationSourceLines(ctx, index, file)
	if file == "" || line <= 0 || len(lines) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	if len(ownerSurfaces) == 0 {
		ownerSurfaces = flowRepairRelationEndpointSurfaces(relation.FromEP)
	}
	receivers := types.AssignmentNavigationReceiverCandidates(types.EvidenceItem{
		AnchorKind: types.AnchorAssignment,
		Snippet:    lines[line],
	})
	if len(receivers) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	closure := ctx.Mutable.EvidenceClosure()
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	gc := &ground.Context{
		Graph: graph, RepoRoot: ctx.RepoRoot,
		LineIndex: map[string]map[int]string{file: lines},
	}
	currentLine := line
	for hop := 0; hop < flowNavigationCallResultContinuationMaxHops; hop++ {
		type continuation struct {
			site flowParserRelationSite
			line int
		}
		var candidates []continuation
		seenLine := map[int]bool{}
		for _, site := range index.relationsByFile[file] {
			next := site.relation
			if next == nil || strings.TrimSpace(next.Kind) != "call" ||
				flowRepairPlanningSurfaceMatchRank(ownerSurfaces, site.ownerSurfaces) == 0 {
				continue
			}
			nextLine := max(next.Line, next.FromEP.Line, next.ToEP.Line)
			if nextLine <= currentLine || seenLine[nextLine] {
				continue
			}
			callee := flowNavigationCallReceiver(next)
			if callee == "" {
				continue
			}
			matched := false
			for _, flow := range ground.DetectArgumentFlowsAtLine(gc, file, nextLine, callee) {
				for _, receiver := range receivers {
					if flowNavigationArgumentMatchesBinding(flow.Argument, receiver) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if matched {
				seenLine[nextLine] = true
				candidates = append(candidates, continuation{site: site, line: nextLine})
			}
		}
		if len(candidates) == 0 {
			return flowOperationRepairReadTarget{}, false
		}
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].line < candidates[j].line })
		advanced := false
		for _, candidate := range candidates {
			if !closure.HasReadLine(file, candidate.line) {
				start := candidate.line - flowOperationRepairReadRadius
				if start < 1 {
					start = 1
				}
				return flowOperationRepairReadTarget{
					file: file, lineRange: types.LineRange{Start: start, End: candidate.line + flowOperationRepairReadRadius},
				}, true
			}
			nextReceivers := types.AssignmentNavigationReceiverCandidates(types.EvidenceItem{
				AnchorKind: types.AnchorAssignment,
				Snippet:    lines[candidate.line],
			})
			if len(nextReceivers) == 0 {
				continue
			}
			currentLine = candidate.line
			receivers = nextReceivers
			advanced = true
			break
		}
		if !advanced {
			return flowOperationRepairReadTarget{}, false
		}
	}
	return flowOperationRepairReadTarget{}, false
}

// flowNavigationBindingRelationSites keeps complete-argument discovery on the
// declaration file, then extends it to parser-proved methods of the same
// owning type. Fields commonly live in one source file while methods that hand
// them to another component live in sibling files (Go receiver methods,
// partial types, header/implementation splits). Restricting argument scans to
// the declaration file made those real handoffs invisible and repeatedly sent
// completion repair to unrelated local helpers.
//
// The extension is navigation-only and deliberately precise: the parser must
// publish a non-empty static owner for the binding, an exact owner identity on
// the enclosing callable, the same language, and either the same package or
// the same source directory. It never creates evidence or relation authority.
func flowNavigationBindingRelationSites(
	index *flowNavigationIndex,
	graph *repotypes.Graph,
	binding flowDeclaredBindingSite,
) []flowParserRelationSite {
	if index == nil {
		return nil
	}
	var out []flowParserRelationSite
	seen := make(map[*repotypes.Relation]int)
	appendSite := func(site flowParserRelationSite, carrierOwnerBridgeRank int) {
		if site.relation == nil {
			return
		}
		site.carrierOwnerBridgeRank = carrierOwnerBridgeRank
		if idx, ok := seen[site.relation]; ok {
			if out[idx].carrierOwnerBridgeRank < carrierOwnerBridgeRank {
				out[idx].carrierOwnerBridgeRank = carrierOwnerBridgeRank
			}
			return
		}
		seen[site.relation] = len(out)
		out = append(out, site)
	}
	for _, site := range index.relationsByFile[binding.file] {
		appendSite(site, 0)
	}
	owner := strings.TrimSpace(binding.owner)
	if owner == "" || graph == nil {
		return out
	}
	declFile := graph.FileIndex[binding.file]
	if declFile == nil {
		return out
	}
	appendOwnerSites := func(staticOwner, ownerFile string, carrierOwnerBridgeRank int) {
		ownerInfo := graph.FileIndex[ownerFile]
		if ownerInfo == nil {
			return
		}
		for _, key := range flowNavigationSurfaceKeys(staticOwner) {
			for _, site := range index.relationsByOwnerKey[key] {
				candidateFile := graph.FileIndex[site.file]
				if candidateFile == nil || candidateFile.Language != ownerInfo.Language ||
					!flowAnyIdentitySurfaceMatches(site.ownerSurfaces, staticOwner) {
					continue
				}
				samePackage := strings.TrimSpace(ownerInfo.Package) != "" &&
					strings.TrimSpace(ownerInfo.Package) == strings.TrimSpace(candidateFile.Package)
				sameDirectory := filepath.ToSlash(filepath.Dir(ownerFile)) ==
					filepath.ToSlash(filepath.Dir(site.file))
				if samePackage || sameDirectory {
					appendSite(site, carrierOwnerBridgeRank)
				}
			}
		}
	}
	appendOwnerSites(owner, binding.file, 0)

	// A nested state member is often consumed through another typed field:
	// Mutable belongs to BusContext, while an Orchestrator field of static type
	// BusContext carries `o.busCtx.Mutable` into a sink. Follow exactly one such
	// parser-owned declaration hop, then inspect operations owned by the outer
	// declaration. This is still only a navigation expansion: both declarations
	// need exact static types and the eventual complete argument must independently
	// match the original binding before it can become a candidate.
	seenBridge := make(map[string]bool)
	for _, bridge := range flowNavigationSymbols(index, []string{owner}) {
		if bridge.symbol == nil || strings.TrimSpace(bridge.symbol.DeclaredType) == "" ||
			!flowRepairSymbolMatchesAnySurface(*bridge.symbol, []string{owner}) {
			continue
		}
		bridgeOwner := strings.TrimSpace(firstNonEmptyFlowRepairString(bridge.symbol.Parent, bridge.symbol.Receiver))
		bridgeFile := canonicalRelationSourcePath(bridge.file)
		key := strings.ToLower(bridgeFile + "\x00" + bridgeOwner)
		if bridgeOwner == "" || bridgeFile == "" || seenBridge[key] {
			continue
		}
		seenBridge[key] = true
		appendOwnerSites(bridgeOwner, bridgeFile, 1)
	}
	return out
}

// flowNavigationCalleeMutationReadTarget follows one already-read carrier
// handoff into a unique parser-owned callee definition and selects an exact
// AST-tagged assignment/member-initializer line mentioning the still-missing
// participant. This is the language-neutral second hop after an argument
// handoff: it helps the model inspect whether the callee actually stores or
// propagates the value instead of repeatedly rereading the caller.
//
// The target is navigation only. LineFeatures establish source shape, not
// data-flow semantics; the explorer must read the line and emit a separately
// grounded assignment/initializer row. Ambiguous callees, missing AST
// features, out-of-scope definitions, and already-read mutations fail closed.
func flowNavigationCalleeMutationReadTarget(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	relation *repotypes.Relation,
	participantSurfaceGroups [][]string,
) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || index == nil || relation == nil ||
		strings.TrimSpace(relation.Kind) != "call" || len(participantSurfaceGroups) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	callee := strings.TrimSpace(relation.ToEP.Name)
	if callee == "" {
		return flowOperationRepairReadTarget{}, false
	}
	var definitions []flowParserSymbolSite
	seen := make(map[*repotypes.Symbol]bool)
	for _, site := range flowNavigationSymbols(index, []string{callee}) {
		symbol := site.symbol
		if symbol == nil || seen[symbol] ||
			(symbol.Kind != "function" && symbol.Kind != "method") ||
			!types.AnswerCodeIdentitySurfacesEquivalent(callee, symbol.Name) ||
			!relationSourceInRequestedScope(site.file, ctx.AnalysisIR.RequestModel) ||
			symbol.Line <= 0 || symbol.EndLine < symbol.Line {
			continue
		}
		seen[symbol] = true
		definitions = append(definitions, site)
	}
	if len(definitions) != 1 {
		return flowOperationRepairReadTarget{}, false
	}
	definition := definitions[0]
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return flowOperationRepairReadTarget{}, false
	}
	file := graph.FileIndex[definition.file]
	if file == nil || len(file.LineFeatures) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	lines := flowNavigationSourceLines(ctx, index, definition.file)
	if len(lines) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	wanted := make(map[string]bool)
	for _, group := range participantSurfaceGroups {
		for _, surface := range group {
			for _, key := range flowParticipantSymbolLookupKeys([]string{surface}) {
				if key = flowRepairPlanningKey(key); len(key) >= 4 {
					wanted[key] = true
				}
			}
		}
	}
	if len(wanted) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	closure := ctx.Mutable.EvidenceClosure()
	for line := definition.symbol.Line; line <= definition.symbol.EndLine; line++ {
		features := file.LineFeatures[line]
		shape := false
		for _, feature := range features {
			if feature == repotypes.LineFeatureAssignment || feature == repotypes.LineFeatureMemberInitializer {
				shape = true
				break
			}
		}
		if !shape || closure.HasReadLine(definition.file, line) {
			continue
		}
		matched := false
		for _, token := range flowNavigationIdentityTokens(lines[line]) {
			if wanted[flowRepairPlanningKey(token)] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		start := line - flowOperationRepairReadRadius
		if start < 1 {
			start = 1
		}
		return flowOperationRepairReadTarget{
			file: definition.file, lineRange: types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
		}, true
	}
	return flowOperationRepairReadTarget{}, false
}

// flowNavigationRequestedParticipantTouchRank prefers one parser operation
// that touches multiple independently requested participant groups. For a
// typed carrier-argument candidate, the exact declaration contributes its
// participant group; caller/callee endpoints and the parser-split complete
// argument roster may contribute other groups. Including sibling arguments is
// important for generic dispatch APIs whose component identity is passed as a
// typed enum/constant rather than appearing in the callable name. For an
// ordinary relation candidate, only parser endpoints contribute.
//
// The rank remains only a SOFT read coordinate: it neither proves that a
// callee consumes a carrier nor creates evidence, an answer relation, or a
// diagram edge. Participant groups stay separate so spelling aliases of one
// participant cannot masquerade as a cross-component join.
func flowNavigationRequestedParticipantTouchRank(
	relation *repotypes.Relation,
	binding flowDeclaredBindingSite,
	ownerSurfaces []string,
	participantSurfaceGroups [][]string,
) int {
	if relation == nil || len(participantSurfaceGroups) == 0 {
		return 0
	}
	bindingSymbol := repotypes.Symbol{Name: binding.alias, DeclaredType: binding.declaredType}
	endpoints := append(
		flowRepairRelationEndpointSurfaces(relation.FromEP),
		flowRepairRelationEndpointSurfaces(relation.ToEP)...,
	)
	endpoints = append(endpoints, ownerSurfaces...)
	touched := 0
	for _, group := range participantSurfaceGroups {
		if len(group) == 0 {
			continue
		}
		bindingMatches := binding.alias != "" && flowRepairSymbolMatchesAnySurface(bindingSymbol, group)
		if bindingMatches || flowRepairPlanningSurfaceMatchRank(group, endpoints) > 0 {
			touched++
		}
	}
	return touched
}

// flowNavigationCarrierHandoffRank is a SOFT ordering signal for complete
// carrier arguments. A carrier passed to a differently-qualified receiving
// API is a better place to inspect a component handoff than the same carrier
// passed to a bare helper in its current owner. The rank never creates an
// EvidenceItem, relation edge, participant match, or completion authority;
// the explorer still has to read the line and emit the exact syntax-owned
// operation, and an absent bridge remains explicitly unproven.
//
// This intentionally uses parser-owned call endpoints instead of source/prose
// keywords. `this` / `self`, the current owner, and a receiver containing the
// carrier binding are local-use shapes. A distinct qualified receiver is only
// a navigation preference, not proof that the callee stores or propagates the
// argument.
func flowNavigationCarrierHandoffRank(relation *repotypes.Relation, binding string) int {
	if relation == nil || strings.TrimSpace(relation.Kind) != "call" {
		return 0
	}
	target := strings.TrimSpace(relation.ToEP.Receiver)
	if target == "" {
		return 0
	}
	plainTarget := strings.ToLower(strings.Trim(strings.TrimSpace(target), "*&()"))
	if plainTarget == "this" || plainTarget == "self" || plainTarget == "super" {
		return 0
	}
	if strings.TrimSpace(binding) != "" &&
		types.AnswerCodeIdentitySurfacesCompatible(binding, target) {
		return 0
	}
	owner := strings.TrimSpace(relation.FromEP.Receiver)
	if owner != "" && (types.AnswerCodeIdentitySurfacesEquivalent(owner, target) ||
		types.AnswerCodeIdentitySurfacesCompatible(owner, target)) {
		return 0
	}
	return 1
}

// flowRepairPlanningSurfaceMatchRank orders parser-owned navigation sites by
// identity precision. Presentation-equivalent and snake/camel-equivalent
// endpoints outrank owner and short-tail compatibility; substring-like
// planning matches remain a last-resort soft navigation signal. The rank never
// becomes relation evidence or a hard answer gate.
func flowRepairPlanningSurfaceMatchRank(wanted, candidates []string) int {
	best := 0
	for _, left := range wanted {
		for _, right := range candidates {
			rank := 0
			switch {
			case types.AnswerCodeIdentitySurfacesEquivalent(left, right):
				rank = 5
			case flowRepairPlanningKey(left) != "" && flowRepairPlanningKey(left) == flowRepairPlanningKey(right):
				rank = 4
			case types.AnswerCodeIdentityOwnsEndpoint(left, right):
				rank = 3
			case types.AnswerCodeIdentitySurfacesCompatible(left, right):
				rank = 2
			case flowRepairPlanningSurfaceMatches(left, right):
				rank = 1
			}
			if rank > best {
				best = rank
			}
		}
	}
	return best
}

func flowOperationRepairRelationKindRank(kind string) int {
	switch strings.TrimSpace(kind) {
	case "call":
		return 3
	case "type_usage":
		return 2
	case "reference":
		return 1
	default:
		return 0
	}
}

// flowRepairParserRelationTargets adds exact parser-observed incident sites to
// the SOFT navigation plan. A declaration often answers only "what is this
// actor?", while the registration, allowlist, callback, or selection operation
// lives at a reference/type-usage/call site in another file. Those sites are
// useful places for Explorer to read next, but they are not relation evidence:
// the model must still read the operation and emit its exact syntax-owned
// endpoints through emit_evidence before completion or diagram validation can
// treat any edge as proved.
//
// Endpoint matching deliberately reuses the existing planning-only normalized
// matcher, so display identities such as snake_case tool names can navigate to
// parser identities such as CamelCase types. This fuzzy bridge never enters a
// hard gate, EvidenceItem, diagram recipe, answer, or conclusion.
func flowRepairParserRelationTargets(ctx *types.BusContext, rm types.RequestModel, surfaces []string) ([]string, []string) {
	if ctx == nil || ctx.Mutable == nil || len(surfaces) == 0 {
		return nil, nil
	}
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		return nil, nil
	}
	var files, aliases []string
	for _, site := range flowNavigationRelationSites(index, surfaces) {
		path := site.file
		if path == "" || !relationSourceInRequestedScope(path, rm) || site.relation == nil {
			continue
		}
		relation := site.relation
		from := append(flowRepairRelationEndpointSurfaces(relation.FromEP), site.ownerSurfaces...)
		to := flowRepairRelationEndpointSurfaces(relation.ToEP)
		if !flowRepairAnyPlanningSurfaceMatches(surfaces, from) &&
			!flowRepairAnyPlanningSurfaceMatches(surfaces, to) {
			continue
		}
		file := path
		files = appendUniqueBounded(files, []string{file}, maxFlowOperationRepairFiles)
		aliases = appendUniqueBounded(aliases, append(from, to...), maxFlowOperationRepairKeywords)
		if len(files) >= maxFlowOperationRepairFiles && len(aliases) >= maxFlowOperationRepairKeywords {
			return files, aliases
		}
	}
	return files, aliases
}

func flowRepairRelationEndpointSurfaces(endpoint repotypes.RelationEndpoint) []string {
	name := strings.TrimSpace(endpoint.Name)
	receiver := strings.TrimSpace(endpoint.Receiver)
	out := appendUniqueBounded(nil, []string{name, receiver}, maxFlowOperationRepairKeywords)
	if name != "" && receiver != "" {
		out = appendUniqueBounded(out, []string{receiver + "." + name}, maxFlowOperationRepairKeywords)
	}
	return out
}

func flowRepairAnyPlanningSurfaceMatches(wanted, candidates []string) bool {
	for _, left := range wanted {
		for _, right := range candidates {
			if types.AnswerCodeIdentitySurfacesCompatible(left, right) ||
				flowRepairPlanningSurfaceMatches(left, right) {
				return true
			}
		}
	}
	return false
}

func firstNonEmptyFlowRepairString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// flowRepairDeclaredBindingTargets projects exact parser-owned declaration
// bindings into the SOFT navigation lane. A user-visible carrier is commonly a
// type (`BusContext`) while operation sites use a field/local binding
// (`busCtx`). Searching only the type name repeatedly lands on declarations and
// misses calls such as BuildAgentContext(o.busCtx, ...).
//
// These aliases are navigation coordinates only. They never create an
// EvidenceItem, satisfy participant coverage, or authorize a diagram edge;
// Explorer must still read an operation line and emit it through the ordinary
// exact grounder. Source-scope filtering prevents a test/example homonym from
// steering a production repair.
func flowRepairDeclaredBindingTargets(ctx *types.BusContext, rm types.RequestModel, surfaces []string) ([]string, []string) {
	if ctx == nil || ctx.Mutable == nil || len(surfaces) == 0 {
		return nil, nil
	}
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		return nil, nil
	}
	files := make([]string, 0, maxFlowOperationRepairFiles)
	aliases := make([]string, 0, maxFlowOperationRepairKeywords)
	for _, site := range flowRepairDeclaredBindingSites(index, rm, surfaces) {
		files = appendUniqueBounded(files, []string{site.file}, maxFlowOperationRepairFiles)
		aliases = appendUniqueBounded(aliases, []string{site.alias}, maxFlowOperationRepairKeywords)
	}
	return files, aliases
}

func flowRepairDeclaredBindingSites(index *flowNavigationIndex, rm types.RequestModel, surfaces []string) []flowDeclaredBindingSite {
	if index == nil || len(surfaces) == 0 {
		return nil
	}
	var out []flowDeclaredBindingSite
	seen := make(map[string]bool)
	for _, site := range flowNavigationSymbols(index, surfaces) {
		symbol := site.symbol
		path := site.file
		if symbol == nil || !relationSourceInRequestedScope(path, rm) {
			continue
		}
		if !flowRepairSymbolMatchesAnySurface(*symbol, surfaces) {
			continue
		}
		alias := strings.TrimSpace(symbol.Name)
		key := strings.ToLower(path + "\x00" + alias)
		if path == "" || alias == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, flowDeclaredBindingSite{
			file: path, alias: alias, declaredType: strings.TrimSpace(symbol.DeclaredType),
			owner: strings.TrimSpace(firstNonEmptyFlowRepairString(symbol.Parent, symbol.Receiver)),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].alias < out[j].alias
	})
	bounded := make([]flowDeclaredBindingSite, 0, min(len(out), maxFlowOperationRepairFiles*maxFlowOperationRepairKeywords))
	files := make(map[string]bool)
	aliases := make(map[string]bool)
	for _, site := range out {
		fileKey := strings.ToLower(site.file)
		aliasKey := strings.ToLower(site.alias)
		if (!files[fileKey] && len(files) >= maxFlowOperationRepairFiles) ||
			(!aliases[aliasKey] && len(aliases) >= maxFlowOperationRepairKeywords) {
			continue
		}
		files[fileKey] = true
		aliases[aliasKey] = true
		bounded = append(bounded, site)
	}
	return bounded
}

func flowRepairSymbolMatchesAnySurface(symbol repotypes.Symbol, surfaces []string) bool {
	name := strings.TrimSpace(symbol.Name)
	declaredType := strings.TrimSpace(symbol.DeclaredType)
	if name == "" {
		return false
	}
	for _, surface := range surfaces {
		if types.AnswerCodeIdentitySurfacesCompatible(surface, name) ||
			(declaredType != "" && types.AnswerCodeIdentitySurfacesCompatible(surface, declaredType)) {
			return true
		}
	}
	return false
}

func flowRepairItemMatchesAnySurface(item types.EvidenceItem, surfaces []string) bool {
	for _, surface := range surfaces {
		for _, endpoint := range []string{item.Subject, item.Object, item.AnchorSymbol, item.OwnerSymbol} {
			if types.AnswerCodeIdentitySurfacesCompatible(surface, endpoint) || flowRepairPlanningSurfaceMatches(surface, endpoint) {
				return true
			}
		}
	}
	return false
}

func flowRepairPlanningSurfaceMatches(left, right string) bool {
	left = flowRepairPlanningKey(left)
	right = flowRepairPlanningKey(right)
	if len(left) < 4 || len(right) < 4 {
		return false
	}
	return strings.Contains(left, right) || strings.Contains(right, left)
}

func flowRepairPlanningKey(raw string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(raw))
}

func appendUniqueBounded(dst, values []string, limit int) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	for _, existing := range dst {
		seen[strings.ToLower(strings.TrimSpace(existing))] = true
	}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		key := strings.ToLower(value)
		if value == "" || seen[key] || len(dst) >= limit {
			continue
		}
		seen[key] = true
		dst = append(dst, value)
	}
	return dst
}
