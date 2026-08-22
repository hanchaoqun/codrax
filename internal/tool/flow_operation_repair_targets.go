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
	flowNavigationIndexCacheKey    = "flow_navigation_index:v7"
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

// flowValueConsumerRepair is one parser-owned continuation coordinate for a
// model-selected call-result assignment.  It is repair/navigation state only:
// the exact argument handoff still has to be inspected and emitted by the
// model before it can become relation or diagram authority.
type flowValueConsumerRepair struct {
	target         flowOperationRepairReadTarget
	argument       string
	receiver       string
	producerSource string
	producerLine   int
	consumerLine   int
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
	continuationMu       sync.Mutex
	// callResultContinuationByOwner caches all call-site continuation depths
	// for one exact (file, typed owner-surface query) domain. A whole owner DAG
	// is solved in one reverse pass; individual ranking call sites then read O(1).
	callResultContinuationByOwner map[string]map[*repotypes.Relation]int
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
		symbolsByKey:                  make(map[string][]flowParserSymbolSite),
		relationsByKey:                make(map[string][]flowParserRelationSite),
		relationsByToken:              make(map[string][]flowParserRelationSite),
		relationsByFile:               make(map[string][]flowParserRelationSite),
		relationsByOwnerKey:           make(map[string][]flowParserRelationSite),
		relationsByTarget:             make(map[*repotypes.Symbol][]flowParserRelationSite),
		sourceLinesByFile:             make(map[string]map[int]string),
		sourceLinesAttempted:          make(map[string]bool),
		callResultContinuationByOwner: make(map[string]map[*repotypes.Relation]int),
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
	var participantSurfaceGroupMissing []bool
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
				participantSurfaceGroupMissing = append(participantSurfaceGroupMissing,
					len(wanted) == 0 || wanted[strings.ToLower(strings.TrimSpace(participant.Identity))])
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
	// When the model has already grounded a carrier handoff, first inspect the
	// uniquely resolved receiver for an un-emitted value-bearing operation that
	// can connect the still-missing participant. Caller-side result continuation
	// remains the next preference after that receiver frontier is exhausted.
	if target, ok := flowNavigationGroundedHandoffCalleeOperationReadTarget(
		ctx, index, missingParticipantSurfaceGroups, evidence,
	); ok {
		return target, true
	}
	if target, ok := flowNavigationGroundedCallResultContinuationReadTarget(
		ctx, index, participantSurfaceGroups, evidence,
	); ok {
		return target, true
	}
	if target, ok := flowNavigationGroundedBodyOperationCallerHandoffReadTarget(
		ctx, index, participantSurfaceGroups, missingParticipantSurfaceGroups, evidence,
	); ok {
		return target, true
	}
	type rankedTarget struct {
		target        flowOperationRepairReadTarget
		relation      *repotypes.Relation
		ownerSurfaces []string
		// connectionGainRank distinguishes an operation that joins at
		// least one still-missing participant to an already-covered
		// requested participant from an equally broad local operation
		// whose touched participants are all still disconnected.
		connectionGainRank int
		// participantTouchRank counts distinct requested participant groups
		// touched by this one parser-owned operation coordinate. It ranks a
		// real cross-participant receiver/caller site ahead of a ubiquitous
		// context argument used by an unrelated helper.
		participantTouchRank int
		// groundedSourceRank keeps an exact source file which already carries
		// citable evidence for the still-missing participant in the bounded
		// candidate set. This is adaptive SOFT navigation only: the existing
		// evidence does not authorize any un-emitted operation from that file.
		groundedSourceRank     int
		carrierOwnerBridgeRank int
		// carrierValueRank distinguishes handing off the carrier itself from
		// handing off one derived member. Both are useful navigation, but an
		// exact whole-carrier transfer is the stronger next place to inspect.
		carrierValueRank int
		carrierRank      int
		handoffRank      int
		// resultContinuationRank is a parser-owned SOFT navigation score.
		// A call whose assigned result is later consumed as a whole value by
		// another call is a more useful pipeline coordinate than an otherwise
		// equal liveness/probe call whose result is only projected by member.
		// It never becomes relation or runtime-order authority.
		resultContinuationRank int
		matchRank              int
		kindRank               int
		line                   int
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
			connectionGainRank: flowNavigationRequestedParticipantConnectionGainRank(
				relation, flowDeclaredBindingSite{}, site.ownerSurfaces,
				participantSurfaceGroups, participantSurfaceGroupMissing,
			),
			participantTouchRank: flowNavigationRequestedParticipantTouchRank(
				relation, flowDeclaredBindingSite{}, site.ownerSurfaces, participantSurfaceGroups,
			),
			groundedSourceRank: flowNavigationGroundedParticipantSourceRank(
				evidence, file, surfaces,
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
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	for _, group := range missingParticipantSurfaceGroups {
		for _, binding := range flowNavigationDeclaredBindingSitesForRepair(
			index, graph, rm, group, participantSurfaceGroups, evidence,
		) {
			key := strings.ToLower(binding.file + "\x00" + binding.alias + "\x00" + binding.owner)
			if seenBindingSites[key] {
				continue
			}
			seenBindingSites[key] = true
			bindingSites = append(bindingSites, binding)
		}
	}
	for _, binding := range bindingSites {
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
				connectionGainRank: flowNavigationRequestedParticipantConnectionGainRank(
					relation, binding,
					append(append(append([]string(nil), site.ownerSurfaces...), binding.owner), argumentSurfaces...),
					participantSurfaceGroups, participantSurfaceGroupMissing,
				),
				participantTouchRank: flowNavigationRequestedParticipantTouchRank(
					relation, binding,
					append(append(append([]string(nil), site.ownerSurfaces...), binding.owner), argumentSurfaces...),
					participantSurfaceGroups,
				),
				groundedSourceRank: flowNavigationGroundedParticipantSourceRank(
					evidence, site.file, surfaces,
				),
				carrierOwnerBridgeRank: site.carrierOwnerBridgeRank,
				carrierValueRank:       flowNavigationCarrierArgumentValueRank(argumentSurfaces, binding.alias),
				carrierRank:            1,
				handoffRank:            flowNavigationCarrierHandoffRank(relation, binding.alias),
				resultContinuationRank: flowNavigationCallResultContinuationDepth(
					ctx, index, relation, site.ownerSurfaces,
				),
				matchRank: argumentRank,
				kindRank:  flowOperationRepairRelationKindRank(relation.Kind),
				line:      line,
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
		if candidates[i].connectionGainRank != candidates[j].connectionGainRank {
			return candidates[i].connectionGainRank > candidates[j].connectionGainRank
		}
		if candidates[i].participantTouchRank != candidates[j].participantTouchRank {
			return candidates[i].participantTouchRank > candidates[j].participantTouchRank
		}
		if candidates[i].groundedSourceRank != candidates[j].groundedSourceRank {
			return candidates[i].groundedSourceRank > candidates[j].groundedSourceRank
		}
		if candidates[i].resultContinuationRank != candidates[j].resultContinuationRank {
			return candidates[i].resultContinuationRank > candidates[j].resultContinuationRank
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
			ctx, index, selected.relation, selected.ownerSurfaces, evidence,
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
		target                 flowOperationRepairReadTarget
		bodyRank               int
		resultContinuationRank int
		missingRank            int
		participantRank        int
		callerLine             int
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
					bodyRank: bodyRank,
					resultContinuationRank: flowNavigationCallResultContinuationDepth(
						ctx, index, relation, site.ownerSurfaces,
					),
					missingRank: missingRank, participantRank: participantRank, callerLine: line,
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
		if candidates[i].resultContinuationRank != candidates[j].resultContinuationRank {
			return candidates[i].resultContinuationRank > candidates[j].resultContinuationRank
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
		target flowOperationRepairReadTarget
		// operationRank prefers an exact parser-tagged assignment/member
		// initializer inside the uniquely resolved receiving callable over a
		// local member call that merely mentions the same participant.  Both
		// remain navigation-only; the source line still has to be inspected and
		// emitted by the model before it can authorize a relation.
		operationRank          int
		matchRank              int
		resultContinuationRank int
		line                   int
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
		// A grounded argument handoff already supplies the caller -> receiving
		// callable half of the component frontier. Prefer an exact value-bearing
		// operation inside that callable (assignment/member initializer) before a
		// getter or other local call. The former can connect the carried value to
		// the missing participant; the latter proves only a local operation and
		// was repeatedly selected ahead of the real binding in production.
		lines := flowNavigationSourceLines(ctx, index, calleeFile)
		for line := resolved.Line; line <= resolved.EndLine && len(lines) > 0; line++ {
			features := calleeInfo.LineFeatures[line]
			anchorKind := types.AnchorKind("")
			for _, feature := range features {
				switch feature {
				case repotypes.LineFeatureMemberInitializer:
					anchorKind = types.AnchorInitializer
				case repotypes.LineFeatureAssignment:
					if anchorKind == "" {
						anchorKind = types.AnchorAssignment
					}
				}
			}
			if anchorKind == "" {
				continue
			}
			operation := types.EvidenceItem{AnchorKind: anchorKind, Snippet: lines[line]}
			receiver, value, ok := types.AssignmentEvidenceEndpoints(operation)
			if !ok || flowNavigationAssignmentOperationAlreadyEmitted(
				evidence, calleeFile, line, receiver, value,
			) {
				continue
			}
			matchRank := 0
			for _, group := range missingParticipantSurfaceGroups {
				matchRank = max(matchRank, flowRepairPlanningSurfaceMatchRank(
					group, []string{receiver, value},
				))
			}
			// Substring-only affinity is insufficient for this precise fast
			// path. A typed-compatible receiver/value endpoint is required.
			if matchRank < 2 {
				continue
			}
			key := strings.ToLower(calleeFile + "\x00" + strings.TrimSpace(resolved.Name) + "\x00mutation\x00" + strconv.Itoa(line))
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
					focusIdentity: receiver + " <- " + value,
					alreadyRead:   closure.HasReadLine(calleeFile, line), receivingCallableBody: true,
				},
				operationRank: 2, matchRank: matchRank, line: line,
			})
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
			if flowNavigationCallOperationAlreadyEmitted(
				evidence, calleeFile, line, site.ownerSurfaces, flowNavigationCallReceiver(relation),
			) {
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
				operationRank: 1, matchRank: matchRank,
				resultContinuationRank: flowNavigationCallResultContinuationDepth(
					ctx, index, relation, site.ownerSurfaces,
				),
				line: line,
			})
		}
	}
	if len(candidates) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].operationRank != candidates[j].operationRank {
			return candidates[i].operationRank > candidates[j].operationRank
		}
		if candidates[i].matchRank != candidates[j].matchRank {
			return candidates[i].matchRank > candidates[j].matchRank
		}
		if candidates[i].resultContinuationRank != candidates[j].resultContinuationRank {
			return candidates[i].resultContinuationRank > candidates[j].resultContinuationRank
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

func flowNavigationAssignmentOperationAlreadyEmitted(
	evidence []types.EvidenceItem,
	source string,
	line int,
	receiver, value string,
) bool {
	source = canonicalRelationSourcePath(source)
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimAssignmentFact ||
			canonicalRelationSourcePath(item.Source) != source || item.LineStart != line {
			continue
		}
		gotReceiver, gotValue, ok := types.AssignmentEvidenceEndpoints(item)
		if ok && types.AnswerCodeIdentitySurfacesEquivalent(gotReceiver, receiver) &&
			types.AnswerCodeIdentitySurfacesEquivalent(gotValue, value) {
			return true
		}
	}
	return false
}

func flowNavigationCallOperationAlreadyEmitted(
	evidence []types.EvidenceItem,
	source string,
	line int,
	ownerSurfaces []string,
	callee string,
) bool {
	source = canonicalRelationSourcePath(source)
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimCallEdge ||
			canonicalRelationSourcePath(item.Source) != source || item.LineStart != line ||
			!flowAnyIdentitySurfaceMatches([]string{item.Object, item.AnchorSymbol}, callee) {
			continue
		}
		if len(ownerSurfaces) == 0 || flowAnyIdentitySurfaceMatches(ownerSurfaces, item.Subject) {
			return true
		}
	}
	return false
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

// flowNavigationCallResultContinuationDepth ranks parser-owned call sites by
// the length of an exact assignment-result -> whole-argument chain. It is a
// SOFT navigation score only. Every hop requires:
//   - an assignment/initializer receiver on the current call line;
//   - a later parser-owned call in the same enclosing callable; and
//   - that receiver as one complete argument of the later call.
//
// Member projections deliberately do not count. Thus a liveness check such as
// `ctx := Build(...); len(ctx.Rows)` cannot outrank a dispatcher path such as
// `ctx := Build(...); output := Execute(ctx); Apply(output)`. The bounded score
// neither proves transfer nor runtime order and never creates evidence.
func flowNavigationCallResultContinuationDepth(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	relation *repotypes.Relation,
	ownerSurfaces []string,
) int {
	if ctx == nil || ctx.Mutable == nil || index == nil || relation == nil ||
		strings.TrimSpace(relation.Kind) != "call" {
		return 0
	}
	file := canonicalRelationSourcePath(strings.TrimSpace(relation.File))
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
		return 0
	}
	if len(ownerSurfaces) == 0 {
		ownerSurfaces = flowRepairRelationEndpointSurfaces(relation.FromEP)
	}
	if len(types.AssignmentNavigationReceiverCandidates(types.EvidenceItem{
		AnchorKind: types.AnchorAssignment,
		Snippet:    lines[line],
	})) == 0 {
		return 0
	}
	ownerKeyParts := make([]string, 0, len(ownerSurfaces))
	for _, surface := range ownerSurfaces {
		if key := types.AnswerCodeIdentitySurfaceKey(surface); key != "" {
			ownerKeyParts = append(ownerKeyParts, key)
		}
	}
	if len(ownerKeyParts) == 0 {
		for _, surface := range ownerSurfaces {
			if surface = strings.ToLower(strings.TrimSpace(surface)); surface != "" {
				ownerKeyParts = append(ownerKeyParts, "raw="+surface)
			}
		}
	}
	sort.Strings(ownerKeyParts)
	ownerKeyParts = compactSortedStrings(ownerKeyParts)
	cacheKey := file + "\x1e" + strings.Join(ownerKeyParts, "\x00")

	index.continuationMu.Lock()
	if cached := index.callResultContinuationByOwner[cacheKey]; cached != nil {
		depth := cached[relation]
		index.continuationMu.Unlock()
		return depth
	}
	depths := flowNavigationCallResultContinuationDepthsForOwner(
		ctx, index, file, lines, ownerSurfaces,
	)
	if index.callResultContinuationByOwner == nil {
		index.callResultContinuationByOwner = make(map[string]map[*repotypes.Relation]int)
	}
	index.callResultContinuationByOwner[cacheKey] = depths
	depth := depths[relation]
	index.continuationMu.Unlock()
	return depth
}

type flowNavigationContinuationSite struct {
	relation     *repotypes.Relation
	line         int
	consumesKeys []string
	producesKeys []string
}

// flowNavigationCallResultContinuationDepthsForOwner solves the complete
// later-line value-continuation DAG for one typed owner domain in a single
// reverse pass. For a site S, every produced assignment identity can feed a
// later site T that consumes the same complete argument identity; depth[S] is
// 1+depth[T], bounded by the existing four-hop presentation rank. Sites on the
// same line are evaluated as one group so they never become "later" than one
// another. This remains navigation-only and creates no relation evidence.
func flowNavigationCallResultContinuationDepthsForOwner(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	file string,
	lines map[int]string,
	ownerSurfaces []string,
) map[*repotypes.Relation]int {
	out := make(map[*repotypes.Relation]int)
	if ctx == nil || ctx.Mutable == nil || index == nil || file == "" || len(lines) == 0 {
		return out
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	gc := &ground.Context{
		Graph: graph, RepoRoot: ctx.RepoRoot,
		LineIndex: map[string]map[int]string{file: lines},
	}
	sites := make([]flowNavigationContinuationSite, 0, len(index.relationsByFile[file]))
	seen := make(map[*repotypes.Relation]bool)
	for _, indexed := range index.relationsByFile[file] {
		relation := indexed.relation
		if relation == nil || seen[relation] || strings.TrimSpace(relation.Kind) != "call" ||
			flowRepairPlanningSurfaceMatchRank(ownerSurfaces, indexed.ownerSurfaces) == 0 {
			continue
		}
		seen[relation] = true
		line := max(relation.Line, relation.FromEP.Line, relation.ToEP.Line)
		callee := flowNavigationCallReceiver(relation)
		if line <= 0 || callee == "" {
			continue
		}
		var consumes []string
		for _, flow := range ground.DetectArgumentFlowsAtLine(gc, file, line, callee) {
			if key := types.AnswerCodeIdentitySurfaceKey(flow.Argument); key != "" {
				consumes = append(consumes, key)
			}
		}
		var produces []string
		for _, receiver := range types.AssignmentNavigationReceiverCandidates(types.EvidenceItem{
			AnchorKind: types.AnchorAssignment,
			Snippet:    lines[line],
		}) {
			if key := types.AnswerCodeIdentitySurfaceKey(receiver); key != "" {
				produces = append(produces, key)
			}
		}
		sort.Strings(consumes)
		sort.Strings(produces)
		sites = append(sites, flowNavigationContinuationSite{
			relation: relation, line: line,
			consumesKeys: compactSortedStrings(consumes),
			producesKeys: compactSortedStrings(produces),
		})
	}
	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].line > sites[j].line
	})

	// bestConsumer[key] is the best one-hop-plus-tail score available at a
	// strictly later source line for that exact complete argument identity.
	bestConsumer := make(map[string]int)
	for start := 0; start < len(sites); {
		end := start + 1
		for end < len(sites) && sites[end].line == sites[start].line {
			end++
		}
		updates := make(map[string]int)
		for i := start; i < end; i++ {
			depth := 0
			for _, produced := range sites[i].producesKeys {
				depth = max(depth, bestConsumer[produced])
			}
			depth = min(depth, flowNavigationCallResultContinuationMaxHops)
			out[sites[i].relation] = depth
			continuation := min(1+depth, flowNavigationCallResultContinuationMaxHops)
			for _, consumed := range sites[i].consumesKeys {
				updates[consumed] = max(updates[consumed], continuation)
			}
		}
		for key, depth := range updates {
			bestConsumer[key] = max(bestConsumer[key], depth)
		}
		start = end
	}
	return out
}

func compactSortedStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	for _, value := range in {
		if len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}

// flowNavigationGroundedCallResultContinuationReadTarget resumes a
// model-authored exact argument handoff on the caller side before another
// reverse/receiver-body search can move to a different call site. The starting
// row, call coordinate, complete argument, assignment receiver and every later
// consumer are parser/typed facts. The helper still returns only an extraction
// coordinate; it authors no assignment, argument, call, edge, or conclusion.
func flowNavigationGroundedCallResultContinuationReadTarget(
	ctx *types.BusContext,
	index *flowNavigationIndex,
	participantSurfaceGroups [][]string,
	evidence []types.EvidenceItem,
) (flowOperationRepairReadTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || index == nil || len(participantSurfaceGroups) == 0 || len(evidence) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	type candidate struct {
		relation      *repotypes.Relation
		ownerSurfaces []string
		depth         int
		file          string
		line          int
	}
	var candidates []candidate
	seen := map[*repotypes.Relation]bool{}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return flowOperationRepairReadTarget{}, false
	}
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimArgumentFlow || item.LineStart <= 0 ||
			!relationSourceInRequestedScope(item.Source, ctx.AnalysisIR.RequestModel) {
			continue
		}
		touchesRequested := false
		for _, group := range participantSurfaceGroups {
			for _, endpoint := range []string{item.Subject, item.Object} {
				if diagramParticipantCandidateEndpointMatches(group, endpoint, item, evidence) {
					touchesRequested = true
					break
				}
			}
			if touchesRequested {
				break
			}
		}
		if !touchesRequested {
			continue
		}
		file := canonicalRelationSourcePath(item.Source)
		info := graph.FileIndex[file]
		if info == nil {
			continue
		}
		lines := flowNavigationSourceLines(ctx, index, file)
		if len(lines) == 0 {
			continue
		}
		gc := &ground.Context{
			Graph: graph, RepoRoot: ctx.RepoRoot,
			LineIndex: map[string]map[int]string{file: lines},
		}
		for _, site := range index.relationsByFile[file] {
			relation := site.relation
			if relation == nil || seen[relation] || strings.TrimSpace(relation.Kind) != "call" ||
				max(relation.Line, relation.FromEP.Line, relation.ToEP.Line) != item.LineStart {
				continue
			}
			callee := flowNavigationCallReceiver(relation)
			visibleTarget := callRelationTargetName(graph, info, relation)
			if callee == "" || (!flowAnyIdentitySurfaceMatches([]string{item.Object, item.AnchorSymbol}, visibleTarget) &&
				!flowAnyIdentitySurfaceMatches([]string{item.Object, item.AnchorSymbol}, callee)) {
				continue
			}
			exactArgument := false
			for _, flow := range ground.DetectArgumentFlowsAtLine(gc, file, item.LineStart, callee) {
				if types.AnswerCodeIdentitySurfacesEquivalent(item.Subject, flow.Argument) ||
					types.AnswerCodeIdentitySurfacesCompatible(item.Subject, flow.Argument) {
					exactArgument = true
					break
				}
			}
			if !exactArgument {
				continue
			}
			depth := flowNavigationCallResultContinuationDepth(ctx, index, relation, site.ownerSurfaces)
			if depth <= 0 {
				continue
			}
			seen[relation] = true
			candidates = append(candidates, candidate{
				relation: relation, ownerSurfaces: append([]string(nil), site.ownerSurfaces...),
				depth: depth, file: file, line: item.LineStart,
			})
		}
	}
	if len(candidates) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth > candidates[j].depth
		}
		if candidates[i].file != candidates[j].file {
			return candidates[i].file < candidates[j].file
		}
		return candidates[i].line < candidates[j].line
	})
	for _, candidate := range candidates {
		if target, ok := flowNavigationCallResultContinuationReadTarget(
			ctx, index, candidate.relation, candidate.ownerSurfaces, evidence,
		); ok {
			return target, true
		}
	}
	return flowOperationRepairReadTarget{}, false
}

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
	evidence []types.EvidenceItem,
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
			site         flowParserRelationSite
			line         int
			continuation int
			handoffRank  int
			receiver     string
			callee       string
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
			matchedReceiver := ""
			for _, flow := range ground.DetectArgumentFlowsAtLine(gc, file, nextLine, callee) {
				for _, receiver := range receivers {
					if flowNavigationArgumentMatchesBinding(flow.Argument, receiver) {
						matchedReceiver = receiver
						break
					}
				}
				if matchedReceiver != "" {
					break
				}
			}
			if matchedReceiver != "" {
				seenLine[nextLine] = true
				candidates = append(candidates, continuation{
					site: site, line: nextLine,
					continuation: flowNavigationCallResultContinuationDepth(
						ctx, index, next, site.ownerSurfaces,
					),
					handoffRank: flowNavigationCarrierHandoffRank(next, receivers[0]),
					receiver:    matchedReceiver,
					callee:      callee,
				})
			}
		}
		if len(candidates) == 0 {
			return flowOperationRepairReadTarget{}, false
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].continuation != candidates[j].continuation {
				return candidates[i].continuation > candidates[j].continuation
			}
			if candidates[i].handoffRank != candidates[j].handoffRank {
				return candidates[i].handoffRank > candidates[j].handoffRank
			}
			return candidates[i].line < candidates[j].line
		})
		advanced := false
		for _, candidate := range candidates {
			calleeIdentity := candidate.callee
			if info := graph.FileIndex[file]; info != nil {
				calleeIdentity = firstNonEmptyFlowRepairString(
					callRelationTargetName(graph, info, candidate.site.relation), candidate.callee,
				)
			}
			if !flowNavigationArgumentFlowAlreadyEmitted(
				evidence, file, candidate.line, candidate.receiver, calleeIdentity,
			) {
				start := candidate.line - flowOperationRepairReadRadius
				if start < 1 {
					start = 1
				}
				return flowOperationRepairReadTarget{
					file: file, lineRange: types.LineRange{Start: start, End: candidate.line + flowOperationRepairReadRadius},
					alreadyRead: closure.HasReadLine(file, candidate.line),
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

// flowOperationMissingSelectedResultConsumer detects the narrow gap between
// "a requested carrier reached a builder/factory" and "the returned local
// value reached an actual consumer".  Participant coverage alone cannot close
// that gap: it can be satisfied by the producer-side argument and assignment
// while leaving the consumer absent from the final relation component.
//
// The detector is deliberately conservative and language-neutral:
//   - the request must carry a required source-flow diagram;
//   - the assignment must already be citable/model-selected and share its exact
//     source line with a citable operation incident to a requested participant;
//   - its RHS must resolve to exactly one parser call on that line;
//   - a later call in the same parser-owned callable must consume the exact
//     whole LHS value (member projections do not count); and
//   - an already-grounded exact argument row closes the obligation.
//
// One bounded selected chain is followed at a time. Parser-owned handoff shape
// ranks a cross-receiver consumer ahead of a same-owner helper, with source
// order used only as the stable tie break. This does not assert runtime order,
// create evidence, or force the final diagram to draw an edge. Ambiguous call
// sites and unsupported assignment syntax fail closed.
func flowOperationMissingSelectedResultConsumer(
	ctx *types.BusContext,
	evidence []types.EvidenceItem,
) (flowValueConsumerRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil ||
		!flowOperationEvidenceRequired(ctx) {
		return flowValueConsumerRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.DiagramHint == nil || !rm.DiagramHint.Required || len(rm.DiagramHint.Participants) == 0 {
		return flowValueConsumerRepair{}, false
	}
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		return flowValueConsumerRepair{}, false
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return flowValueConsumerRepair{}, false
	}

	var participantGroups [][]string
	for _, participant := range flowOperationPlanningParticipants(rm) {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		if resolved := flowResolveParticipantIdentity(ctx, rm, participant); len(resolved.surfaces) > 0 {
			participantGroups = append(participantGroups, resolved.surfaces)
		}
	}
	if len(participantGroups) == 0 {
		return flowValueConsumerRepair{}, false
	}

	type producer struct {
		item     types.EvidenceItem
		receiver string
		value    string
		site     flowParserRelationSite
	}
	operations := types.FlowOperationEvidenceForRequest(evidence, rm)
	var producers []producer
	for _, item := range operations {
		if item.AnchorKind != types.AnchorAssignment && item.AnchorKind != types.AnchorInitializer {
			continue
		}
		receiver, value, ok := types.AssignmentEvidenceEndpoints(item)
		if !ok || receiver == "" || value == "" || item.LineStart <= 0 {
			continue
		}
		source := canonicalRelationSourcePath(item.Source)
		var matching []flowParserRelationSite
		for _, site := range index.relationsByFile[source] {
			relation := site.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" ||
				max(relation.Line, relation.FromEP.Line, relation.ToEP.Line) != item.LineStart ||
				!types.AnswerCodeIdentitySurfacesEquivalent(value, flowNavigationCallReceiver(relation)) {
				continue
			}
			matching = append(matching, site)
		}
		if len(matching) == 1 {
			producers = append(producers, producer{item: item, receiver: receiver, value: value, site: matching[0]})
		}
	}
	if len(producers) == 0 {
		return flowValueConsumerRepair{}, false
	}
	sort.SliceStable(producers, func(i, j int) bool {
		left, right := canonicalRelationSourcePath(producers[i].item.Source), canonicalRelationSourcePath(producers[j].item.Source)
		if left != right {
			return left < right
		}
		return producers[i].item.LineStart < producers[j].item.LineStart
	})

	participantIncidentAtLine := func(source string, line int) bool {
		for _, sibling := range evidence {
			if !sibling.IsCitable() || canonicalRelationSourcePath(sibling.Source) != source ||
				sibling.LineStart != line {
				continue
			}
			for _, group := range participantGroups {
				for _, endpoint := range []string{sibling.Subject, sibling.Object} {
					if diagramParticipantCandidateEndpointMatches(group, endpoint, sibling, evidence) {
						return true
					}
				}
			}
		}
		return false
	}

	// Select one bounded alternating assignment/consumer chain. The first
	// assignment must be incident to a requested participant. A later
	// assignment joins only when an authored whole-value argument on that same
	// line consumes a receiver already selected in the chain. This progresses
	// build(bus)->ctx, Execute(ctx)->output, Apply(output) without turning every
	// incidental call-result assignment in the callable into a checklist.
	selectedReceivers := make([]string, 0, flowNavigationCallResultContinuationMaxHops)
	selectedProducers := make([]producer, 0, flowNavigationCallResultContinuationMaxHops)
	selectedSource := ""
	var selectedOwnerSurfaces []string
	for _, candidate := range producers {
		source := canonicalRelationSourcePath(candidate.item.Source)
		selected := false
		if len(selectedProducers) == 0 {
			selected = participantIncidentAtLine(source, candidate.item.LineStart)
		} else if source == selectedSource &&
			flowRepairPlanningSurfaceMatchRank(selectedOwnerSurfaces, candidate.site.ownerSurfaces) > 0 {
			for _, sibling := range operations {
				if sibling.AnchorKind != types.AnchorArgument ||
					canonicalRelationSourcePath(sibling.Source) != source ||
					sibling.LineStart != candidate.item.LineStart {
					continue
				}
				for _, receiver := range selectedReceivers {
					if types.AnswerCodeIdentitySurfacesEquivalent(receiver, sibling.Subject) {
						selected = true
						break
					}
				}
				if selected {
					break
				}
			}
		}
		if !selected {
			continue
		}
		if len(selectedProducers) == 0 {
			selectedSource = source
			selectedOwnerSurfaces = append([]string(nil), candidate.site.ownerSurfaces...)
		}
		selectedProducers = append(selectedProducers, candidate)
		selectedReceivers = append(selectedReceivers, candidate.receiver)
		if len(selectedProducers) >= flowNavigationCallResultContinuationMaxHops {
			break
		}
	}
	if len(selectedProducers) == 0 {
		return flowValueConsumerRepair{}, false
	}
	lines := flowNavigationSourceLines(ctx, index, selectedSource)
	if len(lines) == 0 {
		return flowValueConsumerRepair{}, false
	}
	gc := &ground.Context{Graph: graph, RepoRoot: ctx.RepoRoot, LineIndex: map[string]map[int]string{selectedSource: lines}}
	type consumer struct {
		line        int
		argument    string
		receiver    string
		handoffRank int
	}
	for _, selected := range selectedProducers {
		var consumers []consumer
		seen := make(map[string]bool)
		for _, site := range index.relationsByFile[selectedSource] {
			relation := site.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" ||
				flowRepairPlanningSurfaceMatchRank(selected.site.ownerSurfaces, site.ownerSurfaces) == 0 {
				continue
			}
			line := max(relation.Line, relation.FromEP.Line, relation.ToEP.Line)
			if line <= selected.item.LineStart {
				continue
			}
			callee := flowNavigationCallReceiver(relation)
			if callee == "" {
				continue
			}
			for _, flow := range ground.DetectArgumentFlowsAtLine(gc, selectedSource, line, callee) {
				// Whole-value consumption only. A diagnostic read of local.Field is
				// real evidence but does not establish that the constructed carrier
				// itself reached the downstream API.
				if !types.AnswerCodeIdentitySurfacesEquivalent(selected.receiver, flow.Argument) {
					continue
				}
				key := strings.ToLower(strconv.Itoa(line) + "\x00" + flow.Argument + "\x00" + flow.Receiver)
				if !seen[key] {
					seen[key] = true
					consumers = append(consumers, consumer{
						line: line, argument: flow.Argument, receiver: flow.Receiver,
						handoffRank: flowNavigationCarrierHandoffRank(relation, selected.receiver),
					})
				}
			}
		}
		if len(consumers) == 0 {
			return flowValueConsumerRepair{}, false
		}
		sort.SliceStable(consumers, func(i, j int) bool {
			if consumers[i].handoffRank != consumers[j].handoffRank {
				return consumers[i].handoffRank > consumers[j].handoffRank
			}
			if consumers[i].line != consumers[j].line {
				return consumers[i].line < consumers[j].line
			}
			if consumers[i].receiver != consumers[j].receiver {
				return consumers[i].receiver < consumers[j].receiver
			}
			return consumers[i].argument < consumers[j].argument
		})
		selectedConsumer := consumers[0]
		// Multiple equally-ranked calls consuming the same value on one source
		// line are not a unique evidence coordinate.
		if len(consumers) > 1 && consumers[1].handoffRank == selectedConsumer.handoffRank &&
			consumers[1].line == selectedConsumer.line {
			return flowValueConsumerRepair{}, false
		}
		covered := false
		for _, item := range operations {
			if item.AnchorKind == types.AnchorArgument && canonicalRelationSourcePath(item.Source) == selectedSource &&
				item.LineStart == selectedConsumer.line && strings.TrimSpace(item.Subject) == selectedConsumer.argument &&
				types.AnswerCodeIdentitySurfacesEquivalent(item.Object, selectedConsumer.receiver) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		start := selectedConsumer.line - flowOperationRepairReadRadius
		if start < 1 {
			start = 1
		}
		return flowValueConsumerRepair{
			target: flowOperationRepairReadTarget{
				file: selectedSource, lineRange: types.LineRange{Start: start, End: selectedConsumer.line + flowOperationRepairReadRadius},
				focusIdentity: selectedConsumer.argument,
				alreadyRead:   ctx.Mutable.EvidenceClosure().HasReadLine(selectedSource, selectedConsumer.line),
			},
			argument: selectedConsumer.argument, receiver: selectedConsumer.receiver,
			producerSource: selectedSource, producerLine: selected.item.LineStart, consumerLine: selectedConsumer.line,
		}, true
	}
	return flowValueConsumerRepair{}, false
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

// flowNavigationRequestedParticipantConnectionGainRank is a SOFT frontier
// preference layered above the raw participant count.  Touching two requested
// participants is not always progress: both may already be in the same still-
// disconnected state-only island.  A candidate that touches at least one
// missing group and at least one already-covered group can join that island to
// the requested flow and is therefore the more useful next source coordinate.
//
// All inputs are parser/analyzer typed.  The rank never closes participant
// coverage, creates evidence, or requires the model to draw an edge.
func flowNavigationRequestedParticipantConnectionGainRank(
	relation *repotypes.Relation,
	binding flowDeclaredBindingSite,
	ownerSurfaces []string,
	participantSurfaceGroups [][]string,
	participantGroupMissing []bool,
) int {
	if relation == nil || len(participantSurfaceGroups) == 0 ||
		len(participantSurfaceGroups) != len(participantGroupMissing) {
		return 0
	}
	bindingSymbol := repotypes.Symbol{Name: binding.alias, DeclaredType: binding.declaredType}
	endpoints := append(
		flowRepairRelationEndpointSurfaces(relation.FromEP),
		flowRepairRelationEndpointSurfaces(relation.ToEP)...,
	)
	endpoints = append(endpoints, ownerSurfaces...)
	touchesMissing, touchesCovered := false, false
	for i, group := range participantSurfaceGroups {
		if len(group) == 0 {
			continue
		}
		touched := (binding.alias != "" && flowRepairSymbolMatchesAnySurface(bindingSymbol, group)) ||
			flowRepairPlanningSurfaceMatchRank(group, endpoints) > 0
		if !touched {
			continue
		}
		if participantGroupMissing[i] {
			touchesMissing = true
		} else {
			touchesCovered = true
		}
	}
	if touchesMissing && touchesCovered {
		return 1
	}
	return 0
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

func flowRepairAllDeclaredBindingSites(index *flowNavigationIndex, rm types.RequestModel, surfaces []string) []flowDeclaredBindingSite {
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
	return out
}

func flowRepairBoundDeclaredBindingSites(out []flowDeclaredBindingSite) []flowDeclaredBindingSite {
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

func flowRepairDeclaredBindingSites(index *flowNavigationIndex, rm types.RequestModel, surfaces []string) []flowDeclaredBindingSite {
	return flowRepairBoundDeclaredBindingSites(flowRepairAllDeclaredBindingSites(index, rm, surfaces))
}

// flowNavigationDeclaredBindingSitesForRepair applies the normal bounded
// binding budget only after parser-owned connection potential has been
// compared.  The ordinary declaration helper sorts lexically before applying
// its six-file cap, which is appropriate for a compact search roster but can
// starve a high-value owner when a common context type has many parameters or
// fields.  Here we inspect only graph metadata first: a binding whose owning
// callable has an operation incident to another requested participant, or
// hands the carrier to a distinct qualified receiver, is retained ahead of
// isolated local bindings.  Source text is still read only for the bounded
// survivors, and later model-authored evidence remains mandatory.
func flowNavigationDeclaredBindingSitesForRepair(
	index *flowNavigationIndex,
	graph *repotypes.Graph,
	rm types.RequestModel,
	currentSurfaces []string,
	participantSurfaceGroups [][]string,
	evidence []types.EvidenceItem,
) []flowDeclaredBindingSite {
	all := flowRepairAllDeclaredBindingSites(index, rm, currentSurfaces)
	if len(all) <= 1 {
		return all
	}
	type rankedBinding struct {
		site               flowDeclaredBindingSite
		groundedSourceRank int
		connectionRank     int
		externalCallRank   int
		bridgeRank         int
	}
	ranked := make([]rankedBinding, 0, len(all))
	for _, binding := range all {
		entry := rankedBinding{
			site: binding,
			groundedSourceRank: flowNavigationGroundedParticipantSourceRank(
				evidence, binding.file, currentSurfaces,
			),
		}
		bindingSymbol := repotypes.Symbol{Name: binding.alias, DeclaredType: binding.declaredType}
		for _, relationSite := range flowNavigationBindingRelationSites(index, graph, binding) {
			relation := relationSite.relation
			if relation == nil || strings.TrimSpace(relation.Kind) != "call" {
				continue
			}
			endpoints := append(
				flowRepairRelationEndpointSurfaces(relation.FromEP),
				flowRepairRelationEndpointSurfaces(relation.ToEP)...,
			)
			endpoints = append(endpoints, relationSite.ownerSurfaces...)
			for _, group := range participantSurfaceGroups {
				if len(group) == 0 || flowRepairSymbolMatchesAnySurface(bindingSymbol, group) {
					continue
				}
				if flowRepairPlanningSurfaceMatchRank(group, endpoints) > 0 {
					entry.connectionRank = 1
					break
				}
			}
			entry.externalCallRank = max(entry.externalCallRank,
				flowNavigationCarrierHandoffRank(relation, binding.alias))
			entry.bridgeRank = max(entry.bridgeRank, relationSite.carrierOwnerBridgeRank)
		}
		ranked = append(ranked, entry)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].connectionRank != ranked[j].connectionRank {
			return ranked[i].connectionRank > ranked[j].connectionRank
		}
		if ranked[i].groundedSourceRank != ranked[j].groundedSourceRank {
			return ranked[i].groundedSourceRank > ranked[j].groundedSourceRank
		}
		if ranked[i].externalCallRank != ranked[j].externalCallRank {
			return ranked[i].externalCallRank > ranked[j].externalCallRank
		}
		if ranked[i].bridgeRank != ranked[j].bridgeRank {
			return ranked[i].bridgeRank > ranked[j].bridgeRank
		}
		if ranked[i].site.file != ranked[j].site.file {
			return ranked[i].site.file < ranked[j].site.file
		}
		return ranked[i].site.alias < ranked[j].site.alias
	})
	ordered := make([]flowDeclaredBindingSite, 0, len(ranked))
	for _, entry := range ranked {
		ordered = append(ordered, entry.site)
	}
	return flowRepairBoundDeclaredBindingSites(ordered)
}

// flowNavigationGroundedParticipantSourceRank is a precise adaptive signal for
// the SOFT source navigator. A model-authored citable row must already come
// from the exact file and carry one of the typed still-missing participant
// surfaces. The rank only prevents that source from being discarded by a
// lexical file budget; it neither validates another operation in the file nor
// creates relation, diagram, or answer authority.
func flowNavigationGroundedParticipantSourceRank(
	evidence []types.EvidenceItem,
	file string,
	participantSurfaces []string,
) int {
	file = canonicalRelationSourcePath(file)
	if file == "" || len(participantSurfaces) == 0 {
		return 0
	}
	for _, item := range evidence {
		if !item.IsCitable() || canonicalRelationSourcePath(item.Source) != file {
			continue
		}
		if flowRepairItemMatchesAnySurface(item, participantSurfaces) {
			return 1
		}
	}
	return 0
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
