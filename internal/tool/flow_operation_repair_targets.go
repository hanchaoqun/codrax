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
)

// flowResolvedParticipantIdentity is a parser-owned navigation projection for
// one request-visible participant. Surfaces may close a hard *search*
// obligation, but never create relation authority; only a separately grounded
// operation can do that.
type flowResolvedParticipantIdentity struct {
	surfaces []string
	files    []string
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
	for _, owner := range rm.DiagramHint.Participants {
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
	var participants []types.DiagramParticipantHint
	if rm.DiagramHint != nil {
		participants = rm.DiagramHint.Participants
	}
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

	keywords := append([]string(nil), surfaces...)
	declaredFiles, declaredAliases := flowRepairDeclaredBindingTargets(ctx, rm, surfaces)
	keywords = appendUniqueBounded(keywords, declaredAliases, maxFlowOperationRepairKeywords)
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
	files = appendUniqueBounded(files, related, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, other, maxFlowOperationRepairFiles)
	files = appendUniqueBounded(files, completionMaterializationReadFiles(ctx.Mutable.EvidenceClosure()), maxFlowOperationRepairFiles)
	return files, keywords
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
