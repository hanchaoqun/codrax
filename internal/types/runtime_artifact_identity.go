package types

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// RuntimeArtifactIDForEvidenceItem returns the canonical runtime artifact ID
// for an EvidenceItem payload. Callers that need artifact lineage must use this
// helper instead of reimplementing evidence ID fallback rules.
func RuntimeArtifactIDForEvidenceItem(item EvidenceItem) string {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = StableEvidenceID(item)
	}
	return strings.TrimSpace(id)
}

// RuntimeArtifactIDForAnswerChain returns the canonical runtime artifact ID
// for an AnswerChain payload.
func RuntimeArtifactIDForAnswerChain(chain AnswerChain) string {
	id := RuntimeArtifactIDForEvidenceItem(chain.Item)
	if id == "" {
		return ""
	}
	return "answer_chain:" + id
}

// RuntimeArtifactIdentityForAggregateFact exposes the same aggregate-fact
// identity used by merge/dedup logic so artifact projection and readiness
// resolution cannot drift.
func RuntimeArtifactIdentityForAggregateFact(fact AnswerAggregateFact) string {
	return strings.TrimSpace(AnswerAggregateFactIdentity(fact))
}

// RuntimeArtifactIDForAggregateFact returns the canonical runtime artifact ID
// for an aggregate fact payload.
func RuntimeArtifactIDForAggregateFact(fact AnswerAggregateFact) string {
	return RuntimeArtifactIDForAggregateFactIdentity(RuntimeArtifactIdentityForAggregateFact(fact))
}

// RuntimeArtifactIDForAggregateFactIdentity returns the canonical runtime
// artifact ID for an already-normalized aggregate fact identity.
func RuntimeArtifactIDForAggregateFactIdentity(identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	return "aggregate_fact:" + RuntimeArtifactHashString(identity)
}

// RuntimeArtifactHashString is the stable short hash used inside runtime
// artifact IDs and content hashes. It is not a security primitive.
func RuntimeArtifactHashString(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}
