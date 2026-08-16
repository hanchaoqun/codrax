package tool

import (
	"sort"
	"strings"
	"unicode"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	maxFlowOperationRepairFiles    = 6
	maxFlowOperationRepairKeywords = 8
	flowOperationRepairReadRadius  = 12
)

type flowOperationRepairReadTarget struct {
	file      string
	lineRange types.LineRange
}

// flowResolvedParticipantIdentity is a parser-owned navigation projection for
// one request-visible participant. Surfaces may close a hard *search*
// obligation, but never create relation authority; only a separately grounded
// operation can do that.
type flowResolvedParticipantIdentity struct {
	surfaces []string
	files    []string
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
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || len(graph.FileIndex) == 0 {
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
	paths := make([]string, 0, len(graph.FileIndex))
	for path := range graph.FileIndex {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !relationSourceInRequestedScope(path, rm) {
			continue
		}
		fi := graph.FileIndex[path]
		if fi == nil {
			continue
		}
		for _, symbol := range fi.Symbols {
			name := strings.TrimSpace(symbol.Name)
			parent := strings.TrimSpace(symbol.Parent)
			declaredType := strings.TrimSpace(symbol.DeclaredType)
			if name == "" || parent == "" || declaredType == "" ||
				!flowAnyIdentitySurfaceMatches(surfaces, name) ||
				!flowAnyIdentitySurfaceMatches(ownerSurfaces, parent) {
				continue
			}
			candidates = append(candidates, candidate{
				file: canonicalRelationSourcePath(path), name: name,
				parent: parent, declaredType: declaredType,
			})
		}
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
		files:    appendUniqueBounded(nil, []string{c.file}, maxFlowOperationRepairFiles),
	}
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
		surfaces = appendUniqueBounded(surfaces, resolved.surfaces, maxFlowOperationRepairKeywords)
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
// operation occurrence that has not yet been read. It is a navigation target
// only: callers may put the range on the lazy-read queue, but the occurrence
// never becomes EvidenceItem authority and never closes a participant or
// relation gate by itself. The explorer must still inspect the source and emit
// the exact syntax-owned operation through emit_evidence.
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
	if rm.DiagramHint != nil {
		for _, participant := range flowOperationPlanningParticipants(rm) {
			if participant.Role != types.DiagramParticipantIncidentRequired {
				continue
			}
			if len(wanted) > 0 && !wanted[strings.ToLower(strings.TrimSpace(participant.Identity))] {
				continue
			}
			resolved := flowResolveParticipantIdentity(ctx, rm, participant)
			surfaces = appendUniqueBounded(surfaces, resolved.surfaces, maxFlowOperationRepairKeywords)
		}
	}
	if len(surfaces) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || len(graph.FileIndex) == 0 {
		return flowOperationRepairReadTarget{}, false
	}
	paths := make([]string, 0, len(graph.FileIndex))
	for path := range graph.FileIndex {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	type rankedTarget struct {
		target    flowOperationRepairReadTarget
		matchRank int
		kindRank  int
		line      int
	}
	var candidates []rankedTarget
	closure := ctx.Mutable.EvidenceClosure()
	for _, path := range paths {
		if !relationSourceInRequestedScope(path, rm) {
			continue
		}
		fi := graph.FileIndex[path]
		if fi == nil {
			continue
		}
		for _, relation := range fi.Relations {
			switch relation.Kind {
			case "call", "reference", "type_usage":
			default:
				continue
			}
			from := flowRepairRelationEndpointSurfaces(relation.FromEP)
			to := flowRepairRelationEndpointSurfaces(relation.ToEP)
			matchRank := max(
				flowRepairPlanningSurfaceMatchRank(surfaces, from),
				flowRepairPlanningSurfaceMatchRank(surfaces, to),
			)
			if matchRank == 0 {
				continue
			}
			file := canonicalRelationSourcePath(firstNonEmptyFlowRepairString(relation.File, fi.RelPath, path))
			line := relation.Line
			if line <= 0 {
				line = relation.FromEP.Line
			}
			if line <= 0 {
				line = relation.ToEP.Line
			}
			if file == "" || line <= 0 || closure.HasReadLine(file, line) {
				continue
			}
			start := line - flowOperationRepairReadRadius
			if start < 1 {
				start = 1
			}
			candidates = append(candidates, rankedTarget{
				target: flowOperationRepairReadTarget{
					file:      file,
					lineRange: types.LineRange{Start: start, End: line + flowOperationRepairReadRadius},
				},
				matchRank: matchRank,
				kindRank:  flowOperationRepairRelationKindRank(relation.Kind),
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
		if candidates[i].kindRank != candidates[j].kindRank {
			return candidates[i].kindRank > candidates[j].kindRank
		}
		if candidates[i].target.file != candidates[j].target.file {
			return candidates[i].target.file < candidates[j].target.file
		}
		return candidates[i].line < candidates[j].line
	})
	return candidates[0].target, true
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
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || len(graph.FileIndex) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(graph.FileIndex))
	for path := range graph.FileIndex {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var files, aliases []string
	for _, path := range paths {
		if !relationSourceInRequestedScope(path, rm) {
			continue
		}
		fi := graph.FileIndex[path]
		if fi == nil {
			continue
		}
		for _, relation := range fi.Relations {
			switch relation.Kind {
			case "call", "reference", "type_usage":
			default:
				continue
			}
			from := flowRepairRelationEndpointSurfaces(relation.FromEP)
			to := flowRepairRelationEndpointSurfaces(relation.ToEP)
			if !flowRepairAnyPlanningSurfaceMatches(surfaces, from) &&
				!flowRepairAnyPlanningSurfaceMatches(surfaces, to) {
				continue
			}
			file := canonicalRelationSourcePath(firstNonEmptyFlowRepairString(relation.File, fi.RelPath, path))
			files = appendUniqueBounded(files, []string{file}, maxFlowOperationRepairFiles)
			aliases = appendUniqueBounded(aliases, append(from, to...), maxFlowOperationRepairKeywords)
			if len(files) >= maxFlowOperationRepairFiles && len(aliases) >= maxFlowOperationRepairKeywords {
				return files, aliases
			}
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
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || len(graph.FileIndex) == 0 {
		return nil, nil
	}
	files := make([]string, 0, maxFlowOperationRepairFiles)
	aliases := make([]string, 0, maxFlowOperationRepairKeywords)
	paths := make([]string, 0, len(graph.FileIndex))
	for path := range graph.FileIndex {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !relationSourceInRequestedScope(path, rm) {
			continue
		}
		fi := graph.FileIndex[path]
		if fi == nil {
			continue
		}
		for _, symbol := range fi.Symbols {
			if !flowRepairSymbolMatchesAnySurface(symbol, surfaces) {
				continue
			}
			files = appendUniqueBounded(files, []string{path}, maxFlowOperationRepairFiles)
			aliases = appendUniqueBounded(aliases, []string{symbol.Name}, maxFlowOperationRepairKeywords)
		}
	}
	return files, aliases
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
