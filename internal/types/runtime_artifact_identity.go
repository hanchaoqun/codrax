package types

import (
	"fmt"
	"hash/fnv"
	"strconv"
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

// RuntimeArtifactIdentityForAnalysisRefinementHandoff returns the typed,
// prose-free identity for a compiler-authored AnalyzeRefine handoff. The
// payload remains the current ProgressDecision carrier; the artifact ledger
// stores only this stable lineage ref.
func RuntimeArtifactIdentityForAnalysisRefinementHandoff(producerNodeID string, decision ProgressDecision) string {
	producerNodeID = strings.TrimSpace(producerNodeID)
	if producerNodeID == "" || progressDecisionEmpty(decision) {
		return ""
	}
	delta := decision.Delta
	parts := []string{
		producerNodeID,
		strconv.FormatBool(decision.ShouldReplan),
		strings.TrimSpace(decision.ReasonCode),
		strings.TrimSpace(string(delta.Kind)),
		strings.TrimSpace(string(delta.DowngradeLane)),
		strconv.FormatUint(uint64(delta.BlockerKey), 10),
		strconv.Itoa(delta.Consecutive),
		strconv.Itoa(delta.HardThreshold),
	}
	return strings.Join(parts, "\x1f")
}

// RuntimeArtifactIDForAnalysisRefinementHandoff returns the stable ledger ID
// for a typed AnalyzeRefine handoff. It hashes the identity so rendered user
// text or prompt fragments cannot leak into artifact refs.
func RuntimeArtifactIDForAnalysisRefinementHandoff(producerNodeID string, decision ProgressDecision) string {
	identity := RuntimeArtifactIdentityForAnalysisRefinementHandoff(producerNodeID, decision)
	if identity == "" {
		return ""
	}
	return "analysis_refinement_handoff:" + RuntimeArtifactHashString(identity)
}

func RuntimeArtifactHashForProgressDecision(decision ProgressDecision) string {
	identity := RuntimeArtifactIdentityForAnalysisRefinementHandoff("progress_decision", decision)
	if identity == "" {
		return ""
	}
	return RuntimeArtifactHashString(identity)
}

func progressDecisionEmpty(decision ProgressDecision) bool {
	delta := decision.Delta
	return !decision.ShouldReplan &&
		strings.TrimSpace(decision.ReasonCode) == "" &&
		strings.TrimSpace(string(delta.Kind)) == "" &&
		strings.TrimSpace(string(delta.DowngradeLane)) == "" &&
		delta.BlockerKey == 0 &&
		delta.Consecutive == 0 &&
		delta.HardThreshold == 0
}

// RuntimeArtifactHashString is the stable short hash used inside runtime
// artifact IDs and content hashes. It is not a security primitive.
func RuntimeArtifactHashString(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}
