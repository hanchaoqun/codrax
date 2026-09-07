package tool

import (
	"go/token"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Reuse the repository's separator/receiver grammar, but never its flat/case-
// folding existence matcher: location recovery must preserve exact owners.
func answerSymbolIdentitySegments(name string) []string {
	return rmtypes.SplitQualifiedSegments(name)
}

// The generic line grounder recognizes a method's leaf on a declaration line.
// That is useful corroboration, not authority to replace a declared owner or
// relocate a qualified symbol. Keep this stronger identity boundary local to
// answer-symbol emission; callers of the generic grounder are unchanged.
func resolveAnswerSymbolLineAnchor(gc *ground.Context, candidates map[string][]answerSymbolGroundedCandidate, file string, line int, name string, radius int) (int, bool, bool) {
	matchedLine, ok, negative := ground.ResolveSymbolLineAnchor(gc, file, line, name, radius)
	if !ok || len(answerSymbolIdentitySegments(name)) < 2 {
		return matchedLine, ok, negative
	}
	sameIdentity, differentOwner := answerSymbolParserLineIdentity(gc, file, matchedLine, name)
	if differentOwner {
		return 0, false, true
	}
	if matchedLine == line || sameIdentity {
		return matchedLine, true, false
	}
	if candidate, found := lookupAnswerSymbolGroundedCandidate(candidates, file, name); found && candidate.Line == matchedLine {
		return matchedLine, true, false
	}
	// A missing owner is unknown, not a reason to reject the model's original
	// location. It also does not prove that a same-leaf neighbour is that entity.
	return 0, false, false
}

// Only a unique, fully scoped parser declaration can contradict an owner.
// A missing scope, more than one owner, another source, or another declaration
// line stays unknown. Complete equivalent parser spellings are positive proof.
func answerSymbolParserLineIdentity(gc *ground.Context, file string, line int, name string) (sameIdentity, differentOwner bool) {
	requested := answerSymbolIdentitySegments(name)
	if len(requested) < 2 {
		return false, false
	}
	_, fi, graphSource, _, ok := ground.ResolveSourceGraphFile(gc, file)
	if !ok {
		return false, false
	}
	key := strings.Join(requested, "\x1f")
	owners := map[string][]string{}
	unknown := false
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		segments := answerSymbolIdentitySegments(sym.Name)
		if sym.Line != line || len(segments) == 0 || segments[len(segments)-1] != requested[len(requested)-1] ||
			(sym.File != "" && canonicalAnswerSymbolCandidateFile(sym.File) != canonicalAnswerSymbolCandidateFile(graphSource)) {
			continue
		}
		aliases := answerSymbolParserDeclarationNames(fi, sym)
		var identity []string
		for _, alias := range aliases {
			parts := answerSymbolIdentitySegments(alias)
			if len(parts) < 2 {
				continue
			}
			aliasKey := strings.Join(parts, "\x1f")
			if len(parts) > len(identity) {
				identity = parts
			}
			if aliasKey == key {
				sameIdentity = true
			}
		}
		if len(identity) == 0 {
			unknown = true
		} else {
			owners[strings.Join(identity, "\x1f")] = identity
		}
	}
	if !sameIdentity && !unknown && len(owners) == 1 {
		for _, owner := range owners {
			if !answerSymbolSimpleOwnerSegments(requested) || !answerSymbolSimpleOwnerSegments(owner) {
				return false, false
			}
			// A local parser parent A cannot contradict ns.A: the missing
			// namespace is unknown. Only overlapping, explicitly declared
			// owner segments can disagree; suffix compatibility grants no
			// alias and cannot authorize a location change.
			for offset := 2; offset <= len(requested) && offset <= len(owner); offset++ {
				if requested[len(requested)-offset] != owner[len(owner)-offset] {
					return false, true
				}
			}
		}
	}
	return sameIdentity, false
}

// Separator/receiver decomposition does not interpret generic type arguments,
// templates or other compound owner expressions. Those forms cannot become
// negative identity proof from unequal display strings. This is deliberately
// a conservative syntax subset, not a language-specific identity normalizer.
func answerSymbolSimpleOwnerSegments(parts []string) bool {
	for _, part := range parts[:len(parts)-1] {
		if !token.IsIdentifier(part) {
			return false
		}
	}
	return true
}

// A bare evidence spelling may gain a qualified lookup key only from the
// parser's declaration at that very source/line. These are internal lookup
// names, not permission to rewrite the model's chosen spelling or conclusion.
func answerSymbolCandidateParserIdentities(gc *ground.Context, candidate answerSymbolGroundedCandidate) (answerSymbolGroundedCandidate, []string) {
	original := []string{candidate.Name}
	_, fi, graphSource, _, ok := ground.ResolveSourceGraphFile(gc, candidate.File)
	if !ok {
		return candidate, original
	}
	keys := answerSymbolCandidateKeys(candidate.Name)
	if len(keys) == 0 {
		return candidate, original
	}
	key := keys[0]
	var names []string
	var matchedIdentity string
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if sym.Line != candidate.Line || (sym.File != "" && canonicalAnswerSymbolCandidateFile(sym.File) != canonicalAnswerSymbolCandidateFile(graphSource)) {
			continue
		}
		aliases := answerSymbolParserDeclarationNames(fi, sym)
		candidateSegments := answerSymbolIdentitySegments(candidate.Name)
		symbolSegments := answerSymbolIdentitySegments(sym.Name)
		matches := len(candidateSegments) == 1 && len(symbolSegments) > 0 && candidateSegments[0] == symbolSegments[len(symbolSegments)-1]
		for _, alias := range aliases {
			if keys := answerSymbolCandidateKeys(alias); len(keys) > 0 && keys[0] == key {
				matches = true
				break
			}
		}
		if !matches || len(aliases) == 0 {
			continue
		}
		identityKeys := answerSymbolCandidateKeys(aliases[0])
		if len(identityKeys) == 0 {
			continue
		}
		identity := identityKeys[0]
		if matchedIdentity != "" && matchedIdentity != identity {
			// More than one owner declares the same bare name on this line.
			return candidate, original
		}
		matchedIdentity = identity
		names = aliases
	}
	if len(names) == 0 {
		return candidate, original
	}
	// Co-referent bare/qualified evidence carriers now share one candidate;
	// the emitted AnswerSymbol still retains the model's original Name.
	candidate.Name = names[0]
	return candidate, append(original, names...)
}

func answerSymbolParserDeclarationNames(fi *rmtypes.FileInfo, sym *rmtypes.Symbol) []string {
	if fi == nil || sym == nil || strings.TrimSpace(sym.Name) == "" {
		return nil
	}
	if sym.Kind == "method" && len(answerSymbolIdentitySegments(sym.Name)) == 1 &&
		strings.TrimSpace(sym.Receiver) == "" && strings.TrimSpace(sym.Parent) == "" {
		// A file package is not the missing declaring type of a method.
		// Retain the known bare spelling without manufacturing Package.Method.
		return []string{sym.Name}
	}
	name := qualifiedEvidenceSymbolNameInFile(fi, sym)
	local := qualifiedEvidenceSymbolName(sym)
	// Some extractors already retain a full scoped Name. Do not prefix its
	// owner a second time and thereby manufacture a new qualified identity.
	if len(answerSymbolIdentitySegments(sym.Name)) > 1 {
		name, local = sym.Name, sym.Name
	}
	names := []string{name, local, sym.Name}
	if pkg := strings.TrimSpace(fi.Package); pkg != "" {
		pkgKey := strings.Join(answerSymbolIdentitySegments(pkg), "\x1f")
		localKey := strings.Join(answerSymbolIdentitySegments(local), "\x1f")
		if pkgKey != "" && localKey != "" && !strings.HasPrefix(localKey, pkgKey+"\x1f") {
			names = append(names, pkg+"."+local)
		}
	}
	return names
}
