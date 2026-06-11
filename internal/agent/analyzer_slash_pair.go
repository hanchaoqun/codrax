package agent

import (
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
)

// slashPairRE matches a standalone identifier pair `A/B`. The
// look-around-free guards (no preceding/following slash, dot, or word
// character) keep multi-segment paths (`a/b/c`), dotted files
// (`pkg.go/x`), and URLs out of scope.
var slashPairRE = regexp.MustCompile(`(?:^|[^\w./])([A-Za-z_][A-Za-z0-9_]*)/([A-Za-z_][A-Za-z0-9_]*)(?:[^\w./]|$)`)

// completeSlashPairEntities returns entities plus the missing half of
// every slash-joined identifier pair in rawRequest where the OTHER
// half was already emitted. Deterministic mention-derived expansion
// (same tier as MentionedEntitiesFromRawRequest): it only completes
// pairs the user literally typed, only when the analyzer anchored one
// side, and only for CamelCase / snake_case shapes — mirroring the
// entities emit contract so plain path words never enter.
func completeSlashPairEntities(rawRequest string, entities []string) []string {
	if strings.TrimSpace(rawRequest) == "" || len(entities) == 0 || !strings.Contains(rawRequest, "/") {
		return entities
	}
	have := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		if k := normalizer.NormalizeCodeKey(e); k != "" {
			have[k] = struct{}{}
		}
	}
	const maxAdded = 4
	out := entities
	added := 0
	for _, m := range slashPairRE.FindAllStringSubmatch(rawRequest, -1) {
		if added >= maxAdded {
			break
		}
		a, b := m[1], m[2]
		ka, kb := normalizer.NormalizeCodeKey(a), normalizer.NormalizeCodeKey(b)
		if ka == "" || kb == "" || ka == kb {
			continue
		}
		_, hasA := have[ka]
		_, hasB := have[kb]
		if hasA == hasB {
			continue // both present or both absent — nothing anchors the pair
		}
		missing, missingKey := a, ka
		if hasA {
			missing, missingKey = b, kb
		}
		if !slashPairIdentifierShape(missing) {
			continue
		}
		out = append(out, missing)
		have[missingKey] = struct{}{}
		added++
	}
	return out
}

// slashPairIdentifierShape mirrors the entities emit contract:
// CamelCase (any uppercase letter) or snake_case (an interior
// underscore), at least two runes. Lowercase path words fail both.
func slashPairIdentifierShape(s string) bool {
	if len(s) < 2 {
		return false
	}
	if strings.ContainsRune(strings.Trim(s, "_"), '_') {
		return true
	}
	return strings.IndexFunc(s, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0
}
