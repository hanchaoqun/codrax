package agent

import "github.com/hanchaoqun/codrax/internal/analysis/normalizer"

// completeSlashPairEntities returns entities plus the missing half of
// every slash-joined identifier pair in rawRequest where the OTHER
// half was already emitted. Deterministic mention-derived expansion
// (same tier as MentionedEntitiesFromRawRequest): it only completes
// pairs the user literally typed, only when the analyzer anchored one
// side, and only for CamelCase / snake_case shapes — mirroring the
// entities emit contract so plain path words never enter.
func completeSlashPairEntities(rawRequest string, entities []string) []string {
	return normalizer.CompleteSlashPairEntities(rawRequest, entities)
}
