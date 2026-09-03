package normalizer

import (
	"regexp"
	"strings"
)

// slashPairRE matches one standalone identifier pair A/B. The guards keep
// paths, URLs, and dotted file/member spellings out of this narrow carrier.
var slashPairRE = regexp.MustCompile(`(?:^|[^\w./])([A-Za-z_][A-Za-z0-9_]*)/([A-Za-z_][A-Za-z0-9_]*)(?:[^\w./]|$)`)

type slashPair struct{ left, right string }

// requestSlashPairs is the single authority for "the current request writes
// two actors as one slash-joined identifier pair". Both halves must be
// CamelCase or snake_case identifier shapes; repository paths
// (internal/agent) and lowercase word pairs (client/server) never qualify.
func requestSlashPairs(rawRequest string) []slashPair {
	if strings.TrimSpace(rawRequest) == "" || !strings.Contains(rawRequest, "/") {
		return nil
	}
	pairs := make([]slashPair, 0, 2)
	for _, match := range slashPairRE.FindAllStringSubmatch(rawRequest, -1) {
		left, right := match[1], match[2]
		leftKey, rightKey := NormalizeCodeKey(left), NormalizeCodeKey(right)
		if leftKey == "" || rightKey == "" || leftKey == rightKey ||
			!slashPairIdentifierShape(left) || !slashPairIdentifierShape(right) {
			continue
		}
		pairs = append(pairs, slashPair{left: left, right: right})
	}
	return pairs
}

// RequestSlashPairs enumerates the current request's identifier pairs as
// [left, right] in request spelling. Emit-side gates use it for the
// judgement-only completion lane (§40.21 ①): when the model's own relation
// scope names a user-typed pair and its rows cover exactly one half, the other
// half is an omitted participant — judged, never written into the roster.
func RequestSlashPairs(rawRequest string) [][2]string {
	pairs := requestSlashPairs(rawRequest)
	if len(pairs) == 0 {
		return nil
	}
	out := make([][2]string, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, [2]string{pair.left, pair.right})
	}
	return out
}

// RequestSlashPairHalves reports whether entity is, verbatim (case-
// insensitive, trimmed), one of the current request's identifier pairs and
// returns the two request-spelled halves. It is a pure predicate: it never
// rewrites, mints, or splits an entity roster. Emit-side gates use it so a
// model-emitted joined entity is judged as the model's own single claim
// (§40.21): the joined entity is covered when the model's own participant
// rows cover both halves, and the roster itself is never rewritten.
func RequestSlashPairHalves(rawRequest, entity string) (left, right string, ok bool) {
	entity = strings.TrimSpace(entity)
	if entity == "" || !strings.Contains(entity, "/") {
		return "", "", false
	}
	for _, candidate := range requestSlashPairs(rawRequest) {
		if strings.EqualFold(entity, candidate.left+"/"+candidate.right) {
			return candidate.left, candidate.right, true
		}
	}
	return "", "", false
}

// CompleteSlashPairEntities adds the missing half of an exact current-request
// A/B identifier pair when the analyzer already emitted the other half. It is
// the ONLY additive slash-pair lane and runs after the emit-side gates
// (agent/analyzer.go). It never splits a joined entity the model emitted:
// §40.21 retired the former CanonicalizeSlashPairEntities split because a
// hard gate must judge the model's original emission, not a rewritten one.
func CompleteSlashPairEntities(rawRequest string, entities []string) []string {
	if len(entities) == 0 {
		return entities
	}
	pairs := requestSlashPairs(rawRequest)
	if len(pairs) == 0 {
		return entities
	}
	working := append([]string(nil), entities...)
	have := make(map[string]struct{}, len(working))
	for _, entity := range working {
		if key := NormalizeCodeKey(entity); key != "" {
			have[key] = struct{}{}
		}
	}
	const maxAdded = 4
	added := 0
	for _, candidate := range pairs {
		if added >= maxAdded {
			break
		}
		leftKey, rightKey := NormalizeCodeKey(candidate.left), NormalizeCodeKey(candidate.right)
		_, hasLeft := have[leftKey]
		_, hasRight := have[rightKey]
		if hasLeft == hasRight {
			continue
		}
		missing, missingKey := candidate.left, leftKey
		if hasLeft {
			missing, missingKey = candidate.right, rightKey
		}
		working = append(working, missing)
		have[missingKey] = struct{}{}
		added++
	}
	return working
}

func slashPairIdentifierShape(value string) bool {
	if len(value) < 2 {
		return false
	}
	if strings.ContainsRune(strings.Trim(value, "_"), '_') {
		return true
	}
	return strings.IndexFunc(value, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0
}
