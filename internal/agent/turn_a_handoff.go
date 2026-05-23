package agent

import "github.com/hanchaoqun/codrax/internal/types"

// buildTurnAHandoffEvidence prepares the bounded evidence slice that Turn A
// stores for the extractor. Strict answer-chain terminals stay first because
// they are the evidence items used to compute TerminalEvidenceCount, while the
// already-ranked explorer evidence fills the remaining budget as support
// context. The count baseline is computed separately; adding support evidence
// here must never be interpreted as adding required answer-symbol members.
func buildTurnAHandoffEvidence(
	ctx *types.AgentContext,
	questionKind string,
	rankedEvidence []types.EvidenceItem,
	answerChains []types.AnswerChain,
) []types.EvidenceItem {
	if len(rankedEvidence) == 0 && len(answerChains) == 0 {
		return nil
	}

	limit := extractorMaxEvidence
	if limit <= 0 {
		limit = len(rankedEvidence) + len(answerChains)
	}
	out := make([]types.EvidenceItem, 0, extractorMinInt(limit, len(rankedEvidence)+len(answerChains)))
	seen := make(map[string]bool, limit)

	add := func(item types.EvidenceItem) bool {
		if len(out) >= limit {
			return false
		}
		id := item.ID
		if id == "" {
			id = types.StableEvidenceID(item)
			item.ID = id
		}
		if seen[id] {
			return true
		}
		seen[id] = true
		out = append(out, item)
		return true
	}

	for _, chain := range answerChains {
		if !chain.StrictOK {
			continue
		}
		if !add(chain.Item) {
			return out
		}
	}

	if len(out) == 0 && !shouldSeedTurnAStrictEvidenceFromRanked(ctx, questionKind) {
		return nil
	}
	for _, item := range rankedEvidence {
		if !add(item) {
			break
		}
	}
	return out
}
