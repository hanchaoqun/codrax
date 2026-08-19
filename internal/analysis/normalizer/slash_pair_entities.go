package normalizer

import (
	"regexp"
	"strings"
)

// slashPairRE matches one standalone identifier pair A/B. The guards keep
// paths, URLs, and dotted file/member spellings out of this narrow carrier.
var slashPairRE = regexp.MustCompile(`(?:^|[^\w./])([A-Za-z_][A-Za-z0-9_]*)/([A-Za-z_][A-Za-z0-9_]*)(?:[^\w./]|$)`)

// CompleteSlashPairEntities adds the missing half of an exact current-request
// A/B identifier pair when the analyzer already emitted the other half.
func CompleteSlashPairEntities(rawRequest string, entities []string) []string {
	return normalizeSlashPairEntities(rawRequest, entities, false)
}

// CanonicalizeSlashPairEntities additionally replaces an analyzer-emitted
// joined A/B entity with the two verbatim current-request identities. It is
// used before required source-flow participant provenance validation so a
// model cannot collapse two participant obligations by collapsing its entity
// roster in the same tool call. It creates no participant and no relation.
func CanonicalizeSlashPairEntities(rawRequest string, entities []string) []string {
	return normalizeSlashPairEntities(rawRequest, entities, true)
}

func normalizeSlashPairEntities(rawRequest string, entities []string, canonicalizeJoined bool) []string {
	if strings.TrimSpace(rawRequest) == "" || len(entities) == 0 || !strings.Contains(rawRequest, "/") {
		return entities
	}

	type pair struct{ left, right string }
	pairs := make([]pair, 0, 2)
	for _, match := range slashPairRE.FindAllStringSubmatch(rawRequest, -1) {
		left, right := match[1], match[2]
		leftKey, rightKey := NormalizeCodeKey(left), NormalizeCodeKey(right)
		if leftKey == "" || rightKey == "" || leftKey == rightKey ||
			!slashPairIdentifierShape(left) || !slashPairIdentifierShape(right) {
			continue
		}
		pairs = append(pairs, pair{left: left, right: right})
	}
	if len(pairs) == 0 {
		return entities
	}

	working := append([]string(nil), entities...)
	if canonicalizeJoined {
		replaced := make([]string, 0, len(working)+2)
		for _, entity := range working {
			matched := false
			for _, candidate := range pairs {
				if !strings.EqualFold(strings.TrimSpace(entity), candidate.left+"/"+candidate.right) {
					continue
				}
				replaced = appendEntityIfMissing(replaced, candidate.left)
				replaced = appendEntityIfMissing(replaced, candidate.right)
				matched = true
				break
			}
			if !matched {
				replaced = appendEntityIfMissing(replaced, entity)
			}
		}
		working = replaced
	}

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

func appendEntityIfMissing(entities []string, candidate string) []string {
	key := NormalizeCodeKey(candidate)
	for _, existing := range entities {
		if NormalizeCodeKey(existing) == key {
			return entities
		}
	}
	return append(entities, candidate)
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
