package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	entityProvenanceRoleSearch = "search"
	entityProvenanceRoleShape  = "shape"
)

// filterEntitiesByProvenance narrows an entity list for a specific
// consumer role. Nil/empty provenance is fail-open for legacy callers.
// Missing provenance for an individual entity is also kept: producers
// are allowed to be partial, but consumers must not silently erase an
// entity unless a matching provenance row explicitly marks it unusable
// for the requested role.
func filterEntitiesByProvenance(
	entities []string,
	provenance []types.EntityProvenance,
	role string,
) []string {
	if len(entities) == 0 || len(provenance) == 0 {
		return entities
	}
	allowByKey := make(map[string]bool, len(provenance))
	seenByKey := make(map[string]bool, len(provenance))
	for _, p := range provenance {
		key := entityProvenanceKey(p.Surface)
		if key == "" {
			continue
		}
		seenByKey[key] = true
		switch role {
		case entityProvenanceRoleShape:
			allowByKey[key] = allowByKey[key] || p.UseForShape
		default:
			allowByKey[key] = allowByKey[key] || p.UseForSearch
		}
	}
	if len(seenByKey) == 0 {
		return entities
	}
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		key := entityProvenanceKey(entity)
		if key == "" {
			continue
		}
		if !seenByKey[key] || allowByKey[key] {
			out = append(out, entity)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func entityProvenanceKey(surface string) string {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return ""
	}
	key := normalizer.NormalizeCodeKey(surface)
	if key != "" {
		return key
	}
	return strings.ToLower(surface)
}
